package server

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/bootstrap"
	"github.com/FernasFragas/Nandocode/internal/state"
	"github.com/FernasFragas/Nandocode/internal/tasks"
	"github.com/FernasFragas/Nandocode/internal/types"
)

func subagentTestSession(t *testing.T) (*Session, *tasks.Supervisor) {
	t.Helper()
	s, sup, _ := subagentTestSessionStore(t)
	return s, sup
}

func subagentTestSessionStore(t *testing.T) (*Session, *tasks.Supervisor, *state.Store[state.App]) {
	t.Helper()
	t.Setenv("NANDOCODEGO_DATA_HOME", t.TempDir())
	app := state.DefaultApp(bootstrap.New(bootstrap.DefaultInitial(t.TempDir())).Snapshot())
	store := state.NewStore(app, nil)
	sup := tasks.NewSupervisor(t.TempDir(), store)
	s := newSession(context.Background(), "subagent-test", app, fakeRunner{})
	s.setCoordinatorRuntime(sup, store, nil, false)
	t.Cleanup(s.stop)
	return s, sup, store
}

func startObservedWorker(t *testing.T, sup *tasks.Supervisor) (string, chan struct{}) {
	t.Helper()
	release := make(chan struct{})
	id, err := sup.Start(context.Background(), types.KindAgent, "inspect repository", func(context.Context, *tasks.OutputWriter) (int, error) {
		<-release
		return 0, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sup.SetWorkerMeta(id, tasks.WorkerMeta{Name: "researcher", Task: "inspect repository", Model: "test-model", Parent: "coordinator"})
	return id, release
}

func TestSubagentLifecycleEvents(t *testing.T) {
	s, sup := subagentTestSession(t)
	id, release := startObservedWorker(t, sup)
	sup.ReportWorkerProgress(id, "FileRead", "internal/main.go", "running")
	sup.ReportWorkerProgress(id, "FileRead", "internal/main.go", "completed")
	close(release)
	deadline := time.After(2 * time.Second)
	for {
		events := s.Replay("")
		var got []string
		for _, e := range events {
			if strings.HasPrefix(e.Type, "subagent_") {
				got = append(got, e.Type)
			}
		}
		if len(got) >= 4 {
			want := []string{"subagent_started", "subagent_progress", "subagent_progress", "subagent_finished"}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("events=%v want=%v", got, want)
				}
			}
			return
		}
		select {
		case <-deadline:
			t.Fatalf("events=%v", got)
		default:
		}
	}
}

func TestSubagentUsageForwarded(t *testing.T) {
	s, sup := subagentTestSession(t)
	id, release := startObservedWorker(t, sup)
	want := agent.Usage{PromptEvalCount: 11, EvalCount: 7, Turns: 2, ToolCalls: 1}
	sup.ReportWorkerUsage(id, want, 2)
	for _, e := range s.Replay("") {
		if e.Type == "subagent_usage" && e.Data["task_id"] == id && e.Data["usage"] == want && e.Data["files_touched"] == 2 {
			close(release)
			return
		}
	}
	close(release)
	t.Fatal("worker terminal usage was not forwarded")
}

func TestSubagentMessageEventOmitsContent(t *testing.T) {
	s, sup := subagentTestSession(t)
	id, release := startObservedWorker(t, sup)
	if err := sup.QueueMessage(id, tasks.PendingMessage{FromAgent: "coordinator", ToAgent: id, Summary: "check tests", Content: "private full message"}); err != nil {
		t.Fatal(err)
	}
	close(release)
	for _, e := range s.Replay("") {
		if e.Type == "subagent_message" {
			if e.Data["from"] != "coordinator" || e.Data["to"] != id || e.Data["summary"] != "check tests" {
				t.Fatalf("message=%v", e.Data)
			}
			if _, ok := e.Data["content"]; ok {
				t.Fatalf("content leaked: %v", e.Data)
			}
			return
		}
	}
	t.Fatal("missing subagent_message")
}

func TestCoordinatorWorkersListedInSessionView(t *testing.T) {
	s, sup := subagentTestSession(t)
	id, release := startObservedWorker(t, sup)
	v := s.view()
	if len(v.Workers) != 1 || v.Workers[0].TaskID != id || v.Workers[0].Name != "researcher" || v.Workers[0].Status != types.StatusRunning {
		t.Fatalf("workers=%+v", v.Workers)
	}
	close(release)
}

func TestSubagentEventsSkippedWithoutStartedMeta(t *testing.T) {
	s, sup, store := subagentTestSessionStore(t)
	release := make(chan struct{})
	id, err := sup.Start(context.Background(), types.KindAgent, "unannounced", func(context.Context, *tasks.OutputWriter) (int, error) {
		<-release
		return 0, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sup.ReportWorkerProgress(id, "Bash", "ls", "running")
	sup.ReportWorkerUsage(id, agent.Usage{Turns: 1}, 0)
	close(release)
	// The store publish follows the worker-finished decision in the
	// supervisor, so once it shows a terminal state nothing more can arrive.
	deadline := time.After(5 * time.Second)
	for store.Get().Tasks[id].Status != types.StatusCompleted {
		select {
		case <-deadline:
			t.Fatal("task never completed")
		default:
			runtime.Gosched()
		}
	}
	for _, e := range s.Replay("") {
		if strings.HasPrefix(e.Type, "subagent_") {
			t.Fatalf("event %s emitted for a worker that was never announced", e.Type)
		}
	}
}
