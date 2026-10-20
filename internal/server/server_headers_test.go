package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/boom" {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	h := securityHeaders(next)
	for _, tc := range []struct {
		name, path string
		wantCode   int
	}{
		{"static page", "/", http.StatusOK},
		{"api", "/v1/models", http.StatusOK},
		{"error response", "/boom", http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if w.Code != tc.wantCode {
				t.Fatalf("status=%d want %d", w.Code, tc.wantCode)
			}
			hdr := w.Header()
			csp := hdr.Get("Content-Security-Policy")
			for _, d := range []string{"default-src 'self'", "connect-src 'self'", "frame-ancestors 'none'", "base-uri 'self'", "form-action 'self'"} {
				if !strings.Contains(csp, d) {
					t.Errorf("CSP %q missing directive %q", csp, d)
				}
			}
			if strings.Contains(csp, "'unsafe-eval'") {
				t.Errorf("CSP must not allow unsafe-eval: %q", csp)
			}
			for k, want := range map[string]string{
				"X-Frame-Options":        "DENY",
				"X-Content-Type-Options": "nosniff",
				"Referrer-Policy":        "strict-origin-when-cross-origin",
			} {
				if got := hdr.Get(k); got != want {
					t.Errorf("%s=%q want %q", k, got, want)
				}
			}
			if pp := hdr.Get("Permissions-Policy"); !strings.Contains(pp, "camera=()") || !strings.Contains(pp, "microphone=()") || !strings.Contains(pp, "geolocation=()") {
				t.Errorf("Permissions-Policy=%q", pp)
			}
			isAPI := strings.HasPrefix(tc.path, "/v1/")
			cc := hdr.Get("Cache-Control")
			if isAPI && !strings.Contains(cc, "no-store") {
				t.Errorf("API response Cache-Control=%q want no-store", cc)
			}
			if !isAPI && strings.Contains(cc, "no-store") {
				t.Errorf("static response should stay cacheable, Cache-Control=%q", cc)
			}
		})
	}
}

func TestRoutesApplySecurityHeadersToRejectedRequests(t *testing.T) {
	s := testServer(t)
	s.cfg.Token = "tok"
	s.cfg.Bind = "127.0.0.1"
	h := s.routes()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	r.RemoteAddr = "127.0.0.1:1"
	r.Host = "127.0.0.1:8080"
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 without a token", w.Code)
	}
	if w.Header().Get("X-Frame-Options") != "DENY" || w.Header().Get("Content-Security-Policy") == "" {
		t.Fatalf("401 response is missing security headers: %v", w.Header())
	}
	if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("401 response is cacheable: %v", w.Header())
	}
}

func TestRoutesApplySecurityHeadersToGuardRejections(t *testing.T) {
	s := testServer(t)
	s.cfg.Token = "tok"
	s.cfg.Bind = "127.0.0.1"
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Host = "attacker.example"
	s.routes().ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403 for a rebinding host", w.Code)
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Content-Security-Policy") == "" {
		t.Fatalf("403 response is missing security headers: %v", w.Header())
	}
}
