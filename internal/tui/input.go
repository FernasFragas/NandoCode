package tui

import tea "github.com/charmbracelet/bubbletea"

const (
	bracketedPasteStart = "\x1b[200~"
	bracketedPasteEnd   = "\x1b[201~"
)

// InputPreprocessor normalizes raw key events before vim handling.
type InputPreprocessor struct {
	inBracketedPaste bool
}

func NewInputPreprocessor() *InputPreprocessor {
	return &InputPreprocessor{}
}

// Process normalizes bracketed paste events.
// Returns (normalized, consume=true) for delimiter-only control events.
func (p *InputPreprocessor) Process(msg tea.KeyMsg) (tea.KeyMsg, bool) {
	if msg.Type != tea.KeyRunes {
		if p.inBracketedPaste {
			msg.Paste = true
		}
		return msg, false
	}
	raw := string(msg.Runes)
	switch raw {
	case bracketedPasteStart:
		p.inBracketedPaste = true
		return msg, true
	case bracketedPasteEnd:
		p.inBracketedPaste = false
		return msg, true
	default:
		if p.inBracketedPaste {
			msg.Paste = true
		}
		return msg, false
	}
}

