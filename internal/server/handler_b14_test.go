package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/credentials"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/llm/modelresolver"
	"github.com/FernasFragas/Nandocode/internal/llm/modelruntime"
)

// countingLLMClient wraps testLLMClient and counts ListModels calls.
type countingLLMClient struct {
	testLLMClient
	mu    sync.Mutex
	lists int
}

func (c *countingLLMClient) ListModels(ctx context.Context) ([]llm.ModelInfo, error) {
	c.mu.Lock()
	c.lists++
	c.mu.Unlock()
	return c.testLLMClient.ListModels(ctx)
}

func (c *countingLLMClient) listCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lists
}

func createTestSession(t *testing.T, s *Server) string {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/v1/sessions", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.handleCreateSession(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create session status=%d body=%s", w.Code, w.Body.String())
	}
	var view SessionView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	return view.SessionID
}

func TestUpdateModelReportsRetiredCloudStub(t *testing.T) {
	local := &testLLMClient{models: []llm.ModelInfo{{Name: "qwen3"}, {Name: "glm-5.1:cloud"}}}
	cloudCatalog := &testLLMClient{models: []llm.ModelInfo{{Name: "glm-5.3"}}}
	s := testServer(t)
	s.modelRuntime = &modelruntime.Service{
		LocalClient:  local,
		LocalBaseURL: "http://localhost:11434",
		Runtime:      llm.NewRuntimeClient(local, llm.ProviderOllamaLocal, "http://localhost:11434"),
		Resolver:     &modelresolver.Resolver{LocalClient: local, CloudClient: cloudCatalog, CloudEnabled: true},
		Creds:        &credentials.Resolver{Store: &testCredStore{v: "test-key"}},
	}
	id := createTestSession(t, s)

	w := httptest.NewRecorder()
	s.handleUpdateModel(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"model":"glm-5.1:cloud"}`)), id)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "model glm-5.1 is no longer available in Ollama Cloud") {
		t.Fatalf("body=%q want a precise retired-model reason", body)
	}
	if strings.Contains(body, "model not found") {
		t.Fatalf("body=%q must not say 'model not found' for a listed model", body)
	}
}

// sessionProviderRunner reports which provider its session's runtime client
// is routed to at the moment of each run.
type sessionProviderRunner struct {
	rc  *llm.RuntimeClient
	out chan llm.Provider
}

func (r sessionProviderRunner) Run(_ context.Context, _ agent.Input) <-chan agent.Event {
	r.out <- r.rc.Snapshot().Provider
	ch := make(chan agent.Event, 1)
	ch <- agent.Terminal{Reason: agent.TerminalCompleted}
	close(ch)
	return ch
}

func TestSessionsKeepIndependentModelRuntimes(t *testing.T) {
	local := &countingLLMClient{testLLMClient: testLLMClient{models: []llm.ModelInfo{{Name: "qwen3"}}}}
	cloudCatalog := &testLLMClient{models: []llm.ModelInfo{{Name: "kimi-k2.6"}}}
	cloudRuntime := &testLLMClient{models: []llm.ModelInfo{{Name: "kimi-k2.6"}}}
	shared := llm.NewRuntimeClient(local, llm.ProviderOllamaLocal, "http://localhost:11434")

	s := testServer(t)
	s.modelRuntime = &modelruntime.Service{
		LocalClient:    local,
		LocalBaseURL:   "http://localhost:11434",
		Runtime:        shared,
		Resolver:       &modelresolver.Resolver{LocalClient: local, CloudClient: cloudCatalog, CloudEnabled: true},
		Creds:          &credentials.Resolver{Store: &testCredStore{v: "test-key"}},
		NewCloudClient: func(string, string) llm.Client { return cloudRuntime },
	}
	providers := map[*llm.RuntimeClient]chan llm.Provider{}
	var pmu sync.Mutex
	s.sessionRuntimes = func(rc *llm.RuntimeClient) (llm.Client, AgentRunner, error) {
		pmu.Lock()
		defer pmu.Unlock()
		ch := make(chan llm.Provider, 4)
		providers[rc] = ch
		return rc, sessionProviderRunner{rc: rc, out: ch}, nil
	}

	idA := createTestSession(t, s)
	idB := createTestSession(t, s)

	w := httptest.NewRecorder()
	s.handleUpdateModel(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"model":"kimi-k2.6:cloud"}`)), idA)
	if w.Code != http.StatusOK {
		t.Fatalf("switch A status=%d body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	s.handleUpdateModel(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"model":"qwen3"}`)), idB)
	if w.Code != http.StatusOK {
		t.Fatalf("switch B status=%d body=%s", w.Code, w.Body.String())
	}

	post := func(id string) {
		t.Helper()
		w := httptest.NewRecorder()
		s.handlePostMessage(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"prompt":"hi"}`)), id)
		if w.Code != http.StatusAccepted {
			t.Fatalf("post %s status=%d body=%s", id, w.Code, w.Body.String())
		}
	}
	waitProvider := func(id string) llm.Provider {
		t.Helper()
		sess, _ := s.registry.get(id)
		var ch chan llm.Provider
		pmu.Lock()
		for rc, c := range providers {
			if sess.runtime == rc {
				ch = c
			}
		}
		pmu.Unlock()
		if ch == nil {
			t.Fatalf("session %s has no per-session runtime client", id)
		}
		select {
		case p := <-ch:
			return p
		case <-time.After(5 * time.Second):
			t.Fatalf("session %s run never started", id)
			return ""
		}
	}
	waitIdle := func(id string) {
		sess, _ := s.registry.get(id)
		eventually(t, "run to finish", func() bool {
			sess.mu.Lock()
			defer sess.mu.Unlock()
			return !sess.running
		})
	}

	// B runs first (local), then A (cloud). With one shared client, B's switch
	// back to local would reroute A, or A's cloud switch would reroute B.
	post(idB)
	if got := waitProvider(idB); got != llm.ProviderOllamaLocal {
		t.Fatalf("session B provider=%s want local", got)
	}
	waitIdle(idB)
	post(idA)
	if got := waitProvider(idA); got != llm.ProviderOllamaCloudAPI {
		t.Fatalf("session A provider=%s want cloud", got)
	}
	waitIdle(idA)
	if got := shared.Snapshot().Provider; got != llm.ProviderOllamaLocal {
		t.Fatalf("shared base runtime was switched to %s; sessions must not touch it", got)
	}

	// A second message on B with an unchanged model must not re-resolve it.
	before := local.listCalls()
	post(idB)
	_ = waitProvider(idB)
	waitIdle(idB)
	if after := local.listCalls(); after != before {
		t.Fatalf("unchanged model re-resolved: ListModels calls %d -> %d", before, after)
	}
}
