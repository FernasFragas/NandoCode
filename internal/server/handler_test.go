package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/bootstrap"
	"github.com/FernasFragas/Nandocode/internal/credentials"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/llm/modelresolver"
	"github.com/FernasFragas/Nandocode/internal/llm/modelruntime"
	"github.com/FernasFragas/Nandocode/internal/state"
)

type fakeClient struct{}

func (fakeClient) Chat(context.Context, *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	return nil, nil
}
func (fakeClient) Embed(context.Context, string, []string) ([][]float32, error) { return nil, nil }
func (fakeClient) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{{Name: "qwen3"}}, nil
}
func (fakeClient) ShowModel(context.Context, string) (llm.ModelDetails, error) {
	return llm.ModelDetails{}, nil
}
func (fakeClient) PullModel(context.Context, string, chan<- llm.PullProgress) error { return nil }

type testLLMClient struct {
	models []llm.ModelInfo
}

func (t *testLLMClient) Chat(context.Context, *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent)
	close(ch)
	return ch, nil
}
func (t *testLLMClient) Embed(context.Context, string, []string) ([][]float32, error) {
	return nil, nil
}
func (t *testLLMClient) ListModels(context.Context) ([]llm.ModelInfo, error) {
	out := make([]llm.ModelInfo, len(t.models))
	copy(out, t.models)
	return out, nil
}
func (t *testLLMClient) ShowModel(context.Context, string) (llm.ModelDetails, error) {
	return llm.ModelDetails{}, nil
}
func (t *testLLMClient) PullModel(context.Context, string, chan<- llm.PullProgress) error { return nil }

type testCredStore struct {
	v string
}

func (t *testCredStore) Get(_, _ string) (string, error) { return t.v, nil }
func (t *testCredStore) Set(_, _ string, s string) error { t.v = s; return nil }
func (t *testCredStore) Delete(_, _ string) error        { t.v = ""; return nil }

type tinyRunner struct{}

func (tinyRunner) Run(_ context.Context, _ agent.Input) <-chan agent.Event {
	ch := make(chan agent.Event, 2)
	ch <- agent.Terminal{Reason: agent.TerminalCompleted, Conversation: []llm.Message{{Role: llm.RoleAssistant, Content: "done"}}}
	close(ch)
	return ch
}

func testServer(t *testing.T) *Server {
	t.Helper()
	init := bootstrap.DefaultInitial(".")
	app := state.DefaultApp(bootstrap.New(init).Snapshot())
	s := &Server{ctx: context.Background(), client: fakeClient{}, runner: tinyRunner{}, agentCfg: agent.DefaultConfig(), baseApp: app, registry: newSessionRegistry(), limiter: NewRateLimiter(100, 10)}
	return s
}

