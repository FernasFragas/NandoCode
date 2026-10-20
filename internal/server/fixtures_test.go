package server

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
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
	"github.com/FernasFragas/Nandocode/internal/tools/todo"
)

var updateGalleryFixtures = flag.Bool("update", false, "record gallery fixtures")

var galleryTime = time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)

type galleryRunner struct {
	scenario string
	// emitted reports whether the session has already published the given
	// event type for the given tool id. The session turns agent events into
	// SSE events on a separate goroutine, while a permission prompt emits
	// from the runner goroutine, so the real server promises no order
	// between the two. The recording waits here so it captures the order a
	// real run shows (tool_use_start, then permission_request).
	emitted func(eventType, id string) bool
}

func (r galleryRunner) waitEmitted(ctx context.Context, eventType, id string) bool {
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for !r.emitted(eventType, id) {
		select {
		case <-tick.C:
		case <-ctx.Done():
			return false
		}
	}
	return true
}

func (r galleryRunner) Run(ctx context.Context, in agent.Input) <-chan agent.Event {
	out := make(chan agent.Event)
	go func() {
		defer close(out)
		send := func(e agent.Event) bool {
			select {
			case out <- e:
				return true
			case <-ctx.Done():
				return false
			}
		}
		if !send(agent.AssistantThinkingDelta{Thinking: "Checking the project."}) {
			return
		}
		if !send(agent.ToolUseStart{ID: "tool-1", Name: "Grep", Input: map[string]any{"pattern": "TODO", "path": "src/main.go"}}) ||
			!send(agent.ToolUseResult{ID: "tool-1", Result: tools.Result{Display: "src/main.go:1: TODO"}}) {
			return
		}
		switch r.scenario {
		case "permission":
			if !send(agent.ToolUseStart{ID: "tool-2", Name: "Bash", Input: map[string]any{"command": "printf ok"}}) {
				return
			}
			if !r.waitEmitted(ctx, "tool_use_start", "tool-2") {
				return
			}
			decision, _, err := in.PermissionPrompt(ctx, permissions.Prompt{ToolName: "Bash", Target: "printf ok", Reason: "Run project checks"})
			if err != nil || decision != permissions.DecisionAllow {
				send(agent.Terminal{Reason: agent.TerminalUnrecoverable, Detail: "permission denied"})
				return
			}
			if !send(agent.ToolUseResult{ID: "tool-2", Result: tools.Result{Display: "ok"}}) {
				return
			}
		case "todo":
			items := []todo.TodoItem{{ID: "todo-1", Content: "Review src/main.go", Status: todo.TodoPending, Priority: todo.PriorityHigh}}
			if !send(agent.ToolUseStart{ID: "tool-2", Name: "TodoWrite", Input: map[string]any{"todos": items}}) {
				return
			}
			result, err := todo.NewTodoWriteTool().Call(in.ToolContext, todo.TodoWriteInput{Todos: items}, nil)
			if err != nil {
				send(agent.Terminal{Reason: agent.TerminalUnrecoverable, Detail: err.Error()})
				return
			}
			if !send(agent.ToolUseResult{ID: "tool-2", Result: result}) {
				return
			}
		}
		if !send(agent.AssistantTextDelta{Content: "Done."}) {
			return
		}
		toolCalls, turns := 1, 2
		if r.scenario != "basic" {
			toolCalls, turns = 2, 3
		}
		send(agent.Terminal{Reason: agent.TerminalCompleted, Usage: agent.Usage{Turns: turns, ToolCalls: toolCalls, DoneReason: "stop"}, Conversation: []llm.Message{{Role: llm.RoleAssistant, Content: "Done."}}})
	}()
	return out
}

func galleryEvents(t *testing.T, scenario string) []SessionEvent {
	t.Helper()
	root := t.TempDir()
	t.Setenv("NANDOCODEGO_DATA_HOME", filepath.Join(root, "data"))
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(project, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "src", "main.go"), []byte("// TODO\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := state.DefaultApp(bootstrap.New(bootstrap.DefaultInitial(project)).Snapshot())
	app.TodoList = todo.NewTodoList()
	var s *Session
	runner := galleryRunner{scenario: scenario, emitted: func(eventType, id string) bool {
		for _, e := range s.Replay("") {
			if e.Type == eventType && e.Data["id"] == id {
				return true
			}
		}
		return false
	}}
	s = newSession(context.Background(), "gallery-"+scenario, app, runner)
	defer s.stop()
	stream := make(chan SessionEvent, 32)
	s.mu.Lock()
	s.subscribers[stream] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.subscribers, stream)
		s.mu.Unlock()
	}()
	if err := s.StartRun(MessageRequest{Prompt: "Inspect src/main.go", MessageID: "message-1"}, agent.DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case event := <-stream:
			if event.Type == "permission_request" {
				id, _ := event.Data["request_id"].(string)
				if !s.broker.Resolve(id, decisionAllow) {
					t.Fatal("permission request could not be resolved")
				}
			}
			if event.Type == "terminal" {
				return stableGalleryEvents(s.Replay(""))
			}
		case <-deadline.C:
			t.Fatal("timed out waiting for gallery run")
		}
	}
}

func stableGalleryEvents(events []SessionEvent) []SessionEvent {
	keep := map[string]bool{"session_ready": true, "run_started": true, "assistant_thinking_delta": true, "assistant_text_delta": true, "tool_use_start": true, "tool_use_result": true, "permission_request": true, "terminal": true}
	out := make([]SessionEvent, 0, len(events))
	for _, event := range events {
		if !keep[event.Type] {
			continue
		}
		event.ID = fmt.Sprint(len(out) + 1)
		event.Time = galleryTime.Add(time.Duration(len(out)) * time.Second)
		if event.Type == "permission_request" {
			event.Data["request_id"] = "permission-1"
		}
		out = append(out, event)
	}
	return out
}

func TestRecordGalleryFixtures(t *testing.T) {
	for _, scenario := range []string{"basic", "permission", "todo"} {
		t.Run(scenario, func(t *testing.T) {
			events := galleryEvents(t, scenario)
			if len(events) < 5 || events[len(events)-1].Type != "terminal" || events[len(events)-1].Data["reason"] != agent.TerminalCompleted {
				t.Fatalf("incomplete fixture stream: %v", events)
			}
			got, err := json.MarshalIndent(events, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')
			path := filepath.Join("web", "dev", "fixtures", "run-"+scenario+".json")
			if *updateGalleryFixtures {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("fixture drift in %s: event shape or content changed; run with -update after review\nwant: %s\ngot: %s", path, strings.TrimSpace(string(want)), strings.TrimSpace(string(got)))
			}
		})
	}
}
