package eval

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/FernasFragas/Nandocode/internal/llm"
)

type RecordedClient struct {
	recording Recording
	nextTurn  int
}

func NewRecordedClient(recording Recording) *RecordedClient {
	return &RecordedClient{recording: recording}
}

func (c *RecordedClient) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	if c.nextTurn >= len(c.recording.Turns) {
		return nil, errors.New("recording exhausted")
	}
	if req == nil {
		return nil, errors.New("chat request is nil")
	}
	if req.Model != c.recording.Model {
		return nil, fmt.Errorf("recording model mismatch: got %q want %q", req.Model, c.recording.Model)
	}
	turn := c.recording.Turns[c.nextTurn]
	c.nextTurn++
	if err := validateRecordedRequest(req, turn); err != nil {
		return nil, err
	}
	ch := make(chan llm.StreamEvent, len(turn.Events))
	go func() {
		defer close(ch)
		for _, evt := range turn.Events {
			select {
			case <-ctx.Done():
				return
			case ch <- evt:
			}
		}
	}()
	return ch, nil
}

func validateRecordedRequest(req *llm.ChatRequest, turn RecordedTurn) error {
	if want := strings.TrimSpace(turn.Match.LatestUserContains); want != "" {
		got := latestUserMessage(req.Messages)
		if !strings.Contains(got, want) {
			return fmt.Errorf("recording request mismatch: latest user message does not contain %q", want)
		}
	}
	if len(turn.Match.RequiredTools) > 0 {
		var got []string
		for _, tool := range req.Tools {
			got = append(got, tool.Function.Name)
		}
		for _, want := range turn.Match.RequiredTools {
			if !slices.Contains(got, want) {
				return fmt.Errorf("recording request mismatch: missing required tool %q", want)
			}
		}
	}
	return nil
}

func latestUserMessage(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == llm.RoleUser {
			return messages[i].Content
		}
	}
	return ""
}

func (c *RecordedClient) Embed(context.Context, string, []string) ([][]float32, error) {
	return nil, errors.New("recorded client does not implement embeddings")
}

func (c *RecordedClient) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{{Name: c.recording.Model, ModifiedAt: time.Time{}}}, nil
}

func (c *RecordedClient) ShowModel(context.Context, string) (llm.ModelDetails, error) {
	return llm.ModelDetails{Name: c.recording.Model}, nil
}

func (c *RecordedClient) PullModel(context.Context, string, chan<- llm.PullProgress) error {
	return errors.New("recorded client cannot pull models")
}
