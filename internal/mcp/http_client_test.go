package mcp

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHTTPClientUsesBearerToken(t *testing.T) {
	var sawAuth string
	baseURL := startLoopbackHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]any{
				"tools": []map[string]any{
					{"name": "x", "description": "d", "inputSchema": map[string]any{"type": "object"}},
				},
			},
		})
	}))

	c, err := startHTTPClient(ServerConfig{Name: "s", URL: baseURL, Transport: TransportHTTP}, func(context.Context) (string, error) {
		return "abc", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.listTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sawAuth != "Bearer abc" {
		t.Fatalf("authorization header = %q", sawAuth)
	}
}

func TestHTTPClientParsesSSEPayload(t *testing.T) {
	baseURL := startLoopbackHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message\n"))
		_, _ = w.Write([]byte(`data: {"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"sse","description":"d","inputSchema":{"type":"object"}}]}}` + "\n\n"))
	}))

	c, err := startHTTPClient(ServerConfig{Name: "s", URL: baseURL, Transport: TransportHTTP}, nil)
	if err != nil {
		t.Fatal(err)
	}
	tools, err := c.listTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || strings.TrimSpace(tools[0].Name) != "sse" {
		t.Fatalf("unexpected tools: %#v", tools)
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
