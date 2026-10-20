package server

import (
	"encoding/json"
	"net/http"
)

// cancelRun aborts the session's active run, if any. The run still ends with
// its normal terminal event (emitted by the run goroutine), so this emits
// nothing itself. It reports whether a run was active.
func (s *Session) cancelRun() bool { return s.ctl.Cancel() }

// handleCancelSession serves POST /v1/sessions/{id}/cancel. It answers 202 as
// soon as the cancel is requested; the client learns the outcome from the
// run's terminal event. Cancelling an idle session is a harmless 202.
func (s *Server) handleCancelSession(w http.ResponseWriter, r *http.Request, id string) {
	sess, ok := s.registry.get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	cancelled := sess.cancelRun()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"cancelled": cancelled})
}
