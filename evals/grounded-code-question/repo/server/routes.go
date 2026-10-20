package server

import "net/http"

// Routes registers the session API.
func (s *Server) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/sessions", s.handleCreateSession)
	mux.HandleFunc("POST /v1/sessions/{id}/model", s.handleUpdateModel)
	mux.HandleFunc("POST /v1/sessions/{id}/messages", s.handleSendMessage)
	return mux
}
