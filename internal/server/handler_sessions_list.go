package server

import (
	"encoding/json"
	"net/http"
	"sort"
	"sync/atomic"
	"time"
)

// sessionListItem is one row of GET /v1/sessions (frozen contract K3).
type sessionListItem struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	CreatedAt  time.Time `json:"created_at"`
	LastActive time.Time `json:"last_active"`
	Running    bool      `json:"running"`
	Model      string    `json:"model"`
}

type sessionListResponse struct {
	Sessions []sessionListItem `json:"sessions"`
}

// listItem snapshots the session under its lock. Title is empty until V5a
// adds session titles; Running is the same flag StartRun and view() use.
func (s *Session) listItem() sessionListItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	return sessionListItem{
		ID:         s.id,
		Title:      s.title,
		CreatedAt:  s.createdAt,
		LastActive: s.lastActive,
		Running:    s.running,
		Model:      s.appState.ActiveModel,
	}
}

// isBlankOrphan reports a session nobody is attached to that never took a
// turn: no subscribers, no run, no conversation, and no event beyond
// session_ready. Such a session can be handed to the next New Session.
func (s *Session) isBlankOrphan() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.blankOrphanLocked()
}

// blankOrphanLocked is isBlankOrphan with s.mu already held. eventCounter is
// incremented atomically by Emit without s.mu, so it is read atomically.
func (s *Session) blankOrphanLocked() bool {
	return s.state == SessionStateReady && !s.running &&
		len(s.subscribers) == 0 && len(s.conversation) == 0 &&
		s.runPrompt == "" && atomic.LoadUint64(&s.eventCounter) <= 1
}

// blankOrphan returns the newest blank orphan, if any.
func (r *sessionRegistry) blankOrphan() (*Session, bool) {
	var best *Session
	var bestAt time.Time
	for _, sess := range r.all() {
		if !sess.isBlankOrphan() {
			continue
		}
		if at := sess.listItem().CreatedAt; best == nil || at.After(bestAt) {
			best, bestAt = sess, at
		}
	}
	return best, best != nil
}

// remove deletes id and reports whether this call removed it, so only one
// of two racing deleters (DELETE and the idle sweep) releases the slot.
func (r *sessionRegistry) remove(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sessions[id]; !ok {
		return false
	}
	delete(r.sessions, id)
	return true
}

// reuseBlankSession answers POST /v1/sessions?reuse=blank with an existing
// blank orphan (200) instead of creating another one. It reports whether
// it answered.
func (s *Server) reuseBlankSession(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Query().Get("reuse") != "blank" {
		return false
	}
	sess, ok := s.registry.blankOrphan()
	if !ok {
		return false
	}
	// Re-check and touch under one lock so a session that gained a
	// subscriber or run since the scan is not handed out.
	sess.mu.Lock()
	if !sess.blankOrphanLocked() {
		sess.mu.Unlock()
		return false
	}
	sess.lastActive = time.Now().UTC()
	sess.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(sess.view())
	return true
}

func (s *Server) handleListSessions(w http.ResponseWriter, _ *http.Request) {
	all := s.registry.all()
	items := make([]sessionListItem, 0, len(all))
	for _, sess := range all {
		items = append(items, sess.listItem())
	}
	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return items[i].ID < items[j].ID
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sessionListResponse{Sessions: items})
}
