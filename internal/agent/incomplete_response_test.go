package agent

import (
	"strings"
	"testing"

	"github.com/FernasFragas/nandocodego/internal/llm"
)

func TestShouldRetryIncompleteAssistantResponse(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "summary promise ending in colon",
			content: "Based on my thorough analysis, here is the comprehensive missing-implementation summary:",
			want:    true,
		},
		{
			name:    "summary promise based on files",
			content: "Based on the files, here is the comprehensive missing-implementation summary:",
			want:    true,
		},
		{
			name:    "let me write promise",
			content: "Now I have the full picture. Let me write the missing-implementation summary:",
			want:    true,
		},
		{
			name:    "report follows below",
			content: "Certainly. The report follows below.",
			want:    true,
		},
		{
			name:    "fix is promise",
			content: "I found the issue. The fix is:",
			want:    true,
		},
		{
			name:    "here are tasks",
			content: "Certainly. Here are the tasks:",
			want:    true,
		},
		{
			name:    "will provide plan",
			content: "I will provide the complete plan now.",
			want:    true,
		},
		{
			name:    "normal short answer",
			content: "Done.",
			want:    false,
		},
		{
			name:    "normal summary sentence",
			content: "Summary: nothing is missing.",
			want:    false,
		},
		{
			name:    "normal analysis complete",
			content: "Analysis complete. No changes needed.",
			want:    false,
		},
		{
			name:    "substantive summary",
			content: "Summary:\n- Agent loop retries incomplete preambles.\n- TUI shows terminal failures.",
			want:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldRetryIncompleteAssistantResponse(llm.Message{Content: tc.content}, "stop")
			if got != tc.want {
				t.Fatalf("shouldRetryIncompleteAssistantResponse() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBuildIncompleteAssistantRetryPrompt(t *testing.T) {
	t.Run("anchored retry prompt", func(t *testing.T) {
		got, ok := buildIncompleteAssistantRetryPrompt(incompleteRetryInput{
			OriginalUserText: "review @docs",
			LastUserContent:  "review @docs",
		})
		if !ok {
			t.Fatal("expected retry prompt")
		}
		if !strings.Contains(got, "Original user request:") {
			t.Fatalf("expected anchored prompt: %q", got)
		}
		if strings.Contains(strings.ToLower(got), "promised answer") {
			t.Fatalf("retry prompt must not mention promised answer: %q", got)
		}
	})

	t.Run("listing prompt preserves tree data", func(t *testing.T) {
		got, ok := buildIncompleteAssistantRetryPrompt(incompleteRetryInput{
			PromptIntent:         "directory_listing",
			OriginalUserText:     "list files in @docs/",
			LastUserContent:      "User request:\nlist files in @docs/\n\nDirectory tree data:\ndocs/\ndocs/a.md",
			LastAssistantContent: "Now I have the full picture:",
		})
		if !ok {
			t.Fatal("expected listing retry prompt")
		}
		if !strings.Contains(got, "Directory tree data:") {
			t.Fatalf("expected tree data in listing retry prompt: %q", got)
		}
		if strings.Contains(strings.ToLower(got), "promised answer") {
			t.Fatalf("retry prompt must not mention promised answer: %q", got)
		}
	})

	t.Run("listing substantive answer disables retry", func(t *testing.T) {
		_, ok := buildIncompleteAssistantRetryPrompt(incompleteRetryInput{
			PromptIntent:         "directory_listing",
			LastUserContent:      "User request:\nlist files\n\nDirectory tree data:\ndocs/\nmanual-tests/",
			LastAssistantContent: "docs/\nmanual-tests/\nmanual-tests/INCOMPLETE-RESPONSE-RECOVERY.md",
		})
		if ok {
			t.Fatal("expected retry to be skipped for substantive listing answer")
		}
	})
}
