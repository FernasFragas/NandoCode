package agent

import (
	"context"
	"errors"
)

// classifyError maps internal errors to terminal reasons and details.
func classifyError(err error) (TerminalReason, string) {
	if err == nil {
		return TerminalCompleted, ""
	}

	// Context cancellation
	if errors.Is(err, context.Canceled) {
		return TerminalAborted, "context canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return TerminalAborted, "context deadline exceeded"
	}

	// Default to unrecoverable
	return TerminalUnrecoverable, err.Error()
}
