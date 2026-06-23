package filewrite

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/tools"
)

func TestFileWriteToolCreatesFile(t *testing.T) {
	dir := t.TempDir()
	tool := NewFileWriteTool()
	result, err := tool.Call(tools.DefaultContext(context.Background(), dir), Input{Path: "out.txt", Content: "hello"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := result.Data.(Output)
	if !out.Created || out.BytesWritten != 5 {
		t.Fatalf("out = %#v", out)
	}
	content, err := os.ReadFile(filepath.Join(dir, "out.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "hello" {
		t.Fatalf("content = %q", content)
	}
}

func TestFileWriteToolOverwritesAndRecordsSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	var snapshotPath string
	var snapshotContent []byte
	ctx := tools.DefaultContext(context.Background(), dir)
	ctx.RecordFileSnapshot = func(path string, content []byte) {
		snapshotPath = path
		snapshotContent = content
	}
	result, err := NewFileWriteTool().Call(ctx, Input{Path: "out.txt", Content: "new"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := result.Data.(Output)
	if out.Created {
		t.Fatal("expected overwrite")
	}
	if snapshotPath != path || string(snapshotContent) != "old" {
		t.Fatalf("snapshot %q %q", snapshotPath, snapshotContent)
	}
}

func TestFileWriteToolRejectsOutsidePathAndMissingParent(t *testing.T) {
	dir := t.TempDir()
	ctx := tools.DefaultContext(context.Background(), dir)
	if _, err := NewFileWriteTool().Call(ctx, Input{Path: "../x", Content: "bad"}, nil); err == nil {
		t.Fatal("expected outside path error")
	}
	if _, err := NewFileWriteTool().Call(ctx, Input{Path: "missing/x", Content: "bad"}, nil); err == nil {
		t.Fatal("expected missing parent error")
	}
}

func TestFileWriteToolPermissions(t *testing.T) {
	tool := NewFileWriteTool()
	input := Input{Path: "a.txt", Content: "x"}
	if tool.IsReadOnly(input) || tool.IsConcurrencySafe(input) || !tool.IsDestructive(input) {
		t.Fatal("unexpected classification")
	}
	if perm := tool.CheckPermissions(tools.Context{}, input); perm.Decision != tools.PermAsk {
		t.Fatalf("default permission = %s", perm.Decision)
	}
	if perm := tool.CheckPermissions(tools.Context{PermissionMode: tools.PermissionPlan}, input); perm.Decision != tools.PermDeny {
		t.Fatalf("plan permission = %s", perm.Decision)
	}
	if perm := tool.CheckPermissions(tools.Context{PermissionMode: tools.PermissionBypassPermissions}, input); perm.Decision != tools.PermAllow {
		t.Fatalf("bypass permission = %s", perm.Decision)
	}
}

func TestFileWriteToolUnmarshalInput(t *testing.T) {
	input, err := NewFileWriteTool().UnmarshalInput(json.RawMessage(`{"path":"a.txt","content":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	got := input.(Input)
	if got.Path != "a.txt" || got.Content != "x" {
		t.Fatalf("got %#v", got)
	}
	if _, err := NewFileWriteTool().UnmarshalInput(json.RawMessage(`{"path":""}`)); err == nil {
		t.Fatal("expected empty path error")
	}
}

func TestAtomicWriteStress(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	for i := 0; i < 50; i++ {
		if err := AtomicWrite(path, []byte("complete-content"), 0o644); err != nil {
			t.Fatal(err)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "complete-content" {
			t.Fatalf("partial content observed: %q", content)
		}
	}
}
