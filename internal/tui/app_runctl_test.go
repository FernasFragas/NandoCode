package tui

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/hooks"
	"github.com/FernasFragas/Nandocode/internal/state"
)

// A prompt typed after the run's Terminal event but before the run command has
// returned (agentDoneMsg) must be queued, not start a second run that would
// replace the first run's cancel handle.
func TestPromptBetweenTerminalAndDoneIsQueued(t *testing.T) {
	r := newBlockingRunner()
	m := newCharModel(t, r)
	m.input.SetValue("first")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	go cmd()
	select {
	case <-r.started:
	case <-time.After(5 * time.Second):
		t.Fatal("run did not start")
	}
	m.handleAgentEvent(agent.Terminal{Reason: agent.TerminalCompleted})
	if m.store.Get().ActiveRun {
		t.Fatal("Terminal should clear ActiveRun")
	}

	m.input.SetValue("second")
	_, cmd2 := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd2 != nil {
		go cmd2()
	}
	select {
	case <-r.started:
		t.Fatal("second prompt started a run while the first was still active")
	case <-time.After(200 * time.Millisecond):
	}
	if q := m.store.Get().QueuedPrompts; len(q) != 1 {
		t.Fatalf("QueuedPrompts = %v, want the second prompt queued", q)
	}
	close(r.release)
}

type fakeCompactHooks struct {
	events []hooks.Event
	deny   bool
	meta   []map[string]string
}

func (f *fakeCompactHooks) Dispatch(_ context.Context, env hooks.Envelope) hooks.Result {
	f.events = append(f.events, env.Event)
	f.meta = append(f.meta, env.Metadata)
	if f.deny && env.Event == hooks.EventPreCompact {
		return hooks.Result{Decision: hooks.DecisionDeny, Reason: "no compaction today"}
	}
	return hooks.Result{}
}

func TestIdleCompactDispatchesPreAndPostCompactHooks(t *testing.T) {
	m := newCharModel(t, &recordingRunner{})
	h := &fakeCompactHooks{}
	m.SetCompactHooks(h, "sess-1")
	m.store.Set(func(a state.App) state.App { a.Messages = charTurns(8); return a })
	runSlash(m, "/compact")
	if len(h.events) != 2 || h.events[0] != hooks.EventPreCompact || h.events[1] != hooks.EventPostCompact {
		t.Fatalf("events = %v", h.events)
	}
	if h.meta[0]["trigger"] != "manual" || h.meta[1]["messages_before"] != "16" {
		t.Fatalf("meta = %v", h.meta)
	}
	if len(m.store.Get().Messages) >= 16 {
		t.Fatal("history should have been compacted")
	}
}

func TestIdleCompactPreCompactDenyKeepsHistory(t *testing.T) {
	m := newCharModel(t, &recordingRunner{})
	h := &fakeCompactHooks{deny: true}
	m.SetCompactHooks(h, "sess-1")
	m.store.Set(func(a state.App) state.App { a.Messages = charTurns(8); return a })
	runSlash(m, "/compact")
	if len(m.store.Get().Messages) != 16 {
		t.Fatalf("messages = %d, want 16 untouched", len(m.store.Get().Messages))
	}
	if len(h.events) != 1 {
		t.Fatalf("events = %v, want only PreCompact", h.events)
	}
}

func TestIdleCompactBelowMinTurnsFiresNoHooks(t *testing.T) {
	m := newCharModel(t, &recordingRunner{})
	h := &fakeCompactHooks{}
	m.SetCompactHooks(h, "sess-1")
	m.store.Set(func(a state.App) state.App { a.Messages = charTurns(1); return a })
	runSlash(m, "/compact")
	if len(h.events) != 0 {
		t.Fatalf("events = %v", h.events)
	}
}

// Between the run's Terminal event and agentDoneMsg the run is over
// (ActiveRun=false) even though the controller has not been released. /compact
// must take the idle path there, as it did before runctl.
func TestCompactBetweenTerminalAndDoneTakesIdlePath(t *testing.T) {
	r := newBlockingRunner()
	m := newCharModel(t, r)
	m.store.Set(func(a state.App) state.App { a.Messages = charTurns(8); return a })
	m.input.SetValue("first")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	go cmd()
	select {
	case <-r.started:
	case <-time.After(5 * time.Second):
		t.Fatal("run did not start")
	}
	m.handleAgentEvent(agent.Terminal{Reason: agent.TerminalCompleted})
	before := len(m.store.Get().Messages)

	runSlash(m, "/compact")
	if g := lastSystemContent(m); g == "[Compact requested...]" {
		t.Fatalf("compact was sent to a finished run: %q", g)
	}
	if after := len(m.store.Get().Messages); after >= before {
		t.Fatalf("history not compacted: %d -> %d", before, after)
	}
	m.Update(agentDoneMsg{})
	close(r.release)
}
