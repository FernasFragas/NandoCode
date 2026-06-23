package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/analysis"
	"github.com/FernasFragas/Nandocode/internal/config"
	"github.com/FernasFragas/Nandocode/internal/hooks"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/llm/modelruntime"
	"github.com/FernasFragas/Nandocode/internal/memory"
	"github.com/FernasFragas/Nandocode/internal/observability"
	"github.com/FernasFragas/Nandocode/internal/paths"
	"github.com/FernasFragas/Nandocode/internal/permissions"
	"github.com/FernasFragas/Nandocode/internal/skills"
	"github.com/FernasFragas/Nandocode/internal/state"
	"github.com/FernasFragas/Nandocode/internal/types"
)

type OutputKind string

const (
	OutputSystem    OutputKind = "system"
	OutputAssistant OutputKind = "assistant"
)

type Output struct {
	Kind    OutputKind
	Content string
	Quit    bool
	Clear   bool
}

type HandlerContext struct {
	Store         *state.Store[state.App]
	LLMClient     llm.Client
	ModelRuntime  *modelruntime.Service
	Meter         *observability.Meter
	MemoryDir     string
	SkillLoader   *skills.Loader
	HookUserPath  string
	HookProjPath  string
	HookSnapshot  *hooks.Snapshot
	HookReloadSet func(hooks.Snapshot)
}

type Handler func(context.Context, []string, HandlerContext) Output

type Registry struct {
	handlers map[string]Handler
}

func New() *Registry {
	return &Registry{handlers: map[string]Handler{}}
}

func (r *Registry) Register(name string, h Handler) {
	key := strings.TrimSpace(strings.ToLower(name))
	if key == "" {
		panic("commands: empty command name")
	}
	if _, ok := r.handlers[key]; ok {
		panic("commands: duplicate command " + key)
	}
	r.handlers[key] = h
}

func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.handlers))
	for k := range r.handlers {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func (r *Registry) Dispatch(ctx context.Context, command string, args []string, hctx HandlerContext) Output {
	h, ok := r.handlers[strings.ToLower(command)]
	if !ok {
		return Output{
			Kind:    OutputSystem,
			Content: "[Error: Unknown command /" + command + ". Available: " + strings.Join(r.Names(), ", ") + "]",
		}
	}
	return h(ctx, args, hctx)
}

func RegisterDefaults(r *Registry) {
	r.Register("help", handleHelp)
	r.Register("clear", handleClear)
	r.Register("exit", handleExit)
	r.Register("model", handleModel)
	r.Register("models", handleModels)
	r.Register("pull", handlePull)
	r.Register("memory", handleMemory)
	r.Register("context", handleContext)
	r.Register("hooks", handleHooks)
	r.Register("permissions", handlePermissions)
	r.Register("skills", handleSkills)
	r.Register("cost", handleCost)
	r.Register("trace", handleTrace)
	r.Register("prompt", handlePrompt)
	r.Register("init", handleInit)
	r.Register("agents", handleAgents)
	r.Register("queue", handleQueue)
	r.Register("compact", handleCompact)
	r.Register("refresh-index", handleRefreshIndex)
	r.Register("analyze-project", handleAnalyzeProject)
	r.Register("checkpoint", handleCheckpoint)
	r.Register("bg", handleBG)
	r.Register("btw", handleBTW)
	r.Register("semantic", handleSemantic)
	r.Register("index", handleIndex)
}

func handleHelp(_ context.Context, _ []string, _ HandlerContext) Output {
	return Output{Kind: OutputSystem, Content: `Commands:
  /help                          - Show this help
  /clear                         - Clear transcript and message history
  /exit                          - Exit the REPL
  /model [name]                  - Show or switch active model
  /models [--cloud|--all]        - List local, cloud, or combined models
  /pull <model>                  - Pull/download model from Ollama
  /memory list                   - List active memory files
  /memory recall                 - Show memory recall mode
  /memory recall <off|fast|llm>  - Set memory recall mode for this session
  /memory show <name>            - Show memory file content
  /memory edit <name>            - Open memory file in $EDITOR
  /memory promote <name>         - Move pending memory file into active set
  /hooks list                    - List hooks by event kind
  /hooks reload yes              - Reload hooks.json (requires "yes")
  /permissions show              - Show mode and source-tagged rules
  /permissions allow <pattern>   - Add session allow rule
  /permissions deny <pattern>    - Add session deny rule
  /skills list                   - List available skills
  /skills show <name>            - Show skill content
  /cost                          - Show session usage summary
  /context status                - Show context mode for this session
  /context <auto|small|large|max>- Set adaptive context mode for this session
  /trace last                    - Show timings and outcome for the latest run
  /trace threshold [duration]    - Show or set slow-stage notice threshold (e.g. 500ms, 2s)
  /prompt last                   - Show metadata for latest final LLM request
  /prompt save last              - Persist latest prompt dump to state dir
  /prompt show last full         - Show full latest prompt content (full mode only)
  /init                          - Create default user config.toml
  /agents list                   - List agents for this session
  /queue list                    - List queued prompts
  /queue clear                   - Clear queued prompts
  /queue drop <index>            - Drop queued prompt by 1-based index
  /compact                       - Request context compaction (TUI-managed)
  /refresh-index                 - Refresh TUI @file completion index
  /analyze-project [path] <q>    - Run project analysis prompt (TUI-managed)
  /semantic on|off|status        - Toggle or inspect semantic retrieval (TUI-managed)
  /index build|refresh|status|clear - Manage semantic index (TUI-managed)
  /checkpoint status             - Show latest analysis checkpoint status
  /checkpoint clear              - Remove latest analysis checkpoint
  /bg                            - Background run status (TUI-managed)
  /btw <question>                - Side question in isolated read-only mode (TUI-managed)
  Ctrl+T                         - Expand/collapse thinking block`}
}

func handleCompact(_ context.Context, _ []string, _ HandlerContext) Output {
	return Output{Kind: OutputSystem, Content: "[Compact is handled by the interactive TUI flow]"}
}

func handleRefreshIndex(_ context.Context, _ []string, _ HandlerContext) Output {
	return Output{Kind: OutputSystem, Content: "[Refresh-index is handled by the interactive TUI flow]"}
}

func handleAnalyzeProject(_ context.Context, _ []string, _ HandlerContext) Output {
	return Output{Kind: OutputSystem, Content: "[Analyze-project is handled by the interactive TUI flow]"}
}

func handleBG(_ context.Context, _ []string, _ HandlerContext) Output {
	return Output{Kind: OutputSystem, Content: "[BG is handled by the interactive TUI flow]"}
}

func handleBTW(_ context.Context, _ []string, _ HandlerContext) Output {
	return Output{Kind: OutputSystem, Content: "[BTW is handled by the interactive TUI flow]"}
}

func handleSemantic(_ context.Context, _ []string, _ HandlerContext) Output {
	return Output{Kind: OutputSystem, Content: "[Semantic controls are handled by the interactive TUI flow]"}
}

func handleIndex(_ context.Context, _ []string, _ HandlerContext) Output {
	return Output{Kind: OutputSystem, Content: "[Index controls are handled by the interactive TUI flow]"}
}

func handleCheckpoint(_ context.Context, args []string, _ HandlerContext) Output {
	if len(args) == 0 {
		return Output{Kind: OutputSystem, Content: "[Error: Usage: /checkpoint status|clear]"}
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "status":
		ckpt, err := analysis.LoadCheckpoint()
		if err != nil {
			if os.IsNotExist(err) {
				return Output{Kind: OutputSystem, Content: "[Checkpoint: none]"}
			}
			return Output{Kind: OutputSystem, Content: "[Error: failed to load checkpoint: " + err.Error() + "]"}
		}
		age := "unknown"
		if !ckpt.UpdatedAt.IsZero() {
			age = time.Since(ckpt.UpdatedAt).Round(time.Second).String()
		}
		status := "completed"
		if ckpt.PendingFinalAnswer {
			status = "pending_final_answer"
		}
		return Output{Kind: OutputSystem, Content: fmt.Sprintf(
			"[Checkpoint: %s | model=%s | updated=%s ago | stage=%s | files=%d]",
			status,
			ckpt.Model,
			age,
			emptyFallback(ckpt.SynthesisStage, "n/a"),
			len(ckpt.InspectedFiles),
		)}
	case "clear":
		if err := analysis.DeleteCheckpoint(); err != nil {
			return Output{Kind: OutputSystem, Content: "[Error: failed to clear checkpoint: " + err.Error() + "]"}
		}
		return Output{Kind: OutputSystem, Content: "[Checkpoint cleared]"}
	default:
		return Output{Kind: OutputSystem, Content: "[Error: Usage: /checkpoint status|clear]"}
	}
}

func handleQueue(_ context.Context, args []string, hctx HandlerContext) Output {
	if hctx.Store == nil {
		return Output{Kind: OutputSystem, Content: "[Error: queue store unavailable]"}
	}
	if len(args) == 0 {
		return Output{Kind: OutputSystem, Content: "[Error: Usage: /queue list|clear|drop <index>]"}
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "list":
		q := hctx.Store.Get().QueuedPrompts
		if len(q) == 0 {
			return Output{Kind: OutputSystem, Content: "[Queue empty]"}
		}
		lines := make([]string, 0, len(q)+1)
		lines = append(lines, "Queued prompts:")
		for i, p := range q {
			preview := strings.TrimSpace(p)
			if len(preview) > 120 {
				preview = preview[:120] + "..."
			}
			lines = append(lines, fmt.Sprintf("  %d. %s", i+1, preview))
		}
		return Output{Kind: OutputSystem, Content: strings.Join(lines, "\n")}
	case "clear":
		hctx.Store.Set(func(app state.App) state.App {
			app.QueuedPrompts = []string{}
			return app
		})
		return Output{Kind: OutputSystem, Content: "[Queue cleared]"}
	case "drop":
		if len(args) < 2 {
			return Output{Kind: OutputSystem, Content: "[Error: Usage: /queue drop <index>]"}
		}
		idx, err := strconv.Atoi(strings.TrimSpace(args[1]))
		if err != nil || idx <= 0 {
			return Output{Kind: OutputSystem, Content: "[Error: queue index must be a positive integer]"}
		}
		idx--
		var dropped string
		var ok bool
		hctx.Store.Set(func(app state.App) state.App {
			if idx < 0 || idx >= len(app.QueuedPrompts) {
				return app
			}
			dropped = strings.TrimSpace(app.QueuedPrompts[idx])
			app.QueuedPrompts = append(app.QueuedPrompts[:idx], app.QueuedPrompts[idx+1:]...)
			ok = true
			return app
		})
		if !ok {
			return Output{Kind: OutputSystem, Content: "[Error: queue index out of range]"}
		}
		if len(dropped) > 120 {
			dropped = dropped[:120] + "..."
		}
		return Output{Kind: OutputSystem, Content: "[Dropped queue item] " + dropped}
	default:
		return Output{Kind: OutputSystem, Content: "[Error: Usage: /queue list|clear|drop <index>]"}
	}
}

// PrepareMemoryEditFile validates and prepares a memory file for editing.
// It creates a frontmatter template when the target does not exist yet.
func PrepareMemoryEditFile(memoryDir, name string) (string, error) {
	path, err := safeMemPath(memoryDir, name)
	if err != nil {
		return "", err
	}
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		slug := strings.TrimSuffix(filepath.Base(path), ".md")
		template := "---\nname: " + slug + "\ndescription: \ntype: user\n---\n\n"
		if writeErr := os.WriteFile(path, []byte(template), 0o600); writeErr != nil {
			return "", fmt.Errorf("could not create file: %w", writeErr)
		}
	}
	return path, nil
}

