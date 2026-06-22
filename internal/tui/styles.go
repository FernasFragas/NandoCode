package tui

import "github.com/charmbracelet/lipgloss"

// Styles holds styling for the TUI.
type Styles struct {
	Border            lipgloss.Style
	Help              lipgloss.Style
	StatusBar         lipgloss.Style
	StatusSuccess     lipgloss.Style
	StatusError       lipgloss.Style
	SemMuted          lipgloss.Style
	SemAccent         lipgloss.Style
	SemWarning        lipgloss.Style
	SemInfo           lipgloss.Style
	ThinkingCollapsed lipgloss.Style
	ThinkingExpanded  lipgloss.Style
	ThinkingBox       lipgloss.Style
	ToolPanel         lipgloss.Style
	Modal             lipgloss.Style
	ModalTitle        lipgloss.Style
	ModalButton       lipgloss.Style
	PickerPanel       lipgloss.Style
	PickerItem        lipgloss.Style
	PickerSelected    lipgloss.Style
	PickerMatch       lipgloss.Style
	PickerDetail      lipgloss.Style
	PickerHint        lipgloss.Style
}

// DefaultStyles returns a default style set.
func DefaultStyles() Styles {
	return Styles{
		Border: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1),

		Help: lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			Italic(true),

		StatusBar: lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("17")).
			Padding(0, 1),

		StatusSuccess: lipgloss.NewStyle().
			Foreground(lipgloss.Color("2")).
			Bold(true),

		StatusError: lipgloss.NewStyle().
			Foreground(lipgloss.Color("1")).
			Bold(true),

		SemMuted: lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")),

		SemAccent: lipgloss.NewStyle().
			Foreground(lipgloss.Color("111")).
			Bold(true),

		SemWarning: lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Bold(true),

		SemInfo: lipgloss.NewStyle().
			Foreground(lipgloss.Color("45")),

		ThinkingCollapsed: lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			Italic(true),

		ThinkingExpanded: lipgloss.NewStyle().
			Foreground(lipgloss.Color("111")).
			Italic(true),

		ThinkingBox: lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")).
			BorderLeft(true).
			BorderStyle(lipgloss.ThickBorder()).
			BorderForeground(lipgloss.Color("237")).
			PaddingLeft(1),

		ToolPanel: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1).
			MarginBottom(1),

		Modal: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(1).
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("233")),

		ModalTitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("207")),

		ModalButton: lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("62")).
			Padding(0, 2),

		PickerPanel: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("241")).
			Padding(0, 1),

		PickerItem: lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")),

		PickerSelected: lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("24")),

		PickerMatch: lipgloss.NewStyle().
			Bold(true),

		PickerDetail: lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")),

		PickerHint: lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")),
	}
}
