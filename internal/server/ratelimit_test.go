package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRateLimiterRequestAndSessionCaps(t *testing.T) {
	rl := NewRateLimiter(2, 1)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:9999"
	if !rl.AllowRequest(req) || !rl.AllowRequest(req) {
		t.Fatal("first two requests should pass")
	}
	if rl.AllowRequest(req) {
		t.Fatal("third request should be rate limited")
	}
	if !rl.AcquireSession() {
		t.Fatal("first session acquire should pass")
	}
	if rl.AcquireSession() {
		t.Fatal("second session acquire should fail")
	}
	rl.ReleaseSession()
	if !rl.AcquireSession() {
		t.Fatal("acquire after release should pass")
	}
}
