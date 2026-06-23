package observability

import (
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/permissions"
)

func TestMeterRecordAgentRun(t *testing.T) {
	m := NewMeter()
	m.RecordAgentRun(agent.Usage{Turns: 2, PromptEvalCount: 10, EvalCount: 5, DoneReason: "stop"}, 100*time.Millisecond, agent.TerminalCompleted)
	s := m.Snapshot()
	if s.AgentRuns != 1 {
		t.Fatalf("agent runs=%d", s.AgentRuns)
	}
	if s.TotalTokens != 15 {
		t.Fatalf("total tokens=%d", s.TotalTokens)
	}
	if s.LastDoneReason != "stop" {
		t.Fatalf("last done reason=%q", s.LastDoneReason)
	}
}

func TestMeterRecordLLMChatRecordsTokensAndAveragesFirstLatency(t *testing.T) {
	m := NewMeter()
	m.RecordLLMChat(10*time.Millisecond, 20*time.Millisecond, 3, 4, "stop", nil)
	m.RecordLLMChat(30*time.Millisecond, 40*time.Millisecond, 5, 6, "stop", nil)

	s := m.Snapshot()
	if s.PromptTokens != 8 || s.CompletionTokens != 10 || s.TotalTokens != 18 {
		t.Fatalf("tokens prompt=%d completion=%d total=%d", s.PromptTokens, s.CompletionTokens, s.TotalTokens)
	}
	if s.LLMFirstTokenLatency != 20*time.Millisecond {
		t.Fatalf("first token latency average=%s", s.LLMFirstTokenLatency)
	}
	if s.LastDoneReason != "stop" {
		t.Fatalf("last done reason=%q", s.LastDoneReason)
	}
}

func TestMeterRecordAgentRetry(t *testing.T) {
	m := NewMeter()
	m.RecordAgentRetry(agent.RetryNotice{
		Kind:       "incomplete_assistant_response",
		Cause:      "assistant response looked incomplete",
		DoneReason: "stop",
	})

	s := m.Snapshot()
	if s.RetryCount != 1 {
		t.Fatalf("retry count=%d", s.RetryCount)
	}
	if s.RetryCountByKind["incomplete_assistant_response"] != 1 {
		t.Fatalf("retry count by kind=%v", s.RetryCountByKind)
	}
	if s.LastRetryKind != "incomplete_assistant_response" || s.LastRetryDoneReason != "stop" {
		t.Fatalf("last retry kind=%q done=%q", s.LastRetryKind, s.LastRetryDoneReason)
	}
}

func TestMeterPermissionDecision(t *testing.T) {
	m := NewMeter()
	m.RecordPermissionDecision(permissions.ModeDefault, permissions.StageMode, "Bash", permissions.DecisionAsk)
	s := m.Snapshot()
	if len(s.PermissionDecisions) != 1 {
		t.Fatalf("permission decisions=%d", len(s.PermissionDecisions))
	}
}

func TestMeterRecordRunTraceCopiesRetryKinds(t *testing.T) {
	m := NewMeter()
	trace := RunTrace{
		RunStartedAt:   time.Now().UTC(),
		TerminalReason: "completed",
		RetryKinds: map[string]int64{
			"incomplete_assistant_response": 1,
		},
	}
	m.RecordRunTrace(trace)
	trace.RetryKinds["incomplete_assistant_response"] = 99
	s := m.Snapshot()
	if s.LastRunTrace.RetryKinds["incomplete_assistant_response"] != 1 {
		t.Fatalf("retry kinds=%v", s.LastRunTrace.RetryKinds)
	}
}

func TestMeterPendingRunStagesConsumeAndReset(t *testing.T) {
	m := NewMeter()
	m.NotePendingRunStage("mention_expand", 12*time.Millisecond)
	got := m.ConsumePendingRunStages()
	if got["mention_expand"] != 12*time.Millisecond {
		t.Fatalf("pending stages=%v", got)
	}
	if again := m.ConsumePendingRunStages(); len(again) != 0 {
		t.Fatalf("expected pending stages to reset, got=%v", again)
	}
}
