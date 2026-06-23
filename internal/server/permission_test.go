package server

import (
	"context"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/bootstrap"
	"github.com/FernasFragas/Nandocode/internal/permissions"
	"github.com/FernasFragas/Nandocode/internal/state"
)

func TestHTTPPermissionBrokerResolve(t *testing.T) {
	init := bootstrap.DefaultInitial(".")
	app := state.DefaultApp(bootstrap.New(init).Snapshot())
	s := newSession(context.Background(), "s1", app, nil)
	b := NewHTTPPermissionBroker(s)
	prompt := b.PromptFunc()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _ = prompt(context.Background(), permissions.Prompt{ToolName: "bash", Target: "*", Reason: "ask"})
	}()
	time.Sleep(20 * time.Millisecond)
	evts := s.Replay("")
	var reqID string
	for _, e := range evts {
		if e.Type == "permission_request" {
			if v, ok := e.Data["request_id"].(string); ok {
				reqID = v
			}
		}
	}
	if reqID == "" {
		t.Fatal("missing permission request id")
	}
	if !b.Resolve(reqID, decisionAllow) {
		t.Fatal("resolve should succeed")
	}
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("prompt did not unblock")
	}
}
