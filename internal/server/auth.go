package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
)

func GenerateToken() (string, error) {
	return randomHex(32)
}

// newSessionID returns an unguessable session identifier.
func newSessionID() (string, error) {
	id, err := randomHex(16)
	if err != nil {
		return "", err
	}
	return "sess_" + id, nil
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// NewAuthMiddleware requires the bearer token on API routes (/v1/...). The
// static web UI is served without it: it holds no session data and must load
// before the user can supply the token. New always configures a token, so an
// empty token only occurs for servers built directly in tests.
func NewAuthMiddleware(token string, next http.Handler) http.Handler {
	if strings.TrimSpace(token) == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isAPIPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		v := r.Header.Get("Authorization")
		const prefix = "Bearer "
		got := strings.TrimSpace(strings.TrimPrefix(v, prefix))
		if !strings.HasPrefix(v, prefix) || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// NewRequestGuard blocks cross-site and DNS-rebinding access to the server:
//   - when bound to a loopback address, the Host header must name a loopback
//     host, so a rebound attacker domain cannot reach the server;
//   - a browser Origin, when present, must be the origin being served;
//   - API requests with a body must be JSON, so pages cannot send them as
//     "simple" cross-origin requests (e.g. text/plain) that skip CORS preflight.
func NewRequestGuard(bind string, next http.Handler) http.Handler {
	checkHost := isLoopbackHost(bind)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if checkHost && !isLoopbackHost(hostOnly(r.Host)) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && !sameOrigin(origin, r.Host) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		if isAPIPath(r.URL.Path) && hasBody(r) && !isJSONContentType(r.Header.Get("Content-Type")) {
			http.Error(w, "content type must be application/json", http.StatusUnsupportedMediaType)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isAPIPath(p string) bool {
	return p == "/v1" || strings.HasPrefix(p, "/v1/")
}

func isLoopbackHost(host string) bool {
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func hostOnly(hostport string) string {
	if host, _, err := net.SplitHostPort(hostport); err == nil {
		return host
	}
	return hostport
}

func sameOrigin(origin, requestHost string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return false // includes the opaque "null" origin
	}
	return strings.EqualFold(u.Host, requestHost)
}

func hasBody(r *http.Request) bool {
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return r.ContentLength != 0 // -1 means a body of unknown length
	}
	return false
}

func isJSONContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	return err == nil && mediaType == "application/json"
}
