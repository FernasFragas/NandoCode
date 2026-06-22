package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/FernasFragas/nandocodego/internal/agent"
	"github.com/FernasFragas/nandocodego/internal/llm"
	"github.com/FernasFragas/nandocodego/internal/permissions"
)

type AgentRunner interface {
	Run(context.Context, agent.Input) <-chan agent.Event
}

type Runner struct {
	next       AgentRunner
	dispatcher *Dispatcher
}

func NewRunner(next AgentRunner, dispatcher *Dispatcher) *Runner {
	return &Runner{next: next, dispatcher: dispatcher}
}

func (r *Runner) Run(ctx context.Context, in agent.Input) <-chan agent.Event {
	out := make(chan agent.Event, 16)
	go func() {
		defer close(out)
		if r.next == nil {
			return
		}
		if r.dispatcher == nil {
			for evt := range r.next.Run(ctx, in) {
				out <- evt
			}
			return
		}

		r.emitSnapshotNotices(ctx, out)
		sessionStart := time.Now()
		r.dispatchWithNotice(ctx, out, r.envelope(EventSessionStart, nil, in, ""))
		sendStageTiming(ctx, out, "hook_session_start", time.Since(sessionStart))
		userPromptStart := time.Now()
		if res := r.dispatchWithNotice(ctx, out, r.envelope(EventUserPromptSubmit, nil, in, summarizeConversation(in.Messages))); res.Decision == DecisionDeny {
			sendStageTiming(ctx, out, "hook_user_prompt_submit", time.Since(userPromptStart))
			reason := res.Reason
			if reason == "" {
				reason = "user prompt blocked by hook"
			}
			sendNotice(ctx, out, reason)
			out <- agent.Terminal{Reason: agent.TerminalStopHook, Detail: reason}
			return
		}
		sendStageTiming(ctx, out, "hook_user_prompt_submit", time.Since(userPromptStart))
		priorHookDecision := in.HookDecision
		in.HookDecision = func(ctx context.Context, req permissions.Request) (permissions.Result, bool) {
			preToolStart := time.Now()
			defer func() { sendStageTiming(ctx, out, "hook_pre_tool_use", time.Since(preToolStart)) }()
			if res, ok := r.preToolDecision(ctx, req, in, out); ok {
				return res, true
			}
			if priorHookDecision != nil {
				return priorHookDecision(ctx, req)
			}
			return permissions.Result{}, false
		}
		priorPost := in.PostToolUse
		in.PostToolUse = func(ctx context.Context, evt agent.ToolHookEvent) {
			postToolStart := time.Now()
			r.dispatchWithNotice(ctx, out, r.envelope(EventPostToolUse, &evt, in, ""))
			if evt.Err != nil {
				r.dispatchWithNotice(ctx, out, r.envelope(EventPostToolUseFail, &evt, in, ""))
			}
			sendStageTiming(ctx, out, "hook_post_tool_use", time.Since(postToolStart))
			if priorPost != nil {
				priorPost(ctx, evt)
			}
		}
		priorDenied := in.PermissionDenied
		in.PermissionDenied = func(ctx context.Context, evt agent.ToolHookEvent) {
			permissionDeniedStart := time.Now()
			r.dispatchWithNotice(ctx, out, r.envelope(EventPermissionDenied, &evt, in, ""))
			sendStageTiming(ctx, out, "hook_permission_denied", time.Since(permissionDeniedStart))
			if priorDenied != nil {
				priorDenied(ctx, evt)
			}
		}
		priorStop := in.StopHook
		in.StopHook = func(ctx context.Context, conversation []llm.Message) (string, bool) {
			stopStart := time.Now()
			defer func() { sendStageTiming(ctx, out, "hook_stop", time.Since(stopStart)) }()
			env := r.envelope(EventStop, nil, in, summarizeConversation(conversation))
			res := r.dispatchWithNotice(ctx, out, env)
			if res.Decision == DecisionDeny {
				return res.Reason, true
			}
			if priorStop != nil {
				return priorStop(ctx, conversation)
			}
			return "", false
		}

		for evt := range r.next.Run(ctx, in) {
			if _, ok := evt.(agent.Terminal); ok {
				sessionEndStart := time.Now()
				r.dispatchWithNotice(ctx, out, r.envelope(EventSessionEnd, nil, in, ""))
				sendStageTiming(ctx, out, "hook_session_end", time.Since(sessionEndStart))
			}
			out <- evt
		}
	}()
	return out
}

