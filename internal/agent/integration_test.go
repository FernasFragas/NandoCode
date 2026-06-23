//go:build integration

package agent

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/llm/ollama"
	"github.com/FernasFragas/Nandocode/internal/tools"
	"github.com/FernasFragas/Nandocode/internal/tools/builtin"
)

func TestAgentIntegrationWithRealOllama(t *testing.T) {
	if os.Getenv("NANDOCODEGO_RUN_OLLAMA_INTEGRATION") != "1" {
		t.Skip("Set NANDOCODEGO_RUN_OLLAMA_INTEGRATION=1 to run")
	}

	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = "qwen3"
	}

	baseURL := os.Getenv("OLLAMA_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	// Create Ollama client
	client := ollama.NewClient(baseURL)

	// Create tool registry
	reg, err := builtin.NewRegistry()
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	// Create agent
	agent, err := New(client, reg)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Run agent with a simple bash command request
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tmpDir := t.TempDir()
	events := agent.Run(ctx, Input{
		Model:        model,
		SystemPrompt: "You are a helpful assistant. You can use the Bash tool to run shell commands.",
		Messages: []llm.Message{
			{
				Role:    llm.RoleUser,
				Content: "Please run the 'ls' command to list the contents of the current directory.",
			},
		},
		ToolContext: tools.Context{
			Context:        ctx,
			WorkingDir:     tmpDir,
			PermissionMode: tools.PermissionBypassPermissions,
		},
	})

	var toolStarted bool
	var toolResult *ToolUseResult
	var terminal *Terminal
	var textDeltas []string

	for evt := range events {
		switch e := evt.(type) {
		case AssistantTextDelta:
			textDeltas = append(textDeltas, e.Content)
			t.Logf("Text: %s", e.Content)
		case AssistantThinkingDelta:
			t.Logf("Thinking: %s", e.Thinking)
		case ToolUseStart:
			toolStarted = true
			t.Logf("Tool start: %s", e.Name)
		case ToolUseProgress:
			t.Logf("Tool progress: %+v", e.Data)
		case ToolUseResult:
			toolResult = &e
			if e.Err != nil {
				t.Logf("Tool error: %v", e.Err)
			} else {
				t.Logf("Tool result: %s", e.Result.Display)
			}
		case RetryNotice:
			t.Logf("Retry: attempt=%d cause=%s", e.Attempt, e.Cause)
		case Terminal:
			terminal = &e
			t.Logf("Terminal: reason=%s detail=%s", e.Reason, e.Detail)
			t.Logf("Usage: turns=%d toolCalls=%d promptEval=%d eval=%d",
				e.Usage.Turns, e.Usage.ToolCalls,
				e.Usage.PromptEvalCount, e.Usage.EvalCount)
		}
	}

	// Assertions
	if !toolStarted {
		t.Error("expected at least one tool to start")
	}

	if toolResult == nil {
		t.Error("expected at least one tool result")
	}

	if terminal == nil {
		t.Fatal("no terminal event")
	}

	if terminal.Reason != TerminalCompleted {
		t.Fatalf("expected completed, got %s: %s", terminal.Reason, terminal.Detail)
	}

	if terminal.Usage.Turns == 0 {
		t.Error("expected at least one turn")
	}

	if terminal.Usage.ToolCalls == 0 {
		t.Error("expected at least one tool call")
	}

	// Check that we got some assistant text
	fullText := strings.Join(textDeltas, "")
	if fullText == "" {
		t.Error("expected some assistant text")
	}

	t.Logf("Integration test completed successfully")
	t.Logf("Final text: %s", fullText)
}
