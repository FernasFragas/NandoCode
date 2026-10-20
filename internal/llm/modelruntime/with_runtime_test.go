package modelruntime

import (
	"context"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/credentials"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/llm/modelresolver"
)

type listClient struct{ llm.Client }

func (listClient) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{{Name: "qwen3"}, {Name: "llama3"}}, nil
}

func TestWithRuntimeSwitchesOnlyTheGivenRuntime(t *testing.T) {
	t.Parallel()
	local := listClient{}
	shared := llm.NewRuntimeClient(local, llm.ProviderOllamaLocal, "shared-base")
	own := llm.NewRuntimeClient(local, llm.ProviderOllamaLocal, "own-base")
	base := &Service{
		LocalClient:  local,
		LocalBaseURL: "local-base",
		Runtime:      shared,
		Resolver:     &modelresolver.Resolver{LocalClient: local},
		Creds:        credentials.NewResolver(),
	}
	svc := base.WithRuntime(own)
	if svc == base || base.Runtime != shared {
		t.Fatal("WithRuntime must copy, not mutate, the base service")
	}
	if _, err := svc.Switch(context.Background(), SwitchOptions{RequestedModel: "qwen3"}); err != nil {
		t.Fatalf("Switch: %v", err)
	}
	if got := own.Snapshot().BaseURL; got != "local-base" {
		t.Fatalf("own runtime base=%q, want it switched", got)
	}
	if got := shared.Snapshot().BaseURL; got != "shared-base" {
		t.Fatalf("shared runtime was touched: base=%q", got)
	}
	if (*Service)(nil).WithRuntime(own) != nil {
		t.Fatal("nil service must stay nil")
	}
}
