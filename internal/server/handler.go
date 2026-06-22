package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/FernasFragas/nandocodego/internal/agent"
	"github.com/FernasFragas/nandocodego/internal/llm"
	"github.com/FernasFragas/nandocodego/internal/llm/modelruntime"
	"github.com/FernasFragas/nandocodego/internal/state"
	"github.com/FernasFragas/nandocodego/internal/tasks"
)

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	if !s.limiter.AllowRequest(r) {
		http.Error(w, "rate limit", http.StatusTooManyRequests)
		return
	}
	if !s.limiter.AcquireSession() {
		http.Error(w, "session limit reached", http.StatusTooManyRequests)
		return
	}
	id := fmt.Sprintf("sess_%d", time.Now().UnixNano())
	app := s.baseApp.Clone()
	runner := s.runner
	var supervisor *tasks.Supervisor
	var taskStore *state.Store[state.App]
	if agent.IsCoordinatorMode() {
		coordRunner, sup, store, err := s.buildCoordinatorSessionRunner(id, app)
		if err != nil {
			http.Error(w, "failed to initialize coordinator session", http.StatusInternalServerError)
			s.limiter.ReleaseSession()
			return
		}
		runner = coordRunner
		supervisor = sup
		taskStore = store
	}
	sess := newSession(s.ctx, id, app, runner)
	sess.semanticCfg = s.semanticCfg
	sess.semanticSvc = s.semanticSvc
	if supervisor != nil {
		sess.setCoordinatorRuntime(supervisor, taskStore, s.client, agent.IsDreamEnabled())
	}
	s.registry.set(sess)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(sess.view())
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request, id string) {
	sess, ok := s.registry.get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sess.view())
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request, id string) {
	sess, ok := s.registry.get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	sess.stop()
	s.registry.del(id)
	s.limiter.ReleaseSession()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePostMessage(w http.ResponseWriter, r *http.Request, id string) {
	sess, ok := s.registry.get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	var req MessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" {
		http.Error(w, "prompt required", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.MessageID) != "" && !sess.allowMessageID(req.MessageID) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{"queued": true, "duplicate": true})
		return
	}
	if s.modelRuntime != nil {
		switchRes, err := s.modelRuntime.Switch(r.Context(), modelruntime.SwitchOptions{
			RequestedModel: sess.activeModel(),
			AllowPrompt:    false,
		})
		if err != nil {
			if errors.Is(err, modelruntime.ErrCredentialRequired) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusPreconditionRequired)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error":      "requires_credential",
					"provider":   string(llm.ProviderOllamaCloudAPI),
					"credential": "OLLAMA_API_KEY",
				})
				return
			}
			http.Error(w, "model unavailable: "+err.Error(), http.StatusBadGateway)
			return
		}
		sess.applyModelSwitch(switchRes)
	}
	if err := sess.StartRun(req, s.agentCfg); err != nil {
		http.Error(w, "run already active", http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"queued": true})
}

func (s *Server) handleResolvePermission(w http.ResponseWriter, r *http.Request, id, reqID string) {
	sess, ok := s.registry.get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	var req PermissionResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if !sess.ResolvePermission(reqID, req.Decision) {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request, id string) {
	sess, ok := s.registry.get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	sseHeaders(w)
	lastID := strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	for _, evt := range sess.Replay(lastID) {
		if err := writeSSE(w, evt); err != nil {
			return
		}
	}
	sub := sess.Subscribe()
	defer sess.Unsubscribe(sub)
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case evt := <-sub:
			if err := writeSSE(w, evt); err != nil {
				return
			}
		case <-t.C:
			if err := writeSSE(w, heartbeatEvent(id)); err != nil {
				return
			}
		}
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := "reachable"
	if _, err := s.client.ListModels(r.Context()); err != nil {
		status = "unreachable"
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "ollama": status})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if s.modelRuntime != nil {
		models, err := s.modelRuntime.ListLocal(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(modelsResponse{Models: models})
		return
	}
	models, err := s.client.ListModels(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(modelsResponse{Models: models})
}

func (s *Server) handleUpdateModel(w http.ResponseWriter, r *http.Request, id string) {
	sess, ok := s.registry.get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	var req ModelUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	req.Model = strings.TrimSpace(req.Model)
	if req.Model == "" {
		http.Error(w, "model required", http.StatusBadRequest)
		return
	}
	sess.mu.Lock()
	running := sess.running
	sess.mu.Unlock()
	if running {
		http.Error(w, "run already active", http.StatusConflict)
		return
	}
	if s.modelRuntime != nil {
		switchRes, err := s.modelRuntime.Switch(r.Context(), modelruntime.SwitchOptions{
			RequestedModel: req.Model,
			AllowPrompt:    false,
		})
		if err != nil {
			if errors.Is(err, modelruntime.ErrModelNotFound) {
				http.Error(w, "model not found", http.StatusBadRequest)
				return
			}
			http.Error(w, "model switch failed: "+err.Error(), http.StatusBadRequest)
			return
		}
		sess.applyModelSwitch(switchRes)
	} else {
		models, err := s.client.ListModels(r.Context())
		if err != nil {
			http.Error(w, "failed to list models", http.StatusBadGateway)
			return
		}
		found := false
		for _, m := range models {
			if m.Name == req.Model {
				found = true
				break
			}
		}
		if !found {
			http.Error(w, "model not found", http.StatusBadRequest)
			return
		}
		sess.mu.Lock()
		sess.appState.ActiveModel = req.Model
		sess.mu.Unlock()
	}
	sess.mu.Lock()
	model := sess.appState.ActiveModel
	provider := sess.appState.LLMProvider
	baseURL := sess.appState.LLMBaseURL
	sess.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"model":    model,
		"provider": provider,
		"base_url": baseURL,
	})
}

