package dirwalk

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWalkExcludesCommonDirs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "keep", "a.txt"), "ok")
	mustWriteFile(t, filepath.Join(root, ".git", "config"), "no")
	mustWriteFile(t, filepath.Join(root, "node_modules", "x.js"), "no")

	entries, _, err := Walk(context.Background(), root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if hasRel(entries, ".git/config") {
		t.Fatal("unexpected .git entry")
	}
	if hasRel(entries, "node_modules/x.js") {
		t.Fatal("unexpected node_modules entry")
	}
	if !hasRel(entries, "keep/a.txt") {
		t.Fatal("missing keep/a.txt")
	}
}

func TestWalkStopsAtMaxFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for i := 0; i < 10; i++ {
		mustWriteFile(t, filepath.Join(root, "d", string(rune('a'+i))+".txt"), "x")
	}
	entries, stats, err := Walk(context.Background(), root, Options{MaxFiles: 3})
	if err != nil {
		t.Fatal(err)
	}
	if !stats.Truncated || stats.Reason != ReasonFileCap {
		t.Fatalf("expected file-cap truncation, got %+v", stats)
	}
	files := countFiles(entries)
	if files > 3 {
		t.Fatalf("expected <=3 files, got %d", files)
	}
}

func TestWalkStopsAtMaxBytes(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "a.txt"), "12345")
	mustWriteFile(t, filepath.Join(root, "b.txt"), "12345")

	entries, stats, err := Walk(context.Background(), root, Options{MaxBytes: 6})
	if err != nil {
		t.Fatal(err)
	}
	if !stats.Truncated || stats.Reason != ReasonByteCap {
		t.Fatalf("expected byte-cap truncation, got %+v", stats)
	}
	if countFiles(entries) > 1 {
		t.Fatalf("expected <=1 file, got %d", countFiles(entries))
	}
}

func TestWalkStopsAtMaxDepth(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "a", "b", "c", "d.txt"), "x")

	entries, stats, err := Walk(context.Background(), root, Options{MaxDepth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !stats.Truncated || stats.Reason != ReasonDepthCap {
		t.Fatalf("expected depth-cap truncation, got %+v", stats)
	}
	if hasRel(entries, "a/b/c/d.txt") {
		t.Fatal("unexpected deep file")
	}
}

func TestWalkSkipsSymlinkDir(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior varies on windows")
	}
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(target, "x.txt"), "x")
	if err := os.Symlink(target, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	entries, _, err := Walk(context.Background(), root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if hasRel(entries, "link/x.txt") {
		t.Fatal("symlinked directory should not be followed")
	}
}

func TestWalkSkipsPermissionDenied(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("permission model differs on windows")
	}
	root := t.TempDir()
	blocked := filepath.Join(root, "blocked")
	if err := os.MkdirAll(blocked, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(blocked, "secret.txt"), "x")
	if err := os.Chmod(blocked, 0o000); err != nil {
		t.Skipf("chmod unsupported: %v", err)
	}
	defer os.Chmod(blocked, 0o755)

	entries, _, err := Walk(context.Background(), root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if hasRel(entries, "blocked/secret.txt") {
		t.Fatal("permission-denied subtree should be skipped")
	}
}

func TestWalkEmptyDirectory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	entries, stats, err := Walk(context.Background(), root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no entries, got %d", len(entries))
	}
	if stats.FileCount != 0 || stats.ByteCount != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hasRel(entries []Entry, rel string) bool {
	for _, e := range entries {
		if e.RelPath == rel {
			return true
		}
	}
	return false
}

func countFiles(entries []Entry) int {
	n := 0
	for _, e := range entries {
		if !e.IsDir && !e.Skipped {
			n++
		}
	}
	return n
}
