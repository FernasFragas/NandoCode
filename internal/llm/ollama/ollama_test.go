package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/llm"
)

func TestNewClientListModelsNoAuthorizationHeader(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("expected no authorization header, got %q", got)
		}
		_, _ = io.WriteString(w, `{"models":[]}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	if _, err := c.ListModels(context.Background()); err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
}

func TestNewClientWithOptionsSendsAuthorizationOnAllEndpoints(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	seen := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.URL.Path] = r.Header.Get("Authorization")
		mu.Unlock()
		switch r.URL.Path {
		case "/api/chat":
			_, _ = io.WriteString(w, `{"model":"m","created_at":"2026-01-01T00:00:00Z","message":{"role":"assistant","content":"ok"},"done":true}`)
		case "/api/tags":
			_, _ = io.WriteString(w, `{"models":[]}`)
		case "/api/show":
			_, _ = io.WriteString(w, `{"parameters":"num_ctx 8192","details":{},"model_info":{"test.context_length":8192}}`)
		case "/api/embed":
			_, _ = io.WriteString(w, `{"embeddings":[[0.1,0.2]]}`)
		case "/api/pull":
			_, _ = io.WriteString(w, `{"status":"success"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClientWithOptions(Options{BaseURL: srv.URL, APIKey: "abc123"})
	ctx := context.Background()
	if _, err := c.Chat(ctx, &llm.ChatRequest{
		Model:    "m",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
		Stream:   true,
	}); err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if _, err := c.ListModels(ctx); err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if _, err := c.ShowModel(ctx, "m"); err != nil {
		t.Fatalf("ShowModel() error = %v", err)
	}
	if _, err := c.Embed(ctx, "m", []string{"x"}); err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	progress := make(chan llm.PullProgress, 4)
	if err := c.PullModel(ctx, "m", progress); err != nil {
		t.Fatalf("PullModel() error = %v", err)
	}

	for _, path := range []string{"/api/chat", "/api/tags", "/api/show", "/api/embed", "/api/pull"} {
		mu.Lock()
		got, ok := seen[path]
		mu.Unlock()
		if !ok {
			t.Fatalf("missing request for %s", path)
		}
		if got != "Bearer abc123" {
			t.Fatalf("%s authorization = %q", path, got)
		}
	}
}

func TestAuthErrorsDoNotLeakAPIKey(t *testing.T) {
	t.Parallel()
	apiKey := "super-secret-api-key"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "denied", http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewClientWithOptions(Options{BaseURL: srv.URL, APIKey: apiKey})
	_, err := c.ListModels(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), apiKey) {
		t.Fatalf("error leaked api key: %v", err)
	}
}

func TestChatStreamingStillDecodesEvents(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		for _, line := range []string{
			`{"model":"m","created_at":"2026-01-01T00:00:00Z","message":{"role":"assistant","content":"hello"},"done":false}`,
			`{"model":"m","created_at":"2026-01-01T00:00:01Z","message":{"role":"assistant","content":" world"},"done":true}`,
		} {
			_, _ = fmt.Fprintln(w, line)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	events, err := c.Chat(context.Background(), &llm.ChatRequest{
		Model:    "m",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
		Stream:   true,
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	var got []llm.StreamEvent
	for evt := range events {
		got = append(got, evt)
	}
	if len(got) != 2 {
		t.Fatalf("events=%d", len(got))
	}
	if got[0].Message.Content != "hello" || !got[1].Done {
		t.Fatalf("unexpected events: %+v", got)
	}
}

func TestEmbedUsesBatchAPIAndParsesEmbeddings(t *testing.T) {
	t.Parallel()
	type embedReq struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}

	var gotReq embedReq
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			t.Fatalf("path = %q, want /api/embed", r.URL.Path)
		}
		requests++
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"embeddings":[[1,2],[3,4]]}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	got, err := c.Embed(context.Background(), "qwen3-embedding:8b", []string{"one", "two"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}

	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
	if gotReq.Model != "qwen3-embedding:8b" {
		t.Fatalf("model = %q", gotReq.Model)
	}
	if len(gotReq.Input) != 2 || gotReq.Input[0] != "one" || gotReq.Input[1] != "two" {
		t.Fatalf("input = %#v", gotReq.Input)
	}
	if len(got) != 2 || len(got[0]) != 2 || len(got[1]) != 2 {
		t.Fatalf("embeddings shape = %dx%d and %d", len(got), len(got[0]), len(got[1]))
	}
	if got[0][0] != 1 || got[0][1] != 2 || got[1][0] != 3 || got[1][1] != 4 {
		t.Fatalf("embeddings = %#v", got)
	}
}

func TestEmbedWithOptionsIncludesOptionalFields(t *testing.T) {
	t.Parallel()

	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			t.Fatalf("path = %q, want /api/embed", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"embeddings":[[0.1,0.2,0.3]]}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	dimensions := 1024
	truncate := true
	_, err := c.EmbedWithOptions(context.Background(), "qwen3-embedding:8b", []string{"hello"}, &llm.EmbedOptions{
		Dimensions: &dimensions,
		Truncate:   &truncate,
		KeepAlive:  "5m",
		Options: map[string]any{
			"seed": float64(1),
		},
	})
	if err != nil {
		t.Fatalf("EmbedWithOptions() error = %v", err)
	}

	if got["dimensions"] != float64(dimensions) {
		t.Fatalf("dimensions = %#v", got["dimensions"])
	}
	if got["truncate"] != true {
		t.Fatalf("truncate = %#v", got["truncate"])
	}
	if got["keep_alive"] != "5m" {
		t.Fatalf("keep_alive = %#v", got["keep_alive"])
	}
	options, ok := got["options"].(map[string]any)
	if !ok {
		t.Fatalf("options type = %T", got["options"])
	}
	if options["seed"] != float64(1) {
		t.Fatalf("options.seed = %#v", options["seed"])
	}
}

func TestEmbedParsesLegacySingleEmbeddingField(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"embedding":[9.5,8.5]}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	got, err := c.Embed(context.Background(), "m", []string{"x"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(got) != 1 || len(got[0]) != 2 {
		t.Fatalf("embeddings shape = %#v", got)
	}
	if got[0][0] != 9.5 || got[0][1] != 8.5 {
		t.Fatalf("embeddings = %#v", got)
	}
}
