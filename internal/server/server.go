package server

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/bootstrap"
	"github.com/FernasFragas/Nandocode/internal/config"
	"github.com/FernasFragas/Nandocode/internal/credentials"
	"github.com/FernasFragas/Nandocode/internal/hooks"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/llm/modelresolver"
	"github.com/FernasFragas/Nandocode/internal/llm/modelruntime"
	"github.com/FernasFragas/Nandocode/internal/llm/ollama"
	"github.com/FernasFragas/Nandocode/internal/mcp"
	"github.com/FernasFragas/Nandocode/internal/memory"
	"github.com/FernasFragas/Nandocode/internal/observability"
	"github.com/FernasFragas/Nandocode/internal/paths"
	"github.com/FernasFragas/Nandocode/internal/semantic"
	"github.com/FernasFragas/Nandocode/internal/state"
	"github.com/FernasFragas/Nandocode/internal/tasks"
	"github.com/FernasFragas/Nandocode/internal/tools"
	"github.com/FernasFragas/Nandocode/internal/tools/agenttool"
	"github.com/FernasFragas/Nandocode/internal/tools/builtin"
	"github.com/FernasFragas/Nandocode/internal/tools/sendmessage"
	"github.com/FernasFragas/Nandocode/internal/tools/tasktool"
	"github.com/FernasFragas/Nandocode/internal/types"
)

//go:embed web/index.html
var webUI embed.FS

type Config struct {
	Bind                      string
	Port                      int
	Token                     string
	NoUI                      bool
	Model                     string
	OllamaURL                 string
	NumCtx                    int
	LLMStreamIdleTimeout      string
	CloudLLMStreamIdleTimeout string
	MaxSessions               int
	IdleTimeout               time.Duration
	ReadTimeout               time.Duration
	WriteTimeout              time.Duration
}

type Server struct {
	cfg          Config
	logger       *slog.Logger
	ctx          context.Context
	cancel       context.CancelFunc
	client       llm.Client
	modelRuntime *modelruntime.Service
	runner       AgentRunner
	agentCfg     agent.Config
	baseApp      state.App
	baseTools    *tools.Registry
	semanticCfg  semantic.Config
	semanticSvc  semantic.Service
	registry     *sessionRegistry
	limiter      *RateLimiter
	httpServer   *http.Server
}

