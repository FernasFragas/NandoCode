// Package cli provides the command-line interface for nandocodego.
package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/FernasFragas/nandocodego/internal/agent"
	"github.com/FernasFragas/nandocodego/internal/bootstrap"
	"github.com/FernasFragas/nandocodego/internal/config"
	"github.com/FernasFragas/nandocodego/internal/credentials"
	"github.com/FernasFragas/nandocodego/internal/hooks"
	"github.com/FernasFragas/nandocodego/internal/llm"
	"github.com/FernasFragas/nandocodego/internal/llm/modelresolver"
	"github.com/FernasFragas/nandocodego/internal/llm/modelruntime"
	"github.com/FernasFragas/nandocodego/internal/llm/ollama"
	"github.com/FernasFragas/nandocodego/internal/mcp"
	"github.com/FernasFragas/nandocodego/internal/memory"
	"github.com/FernasFragas/nandocodego/internal/observability"
	"github.com/FernasFragas/nandocodego/internal/paths"
	"github.com/FernasFragas/nandocodego/internal/semantic"
	"github.com/FernasFragas/nandocodego/internal/skills"
	"github.com/FernasFragas/nandocodego/internal/state"
	"github.com/FernasFragas/nandocodego/internal/tasks"
	"github.com/FernasFragas/nandocodego/internal/tools"
	"github.com/FernasFragas/nandocodego/internal/tools/agenttool"
	"github.com/FernasFragas/nandocodego/internal/tools/builtin"
	"github.com/FernasFragas/nandocodego/internal/tools/selfinfo"
	"github.com/FernasFragas/nandocodego/internal/tools/sendmessage"
	"github.com/FernasFragas/nandocodego/internal/tools/skilltool"
	"github.com/FernasFragas/nandocodego/internal/tools/tasktool"
	"github.com/FernasFragas/nandocodego/internal/tools/todo"
	"github.com/FernasFragas/nandocodego/internal/tui"
	"github.com/FernasFragas/nandocodego/internal/types"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

// replOptions holds options for REPL execution.
type replOptions struct {
	model                     string
	ollamaURL                 string
	noAltScreen               bool
	numCtx                    int
	llmStreamIdleTimeout      string
	cloudLLMStreamIdleTimeout string
}

type startupParallelDeps struct {
	showModel      func(context.Context) (llm.ModelDetails, error)
	newSkillLoader func() (*skills.Loader, error)
	loadMCPConfig  func() (mcp.Config, []string)
	startMCP       func(context.Context, mcp.Config) (*mcp.Manager, []string)
	noteStage      func(string, time.Duration)
}

type startupParallelResult struct {
	startupMaxOutputTokens int
	startupNumCtx          int
	modelLimits            *llm.ModelLimits
	modelWarning           string
	skillLoader            *skills.Loader
	mcpMgr                 *mcp.Manager
	mcpConfigWarnings      []string
	mcpStartupWarnings     []string
}

