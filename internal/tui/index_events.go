package tui

import (
	"github.com/FernasFragas/Nandocode/internal/semantic"
	tea "github.com/charmbracelet/bubbletea"
)

// tuiIndexEventSink bridges semantic progress events into Bubble Tea messages.
// It only forwards immutable messages and never mutates Model state directly.
type tuiIndexEventSink struct {
	send func(tea.Msg)
}

func (s tuiIndexEventSink) Publish(evt semantic.Event) {
	if s.send == nil {
		return
	}
	s.send(indexProgressMsg{Event: evt})
}

func (m *Model) newIndexEventSink() semantic.EventSink {
	if m == nil || m.program == nil {
		return tuiIndexEventSink{}
	}
	return tuiIndexEventSink{send: m.program.Send}
}
