package server

import "time"

// acquireSessionSlot reserves a session slot. At the cap it evicts the oldest
// idle session (not running, no subscribers) and takes its slot. The evicted
// slot is released only if this call removed the session from the registry,
// so DELETE and sweepIdle racing with it cannot double-release.
func (s *Server) acquireSessionSlot() bool {
	if s.limiter.AcquireSession() {
		return true
	}
	for attempt := 0; attempt < 3; attempt++ {
		victim := s.oldestEvictable()
		if victim == nil {
			return false
		}
		victim.mu.Lock()
		if victim.running || victim.state != SessionStateReady || len(victim.subscribers) > 0 {
			victim.mu.Unlock()
			continue
		}
		victim.state = SessionStateClosing
		victim.mu.Unlock()
		victim.stop()
		if s.registry.remove(victim.id) {
			s.limiter.ReleaseSession()
		}
		if s.limiter.AcquireSession() {
			return true
		}
	}
	return false
}

func (s *Server) oldestEvictable() *Session {
	var oldest *Session
	var at time.Time
	for _, c := range s.registry.all() {
		c.mu.Lock()
		idle := !c.running && c.state == SessionStateReady && len(c.subscribers) == 0
		created := c.lastActive
		c.mu.Unlock()
		if idle && (oldest == nil || created.Before(at) || (created.Equal(at) && c.id < oldest.id)) {
			oldest, at = c, created
		}
	}
	return oldest
}
