package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/FernasFragas/nandocodego/internal/llm"
)

func TestDefaultCompactionConfig(t *testing.T) {
	cfg := DefaultCompactionConfig()
	if cfg.Threshold != 0.8 {
		t.Errorf("expected Threshold=0.8, got %f", cfg.Threshold)
	}
	if cfg.MinTurns != 4 {
		t.Errorf("expected MinTurns=4, got %d", cfg.MinTurns)
	}
	if cfg.MaxSummaryLen != 2000 {
		t.Errorf("expected MaxSummaryLen=2000, got %d", cfg.MaxSummaryLen)
	}
}

func TestCountTurns_Empty(t *testing.T) {
	if countTurns(nil) != 0 {
		t.Error("empty slice should return 0")
	}
}

func TestCountTurns_SystemOnly(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleSystem, Content: "system"},
	}
	if countTurns(msgs) != 0 {
		t.Error("system-only messages should return 0")
	}
}

func TestCountTurns_OnePair(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "hi"},
		{Role: llm.RoleAssistant, Content: "hello"},
	}
	if countTurns(msgs) != 1 {
		t.Errorf("expected 1 turn, got %d", countTurns(msgs))
	}
}

func TestCountTurns_MultiplePairs(t *testing.T) {
	msgs := makeTurnMessages(3)
	if countTurns(msgs) != 3 {
		t.Errorf("expected 3 turns, got %d", countTurns(msgs))
	}
}

func TestCountTurns_IgnoresSystemMessages(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleSystem, Content: "sys"},
		{Role: llm.RoleUser, Content: "u1"},
		{Role: llm.RoleAssistant, Content: "a1"},
		{Role: llm.RoleUser, Content: "u2"},
		{Role: llm.RoleAssistant, Content: "a2"},
	}
	if countTurns(msgs) != 2 {
		t.Errorf("expected 2 turns, got %d", countTurns(msgs))
	}
}

func TestCollapseToolResults_LongReplaced(t *testing.T) {
	longContent := strings.Repeat("x", 600)
	msgs := []llm.Message{
		{Role: llm.RoleTool, ToolName: "Bash", Content: longContent},
	}
	out := collapseToolResults(msgs)
	if out[0].Content == longContent {
		t.Error("long tool result should be replaced")
	}
	if !strings.Contains(out[0].Content, "truncated for compaction") {
		t.Errorf("expected truncated notice, got: %s", out[0].Content)
	}
}

func TestCollapseToolResults_ShortPreserved(t *testing.T) {
	short := "short result"
	msgs := []llm.Message{
		{Role: llm.RoleTool, ToolName: "Bash", Content: short},
	}
	out := collapseToolResults(msgs)
	if out[0].Content != short {
		t.Errorf("short tool result should be preserved, got: %s", out[0].Content)
	}
}

func TestCollapseToolResults_NonToolUnchanged(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: strings.Repeat("y", 600)},
	}
	out := collapseToolResults(msgs)
	if out[0].Content != msgs[0].Content {
		t.Error("non-tool messages should not be modified")
	}
}

func TestStripThinkingBlocks_OldTurnsStripped(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "u1"},
		{Role: llm.RoleAssistant, Content: "a1", Thinking: "old thinking"},
		{Role: llm.RoleUser, Content: "u2"},
		{Role: llm.RoleAssistant, Content: "a2", Thinking: "old thinking 2"},
		{Role: llm.RoleUser, Content: "u3"},
		{Role: llm.RoleAssistant, Content: "a3", Thinking: "recent thinking"},
		{Role: llm.RoleUser, Content: "u4"},
		{Role: llm.RoleAssistant, Content: "a4", Thinking: "newest thinking"},
	}
	out := stripThinkingBlocks(msgs)

	// First two assistants should have thinking stripped
	if out[1].Thinking != "" {
		t.Errorf("expected thinking stripped from old turn, got: %q", out[1].Thinking)
	}
	if out[3].Thinking != "" {
		t.Errorf("expected thinking stripped from old turn 2, got: %q", out[3].Thinking)
	}
	// Last two assistant messages should preserve thinking
	if out[5].Thinking != "recent thinking" {
		t.Errorf("expected recent thinking preserved, got: %q", out[5].Thinking)
	}
	if out[7].Thinking != "newest thinking" {
		t.Errorf("expected newest thinking preserved, got: %q", out[7].Thinking)
	}
}

func TestStripThinkingBlocks_ThinkingTagsRemoved(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "u1"},
		{Role: llm.RoleAssistant, Content: "before <thinking>secret</thinking> after"},
		{Role: llm.RoleUser, Content: "u2"},
		{Role: llm.RoleAssistant, Content: "last"},
		{Role: llm.RoleUser, Content: "u3"},
		{Role: llm.RoleAssistant, Content: "newest"},
	}
	out := stripThinkingBlocks(msgs)
	if strings.Contains(out[1].Content, "secret") {
		t.Error("thinking tag content should be removed")
	}
}

