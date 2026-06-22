package tasks

import (
	"time"

	"github.com/FernasFragas/nandocodego/internal/types"
)

type TaskState interface {
	isTaskState()
	ToSummary() types.TaskSummary
}

type PendingTask struct {
	ID          string
	Kind        types.TaskKind
	Description string
	CreatedAt   time.Time
}

type RunningTask struct {
	PendingTask
	StartedAt  time.Time
	OutputFile string
	AgentMeta  *AgentTaskMetadata
}

type AgentTaskMetadata struct {
	Mailbox     *Mailbox
	MailboxFile string
	AgentName   string
}

type CompletedTask struct {
	RunningTask
	FinishedAt time.Time
	ExitCode   int
}

type FailedTask struct {
	RunningTask
	FinishedAt time.Time
	Err        error
}

type KilledTask struct {
	RunningTask
	FinishedAt time.Time
}

func (PendingTask) isTaskState()   {}
func (RunningTask) isTaskState()   {}
func (CompletedTask) isTaskState() {}
func (FailedTask) isTaskState()    {}
func (KilledTask) isTaskState()    {}

func (s PendingTask) ToSummary() types.TaskSummary {
	return types.TaskSummary{
		ID:          s.ID,
		Kind:        s.Kind,
		Description: s.Description,
		Status:      types.StatusPending,
		CreatedAt:   s.CreatedAt,
	}
}

func (s RunningTask) ToSummary() types.TaskSummary {
	out := s.PendingTask.ToSummary()
	out.Status = types.StatusRunning
	out.StartedAt = s.StartedAt
	out.OutputFile = s.OutputFile
	return out
}

func (s CompletedTask) ToSummary() types.TaskSummary {
	out := s.RunningTask.ToSummary()
	out.Status = types.StatusCompleted
	out.FinishedAt = s.FinishedAt
	out.ExitCode = s.ExitCode
	return out
}

func (s FailedTask) ToSummary() types.TaskSummary {
	out := s.RunningTask.ToSummary()
	out.Status = types.StatusFailed
	out.FinishedAt = s.FinishedAt
	if s.Err != nil {
		out.Err = s.Err.Error()
	}
	return out
}

func (s KilledTask) ToSummary() types.TaskSummary {
	out := s.RunningTask.ToSummary()
	out.Status = types.StatusKilled
	out.FinishedAt = s.FinishedAt
	return out
}
