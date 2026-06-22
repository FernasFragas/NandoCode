// Package todo implements the TodoWrite and TodoRead tools.
package todo

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/FernasFragas/nandocodego/internal/tools"
)

// TodoStatus is the status of a todo item.
type TodoStatus string

const (
	TodoPending    TodoStatus = "pending"
	TodoInProgress TodoStatus = "in_progress"
	TodoCompleted  TodoStatus = "completed"
)

// TodoPriority is the priority of a todo item.
type TodoPriority string

const (
	PriorityHigh   TodoPriority = "high"
	PriorityMedium TodoPriority = "medium"
	PriorityLow    TodoPriority = "low"
)

// TodoItem is a single item in the todo list.
type TodoItem struct {
	ID       string       `json:"id"`
	Content  string       `json:"content"`
	Status   TodoStatus   `json:"status"`
	Priority TodoPriority `json:"priority"`
}

// TodoList is a session-scoped, mutex-protected todo list.
type TodoList struct {
	mu    sync.RWMutex
	items []TodoItem
}

// NewTodoList creates an empty TodoList.
func NewTodoList() *TodoList {
	return &TodoList{}
}

// Replace validates and atomically replaces the full todo list.
func (tl *TodoList) Replace(items []TodoItem) error {
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.ID) == "" {
			return errors.New("todo item ID must not be empty")
		}
		if strings.TrimSpace(item.Content) == "" {
			return fmt.Errorf("todo item %q content must not be empty", item.ID)
		}
		if !validStatus(item.Status) {
			return fmt.Errorf("todo item %q has invalid status %q; must be pending, in_progress, or completed", item.ID, item.Status)
		}
		if !validPriority(item.Priority) {
			return fmt.Errorf("todo item %q has invalid priority %q; must be high, medium, or low", item.ID, item.Priority)
		}
		if seen[item.ID] {
			return fmt.Errorf("duplicate todo ID %q", item.ID)
		}
		seen[item.ID] = true
	}
	tl.mu.Lock()
	tl.items = append([]TodoItem(nil), items...)
	tl.mu.Unlock()
	return nil
}

// All returns a copy of the current todo list.
func (tl *TodoList) All() []TodoItem {
	tl.mu.RLock()
	defer tl.mu.RUnlock()
	return append([]TodoItem(nil), tl.items...)
}

func validStatus(s TodoStatus) bool {
	return s == TodoPending || s == TodoInProgress || s == TodoCompleted
}

func validPriority(p TodoPriority) bool {
	return p == PriorityHigh || p == PriorityMedium || p == PriorityLow
}

// checklistDisplay formats items as a checklist string.
func checklistDisplay(items []TodoItem) string {
	if len(items) == 0 {
		return "(empty todo list)\n"
	}
	var b strings.Builder
	for _, item := range items {
		var marker string
		switch item.Status {
		case TodoCompleted:
			marker = "[x]"
		case TodoInProgress:
			marker = "[~]"
		default:
			marker = "[ ]"
		}
		fmt.Fprintf(&b, "- %s %s [%s]\n", marker, item.Content, item.Priority)
	}
	return b.String()
}

// TodoWriteInput is the input for TodoWrite.
type TodoWriteInput struct {
	Todos []TodoItem `json:"todos"`
}

// TodoReadInput is the input for TodoRead (empty).
type TodoReadInput struct{}

// NewTodoWriteTool creates a TodoWrite tool.
func NewTodoWriteTool() tools.Tool {
	return tools.BuildTool(tools.Spec{
		Name:        "TodoWrite",
		Description: "Replace the session todo list with a new list of items.",
		Schema:      todoWriteSchema(),
		Unmarshal:   unmarshalWrite,
		IsReadOnlyFunc: func(input any) bool {
			return false
		},
		IsConcurrentFunc: func(input any) bool {
			return false
		},
		IsDestructiveFunc: func(input any) bool {
			return false
		},
		CheckPermFunc: func(ctx tools.Context, input any) tools.PermissionResult {
			return tools.PermissionResult{Decision: tools.PermAllow, UpdatedInput: input}
		},
		CallFunc: callWrite,
		RenderFunc: func(input any, result tools.Result) tools.RenderHints {
			in, _ := input.(TodoWriteInput)
			return tools.RenderHints{Title: "TodoWrite", Summary: fmt.Sprintf("%d todos", len(in.Todos))}
		},
	})
}

// NewTodoReadTool creates a TodoRead tool.
func NewTodoReadTool() tools.Tool {
	return tools.BuildTool(tools.Spec{
		Name:        "TodoRead",
		Description: "Read the current session todo list.",
		Schema:      todoReadSchema(),
		Unmarshal: func(raw json.RawMessage) (any, error) {
			return TodoReadInput{}, nil
		},
		IsReadOnlyFunc: func(input any) bool {
			return true
		},
		IsConcurrentFunc: func(input any) bool {
			return true
		},
		IsDestructiveFunc: func(input any) bool {
			return false
		},
		CheckPermFunc: func(ctx tools.Context, input any) tools.PermissionResult {
			return tools.PermissionResult{Decision: tools.PermAllow, UpdatedInput: input}
		},
		CallFunc: callRead,
		RenderFunc: func(input any, result tools.Result) tools.RenderHints {
			items, _ := result.Data.([]TodoItem)
			return tools.RenderHints{Title: "TodoRead", Summary: fmt.Sprintf("%d todos", len(items))}
		},
	})
}

func todoWriteSchema() map[string]any {
	itemSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":       tools.StringProperty("Unique identifier for the todo item."),
			"content":  tools.StringProperty("Description of the todo item."),
			"status":   map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed"}, "description": "Status of the item."},
			"priority": map[string]any{"type": "string", "enum": []string{"high", "medium", "low"}, "description": "Priority of the item."},
		},
		"required": []string{"id", "content", "status", "priority"},
	}
	return tools.ObjectSchema(map[string]any{
		"todos": map[string]any{
			"type":        "array",
			"items":       itemSchema,
			"description": "The complete list of todo items (replaces current list).",
		},
	}, []string{"todos"})
}

func todoReadSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func unmarshalWrite(raw json.RawMessage) (any, error) {
	var input TodoWriteInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if input.Todos == nil {
		return nil, errors.New("todos field is required")
	}
	return input, nil
}

func getTodoList(ctx tools.Context) (*TodoList, error) {
	if ctx.TodoList == nil {
		return nil, errors.New("todo list not initialized in this session")
	}
	tl, ok := ctx.TodoList.(*TodoList)
	if !ok {
		return nil, errors.New("invalid todo list type in context")
	}
	return tl, nil
}

func callWrite(ctx tools.Context, input any, _ chan<- tools.ProgressEvent) (tools.Result, error) {
	in, ok := input.(TodoWriteInput)
	if !ok {
		return tools.Result{}, errors.New("invalid TodoWrite input")
	}
	tl, err := getTodoList(ctx)
	if err != nil {
		return tools.Result{}, err
	}
	if err := tl.Replace(in.Todos); err != nil {
		return tools.Result{}, err
	}
	display := checklistDisplay(in.Todos)
	return tools.Result{Data: in.Todos, Display: display}, nil
}

func callRead(ctx tools.Context, input any, _ chan<- tools.ProgressEvent) (tools.Result, error) {
	tl, err := getTodoList(ctx)
	if err != nil {
		return tools.Result{}, err
	}
	items := tl.All()
	display := checklistDisplay(items)
	return tools.Result{Data: items, Display: display}, nil
}
