package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/bootstrap"
	"github.com/FernasFragas/Nandocode/internal/config"
	"github.com/FernasFragas/Nandocode/internal/contextpack"
	"github.com/FernasFragas/Nandocode/internal/credentials"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/llm/modelresolver"
	"github.com/FernasFragas/Nandocode/internal/llm/modelruntime"
	"github.com/FernasFragas/Nandocode/internal/llm/ollama"
	"github.com/FernasFragas/Nandocode/internal/mentions"
	"github.com/FernasFragas/Nandocode/internal/observability"
	"github.com/FernasFragas/Nandocode/internal/retrievalroute"
	"github.com/FernasFragas/Nandocode/internal/tools"
	"github.com/FernasFragas/Nandocode/internal/tools/builtin"
	"github.com/spf13/cobra"
)

type printOptions struct {
	input                     string
	jsonOutput                bool
	model                     string
	ollamaURL                 string
	numCtx                    int
	llmStreamIdleTimeout      string
	cloudLLMStreamIdleTimeout string
}

type exitCoder interface {
	ExitCode() int
}

type cliExitError struct {
	code int
	err  error
}

func (e *cliExitError) Error() string {
	return e.err.Error()
}

func (e *cliExitError) Unwrap() error {
	return e.err
}

func (e *cliExitError) ExitCode() int {
	return e.code
}

