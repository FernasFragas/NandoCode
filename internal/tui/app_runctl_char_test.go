package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/analysis"
	"github.com/FernasFragas/Nandocode/internal/bootstrap"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/state"
)

// Characterization tests for run control (cancel, compact, terminal handling,
// checkpoint, /clear). They pin the behaviour the TUI had before it moved onto
// internal/runctl and must pass both before and after that switch.

// blockingRunner starts a run that stays open until released.
type blockingRunner struct {
	started chan agent.Input
	release chan struct{}
}

func newBlockingRunner() *blockingRunner {
	return &blockingRunner{started: make(chan agent.Input, 4), release: make(chan struct{})}
}

func (r *blockingRunner) Run(ctx context.Context, in agent.Input) <-chan agent.Event {
	r.started <- in
	out := make(chan agent.Event)
	go func() {
		defer close(out)
		select {
		case <-r.release:
		case <-ctx.Done():
		}
	}()
	return out
}

func newCharModel(t *testing.T, runner AgentRunner) *Model {
	t.Helper()
	initial := bootstrap.DefaultInitial(t.TempDir())
	initial.DefaultModel = "test-model"
	store := state.NewStore(state.DefaultApp(bootstrap.New(initial).Snapshot()), nil)
	model, err := New(store, runner, nil, nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return model
}

func charTurns(n int) []llm.Message {
	var msgs []llm.Message
	for i := 0; i < n; i++ {
		msgs = append(msgs,
			llm.Message{Role: llm.RoleUser, Content: fmt.Sprintf("q%d", i)},
			llm.Message{Role: llm.RoleAssistant, Content: fmt.Sprintf("a%d", i)})
	}
	return msgs
}

func lastSystemContent(m *Model) string {
	if len(m.transcript) == 0 {
		return ""
	}
	return m.transcript[len(m.transcript)-1].Content
}

func runSlash(m *Model, text string) {
	m.input.SetValue(text)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		go cmd()
	}
}

func TestCharCompactIdleBelowMinTurnsSkips(t *testing.T) {
	m := newCharModel(t, &recordingRunner{})
	m.store.Set(func(a state.App) state.App { a.Messages = charTurns(1); return a })
	runSlash(m, "/compact")
	if got := lastSystemContent(m); got != "[Compact skipped: not enough turns to compact (need at least 4, have 1)]" {
		t.Fatalf("transcript = %q", got)
	}
	if n := len(m.store.Get().Messages); n != 2 {
		t.Fatalf("messages = %d, want 2 untouched", n)
	}
}

