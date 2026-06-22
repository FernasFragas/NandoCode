package modelresolver

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FernasFragas/nandocodego/internal/llm"
)

type fakeClient struct {
	models    []llm.ModelInfo
	listErr   error
	listCalls int
}

func (f *fakeClient) Chat(context.Context, *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent)
	close(ch)
	return ch, nil
}
func (f *fakeClient) Embed(context.Context, string, []string) ([][]float32, error) { return nil, nil }
func (f *fakeClient) ListModels(context.Context) ([]llm.ModelInfo, error) {
	f.listCalls++
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]llm.ModelInfo, len(f.models))
	copy(out, f.models)
	return out, nil
}
func (f *fakeClient) ShowModel(context.Context, string) (llm.ModelDetails, error) {
	return llm.ModelDetails{}, nil
}
func (f *fakeClient) PullModel(context.Context, string, chan<- llm.PullProgress) error { return nil }

func TestResolveExactLocalWinsOverCloud(t *testing.T) {
	t.Parallel()
	local := &fakeClient{models: []llm.ModelInfo{{Name: "gpt-oss:120b"}}}
	cloud := &fakeClient{models: []llm.ModelInfo{{Name: "gpt-oss:120b"}}}
	r := &Resolver{LocalClient: local, CloudClient: cloud, CloudEnabled: true}
	got, err := r.Resolve(context.Background(), "gpt-oss:120b")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.Provider != llm.ProviderOllamaLocal {
		t.Fatalf("provider=%s", got.Provider)
	}
	if cloud.listCalls != 0 {
		t.Fatalf("expected no cloud lookup, calls=%d", cloud.listCalls)
	}
}

func TestResolveCloudExactMatch(t *testing.T) {
	t.Parallel()
	local := &fakeClient{}
	cloud := &fakeClient{models: []llm.ModelInfo{{Name: "gpt-oss:120b"}}}
	r := &Resolver{LocalClient: local, CloudClient: cloud, CloudEnabled: true}
	got, err := r.Resolve(context.Background(), "gpt-oss:120b")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.Provider != llm.ProviderOllamaCloudAPI || got.Model != "gpt-oss:120b" {
		t.Fatalf("resolved=%+v", got)
	}
}

func TestResolveCloudSuffixAlias(t *testing.T) {
	t.Parallel()
	local := &fakeClient{}
	cloud := &fakeClient{models: []llm.ModelInfo{{Name: "gpt-oss:120b"}}}
	r := &Resolver{LocalClient: local, CloudClient: cloud, CloudEnabled: true}
	got, err := r.Resolve(context.Background(), "gpt-oss:120b-cloud")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !got.AliasUsed || got.Model != "gpt-oss:120b" {
		t.Fatalf("resolved=%+v", got)
	}
}

func TestResolveColonCloudSuffixBypassesLocal(t *testing.T) {
	t.Parallel()
	local := &fakeClient{models: []llm.ModelInfo{{Name: "kimi-k2.6:cloud"}}}
	cloud := &fakeClient{models: []llm.ModelInfo{{Name: "kimi-k2.6"}}}
	r := &Resolver{LocalClient: local, CloudClient: cloud, CloudEnabled: true}
	got, err := r.Resolve(context.Background(), "kimi-k2.6:cloud")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.Provider != llm.ProviderOllamaCloudAPI || got.Model != "kimi-k2.6" {
		t.Fatalf("resolved=%+v", got)
	}
	if !got.AliasUsed || got.AliasReason != "trimmed final :cloud suffix" {
		t.Fatalf("expected :cloud alias metadata, resolved=%+v", got)
	}
	if local.listCalls != 0 {
		t.Fatalf("expected :cloud to bypass local lookup, calls=%d", local.listCalls)
	}
}

func TestResolveColonCloudSuffixHonorsCloudDisabled(t *testing.T) {
	t.Parallel()
	r := &Resolver{
		LocalClient:  &fakeClient{models: []llm.ModelInfo{{Name: "kimi-k2.6:cloud"}}},
		CloudClient:  &fakeClient{models: []llm.ModelInfo{{Name: "kimi-k2.6"}}},
		CloudEnabled: false,
	}
	_, err := r.Resolve(context.Background(), "kimi-k2.6:cloud")
	if !errors.Is(err, ErrCloudDisabled) {
		t.Fatalf("expected ErrCloudDisabled, got %v", err)
	}
}

func TestResolveCloudSuffixLocalExactStaysLocal(t *testing.T) {
	t.Parallel()
	local := &fakeClient{models: []llm.ModelInfo{{Name: "gpt-oss:120b-cloud"}}}
	cloud := &fakeClient{models: []llm.ModelInfo{{Name: "gpt-oss:120b"}}}
	r := &Resolver{LocalClient: local, CloudClient: cloud, CloudEnabled: true}
	got, err := r.Resolve(context.Background(), "gpt-oss:120b-cloud")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.Provider != llm.ProviderOllamaLocal {
		t.Fatalf("resolved=%+v", got)
	}
}

func TestResolveCloudDisabled(t *testing.T) {
	t.Parallel()
	r := &Resolver{LocalClient: &fakeClient{}, CloudClient: &fakeClient{}, CloudEnabled: false}
	_, err := r.Resolve(context.Background(), "gpt-oss:120b")
	if !errors.Is(err, ErrCloudDisabled) {
		t.Fatalf("expected ErrCloudDisabled, got %v", err)
	}
}

func TestResolveLocalMatchUnaffectedByCloudFailure(t *testing.T) {
	t.Parallel()
	local := &fakeClient{models: []llm.ModelInfo{{Name: "qwen3.6:35b"}}}
	cloud := &fakeClient{listErr: errors.New("boom")}
	r := &Resolver{LocalClient: local, CloudClient: cloud, CloudEnabled: true}
	got, err := r.Resolve(context.Background(), "qwen3.6:35b")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.Provider != llm.ProviderOllamaLocal {
		t.Fatalf("resolved=%+v", got)
	}
}

func TestCloudCatalogCacheAvoidsRepeatedCalls(t *testing.T) {
	t.Parallel()
	local := &fakeClient{}
	cloud := &fakeClient{models: []llm.ModelInfo{{Name: "gpt-oss:120b"}}}
	now := time.Now()
	r := &Resolver{
		LocalClient:  local,
		CloudClient:  cloud,
		CloudEnabled: true,
		SuccessTTL:   time.Minute,
		Now: func() time.Time {
			return now
		},
	}
	if _, err := r.ListCloud(context.Background()); err != nil {
		t.Fatalf("ListCloud() error = %v", err)
	}
	if _, err := r.ListCloud(context.Background()); err != nil {
		t.Fatalf("ListCloud() second error = %v", err)
	}
	if cloud.listCalls != 1 {
		t.Fatalf("cloud list calls=%d", cloud.listCalls)
	}
}
