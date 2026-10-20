package server

import "github.com/FernasFragas/Nandocode/internal/permissions"

// emitPermissionResolved tells the browser that a permission_request is no
// longer open (frozen contract K2). decision is one of "allow", "deny",
// "always_allow" or "timeout".
func emitPermissionResolved(s *Session, requestID, decision string) {
	s.Emit("permission_resolved", map[string]any{"request_id": requestID, "decision": decision})
}

// expire closes a prompt that ended without a user answer (timeout or run
// cancel) and emits permission_resolved. If a user decision already claimed
// the prompt, it returns that decision with won=true and emits nothing here,
// because Resolve emitted the event.
func (b *HTTPPermissionBroker) expire(id string, ch chan permissionResult, decision string) (permissions.Decision, bool) {
	b.mu.Lock()
	_, open := b.pending[id]
	delete(b.pending, id)
	b.mu.Unlock()
	if open {
		emitPermissionResolved(b.session, id, decision)
		return permissions.DecisionDeny, false
	}
	res := <-ch
	if res.decision == decisionAllow || res.decision == decisionAlwaysAllow {
		return permissions.DecisionAllow, true
	}
	return permissions.DecisionDeny, true
}
