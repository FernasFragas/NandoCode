package agenttool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/FernasFragas/nandocodego/internal/agent"
	"github.com/FernasFragas/nandocodego/internal/llm"
	"github.com/FernasFragas/nandocodego/internal/permissions"
	"github.com/FernasFragas/nandocodego/internal/tasks"
	"github.com/FernasFragas/nandocodego/internal/tools"
	"github.com/FernasFragas/nandocodego/internal/types"
)

type Input struct {
	Task            string `json:"task"`
	Name            string `json:"name,omitempty"`
	Model           string `json:"model,omitempty"`
	PermissionMode  string `json:"permission_mode,omitempty"`
	Background      bool   `json:"background,omitempty"`
	RunInBackground *bool  `json:"run_in_background,omitempty"`
	MaxTurns        int    `json:"max_turns,omitempty"`
}

type Tool struct {
	client           llm.Client
	registry         *tools.Registry
	config           agent.Config
	session          string
	getModel         func() string
	getProvider      func() string
	permissionPrompt permissions.PromptFunc
	promptTimeout    time.Duration
	supervisor       *tasks.Supervisor
}

func New(client llm.Client, registry *tools.Registry, cfg agent.Config, sessionID string, getModel func() string, getProvider func() string) *Tool {
	return &Tool{
		client:        client,
		registry:      registry,
		config:        cfg,
		session:       sessionID,
		getModel:      getModel,
		getProvider:   getProvider,
		promptTimeout: 30 * time.Second,
	}
}

func (t *Tool) SetSupervisor(sup *tasks.Supervisor) {
	t.supervisor = sup
}

func (t *Tool) SetPermissionPrompt(fn permissions.PromptFunc) {
	t.permissionPrompt = fn
}

func (t *Tool) Name() string { return "Agent" }

func (t *Tool) Description() string {
	return "Spawn a sub-agent to complete a bounded task and return its result."
}

func (t *Tool) Aliases() []string { return nil }

func (t *Tool) JSONSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task":              map[string]any{"type": "string"},
			"name":              map[string]any{"type": "string"},
			"permission_mode":   map[string]any{"type": "string"},
			"background":        map[string]any{"type": "boolean"},
			"run_in_background": map[string]any{"type": "boolean"},
			"max_turns":         map[string]any{"type": "integer", "minimum": 20},
		},
		"required": []string{"task"},
	}
}

func (t *Tool) UnmarshalInput(raw json.RawMessage) (any, error) {
	var in Input
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Task) == "" {
		return nil, fmt.Errorf("task is required")
	}
	if in.RunInBackground != nil {
		in.Background = *in.RunInBackground
	}
	return in, nil
}

func (t *Tool) IsEnabled(ctx tools.Context) bool { return true }
func (t *Tool) IsReadOnly(input any) bool        { return false }
func (t *Tool) IsConcurrencySafe(input any) bool { return false }
func (t *Tool) IsDestructive(input any) bool     { return true }
func (t *Tool) CheckPermissions(ctx tools.Context, input any) tools.PermissionResult {
	return tools.PermissionResult{Decision: tools.PermAsk, Reason: "sub-agent execution requires approval", UpdatedInput: input}
}

func (t *Tool) Call(ctx tools.Context, input any, progress chan<- tools.ProgressEvent) (tools.Result, error) {
	in, ok := input.(Input)
	if !ok {
		return tools.Result{}, fmt.Errorf("invalid input type")
	}
	if ctx.IsSubagent {
		return tools.Result{}, fmt.Errorf("sub-agent recursion not allowed")
	}

	permMode := permissions.Mode(in.PermissionMode).Normalize()
	model := strings.TrimSpace(in.Model)
	if model == "" {
		model = t.getModel()
	}
	provider := ""
	if t.getProvider != nil {
		provider = strings.TrimSpace(t.getProvider())
	}

	if agent.IsCoordinatorMode() {
		if t.supervisor == nil {
			return tools.Result{}, fmt.Errorf("coordinator mode requires task supervisor")
		}
		coordinatorCfg := agent.ReadCoordinatorConfig()
		if t.supervisor.ActiveWorkerCount() >= coordinatorCfg.MaxWorkers {
			return tools.Result{}, fmt.Errorf("coordinator worker limit reached (%d)", coordinatorCfg.MaxWorkers)
		}
		if !in.Background {
			in.Background = true
		}
		if !in.Background {
			return tools.Result{}, fmt.Errorf("coordinator mode requires background worker launch")
		}
		taskIDCh := make(chan string, 1)
		taskID, err := t.supervisor.Start(ctx.EffectiveContext(), types.KindAgent, "coordinator worker", tasks.AgentRunFuncWithMailbox(
			t.client, t.registry, t.config, t.session, ctx, in.Task, model, provider, t.supervisor, taskIDCh,
		))
		if err != nil {
			return tools.Result{}, err
		}
		taskIDCh <- taskID
		if name := strings.TrimSpace(in.Name); name != "" {
			if err := t.supervisor.RegisterName(name, taskID); err != nil {
				return tools.Result{}, err
			}
		}
		st, _ := t.supervisor.Get(taskID)
		s := st.ToSummary()
		return tools.Result{
			Display: fmt.Sprintf("worker launched: %s", taskID),
			Data: map[string]any{
				"status":      "async_launched",
				"task_id":     taskID,
				"output_file": s.OutputFile,
				"name":        strings.TrimSpace(in.Name),
			},
		}, nil
	}

	parentInput := agent.Input{
		Model:            model,
		LLMProvider:      provider,
		ToolContext:      ctx,
		PermissionMode:   permMode,
		PermissionRules:  permissions.Rules{},
		PermissionPrompt: t.wrapPermissionPrompt(),
		IsSubagent:       false,
	}
	result, err := agent.RunSubagent(ctx.EffectiveContext(), parentInput, agent.SubagentParams{
		Mode:           agent.SpawnBuiltin,
		Task:           in.Task,
		PermissionMode: permMode,
		Background:     in.Background,
		MaxTurns:       safeAgentToolMaxTurns(in.MaxTurns),
		SessionID:      t.session,
	}, t.client, t.registry, t.config)
	if err != nil {
		return tools.Result{}, err
	}
	return tools.Result{Display: result}, nil
}

func safeAgentToolMaxTurns(maxTurns int) int {
	if maxTurns > 0 && maxTurns < 20 {
		return 0
	}
	return maxTurns
}

func (t *Tool) Render(input any, result tools.Result) tools.RenderHints {
	return tools.RenderHints{Title: "Agent", Summary: "sub-agent"}
}

func (t *Tool) wrapPermissionPrompt() permissions.PromptFunc {
	if t.permissionPrompt == nil {
		return nil
	}
	timeout := t.promptTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return func(ctx context.Context, prompt permissions.Prompt) (permissions.Decision, string, error) {
		childCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		done := make(chan struct {
			decision permissions.Decision
			reason   string
			err      error
		}, 1)
		go func() {
			d, r, err := t.permissionPrompt(childCtx, prompt)
			done <- struct {
				decision permissions.Decision
				reason   string
				err      error
			}{decision: d, reason: r, err: err}
		}()
		select {
		case <-childCtx.Done():
			if childCtx.Err() == context.DeadlineExceeded {
				return permissions.DecisionDeny, "escalation timeout", nil
			}
			return permissions.DecisionDeny, "escalation cancelled", nil
		case out := <-done:
			return out.decision, out.reason, out.err
		}
	}
}
