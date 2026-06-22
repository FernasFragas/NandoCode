package tui

import (
	"context"

	"github.com/FernasFragas/nandocodego/internal/agent"
	tea "github.com/charmbracelet/bubbletea"
)

// AgentRunner interface represents an agent that can run with a given input.
type AgentRunner interface {
	Run(context.Context, agent.Input) <-chan agent.Event
}

// startAgentCmd returns a Bubble Tea command that runs the agent asynchronously.
func startAgentCmd(ctx context.Context, runner AgentRunner, input agent.Input, send func(tea.Msg)) tea.Cmd {
	return func() tea.Msg {
		events := runner.Run(ctx, input)
		drainAgentEvents(ctx, events, send)
		return agentDoneMsg{}
	}
}

// drainAgentEvents drains events from the agent channel and sends them to the program.
func drainAgentEvents(ctx context.Context, events <-chan agent.Event, send func(tea.Msg)) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-events:
			if !ok {
				return
			}
			send(agentEventMsg{Event: evt})
		}
	}
}
