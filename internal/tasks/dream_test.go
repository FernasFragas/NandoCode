package tasks

import (
	"context"
	"testing"
	"time"

	"github.com/FernasFragas/nandocodego/internal/llm"
	"github.com/FernasFragas/nandocodego/internal/types"
)

type dreamFakeClient struct {
	block bool
}

func (f *dreamFakeClient) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent, 2)
	go func() {
		defer close(ch)
		if f.block {
			<-ctx.Done()
			return
		}
		ch <- llm.StreamEvent{Message: llm.Message{Role: llm.RoleAssistant, Content: "dream-output"}}
		ch <- llm.StreamEvent{Done: true, DoneReason: "stop"}
	}()
	return ch, nil
}
func (f *dreamFakeClient) Embed(ctx context.Context, model string, input []string) ([][]float32, error) {
	return nil, nil
}
func (f *dreamFakeClient) ListModels(ctx context.Context) ([]llm.ModelInfo, error) {
	return nil, nil
}
func (f *dreamFakeClient) ShowModel(ctx context.Context, name string) (llm.ModelDetails, error) {
	return llm.ModelDetails{}, nil
}
func (f *dreamFakeClient) PullModel(ctx context.Context, name string, progress chan<- llm.PullProgress) error {
	return nil
}

func TestSpawnDreamAndConsumeResult(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(t.TempDir(), nil)
	id, err := s.SpawnDream(context.Background(), &dreamFakeClient{}, "m", "sys")
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("expected dream task id")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if got := s.ConsumeDreamResult(30 * time.Second); got != "" {
			if got != "dream-output" {
				t.Fatalf("unexpected dream output %q", got)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("dream did not produce result")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSpawnDreamSkippedWhileActiveWorkers(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(t.TempDir(), nil)
	workerID, err := s.Start(context.Background(), types.KindAgent, "worker", func(ctx context.Context, out *OutputWriter) (int, error) {
		<-ctx.Done()
		return 137, ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Stop(workerID) })
	id, err := s.SpawnDream(context.Background(), &dreamFakeClient{}, "m", "sys")
	if err != nil {
		t.Fatal(err)
	}
	if id != "" {
		t.Fatalf("expected no dream id while workers active, got %q", id)
	}
}

func TestKillDreamReturnsQuickly(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(t.TempDir(), nil)
	id, err := s.SpawnDream(context.Background(), &dreamFakeClient{block: true}, "m", "sys")
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("expected dream task id")
	}
	start := time.Now()
	if err := s.KillDream(); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatalf("KillDream took too long: %s", time.Since(start))
	}
}

func TestDreamNotIncludedInList(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(t.TempDir(), nil)
	_, err := s.SpawnDream(context.Background(), &dreamFakeClient{}, "m", "sys")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	for _, st := range s.List() {
		if st.ToSummary().Kind == types.KindDream {
			t.Fatal("dream task should not appear in List")
		}
	}
}
