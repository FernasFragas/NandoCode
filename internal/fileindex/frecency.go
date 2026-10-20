package fileindex

import "sync"

// Frecency tracks in-session recent picks.
type Frecency struct {
	scores map[string]float64
	mu     sync.Mutex
}

// NewFrecency creates an empty frecency tracker.
func NewFrecency() *Frecency {
	return &Frecency{
		scores: make(map[string]float64, 128),
	}
}

// Touch bumps recency score for a relative path.
func (f *Frecency) Touch(rel string) {
	if rel == "" {
		return
	}
	f.mu.Lock()
	f.scores[rel] = f.scores[rel] + 1.0
	f.mu.Unlock()
}

// Score returns the tracked score for rel.
func (f *Frecency) Score(rel string) float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.scores[rel]
}

// Decay lowers all scores to avoid permanent lock-in.
func (f *Frecency) Decay() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for k, v := range f.scores {
		v = v * 0.5
		if v < 0.01 {
			delete(f.scores, k)
			continue
		}
		f.scores[k] = v
	}
}
