package modelruntime

import (
	"context"
	"errors"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/credentials"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/llm/modelresolver"
)

type showClient struct {
	fakeClient
	details  llm.ModelDetails
	showErr  error
	showName string
}

func (c *showClient) ShowModel(_ context.Context, name string) (llm.ModelDetails, error) {
	c.showName = name
	return c.details, c.showErr
}

func newActivateService(local *showClient, cloudCatalog llm.Client) (*Service, *llm.RuntimeClient) {
	rt := llm.NewRuntimeClient(local, llm.ProviderOllamaLocal, "http://localhost:11434")
	return &Service{
		LocalClient:  local,
		LocalBaseURL: "http://localhost:11434",
		Runtime:      rt,
		Resolver:     &modelresolver.Resolver{LocalClient: local, CloudClient: cloudCatalog, CloudEnabled: true},
		Creds:        &credentials.Resolver{Store: &memStore{}},
	}, rt
}

func TestActivateLocalSwitchesAndRefreshesLimits(t *testing.T) {
	t.Parallel()
	local := &showClient{
		fakeClient: fakeClient{models: []llm.ModelInfo{{Name: "qwen3.6:35b"}}},
		details:    llm.ModelDetails{ContextLength: 32768},
	}
	svc, _ := newActivateService(local, &fakeClient{})
	res, err := svc.Activate(context.Background(), "qwen3.6:35b")
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if res.Resolved.Model != "qwen3.6:35b" || res.UsedCloud {
		t.Fatalf("unexpected result: %+v", res)
	}
	want := llm.ComputeLimits(local.details)
	if !res.HasLimits || res.Limits != want {
		t.Fatalf("limits=%+v has=%v want %+v", res.Limits, res.HasLimits, want)
	}
	if local.showName != "qwen3.6:35b" {
		t.Fatalf("ShowModel called with %q", local.showName)
	}
}

func TestActivateCloudWithoutCredentialNeverPrompts(t *testing.T) {
	t.Setenv("OLLAMA_API_KEY", "")
	local := &showClient{}
	cloudCatalog := &fakeClient{models: []llm.ModelInfo{{Name: "gpt-oss:120b"}}}
	svc, rt := newActivateService(local, cloudCatalog)
	_, err := svc.Activate(context.Background(), "gpt-oss:120b")
	if !errors.Is(err, ErrCredentialRequired) {
		t.Fatalf("err=%v, want ErrCredentialRequired", err)
	}
	if snap := rt.Snapshot(); snap.Provider != llm.ProviderOllamaLocal {
		t.Fatalf("runtime switched despite missing credential: %s", snap.Provider)
	}
}

func TestActivateLimitsFailureIsNotFatal(t *testing.T) {
	t.Parallel()
	local := &showClient{
		fakeClient: fakeClient{models: []llm.ModelInfo{{Name: "qwen3.6:35b"}}},
		showErr:    errors.New("boom"),
	}
	svc, rt := newActivateService(local, &fakeClient{})
	res, err := svc.Activate(context.Background(), "qwen3.6:35b")
	if err != nil {
		t.Fatalf("Activate must succeed when only limits fail: %v", err)
	}
	if res.HasLimits || res.LimitsErr == nil {
		t.Fatalf("want HasLimits=false and LimitsErr set, got %+v", res)
	}
	if snap := rt.Snapshot(); snap.Provider != llm.ProviderOllamaLocal {
		t.Fatalf("provider=%s", snap.Provider)
	}
}

func TestActivateUnknownModelLeavesRuntimeUntouched(t *testing.T) {
	t.Parallel()
	local := &showClient{fakeClient: fakeClient{models: []llm.ModelInfo{{Name: "qwen3.6:35b"}}}}
	svc, rt := newActivateService(local, &fakeClient{})
	_, err := svc.Activate(context.Background(), "does-not-exist")
	if !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("err=%v, want ErrModelNotFound", err)
	}
	if got := rt.Snapshot().BaseURL; got != "http://localhost:11434" {
		t.Fatalf("base=%q", got)
	}
}

func TestActivateNilServiceErrors(t *testing.T) {
	t.Parallel()
	if _, err := (*Service)(nil).Activate(context.Background(), "x"); err == nil {
		t.Fatal("nil service must error")
	}
}