func TestCreateSessionAndGet(t *testing.T) {
	s := testServer(t)
	r := httptest.NewRequest(http.MethodPost, "/v1/sessions", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.handleCreateSession(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status=%d", w.Code)
	}
	var view SessionView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.SessionID == "" {
		t.Fatal("missing session id")
	}
	if view.CoordinatorMode != s.baseApp.CoordinatorMode {
		t.Fatalf("coordinator_mode=%v want=%v", view.CoordinatorMode, s.baseApp.CoordinatorMode)
	}
	w = httptest.NewRecorder()
	s.handleGetSession(w, httptest.NewRequest(http.MethodGet, "/", nil), view.SessionID)
	if w.Code != http.StatusOK {
		t.Fatalf("get status=%d", w.Code)
	}
}

func TestPostMessageConflictAndDuplicate(t *testing.T) {
	s := testServer(t)
	r := httptest.NewRequest(http.MethodPost, "/v1/sessions", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.handleCreateSession(w, r)
	var view SessionView
	_ = json.Unmarshal(w.Body.Bytes(), &view)

	msg := `{"prompt":"hello","message_id":"m1"}`
	w = httptest.NewRecorder()
	s.handlePostMessage(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(msg)), view.SessionID)
	if w.Code != http.StatusAccepted {
		t.Fatalf("post status=%d", w.Code)
	}

	w = httptest.NewRecorder()
	s.handlePostMessage(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(msg)), view.SessionID)
	if w.Code != http.StatusAccepted {
		t.Fatalf("dup status=%d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"duplicate":true`) {
		t.Fatalf("dup body=%s", w.Body.String())
	}

	time.Sleep(30 * time.Millisecond)
	if sess, ok := s.registry.get(view.SessionID); ok {
		sess.mu.Lock()
		sess.running = true
		sess.mu.Unlock()
		w = httptest.NewRecorder()
		s.handlePostMessage(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"prompt":"x"}`)), view.SessionID)
		if w.Code != http.StatusConflict {
			t.Fatalf("conflict status=%d", w.Code)
		}
	}
}

func TestHealthAndModels(t *testing.T) {
	s := testServer(t)
	w := httptest.NewRecorder()
	s.handleHealth(w, httptest.NewRequest(http.MethodGet, "/v1/health", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("health status=%d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"status":"ok"`) {
		t.Fatalf("health body=%s", w.Body.String())
	}

	w = httptest.NewRecorder()
	s.handleModels(w, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("models status=%d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "qwen3") {
		t.Fatalf("models body=%s", w.Body.String())
	}
}

func TestRoutesServeRichEmbeddedUI(t *testing.T) {
	s := testServer(t)
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{
		`id="model-picker"`,
		`id="modal-overlay"`,
		`currentReader = r.body.getReader()`,
		`eventPayload(msg)`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("served UI missing %q", want)
		}
	}
	for _, old := range []string{
		"new EventSource",
		"Connect SSE",
	} {
		if strings.Contains(body, old) {
			t.Fatalf("served UI still contains old manual SSE marker %q", old)
		}
	}
}

func TestPostMessageRequiresCredentialForCloudModel(t *testing.T) {
	t.Setenv("OLLAMA_API_KEY", "")
	s := testServer(t)
	s.baseApp.ActiveModel = "gpt-oss:120b"

	local := &testLLMClient{models: []llm.ModelInfo{}}
	cloud := &testLLMClient{models: []llm.ModelInfo{{Name: "gpt-oss:120b"}}}
	runtime := llm.NewRuntimeClient(local, llm.ProviderOllamaLocal, "http://localhost:11434")
	s.modelRuntime = &modelruntime.Service{
		LocalClient:  local,
		LocalBaseURL: "http://localhost:11434",
		Runtime:      runtime,
		Resolver: &modelresolver.Resolver{
			LocalClient:  local,
			CloudClient:  cloud,
			CloudEnabled: true,
		},
		Creds: &credentials.Resolver{Store: &testCredStore{}},
	}

	r := httptest.NewRequest(http.MethodPost, "/v1/sessions", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.handleCreateSession(w, r)
	var view SessionView
	_ = json.Unmarshal(w.Body.Bytes(), &view)

	w = httptest.NewRecorder()
	s.handlePostMessage(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"prompt":"hello"}`)), view.SessionID)
	if w.Code != http.StatusPreconditionRequired {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"error":"requires_credential"`) {
		t.Fatalf("expected credential error, got %s", w.Body.String())
	}
}

func TestEventsHandlerLastEventIDReplay(t *testing.T) {
	s := testServer(t)
	r := httptest.NewRequest(http.MethodPost, "/v1/sessions", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.handleCreateSession(w, r)
	var view SessionView
	_ = json.Unmarshal(w.Body.Bytes(), &view)
	sess, ok := s.registry.get(view.SessionID)
	if !ok {
		t.Fatal("missing session")
	}
	sess.Emit("a", map[string]any{"v": 1})
	sess.Emit("b", map[string]any{"v": 2})
	sess.Emit("c", map[string]any{"v": 3})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+view.SessionID+"/events", nil).WithContext(ctx)
	req.Header.Set("Last-Event-ID", "3")
	rr := httptest.NewRecorder()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.handleEvents(rr, req, view.SessionID)
	}()
	time.Sleep(25 * time.Millisecond)
	cancel()
	wg.Wait()
	body := rr.Body.String()
	if !strings.Contains(body, "event: c") {
		t.Fatalf("expected replayed event c, got body=%q", body)
	}
	if strings.Contains(body, "event: b") {
		t.Fatalf("did not expect replayed event b when last id is 3, got body=%q", body)
	}
}

