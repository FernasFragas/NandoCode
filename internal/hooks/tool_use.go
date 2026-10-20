package hooks

import (
	"context"
	"strings"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/permissions"
)

// CheckToolUse runs the same PreToolUse dispatch used by Runner.Run.
func (r *Runner) CheckToolUse(ctx context.Context, req permissions.Request, in agent.Input) (permissions.Result, bool, []agent.Event) {
	out := make(chan agent.Event, 16)
	result, decided := r.preToolDecision(ctx, req, in, out)
	close(out)
	var notices []agent.Event
	for evt := range out {
		notices = append(notices, evt)
	}
	return result, decided, notices
}

// PostToolUse dispatches the same tool result envelope as Runner.Run. Unlike
// Dispatch, it also returns the hooks' additional_context (joined), which the
// browser save shows as a notice. Other events are unaffected.
func (r *Runner) PostToolUse(ctx context.Context, in agent.Input, evt agent.ToolHookEvent) Result {
	results := r.dispatcher.dispatchAll(ctx, r.envelope(EventPostToolUse, &evt, in, ""))
	if len(results) == 0 {
		return Result{}
	}
	out := aggregate(results)
	var contexts []string
	for _, res := range results {
		if c := strings.TrimSpace(res.AdditionalContext); c != "" {
			contexts = append(contexts, c)
		}
	}
	out.AdditionalContext = strings.Join(contexts, "; ")
	return out
}
