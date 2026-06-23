package tasks

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/tools"
	"github.com/FernasFragas/Nandocode/internal/types"
)

type mailboxFakeClient struct {
	mu       sync.Mutex
	requests []*llm.ChatRequest
	calls    int
}

func (c *mailboxFakeClient) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	c.mu.Lock()
	c.calls++
	call := c.calls
	c.requests = append(c.requests, req)
	c.mu.Unlock()
	ch := make(chan llm.StreamEvent, 2)
	if call == 1 {
		ch <- llm.StreamEvent{Message: llm.Message{Role: llm.RoleAssistant, Content: "turn1"}}
		time.Sleep(100 * time.Millisecond)
		ch <- llm.StreamEvent{Done: true, DoneReason: "stop"}
	} else {
		ch <- llm.StreamEvent{Message: llm.Message{Role: llm.RoleAssistant, Content: "turn2"}}
		ch <- llm.StreamEvent{Done: true, DoneReason: "stop"}
	}
	close(ch)
	return ch, nil
}
func (c *mailboxFakeClient) Embed(ctx context.Context, model string, input []string) ([][]float32, error) {
	return nil, nil
}
func (c *mailboxFakeClient) ListModels(ctx context.Context) ([]llm.ModelInfo, error) { return nil, nil }
func (c *mailboxFakeClient) ShowModel(ctx context.Context, name string) (llm.ModelDetails, error) {
	return llm.ModelDetails{}, nil
}
func (c *mailboxFakeClient) PullModel(ctx context.Context, name string, progress chan<- llm.PullProgress) error {
	return nil
}

func TestAgentRunFuncWithMailboxDrainsMessages(t *testing.T) {
	sup := NewSupervisor(t.TempDir(), nil)
	fc := &mailboxFakeClient{}
	taskIDCh := make(chan string, 1)
	id, err := sup.Start(context.Background(), types.KindAgent, "worker", AgentRunFuncWithMailbox(
		fc,
		tools.NewRegistry(),
		agent.DefaultConfig(),
		"s1",
		tools.Context{WorkingDir: t.TempDir()},
		"do work",
		"test-model",
		"",
		sup,
		taskIDCh,
	))
	if err != nil {
		t.Fatal(err)
	}
	taskIDCh <- id
	// queue after first turn likely completed, before next request build
	time.Sleep(10 * time.Millisecond)
	if err := sup.QueueMessage(id, PendingMessage{ID: "m1", FromAgent: "coordinator", ToAgent: id, Content: "follow-up"}); err != nil {
		t.Fatal(err)
	}
	waitTaskTerminal(t, sup, id)
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if len(fc.requests) < 2 {
		t.Fatalf("expected >=2 requests, got %d", len(fc.requests))
	}
	found := false
	for _, m := range fc.requests[1].Messages {
		if m.Role == llm.RoleUser && m.Content != "" && m.Content != "do work" {
			if m.Content == "[mailbox from=coordinator id=m1] follow-up" {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("expected mailbox message in second request")
	}
}
