package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScopeRootFindsGitAncestor(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := ScopeRoot(sub, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("expected %q, got %q", root, got)
	}
}
