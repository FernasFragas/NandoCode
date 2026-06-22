package server

import "sync"

type RingBuffer[T any] struct {
	mu   sync.RWMutex
	cap  int
	data []T
}

func NewRingBuffer[T any](capacity int) *RingBuffer[T] {
	if capacity < 1 {
		capacity = 1
	}
	return &RingBuffer[T]{cap: capacity, data: make([]T, 0, capacity)}
}

func (r *RingBuffer[T]) Append(v T) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.data) == r.cap {
		copy(r.data, r.data[1:])
		r.data[len(r.data)-1] = v
		return
	}
	r.data = append(r.data, v)
}

func (r *RingBuffer[T]) Snapshot() []T {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]T, len(r.data))
	copy(out, r.data)
	return out
}
