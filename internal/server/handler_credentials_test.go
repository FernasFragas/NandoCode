package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/credentials"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/llm/modelresolver"
	"github.com/FernasFragas/Nandocode/internal/llm/modelruntime"
)

const secretKey = "sk-super-secret-ollama-key-12345"

// limitsClient serves a fixed catalog and per-model context lengths, and
// records every chat request so tests can prove none was sent.
type limitsClient struct {
	models []llm.ModelInfo
	ctxLen map[string]int64

	mu      sync.Mutex
	chats   int
	listErr error
}

func (c *limitsClient) Chat(context.Context, *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	c.mu.Lock()
	c.chats++
	c.mu.Unlock()
	ch := make(chan llm.StreamEvent)
	close(ch)
	return ch, nil
}

func (c *limitsClient) Embed(context.Context, string, []string) ([][]float32, error) {
	return nil, nil
}

func (c *limitsClient) ListModels(context.Context) ([]llm.ModelInfo, error) {
	if c.listErr != nil {
		return nil, c.listErr
	}
	return append([]llm.ModelInfo(nil), c.models...), nil
}

func (c *limitsClient) ShowModel(_ context.Context, name string) (llm.ModelDetails, error) {
	return llm.ModelDetails{Name: name, ContextLength: c.ctxLen[name]}, nil
}

func (c *limitsClient) PullModel(context.Context, string, chan<- llm.PullProgress) error {
	return nil
}

func (c *limitsClient) chatCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.chats
}

// maxOutRunner records the per-run MaxOutputTokens the session prepared.
type maxOutRunner struct{ out chan int }

func (r maxOutRunner) Run(_ context.Context, in agent.Input) <-chan agent.Event {
	r.out <- in.MaxOutputTokens
	ch := make(chan agent.Event, 1)
	ch <- agent.Terminal{Reason: agent.TerminalCompleted}
	close(ch)
	return ch
}

func credTestServer(t *testing.T, local, cloudCatalog llm.Client, store credentials.Store, newCloud func(string, string) llm.Client) *Server {
	t.Helper()
	t.Setenv("OLLAMA_API_KEY", "")
	s := testServer(t)
	s.modelRuntime = &modelruntime.Service{
		LocalClient:    local,
		LocalBaseURL:   "http://localhost:11434",
		Runtime:        llm.NewRuntimeClient(local, llm.ProviderOllamaLocal, "http://localhost:11434"),
		Resolver:       &modelresolver.Resolver{LocalClient: local, CloudClient: cloudCatalog, CloudEnabled: true},
		Creds:          &credentials.Resolver{Store: store},
		NewCloudClient: newCloud,
	}
	return s
}

func TestUpdateModelRefreshesLimitsAfterSwitch(t *testing.T) {
	local := &limitsClient{
		models: []llm.ModelInfo{{Name: "small"}, {Name: "big"}},
		ctxLen: map[string]int64{"small": 8192, "big": 131072},
	}
	s := credTestServer(t, local, &limitsClient{}, &testCredStore{}, nil)
	runs := make(chan int, 2)
	s.sessionRuntimes = nil
	s.runner = maxOutRunner{out: runs}
	id := createTestSession(t, s)

	switchTo := func(model string) {
		t.Helper()
		w := httptest.NewRecorder()
		s.handleUpdateModel(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"model":"`+model+`"}`)), id)
		if w.Code != http.StatusOK {
			t.Fatalf("switch %s status=%d body=%s", model, w.Code, w.Body.String())
		}
	}
	switchTo("small")
	sess, _ := s.registry.get(id)
	sess.mu.Lock()
	small := sess.appState.MaxOutputTokens
	sess.mu.Unlock()
	if small != 4096 {
		t.Fatalf("MaxOutputTokens after small=%d want 4096", small)
	}
	switchTo("big")
	sess.mu.Lock()
	gotMax, gotCtx := sess.appState.MaxOutputTokens, sess.appState.RuntimeNumCtx
	sess.mu.Unlock()
	if gotMax != 65536 || gotCtx != 131072 {
		t.Fatalf("limits after big: max=%d numctx=%d want 65536/131072", gotMax, gotCtx)
	}
	w := httptest.NewRecorder()
	s.handlePostMessage(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"prompt":"hi"}`)), id)
	if w.Code != http.StatusAccepted {
		t.Fatalf("post status=%d body=%s", w.Code, w.Body.String())
	}
	if got := <-runs; got != 65536 {
		t.Fatalf("run MaxOutputTokens=%d want 65536", got)
	}
}

