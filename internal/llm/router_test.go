package llm

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type stubClient struct {
	name                  string
	chatCalls             atomic.Int64
	embedCalls            atomic.Int64
	embedWithOptionsCalls atomic.Int64
	lastEmbedWithOptions  *EmbedOptions
	ch                    chan StreamEvent
}

func (s *stubClient) Chat(context.Context, *ChatRequest) (<-chan StreamEvent, error) {
	s.chatCalls.Add(1)
	return s.ch, nil
}
func (s *stubClient) Embed(context.Context, string, []string) ([][]float32, error) {
	s.embedCalls.Add(1)
	return [][]float32{{1, 2}}, nil
}
func (s *stubClient) EmbedWithOptions(_ context.Context, _ string, _ []string, opts *EmbedOptions) ([][]float32, error) {
	s.embedWithOptionsCalls.Add(1)
	s.lastEmbedWithOptions = opts
	return [][]float32{{3, 4}}, nil
}
func (s *stubClient) ListModels(context.Context) ([]ModelInfo, error) { return nil, nil }
func (s *stubClient) ShowModel(context.Context, string) (ModelDetails, error) {
	return ModelDetails{}, nil
}
func (s *stubClient) PullModel(context.Context, string, chan<- PullProgress) error { return nil }

func TestRuntimeClientSwitchAndSnapshot(t *testing.T) {
	t.Parallel()
	a := &stubClient{name: "a", ch: make(chan StreamEvent)}
	close(a.ch)
	r := NewRuntimeClient(a, ProviderOllamaLocal, "http://localhost:11434")

	snap := r.Snapshot()
	if snap.Provider != ProviderOllamaLocal {
		t.Fatalf("provider=%s", snap.Provider)
	}

	b := &stubClient{name: "b", ch: make(chan StreamEvent)}
	close(b.ch)
	r.Switch(b, ProviderOllamaCloudAPI, OllamaCloudBaseURL)
	snap = r.Snapshot()
	if snap.Provider != ProviderOllamaCloudAPI || snap.BaseURL != OllamaCloudBaseURL {
		t.Fatalf("snapshot=%+v", snap)
	}

	if _, err := r.Chat(context.Background(), &ChatRequest{}); err != nil {
		t.Fatalf("Chat() error=%v", err)
	}
	if b.chatCalls.Load() != 1 {
		t.Fatalf("b calls=%d", b.chatCalls.Load())
	}
}

func TestRuntimeClientInFlightChatUsesOldClient(t *testing.T) {
	t.Parallel()
	aCh := make(chan StreamEvent, 1)
	aCh <- StreamEvent{Done: true}
	close(aCh)
	a := &stubClient{name: "a", ch: aCh}

	bCh := make(chan StreamEvent, 1)
	bCh <- StreamEvent{Done: true}
	close(bCh)
	b := &stubClient{name: "b", ch: bCh}

	r := NewRuntimeClient(a, ProviderOllamaLocal, "http://localhost:11434")
	stream, err := r.Chat(context.Background(), &ChatRequest{})
	if err != nil {
		t.Fatalf("Chat() error=%v", err)
	}
	r.Switch(b, ProviderOllamaCloudAPI, OllamaCloudBaseURL)

	select {
	case <-stream:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for stream")
	}
	if a.chatCalls.Load() != 1 || b.chatCalls.Load() != 0 {
		t.Fatalf("a=%d b=%d", a.chatCalls.Load(), b.chatCalls.Load())
	}
}

func TestRuntimeClientEmbedWithOptionsDelegatesAndPreservesFields(t *testing.T) {
	t.Parallel()
	a := &stubClient{name: "a", ch: make(chan StreamEvent)}
	close(a.ch)
	r := NewRuntimeClient(a, ProviderOllamaLocal, "http://localhost:11434")

	dims := 1024
	truncate := true
	opts := &EmbedOptions{
		Dimensions: &dims,
		Truncate:   &truncate,
		KeepAlive:  "30m",
	}
	got, err := r.EmbedWithOptions(context.Background(), "embed-model", []string{"x"}, opts)
	if err != nil {
		t.Fatalf("EmbedWithOptions() error=%v", err)
	}
	if len(got) != 1 || len(got[0]) != 2 || got[0][0] != 3 {
		t.Fatalf("EmbedWithOptions() result=%v", got)
	}
	if a.embedWithOptionsCalls.Load() != 1 {
		t.Fatalf("embedWithOptions calls=%d", a.embedWithOptionsCalls.Load())
	}
	if a.lastEmbedWithOptions == nil {
		t.Fatal("lastEmbedWithOptions=nil")
	}
	if a.lastEmbedWithOptions.Dimensions == nil || *a.lastEmbedWithOptions.Dimensions != 1024 {
		t.Fatalf("dimensions=%v", a.lastEmbedWithOptions.Dimensions)
	}
	if a.lastEmbedWithOptions.Truncate == nil || !*a.lastEmbedWithOptions.Truncate {
		t.Fatalf("truncate=%v", a.lastEmbedWithOptions.Truncate)
	}
	if a.lastEmbedWithOptions.KeepAlive != "30m" {
		t.Fatalf("keep_alive=%q", a.lastEmbedWithOptions.KeepAlive)
	}
}

type embedOnlyClient struct {
	chatCalls  atomic.Int64
	embedCalls atomic.Int64
	ch         chan StreamEvent
}

func (e *embedOnlyClient) Chat(context.Context, *ChatRequest) (<-chan StreamEvent, error) {
	e.chatCalls.Add(1)
	return e.ch, nil
}
func (e *embedOnlyClient) Embed(context.Context, string, []string) ([][]float32, error) {
	e.embedCalls.Add(1)
	return [][]float32{{1, 2}}, nil
}
func (e *embedOnlyClient) ListModels(context.Context) ([]ModelInfo, error) { return nil, nil }
func (e *embedOnlyClient) ShowModel(context.Context, string) (ModelDetails, error) {
	return ModelDetails{}, nil
}
func (e *embedOnlyClient) PullModel(context.Context, string, chan<- PullProgress) error { return nil }

func TestRuntimeClientEmbedWithOptionsFallsBackToEmbed(t *testing.T) {
	t.Parallel()
	a := &embedOnlyClient{ch: make(chan StreamEvent)}
	close(a.ch)
	r := NewRuntimeClient(a, ProviderOllamaLocal, "http://localhost:11434")

	got, err := r.EmbedWithOptions(context.Background(), "embed-model", []string{"x"}, &EmbedOptions{})
	if err != nil {
		t.Fatalf("EmbedWithOptions() error=%v", err)
	}
	if len(got) != 1 || len(got[0]) != 2 || got[0][0] != 1 {
		t.Fatalf("EmbedWithOptions() result=%v", got)
	}
	if a.embedCalls.Load() != 1 {
		t.Fatalf("embed calls=%d", a.embedCalls.Load())
	}
}
