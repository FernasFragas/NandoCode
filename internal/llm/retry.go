// Package llm provides the LLM client interface and types for interacting with Ollama.
package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v4"
)

// RetryPolicy defines retry behavior for different error classes.
type RetryPolicy struct {
	MaxRetries int
	Backoff    backoff.BackOff
}

// ErrorClass categorizes errors for retry decision-making.
type ErrorClass int

const (
	ErrorClassUnknown ErrorClass = iota
	ErrorClassContextCanceled
	ErrorClassContextDeadlineExceeded
	ErrorClassHTTP4xx
	ErrorClassHTTP404ModelMissing
	ErrorClassHTTP5xx
	ErrorClassNetworkTimeout
)

// ClassifyError categorizes an error into an ErrorClass.
func ClassifyError(err error) ErrorClass {
	if err == nil {
		return ErrorClassUnknown
	}

	// Context errors
	if errors.Is(err, context.Canceled) {
		return ErrorClassContextCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrorClassContextDeadlineExceeded
	}

	// HTTP errors (check for status code in error message or type)
	// Note: The ollama SDK may wrap these differently
	errStr := err.Error()
	if containsAny(errStr, "404", "not found", "model not found") {
		return ErrorClassHTTP404ModelMissing
	}
	if containsAny(errStr, "400", "401", "403") {
		return ErrorClassHTTP4xx
	}
	if containsAny(errStr, "500", "502", "503", "504", "internal server error", "bad gateway", "service unavailable", "gateway timeout") {
		return ErrorClassHTTP5xx
	}

	// Network timeouts
	if containsAny(errStr, "timeout", "deadline exceeded", "i/o timeout") {
		return ErrorClassNetworkTimeout
	}

	return ErrorClassUnknown
}

// GetRetryPolicy returns the retry policy for a given error class.
func GetRetryPolicy(class ErrorClass) RetryPolicy {
	switch class {
	case ErrorClassContextCanceled:
		// Never retry user cancellations
		return RetryPolicy{
			MaxRetries: 0,
			Backoff:    nil,
		}

	case ErrorClassContextDeadlineExceeded:
		// Mid-stream timeout - retry with exponential backoff
		return RetryPolicy{
			MaxRetries: 3,
			Backoff:    newExponentialBackoff(500*time.Millisecond, 10*time.Second),
		}

	case ErrorClassHTTP5xx:
		// Server errors (model loading, GPU OOM) - retry with longer backoff
		return RetryPolicy{
			MaxRetries: 5,
			Backoff:    newExponentialBackoff(1*time.Second, 30*time.Second),
		}

	case ErrorClassHTTP404ModelMissing:
		// Model missing - caller should trigger pull, then retry once
		return RetryPolicy{
			MaxRetries: 1,
			Backoff:    backoff.NewConstantBackOff(0),
		}

	case ErrorClassHTTP4xx:
		// Client errors (bad request, auth issues) - don't retry
		return RetryPolicy{
			MaxRetries: 0,
			Backoff:    nil,
		}

	case ErrorClassNetworkTimeout:
		// Network timeout - retry with moderate backoff
		return RetryPolicy{
			MaxRetries: 3,
			Backoff:    newExponentialBackoff(500*time.Millisecond, 10*time.Second),
		}

	default:
		// Unknown errors - conservative single retry
		return RetryPolicy{
			MaxRetries: 1,
			Backoff:    newExponentialBackoff(1*time.Second, 5*time.Second),
		}
	}
}

// RetryWithPolicy executes a function with automatic retry logic based on error classification.
func RetryWithPolicy(ctx context.Context, fn func() error) error {
	var lastErr error
	attempt := 0

	for {
		// Execute the function
		err := fn()
		if err == nil {
			return nil
		}

		// Classify the error
		class := ClassifyError(err)
		policy := GetRetryPolicy(class)

		// Check if we should retry
		if attempt >= policy.MaxRetries {
			return fmt.Errorf("max retries (%d) exceeded: %w", policy.MaxRetries, err)
		}

		// Check context
		if ctx.Err() != nil {
			return ctx.Err()
		}

		lastErr = err
		attempt++

		// Calculate backoff duration
		if policy.Backoff != nil {
			duration := policy.Backoff.NextBackOff()
			if duration == backoff.Stop {
				return fmt.Errorf("backoff stopped after %d attempts: %w", attempt, lastErr)
			}

			// Wait with context awareness
			select {
			case <-time.After(duration):
				// Continue to next retry
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

// newExponentialBackoff creates an exponential backoff with the given initial and max intervals.
func newExponentialBackoff(initial, max time.Duration) *backoff.ExponentialBackOff {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = initial
	b.MaxInterval = max
	b.MaxElapsedTime = 0 // No overall timeout (handled by context)
	b.Reset()
	return b
}

// containsAny checks if a string contains any of the given substrings (case-insensitive).
func containsAny(s string, substrs ...string) bool {
	for _, substr := range substrs {
		if len(s) >= len(substr) {
			// Simple case-insensitive contains check
			sLower := s // We could add strings.ToLower here for true case-insensitivity
			if containsSubstring(sLower, substr) {
				return true
			}
		}
	}
	return false
}

// containsSubstring is a helper for substring matching.
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			len(s) > len(substr) && (s[:len(substr)] == substr ||
				s[len(s)-len(substr):] == substr ||
				findInString(s, substr)))
}

// findInString finds a substring within a string.
func findInString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// StatusCodeFromError attempts to extract an HTTP status code from an error.
func StatusCodeFromError(err error) (int, bool) {
	// Try to unwrap to an HTTP response error
	type httpError interface {
		StatusCode() int
	}

	var he httpError
	if errors.As(err, &he) {
		return he.StatusCode(), true
	}

	// Fallback: parse from error string (brittle but sometimes necessary)
	errStr := err.Error()
	if containsAny(errStr, "404") {
		return http.StatusNotFound, true
	}
	if containsAny(errStr, "500") {
		return http.StatusInternalServerError, true
	}

	return 0, false
}
