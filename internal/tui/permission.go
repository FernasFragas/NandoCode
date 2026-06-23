package tui

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/FernasFragas/Nandocode/internal/permissions"
	tea "github.com/charmbracelet/bubbletea"
)

// permissionDecision represents a user choice on a permission prompt.
type permissionDecision string

const (
	decisionAllow       permissionDecision = "allow"
	decisionDeny        permissionDecision = "deny"
	decisionAlwaysAllow permissionDecision = "always_allow"
)

// permissionRequest captures a permission prompt request.
type permissionRequest struct {
	ID       string
	ToolName string
	Target   string
	Reason   string
}

var promptIDCounter atomic.Uint64

// PermissionBroker bridges agent goroutine permission prompts to the TUI.
type PermissionBroker struct {
	send    func(tea.Msg)
	mu      sync.Mutex
	pending map[string]chan permissionDecision
}

// NewPermissionBroker creates a new permission broker.
func NewPermissionBroker(send func(tea.Msg)) *PermissionBroker {
	return &PermissionBroker{
		send:    send,
		pending: make(map[string]chan permissionDecision),
	}
}

// PromptFunc returns a permissions.PromptFunc for use with the agent.
func (b *PermissionBroker) PromptFunc() permissions.PromptFunc {
	return func(ctx context.Context, prompt permissions.Prompt) (permissions.Decision, string, error) {
		id := generatePromptID()
		ch := make(chan permissionDecision, 1)

		b.mu.Lock()
		b.pending[id] = ch
		b.mu.Unlock()

		// Send the prompt message to the TUI
		if b.send != nil {
			b.send(permissionPromptMsg{
				Request: permissionRequest{
					ID:       id,
					ToolName: prompt.ToolName,
					Target:   prompt.Target,
					Reason:   prompt.Reason,
				},
			})
		}

		// Wait for decision or context cancellation
		select {
		case <-ctx.Done():
			b.mu.Lock()
			delete(b.pending, id)
			b.mu.Unlock()
			if b.send != nil {
				b.send(permissionCancelledMsg{ID: id})
			}
			return permissions.DecisionDeny, "cancelled", nil

		case decision := <-ch:
			switch decision {
			case decisionAllow, decisionAlwaysAllow:
				return permissions.DecisionAllow, "approved", nil
			default:
				return permissions.DecisionDeny, "denied", nil
			}
		}
	}
}

// generatePromptID creates a unique prompt ID.
func generatePromptID() string {
	return fmt.Sprintf("perm_%d", promptIDCounter.Add(1))
}

// Resolve records a user decision for a pending prompt.
func (b *PermissionBroker) Resolve(id string, decision permissionDecision) {
	b.mu.Lock()
	ch, exists := b.pending[id]
	if exists {
		delete(b.pending, id)
	}
	b.mu.Unlock()

	if exists && ch != nil {
		select {
		case ch <- decision:
		default:
			// Channel already has a value or is closed; ignore
		}
	}
}

// CancelAll denies all outstanding prompts (used on REPL exit).
func (b *PermissionBroker) CancelAll() {
	b.mu.Lock()
	pending := b.pending
	b.pending = make(map[string]chan permissionDecision)
	b.mu.Unlock()

	for _, ch := range pending {
		if ch != nil {
			select {
			case ch <- decisionDeny:
			default:
			}
		}
	}
}
