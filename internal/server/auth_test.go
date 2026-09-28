package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAuthMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := NewAuthMiddleware("secret", next)

	cases := []struct {
		name, path, auth string
		want             int
	}{
		{"api without token", "/v1/health", "", http.StatusUnauthorized},
		{"api with wrong token", "/v1/sessions", "Bearer secreT", http.StatusUnauthorized},
		{"api with token prefix only", "/v1/sessions", "Bearer secretextra", http.StatusUnauthorized},
		{"api with token", "/v1/health", "Bearer secret", http.StatusNoContent},
		{"static ui is public", "/", "", http.StatusNoContent},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		if tc.auth != "" {
			req.Header.Set("Authorization", tc.auth)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != tc.want {
			t.Errorf("%s: status=%d want=%d", tc.name, rr.Code, tc.want)
		}
	}
}

func TestRequestGuard(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	loopback := NewRequestGuard("127.0.0.1", next)
	public := NewRequestGuard("0.0.0.0", next)

	cases := []struct {
		name        string
		h           http.Handler
		method      string
		path        string
		host        string
		origin      string
		contentType string
		body        string
		want        int
	}{
		{"same-origin json post", loopback, http.MethodPost, "/v1/sessions", "127.0.0.1:8080", httpURL("127.0.0.1:8080", ""), "application/json", "{}", http.StatusNoContent},
		{"localhost host", loopback, http.MethodGet, "/v1/health", "localhost:8080", "", "", "", http.StatusNoContent},
		{"ipv6 loopback host", loopback, http.MethodGet, "/v1/health", "[::1]:8080", "", "", "", http.StatusNoContent},
		{"cli post without body", loopback, http.MethodPost, "/v1/sessions", "127.0.0.1:8080", "", "", "", http.StatusNoContent},
		{"json with charset", loopback, http.MethodPost, "/v1/sessions", "127.0.0.1:8080", "", "application/json; charset=utf-8", "{}", http.StatusNoContent},
		{"dns rebinding host", loopback, http.MethodGet, "/v1/health", "attacker.example:8080", "", "", "", http.StatusForbidden},
		{"rebinding host on ui", loopback, http.MethodGet, "/", "attacker.example", "", "", "", http.StatusForbidden},
		{"cross-site origin", loopback, http.MethodPost, "/v1/sessions", "127.0.0.1:8080", httpURL("attacker.example", ""), "application/json", "{}", http.StatusForbidden},
		{"other local port origin", loopback, http.MethodPost, "/v1/sessions", "127.0.0.1:8080", httpURL("127.0.0.1:3000", ""), "application/json", "{}", http.StatusForbidden},
		{"null origin", loopback, http.MethodPost, "/v1/sessions", "127.0.0.1:8080", "null", "application/json", "{}", http.StatusForbidden},
		{"text/plain body", loopback, http.MethodPost, "/v1/sessions/x/messages", "127.0.0.1:8080", "", "text/plain", `{"prompt":"hi"}`, http.StatusUnsupportedMediaType},
		{"form body", loopback, http.MethodPost, "/v1/sessions", "127.0.0.1:8080", "", "application/x-www-form-urlencoded", "a=b", http.StatusUnsupportedMediaType},
		{"public bind skips host check", public, http.MethodGet, "/v1/health", "nandocode.lan:8080", "", "", "", http.StatusNoContent},
		{"public bind still checks origin", public, http.MethodPost, "/v1/sessions", "nandocode.lan:8080", httpURL("attacker.example", ""), "application/json", "{}", http.StatusForbidden},
	}
	for _, tc := range cases {
		var body *bytes.Reader
		if tc.body != "" {
			body = bytes.NewReader([]byte(tc.body))
		}
		var req *http.Request
		if body != nil {
			req = httptest.NewRequest(tc.method, tc.path, body)
		} else {
			req = httptest.NewRequest(tc.method, tc.path, nil)
		}
		req.Host = tc.host
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		if tc.contentType != "" {
			req.Header.Set("Content-Type", tc.contentType)
		}
		rr := httptest.NewRecorder()
		tc.h.ServeHTTP(rr, req)
		if rr.Code != tc.want {
			t.Errorf("%s: status=%d want=%d (%s)", tc.name, rr.Code, tc.want, strings.TrimSpace(rr.Body.String()))
		}
	}
}

func TestNewSessionIDIsRandom(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id, err := newSessionID()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(id, "sess_") || len(id) != len("sess_")+32 {
			t.Fatalf("id = %q, want sess_ + 32 hex chars", id)
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

func TestAnnounce(t *testing.T) {
	var out bytes.Buffer
	announce(&out, Config{Bind: "127.0.0.1", Port: 8080, Token: "abc123", tokenGenerated: true})
	if !strings.Contains(out.String(), httpURL("127.0.0.1:8080", "/")+"#token=abc123") {
		t.Fatalf("generated-token announcement = %q", out.String())
	}

	out.Reset()
	announce(&out, Config{Bind: "0.0.0.0", Port: 9000, Token: "user-secret"})
	if strings.Contains(out.String(), "user-secret") || !strings.Contains(out.String(), httpURL("localhost:9000", "/")) {
		t.Fatalf("user-token announcement = %q, must not echo the token", out.String())
	}
}

// httpURL builds an http URL for Origin headers and expected output.
func httpURL(host, path string) string {
	return (&url.URL{Scheme: "http", Host: host, Path: path}).String()
}
