package server

import "github.com/FernasFragas/Nandocode/internal/tasks"

// emitSubagentEvent maps supervisor facts onto the frozen browser SSE contract.
func (s *Session) emitSubagentEvent(e tasks.WorkerEvent) {
	w := e.Worker
	switch e.Type {
	case "started":
		s.Emit("subagent_started", map[string]any{"task_id": w.TaskID, "name": w.Name, "task": w.Task, "model": w.Model, "parent": w.Parent, "started_at": w.StartedAt})
	case "progress":
		s.Emit("subagent_progress", map[string]any{"task_id": w.TaskID, "tool": e.Tool, "target": e.Target, "status": e.ProgressStatus})
	case "usage":
		s.Emit("subagent_usage", map[string]any{"task_id": w.TaskID, "usage": w.Usage, "files_touched": w.FilesTouched})
	case "message":
		m := e.Message
		s.Emit("subagent_message", map[string]any{"from": m.FromAgent, "to": m.ToAgent, "summary": m.Summary, "at": m.Timestamp.UTC().Format("2006-01-02T15:04:05Z")})
	case "finished":
		s.Emit("subagent_finished", map[string]any{"task_id": w.TaskID, "status": w.Status, "finished_at": w.FinishedAt, "error": w.Error})
	}
}
