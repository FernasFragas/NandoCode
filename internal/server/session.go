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

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/contextpack"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/llm/modelruntime"
	"github.com/FernasFragas/Nandocode/internal/permissions"
	"github.com/FernasFragas/Nandocode/internal/retrievalroute"
	"github.com/FernasFragas/Nandocode/internal/runctl"
	"github.com/FernasFragas/Nandocode/internal/semantic"
	"github.com/FernasFragas/Nandocode/internal/state"
	"github.com/FernasFragas/Nandocode/internal/tasks"
	"github.com/FernasFragas/Nandocode/internal/tools/todo"
	"github.com/FernasFragas/Nandocode/internal/turnprep"
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
	activeWrites  map[string]string // tool call ID to resolved path
	saveHooks     map[string]saveHookMemo
	broker        *HTTPPermissionBroker
	lastTerminal  TerminalSnapshot
	supervisor    *tasks.Supervisor
	taskStore     *state.Store[state.App]
	llmClient     llm.Client
	dreamEnabled  bool
	taskWatchStop context.CancelFunc
	semanticCfg   semantic.Config
	semanticSvc   semantic.Service
	runtime       *llm.RuntimeClient // per-session model router; nil when the server shares one
	models        *modelruntime.Service
	switchedModel string // ActiveModel the runtime was last switched to
	runPrompt     string // raw user prompt of the active run
	ctl           *runctl.Controller
	todoWrites    map[string]struct{}
	title         string
	titlePrompt   string
	titlePending  bool
	titleDone     bool
	titleStarted  bool
	titleCancel   context.CancelFunc
	titleFinished chan struct{}
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
	app.TodoList = todo.NewTodoList()
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
		ctl:           runctl.New(),
		todoWrites:    make(map[string]struct{}),
	}
	s.broker = NewHTTPPermissionBroker(s)
	s.Emit("session_ready", nil)
	return s
}

func (s *Session) setCoordinatorRuntime(sup *tasks.Supervisor, taskStore *state.Store[state.App], client llm.Client, dreamEnabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.supervisor = sup
	if sup != nil {
		sup.SetWorkerObserver(s.emitSubagentEvent)
	}
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
	var workers []tasks.WorkerSnapshot
	if s.supervisor != nil {
		workers = s.supervisor.WorkerSnapshots()
	}
	return SessionView{
		SessionID:       s.id,
		CreatedAt:       s.createdAt,
		LastActive:      s.lastActive,
		State:           s.state,
		Running:         s.running,
		CoordinatorMode: s.appState.CoordinatorMode,
		WorkerCount:     s.appState.WorkerCount,
		Workers:         workers,
		WorkspaceRoot:   s.appState.ToolSettings.WorkingDir,
	}
}

func (s *Session) activeModel() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.appState.ActiveModel
}

// setRuntime binds the session to its own model runtime.
func (s *Session) setRuntime(rt sessionRuntime) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runtime = rt.rc
	s.llmClient = rt.client
	s.models = rt.models
}

func (s *Session) modelService() *modelruntime.Service {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.models
}

// modelSwitchCurrent reports whether the session's runtime is already
// switched to its active model, so a message does not re-resolve it.
func (s *Session) modelSwitchCurrent() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.switchedModel != "" && s.switchedModel == s.appState.ActiveModel
}

func (s *Session) applyModelSwitch(result modelruntime.SwitchResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appState.ActiveModel = result.Resolved.Model
	s.switchedModel = result.Resolved.Model
	s.appState.LLMProvider = string(result.Resolved.Provider)
	if strings.TrimSpace(result.Resolved.BaseURL) != "" {
		s.appState.LLMBaseURL = result.Resolved.BaseURL
	}
}

