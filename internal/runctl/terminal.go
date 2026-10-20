package runctl

import (
	"strings"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/analysis"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/state"
)

// TerminalOptions tunes ApplyTerminal for the calling surface.
type TerminalOptions struct {
	// Side marks an isolated side question (/btw): the outcome is recorded but
	// the conversation history is left alone.
	Side bool
	// UserPrompt is the user message of this turn when the caller has not
	// already appended it to app.Messages (the browser server). The TUI appends
	// the prompt when the run starts and leaves this empty.
	UserPrompt string
}

// ApplyTerminal returns app updated with the outcome of a finished run: the run
// is no longer active, the terminal reason, detail and usage are recorded, and
// the messages the run added are appended to the history (after UserPrompt, if
// set). It never modifies app's backing arrays.
func ApplyTerminal(app state.App, t agent.Terminal, opts TerminalOptions) state.App {
	app.ActiveRun = false
	app.TerminalReason = t.Reason
	app.TerminalDetail = t.Detail
	app.Usage = t.Usage
	if opts.Side {
		return app
	}
	add := len(t.Conversation)
	if opts.UserPrompt != "" {
		add++
	}
	if add == 0 {
		return app
	}
	msgs := make([]llm.Message, 0, len(app.Messages)+add)
	msgs = append(msgs, app.Messages...)
	if opts.UserPrompt != "" {
		msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: opts.UserPrompt})
	}
	msgs = append(msgs, t.Conversation...)
	app.Messages = msgs
	return app
}

// BuildCheckpoint builds the durable analysis checkpoint for a finished run
// from the (already updated) app state. fallbackAssistant is used when no
// assistant message is in the history, for example the last transcript text.
func BuildCheckpoint(app state.App, t agent.Terminal, fallbackAssistant string) analysis.Checkpoint {
	var lastUser, lastAssistant string
	for i := len(app.Messages) - 1; i >= 0; i-- {
		m := app.Messages[i]
		if lastUser == "" && m.Role == llm.RoleUser && strings.TrimSpace(m.Content) != "" {
			lastUser = m.Content
		}
		if lastAssistant == "" && m.Role == llm.RoleAssistant && strings.TrimSpace(m.Content) != "" {
			lastAssistant = m.Content
		}
		if lastUser != "" && lastAssistant != "" {
			break
		}
	}
	if lastAssistant == "" && strings.TrimSpace(fallbackAssistant) != "" {
		lastAssistant = fallbackAssistant
	}
	pending := t.Reason != agent.TerminalCompleted || analysis.LooksLikeIncompleteFinalAnswer(lastAssistant)
	return analysis.Checkpoint{
		Model:               app.ActiveModel,
		WorkingDir:          app.ToolSettings.WorkingDir,
		LastUserPrompt:      lastUser,
		LastAssistantOutput: lastAssistant,
		TerminalReason:      string(t.Reason),
		TerminalDetail:      t.Detail,
		PendingFinalAnswer:  pending,
		InspectedFiles:      analysis.ExtractMentionedPaths(lastUser),
		SynthesisStage:      synthesisStage(t, pending),
	}
}

// SaveRunCheckpoint builds and persists the run checkpoint.
func SaveRunCheckpoint(app state.App, t agent.Terminal, fallbackAssistant string) error {
	return analysis.SaveCheckpoint(BuildCheckpoint(app, t, fallbackAssistant))
}

func synthesisStage(t agent.Terminal, pending bool) string {
	if pending {
		return "synthesis_pending"
	}
	if t.Reason == agent.TerminalCompleted {
		return "completed"
	}
	return "interrupted"
}
