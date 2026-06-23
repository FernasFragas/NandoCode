package hooks

import (
	"context"
	"errors"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/llm"
)

type fakeHookClient struct {
	content string
	err     error
	req     *llm.ChatRequest
}

func (f *fakeHookClient) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	f.req = req
	if f.err != nil {
		return nil, f.err
	}
	ch := make(chan llm.StreamEvent, 1)
	ch <- llm.StreamEvent{Message: llm.Message{Role: llm.RoleAssistant, Content: f.content}, Done: true}
	close(ch)
	return ch, nil
}

func (f *fakeHookClient) Embed(context.Context, string, []string) ([][]float32, error) {
	return nil, nil
}

func (f *fakeHookClient) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return nil, nil
}

func (f *fakeHookClient) ShowModel(context.Context, string) (llm.ModelDetails, error) {
	return llm.ModelDetails{}, nil
}

func (f *fakeHookClient) PullModel(context.Context, string, chan<- llm.PullProgress) error {
	return nil
}

func TestRunPromptHookParsesDecision(t *testing.T) {
	client := &fakeHookClient{content: `{"decision":"ask","reason":"needs confirmation"}`}
	h := Hook{Kind: KindPrompt, Event: EventPreToolUse, Prompt: "check this"}
	cfg := DefaultConfig()
	cfg.Model = "test-model"

	res := runPromptHook(context.Background(), client, h, Envelope{Event: EventPreToolUse}, cfg)
	if res.Decision != DecisionAsk {
		t.Fatalf("expected ask, got %q", res.Decision)
	}
	if res.Reason != "needs confirmation" {
		t.Fatalf("expected reason, got %q", res.Reason)
	}
	if client.req == nil || client.req.Model != "test-model" || client.req.Stream {
		t.Fatalf("prompt hook did not issue bounded non-stream request: %#v", client.req)
	}
}

func TestRunPromptHookInvalidJSONWarns(t *testing.T) {
	client := &fakeHookClient{content: `not-json`}
	h := Hook{Kind: KindPrompt, Event: EventPreToolUse, Prompt: "check this"}
	res := runPromptHook(context.Background(), client, h, Envelope{Event: EventPreToolUse}, DefaultConfig())
	if res.Warning == "" {
		t.Fatalf("expected invalid JSON warning")
	}
	if res.Decision != DecisionNone {
		t.Fatalf("expected no decision, got %q", res.Decision)
	}
}

func TestRunPromptHookChatErrorWarns(t *testing.T) {
	client := &fakeHookClient{err: errors.New("chat unavailable")}
	h := Hook{Kind: KindPrompt, Event: EventPreToolUse, Prompt: "check this"}
	res := runPromptHook(context.Background(), client, h, Envelope{Event: EventPreToolUse}, DefaultConfig())
	if res.Warning != "prompt hook failed: chat unavailable" {
		t.Fatalf("expected chat warning, got %q", res.Warning)
	}
}

func TestParseHookJSONIgnoresUpdatedInput(t *testing.T) {
	res := parseHookJSON(`{"decision":"allow","updated_input":{"command":"echo changed"}}`)
	if res.Decision != DecisionAllow {
		t.Fatalf("expected allow, got %q", res.Decision)
	}
	if res.Warning != "hook updated_input ignored in Phase 9" {
		t.Fatalf("expected ignored updated_input warning, got %q", res.Warning)
	}
	if res.UpdatedInput != nil {
		t.Fatalf("expected updated_input to be cleared")
	}
}
