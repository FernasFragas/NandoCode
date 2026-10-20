package server

import (
	"context"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/bootstrap"
	"github.com/FernasFragas/Nandocode/internal/permissions"
	"github.com/FernasFragas/Nandocode/internal/state"
)

type promptOutcome struct {
	decision permissions.Decision
	reason   string
}

// startPrompt runs one prompt and returns its request id and result channel.
func startPrompt(t *testing.T, ctx context.Context, b *HTTPPermissionBroker, s *Session) (string, chan promptOutcome, chan SessionEvent) {
	t.Helper()
	stream := s.Subscribe()
	t.Cleanup(func() { s.Unsubscribe(stream) })
	out := make(chan promptOutcome, 1)
	go func() {
		d, r, _ := b.PromptFunc()(ctx, permissions.Prompt{ToolName: "Bash", Target: "echo hi", Reason: "ask"})
		out <- promptOutcome{d, r}
	}()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case e := <-stream:
			if e.Type == "permission_request" {
				id, _ := e.Data["request_id"].(string)
				return id, out, stream
			}
		case <-deadline.C:
			t.Fatal("no permission_request")
		}
	}
}

func waitResolved(t *testing.T, stream chan SessionEvent) SessionEvent {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case e := <-stream:
			if e.Type == "permission_resolved" {
				return e
			}
		case <-deadline.C:
			t.Fatal("no permission_resolved")
		}
	}
}

func newPermTestSession(t *testing.T) (*Session, *HTTPPermissionBroker) {
	t.Helper()
	app := state.DefaultApp(bootstrap.New(bootstrap.DefaultInitial(".")).Snapshot())
	s := newSession(context.Background(), "s1", app, nil)
	return s, NewHTTPPermissionBroker(s)
}

func countResolved(s *Session) int {
	n := 0
	for _, e := range s.Replay("") {
		if e.Type == "permission_resolved" {
			n++
		}
	}
	return n
}

func TestPermissionCardClosesOnBrokerTimeout(t *testing.T) {
	s, b := newPermTestSession(t)
	b.timeout = 20 * time.Millisecond
	id, out, stream := startPrompt(t, context.Background(), b, s)
	e := waitResolved(t, stream)
	if e.Data["request_id"] != id || e.Data["decision"] != "timeout" {
		t.Fatalf("event = %v", e.Data)
	}
	if r := <-out; r.decision != permissions.DecisionDeny || r.reason != "timeout" {
		t.Fatalf("outcome = %+v", r)
	}
	if b.Resolve(id, decisionAllow) {
		t.Fatal("late resolve must fail")
	}
	if n := countResolved(s); n != 1 {
		t.Fatalf("resolved events = %d", n)
	}
}

func TestPermissionResolvedOnUserDecisions(t *testing.T) {
	for _, tc := range []struct {
		in   permissionDecision
		want string
	}{{decisionAllow, "allow"}, {decisionDeny, "deny"}, {decisionAlwaysAllow, "always_allow"}} {
		t.Run(tc.want, func(t *testing.T) {
			s, b := newPermTestSession(t)
			id, out, stream := startPrompt(t, context.Background(), b, s)
			if !b.Resolve(id, tc.in) {
				t.Fatal("resolve failed")
			}
			e := waitResolved(t, stream)
			if e.Data["request_id"] != id || e.Data["decision"] != tc.want {
				t.Fatalf("event = %v", e.Data)
			}
			<-out
			if b.Resolve(id, decisionAllow) {
				t.Fatal("second resolve must fail")
			}
			if n := countResolved(s); n != 1 {
				t.Fatalf("resolved events = %d", n)
			}
		})
	}
}

func TestPermissionResolvedOnRunCancel(t *testing.T) {
	s, b := newPermTestSession(t)
	ctx, cancel := context.WithCancel(context.Background())
	id, out, stream := startPrompt(t, ctx, b, s)
	cancel()
	e := waitResolved(t, stream)
	if e.Data["request_id"] != id || e.Data["decision"] != "deny" {
		t.Fatalf("event = %v", e.Data)
	}
	if r := <-out; r.reason != "cancelled" {
		t.Fatalf("outcome = %+v", r)
	}
}
