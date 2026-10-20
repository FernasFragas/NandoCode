package tasks

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/types"
)

type eventRecorder struct {
	mu       sync.Mutex
	events   []WorkerEvent
	finished chan string
}

func newRecorder(sup *Supervisor) *eventRecorder {
	r := &eventRecorder{finished: make(chan string, 1024)}
	sup.SetWorkerObserver(func(e WorkerEvent) {
		r.mu.Lock()
		r.events = append(r.events, e)
		r.mu.Unlock()
		if e.Type == "finished" {
			r.finished <- e.Worker.TaskID
		}
	})
	return r
}

func (r *eventRecorder) types(id string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, e := range r.events {
		if e.Type == "message" {
			if e.Message.ToAgent == id {
				out = append(out, "message")
			}
			continue
		}
		if e.Worker.TaskID == id {
			out = append(out, e.Type)
		}
	}
	return out
}

func (r *eventRecorder) waitFinished(t *testing.T, id string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case got := <-r.finished:
			if got == id {
				return
			}
		case <-deadline:
			t.Fatalf("no finished event for %s; events=%v", id, r.types(id))
		}
	}
}

func assertStartedFirstFinishedOnceLast(t *testing.T, got []string) {
	t.Helper()
	if len(got) < 2 || got[0] != "started" || got[len(got)-1] != "finished" {
		t.Fatalf("events=%v want started...finished", got)
	}
	n := 0
	for _, g := range got {
		if g == "finished" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("events=%v want exactly one finished", got)
	}
}

func startBlocked(t *testing.T, sup *Supervisor, run RunFunc) string {
	t.Helper()
	id, err := sup.Start(context.Background(), types.KindAgent, "task", run)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSetWorkerMetaAfterKillStillFinishes(t *testing.T) {
	sup := NewSupervisor(t.TempDir(), nil)
	rec := newRecorder(sup)
	id := startBlocked(t, sup, func(ctx context.Context, _ *OutputWriter) (int, error) {
		<-ctx.Done()
		return 137, ctx.Err()
	})
	if err := sup.Stop(id); err != nil {
		t.Fatal(err)
	}
	waitForStatus(t, sup, id, types.StatusKilled, 5*time.Second)
	sup.SetWorkerMeta(id, WorkerMeta{Name: "late"})
	rec.waitFinished(t, id)
	assertStartedFirstFinishedOnceLast(t, rec.types(id))
}

func TestWorkerFinishedExactlyOnceOnEveryPath(t *testing.T) {
	cases := map[string]struct {
		run  func(ctx context.Context, release chan struct{}) (int, error)
		stop bool
	}{
		"complete": {run: func(context.Context, chan struct{}) (int, error) { return 0, nil }},
		"error":    {run: func(context.Context, chan struct{}) (int, error) { return 1, errors.New("boom") }},
		"panic":    {run: func(context.Context, chan struct{}) (int, error) { panic("boom") }},
		"stop": {stop: true, run: func(ctx context.Context, _ chan struct{}) (int, error) {
			<-ctx.Done()
			return 137, ctx.Err()
		}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			sup := NewSupervisor(t.TempDir(), nil)
			rec := newRecorder(sup)
			release := make(chan struct{})
			ready := make(chan struct{})
			id := startBlocked(t, sup, func(ctx context.Context, _ *OutputWriter) (int, error) {
				<-ready
				return tc.run(ctx, release)
			})
			sup.SetWorkerMeta(id, WorkerMeta{Name: name})
			if tc.stop {
				_ = sup.Stop(id)
			}
			close(ready)
			rec.waitFinished(t, id)
			sup.ReportWorkerProgress(id, "Bash", "ls", "running")
			sup.ReportWorkerUsage(id, sup.WorkerSnapshots()[0].Usage, 0)
			assertStartedFirstFinishedOnceLast(t, rec.types(id))
		})
	}
}

