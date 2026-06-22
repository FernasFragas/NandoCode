package memory

import (
	"context"
	"testing"
	"time"

	"github.com/FernasFragas/nandocodego/internal/agent"
	"github.com/FernasFragas/nandocodego/internal/llm"
	"github.com/FernasFragas/nandocodego/internal/tools"
)

type fakeMemoryRunner struct {
	events []agent.Event
}

func (f fakeMemoryRunner) Run(context.Context, agent.Input) <-chan agent.Event {
	ch := make(chan agent.Event, len(f.events))
	for _, evt := range f.events {
		ch <- evt
	}
	close(ch)
	return ch
}

type blockingExtractClient struct{}

func (blockingExtractClient) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent, 1)
	go func() {
		defer close(ch)
		if req.Format != nil {
			<-ctx.Done()
			return
		}
		ch <- llm.StreamEvent{Message: llm.Message{Content: `{"selected":[]}`}}
		ch <- llm.StreamEvent{Done: true, DoneReason: "stop"}
	}()
	return ch, nil
}

func (blockingExtractClient) Embed(context.Context, string, []string) ([][]float32, error) {
	return nil, nil
}
func (blockingExtractClient) ListModels(context.Context) ([]llm.ModelInfo, error) { return nil, nil }
func (blockingExtractClient) ShowModel(context.Context, string) (llm.ModelDetails, error) {
	return llm.ModelDetails{}, nil
}
func (blockingExtractClient) PullModel(context.Context, string, chan<- llm.PullProgress) error {
	return nil
}

func TestRunnerDoesNotWaitForExtractionAfterTerminal(t *testing.T) {
	cfg := DefaultConfig("test")
	cfg.RecallTimeout = time.Second
	cfg.ExtractTimeout = time.Second
	r := NewRunner(fakeMemoryRunner{events: []agent.Event{
		agent.Terminal{
			Reason: agent.TerminalCompleted,
			Conversation: []llm.Message{
				{Role: llm.RoleUser, Content: "remember this"},
				{Role: llm.RoleAssistant, Content: "ok"},
			},
		},
	}}, blockingExtractClient{}, cfg)

	events := r.Run(context.Background(), agent.Input{
		Model:       "test",
		ToolContext: tools.Context{WorkingDir: t.TempDir()},
	})

	sawTerminal := false
	select {
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for events")
	default:
	}
	for evt := range events {
		if _, ok := evt.(agent.Terminal); ok {
			sawTerminal = true
			break
		}
	}
	if !sawTerminal {
		t.Fatal("expected terminal event")
	}
	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("expected runner channel to close without waiting for extraction")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("runner waited for detached extraction")
	}
}
