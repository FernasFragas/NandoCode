package tasks

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrMailboxFull        = errors.New("mailbox is full")
	ErrMessageTooLarge    = errors.New("message exceeds byte limit")
	ErrDuplicateMessageID = errors.New("duplicate message id")
)

const (
	DefaultMailboxCapacity     = 128
	DefaultMailboxMessageBytes = 64 * 1024
)

type PendingMessage struct {
	ID        string    `json:"id"`
	FromAgent string    `json:"from_agent"`
	ToAgent   string    `json:"to_agent"`
	Summary   string    `json:"summary,omitempty"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

type Mailbox struct {
	mu           sync.Mutex
	queue        []PendingMessage
	seenIDs      map[string]struct{}
	capacity     int
	maxMsgBytes  int
	notify       chan struct{}
	notifyClosed bool
}

func NewMailbox() *Mailbox {
	return NewMailboxWithLimits(DefaultMailboxCapacity, DefaultMailboxMessageBytes)
}

func NewMailboxWithLimits(capacity, maxMsgBytes int) *Mailbox {
	if capacity <= 0 {
		capacity = DefaultMailboxCapacity
	}
	if maxMsgBytes <= 0 {
		maxMsgBytes = DefaultMailboxMessageBytes
	}
	return &Mailbox{
		queue:       make([]PendingMessage, 0, capacity),
		seenIDs:     make(map[string]struct{}),
		capacity:    capacity,
		maxMsgBytes: maxMsgBytes,
		notify:      make(chan struct{}),
	}
}

func (m *Mailbox) Enqueue(msg PendingMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.maxMsgBytes > 0 && len(msg.Content) > m.maxMsgBytes {
		return ErrMessageTooLarge
	}
	if msg.ID != "" {
		if _, exists := m.seenIDs[msg.ID]; exists {
			return ErrDuplicateMessageID
		}
	}
	if len(m.queue) >= m.capacity {
		return ErrMailboxFull
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now().UTC()
	}
	m.queue = append(m.queue, msg)
	if msg.ID != "" {
		m.seenIDs[msg.ID] = struct{}{}
	}
	if !m.notifyClosed {
		close(m.notify)
		m.notifyClosed = true
	}
	return nil
}

func (m *Mailbox) Drain() []PendingMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.queue) == 0 {
		return nil
	}
	out := make([]PendingMessage, len(m.queue))
	copy(out, m.queue)
	m.queue = m.queue[:0]
	m.notify = make(chan struct{})
	m.notifyClosed = false
	return out
}

func (m *Mailbox) Notify() <-chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.notify
}

func (m *Mailbox) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.queue)
}
