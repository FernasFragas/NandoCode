package fileedit

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/FernasFragas/nandocodego/internal/tools"
)

func makeCtx(t *testing.T, dir string) tools.Context {
	t.Helper()
	snaps := make(map[string][]byte)
	var mu sync.RWMutex
	ctx := tools.DefaultContext(context.Background(), dir)
	ctx.PermissionMode = tools.PermissionBypassPermissions
	ctx.RecordFileSnapshot = func(path string, content []byte) {
		mu.Lock()
		snaps[path] = append([]byte(nil), content...)
		mu.Unlock()
	}
	ctx.ReadFileSnapshot = func(path string) ([]byte, bool) {
		mu.RLock()
		defer mu.RUnlock()
		b, ok := snaps[path]
		return b, ok
	}
	return ctx
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFileEdit_SimpleReplacement(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "foo.txt")
	writeFile(t, p, "hello world\n")
	ctx := makeCtx(t, dir)
	ctx.RecordFileSnapshot(p, []byte("hello world\n"))

	tool := NewFileEditTool()
	res, err := tool.Call(ctx, Input{Path: p, OldString: "hello", NewString: "goodbye"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "goodbye world\n" {
		t.Fatalf("expected 'goodbye world\\n', got %q", string(got))
	}
	if res.Display == "" {
		t.Error("expected non-empty diff display")
	}
}

func TestFileEdit_StalenesserError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "foo.txt")
	writeFile(t, p, "original content\n")
	ctx := makeCtx(t, dir)
	// Record stale snapshot
	ctx.RecordFileSnapshot(p, []byte("old content\n"))

	tool := NewFileEditTool()
	_, err := tool.Call(ctx, Input{Path: p, OldString: "original", NewString: "new"}, nil)
	if err == nil {
		t.Fatal("expected staleness error")
	}
}

func TestFileEdit_NoOpRejection(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "foo.txt")
	writeFile(t, p, "content\n")
	ctx := makeCtx(t, dir)

	tool := NewFileEditTool()
	_, err := tool.Call(ctx, Input{Path: p, OldString: "content", NewString: "content"}, nil)
	if err == nil {
		t.Fatal("expected no-op rejection error")
	}
}

func TestFileEdit_OldStringNotFound(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "foo.txt")
	writeFile(t, p, "hello world\n")
	ctx := makeCtx(t, dir)
	ctx.RecordFileSnapshot(p, []byte("hello world\n"))

	tool := NewFileEditTool()
	_, err := tool.Call(ctx, Input{Path: p, OldString: "nonexistent", NewString: "x"}, nil)
	if err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestFileEdit_NonUniqueWithoutReplaceAll(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "foo.txt")
	writeFile(t, p, "abc\nabc\n")
	ctx := makeCtx(t, dir)
	ctx.RecordFileSnapshot(p, []byte("abc\nabc\n"))

	tool := NewFileEditTool()
	_, err := tool.Call(ctx, Input{Path: p, OldString: "abc", NewString: "xyz"}, nil)
	if err == nil {
		t.Fatal("expected ambiguity error")
	}
}

func TestFileEdit_ReplaceAll(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "foo.txt")
	writeFile(t, p, "abc\nabc\n")
	ctx := makeCtx(t, dir)
	ctx.RecordFileSnapshot(p, []byte("abc\nabc\n"))

	tool := NewFileEditTool()
	_, err := tool.Call(ctx, Input{Path: p, OldString: "abc", NewString: "xyz", ReplaceAll: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "xyz\nxyz\n" {
		t.Fatalf("expected 'xyz\\nxyz\\n', got %q", string(got))
	}
}

func TestFileEdit_TrailingWhitespaceNormalization(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "foo.txt")
	// File has trailing spaces on a line
	writeFile(t, p, "func foo()  \nreturn 0\n")
	ctx := makeCtx(t, dir)
	ctx.RecordFileSnapshot(p, []byte("func foo()  \nreturn 0\n"))

	tool := NewFileEditTool()
	// old_string without trailing spaces should still match
	_, err := tool.Call(ctx, Input{Path: p, OldString: "func foo()\nreturn 0", NewString: "func bar()\nreturn 1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFileEdit_PathOutsideWorkingDir(t *testing.T) {
	dir := t.TempDir()
	ctx := makeCtx(t, dir)

	tool := NewFileEditTool()
	_, err := tool.Call(ctx, Input{Path: "/etc/passwd", OldString: "root", NewString: "x"}, nil)
	if err == nil {
		t.Fatal("expected path containment error")
	}
}

func TestFileEdit_Race(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "race.txt")
	writeFile(t, p, "value\n")
	ctx := makeCtx(t, dir)
	ctx.RecordFileSnapshot(p, []byte("value\n"))

	tool := NewFileEditTool()
	// Read snapshot concurrently
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx.RecordFileSnapshot(p, []byte("value\n"))
			ctx.ReadFileSnapshot(p)
		}()
	}
	wg.Wait()
	// Single edit should work
	_, err := tool.Call(ctx, Input{Path: p, OldString: "value", NewString: "replaced"}, nil)
	if err != nil {
		t.Fatal(err)
	}
}
