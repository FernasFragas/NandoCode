package auth

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestEnsureTokenUsesCachedToken(t *testing.T) {
	store := &memoryStore{m: map[string]string{}}
	tok := tokenEnvelope{AccessToken: "cached", Expiry: time.Now().UTC().Add(2 * time.Hour)}
	b, _ := json.Marshal(tok)
	_ = store.Save("srv", string(b))

	f := &PKCEFlow{
		ServerID:    "srv",
		ResourceURL: "http://127.0.0.1:11434",
		Store:       store,
	}
	got, err := f.EnsureToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "cached" {
		t.Fatalf("got %q", got)
	}
}

func TestGeneratePKCEDistinct(t *testing.T) {
	v1, c1, err := generatePKCE()
	if err != nil {
		t.Fatal(err)
	}
	v2, c2, err := generatePKCE()
	if err != nil {
		t.Fatal(err)
	}
	if v1 == v2 || c1 == c2 {
		t.Fatal("expected distinct verifier/challenge pairs")
	}
}

func TestEnsureTokenFullFlow(t *testing.T) {
	store := &memoryStore{m: map[string]string{}}
	mux := http.NewServeMux()
	baseURL := startLoopbackHTTPServer(t, mux)

	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"authorization_servers": []string{baseURL}})
	})
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"authorization_endpoint": baseURL + "/authorize",
			"token_endpoint":         baseURL + "/token",
		})
	})
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		redirectURI := q.Get("redirect_uri")
		state := q.Get("state")
		u, _ := url.Parse(redirectURI)
		v := u.Query()
		v.Set("code", "abc123")
		v.Set("state", state)
		u.RawQuery = v.Encode()
		http.Redirect(w, r, u.String(), http.StatusFound)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("grant_type") == "authorization_code" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "fresh-token",
				"token_type":    "Bearer",
				"refresh_token": "refresh-1",
				"expires_in":    3600,
			})
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	})

	httpClient := &http.Client{Timeout: 10 * time.Second}
	f := &PKCEFlow{
		ServerID:    "srv",
		ResourceURL: baseURL,
		Store:       store,
		HTTPClient:  httpClient,
		OpenBrowser: func(raw string) error {
			resp, err := httpClient.Get(raw)
			if err != nil {
				return err
			}
			resp.Body.Close()
			return nil
		},
	}
	got, err := f.EnsureToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "fresh-token" {
		t.Fatalf("got %q", got)
	}
	stored, _ := store.Load("srv")
	if !strings.Contains(stored, "fresh-token") {
		t.Fatalf("stored token missing access token: %s", stored)
	}
}

func startLoopbackHTTPServer(t *testing.T, handler http.Handler) string {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("loopback listener unavailable in this environment: %v", err)
		return ""
	}
	srv := &http.Server{Handler: handler}
	go func() {
		_ = srv.Serve(ln)
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	return "http://" + ln.Addr().String()
}
