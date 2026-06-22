package todo

import (
	"context"
	"sync"
	"testing"

	"github.com/FernasFragas/nandocodego/internal/tools"
)

func makeCtx(t *testing.T) tools.Context {
	t.Helper()
	ctx := tools.DefaultContext(context.Background(), t.TempDir())
	ctx.PermissionMode = tools.PermissionBypassPermissions
	ctx.TodoList = NewTodoList()
	return ctx
}

func validItem(id string) TodoItem {
	return TodoItem{ID: id, Content: "Do something", Status: TodoPending, Priority: PriorityHigh}
}

func TestTodo_WriteAndRead(t *testing.T) {
	ctx := makeCtx(t)
	wTool := NewTodoWriteTool()
	rTool := NewTodoReadTool()

	items := []TodoItem{
		validItem("1"),
		{ID: "2", Content: "Fix bug", Status: TodoInProgress, Priority: PriorityMedium},
	}
	_, err := wTool.Call(ctx, TodoWriteInput{Todos: items}, nil)
	if err != nil {
		t.Fatal(err)
	}

	res, err := rTool.Call(ctx, TodoReadInput{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := res.Data.([]TodoItem)
	if len(got) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got))
	}
	if got[0].ID != "1" || got[1].ID != "2" {
		t.Error("items returned in wrong order or wrong IDs")
	}
}

func TestTodo_WriteReplacesAll(t *testing.T) {
	ctx := makeCtx(t)
	wTool := NewTodoWriteTool()

	// Write first list
	_, err := wTool.Call(ctx, TodoWriteInput{Todos: []TodoItem{validItem("old")}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Replace with new list
	_, err = wTool.Call(ctx, TodoWriteInput{Todos: []TodoItem{validItem("new")}}, nil)
	if err != nil {
		t.Fatal(err)
	}

	rTool := NewTodoReadTool()
	res, _ := rTool.Call(ctx, TodoReadInput{}, nil)
	items := res.Data.([]TodoItem)
	if len(items) != 1 || items[0].ID != "new" {
		t.Errorf("expected only 'new' item, got %v", items)
	}
}

func TestTodo_DuplicateIDRejected(t *testing.T) {
	ctx := makeCtx(t)
	wTool := NewTodoWriteTool()
	_, err := wTool.Call(ctx, TodoWriteInput{Todos: []TodoItem{validItem("dup"), validItem("dup")}}, nil)
	if err == nil {
		t.Error("expected duplicate ID error")
	}
}

func TestTodo_InvalidStatusRejected(t *testing.T) {
	ctx := makeCtx(t)
	wTool := NewTodoWriteTool()
	item := TodoItem{ID: "1", Content: "Do it", Status: "invalid", Priority: PriorityHigh}
	_, err := wTool.Call(ctx, TodoWriteInput{Todos: []TodoItem{item}}, nil)
	if err == nil {
		t.Error("expected invalid status error")
	}
}

func TestTodo_InvalidPriorityRejected(t *testing.T) {
	ctx := makeCtx(t)
	wTool := NewTodoWriteTool()
	item := TodoItem{ID: "1", Content: "Do it", Status: TodoPending, Priority: "urgent"}
	_, err := wTool.Call(ctx, TodoWriteInput{Todos: []TodoItem{item}}, nil)
	if err == nil {
		t.Error("expected invalid priority error")
	}
}

func TestTodo_EmptyContentRejected(t *testing.T) {
	ctx := makeCtx(t)
	wTool := NewTodoWriteTool()
	item := TodoItem{ID: "1", Content: "", Status: TodoPending, Priority: PriorityLow}
	_, err := wTool.Call(ctx, TodoWriteInput{Todos: []TodoItem{item}}, nil)
	if err == nil {
		t.Error("expected empty content error")
	}
}

func TestTodo_ReadEmptyList(t *testing.T) {
	ctx := makeCtx(t)
	rTool := NewTodoReadTool()
	res, err := rTool.Call(ctx, TodoReadInput{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Display == "" {
		t.Error("expected non-empty display for empty list")
	}
}

func TestTodo_ConcurrentWriteRead(t *testing.T) {
	ctx := makeCtx(t)
	wTool := NewTodoWriteTool()
	rTool := NewTodoReadTool()

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			items := []TodoItem{{
				ID:       "item",
				Content:  "concurrent",
				Status:   TodoPending,
				Priority: PriorityLow,
			}}
			wTool.Call(ctx, TodoWriteInput{Todos: items}, nil)
		}(i)
		go func() {
			defer wg.Done()
			rTool.Call(ctx, TodoReadInput{}, nil)
		}()
	}
	wg.Wait()
}

func TestTodo_NilTodoListReturnsError(t *testing.T) {
	ctx := tools.DefaultContext(context.Background(), t.TempDir())
	ctx.PermissionMode = tools.PermissionBypassPermissions
	// TodoList is nil
	wTool := NewTodoWriteTool()
	_, err := wTool.Call(ctx, TodoWriteInput{Todos: []TodoItem{validItem("1")}}, nil)
	if err == nil {
		t.Error("expected error for nil TodoList")
	}
}

func TestTodo_ChecklistDisplay(t *testing.T) {
	items := []TodoItem{
		{ID: "1", Content: "Done", Status: TodoCompleted, Priority: PriorityHigh},
		{ID: "2", Content: "Working", Status: TodoInProgress, Priority: PriorityMedium},
		{ID: "3", Content: "Pending", Status: TodoPending, Priority: PriorityLow},
	}
	display := checklistDisplay(items)
	if !contains(display, "[x]") {
		t.Error("expected [x] for completed")
	}
	if !contains(display, "[~]") {
		t.Error("expected [~] for in_progress")
	}
	if !contains(display, "[ ]") {
		t.Error("expected [ ] for pending")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
