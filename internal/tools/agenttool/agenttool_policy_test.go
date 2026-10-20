package agenttool

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/permissions"
	"github.com/FernasFragas/Nandocode/internal/tools"
	"github.com/FernasFragas/Nandocode/internal/tools/bash"
)

// scriptedLLM replays one scripted assistant message per Chat call.
type scriptedLLM struct {
	fakeLLM
	mu     sync.Mutex
	script []llm.Message
	calls  int
}

func (s *scriptedLLM) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	s.mu.Lock()
	msg := llm.Message{Content: "done"}
	if s.calls < len(s.script) {
		msg = s.script[s.calls]
	}
	s.calls++
	s.mu.Unlock()
	ch := make(chan llm.StreamEvent, 2)
	ch <- llm.StreamEvent{Message: msg}
	ch <- llm.StreamEvent{Done: true, DoneReason: "stop"}
	close(ch)
	return ch, nil
}

func toolCallMessage(name string, args map[string]any) llm.Message {
	var tc llm.ToolCall
	tc.Function.Name = name
	tc.Function.Arguments = args
	return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{tc}}
}

// runParent drives a parent agent that calls the Agent tool; the sub-agent then
// edits marker with Bash (a write that is not intrinsically denied). It returns whether the file changed.
func runParent(t *testing.T, parentMode permissions.Mode, rules permissions.Rules, requested string, hook permissions.HookDecisionFunc) bool {
	t.Helper()
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker.txt")
	if err := os.WriteFile(marker, []byte("aaa"), 0o644); err != nil {
		t.Fatal(err)
	}

	script := &scriptedLLM{script: []llm.Message{
		toolCallMessage("Agent", map[string]any{"task": "write the file", "permission_mode": requested}),
		toolCallMessage("Bash", map[string]any{"command": "sed -i.bak s/a/b/g " + marker}),
		{Role: llm.RoleAssistant, Content: "child finished"},
		{Role: llm.RoleAssistant, Content: "parent finished"},
	}}

	reg := tools.NewRegistry()
	if err := reg.Register(bash.NewBashTool()); err != nil {
		t.Fatal(err)
	}
	at := New(script, reg, agent.DefaultConfig(), "s1", func() string { return "m" }, func() string { return string(llm.ProviderOllamaLocal) })
	if err := reg.Register(at); err != nil {
		t.Fatal(err)
	}

	a, err := agent.New(script, reg, agent.WithConfig(agent.DefaultConfig()))
	if err != nil {
		t.Fatal(err)
	}
	tctx := tools.DefaultContext(context.Background(), dir)
	tctx.PermissionMode = permissions.ToToolsMode(parentMode)
	allowAgentOnly := func(ctx context.Context, req permissions.Request) (permissions.Result, bool) {
		if req.ToolName == "Agent" {
			return permissions.Result{Decision: permissions.DecisionAllow, Stage: permissions.StageHook, Reason: "test"}, true
		}
		if hook != nil {
			return hook(ctx, req)
		}
		return permissions.Result{}, false
	}
	events := a.Run(context.Background(), agent.Input{
		Model:           "m",
		Messages:        []llm.Message{{Role: llm.RoleUser, Content: "go"}},
		ToolContext:     tctx,
		PermissionMode:  parentMode,
		PermissionRules: rules,
		HookDecision:    allowAgentOnly,
	})
	for range events {
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	return string(got) != "aaa"
}

func TestSubagentCannotEscapePlanModeParent(t *testing.T) {
	if runParent(t, permissions.ModePlan, permissions.Rules{}, "bypass", nil) {
		t.Fatal("sub-agent requested bypass under a plan-mode parent and modified a file")
	}
}

func TestSubagentInheritsParentDenyRules(t *testing.T) {
	rules := permissions.Rules{AlwaysDeny: []permissions.Rule{{Pattern: "Bash(sed *)", Source: permissions.SourceUser}}}
	if runParent(t, permissions.ModeDefault, rules, "bypass", nil) {
		t.Fatal("sub-agent ignored the parent's deny rule")
	}
}

func TestSubagentInheritsParentHook(t *testing.T) {
	deny := func(ctx context.Context, req permissions.Request) (permissions.Result, bool) {
		if req.ToolName == "Bash" {
			return permissions.Result{Decision: permissions.DecisionDeny, Stage: permissions.StageHook, Reason: "hook says no"}, true
		}
		return permissions.Result{}, false
	}
	if runParent(t, permissions.ModeDefault, permissions.Rules{}, "bypass", deny) {
		t.Fatal("sub-agent ignored the parent's hook decision")
	}
}

func TestSubagentBypassParentStillAllowsWrite(t *testing.T) {
	if !runParent(t, permissions.ModeBypass, permissions.Rules{}, "", nil) {
		t.Fatal("sanity check failed: a bypass parent with no rules should let the sub-agent write")
	}
}