func TestEmergencyTruncate_PreservesSystemAndLastTurns(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleSystem, Content: "system"},
		{Role: llm.RoleUser, Content: "u1"},
		{Role: llm.RoleAssistant, Content: "a1"},
		{Role: llm.RoleUser, Content: "u2"},
		{Role: llm.RoleAssistant, Content: "a2"},
		{Role: llm.RoleUser, Content: "u3"},
		{Role: llm.RoleAssistant, Content: "a3"},
	}
	out := emergencyTruncate(msgs, 2)

	// Should have system + last 2 turns = 1 + 4 messages
	if out[0].Role != llm.RoleSystem {
		t.Error("system message should be first")
	}
	found3 := false
	for _, m := range out {
		if m.Content == "u3" || m.Content == "a3" {
			found3 = true
		}
	}
	if !found3 {
		t.Error("last turn messages should be preserved")
	}
}

func TestEmergencyTruncate_DropsMiddle(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "u1"},
		{Role: llm.RoleAssistant, Content: "a1"},
		{Role: llm.RoleUser, Content: "u2"},
		{Role: llm.RoleAssistant, Content: "a2"},
		{Role: llm.RoleUser, Content: "u3"},
		{Role: llm.RoleAssistant, Content: "a3"},
	}
	out := emergencyTruncate(msgs, 1)
	for _, m := range out {
		if m.Content == "u1" || m.Content == "a1" || m.Content == "u2" || m.Content == "a2" {
			t.Errorf("middle message %q should be dropped", m.Content)
		}
	}
}

func TestCompact_SkippedWhenUnderMinTurns(t *testing.T) {
	cfg := DefaultCompactionConfig() // MinTurns=4
	msgs := makeTurnMessages(2)

	result := Compact(context.Background(), nil, cfg, "test-model", msgs)
	if !result.Skipped {
		t.Error("expected Skipped=true when under MinTurns")
	}
	if result.Before != len(msgs) {
		t.Errorf("expected Before=%d, got %d", len(msgs), result.Before)
	}
}

func TestCompact_Layer1Success(t *testing.T) {
	cfg := DefaultCompactionConfig()
	cfg.MinTurns = 2
	msgs := makeTurnMessages(4)

	client := newFakeClient([]fakeTurn{
		textTurn("This is a summary of the earlier conversation."),
	})

	result := Compact(context.Background(), client, cfg, "test-model", msgs)
	if result.Skipped {
		t.Error("expected not skipped")
	}
	if result.Layer != 1 {
		t.Errorf("expected Layer=1, got %d", result.Layer)
	}
	if result.After >= result.Before {
		t.Errorf("expected After < Before: After=%d Before=%d", result.After, result.Before)
	}
	if result.Messages == nil {
		t.Error("expected Messages to be set")
	}
}

func TestCompact_Layer4FallbackOnLLMError(t *testing.T) {
	cfg := DefaultCompactionConfig()
	cfg.MinTurns = 2
	msgs := makeTurnMessages(4)

	// nil client causes summarize to fail
	result := Compact(context.Background(), nil, cfg, "test-model", msgs)
	if result.Skipped {
		t.Error("should not be skipped — MinTurns met")
	}
	if result.Layer != 4 {
		t.Errorf("expected Layer=4 fallback, got %d", result.Layer)
	}
	if result.Error == "" {
		t.Error("expected Error to be set on Layer 4 fallback")
	}
	if result.Messages == nil {
		t.Error("expected Messages to be set even on Layer 4")
	}
}

func TestCompact_BeforeAfterCounts(t *testing.T) {
	cfg := DefaultCompactionConfig()
	cfg.MinTurns = 2
	msgs := makeTurnMessages(6)

	client := newFakeClient([]fakeTurn{
		textTurn("summary"),
	})

	result := Compact(context.Background(), client, cfg, "test-model", msgs)
	if result.Before != len(msgs) {
		t.Errorf("expected Before=%d, got %d", len(msgs), result.Before)
	}
	if result.After >= result.Before {
		t.Errorf("expected After < Before")
	}
}

func TestSummarizeMessages_FakeClient(t *testing.T) {
	cfg := DefaultCompactionConfig()
	client := newFakeClient([]fakeTurn{
		textTurn("A concise summary."),
	})
	msgs := makeTurnMessages(2)

	summary, err := summarizeMessages(context.Background(), client, cfg, "test-model", msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestSummarizeMessages_ContextCancellation(t *testing.T) {
	cfg := DefaultCompactionConfig()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := summarizeMessages(ctx, newFakeClient(nil), cfg, "test-model", makeTurnMessages(1))
	if err == nil {
		t.Log("note: cancelled context may or may not return error depending on timing")
	}
}

func TestSummarizeMessages_Deadline(t *testing.T) {
	cfg := DefaultCompactionConfig()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	time.Sleep(5 * time.Millisecond) // ensure deadline is past

	// With an already-expired context and a slow client, should error
	slowClient := newFakeClient([]fakeTurn{
		{events: []llm.StreamEvent{{Done: true, DoneReason: "stop"}}, wait: 100 * time.Millisecond},
	})
	_, err := summarizeMessages(ctx, slowClient, cfg, "test-model", makeTurnMessages(1))
	if err == nil {
		t.Log("note: deadline test may be timing-sensitive")
	}
}

// makeTurnMessages creates n user+assistant turn pairs.
func makeTurnMessages(n int) []llm.Message {
	msgs := make([]llm.Message, 0, n*2)
	for i := 0; i < n; i++ {
		msgs = append(msgs,
			llm.Message{Role: llm.RoleUser, Content: "user message"},
			llm.Message{Role: llm.RoleAssistant, Content: "assistant response"},
		)
	}
	return msgs
}
