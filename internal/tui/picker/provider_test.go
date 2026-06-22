package picker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FernasFragas/nandocodego/internal/commands"
	"github.com/FernasFragas/nandocodego/internal/tui/fileindex"
)

func TestFileProviderRanksBasenamePrefixHigher(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "internal/tui/app.go"), "a")
	mustWrite(t, filepath.Join(root, "docs/wrap.md"), "a")
	mustWrite(t, filepath.Join(root, "tmp/app_notes.txt"), "a")

	idx := fileindex.New(root)
	if err := idx.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	p := NewFileProvider(idx, fileindex.NewFrecency())
	got := p.Suggest("app", 5)
	if len(got) == 0 {
		t.Fatal("expected suggestions")
	}
	if got[0].Insert != "internal/tui/app.go" && got[0].Insert != "tmp/app_notes.txt" {
		t.Fatalf("unexpected top result: %#v", got[0])
	}
}

func TestFileProviderFrecencyBoost(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "src/alpha.go"), "a")
	mustWrite(t, filepath.Join(root, "src/beta.go"), "b")

	idx := fileindex.New(root)
	if err := idx.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	freq := fileindex.NewFrecency()
	freq.Touch("src/beta.go")
	freq.Touch("src/beta.go")
	freq.Touch("src/beta.go")
	p := NewFileProvider(idx, freq)
	got := p.Suggest("go", 5)
	if len(got) < 2 {
		t.Fatalf("expected >=2 suggestions, got %d", len(got))
	}
	if got[0].Insert != "src/beta.go" {
		t.Fatalf("expected frecency winner src/beta.go, got %s", got[0].Insert)
	}
}

func TestFileProviderEmptyQueryIncludesTopDirs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "internal/x.go"), "x")
	mustWrite(t, filepath.Join(root, "cmd/main.go"), "m")
	idx := fileindex.New(root)
	if err := idx.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	p := NewFileProvider(idx, fileindex.NewFrecency())
	got := p.Suggest("", 8)
	if len(got) == 0 {
		t.Fatal("expected suggestions")
	}
	foundDir := false
	for _, s := range got {
		if s.IsDir && (s.Insert == "internal" || s.Insert == "cmd") {
			foundDir = true
		}
	}
	if !foundDir {
		t.Fatalf("expected top-level dir in results: %#v", got)
	}
}

func TestFileProviderRejectsOutsideRootQuery(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "a")
	idx := fileindex.New(root)
	if err := idx.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	p := NewFileProvider(idx, fileindex.NewFrecency())
	if got := p.Suggest("../secret", 8); len(got) != 0 {
		t.Fatalf("expected empty, got %#v", got)
	}
}

func TestFileProviderPathAwareQueryFiltering(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "internal/tui/app.go"), "a")
	mustWrite(t, filepath.Join(root, "internal/tools/pathsafe.go"), "a")
	mustWrite(t, filepath.Join(root, "cmd/main.go"), "a")
	idx := fileindex.New(root)
	if err := idx.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	p := NewFileProvider(idx, fileindex.NewFrecency())
	got := p.Suggest("internal/t", 10)
	if len(got) == 0 {
		t.Fatal("expected path-aware matches")
	}
	for _, s := range got {
		if !strings.HasPrefix(s.Insert, "internal/") {
			t.Fatalf("unexpected non-internal match for path-aware query: %#v", s)
		}
	}
}

func TestCommandProviderListsAndFilters(t *testing.T) {
	t.Parallel()
	reg := commands.New()
	commands.RegisterDefaults(reg)
	p := NewCommandProvider(reg)
	all := p.Suggest("", 50)
	if len(all) == 0 {
		t.Fatal("expected commands")
	}
	filtered := p.Suggest("he", 10)
	if len(filtered) == 0 {
		t.Fatal("expected filtered commands")
	}
	if filtered[0].Insert != "help" {
		t.Fatalf("expected /help first, got %#v", filtered[0])
	}
	if filtered[0].Detail != "session" {
		t.Fatalf("expected command detail metadata, got %#v", filtered[0])
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
