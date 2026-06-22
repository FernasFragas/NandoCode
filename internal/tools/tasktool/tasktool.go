package tasktool

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/FernasFragas/nandocodego/internal/agent"
	"github.com/FernasFragas/nandocodego/internal/llm"
	"github.com/FernasFragas/nandocodego/internal/tasks"
	"github.com/FernasFragas/nandocodego/internal/tools"
	"github.com/FernasFragas/nandocodego/internal/types"
)

type CreateInput struct {
	Kind        string `json:"kind"`
	Description string `json:"description"`
	Command     string `json:"command,omitempty"`
	WorkingDir  string `json:"working_dir,omitempty"`
	Task        string `json:"task,omitempty"`
	Model       string `json:"model,omitempty"`
}

type IDInput struct {
	TaskID    string `json:"task_id"`
	TailLines int    `json:"tail_lines,omitempty"`
}

type OutputInput struct {
	TaskID   string `json:"task_id"`
	MaxLines int    `json:"max_lines,omitempty"`
}

type ListInput struct {
	Kind string `json:"kind,omitempty"`
}

type CreateTool struct {
	sup         *tasks.Supervisor
	client      llm.Client
	registry    *tools.Registry
	config      agent.Config
	session     string
	getModel    func() string
	getProvider func() string
}

type ListTool struct{ sup *tasks.Supervisor }
type GetTool struct{ sup *tasks.Supervisor }
type OutputTool struct{ sup *tasks.Supervisor }
type StopTool struct{ sup *tasks.Supervisor }

func NewAll(sup *tasks.Supervisor) []tools.Tool {
	return []tools.Tool{
		&CreateTool{sup: sup},
		&ListTool{sup: sup},
		&GetTool{sup: sup},
		&OutputTool{sup: sup},
		&StopTool{sup: sup},
	}
}

func NewWithAgent(sup *tasks.Supervisor, client llm.Client, registry *tools.Registry, cfg agent.Config, sessionID string, getModel func() string, getProvider func() string) []tools.Tool {
	return []tools.Tool{
		&CreateTool{sup: sup, client: client, registry: registry, config: cfg, session: sessionID, getModel: getModel, getProvider: getProvider},
		&ListTool{sup: sup},
		&GetTool{sup: sup},
		&OutputTool{sup: sup},
		&StopTool{sup: sup},
	}
}

func (t *CreateTool) Name() string        { return "TaskCreate" }
func (t *ListTool) Name() string          { return "TaskList" }
func (t *GetTool) Name() string           { return "TaskGet" }
func (t *OutputTool) Name() string        { return "TaskOutput" }
func (t *StopTool) Name() string          { return "TaskStop" }
func (t *CreateTool) Description() string { return "Create a background task and return its task ID." }
func (t *ListTool) Description() string   { return "List tracked background tasks." }
func (t *GetTool) Description() string    { return "Get one task status snapshot." }
func (t *OutputTool) Description() string { return "Read JSONL output lines for a task." }
func (t *StopTool) Description() string   { return "Stop a running task by ID." }
func (t *CreateTool) Aliases() []string   { return nil }
func (t *ListTool) Aliases() []string     { return nil }
func (t *GetTool) Aliases() []string      { return nil }
func (t *OutputTool) Aliases() []string   { return nil }
func (t *StopTool) Aliases() []string     { return nil }

func objectSchema(required ...string) map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}, "required": required}
}
func (t *CreateTool) JSONSchema() map[string]any { return objectSchema("kind", "description") }
func (t *ListTool) JSONSchema() map[string]any   { return objectSchema() }
func (t *GetTool) JSONSchema() map[string]any    { return objectSchema("task_id") }
func (t *OutputTool) JSONSchema() map[string]any { return objectSchema("task_id") }
func (t *StopTool) JSONSchema() map[string]any   { return objectSchema("task_id") }

func (t *CreateTool) UnmarshalInput(raw json.RawMessage) (any, error) {
	var in CreateInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Kind) == "" || strings.TrimSpace(in.Description) == "" {
		return nil, fmt.Errorf("kind and description are required")
	}
	return in, nil
}
func (t *ListTool) UnmarshalInput(raw json.RawMessage) (any, error) {
	var in ListInput
	if len(raw) == 0 || string(raw) == "null" {
		return in, nil
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, err
	}
	return in, nil
}
func (t *GetTool) UnmarshalInput(raw json.RawMessage) (any, error) {
	var in IDInput
	err := json.Unmarshal(raw, &in)
	return in, err
}
func (t *OutputTool) UnmarshalInput(raw json.RawMessage) (any, error) {
	var in OutputInput
	err := json.Unmarshal(raw, &in)
	return in, err
}
func (t *StopTool) UnmarshalInput(raw json.RawMessage) (any, error) {
	var in IDInput
	err := json.Unmarshal(raw, &in)
	return in, err
}

