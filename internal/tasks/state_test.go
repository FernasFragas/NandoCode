package tasks

import (
	"errors"
	"testing"
	"time"

	"github.com/FernasFragas/nandocodego/internal/types"
)

func TestTaskStateToSummaryStatuses(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	base := PendingTask{ID: "id1", Kind: types.KindBash, Description: "d", CreatedAt: now}
	running := RunningTask{PendingTask: base, StartedAt: now, OutputFile: "out.jsonl"}

	cases := []struct {
		name string
		st   TaskState
		want types.TaskStatus
	}{
		{"pending", base, types.StatusPending},
		{"running", running, types.StatusRunning},
		{"completed", CompletedTask{RunningTask: running, FinishedAt: now, ExitCode: 0}, types.StatusCompleted},
		{"failed", FailedTask{RunningTask: running, FinishedAt: now, Err: errors.New("boom")}, types.StatusFailed},
		{"killed", KilledTask{RunningTask: running, FinishedAt: now}, types.StatusKilled},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.st.ToSummary()
			if got.Status != tc.want {
				t.Fatalf("status=%q want=%q", got.Status, tc.want)
			}
		})
	}
}

func TestTaskStateTypeSwitchCoversAll(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	base := PendingTask{ID: "id", Kind: types.KindAgent, Description: "x", CreatedAt: now}
	states := []TaskState{
		base,
		RunningTask{PendingTask: base, StartedAt: now},
		CompletedTask{RunningTask: RunningTask{PendingTask: base, StartedAt: now}, FinishedAt: now},
		FailedTask{RunningTask: RunningTask{PendingTask: base, StartedAt: now}, FinishedAt: now, Err: errors.New("e")},
		KilledTask{RunningTask: RunningTask{PendingTask: base, StartedAt: now}, FinishedAt: now},
	}
	for _, st := range states {
		switch st.(type) {
		case PendingTask, RunningTask, CompletedTask, FailedTask, KilledTask:
		default:
			t.Fatalf("uncovered state type %T", st)
		}
	}
}
