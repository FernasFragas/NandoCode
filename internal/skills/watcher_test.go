package skills

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcherOnChangeCallback(t *testing.T) {
	t.Parallel()
	user := t.TempDir()
	proj := t.TempDir()
	l, err := NewLoader(user, proj, BundledFS)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	ch := make(chan string, 2)
	l.OnChange(func(name string, src Source) { ch <- name + ":" + src.String() })

	writeSkill(t, filepath.Join(user, "u.md"), "u-skill", "d")
	select {
	case <-ch:
	case <-time.After(1 * time.Second):
		t.Fatal("onChange not triggered")
	}
}

func TestWatcherUpdatesModifiedSkill(t *testing.T) {
	t.Parallel()
	user := t.TempDir()
	proj := t.TempDir()
	path := filepath.Join(proj, "s.md")
	writeSkill(t, path, "watched", "old")

	l, err := NewLoader(user, proj, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	writeSkill(t, path, "watched", "new")
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		sf, ok := l.Lookup("watched")
		if ok && sf.Description == "new" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("updated skill not detected within 1s")
}

func TestWatcherRemovesDeletedSkill(t *testing.T) {
	t.Parallel()
	user := t.TempDir()
	proj := t.TempDir()
	path := filepath.Join(proj, "delete.md")
	writeSkill(t, path, "to-delete", "d")

	l, err := NewLoader(user, proj, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if _, ok := l.Lookup("to-delete"); !ok {
		t.Fatal("expected skill before deletion")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := l.Lookup("to-delete"); !ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("deleted skill still present after 1s")
}

func TestWatcherRenameRemovesOldPathSkill(t *testing.T) {
	t.Parallel()
	user := t.TempDir()
	proj := t.TempDir()
	oldPath := filepath.Join(user, "rename.md")
	newPath := filepath.Join(user, "renamed-away.tmp")
	writeSkill(t, oldPath, "renamed-skill", "d")

	l, err := NewLoader(user, proj, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if _, ok := l.Lookup("renamed-skill"); !ok {
		t.Fatal("expected initial skill")
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := l.Lookup("renamed-skill"); !ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("renamed-away skill still present after 1s")
}
