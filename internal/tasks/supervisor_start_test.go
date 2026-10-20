package tasks

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/bootstrap"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/permissions"
	"github.com/FernasFragas/Nandocode/internal/state"
	"github.com/FernasFragas/Nandocode/internal/tools"
	"github.com/FernasFragas/Nandocode/internal/types"
)

// A failed Start must leave no trace: no task in the supervisor, no pending
// summary in the app state, and nothing to cancel.
func TestSupervisorStartRollbackLeavesNoPhantomTask(t *testing.T) {
	t.Parallel()
	st := state.NewStore(state.DefaultApp(bootstrap.New(bootstrap.DefaultInitial(".")).Snapshot()), nil)
	// The session dir is a regular file, so MkdirAll for the output file fails.
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewSupervisor(filepath.Join(blocker, "tasks"), st)

	ran := false
	id, err := s.Start(context.Background(), types.KindBash, "doomed", func(ctx context.Context, out *OutputWriter) (int, error) {
		ran = true
		return 0, nil
	})
	if err == nil {
		t.Fatalf("expected Start to fail, got id %q", id)
	}
	if ran {
		t.Fatal("run func must not execute after a failed Start")
	}
	if got := s.List(); len(got) != 0 {
		t.Fatalf("supervisor kept %d phantom task(s) after rollback", len(got))
	}
	if got := st.Get().Tasks; len(got) != 0 {
		t.Fatalf("app state kept %d phantom task summary(ies) after rollback: %+v", len(got), got)
	}
	s.mu.RLock()
	cancels := len(s.cancels)
	s.mu.RUnlock()
	if cancels != 0 {
		t.Fatalf("rollback leaked %d cancel func(s)", cancels)
	}
}

// toolUseClient asks for one read-only tool call, then finishes.
type toolUseClient struct {
	mailboxFakeClient
}

func (c *toolUseClient) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	c.mu.Lock()
	c.calls++
	call := c.calls
	c.mu.Unlock()
	ch := make(chan llm.StreamEvent, 2)
	if call == 1 {
		var tc llm.ToolCall
		tc.Function.Name = "Probe"
		tc.Function.Arguments = map[string]any{}
		ch <- llm.StreamEvent{Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{tc}}}
		ch <- llm.StreamEvent{Done: true, DoneReason: "stop"}
	} else {
		ch <- llm.StreamEvent{Message: llm.Message{Role: llm.RoleAssistant, Content: "finished"}}
		ch <- llm.StreamEvent{Done: true, DoneReason: "stop"}
	}
	close(ch)
	return ch, nil
}

// A coordinator worker runs with the parent's permission mode, so a read-only
// tool the parent could run without a prompt is not denied for the worker.
func TestAgentRunFuncWorkerCanUseTools(t *testing.T) {
	t.Parallel()
	probe := tools.BuildTool(tools.Spec{
		Name:        "Probe",
		Description: "Read-only probe.",
		Schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		Unmarshal:   func(json.RawMessage) (any, error) { return map[string]any{}, nil },
		IsReadOnlyFunc: func(any) bool {
			return true
		},
		IsConcurrentFunc:  func(any) bool { return true },
		IsDestructiveFunc: func(any) bool { return false },
		CallFunc: func(tools.Context, any, chan<- tools.ProgressEvent) (tools.Result, error) {
			return tools.Result{Display: "probe ok"}, nil
		},
	})
	reg := tools.NewRegistry()
	if err := reg.Register(probe); err != nil {
		t.Fatal(err)
	}
	sup := NewSupervisor(filepath.Join(t.TempDir(), "tasks"), nil)
	taskIDCh := make(chan string, 1)
	id, err := sup.Start(context.Background(), types.KindAgent, "worker", AgentRunFuncWithMailbox(
		&toolUseClient{}, reg, agent.DefaultConfig(), "s1",
		tools.Context{WorkingDir: t.TempDir()},
		"probe it", "test-model", "", sup, taskIDCh,
	))
	if err != nil {
		t.Fatal(err)
	}
	taskIDCh <- id
	waitForStatus(t, sup, id, types.StatusCompleted, 5*time.Second)

	st, _ := sup.Get(id)
	data, err := os.ReadFile(st.ToSummary().OutputFile)
	if err != nil {
		t.Fatal(err)
	}
	var sawResult bool
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, `\"kind\":\"tool_result\"`) {
			continue
		}
		sawResult = true
		if !strings.Contains(line, `\"ok\":true`) {
			t.Fatalf("worker tool call was denied or failed: %s", line)
		}
	}
	if !sawResult {
		t.Fatalf("no tool_result recorded in worker output:\n%s", data)
	}
}

func TestWorkerPermissionModeNeverBypass(t *testing.T) {
	t.Parallel()
	for _, mode := range []tools.PermissionMode{
		tools.PermissionBypassPermissions, tools.PermissionDontAsk, tools.PermissionPlan,
		tools.PermissionDefault, "", "acceptEdits", "auto", "unknown",
	} {
		got := workerPermissionMode(tools.Context{PermissionMode: mode})
		if got == permissions.ModeBypass {
			t.Fatalf("parent mode %q gave the worker bypass", mode)
		}
	}
	if got := workerPermissionMode(tools.Context{PermissionMode: tools.PermissionPlan}); got != permissions.ModePlan {
		t.Fatalf("plan parent: worker mode %q want plan", got)
	}
}

// A bypass parent with a deny rule must not hand workers a bypass that skips
// the rule: a mutating tool is denied for the worker.
func TestWorkerUnderBypassParentCannotRunMutatingTool(t *testing.T) {
	t.Parallel()
	ran := make(chan struct{}, 1)
	mutate := tools.BuildTool(tools.Spec{
		Name:        "Probe",
		Description: "Mutating tool.",
		Schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		Unmarshal:   func(json.RawMessage) (any, error) { return map[string]any{}, nil },
		CallFunc: func(tools.Context, any, chan<- tools.ProgressEvent) (tools.Result, error) {
			ran <- struct{}{}
			return tools.Result{Display: "wrote"}, nil
		},
	})
	reg := tools.NewRegistry()
	if err := reg.Register(mutate); err != nil {
		t.Fatal(err)
	}
	sup := NewSupervisor(filepath.Join(t.TempDir(), "tasks"), nil)
	taskIDCh := make(chan string, 1)
	id, err := sup.Start(context.Background(), types.KindAgent, "worker", AgentRunFuncWithMailbox(
		&toolUseClient{}, reg, agent.DefaultConfig(), "s1",
		tools.Context{WorkingDir: t.TempDir(), PermissionMode: tools.PermissionBypassPermissions},
		"write it", "test-model", "", sup, taskIDCh,
	))
	if err != nil {
		t.Fatal(err)
	}
	taskIDCh <- id
	waitForStatus(t, sup, id, types.StatusCompleted, 5*time.Second)
	select {
	case <-ran:
		t.Fatal("mutating tool ran for a worker under a bypass parent")
	default:
	}
	st, _ := sup.Get(id)
	data, err := os.ReadFile(st.ToSummary().OutputFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `\"kind\":\"tool_result\"`) || !strings.Contains(string(data), `\"ok\":false`) {
		t.Fatalf("expected a denied tool_result:\n%s", data)
	}
}
