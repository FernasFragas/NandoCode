package llm

import (
	"context"
	"errors"
	"testing"
)

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorClass
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: ErrorClassUnknown,
		},
		{
			name:     "context canceled",
			err:      context.Canceled,
			expected: ErrorClassContextCanceled,
		},
		{
			name:     "context deadline exceeded",
			err:      context.DeadlineExceeded,
			expected: ErrorClassContextDeadlineExceeded,
		},
		{
			name:     "404 model not found",
			err:      errors.New("model not found: 404"),
			expected: ErrorClassHTTP404ModelMissing,
		},
		{
			name:     "500 internal server error",
			err:      errors.New("internal server error: 500"),
			expected: ErrorClassHTTP5xx,
		},
		{
			name:     "timeout error",
			err:      errors.New("request timeout"),
			expected: ErrorClassNetworkTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyError(tt.err)
			if got != tt.expected {
				t.Errorf("ClassifyError() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetRetryPolicy(t *testing.T) {
	tests := []struct {
		name          string
		class         ErrorClass
		expectRetries bool
	}{
		{
			name:          "context canceled - no retries",
			class:         ErrorClassContextCanceled,
			expectRetries: false,
		},
		{
			name:          "deadline exceeded - should retry",
			class:         ErrorClassContextDeadlineExceeded,
			expectRetries: true,
		},
		{
			name:          "5xx error - should retry",
			class:         ErrorClassHTTP5xx,
			expectRetries: true,
		},
		{
			name:          "4xx error - no retries",
			class:         ErrorClassHTTP4xx,
			expectRetries: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := GetRetryPolicy(tt.class)
			hasRetries := policy.MaxRetries > 0
			if hasRetries != tt.expectRetries {
				t.Errorf("GetRetryPolicy() retries = %v, want %v", hasRetries, tt.expectRetries)
			}
		})
	}
}

func TestRetryWithPolicy(t *testing.T) {
	t.Run("succeeds on first attempt", func(t *testing.T) {
		attempts := 0
		err := RetryWithPolicy(context.Background(), func() error {
			attempts++
			return nil
		})

		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if attempts != 1 {
			t.Errorf("expected 1 attempt, got %d", attempts)
		}
	})

	t.Run("no retry on context canceled", func(t *testing.T) {
		attempts := 0
		err := RetryWithPolicy(context.Background(), func() error {
			attempts++
			return context.Canceled
		})

		if err == nil {
			t.Error("expected error, got nil")
		}
		if attempts != 1 {
			t.Errorf("expected 1 attempt, got %d", attempts)
		}
	})
}