func runPrint(ctx context.Context, cmd *cobra.Command, opts printOptions) error {
	if strings.TrimSpace(opts.input) == "" {
		return errors.New("--print requires non-empty input")
	}
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to resolve working directory: %w", err)
	}
	initial := bootstrap.DefaultInitial(wd)
	initial.LLMProvider = string(llm.ProviderOllamaLocal)
	initial.LLMBaseURL = initial.OllamaBaseURL
	if opts.numCtx > 0 {
		initial.NumCtx = opts.numCtx
	}
	modelOverride := ptrOrNil(opts.model)
	urlOverride := ptrOrNil(opts.ollamaURL)
	cfgRes, loadErr := config.Load(
		initial.ConfigDir+"/config.toml",
		initial.WorkingDir+"/.nandocodego/config.toml",
		config.FlagOverrides{
			Model:                     modelOverride,
			OllamaURL:                 urlOverride,
			LLMStreamIdleTimeout:      ptrOrNil(opts.llmStreamIdleTimeout),
			CloudLLMStreamIdleTimeout: ptrOrNil(opts.cloudLLMStreamIdleTimeout),
		},
	)
	if loadErr == nil {
		for _, warning := range cfgRes.Warnings {
			fmt.Fprintln(cmd.ErrOrStderr(), "Config warning:", warning)
		}
		initial.DefaultModel = cfgRes.Config.DefaultModel
		initial.OllamaBaseURL = cfgRes.Config.OllamaBaseURL
		initial.LLMBaseURL = cfgRes.Config.OllamaBaseURL
		initial.OllamaCloudEnabled = cfgRes.Config.OllamaCloudEnabled
		initial.KeepAlive = cfgRes.Config.ChatKeepAlive
		initial.LLMStreamIdleTimeout = cfgRes.Config.LLMStreamIdleTimeout
		initial.CloudLLMStreamIdleTimeout = cfgRes.Config.CloudLLMStreamIdleTimeout
		initial.MaxTurns = cfgRes.Config.MaxTurns
		initial.MaxConcurrentTools = cfgRes.Config.MaxConcurrentTools
		initial.BashTimeout = cfgRes.Config.BashTimeout
		initial.MaxReadChars = cfgRes.Config.MaxReadChars
		initial.MaxResultChars = cfgRes.Config.MaxResultChars
		initial.MaxDirFiles = cfgRes.Config.MaxDirFiles
		initial.MaxPromptFiles = cfgRes.Config.MaxPromptFiles
		initial.MaxDirBytes = cfgRes.Config.MaxDirBytes
		initial.MaxPromptBytes = cfgRes.Config.MaxPromptBytes
		initial.MaxDirDepth = cfgRes.Config.MaxDirDepth
		initial.MentionDirectorySource = cfgRes.Config.MentionDirectorySource
		initial.MentionIncludeGitignoredOnExplicit = cfgRes.Config.MentionIncludeGitignoredOnExplicit
		initial.PromptDumpMode = cfgRes.Config.PromptDumpMode
		initial.PromptDumpKeep = cfgRes.Config.PromptDumpKeep
		initial.PromptPreviewChars = cfgRes.Config.PromptPreviewChars
		initial.PermissionMode = cfgRes.Config.PermissionMode
		initial.ContextMode = cfgRes.Config.ContextMode
		initial.MemoryRecallMode = cfgRes.Config.MemoryRecallMode
	} else {
		return &cliExitError{code: 3, err: fmt.Errorf("failed to load config: %w", loadErr)}
	}
	toolCtx := tools.DefaultContext(ctx, initial.WorkingDir)
	toolCtx.MaxReadChars = initial.MaxReadChars
	toolCtx.MaxResultChars = initial.MaxResultChars
	toolCtx.MaxDirFiles = initial.MaxDirFiles
	toolCtx.MaxPromptFiles = initial.MaxPromptFiles
	toolCtx.MaxDirBytes = initial.MaxDirBytes
	toolCtx.MaxPromptBytes = initial.MaxPromptBytes
	toolCtx.MaxDirDepth = initial.MaxDirDepth
	toolCtx.MentionDirectorySource = initial.MentionDirectorySource
	toolCtx.MentionIncludeGitignoredOnExplicit = initial.MentionIncludeGitignoredOnExplicit
	toolCtx.PromptDumpMode = initial.PromptDumpMode
	toolCtx.PromptDumpKeep = initial.PromptDumpKeep
	toolCtx.PromptPreviewChars = initial.PromptPreviewChars
	toolCtx.BashTimeout = initial.BashTimeout
	agentCfg := agent.Config{
		MaxTurns:           initial.MaxTurns,
		MaxConcurrentTools: initial.MaxConcurrentTools,
		MaxOutputTokens:    initial.MaxOutputTokens,
		LengthRetryTokens:  initial.LengthRetryTokens,
		ChatKeepAlive:      initial.KeepAlive,
		NumCtx:             initial.NumCtx,
		Watchdog:           llm.WithIdleTimeout(llm.DefaultWatchdogConfig(), initial.LLMStreamIdleTimeout),
		CloudWatchdog:      llm.WithIdleTimeout(llm.DefaultCloudWatchdogConfig(), initial.CloudLLMStreamIdleTimeout),
		ContextMode:        initial.ContextMode,
		ContextMinNumCtx:   8192,
		ContextMaxNumCtx:   0,
		ContextReserve:     4096,
	}
	localClient := ollama.NewClient(initial.OllamaBaseURL)
	runtimeClient := llm.NewRuntimeClient(localClient, llm.ProviderOllamaLocal, initial.OllamaBaseURL)
	modelRuntimeSvc := &modelruntime.Service{
		LocalClient:  localClient,
		LocalBaseURL: initial.OllamaBaseURL,
		Runtime:      runtimeClient,
		Resolver: &modelresolver.Resolver{
			LocalClient:  localClient,
			CloudClient:  ollama.NewClient(llm.OllamaCloudBaseURL),
			CloudEnabled: initial.OllamaCloudEnabled,
		},
		Creds: credentials.NewResolver(),
	}
	switchRes, err := modelRuntimeSvc.Switch(ctx, modelruntime.SwitchOptions{
		RequestedModel: initial.DefaultModel,
		AllowPrompt:    false,
	})
	if err != nil {
		if errors.Is(err, modelruntime.ErrCredentialRequired) {
			return &cliExitError{code: 2, err: errors.New("cloud model requires OLLAMA_API_KEY or keychain credential in --print mode")}
		}
		return &cliExitError{code: 2, err: fmt.Errorf("failed to resolve model %q: %w", initial.DefaultModel, err)}
	}
	initial.DefaultModel = switchRes.Resolved.Model
	initial.LLMProvider = string(switchRes.Resolved.Provider)
	initial.LLMBaseURL = switchRes.Resolved.BaseURL
	if strings.TrimSpace(initial.LLMBaseURL) == "" {
		initial.LLMBaseURL = initial.OllamaBaseURL
	}

	in, err := buildPrintInput(opts.input, toolCtx, agentCfg, initial.DefaultModel, initial.ContextMode, initial.MaxOutputTokens)
	if err != nil {
		var tooLarge contextpack.ErrEvidenceTooLarge
		if errors.As(err, &tooLarge) {
			return &cliExitError{code: 2, err: err}
		}
		return &cliExitError{code: 2, err: err}
	}

	meter := observability.NewMeter()
	bridge, _ := observability.NewBridgeFromEnv(nil)
	defer bridge.Shutdown(ctx)
	client := observability.WrapLLMClient(runtimeClient, meter, bridge)
	registry, err := builtin.NewRegistry()
	if err != nil {
		return err
	}
	registry, err = observability.WrapRegistry(registry, meter, bridge)
	if err != nil {
		return err
	}
	agentCfg.PermissionObserver = observability.PermissionObserver(meter, bridge)
	agentCfg.ToolBatchObserver = func(batchSize int, safe bool, duration time.Duration) {
		meter.RecordToolBatch(batchSize, safe, duration)
		bridge.RecordToolBatch(batchSize, safe, duration)
	}
	runner, err := agent.New(client, registry, agent.WithConfig(agentCfg))
	if err != nil {
		return err
	}

	in.Model = initial.DefaultModel
	in.LLMProvider = initial.LLMProvider
	in.ContextMode = initial.ContextMode
	in.ToolContext = toolCtx
	content, toolUses, warnings, term, err := collectPrintOutput(runner.Run(ctx, in))
	if err != nil {
		return err
	}
	for _, msg := range warnings {
		fmt.Fprintln(cmd.ErrOrStderr(), msg)
	}
	if term.Reason != agent.TerminalCompleted {
		return &cliExitError{
			code: codeForTerminalReason(term.Reason),
			err:  fmt.Errorf("print mode failed: %s %s", term.Reason, term.Detail),
		}
	}
	return writePrintOutput(cmd.OutOrStdout(), content, toolUses, term, opts.jsonOutput)
}

