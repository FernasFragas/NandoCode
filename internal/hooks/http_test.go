package hooks

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestLoadSnapshotDisablesUnsafeHTTPHook(t *testing.T) {
	unsafeURL := joinURL("http", "10.0.0.1", "/hook")
	path := writeHookConfig(t, `{"hooks":[{"kind":"http","event":"PreToolUse","url":"`+unsafeURL+`"}]}`)
	snap := LoadSnapshot(LoadOptions{UserPath: path})
	if len(snap.Hooks) != 0 {
		t.Fatalf("expected no executable hooks, got %d", len(snap.Hooks))
	}
	if len(snap.Disabled) != 1 {
		t.Fatalf("expected one disabled hook, got %d", len(snap.Disabled))
	}
	if !strings.Contains(snap.Disabled[0].Reason, "destination rejected") {
		t.Fatalf("unexpected disable reason: %q", snap.Disabled[0].Reason)
	}
	if len(snap.Warnings) == 0 {
		t.Fatalf("expected startup warning for disabled unsafe hook")
	}
}

func TestLoadSnapshotAllowsLocalhostHTTPHook(t *testing.T) {
	localURL := joinURL("http", "localhost:8080", "/hook")
	path := writeHookConfig(t, `{"hooks":[{"kind":"http","event":"PreToolUse","url":"`+localURL+`"}]}`)
	snap := LoadSnapshot(LoadOptions{UserPath: path})
	if len(snap.Disabled) != 0 {
		t.Fatalf("expected no disabled hooks, got %d", len(snap.Disabled))
	}
	if len(snap.Hooks) != 1 {
		t.Fatalf("expected one executable hook, got %d", len(snap.Hooks))
	}
}

func TestRunHTTPHookReturnsDecision(t *testing.T) {
	baseURL := startIPv4HTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"decision": "allow", "reason": "ok"})
	}))

	res := runHTTPHook(context.Background(), Hook{
		Kind:  KindHTTP,
		Event: EventPreToolUse,
		URL:   baseURL,
	}, Envelope{Event: EventPreToolUse}, DefaultConfig())
	if res.Decision != DecisionAllow {
		t.Fatalf("expected allow decision, got %q", res.Decision)
	}
}

func TestRunHTTPHookTimeoutReturnsWarning(t *testing.T) {
	baseURL := startIPv4HTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]any{"decision": "allow"})
	}))

	cfg := DefaultConfig()
	cfg.DefaultTimeout = 20 * time.Millisecond
	res := runHTTPHook(context.Background(), Hook{
		Kind:  KindHTTP,
		Event: EventPreToolUse,
		URL:   baseURL,
	}, Envelope{Event: EventPreToolUse}, cfg)
	if strings.TrimSpace(res.Warning) == "" {
		t.Fatalf("expected timeout warning, got %#v", res)
	}
	if res.Decision != DecisionNone {
		t.Fatalf("expected no decision on timeout, got %q", res.Decision)
	}
}

func startIPv4HTTPServer(t *testing.T, h http.Handler) string {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("loopback listener unavailable in this environment: %v", err)
	}
	srv := &http.Server{Handler: h}
	go func() {
		_ = srv.Serve(ln)
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	return joinURL("http", ln.Addr().String(), "")
}

func joinURL(scheme, host, path string) string {
	return scheme + "://" + host + path
}
