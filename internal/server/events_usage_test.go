package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/bootstrap"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/state"
	"github.com/FernasFragas/Nandocode/internal/tools"
)

type usageTurnClient struct {
	turns [][]llm.StreamEvent
	next  int
}

func (c *usageTurnClient) Chat(_ context.Context, _ *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent, 4)
	for _, event := range c.turns[c.next] {
		ch <- event
	}
	c.next++
	close(ch)
	return ch, nil
}

func (*usageTurnClient) Embed(context.Context, string, []string) ([][]float32, error) {
	return nil, nil
}
func (*usageTurnClient) ListModels(context.Context) ([]llm.ModelInfo, error) { return nil, nil }
func (*usageTurnClient) ShowModel(context.Context, string) (llm.ModelDetails, error) {
	return llm.ModelDetails{}, nil
}
func (*usageTurnClient) PullModel(context.Context, string, chan<- llm.PullProgress) error { return nil }

func testTurnSession(t *testing.T, client llm.Client, registry *tools.Registry) *Session {
	t.Helper()
	root := t.TempDir()
	t.Setenv("NANDOCODEGO_DATA_HOME", filepath.Join(root, "data"))
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	app := state.DefaultApp(bootstrap.New(bootstrap.DefaultInitial(project)).Snapshot())
	ag, err := agent.New(client, registry)
	if err != nil {
		t.Fatal(err)
	}
	s := newSession(context.Background(), "usage-test", app, ag)
	t.Cleanup(s.stop)
	return s
}

func TestServerRunEmitsUsageUpdatePerTurn(t *testing.T) {
	call := llm.ToolCall{}
	call.Function.Name = "MissingTool"
	call.Function.Arguments = map[string]any{}
	client := &usageTurnClient{turns: [][]llm.StreamEvent{
		{{Message: llm.Message{ToolCalls: []llm.ToolCall{call}}}, {Done: true, DoneReason: "stop", PromptEvalCount: 11, EvalCount: 3, TotalDuration: 100}},
		{{Message: llm.Message{Content: "done"}}, {Done: true, DoneReason: "stop", PromptEvalCount: 13, EvalCount: 5, TotalDuration: 200}},
	}}
	s := testTurnSession(t, client, tools.NewRegistry())
	runAndWait(t, s, "do work")
	var usages []agent.Usage
	terminal := -1
	for i, event := range s.Replay("") {
		if event.Type == "terminal" {
			terminal = i
		}
		if event.Type == "usage_update" {
			usage, ok := event.Data["usage"].(agent.Usage)
			if !ok {
				t.Fatalf("usage payload = %#v", event.Data)
			}
			usages = append(usages, usage)
			if terminal >= 0 {
				t.Fatal("usage_update after terminal")
			}
		}
	}
	if len(usages) != 2 {
		t.Fatalf("got %d usage updates, want 2: %+v", len(usages), usages)
	}
	if usages[0].Turns != 1 || usages[0].PromptEvalCount != 11 || usages[0].EvalCount != 3 || usages[0].ToolCalls != 1 || usages[0].TotalDuration != 100 {
		t.Fatalf("first usage = %+v", usages[0])
	}
	if usages[1].Turns != 2 || usages[1].PromptEvalCount != 24 || usages[1].EvalCount != 8 || usages[1].ToolCalls != 1 || usages[1].TotalDuration != 300 {
		t.Fatalf("second usage = %+v", usages[1])
	}
}