func prepareStartupParallel(ctx context.Context, deps startupParallelDeps, defaultMaxOutputTokens, defaultNumCtx int) (startupParallelResult, error) {
	result := startupParallelResult{
		startupMaxOutputTokens: defaultMaxOutputTokens,
		startupNumCtx:          defaultNumCtx,
	}
	var mu sync.Mutex

	group, groupCtx := errgroup.WithContext(ctx)

	group.Go(func() error {
		start := time.Now()
		defer func() {
			if deps.noteStage != nil {
				deps.noteStage("startup_model_limits", time.Since(start))
			}
		}()
		if deps.showModel == nil {
			return nil
		}
		details, err := deps.showModel(groupCtx)
		if err != nil {
			mu.Lock()
			result.modelWarning = "Warning: could not fetch model limits from Ollama (" + err.Error() + "), using defaults"
			mu.Unlock()
			return nil
		}
		limits := llm.ComputeLimits(details)
		mu.Lock()
		result.modelLimits = &limits
		result.startupMaxOutputTokens = capOutputBudget(defaultMaxOutputTokens, limits.MaxOutputTokens)
		if result.startupNumCtx == 0 {
			result.startupNumCtx = limits.NumCtx
		}
		mu.Unlock()
		return nil
	})

	group.Go(func() error {
		start := time.Now()
		defer func() {
			if deps.noteStage != nil {
				deps.noteStage("startup_skills_loader", time.Since(start))
			}
		}()
		if deps.newSkillLoader == nil {
			return nil
		}
		loader, err := deps.newSkillLoader()
		if err != nil {
			return fmt.Errorf("failed to create skills loader: %w", err)
		}
		mu.Lock()
		result.skillLoader = loader
		mu.Unlock()
		return nil
	})

	group.Go(func() error {
		start := time.Now()
		defer func() {
			if deps.noteStage != nil {
				deps.noteStage("startup_mcp_bootstrap", time.Since(start))
			}
		}()
		if deps.loadMCPConfig == nil || deps.startMCP == nil {
			return nil
		}
		mcpConfig, mcpConfigWarnings := deps.loadMCPConfig()
		mcpMgr, mcpWarnings := deps.startMCP(groupCtx, mcpConfig)
		mu.Lock()
		result.mcpConfigWarnings = append([]string(nil), mcpConfigWarnings...)
		result.mcpStartupWarnings = append([]string(nil), mcpWarnings...)
		result.mcpMgr = mcpMgr
		mu.Unlock()
		return nil
	})

	if err := group.Wait(); err != nil {
		return startupParallelResult{}, err
	}
	return result, nil
}

func capOutputBudget(defaultBudget, modelMax int) int {
	if defaultBudget <= 0 {
		return modelMax
	}
	if modelMax <= 0 {
		return defaultBudget
	}
	if defaultBudget > modelMax {
		return modelMax
	}
	return defaultBudget
}

