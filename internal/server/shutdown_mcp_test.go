package server

import (
	"context"
	"net/http"
	"testing"
)

// Shutdown must close the MCP manager (stdio children, HTTP sessions), and do
// it once even when Shutdown is called twice.
func TestShutdownClosesMCPManager(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	closed := 0
	s := &Server{
		ctx:        ctx,
		cancel:     cancel,
		registry:   newSessionRegistry(),
		httpServer: &http.Server{},
		closeMCP:   func() { closed++ },
	}
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if closed != 1 {
		t.Fatalf("MCP manager closed %d times, want 1", closed)
	}
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if closed != 1 {
		t.Fatalf("second Shutdown closed MCP again (%d)", closed)
	}
}
