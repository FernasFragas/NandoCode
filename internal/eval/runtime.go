package eval

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/bootstrap"
	"github.com/FernasFragas/Nandocode/internal/config"
	"github.com/FernasFragas/Nandocode/internal/credentials"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/llm/modelresolver"
	"github.com/FernasFragas/Nandocode/internal/llm/modelruntime"
	"github.com/FernasFragas/Nandocode/internal/llm/ollama"
	"github.com/FernasFragas/Nandocode/internal/permissions"
	"github.com/FernasFragas/Nandocode/internal/tools"
	"github.com/FernasFragas/Nandocode/internal/tools/builtin"
)

const evaluatorSystemPrompt = `You are evaluating whether you can complete a coding task correctly.

Work only inside the provided workspace. Read files before modifying them. Run tests when the task asks for them. Stop after the task is complete and summarize what you changed and what tests you ran.`

type runtimeBundle struct {
	model    string
	provider Provider
	client   llm.Client
	registry *tools.Registry
	agentCfg agent.Config
	// recorded is set for the recorded provider so replay errors can be
	// reported as infrastructure errors after the run.
	recorded *RecordedClient
}

func buildRuntime(ctx context.Context, fixture Fixture, workspaceDir string, opts RunOptions) (runtimeBundle, error) {
	registry, err := builtin.NewRegistry()
	if err != nil {
		return runtimeBundle{}, err
	}
	cfg := agent.DefaultConfig()
	cfg.MaxTurns = fixture.Config.Execution.MaxTurns
	cfg.ContextMode = "auto"

	switch opts.Provider {
	case ProviderRecorded, "":
		recordingName := fixture.Config.Model.Recorded.Recording
		if strings.TrimSpace(opts.Recording) != "" {
			recordingName = opts.Recording
		}
		recording, err := LoadRecording(fixture.Root, recordingName)
		if err != nil {
			return runtimeBundle{}, fmt.Errorf("load recording %q: %w", recordingName, err)
		}
		client := NewRecordedClient(recording)
		return runtimeBundle{
			model:    recording.Model,
			provider: ProviderRecorded,
			client:   client,
			registry: registry,
			agentCfg: cfg,
			recorded: client,
		}, nil
	case ProviderLive:
		model, client, liveCfg, err := buildLiveClient(ctx, opts)
		if err != nil {
			return runtimeBundle{}, err
		}
		cfg.ChatKeepAlive = liveCfg.KeepAlive
		cfg.NumCtx = liveCfg.NumCtx
		cfg.Watchdog = llm.WithIdleTimeout(llm.DefaultWatchdogConfig(), liveCfg.LLMStreamIdleTimeout)
		cfg.CloudWatchdog = llm.WithIdleTimeout(llm.DefaultCloudWatchdogConfig(), liveCfg.CloudLLMStreamIdleTimeout)
		cfg.MaxConcurrentTools = liveCfg.MaxConcurrentTools
		if liveCfg.MaxTurns > 0 && fixture.Config.Execution.MaxTurns == 0 {
			cfg.MaxTurns = liveCfg.MaxTurns
		}
		return runtimeBundle{
			model:    model,
			provider: ProviderLive,
			client:   withChatOptions(client, map[string]any{"temperature": fixture.Config.Model.Live.Temperature}),
			registry: registry,
			agentCfg: cfg,
		}, nil
	default:
		return runtimeBundle{}, fmt.Errorf("unsupported provider %q", opts.Provider)
	}
}

type liveRuntimeConfig struct {
	KeepAlive                 string
	NumCtx                    int
	LLMStreamIdleTimeout      time.Duration
	CloudLLMStreamIdleTimeout time.Duration
	MaxTurns                  int
	MaxConcurrentTools        int
}