// applyActivation refreshes the per-session limits after a model switch.
func (s *Session) applyActivation(res modelruntime.ActivateResult) {
	s.applyModelSwitch(res.SwitchResult)
	if !res.HasLimits {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appState.MaxOutputTokens = res.Limits.MaxOutputTokens
	if res.Limits.NumCtx > 0 {
		s.appState.RuntimeNumCtx = res.Limits.NumCtx
	}
	s.appState.ToolSettings.MaxResultChars = res.Limits.MaxResultChars
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
	if s.running || s.state == SessionStateClosing {
		s.mu.Unlock()
		return errors.New("run already active")
	}
	run, err := s.ctl.Begin(s.ctx)
	if err != nil {
		s.mu.Unlock()
		return errors.New("run already active")
	}
	s.running = true
	s.state = SessionStateRunning
	s.todoWrites = make(map[string]struct{})
	// A title request must yield to an interactive run. The handler returns
	// immediately; the run goroutine waits only for the canceled request to exit.
	var titleFinished <-chan struct{}
	if s.titlePending && !s.titleDone {
		s.titlePending = false
		s.titlePrompt = ""
		if s.titleCancel != nil {
			s.titleCancel()
			titleFinished = s.titleFinished
		}
	}
	app := s.appState.Clone()
	conv := append([]llm.Message(nil), s.conversation...)
	s.runPrompt = input.Prompt
	mode := s.permMode
	rules := s.permRules
	s.mu.Unlock()
	if s.supervisor != nil {
		_ = s.supervisor.KillDream()
	}

	go func() {
		if titleFinished != nil {
			<-titleFinished
		}
		s.runAgent(run, input, cfg, app, conv, mode, rules)
	}()
	return nil
}

func (s *Session) runAgent(run runctl.Run, req MessageRequest, cfg agent.Config, app state.App, history []llm.Message, mode permissions.Mode, rules permissions.Rules) {
	defer func() {
		s.ctl.Finish(run)
		s.clearFileWrites()
		s.mu.Lock()
		s.running = false
		s.state = SessionStateReady
		titlePending := s.titlePending && !s.titleDone
		s.mu.Unlock()
		if titlePending {
			go s.generateSessionTitle()
		}
		if s.supervisor != nil && s.dreamEnabled && s.llmClient != nil {
			_, _ = s.supervisor.SpawnDream(s.ctx, s.llmClient, app.ActiveModel, "Speculative follow-up analysis.")
		}
	}()
	if app.RuntimeNumCtx > 0 {
		cfg.NumCtx = app.RuntimeNumCtx
	}
	toolCtx := app.ToolContext(run.Ctx)
	prep, err := turnprep.Prepare(turnprep.Request{
		Ctx:             run.Ctx,
		Prompt:          req.Prompt,
		ToolContext:     toolCtx,
		AgentConfig:     cfg,
		Model:           app.ActiveModel,
		LLMProvider:     app.LLMProvider,
		ContextMode:     app.ContextMode,
		MaxOutputTokens: app.MaxOutputTokens,
		History:         history,
		Semantic:        turnprep.Semantic{Service: s.semanticSvc, Config: s.semanticCfg},
		Grounding:       true,
		Hooks: turnprep.Hooks{
			OnRoute: s.emitRouteDecided,
			OnSemanticStart: func() {
				s.Emit("semantic_query_embed_started", nil)
			},
			OnSemanticStage: func(evt semantic.RetrieveStageEvent) {
				s.Emit("semantic_stage_timing", map[string]any{
					"stage":       string(evt.Stage),
					"duration_ms": evt.Duration.Milliseconds(),
					"cache_hit":   evt.CacheHit,
				})
			},
		},
	})
	if err != nil {
		s.prepFailed(run, err)
		return
	}
	s.emitSemanticOutcome(prep)

	in := prep.Input
	in.PermissionMode = mode
	in.PermissionRules = rules
	in.PermissionPrompt = s.broker.PromptFunc()
	if app.CoordinatorMode {
		in.SystemPrompt = agent.BuildCoordinatorSystemPrompt(nil, "")
	}
	s.Emit("run_started", map[string]any{"message_id": req.MessageID})
	in.ParentAbort = run.ParentAbort
	for evt := range s.runner.Run(run.Ctx, in) {
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
		s.trackFileWriteStart(e)
		s.Emit("tool_use_start", map[string]any{"id": e.ID, "name": e.Name, "input": e.Input})
		s.noteTodoToolStart(e)
	case agent.ToolUseProgress:
		s.Emit("tool_use_progress", map[string]any{"id": e.ID, "data": e.Data})
	case agent.ToolUseResult:
		s.trackFileWriteResult(e.ID)
		err := ""
		if e.Err != nil {
			err = e.Err.Error()
		}
		s.Emit("tool_use_result", map[string]any{"id": e.ID, "result": e.Result.Display, "error": err})
		s.emitTodoUpdatedAfterResult(e)
	case agent.TurnUsage:
		s.emitUsageUpdate(e.Usage)
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
		s.clearFileWrites()
		s.mu.Lock()
		if !s.titleDone && !s.titlePending && s.runPrompt != "" {
			s.titlePrompt = s.runPrompt
			s.titlePending = true
		}
		// e.Conversation holds only what this run added (assistant and tool
		// messages), so ApplyTerminal appends it after the user prompt.
		turn := runctl.ApplyTerminal(state.App{Messages: s.conversation}, e, runctl.TerminalOptions{UserPrompt: s.runPrompt})
		s.conversation = turn.Messages
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

// AllowSession records an "Always Allow" for exactly this tool and target. It
// applies to later runs of this session; deny and ask rules still win.
func (s *Session) AllowSession(toolName, target string) {
	rule, ok := permissions.SessionAllowRule(toolName, target)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.permRules.AlwaysAllow = append(s.permRules.AlwaysAllow, rule)
}

func (s *Session) stop() {
	s.cancel()
	s.clearFileWrites()
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

func (s *Session) emitRouteDecided(d retrievalroute.Decision) {
	s.Emit("retrieval_route_decided", map[string]any{
		"action":            string(d.Action),
		"reason":            string(d.Reason),
		"allow_embedding":   d.AllowEmbedding,
		"tool_mode":         string(d.ToolMode),
		"request_profile":   d.RequestProfile,
		"profile":           d.Profile,
		"max_records":       d.MaxRecords,
		"max_files":         d.MaxFiles,
		"max_context_bytes": d.MaxContextBytes,
		"deadline_ms":       d.Deadline.Milliseconds(),
	})
}

// emitSemanticOutcome turns the turnprep semantic outcome into the same
// events the server has always emitted, in the same order.
func (s *Session) emitSemanticOutcome(prep turnprep.Prepared) {
	out := prep.Semantic
	if !out.Attempted {
		s.Emit("semantic_skipped", map[string]any{"reason": string(prep.Route.Reason)})
		return
	}
	if out.Kind != turnprep.SemanticSkipped {
		s.Emit("semantic_query_embed_finished", map[string]any{"duration_ms": out.Duration.Milliseconds()})
	}
	switch out.Kind {
	case turnprep.SemanticUsed:
		res := out.Result
		payload := func() map[string]any {
			return map[string]any{
				"records":       len(res.Records),
				"files":         len(res.Files),
				"context_bytes": res.ContextBytes,
				"stale_dropped": res.StaleDropped,
			}
		}
		s.Emit("semantic_search_finished", payload())
		s.Emit("semantic_retrieval", payload())
	case turnprep.SemanticFallback:
		s.Emit("semantic_skipped", map[string]any{"reason": out.Reason})
		s.Emit("semantic_retrieval", map[string]any{"fallback": true, "reason": out.Reason})
	case turnprep.SemanticError:
		s.Emit("semantic_retrieval", map[string]any{"error": out.Err.Error()})
	}
}

// prepFailed ends a run whose prompt preparation failed. When the run was
// cancelled meanwhile it reports one aborted terminal and records nothing in
// the history (the turn never started, so no lone user message is left). Any
// other failure stays an error event.
func (s *Session) prepFailed(run runctl.Run, err error) {
	if run.Ctx.Err() != nil {
		s.mu.Lock()
		s.lastTerminal = TerminalSnapshot{Reason: agent.TerminalAborted}
		s.mu.Unlock()
		s.Emit("terminal", map[string]any{"reason": agent.TerminalAborted, "detail": "", "usage": agent.Usage{}})
		return
	}
	var tooLarge contextpack.ErrEvidenceTooLarge
	if errors.As(err, &tooLarge) {
		s.Emit("error", map[string]any{"code": "evidence_too_large", "error": tooLarge.Error(), "budget_tokens": tooLarge.BudgetTokens, "estimated_tokens": tooLarge.EstimatedTokens})
	} else {
		s.Emit("error", map[string]any{"code": "prompt_pack_failed", "error": err.Error()})
	}
}