func New(ctx context.Context, logger *slog.Logger, cfg Config) (*Server, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.Bind == "" {
		cfg.Bind = "127.0.0.1"
	}
	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	if cfg.MaxSessions == 0 {
		cfg.MaxSessions = 10
	}
	if cfg.IdleTimeout == 0 {
		cfg.IdleTimeout = 30 * time.Minute
	}
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 30 * time.Second
	}
	if err := validateNonLoopback(cfg.Bind, cfg.Token); err != nil {
		return nil, err
	}
	cctx, cancel := context.WithCancel(ctx)

	wd, err := os.Getwd()
	if err != nil {
		cancel()
		return nil, err
	}
	init := bootstrap.DefaultInitial(wd)
	init.LLMProvider = string(llm.ProviderOllamaLocal)
	init.LLMBaseURL = init.OllamaBaseURL
	cfgRes, loadErr := config.Load(filepath.Join(init.ConfigDir, "config.toml"), filepath.Join(init.WorkingDir, ".nandocodego", "config.toml"), config.FlagOverrides{
		Model:                     ptrOrNil(cfg.Model),
		OllamaURL:                 ptrOrNil(cfg.OllamaURL),
		LLMStreamIdleTimeout:      ptrOrNil(cfg.LLMStreamIdleTimeout),
		CloudLLMStreamIdleTimeout: ptrOrNil(cfg.CloudLLMStreamIdleTimeout),
	})
	if loadErr == nil {
		init.DefaultModel = cfgRes.Config.DefaultModel
		init.OllamaBaseURL = cfgRes.Config.OllamaBaseURL
		init.LLMBaseURL = cfgRes.Config.OllamaBaseURL
		init.OllamaCloudEnabled = cfgRes.Config.OllamaCloudEnabled
		init.KeepAlive = cfgRes.Config.ChatKeepAlive
		init.LLMStreamIdleTimeout = cfgRes.Config.LLMStreamIdleTimeout
		init.CloudLLMStreamIdleTimeout = cfgRes.Config.CloudLLMStreamIdleTimeout
		init.PermissionMode = cfgRes.Config.PermissionMode
		init.MaxTurns = cfgRes.Config.MaxTurns
		init.MaxConcurrentTools = cfgRes.Config.MaxConcurrentTools
		init.ContextMode = cfgRes.Config.ContextMode
		init.MemoryRecallMode = cfgRes.Config.MemoryRecallMode
	}
	semanticCfg := semantic.DefaultConfig()
	if loadErr == nil {
		semanticCfg = cfgRes.Config.SemanticIndex
	}
	if cfg.Model != "" {
		init.DefaultModel = cfg.Model
	}
	if cfg.OllamaURL != "" {
		init.OllamaBaseURL = cfg.OllamaURL
		init.LLMBaseURL = cfg.OllamaURL
	}
	if cfg.NumCtx > 0 {
		init.NumCtx = cfg.NumCtx
	}

	localClient := ollama.NewClient(init.OllamaBaseURL)
	runtimeClient := llm.NewRuntimeClient(localClient, llm.ProviderOllamaLocal, init.OllamaBaseURL)
	meter := observability.NewMeter()
	bridge, _ := observability.NewBridgeFromEnv(nil)
	client := observability.WrapLLMClient(runtimeClient, meter, bridge)
	modelRuntimeSvc := &modelruntime.Service{
		LocalClient:  localClient,
		LocalBaseURL: init.OllamaBaseURL,
		Runtime:      runtimeClient,
		Resolver: &modelresolver.Resolver{
			LocalClient:  localClient,
			CloudClient:  ollama.NewClient(llm.OllamaCloudBaseURL),
			CloudEnabled: init.OllamaCloudEnabled,
		},
		Creds: credentials.NewResolver(),
	}
	registry, err := builtin.NewRegistry()
	if err != nil {
		cancel()
		return nil, err
	}
	mcpConfig, _ := mcp.LoadConfig(filepath.Join(init.ConfigDir, "config.toml"), filepath.Join(init.WorkingDir, ".nandocodego", "config.toml"))
	mcpMgr, _ := mcp.Start(cctx, mcpConfig)
	for _, w := range mcpMgr.RegisterInto(registry) {
		logger.Warn("mcp warning", "warning", w)
	}

	agentCfg := agent.DefaultConfig()
	agentCfg.MaxTurns = init.MaxTurns
	agentCfg.MaxConcurrentTools = init.MaxConcurrentTools
	agentCfg.MaxOutputTokens = init.MaxOutputTokens
	agentCfg.ChatKeepAlive = init.KeepAlive
	agentCfg.NumCtx = init.NumCtx
	agentCfg.ContextMode = init.ContextMode
	agentCfg.Watchdog = llm.WithIdleTimeout(llm.DefaultWatchdogConfig(), init.LLMStreamIdleTimeout)
	agentCfg.CloudWatchdog = llm.WithIdleTimeout(llm.DefaultCloudWatchdogConfig(), init.CloudLLMStreamIdleTimeout)
	rawRunner, err := agent.New(client, registry, agent.WithConfig(agentCfg))
	if err != nil {
		cancel()
		return nil, err
	}
	memCfg := memory.DefaultConfig(init.DefaultModel)
	memCfg.RecallMode = init.MemoryRecallMode
	memoryRunner := memory.NewRunner(rawRunner, client, memCfg)
	hookSnapshot := hooks.LoadSnapshot(hooks.LoadOptions{UserPath: filepath.Join(init.ConfigDir, "hooks.json"), ProjectPath: filepath.Join(init.WorkingDir, ".nandocodego", "hooks.json")})
	hookDispatcher := hooks.NewDispatcher(hookSnapshot, client, hooks.Config{SessionID: init.SessionID, Model: init.DefaultModel, PermissionMode: init.PermissionMode.String(), WorkingDir: init.WorkingDir})
	runner := hooks.NewRunner(memoryRunner, hookDispatcher)

	bootstrap.InitGlobal(init)
	app := state.DefaultApp(bootstrap.Global().Snapshot())
	app.CoordinatorMode = agent.IsCoordinatorMode()
	if scopeRoot, err := memory.ScopeRoot(init.WorkingDir, init.GitRoot); err == nil {
		app.ToolSettings.AdditionalWorkingDirs = append(app.ToolSettings.AdditionalWorkingDirs, memory.DirForScope(scopeRoot))
	}

	s := &Server{
		cfg:          cfg,
		logger:       logger,
		ctx:          cctx,
		cancel:       cancel,
		client:       client,
		modelRuntime: modelRuntimeSvc,
		runner:       runner,
		agentCfg:     agentCfg,
		baseApp:      app,
		baseTools:    registry,
		semanticCfg:  semanticCfg,
		semanticSvc:  semantic.NewLocalService(semantic.NewLocalStore(paths.CacheDir()), semantic.LLMEmbedder{Client: client}),
		registry:     newSessionRegistry(),
		limiter:      NewRateLimiter(100, cfg.MaxSessions),
	}
	s.httpServer = &http.Server{Addr: cfg.Bind + ":" + strconv.Itoa(cfg.Port), Handler: s.routes(), ReadTimeout: cfg.ReadTimeout, WriteTimeout: cfg.WriteTimeout}
	go s.sweepIdle()
	return s, nil
}

