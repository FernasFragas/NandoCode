package tasks

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/FernasFragas/nandocodego/internal/agent"
	"github.com/FernasFragas/nandocodego/internal/llm"
	"github.com/FernasFragas/nandocodego/internal/tools"
	"github.com/FernasFragas/nandocodego/internal/types"
)

func TestReplayMessagesFromOutput(t *testing.T) {
	sup := NewSupervisor(filepath.Join(t.TempDir(), "tasks"), nil)
	id, err := sup.Start(context.Background(), types.KindAgent, "agent", AgentRunFunc(
		&replayFakeClient{},
		tools.NewRegistry(),
		agent.DefaultConfig(),
		"s1",
		tools.Context{WorkingDir: t.TempDir()},
		"task",
		"test-model",
		"",
	))
	if err != nil {
		t.Fatal(err)
	}
	waitTaskTerminal(t, sup, id)
	st, _ := sup.Get(id)
	msgs, err := ReplayMessagesFromOutput(st.ToSummary().OutputFile, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) == 0 {
		t.Fatal("expected replay messages")
	}
}

type replayFakeClient struct{}

func (c *replayFakeClient) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent, 2)
	ch <- llm.StreamEvent{Message: llm.Message{Role: llm.RoleAssistant, Content: "ok"}}
	ch <- llm.StreamEvent{Done: true, DoneReason: "stop"}
	close(ch)
	return ch, nil
}
func (c *replayFakeClient) Embed(ctx context.Context, model string, input []string) ([][]float32, error) {
	return nil, nil
}
func (c *replayFakeClient) ListModels(ctx context.Context) ([]llm.ModelInfo, error) { return nil, nil }
func (c *replayFakeClient) ShowModel(ctx context.Context, name string) (llm.ModelDetails, error) {
	return llm.ModelDetails{}, nil
}
func (c *replayFakeClient) PullModel(ctx context.Context, name string, progress chan<- llm.PullProgress) error {
	return nil
}

func waitTaskTerminal(t *testing.T, sup *Supervisor, id string) {
	t.Helper()
	for i := 0; i < 200; i++ {
		st, ok := sup.Get(id)
		if !ok {
			t.Fatalf("task %s missing", id)
		}
		s := st.ToSummary().Status
		if s == types.StatusCompleted || s == types.StatusFailed || s == types.StatusKilled {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("task %s did not reach terminal", id)
}