func TestUpdateModelKeylessCloudSwitchRefused(t *testing.T) {
	local := &limitsClient{models: []llm.ModelInfo{{Name: "qwen3"}}, ctxLen: map[string]int64{"qwen3": 8192}}
	cloud := &limitsClient{models: []llm.ModelInfo{{Name: "kimi-k2.6"}}, ctxLen: map[string]int64{"kimi-k2.6": 262144}}
	built := 0
	s := credTestServer(t, local, cloud, &testCredStore{}, func(string, string) llm.Client { built++; return cloud })
	id := createTestSession(t, s)
	sess, _ := s.registry.get(id)
	sess.mu.Lock()
	before := sess.appState.ActiveModel
	sess.mu.Unlock()

	w := httptest.NewRecorder()
	s.handleUpdateModel(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"model":"kimi-k2.6:cloud"}`)), id)
	if w.Code != http.StatusPreconditionRequired {
		t.Fatalf("status=%d want 428 body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body["error"] != "requires_credential" {
		t.Fatalf("body=%s", w.Body.String())
	}
	if built != 0 || cloud.chatCount() != 0 {
		t.Fatalf("cloud client used without credential: built=%d chats=%d", built, cloud.chatCount())
	}
	sess.mu.Lock()
	after, provider := sess.appState.ActiveModel, sess.appState.LLMProvider
	sess.mu.Unlock()
	if after != before || provider == string(llm.ProviderOllamaCloudAPI) {
		t.Fatalf("session changed on refused switch: model=%q provider=%q", after, provider)
	}
}

type memStore struct {
	mu   sync.Mutex
	v    string
	sets int
}

func (m *memStore) Get(_, _ string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.v, nil
}

func (m *memStore) Set(_, _, s string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.v = s
	m.sets++
	return nil
}
func (m *memStore) Delete(_, _ string) error { m.mu.Lock(); defer m.mu.Unlock(); m.v = ""; return nil }

func postCredential(s *Server, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/credentials/ollama-cloud", strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("Content-Type", "application/json")
	s.routes().ServeHTTP(w, r)
	return w
}

func captureLogger(s *Server) *bytes.Buffer {
	var buf bytes.Buffer
	s.logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return &buf
}

func TestCredentialEndpointUseOnceNeverEchoesOrLogsKey(t *testing.T) {
	local := &limitsClient{models: []llm.ModelInfo{{Name: "qwen3"}}}
	cloud := &limitsClient{models: []llm.ModelInfo{{Name: "kimi-k2.6"}}, ctxLen: map[string]int64{"kimi-k2.6": 262144}}
	store := &memStore{}
	var gotKey string
	s := credTestServer(t, local, cloud, store, func(_, key string) llm.Client { gotKey = key; return cloud })
	logs := captureLogger(s)

	w := postCredential(s, `{"key":"`+secretKey+`","save":false}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), secretKey) || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("response echoes key: %s", w.Body.String())
	}
	if strings.Contains(logs.String(), secretKey) {
		t.Fatalf("logs contain key: %s", logs.String())
	}
	if store.sets != 0 {
		t.Fatalf("use-once saved to keychain")
	}
	if gotKey != secretKey {
		t.Fatalf("validation did not use the supplied key")
	}
	// The key now unlocks a cloud switch for sessions.
	id := createTestSession(t, s)
	w = httptest.NewRecorder()
	s.handleUpdateModel(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"model":"kimi-k2.6:cloud"}`)), id)
	if w.Code != http.StatusOK {
		t.Fatalf("switch after credential status=%d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), secretKey) || strings.Contains(logs.String(), secretKey) {
		t.Fatal("key leaked on switch")
	}
}

func TestCloudSwitchCredentialEndpointRetry(t *testing.T) {
	local := &limitsClient{models: []llm.ModelInfo{{Name: "qwen3"}}}
	cloud := &limitsClient{models: []llm.ModelInfo{{Name: "kimi-k2.6"}}, ctxLen: map[string]int64{"kimi-k2.6": 262144}}
	store := &memStore{}
	s := credTestServer(t, local, cloud, store, func(_, key string) llm.Client {
		if key != secretKey {
			t.Fatalf("unexpected cloud key")
		}
		return cloud
	})
	id := createTestSession(t, s)
	switchModel := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+id+"/model", strings.NewReader(`{"model":"kimi-k2.6:cloud"}`))
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("Content-Type", "application/json")
		s.routes().ServeHTTP(w, r)
		return w
	}
	first := switchModel()
	if first.Code != http.StatusPreconditionRequired {
		t.Fatalf("initial switch status=%d body=%s", first.Code, first.Body.String())
	}
	var required map[string]any
	if err := json.Unmarshal(first.Body.Bytes(), &required); err != nil || required["error"] != "requires_credential" {
		t.Fatalf("initial switch body=%s err=%v", first.Body.String(), err)
	}
	if cloud.chatCount() != 0 {
		t.Fatal("cloud chat sent before credential")
	}
	saved := postCredential(s, `{"key":"`+secretKey+`","save":false}`)
	if saved.Code != http.StatusOK {
		t.Fatalf("credential status=%d body=%s", saved.Code, saved.Body.String())
	}
	retry := switchModel()
	if retry.Code != http.StatusOK {
		t.Fatalf("retry status=%d body=%s", retry.Code, retry.Body.String())
	}
	var selected map[string]any
	if err := json.Unmarshal(retry.Body.Bytes(), &selected); err != nil || selected["provider"] != string(llm.ProviderOllamaCloudAPI) {
		t.Fatalf("retry body=%s err=%v", retry.Body.String(), err)
	}
	if store.sets != 0 || strings.Contains(first.Body.String()+saved.Body.String()+retry.Body.String(), secretKey) {
		t.Fatal("use-once credential was persisted or exposed")
	}
}

func TestCredentialEndpointSaveStoresInKeychain(t *testing.T) {
	cloud := &limitsClient{models: []llm.ModelInfo{{Name: "kimi-k2.6"}}}
	store := &memStore{}
	s := credTestServer(t, &limitsClient{}, cloud, store, func(string, string) llm.Client { return cloud })
	logs := captureLogger(s)
	w := postCredential(s, `{"key":"`+secretKey+`","save":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if store.v != secretKey || store.sets != 1 {
		t.Fatalf("keychain not written: sets=%d", store.sets)
	}
	if strings.Contains(w.Body.String(), secretKey) || strings.Contains(logs.String(), secretKey) {
		t.Fatal("key leaked")
	}
}

