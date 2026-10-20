package server

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestNewSessionAtCapEvictsOldestIdle(t *testing.T) {
	t.Setenv("NANDOCODEGO_DATA_HOME", t.TempDir())
	s := testServer(t)
	s.limiter = NewRateLimiter(1000, 3)
	base := time.Now().Add(-time.Hour)
	running := addTestSession(t, s, "running", base)
	running.mu.Lock()
	running.running = true
	running.mu.Unlock()
	addTestSession(t, s, "old-idle", base.Add(time.Minute))
	addTestSession(t, s, "new-idle", base.Add(2*time.Minute))
	code, id := newSessionCall(t, s, "")
	if code != http.StatusCreated || id == "" {
		t.Fatalf("new session status=%d id=%q", code, id)
	}
	if _, ok := s.registry.get("running"); !ok {
		t.Fatal("evicted active session")
	}
	if _, ok := s.registry.get("old-idle"); ok {
		t.Fatal("oldest idle session survived")
	}
	if _, ok := s.registry.get("new-idle"); !ok {
		t.Fatal("newer idle session was evicted")
	}
	if len(s.registry.all()) != 3 {
		t.Fatal("registry exceeded cap")
	}
}

func TestEvictionSkipsSessionsWithSubscribers(t *testing.T) {
	t.Setenv("NANDOCODEGO_DATA_HOME", t.TempDir())
	s := testServer(t)
	s.limiter = NewRateLimiter(1000, 1)
	sub := addTestSession(t, s, "watched", time.Now().Add(-time.Hour))
	ch := sub.Subscribe()
	defer sub.Unsubscribe(ch)
	if code, _ := newSessionCall(t, s, ""); code != http.StatusTooManyRequests {
		t.Fatalf("status=%d, want 429", code)
	}
	if _, ok := s.registry.get("watched"); !ok {
		t.Fatal("evicted session with subscriber")
	}
}

func TestEvictionPicksLeastRecentlyActive(t *testing.T) {
	t.Setenv("NANDOCODEGO_DATA_HOME", t.TempDir())
	s := testServer(t)
	s.limiter = NewRateLimiter(1000, 2)
	base := time.Now().Add(-time.Hour)
	old := addTestSession(t, s, "old-but-active", base)
	addTestSession(t, s, "new-but-stale", base.Add(time.Minute))
	old.mu.Lock()
	old.lastActive = base.Add(30 * time.Minute)
	old.mu.Unlock()
	if code, _ := newSessionCall(t, s, ""); code != http.StatusCreated {
		t.Fatalf("status=%d", code)
	}
	if _, ok := s.registry.get("old-but-active"); !ok {
		t.Fatal("evicted the recently active session")
	}
	if _, ok := s.registry.get("new-but-stale"); ok {
		t.Fatal("stale session survived")
	}
}

func TestEvictionBreaksLastActiveTieByID(t *testing.T) {
	t.Setenv("NANDOCODEGO_DATA_HOME", t.TempDir())
	s := testServer(t)
	s.limiter = NewRateLimiter(1000, 2)
	lastActive := time.Now().Add(-time.Hour)
	addTestSession(t, s, "z-session", lastActive)
	addTestSession(t, s, "a-session", lastActive)
	if code, _ := newSessionCall(t, s, ""); code != http.StatusCreated {
		t.Fatalf("status=%d", code)
	}
	if _, ok := s.registry.get("a-session"); ok {
		t.Fatal("tie did not evict lowest ID")
	}
	if _, ok := s.registry.get("z-session"); !ok {
		t.Fatal("tie evicted higher ID")
	}
}

func TestEvictRacingDeleteAndSweepReleasesSlotOnce(t *testing.T) {
	t.Setenv("NANDOCODEGO_DATA_HOME", t.TempDir())
	s := testServer(t)
	s.limiter = NewRateLimiter(1000, 2)
	base := time.Now().Add(-time.Hour)
	victim := addTestSession(t, s, "victim", base)
	watchCtx, cancelWatch := context.WithCancel(context.Background())
	victim.mu.Lock()
	victim.taskWatchStop = cancelWatch
	victim.mu.Unlock()
	keeper := addTestSession(t, s, "keeper", base.Add(time.Minute))
	keeper.mu.Lock()
	keeper.running = true
	keeper.mu.Unlock()
	var wg sync.WaitGroup
	var evictOK bool
	wg.Add(3)
	go func() { defer wg.Done(); evictOK = s.acquireSessionSlot() }()
	go func() {
		defer wg.Done()
		routeDo(t, s, http.MethodDelete, "/v1/sessions/victim")
	}()
	go func() { // body of sweepIdle for one session
		defer wg.Done()
		if sess, ok := s.registry.get("victim"); ok {
			sess.stop()
			if s.registry.remove("victim") {
				s.limiter.ReleaseSession()
			}
		}
	}()
	wg.Wait()
	// Slots: keeper (1) plus the evictor's slot if it got one.
	want := 1
	if evictOK {
		want = 2
	}
	s.limiter.mu.Lock()
	got := s.limiter.sessions
	s.limiter.mu.Unlock()
	if got != want {
		t.Fatalf("limiter sessions=%d want %d (evictOK=%v)", got, want, evictOK)
	}
	select {
	case <-victim.ctx.Done():
	default:
		t.Fatal("victim context survived removal")
	}
	select {
	case <-watchCtx.Done():
	default:
		t.Fatal("victim task watcher survived removal")
	}
}
