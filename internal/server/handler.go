package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/llm/modelruntime"
	"github.com/FernasFragas/Nandocode/internal/state"
	"github.com/FernasFragas/Nandocode/internal/tasks"
)

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	if !s.limiter.AllowRequest(r) {
		http.Error(w, "rate limit", http.StatusTooManyRequests)
		return
	}
	if s.reuseBlankSession(w, r) {
		return
	}
	if !s.acquireSessionSlot() {
		http.Error(w, "session limit reached", http.StatusTooManyRequests)
		return
	}
	id, err := newSessionID()
	if err != nil {
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		s.limiter.ReleaseSession()
		return
	}
	app := s.baseApp.Clone()
	rt, err := s.newSessionRuntime()
	if err != nil {
		http.Error(w, "failed to initialize session runtime", http.StatusInternalServerError)
		s.limiter.ReleaseSession()
		return
	}
	runner := rt.runner
	var supervisor *tasks.Supervisor
	var taskStore *state.Store[state.App]
	if agent.IsCoordinatorMode() {
		coordRunner, sup, store, err := s.buildCoordinatorSessionRunner(rt.client, id, app)
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
	sess.setRuntime(rt)
	sess.semanticCfg = s.semanticCfg
	sess.semanticSvc = s.semanticSvc
	if supervisor != nil {
		sess.setCoordinatorRuntime(supervisor, taskStore, rt.client, agent.IsDreamEnabled())
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
	if s.registry.remove(id) {
		s.limiter.ReleaseSession()
	}
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
	if models := sess.modelService(); models != nil && !sess.modelSwitchCurrent() {
		activated, err := models.Activate(r.Context(), sess.activeModel())
		if err != nil {
			if errors.Is(err, modelruntime.ErrCredentialRequired) {
				writeCredentialRequired(w)
				return
			}
			http.Error(w, "model unavailable: "+err.Error(), http.StatusBadGateway)
			return
		}
		sess.applyActivation(activated)
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
		// The model picker uses this catalog for both local and direct Cloud
		// selection. Keep local entries first and usable if Cloud is unavailable.
		if cloud, err := s.modelRuntime.ListCloud(r.Context()); err == nil {
			seen := make(map[string]bool, len(models)+len(cloud))
			for _, model := range models {
				seen[model.Name] = true
			}
			for _, model := range cloud {
				if !seen[model.Name] {
					models = append(models, model)
					seen[model.Name] = true
				}
			}
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
	if models := sess.modelService(); models != nil {
		activated, err := models.Activate(r.Context(), req.Model)
		if err != nil {
			if errors.Is(err, modelruntime.ErrCredentialRequired) {
				writeCredentialRequired(w)
				return
			}
			if errors.Is(err, modelruntime.ErrModelNotFound) {
				http.Error(w, "model not found", http.StatusBadRequest)
				return
			}
			if errors.Is(err, modelruntime.ErrCloudModelRetired) {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, "model switch failed: "+err.Error(), http.StatusBadRequest)
			return
		}
		sess.applyActivation(activated)
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
