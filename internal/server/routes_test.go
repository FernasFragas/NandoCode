package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func routeDo(t *testing.T, s *Server, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(""))
	r.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, r)
	return w
}

// TestRouteTableWrongMethodIs405 pins the method + path table: a known path
// with the wrong method answers 405 with an Allow header, not 404. GET on a
// POST-only path is not listed: the GET static fallback ("GET /") claims it
// and answers 404.
func TestRouteTableWrongMethodIs405(t *testing.T) {
	s := testServer(t)
	cases := []struct {
		method, path, allow string
	}{
		{http.MethodPost, "/v1/health", "GET"},
		{http.MethodDelete, "/v1/models", "GET"},
		{http.MethodPut, "/v1/sessions", "POST"},
		{http.MethodPost, "/v1/sessions/abc", "GET"},
		{http.MethodPut, "/v1/sessions/abc", "DELETE"},
		{http.MethodPost, "/v1/sessions/abc/events", "GET"},
		{http.MethodPut, "/v1/sessions/abc/messages", "POST"},
		{http.MethodPut, "/v1/sessions/abc/permissions/r1", "POST"},
		{http.MethodPut, "/v1/sessions/abc/model", "POST"},
		{http.MethodPost, "/v1/sessions/abc/files", "GET"},
	}
	for _, c := range cases {
		w := routeDo(t, s, c.method, c.path)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: status=%d, want 405", c.method, c.path, w.Code)
			continue
		}
		if !strings.Contains(w.Header().Get("Allow"), c.allow) {
			t.Errorf("%s %s: Allow=%q, want it to contain %q", c.method, c.path, w.Header().Get("Allow"), c.allow)
		}
	}
}

// TestRouteTableDispatch checks each route reaches its handler.
func TestRouteTableDispatch(t *testing.T) {
	s := testServer(t)
	if w := routeDo(t, s, http.MethodGet, "/v1/health"); w.Code != http.StatusOK {
		t.Fatalf("health status=%d", w.Code)
	}
	if w := routeDo(t, s, http.MethodGet, "/v1/models"); w.Code != http.StatusOK {
		t.Fatalf("models status=%d", w.Code)
	}
	w := routeDo(t, s, http.MethodPost, "/v1/sessions")
	if w.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", w.Code, w.Body.String())
	}
	var view SessionView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil || view.SessionID == "" {
		t.Fatalf("bad session view: %v %s", err, w.Body.String())
	}
	base := "/v1/sessions/" + view.SessionID
	if w := routeDo(t, s, http.MethodGet, base); w.Code != http.StatusOK {
		t.Fatalf("get session status=%d", w.Code)
	}
	if w := routeDo(t, s, http.MethodGet, base+"/files"); w.Code != http.StatusOK {
		t.Fatalf("files status=%d body=%s", w.Code, w.Body.String())
	}
	if w := routeDo(t, s, http.MethodPost, base+"/messages"); w.Code != http.StatusBadRequest {
		t.Fatalf("messages with empty body status=%d, want 400 from the handler", w.Code)
	}
	if w := routeDo(t, s, http.MethodPost, base+"/permissions/nope"); w.Code != http.StatusBadRequest {
		t.Fatalf("permissions with empty body status=%d, want 400 from the handler", w.Code)
	}
	if w := routeDo(t, s, http.MethodDelete, base); w.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d", w.Code)
	}
	if w := routeDo(t, s, http.MethodGet, base); w.Code != http.StatusNotFound {
		t.Fatalf("get after delete status=%d, want 404", w.Code)
	}
}

func TestRouteTableUnknownIs404(t *testing.T) {
	s := testServer(t)
	for _, path := range []string{
		"/v1/nope",
		"/v1/sessions/",
		"/v1/sessions/abc/unknown",
		"/v1/sessions/abc/events/extra",
		"/v1/sessions/missing",
		"/v1/sessions/missing/files",
		"/v1/sessions/missing/events",
	} {
		if w := routeDo(t, s, http.MethodGet, path); w.Code != http.StatusNotFound {
			t.Errorf("GET %s: status=%d, want 404", path, w.Code)
		}
	}
	for _, c := range [][2]string{
		{http.MethodPost, "/v1/sessions/missing/messages"},
		{http.MethodPost, "/v1/sessions/missing/model"},
		{http.MethodPost, "/v1/sessions/missing/permissions/r1"},
		{http.MethodDelete, "/v1/sessions/missing"},
	} {
		if w := routeDo(t, s, c[0], c[1]); w.Code != http.StatusNotFound {
			t.Errorf("%s %s: status=%d, want 404", c[0], c[1], w.Code)
		}
	}
}

// TestRouteTablePreservesStaticAndDev keeps the B1-1 static embed, V1c /dev
// gating and B1-4 security headers working under pattern routing.
func TestRouteTablePreservesStaticAndDev(t *testing.T) {
	s := testServer(t)
	w := routeDo(t, s, http.MethodGet, "/js/markdown.js")
	if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Content-Type"), "javascript") {
		t.Fatalf("static js: status=%d ct=%q", w.Code, w.Header().Get("Content-Type"))
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("missing security headers: %v", w.Header())
	}
	for _, noUI := range []bool{false, true} {
		for _, path := range []string{"/dev", "/dev/", "/dev/index.html", "/dev/x/y"} {
			s := testServer(t)
			s.cfg.NoUI = noUI
			w := routeDo(t, s, http.MethodGet, path)
			if w.Code != http.StatusNotFound {
				t.Errorf("noUI=%v GET %s without --dev: status=%d, want 404", noUI, path, w.Code)
			}
			if loc := w.Header().Get("Location"); loc != "" {
				t.Errorf("noUI=%v GET %s without --dev leaks Location %q", noUI, path, loc)
			}
		}
	}
	s.cfg.NoUI = true
	if w := routeDo(t, s, http.MethodGet, "/"); w.Code != http.StatusNotFound {
		t.Fatalf("NoUI GET / status=%d, want 404", w.Code)
	}
	s = testServer(t)
	s.cfg.Dev = true
	if w := routeDo(t, s, http.MethodGet, "/dev/"); w.Code != http.StatusOK {
		t.Fatalf("with --dev GET /dev/ status=%d", w.Code)
	}
	if w := routeDo(t, s, http.MethodGet, "/dev"); w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("with --dev GET /dev status=%d, want 307", w.Code)
	}
}

// TestRouteTableDevHiddenForEveryMethod: without --dev, /dev answers a plain
// 404 for every method, never 405 or an Allow header that reveals the route.
func TestRouteTableDevHiddenForEveryMethod(t *testing.T) {
	for _, noUI := range []bool{false, true} {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions} {
			for _, path := range []string{"/dev", "/dev/", "/dev/x"} {
				s := testServer(t)
				s.cfg.NoUI = noUI
				w := routeDo(t, s, method, path)
				if w.Code != http.StatusNotFound {
					t.Errorf("noUI=%v %s %s without --dev: status=%d, want 404", noUI, method, path, w.Code)
				}
				if a := w.Header().Get("Allow"); a != "" {
					t.Errorf("noUI=%v %s %s without --dev leaks Allow %q", noUI, method, path, a)
				}
			}
		}
	}
}