func (t *CreateTool) IsEnabled(ctx tools.Context) bool { return true }
func (t *ListTool) IsEnabled(ctx tools.Context) bool   { return true }
func (t *GetTool) IsEnabled(ctx tools.Context) bool    { return true }
func (t *OutputTool) IsEnabled(ctx tools.Context) bool { return true }
func (t *StopTool) IsEnabled(ctx tools.Context) bool   { return true }
func (t *CreateTool) IsReadOnly(input any) bool        { return false }
func (t *ListTool) IsReadOnly(input any) bool          { return true }
func (t *GetTool) IsReadOnly(input any) bool           { return true }
func (t *OutputTool) IsReadOnly(input any) bool        { return true }
func (t *StopTool) IsReadOnly(input any) bool          { return false }
func (t *CreateTool) IsConcurrencySafe(input any) bool { return false }
func (t *ListTool) IsConcurrencySafe(input any) bool   { return true }
func (t *GetTool) IsConcurrencySafe(input any) bool    { return true }
func (t *OutputTool) IsConcurrencySafe(input any) bool { return true }
func (t *StopTool) IsConcurrencySafe(input any) bool   { return false }
func (t *CreateTool) IsDestructive(input any) bool     { return true }
func (t *ListTool) IsDestructive(input any) bool       { return false }
func (t *GetTool) IsDestructive(input any) bool        { return false }
func (t *OutputTool) IsDestructive(input any) bool     { return false }
func (t *StopTool) IsDestructive(input any) bool       { return true }
func (t *CreateTool) CheckPermissions(ctx tools.Context, input any) tools.PermissionResult {
	return tools.PermissionResult{Decision: tools.PermAsk, Reason: "task creation requires approval", UpdatedInput: input}
}
func (t *ListTool) CheckPermissions(ctx tools.Context, input any) tools.PermissionResult {
	return tools.PermissionResult{Decision: tools.PermAllow, UpdatedInput: input}
}
func (t *GetTool) CheckPermissions(ctx tools.Context, input any) tools.PermissionResult {
	return tools.PermissionResult{Decision: tools.PermAllow, UpdatedInput: input}
}
func (t *OutputTool) CheckPermissions(ctx tools.Context, input any) tools.PermissionResult {
	return tools.PermissionResult{Decision: tools.PermAllow, UpdatedInput: input}
}
func (t *StopTool) CheckPermissions(ctx tools.Context, input any) tools.PermissionResult {
	return tools.PermissionResult{Decision: tools.PermAllow, UpdatedInput: input}
}

func (t *CreateTool) Call(ctx tools.Context, input any, _ chan<- tools.ProgressEvent) (tools.Result, error) {
	in := input.(CreateInput)
	if t.sup == nil {
		return tools.Result{}, fmt.Errorf("task supervisor unavailable")
	}
	switch strings.ToLower(strings.TrimSpace(in.Kind)) {
	case "bash", string(types.KindBash):
		cmd := strings.TrimSpace(in.Command)
		if cmd == "" {
			return tools.Result{}, fmt.Errorf("command is required for bash tasks")
		}
		wd := in.WorkingDir
		if wd == "" {
			wd = ctx.WorkingDir
		}
		id, err := t.sup.Start(ctx.EffectiveContext(), types.KindBash, in.Description, tasks.BashRunFunc(cmd, wd))
		if err != nil {
			return tools.Result{}, err
		}
		st, _ := t.sup.Get(id)
		s := st.ToSummary()
		return tools.Result{Display: fmt.Sprintf("Task %s running (%s)", id, s.OutputFile), Data: map[string]any{
			"task_id":     id,
			"status":      s.Status,
			"output_file": s.OutputFile,
		}}, nil
	case "agent", string(types.KindAgent):
		if t.client == nil || t.registry == nil {
			return tools.Result{}, fmt.Errorf("agent support not available in this context")
		}
		task := strings.TrimSpace(in.Task)
		if task == "" {
			return tools.Result{}, fmt.Errorf("task is required for agent tasks")
		}
		model := strings.TrimSpace(in.Model)
		if model == "" {
			model = t.getModel()
		}
		provider := ""
		if t.getProvider != nil {
			provider = strings.TrimSpace(t.getProvider())
		}
		id, err := t.sup.Start(ctx.EffectiveContext(), types.KindAgent, in.Description,
			tasks.AgentRunFunc(t.client, t.registry, t.config, t.session, ctx, task, model, provider))
		if err != nil {
			return tools.Result{}, err
		}
		st, _ := t.sup.Get(id)
		s := st.ToSummary()
		return tools.Result{Display: fmt.Sprintf("Task %s running (%s)", id, s.OutputFile), Data: map[string]any{
			"task_id":     id,
			"status":      s.Status,
			"output_file": s.OutputFile,
		}}, nil
	case "mcp", string(types.KindMCP), "remote", string(types.KindRemote):
		return tools.Result{}, fmt.Errorf("kind %q is reserved and not implemented in this phase", in.Kind)
	default:
		return tools.Result{}, fmt.Errorf("unsupported kind %q", in.Kind)
	}
}

