package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

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
func (t *testCredStore) Set(_, _, s string) error        { t.v = s; return nil }
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

	// Wait for the first run to finish instead of sleeping.
	if sess, ok := s.registry.get(view.SessionID); ok {
		eventually(t, "first run to finish", func() bool {
			sess.mu.Lock()
			defer sess.mu.Unlock()
			return !sess.running
		})
	}
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

func TestModelsIncludesCloudOnlyCatalogEntries(t *testing.T) {
	s := testServer(t)
	local := &testLLMClient{models: []llm.ModelInfo{{Name: "qwen3"}, {Name: "shared"}}}
	cloud := &testLLMClient{models: []llm.ModelInfo{{Name: "kimi-k2.6"}, {Name: "shared"}}}
	s.modelRuntime = &modelruntime.Service{
		LocalClient: local,
		Resolver: &modelresolver.Resolver{
			LocalClient: local, CloudClient: cloud, CloudEnabled: true,
		},
	}
	w := httptest.NewRecorder()
	s.handleModels(w, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var response modelsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, model := range response.Models {
		names = append(names, model.Name)
	}
	if got, want := strings.Join(names, ","), "qwen3,shared,kimi-k2.6"; got != want {
		t.Fatalf("models=%q, want %q", got, want)
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
		`id="header"`,
		`id="transcript"`,
		`id="message-input"`,
		`href="css/tokens.css"`,
		`href="css/components.css"`,
		`href="css/app.css"`,
		`<script type="module" src="js/app.js"></script>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("served UI missing %q", want)
		}
	}
	// BV-2: no inline style and no inline script (the CSP forbids both).
	if strings.Contains(body, `style="`) {
		t.Fatalf("served UI still has an inline style attribute")
	}
	if strings.Contains(body, "<style") {
		t.Fatalf("served UI still has a <style> block")
	}
	scriptTag := regexp.MustCompile(`(?is)<script\b([^>]*)>(.*?)</script>`)
	for _, m := range scriptTag.FindAllStringSubmatch(body, -1) {
		if !strings.Contains(m[1], "src=") || strings.TrimSpace(m[2]) != "" {
			t.Fatalf("served UI has an inline script: %q", m[0])
		}
	}
	if regexp.MustCompile(`(?i)\son[a-z]+\s*=`).MatchString(body) {
		t.Fatalf("served UI has an inline event handler attribute")
	}
	for _, inlined := range []string{"function renderMarkdown(", "function escapeHtml(", "currentReader = r.body.getReader()"} {
		if strings.Contains(body, inlined) {
			t.Fatalf("served UI still inlines %q", inlined)
		}
	}
	csp := w.Header().Get("Content-Security-Policy")
	if strings.Contains(csp, "unsafe-inline") {
		t.Fatalf("CSP still allows unsafe-inline: %q", csp)
	}

	// Every asset the page references (and the module graph app.js pulls in)
	// must be served with the right content type.
	assets := regexp.MustCompile(`(?:href|src)="([^"#]+)"`).FindAllStringSubmatch(body, -1)
	if len(assets) < 4 {
		t.Fatalf("expected the page to reference assets, got %v", assets)
	}
	for _, m := range assets {
		path := "/" + m[1]
		aw := httptest.NewRecorder()
		s.routes().ServeHTTP(aw, httptest.NewRequest(http.MethodGet, path, nil))
		if aw.Code != http.StatusOK {
			t.Fatalf("%s status=%d", path, aw.Code)
		}
		ct := aw.Header().Get("Content-Type")
		switch {
		case strings.HasSuffix(path, ".js") && !strings.Contains(ct, "javascript"):
			t.Fatalf("%s content type=%q, want JavaScript", path, ct)
		case strings.HasSuffix(path, ".css") && !strings.Contains(ct, "text/css"):
			t.Fatalf("%s content type=%q, want text/css", path, ct)
		}
	}
	for _, mod := range []string{"/js/app.js", "/js/sse.js", "/js/token.js", "/js/layout.js", "/js/markdown.js", "/js/features/transcript.js"} {
		mw := httptest.NewRecorder()
		s.routes().ServeHTTP(mw, httptest.NewRequest(http.MethodGet, mod, nil))
		if mw.Code != http.StatusOK {
			t.Fatalf("%s status=%d", mod, mw.Code)
		}
		if ct := mw.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
			t.Fatalf("%s content type=%q, want JavaScript", mod, ct)
		}
	}
	mw := httptest.NewRecorder()
	s.routes().ServeHTTP(mw, httptest.NewRequest(http.MethodGet, "/js/markdown.js", nil))
	if !strings.Contains(mw.Body.String(), "export function renderMarkdown") {
		t.Fatalf("markdown module body missing renderMarkdown export")
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

// lockedRecorder makes a recorder safe to read while a handler goroutine
// is still writing to it.
type lockedRecorder struct {
	*httptest.ResponseRecorder
	mu sync.Mutex
}

func (l *lockedRecorder) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.ResponseRecorder.Write(b)
}

func (l *lockedRecorder) WriteHeader(code int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ResponseRecorder.WriteHeader(code)
}

func (l *lockedRecorder) Flush() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ResponseRecorder.Flush()
}

func (l *lockedRecorder) bodyString() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.Body.String()
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
	rr := &lockedRecorder{ResponseRecorder: httptest.NewRecorder()}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.handleEvents(rr, req, view.SessionID)
	}()
	// Wait until the replay has been written instead of sleeping.
	eventually(t, "replayed event c", func() bool {
		return strings.Contains(rr.bodyString(), "event: c")
	})
	cancel()
	wg.Wait()
	body := rr.bodyString()
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

func TestUpdateModelRequiresCredentialForKeylessCloud(t *testing.T) {
	t.Setenv("OLLAMA_API_KEY", "")
	local := &testLLMClient{models: []llm.ModelInfo{{Name: "qwen3"}}}
	cloud := &testLLMClient{models: []llm.ModelInfo{{Name: "kimi-k2.6"}}}
	createdCloudClient := false
	s := testServer(t)
	s.modelRuntime = &modelruntime.Service{
		LocalClient: local, LocalBaseURL: "http://localhost:11434",
		Runtime:        llm.NewRuntimeClient(local, llm.ProviderOllamaLocal, "http://localhost:11434"),
		Resolver:       &modelresolver.Resolver{LocalClient: local, CloudClient: cloud, CloudEnabled: true},
		Creds:          &credentials.Resolver{Store: &testCredStore{}},
		NewCloudClient: func(string, string) llm.Client { createdCloudClient = true; return cloud },
	}
	id := createTestSession(t, s)
	w := httptest.NewRecorder()
	s.handleUpdateModel(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"model":"kimi-k2.6:cloud"}`)), id)
	if w.Code != http.StatusPreconditionRequired {
		t.Fatalf("status=%d want 428 body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "requires_credential" || body["credential"] != "OLLAMA_API_KEY" {
		t.Fatalf("credential response=%v", body)
	}
	if createdCloudClient {
		t.Fatal("cloud client created before credential")
	}
}
