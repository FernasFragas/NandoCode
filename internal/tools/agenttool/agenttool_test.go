package agenttool

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/permissions"
	"github.com/FernasFragas/Nandocode/internal/tasks"
	"github.com/FernasFragas/Nandocode/internal/tools"
	"github.com/FernasFragas/Nandocode/internal/types"
)

type fakeLLM struct{}

func (f *fakeLLM) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent, 2)
	ch <- llm.StreamEvent{Message: llm.Message{Content: "child-result"}}
	ch <- llm.StreamEvent{Done: true, DoneReason: "stop"}
	close(ch)
	return ch, nil
}

func (f *fakeLLM) Embed(ctx context.Context, model string, input []string) ([][]float32, error) {
	return nil, nil
}
func (f *fakeLLM) ListModels(ctx context.Context) ([]llm.ModelInfo, error) { return nil, nil }
func (f *fakeLLM) ShowModel(ctx context.Context, name string) (llm.ModelDetails, error) {
	return llm.ModelDetails{}, nil
}

func (f *fakeLLM) PullModel(ctx context.Context, name string, progress chan<- llm.PullProgress) error {
	return nil
}

func TestCallRejectsRecursion(t *testing.T) {
	t.Parallel()
	reg := tools.NewRegistry()
	tool := New(&fakeLLM{}, reg, agent.DefaultConfig(), "s1", func() string { return "llama2" }, func() string { return string(llm.ProviderOllamaLocal) })
	_, err := tool.Call(tools.Context{IsSubagent: true}, Input{Task: "x"}, nil)
	if err == nil || !strings.Contains(err.Error(), "recursion") {
		t.Fatalf("expected recursion error, got %v", err)
	}
}