func buildPrintInput(input string, toolCtx tools.Context, cfg agent.Config, model, contextMode string, maxOutputTokens int) (agent.Input, error) {
	packed, _, err := contextpack.BuildCurrentTurnPrompt(input, toolCtx, cfg, agent.Input{
		Model:           model,
		ContextMode:     contextMode,
		MaxOutputTokens: maxOutputTokens,
	}, nil)
	if err != nil {
		return agent.Input{}, err
	}
	currentTurnPaths := make([]string, 0, len(packed.Files))
	for _, f := range packed.Files {
		if strings.TrimSpace(f.Path) == "" {
			continue
		}
		currentTurnPaths = append(currentTurnPaths, f.Path)
	}
	currentTurnDirs := make([]string, 0, len(packed.Dirs))
	for _, d := range packed.Dirs {
		if strings.TrimSpace(d.Path) == "" {
			continue
		}
		currentTurnDirs = append(currentTurnDirs, d.Path)
	}
	// Print mode does not run semantic retrieval itself, but it should still
	// inherit the same cheap-prompt tool-mode decision used by interactive flows.
	routeDecision := retrievalroute.Decide(retrievalroute.Input{
		RawPrompt:            input,
		ShouldQuery:          true,
		AttachmentPolicy:     string(packed.ExpansionReport.Intent.AttachmentPolicy),
		CurrentTurnPaths:     currentTurnPaths,
		CurrentTurnDirs:      currentTurnDirs,
		AttachedFileCount:    len(packed.Files),
		AttachedContextBytes: len(packed.Prompt),
		IndexKnown:           false,
		HasIndex:             false,
		IndexCompatible:      false,
		SemanticEnabled:      true,
		SemanticMode:         "auto",
		PromptIntent:         string(packed.ExpansionReport.Intent.Kind),
	}, retrievalroute.Config{Mode: "auto"})
	historyPolicy := agent.HistoryPolicyDefault
	if packed.ExpansionReport.Intent.AttachmentPolicy == mentions.AttachListingTreeOnly ||
		packed.ExpansionReport.Intent.Kind == mentions.IntentFileStatus {
		historyPolicy = agent.HistoryPolicyLatestOnly
	}
	evidencePack := packed.PackReport
	return agent.Input{
		Model:            model,
		ContextMode:      contextMode,
		PromptIntent:     string(packed.ExpansionReport.Intent.Kind),
		AttachmentPolicy: string(packed.ExpansionReport.Intent.AttachmentPolicy),
		OriginalUserText: input,
		HistoryPolicy:    historyPolicy,
		ToolMode:         string(routeDecision.ToolMode),
		RouteAction:      string(routeDecision.Action),
		RouteReason:      string(routeDecision.Reason),
		RouteProfile:     routeDecision.RequestProfile,
		ToolContext:      toolCtx,
		EvidencePack:     &evidencePack,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: packed.Prompt},
		},
	}, nil
}

func ptrOrNil(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	v := s
	return &v
}

func collectPrintOutput(events <-chan agent.Event) (string, []map[string]any, []string, agent.Terminal, error) {
	var content strings.Builder
	toolUses := make([]map[string]any, 0, 8)
	warnings := make([]string, 0, 2)
	var term agent.Terminal
	seenTerminal := false
	for evt := range events {
		switch e := evt.(type) {
		case agent.AssistantTextDelta:
			content.WriteString(e.Content)
		case agent.ToolUseStart:
			toolUses = append(toolUses, map[string]any{"id": e.ID, "name": e.Name, "input": e.Input})
		case agent.ToolUseResult:
			toolUses = append(toolUses, map[string]any{"id": e.ID, "ok": e.Err == nil, "output": e.Result.Display})
		case agent.LLMIdleWarning:
			msg := "[Still waiting for model stream"
			if strings.TrimSpace(e.Provider) != "" {
				msg += " (" + strings.TrimSpace(e.Provider) + ")"
			}
			msg += fmt.Sprintf("; idle %s]", e.Timeout)
			warnings = append(warnings, msg)
		case agent.Terminal:
			term = e
			seenTerminal = true
		}
	}
	if !seenTerminal {
		return "", nil, nil, agent.Terminal{}, errors.New("print mode did not receive terminal event")
	}
	return content.String(), toolUses, warnings, term, nil
}

func writePrintOutput(w io.Writer, content string, toolUses []map[string]any, term agent.Terminal, jsonOutput bool) error {
	if jsonOutput {
		out := map[string]any{
			"content":   content,
			"tool_uses": toolUses,
			"usage":     term.Usage,
		}
		enc := json.NewEncoder(w)
		return enc.Encode(out)
	}
	_, err := fmt.Fprintln(w, content)
	return err
}

func codeForTerminalReason(reason agent.TerminalReason) int {
	switch reason {
	case agent.TerminalUnrecoverable:
		return 2
	case agent.TerminalAborted:
		return 1
	default:
		return 1
	}
}
