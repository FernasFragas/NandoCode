package types

import "time"

type TaskKind string

const (
	KindBash   TaskKind = "b"
	KindAgent  TaskKind = "a"
	KindMCP    TaskKind = "m"
	KindRemote TaskKind = "r"
	KindDream  TaskKind = "d"
)

type TaskStatus string

const (
	StatusPending   TaskStatus = "pending"
	StatusRunning   TaskStatus = "running"
	StatusCompleted TaskStatus = "completed"
	StatusFailed    TaskStatus = "failed"
	StatusKilled    TaskStatus = "killed"
)

type TaskSummary struct {
	ID          string
	Kind        TaskKind
	Description string
	Status      TaskStatus
	CreatedAt   time.Time
	StartedAt   time.Time
	FinishedAt  time.Time
	ExitCode    int
	OutputFile  string
	Err         string
}