func TestCallForeground(t *testing.T) {
	t.Parallel()
	reg := tools.NewRegistry()
	tool := New(&fakeLLM{}, reg, agent.DefaultConfig(), "s1", func() string { return "llama2" }, func() string { return string(llm.ProviderOllamaLocal) })
	res, err := tool.Call(tools.Context{Context: context.Background()}, Input{Task: "x", Model: "m"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(res.Display) == "" {
		t.Fatal("expected display")
	}
}

func TestSchemaRequiresTask(t *testing.T) {
	t.Parallel()
	tool := New(&fakeLLM{}, tools.NewRegistry(), agent.DefaultConfig(), "s1", func() string { return "llama2" }, func() string { return string(llm.ProviderOllamaLocal) })
	s := tool.JSONSchema()
	req, _ := s["required"].([]string)
	if len(req) == 0 || req[0] != "task" {
		t.Fatalf("expected task required, got %#v", s["required"])
	}
	_, err := tool.UnmarshalInput(json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected validation error")
	}
	props, _ := s["properties"].(map[string]any)
	if _, ok := props["name"]; !ok {
		t.Fatal("expected name property in schema")
	}
	if _, ok := props["run_in_background"]; !ok {
		t.Fatal("expected run_in_background property in schema")
	}
}

func TestUnmarshalRunInBackgroundAlias(t *testing.T) {
	t.Parallel()
	tool := New(&fakeLLM{}, tools.NewRegistry(), agent.DefaultConfig(), "s1", func() string { return "llama2" }, func() string { return string(llm.ProviderOllamaLocal) })
	v, err := tool.UnmarshalInput(json.RawMessage(`{"task":"x","run_in_background":true}`))
	if err != nil {
		t.Fatal(err)
	}
	in := v.(Input)
	if !in.Background {
		t.Fatal("expected background=true from run_in_background alias")
	}
}

func TestCoordinatorModeLaunchesSupervisorTask(t *testing.T) {
	t.Setenv("NANDOCODEGO_COORDINATOR", "1")
	reg := tools.NewRegistry()
	tool := New(&fakeLLM{}, reg, agent.DefaultConfig(), "s1", func() string { return "llama2" }, func() string { return string(llm.ProviderOllamaLocal) })
	sup := tasks.NewSupervisor(filepath.Join(t.TempDir(), "tasks"), nil)
	tool.SetSupervisor(sup)
	res, err := tool.Call(tools.Context{Context: context.Background()}, Input{Task: "x", Name: "worker-a"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Display, "worker launched: a-") {
		t.Fatalf("unexpected display: %q", res.Display)
	}
	if _, ok := sup.LookupByName("worker-a"); !ok {
		t.Fatal("expected worker name registration")
	}
}

func TestSafeAgentToolMaxTurnsIgnoresTinyCaps(t *testing.T) {
	t.Parallel()
	if got := safeAgentToolMaxTurns(1); got != 0 {
		t.Fatalf("safeAgentToolMaxTurns(1)=%d, want 0", got)
	}
	if got := safeAgentToolMaxTurns(20); got != 20 {
		t.Fatalf("safeAgentToolMaxTurns(20)=%d, want 20", got)
	}
}

func TestWrapPermissionPromptForwardsDecision(t *testing.T) {
	t.Parallel()
	tool := New(&fakeLLM{}, tools.NewRegistry(), agent.DefaultConfig(), "s1", func() string { return "llama2" }, func() string { return string(llm.ProviderOllamaLocal) })
	tool.promptTimeout = 2 * time.Second
	tool.SetPermissionPrompt(func(ctx context.Context, prompt permissions.Prompt) (permissions.Decision, string, error) {
		return permissions.DecisionAllow, "ok", nil
	})
	fn := tool.wrapPermissionPrompt()
	decision, reason, err := fn(context.Background(), permissions.Prompt{ToolName: "Bash"})
	if err != nil {
		t.Fatal(err)
	}
	if decision != permissions.DecisionAllow || reason != "ok" {
		t.Fatalf("unexpected prompt result: %v %q", decision, reason)
	}
}

func TestWrapPermissionPromptTimeoutReturnsDeny(t *testing.T) {
	t.Parallel()
	tool := New(&fakeLLM{}, tools.NewRegistry(), agent.DefaultConfig(), "s1", func() string { return "llama2" }, func() string { return string(llm.ProviderOllamaLocal) })
	tool.promptTimeout = 20 * time.Millisecond
	tool.SetPermissionPrompt(func(ctx context.Context, prompt permissions.Prompt) (permissions.Decision, string, error) {
		<-ctx.Done()
		return permissions.DecisionAllow, "late", nil
	})
	fn := tool.wrapPermissionPrompt()
	decision, reason, err := fn(context.Background(), permissions.Prompt{ToolName: "Bash"})
	if err != nil {
		t.Fatal(err)
	}
	if decision != permissions.DecisionDeny {
		t.Fatalf("decision = %v, want deny", decision)
	}
	if !strings.Contains(reason, "timeout") {
		t.Fatalf("reason = %q", reason)
	}
}

func TestCoordinatorDuplicateNameLaunchesNothing(t *testing.T) {
	t.Setenv("NANDOCODEGO_COORDINATOR", "1")
	tool := New(&fakeLLM{}, tools.NewRegistry(), agent.DefaultConfig(), "s1", func() string { return "llama2" }, func() string { return string(llm.ProviderOllamaLocal) })
	sup := tasks.NewSupervisor(filepath.Join(t.TempDir(), "tasks"), nil)
	tool.SetSupervisor(sup)
	release := make(chan struct{})
	defer close(release)
	holder, err := sup.Start(context.Background(), types.KindAgent, "holder", func(context.Context, *tasks.OutputWriter) (int, error) {
		<-release
		return 0, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sup.RegisterName("dup", holder); err != nil {
		t.Fatal(err)
	}
	if _, err := tool.Call(tools.Context{Context: context.Background()}, Input{Task: "x", Name: "dup"}, nil); err == nil {
		t.Fatal("expected duplicate-name error")
	}
	agents := 0
	for _, st := range sup.List() {
		if st.ToSummary().Kind == types.KindAgent {
			agents++
		}
	}
	if agents != 1 {
		t.Fatalf("a rejected launch left %d agent tasks, want only the holder", agents-1)
	}
	if got := sup.WorkerSnapshots(); len(got) != 0 {
		t.Fatalf("rejected launch was announced: %+v", got)
	}
}

func TestNormalSubagentEmitsWorkerEvents(t *testing.T) {
	tool := New(&fakeLLM{}, tools.NewRegistry(), agent.DefaultConfig(), "s1", func() string { return "llama2" }, func() string { return string(llm.ProviderOllamaLocal) })
	sup := tasks.NewSupervisor(filepath.Join(t.TempDir(), "tasks"), nil)
	tool.SetSupervisor(sup)
	var mu sync.Mutex
	var seen []string
	finished := make(chan struct{})
	sup.SetWorkerObserver(func(e tasks.WorkerEvent) {
		mu.Lock()
		seen = append(seen, e.Type)
		mu.Unlock()
		if e.Type == "finished" {
			close(finished)
		}
	})
	if _, err := tool.Call(tools.Context{Context: context.Background()}, Input{Task: "x", Model: "m"}, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		mu.Lock()
		defer mu.Unlock()
		t.Fatalf("no finished event; events=%v", seen)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) < 3 || seen[0] != "started" || seen[len(seen)-1] != "finished" {
		t.Fatalf("events=%v", seen)
	}
	hasUsage := false
	for _, e := range seen {
		hasUsage = hasUsage || e == "usage"
	}
	if !hasUsage {
		t.Fatalf("no usage event: %v", seen)
	}
	snaps := sup.WorkerSnapshots()
	if len(snaps) != 1 || snaps[0].Parent != "main" || snaps[0].Task != "x" {
		t.Fatalf("snapshots=%+v", snaps)
	}
}

func TestNormalSubagentPanicReturnsErrorAndFinishesOnce(t *testing.T) {
	orig := runSubagentFn
	runSubagentFn = func(context.Context, agent.Input, agent.SubagentParams, llm.Client, *tools.Registry, agent.Config) (string, error) {
		panic("boom")
	}
	defer func() { runSubagentFn = orig }()
	tool := New(&fakeLLM{}, tools.NewRegistry(), agent.DefaultConfig(), "s1", func() string { return "llama2" }, func() string { return string(llm.ProviderOllamaLocal) })
	sup := tasks.NewSupervisor(filepath.Join(t.TempDir(), "tasks"), nil)
	tool.SetSupervisor(sup)
	var mu sync.Mutex
	var statuses []types.TaskStatus
	finished := make(chan struct{}, 4)
	sup.SetWorkerObserver(func(e tasks.WorkerEvent) {
		if e.Type == "finished" {
			mu.Lock()
			statuses = append(statuses, e.Worker.Status)
			mu.Unlock()
			finished <- struct{}{}
		}
	})
	res, err := tool.Call(tools.Context{Context: context.Background()}, Input{Task: "x", Model: "m"}, nil)
	if err == nil || !strings.Contains(err.Error(), "sub-agent failed") {
		t.Fatalf("want sub-agent failed error, got res=%q err=%v", res.Display, err)
	}
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("no finished event")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(statuses) != 1 || statuses[0] != types.StatusFailed {
		t.Fatalf("finished statuses=%v", statuses)
	}
}
