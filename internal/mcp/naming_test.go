package mcp

import "testing"

func TestToolName(t *testing.T) {
	t.Parallel()
	got := toolName("File System", "Read-File")
	want := "mcp__file_system__read_file"
	if got != want {
		t.Fatalf("toolName mismatch: got %q want %q", got, want)
	}
}

func TestSanitizeNameFallback(t *testing.T) {
	t.Parallel()
	if got := sanitizeName("   "); got != "unknown" {
		t.Fatalf("sanitizeName fallback mismatch: got %q", got)
	}
}
