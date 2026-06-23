package tasks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/bootstrap"
	"github.com/FernasFragas/Nandocode/internal/state"
	"github.com/FernasFragas/Nandocode/internal/types"
)

func TestSupervisorStartComplete(t *testing.T) {
	t.Parallel()
	st := state.NewStore(state.DefaultApp(bootstrap.New(bootstrap.DefaultInitial(".")).Snapshot()), nil)
	s := NewSupervisor(filepath.Join(t.TempDir(), "tasks"), st)
	id, err := s.Start(context.Background(), types.KindBash, "ok", func(ctx context.Context, out *OutputWriter) (int, error) {
		_ = out.WriteText("stdout", "hello")
		return 0, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, ok := s.Get(id)
		if ok && got.ToSummary().Status == types.StatusCompleted {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("task %s did not complete", id)
}

func TestSupervisorStartNonBlocking(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(filepath.Join(t.TempDir(), "tasks"), nil)
	start := time.Now()
	id, err := s.Start(context.Background(), types.KindBash, "slow", func(ctx context.Context, out *OutputWriter) (int, error) {
		time.Sleep(200 * time.Millisecond)
		return 0, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("expected id")
	}
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Fatalf("start took too long: %v", d)
	}
}

func TestSupervisorStop(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(filepath.Join(t.TempDir(), "tasks"), nil)
	id, err := s.Start(context.Background(), types.KindBash, "sleep", func(ctx context.Context, out *OutputWriter) (int, error) {
		<-ctx.Done()
		return 137, ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Stop(id); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, ok := s.Get(id)
		if ok && got.ToSummary().Status == types.StatusKilled {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("task %s did not transition to killed", id)
}

func TestSupervisorFailedTask(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(filepath.Join(t.TempDir(), "tasks"), nil)
	id, err := s.Start(context.Background(), types.KindBash, "fail", func(ctx context.Context, out *OutputWriter) (int, error) {
		return 1, errors.New("boom")
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, ok := s.Get(id)
		if ok && got.ToSummary().Status == types.StatusFailed {
			if got.ToSummary().Err == "" {
				t.Fatal("expected error summary")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("task did not transition to failed")
}

func TestBashRunFuncNonZeroExitIsCompletedWithExitCode(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(filepath.Join(t.TempDir(), "tasks"), nil)
	id, err := s.Start(context.Background(), types.KindBash, "nonzero", BashRunFunc("exit 7", ""))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, ok := s.Get(id)
		if !ok {
			t.Fatal("task missing")
		}
		sum := got.ToSummary()
		if sum.Status == types.StatusCompleted {
			if sum.ExitCode != 7 {
				t.Fatalf("expected exit code 7, got %d", sum.ExitCode)
			}
			return
		}
		if sum.Status == types.StatusFailed {
			t.Fatalf("expected completed status for non-zero command, got failed: %+v", sum)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("non-zero bash task did not complete")
}

func TestSupervisorStopErrors(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(filepath.Join(t.TempDir(), "tasks"), nil)
	if err := s.Stop("missing"); err == nil {
		t.Fatal("expected missing task stop error")
	}
	id, err := s.Start(context.Background(), types.KindBash, "done", func(ctx context.Context, out *OutputWriter) (int, error) {
		return 0, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForStatus(t, s, id, types.StatusCompleted, 2*time.Second)
	if err := s.Stop(id); err == nil {
		t.Fatal("expected stop error for completed task")
	}
}

func TestSupervisorListSortedAndConcurrentStarts(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(filepath.Join(t.TempDir(), "tasks"), nil)
	block := make(chan struct{})
	var wg sync.WaitGroup
	ids := make(chan string, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := s.Start(context.Background(), types.KindBash, "x", func(ctx context.Context, out *OutputWriter) (int, error) {
				<-block
				return 0, nil
			})
			if err != nil {
				t.Errorf("start error: %v", err)
				return
			}
			ids <- id
		}()
	}
	wg.Wait()
	close(block)
	close(ids)
	seen := map[string]struct{}{}
	for id := range ids {
		if _, ok := seen[id]; ok {
			t.Fatalf("duplicate id %s", id)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != 100 {
		t.Fatalf("expected 100 unique IDs, got %d", len(seen))
	}
	list := s.List()
	for i := 1; i < len(list); i++ {
		if list[i-1].ToSummary().CreatedAt.After(list[i].ToSummary().CreatedAt) {
			t.Fatal("list is not sorted by creation time ascending")
		}
	}
}

func TestOutputReadableDuringExecution(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(filepath.Join(t.TempDir(), "tasks"), nil)
	id, err := s.Start(context.Background(), types.KindBash, "stream", func(ctx context.Context, out *OutputWriter) (int, error) {
		for i := 0; i < 5; i++ {
			if err := out.WriteText("stdout", "tick"); err != nil {
				return 1, err
			}
			time.Sleep(30 * time.Millisecond)
		}
		return 0, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(600 * time.Millisecond)
	readAny := false
	for time.Now().Before(deadline) {
		st, ok := s.Get(id)
		if !ok {
			t.Fatal("task disappeared")
		}
		file := st.ToSummary().OutputFile
		if file == "" {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		lines, err := TailLines(file, 10)
		if err == nil && len(lines) > 0 {
			readAny = true
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if !readAny {
		t.Fatal("expected to read output lines while task running")
	}
}

func TestSupervisorPublishesTransitionsToStore(t *testing.T) {
	t.Parallel()
	st := state.NewStore(state.DefaultApp(bootstrap.New(bootstrap.DefaultInitial(".")).Snapshot()), nil)
	before := st.Get()
	if len(before.Tasks) != 0 {
		t.Fatal("expected empty initial tasks")
	}
	s := NewSupervisor(filepath.Join(t.TempDir(), "tasks"), st)
	id, err := s.Start(context.Background(), types.KindBash, "ok", func(ctx context.Context, out *OutputWriter) (int, error) {
		time.Sleep(30 * time.Millisecond)
		return 0, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForStatus(t, s, id, types.StatusCompleted, 2*time.Second)
	got := st.Get()
	summary, ok := got.Tasks[id]
	if !ok {
		t.Fatalf("expected task summary in app state for %s", id)
	}
	if summary.Status != types.StatusCompleted {
		t.Fatalf("expected completed summary, got %q", summary.Status)
	}
	if len(before.Tasks) != 0 {
		t.Fatal("expected old snapshot to remain unchanged")
	}
}

func TestSupervisorQueueAndDrainMessages(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(filepath.Join(t.TempDir(), "tasks"), nil)
	id, err := s.Start(context.Background(), types.KindAgent, "agent", func(ctx context.Context, out *OutputWriter) (int, error) {
		<-ctx.Done()
		return 137, ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := PendingMessage{ID: "m1", FromAgent: "coordinator", ToAgent: id, Content: "ping"}
	if err := s.QueueMessage(id, msg); err != nil {
		t.Fatal(err)
	}
	drained, err := s.DrainMessages(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(drained) != 1 {
		t.Fatalf("drained=%d want=1", len(drained))
	}
	if drained[0].Content != "ping" {
		t.Fatalf("unexpected content %q", drained[0].Content)
	}
	if err := s.Stop(id); err != nil {
		t.Fatal(err)
	}
}

func TestSupervisorQueueMessagePersistsMailboxJSONL(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "tasks")
	s := NewSupervisor(root, nil)
	id, err := s.Start(context.Background(), types.KindAgent, "agent", func(ctx context.Context, out *OutputWriter) (int, error) {
		<-ctx.Done()
		return 137, ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.QueueMessage(id, PendingMessage{ID: "m1", Content: "x"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, id+".mailbox.jsonl")
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := st.Mode().Perm(); got != 0o600 {
		t.Fatalf("mailbox mode=%o want=600", got)
	}
	if err := s.Stop(id); err != nil {
		t.Fatal(err)
	}
}

func TestSupervisorNameRegistry(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(filepath.Join(t.TempDir(), "tasks"), nil)
	id, err := s.Start(context.Background(), types.KindAgent, "agent", func(ctx context.Context, out *OutputWriter) (int, error) {
		<-ctx.Done()
		return 137, ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RegisterName("worker-a", id); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.LookupByName("worker-a"); !ok {
		t.Fatal("expected name lookup success")
	}
	if err := s.RegisterName("worker-a", "another"); err == nil {
		t.Fatal("expected duplicate live name error")
	}
	s.UnregisterName("worker-a")
	if _, ok := s.LookupByName("worker-a"); ok {
		t.Fatal("expected lookup miss after unregister")
	}
	if err := s.Stop(id); err != nil {
		t.Fatal(err)
	}
}

func TestActiveWorkerCount(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(filepath.Join(t.TempDir(), "tasks"), nil)
	id1, err := s.Start(context.Background(), types.KindAgent, "a1", func(ctx context.Context, out *OutputWriter) (int, error) {
		<-ctx.Done()
		return 137, ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	id2, err := s.Start(context.Background(), types.KindBash, "b1", func(ctx context.Context, out *OutputWriter) (int, error) {
		<-ctx.Done()
		return 137, ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := s.ActiveWorkerCount(); got != 1 {
		t.Fatalf("count=%d want=1", got)
	}
	_ = s.Stop(id1)
	_ = s.Stop(id2)
}

func waitForStatus(t *testing.T, s *Supervisor, id string, want types.TaskStatus, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		got, ok := s.Get(id)
		if ok && got.ToSummary().Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("task %s did not reach status %s in %s", id, want, timeout)
}
