package tui

import (
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/llm/modelruntime"
	"github.com/FernasFragas/Nandocode/internal/semantic"
	"github.com/charmbracelet/bubbletea"
)

// agentEventMsg wraps an agent.Event for Bubble Tea dispatch.
type agentEventMsg struct {
	Event agent.Event
}

// agentDoneMsg signals that the agent event channel has closed.
type agentDoneMsg struct{}

// agentStartFailedMsg signals an error starting the agent.
type agentStartFailedMsg struct {
	Err error
}

// permissionPromptMsg notifies the TUI of a pending permission prompt.
type permissionPromptMsg struct {
	Request permissionRequest
}

type permissionCancelledMsg struct {
	ID string
}

// permissionResolvedMsg returns the user's decision for a permission prompt.
type permissionResolvedMsg struct {
	ID       string
	Decision permissionDecision
}

// slashCommandMsg signals a parsed slash command.
type slashCommandMsg struct {
	Command string
	Args    []string
}

type skillChangedMsg struct {
	Name   string
	Source string
}

// tickMsg is sent periodically for UI updates.
type tickMsg time.Time

type fileIndexRefreshedMsg struct {
	Err       error
	Count     int
	Truncated bool
	Source    string
}

type memoryEditDoneMsg struct {
	Path string
	Err  error
}

type modelSwitchStartedMsg struct {
	Requested string
}

type modelSwitchNeedsCredentialMsg struct {
	Requested           string
	Resolved            llm.ResolvedModel
	ForPromptSubmission bool
	Input               string
	DisplayInput        string
	PreExpanded         bool
}

type modelSwitchCompletedMsg struct {
	Result              modelruntime.SwitchResult
	ForPromptSubmission bool
	Input               string
	DisplayInput        string
	PreExpanded         bool
}

type modelSwitchFailedMsg struct {
	Requested           string
	Err                 error
	ForPromptSubmission bool
	Input               string
	DisplayInput        string
	PreExpanded         bool
}

type cloudCredentialResolvedMsg struct {
	Key    string
	Save   bool
	Cancel bool
}

type indexOpDoneMsg struct {
	Content string
	Err     error
}

type indexProgressMsg struct {
	Event semantic.Event
}

// ProgramSender is an interface to send messages to the Bubble Tea program.
// This allows testing without a real tea.Program.
type ProgramSender interface {
	Send(msg tea.Msg)
}
