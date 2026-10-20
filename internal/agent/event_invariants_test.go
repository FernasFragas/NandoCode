package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/tools"
)

// assertEventInvariants checks the event-stream contract of one agent run:
//   - exactly one Terminal, and it is the last event;
//   - every ToolUseResult and ToolUseProgress has an earlier ToolUseStart with
//     the same ID, and progress never follows the result;
//   - every ToolUseStart gets exactly one result when the run was not aborted;
//   - TurnUsage is cumulative and never decreases;
//   - tool messages in the conversation keep the order of the tool calls of the
//     assistant message that produced them (Ollama has no tool-call IDs, so
//     order is the only link).
func assertEventInvariants(t *testing.T, events []Event) Terminal {
	t.Helper()
	started := map[string]bool{}
	resulted := map[string]bool{}
	terminals := 0
	var terminal Terminal
	var lastUsage Usage
	for i, evt := range events {
		if terminals > 0 {
			t.Fatalf("event %d (%T) arrived after Terminal", i, evt)
		}
		switch e := evt.(type) {
		case ToolUseStart:
			if started[e.ID] {
				t.Fatalf("duplicate ToolUseStart for %s", e.ID)
			}
			started[e.ID] = true
		case ToolUseProgress:
			if !started[e.ID] {
				t.Fatalf("ToolUseProgress for %s has no earlier ToolUseStart", e.ID)
			}
			if resulted[e.ID] {
				t.Fatalf("ToolUseProgress for %s arrived after its result", e.ID)
			}
		case ToolUseResult:
			if !started[e.ID] {
				t.Fatalf("ToolUseResult for %s has no earlier ToolUseStart", e.ID)
			}
			if resulted[e.ID] {
				t.Fatalf("duplicate ToolUseResult for %s", e.ID)
			}
			resulted[e.ID] = true
		case TurnUsage:
			if e.Usage.Turns < lastUsage.Turns || e.Usage.EvalCount < lastUsage.EvalCount || e.Usage.ToolCalls < lastUsage.ToolCalls {
				t.Fatalf("TurnUsage went backwards: %+v after %+v", e.Usage, lastUsage)
			}
			lastUsage = e.Usage
		case Terminal:
			terminals++
			terminal = e
		}
	}
	if terminals != 1 {
		t.Fatalf("expected exactly one Terminal, got %d", terminals)
	}
	if terminal.Reason != TerminalAborted {
		for id := range started {
			if !resulted[id] {
				t.Fatalf("ToolUseStart %s never got a ToolUseResult", id)
			}
		}
	}
	assertToolMessageOrder(t, terminal.Conversation)
	return terminal
}

// assertToolMessageOrder checks that the tool messages following each assistant
// message with tool calls have the same names, in the same order, as the calls.
func assertToolMessageOrder(t *testing.T, conv []llm.Message) {
	t.Helper()
	for i := 0; i < len(conv); i++ {
		if conv[i].Role != llm.RoleAssistant || len(conv[i].ToolCalls) == 0 {
			continue
		}
		var want, got []string
		for _, c := range conv[i].ToolCalls {
			want = append(want, c.Function.Name)
		}
		for j := i + 1; j < len(conv) && conv[j].Role == llm.RoleTool; j++ {
			got = append(got, conv[j].ToolName)
		}
		if strings.Join(want, ",") != strings.Join(got, ",") {
			t.Fatalf("tool messages out of call order: calls=%v messages=%v", want, got)
		}
	}
}

func collectEvents(ch <-chan Event) []Event {
	var out []Event
	for evt := range ch {
		out = append(out, evt)
	}
	return out
}

func multiToolCallTurn(names ...string) fakeTurn {
	calls := make([]llm.ToolCall, 0, len(names))
	for _, n := range names {
		var c llm.ToolCall
		c.Function.Name = n
		c.Function.Arguments = map[string]any{}
		calls = append(calls, c)
	}
	return fakeTurn{events: []llm.StreamEvent{
		{Message: llm.Message{ToolCalls: calls}},
		{Done: true, DoneReason: "stop"},
	}}
}

func testSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

func anyInput(raw json.RawMessage) (any, error) { return map[string]any{}, nil }

// probeTool is a read-only, concurrency-safe tool that reports two progress events.
func probeTool(name string) tools.Tool {
	return tools.BuildTool(tools.Spec{
		Name:              name,
		Description:       "Probe tool.",
		Schema:            testSchema(),
		Unmarshal:         anyInput,
		IsReadOnlyFunc:    func(any) bool { return true },
		IsConcurrentFunc:  func(any) bool { return true },
		IsDestructiveFunc: func(any) bool { return false },
		CallFunc: func(_ tools.Context, _ any, progress chan<- tools.ProgressEvent) (tools.Result, error) {
			if progress != nil {
				progress <- tools.ProgressEvent{Tool: name, Message: "one"}
				progress <- tools.ProgressEvent{Tool: name, Message: "two"}
			}
			return tools.Result{Display: name + " done"}, nil
		},
	})
}

