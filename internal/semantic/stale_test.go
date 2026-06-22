package semantic

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceIDStable(t *testing.T) {
	t.Parallel()
	td := t.TempDir()
	id1, err := WorkspaceID(td, "qwen3-embedding:8b", 1024, SchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	id2, err := WorkspaceID(td, "qwen3-embedding:8b", 1024, SchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("workspace id is not stable: %q != %q", id1, id2)
	}
}

func TestIsRecordStale(t *testing.T) {
	t.Parallel()
	td := t.TempDir()
	p := filepath.Join(td, "x.go")
	if err := os.WriteFile(p, []byte("package x"), 0o644); err != nil {
		t.Fatal(err)
	}
	h, err := HashFile(p)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := IsRecordStale(Record{ContentHash: h}, p)
	if err != nil {
		t.Fatal(err)
	}
	if stale {
		t.Fatalf("expected record to be fresh")
	}
	if err := os.WriteFile(p, []byte("package y"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale, err = IsRecordStale(Record{ContentHash: h}, p)
	if err != nil {
		t.Fatal(err)
	}
	if !stale {
		t.Fatalf("expected record to be stale after content change")
	}
}
