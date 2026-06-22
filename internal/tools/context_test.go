package tools

import (
	"context"
	"testing"
	"time"
)

func TestDefaultContext(t *testing.T) {
	dir := t.TempDir()
	ctx := DefaultContext(context.Background(), dir)
	if ctx.WorkingDir == "" {
		t.Fatal("WorkingDir is empty")
	}
	if ctx.EffectiveContext() == nil {
		t.Fatal("EffectiveContext is nil")
	}
	if ctx.EffectiveBashTimeout() != 30*time.Second {
		t.Fatalf("BashTimeout = %s", ctx.EffectiveBashTimeout())
	}
	if ctx.EffectiveMaxReadChars() <= 0 || ctx.EffectiveMaxResultChars() <= 0 {
		t.Fatal("limits are not set")
	}
	if ctx.EffectiveMaxDirFiles() <= 0 || ctx.EffectiveMaxPromptFiles() <= 0 {
		t.Fatal("directory file caps are not set")
	}
	if ctx.EffectiveMaxDirBytes() <= 0 || ctx.EffectiveMaxPromptBytes() <= 0 {
		t.Fatal("directory byte caps are not set")
	}
	if ctx.EffectiveMaxDirDepth() <= 0 {
		t.Fatal("directory depth cap is not set")
	}
}

func TestContextFallbacks(t *testing.T) {
	ctx := Context{}
	if ctx.EffectiveContext() == nil {
		t.Fatal("EffectiveContext is nil")
	}
	if ctx.EffectiveBashTimeout() != 30*time.Second {
		t.Fatalf("unexpected timeout: %s", ctx.EffectiveBashTimeout())
	}
}
