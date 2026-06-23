package modelruntime

import (
	"context"
	"errors"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/credentials"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/llm/modelresolver"
)

type fakeClient struct {
	models      []llm.ModelInfo
	listErr     error
	pulledModel string
}

func (f *fakeClient) Chat(context.Context, *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent)
	close(ch)
	return ch, nil
}
func (f *fakeClient) Embed(context.Context, string, []string) ([][]float32, error) { return nil, nil }
func (f *fakeClient) ListModels(context.Context) ([]llm.ModelInfo, error) {
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
func (f *fakeClient) PullModel(_ context.Context, name string, _ chan<- llm.PullProgress) error {
	f.pulledModel = name
	return nil
}

type memStore struct {
	v string
}

func (m *memStore) Get(_, _ string) (string, error) { return m.v, nil }
func (m *memStore) Set(_, _ string, s string) error { m.v = s; return nil }
func (m *memStore) Delete(_, _ string) error        { m.v = ""; return nil }

func TestSwitchLocalModel(t *testing.T) {
	t.Parallel()
	local := &fakeClient{models: []llm.ModelInfo{{Name: "qwen3.6:35b"}}}
	cloudCatalog := &fakeClient{models: []llm.ModelInfo{{Name: "gpt-oss:120b"}}}
	runtime := llm.NewRuntimeClient(local, llm.ProviderOllamaLocal, "http://localhost:11434")
	svc := &Service{
		LocalClient:  local,
		LocalBaseURL: "http://localhost:11434",
		Runtime:      runtime,
		Resolver:     &modelresolver.Resolver{LocalClient: local, CloudClient: cloudCatalog, CloudEnabled: true},
		Creds:        &credentials.Resolver{Store: &memStore{}},
	}
	res, err := svc.Switch(context.Background(), SwitchOptions{RequestedModel: "qwen3.6:35b"})
	if err != nil {
		t.Fatalf("Switch() error = %v", err)
	}
	if res.UsedCloud {
		t.Fatalf("expected local switch")
	}
	if snap := runtime.Snapshot(); snap.Provider != llm.ProviderOllamaLocal {
		t.Fatalf("provider=%s", snap.Provider)
	}
}

func TestSwitchCloudRequiresCredentialWhenNonInteractive(t *testing.T) {
	t.Setenv("OLLAMA_API_KEY", "")
	local := &fakeClient{}
	cloudCatalog := &fakeClient{models: []llm.ModelInfo{{Name: "gpt-oss:120b"}}}
	svc := &Service{
		LocalClient:  local,
		LocalBaseURL: "http://localhost:11434",
		Runtime:      llm.NewRuntimeClient(local, llm.ProviderOllamaLocal, "http://localhost:11434"),
		Resolver:     &modelresolver.Resolver{LocalClient: local, CloudClient: cloudCatalog, CloudEnabled: true},
		Creds:        &credentials.Resolver{Store: &memStore{}},
	}
	_, err := svc.Switch(context.Background(), SwitchOptions{RequestedModel: "gpt-oss:120b", AllowPrompt: false})
	if !errors.Is(err, ErrCredentialRequired) {
		t.Fatalf("expected ErrCredentialRequired, got %v", err)
	}
}

func TestSwitchCloudWithEnvKey(t *testing.T) {
	t.Setenv("OLLAMA_API_KEY", "env-key")
	local := &fakeClient{}
	cloudCatalog := &fakeClient{models: []llm.ModelInfo{{Name: "gpt-oss:120b"}}}
	cloudRuntimeClient := &fakeClient{models: []llm.ModelInfo{{Name: "gpt-oss:120b"}}}
	runtime := llm.NewRuntimeClient(local, llm.ProviderOllamaLocal, "http://localhost:11434")
	svc := &Service{
		LocalClient:  local,
		LocalBaseURL: "http://localhost:11434",
		Runtime:      runtime,
		Resolver:     &modelresolver.Resolver{LocalClient: local, CloudClient: cloudCatalog, CloudEnabled: true},
		Creds:        &credentials.Resolver{Store: &memStore{}},
		NewCloudClient: func(baseURL, apiKey string) llm.Client {
			if baseURL != llm.OllamaCloudBaseURL {
				t.Fatalf("baseURL=%q", baseURL)
			}
			if apiKey != "env-key" {
				t.Fatalf("apiKey=%q", apiKey)
			}
			return cloudRuntimeClient
		},
	}
	res, err := svc.Switch(context.Background(), SwitchOptions{RequestedModel: "gpt-oss:120b"})
	if err != nil {
		t.Fatalf("Switch() error = %v", err)
	}
	if !res.UsedCloud {
		t.Fatalf("expected cloud switch")
	}
	if snap := runtime.Snapshot(); snap.Provider != llm.ProviderOllamaCloudAPI {
		t.Fatalf("provider=%s", snap.Provider)
	}
}

func TestPullLocalUsesLocalClient(t *testing.T) {
	t.Parallel()
	local := &fakeClient{}
	svc := &Service{LocalClient: local}
	if err := svc.PullLocal(context.Background(), "qwen3.6:35b", make(chan llm.PullProgress, 1)); err != nil {
		t.Fatalf("PullLocal() error = %v", err)
	}
	if local.pulledModel != "qwen3.6:35b" {
		t.Fatalf("pulledModel=%q", local.pulledModel)
	}
}
