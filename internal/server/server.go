package server

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/bootstrap"
	"github.com/FernasFragas/Nandocode/internal/bootstrap/applyconfig"
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

//go:embed web
var webUI embed.FS

type Config struct {
	Bind                      string
	Port                      int
	Token                     string
	NoUI                      bool
	Dev                       bool
	Model                     string
	OllamaURL                 string
	NumCtx                    int
	LLMStreamIdleTimeout      string
	CloudLLMStreamIdleTimeout string
	MaxSessions               int
	IdleTimeout               time.Duration
	ReadTimeout               time.Duration
	WriteTimeout              time.Duration

	// tokenGenerated records that New created Token because none was given.
	tokenGenerated bool
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
	hookSnapshot hooks.Snapshot
	hookCfg      hooks.Config
	indexRuns    indexRunRegistry
	registry     *sessionRegistry
	limiter      *RateLimiter
	httpServer   *http.Server

	// sessionRuntimes builds the per-session client and runner stack.
	sessionRuntimes sessionRuntimeFactory

	// closeMCP stops the MCP manager's stdio children and HTTP sessions.
	closeMCP  func()
	closeOnce sync.Once
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
	// The API always requires a bearer token. Without --token (loopback only,
	// enforced above) a random one is generated and announced at startup.
	if strings.TrimSpace(cfg.Token) == "" {
		token, err := GenerateToken()
		if err != nil {
			return nil, fmt.Errorf("generate server token: %w", err)
		}
		cfg.Token = token
		cfg.tokenGenerated = true
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
		applyconfig.ApplyConfig(&init, cfgRes.Config, cfgRes.Sources.SlowStageNoticeThreshold)
	}
	semanticCfg := semantic.DefaultConfig()
	if loadErr == nil {
		semanticCfg = cfgRes.Config.SemanticIndex
	}
	bootstrap.ApplyOverrides(&init, bootstrap.Overrides{Model: cfg.Model, OllamaURL: cfg.OllamaURL, NumCtx: cfg.NumCtx})

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
	memCfg := memory.DefaultConfig(init.DefaultModel)
	memCfg.RecallMode = init.MemoryRecallMode
	hookSnapshot := hooks.LoadSnapshot(hooks.LoadOptions{UserPath: filepath.Join(init.ConfigDir, "hooks.json"), ProjectPath: filepath.Join(init.WorkingDir, ".nandocodego", "hooks.json")})
	sessionRuntimes := newSessionRuntimeFactory(sessionRuntimeDeps{
		meter:        meter,
		bridge:       bridge,
		registry:     registry,
		agentCfg:     agentCfg,
		memoryCfg:    memCfg,
		hookSnapshot: hookSnapshot,
		hookCfg:      hooks.Config{SessionID: init.SessionID, Model: init.DefaultModel, PermissionMode: init.PermissionMode.String(), WorkingDir: init.WorkingDir},
	})
	// The base runtime serves health checks, embeddings and server-level
	// work. It always stays on the local daemon: sessions switch their own.
	_, runner, err := sessionRuntimes(runtimeClient)
	if err != nil {
		mcpMgr.Close()
		cancel()
		return nil, err
	}

	bootstrap.InitGlobal(init)
	app := state.DefaultApp(bootstrap.Global().Snapshot())
	app.CoordinatorMode = agent.IsCoordinatorMode()
	if scopeRoot, err := memory.ScopeRoot(init.WorkingDir, init.GitRoot); err == nil {
		app.ToolSettings.AdditionalWorkingDirs = append(app.ToolSettings.AdditionalWorkingDirs, memory.DirForScope(scopeRoot))
	}

	s := &Server{
		cfg:             cfg,
		logger:          logger,
		ctx:             cctx,
		cancel:          cancel,
		client:          client,
		modelRuntime:    modelRuntimeSvc,
		runner:          runner,
		sessionRuntimes: sessionRuntimes,
		closeMCP:        mcpMgr.Close,
		agentCfg:        agentCfg,
		baseApp:         app,
		baseTools:       registry,
		semanticCfg:     semanticCfg,
		semanticSvc:     semantic.NewLocalService(semantic.NewLocalStore(paths.CacheDir()), semantic.LLMEmbedder{Client: localClient}),
		hookSnapshot:    hookSnapshot,
		hookCfg:         hooks.Config{SessionID: init.SessionID, Model: init.DefaultModel, PermissionMode: init.PermissionMode.String(), WorkingDir: init.WorkingDir},
		registry:        newSessionRegistry(),
		limiter:         NewRateLimiter(100, cfg.MaxSessions),
	}
	s.httpServer = &http.Server{Addr: cfg.Bind + ":" + strconv.Itoa(cfg.Port), Handler: s.routes(), ReadTimeout: cfg.ReadTimeout, WriteTimeout: cfg.WriteTimeout}
	go s.sweepIdle()
	return s, nil
}

