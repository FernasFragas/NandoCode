package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadIndexCapsByLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "MEMORY.md")
	content := "a\nb\nc\nd\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err := LoadIndex(path, 2, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if !idx.Capped {
		t.Fatalf("expected capped index")
	}
	if idx.Content != "a\nb" {
		t.Fatalf("unexpected capped content: %q", idx.Content)
	}
}
