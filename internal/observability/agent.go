package observability

import (
	"context"
	"time"

	"github.com/FernasFragas/nandocodego/internal/agent"
)

// Runner matches the agent runner interface used by hooks and TUI.
type Runner interface {
	Run(context.Context, agent.Input) <-chan agent.Event
}

type observedRunner struct {
	next   Runner
	meter  *Meter
	bridge Bridge
}

// WrapRunner decorates an agent runner and records run-level usage on terminal events.
func WrapRunner(next Runner, meter *Meter, bridge Bridge) Runner {
	if next == nil {
		return nil
	}
	if bridge == nil {
		bridge = noopBridge{}
	}
	return &observedRunner{next: next, meter: meter, bridge: bridge}
}

func (o *observedRunner) Run(ctx context.Context, in agent.Input) <-chan agent.Event {
	out := make(chan agent.Event, 16)
	start := time.Now()
	startTokens := int64(0)
	trace := RunTrace{
		RunStartedAt:   start.UTC(),
		RetryKinds:     map[string]int64{},
		StageLatencies: map[string]time.Duration{},
		ContextMode:    in.ContextMode,
		ToolMode:       in.ToolMode,
		RouteAction:    in.RouteAction,
		RouteReason:    in.RouteReason,
		RouteProfile:   in.RouteProfile,
	}
	firstEventRecorded := false
	firstAssistantRecorded := false
	firstThinkingRecorded := false
	firstToolStartRecorded := false
	firstRetryRecorded := false
	compactionStartRecorded := false
	if o.meter != nil {
		startTokens = o.meter.Snapshot().TotalTokens
		for stage, dur := range o.meter.ConsumePendingRunStages() {
			trace.StageLatencies[stage] = dur
		}
		if mode, dirs, discovered, bodies, listingIntent, ok := o.meter.ConsumePendingRunExpansion(); ok {
			trace.MentionMode = mode
			trace.MentionDirs = dirs
			trace.MentionFilesDiscovered = discovered
			trace.MentionFileBodies = bodies
			trace.MentionListingIntent = listingIntent
		}
		o.meter.RecordCurrentRunTrace(trace)
	}
	go func() {
		defer close(out)
		for evt := range o.next.Run(ctx, in) {
			terminalEvent := false
			switch e := evt.(type) {
			case agent.AssistantTextDelta:
				if !firstEventRecorded {
					trace.FirstEventLatency = time.Since(start)
					firstEventRecorded = true
				}
				if !firstAssistantRecorded {
					trace.FirstAssistantLatency = time.Since(start)
					firstAssistantRecorded = true
				}
			case agent.AssistantThinkingDelta:
				if !firstEventRecorded {
					trace.FirstEventLatency = time.Since(start)
					firstEventRecorded = true
				}
				if !firstThinkingRecorded {
					trace.FirstThinkingLatency = time.Since(start)
					firstThinkingRecorded = true
				}
				if !firstAssistantRecorded {
					trace.FirstAssistantLatency = time.Since(start)
					firstAssistantRecorded = true
				}
			case agent.ToolUseStart:
				if !firstEventRecorded {
					trace.FirstEventLatency = time.Since(start)
					firstEventRecorded = true
				}
				if !firstToolStartRecorded {
					trace.FirstToolStartLatency = time.Since(start)
					firstToolStartRecorded = true
				}
			case agent.RetryNotice:
				if !firstRetryRecorded {
					trace.FirstRetryLatency = time.Since(start)
					firstRetryRecorded = true
				}
				trace.RetryCount++
				kind := e.Kind
				if kind == "" {
					kind = "unknown"
				}
				trace.RetryKinds[kind]++
				o.meter.RecordAgentRetry(e)
			case agent.CompactionStarted:
				if !compactionStartRecorded {
					trace.CompactionStartLatency = time.Since(start)
					compactionStartRecorded = true
				}
				trace.StageLatencies["compaction_started"] = trace.CompactionStartLatency
			case agent.CompactionCompleted:
				trace.CompactionEndLatency = time.Since(start)
				trace.StageLatencies["compaction_completed"] = trace.CompactionEndLatency
			case agent.StageTiming:
				if e.Stage != "" && e.Duration > 0 {
					trace.StageLatencies[e.Stage] = e.Duration
				}
			case agent.Terminal:
				terminalEvent = true
				dur := time.Since(start)
				if o.meter != nil {
					for stage, stageDur := range o.meter.ConsumePendingRunStages() {
						trace.StageLatencies[stage] = stageDur
					}
				}
				usage := e.Usage
				if o.meter != nil && o.meter.Snapshot().TotalTokens > startTokens {
					usage.PromptEvalCount = 0
					usage.EvalCount = 0
				}
				o.meter.RecordAgentRun(usage, dur, e.Reason)
				o.bridge.RecordAgentRun(usage, dur, e.Reason)
				trace.TerminalLatency = dur
				trace.TerminalReason = string(e.Reason)
				trace.DoneReason = usage.DoneReason
				o.meter.RecordRunTrace(trace)
			}
			if o.meter != nil && !terminalEvent {
				o.meter.RecordCurrentRunTrace(trace)
			}
			select {
			case out <- evt:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}
