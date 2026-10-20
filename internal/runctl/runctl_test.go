package runctl

import (
	"context"
	"errors"
	"testing"
	"time"
)

func recv(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestControllerCancelEndsContextAndAbort(t *testing.T) {
	c := New()
	run, err := c.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if !c.Active() {
		t.Fatal("controller should be active after Begin")
	}
	if !c.Cancel() {
		t.Fatal("Cancel should report that a run was canceled")
	}
	recv(t, run.Ctx.Done(), "run ctx")
	recv(t, run.ParentAbort, "parent abort")
}

func TestControllerCancelWithoutRunIsNoop(t *testing.T) {
	c := New()
	if c.Cancel() {
		t.Fatal("Cancel with no run must return false")
	}
}

func TestControllerCancelIsIdempotent(t *testing.T) {
	c := New()
	if _, err := c.Begin(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !c.Cancel() {
		t.Fatal("first Cancel should return true")
	}
	c.Cancel() // must not panic
}

func TestControllerParentCancelPropagates(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	c := New()
	run, err := c.Begin(parent)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	recv(t, run.ParentAbort, "parent abort after parent cancel")
}

func TestControllerBeginWhileActiveFails(t *testing.T) {
	c := New()
	if _, err := c.Begin(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Begin(context.Background()); !errors.Is(err, ErrRunActive) {
		t.Fatalf("second Begin err = %v, want ErrRunActive", err)
	}
}

func TestControllerFinishReleasesAndCancelsContext(t *testing.T) {
	c := New()
	run, _ := c.Begin(context.Background())
	c.Finish(run)
	if c.Active() {
		t.Fatal("Active after Finish")
	}
	recv(t, run.Ctx.Done(), "ctx canceled by Finish (no leak)")
	if _, err := c.Begin(context.Background()); err != nil {
		t.Fatalf("Begin after Finish: %v", err)
	}
}

func TestControllerStaleFinishDoesNotReleaseNewRun(t *testing.T) {
	c := New()
	old, _ := c.Begin(context.Background())
	c.Finish(old)
	fresh, _ := c.Begin(context.Background())
	c.Finish(old) // stale
	if !c.Active() {
		t.Fatal("stale Finish released the new run")
	}
	select {
	case <-fresh.Ctx.Done():
		t.Fatal("stale Finish canceled the new run")
	default:
	}
}

func TestControllerRequestCompact(t *testing.T) {
	c := New()
	if got := c.RequestCompact(); got != CompactNoRun {
		t.Fatalf("no run: got %v want CompactNoRun", got)
	}
	run, _ := c.Begin(context.Background())
	if got := c.RequestCompact(); got != CompactRequested {
		t.Fatalf("first: got %v want CompactRequested", got)
	}
	if got := c.RequestCompact(); got != CompactPending {
		t.Fatalf("second: got %v want CompactPending", got)
	}
	recv(t, run.CompactRequest, "compact signal")
	if got := c.RequestCompact(); got != CompactRequested {
		t.Fatalf("after consume: got %v want CompactRequested", got)
	}
}

func TestControllerRunKind(t *testing.T) {
	c := New()
	run, _ := c.Begin(context.Background())
	if c.Kind() != KindMain {
		t.Fatalf("default kind = %q", c.Kind())
	}
	c.Finish(run)
	if _, err := c.BeginKind(context.Background(), KindSide); err != nil {
		t.Fatal(err)
	}
	if c.Kind() != KindSide {
		t.Fatalf("kind = %q want side", c.Kind())
	}
}
