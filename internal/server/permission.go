package server

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/FernasFragas/Nandocode/internal/permissions"
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

// pendingPermission is one open prompt. Tool and target are kept so that
// always_allow can build a rule for exactly this call.
type pendingPermission struct {
	ch       chan permissionResult
	toolName string
	target   string
}

type HTTPPermissionBroker struct {
	session *Session
	mu      sync.Mutex
	pending map[string]pendingPermission
	// timeout bounds one prompt; zero means defaultPermissionTimeout.
	timeout time.Duration
}

const defaultPermissionTimeout = 30 * time.Second

var permissionCounter atomic.Uint64

func NewHTTPPermissionBroker(session *Session) *HTTPPermissionBroker {
	return &HTTPPermissionBroker{session: session, pending: map[string]pendingPermission{}}
}

func (b *HTTPPermissionBroker) PromptFunc() permissions.PromptFunc {
	return func(ctx context.Context, prompt permissions.Prompt) (permissions.Decision, string, error) {
		id := fmt.Sprintf("perm_%d", permissionCounter.Add(1))
		ch := make(chan permissionResult, 1)
		b.mu.Lock()
		b.pending[id] = pendingPermission{ch: ch, toolName: prompt.ToolName, target: prompt.Target}
		b.mu.Unlock()
		b.session.Emit("permission_request", map[string]any{"request_id": id, "tool_name": prompt.ToolName, "target": prompt.Target, "reason": prompt.Reason})
		defer func() {
			b.mu.Lock()
			delete(b.pending, id)
			b.mu.Unlock()
		}()
		select {
		case <-ctx.Done():
			if res, won := b.expire(id, ch, "deny"); won {
				return res, resolvedReason(res), nil
			}
			return permissions.DecisionDeny, "cancelled", nil
		case <-time.After(b.promptTimeout()):
			if res, won := b.expire(id, ch, "timeout"); won {
				return res, resolvedReason(res), nil
			}
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
	p, ok := b.pending[id]
	if ok {
		delete(b.pending, id)
	}
	b.mu.Unlock()
	if !ok {
		return false
	}
	// Record the rule before releasing the agent so the next run sees it.
	if d == decisionAlwaysAllow {
		b.session.AllowSession(p.toolName, p.target)
	}
	emitPermissionResolved(b.session, id, string(d))
	select {
	case p.ch <- permissionResult{decision: d}:
	default:
	}
	return true
}

func (b *HTTPPermissionBroker) promptTimeout() time.Duration {
	if b.timeout > 0 {
		return b.timeout
	}
	return defaultPermissionTimeout
}

func resolvedReason(d permissions.Decision) string {
	if d == permissions.DecisionAllow {
		return "approved"
	}
	return "denied"
}
