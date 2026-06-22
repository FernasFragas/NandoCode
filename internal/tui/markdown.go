package tui

import (
	"github.com/charmbracelet/glamour"
)

// MarkdownRenderer caches a Glamour renderer and resizes it as needed.
type MarkdownRenderer struct {
	width    int
	renderer *glamour.TermRenderer
}

// NewMarkdownRenderer creates a new cached markdown renderer.
func NewMarkdownRenderer(width int) (*MarkdownRenderer, error) {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, err
	}

	return &MarkdownRenderer{
		width:    width,
		renderer: renderer,
	}, nil
}

// Render renders markdown to ANSI string.
func (r *MarkdownRenderer) Render(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	return r.renderer.Render(s)
}

// Resize updates the render width if it has changed.
func (r *MarkdownRenderer) Resize(width int) error {
	if width == r.width {
		return nil
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return err
	}

	r.width = width
	r.renderer = renderer
	return nil
}