func emptyFallback(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func handleClear(_ context.Context, _ []string, _ HandlerContext) Output {
	return Output{Kind: OutputSystem, Content: "[Cleared transcript and message history]", Clear: true}
}

func handleExit(_ context.Context, _ []string, _ HandlerContext) Output {
	return Output{Kind: OutputSystem, Content: "[Exiting REPL]", Quit: true}
}

func handleModel(ctx context.Context, args []string, hctx HandlerContext) Output {
	if len(args) == 0 {
		app := hctx.Store.Get()
		msg := "[Current model: " + app.ActiveModel + "]"
		if strings.TrimSpace(app.LLMProvider) != "" {
			msg = "[Current model: " + app.ActiveModel + " | provider: " + app.LLMProvider + "]"
		}
		return Output{Kind: OutputSystem, Content: msg}
	}
	name := strings.TrimSpace(strings.Join(args, " "))
	if name == "" {
		return Output{Kind: OutputSystem, Content: "[Error: /model requires a model name. Usage: /model <name>]"}
	}
	if hctx.ModelRuntime != nil {
		res, err := hctx.ModelRuntime.Switch(ctx, modelruntime.SwitchOptions{
			RequestedModel: name,
			AllowPrompt:    false,
		})
		if err != nil {
			switch {
			case errors.Is(err, modelruntime.ErrCredentialRequired):
				return Output{Kind: OutputSystem, Content: "[Error: Ollama Cloud model requires OLLAMA_API_KEY or keychain credential in non-interactive mode]"}
			case errors.Is(err, modelruntime.ErrCredentialCanceled):
				return Output{Kind: OutputSystem, Content: "[Model switch canceled]"}
			case errors.Is(err, modelruntime.ErrUnauthorized):
				return Output{Kind: OutputSystem, Content: "[Error: invalid Ollama Cloud API key]"}
			default:
				return Output{Kind: OutputSystem, Content: "[Error: " + err.Error() + "]"}
			}
		}
		hctx.Store.Set(func(app state.App) state.App {
			app.ActiveModel = res.Resolved.Model
			app.LLMProvider = string(res.Resolved.Provider)
			app.LLMBaseURL = res.Resolved.BaseURL
			if app.LLMBaseURL == "" {
				app.LLMBaseURL = hctx.ModelRuntime.LocalBaseURL
			}
			return app
		})
		msg := res.Message
		if res.Resolved.AliasUsed {
			msg = "[Model " + res.Resolved.RequestedName + " is local-cloud naming. Using Ollama Cloud API model: " + res.Resolved.Model + "]\n" + msg
		}
		return Output{Kind: OutputSystem, Content: msg}
	}
	if hctx.LLMClient == nil {
		return Output{Kind: OutputSystem, Content: "[Error: model validation unavailable]"}
	}
	models, err := hctx.LLMClient.ListModels(ctx)
	if err != nil {
		return Output{Kind: OutputSystem, Content: "[Error: failed to list models: " + err.Error() + "]"}
	}
	found := false
	for _, m := range models {
		if m.Name == name {
			found = true
			break
		}
	}
	if !found {
		return Output{Kind: OutputSystem, Content: "[Error: model not found locally: " + name + ". Try /pull " + name + "]"}
	}
	hctx.Store.Set(func(app state.App) state.App {
		app.ActiveModel = name
		return app
	})

	// Fetch live limits for the new model and update the store so the next run uses them.
	msg := "[Switched to model: " + name + "]"
	if details, err := hctx.LLMClient.ShowModel(ctx, name); err == nil {
		limits := llm.ComputeLimits(details)
		hctx.Store.Set(func(app state.App) state.App {
			app.MaxOutputTokens = limits.MaxOutputTokens
			app.ToolSettings.MaxResultChars = limits.MaxResultChars
			return app
		})
		msg += fmt.Sprintf(" | context: %d tokens | max output: %d tokens | result buffer: %d chars",
			details.ContextLength, limits.MaxOutputTokens, limits.MaxResultChars)
	}
	return Output{Kind: OutputSystem, Content: msg}
}

func handleModels(ctx context.Context, args []string, hctx HandlerContext) Output {
	mode, modeErr := parseModelsListMode(args)
	if modeErr != "" {
		return Output{Kind: OutputSystem, Content: "[Error: " + modeErr + "]"}
	}
	if hctx.ModelRuntime != nil {
		return renderModelsViaRuntime(ctx, mode, hctx.ModelRuntime)
	}
	if hctx.LLMClient == nil {
		return Output{Kind: OutputSystem, Content: "[Error: no LLM client available]"}
	}
	if mode != "local" {
		return Output{Kind: OutputSystem, Content: "[Error: cloud model listing requires model runtime service]"}
	}
	models, err := hctx.LLMClient.ListModels(ctx)
	if err != nil {
		return Output{Kind: OutputSystem, Content: "[Error: failed to list models: " + err.Error() + "]"}
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Name < models[j].Name })
	if len(models) == 0 {
		return Output{Kind: OutputSystem, Content: "[No models found]"}
	}
	lines := []string{"Models:"}
	for _, m := range models {
		lines = append(lines, fmt.Sprintf("  %s (%s)", m.Name, bytesHuman(m.Size)))
	}
	return Output{Kind: OutputSystem, Content: strings.Join(lines, "\n")}
}

