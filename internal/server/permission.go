package server

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/FernasFragas/nandocodego/internal/permissions"
)

type permissionDecision string

const (
	decisionAllow       permissionDecision = "allow"
	decisionDeny        permissionDecision = "deny"
	decisionAlwaysAllow permissionDecision = "always_allow"
)

type permissionResult struct {
	decision permissionDecision
}

type HTTPPermissionBroker struct {
	session *Session
	mu      sync.Mutex
	pending map[string]chan permissionResult
}

var permissionCounter atomic.Uint64

func NewHTTPPermissionBroker(session *Session) *HTTPPermissionBroker {
	return &HTTPPermissionBroker{session: session, pending: map[string]chan permissionResult{}}
}

func (b *HTTPPermissionBroker) PromptFunc() permissions.PromptFunc {
	return func(ctx context.Context, prompt permissions.Prompt) (permissions.Decision, string, error) {
		id := fmt.Sprintf("perm_%d", permissionCounter.Add(1))
		ch := make(chan permissionResult, 1)
		b.mu.Lock()
		b.pending[id] = ch
		b.mu.Unlock()
		b.session.Emit("permission_request", map[string]any{"request_id": id, "tool_name": prompt.ToolName, "target": prompt.Target, "reason": prompt.Reason})
		defer func() {
			b.mu.Lock()
			delete(b.pending, id)
			b.mu.Unlock()
		}()
		select {
		case <-ctx.Done():
			return permissions.DecisionDeny, "cancelled", nil
		case <-time.After(30 * time.Second):
			return permissions.DecisionDeny, "timeout", nil
		case res := <-ch:
			switch res.decision {
			case decisionAllow, decisionAlwaysAllow:
				return permissions.DecisionAllow, "approved", nil
			default:
				return permissions.DecisionDeny, "denied", nil
			}
		}
	}
}

func (b *HTTPPermissionBroker) Resolve(id string, d permissionDecision) bool {
	b.mu.Lock()
	ch, ok := b.pending[id]
	if ok {
		delete(b.pending, id)
	}
	b.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case ch <- permissionResult{decision: d}:
	default:
	}
	if d == decisionAlwaysAllow {
		b.session.AllowAllRules()
	}
	return true
}
