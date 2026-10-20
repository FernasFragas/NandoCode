package runctl

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/hooks"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/state"
)

func turns(n int) []llm.Message {
	var out []llm.Message
	for i := 0; i < n; i++ {
		out = append(out,
			msg(llm.RoleUser, fmt.Sprintf("q%d", i)),
			msg(llm.RoleAssistant, fmt.Sprintf("a%d", i)))
	}
	return out
}

type fakeHooks struct {
	mu        sync.Mutex
	envs      []hooks.Envelope
	preDeny   bool
	preReason string
}

func (f *fakeHooks) Dispatch(_ context.Context, env hooks.Envelope) hooks.Result {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.envs = append(f.envs, env)
	if env.Event == hooks.EventPreCompact && f.preDeny {
		return hooks.Result{Decision: hooks.DecisionDeny, Reason: f.preReason}
	}
	return hooks.Result{}
}

func (f *fakeHooks) events() []hooks.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []hooks.Event
	for _, e := range f.envs {
		out = append(out, e.Event)
	}
	return out
}

func TestResetConversationClearsHistoryAndRunFields(t *testing.T) {
	app := state.App{
		Messages:        turns(2),
		LastRetryNotice: "retry",
		TerminalReason:  agent.TerminalAborted,
		TerminalDetail:  "x",
		ActiveTools:     map[string]state.ToolUse{"t": {ID: "t"}},
		InputBuffer:     "keep me",
		ActiveModel:     "keep-model",
	}
	got := ResetConversation(app)
	if len(got.Messages) != 0 || got.Messages == nil {
		t.Errorf("messages = %#v, want empty non-nil", got.Messages)
	}
	if got.LastRetryNotice != "" || got.TerminalReason != "" || got.TerminalDetail != "" || len(got.ActiveTools) != 0 || got.ActiveTools == nil {
		t.Errorf("run fields not reset: %+v", got)
	}
	if got.InputBuffer != "keep me" || got.ActiveModel != "keep-model" {
		t.Errorf("unrelated fields changed: %+v", got)
	}
}

func TestCompactConversationSkipsBelowMinTurns(t *testing.T) {
	h := &fakeHooks{}
	msgs := turns(2)
	out := CompactConversation(context.Background(), CompactDeps{Hooks: h}, msgs)
	if !out.Skipped || out.Denied || len(out.Messages) != len(msgs) {
		t.Fatalf("outcome = %+v", out)
	}
	if out.Before != 4 || out.After != 4 {
		t.Errorf("before/after = %d/%d", out.Before, out.After)
	}
}

func TestCompactConversationEmergencyTruncateWithoutClient(t *testing.T) {
	msgs := turns(10)
	out := CompactConversation(context.Background(), CompactDeps{}, msgs)
	if out.Skipped || out.Denied || out.Err != nil {
		t.Fatalf("outcome = %+v", out)
	}
	min := agent.DefaultCompactionConfig().MinTurns
	if want := min * 2; len(out.Messages) != want {
		t.Fatalf("kept %d messages, want %d", len(out.Messages), want)
	}
	if out.Before != 20 || out.After != len(out.Messages) {
		t.Errorf("before/after = %d/%d", out.Before, out.After)
	}
	if out.Messages[len(out.Messages)-1].Content != "a9" {
		t.Error("most recent turn must be kept")
	}
	if len(msgs) != 20 {
		t.Error("input slice must not shrink")
	}
}

type summaryClient struct {
	llm.Client
	text string
	err  error
}

func (c summaryClient) Chat(_ context.Context, _ *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	if c.err != nil {
		return nil, c.err
	}
	ch := make(chan llm.StreamEvent, 2)
	ch <- llm.StreamEvent{Message: llm.Message{Role: llm.RoleAssistant, Content: c.text}}
	ch <- llm.StreamEvent{Done: true}
	close(ch)
	return ch, nil
}