func (s *Server) handleGetTree(w http.ResponseWriter, r *http.Request, id string) {
	sess, ok := s.registry.get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	qPath := r.URL.Query().Get("path")
	if qPath == "" {
		qPath = "."
	}
	depth := 2
	if dStr := r.URL.Query().Get("depth"); dStr != "" {
		if d, err := strconv.Atoi(dStr); err == nil {
			if d > 4 {
				d = 4
			}
			if d < 0 {
				d = 0
			}
			depth = d
		}
	}
	sess.mu.Lock()
	base := sess.appState.ToolSettings.WorkingDir
	additional := append([]string(nil), sess.appState.ToolSettings.AdditionalWorkingDirs...)
	sess.mu.Unlock()
	if base == "" {
		var err error
		base, err = os.Getwd()
		if err != nil {
			http.Error(w, "unable to determine working directory", http.StatusInternalServerError)
			return
		}
	}
	target := filepath.Join(base, qPath)
	cleanTarget := filepath.Clean(target)
	allowedPrefixes := []string{filepath.Clean(base)}
	for _, d := range additional {
		if d != "" {
			allowedPrefixes = append(allowedPrefixes, filepath.Clean(d))
		}
	}
	safe := false
	for _, prefix := range allowedPrefixes {
		if isPathInside(cleanTarget, prefix) {
			safe = true
			break
		}
	}
	if !safe {
		http.Error(w, "path traversal not allowed", http.StatusForbidden)
		return
	}
	info, err := os.Stat(cleanTarget)
	if err != nil {
		http.Error(w, "path not found", http.StatusNotFound)
		return
	}
	if !info.IsDir() {
		http.Error(w, "not a directory", http.StatusBadRequest)
		return
	}
	const maxFiles = 500
	excludes := map[string]bool{
		".git":         true,
		"node_modules": true,
		"vendor":       true,
		"dist":         true,
		".DS_Store":    true,
		"build":        true,
		"out":          true,
		"bin":          true,
		"target":       true,
		"__pycache__":  true,
	}
	entries := []TreeEntry{}
	truncated := false
	reason := ""
	err = filepath.WalkDir(cleanTarget, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, relErr := filepath.Rel(cleanTarget, path)
		if relErr != nil {
			return nil
		}
		if rel == "." {
			return nil
		}
		relDepth := strings.Count(rel, string(filepath.Separator)) + 1
		if relDepth > depth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if excludes[name] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if len(entries) >= maxFiles {
			truncated = true
			reason = "max_files"
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		entries = append(entries, TreeEntry{
			Path:  filepath.ToSlash(rel),
			Name:  name,
			IsDir: d.IsDir(),
		})
		return nil
	})
	if err != nil {
		http.Error(w, "walk error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(TreeResponse{
		Root:    cleanTarget,
		Entries: entries,
		Stats: TreeStats{
			Truncated: truncated,
			Reason:    reason,
			Source:    "local",
		},
	})
}

func isPathInside(target, prefix string) bool {
	target = filepath.Clean(target)
	prefix = filepath.Clean(prefix)
	if target == prefix {
		return true
	}
	sep := string(filepath.Separator)
	if !strings.HasSuffix(prefix, sep) {
		prefix += sep
	}
	return strings.HasPrefix(target, prefix)
}
