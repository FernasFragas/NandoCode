package hooks

import (
	"context"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/permissions"
	"github.com/FernasFragas/Nandocode/internal/tools"
)

type fakeAgentRunner struct {
	called bool
	events []agent.Event
	runFn  func(context.Context, agent.Input, chan<- agent.Event)
}

func TestHookTimingStagesIncludeSessionEndAndStop(t *testing.T) {
	next := &fakeAgentRunner{
		runFn: func(ctx context.Context, in agent.Input, out chan<- agent.Event) {
			if in.StopHook != nil {
				in.StopHook(ctx, []llm.Message{{Role: llm.RoleUser, Content: "continue"}})
			}
			out <- agent.Terminal{Reason: agent.TerminalCompleted}
		},
	}
	dispatcher := NewDispatcher(Snapshot{Hooks: []Hook{{
		Kind:    KindPrompt,
		Event:   EventStop,
		Prompt:  "check",
		Enabled: true,
	}}}, &fakeHookClient{content: `{"decision":"allow"}`}, DefaultConfig())
	runner := NewRunner(next, dispatcher)

	input := agent.Input{Model: "test-model"}
	sawSessionEnd := false
	sawStop := false
	for evt := range runner.Run(context.Background(), input) {
		st, ok := evt.(agent.StageTiming)
		if !ok {
			continue
		}
		if st.Stage == "hook_stop" {
			sawStop = true
		}
		if st.Stage == "hook_session_end" {
			sawSessionEnd = true
		}
	}
	if !sawStop {
		t.Fatal("expected hook_stop stage timing")
	}
	if !sawSessionEnd {
		t.Fatal("expected hook_session_end stage timing")
	}
}

func TestHookTimingPrePostAndPermissionDenied(t *testing.T) {
	next := &fakeAgentRunner{
		runFn: func(ctx context.Context, in agent.Input, out chan<- agent.Event) {
			if in.HookDecision != nil {
				in.HookDecision(ctx, permissions.Request{
					ToolName: "Bash",
					Input:    map[string]string{"command": "echo hi"},
				})
			}
			if in.PostToolUse != nil {
				in.PostToolUse(ctx, agent.ToolHookEvent{
					ToolName: "Bash",
					Input:    map[string]string{"command": "echo hi"},
					Result:   tools.Result{Display: "ok"},
				})
			}
			if in.PermissionDenied != nil {
				in.PermissionDenied(ctx, agent.ToolHookEvent{
					ToolName: "Bash",
					Input:    map[string]string{"command": "echo blocked"},
				})
			}
			out <- agent.Terminal{Reason: agent.TerminalCompleted}
		},
	}
	dispatcher := NewDispatcher(Snapshot{Hooks: []Hook{{
		Kind:    KindPrompt,
		Event:   EventPreToolUse,
		Prompt:  "check",
		Enabled: true,
	}}}, &fakeHookClient{content: `{"decision":"allow"}`}, DefaultConfig())
	runner := NewRunner(next, dispatcher)
	var sawPre, sawPost, sawDenied bool
	for evt := range runner.Run(context.Background(), agent.Input{Model: "test-model"}) {
		st, ok := evt.(agent.StageTiming)
		if !ok {
			continue
		}
		switch st.Stage {
		case "hook_pre_tool_use":
			sawPre = true
		case "hook_post_tool_use":
			sawPost = true
		case "hook_permission_denied":
			sawDenied = true
		}
	}
	if !sawPre || !sawPost || !sawDenied {
		t.Fatalf("expected all timings, pre=%v post=%v denied=%v", sawPre, sawPost, sawDenied)
	}
}

func (f *fakeAgentRunner) Run(ctx context.Context, in agent.Input) <-chan agent.Event {
	f.called = true
	ch := make(chan agent.Event, len(f.events)+4)
	if f.runFn != nil {
		go func() {
			defer close(ch)
			f.runFn(ctx, in, ch)
		}()
		return ch
	}
	for _, evt := range f.events {
		ch <- evt
	}
	close(ch)
	return ch
}

func TestPreToolAskUsesPermissionPrompt(t *testing.T) {
	dispatcher := NewDispatcher(Snapshot{Hooks: []Hook{{
		Kind:    KindPrompt,
		Event:   EventPreToolUse,
		Matcher: "Bash(rm -rf*)",
		Prompt:  "check",
		Enabled: true,
	}}}, &fakeHookClient{content: `{"decision":"ask","reason":"needs confirmation"}`}, DefaultConfig())
	runner := NewRunner(nil, dispatcher)
	out := make(chan agent.Event, 4)
	promptCalled := false

	res, ok := runner.preToolDecision(context.Background(), permissions.Request{
		ToolName: "Bash",
		Input:    map[string]string{"command": "rm -rf /tmp/demo"},
		Prompt: func(ctx context.Context, prompt permissions.Prompt) (permissions.Decision, string, error) {
			promptCalled = true
			if prompt.Reason != "needs confirmation" {
				t.Fatalf("expected hook reason in prompt, got %q", prompt.Reason)
			}
			return permissions.DecisionAllow, "approved by test", nil
		},
	}, agent.Input{Model: "test-model"}, out)
	if !ok {
		t.Fatalf("expected decisive result")
	}
	if !promptCalled {
		t.Fatalf("expected permission prompt callback")
	}
	if res.Decision != permissions.DecisionAllow {
		t.Fatalf("expected allow from prompt, got %q", res.Decision)
	}
	if res.Stage != permissions.StagePrompt {
		t.Fatalf("expected prompt stage, got %q", res.Stage)
	}
}

func TestUserPromptSubmitDenyStopsBeforeNextRunner(t *testing.T) {
	next := &fakeAgentRunner{events: []agent.Event{agent.Terminal{Reason: agent.TerminalCompleted}}}
	dispatcher := NewDispatcher(Snapshot{Hooks: []Hook{{
		Kind:    KindPrompt,
		Event:   EventUserPromptSubmit,
		Prompt:  "check",
		Enabled: true,
	}}}, &fakeHookClient{content: `{"decision":"deny","reason":"prompt blocked"}`}, DefaultConfig())
	runner := NewRunner(next, dispatcher)

	var terminal agent.Terminal
	sawHookStageTiming := false
	for evt := range runner.Run(context.Background(), agent.Input{
		Model:    "test-model",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hello"}},
	}) {
		if st, ok := evt.(agent.StageTiming); ok && st.Stage == "hook_user_prompt_submit" {
			sawHookStageTiming = true
		}
		if tEvt, ok := evt.(agent.Terminal); ok {
			terminal = tEvt
		}
	}
	if next.called {
		t.Fatalf("expected prompt hook to stop before next runner")
	}
	if terminal.Reason != agent.TerminalStopHook {
		t.Fatalf("expected stop hook terminal reason, got %q", terminal.Reason)
	}
	if terminal.Detail != "prompt blocked" {
		t.Fatalf("expected hook reason, got %q", terminal.Detail)
	}
	if !sawHookStageTiming {
		t.Fatalf("expected hook timing event")
	}
}
