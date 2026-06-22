package agent

import (
	"context"
	"time"

	"github.com/FernasFragas/nandocodego/internal/llm"
)

// fakeClient implements llm.Client for testing.
type fakeClient struct {
	turns      []fakeTurn
	currentIdx int
	requests   []*llm.ChatRequest
}

// fakeTurn defines one model turn response.
type fakeTurn struct {
	events []llm.StreamEvent
	err    error
	wait   time.Duration
}

func newFakeClient(turns []fakeTurn) *fakeClient {
	return &fakeClient{turns: turns}
}

func (f *fakeClient) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	// Record request
	f.requests = append(f.requests, req)

	// Check if we have more turns
	if f.currentIdx >= len(f.turns) {
		return nil, context.Canceled
	}

	turn := f.turns[f.currentIdx]
	f.currentIdx++

	// Return error if specified
	if turn.err != nil {
		return nil, turn.err
	}

	// Create event channel
	events := make(chan llm.StreamEvent, len(turn.events))

	// Stream events asynchronously
	go func() {
		defer close(events)
		for _, evt := range turn.events {
			if turn.wait > 0 {
				select {
				case <-time.After(turn.wait):
				case <-ctx.Done():
					return
				}
			}
			select {
			case events <- evt:
			case <-ctx.Done():
				return
			}
		}
	}()

	return events, nil
}

func (f *fakeClient) Embed(ctx context.Context, model string, input []string) ([][]float32, error) {
	return nil, nil
}

func (f *fakeClient) ListModels(ctx context.Context) ([]llm.ModelInfo, error) {
	return nil, nil
}

func (f *fakeClient) ShowModel(ctx context.Context, name string) (llm.ModelDetails, error) {
	return llm.ModelDetails{}, nil
}

func (f *fakeClient) PullModel(ctx context.Context, name string, progress chan<- llm.PullProgress) error {
	return nil
}

// Helper functions to build fake turns

func textTurn(content string) fakeTurn {
	return fakeTurn{
		events: []llm.StreamEvent{
			{Message: llm.Message{Content: content}, Done: false},
			{Done: true, DoneReason: "stop"},
		},
	}
}

func thinkingTurn(thinking, content string) fakeTurn {
	return fakeTurn{
		events: []llm.StreamEvent{
			{Message: llm.Message{Thinking: thinking}, Done: false},
			{Message: llm.Message{Content: content}, Done: false},
			{Done: true, DoneReason: "stop"},
		},
	}
}

func toolCallTurn(toolName string, args map[string]any) fakeTurn {
	return fakeTurn{
		events: []llm.StreamEvent{
			{
				Message: llm.Message{
					ToolCalls: []llm.ToolCall{
						{
							Function: struct {
								Name      string         `json:"name"`
								Arguments map[string]any `json:"arguments"`
							}{
								Name:      toolName,
								Arguments: args,
							},
						},
					},
				},
				Done: false,
			},
			{Done: true, DoneReason: "stop"},
		},
	}
}

func lengthTurn() fakeTurn {
	return fakeTurn{
		events: []llm.StreamEvent{
			{Message: llm.Message{Content: "partial"}, Done: false},
			{Done: true, DoneReason: "length"},
		},
	}
}
