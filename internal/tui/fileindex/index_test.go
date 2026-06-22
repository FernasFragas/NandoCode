package fileindex

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRefreshWalkFallbackExcludesCommonDirs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "keep", "a.txt"), "ok")
	mustWriteFile(t, filepath.Join(root, ".git", "config"), "no")
	mustWriteFile(t, filepath.Join(root, "node_modules", "x.js"), "no")

	idx := New(root)
	if err := idx.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	entries := idx.Snapshot()
	if containsRel(entries, ".git/config") {
		t.Fatal("unexpected .git file in index")
	}
	if containsRel(entries, "node_modules/x.js") {
		t.Fatal("unexpected node_modules file in index")
	}
	if !containsRel(entries, "keep/a.txt") {
		t.Fatal("missing keep/a.txt")
	}
}

func TestRefreshIsIdempotent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "a.txt"), "ok")
	idx := New(root)
	if err := idx.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	first := idx.Snapshot()
	if err := idx.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	second := idx.Snapshot()
	if len(first) != len(second) {
		t.Fatalf("len mismatch: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("entry mismatch at %d: %#v vs %#v", i, first[i], second[i])
		}
	}
}

func TestRefreshSetsTruncatedFlag(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for i := 0; i < 10; i++ {
		mustWriteFile(t, filepath.Join(root, "d", string(rune('a'+i))+".txt"), "x")
	}
	idx := New(root)
	idx.maxEntries = 3
	if err := idx.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !idx.Truncated() {
		t.Fatal("expected truncated=true")
	}
}

func containsRel(entries []Entry, want string) bool {
	for _, e := range entries {
		if e.Rel == want {
			return true
		}
	}
	return false
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