func (s *Server) buildCoordinatorSessionRunner(client llm.Client, sessionID string, app state.App) (AgentRunner, *tasks.Supervisor, *state.Store[state.App], error) {
	store := state.NewStore(app, nil)
	supervisor := tasks.NewSupervisor(paths.SessionTasksDir(sessionID), store)
	workerRegistry := agent.BuildWorkerRegistry(s.baseTools)
	agentTool := agenttool.New(client, workerRegistry, s.agentCfg, sessionID, func() string { return app.ActiveModel }, func() string { return app.LLMProvider })
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
			tasks.AgentRunFuncWithMailbox(client, workerRegistry, s.agentCfg, sessionID, tctx, resumeTask, app.ActiveModel, app.LLMProvider, supervisor, taskIDCh),
		)
		if err != nil {
			return "", err
		}
		supervisor.SetWorkerMeta(newID, tasks.WorkerMeta{Task: resumeTask, Model: app.ActiveModel, Parent: "coordinator"})
		taskIDCh <- newID
		return newID, nil
	}))
	var taskStop tools.Tool
	for _, tt := range tasktool.NewWithAgent(supervisor, client, workerRegistry, s.agentCfg, sessionID, func() string { return app.ActiveModel }, func() string { return app.LLMProvider }) {
		if tt.Name() == "TaskStop" {
			taskStop = tt
			break
		}
	}
	reg := agent.BuildCoordinatorRegistry(agentTool, sendMsg, taskStop)
	coordRunner, err := agent.New(client, reg, agent.WithConfig(s.agentCfg))
	if err != nil {
		return nil, nil, nil, err
	}
	return coordRunner, supervisor, store, nil
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CSP: the embedded UI has no inline script or style (BV-2), so neither
		// 'unsafe-inline' nor 'unsafe-eval' is allowed.
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; connect-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self';")
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

// staticJSContentType pins the JavaScript content type for ES modules so the
// result does not depend on the host's MIME database (browsers refuse module
// scripts served as text/plain).
func staticJSContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".js") {
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		}
		next.ServeHTTP(w, r)
	})
}

// sessionHandler adapts a per-session handler to a pattern route that
// declares the {id} path wildcard.
func sessionHandler(h func(w http.ResponseWriter, r *http.Request, id string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h(w, r, r.PathValue("id"))
	}
}

