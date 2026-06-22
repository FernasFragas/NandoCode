package server

import "sync"

type RecentIDs struct {
	mu    sync.Mutex
	cap   int
	order []string
	set   map[string]struct{}
}

func NewRecentIDs(capacity int) *RecentIDs {
	if capacity < 1 {
		capacity = 1
	}
	return &RecentIDs{cap: capacity, order: make([]string, 0, capacity), set: make(map[string]struct{}, capacity)}
}

func (r *RecentIDs) SeenOrAdd(id string) bool {
	if id == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.set[id]; ok {
		return true
	}
	if len(r.order) == r.cap {
		old := r.order[0]
		r.order = r.order[1:]
		delete(r.set, old)
	}
	r.order = append(r.order, id)
	r.set[id] = struct{}{}
	return false
}
