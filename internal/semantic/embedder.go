package semantic

import (
	"context"
	"fmt"
	"strings"

	"github.com/FernasFragas/Nandocode/internal/llm"
)

// LLMEmbedder adapts the shared llm.Client to the semantic Embedder contract.
type LLMEmbedder struct {
	Client llm.Client
}

// Embed performs batch embedding using the llm client.
func (e LLMEmbedder) Embed(ctx context.Context, req EmbedRequest) (EmbedResult, error) {
	if e.Client == nil {
		return EmbedResult{}, fmt.Errorf("semantic embedder: nil llm client")
	}
	if strings.TrimSpace(req.Model) == "" {
		return EmbedResult{}, fmt.Errorf("semantic embedder: model is required")
	}
	if len(req.Input) == 0 {
		return EmbedResult{Vectors: [][]float32{}, Dimensions: req.Dimensions}, nil
	}

	var vectors [][]float32
	if withOpts, ok := e.Client.(llm.EmbedderWithOptions); ok {
		opts := &llm.EmbedOptions{
			KeepAlive: req.KeepAlive,
			Truncate:  req.Truncate,
		}
		if req.Dimensions > 0 {
			dims := req.Dimensions
			opts.Dimensions = &dims
		}
		out, err := withOpts.EmbedWithOptions(ctx, req.Model, req.Input, opts)
		if err != nil {
			return EmbedResult{}, err
		}
		vectors = out
	} else {
		out, err := e.Client.Embed(ctx, req.Model, req.Input)
		if err != nil {
			return EmbedResult{}, err
		}
		vectors = out
	}

	if len(vectors) != len(req.Input) {
		return EmbedResult{}, fmt.Errorf("semantic embedder: vector count mismatch got %d want %d", len(vectors), len(req.Input))
	}
	dims := req.Dimensions
	if dims <= 0 && len(vectors) > 0 {
		dims = len(vectors[0])
	}
	return EmbedResult{Vectors: vectors, Dimensions: dims}, nil
}
