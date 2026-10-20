package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/llm"
)

type listedSession struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	CreatedAt  time.Time `json:"created_at"`
	LastActive time.Time `json:"last_active"`
	Running    bool      `json:"running"`
	Model      string    `json:"model"`
}

func addTestSession(t *testing.T, s *Server, id string, created time.Time) *Session {
	t.Helper()
	if !s.limiter.AcquireSession() {
		t.Fatal("limiter refused test session")
	}
	sess := newSession(s.ctx, id, s.baseApp.Clone(), tinyRunner{})
	sess.mu.Lock()
	sess.createdAt = created
	sess.lastActive = created
	sess.mu.Unlock()
	s.registry.set(sess)
	return sess
}

func listSessions(t *testing.T, s *Server) []listedSession {
	t.Helper()
	w := routeDo(t, s, http.MethodGet, "/v1/sessions")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /v1/sessions status=%d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Sessions []listedSession `json:"sessions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Sessions
}

func TestListSessionsNewestFirstWithFields(t *testing.T) {
	s := testServer(t)
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	addTestSession(t, s, "old", base)
	mid := addTestSession(t, s, "mid", base.Add(time.Hour))
	addTestSession(t, s, "new", base.Add(2*time.Hour))
	mid.mu.Lock()
	mid.running = true
	mid.appState.ActiveModel = "qwen3"
	mid.mu.Unlock()

	got := listSessions(t, s)
	if len(got) != 3 || got[0].ID != "new" || got[1].ID != "mid" || got[2].ID != "old" {
		t.Fatalf("order = %+v, want new, mid, old", got)
	}
	if !got[1].Running || got[0].Running {
		t.Fatalf("running flags wrong: %+v", got)
	}
	if got[1].Model != "qwen3" {
		t.Fatalf("model = %q", got[1].Model)
	}
	if got[1].CreatedAt.IsZero() || got[1].LastActive.IsZero() {
		t.Fatalf("times missing: %+v", got[1])
	}
}

func TestListSessionsEmptyIsArrayNotNull(t *testing.T) {
	s := testServer(t)
	w := routeDo(t, s, http.MethodGet, "/v1/sessions")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"sessions":[]`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestListSessionsKeepsCreate(t *testing.T) {
	s := testServer(t)
	w := routeDo(t, s, http.MethodPost, "/v1/sessions")
	if w.Code != http.StatusCreated {
		t.Fatalf("POST status=%d", w.Code)
	}
}

func newSessionCall(t *testing.T, s *Server, query string) (int, string) {
	t.Helper()
	w := routeDo(t, s, http.MethodPost, "/v1/sessions"+query)
	if w.Code >= 300 {
		return w.Code, ""
	}
	var v SessionView
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	return w.Code, v.SessionID
}

func TestNewSessionDoesNotReachCapThroughOrphans(t *testing.T) {
	s := testServer(t)
	s.limiter = NewRateLimiter(1000, 2)
	first, id := newSessionCall(t, s, "?reuse=blank")
	if first != http.StatusCreated {
		t.Fatalf("first create status=%d", first)
	}
	// Repeated New Session with no one attached and nothing typed must
	// hand back the same blank session instead of leaking a new one.
	for i := 0; i < 5; i++ {
		code, got := newSessionCall(t, s, "?reuse=blank")
		if code != http.StatusOK || got != id {
			t.Fatalf("call %d: status=%d id=%q, want 200 and %q", i, code, got, id)
		}
	}
	if n := len(s.registry.all()); n != 1 {
		t.Fatalf("registry has %d sessions, want 1", n)
	}
}

func TestNewSessionReuseSkipsUsedAttachedAndRunning(t *testing.T) {
	s := testServer(t)
	used := addTestSession(t, s, "used", time.Now().Add(-3*time.Hour))
	used.mu.Lock()
	used.conversation = []llm.Message{{Role: llm.RoleUser, Content: "hi"}}
	used.mu.Unlock()
	attached := addTestSession(t, s, "attached", time.Now().Add(-2*time.Hour))
	ch := attached.Subscribe()
	defer attached.Unsubscribe(ch)
	running := addTestSession(t, s, "running", time.Now().Add(-time.Hour))
	running.mu.Lock()
	running.running = true
	running.mu.Unlock()

	code, id := newSessionCall(t, s, "?reuse=blank")
	if code != http.StatusCreated || id == "used" || id == "attached" || id == "running" {
		t.Fatalf("status=%d id=%q, want a fresh session", code, id)
	}
}

func TestNewSessionWithoutReuseKeepsCreating(t *testing.T) {
	s := testServer(t)
	_, a := newSessionCall(t, s, "")
	code, b := newSessionCall(t, s, "")
	if code != http.StatusCreated || a == b {
		t.Fatalf("status=%d a=%q b=%q, want two distinct sessions", code, a, b)
	}
}

func TestDeleteSessionReleasesSlotOnce(t *testing.T) {
	s := testServer(t)
	s.limiter = NewRateLimiter(1000, 1)
	addTestSession(t, s, "a", time.Now())
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		s.handleDeleteSession(w, httptest.NewRequest(http.MethodDelete, "/", nil), "a")
		_ = w
	}
	if !s.limiter.AcquireSession() {
		t.Fatal("slot not released after delete")
	}
	if s.limiter.AcquireSession() {
		t.Fatal("second delete released a slot it did not own")
	}
}

func TestBlankOrphanCheckIsRaceFreeWithEmit(t *testing.T) {
	s := testServer(t)
	sess := addTestSession(t, s, "race", time.Now())
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			sess.Emit("probe", nil)
		}
	}()
	for i := 0; i < 200; i++ {
		_ = sess.isBlankOrphan()
		_, _ = s.registry.blankOrphan()
	}
	<-done
}