// mutateTool is not read-only: in default mode with no prompt it is denied.
func mutateTool(name string) tools.Tool {
	return tools.BuildTool(tools.Spec{
		Name:        name,
		Description: "Mutating tool.",
		Schema:      testSchema(),
		Unmarshal:   anyInput,
		CallFunc: func(tools.Context, any, chan<- tools.ProgressEvent) (tools.Result, error) {
			return tools.Result{Display: name + " done"}, nil
		},
	})
}

func invariantRegistry(t *testing.T, ts ...tools.Tool) *tools.Registry {
	t.Helper()
	reg := tools.NewRegistry()
	for _, tl := range ts {
		if err := reg.Register(tl); err != nil {
			t.Fatal(err)
		}
	}
	return reg
}

func runInvariantAgent(t *testing.T, reg *tools.Registry, turns ...fakeTurn) []Event {
	t.Helper()
	ag, err := New(newFakeClient(turns), reg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	return collectEvents(ag.Run(ctx, Input{
		Model:    "test-model",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "go"}},
		ToolContext: tools.Context{
			Context:    ctx,
			WorkingDir: t.TempDir(),
		},
	}))
}

func TestEventInvariantsDeniedToolHasStart(t *testing.T) {
	reg := invariantRegistry(t, probeTool("ProbeA"), mutateTool("Mutate"))
	events := runInvariantAgent(t, reg, multiToolCallTurn("Mutate"), textTurn("done"))
	assertEventInvariants(t, events)
}

func TestEventInvariantsMixedBatchKeepsCallOrder(t *testing.T) {
	reg := invariantRegistry(t, probeTool("ProbeA"), probeTool("ProbeB"), mutateTool("Mutate"))
	events := runInvariantAgent(t, reg,
		multiToolCallTurn("Missing", "ProbeA", "Mutate", "ProbeB"),
		textTurn("done"))
	term := assertEventInvariants(t, events)
	if term.Reason != TerminalCompleted {
		t.Fatalf("terminal=%s %s", term.Reason, term.Detail)
	}
	var names []string
	for _, m := range term.Conversation {
		if m.Role == llm.RoleTool {
			names = append(names, m.ToolName)
		}
	}
	if got, want := strings.Join(names, ","), "Missing,ProbeA,Mutate,ProbeB"; got != want {
		t.Fatalf("tool message order=%s want=%s", got, want)
	}
}

func TestEventInvariantsEmitToolUseProgress(t *testing.T) {
	// Two safe probes run as a concurrent batch; a lone probe runs singly.
	for _, calls := range [][]string{{"ProbeA", "ProbeB"}, {"ProbeA"}} {
		reg := invariantRegistry(t, probeTool("ProbeA"), probeTool("ProbeB"))
		events := runInvariantAgent(t, reg, multiToolCallTurn(calls...), textTurn("done"))
		assertEventInvariants(t, events)
		progress := 0
		for _, evt := range events {
			if _, ok := evt.(ToolUseProgress); ok {
				progress++
			}
		}
		if want := 2 * len(calls); progress != want {
			t.Fatalf("calls=%v: ToolUseProgress=%d want=%d", calls, progress, want)
		}
	}
}

// cancelOnChatClient cancels the run context from inside Chat and fails.
type cancelOnChatClient struct {
	fakeClient
	cancel context.CancelFunc
}

func (c *cancelOnChatClient) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	c.cancel()
	return nil, fmt.Errorf("request failed: %w", ctx.Err())
}

func TestTerminalNotLostOnCancel(t *testing.T) {
	for i := 0; i < 100; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		client := &cancelOnChatClient{cancel: cancel}
		ag, err := New(client, nil)
		if err != nil {
			t.Fatal(err)
		}
		events := collectEvents(ag.Run(ctx, Input{
			Model:    "test-model",
			Messages: []llm.Message{{Role: llm.RoleUser, Content: "go"}},
		}))
		cancel()
		assertEventInvariants(t, events)
	}
}

// hangingClient opens a stream that never produces data and records the
// context the upstream request ran under.
type hangingClient struct {
	fakeClient
	mu     sync.Mutex
	reqCtx context.Context
}

func (c *hangingClient) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	c.mu.Lock()
	c.reqCtx = ctx
	c.mu.Unlock()
	return make(chan llm.StreamEvent), nil
}

func TestWatchdogTimeoutCancelsUpstreamRequest(t *testing.T) {
	client := &hangingClient{}
	wd := llm.WatchdogConfig{IdleTimeout: 30 * time.Millisecond}
	ag, err := New(client, nil, WithWatchdog(wd))
	if err != nil {
		t.Fatal(err)
	}
	events := collectEvents(ag.Run(context.Background(), Input{
		Model:    "test-model",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "go"}},
	}))
	term := assertEventInvariants(t, events)
	if term.Reason != TerminalUnrecoverable {
		t.Fatalf("terminal=%s want unrecoverable", term.Reason)
	}
	client.mu.Lock()
	reqCtx := client.reqCtx
	client.mu.Unlock()
	select {
	case <-reqCtx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("watchdog timeout left the upstream request context running")
	}
}
