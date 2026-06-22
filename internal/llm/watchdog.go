// Package llm provides the LLM client interface and types for interacting with Ollama.
package llm

import (
	"context"
	"time"
)

// WatchdogConfig configures the stream watchdog behavior.
type WatchdogConfig struct {
	// IdleTimeout is the maximum time to wait between chunks before considering the stream hung.
	IdleTimeout time.Duration

	// IdleWarningTimeout is when to emit an idle warning (e.g., "still waiting...").
	IdleWarningTimeout time.Duration

	// OnIdleWarning is called when the idle warning threshold is reached.
	OnIdleWarning func()
}

// DefaultWatchdogConfig returns sensible defaults for the watchdog.
func DefaultWatchdogConfig() WatchdogConfig {
	return WatchdogConfig{
		IdleTimeout:        90 * time.Second,
		IdleWarningTimeout: 45 * time.Second,
		OnIdleWarning:      nil,
	}
}

// DefaultCloudWatchdogConfig returns watchdog defaults suitable for cloud models.
func DefaultCloudWatchdogConfig() WatchdogConfig {
	return WatchdogConfig{
		IdleTimeout:        5 * time.Minute,
		IdleWarningTimeout: 60 * time.Second,
		OnIdleWarning:      nil,
	}
}

// WithIdleTimeout returns base with IdleTimeout set to idle and warning clamped to half idle when needed.
func WithIdleTimeout(base WatchdogConfig, idle time.Duration) WatchdogConfig {
	cfg := base
	if idle <= 0 {
		return cfg
	}
	cfg.IdleTimeout = idle
	if cfg.IdleWarningTimeout <= 0 || cfg.IdleWarningTimeout >= cfg.IdleTimeout {
		cfg.IdleWarningTimeout = cfg.IdleTimeout / 2
	}
	return cfg
}

// WithIdleWarning returns a new config with the idle warning callback set.
func (c WatchdogConfig) WithIdleWarning(timeout time.Duration, fn func()) WatchdogConfig {
	c.IdleWarningTimeout = timeout
	c.OnIdleWarning = fn
	return c
}

// WatchStream wraps a stream channel with watchdog timeout protection.
// If no event arrives within IdleTimeout, it cancels the context and emits a synthetic "done" event.
// The returned channel will be closed when the input channel closes or the watchdog fires.
func WatchStream(ctx context.Context, input <-chan StreamEvent, config WatchdogConfig) (<-chan StreamEvent, context.CancelFunc) {
	output := make(chan StreamEvent)
	watchdogCtx, cancel := context.WithCancel(ctx)

	go func() {
		defer close(output)
		defer cancel()

		idleTimer := time.NewTimer(config.IdleTimeout)
		defer idleTimer.Stop()

		var warningTimer *time.Timer
		if config.OnIdleWarning != nil && config.IdleWarningTimeout > 0 {
			warningTimer = time.NewTimer(config.IdleWarningTimeout)
			defer warningTimer.Stop()
		}

		warningFired := false

		for {
			select {
			case <-watchdogCtx.Done():
				// Context cancelled externally
				return

			case <-idleTimer.C:
				// Watchdog timeout - emit synthetic done event
				output <- StreamEvent{
					Done:       true,
					DoneReason: "watchdog_timeout",
				}
				return

			case <-func() <-chan time.Time {
				if warningTimer != nil {
					return warningTimer.C
				}
				// Never fires if no warning timer
				return make(chan time.Time)
			}():
				// Idle warning threshold reached
				if !warningFired && config.OnIdleWarning != nil {
					config.OnIdleWarning()
					warningFired = true
				}

			case event, ok := <-input:
				if !ok {
					// Input channel closed normally
					return
				}

				// Reset timers on every event
				if !idleTimer.Stop() {
					select {
					case <-idleTimer.C:
					default:
					}
				}
				idleTimer.Reset(config.IdleTimeout)

				if warningTimer != nil && !warningFired {
					if !warningTimer.Stop() {
						select {
						case <-warningTimer.C:
						default:
						}
					}
					warningTimer.Reset(config.IdleWarningTimeout)
				}

				// Forward the event
				select {
				case output <- event:
				case <-watchdogCtx.Done():
					return
				}

				// If this is a done event, we're finished
				if event.Done {
					return
				}
			}
		}
	}()

	return output, cancel
}