func (t *ListTool) Call(ctx tools.Context, input any, _ chan<- tools.ProgressEvent) (tools.Result, error) {
	in, _ := input.(ListInput)
	wantKind := strings.TrimSpace(strings.ToLower(in.Kind))
	all := t.sup.List()
	out := make([]types.TaskSummary, 0, len(all))
	for _, st := range all {
		s := st.ToSummary()
		if wantKind != "" && wantKind != strings.ToLower(string(s.Kind)) && wantKind != kindAlias(s.Kind) {
			continue
		}
		out = append(out, s)
	}
	return tools.Result{Data: out, Display: fmt.Sprintf("%d tasks", len(out))}, nil
}

func (t *GetTool) Call(ctx tools.Context, input any, _ chan<- tools.ProgressEvent) (tools.Result, error) {
	in := input.(IDInput)
	st, ok := t.sup.Get(strings.TrimSpace(in.TaskID))
	if !ok {
		return tools.Result{}, fmt.Errorf("task not found: %s", in.TaskID)
	}
	summary := st.ToSummary()
	tailLines := in.TailLines
	if tailLines <= 0 {
		tailLines = 20
	}
	if tailLines > 100 {
		tailLines = 100
	}
	lines, _ := tasks.TailLines(filepath.Clean(summary.OutputFile), tailLines)
	resp := map[string]any{
		"task_id":     summary.ID,
		"kind":        summary.Kind,
		"status":      summary.Status,
		"description": summary.Description,
		"started_at":  summary.StartedAt,
		"finished_at": summary.FinishedAt,
		"exit_code":   summary.ExitCode,
		"output_tail": lines,
	}
	return tools.Result{Data: resp, Display: fmt.Sprintf("%s %s", summary.ID, summary.Status)}, nil
}

func (t *OutputTool) Call(ctx tools.Context, input any, _ chan<- tools.ProgressEvent) (tools.Result, error) {
	in := input.(OutputInput)
	st, ok := t.sup.Get(strings.TrimSpace(in.TaskID))
	if !ok {
		return tools.Result{}, fmt.Errorf("task not found: %s", in.TaskID)
	}
	max := in.MaxLines
	if max <= 0 {
		max = 20
	}
	if max > 200 {
		max = 200
	}
	lines, err := tasks.TailLines(filepath.Clean(st.ToSummary().OutputFile), max)
	if err != nil {
		return tools.Result{}, err
	}
	return tools.Result{Data: lines, Display: fmt.Sprintf("%d output lines", len(lines))}, nil
}

func (t *StopTool) Call(ctx tools.Context, input any, _ chan<- tools.ProgressEvent) (tools.Result, error) {
	in := input.(IDInput)
	id := strings.TrimSpace(in.TaskID)
	if err := t.sup.Stop(id); err != nil {
		return tools.Result{}, err
	}
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		st, ok := t.sup.Get(id)
		if ok {
			s := st.ToSummary()
			if s.Status == types.StatusKilled || s.Status == types.StatusCompleted || s.Status == types.StatusFailed {
				return tools.Result{Data: s, Display: fmt.Sprintf("%s %s", s.ID, s.Status)}, nil
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	st, ok := t.sup.Get(id)
	if !ok {
		return tools.Result{}, fmt.Errorf("task not found: %s", id)
	}
	return tools.Result{Data: st.ToSummary(), Display: fmt.Sprintf("%s stop requested", id)}, nil
}

func (t *CreateTool) Render(input any, result tools.Result) tools.RenderHints {
	return tools.RenderHints{Title: "TaskCreate", Summary: "start"}
}
func (t *ListTool) Render(input any, result tools.Result) tools.RenderHints {
	return tools.RenderHints{Title: "TaskList", Summary: "list"}
}
func (t *GetTool) Render(input any, result tools.Result) tools.RenderHints {
	return tools.RenderHints{Title: "TaskGet", Summary: "status"}
}
func (t *OutputTool) Render(input any, result tools.Result) tools.RenderHints {
	return tools.RenderHints{Title: "TaskOutput", Summary: "output"}
}
func (t *StopTool) Render(input any, result tools.Result) tools.RenderHints {
	return tools.RenderHints{Title: "TaskStop", Summary: "stop"}
}

func kindAlias(k types.TaskKind) string {
	switch k {
	case types.KindBash:
		return "bash"
	case types.KindAgent:
		return "agent"
	case types.KindMCP:
		return "mcp"
	case types.KindRemote:
		return "remote"
	default:
		return ""
	}
}
