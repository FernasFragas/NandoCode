package picker

// Trigger describes completion kind.
type Trigger int

const (
	TriggerFile Trigger = iota
	TriggerCommand
)

// Suggestion is one picker row.
type Suggestion struct {
	Display    string
	Insert     string
	Detail     string
	IsDir      bool
	MatchRunes []int
	Score      float64
}

// Provider returns ranked suggestions.
type Provider interface {
	Suggest(query string, limit int) []Suggestion
}

// State stores picker UI state inside TUI model.
type State struct {
	Visible bool
	Trigger Trigger
	Token   Context
	Items   []Suggestion
	Index   int
}