func TestWorkerMessageDroppedAfterFinish(t *testing.T) {
	sup := NewSupervisor(t.TempDir(), nil)
	rec := newRecorder(sup)
	release := make(chan struct{})
	id := startBlocked(t, sup, func(context.Context, *OutputWriter) (int, error) {
		<-release
		return 0, nil
	})
	sup.SetWorkerMeta(id, WorkerMeta{Name: "w"})
	if err := sup.QueueMessage(id, PendingMessage{FromAgent: "c", ToAgent: id, Summary: "before"}); err != nil {
		t.Fatal(err)
	}
	close(release)
	rec.waitFinished(t, id)
	_ = sup.QueueMessage(id, PendingMessage{FromAgent: "c", ToAgent: id, Summary: "after"})
	got := rec.types(id)
	if strings.Join(got, ",") != "started,message,finished" {
		t.Fatalf("events=%v", got)
	}
}

func TestWorkerMessageSummaryCappedAndSanitized(t *testing.T) {
	sup := NewSupervisor(t.TempDir(), nil)
	rec := newRecorder(sup)
	release := make(chan struct{})
	id := startBlocked(t, sup, func(context.Context, *OutputWriter) (int, error) {
		<-release
		return 0, nil
	})
	defer close(release)
	sup.SetWorkerMeta(id, WorkerMeta{Name: "w"})
	long := "a\x1b[31m\nb\t" + strings.Repeat("é", 500)
	if err := sup.QueueMessage(id, PendingMessage{FromAgent: "c", ToAgent: id, Summary: long, Content: "secret"}); err != nil {
		t.Fatal(err)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	for _, e := range rec.events {
		if e.Type != "message" {
			continue
		}
		s := e.Message.Summary
		if n := len([]rune(s)); n > 200 {
			t.Fatalf("summary has %d runes", n)
		}
		for _, r := range s {
			if r < 0x20 || r == 0x7f {
				t.Fatalf("control char %q in summary", r)
			}
		}
		if e.Message.Content != "" {
			t.Fatal("content must not reach the observer")
		}
		return
	}
	t.Fatal("no message event")
}

func TestWorkerSnapshotsOnlyListTrackedWorkers(t *testing.T) {
	sup := NewSupervisor(t.TempDir(), nil)
	release := make(chan struct{})
	defer close(release)
	run := func(context.Context, *OutputWriter) (int, error) { <-release; return 0, nil }
	tracked := startBlocked(t, sup, run)
	startBlocked(t, sup, run)
	sup.SetWorkerMeta(tracked, WorkerMeta{Name: "tracked"})
	got := sup.WorkerSnapshots()
	if len(got) != 1 || got[0].TaskID != tracked {
		t.Fatalf("snapshots=%+v", got)
	}
}

func TestWorkerEventOrderingStress(t *testing.T) {
	sup := NewSupervisor(t.TempDir(), nil)
	rec := newRecorder(sup)
	const n = 40
	var wg sync.WaitGroup
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		i := i
		ready := make(chan struct{})
		id := startBlocked(t, sup, func(ctx context.Context, _ *OutputWriter) (int, error) {
			<-ready
			switch i % 3 {
			case 0:
				return 0, nil
			case 1:
				return 1, errors.New("boom")
			}
			<-ctx.Done()
			return 137, ctx.Err()
		})
		ids[i] = id
		wg.Add(2)
		go func() {
			defer wg.Done()
			sup.SetWorkerMeta(id, WorkerMeta{Name: "w"})
			sup.ReportWorkerProgress(id, "Bash", "ls", "running")
			sup.ReportWorkerUsage(id, sup.WorkerSnapshots()[0].Usage, 1)
		}()
		go func() {
			defer wg.Done()
			close(ready)
			if i%3 == 2 {
				_ = sup.Stop(id)
			}
		}()
	}
	wg.Wait()
	for _, id := range ids {
		rec.waitFinished(t, id)
		got := rec.types(id)
		assertStartedFirstFinishedOnceLast(t, got)
	}
}
