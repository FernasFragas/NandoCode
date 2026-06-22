package server

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/FernasFragas/nandocodego/internal/agent"
	"github.com/FernasFragas/nandocodego/internal/analysis"
	"github.com/FernasFragas/nandocodego/internal/contextpack"
	"github.com/FernasFragas/nandocodego/internal/llm"
	"github.com/FernasFragas/nandocodego/internal/llm/modelruntime"
	"github.com/FernasFragas/nandocodego/internal/mentions"
	"github.com/FernasFragas/nandocodego/internal/permissions"
	"github.com/FernasFragas/nandocodego/internal/retrievalroute"
	"github.com/FernasFragas/nandocodego/internal/semantic"
	"github.com/FernasFragas/nandocodego/internal/state"
	"github.com/FernasFragas/nandocodego/internal/tasks"
)

type AgentRunner interface {
	Run(context.Context, agent.Input) <-chan agent.Event
}

type Session struct {
	id         string
	createdAt  time.Time
	lastActive time.Time
	state      SessionState
	ctx        context.Context
	cancel     context.CancelFunc
	runner     AgentRunner
	appState   state.App

	mu            sync.Mutex
	running       bool
	conversation  []llm.Message
	events        *RingBuffer[SessionEvent]
	subscribers   map[chan SessionEvent]struct{}
	eventCounter  uint64
	recentMessage *RecentIDs
	permRules     permissions.Rules
	permMode      permissions.Mode
	broker        *HTTPPermissionBroker
	lastTerminal  TerminalSnapshot
	supervisor    *tasks.Supervisor
	taskStore     *state.Store[state.App]
	llmClient     llm.Client
	dreamEnabled  bool
	taskWatchStop context.CancelFunc
	semanticCfg   semantic.Config
	semanticSvc   semantic.Service
}

type sessionRegistry struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

func newSessionRegistry() *sessionRegistry {
	return &sessionRegistry{sessions: map[string]*Session{}}
}

func (r *sessionRegistry) set(s *Session) { r.mu.Lock(); r.sessions[s.id] = s; r.mu.Unlock() }
func (r *sessionRegistry) get(id string) (*Session, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.sessions[id]
	return s, ok
}
func (r *sessionRegistry) del(id string) { r.mu.Lock(); delete(r.sessions, id); r.mu.Unlock() }
func (r *sessionRegistry) all() []*Session {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Session, 0, len(r.sessions))
	for _, s := range r.sessions {
		out = append(out, s)
	}
	return out
}

func newSession(parent context.Context, id string, app state.App, runner AgentRunner) *Session {
	ctx, cancel := context.WithCancel(parent)
	now := time.Now().UTC()
	s := &Session{
		id:            id,
		createdAt:     now,
		lastActive:    now,
		state:         SessionStateReady,
		ctx:           ctx,
		cancel:        cancel,
		runner:        runner,
		appState:      app,
		events:        NewRingBuffer[SessionEvent](200),
		subscribers:   map[chan SessionEvent]struct{}{},
		recentMessage: NewRecentIDs(128),
		permRules:     app.PermissionRules,
		permMode:      app.PermissionMode,
	}
	s.broker = NewHTTPPermissionBroker(s)
	s.Emit("session_ready", nil)
	return s
}

func (s *Session) setCoordinatorRuntime(sup *tasks.Supervisor, taskStore *state.Store[state.App], client llm.Client, dreamEnabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.supervisor = sup
	s.taskStore = taskStore
	s.llmClient = client
	s.dreamEnabled = dreamEnabled
	if s.taskWatchStop != nil {
		s.taskWatchStop()
	}
	if s.taskStore != nil {
		wctx, cancel := context.WithCancel(s.ctx)
		s.taskWatchStop = cancel
		go s.watchTaskLifecycle(wctx)
	}
}