func handlePull(ctx context.Context, args []string, hctx HandlerContext) Output {
	if len(args) == 0 {
		return Output{Kind: OutputSystem, Content: "[Error: Usage: /pull <model>]"}
	}
	name := strings.TrimSpace(strings.Join(args, " "))
	progress := make(chan llm.PullProgress, 32)
	done := make(chan error, 1)
	if hctx.ModelRuntime != nil {
		go func() { done <- hctx.ModelRuntime.PullLocal(ctx, name, progress) }()
	} else {
		if hctx.LLMClient == nil {
			return Output{Kind: OutputSystem, Content: "[Error: no LLM client available]"}
		}
		go func() { done <- hctx.LLMClient.PullModel(ctx, name, progress) }()
	}
	last := "started"
loop:
	for {
		select {
		case p := <-progress:
			if p.Status != "" {
				last = p.Status
			}
		case err := <-done:
			if err != nil {
				return Output{Kind: OutputSystem, Content: "[Error: pull failed: " + err.Error() + "]"}
			}
			break loop
		}
	}
	msg := "[Pull completed: " + name + " (" + last + ")]"
	if hctx.Store != nil && hctx.Store.Get().LLMProvider == string(llm.ProviderOllamaCloudAPI) {
		msg = msg + "\n[/pull always targets your local Ollama daemon]"
	}
	return Output{Kind: OutputSystem, Content: msg}
}

func parseModelsListMode(args []string) (string, string) {
	mode := "local"
	for _, arg := range args {
		flag := strings.ToLower(strings.TrimSpace(arg))
		switch flag {
		case "":
			continue
		case "--cloud":
			if mode != "local" {
				return "", "choose only one of --cloud or --all"
			}
			mode = "cloud"
		case "--all":
			if mode != "local" {
				return "", "choose only one of --cloud or --all"
			}
			mode = "all"
		default:
			return "", "usage: /models [--cloud|--all]"
		}
	}
	return mode, ""
}

func renderModelsViaRuntime(ctx context.Context, mode string, runtime *modelruntime.Service) Output {
	switch mode {
	case "cloud":
		cloud, err := runtime.ListCloud(ctx)
		if err != nil {
			return Output{Kind: OutputSystem, Content: "[Error: failed to list Ollama Cloud models: " + err.Error() + "]"}
		}
		sort.Slice(cloud, func(i, j int) bool { return cloud[i].Name < cloud[j].Name })
		lines := []string{"Models:", "  Ollama Cloud"}
		for _, m := range cloud {
			lines = append(lines, "    "+m.Name)
		}
		if len(cloud) == 0 {
			lines = append(lines, "    [No cloud models found]")
		}
		return Output{Kind: OutputSystem, Content: strings.Join(lines, "\n")}
	case "all":
		local, localErr := runtime.ListLocal(ctx)
		cloud, cloudErr := runtime.ListCloud(ctx)
		if localErr != nil {
			return Output{Kind: OutputSystem, Content: "[Error: failed to list local models: " + localErr.Error() + "]"}
		}
		sort.Slice(local, func(i, j int) bool { return local[i].Name < local[j].Name })
		lines := []string{"Models:", "  Local"}
		for _, m := range local {
			lines = append(lines, fmt.Sprintf("    %s (%s)", m.Name, bytesHuman(m.Size)))
		}
		if len(local) == 0 {
			lines = append(lines, "    [No local models found]")
		}
		lines = append(lines, "", "  Ollama Cloud")
		if cloudErr != nil {
			lines = append(lines, "    [Unavailable: failed to list Ollama Cloud models: "+cloudErr.Error()+"]")
		} else {
			sort.Slice(cloud, func(i, j int) bool { return cloud[i].Name < cloud[j].Name })
			for _, m := range cloud {
				lines = append(lines, "    "+m.Name)
			}
			if len(cloud) == 0 {
				lines = append(lines, "    [No cloud models found]")
			}
		}
		return Output{Kind: OutputSystem, Content: strings.Join(lines, "\n")}
	default:
		local, err := runtime.ListLocal(ctx)
		if err != nil {
			return Output{Kind: OutputSystem, Content: "[Error: failed to list local models: " + err.Error() + "]"}
		}
		sort.Slice(local, func(i, j int) bool { return local[i].Name < local[j].Name })
		lines := []string{"Models:", "  Local"}
		for _, m := range local {
			lines = append(lines, fmt.Sprintf("    %s (%s)", m.Name, bytesHuman(m.Size)))
		}
		if len(local) == 0 {
			lines = append(lines, "    [No local models found]")
		}
		return Output{Kind: OutputSystem, Content: strings.Join(lines, "\n")}
	}
}

