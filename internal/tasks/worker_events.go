package tasks

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/types"
)

// WorkerMeta contains the launch details that are absent from TaskSummary.
type WorkerMeta struct {
	Name         string
	Task         string
	Model        string
	Parent       string
	usage        agent.Usage
	filesTouched int
	finished     bool
}

type WorkerSnapshot struct {
	TaskID       string           `json:"task_id"`
	Name         string           `json:"name"`
	Task         string           `json:"task"`
	Model        string           `json:"model"`
	Parent       string           `json:"parent"`
	Status       types.TaskStatus `json:"status"`
	StartedAt    string           `json:"started_at"`
	FinishedAt   string           `json:"finished_at,omitempty"`
	Error        string           `json:"error,omitempty"`
	Usage        agent.Usage      `json:"usage"`
	FilesTouched int              `json:"files_touched"`
}

type WorkerEvent struct {
	Type           string
	Worker         WorkerSnapshot
	Tool           string
	Target         string
	ProgressStatus string
	Message        PendingMessage
}

func (s *Supervisor) SetWorkerObserver(fn func(WorkerEvent)) {
	s.mu.Lock()
	s.workerObserver = fn
	s.mu.Unlock()
}

// SetWorkerMeta announces a worker. If the task already ended (a stop or a
// parent cancel landed between Start and this call) it emits started and then
// the one finished, so every started has exactly one finished.
func (s *Supervisor) SetWorkerMeta(id string, meta WorkerMeta) {
	s.emitMu.Lock()
	defer s.emitMu.Unlock()
	s.mu.Lock()
	st, ok := s.tasks[id]
	if !ok || st.ToSummary().Kind != types.KindAgent {
		s.mu.Unlock()
		return
	}
	sum := st.ToSummary()
	terminal := sum.Status != types.StatusPending && sum.Status != types.StatusRunning
	meta.finished = terminal
	s.workerMeta[id] = meta
	worker := s.workerSnapshotLocked(id, sum)
	fn := s.workerObserver
	s.mu.Unlock()
	if fn == nil {
		return
	}
	fn(WorkerEvent{Type: "started", Worker: worker})
	if terminal {
		fn(WorkerEvent{Type: "finished", Worker: worker})
	}
}

// WorkerSnapshots lists the tracked workers (those announced with
// SetWorkerMeta), the same set the subagent events describe.
func (s *Supervisor) WorkerSnapshots() []WorkerSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]WorkerSnapshot, 0)
	for id, st := range s.tasks {
		sum := st.ToSummary()
		if _, tracked := s.workerMeta[id]; tracked && sum.Kind == types.KindAgent {
			out = append(out, s.workerSnapshotLocked(id, sum))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt < out[j].StartedAt })
	return out
}

func (s *Supervisor) workerSnapshotLocked(id string, sum types.TaskSummary) WorkerSnapshot {
	m := s.workerMeta[id]
	w := WorkerSnapshot{TaskID: id, Name: m.Name, Task: m.Task, Model: m.Model, Parent: m.Parent, Status: sum.Status, StartedAt: utcTime(sum.StartedAt), FinishedAt: utcTime(sum.FinishedAt), Error: sum.Err, Usage: m.usage, FilesTouched: m.filesTouched}
	if w.Task == "" {
		w.Task = sum.Description
	}
	return w
}

func utcTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func (s *Supervisor) ReportWorkerProgress(id, tool, target, status string) {
	s.emitMu.Lock()
	defer s.emitMu.Unlock()
	s.mu.RLock()
	st, ok := s.tasks[id]
	m, tracked := s.workerMeta[id]
	fn := s.workerObserver
	var w WorkerSnapshot
	if ok && tracked && !m.finished {
		w = s.workerSnapshotLocked(id, st.ToSummary())
	} else {
		ok = false
	}
	s.mu.RUnlock()
	if ok && fn != nil {
		fn(WorkerEvent{Type: "progress", Worker: w, Tool: tool, Target: target, ProgressStatus: status})
	}
}

func (s *Supervisor) ReportWorkerUsage(id string, usage agent.Usage, filesTouched int) {
	s.emitMu.Lock()
	defer s.emitMu.Unlock()
	s.mu.Lock()
	st, ok := s.tasks[id]
	m, tracked := s.workerMeta[id]
	if !ok || !tracked || m.finished {
		s.mu.Unlock()
		return
	}
	m.usage = usage
	m.filesTouched = filesTouched
	s.workerMeta[id] = m
	w := s.workerSnapshotLocked(id, st.ToSummary())
	fn := s.workerObserver
	s.mu.Unlock()
	if fn != nil {
		fn(WorkerEvent{Type: "usage", Worker: w})
	}
}

func (s *Supervisor) reportWorkerFinished(sum types.TaskSummary) {
	s.emitMu.Lock()
	defer s.emitMu.Unlock()
	s.mu.Lock()
	m, tracked := s.workerMeta[sum.ID]
	if !tracked || m.finished {
		s.mu.Unlock()
		return
	}
	m.finished = true
	s.workerMeta[sum.ID] = m
	w := s.workerSnapshotLocked(sum.ID, sum)
	fn := s.workerObserver
	s.mu.Unlock()
	if fn != nil {
		fn(WorkerEvent{Type: "finished", Worker: w})
	}
}

// maxMessageSummaryRunes bounds the mailbox summary shown in the browser.
const maxMessageSummaryRunes = 200

// sanitizeSummary replaces control characters, trims, and caps the length.
func sanitizeSummary(in string) string {
	b := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, in)
	b = strings.Join(strings.Fields(b), " ")
	if r := []rune(b); len(r) > maxMessageSummaryRunes {
		b = string(r[:maxMessageSummaryRunes])
	}
	return b
}

// reportWorkerMessage forwards mailbox traffic for a live, tracked worker. The
// full content is never passed on, and nothing follows a worker's finished.
func (s *Supervisor) reportWorkerMessage(taskID string, msg PendingMessage) {
	s.emitMu.Lock()
	defer s.emitMu.Unlock()
	s.mu.RLock()
	m, tracked := s.workerMeta[taskID]
	fn := s.workerObserver
	s.mu.RUnlock()
	if !tracked || m.finished || fn == nil {
		return
	}
	msg.Content = ""
	msg.Summary = sanitizeSummary(msg.Summary)
	msg.FromAgent = sanitizeSummary(msg.FromAgent)
	msg.ToAgent = sanitizeSummary(msg.ToAgent)
	fn(WorkerEvent{Type: "message", Message: msg})
}

// WorkerToolTarget extracts a short target and a file path from a tool input.
// Tool inputs vary by tool; count only concrete path fields in actual calls.
func WorkerToolTarget(input any) (target, path string) {
	b, err := json.Marshal(input)
	if err != nil {
		return "", ""
	}
	var fields map[string]any
	if json.Unmarshal(b, &fields) != nil {
		return "", ""
	}
	for _, key := range []string{"path", "file_path", "filepath", "target", "command"} {
		if v, ok := fields[key].(string); ok && strings.TrimSpace(v) != "" {
			v = strings.TrimSpace(v)
			if key == "path" || key == "file_path" || key == "filepath" {
				path = v
			}
			if len(v) > 160 {
				v = v[:160]
			}
			return v, path
		}
	}
	return "", ""
}
