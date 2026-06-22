package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestInputPreprocessorBracketedPasteLifecycle(t *testing.T) {
	p := NewInputPreprocessor()

	start := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(bracketedPasteStart)}
	_, consume := p.Process(start)
	if !consume {
		t.Fatal("expected start delimiter to be consumed")
	}

	payload := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hello")}
	got, consume := p.Process(payload)
	if consume {
		t.Fatal("expected payload not consumed")
	}
	if !got.Paste {
		t.Fatal("expected payload inside bracketed paste to be marked as Paste")
	}

	end := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(bracketedPasteEnd)}
	_, consume = p.Process(end)
	if !consume {
		t.Fatal("expected end delimiter to be consumed")
	}

	plain := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}
	got, _ = p.Process(plain)
	if got.Paste {
		t.Fatal("expected paste marker cleared after bracketed paste end")
	}
}

