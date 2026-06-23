package observability

import (
	"context"
	"time"

	"github.com/FernasFragas/Nandocode/internal/llm"
)

type observedLLMClient struct {
	next   llm.Client
	meter  *Meter
	bridge Bridge
}

// WrapLLMClient decorates an llm.Client with in-memory metrics and bridge forwarding.
func WrapLLMClient(next llm.Client, meter *Meter, bridge Bridge) llm.Client {
	if next == nil {
		return nil
	}
	if bridge == nil {
		bridge = noopBridge{}
	}
	return &observedLLMClient{next: next, meter: meter, bridge: bridge}
}

func (o *observedLLMClient) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	start := time.Now()
	stream, err := o.next.Chat(ctx, req)
	if err != nil {
		o.meter.RecordLLMCallError()
		o.bridge.RecordLLMChat(0, time.Since(start), 0, 0, "", err)
		return nil, err
	}

	out := make(chan llm.StreamEvent)
	go func() {
		defer close(out)
		var (
			firstTokenSeen   bool
			firstLatency     time.Duration
			promptTokens     int64
			completionTokens int64
			doneReason       string
		)
		for evt := range stream {
			if !firstTokenSeen && hasFirstTokenSignal(evt) {
				firstTokenSeen = true
				firstLatency = time.Since(start)
			}
			if evt.Done {
				promptTokens = evt.PromptEvalCount
				completionTokens = evt.EvalCount
				doneReason = evt.DoneReason
			}
			select {
			case out <- evt:
			case <-ctx.Done():
				return
			}
		}
		dur := time.Since(start)
		o.meter.RecordLLMChat(firstLatency, dur, promptTokens, completionTokens, doneReason, nil)
		o.bridge.RecordLLMChat(firstLatency, dur, promptTokens, completionTokens, doneReason, nil)
	}()

	return out, nil
}

func (o *observedLLMClient) Embed(ctx context.Context, model string, input []string) ([][]float32, error) {
	return o.EmbedWithOptions(ctx, model, input, nil)
}

func (o *observedLLMClient) EmbedWithOptions(ctx context.Context, model string, input []string, opts *llm.EmbedOptions) ([][]float32, error) {
	start := time.Now()
	var (
		out [][]float32
		err error
	)
	if withOpts, ok := o.next.(llm.EmbedderWithOptions); ok {
		out, err = withOpts.EmbedWithOptions(ctx, model, input, opts)
	} else {
		out, err = o.next.Embed(ctx, model, input)
	}
	o.meter.RecordLLMChat(0, time.Since(start), 0, 0, "", err)
	// Mark embed spans separately for bridge consumers without widening Meter schema.
	o.bridge.RecordLLMChat(0, time.Since(start), 0, 0, "embed", err)
	return out, err
}

func (o *observedLLMClient) ListModels(ctx context.Context) ([]llm.ModelInfo, error) {
	start := time.Now()
	out, err := o.next.ListModels(ctx)
	o.meter.RecordLLMChat(0, time.Since(start), 0, 0, "", err)
	o.bridge.RecordLLMChat(0, time.Since(start), 0, 0, "", err)
	return out, err
}

func (o *observedLLMClient) ShowModel(ctx context.Context, name string) (llm.ModelDetails, error) {
	return o.next.ShowModel(ctx, name)
}

func (o *observedLLMClient) PullModel(ctx context.Context, name string, progress chan<- llm.PullProgress) error {
	start := time.Now()
	err := o.next.PullModel(ctx, name, progress)
	o.meter.RecordLLMChat(0, time.Since(start), 0, 0, "", err)
	o.bridge.RecordLLMChat(0, time.Since(start), 0, 0, "", err)
	return err
}

func hasFirstTokenSignal(evt llm.StreamEvent) bool {
	if evt.Message.Content != "" || evt.Message.Thinking != "" {
		return true
	}
	if len(evt.Message.ToolCalls) > 0 {
		return true
	}
	return evt.PromptEvalCount > 0 || evt.EvalCount > 0
}
