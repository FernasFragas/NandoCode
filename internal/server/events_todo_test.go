package server

import (
	"context"
	"reflect"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/bootstrap"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/state"
	"github.com/FernasFragas/Nandocode/internal/tools"
	"github.com/FernasFragas/Nandocode/internal/tools/todo"
)

func TestServerSessionTodoWriteEmitsTodoUpdated(t *testing.T) {
	items := []todo.TodoItem{{ID: "one", Content: "Fix test", Status: todo.TodoInProgress, Priority: todo.PriorityHigh}}
	call := llm.ToolCall{}
	call.Function.Name = "TodoWrite"
	call.Function.Arguments = map[string]any{"todos": items}
	client := &usageTurnClient{turns: [][]llm.StreamEvent{
		{{Message: llm.Message{ToolCalls: []llm.ToolCall{call}}}, {Done: true, DoneReason: "stop"}},
		{{Message: llm.Message{Content: "done"}}, {Done: true, DoneReason: "stop"}},
	}}
	registry := tools.NewRegistry()
	if err := registry.Register(todo.NewTodoWriteTool()); err != nil {
		t.Fatal(err)
	}
	s := testTurnSession(t, client, registry)
	runAndWait(t, s, "make a todo")
	var got []todo.TodoItem
	resultIndex, updateIndex := -1, -1
	for i, event := range s.Replay("") {
		if event.Type == "tool_use_result" {
			if event.Data["error"] != "" {
				t.Fatalf("TodoWrite failed: %v", event.Data["error"])
			}
			resultIndex = i
		}
		if event.Type == "todo_updated" {
			updateIndex = i
			var ok bool
			got, ok = event.Data["todos"].([]todo.TodoItem)
			if !ok {
				t.Fatalf("todo payload = %#v", event.Data)
			}
		}
	}
	if !reflect.DeepEqual(got, items) {
		t.Fatalf("todos = %+v, want %+v", got, items)
	}
	if resultIndex < 0 || updateIndex <= resultIndex {
		t.Fatalf("result index %d, update index %d", resultIndex, updateIndex)
	}
}

func TestServerTodoListsAreSessionScoped(t *testing.T) {
	t.Setenv("NANDOCODEGO_DATA_HOME", t.TempDir())
	app := state.DefaultApp(bootstrap.New(bootstrap.DefaultInitial(t.TempDir())).Snapshot())
	first := newSession(context.Background(), "first", app, tinyRunner{})
	second := newSession(context.Background(), "second", app, tinyRunner{})
	t.Cleanup(first.stop)
	t.Cleanup(second.stop)
	firstList := first.appState.TodoList.(*todo.TodoList)
	secondList := second.appState.TodoList.(*todo.TodoList)
	if firstList == secondList {
		t.Fatal("sessions share a todo list")
	}
	if err := firstList.Replace([]todo.TodoItem{{ID: "one", Content: "private", Status: todo.TodoPending, Priority: todo.PriorityLow}}); err != nil {
		t.Fatal(err)
	}
	if got := secondList.All(); len(got) != 0 {
		t.Fatalf("second session sees first todos: %+v", got)
	}
}
