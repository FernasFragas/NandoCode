package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoaderPriorityAndMCP(t *testing.T) {
	t.Parallel()
	user := t.TempDir()
	proj := t.TempDir()
	writeSkill(t, filepath.Join(user, "s.md"), "same", "user")
	writeSkill(t, filepath.Join(proj, "s.md"), "same", "project")

	l, err := NewLoader(user, proj, BundledFS)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	sf, ok := l.Lookup("same")
	if !ok || sf.Source != SourceProject {
		t.Fatalf("expected project override, got %#v", sf)
	}
	l.AddMCPSkill(SkillFile{Name: "same", Description: "mcp", Body: "mcp body"})
	sf, ok = l.Lookup("same")
	if !ok || sf.Source != SourceMCP {
		t.Fatalf("expected mcp override, got %#v", sf)
	}
}

func TestLoaderWatcherCreate(t *testing.T) {
	t.Parallel()
	user := t.TempDir()
	proj := t.TempDir()
	l, err := NewLoader(user, proj, BundledFS)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	writeSkill(t, filepath.Join(proj, "new.md"), "new-skill", "d")
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := l.Lookup("new-skill"); ok {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("new skill not detected within 1s")
}

func TestLoaderSkipsInvalidAndLoadsValid(t *testing.T) {
	t.Parallel()
	user := t.TempDir()
	proj := t.TempDir()
	writeSkill(t, filepath.Join(user, "good.md"), "good", "desc")
	if err := os.WriteFile(filepath.Join(user, "bad.md"), []byte("---\nname: [\ndescription: bad\n---\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := NewLoader(user, proj, BundledFS)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if _, ok := l.Lookup("good"); !ok {
		t.Fatal("expected valid skill to load")
	}
	if _, ok := l.Lookup("bad"); ok {
		t.Fatal("expected invalid skill to be skipped")
	}
}

func TestLoaderDuplicateNameInTierLaterFilenameWins(t *testing.T) {
	t.Parallel()
	user := t.TempDir()
	proj := t.TempDir()
	writeSkill(t, filepath.Join(user, "a.md"), "same", "first")
	writeSkill(t, filepath.Join(user, "z.md"), "same", "second")

	l, err := NewLoader(user, proj, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	sf, ok := l.Lookup("same")
	if !ok {
		t.Fatal("expected skill to load")
	}
	if sf.Description != "second" {
		t.Fatalf("expected lexicographically later file to win, got description=%q", sf.Description)
	}
}

func TestLoaderNonexistentDirsAreEmpty(t *testing.T) {
	t.Parallel()
	user := filepath.Join(t.TempDir(), "missing-user")
	proj := filepath.Join(t.TempDir(), "missing-proj")
	l, err := NewLoader(user, proj, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if got := len(l.List()); got != 0 {
		t.Fatalf("expected no skills for missing dirs, got %d", got)
	}
}

func TestLoaderListSortedByName(t *testing.T) {
	t.Parallel()
	user := t.TempDir()
	proj := t.TempDir()
	writeSkill(t, filepath.Join(user, "b.md"), "bravo", "d")
	writeSkill(t, filepath.Join(user, "a.md"), "alpha", "d")

	l, err := NewLoader(user, proj, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	list := l.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(list))
	}
	if list[0].Name != "alpha" || list[1].Name != "bravo" {
		t.Fatalf("expected sorted list, got %q then %q", list[0].Name, list[1].Name)
	}
}

func TestLoaderReadBody(t *testing.T) {
	t.Parallel()
	user := t.TempDir()
	proj := t.TempDir()
	writeSkill(t, filepath.Join(user, "body.md"), "read-body", "d")

	l, err := NewLoader(user, proj, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	sf, ok := l.Lookup("read-body")
	if !ok {
		t.Fatal("expected skill to exist")
	}
	body, err := l.ReadBody(sf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "body") {
		t.Fatalf("expected body content, got %q", body)
	}
}

func writeSkill(t *testing.T, path, name, desc string) {
	t.Helper()
	content := "---\nname: " + name + "\ndescription: " + desc + "\n---\nbody"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