func handleMemory(ctx context.Context, args []string, hctx HandlerContext) Output {
	if len(args) == 0 {
		return Output{Kind: OutputSystem, Content: "[Error: Usage: /memory list|show|edit|promote <name>]"}
	}
	if hctx.MemoryDir == "" {
		return Output{Kind: OutputSystem, Content: "[Error: memory directory not available (no git root found)]"}
	}
	switch args[0] {
	case "recall":
		app := hctx.Store.Get()
		if len(args) == 1 {
			mode := app.MemoryRecallMode
			if mode == "" {
				mode = "fast"
			}
			return Output{Kind: OutputSystem, Content: "[Memory recall mode: " + mode + "]"}
		}
		mode := strings.ToLower(strings.TrimSpace(args[1]))
		if mode != "off" && mode != "fast" && mode != "llm" {
			return Output{Kind: OutputSystem, Content: "[Error: Usage: /memory recall <off|fast|llm>]"}
		}
		hctx.Store.Set(func(app state.App) state.App {
			app.MemoryRecallMode = mode
			return app
		})
		return Output{Kind: OutputSystem, Content: "[Set memory recall mode: " + mode + "]"}
	case "list":
		res, err := memory.Scan(ctx, hctx.MemoryDir)
		if err != nil {
			return Output{Kind: OutputSystem, Content: "[Error: " + err.Error() + "]"}
		}
		pendingDir := filepath.Join(hctx.MemoryDir, "pending")
		pendingEntries, _ := os.ReadDir(pendingDir)
		if len(res.Entries) == 0 && len(pendingEntries) == 0 && len(res.Warnings) == 0 {
			return Output{Kind: OutputSystem, Content: "[No memory files] (dir: " + hctx.MemoryDir + ")"}
		}
		lines := []string{"Memory files (" + hctx.MemoryDir + "):"}
		for _, e := range res.Entries {
			lines = append(lines, fmt.Sprintf("  %s (%s, %s)", e.Filename, e.Type, e.UpdatedAt.Format(time.RFC3339)))
		}
		for _, de := range pendingEntries {
			if !de.IsDir() {
				lines = append(lines, fmt.Sprintf("  pending/%s (draft — use /memory promote to activate)", de.Name()))
			}
		}
		for _, w := range res.Warnings {
			lines = append(lines, "  [skipped] "+w)
		}
		return Output{Kind: OutputSystem, Content: strings.Join(lines, "\n")}
	case "show":
		if len(args) < 2 {
			return Output{Kind: OutputSystem, Content: "[Error: Usage: /memory show <name>]"}
		}
		path, err := safeMemPath(hctx.MemoryDir, args[1])
		if err != nil {
			return Output{Kind: OutputSystem, Content: "[Error: " + err.Error() + "]"}
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return Output{Kind: OutputSystem, Content: "[Error: " + err.Error() + "]"}
		}
		return Output{Kind: OutputAssistant, Content: string(b)}
	case "edit":
		if len(args) < 2 {
			return Output{Kind: OutputSystem, Content: "[Error: Usage: /memory edit <name>]"}
		}
		path, err := PrepareMemoryEditFile(hctx.MemoryDir, args[1])
		if err != nil {
			return Output{Kind: OutputSystem, Content: "[Error: " + err.Error() + "]"}
		}
		editor := strings.TrimSpace(os.Getenv("EDITOR"))
		if editor == "" {
			editor = "vi"
		}
		c := exec.CommandContext(ctx, editor, path)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return Output{Kind: OutputSystem, Content: "[Error: editor failed: " + err.Error() + "]"}
		}
		return Output{Kind: OutputSystem, Content: "[Edited memory file: " + filepath.Base(path) + "]"}
	case "promote":
		if len(args) < 2 {
			return Output{Kind: OutputSystem, Content: "[Error: Usage: /memory promote <name>]"}
		}
		name := filepath.Base(args[1])
		src := filepath.Join(hctx.MemoryDir, "pending", name)
		dst := filepath.Join(hctx.MemoryDir, name)
		if _, err := os.Stat(src); err != nil {
			return Output{Kind: OutputSystem, Content: "[Error: pending file not found: " + name + "]"}
		}
		if err := os.Rename(src, dst); err != nil {
			return Output{Kind: OutputSystem, Content: "[Error: promote failed: " + err.Error() + "]"}
		}
		return Output{Kind: OutputSystem, Content: "[Promoted memory file: " + name + "]"}
	default:
		return Output{Kind: OutputSystem, Content: "[Error: Usage: /memory list|recall|show|edit|promote <name>]"}
	}
}

func handleContext(_ context.Context, args []string, hctx HandlerContext) Output {
	app := hctx.Store.Get()
	mode := app.ContextMode
	if mode == "" {
		mode = "auto"
	}
	if len(args) == 0 || strings.ToLower(args[0]) == "status" {
		return Output{Kind: OutputSystem, Content: "[Context mode: " + mode + "]"}
	}
	next := strings.ToLower(strings.TrimSpace(args[0]))
	if next != "auto" && next != "small" && next != "large" && next != "max" {
		return Output{Kind: OutputSystem, Content: "[Error: Usage: /context status|auto|small|large|max]"}
	}
	hctx.Store.Set(func(app state.App) state.App {
		app.ContextMode = next
		return app
	})
	return Output{Kind: OutputSystem, Content: "[Set context mode: " + next + "]"}
}

