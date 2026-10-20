package server

import (
	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/tools/todo"
)

func (s *Session) noteTodoToolStart(e agent.ToolUseStart) {
	if e.Name != "TodoWrite" {
		return
	}
	s.mu.Lock()
	s.todoWrites[e.ID] = struct{}{}
	s.mu.Unlock()
}

func (s *Session) emitTodoUpdatedAfterResult(e agent.ToolUseResult) {
	s.mu.Lock()
	_, wasTodoWrite := s.todoWrites[e.ID]
	delete(s.todoWrites, e.ID)
	list, ok := s.appState.TodoList.(*todo.TodoList)
	s.mu.Unlock()
	if !wasTodoWrite || e.Err != nil || !ok {
		return
	}
	items := list.All()
	if items == nil {
		items = []todo.TodoItem{}
	}
	s.Emit("todo_updated", map[string]any{"todos": items})
}
