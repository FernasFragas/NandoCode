package bash

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/FernasFragas/nandocodego/internal/tools"
)

func TestBashExecution(t *testing.T) {
	dir := t.TempDir()
	ctx := tools.DefaultContext(context.Background(), dir)
	result, err := NewBashTool().Call(ctx, Input{Command: "printf hello"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := result.Data.(Output)
	if out.ExitCode != 0 || out.Stdout != "hello" {
		t.Fatalf("out = %#v", out)
	}
}

func TestBashCapturesStderrAndNonZeroExit(t *testing.T) {
	dir := t.TempDir()
	ctx := tools.DefaultContext(context.Background(), dir)
	result, err := NewBashTool().Call(ctx, Input{Command: "printf bad >&2; exit 7"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := result.Data.(Output)
	if out.ExitCode != 7 || !strings.Contains(out.Stderr, "bad") {
		t.Fatalf("out = %#v", out)
	}
}

func TestBashCancellation(t *testing.T) {
	dir := t.TempDir()
	ctx := tools.DefaultContext(context.Background(), dir)
	ctx.BashTimeout = 20 * time.Millisecond
	result, err := NewBashTool().Call(ctx, Input{Command: "sleep 1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := result.Data.(Output)
	if out.ExitCode != -1 {
		t.Fatalf("out = %#v", out)
	}
}

func TestBashWorkingDirAndEnv(t *testing.T) {
	dir := t.TempDir()
	ctx := tools.DefaultContext(context.Background(), dir)
	result, err := NewBashTool().Call(ctx, Input{Command: "printf \"$PWD:$PHASE3_VALUE\"", Env: map[string]string{"PHASE3_VALUE": "ok"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := result.Data.(Output)
	if !strings.Contains(out.Stdout, dir+":ok") {
		t.Fatalf("stdout = %q", out.Stdout)
	}
}

func TestBashProgress(t *testing.T) {
	dir := t.TempDir()
	ctx := tools.DefaultContext(context.Background(), dir)
	progress := make(chan tools.ProgressEvent, 4)
	result, err := NewBashTool().Call(ctx, Input{Command: "printf hello"}, progress)
	if err != nil {
		t.Fatal(err)
	}
	if result.Data.(Output).Stdout != "hello" {
		t.Fatalf("result = %#v", result.Data)
	}
	close(progress)
	seen := false
	for event := range progress {
		if event.Stream == "stdout" && event.Message == "hello" {
			seen = true
		}
	}
	if !seen {
		t.Fatal("expected stdout progress event")
	}
}

func TestBashUnmarshalInput(t *testing.T) {
	input, err := NewBashTool().UnmarshalInput(json.RawMessage(`{"command":"ls","timeout_ms":10,"env":{"A":"B"}}`))
	if err != nil {
		t.Fatal(err)
	}
	got := input.(Input)
	if got.Command != "ls" || got.TimeoutMS != 10 || got.Env["A"] != "B" {
		t.Fatalf("got %#v", got)
	}
	if _, err := NewBashTool().UnmarshalInput(json.RawMessage(`{"command":""}`)); err == nil {
		t.Fatal("expected empty command error")
	}
	if _, err := NewBashTool().UnmarshalInput(json.RawMessage(`{"command":"ls","env":{"1BAD":"x"}}`)); err == nil {
		t.Fatal("expected invalid env error")
	}
}
