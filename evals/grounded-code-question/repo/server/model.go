package server

import (
	"encoding/json"
	"net/http"
)

// ModelCatalog lists the models a session may use.
type ModelCatalog interface {
	Local() []string
	Cloud() []string
}

type Server struct {
	catalog      ModelCatalog
	cloudEnabled bool
	hasCloudKey  bool
	models       map[string]string
}

type modelRequest struct {
	Model string `json:"model"`
}

// handleUpdateModel switches the active model of one session.
// A cloud-listed model is accepted only when cloud is enabled and a cloud
// credential is present; otherwise the request fails with 403.
func (s *Server) handleUpdateModel(w http.ResponseWriter, r *http.Request) {
	var req modelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Model == "" {
		http.Error(w, "model is required", http.StatusBadRequest)
		return
	}
	if contains(s.catalog.Local(), req.Model) {
		s.models[r.PathValue("id")] = req.Model
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if contains(s.catalog.Cloud(), req.Model) {
		if !s.cloudEnabled || !s.hasCloudKey {
			http.Error(w, "cloud model requires an enabled cloud and a credential", http.StatusForbidden)
			return
		}
		s.models[r.PathValue("id")] = req.Model
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Error(w, "unknown model", http.StatusNotFound)
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {}
func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request)   {}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
