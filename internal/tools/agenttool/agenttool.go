package agenttool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/permissions"
	"github.com/FernasFragas/Nandocode/internal/tasks"
	"github.com/FernasFragas/Nandocode/internal/tools"
	"github.com/FernasFragas/Nandocode/internal/types"
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

	policyMu sync.Mutex
	policy   *agent.ParentPolicy
}

func New(client llm.Client, registry *tools.Registry, cfg agent.Config, sessionID string, getModel, getProvider func() string) *Tool {
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

// SetParentPolicy records the permission policy (mode, rules and hooks) of the
// agent run that is calling this tool. The agent loop calls it before every
// tool call, so the sub-agent can be held to the parent's policy.
func (t *Tool) SetParentPolicy(p agent.ParentPolicy) {
	t.policyMu.Lock()
	defer t.policyMu.Unlock()
	t.policy = &p
}

// parentPolicy returns the calling run's policy. When none was recorded it
// falls back to the mode carried by the tool context, with no rules or hooks.
func (t *Tool) parentPolicy(ctx tools.Context) agent.ParentPolicy {
	t.policyMu.Lock()
	p := t.policy
	t.policyMu.Unlock()
	if p != nil {
		return *p
	}
	return agent.ParentPolicy{Mode: permissions.FromToolsMode(ctx.PermissionMode)}
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

	// A sub-agent can never be looser than its parent: the model-supplied
	// permission_mode is clamped, and the parent's rules and hooks carry over.
	parent := t.parentPolicy(ctx)
	permMode := permissions.ClampMode(parent.Mode, permissions.Mode(in.PermissionMode))
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
		name := strings.TrimSpace(in.Name)
		if name != "" {
			// Refuse before any worker starts, so a rejected name leaves
			// nothing running and nothing counted against MaxWorkers.
			if err := t.supervisor.CheckName(name); err != nil {
				return tools.Result{}, err
			}
		}
		taskIDCh := make(chan string, 1)
		taskID, err := t.supervisor.Start(ctx.EffectiveContext(), types.KindAgent, "coordinator worker", tasks.AgentRunFuncWithMailbox(
			t.client, t.registry, t.config, t.session, ctx, in.Task, model, provider, t.supervisor, taskIDCh,
		))
		if err != nil {
			return tools.Result{}, err
		}
		if name != "" {
			if err := t.supervisor.RegisterName(name, taskID); err != nil {
				// Lost a race for the name: stop the worker before it is
				// announced; it exits on its context at the taskIDCh wait.
				_ = t.supervisor.Stop(taskID)
				return tools.Result{}, err
			}
		}
		t.supervisor.SetWorkerMeta(taskID, tasks.WorkerMeta{Name: name, Task: in.Task, Model: model, Parent: "coordinator"})
		taskIDCh <- taskID
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
		PermissionMode:   parent.Mode.Normalize(),
		PermissionRules:  parent.Rules,
		PermissionPrompt: t.wrapPermissionPrompt(),
		HookDecision:     parent.HookDecision,
		PostToolUse:      parent.PostToolUse,
		PermissionDenied: parent.PermissionDenied,
		IsSubagent:       false,
	}
	params := agent.SubagentParams{
		Mode:           agent.SpawnBuiltin,
		Task:           in.Task,
		PermissionMode: permMode,
		Background:     in.Background,
		MaxTurns:       safeAgentToolMaxTurns(in.MaxTurns),
		SessionID:      t.session,
	}
	if t.supervisor == nil {
		result, err := agent.RunSubagent(ctx.EffectiveContext(), parentInput, params, t.client, t.registry, t.config)
		if err != nil {
			return tools.Result{}, err
		}
		return tools.Result{Display: result}, nil
	}
	result, err := t.runTracked(ctx, parentInput, params, in, model)
	if err != nil {
		return tools.Result{}, err
	}
	return tools.Result{Display: result}, nil
}

// runTracked runs a normal (non-coordinator) sub-agent as a supervisor task so
// it is announced to the browser like a coordinator worker. The call still
// blocks until the sub-agent ends, and the permission clamp is unchanged: the
// same parentInput and params reach agent.RunSubagent.
func (t *Tool) runTracked(ctx tools.Context, parentInput agent.Input, params agent.SubagentParams, in Input, model string) (string, error) {
	var (
		result string
		runErr error
		done   = make(chan struct{})
		idCh   = make(chan string, 1)
		paths  = map[string]struct{}{}
		names  = map[string]string{}
		tgts   = map[string]string{}
	)
	sup := t.supervisor
	taskID, err := sup.Start(ctx.EffectiveContext(), types.KindAgent, "sub-agent", func(runCtx context.Context, _ *tasks.OutputWriter) (int, error) {
		defer close(done)
		// Runs before close(done): record the panic for the caller, then let
		// it propagate so the supervisor marks the task failed (one finished).
		defer func() {
			if r := recover(); r != nil {
				runErr = fmt.Errorf("sub-agent failed: panic: %v", r)
				panic(r)
			}
		}()
		var id string
		select {
		case id = <-idCh:
		case <-runCtx.Done():
			runErr = runCtx.Err()
			return 137, runErr
		}
		p := params
		p.OnEvent = func(e agent.Event) {
			switch ev := e.(type) {
			case agent.ToolUseStart:
				target, path := tasks.WorkerToolTarget(ev.Input)
				names[ev.ID], tgts[ev.ID] = ev.Name, target
				if path != "" {
					paths[path] = struct{}{}
				}
				sup.ReportWorkerProgress(id, ev.Name, target, "running")
			case agent.ToolUseResult:
				status := "completed"
				if ev.Err != nil {
					status = "failed"
				}
				sup.ReportWorkerProgress(id, names[ev.ID], tgts[ev.ID], status)
			case agent.Terminal:
				sup.ReportWorkerUsage(id, ev.Usage, len(paths))
			}
		}
		result, runErr = runSubagentFn(runCtx, parentInput, p, t.client, t.registry, t.config)
		if runErr != nil {
			return 1, runErr
		}
		return 0, nil
	})
	if err != nil {
		return "", err
	}
	sup.SetWorkerMeta(taskID, tasks.WorkerMeta{Name: strings.TrimSpace(in.Name), Task: in.Task, Model: model, Parent: "main"})
	idCh <- taskID
	<-done
	return result, runErr
}

// runSubagentFn is replaceable so tests can force a panic.
var runSubagentFn = agent.RunSubagent

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