func buildLiveClient(ctx context.Context, opts RunOptions) (string, llm.Client, liveRuntimeConfig, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", nil, liveRuntimeConfig{}, err
	}
	initial := bootstrap.DefaultInitial(wd)
	initial.LLMProvider = string(llm.ProviderOllamaLocal)
	initial.LLMBaseURL = initial.OllamaBaseURL
	modelOverride := stringPtrOrNil(opts.Model)
	urlOverride := stringPtrOrNil(opts.OllamaURL)
	cfgRes, err := config.Load(
		initial.ConfigDir+"/config.toml",
		initial.WorkingDir+"/.nandocodego/config.toml",
		config.FlagOverrides{
			Model:     modelOverride,
			OllamaURL: urlOverride,
		},
	)
	if err != nil {
		return "", nil, liveRuntimeConfig{}, fmt.Errorf("load config: %w", err)
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
	if opts.Model != "" {
		initial.DefaultModel = opts.Model
	}
	if opts.OllamaURL != "" {
		initial.OllamaBaseURL = opts.OllamaURL
		initial.LLMBaseURL = opts.OllamaURL
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
			return "", nil, liveRuntimeConfig{}, errors.New("cloud model requires OLLAMA_API_KEY or keychain credential in eval mode")
		}
		return "", nil, liveRuntimeConfig{}, fmt.Errorf("resolve model %q: %w", initial.DefaultModel, err)
	}

	return switchRes.Resolved.Model, runtimeClient, liveRuntimeConfig{
		KeepAlive:                 initial.KeepAlive,
		NumCtx:                    initial.NumCtx,
		LLMStreamIdleTimeout:      initial.LLMStreamIdleTimeout,
		CloudLLMStreamIdleTimeout: initial.CloudLLMStreamIdleTimeout,
		MaxTurns:                  initial.MaxTurns,
		MaxConcurrentTools:        max(initial.MaxConcurrentTools, 1),
	}, nil
}

func stringPtrOrNil(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

func buildAgentInput(ctx context.Context, fixture Fixture, workspaceDir string, cfg ScoringConfig, model string) (agent.Input, permissions.PromptFunc) {
	toolCtx := tools.DefaultContext(ctx, workspaceDir)
	toolCtx.PermissionMode = permissions.ToToolsMode(permissions.Mode(cfg.Execution.PermissionMode).Normalize())
	toolCtx.Env = os.Environ()
	promptFunc := permissionPromptFor(cfg.Execution.ApprovalStrategy)
	input := agent.Input{
		Model:        model,
		SystemPrompt: evaluatorSystemPrompt,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: fixture.Task},
		},
		ToolContext:      toolCtx,
		PermissionMode:   permissions.Mode(cfg.Execution.PermissionMode).Normalize(),
		PermissionRules:  permissions.Rules{},
		PermissionPrompt: promptFunc,
		MaxOutputTokens:  8192,
	}
	return input, promptFunc
}

func permissionPromptFor(strategy ApprovalStrategy) permissions.PromptFunc {
	switch strategy {
	case ApprovalStrategyAllow:
		return func(context.Context, permissions.Prompt) (permissions.Decision, string, error) {
			return permissions.DecisionAllow, "approved by eval policy", nil
		}
	case ApprovalStrategyDeny:
		return func(context.Context, permissions.Prompt) (permissions.Decision, string, error) {
			return permissions.DecisionDeny, "denied by eval policy", nil
		}
	case ApprovalStrategyUnavailable:
		return nil
	case ApprovalStrategyScripted:
		return func(context.Context, permissions.Prompt) (permissions.Decision, string, error) {
			return permissions.DecisionDeny, "scripted approvals are not implemented", nil
		}
	default:
		return func(context.Context, permissions.Prompt) (permissions.Decision, string, error) {
			return permissions.DecisionAllow, "approved by eval policy", nil
		}
	}
}

// chatOptionsClient applies fixed model options (such as temperature) to every
// chat request, so live runs honour model.live settings without changing the
// shared agent loop.
type chatOptionsClient struct {
	llm.Client
	options map[string]any
}

func withChatOptions(client llm.Client, options map[string]any) llm.Client {
	return &chatOptionsClient{Client: client, options: options}
}

func (c *chatOptionsClient) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	if req != nil {
		clone := *req
		clone.Options = make(map[string]any, len(req.Options)+len(c.options))
		for k, v := range req.Options {
			clone.Options[k] = v
		}
		for k, v := range c.options {
			clone.Options[k] = v
		}
		req = &clone
	}
	return c.Client.Chat(ctx, req)
}
