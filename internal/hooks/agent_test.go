package hooks

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/llm"
)

type fakeAgentHookClient struct{ out string }

func (f *fakeAgentHookClient) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent, 1)
	ch <- llm.StreamEvent{Message: llm.Message{Content: f.out}}
	close(ch)
	return ch, nil
}
func (f *fakeAgentHookClient) Embed(ctx context.Context, model string, input []string) ([][]float32, error) {
	return nil, nil
}
func (f *fakeAgentHookClient) ListModels(ctx context.Context) ([]llm.ModelInfo, error) {
	return nil, nil
}
func (f *fakeAgentHookClient) ShowModel(ctx context.Context, name string) (llm.ModelDetails, error) {
	return llm.ModelDetails{}, nil
}
func (f *fakeAgentHookClient) PullModel(ctx context.Context, name string, progress chan<- llm.PullProgress) error {
	return nil
}

type blockingAgentHookClient struct{}

func (b *blockingAgentHookClient) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	<-ctx.Done()
	return nil, errors.New("context canceled")
}
func (b *blockingAgentHookClient) Embed(ctx context.Context, model string, input []string) ([][]float32, error) {
	return nil, nil
}
func (b *blockingAgentHookClient) ListModels(ctx context.Context) ([]llm.ModelInfo, error) {
	return nil, nil
}
func (b *blockingAgentHookClient) ShowModel(ctx context.Context, name string) (llm.ModelDetails, error) {
	return llm.ModelDetails{}, nil
}
func (b *blockingAgentHookClient) PullModel(ctx context.Context, name string, progress chan<- llm.PullProgress) error {
	return nil
}

func TestRunAgentHookDeny(t *testing.T) {
	t.Parallel()
	res := runAgentHook(context.Background(), &fakeAgentHookClient{out: `{"decision":"deny","reason":"x"}`}, Hook{
		Prompt: "check", TimeoutSec: 1,
	}, Envelope{}, Config{Model: "m", DefaultTimeout: 1})
	if res.Decision != DecisionDeny {
		t.Fatalf("expected deny, got %q", res.Decision)
	}
}

func TestRunAgentHookFailOpen(t *testing.T) {
	t.Parallel()
	res := runAgentHook(context.Background(), &fakeAgentHookClient{out: `not-json`}, Hook{
		Prompt: "check", TimeoutSec: 1,
	}, Envelope{}, Config{Model: "m", DefaultTimeout: 1})
	if res.Decision != DecisionAllow {
		t.Fatalf("expected allow fail-open, got %q", res.Decision)
	}
}

func TestRunAgentHookTimeoutFailOpenWithWarning(t *testing.T) {
	t.Parallel()
	res := runAgentHook(context.Background(), &blockingAgentHookClient{}, Hook{
		Prompt: "check", TimeoutSec: 1,
	}, Envelope{}, Config{Model: "m", DefaultTimeout: 5 * time.Millisecond})
	if res.Decision != DecisionAllow {
		t.Fatalf("expected allow fail-open, got %q", res.Decision)
	}
	if res.Warning == "" {
		t.Fatalf("expected warning on timeout fail-open")
	}
}
