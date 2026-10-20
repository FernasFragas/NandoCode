package server

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/bootstrap"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/permissions"
	"github.com/FernasFragas/Nandocode/internal/state"
)

// eventually polls cond on a ticker until it holds or a deadline passes.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	if cond() {
		return
	}
	tick := time.NewTicker(2 * time.Millisecond)
	defer tick.Stop()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-tick.C:
			if cond() {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

// recordingRunner records every Input it is given and completes each run with
// a canned assistant answer "A<n>".
type recordingRunner struct {
	mu     sync.Mutex
	inputs []agent.Input
}

func (r *recordingRunner) Run(_ context.Context, in agent.Input) <-chan agent.Event {
	r.mu.Lock()
	r.inputs = append(r.inputs, in)
	n := len(r.inputs)
	r.mu.Unlock()
	ch := make(chan agent.Event, 1)
	ch <- agent.Terminal{Reason: agent.TerminalCompleted, Conversation: []llm.Message{{Role: llm.RoleAssistant, Content: "A" + string(rune('0'+n))}}}
	close(ch)
	return ch
}

func (r *recordingRunner) input(i int) agent.Input {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.inputs[i]
}

// runAndWait starts one run and blocks until its terminal event is emitted.
func runAndWait(t *testing.T, s *Session, prompt string) {
	t.Helper()
	sub := s.Subscribe()
	defer s.Unsubscribe(sub)
	if err := s.StartRun(MessageRequest{Prompt: prompt}, agent.DefaultConfig()); err != nil {
		t.Fatalf("StartRun(%q) err=%v", prompt, err)
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case evt := <-sub:
			if evt.Type == "terminal" {
				// running flips to false in a deferred func just after the last event.
				eventually(t, "run to finish", func() bool {
					s.mu.Lock()
					defer s.mu.Unlock()
					return !s.running
				})
				return
			}
		case <-deadline:
			t.Fatalf("no terminal event for %q", prompt)
		}
	}
}

func TestStartRunKeepsConversationAcrossTurns(t *testing.T) {
	init := bootstrap.DefaultInitial(t.TempDir())
	app := state.DefaultApp(bootstrap.New(init).Snapshot())
	rr := &recordingRunner{}
	s := newSession(context.Background(), "s-hist", app, rr)

	runAndWait(t, s, "Q1")
	runAndWait(t, s, "Q2")

	got := rr.input(1).Messages
	want := []struct {
		role    llm.Role
		content string
	}{{llm.RoleUser, "Q1"}, {llm.RoleAssistant, "A1"}, {llm.RoleUser, "Q2"}}
	if len(got) != len(want) {
		t.Fatalf("second run saw %d messages, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Role != w.role || !strings.Contains(got[i].Content, w.content) {
			t.Fatalf("message %d = %s %q, want %s containing %q", i, got[i].Role, got[i].Content, w.role, w.content)
		}
	}
}

func TestAlwaysAllowPersistsExactRuleForNextRun(t *testing.T) {
	init := bootstrap.DefaultInitial(t.TempDir())
	app := state.DefaultApp(bootstrap.New(init).Snapshot())
	rr := &recordingRunner{}
	s := newSession(context.Background(), "s-allow", app, rr)

	// Simulate the agent asking for permission during a run.
	promptDone := make(chan struct{})
	go func() {
		defer close(promptDone)
		_, _, _ = s.broker.PromptFunc()(context.Background(), permissions.Prompt{ToolName: "Bash", Target: "go test ./...", Reason: "ask"})
	}()
	var reqID string
	eventually(t, "permission_request", func() bool {
		for _, e := range s.Replay("") {
			if e.Type == "permission_request" {
				reqID, _ = e.Data["request_id"].(string)
			}
		}
		return reqID != ""
	})
	if !s.ResolvePermission(reqID, "always_allow") {
		t.Fatal("always_allow resolve failed")
	}
	<-promptDone

	runAndWait(t, s, "next")
	rules := rr.input(0).PermissionRules
	if _, d, ok := rules.FirstMatchingRule("Bash", "go test ./..."); !ok || d != permissions.DecisionAllow {
		t.Fatalf("approved call must be auto-allowed on the next run, ok=%v d=%v", ok, d)
	}
	if _, _, ok := rules.FirstMatchingRule("Bash", "rm -rf /"); ok {
		t.Fatal("Always Allow must only cover the approved tool and target, not every call")
	}
	if _, _, ok := rules.FirstMatchingRule("Write", "go test ./..."); ok {
		t.Fatal("Always Allow must not cover other tools")
	}
}
