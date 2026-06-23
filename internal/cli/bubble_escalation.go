package cli

import (
	"context"
	"time"

	"github.com/FernasFragas/Nandocode/internal/permissions"
)

// TUIBubbleEscalation forwards sub-agent permission prompts to the same prompt
// callback used by the top-level run.
type TUIBubbleEscalation struct {
	PromptFunc permissions.PromptFunc
	Timeout    time.Duration
}

// Ask implements the bubble escalation contract with a bounded timeout.
func (b TUIBubbleEscalation) Ask(ctx context.Context, prompt permissions.Prompt) (permissions.Decision, string, error) {
	if b.PromptFunc == nil {
		return permissions.DecisionDeny, "escalation unavailable", nil
	}
	timeout := b.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	childCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	done := make(chan struct {
		decision permissions.Decision
		reason   string
		err      error
	}, 1)
	go func() {
		d, r, err := b.PromptFunc(childCtx, prompt)
		done <- struct {
			decision permissions.Decision
			reason   string
			err      error
		}{decision: d, reason: r, err: err}
	}()
	select {
	case <-childCtx.Done():
		if childCtx.Err() == context.DeadlineExceeded {
			return permissions.DecisionDeny, "escalation timeout", nil
		}
		return permissions.DecisionDeny, "escalation cancelled", nil
	case out := <-done:
		return out.decision, out.reason, out.err
	}
}