func TestCompactConversationSummarizesWhenClientPresent(t *testing.T) {
	out := CompactConversation(context.Background(), CompactDeps{Client: summaryClient{text: "the summary"}, Model: "m"}, turns(10))
	if out.Err != nil || out.Skipped {
		t.Fatalf("outcome = %+v", out)
	}
	if out.Result.Layer != 1 || out.Result.Summary != "the summary" {
		t.Fatalf("result = %+v", out.Result)
	}
	if out.Messages[0].Role != llm.RoleSystem {
		t.Error("summary message should lead the history")
	}
}

func TestCompactConversationFallsBackWhenSummaryFails(t *testing.T) {
	out := CompactConversation(context.Background(), CompactDeps{Client: summaryClient{err: errors.New("boom")}, Model: "m"}, turns(10))
	if out.Result.Layer != 4 || len(out.Messages) == 0 || out.Messages[len(out.Messages)-1].Content != "a9" {
		t.Fatalf("outcome = %+v", out)
	}
}

func TestCompactConversationDispatchesPreAndPostHooks(t *testing.T) {
	h := &fakeHooks{}
	deps := CompactDeps{Hooks: h, SessionID: "s1", WorkingDir: "/w", Trigger: "manual"}
	out := CompactConversation(context.Background(), deps, turns(10))
	if out.Skipped || out.Denied {
		t.Fatalf("outcome = %+v", out)
	}
	got := h.events()
	if len(got) != 2 || got[0] != hooks.EventPreCompact || got[1] != hooks.EventPostCompact {
		t.Fatalf("events = %v", got)
	}
	pre, post := h.envs[0], h.envs[1]
	if pre.SessionID != "s1" || pre.WorkingDir != "/w" || pre.Metadata["trigger"] != "manual" || pre.Metadata["messages_before"] != "20" {
		t.Errorf("pre envelope = %+v", pre)
	}
	if post.Metadata["messages_before"] != "20" || post.Metadata["messages_after"] != fmt.Sprint(out.After) {
		t.Errorf("post envelope = %+v", post)
	}
	if pre.Timestamp.IsZero() {
		t.Error("timestamp not set")
	}
}

func TestCompactConversationPreCompactDenySkipsCompaction(t *testing.T) {
	h := &fakeHooks{preDeny: true, preReason: "not now"}
	msgs := turns(10)
	out := CompactConversation(context.Background(), CompactDeps{Hooks: h}, msgs)
	if !out.Denied || out.DenyReason != "not now" {
		t.Fatalf("outcome = %+v", out)
	}
	if len(out.Messages) != len(msgs) || out.Before != out.After {
		t.Fatalf("history must be untouched on deny: %+v", out)
	}
	if got := h.events(); len(got) != 1 || got[0] != hooks.EventPreCompact {
		t.Fatalf("PostCompact must not fire after deny: %v", got)
	}
}

func TestCompactConversationBelowMinTurnsFiresNoHooks(t *testing.T) {
	h := &fakeHooks{}
	CompactConversation(context.Background(), CompactDeps{Hooks: h}, turns(1))
	if len(h.events()) != 0 {
		t.Fatalf("hooks fired for a no-op compaction: %v", h.events())
	}
}

func TestApplyCompactionStoresMessagesOnlyWhenChanged(t *testing.T) {
	app := state.App{Messages: turns(10)}
	out := CompactConversation(context.Background(), CompactDeps{}, app.Messages)
	got := ApplyCompaction(app, out)
	if len(got.Messages) != out.After {
		t.Fatalf("messages = %d want %d", len(got.Messages), out.After)
	}
	denied := CompactOutcome{Denied: true, Messages: nil}
	if kept := ApplyCompaction(app, denied); len(kept.Messages) != len(app.Messages) {
		t.Fatal("denied compaction must not touch history")
	}
	skipped := CompactOutcome{Skipped: true}
	if kept := ApplyCompaction(app, skipped); len(kept.Messages) != len(app.Messages) {
		t.Fatal("skipped compaction must not touch history")
	}
}