func handleHooks(_ context.Context, args []string, hctx HandlerContext) Output {
	if len(args) == 0 {
		return Output{Kind: OutputSystem, Content: "[Error: Usage: /hooks list|reload yes]"}
	}
	switch args[0] {
	case "list":
		if hctx.HookSnapshot == nil {
			return Output{Kind: OutputSystem, Content: "[No hooks loaded]"}
		}
		if len(hctx.HookSnapshot.Hooks) == 0 {
			return Output{Kind: OutputSystem, Content: "[No active hooks]"}
		}
		grouped := map[string][]hooks.Hook{}
		keys := make([]string, 0, 8)
		for _, h := range hctx.HookSnapshot.Hooks {
			k := string(h.Event)
			if _, ok := grouped[k]; !ok {
				keys = append(keys, k)
			}
			grouped[k] = append(grouped[k], h)
		}
		sort.Strings(keys)
		lines := []string{"Hooks by event:"}
		for _, k := range keys {
			lines = append(lines, "  "+k+":")
			for _, h := range grouped[k] {
				lines = append(lines, fmt.Sprintf("    - %s matcher=%q source=%s", h.Kind, h.Matcher, h.Source))
			}
		}
		return Output{Kind: OutputSystem, Content: strings.Join(lines, "\n")}
	case "reload":
		if len(args) < 2 || strings.ToLower(args[1]) != "yes" {
			return Output{Kind: OutputSystem, Content: `[Cancelled: /hooks reload requires confirmation. Run "/hooks reload yes".]`}
		}
		snap := hooks.LoadSnapshot(hooks.LoadOptions{UserPath: hctx.HookUserPath, ProjectPath: hctx.HookProjPath})
		if hctx.HookReloadSet != nil {
			hctx.HookReloadSet(snap)
		}
		if hctx.HookSnapshot != nil {
			*hctx.HookSnapshot = snap
		}
		return Output{Kind: OutputSystem, Content: fmt.Sprintf("[Hooks reloaded: %d active, %d disabled, %d warnings]", len(snap.Hooks), len(snap.Disabled), len(snap.Warnings))}
	default:
		return Output{Kind: OutputSystem, Content: "[Error: Usage: /hooks list|reload yes]"}
	}
}

func handlePermissions(_ context.Context, args []string, hctx HandlerContext) Output {
	if len(args) == 0 {
		return Output{Kind: OutputSystem, Content: "[Error: Usage: /permissions show|allow|deny <pattern>]"}
	}
	switch args[0] {
	case "show":
		s := hctx.Store.Get()
		lines := []string{"Permission mode: " + s.PermissionMode.String()}
		appendRules := func(title string, rules []permissions.Rule) {
			lines = append(lines, title+":")
			if len(rules) == 0 {
				lines = append(lines, "  (none)")
				return
			}
			for _, r := range rules {
				lines = append(lines, fmt.Sprintf("  - %s [%s]", r.Pattern, r.Source.String()))
			}
		}
		appendRules("Always allow", s.PermissionRules.AlwaysAllow)
		appendRules("Always ask", s.PermissionRules.AlwaysAsk)
		appendRules("Always deny", s.PermissionRules.AlwaysDeny)
		return Output{Kind: OutputSystem, Content: strings.Join(lines, "\n")}
	case "allow", "deny":
		if len(args) < 2 {
			return Output{Kind: OutputSystem, Content: "[Error: Usage: /permissions " + args[0] + " <pattern>]"}
		}
		pattern := strings.Join(args[1:], " ")
		if tool, glob, ok := permissions.ParsePattern(pattern); !ok || strings.TrimSpace(tool) == "" || strings.TrimSpace(glob) == "" {
			return Output{Kind: OutputSystem, Content: "[Error: invalid pattern. Expected ToolName(glob)]"}
		}
		rule := permissions.Rule{Pattern: pattern, Source: permissions.SourceSession}
		hctx.Store.Set(func(app state.App) state.App {
			if args[0] == "allow" {
				app.PermissionRules.AlwaysAllow = append(app.PermissionRules.AlwaysAllow, rule)
			} else {
				app.PermissionRules.AlwaysDeny = append(app.PermissionRules.AlwaysDeny, rule)
			}
			return app
		})
		return Output{Kind: OutputSystem, Content: "[Added session " + args[0] + " rule: " + pattern + "]"}
	default:
		return Output{Kind: OutputSystem, Content: "[Error: Usage: /permissions show|allow|deny <pattern>]"}
	}
}

func handleSkills(_ context.Context, args []string, hctx HandlerContext) Output {
	if len(args) == 0 {
		return Output{Kind: OutputSystem, Content: "[Error: Usage: /skills list|show <name>]"}
	}
	if hctx.SkillLoader == nil {
		return Output{Kind: OutputSystem, Content: "[Error: skills loader is not available]"}
	}
	switch args[0] {
	case "list":
		list := hctx.SkillLoader.List()
		if len(list) == 0 {
			return Output{Kind: OutputSystem, Content: "[No skills loaded]"}
		}
		lines := []string{"Skills:"}
		for _, s := range list {
			lines = append(lines, fmt.Sprintf("  %s  (%s)  %s", s.Name, s.Source.String(), s.Description))
		}
		return Output{Kind: OutputSystem, Content: strings.Join(lines, "\n")}
	case "show":
		if len(args) < 2 {
			return Output{Kind: OutputSystem, Content: "[Error: Usage: /skills show <name>]"}
		}
		name := strings.Join(args[1:], " ")
		sf, ok := hctx.SkillLoader.Lookup(name)
		if !ok {
			return Output{Kind: OutputSystem, Content: "[Error: Unknown skill " + name + "]"}
		}
		body, err := hctx.SkillLoader.ReadBody(sf)
		if err != nil {
			return Output{Kind: OutputSystem, Content: "[Error: Failed to load skill body: " + err.Error() + "]"}
		}
		return Output{Kind: OutputAssistant, Content: fmt.Sprintf("## %s\nSource: %s\n\n%s", sf.Name, sf.Source.String(), body)}
	default:
		return Output{Kind: OutputSystem, Content: "[Error: Usage: /skills list|show <name>]"}
	}
}

func handleCost(_ context.Context, _ []string, hctx HandlerContext) Output {
	if hctx.Meter != nil {
		snap := hctx.Meter.Snapshot()
		lines := []string{
			"Session usage:",
			fmt.Sprintf("  Prompt tokens:      %d", snap.PromptTokens),
			fmt.Sprintf("  Completion tokens:  %d", snap.CompletionTokens),
			fmt.Sprintf("  Total tokens:       %d", snap.TotalTokens),
			fmt.Sprintf("  Last done reason:   %s", fallbackDisplay(snap.LastDoneReason, "<none>")),
			fmt.Sprintf("  Retries:            %d", snap.RetryCount),
			fmt.Sprintf("  LLM calls:          %d", snap.LLMCalls),
			fmt.Sprintf("  Tool calls:         %d", snap.ToolCalls),
			fmt.Sprintf("  Runs:               %d", snap.AgentRuns),
		}
		if snap.LastRetryKind != "" || snap.LastRetryReason != "" {
			lines = append(lines,
				fmt.Sprintf("  Last retry kind:    %s", fallbackDisplay(snap.LastRetryKind, "<unknown>")),
				fmt.Sprintf("  Last retry reason:  %s", fallbackDisplay(snap.LastRetryReason, "<none>")),
				fmt.Sprintf("  Last retry done:    %s", fallbackDisplay(snap.LastRetryDoneReason, "<none>")),
			)
		}
		if !snap.StartedAt.IsZero() {
			lines = append(lines, fmt.Sprintf("  Session duration:   %s", time.Since(snap.StartedAt).Round(time.Millisecond)))
		}
		return Output{Kind: OutputSystem, Content: strings.Join(lines, "\n")}
	}

	u := hctx.Store.Get().Usage
	return Output{
		Kind: OutputSystem,
		Content: fmt.Sprintf("Usage: turns=%d prompt_tokens=%d completion_tokens=%d tool_calls=%d total_duration_ns=%d done_reason=%s",
			u.Turns, u.PromptEvalCount, u.EvalCount, u.ToolCalls, u.TotalDuration, fallbackDisplay(u.DoneReason, "<none>")),
	}
}

