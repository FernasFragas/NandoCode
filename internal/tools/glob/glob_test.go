package glob

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/FernasFragas/nandocodego/internal/tools"
)

func makeCtx(t *testing.T, dir string) tools.Context {
	t.Helper()
	ctx := tools.DefaultContext(context.Background(), dir)
	ctx.PermissionMode = tools.PermissionBypassPermissions
	return ctx
}

func TestGlob_SimplePattern(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "foo.go"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "bar.go"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "baz.txt"), []byte(""), 0o644)

	tool := NewGlobTool()
	res, err := tool.Call(makeCtx(t, dir), Input{Pattern: "*.go"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := res.Data.(Output)
	if out.TotalMatched != 2 {
		t.Fatalf("expected 2 matches, got %d", out.TotalMatched)
	}
}

func TestGlob_RecursivePattern(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0o755)
	os.WriteFile(filepath.Join(dir, "root.go"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "sub", "child.go"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "sub", "deep", "nested.go"), []byte(""), 0o644)

	tool := NewGlobTool()
	res, err := tool.Call(makeCtx(t, dir), Input{Pattern: "**/*.go"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := res.Data.(Output)
	if out.TotalMatched < 3 {
		t.Fatalf("expected at least 3 matches, got %d: %v", out.TotalMatched, out.Paths)
	}
}

func TestGlob_ExcludesDotGit(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	os.WriteFile(filepath.Join(dir, ".git", "config"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(""), 0o644)

	tool := NewGlobTool()
	res, err := tool.Call(makeCtx(t, dir), Input{Pattern: "**/*"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := res.Data.(Output)
	for _, p := range out.Paths {
		if p == ".git/config" {
			t.Error("should not have included .git/config")
		}
	}
}

func TestGlob_ExcludesNodeModules(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "node_modules"), 0o755)
	os.WriteFile(filepath.Join(dir, "node_modules", "pkg.js"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "index.js"), []byte(""), 0o644)

	tool := NewGlobTool()
	res, err := tool.Call(makeCtx(t, dir), Input{Pattern: "**/*.js"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := res.Data.(Output)
	for _, p := range out.Paths {
		if p == "node_modules/pkg.js" {
			t.Error("should not have included node_modules/pkg.js")
		}
	}
}

func TestGlob_ExcludesVendor(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "vendor"), 0o755)
	os.WriteFile(filepath.Join(dir, "vendor", "dep.go"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(""), 0o644)

	tool := NewGlobTool()
	res, err := tool.Call(makeCtx(t, dir), Input{Pattern: "**/*.go"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := res.Data.(Output)
	for _, p := range out.Paths {
		if p == "vendor/dep.go" {
			t.Error("should not have included vendor/dep.go")
		}
	}
}

func TestGlob_TruncatesAt1000(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 1001; i++ {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%04d.txt", i)), []byte(""), 0o644)
	}

	tool := NewGlobTool()
	res, err := tool.Call(makeCtx(t, dir), Input{Pattern: "*.txt"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := res.Data.(Output)
	if !out.Truncated {
		t.Error("expected Truncated=true")
	}
	if len(out.Paths) != 1000 {
		t.Fatalf("expected 1000 paths, got %d", len(out.Paths))
	}
	if out.TotalMatched != 1001 {
		t.Fatalf("expected TotalMatched=1001, got %d", out.TotalMatched)
	}
}

func TestGlob_EmptyPatternError(t *testing.T) {
	dir := t.TempDir()
	tool := NewGlobTool()
	_, err := tool.Call(makeCtx(t, dir), Input{Pattern: ""}, nil)
	if err == nil {
		t.Error("expected error for empty pattern")
	}
}

func TestGlob_SortedResults(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "z.go"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "a.go"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "m.go"), []byte(""), 0o644)

	tool := NewGlobTool()
	res, err := tool.Call(makeCtx(t, dir), Input{Pattern: "*.go"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := res.Data.(Output)
	for i := 1; i < len(out.Paths); i++ {
		if out.Paths[i] < out.Paths[i-1] {
			t.Errorf("results not sorted: %v", out.Paths)
		}
	}
}

func TestGlob_BasePathOutsideWorkingDir(t *testing.T) {
	dir := t.TempDir()
	tool := NewGlobTool()
	_, err := tool.Call(makeCtx(t, dir), Input{Pattern: "*.go", BasePath: "/etc"}, nil)
	if err == nil {
		t.Error("expected error for base_path outside working directory")
	}
}