func (r *Runner) emitSnapshotNotices(ctx context.Context, out chan<- agent.Event) {
	for _, warning := range r.dispatcher.Snapshot.Warnings {
		sendNotice(ctx, out, "hook config warning: "+warning)
	}
	for _, disabled := range r.dispatcher.Snapshot.Disabled {
		sendNotice(ctx, out, fmt.Sprintf("hook disabled: %s %s from %s: %s", disabled.Hook.Kind, disabled.Hook.Event, disabled.Hook.Source, disabled.Reason))
	}
}

func (r *Runner) dispatchWithNotice(ctx context.Context, out chan<- agent.Event, env Envelope) Result {
	res := r.dispatcher.Dispatch(ctx, env)
	if res.Warning != "" {
		sendNotice(ctx, out, res.Warning)
	}
	return res
}

func (r *Runner) preToolDecision(ctx context.Context, req permissions.Request, in agent.Input, out chan<- agent.Event) (permissions.Result, bool) {
	target := permissions.ExtractTarget(req.Input)
	evt := agent.ToolHookEvent{
		ToolName:       req.ToolName,
		Input:          req.Input,
		Target:         target,
		ToolContext:    req.ToolContext,
		PermissionMode: req.Mode,
		Model:          in.Model,
	}
	res := r.dispatcher.Dispatch(ctx, r.envelope(EventPreToolUse, &evt, in, ""))
	if res.Warning != "" {
		sendNotice(ctx, out, res.Warning)
	}
	if !res.Decisive() {
		return permissions.Result{}, false
	}
	if res.Decision == DecisionAsk && req.Prompt != nil {
		decision, reason, err := req.Prompt(ctx, permissions.Prompt{
			ToolName: req.ToolName,
			Target:   target,
			Reason:   res.Reason,
		})
		if err == nil {
			if reason == "" {
				reason = res.Reason
			}
			return permissions.Result{
				Decision:     decision,
				Stage:        permissions.StagePrompt,
				Reason:       reason,
				UpdatedInput: req.Input,
			}, true
		}
	}
	stage := permissions.StageHook
	return permissions.Result{
		Decision:     mapDecision(res.Decision),
		Stage:        stage,
		Reason:       res.Reason,
		UpdatedInput: req.Input,
	}, true
}

func (r *Runner) envelope(event Event, toolEvt *agent.ToolHookEvent, in agent.Input, summary string) Envelope {
	env := Envelope{
		Event:               event,
		Timestamp:           time.Now().UTC(),
		SessionID:           r.dispatcher.Config.SessionID,
		WorkingDir:          r.dispatcher.Config.WorkingDir,
		ConversationSummary: summary,
		Metadata: map[string]string{
			"model":           in.Model,
			"permission_mode": string(in.PermissionMode),
		},
	}
	if toolEvt != nil {
		env.Tool = &ToolInput{
			Name:          toolEvt.ToolName,
			Target:        toolEvt.Target,
			InputSummary:  summarizeInput(toolEvt.Input),
			ResultSummary: summarizeResult(toolEvt.Result),
		}
		if toolEvt.Err != nil {
			env.Tool.Error = sanitize(toolEvt.Err.Error())
		}
	}
	return env
}

func mapDecision(d Decision) permissions.Decision {
	switch d {
	case DecisionAllow:
		return permissions.DecisionAllow
	case DecisionAsk:
		return permissions.DecisionAsk
	case DecisionDeny:
		return permissions.DecisionDeny
	default:
		return permissions.DecisionDeny
	}
}

func sendNotice(ctx context.Context, out chan<- agent.Event, msg string) {
	select {
	case out <- agent.HookNotice{Message: msg}:
	case <-ctx.Done():
	}
}

func sendStageTiming(ctx context.Context, out chan<- agent.Event, stage string, dur time.Duration) {
	if stage == "" || dur <= 0 {
		return
	}
	select {
	case out <- agent.StageTiming{Stage: stage, Duration: dur}:
	case <-ctx.Done():
	}
}

func summarizeInput(input any) string {
	b, err := json.Marshal(input)
	if err != nil {
		return ""
	}
	s := sanitize(string(b))
	return filepath.ToSlash(s)
}

func summarizeConversation(messages []llm.Message) string {
	const max = 2000
	var out string
	for _, m := range messages {
		if m.Content == "" {
			continue
		}
		out += string(m.Role) + ": " + m.Content + "\n"
		if len(out) > max {
			return out[:max] + "\n<truncated>"
		}
	}
	return out
}

func summarizeResult(result any) string {
	b, err := json.Marshal(result)
	if err != nil {
		return ""
	}
	return sanitize(string(b))
}