func TestCharCompactIdleEmergencyTruncates(t *testing.T) {
	m := newCharModel(t, &recordingRunner{})
	msgs := charTurns(8)
	m.store.Set(func(a state.App) state.App { a.Messages = msgs; return a })
	want := agent.EmergencyTruncateExported(msgs, agent.DefaultCompactionConfig().MinTurns)
	runSlash(m, "/compact")
	got := m.store.Get().Messages
	if len(got) != len(want) || len(got) >= len(msgs) {
		t.Fatalf("messages = %d, want %d (< %d)", len(got), len(want), len(msgs))
	}
	for i := range want {
		if got[i].Role != want[i].Role || got[i].Content != want[i].Content {
			t.Fatalf("message %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	wantLine := fmt.Sprintf("[Compacted: %d → %d messages (emergency truncate)]", len(msgs), len(want))
	if g := lastSystemContent(m); g != wantLine {
		t.Fatalf("transcript = %q, want %q", g, wantLine)
	}
}

func TestCharCompactAndCancelDuringRun(t *testing.T) {
	r := newBlockingRunner()
	m := newCharModel(t, r)
	m.input.SetValue("hello")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected run command")
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var in agent.Input
	select {
	case in = <-r.started:
	case <-time.After(5 * time.Second):
		t.Fatal("run did not start")
	}
	if !m.store.Get().ActiveRun {
		t.Fatal("expected ActiveRun")
	}

	runSlash(m, "/compact")
	if g := lastSystemContent(m); g != "[Compact requested...]" {
		t.Fatalf("first compact = %q", g)
	}
	runSlash(m, "/compact")
	if g := lastSystemContent(m); g != "[Compact already pending]" {
		t.Fatalf("second compact = %q", g)
	}
	select {
	case <-in.CompactRequest:
	default:
		t.Fatal("compact signal not delivered to run")
	}
	runSlash(m, "/compact")
	if g := lastSystemContent(m); g != "[Compact requested...]" {
		t.Fatalf("compact after consume = %q", g)
	}

	select {
	case <-in.ParentAbort:
		t.Fatal("abort closed before cancel")
	default:
	}
	_, quit := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !quit {
		t.Fatal("ctrl-c should be consumed")
	}
	select {
	case <-in.ParentAbort:
	case <-time.After(5 * time.Second):
		t.Fatal("ParentAbort not closed after ctrl-c")
	}
	select {
	case <-in.ToolContext.Context.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("run context not cancelled")
	}
	select {
	case msg := <-done:
		if _, ok := msg.(agentDoneMsg); !ok {
			t.Fatalf("msg = %T", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run command did not finish")
	}
}

func TestCharPromptWhileRunActiveIsQueued(t *testing.T) {
	r := &recordingRunner{}
	m := newCharModel(t, r)
	m.store.Set(func(a state.App) state.App { a.ActiveRun = true; return a })
	m.input.SetValue("second")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		cmd()
	}
	app := m.store.Get()
	if r.calls != 0 || len(app.QueuedPrompts) != 1 {
		t.Fatalf("calls=%d queued=%v", r.calls, app.QueuedPrompts)
	}
	last := app.Messages[len(app.Messages)-1]
	if last.Role != llm.RoleUser {
		t.Fatalf("last message = %+v", last)
	}
}

func TestCharAgentDoneStartsQueuedPrompt(t *testing.T) {
	r := &recordingRunner{}
	m := newCharModel(t, r)
	m.store.Set(func(a state.App) state.App {
		a.ActiveRun = true
		a.Messages = []llm.Message{{Role: llm.RoleUser, Content: "queued one"}}
		a.QueuedPrompts = []string{"queued one"}
		return a
	})
	_, cmd := m.Update(agentDoneMsg{})
	if cmd == nil {
		t.Fatal("expected queued run to start")
	}
	// Batched commands may wrap the run; execute and look for a run.
	runAll(cmd)
	if r.calls != 1 {
		t.Fatalf("runner calls = %d, want 1", r.calls)
	}
	if !m.store.Get().ActiveRun || len(m.store.Get().QueuedPrompts) != 0 {
		t.Fatalf("state = %+v", m.store.Get().QueuedPrompts)
	}
}

func runAll(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	if b, ok := msg.(tea.BatchMsg); ok {
		for _, c := range b {
			runAll(c)
		}
	}
}

func TestCharTerminalAppendsConversationAndRecordsOutcome(t *testing.T) {
	m := newCharModel(t, &recordingRunner{})
	m.store.Set(func(a state.App) state.App {
		a.ActiveRun = true
		a.Messages = []llm.Message{{Role: llm.RoleUser, Content: "do it"}}
		return a
	})
	usage := agent.Usage{Turns: 2, ToolCalls: 3}
	m.handleAgentEvent(agent.Terminal{
		Reason:       agent.TerminalCompleted,
		Detail:       "ok",
		Usage:        usage,
		Conversation: []llm.Message{{Role: llm.RoleAssistant, Content: "done."}},
	})
	app := m.store.Get()
	if app.ActiveRun || app.TerminalReason != agent.TerminalCompleted || app.TerminalDetail != "ok" || app.Usage != usage {
		t.Fatalf("outcome not recorded: %+v", app)
	}
	if len(app.Messages) != 2 || app.Messages[1].Content != "done." {
		t.Fatalf("messages = %+v", app.Messages)
	}
}

func TestCharTerminalBTWRecordsOutcomeButKeepsHistory(t *testing.T) {
	m := newCharModel(t, &recordingRunner{})
	m.store.Set(func(a state.App) state.App {
		a.ActiveRun = true
		a.Messages = []llm.Message{{Role: llm.RoleUser, Content: "main"}}
		return a
	})
	m.activeRunKind = "btw"
	m.handleAgentEvent(agent.Terminal{
		Reason:       agent.TerminalMaxTurns,
		Conversation: []llm.Message{{Role: llm.RoleAssistant, Content: "side"}},
	})
	app := m.store.Get()
	if len(app.Messages) != 1 || app.ActiveRun || app.TerminalReason != agent.TerminalMaxTurns {
		t.Fatalf("app = %+v", app)
	}
	if m.activeRunKind != "main" {
		t.Fatalf("activeRunKind = %q", m.activeRunKind)
	}
}

func TestCharTerminalSavesCheckpoint(t *testing.T) {
	m := newCharModel(t, &recordingRunner{})
	_ = analysis.DeleteCheckpoint()
	m.store.Set(func(a state.App) state.App {
		a.ActiveModel = "test-model"
		a.Messages = []llm.Message{{Role: llm.RoleUser, Content: "inspect @main.go"}}
		return a
	})
	m.handleAgentEvent(agent.Terminal{
		Reason:       agent.TerminalMaxTurns,
		Detail:       "limit",
		Conversation: []llm.Message{{Role: llm.RoleAssistant, Content: "partial answer"}},
	})
	c, err := analysis.LoadCheckpoint()
	if err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if c.Model != "test-model" || c.LastUserPrompt != "inspect @main.go" || c.LastAssistantOutput != "partial answer" ||
		c.TerminalReason != string(agent.TerminalMaxTurns) || c.TerminalDetail != "limit" ||
		!c.PendingFinalAnswer || c.SynthesisStage != "synthesis_pending" {
		t.Fatalf("checkpoint = %+v", c)
	}
}

func TestCharTerminalCheckpointFallsBackToTranscriptAssistant(t *testing.T) {
	m := newCharModel(t, &recordingRunner{})
	_ = analysis.DeleteCheckpoint()
	m.transcript = append(m.transcript, TranscriptItem{Kind: TranscriptAssistant, Content: "from transcript"})
	m.store.Set(func(a state.App) state.App {
		a.Messages = []llm.Message{{Role: llm.RoleUser, Content: "hi"}}
		return a
	})
	m.handleAgentEvent(agent.Terminal{Reason: agent.TerminalCompleted})
	c, err := analysis.LoadCheckpoint()
	if err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if !strings.Contains(c.LastAssistantOutput, "from transcript") {
		t.Fatalf("checkpoint = %+v", c)
	}
}
