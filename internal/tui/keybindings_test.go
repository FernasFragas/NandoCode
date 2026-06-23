package tui

import (
	"testing"
	"time"

	"github.com/FernasFragas/Nandocode/internal/state"
)

func TestBindingStackPushPopAndTop(t *testing.T) {
	s := NewBindingStack()
	if got := s.Top(); got != ContextGlobal {
		t.Fatalf("top=%q want global", got)
	}
	s.Push(ContextVimInsert)
	s.Push(ContextScroll)
	if got := s.Top(); got != ContextScroll {
		t.Fatalf("top=%q want scroll", got)
	}
	s.Pop(ContextScroll)
	if got := s.Top(); got != ContextVimInsert {
		t.Fatalf("top=%q want vim_insert", got)
	}
}

func TestSyncBindingContextsModalPriority(t *testing.T) {
	m := newTestModel(t)
	m.vim.EnterNormal()
	m.transcript = append(m.transcript, CreateSystemItem("row"))
	m.store.Set(func(app state.App) state.App {
		app.PermissionPrompt = &state.PermissionPrompt{ID: "p1", ToolName: "bash", Target: "pwd"}
		return app
	})
	m.syncBindingContexts()
	stack := m.bindingStack.Snapshot()
	if got := m.bindingStack.Top(); got != ContextModal {
		t.Fatalf("expected modal top context, got %q in stack %v", got, stack)
	}
	for _, c := range stack {
		if c == ContextScroll {
			t.Fatalf("expected no scroll context while modal is active, stack=%v", stack)
		}
	}

	m.store.Set(func(app state.App) state.App {
		app.PermissionPrompt = nil
		return app
	})
	m.syncBindingContexts()
	stack = m.bindingStack.Snapshot()
	if got := m.bindingStack.Top(); got != ContextScroll {
		t.Fatalf("expected scroll context restored after modal close, got %q in stack %v", got, stack)
	}
}

func TestChordInterceptorGGAndTimeout(t *testing.T) {
	c := NewChordInterceptor(time.Second)
	now := time.Now()
	c.Start("g", now)
	if action, ok := c.Match("g", now.Add(500*time.Millisecond)); !ok || action != "goto_top" {
		t.Fatalf("expected gg goto_top, got action=%q ok=%v", action, ok)
	}

	c.Start("g", now)
	if action, ok := c.Match("g", now.Add(2*time.Second)); ok || action != "" {
		t.Fatalf("expected expired chord miss, got action=%q ok=%v", action, ok)
	}
}
