package agent

import (
	"context"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/llm"
)

func TestRunForkUsesPromptAndMessages(t *testing.T) {
	t.Parallel()
	client := newFakeClient([]fakeTurn{textTurn("fork-output")})
	parentConv := []llm.Message{{Role: llm.RoleUser, Content: "hello"}}
	parentIn := Input{Model: "test-model"}
	out, err := runFork(context.Background(), parentConv, parentIn, "custom prompt", nil, client, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if out != "fork-output" {
		t.Fatalf("unexpected output: %q", out)
	}
	if len(client.requests) == 0 {
		t.Fatal("expected request")
	}
	msgs := client.requests[0].Messages
	found := false
	for _, m := range msgs {
		if m.Role == llm.RoleUser && m.Content == "hello" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected parent conversation in child messages")
	}
}
