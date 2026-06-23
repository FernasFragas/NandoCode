package observability

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/permissions"
)

type fakeLLM struct {
	chat             func(context.Context, *llm.ChatRequest) (<-chan llm.StreamEvent, error)
	embed            func(context.Context, string, []string) ([][]float32, error)
	embedWithOptions func(context.Context, string, []string, *llm.EmbedOptions) ([][]float32, error)
}

func (f *fakeLLM) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	return f.chat(ctx, req)
}
func (f *fakeLLM) Embed(ctx context.Context, model string, input []string) ([][]float32, error) {
	if f.embed != nil {
		return f.embed(ctx, model, input)
	}
	return nil, nil
}
func (f *fakeLLM) EmbedWithOptions(ctx context.Context, model string, input []string, opts *llm.EmbedOptions) ([][]float32, error) {
	if f.embedWithOptions != nil {
		return f.embedWithOptions(ctx, model, input, opts)
	}
	return nil, nil
}
func (f *fakeLLM) ListModels(context.Context) ([]llm.ModelInfo, error) { return nil, nil }
func (f *fakeLLM) ShowModel(context.Context, string) (llm.ModelDetails, error) {
	return llm.ModelDetails{}, nil
}
func (f *fakeLLM) PullModel(context.Context, string, chan<- llm.PullProgress) error {
	return nil
}

func TestLLMDecoratorFirstTokenLatency(t *testing.T) {
	m := NewMeter()
	c := WrapLLMClient(&fakeLLM{
		chat: func(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
			ch := make(chan llm.StreamEvent, 2)
			go func() {
				defer close(ch)
				time.Sleep(15 * time.Millisecond)
				ch <- llm.StreamEvent{Message: llm.Message{Content: "hello"}}
				ch <- llm.StreamEvent{Done: true, PromptEvalCount: 2, EvalCount: 3}
			}()
			return ch, nil
		},
	}, m, nil)

	stream, err := c.Chat(context.Background(), &llm.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for range stream {
	}
	snap := m.Snapshot()
	if snap.LLMCalls != 1 {
		t.Fatalf("llm calls=%d", snap.LLMCalls)
	}
	if snap.LLMFirstTokenLatency <= 0 {
		t.Fatalf("first token latency=%s", snap.LLMFirstTokenLatency)
	}
	if snap.PromptTokens != 2 || snap.CompletionTokens != 3 || snap.TotalTokens != 5 {
		t.Fatalf("llm decorator token totals: prompt=%d completion=%d total=%d", snap.PromptTokens, snap.CompletionTokens, snap.TotalTokens)
	}
}

type captureBridge struct {
	lastDoneReason string
	lastErr        error
}

func (c *captureBridge) RecordToolCall(string, time.Duration, error) {}
func (c *captureBridge) RecordLLMChat(_ time.Duration, _ time.Duration, _ int64, _ int64, doneReason string, err error) {
	c.lastDoneReason = doneReason
	c.lastErr = err
}
func (c *captureBridge) RecordAgentRun(agent.Usage, time.Duration, agent.TerminalReason) {}
func (c *captureBridge) RecordToolBatch(int, bool, time.Duration)                        {}
func (c *captureBridge) RecordPermissionDecision(permissions.Mode, permissions.Stage, string, permissions.Decision) {
}
func (c *captureBridge) Shutdown(context.Context) error { return nil }

func TestLLMDecoratorEmbedWithOptionsPropagatesFields(t *testing.T) {
	t.Parallel()
	m := NewMeter()
	bridge := &captureBridge{}
	var gotOpts *llm.EmbedOptions
	c := WrapLLMClient(&fakeLLM{
		chat: func(context.Context, *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
			return nil, nil
		},
		embedWithOptions: func(_ context.Context, _ string, _ []string, opts *llm.EmbedOptions) ([][]float32, error) {
			gotOpts = opts
			return [][]float32{{1, 2, 3}}, nil
		},
	}, m, bridge)

	withOpts, ok := c.(llm.EmbedderWithOptions)
	if !ok {
		t.Fatal("wrapped client does not implement EmbedderWithOptions")
	}
	dims := 2048
	truncate := true
	_, err := withOpts.EmbedWithOptions(context.Background(), "embed-model", []string{"x"}, &llm.EmbedOptions{
		Dimensions: &dims,
		Truncate:   &truncate,
		KeepAlive:  "5m",
	})
	if err != nil {
		t.Fatalf("EmbedWithOptions() error=%v", err)
	}
	if gotOpts == nil {
		t.Fatal("gotOpts=nil")
	}
	if gotOpts.Dimensions == nil || *gotOpts.Dimensions != 2048 {
		t.Fatalf("dimensions=%v", gotOpts.Dimensions)
	}
	if gotOpts.Truncate == nil || !*gotOpts.Truncate {
		t.Fatalf("truncate=%v", gotOpts.Truncate)
	}
	if gotOpts.KeepAlive != "5m" {
		t.Fatalf("keep_alive=%q", gotOpts.KeepAlive)
	}

	snap := m.Snapshot()
	if snap.LLMCalls != 1 {
		t.Fatalf("LLMCalls=%d", snap.LLMCalls)
	}
	if bridge.lastDoneReason != "embed" {
		t.Fatalf("bridge doneReason=%q", bridge.lastDoneReason)
	}
}

func TestLLMDecoratorEmbedWithOptionsTracksErrors(t *testing.T) {
	t.Parallel()
	m := NewMeter()
	bridge := &captureBridge{}
	expectedErr := errors.New("embed failed")
	c := WrapLLMClient(&fakeLLM{
		chat: func(context.Context, *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
			return nil, nil
		},
		embedWithOptions: func(_ context.Context, _ string, _ []string, _ *llm.EmbedOptions) ([][]float32, error) {
			return nil, expectedErr
		},
	}, m, bridge)

	withOpts, ok := c.(llm.EmbedderWithOptions)
	if !ok {
		t.Fatal("wrapped client does not implement EmbedderWithOptions")
	}
	if _, err := withOpts.EmbedWithOptions(context.Background(), "embed-model", []string{"x"}, nil); !errors.Is(err, expectedErr) {
		t.Fatalf("EmbedWithOptions() err=%v", err)
	}
	snap := m.Snapshot()
	if snap.LLMErrors != 1 {
		t.Fatalf("LLMErrors=%d", snap.LLMErrors)
	}
	if !errors.Is(bridge.lastErr, expectedErr) {
		t.Fatalf("bridge err=%v", bridge.lastErr)
	}
}
