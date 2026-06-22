package mcp

import (
	"context"
	"strings"
	"testing"
)

func TestRenderMCPContentIncludesNonTextPlaceholders(t *testing.T) {
	t.Parallel()
	lines := renderMCPContent([]map[string]any{
		{"type": "text", "text": "hello"},
		{"type": "image", "mimeType": "image/png"},
		{"type": "resource", "uri": "file:///tmp/data.json"},
	})
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[0] != "hello" {
		t.Fatalf("unexpected text line: %q", lines[0])
	}
	if lines[1] == "" || lines[2] == "" {
		t.Fatalf("expected placeholders for non-text content")
	}
}

func TestStartSkipsDisabledAndUntrustedServers(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Servers: []ServerConfig{
			{Name: "disabled", Enabled: false, Trusted: true, Transport: TransportStdio, Command: "noop"},
			{Name: "project_untrusted", Enabled: true, Trusted: false, Transport: TransportStdio, Command: "noop"},
		},
	}
	mgr, warnings := Start(context.Background(), cfg)
	if mgr == nil {
		t.Fatal("expected manager")
	}
	defer mgr.Close()
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d (%v)", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "untrusted") {
		t.Fatalf("expected untrusted warning, got %q", warnings[0])
	}
	statuses := mgr.ServerStatuses()
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}
	if statuses[0].Connected || statuses[1].Connected {
		t.Fatalf("expected no connected servers, got %#v", statuses)
	}
}

func TestMCPToolDescriptionTruncatesLongServerDescription(t *testing.T) {
	t.Parallel()
	raw := strings.Repeat("a", 200)
	desc := mcpToolDescription(raw, "x")
	if len(desc) > 100 {
		t.Fatalf("description should be <= 100 chars, got %d", len(desc))
	}
	if !strings.HasSuffix(desc, ".") {
		t.Fatalf("description should end with period: %q", desc)
	}
}