func (s *Server) buildCoordinatorSessionRunner(sessionID string, app state.App) (AgentRunner, *tasks.Supervisor, *state.Store[state.App], error) {
	store := state.NewStore(app, nil)
	supervisor := tasks.NewSupervisor(paths.SessionTasksDir(sessionID), store)
	workerRegistry := agent.BuildWorkerRegistry(s.baseTools)
	agentTool := agenttool.New(s.client, workerRegistry, s.agentCfg, sessionID, func() string { return app.ActiveModel }, func() string { return app.LLMProvider })
	agentTool.SetSupervisor(supervisor)
	sendMsg := sendmessage.New(supervisor, nil, sendmessage.WithResumeFunc(func(tctx tools.Context, taskID string, msg tasks.PendingMessage) (string, error) {
		st, ok := supervisor.Get(taskID)
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
			taskID, sum.Description, strings.TrimSpace(replay.String()), msg.Content,
		)
		taskIDCh := make(chan string, 1)
		newID, err := supervisor.Start(
			tctx.EffectiveContext(),
			types.KindAgent,
			sum.Description+" (resumed)",
			tasks.AgentRunFuncWithMailbox(s.client, workerRegistry, s.agentCfg, sessionID, tctx, resumeTask, app.ActiveModel, app.LLMProvider, supervisor, taskIDCh),
		)
		if err != nil {
			return "", err
		}
		taskIDCh <- newID
		return newID, nil
	}))
	var taskStop tools.Tool
	for _, tt := range tasktool.NewWithAgent(supervisor, s.client, workerRegistry, s.agentCfg, sessionID, func() string { return app.ActiveModel }, func() string { return app.LLMProvider }) {
		if tt.Name() == "TaskStop" {
			taskStop = tt
			break
		}
	}
	reg := agent.BuildCoordinatorRegistry(agentTool, sendMsg, taskStop)
	coordRunner, err := agent.New(s.client, reg, agent.WithConfig(s.agentCfg))
	if err != nil {
		return nil, nil, nil, err
	}
	return coordRunner, supervisor, store, nil
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CSP: unsafe-inline is required for the embedded SPA which uses inline CSS/JS.
		// This is acceptable because the server binds to localhost by default.
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; connect-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self';")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		if strings.HasPrefix(r.URL.Path, "/v1/") {
			w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/health", s.handleHealth)
	mux.HandleFunc("/v1/models", s.handleModels)
	mux.HandleFunc("/v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			s.handleCreateSession(w, r)
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/v1/sessions/", s.sessionRoutes)
	if !s.cfg.NoUI {
		sub, err := fs.Sub(webUI, "web")
		if err == nil {
			mux.Handle("/", http.FileServer(http.FS(sub)))
		}
	}
	return NewAuthMiddleware(s.cfg.Token, securityHeaders(mux))
}

func (s *Server) sessionRoutes(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
	parts := strings.Split(rest, "/")
	if len(parts) < 1 || strings.TrimSpace(parts[0]) == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			s.handleGetSession(w, r, id)
		case http.MethodDelete:
			s.handleDeleteSession(w, r, id)
		default:
			http.NotFound(w, r)
		}
		return
	}
	switch parts[1] {
	case "events":
		if r.Method == http.MethodGet {
			s.handleEvents(w, r, id)
			return
		}
	case "messages":
		if r.Method == http.MethodPost {
			s.handlePostMessage(w, r, id)
			return
		}
	case "permissions":
		if r.Method == http.MethodPost && len(parts) >= 3 {
			s.handleResolvePermission(w, r, id, parts[2])
			return
		}
	case "model":
		if r.Method == http.MethodPost {
			s.handleUpdateModel(w, r, id)
			return
		}
	case "tree":
		if r.Method == http.MethodGet {
			s.handleGetTree(w, r, id)
			return
		}
	}
	http.NotFound(w, r)
}

func (s *Server) sweepIdle() {
	t := time.NewTicker(1 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-t.C:
			now := time.Now().UTC()
			for _, sess := range s.registry.all() {
				view := sess.view()
				if now.Sub(view.LastActive) > s.cfg.IdleTimeout {
					sess.stop()
					s.registry.del(view.SessionID)
					s.limiter.ReleaseSession()
				}
			}
		}
	}
}

func (s *Server) Start() error {
	s.logger.Info("server starting", "addr", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.cancel()
	for _, sess := range s.registry.all() {
		sess.stop()
	}
	return s.httpServer.Shutdown(ctx)
}

func RunUntilSignal(ctx context.Context, logger *slog.Logger, cfg Config) error {
	srv, err := New(ctx, logger, cfg)
	if err != nil {
		return err
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	select {
	case <-ctx.Done():
	case <-sigCh:
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	startErr := <-errCh
	if startErr != nil && !errors.Is(startErr, http.ErrServerClosed) {
		return startErr
	}
	return nil
}

func ptrOrNil(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	v := s
	return &v
}