func (s *Session) watchTaskLifecycle(ctx context.Context) {
	known := map[string]string{}
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if s.taskStore == nil {
				continue
			}
			app := s.taskStore.Get()
			for id, ts := range app.Tasks {
				status := string(ts.Status)
				if prev, ok := known[id]; ok && prev == status {
					continue
				}
				known[id] = status
				s.Emit("task_lifecycle", map[string]any{
					"task_id":     ts.ID,
					"kind":        ts.Kind,
					"status":      ts.Status,
					"description": ts.Description,
					"output_file": ts.OutputFile,
				})
			}
		}
	}
}

func (s *Session) view() SessionView {
	s.mu.Lock()
	defer s.mu.Unlock()
	return SessionView{
		SessionID:       s.id,
		CreatedAt:       s.createdAt,
		LastActive:      s.lastActive,
		State:           s.state,
		Running:         s.running,
		CoordinatorMode: s.appState.CoordinatorMode,
		WorkerCount:     s.appState.WorkerCount,
	}
}

func (s *Session) activeModel() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.appState.ActiveModel
}

func (s *Session) applyModelSwitch(result modelruntime.SwitchResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appState.ActiveModel = result.Resolved.Model
	s.appState.LLMProvider = string(result.Resolved.Provider)
	if strings.TrimSpace(result.Resolved.BaseURL) != "" {
		s.appState.LLMBaseURL = result.Resolved.BaseURL
	}
}

func (s *Session) touch() { s.mu.Lock(); s.lastActive = time.Now().UTC(); s.mu.Unlock() }

func (s *Session) allowMessageID(messageID string) bool {
	return !s.recentMessage.SeenOrAdd(strings.TrimSpace(messageID))
}

func (s *Session) Subscribe() chan SessionEvent {
	ch := make(chan SessionEvent, 64)
	s.mu.Lock()
	s.subscribers[ch] = struct{}{}
	s.mu.Unlock()
	return ch
}

func (s *Session) Unsubscribe(ch chan SessionEvent) {
	s.mu.Lock()
	delete(s.subscribers, ch)
	s.mu.Unlock()
	close(ch)
}

func (s *Session) Emit(eventType string, data map[string]any) {
	id := atomic.AddUint64(&s.eventCounter, 1)
	evt := SessionEvent{ID: strconv.FormatUint(id, 10), Type: eventType, Time: time.Now().UTC(), SessionID: s.id, Data: data}
	s.events.Append(evt)
	s.mu.Lock()
	for ch := range s.subscribers {
		select {
		case ch <- evt:
		default:
		}
	}
	s.lastActive = time.Now().UTC()
	s.mu.Unlock()
}

func (s *Session) Replay(lastID string) []SessionEvent {
	items := s.events.Snapshot()
	if lastID == "" {
		return items
	}
	idx := -1
	for i := range items {
		if items[i].ID == lastID {
			idx = i
			break
		}
	}
	if idx < 0 || idx+1 >= len(items) {
		return nil
	}
	return items[idx+1:]
}

func (s *Session) StartRun(input MessageRequest, cfg agent.Config) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errors.New("run already active")
	}
	s.running = true
	s.state = SessionStateRunning
	app := s.appState.Clone()
	conv := append([]llm.Message(nil), s.conversation...)
	mode := s.permMode
	rules := s.permRules
	s.mu.Unlock()
	if s.supervisor != nil {
		_ = s.supervisor.KillDream()
	}

	go s.runAgent(input, cfg, app, conv, mode, rules)
	return nil
}

