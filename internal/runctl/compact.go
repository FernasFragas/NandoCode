package runctl

import (
	"context"
	"strconv"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/hooks"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/state"
)

// HookDispatcher is the part of *hooks.Dispatcher the compact operation needs.
type HookDispatcher interface {
	Dispatch(ctx context.Context, env hooks.Envelope) hooks.Result
}

// CompactDeps carries what CompactConversation needs. Every field is optional.
type CompactDeps struct {
	// Client enables LLM summarization (layer 1). Without it the history is
	// emergency-truncated to the last MinTurns turns.
	Client llm.Client
	// Model is the session model, used for the summary side-query.
	Model string
	// Config overrides the compaction settings; the zero value means
	// agent.DefaultCompactionConfig.
	Config agent.CompactionConfig
	// Hooks receives PreCompact and PostCompact; nil disables both.
	Hooks      HookDispatcher
	SessionID  string
	WorkingDir string
	// Trigger is recorded in the hook metadata ("manual" by default).
	Trigger string
}

// CompactOutcome is the result of CompactConversation. Messages is always the
// history to keep: the original on Skipped or Denied.
type CompactOutcome struct {
	Messages   []llm.Message
	Before     int
	After      int
	Skipped    bool
	Denied     bool
	DenyReason string
	Result     agent.CompactionResult
	Err        error
}

// Changed reports whether the history was replaced.
func (o CompactOutcome) Changed() bool { return !o.Skipped && !o.Denied }

// ResetConversation is the shared /clear state operation: empty history and
// cleared run residue. Unrelated settings (model, input buffer) are kept.
func ResetConversation(app state.App) state.App {
	app.Messages = []llm.Message{}
	app.LastRetryNotice = ""
	app.TerminalReason = ""
	app.TerminalDetail = ""
	app.ActiveTools = map[string]state.ToolUse{}
	return app
}

// ApplyCompaction stores a compaction outcome in app. A skipped or denied
// outcome leaves the history untouched.
func ApplyCompaction(app state.App, o CompactOutcome) state.App {
	if !o.Changed() {
		return app
	}
	app.Messages = o.Messages
	return app
}

// CompactConversation is the shared /compact operation for an idle history. It
// skips histories with fewer than MinTurns turns, dispatches PreCompact (a deny
// skips compaction), compacts, then dispatches PostCompact. It does not modify
// msgs.
func CompactConversation(ctx context.Context, deps CompactDeps, msgs []llm.Message) CompactOutcome {
	cfg := deps.Config
	if cfg.MinTurns <= 0 {
		def := agent.DefaultCompactionConfig()
		if cfg.Threshold == 0 {
			cfg.Threshold = def.Threshold
		}
		if cfg.MaxSummaryLen == 0 {
			cfg.MaxSummaryLen = def.MaxSummaryLen
		}
		cfg.MinTurns = def.MinTurns
	}
	before := len(msgs)
	out := CompactOutcome{Messages: msgs, Before: before, After: before}
	if agent.CountTurnsExported(msgs) < cfg.MinTurns {
		out.Skipped = true
		return out
	}
	trigger := deps.Trigger
	if trigger == "" {
		trigger = "manual"
	}
	meta := map[string]string{"trigger": trigger, "messages_before": strconv.Itoa(before)}
	if deps.Hooks != nil {
		res := deps.Hooks.Dispatch(ctx, hooks.Envelope{
			Event:      hooks.EventPreCompact,
			Timestamp:  time.Now().UTC(),
			SessionID:  deps.SessionID,
			WorkingDir: deps.WorkingDir,
			Metadata:   copyMeta(meta),
		})
		if res.Decision == hooks.DecisionDeny {
			out.Denied = true
			out.DenyReason = res.Reason
			return out
		}
	}

	if deps.Client != nil {
		out.Result = agent.Compact(ctx, deps.Client, cfg, deps.Model, msgs)
	} else {
		kept := agent.EmergencyTruncateExported(msgs, cfg.MinTurns)
		out.Result = agent.CompactionResult{Before: before, After: len(kept), Layer: 4, Messages: kept}
	}
	if out.Result.Skipped {
		out.Skipped = true
		return out
	}
	out.Messages = out.Result.Messages
	out.After = len(out.Messages)

	if deps.Hooks != nil {
		meta["messages_after"] = strconv.Itoa(out.After)
		meta["layer"] = strconv.Itoa(out.Result.Layer)
		deps.Hooks.Dispatch(ctx, hooks.Envelope{
			Event:      hooks.EventPostCompact,
			Timestamp:  time.Now().UTC(),
			SessionID:  deps.SessionID,
			WorkingDir: deps.WorkingDir,
			Metadata:   meta,
		})
	}
	return out
}

func copyMeta(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
