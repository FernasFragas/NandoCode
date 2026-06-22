package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/FernasFragas/nandocodego/internal/agent"
	"github.com/FernasFragas/nandocodego/internal/ids"
	"github.com/FernasFragas/nandocodego/internal/llm"
	"github.com/FernasFragas/nandocodego/internal/permissions"
	"github.com/FernasFragas/nandocodego/internal/state"
	"github.com/FernasFragas/nandocodego/internal/tools"
	"github.com/FernasFragas/nandocodego/internal/types"
)

type RunFunc func(ctx context.Context, out *OutputWriter) (int, error)

type Supervisor struct {
	mu                sync.RWMutex
	tasks             map[string]TaskState
	cancels           map[string]context.CancelFunc
	sessionDir        string
	store             *state.Store[state.App]
	agentNameRegistry map[string]string
	dreamTaskID       string
	dreamResult       string
	dreamCompletedAt  time.Time
}

func NewSupervisor(sessionDir string, store *state.Store[state.App]) *Supervisor {
	return &Supervisor{
		tasks:             map[string]TaskState{},
		cancels:           map[string]context.CancelFunc{},
		sessionDir:        sessionDir,
		store:             store,
		agentNameRegistry: map[string]string{},
	}
}

var agentNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

func (s *Supervisor) Start(ctx context.Context, kind types.TaskKind, desc string, run RunFunc) (string, error) {
	if run == nil {
		return "", fmt.Errorf("run func is required")
	}
	id := ids.New(kind)
	p := PendingTask{ID: id, Kind: kind, Description: desc, CreatedAt: time.Now().UTC()}
	s.mu.Lock()
	s.tasks[id] = p
	s.mu.Unlock()
	s.publish(p.ToSummary())

	taskCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancels[id] = cancel
	s.mu.Unlock()

	outputPath := filepath.Join(s.sessionDir, id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o700); err != nil {
		s.mu.Lock()
		delete(s.tasks, id)
		delete(s.cancels, id)
		s.mu.Unlock()
		return "", err
	}
	out, err := NewOutputWriter(outputPath)
	if err != nil {
		s.mu.Lock()
		delete(s.tasks, id)
		delete(s.cancels, id)
		s.mu.Unlock()
		return "", err
	}
	r := RunningTask{PendingTask: p, StartedAt: time.Now().UTC(), OutputFile: outputPath}
	s.mu.Lock()
	s.tasks[id] = r
	s.mu.Unlock()
	s.publish(r.ToSummary())

	go func() {
		defer out.Close()
		defer func() {
			if rec := recover(); rec != nil {
				finishedAt := time.Now().UTC()
				s.mu.Lock()
				delete(s.cancels, id)
				cur, ok := s.tasks[id]
				if ok {
					if running, ok := cur.(RunningTask); ok {
						f := FailedTask{RunningTask: running, FinishedAt: finishedAt, Err: fmt.Errorf("task panic: %v", rec)}
						s.tasks[id] = f
						_ = out.WriteExit(1)
						s.mu.Unlock()
						s.publish(f.ToSummary())
						return
					}
				}
				s.mu.Unlock()
			}
		}()
		code, runErr := run(taskCtx, out)
		finishedAt := time.Now().UTC()

		s.mu.Lock()
		delete(s.cancels, id)
		cur, ok := s.tasks[id]
		if !ok {
			s.mu.Unlock()
			return
		}
		running, ok := cur.(RunningTask)
		if !ok {
			s.mu.Unlock()
			return
		}
		if taskCtx.Err() == context.Canceled {
			k := KilledTask{RunningTask: running, FinishedAt: finishedAt}
			s.tasks[id] = k
			_ = out.WriteExit(137)
			s.mu.Unlock()
			s.publish(k.ToSummary())
			return
		}
		if runErr != nil {
			f := FailedTask{RunningTask: running, FinishedAt: finishedAt, Err: runErr}
			s.tasks[id] = f
			_ = out.WriteExit(code)
			s.mu.Unlock()
			s.publish(f.ToSummary())
			return
		}
		c := CompletedTask{RunningTask: running, FinishedAt: finishedAt, ExitCode: code}
		s.tasks[id] = c
		_ = out.WriteExit(code)
		s.mu.Unlock()
		s.publish(c.ToSummary())
	}()

	return id, nil
}

func (s *Supervisor) Stop(id string) error {
	s.mu.RLock()
	cancel, ok := s.cancels[id]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("task %s is not running", id)
	}
	cancel()
	return nil
}

func (s *Supervisor) Get(id string) (TaskState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.tasks[id]
	return st, ok
}

