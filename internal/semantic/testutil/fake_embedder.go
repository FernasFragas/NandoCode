package testutil

import (
	"context"
	"fmt"

	"github.com/FernasFragas/Nandocode/internal/semantic"
)

type FakeEmbedder struct {
	Dimensions int
	Calls      int
	FailErr    error
}

func (f *FakeEmbedder) Embed(ctx context.Context, req semantic.EmbedRequest) (semantic.EmbedResult, error) {
	_ = ctx
	if f.FailErr != nil {
		return semantic.EmbedResult{}, f.FailErr
	}
	f.Calls++
	dims := req.Dimensions
	if dims <= 0 {
		dims = f.Dimensions
	}
	if dims <= 0 {
		dims = semantic.DefaultDimensions
	}

	out := make([][]float32, len(req.Input))
	for i, text := range req.Input {
		vec := DeterministicVector(text, dims)
		if !semantic.NormalizeVector(vec) {
			return semantic.EmbedResult{}, fmt.Errorf("generated empty deterministic vector")
		}
		out[i] = vec
	}
	return semantic.EmbedResult{
		Vectors:    out,
		Dimensions: dims,
	}, nil
}