func TestUpdateModel(t *testing.T) {
	s := testServer(t)
	r := httptest.NewRequest(http.MethodPost, "/v1/sessions", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.handleCreateSession(w, r)
	var view SessionView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}

	// Valid model update
	body := `{"model":"qwen3"}`
	w = httptest.NewRecorder()
	s.handleUpdateModel(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)), view.SessionID)
	if w.Code != http.StatusOK {
		t.Fatalf("update model status=%d body=%s", w.Code, w.Body.String())
	}
	var res map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res["model"] != "qwen3" {
		t.Fatalf("expected model qwen3, got %v", res["model"])
	}

	// Empty model
	w = httptest.NewRecorder()
	s.handleUpdateModel(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"model":""}`)), view.SessionID)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty model status=%d", w.Code)
	}

	// Missing session
	w = httptest.NewRecorder()
	s.handleUpdateModel(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"model":"qwen3"}`)), "bad-id")
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing session status=%d", w.Code)
	}

	// Active run
	sess, _ := s.registry.get(view.SessionID)
	sess.mu.Lock()
	sess.running = true
	sess.mu.Unlock()
	w = httptest.NewRecorder()
	s.handleUpdateModel(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"model":"qwen3"}`)), view.SessionID)
	if w.Code != http.StatusConflict {
		t.Fatalf("active run status=%d", w.Code)
	}
}

func TestUpdateModelWithModelRuntime(t *testing.T) {
	local := &testLLMClient{models: []llm.ModelInfo{{Name: "qwen3"}}}
	cloudCatalog := &testLLMClient{models: []llm.ModelInfo{{Name: "kimi-k2.6"}}}
	cloudRuntime := &testLLMClient{models: []llm.ModelInfo{{Name: "kimi-k2.6"}}}
	runtime := llm.NewRuntimeClient(local, llm.ProviderOllamaLocal, "http://localhost:11434")

	newServer := func() *Server {
		s := testServer(t)
		s.modelRuntime = &modelruntime.Service{
			LocalClient:  local,
			LocalBaseURL: "http://localhost:11434",
			Runtime:      runtime,
			Resolver: &modelresolver.Resolver{
				LocalClient:  local,
				CloudClient:  cloudCatalog,
				CloudEnabled: true,
			},
			Creds: &credentials.Resolver{Store: &testCredStore{v: "test-key"}},
			NewCloudClient: func(baseURL, apiKey string) llm.Client {
				return cloudRuntime
			},
		}
		return s
	}

	t.Run("accepts local model", func(t *testing.T) {
		s := newServer()
		r := httptest.NewRequest(http.MethodPost, "/v1/sessions", nil)
		r.RemoteAddr = "127.0.0.1:1234"
		w := httptest.NewRecorder()
		s.handleCreateSession(w, r)
		var view SessionView
		if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}

		w = httptest.NewRecorder()
		s.handleUpdateModel(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"model":"qwen3"}`)), view.SessionID)
		if w.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"model":"qwen3"`) {
			t.Fatalf("body=%s", w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"provider":"`+string(llm.ProviderOllamaLocal)+`"`) {
			t.Fatalf("body=%s", w.Body.String())
		}
	})

	t.Run("accepts listed cloud alias", func(t *testing.T) {
		s := newServer()
		r := httptest.NewRequest(http.MethodPost, "/v1/sessions", nil)
		r.RemoteAddr = "127.0.0.1:1234"
		w := httptest.NewRecorder()
		s.handleCreateSession(w, r)
		var view SessionView
		if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}

		w = httptest.NewRecorder()
		s.handleUpdateModel(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"model":"kimi-k2.6:cloud"}`)), view.SessionID)
		if w.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"model":"kimi-k2.6"`) {
			t.Fatalf("body=%s", w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"provider":"`+string(llm.ProviderOllamaCloudAPI)+`"`) {
			t.Fatalf("body=%s", w.Body.String())
		}
	})

	t.Run("rejects unknown model", func(t *testing.T) {
		s := newServer()
		r := httptest.NewRequest(http.MethodPost, "/v1/sessions", nil)
		r.RemoteAddr = "127.0.0.1:1234"
		w := httptest.NewRecorder()
		s.handleCreateSession(w, r)
		var view SessionView
		if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}

		w = httptest.NewRecorder()
		s.handleUpdateModel(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"model":"missing-model"}`)), view.SessionID)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "model not found") {
			t.Fatalf("body=%s", w.Body.String())
		}
	})
}

