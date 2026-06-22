package tasks

import (
	"errors"
	"sync"
	"testing"
)

func TestMailboxRoundTrip(t *testing.T) {
	t.Parallel()
	m := NewMailboxWithLimits(4, 128)
	if err := m.Enqueue(PendingMessage{ID: "m1", Content: "one"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Enqueue(PendingMessage{ID: "m2", Content: "two"}); err != nil {
		t.Fatal(err)
	}
	if got := m.Len(); got != 2 {
		t.Fatalf("len=%d want=2", got)
	}
	drained := m.Drain()
	if len(drained) != 2 {
		t.Fatalf("drain len=%d want=2", len(drained))
	}
	if got := m.Len(); got != 0 {
		t.Fatalf("len after drain=%d want=0", got)
	}
}

func TestMailboxDuplicateAndCapacity(t *testing.T) {
	t.Parallel()
	m := NewMailboxWithLimits(1, 1024)
	if err := m.Enqueue(PendingMessage{ID: "m1", Content: "one"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Enqueue(PendingMessage{ID: "m1", Content: "one"}); !errors.Is(err, ErrDuplicateMessageID) {
		t.Fatalf("expected duplicate error, got %v", err)
	}
	if err := m.Enqueue(PendingMessage{ID: "m2", Content: "two"}); !errors.Is(err, ErrMailboxFull) {
		t.Fatalf("expected full error, got %v", err)
	}
}

func TestMailboxMessageSize(t *testing.T) {
	t.Parallel()
	m := NewMailboxWithLimits(4, 3)
	if err := m.Enqueue(PendingMessage{ID: "m1", Content: "abcd"}); !errors.Is(err, ErrMessageTooLarge) {
		t.Fatalf("expected message too large error, got %v", err)
	}
}

func TestMailboxConcurrent(t *testing.T) {
	t.Parallel()
	m := NewMailboxWithLimits(200, 1024)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = m.Enqueue(PendingMessage{ID: string(rune('a' + i)), Content: "x"})
		}(i)
	}
	wg.Wait()
	if got := m.Len(); got == 0 {
		t.Fatal("expected queued messages")
	}
	_ = m.Drain()
}