func (s *Session) runAgent(req MessageRequest, cfg agent.Config, app state.App, history []llm.Message, mode permissions.Mode, rules permissions.Rules) {
	defer func() {
		s.mu.Lock()
		s.running = false
		s.state = SessionStateReady
		s.mu.Unlock()
		if s.supervisor != nil && s.dreamEnabled && s.llmClient != nil {
			_, _ = s.supervisor.SpawnDream(s.ctx, s.llmClient, app.ActiveModel, "Speculative follow-up analysis.")
		}
	}()
	toolCtx := app.ToolContext(s.ctx)
	packed, _, err := contextpack.BuildCurrentTurnPrompt(req.Prompt, toolCtx, cfg, agent.Input{Model: app.ActiveModel, ContextMode: app.ContextMode, MaxOutputTokens: app.MaxOutputTokens}, history)
	if err != nil {
		var tooLarge contextpack.ErrEvidenceTooLarge
		if errors.As(err, &tooLarge) {
			s.Emit("error", map[string]any{"code": "evidence_too_large", "error": tooLarge.Error(), "budget_tokens": tooLarge.BudgetTokens, "estimated_tokens": tooLarge.EstimatedTokens})
		} else {
			s.Emit("error", map[string]any{"code": "prompt_pack_failed", "error": err.Error()})
		}
		return
	}
	historyPolicy := agent.HistoryPolicyDefault
	if packed.ExpansionReport.Intent.AttachmentPolicy == mentions.AttachListingTreeOnly || packed.ExpansionReport.Intent.Kind == mentions.IntentFileStatus {
		historyPolicy = agent.HistoryPolicyLatestOnly
	}
	finalPrompt := packed.Prompt
	explicit := analysis.ExtractMentionedPaths(req.Prompt)
	currentTurn := make([]string, 0, len(packed.Files))
	for _, f := range packed.Files {
		if strings.TrimSpace(f.Path) == "" {
			continue
		}
		currentTurn = append(currentTurn, f.Path)
	}
	currentDirs := make([]string, 0, len(packed.Dirs))
	for _, d := range packed.Dirs {
		if strings.TrimSpace(d.Path) == "" {
			continue
		}
		currentDirs = append(currentDirs, d.Path)
	}
	routeCfg := serverSemanticRouteConfig(s.semanticCfg, s.semanticSvc != nil)
	routeDecision := retrievalroute.Decide(retrievalroute.Input{
		RawPrompt:            req.Prompt,
		ShouldQuery:          true,
		AttachmentPolicy:     string(packed.ExpansionReport.Intent.AttachmentPolicy),
		CurrentTurnPaths:     currentTurn,
		CurrentTurnDirs:      currentDirs,
		AttachedFileCount:    len(packed.Files),
		AttachedContextBytes: len(packed.Prompt),
		IndexKnown:           false,
		HasIndex:             false,
		IndexCompatible:      false,
		SemanticEnabled:      s.semanticSvc != nil && s.semanticCfg.Enabled,
		SemanticMode:         routeCfg.Mode,
		PromptIntent:         string(packed.ExpansionReport.Intent.Kind),
	}, routeCfg)
	s.Emit("retrieval_route_decided", map[string]any{
		"action":            string(routeDecision.Action),
		"reason":            string(routeDecision.Reason),
		"allow_embedding":   routeDecision.AllowEmbedding,
		"tool_mode":         string(routeDecision.ToolMode),
		"request_profile":   routeDecision.RequestProfile,
		"profile":           routeDecision.Profile,
		"max_records":       routeDecision.MaxRecords,
		"max_files":         routeDecision.MaxFiles,
		"max_context_bytes": routeDecision.MaxContextBytes,
		"deadline_ms":       routeDecision.Deadline.Milliseconds(),
	})

	if routeDecision.AllowEmbedding && s.semanticSvc != nil {
		semanticStart := time.Now()
		s.Emit("semantic_query_embed_started", nil)
		res, err := s.semanticSvc.Retrieve(s.ctx, semantic.RetrieveRequest{
			Root:                 app.ToolSettings.WorkingDir,
			Query:                req.Prompt,
			ExplicitPaths:        explicit,
			CurrentTurnPaths:     currentTurn,
			Deadline:             routeDecision.Deadline,
			RouteAction:          string(routeDecision.Action),
			RouteReason:          string(routeDecision.Reason),
			RouteProfile:         routeDecision.Profile,
			UseCurrentPathWeight: routeDecision.UseCurrentPathWeight,
			MaxRecords:           routeDecision.MaxRecords,
			MaxFiles:             routeDecision.MaxFiles,
			MaxContextBytes:      routeDecision.MaxContextBytes,
			Observer: func(evt semantic.RetrieveStageEvent) {
				s.Emit("semantic_stage_timing", map[string]any{
					"stage":       string(evt.Stage),
					"duration_ms": evt.Duration.Milliseconds(),
					"cache_hit":   evt.CacheHit,
				})
			},
		})
		switch {
		case err == nil && res.Used:
			s.Emit("semantic_query_embed_finished", map[string]any{"duration_ms": time.Since(semanticStart).Milliseconds()})
			finalPrompt = strings.TrimRight(finalPrompt, "\n") + "\n\n" + strings.TrimSpace(res.RenderedContext)
			s.Emit("semantic_search_finished", map[string]any{
				"records":       len(res.Records),
				"files":         len(res.Files),
				"context_bytes": res.ContextBytes,
				"stale_dropped": res.StaleDropped,
			})
			s.Emit("semantic_retrieval", map[string]any{
				"records":       len(res.Records),
				"files":         len(res.Files),
				"context_bytes": res.ContextBytes,
				"stale_dropped": res.StaleDropped,
			})
		case err != nil && semantic.IsFallbackError(err):
			s.Emit("semantic_query_embed_finished", map[string]any{"duration_ms": time.Since(semanticStart).Milliseconds()})
			s.Emit("semantic_skipped", map[string]any{"reason": err.Error()})
			s.Emit("semantic_retrieval", map[string]any{"fallback": true, "reason": err.Error()})
		case err != nil:
			s.Emit("semantic_query_embed_finished", map[string]any{"duration_ms": time.Since(semanticStart).Milliseconds()})
			s.Emit("semantic_retrieval", map[string]any{"error": err.Error()})
		case res.FallbackReason != "":
			s.Emit("semantic_query_embed_finished", map[string]any{"duration_ms": time.Since(semanticStart).Milliseconds()})
			s.Emit("semantic_skipped", map[string]any{"reason": res.FallbackReason})
			s.Emit("semantic_retrieval", map[string]any{"fallback": true, "reason": res.FallbackReason})
		}
	} else {
		s.Emit("semantic_skipped", map[string]any{"reason": string(routeDecision.Reason)})
	}

	msgs := append(history, llm.Message{Role: llm.RoleUser, Content: finalPrompt})
	in := agent.Input{
		Model:            app.ActiveModel,
		LLMProvider:      app.LLMProvider,
		ContextMode:      app.ContextMode,
		PromptIntent:     string(packed.ExpansionReport.Intent.Kind),
		AttachmentPolicy: string(packed.ExpansionReport.Intent.AttachmentPolicy),
		OriginalUserText: req.Prompt,
		HistoryPolicy:    historyPolicy,
		ToolMode:         string(routeDecision.ToolMode),
		RouteAction:      string(routeDecision.Action),
		RouteReason:      string(routeDecision.Reason),
		RouteProfile:     routeDecision.RequestProfile,
		ToolContext:      toolCtx,
		EvidencePack:     &packed.PackReport,
		Messages:         msgs,
		PermissionMode:   mode,
		PermissionRules:  rules,
		PermissionPrompt: s.broker.PromptFunc(),
		MaxOutputTokens:  app.MaxOutputTokens,
	}
	if app.CoordinatorMode {
		in.SystemPrompt = agent.BuildCoordinatorSystemPrompt(nil, "")
	}
	s.Emit("run_started", map[string]any{"message_id": req.MessageID})
	for evt := range s.runner.Run(s.ctx, in) {
		s.handleAgentEvent(evt)
	}
}

