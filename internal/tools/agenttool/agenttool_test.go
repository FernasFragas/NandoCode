package agenttool

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/permissions"
	"github.com/FernasFragas/Nandocode/internal/tasks"
	"github.com/FernasFragas/Nandocode/internal/tools"
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
