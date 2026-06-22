package llm

import (
	"context"
	"sync"
)

// RuntimeSnapshot reports runtime client routing metadata.
type RuntimeSnapshot struct {
	Provider Provider
	BaseURL  string
}

// RuntimeClient is a switchable llm.Client router.
type RuntimeClient struct {
	mu       sync.RWMutex
	current  Client
	provider Provider
	baseURL  string
}

// NewRuntimeClient constructs a runtime client router.
func NewRuntimeClient(initial Client, provider Provider, baseURL string) *RuntimeClient {
	return &RuntimeClient{
		current:  initial,
		provider: provider,
		baseURL:  baseURL,
	}
}

// Switch atomically replaces the target client and metadata.
func (r *RuntimeClient) Switch(next Client, provider Provider, baseURL string) {
	r.mu.Lock()
	r.current = next
	r.provider = provider
	r.baseURL = baseURL
	r.mu.Unlock()
}

// Snapshot returns current provider/base URL metadata.
func (r *RuntimeClient) Snapshot() RuntimeSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return RuntimeSnapshot{
		Provider: r.provider,
		BaseURL:  r.baseURL,
	}
}

func (r *RuntimeClient) currentClient() Client {
	r.mu.RLock()
	c := r.current
	r.mu.RUnlock()
	return c
}

// Chat delegates to the currently selected client.
func (r *RuntimeClient) Chat(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	return r.currentClient().Chat(ctx, req)
}

// Embed delegates to the currently selected client.
func (r *RuntimeClient) Embed(ctx context.Context, model string, input []string) ([][]float32, error) {
	return r.currentClient().Embed(ctx, model, input)
}

// EmbedWithOptions delegates to the currently selected client when supported.
// It falls back to Embed to preserve compatibility with clients that only implement Embed.
func (r *RuntimeClient) EmbedWithOptions(ctx context.Context, model string, input []string, opts *EmbedOptions) ([][]float32, error) {
	client := r.currentClient()
	if withOpts, ok := client.(EmbedderWithOptions); ok {
		return withOpts.EmbedWithOptions(ctx, model, input, opts)
	}
	return client.Embed(ctx, model, input)
}

// ListModels delegates to the currently selected client.
func (r *RuntimeClient) ListModels(ctx context.Context) ([]ModelInfo, error) {
	return r.currentClient().ListModels(ctx)
}

// ShowModel delegates to the currently selected client.
func (r *RuntimeClient) ShowModel(ctx context.Context, name string) (ModelDetails, error) {
	return r.currentClient().ShowModel(ctx, name)
}

// PullModel delegates to the currently selected client.
func (r *RuntimeClient) PullModel(ctx context.Context, name string, progress chan<- PullProgress) error {
	return r.currentClient().PullModel(ctx, name, progress)
}
