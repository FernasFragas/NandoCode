package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/hooks"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/runctl"
	"github.com/FernasFragas/Nandocode/internal/semantic"
	"github.com/FernasFragas/Nandocode/internal/state"
	"github.com/FernasFragas/Nandocode/internal/tools/todo"
)

// Session operations (BF-5): clear, compact, cost and the semantic index.
// Clear and compact reuse the TUI's shared operations in internal/runctl.

// indexRuns tracks the workspace roots with an index build in flight, so two
// builds of the same root cannot run at once. Entries are removed when the
// build ends.
type indexRunRegistry struct {
	mu    sync.Mutex
	roots map[string]struct{}
}

func (s *Server) acquireIndexRun(root string) bool {
	s.indexRuns.mu.Lock()
	defer s.indexRuns.mu.Unlock()
	if _, busy := s.indexRuns.roots[root]; busy {
		return false
	}
	if s.indexRuns.roots == nil {
		s.indexRuns.roots = make(map[string]struct{})
	}
	s.indexRuns.roots[root] = struct{}{}
	return true
}

func (s *Server) releaseIndexRun(root string) {
	s.indexRuns.mu.Lock()
	delete(s.indexRuns.roots, root)
	s.indexRuns.mu.Unlock()
}

func writeOpsJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// beginIdleOp reserves the session for a history operation. A run, or another
// operation, in progress refuses it.
func (s *Session) beginIdleOp() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return false
	}
	s.running = true
	s.state = SessionStateRunning
	return true
}

func (s *Session) endIdleOp() {
	s.mu.Lock()
	s.running = false
	s.state = SessionStateReady
	s.mu.Unlock()
}

func (s *Server) handleClearSession(w http.ResponseWriter, r *http.Request, id string) {
	sess, ok := s.registry.get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !sess.beginIdleOp() {
		http.Error(w, "run active", http.StatusConflict)
		return
	}
	defer sess.endIdleOp()
	sess.mu.Lock()
	sess.appState.Messages = sess.conversation
	sess.appState = runctl.ResetConversation(sess.appState)
	sess.conversation = sess.appState.Messages
	sess.lastTerminal = TerminalSnapshot{}
	sess.appState.TodoList = todo.NewTodoList()
	sess.mu.Unlock()
	sess.Emit("session_cleared", nil)
	sess.Emit("todo_updated", map[string]any{"todos": []todo.TodoItem{}})
	writeOpsJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleCompactSession(w http.ResponseWriter, r *http.Request, id string) {
	sess, ok := s.registry.get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !sess.beginIdleOp() {
		http.Error(w, "run active", http.StatusConflict)
		return
	}
	defer sess.endIdleOp()

	sess.mu.Lock()
	msgs := append([]llm.Message(nil), sess.conversation...)
	model := sess.appState.ActiveModel
	workDir := sess.appState.ToolSettings.WorkingDir
	permissionMode := sess.appState.PermissionMode.String()
	client := sess.llmClient
	if client == nil {
		client = s.client
	}
	sess.mu.Unlock()
	hookCfg := s.hookCfg
	hookCfg.SessionID = sess.id
	hookCfg.Model = model
	hookCfg.WorkingDir = workDir
	hookCfg.PermissionMode = permissionMode
	dispatcher := hooks.NewDispatcher(s.hookSnapshot, client, hookCfg)

	sess.Emit("compaction_started", map[string]any{"turn_count": agent.CountTurnsExported(msgs)})
	out := runctl.CompactConversation(r.Context(), runctl.CompactDeps{
		Client:     client,
		Hooks:      dispatcher,
		Model:      model,
		SessionID:  sess.id,
		WorkingDir: workDir,
		Trigger:    "manual",
	}, msgs)

	if out.Changed() {
		sess.mu.Lock()
		sess.conversation = runctl.ApplyCompaction(state.App{Messages: sess.conversation}, out).Messages
		sess.mu.Unlock()
	}
	sess.Emit("compaction_completed", map[string]any{
		"before":  out.Before,
		"after":   out.After,
		"layer":   out.Result.Layer,
		"skipped": out.Skipped,
		"denied":  out.Denied,
	})
	writeOpsJSON(w, http.StatusOK, map[string]any{
		"ok": true, "before": out.Before, "after": out.After, "skipped": out.Skipped, "denied": out.Denied,
	})
}

func (s *Server) handleSessionCost(w http.ResponseWriter, r *http.Request, id string) {
	sess, ok := s.registry.get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	sess.mu.Lock()
	last := sess.lastTerminal
	msgs := len(sess.conversation)
	turns := agent.CountTurnsExported(sess.conversation)
	model := sess.appState.ActiveModel
	sess.mu.Unlock()
	writeOpsJSON(w, http.StatusOK, map[string]any{
		"session_id": id,
		"model":      model,
		"messages":   msgs,
		"turns":      turns,
		"last_run":   map[string]any{"reason": last.Reason, "detail": last.Detail, "usage": last.Usage},
	})
}

func (s *Server) handleIndexStatus(w http.ResponseWriter, r *http.Request, id string) {
	sess, ok := s.registry.get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	svc, _, root := sess.indexTarget()
	if svc == nil {
		http.Error(w, "semantic index unavailable", http.StatusServiceUnavailable)
		return
	}
	st, err := svc.Status(r.Context(), root)
	if err != nil {
		http.Error(w, "index status failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeOpsJSON(w, http.StatusOK, map[string]any{
		"exists":         st.Exists,
		"compatible":     st.Compatible,
		"enabled":        st.Enabled,
		"model":          st.Model,
		"dimensions":     st.Dimensions,
		"schema_version": st.SchemaVersion,
		"files":          st.FileCount,
		"records":        st.RecordCount,
		"vectors":        st.VectorCount,
		"updated_at":     st.UpdatedAt,
		"running":        s.indexBusy(root),
	})
}

func (s *Server) indexBusy(root string) bool {
	s.indexRuns.mu.Lock()
	defer s.indexRuns.mu.Unlock()
	_, busy := s.indexRuns.roots[root]
	return busy
}

// indexTarget returns the session's semantic service, config and workspace.
func (s *Session) indexTarget() (semantic.Service, semantic.Config, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.semanticSvc, s.semanticCfg, s.appState.ToolSettings.WorkingDir
}

type indexBuildRequest struct {
	Mode string `json:"mode"`
}

// handleIndexBuild starts a build or refresh in the background and answers
// 202; progress arrives as index_progress events and the end as index_done or
// index_error.
func (s *Server) handleIndexBuild(w http.ResponseWriter, r *http.Request, id string) {
	sess, ok := s.registry.get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	var req indexBuildRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "build"
	}
	if mode != "build" && mode != "refresh" {
		http.Error(w, "mode must be build or refresh", http.StatusBadRequest)
		return
	}
	svc, cfg, root := sess.indexTarget()
	if svc == nil {
		http.Error(w, "semantic index unavailable", http.StatusServiceUnavailable)
		return
	}
	if !s.acquireIndexRun(root) {
		http.Error(w, "index already running", http.StatusConflict)
		return
	}
	go s.runIndex(sess, svc, cfg, root, mode)
	writeOpsJSON(w, http.StatusAccepted, map[string]any{"started": true, "mode": mode})
}