func (s *Session) handleAgentEvent(evt agent.Event) {
	switch e := evt.(type) {
	case agent.AssistantTextDelta:
		s.Emit("assistant_text_delta", map[string]any{"content": e.Content})
	case agent.AssistantThinkingDelta:
		s.Emit("assistant_thinking_delta", map[string]any{"thinking": e.Thinking})
	case agent.ToolUseStart:
		s.Emit("tool_use_start", map[string]any{"id": e.ID, "name": e.Name, "input": e.Input})
	case agent.ToolUseProgress:
		s.Emit("tool_use_progress", map[string]any{"id": e.ID, "data": e.Data})
	case agent.ToolUseResult:
		err := ""
		if e.Err != nil {
			err = e.Err.Error()
		}
		s.Emit("tool_use_result", map[string]any{"id": e.ID, "result": e.Result.Display, "error": err})
	case agent.HookNotice:
		s.Emit("hook_notice", map[string]any{"message": e.Message})
	case agent.RetryNotice:
		s.Emit("retry_notice", map[string]any{"cause": e.Cause, "attempt": e.Attempt})
	case agent.LLMIdleWarning:
		s.Emit("llm_idle_warning", map[string]any{
			"provider":    e.Provider,
			"timeout_ms":  e.Timeout.Milliseconds(),
			"timeout_str": e.Timeout.String(),
		})
	case agent.LLMRequestStarted:
		s.Emit("llm_request_started", nil)
	case agent.LLMStreamOpened:
		s.Emit("llm_stream_opened", map[string]any{"latency_ms": e.Latency.Milliseconds()})
	case agent.FirstTokenReceived:
		s.Emit("first_token_received", map[string]any{"latency_ms": e.Latency.Milliseconds()})
	case agent.StageTiming:
		s.Emit("stage_timing", map[string]any{"stage": e.Stage, "duration_ms": e.Duration.Milliseconds()})
	case agent.PromptPackReport:
		s.Emit("prompt_pack_report", map[string]any{"estimated_included": e.EstimatedIncluded, "estimated_skipped": e.EstimatedSkipped})
	case agent.Terminal:
		s.mu.Lock()
		s.conversation = append([]llm.Message(nil), e.Conversation...)
		s.lastTerminal = TerminalSnapshot{Reason: e.Reason, Detail: e.Detail, Usage: e.Usage}
		s.mu.Unlock()
		s.Emit("terminal", map[string]any{"reason": e.Reason, "detail": e.Detail, "usage": e.Usage})
	}
}