func TestCredentialEndpointRejectsBadInputWithoutLeaking(t *testing.T) {
	cloud := &limitsClient{listErr: errString("ollama API error (status 401): unauthorized " + secretKey)}
	store := &memStore{}
	s := credTestServer(t, &limitsClient{}, cloud, store, func(string, string) llm.Client { return cloud })
	logs := captureLogger(s)

	for _, tc := range []struct {
		name, body string
		want       int
	}{
		{"invalid json", `{`, http.StatusBadRequest},
		{"empty key", `{"key":"   "}`, http.StatusBadRequest},
		{"unauthorized key", `{"key":"` + secretKey + `","save":true}`, http.StatusUnauthorized},
	} {
		w := postCredential(s, tc.body)
		if w.Code != tc.want {
			t.Fatalf("%s: status=%d want %d body=%s", tc.name, w.Code, tc.want, w.Body.String())
		}
		if strings.Contains(w.Body.String(), secretKey) {
			t.Fatalf("%s: response leaks key", tc.name)
		}
	}
	if store.sets != 0 || store.v != "" {
		t.Fatal("invalid key was persisted")
	}
	if strings.Contains(logs.String(), secretKey) {
		t.Fatalf("logs contain key: %s", logs.String())
	}
	// A rejected key must not unlock cloud use.
	if _, _, err := s.modelRuntime.Creds.Resolve(context.Background(), credentials.ResolveOptions{}); err == nil {
		t.Fatal("rejected key stayed in session memory")
	}
}

type errString string

func (e errString) Error() string { return string(e) }