// routes is the single route table. Adding an endpoint is one line: register
// "METHOD /path/{wildcard}" here. A known path with the wrong method answers
// 405 with an Allow header; unknown paths answer 404.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", s.handleHealth)
	mux.HandleFunc("GET /v1/models", s.handleModels)
	mux.HandleFunc("POST /v1/sessions", s.handleCreateSession)
	mux.HandleFunc("GET /v1/sessions", s.handleListSessions)
	mux.HandleFunc("POST /v1/credentials/ollama-cloud", s.handleOllamaCloudCredential)
	mux.HandleFunc("GET /v1/sessions/{id}", sessionHandler(s.handleGetSession))
	mux.HandleFunc("DELETE /v1/sessions/{id}", sessionHandler(s.handleDeleteSession))
	mux.HandleFunc("GET /v1/sessions/{id}/events", sessionHandler(s.handleEvents))
	mux.HandleFunc("POST /v1/sessions/{id}/messages", sessionHandler(s.handlePostMessage))
	mux.HandleFunc("POST /v1/sessions/{id}/permissions/{reqID}", func(w http.ResponseWriter, r *http.Request) {
		s.handleResolvePermission(w, r, r.PathValue("id"), r.PathValue("reqID"))
	})
	mux.HandleFunc("POST /v1/sessions/{id}/model", sessionHandler(s.handleUpdateModel))
	mux.HandleFunc("GET /v1/sessions/{id}/files", sessionHandler(s.handleGetFiles))
	mux.HandleFunc("PUT /v1/sessions/{id}/files", sessionHandler(s.handlePutFiles))
	mux.HandleFunc("POST /v1/sessions/{id}/cancel", sessionHandler(s.handleCancelSession))
	mux.HandleFunc("POST /v1/sessions/{id}/clear", sessionHandler(s.handleClearSession))
	mux.HandleFunc("POST /v1/sessions/{id}/compact", sessionHandler(s.handleCompactSession))
	mux.HandleFunc("GET /v1/sessions/{id}/cost", sessionHandler(s.handleSessionCost))
	mux.HandleFunc("GET /v1/sessions/{id}/index/status", sessionHandler(s.handleIndexStatus))
	mux.HandleFunc("POST /v1/sessions/{id}/index/build", sessionHandler(s.handleIndexBuild))
	// With --dev the gallery is registered (GET only). Without it nothing
	// matches /dev with a method, and hideDev below answers 404 for every
	// method so the route's existence is not revealed (no 405/Allow, no 307).
	if s.cfg.Dev {
		mux.Handle("GET /dev/", s.devHandler())
		mux.Handle("GET /dev", s.devHandler())
	}
	if !s.cfg.NoUI {
		sub, err := fs.Sub(webUI, "web")
		if err == nil {
			mux.Handle("GET /", staticJSContentType(http.FileServer(http.FS(sub))))
		}
	}
	var root http.Handler = mux
	if !s.cfg.Dev {
		root = hideDev(mux)
	}
	return securityHeaders(NewRequestGuard(s.cfg.Bind, NewAuthMiddleware(s.cfg.Token, root)))
}

// hideDev answers 404 for /dev and everything under it, whatever the method.
func hideDev(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/dev" || strings.HasPrefix(r.URL.Path, "/dev/") {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
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
					if s.registry.remove(view.SessionID) {
						s.limiter.ReleaseSession()
					}
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
	err := s.httpServer.Shutdown(ctx)
	if s.closeMCP != nil {
		s.closeOnce.Do(s.closeMCP)
	}
	return err
}

func RunUntilSignal(ctx context.Context, logger *slog.Logger, cfg Config) error {
	srv, err := New(ctx, logger, cfg)
	if err != nil {
		return err
	}
	announce(os.Stderr, srv.cfg)
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

// announce prints where to open the web UI. A generated token is carried in
// the URL fragment, which browsers never send to the server, and is written
// only to the terminal, never to the structured logger.
func announce(w io.Writer, cfg Config) {
	host := cfg.Bind
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	uiURL := "http://" + net.JoinHostPort(host, strconv.Itoa(cfg.Port)) + "/"
	if !cfg.tokenGenerated {
		fmt.Fprintf(w, "nandocodego server: open %s and enter your --token in the Token field\n", uiURL)
		return
	}
	fmt.Fprintf(w, "nandocodego server: open %s#token=%s\n", uiURL, cfg.Token)
	fmt.Fprintf(w, "nandocodego server: API clients must send \"Authorization: Bearer %s\"\n", cfg.Token)
}

func ptrOrNil(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	v := s
	return &v
}
