package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDevGalleryRequiresFlag(t *testing.T) {
	for _, noUI := range []bool{false, true} {
		for _, path := range []string{"/dev/", "/dev/index.html", "/dev"} {
			s := testServer(t)
			s.cfg.NoUI = noUI
			s.cfg.Dev = false
			w := httptest.NewRecorder()
			s.routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
			if w.Code != http.StatusNotFound {
				t.Fatalf("noUI=%v path=%s without --dev: status=%d, want 404", noUI, path, w.Code)
			}
		}
	}

	s := testServer(t)
	s.cfg.Dev = true
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/dev/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("with --dev: status=%d, want 200", w.Code)
	}
}