func handleTrace(_ context.Context, args []string, hctx HandlerContext) Output {
	if len(args) == 0 {
		return Output{Kind: OutputSystem, Content: "[Error: Usage: /trace last|threshold [duration]]"}
	}
	switch strings.ToLower(args[0]) {
	case "threshold":
		current := 750 * time.Millisecond
		source := "default"
		if hctx.Store != nil {
			app := hctx.Store.Get()
			current = app.SlowStageNoticeThreshold
			source = app.SlowStageNoticeThresholdSource
		}
		if current <= 0 {
			current = 750 * time.Millisecond
			source = "default"
		}
		if len(args) == 1 {
			return Output{Kind: OutputSystem, Content: fmt.Sprintf("[Slow-stage notice threshold: %s (source: %s)]", current, fallbackDisplay(source, "default"))}
		}
		d, err := time.ParseDuration(strings.TrimSpace(args[1]))
		if err != nil || d <= 0 {
			return Output{Kind: OutputSystem, Content: "[Error: invalid duration. Example: /trace threshold 750ms]"}
		}
		hctx.Store.Set(func(app state.App) state.App {
			app.SlowStageNoticeThreshold = d
			app.SlowStageNoticeThresholdSource = "session"
			return app
		})
		return Output{Kind: OutputSystem, Content: fmt.Sprintf("[Set slow-stage notice threshold: %s]", d)}
	case "last":
		// continue below
	default:
		return Output{Kind: OutputSystem, Content: "[Error: Usage: /trace last|threshold [duration]]"}
	}
	if hctx.Meter == nil {
		return Output{Kind: OutputSystem, Content: "[Error: run trace is unavailable]"}
	}
	snap := hctx.Meter.Snapshot()
	trace := snap.LastRunTrace
	activeTrace := snap.CurrentRunTrace
	active := snap.CurrentRunActive && !activeTrace.RunStartedAt.IsZero()
	if trace.RunStartedAt.IsZero() && !active {
		return Output{Kind: OutputSystem, Content: "[No run trace recorded yet]"}
	}
	label := "Last run trace:"
	if active && trace.RunStartedAt.IsZero() {
		trace = activeTrace
		label = "Current run trace (active):"
	} else if active {
		label = "Last run trace: [active run also in progress]"
	}

	lines := []string{
		label,
		fmt.Sprintf("  Started:                %s", trace.RunStartedAt.Format(time.RFC3339)),
		fmt.Sprintf("  First event latency:    %s", trace.FirstEventLatency.Round(time.Millisecond)),
		fmt.Sprintf("  First assistant event:  %s", trace.FirstAssistantLatency.Round(time.Millisecond)),
		fmt.Sprintf("  First thinking event:   %s", trace.FirstThinkingLatency.Round(time.Millisecond)),
		fmt.Sprintf("  First tool start:       %s", trace.FirstToolStartLatency.Round(time.Millisecond)),
		fmt.Sprintf("  First retry:            %s", trace.FirstRetryLatency.Round(time.Millisecond)),
		fmt.Sprintf("  Compaction started:     %s", trace.CompactionStartLatency.Round(time.Millisecond)),
		fmt.Sprintf("  Compaction completed:   %s", trace.CompactionEndLatency.Round(time.Millisecond)),
		fmt.Sprintf("  Terminal latency:       %s", trace.TerminalLatency.Round(time.Millisecond)),
		fmt.Sprintf("  Terminal reason:        %s", fallbackDisplay(trace.TerminalReason, "<none>")),
		fmt.Sprintf("  Done reason:            %s", fallbackDisplay(trace.DoneReason, "<none>")),
		fmt.Sprintf("  Retries:                %d", trace.RetryCount),
	}
	if trace.MentionDirs > 0 || trace.MentionListingIntent || trace.MentionMode != "" {
		lines = append(lines,
			fmt.Sprintf("  Mention mode:           %s", fallbackDisplay(trace.MentionMode, "<none>")),
			fmt.Sprintf("  Mention dirs/files:     %d / %d", trace.MentionDirs, trace.MentionFilesDiscovered),
			fmt.Sprintf("  Mention file bodies:    %d", trace.MentionFileBodies),
			fmt.Sprintf("  Mention listing intent: %t", trace.MentionListingIntent),
		)
	}
	if active && trace.RunStartedAt.Equal(activeTrace.RunStartedAt) {
		lines = append(lines, "  Note: terminal trace not recorded yet (run still active).")
	}
	threshold := 750 * time.Millisecond
	thresholdSource := "default"
	contextMode := ""
	runtimeNumCtx := 0
	if hctx.Store != nil {
		app := hctx.Store.Get()
		threshold = app.SlowStageNoticeThreshold
		thresholdSource = app.SlowStageNoticeThresholdSource
		contextMode = strings.TrimSpace(app.ContextMode)
		runtimeNumCtx = app.RuntimeNumCtx
	}
	if threshold <= 0 {
		threshold = 750 * time.Millisecond
		thresholdSource = "default"
	}
	lines = append(lines, fmt.Sprintf("  Context mode:         %s", fallbackDisplay(contextMode, "<none>")))
	if runtimeNumCtx > 0 {
		lines = append(lines, fmt.Sprintf("  Effective context:    %d", runtimeNumCtx))
	}
	if v := strings.TrimSpace(trace.ToolMode); v != "" {
		lines = append(lines, fmt.Sprintf("  Tool mode:            %s", v))
	}
	if v := strings.TrimSpace(trace.RouteProfile); v != "" {
		lines = append(lines, fmt.Sprintf("  Route profile:        %s", v))
	}
	if v := strings.TrimSpace(trace.RouteAction); v != "" {
		lines = append(lines, fmt.Sprintf("  Route action:         %s", v))
	}
	if v := strings.TrimSpace(trace.RouteReason); v != "" {
		lines = append(lines, fmt.Sprintf("  Route reason:         %s", v))
	}
	if trace.PromptPackInputBudget > 0 || trace.PromptPackIncluded > 0 || trace.PromptPackSkipped > 0 || trace.PromptPackDroppedBlocks > 0 {
		lines = append(lines,
			fmt.Sprintf("  Prompt pack budget:   %d", trace.PromptPackInputBudget),
			fmt.Sprintf("  Prompt pack kept:     %d", trace.PromptPackIncluded),
			fmt.Sprintf("  Prompt pack skipped:  %d", trace.PromptPackSkipped),
			fmt.Sprintf("  Mention blocks drop:  %d", trace.PromptPackDroppedBlocks),
		)
	}
	if trace.EvidencePacked || trace.EvidenceFiles > 0 || trace.EvidenceOmitted > 0 {
		lines = append(lines,
			fmt.Sprintf("  Evidence packed:      %t", trace.EvidencePacked),
			fmt.Sprintf("  Evidence budget:      %d", trace.EvidenceBudget),
			fmt.Sprintf("  Evidence files:       %d", trace.EvidenceFiles),
			fmt.Sprintf("  Evidence raw bytes:   %d", trace.EvidenceRawBytes),
			fmt.Sprintf("  Evidence omitted raw: %d", trace.EvidenceRawBytesOmitted),
			fmt.Sprintf("  Evidence excerpted:   %d", trace.EvidenceExcerpted),
			fmt.Sprintf("  Evidence omitted:     %d", trace.EvidenceOmitted),
		)
	}
	lines = append(lines, fmt.Sprintf("  Slow-stage threshold:   %s (source: %s)", threshold, fallbackDisplay(thresholdSource, "default")))
	lines = append(lines, fmt.Sprintf("  Diagnosis:            %s", traceDiagnosis(trace, threshold)))
	slowestStages := traceSlowestStages(trace.StageLatencies, 3)
	if len(slowestStages) > 0 {
		lines = append(lines, "  Slowest stages:")
		for _, stage := range slowestStages {
			lines = append(lines, fmt.Sprintf("    %s: %s", stage.Name, stage.Duration.Round(time.Millisecond)))
		}
	}
	if len(trace.RetryKinds) > 0 {
		lines = append(lines, "  Retry kinds:")
		kinds := make([]string, 0, len(trace.RetryKinds))
		for k := range trace.RetryKinds {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		for _, k := range kinds {
			lines = append(lines, fmt.Sprintf("    %s: %d", k, trace.RetryKinds[k]))
		}
	}
	if len(trace.StageLatencies) > 0 {
		lines = append(lines, "  Stage latencies:")
		stages := make([]string, 0, len(trace.StageLatencies))
		for stage := range trace.StageLatencies {
			stages = append(stages, stage)
		}
		sort.Strings(stages)
		for _, stage := range stages {
			lines = append(lines, fmt.Sprintf("    %s: %s", stage, trace.StageLatencies[stage].Round(time.Millisecond)))
		}
	}
	return Output{Kind: OutputSystem, Content: strings.Join(lines, "\n")}
}

type traceStage struct {
	Name     string
	Duration time.Duration
}

func traceSlowestStages(stages map[string]time.Duration, limit int) []traceStage {
	if len(stages) == 0 || limit <= 0 {
		return nil
	}
	out := make([]traceStage, 0, len(stages))
	for name, dur := range stages {
		if dur <= 0 {
			continue
		}
		out = append(out, traceStage{Name: name, Duration: dur})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Duration == out[j].Duration {
			return out[i].Name < out[j].Name
		}
		return out[i].Duration > out[j].Duration
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func traceDiagnosis(trace observability.RunTrace, threshold time.Duration) string {
	top := traceSlowestStages(trace.StageLatencies, 1)
	if len(top) > 0 && top[0].Duration >= threshold {
		stage := traceStageLabel(top[0].Name)
		details := []string{fmt.Sprintf("%s was the main recorded cost (%s", stage, top[0].Duration.Round(time.Millisecond))}
		if trace.TerminalLatency > 0 {
			share := int((100 * top[0].Duration) / trace.TerminalLatency)
			if share > 0 {
				details = append(details, fmt.Sprintf("%d%% of terminal", share))
			}
		}
		msg := strings.Join(details, ", ") + ")"
		if trace.RetryCount > 0 {
			msg += fmt.Sprintf("; retries=%d", trace.RetryCount)
		}
		return msg
	}
	if trace.FirstAssistantLatency >= threshold {
		return fmt.Sprintf("slow first assistant event (%s) before steady output", trace.FirstAssistantLatency.Round(time.Millisecond))
	}
	if trace.RetryCount > 0 {
		return fmt.Sprintf("no recorded stage exceeded %s; retries=%d contributed", threshold, trace.RetryCount)
	}
	if threshold > 0 {
		return fmt.Sprintf("no recorded stage exceeded %s", threshold)
	}
	return "no recorded bottleneck"
}

func traceStageLabel(name string) string {
	switch name {
	case "mention_expand":
		return "mention expansion"
	case "memory_recall":
		return "memory recall"
	case "prompt_pack":
		return "prompt packing"
	case "semantic_retrieve":
		return "semantic retrieval"
	case "semantic_embed":
		return "semantic query embed"
	case "semantic_manifest":
		return "semantic manifest load"
	case "semantic_records":
		return "semantic record load"
	case "semantic_vectors":
		return "semantic vector load"
	case "semantic_score":
		return "semantic scoring"
	case "semantic_render":
		return "semantic context render"
	case "compaction_started", "compaction_completed":
		return "compaction"
	default:
		return strings.ReplaceAll(name, "_", " ")
	}
}

func handlePrompt(_ context.Context, args []string, _ HandlerContext) Output {
	if len(args) == 0 {
		return Output{Kind: OutputSystem, Content: "[Error: Usage: /prompt last|save last|show last full]"}
	}
	switch strings.ToLower(strings.Join(args, " ")) {
	case "last":
		dump, ok := agent.LatestPromptDump()
		if !ok {
			return Output{Kind: OutputSystem, Content: "[No prompt dump recorded yet]"}
		}
		lines := []string{
			"Last prompt dump:",
			fmt.Sprintf("  Time:                 %s", dump.CreatedAt.Format(time.RFC3339)),
			fmt.Sprintf("  Model:                %s", fallbackDisplay(dump.Model, "<none>")),
			fmt.Sprintf("  Dump mode:            %s", fallbackDisplay(dump.DumpMode, "off")),
			fmt.Sprintf("  Intent:               %s", fallbackDisplay(dump.Intent, "<none>")),
			fmt.Sprintf("  Attachment policy:    %s", fallbackDisplay(dump.AttachmentPolicy, "<none>")),
			fmt.Sprintf("  History policy:       %s", fallbackDisplay(dump.HistoryPolicy, "<none>")),
			fmt.Sprintf("  Memory policy:        %s", fallbackDisplay(dump.MemoryPolicy, "<none>")),
			fmt.Sprintf("  Retry policy:         %s", fallbackDisplay(dump.RetryPolicy, "<none>")),
			fmt.Sprintf("  File bodies:          %d", dump.IncludedFileBodies),
			fmt.Sprintf("  Directory tree:       %t", dump.DirectoryTreeAttached),
			fmt.Sprintf("  Messages:             %d", dump.MessageCount),
			fmt.Sprintf("  Estimated tokens:     %d", dump.EstimatedTokens),
			fmt.Sprintf("  Tools:                %d", dump.ToolCount),
		}
		if len(dump.Options) > 0 {
			keys := make([]string, 0, len(dump.Options))
			for key := range dump.Options {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			lines = append(lines, "  Options:")
			for _, key := range keys {
				lines = append(lines, fmt.Sprintf("    %s: %v", key, dump.Options[key]))
			}
		}
		if dump.PromptPackReport != nil {
			rep := dump.PromptPackReport
			lines = append(lines,
				fmt.Sprintf("  Prompt pack kept:     %d", rep.IncludedMessages),
				fmt.Sprintf("  Prompt pack skipped:  %d", rep.SkippedMessages),
				fmt.Sprintf("  Mention blocks drop:  %d", rep.DroppedMentionBlocks),
				fmt.Sprintf("  Last user kept:       %t", rep.LastUserMessageIncluded),
			)
		}
		if dump.EvidencePack != nil {
			rep := dump.EvidencePack
			lines = append(lines,
				fmt.Sprintf("  Evidence packed:      %t", rep.Packed),
				fmt.Sprintf("  Evidence budget:      %d", rep.BudgetTokens),
				fmt.Sprintf("  Evidence files:       %d", rep.FilesReferenced),
				fmt.Sprintf("  Evidence raw bytes:   %d", rep.RawBytesIncluded),
				fmt.Sprintf("  Evidence omitted raw: %d", rep.RawBytesOmitted),
				fmt.Sprintf("  Evidence excerpted:   %d", rep.FilesExcerpted),
				fmt.Sprintf("  Evidence omitted:     %d", rep.FilesOmitted),
				fmt.Sprintf("  Evidence anchor:      %t", rep.AnchorAdded),
				fmt.Sprintf("  Evidence ranges:      %d", len(rep.IncludedRanges)),
			)
			for i, item := range rep.LargestOmitted {
				if i >= 3 {
					break
				}
				lines = append(lines, fmt.Sprintf("  Omitted[%d]:          %s (%s, %d bytes)", i+1, item.Path, fallbackDisplay(item.Reason, "unknown"), item.BytesOmitted))
			}
			for i, r := range rep.IncludedRanges {
				if i >= 5 {
					break
				}
				lines = append(lines, fmt.Sprintf("  Range[%d]:            %s %d-%d (%s, %d bytes)", i+1, r.Path, r.StartLine, r.EndLine, fallbackDisplay(r.Kind, "range"), r.Bytes))
			}
		}
		if strings.EqualFold(strings.TrimSpace(dump.DumpMode), "off") {
			lines = append(lines, "  [Prompt preview disabled. Set prompt_dump_mode=\"metadata\" or NANDOCODEGO_PROMPT_DUMP=metadata.]")
		}
		lines = append(lines, "  Message previews:")
		for _, msg := range dump.Messages {
			preview := msg.ContentPreview
			if preview == "" && msg.Content != "" {
				preview = msg.Content
			}
			preview = strings.ReplaceAll(preview, "\n", "\\n")
			if len(preview) > 120 {
				preview = preview[:120] + "..."
			}
			lines = append(lines, fmt.Sprintf("    #%d %s bytes=%d tokens~%d %s", msg.Index, msg.Role, msg.Bytes, msg.EstimatedTokens, preview))
		}
		return Output{Kind: OutputSystem, Content: strings.Join(lines, "\n")}
	case "save last":
		path, err := agent.SaveLatestPromptDump()
		if err != nil {
			return Output{Kind: OutputSystem, Content: "[Error: " + err.Error() + "]"}
		}
		return Output{Kind: OutputSystem, Content: "[Prompt dump saved: " + path + "]"}
	case "show last full":
		dump, ok := agent.LatestPromptDump()
		if !ok {
			return Output{Kind: OutputSystem, Content: "[No prompt dump recorded yet]"}
		}
		lines := []string{"Last prompt dump (full):"}
		for _, msg := range dump.Messages {
			lines = append(lines, fmt.Sprintf("## #%d %s", msg.Index, msg.Role))
			if msg.Content == "" {
				lines = append(lines, "[full content not available in current mode]")
				continue
			}
			lines = append(lines, msg.Content)
		}
		return Output{Kind: OutputSystem, Content: strings.Join(lines, "\n")}
	default:
		return Output{Kind: OutputSystem, Content: "[Error: Usage: /prompt last|save last|show last full]"}
	}
}

func fallbackDisplay(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func handleInit(_ context.Context, _ []string, _ HandlerContext) Output {
	dir := paths.ConfigDir()
	target := filepath.Join(dir, "config.toml")
	if _, err := os.Stat(target); err == nil {
		return Output{Kind: OutputSystem, Content: "[Config already exists at " + target + "]"}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Output{Kind: OutputSystem, Content: "[Error: " + err.Error() + "]"}
	}
	if err := os.WriteFile(target, []byte(config.DefaultConfigTOML()), 0o644); err != nil {
		return Output{Kind: OutputSystem, Content: "[Error: " + err.Error() + "]"}
	}
	return Output{Kind: OutputSystem, Content: "[Created config at " + target + "]"}
}

func handleAgents(_ context.Context, args []string, hctx HandlerContext) Output {
	if len(args) > 0 && args[0] != "list" {
		return Output{Kind: OutputSystem, Content: "[Error: Usage: /agents list]"}
	}
	allTasks := hctx.Store.Get().Tasks
	var agentTasks []types.TaskSummary
	for _, t := range allTasks {
		if t.Kind == types.KindAgent {
			agentTasks = append(agentTasks, t)
		}
	}
	if len(agentTasks) == 0 {
		return Output{Kind: OutputSystem, Content: "[No agent tasks.]"}
	}
	sort.Slice(agentTasks, func(i, j int) bool { return agentTasks[i].CreatedAt.Before(agentTasks[j].CreatedAt) })
	lines := []string{"Agent Tasks:"}
	lines = append(lines, "| ID | Status | Description | Started | Finished |")
	lines = append(lines, "|---|---|---|---|---|")
	for _, t := range agentTasks {
		started := t.CreatedAt.Format("15:04:05")
		finished := ""
		if !t.FinishedAt.IsZero() {
			finished = t.FinishedAt.Format("15:04:05")
		}
		lines = append(lines, fmt.Sprintf("| %s | %s | %s | %s | %s |", t.ID, t.Status, t.Description, started, finished))
	}
	return Output{Kind: OutputSystem, Content: strings.Join(lines, "\n")}
}

func safeMemPath(memDir, name string) (string, error) {
	if filepath.Base(name) != name || strings.Contains(name, "..") {
		return "", fmt.Errorf("invalid memory filename")
	}
	path := filepath.Join(memDir, name)
	clean := filepath.Clean(path)
	root := filepath.Clean(memDir) + string(os.PathSeparator)
	if !strings.HasPrefix(clean+string(os.PathSeparator), root) {
		return "", fmt.Errorf("path outside memory directory")
	}
	return path, nil
}

func bytesHuman(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
