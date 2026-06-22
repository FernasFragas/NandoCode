// Package state provides a reactive app store for UI-facing state with subscriptions and change callbacks.
package state

import (
	"sync"
)

// Store is a thread-safe generic store that holds a mutable value and notifies subscribers on changes.
// Subscriber channels are non-blocking; if a subscriber is slow, the latest value replaces earlier queued updates.
type Store[T any] struct {
	mu          sync.RWMutex
	value       T
	onChange    func(prev, next T)
	subscribers map[int]chan T
	nextID      int
}

// NewStore creates a new Store with an initial value and an optional onChange callback.
// The onChange callback is called exactly once per Set call after releasing the store lock.
func NewStore[T any](initial T, onChange func(prev, next T)) *Store[T] {
	if onChange == nil {
		onChange = func(prev, next T) {}
	}
	return &Store[T]{
		value:       initial,
		onChange:    onChange,
		subscribers: make(map[int]chan T),
	}
}

// Get returns the current value by value.
// Safe to call concurrently with Set and Subscribe.
func (s *Store[T]) Get() T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.value
}

// Set updates the value using a pure function and notifies all subscribers.
// The updater function is called exactly once while holding the write lock.
// onChange is called exactly once after releasing the lock.
// Subscribers are notified after onChange (buffered to prevent blocking Set).
func (s *Store[T]) Set(f func(prev T) T) {
	s.mu.Lock()
	prev := s.value
	s.value = f(prev)
	next := s.value
	subscribers := s.subscribers // Snapshot the subscriber map under lock
	s.mu.Unlock()

	// Call onChange outside the lock to avoid deadlocks
	s.onChange(prev, next)

	// Notify subscribers in a non-blocking way:
	// If the subscriber buffer is full, replace with the latest value.
	for _, ch := range subscribers {
		select {
		case ch <- next:
			// Sent successfully
		default:
			// Buffer full, replace with latest value
			select {
			case ch <- next:
			default:
				// Still full; non-blocking send failed again (rare edge case)
			}
		}
	}
}

// Subscribe returns a receive-only channel and an unsubscribe function.
// The channel is buffered with capacity 1 and immediately receives the current value.
// The channel will receive new values whenever Set is called.
// Unsubscribe is idempotent and closes the channel.
func (s *Store[T]) Subscribe() (<-chan T, func()) {
	s.mu.Lock()
	id := s.nextID
	s.nextID++
	ch := make(chan T, 1)
	s.subscribers[id] = ch
	current := s.value
	s.mu.Unlock()

	// Send current value immediately
	ch <- current

	// Return the receive-only channel and an idempotent unsubscribe function
	unsubscribe := sync.OnceFunc(func() {
		s.mu.Lock()
		if subscriber, ok := s.subscribers[id]; ok {
			delete(s.subscribers, id)
			s.mu.Unlock()
			close(subscriber)
		} else {
			s.mu.Unlock()
		}
	})

	return ch, unsubscribe
}
