package tools

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolvePath(t *testing.T) {
	root := t.TempDir()
	ctx := DefaultContext(context.Background(), root)
	if err := os.WriteFile(filepath.Join(root, "inside.txt"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ResolvePath(ctx, "inside.txt", PathRead)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(root, "inside.txt") {
		t.Fatalf("got %q", got)
	}

	if _, err := ResolvePath(ctx, "../outside.txt", PathWrite); err == nil {
		t.Fatal("expected outside path denial")
	}
	if _, err := ResolvePath(ctx, "/dev/random", PathRead); err == nil && runtime.GOOS != "windows" {
		t.Fatal("expected special path denial")
	}

	missing, err := ResolvePath(ctx, "new.txt", PathWrite)
	if err != nil {
		t.Fatal(err)
	}
	if missing != filepath.Join(root, "new.txt") {
		t.Fatalf("missing path = %q", missing)
	}
}

func TestResolvePathAdditionalWorkingDir(t *testing.T) {
	root := t.TempDir()
	extra := t.TempDir()
	path := filepath.Join(extra, "extra.txt")
	if err := os.WriteFile(path, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := DefaultContext(context.Background(), root)
	ctx.AdditionalWorkingDirs = []string{extra}
	got, err := ResolvePath(ctx, path, PathRead)
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("got %q", got)
	}
}

func TestResolvePathRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges vary on Windows")
	}
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	ctx := DefaultContext(context.Background(), root)
	if _, err := ResolvePath(ctx, "link.txt", PathRead); err == nil {
		t.Fatal("expected symlink escape denial")
	}
}

func TestResolvePathRejectsMissingParentForWrite(t *testing.T) {
	root := t.TempDir()
	ctx := DefaultContext(context.Background(), root)
	if _, err := ResolvePath(ctx, "missing/out.txt", PathWrite); err == nil {
		t.Fatal("expected missing parent error")
	}
}