func (s *Server) runIndex(sess *Session, svc semantic.Service, cfg semantic.Config, root, mode string) {
	var release sync.Once
	releaseRun := func() { release.Do(func() { s.releaseIndexRun(root) }) }
	defer releaseRun()
	sink := indexEventSink{sess: sess, mode: mode}
	sess.Emit("index_started", map[string]any{"mode": mode})
	var (
		report semantic.BuildReport
		err    error
	)
	if mode == "refresh" {
		report, err = svc.Refresh(sess.ctx, semantic.RefreshRequest{Root: root, Config: cfg, MaxFiles: cfg.PromptRefreshMaxFiles, Timeout: cfg.PromptRefreshTimeout, EventSink: sink})
	} else {
		report, err = svc.Build(sess.ctx, semantic.BuildRequest{Root: root, Config: cfg, EventSink: sink})
	}
	releaseRun()
	if err != nil {
		sess.Emit("index_error", map[string]any{"mode": mode, "error": err.Error()})
		return
	}
	sess.Emit("index_done", map[string]any{
		"mode":          mode,
		"files_seen":    report.FilesSeen,
		"files_indexed": report.FilesIndexed,
		"files_skipped": report.FilesSkipped,
		"records":       report.RecordsIndexed,
		"batches":       report.EmbedBatches,
		"duration_ms":   report.Duration.Milliseconds(),
	})
}

// indexEventSink turns semantic progress events into index_progress session
// events. File paths are left out on purpose.
type indexEventSink struct {
	sess *Session
	mode string
}

func (k indexEventSink) Publish(e semantic.Event) {
	k.sess.Emit("index_progress", map[string]any{
		"mode":          k.mode,
		"stage":         string(e.Stage),
		"message":       e.Message,
		"files_total":   e.FilesTotal,
		"files_seen":    e.FilesSeen,
		"files_done":    e.FilesDone,
		"files_indexed": e.FilesIndexed,
		"records_done":  e.RecordsDone,
		"records_total": e.RecordsTotal,
		"batches_done":  e.BatchesDone,
		"total_batches": e.TotalBatches,
		"duration_ms":   e.Duration.Milliseconds(),
	})
}
