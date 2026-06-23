package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/llm"
)

func TestSubagentParamsZeroValueValid(t *testing.T) {
	t.Parallel()
	var p SubagentParams
	if p.Mode != "" || p.Task != "" || p.Model != "" || p.MaxTurns != 0 || p.Background {
		t.Fatal("expected zero-value params")
	}
}

func TestRunSubagentRejectsEmptyTask(t *testing.T) {
	t.Parallel()
	_, err := runSubagent(context.Background(), Input{}, SubagentParams{}, newFakeClient(nil), nil, DefaultConfig())
	if err == nil || !strings.Contains(err.Error(), "task is required") {
		t.Fatalf("expected task-required error, got %v", err)
	}
}

func TestRunSubagentRejectsRecursion(t *testing.T) {
	t.Parallel()
	_, err := runSubagent(context.Background(), Input{IsSubagent: true}, SubagentParams{Task: "x"}, newFakeClient(nil), nil, DefaultConfig())
	if err == nil || !strings.Contains(err.Error(), "recursion") {
		t.Fatalf("expected recursion error, got %v", err)
	}
}

func TestRunSubagentReturnsTaskID(t *testing.T) {
	t.Setenv("NANDOCODEGO_DATA_HOME", t.TempDir())
	client := newFakeClient([]fakeTurn{textTurn("done")})
	id, err := runSubagent(context.Background(), Input{Model: "test"}, SubagentParams{Task: "summarize", Background: true, SessionID: "s1"}, client, nil, DefaultConfig())
	if err != nil {
		t.Fatalf("runSubagent failed: %v", err)
	}
	if len(id) == 0 {
		t.Fatal("expected non-empty task id")
	}
	p := filepath.Join(os.Getenv("NANDOCODEGO_DATA_HOME"), "sessions", "s1", "tasks", id+".jsonl")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("expected task output file: %v", err)
	}
}

func TestRunSubagentBackgroundWritesThinkingJSONL(t *testing.T) {
	t.Setenv("NANDOCODEGO_DATA_HOME", t.TempDir())
	client := newFakeClient([]fakeTurn{thinkingTurn("working it out", "done")})
	id, err := runSubagent(context.Background(), Input{Model: "test"}, SubagentParams{
		Task:       "summarize",
		Background: true,
		SessionID:  "s1",
	}, client, nil, DefaultConfig())
	if err != nil {
		t.Fatalf("runSubagent failed: %v", err)
	}
	p := filepath.Join(os.Getenv("NANDOCODEGO_DATA_HOME"), "sessions", "s1", "tasks", id+".jsonl")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read task output: %v", err)
	}
	out := string(b)
	if !strings.Contains(out, `"kind":"thinking"`) || !strings.Contains(out, `"thinking":"working it out"`) {
		t.Fatalf("missing thinking JSONL entry:\n%s", out)
	}
}

func TestRunSubagentParentAbortCancelsChild(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	parentAbort := make(chan struct{})

	done := make(chan error, 1)
	go func() {
		client := newFakeClient([]fakeTurn{{wait: 300 * time.Millisecond, events: []llm.StreamEvent{{Done: true, DoneReason: "stop"}}}})
		_, err := runSubagent(ctx, Input{ParentAbort: parentAbort, Model: "test"}, SubagentParams{Task: "x"}, client, nil, DefaultConfig())
		done <- err
	}()

	close(parentAbort)

	select {
	case <-done:
		// child must return quickly when parent abort fires.
	case <-time.After(200 * time.Millisecond):
		t.Fatal("sub-agent did not react within 200ms")
	}
}

func TestRunSubagentChildCancelDoesNotCancelParentContext(t *testing.T) {
	t.Parallel()
	parentCtx, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()
	parentAbort := make(chan struct{})

	client := newFakeClient([]fakeTurn{{wait: 300 * time.Millisecond, events: []llm.StreamEvent{{Done: true, DoneReason: "stop"}}}})
	done := make(chan error, 1)
	go func() {
		_, err := runSubagent(parentCtx, Input{ParentAbort: parentAbort, Model: "test"}, SubagentParams{Task: "x"}, client, nil, DefaultConfig())
		done <- err
	}()

	close(parentAbort)
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("expected child to stop after parent abort")
	}
	if parentCtx.Err() != nil {
		t.Fatalf("parent context should remain active, got %v", parentCtx.Err())
	}
}

func TestRunSubagentAbortedReturnsError(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := newFakeClient([]fakeTurn{textTurn("x")})
	_, err := runSubagent(ctx, Input{Model: "test"}, SubagentParams{Task: "x"}, client, nil, DefaultConfig())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunSubagentMaxTurnsReturnsError(t *testing.T) {
	t.Parallel()
	client := newFakeClient([]fakeTurn{
		toolCallTurn("UnknownTool", map[string]any{"a": "b"}),
		toolCallTurn("UnknownTool", map[string]any{"a": "b"}),
	})
	_, err := runSubagent(context.Background(), Input{Model: "test"}, SubagentParams{Task: "x", MaxTurns: 1}, client, nil, DefaultConfig())
	if err == nil {
		t.Fatal("expected max turns error")
	}
}

func TestRunSubagentRaisesLowInheritedMaxTurns(t *testing.T) {
	t.Parallel()
	client := newFakeClient([]fakeTurn{
		toolCallTurn("UnknownTool", map[string]any{"a": "b"}),
		toolCallTurn("UnknownTool", map[string]any{"a": "b"}),
		textTurn("done"),
	})
	cfg := DefaultConfig()
	cfg.MaxTurns = 1
	out, err := runSubagent(context.Background(), Input{Model: "test"}, SubagentParams{Task: "x"}, client, nil, cfg)
	if err != nil {
		t.Fatalf("runSubagent failed: %v", err)
	}
	if out != "done" {
		t.Fatalf("output=%q, want done", out)
	}
}
