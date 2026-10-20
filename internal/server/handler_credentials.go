package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/FernasFragas/Nandocode/internal/credentials"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/llm/ollama"
)

// maxCredentialBody bounds the key request; an API key is far smaller.
const maxCredentialBody = 8 << 10

type credentialRequest struct {
	Key  string `json:"key"`
	Save bool   `json:"save"`
}

// writeCredentialRequired answers the "choose a cloud model without a key"
// case the same way for the message and model endpoints.
func writeCredentialRequired(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusPreconditionRequired)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":      "requires_credential",
		"provider":   string(llm.ProviderOllamaCloudAPI),
		"credential": "OLLAMA_API_KEY",
	})
}

// handleOllamaCloudCredential accepts an Ollama Cloud API key from the
// browser. The key is validated against the cloud catalog (which sends no
// project context), then held in process memory (save=false) or also written
// to the OS keychain (save=true). The key is never logged or echoed.
func (s *Server) handleOllamaCloudCredential(w http.ResponseWriter, r *http.Request) {
	svc := s.modelRuntime
	if svc == nil || svc.Creds == nil {
		http.Error(w, "credentials unavailable", http.StatusServiceUnavailable)
		return
	}
	var req credentialRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCredentialBody)).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	key := strings.TrimSpace(req.Key)
	if key == "" {
		http.Error(w, "key required", http.StatusBadRequest)
		return
	}
	newClient := svc.NewCloudClient
	if newClient == nil {
		newClient = func(baseURL, apiKey string) llm.Client {
			return ollama.NewClientWithOptions(ollama.Options{BaseURL: baseURL, APIKey: apiKey})
		}
	}
	if _, err := newClient(llm.OllamaCloudBaseURL, key).ListModels(r.Context()); err != nil {
		// err text may echo provider detail; classify it, never relay it.
		msg := err.Error()
		switch {
		case strings.Contains(msg, "status 401"), strings.Contains(msg, "status 403"):
			http.Error(w, "ollama cloud rejected the key", http.StatusUnauthorized)
		default:
			http.Error(w, "ollama cloud unavailable", http.StatusBadGateway)
		}
		s.logCredential("ollama cloud key rejected")
		return
	}
	source := credentials.KeySourceSession
	if err := svc.Creds.Remember(key, req.Save); err != nil {
		s.logCredential("ollama cloud key save failed")
		http.Error(w, "could not save key to keychain", http.StatusInternalServerError)
		return
	}
	if req.Save {
		source = credentials.KeySourceKeychain
	}
	s.logCredential("ollama cloud key stored", "source", string(source))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":       true,
		"provider": string(llm.ProviderOllamaCloudAPI),
		"stored":   string(source),
	})
}

func (s *Server) logCredential(msg string, args ...any) {
	if s.logger != nil {
		s.logger.Info(msg, args...)
	}
}