func (s *Supervisor) List() []TaskState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]TaskState, 0, len(s.tasks))
	for _, st := range s.tasks {
		if st.ToSummary().Kind == types.KindDream {
			continue
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ToSummary().CreatedAt.Before(out[j].ToSummary().CreatedAt)
	})
	return out
}

func (s *Supervisor) publish(summary types.TaskSummary) {
	if s.store == nil {
		return
	}
	s.store.Set(func(app state.App) state.App {
		tasks := make(map[string]types.TaskSummary, len(app.Tasks)+1)
		for k, v := range app.Tasks {
			tasks[k] = v
		}
		tasks[summary.ID] = summary
		app.Tasks = tasks
		runningWorkers := 0
		for _, ts := range tasks {
			if ts.Kind == types.KindAgent && (ts.Status == types.StatusPending || ts.Status == types.StatusRunning) {
				runningWorkers++
			}
		}
		app.WorkerCount = runningWorkers
		return app
	})
}

func (s *Supervisor) QueueMessage(taskID string, msg PendingMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.tasks[taskID]
	if !ok {
		return fmt.Errorf("task %s not found", taskID)
	}
	running, ok := st.(RunningTask)
	if !ok {
		return fmt.Errorf("task %s is not running", taskID)
	}
	if running.AgentMeta == nil {
		running.AgentMeta = &AgentTaskMetadata{}
	}
	if running.AgentMeta.Mailbox == nil {
		running.AgentMeta.Mailbox = NewMailbox()
	}
	if running.AgentMeta.MailboxFile == "" {
		running.AgentMeta.MailboxFile = filepath.Join(s.sessionDir, taskID+".mailbox.jsonl")
	}
	err := running.AgentMeta.Mailbox.Enqueue(msg)
	if errors.Is(err, ErrDuplicateMessageID) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := appendMailboxJSONL(running.AgentMeta.MailboxFile, msg); err != nil {
		return err
	}
	s.tasks[taskID] = running
	return nil
}

func (s *Supervisor) DrainMessages(taskID string) ([]PendingMessage, error) {
	s.mu.RLock()
	st, ok := s.tasks[taskID]
	if !ok {
		s.mu.RUnlock()
		return nil, fmt.Errorf("task %s not found", taskID)
	}
	running, ok := st.(RunningTask)
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("task %s is not running", taskID)
	}
	if running.AgentMeta == nil || running.AgentMeta.Mailbox == nil {
		return nil, nil
	}
	return running.AgentMeta.Mailbox.Drain(), nil
}

func appendMailboxJSONL(path string, msg PendingMessage) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(msg)
}

func (s *Supervisor) RegisterName(name, taskID string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("agent name is required")
	}
	if !agentNamePattern.MatchString(trimmed) {
		return fmt.Errorf("invalid agent name %q", name)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.agentNameRegistry[trimmed]; ok && existing != taskID {
		if st, exists := s.tasks[existing]; exists {
			status := st.ToSummary().Status
			if status == types.StatusPending || status == types.StatusRunning {
				return fmt.Errorf("agent name %q already in use", trimmed)
			}
		}
	}
	s.agentNameRegistry[trimmed] = taskID
	return nil
}

func (s *Supervisor) LookupByName(name string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	taskID, ok := s.agentNameRegistry[strings.TrimSpace(name)]
	return taskID, ok
}

func (s *Supervisor) UnregisterName(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.agentNameRegistry, strings.TrimSpace(name))
}

func (s *Supervisor) ActiveWorkerCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, st := range s.tasks {
		sum := st.ToSummary()
		if sum.Kind == types.KindAgent && (sum.Status == types.StatusPending || sum.Status == types.StatusRunning) {
			count++
		}
	}
	return count
}

func (s *Supervisor) KillDream() error {
	s.mu.RLock()
	dreamID := s.dreamTaskID
	s.mu.RUnlock()
	if dreamID == "" {
		return nil
	}
	if err := s.Stop(dreamID); err != nil {
		if strings.Contains(err.Error(), "not running") {
			return nil
		}
		return err
	}
	return nil
}

// BashRunFunc runs bash command and writes stdout/stderr lines to JSONL.
func BashRunFunc(command, workingDir string) RunFunc {
	return func(ctx context.Context, out *OutputWriter) (int, error) {
		cmd := exec.CommandContext(ctx, "bash", "-lc", command)
		if workingDir != "" {
			cmd.Dir = workingDir
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return 1, err
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			return 1, err
		}
		if err := cmd.Start(); err != nil {
			return 1, err
		}
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); pumpStream(stdout, "stdout", out) }()
		go func() { defer wg.Done(); pumpStream(stderr, "stderr", out) }()
		err = cmd.Wait()
		wg.Wait()
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				return ee.ExitCode(), nil
			}
			return 1, err
		}
		return 0, nil
	}
}