// runREPL launches the interactive REPL.
func runREPL(ctx context.Context, cmd *cobra.Command, opts replOptions) error {
	// Build bootstrap initial state
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	initial := bootstrap.DefaultInitial(wd)
	initial.LLMProvider = string(llm.ProviderOllamaLocal)
	initial.LLMBaseURL = initial.OllamaBaseURL
	startupNotices := make([]string, 0, 16)
	cfgRes, cfgErr := config.Load(
		filepath.Join(initial.ConfigDir, "config.toml"),
		filepath.Join(wd, ".nandocodego", "config.toml"),
		config.FlagOverrides{
			Model:                     ptrOrNil(opts.model),
			OllamaURL:                 ptrOrNil(opts.ollamaURL),
			LLMStreamIdleTimeout:      ptrOrNil(opts.llmStreamIdleTimeout),
			CloudLLMStreamIdleTimeout: ptrOrNil(opts.cloudLLMStreamIdleTimeout),
		},
	)
	if cfgErr == nil {
		for _, warning := range cfgRes.Warnings {
			startupNotices = append(startupNotices, "Config warning: "+warning)
		}
		initial.DefaultModel = cfgRes.Config.DefaultModel
		initial.OllamaBaseURL = cfgRes.Config.OllamaBaseURL
		initial.LLMBaseURL = cfgRes.Config.OllamaBaseURL
		initial.OllamaCloudEnabled = cfgRes.Config.OllamaCloudEnabled
		initial.KeepAlive = cfgRes.Config.ChatKeepAlive
		initial.LLMStreamIdleTimeout = cfgRes.Config.LLMStreamIdleTimeout
		initial.CloudLLMStreamIdleTimeout = cfgRes.Config.CloudLLMStreamIdleTimeout
		initial.PermissionMode = cfgRes.Config.PermissionMode
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
		initial.SlowStageNoticeThreshold = cfgRes.Config.SlowStageNoticeThreshold
		initial.SlowStageNoticeThresholdSource = cfgRes.Sources.SlowStageNoticeThreshold
		initial.ContextMode = cfgRes.Config.ContextMode
		initial.MemoryRecallMode = cfgRes.Config.MemoryRecallMode
	} else {
		startupNotices = append(startupNotices, "Config warning: failed to load config.toml, using defaults ("+cfgErr.Error()+")")
	}

	// Apply CLI flag overrides
	if opts.model != "" {
		initial.DefaultModel = opts.model
	}
	if opts.ollamaURL != "" {
		initial.OllamaBaseURL = opts.ollamaURL
		initial.LLMBaseURL = opts.ollamaURL
	}
	if opts.numCtx > 0 {
		initial.NumCtx = opts.numCtx
	}

	// Initialize bootstrap before any Global call so config and flag overrides apply.
	bootstrap.InitGlobal(initial)
	snap := bootstrap.Global().Snapshot()

	// Build app state
	appState := state.DefaultApp(snap)
	appState.TodoList = todo.NewTodoList()
	scopeRoot, err := memory.ScopeRoot(snap.WorkingDir, snap.GitRoot)
	if err == nil {
		memDir := memory.DirForScope(scopeRoot)
		appState.ToolSettings.AdditionalWorkingDirs = append(appState.ToolSettings.AdditionalWorkingDirs, memDir)
	}
	store := state.NewStore(appState, state.OnChange)
	taskSupervisor := tasks.NewSupervisor(paths.SessionTasksDir(snap.SessionID), store)

	// Build local Ollama client and a switchable runtime router.
	localClient := ollama.NewClient(snap.OllamaBaseURL)
	runtimeClient := llm.NewRuntimeClient(localClient, llm.ProviderOllamaLocal, snap.OllamaBaseURL)
	meter := observability.NewMeter()
	bridge, telemetrySettings := observability.NewBridgeFromEnv(nil)
	client := observability.WrapLLMClient(runtimeClient, meter, bridge)
	defer bridge.Shutdown(ctx)
	if telemetrySettings.Warning != "" {
		startupNotices = append(startupNotices, "Telemetry warning: "+telemetrySettings.Warning)
	}

	compactionCfg := agent.DefaultCompactionConfig()
	modelCap := llm.ModelCapabilities(snap.DefaultModel)
	if modelCap.RecommendedNumCtx > 0 {
		compactionCfg.MaxContextTokens = int64(modelCap.RecommendedNumCtx)
	}

	parallelStartupStartedAt := time.Now()
	parallelStartup, err := prepareStartupParallel(ctx, startupParallelDeps{
		showModel: func(ctx context.Context) (llm.ModelDetails, error) {
			return localClient.ShowModel(ctx, snap.DefaultModel)
		},
		newSkillLoader: func() (*skills.Loader, error) {
			return skills.NewLoader(paths.SkillsDir(), filepath.Join(snap.WorkingDir, paths.ProjectSkillsDir()), skills.BundledFS)
		},
		loadMCPConfig: func() (mcp.Config, []string) {
			return mcp.LoadConfig(
				filepath.Join(snap.ConfigDir, "config.toml"),
				filepath.Join(snap.WorkingDir, ".nandocodego", "config.toml"),
			)
		},
		startMCP: func(ctx context.Context, cfg mcp.Config) (*mcp.Manager, []string) {
			return mcp.Start(ctx, cfg)
		},
		noteStage: meter.NotePendingRunStage,
	}, snap.MaxOutputTokens, snap.NumCtx)
	if err != nil {
		return err
	}
	meter.NotePendingRunStage("startup_parallel_prepare", time.Since(parallelStartupStartedAt))

	startupMaxOutputTokens := parallelStartup.startupMaxOutputTokens
	startupNumCtx := parallelStartup.startupNumCtx
	if parallelStartup.modelLimits != nil {
		limits := parallelStartup.modelLimits
		store.Set(func(app state.App) state.App {
			app.MaxOutputTokens = startupMaxOutputTokens
			app.RuntimeNumCtx = startupNumCtx
			app.ToolSettings.MaxResultChars = limits.MaxResultChars
			return app
		})
	} else {
		if parallelStartup.modelWarning != "" {
			startupNotices = append(startupNotices, parallelStartup.modelWarning)
		}
		store.Set(func(app state.App) state.App {
			app.RuntimeNumCtx = startupNumCtx
			return app
		})
	}

	agentCfg := agent.Config{
		MaxTurns:           snap.MaxTurns,
		MaxConcurrentTools: snap.MaxConcurrentTools,
		MaxOutputTokens:    startupMaxOutputTokens,
		LengthRetryTokens:  snap.LengthRetryTokens,
		ChatKeepAlive:      snap.KeepAlive,
		NumCtx:             startupNumCtx,
		PermissionObserver: observability.PermissionObserver(meter, bridge),
		ToolBatchObserver: func(batchSize int, safe bool, duration time.Duration) {
			meter.RecordToolBatch(batchSize, safe, duration)
			bridge.RecordToolBatch(batchSize, safe, duration)
		},
		Watchdog:         llm.WithIdleTimeout(llm.DefaultWatchdogConfig(), snap.LLMStreamIdleTimeout),
		CloudWatchdog:    llm.WithIdleTimeout(llm.DefaultCloudWatchdogConfig(), snap.CloudLLMStreamIdleTimeout),
		Compaction:       compactionCfg,
		ContextMode:      snap.ContextMode,
		ContextMinNumCtx: 8192,
		ContextMaxNumCtx: 0,
		ContextReserve:   4096,
	}
	// Build built-in tool registry
	registry, err := builtin.NewRegistry()
	if err != nil {
		return fmt.Errorf("failed to create tool registry: %w", err)
	}
	skillLoader := parallelStartup.skillLoader
	if skillLoader == nil {
		return fmt.Errorf("failed to create skills loader: startup returned nil loader")
	}
	defer skillLoader.Close()
	if err := registry.Register(skilltool.New(skillLoader)); err != nil {
		return fmt.Errorf("failed to register skill tool: %w", err)
	}
	for _, warning := range parallelStartup.mcpConfigWarnings {
		fmt.Fprintf(os.Stderr, "MCP config warning: %s\n", warning)
		startupNotices = append(startupNotices, "MCP config warning: "+warning)
	}
	mcpMgr := parallelStartup.mcpMgr
	if mcpMgr == nil {
		mcpMgr = &mcp.Manager{}
	}
	for _, warning := range parallelStartup.mcpStartupWarnings {
		fmt.Fprintf(os.Stderr, "MCP startup warning: %s\n", warning)
		startupNotices = append(startupNotices, "MCP startup warning: "+warning)
	}
	for _, warning := range mcpMgr.RegisterInto(registry) {
		fmt.Fprintf(os.Stderr, "MCP registry warning: %s\n", warning)
		startupNotices = append(startupNotices, "MCP registry warning: "+warning)
	}
	defer mcpMgr.Close()

	observedRegistry, err := observability.WrapRegistry(registry, meter, bridge)
	if err != nil {
		return fmt.Errorf("failed to wrap tool registry for observability: %w", err)
	}
	registry = observedRegistry
	taskTools := tasktool.NewWithAgent(taskSupervisor, client, registry, agentCfg, snap.SessionID, func() string { return store.Get().ActiveModel }, func() string { return store.Get().LLMProvider })
	var taskStopTool tools.Tool
	for _, tt := range taskTools {
		if err := registry.Register(observability.WrapTool(tt, meter, bridge)); err != nil {
			return fmt.Errorf("failed to register task tool %s: %w", tt.Name(), err)
		}
		if tt.Name() == "TaskStop" {
			taskStopTool = tt
		}
	}
	workerRegistry := registry
	if agent.IsCoordinatorMode() {
		workerRegistry = agent.BuildWorkerRegistry(registry)
	}
	agentTool := agenttool.New(client, workerRegistry, agentCfg, snap.SessionID, func() string { return store.Get().ActiveModel }, func() string { return store.Get().LLMProvider })
	agentTool.SetSupervisor(taskSupervisor)
	if err := registry.Register(observability.WrapTool(agentTool, meter, bridge)); err != nil {
		return fmt.Errorf("failed to register agent tool: %w", err)
	}
	sendMessageTool := sendmessage.New(taskSupervisor, nil)
	sendMessageTool = sendmessage.New(taskSupervisor, nil, sendmessage.WithResumeFunc(func(tctx tools.Context, taskID string, msg tasks.PendingMessage) (string, error) {
		st, ok := taskSupervisor.Get(taskID)
		if !ok {
			return "", fmt.Errorf("task %s not found", taskID)
		}
		sum := st.ToSummary()
		if sum.Kind != types.KindAgent {
			return "", fmt.Errorf("task %s is not an agent task", taskID)
		}
		replayed, _ := tasks.ReplayMessagesFromOutput(sum.OutputFile, 50)
		var replay strings.Builder
		for _, msgLine := range replayed {
			content := strings.TrimSpace(msgLine.Content)
			thinking := strings.TrimSpace(msgLine.Thinking)
			if content == "" && thinking == "" {
				continue
			}
			role := string(msgLine.Role)
			if role == "" {
				role = "unknown"
			}
			replay.WriteString("[")
			replay.WriteString(role)
			replay.WriteString("] ")
			if thinking != "" {
				replay.WriteString("(thinking) ")
				replay.WriteString(thinking)
				replay.WriteString(" ")
			}
			if content != "" {
				replay.WriteString(content)
			}
			replay.WriteString("\n")
		}
		if replay.Len() == 0 {
			lines, _ := tasks.TailLines(sum.OutputFile, 200)
			for _, line := range lines {
				if strings.TrimSpace(line.Text) == "" {
					continue
				}
				replay.WriteString(line.Text)
				if !strings.HasSuffix(line.Text, "\n") {
					replay.WriteString("\n")
				}
			}
		}
		if replay.Len() == 0 {
			replay.WriteString("(no replay records available)")
			if !strings.HasSuffix(replay.String(), "\n") {
				replay.WriteString("\n")
			}
		}
		resumeTask := fmt.Sprintf(
			"Resume previous worker context.\nOriginal task ID: %s\nOriginal description: %s\nPrior execution log tail:\n%s\nIncoming message:\n%s",
			taskID,
			sum.Description,
			strings.TrimSpace(replay.String()),
			msg.Content,
		)
		taskIDCh := make(chan string, 1)
		newID, err := taskSupervisor.Start(
			tctx.EffectiveContext(),
			types.KindAgent,
			sum.Description+" (resumed)",
			tasks.AgentRunFuncWithMailbox(client, workerRegistry, agentCfg, snap.SessionID, tctx, resumeTask, store.Get().ActiveModel, store.Get().LLMProvider, taskSupervisor, taskIDCh),
		)
		if err != nil {
			return "", err
		}
		taskIDCh <- newID
		return newID, nil
	}))
	if err := registry.Register(observability.WrapTool(sendMessageTool, meter, bridge)); err != nil {
		return fmt.Errorf("failed to register sendmessage tool: %w", err)
	}
	selfInfoTool := selfinfo.New(client, func() string { return store.Get().ActiveModel }, agentCfg.MaxTurns)
	if err := registry.Register(observability.WrapTool(selfInfoTool, meter, bridge)); err != nil {
		return fmt.Errorf("failed to register selfinfo tool: %w", err)
	}
	activeRegistry := registry
	if agent.IsCoordinatorMode() {
		activeRegistry = agent.BuildCoordinatorRegistry(agentTool, sendMessageTool, taskStopTool)
		store.Set(func(app state.App) state.App {
			app.CoordinatorMode = true
			return app
		})
		startupNotices = append(startupNotices, "[coordinator] mode active - direct file tools disabled")
	}

	// Build agent
	agentRunner, err := agent.New(client, activeRegistry, agent.WithConfig(agentCfg))
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	memCfg := memory.DefaultConfig(snap.DefaultModel)
	memCfg.RecallMode = snap.MemoryRecallMode
	memoryRunner := memory.NewRunner(agentRunner, client, memCfg)
	hookSnapshot := hooks.LoadSnapshot(hooks.LoadOptions{
		UserPath:    filepath.Join(snap.ConfigDir, "hooks.json"),
		ProjectPath: filepath.Join(snap.WorkingDir, ".nandocodego", "hooks.json"),
	})
	hookUserPath := filepath.Join(snap.ConfigDir, "hooks.json")
	hookProjectPath := filepath.Join(snap.WorkingDir, ".nandocodego", "hooks.json")
	hookDispatcher := hooks.NewDispatcher(hookSnapshot, client, hooks.Config{
		SessionID:      snap.SessionID,
		Model:          snap.DefaultModel,
		PermissionMode: snap.PermissionMode.String(),
		WorkingDir:     snap.WorkingDir,
	})
	hookRunner := hooks.NewRunner(memoryRunner, hookDispatcher)
	observedRunner := observability.WrapRunner(hookRunner, meter, bridge)
	memDir := ""
	if scopeRoot, err := memory.ScopeRoot(snap.WorkingDir, snap.GitRoot); err == nil {
		memDir = memory.DirForScope(scopeRoot)
		if err := os.MkdirAll(filepath.Join(memDir, "pending"), 0o700); err != nil {
			startupNotices = append(startupNotices, "Warning: could not create memory directory: "+err.Error())
			memDir = ""
		}
	}
	modelRuntimeSvc := &modelruntime.Service{
		LocalClient:  localClient,
		LocalBaseURL: snap.OllamaBaseURL,
		Runtime:      runtimeClient,
		Resolver: &modelresolver.Resolver{
			LocalClient:  localClient,
			CloudClient:  ollama.NewClient(llm.OllamaCloudBaseURL),
			CloudEnabled: snap.OllamaCloudEnabled,
		},
		Creds: credentials.NewResolver(),
	}
	semanticCfg := semantic.DefaultConfig()
	if cfgErr == nil {
		semanticCfg = cfgRes.Config.SemanticIndex
	}
	semanticStore := semantic.NewLocalStore(paths.CacheDir())
	semanticSvc := semantic.NewLocalService(semanticStore, semantic.LLMEmbedder{Client: client})

	// Build TUI model
	tuiModel, err := tui.New(store, observedRunner, nil, skillLoader, client, &hookSnapshot, hookDispatcher.SetSnapshot, memDir, hookUserPath, hookProjectPath, startupNotices)
	if err != nil {
		return fmt.Errorf("failed to create TUI model: %w", err)
	}
	if agent.IsDreamEnabled() {
		tuiModel.SetDreamHooks(
			func() { _ = taskSupervisor.KillDream() },
			func() string { return taskSupervisor.ConsumeDreamResult(30 * time.Second) },
			func() {
				if taskSupervisor.ActiveWorkerCount() > 0 {
					return
				}
				_, _ = taskSupervisor.SpawnDream(context.Background(), client, store.Get().ActiveModel, "Speculative follow-up analysis.")
			},
		)
	}
	tuiModel.SetMeter(meter)
	tuiModel.SetModelRuntime(modelRuntimeSvc)
	tuiModel.SetSemanticService(semanticSvc, semanticCfg)

	// Build Bubble Tea program
	teaOpts := []tea.ProgramOption{
		tea.WithFPS(60),
	}
	if !opts.noAltScreen {
		teaOpts = append(teaOpts, tea.WithAltScreen())
	}

	program := tea.NewProgram(tuiModel, teaOpts...)

	// Wire the program sender
	programSender := &realProgramSender{program: program}
	tuiModel.SetProgramSender(programSender)

	// Run the program
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("REPL failed: %w", err)
	}

	return nil
}

// realProgramSender wraps tea.Program.Send for production use.
type realProgramSender struct {
	program *tea.Program
}

// Send sends a message to the program.
func (s *realProgramSender) Send(msg tea.Msg) {
	s.program.Send(msg)
}
