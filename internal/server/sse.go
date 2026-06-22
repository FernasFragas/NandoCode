package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func writeSSE(w http.ResponseWriter, evt SessionEvent) error {
	b, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", evt.ID, evt.Type, b); err != nil {
		return err
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

func sseHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
}

func heartbeatEvent(sessionID string) SessionEvent {
	return SessionEvent{
		ID:        fmt.Sprintf("hb-%d", time.Now().UnixNano()),
		Type:      "heartbeat",
		Time:      time.Now().UTC(),
		SessionID: sessionID,
	}
}
