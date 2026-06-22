package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/FernasFragas/nandocodego/internal/permissions"
)

func TestTUIBubbleEscalationForwardsPrompt(t *testing.T) {
	t.Parallel()
	escalation := TUIBubbleEscalation{
		PromptFunc: func(ctx context.Context, prompt permissions.Prompt) (permissions.Decision, string, error) {
			if prompt.ToolName != "Bash" {
				t.Fatalf("unexpected prompt: %#v", prompt)
			}
			return permissions.DecisionAllow, "ok", nil
		},
		Timeout: 2 * time.Second,
	}
	decision, reason, err := escalation.Ask(context.Background(), permissions.Prompt{ToolName: "Bash"})
	if err != nil {
		t.Fatal(err)
	}
	if decision != permissions.DecisionAllow || reason != "ok" {
		t.Fatalf("unexpected result: %v %q", decision, reason)
	}
}

func TestTUIBubbleEscalationTimeoutReturnsDeny(t *testing.T) {
	t.Parallel()
	escalation := TUIBubbleEscalation{
		PromptFunc: func(ctx context.Context, prompt permissions.Prompt) (permissions.Decision, string, error) {
			<-ctx.Done()
			return permissions.DecisionAllow, "late", nil
		},
		Timeout: 20 * time.Millisecond,
	}
	decision, reason, err := escalation.Ask(context.Background(), permissions.Prompt{ToolName: "Bash"})
	if err != nil {
		t.Fatal(err)
	}
	if decision != permissions.DecisionDeny {
		t.Fatalf("decision = %v, want deny", decision)
	}
	if !strings.Contains(reason, "timeout") {
		t.Fatalf("reason = %q, want timeout", reason)
	}
}
