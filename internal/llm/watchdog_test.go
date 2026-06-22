package llm

import (
	"context"
	"testing"
	"time"
)

func TestWatchStream(t *testing.T) {
	t.Run("forwards events normally", func(t *testing.T) {
		input := make(chan StreamEvent, 3)
		input <- StreamEvent{Message: Message{Content: "hello"}}
		input <- StreamEvent{Message: Message{Content: " world"}}
		input <- StreamEvent{Done: true}
		close(input)

		config := DefaultWatchdogConfig()
		config.IdleTimeout = 1 * time.Second

		ctx := context.Background()
		output, cancel := WatchStream(ctx, input, config)
		defer cancel()

		events := collectEvents(output, 3)

		if len(events) != 3 {
			t.Fatalf("expected 3 events, got %d", len(events))
		}
		if events[0].Message.Content != "hello" {
			t.Errorf("first event content = %q, want %q", events[0].Message.Content, "hello")
		}
		if !events[2].Done {
			t.Error("last event should be done")
		}
	})

	t.Run("fires watchdog on timeout", func(t *testing.T) {
		input := make(chan StreamEvent)

		config := DefaultWatchdogConfig()
		config.IdleTimeout = 100 * time.Millisecond

		ctx := context.Background()
		output, cancel := WatchStream(ctx, input, config)
		defer cancel()

		var event StreamEvent
		select {
		case event = <-output:
		case <-time.After(500 * time.Millisecond):
			t.Fatal("timeout waiting for watchdog event")
		}

		if !event.Done {
			t.Error("watchdog event should have Done=true")
		}
		if event.DoneReason != "watchdog_timeout" {
			t.Errorf("DoneReason = %q, want %q", event.DoneReason, "watchdog_timeout")
		}
	})

	t.Run("calls idle warning", func(t *testing.T) {
		input := make(chan StreamEvent)
		defer close(input)

		warningCalled := false
		config := DefaultWatchdogConfig()
		config.IdleWarningTimeout = 50 * time.Millisecond
		config.IdleTimeout = 200 * time.Millisecond
		config.OnIdleWarning = func() {
			warningCalled = true
		}

		ctx := context.Background()
		output, cancel := WatchStream(ctx, input, config)
		defer cancel()

		// Wait for warning to fire
		time.Sleep(100 * time.Millisecond)

		// Drain one event (watchdog timeout)
		<-output

		if !warningCalled {
			t.Error("idle warning should have been called")
		}
	})
}

func TestDefaultCloudWatchdogConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultCloudWatchdogConfig()
	if cfg.IdleTimeout != 5*time.Minute {
		t.Fatalf("IdleTimeout=%s, want 5m", cfg.IdleTimeout)
	}
	if cfg.IdleWarningTimeout != 60*time.Second {
		t.Fatalf("IdleWarningTimeout=%s, want 60s", cfg.IdleWarningTimeout)
	}
}

func TestWithIdleTimeout(t *testing.T) {
	t.Parallel()
	base := DefaultWatchdogConfig()
	got := WithIdleTimeout(base, 30*time.Second)
	if got.IdleTimeout != 30*time.Second {
		t.Fatalf("IdleTimeout=%s, want 30s", got.IdleTimeout)
	}
	if got.IdleWarningTimeout != 15*time.Second {
		t.Fatalf("IdleWarningTimeout=%s, want 15s", got.IdleWarningTimeout)
	}
}

func collectEvents(ch <-chan StreamEvent, max int) []StreamEvent {
	events := make([]StreamEvent, 0, max)
	for event := range ch {
		events = append(events, event)
		if len(events) >= max {
			break
		}
	}
	return events
}
