package observability

import (
	"context"
	"testing"
	"time"

	"github.com/FernasFragas/nandocodego/internal/agent"
)

type fakeRunner struct {
	events []agent.Event
}

func (f *fakeRunner) Run(context.Context, agent.Input) <-chan agent.Event {
	ch := make(chan agent.Event, len(f.events))
	for _, e := range f.events {
		ch <- e
	}
	close(ch)
	return ch
}

type recordingTokenRunner struct {
	meter *Meter
}

func (r recordingTokenRunner) Run(context.Context, agent.Input) <-chan agent.Event {
	ch := make(chan agent.Event, 1)
	r.meter.RecordLLMChat(time.Millisecond, 2*time.Millisecond, 4, 6, "stop", nil)
	ch <- agent.Terminal{Reason: agent.TerminalCompleted, Usage: agent.Usage{Turns: 1, PromptEvalCount: 4, EvalCount: 6}}
	close(ch)
	return ch
}

func TestRunnerDecoratorRecordsTerminalUsage(t *testing.T) {
	m := NewMeter()
	r := WrapRunner(&fakeRunner{
		events: []agent.Event{
			agent.AssistantTextDelta{Content: "x"},
			agent.Terminal{Reason: agent.TerminalCompleted, Usage: agent.Usage{Turns: 1, PromptEvalCount: 4, EvalCount: 6}},
		},
	}, m, nil)
	start := time.Now()
	for range r.Run(context.Background(), agent.Input{Model: "test"}) {
	}
	_ = start
	s := m.Snapshot()
	if s.AgentRuns != 1 {
		t.Fatalf("agent runs=%d", s.AgentRuns)
	}
	if s.TotalTokens != 10 {
		t.Fatalf("total tokens=%d", s.TotalTokens)
	}
}

func TestRunnerDecoratorDoesNotDoubleCountObservedLLMTokens(t *testing.T) {
	m := NewMeter()
	r := WrapRunner(recordingTokenRunner{meter: m}, m, nil)
	for range r.Run(context.Background(), agent.Input{Model: "test"}) {
	}
	s := m.Snapshot()
	if s.AgentRuns != 1 {
		t.Fatalf("agent runs=%d", s.AgentRuns)
	}
	if s.TotalTokens != 10 {
		t.Fatalf("total tokens=%d", s.TotalTokens)
	}
}

func TestRunnerDecoratorRecordsRetryNotices(t *testing.T) {
	m := NewMeter()
	r := WrapRunner(&fakeRunner{
		events: []agent.Event{
			agent.AssistantThinkingDelta{Thinking: "plan"},
			agent.ToolUseStart{ID: "tool-1", Name: "Read"},
			agent.StageTiming{Stage: "hook_user_prompt_submit", Duration: 7 * time.Millisecond},
			agent.CompactionStarted{TurnCount: 8, ContextTokens: 10000},
			agent.CompactionCompleted{},
			agent.RetryNotice{Kind: "incomplete_assistant_response", Cause: "incomplete", DoneReason: "stop"},
			agent.Terminal{Reason: agent.TerminalCompleted, Usage: agent.Usage{Turns: 1, DoneReason: "stop"}},
		},
	}, m, nil)

	for range r.Run(context.Background(), agent.Input{
		Model:        "test",
		ContextMode:  "large",
		ToolMode:     agent.ToolModeNone,
		RouteAction:  "skip_all_retrieval",
		RouteReason:  "skip_general_prompt",
		RouteProfile: "general_prompt",
	}) {
	}

	s := m.Snapshot()
	if s.RetryCount != 1 {
		t.Fatalf("retry count=%d", s.RetryCount)
	}
	if s.LastRetryKind != "incomplete_assistant_response" {
		t.Fatalf("last retry kind=%q", s.LastRetryKind)
	}
	if s.LastRunTrace.RunStartedAt.IsZero() {
		t.Fatal("expected last run trace to be recorded")
	}
	if s.LastRunTrace.TerminalReason != string(agent.TerminalCompleted) {
		t.Fatalf("terminal reason=%q", s.LastRunTrace.TerminalReason)
	}
	if s.LastRunTrace.RetryKinds["incomplete_assistant_response"] != 1 {
		t.Fatalf("retry kinds=%v", s.LastRunTrace.RetryKinds)
	}
	if s.LastRunTrace.FirstThinkingLatency <= 0 {
		t.Fatalf("first thinking latency=%s", s.LastRunTrace.FirstThinkingLatency)
	}
	if s.LastRunTrace.FirstToolStartLatency <= 0 {
		t.Fatalf("first tool start latency=%s", s.LastRunTrace.FirstToolStartLatency)
	}
	if s.LastRunTrace.CompactionStartLatency <= 0 || s.LastRunTrace.CompactionEndLatency <= 0 {
		t.Fatalf("compaction latencies start=%s end=%s", s.LastRunTrace.CompactionStartLatency, s.LastRunTrace.CompactionEndLatency)
	}
	if s.LastRunTrace.StageLatencies["hook_user_prompt_submit"] != 7*time.Millisecond {
		t.Fatalf("stage latencies=%v", s.LastRunTrace.StageLatencies)
	}
	if s.LastRunTrace.ContextMode != "large" {
		t.Fatalf("context mode=%q", s.LastRunTrace.ContextMode)
	}
	if s.LastRunTrace.ToolMode != agent.ToolModeNone {
		t.Fatalf("tool mode=%q", s.LastRunTrace.ToolMode)
	}
	if s.LastRunTrace.RouteAction != "skip_all_retrieval" || s.LastRunTrace.RouteReason != "skip_general_prompt" || s.LastRunTrace.RouteProfile != "general_prompt" {
		t.Fatalf("route metadata=%+v", s.LastRunTrace)
	}
}

func TestRunnerDecoratorCarriesPendingExpansionIntoRunTrace(t *testing.T) {
	m := NewMeter()
	m.NotePendingRunExpansion("tree", 1, 52, 0, true)
	r := WrapRunner(&fakeRunner{
		events: []agent.Event{
			agent.Terminal{Reason: agent.TerminalCompleted, Usage: agent.Usage{Turns: 1, DoneReason: "stop"}},
		},
	}, m, nil)
	for range r.Run(context.Background(), agent.Input{Model: "test"}) {
	}
	s := m.Snapshot()
	if s.LastRunTrace.MentionMode != "tree" {
		t.Fatalf("mention mode=%q", s.LastRunTrace.MentionMode)
	}
	if s.LastRunTrace.MentionDirs != 1 || s.LastRunTrace.MentionFilesDiscovered != 52 {
		t.Fatalf("mention counts dirs=%d files=%d", s.LastRunTrace.MentionDirs, s.LastRunTrace.MentionFilesDiscovered)
	}
	if !s.LastRunTrace.MentionListingIntent {
		t.Fatal("expected mention listing intent")
	}
}
