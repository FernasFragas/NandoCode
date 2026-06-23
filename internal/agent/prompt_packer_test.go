package agent

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/llm"
)

func TestPackPromptHistory_NoTrimWhenWithinBudget(t *testing.T) {
	history := []llm.Message{
		{Role: llm.RoleSystem, Content: "system"},
		{Role: llm.RoleUser, Content: "hello"},
		{Role: llm.RoleAssistant, Content: "world"},
	}
	got := packPromptHistory(history, 10_000)
	if got.Report.SkippedMessages != 0 {
		t.Fatalf("expected no skipped messages, got %d", got.Report.SkippedMessages)
	}
	if len(got.Messages) != len(history) {
		t.Fatalf("expected %d messages, got %d", len(history), len(got.Messages))
	}
}

func TestPackPromptHistory_PrefersNewestAndSystem(t *testing.T) {
	history := []llm.Message{
		{Role: llm.RoleSystem, Content: "system anchor"},
		{Role: llm.RoleUser, Content: "older user"},
		{Role: llm.RoleAssistant, Content: "older assistant"},
		{Role: llm.RoleUser, Content: "latest user input must survive"},
	}
	// Tight budget should force dropping middle messages.
	got := packPromptHistory(history, 30)
	if len(got.Messages) < 2 {
		t.Fatalf("expected at least system+last messages, got %d", len(got.Messages))
	}
	if got.Messages[0].Role != llm.RoleSystem {
		t.Fatalf("expected first packed message to be system, got %s", got.Messages[0].Role)
	}
	last := got.Messages[len(got.Messages)-1]
	if last.Content != "latest user input must survive" {
		t.Fatalf("expected last message preserved, got %q", last.Content)
	}
	if got.Report.SkippedMessages == 0 {
		t.Fatal("expected some skipped messages")
	}
}

func TestPackPromptHistory_ForcedLastMessage(t *testing.T) {
	history := []llm.Message{
		{Role: llm.RoleSystem, Content: "system"},
		{Role: llm.RoleUser, Content: "this is intentionally very long so it cannot fit in tiny budget"},
	}
	got := packPromptHistory(history, 1)
	if len(got.Messages) != 2 {
		t.Fatalf("expected forced inclusion of last message with system, got %d messages", len(got.Messages))
	}
	if !got.Report.ForcedIncludeLast {
		t.Fatal("expected ForcedIncludeLast=true")
	}
}

func TestPackPromptHistory_TinyBudgetReportsMentionBlockDrops(t *testing.T) {
	history := []llm.Message{
		{Role: llm.RoleSystem, Content: "system"},
		{Role: llm.RoleUser, Content: "review @docs\n\n<directory path=\"docs\"><tree>docs/a.txt</tree></directory>\n<file path=\"docs/a.txt\">a</file>"},
		{Role: llm.RoleAssistant, Content: strings.Repeat("A", 4000)},
		{Role: llm.RoleUser, Content: "latest user prompt"},
	}
	got := packPromptHistory(history, 40)
	if !got.Report.LastUserMessageIncluded {
		t.Fatal("expected latest user message to be included")
	}
	if got.Report.DroppedMentionBlocks == 0 {
		t.Fatalf("expected dropped mention blocks to be reported, got %+v", got.Report)
	}
}

func TestPackPromptHistory_Deterministic(t *testing.T) {
	history := make([]llm.Message, 0, 120)
	history = append(history, llm.Message{Role: llm.RoleSystem, Content: "system"})
	for i := 0; i < 119; i++ {
		role := llm.RoleUser
		if i%2 == 1 {
			role = llm.RoleAssistant
		}
		history = append(history, llm.Message{
			Role:    role,
			Content: "msg-" + strconv.Itoa(i) + "-" + strings.Repeat("x", 64),
		})
	}
	first := packPromptHistory(history, 1200)
	second := packPromptHistory(history, 1200)
	if len(first.Messages) != len(second.Messages) {
		t.Fatalf("message count differs: %d vs %d", len(first.Messages), len(second.Messages))
	}
	for i := range first.Messages {
		if first.Messages[i].Role != second.Messages[i].Role || first.Messages[i].Content != second.Messages[i].Content {
			t.Fatalf("non-deterministic output at index %d", i)
		}
	}
	if !reflect.DeepEqual(first.Report, second.Report) {
		t.Fatalf("report differs: %#v vs %#v", first.Report, second.Report)
	}
}

func BenchmarkPackPromptHistory_LargeHistory(b *testing.B) {
	history := make([]llm.Message, 0, 301)
	history = append(history, llm.Message{Role: llm.RoleSystem, Content: "system anchor"})
	for i := 0; i < 300; i++ {
		role := llm.RoleUser
		if i%2 == 1 {
			role = llm.RoleAssistant
		}
		history = append(history, llm.Message{
			Role:    role,
			Content: "message-" + strconv.Itoa(i) + "-" + strings.Repeat("0123456789", 24),
		})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = packPromptHistory(history, 4096)
	}
}