func TestGetTree(t *testing.T) {
	s := testServer(t)
	r := httptest.NewRequest(http.MethodPost, "/v1/sessions", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.handleCreateSession(w, r)
	var view SessionView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}

	sess, _ := s.registry.get(view.SessionID)
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "sub"), 0o755)
	os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("hello"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "sub", "nested.txt"), []byte("world"), 0o644)
	os.MkdirAll(filepath.Join(tmpDir, ".git"), 0o755)
	os.WriteFile(filepath.Join(tmpDir, ".git", "config"), []byte(""), 0o644)

	sess.mu.Lock()
	sess.appState.ToolSettings.WorkingDir = tmpDir
	sess.mu.Unlock()

	// Valid tree
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+view.SessionID+"/tree", nil)
	w = httptest.NewRecorder()
	s.handleGetTree(w, req, view.SessionID)
	if w.Code != http.StatusOK {
		t.Fatalf("tree status=%d body=%s", w.Code, w.Body.String())
	}
	var tree TreeResponse
	if err := json.Unmarshal(w.Body.Bytes(), &tree); err != nil {
		t.Fatal(err)
	}
	if tree.Root != tmpDir {
		t.Fatalf("expected root %s, got %s", tmpDir, tree.Root)
	}
	hasFile := false
	hasSub := false
	hasGit := false
	for _, e := range tree.Entries {
		if e.Name == "file.txt" {
			hasFile = true
		}
		if e.Name == "sub" {
			hasSub = true
		}
		if e.Name == ".git" {
			hasGit = true
		}
	}
	if !hasFile || !hasSub {
		t.Fatalf("expected file.txt and sub in entries, got %d entries", len(tree.Entries))
	}
	if hasGit {
		t.Fatalf(".git should be excluded")
	}

	// Path traversal
	req = httptest.NewRequest(http.MethodGet, "/v1/sessions/"+view.SessionID+"/tree?path=..", nil)
	w = httptest.NewRecorder()
	s.handleGetTree(w, req, view.SessionID)
	if w.Code != http.StatusForbidden && w.Code != http.StatusBadRequest {
		t.Fatalf("traversal status=%d", w.Code)
	}

	// Deep depth beyond max is capped
	deepDir := filepath.Join(tmpDir, "a", "b", "c", "d")
	os.MkdirAll(deepDir, 0o755)
	os.WriteFile(filepath.Join(deepDir, "deep.txt"), []byte("deep"), 0o644)
	req = httptest.NewRequest(http.MethodGet, "/v1/sessions/"+view.SessionID+"/tree?depth=10", nil)
	w = httptest.NewRecorder()
	s.handleGetTree(w, req, view.SessionID)
	if w.Code != http.StatusOK {
		t.Fatalf("deep depth status=%d body=%s", w.Code, w.Body.String())
	}
	var tree2 TreeResponse
	if err := json.Unmarshal(w.Body.Bytes(), &tree2); err != nil {
		t.Fatal(err)
	}
	hasDeep := false
	for _, e := range tree2.Entries {
		if e.Name == "deep.txt" {
			hasDeep = true
		}
	}
	if hasDeep {
		t.Fatalf("deep.txt should be excluded when depth is capped at 4")
	}
}