func (s *Session) ResolvePermission(reqID, d string) bool {
	decision := permissionDecision(strings.TrimSpace(d))
	switch decision {
	case decisionAllow, decisionDeny, decisionAlwaysAllow:
		return s.broker.Resolve(reqID, decision)
	default:
		return false
	}
}

func (s *Session) AllowAllRules() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.permRules.AlwaysAllow = append(s.permRules.AlwaysAllow, permissions.Rule{Pattern: "*(*)", Source: permissions.SourceSession})
}

func (s *Session) stop() {
	s.cancel()
	s.mu.Lock()
	if s.taskWatchStop != nil {
		s.taskWatchStop()
		s.taskWatchStop = nil
	}
	s.state = SessionStateClosing
	s.mu.Unlock()
}

func validateNonLoopback(bindAddr, token string) error {
	if strings.TrimSpace(token) != "" {
		return nil
	}
	if bindAddr == "127.0.0.1" || bindAddr == "localhost" || bindAddr == "::1" {
		return nil
	}
	return fmt.Errorf("refusing non-loopback bind without --token")
}

func serverSemanticRouteConfig(cfg semantic.Config, hasService bool) retrievalroute.Config {
	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if mode == "" {
		mode = "auto"
	}
	if !cfg.Enabled || !hasService {
		mode = "off"
	}
	return retrievalroute.Config{
		Mode: mode,
		Light: retrievalroute.Limits{
			MaxRecords:      cfg.LightTopKRecords,
			MaxFiles:        cfg.LightTopKFiles,
			MaxContextBytes: cfg.LightMaxContextBytes,
			Deadline:        time.Duration(cfg.LightDeadlineMS) * time.Millisecond,
		},
		Full: retrievalroute.Limits{
			MaxRecords:      cfg.FullTopKRecords,
			MaxFiles:        cfg.FullTopKFiles,
			MaxContextBytes: cfg.FullMaxContextBytes,
			Deadline:        time.Duration(cfg.FullDeadlineMS) * time.Millisecond,
		},
		Deep: retrievalroute.Limits{
			MaxRecords:      cfg.DeepTopKRecords,
			MaxFiles:        cfg.DeepTopKFiles,
			MaxContextBytes: cfg.DeepMaxContextBytes,
			Deadline:        time.Duration(cfg.DeepDeadlineMS) * time.Millisecond,
		},
	}
}