func pumpStream(r io.Reader, stream string, out *OutputWriter) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			_ = out.WriteText(stream, string(buf[:n]))
		}
		if err != nil {
			return
		}
	}
}

// AgentRunFunc spawns a sub-agent and writes its events to JSONL.
func AgentRunFunc(client llm.Client, registry *tools.Registry, config agent.Config, sessionID string, ctx tools.Context, task, model, provider string) RunFunc {
	return agentRunFuncInternal(client, registry, config, sessionID, ctx, task, model, provider, nil, nil)
}

func AgentRunFuncWithMailbox(client llm.Client, registry *tools.Registry, config agent.Config, sessionID string, ctx tools.Context, task, model, provider string, supervisor *Supervisor, taskIDCh <-chan string) RunFunc {
	return agentRunFuncInternal(client, registry, config, sessionID, ctx, task, model, provider, supervisor, taskIDCh)
}

func agentRunFuncInternal(client llm.Client, registry *tools.Registry, config agent.Config, sessionID string, ctx tools.Context, task, model, provider string, supervisor *Supervisor, taskIDCh <-chan string) RunFunc {
	return func(runCtx context.Context, out *OutputWriter) (int, error) {
		if client == nil || registry == nil {
			return 1, fmt.Errorf("agent client or registry unavailable")
		}
		taskID := ""
		if taskIDCh != nil {
			select {
			case taskID = <-taskIDCh:
			case <-runCtx.Done():
				return 137, runCtx.Err()
			}
		}
		ag, err := agent.New(client, registry, agent.WithConfig(config))
		if err != nil {
			return 1, err
		}
		in := agent.Input{
			Model:           model,
			LLMProvider:     provider,
			SystemPrompt:    "You are a delegated sub-agent. Complete the assigned task and return concise results.",
			Messages:        []llm.Message{{Role: llm.RoleUser, Content: task}},
			ToolContext:     ctx,
			IsSubagent:      true,
			PermissionMode:  permissions.ModeBubble,
			PermissionRules: permissions.Rules{},
		}
		if supervisor != nil && taskID != "" {
			in.PendingMessagesProvider = func(context.Context) []llm.Message {
				msgs, err := supervisor.DrainMessages(taskID)
				if err != nil || len(msgs) == 0 {
					return nil
				}
				out := make([]llm.Message, 0, len(msgs))
				for _, m := range msgs {
					content := fmt.Sprintf("[mailbox from=%s id=%s] %s", strings.TrimSpace(m.FromAgent), strings.TrimSpace(m.ID), strings.TrimSpace(m.Content))
					out = append(out, llm.Message{Role: llm.RoleUser, Content: content})
				}
				return out
			}
		}
		in.ToolContext.Context = runCtx
		events := ag.Run(runCtx, in)
		exitCode := 0
		for evt := range events {
			switch e := evt.(type) {
			case agent.AssistantTextDelta:
				writeJSONL(out, map[string]any{"ts": time.Now().UTC().Format(time.RFC3339Nano), "kind": "text_delta", "content": e.Content})
			case agent.AssistantThinkingDelta:
				writeJSONL(out, map[string]any{"ts": time.Now().UTC().Format(time.RFC3339Nano), "kind": "thinking_delta", "content": e.Thinking})
			case agent.ToolUseStart:
				writeJSONL(out, map[string]any{"ts": time.Now().UTC().Format(time.RFC3339Nano), "kind": "tool_start", "tool_name": e.Name, "tool_id": e.ID})
			case agent.ToolUseResult:
				writeJSONL(out, map[string]any{"ts": time.Now().UTC().Format(time.RFC3339Nano), "kind": "tool_result", "tool_id": e.ID, "ok": e.Err == nil})
			case agent.Terminal:
				if e.Reason != agent.TerminalCompleted {
					exitCode = 1
				}
				writeJSONL(out, map[string]any{"ts": time.Now().UTC().Format(time.RFC3339Nano), "kind": "terminal", "reason": string(e.Reason)})
				for _, msg := range e.Conversation {
					writeJSONL(out, map[string]any{
						"ts":      time.Now().UTC().Format(time.RFC3339Nano),
						"kind":    "message",
						"message": msg,
					})
				}
			}
		}
		return exitCode, nil
	}
}

func writeJSONL(out *OutputWriter, data map[string]any) {
	if out == nil {
		return
	}
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	_ = out.WriteText("stdout", string(b)+"\n")
}
