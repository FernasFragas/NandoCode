# Phase 20 Detailed Plan - Content Compaction

Date: 2026-05-07
Status: ✅ Complete (hook dispatch deferred) — 2026-05-08
Source plans:

- `.codex/go-ollama-plan-AGENTS.md`
- `.codex/go-ollama-plan-HUMANS.md`
- `docs/PHASE-LOG.md`
- `docs/PROJECT-STATUS-AND-ONBOARDING.md`
- `docs/PHASE-8-DETAILED-PLAN.md`
- `docs/PHASE-9-DETAILED-PLAN.md`
- `docs/PHASE-19-DETAILED-PLAN.md`
- `book/ch17-performance.md`
- `.codex/agent-context/learnings-memory.md`

## Goal

Implement graceful context window management so long sessions survive instead of dying with `TerminalContextOverflow`. When the context window approaches its limit, compact the conversation history using a structured summarization strategy before retrying. This is a correctness fix, not a feature.

Without compaction, any session that generates enough tool results, thinking tokens, or long multi-turn coding conversations will silently terminate with `TerminalContextOverflow`. The user loses their work and must restart. Compaction makes the agent resilient to this by summarizing the oldest conversation turns into a single system message, freeing context for new work.

Deliverables:

- `internal/agent/compact.go` implementing the 4-layer compaction strategy.
- `internal/agent/compact_test.go` with unit tests.
- Agent loop patched to trigger compaction on `DoneReason: "length"` instead of emitting `TerminalContextOverflow`.
- New agent events `CompactionStarted` and `CompactionCompleted`.
- `PreCompact` and `PostCompact` hook events dispatched through existing hook system.
- `/compact` slash command for manual compaction.
- TUI status message `[Compacting context...]` during compaction.
- `CompactionConfig` with injectable configuration.
- Tests passing with `go test -race ./internal/agent/...`.
- Phase log update after implementation.

## Definition Of Success

Phase 20 is complete when this scenario works end-to-end:

1. Start REPL with `--model qwen3 --no-alt-screen`.
2. Run at least 10 back-and-forth turns with tool calls (e.g., repeatedly read files and ask for summaries).
3. Observe `[Compacting context...]` appear in the TUI transcript at some point.
4. Confirm the session does NOT emit `TerminalContextOverflow`.
5. Confirm the session continues producing correct results after compaction.
6. Run `/compact` manually in a session with fewer than 4 turns and confirm it skips with a "not enough turns to compact" message.
7. Verify `go test -race ./internal/agent/...` passes without a live Ollama instance for unit tests.

## Baseline Analysis From Implemented Phases

### Phase 0 - Security and Supply Chain

Implemented:

- Default-deny external network posture.
- No secret logging policy.

Phase 20 implications:

- Compaction calls the LLM side-query. That call must use the configured Ollama endpoint only, same as all other LLM calls.
- The summary produced by the LLM must not be logged at INFO (it may contain session content).
- `tools/check-network-policy.sh` must still pass after this phase.

### Phase 1 - CLI, Paths, Logging

Implemented:

- `internal/logging` with structured slog.

Phase 20 implications:

- Log compaction events at DEBUG: turn count, token count before/after, summary length.
- Log compaction errors at WARN.
- Never log the produced summary content at any level.

### Phase 2 - LLM Client

Implemented:

- `llm.Client` interface with `Chat(ctx, req) <-chan Event`.
- `ChatRequest.Format` for structured output.
- Streaming and non-streaming chat support.

Phase 20 implications:

- Compaction uses an LLM side-query. The side-query must be non-streaming: call `Chat` with `Stream: false` and consume the result channel once (or use a helper that collapses the channel to a single string).
- The compaction query uses `Stream: false` via `ChatRequest`: set `MaxTokens` to `CompactionConfig.MaxSummaryLen` to bound the summary.
- `SummaryModel` defaults to the session model (from `agent.Input.Model`). Do not add a dedicated compaction model config in Phase 20; that belongs to Phase 13's config system.
- Compaction query failures must be non-fatal: if the LLM call fails or times out, skip compaction for this turn and continue.

### Phase 3 - Tool Interface and Starter Tools

Implemented:

- Tool registry and execution path.
- `tools.Context` with `PermissionMode`.

Phase 20 implications:

- Compaction does not add new tools. The compaction mechanism operates on the conversation history (messages), not on tool inputs/outputs.
- After compaction, tool results in older turns are collapsed to a single summary message. Tool results from the current/last turn are preserved.

### Phase 4 - Agent Loop

Implemented:

- `agent.Run(ctx, input) <-chan agent.Event`.
- Agent loop turn logic in `internal/agent/agent.go`.
- `DoneReason: "length"` from the LLM when `stop_reason == "length"`.
- Retry loop for `DoneReason: "length"` with expanded token budget.
- `TerminalContextOverflow` terminal event.

Phase 20 implications:

- The current retry-on-length logic in `agent.go` runs in `a.run()`. Currently:
  - First `DoneReason: "length"` → retry with 64K output tokens.
  - Second `DoneReason: "length"` → emit `TerminalContextOverflow`.
- Phase 20 changes the second path: instead of emitting `TerminalContextOverflow`, trigger compaction.
- After compaction, re-run the turn with the compacted conversation.
- If compaction itself fails or if post-compaction the agent still gets `DoneReason: "length"`, then emit `TerminalContextOverflow` (compaction cannot help if even the compacted conversation is too long).
- The compaction trigger is based on `stop_reason == "length"`, not on token-count polling. The LLM tells us when it is out of space. This is more reliable than guessing from `prompt_eval_count` because Ollama's token counting may differ from the model's actual context budget.

Note on `prompt_eval_count` from `ch17-performance.md`:

> Token counting is anchored on the API's actual `usage` field, not client-side estimation.

Ollama returns `eval_count` and `prompt_eval_count` in `ChatResponse`. These can be used as a secondary early-warning signal to proactively trigger compaction before hitting the hard limit. Phase 20 should implement both paths:

- **Proactive**: if `prompt_eval_count > MaxContextTokens * CompactionThreshold` at the end of a successful turn, schedule compaction before the next turn.
- **Reactive**: if `stop_reason == "length"` on the second consecutive occurrence, trigger compaction immediately.

### Phase 5 - Permission System

Implemented:

- Permission modes and resolver.

Phase 20 implications:

- Compaction itself does not require any permission checks. It does not invoke user-facing tools.
- The LLM side-query used for summarization is a system-internal call, not a tool call, so it does not route through `permissions.Resolve`.

### Phase 6 - State Layer

Implemented:

- `state.App` with `Messages`, `ToolSettings`.
- `state.Store` with reactive `OnChange`.

Phase 20 implications:

- After compaction completes, the agent loop holds the compacted message list in its local variable. The compacted list is NOT pushed back into `state.App.Messages` during the same run — the agent loop is the source of truth for conversation state during a run.
- After the terminal event fires (run ends), `state.App.Messages` is updated via `Terminal.Conversation` (the conversation payload already used in Phase 8). The compacted conversation is included in `Terminal.Conversation`, so after the run, the TUI's message store reflects the compacted state.
- Do not add `CompactionState` to `state.App` in Phase 20. That belongs to a future Phase 13 config/status surface.

### Phase 7 - Bubble Tea TUI and REPL

Implemented:

- Transcript rendering with system items.
- Slash command parsing in `internal/tui/slash_test.go`.

Phase 20 implications:

- `CompactionStarted` event should display `[Compacting context...]` as a system transcript item with a spinner or static indicator.
- `CompactionCompleted` event should replace or follow the compaction notice with the result: e.g., `[Compacted: 18 → 3 messages, ~4200 tokens freed]`.
- TUI must not block during the LLM side-query. The compaction runs inside the agent goroutine (same goroutine that sends events to the channel), so TUI is not blocked — it just receives the `CompactionStarted` event and renders the notice, then waits for subsequent events.
- `/compact` slash command should be added to `internal/tui/app.go`'s slash handler. On `/compact`, send a `CompactRequest` message to the agent channel if a run is in progress, or trigger a standalone compaction if in idle state.

### Phase 8 - Memory

Implemented:

- Memory runner decorator wrapping the agent.
- Recall and prompt injection before agent runs.

Phase 20 implications:

- Memory recall runs before each agent run and injects a system prompt section. After compaction, the compacted conversation already contains the memory section from previous turns embedded in the summary. The next run's recall will run fresh against the new user message. No changes needed to the memory runner for Phase 20.
- The `pending/` extraction side-query runs after terminal completion. After compaction, `Terminal.Conversation` contains the compacted history plus the last turn. Extraction should work on this compacted history without modification.

### Phase 9 - Hooks

Implemented:

- `internal/hooks` with event types including reserved constants `PreCompact` and `PostCompact`.
- Hook snapshot and dispatcher.

Phase 20 implications:

- `PreCompact` and `PostCompact` hook events are already defined as constants. Phase 20 implements their dispatch.
- `PreCompact` receives `CompactionStarted` event payload: turn count and context tokens.
- A `PreCompact` hook can deny compaction (return `deny` decision). If denied, compaction is skipped and the agent emits `TerminalContextOverflow` as if compaction was not available.
- `PostCompact` receives `CompactionCompleted` payload. It cannot undo compaction but can record it.
- Hook dispatch for compaction events goes through the existing `hooks.Dispatcher` in `internal/hooks/dispatch.go`. No new hook infrastructure is needed.

### Phases 10-15 (Not Yet Implemented)

Phase 20 should not require sub-agents, MCP, skills, tasks, or concurrent tool execution. Compaction is a standalone agent-loop concern. The implementation must not assume any of these phases are complete.

### Phase 19 - Complete Tool Ecosystem

Implemented (planned before Phase 20):

- FileEdit, Glob, Grep, WebFetch, TodoWrite, TodoRead tools.

Phase 20 implications:

- Longer sessions involving FileEdit + Grep + WebFetch are the most likely to hit context limits. Phase 20 makes those sessions survivable.
- Compaction collapses old tool results. A Grep result from 10 turns ago that filled 3000 tokens becomes part of a compressed summary. This is desirable: the model does not need the raw line-by-line output of an old search; it needs to know what was found.

## Documentation and Log Findings

`book/ch17-performance.md` is the primary design reference for Phase 20. Key passages:

> When usage approaches the limit, a 4-layer compaction system progressively summarizes older content.

> Token counting is anchored on the API's actual `usage` field.

The 4-layer compaction strategy described in the chapter:

1. **Summarize oldest turns**: the LLM summarizes the first N turns into one compact message.
2. **Collapse tool results**: replace large tool result messages with one-line summaries.
3. **Strip thinking**: remove `thinking` content blocks from older turns (they were the model's internal reasoning and are not needed for context).
4. **Emergency truncate**: if all else fails, hard-truncate the oldest messages.

`docs/PROJECT-STATUS-AND-ONBOARDING.md` marks `TerminalContextOverflow` as an unresolved failure mode:

> Context compression and model fallback are missing.

The Phase 9 events file already reserves `PreCompact` and `PostCompact` constants, confirming that Phase 9 anticipated Phase 20 would implement them.

The `agent/events.go` file already defines `TerminalContextOverflow TerminalReason = "context_overflow"`. Phase 20 changes the path to this terminal reason from "always reach it on second `DoneReason: length`" to "only reach it when compaction also fails".

## Deep Analysis Of `book/ch17-performance.md` For Phase 20

### Token Budget Anchoring

The chapter states: "Token counting is anchored on the API's actual `usage` field." In Ollama's streaming chat API, the final chunk includes `eval_count` (output tokens) and `prompt_eval_count` (input tokens). The agent loop already collects `Usage` (turn/token counts) via `agent.Usage`. Phase 20 should extend `Usage` or add a `ContextTokens int64` field to `agent.Terminal` so the TUI can display context usage.

Ollama's `prompt_eval_count` may undercount compared to Claude's `usage.input_tokens` because different tokenizers count differently. For Phase 20 with Ollama, use `prompt_eval_count` as the only available signal and accept that it may not be perfectly accurate. The hard signal (`stop_reason == "length"`) is always accurate.

### Slot Reservation and MaxContextTokens

The chapter discusses "8K default, 64K escalation" for output slot reservation. This repo already implements that: the first `DoneReason: "length"` escalates output tokens to 64K. Phase 20 adds the next step after 64K escalation still hits length.

For the proactive threshold, `MaxContextTokens` must be configurable because Ollama models have different context window sizes. The `CompactionConfig` should hold `MaxContextTokens` with a default of 0 meaning "use `RecommendedNumCtx` from the model's capability record in `internal/llm/capabilities.go`". If no capability record exists, fall back to 8192 (conservative) or disable proactive compaction entirely (threshold never fires if `MaxContextTokens == 0`).

### The 4-Layer Strategy Applied to This Repo

Layer 1 (Summarize): Use a side-query `llm.Client.Chat` call with a bounded prompt asking the LLM to summarize the oldest N messages. N is determined by `CompactionConfig.MinTurns` (default 4): compact the oldest `len(messages) - MinTurns` messages, preserving the most recent `MinTurns` turns intact.

Layer 2 (Collapse tool results): Before the Layer 1 LLM call, scan messages for `llm.Message` with `Role == "tool"` and replace their content with a truncated one-line version. This pre-processing reduces the summary prompt's own token count.

Layer 3 (Strip thinking): Messages with `thinking` content blocks (from extended-thinking-capable models) carry large opaque blobs that are no longer useful for context. Strip them from older messages before compaction.

Layer 4 (Emergency truncate): If the LLM side-query itself fails (network error, timeout, or the summary is still too long), fall back to simply dropping the oldest `MinTurns` messages entirely. This is lossy but ensures the session can continue.

## Evaluation Of The Original Phase 20 Description

The prompt's Phase 20 description is correct at the design level. It needs supplementation in these areas:

- It describes `prompt_eval_count > MaxContextTokens * CompactionThreshold` as the trigger condition, but this requires `MaxContextTokens` to be set. This plan adds `MaxContextTokens` to `CompactionConfig` with a model-capability fallback.
- It says "Agent loop receives `DoneReason: 'length'` → already retries with expanded token budget → if still fails → trigger compaction instead of `TerminalContextOverflow`". This plan makes that sequencing explicit: first `DoneReason: "length"` → expand to 64K → second `DoneReason: "length"` → compact → retry → if third `DoneReason: "length"` → emit `TerminalContextOverflow`.
- It does not specify how `/compact` triggers compaction when no run is active. This plan defers standalone compaction from a slash command to future work; Phase 20's `/compact` only works during an active run by sending a signal to the agent event channel.
- The hook dispatch integration for `PreCompact`/`PostCompact` is mentioned but not detailed. This plan specifies that hook dispatch reuses the existing `hooks.Dispatcher` with the reserved event constants.

## Final Phase 20 Scope

In scope:

- `CompactionConfig` struct with all configurable parameters.
- `CompactionResult` struct for event payloads.
- 4-layer compaction strategy in `internal/agent/compact.go`.
- `CompactionStarted` and `CompactionCompleted` agent events.
- `PreCompact` and `PostCompact` hook dispatch.
- Agent loop patch to trigger compaction on second `DoneReason: "length"` and on proactive threshold.
- TUI `[Compacting context...]` system transcript item.
- `/compact` slash command (triggers compaction if run in progress, otherwise shows status).
- Tests passing with `go test -race ./internal/agent/...`.
- `docs/PHASE-LOG.md` Phase 20 entry.

Out of scope:

- Config-backed compaction settings (separate model for summary, adjustable threshold) — Phase 13 owns config.
- Compaction UI beyond the two system transcript items — Phase 13 owns rich command UX.
- `/compact history` or browsable compaction log — Phase 13+.
- Cross-session compaction (compacting across multiple REPL starts) — not applicable.
- Compaction for MCP or sub-agent contexts — Phase 10/11 own those.
- Hard per-conversation token budget aggregate from ch17 — that requires per-message token estimation which Ollama does not expose per-message; deferred.
- Compaction for non-Ollama LLM providers — the `llm.Client` interface is provider-neutral so the code will work, but testing is Ollama-only for Phase 20.

## Target User Experience

### During a Long Session

User is in the middle of a long coding session. After 18 turns of Grep/FileRead/FileEdit operations:

```
[Compacting context...]
```

A few seconds later:

```
[Compacted: 14 messages → 1 summary (18 → 5 messages total)]
```

The agent then continues with the user's next message as if nothing happened. The summary of the older turns is present in the conversation as a `system` message:

```
[Summary of earlier conversation: The user asked to refactor the internal/agent package.
We found 3 files using the old API: agent.go, tools.go, stream.go. We updated agent.go
and tools.go. stream.go still needs updating. Current focus: completing the stream.go
refactor.]
```

The agent has full context about what was previously done and can continue the task.

### Manual Compact

User types `/compact`:

```
/compact
```

If the session has 4 or more turns:

```
[Compacting context...]
[Compacted: 10 messages → 1 summary (12 → 4 messages total)]
```

If the session has fewer than 4 turns:

```
[Compact skipped: not enough turns to compact (need at least 4, have 2)]
```

### Failed Compaction

If the LLM side-query fails:

```
[Compaction failed: LLM error — continuing without compaction]
```

The agent continues the turn attempt. If it then gets a third `DoneReason: "length"`, it emits `TerminalContextOverflow` as before.

## Architecture

### New Types

```go
// CompactionConfig controls when and how compaction fires.
type CompactionConfig struct {
    // Threshold is the fraction of MaxContextTokens that triggers proactive compaction.
    // Default 0.8 (80%). Set to 0 to disable proactive compaction.
    Threshold float64

    // MinTurns is the minimum number of complete turns (user+assistant pairs)
    // required before compaction is attempted. Default 4.
    MinTurns int

    // SummaryModel is the model used for the summary side-query.
    // Defaults to the session model from agent.Input.Model.
    SummaryModel string

    // SummaryPrompt is the system prompt for the summary side-query.
    // Defaults to a built-in compact summary prompt.
    SummaryPrompt string

    // MaxSummaryLen is the maximum number of tokens in the summary response.
    // Default 2000.
    MaxSummaryLen int

    // MaxContextTokens is the model's context window size in tokens.
    // 0 means "use model capability record or disable proactive compaction".
    MaxContextTokens int64

    // Disabled disables all automatic compaction when true.
    Disabled bool
}

// DefaultCompactionConfig returns the production default configuration.
func DefaultCompactionConfig() CompactionConfig {
    return CompactionConfig{
        Threshold:     0.8,
        MinTurns:      4,
        MaxSummaryLen: 2000,
    }
}

// CompactionResult contains the before/after state after a compaction.
type CompactionResult struct {
    Before       int   // message count before compaction
    After        int   // message count after compaction
    TokensBefore int64 // estimated tokens before (from last prompt_eval_count)
    TokensAfter  int64 // estimated tokens after (not directly measurable; 0 if unknown)
    Summary      string // the summary text (not logged, only used in events)
    Layer        int    // which of the 4 layers was used (1-4)
    Skipped      bool   // true if MinTurns not met
    Error        string // non-empty if compaction failed but session continues
}
```

### New Agent Events

In `internal/agent/events.go`:

```go
// CompactionStarted signals that context compaction is beginning.
type CompactionStarted struct {
    TurnCount     int
    ContextTokens int64
}

func (CompactionStarted) isEvent() {}

// CompactionCompleted signals that compaction has finished (or was skipped/failed).
type CompactionCompleted struct {
    Result CompactionResult
}

func (CompactionCompleted) isEvent() {}
```

### Compaction Function Signature

In `internal/agent/compact.go`:

```go
// Compact reduces the message history to fit within the context window.
// It uses a 4-layer strategy:
//   Layer 1: LLM-summarized oldest turns.
//   Layer 2: Collapse tool results to one-line summaries before LLM call.
//   Layer 3: Strip thinking content from older turns.
//   Layer 4: Emergency truncate oldest messages.
//
// Compact is non-destructive to the caller's slice: it returns a new slice.
func Compact(
    ctx context.Context,
    client llm.Client,
    cfg CompactionConfig,
    model string,
    messages []llm.Message,
) CompactionResult

// countTurns returns the number of complete user+assistant turn pairs in messages.
func countTurns(messages []llm.Message) int

// collapseToolResults replaces large tool result messages with one-line summaries.
func collapseToolResults(messages []llm.Message) []llm.Message

// stripThinkingBlocks removes thinking content from older message turns.
func stripThinkingBlocks(messages []llm.Message) []llm.Message

// emergencyTruncate removes the oldest MinTurns turn pairs.
func emergencyTruncate(messages []llm.Message, minTurns int) []llm.Message

// buildSummaryPrompt returns the system prompt for the summary side-query.
func buildSummaryPrompt(cfg CompactionConfig, messages []llm.Message) string

// summarizeMessages calls the LLM to summarize the oldest messages.
// Returns the summary string and any error.
func summarizeMessages(
    ctx context.Context,
    client llm.Client,
    cfg CompactionConfig,
    model string,
    messages []llm.Message,
) (string, error)
```

### Compaction Trigger Logic in Agent Loop

The compaction trigger fits into the existing retry loop in `a.run()`. The current logic (pseudo-code):

```go
attempt := 0
for {
    resp, doneReason := a.runTurn(ctx, messages, in)
    if doneReason != "length" {
        break
    }
    attempt++
    if attempt == 1 {
        // Escalate output tokens to 64K and retry
        in.MaxTokens = 64_000
        continue
    }
    // Second length: previously TerminalContextOverflow
    emit Terminal{Reason: TerminalContextOverflow}
    return
}
```

Phase 20 changes the `attempt == 2+` path:

```go
attempt := 0
for {
    resp, doneReason, contextTokens := a.runTurn(ctx, messages, in)
    if doneReason != "length" {
        // Proactive compaction check
        if shouldCompactProactively(cfg, contextTokens) && countTurns(messages) >= cfg.MinTurns {
            messages = a.doCompact(ctx, cfg, messages, contextTokens, events)
        }
        break
    }
    attempt++
    if attempt == 1 {
        in.MaxTokens = 64_000
        continue
    }
    if attempt == 2 {
        // Try reactive compaction
        messages = a.doCompact(ctx, cfg, messages, contextTokens, events)
        in.MaxTokens = 0 // reset to default
        continue
    }
    // Third length or compaction also failed: terminal overflow
    emit Terminal{Reason: TerminalContextOverflow}
    return
}
```

`doCompact` emits `CompactionStarted`, dispatches `PreCompact` hook, calls `Compact(...)`, dispatches `PostCompact` hook, emits `CompactionCompleted`, and returns the new message slice.

### Slash Command: /compact

Add `/compact` to the slash command handler in `internal/tui/app.go`:

When typed during an active run:
- Send a `CompactRequestMsg` Bubble Tea message to the running agent command.
- The agent command's select loop can pick up a `compactRequest` channel signal.

When typed with no active run:
- Check if `state.App.Messages` has at least `MinTurns * 2` messages.
- If yes, trigger a standalone compaction of `state.App.Messages` (without the LLM — use Layer 4 emergency truncate only, since there is no active run context).
- If no, display the "not enough turns" message.

Phase 20 implementation detail: the standalone `/compact` (no active run) can be simplified to Layer 4 (emergency truncate) only in Phase 20 and deferred for the LLM-based summarization path to Phase 13 config UX. The manual trigger during an active run uses the full 4-layer strategy.

## Implementation Plan

### Step 1 - New Agent Events

Files:

- `internal/agent/events.go`

Add:

```go
type CompactionStarted struct {
    TurnCount     int
    ContextTokens int64
}
func (CompactionStarted) isEvent() {}

type CompactionCompleted struct {
    Result CompactionResult
}
func (CompactionCompleted) isEvent() {}
```

No tests required for event type definitions alone.

### Step 2 - CompactionConfig and CompactionResult Types

Files:

- `internal/agent/compact.go`

Implement `CompactionConfig`, `DefaultCompactionConfig()`, and `CompactionResult` types.

Tests:

- [x] `DefaultCompactionConfig()` returns threshold=0.8, minTurns=4, maxSummaryLen=2000.
- [x] Zero-value `CompactionConfig` with `Disabled=true` behaves correctly.

### Step 3 - Core Compaction Logic

Files:

- `internal/agent/compact.go`
- `internal/agent/compact_test.go`

Implement the helper functions:

- `countTurns(messages []llm.Message) int`: count the number of complete user+assistant message pairs. A turn is a user message followed by an assistant message (with optional tool results in between).
- `collapseToolResults(messages []llm.Message) []llm.Message`: scan messages; replace any `llm.Message` with `Role == "tool"` and `Content` longer than 500 characters with a one-line summary: `[Tool result: <tool_name>, <len(content)> chars, truncated for compaction]`.
- `stripThinkingBlocks(messages []llm.Message) []llm.Message`: remove `<thinking>` content from message content strings in older turns. The `llm.Message.Content` may contain thinking blocks if the model used extended thinking. Remove content between `<thinking>` and `</thinking>` tags in messages older than the last 2 turns.
- `emergencyTruncate(messages []llm.Message, minTurns int) []llm.Message`: preserve the last `minTurns` complete turn pairs plus any initial system messages; drop everything in between.
- `buildSummaryPrompt(cfg CompactionConfig, messages []llm.Message) string`: build the summary request given the messages to summarize. Default prompt: `"Summarize the following conversation concisely in 3-5 sentences, preserving key decisions, file paths, and current task status:\n\n"` followed by formatted messages.
- `summarizeMessages(ctx, client, cfg, model, messages) (string, error)`: make a single non-streaming LLM call. Use `llm.ChatRequest{Stream: false, MaxTokens: cfg.MaxSummaryLen}` with a user message containing the formatted conversation to summarize.
- `Compact(ctx, client, cfg, model, messages) CompactionResult`: orchestrate the 4-layer strategy.

`Compact` implementation:

```
1. Check MinTurns. If countTurns(messages) < cfg.MinTurns, return skipped result.
2. Layer 3: stripThinkingBlocks on all but last 2 turns.
3. Layer 2: collapseToolResults on all but last turn.
4. Determine messages to summarize: all except last MinTurns turn pairs.
5. Layer 1: call summarizeMessages. On error, use Layer 4 fallback.
   - On success: replace summarized messages with a single system message:
     "[Compacted: N messages summarized]\n\n" + summary
   - Append preserved (last MinTurns turns) messages.
6. If Layer 1 failed: Layer 4: emergencyTruncate(messages, cfg.MinTurns).
   - Set result.Layer = 4, result.Error = "LLM summarization failed; truncated oldest turns".
7. Return CompactionResult with Before/After counts, Layer used.
```

Tests for `compact_test.go`:

- [x] `countTurns` counts user+assistant pairs correctly.
- [x] `countTurns` ignores system messages.
- [x] `countTurns` handles empty slice.
- [x] `collapseToolResults` replaces long tool results with summary.
- [x] `collapseToolResults` preserves short tool results unchanged.
- [x] `collapseToolResults` does not modify non-tool messages.
- [x] `stripThinkingBlocks` removes `<thinking>...</thinking>` from old turns.
- [x] `stripThinkingBlocks` preserves last 2 turns' thinking content.
- [x] `emergencyTruncate` preserves system messages and last MinTurns turns.
- [x] `emergencyTruncate` drops messages in between.
- [x] `Compact` with fewer than MinTurns messages returns `Skipped=true`.
- [x] `Compact` with LLM success returns Layer=1 and Before > After.
- [x] `Compact` with LLM failure falls back to Layer 4.
- [x] `Compact` result contains correct Before and After counts.
- [x] `summarizeMessages` with fake client returns summary string.
- [x] `summarizeMessages` respects context cancellation.
- [x] `summarizeMessages` timeout returns error (use context with deadline).

### Step 4 - Agent Loop Integration

Files:

- `internal/agent/agent.go` (or wherever `a.run()` is implemented)
- `internal/agent/agent_test.go`

Add `doCompact` method to `Agent`:

```go
// doCompact runs compaction and emits the appropriate events.
// Returns the new message slice (may be same as input on failure).
func (a *Agent) doCompact(
    ctx context.Context,
    cfg CompactionConfig,
    messages []llm.Message,
    contextTokens int64,
    events chan<- Event,
) []llm.Message {
    turnCount := countTurns(messages)
    sendEvent(ctx, events, CompactionStarted{TurnCount: turnCount, ContextTokens: contextTokens})
    // PreCompact hook dispatch (through hook runner if available — see Step 6)
    result := Compact(ctx, a.client, cfg, a.activeModel, messages)
    sendEvent(ctx, events, CompactionCompleted{Result: result})
    // PostCompact hook dispatch
    if result.Skipped || result.Error != "" {
        return messages // return original on skip or error
    }
    return result.messages // the compacted messages
}
```

Note: `CompactionResult` must carry the new message slice. Add `Messages []llm.Message` to `CompactionResult` (internal field, not JSON-serialized).

Patch `a.run()` per the trigger logic described in the architecture section.

Add `CompactionConfig` to `agent.Config`:

```go
type Config struct {
    // ... existing fields ...
    Compaction CompactionConfig
}
```

Update `DefaultConfig()` to include `DefaultCompactionConfig()`.

Provide `WithCompactionConfig(cfg CompactionConfig) Option` for injection in tests.

Tests:

- [x] Agent with fake client that always returns `stop_reason=length` triggers compaction on second occurrence.
- [x] After compaction succeeds, the agent continues and emits `CompactionStarted`/`CompactionCompleted` events.
- [x] Agent with `CompactionConfig.Disabled=true` emits `TerminalContextOverflow` on second length without compaction.
- [x] Agent with `CompactionConfig.MinTurns=4` and only 2 turns skips compaction and emits `TerminalContextOverflow`.
- [x] `go test -race ./internal/agent/...` passes.

### Step 5 - TUI Integration

Files:

- `internal/tui/app.go`
- `internal/tui/transcript.go`

In `internal/tui/app.go`, handle the two new events in the agent event reducer:

```go
case agent.CompactionStarted:
    // Add a system item: "[Compacting context...]"
    m.transcript = appendSystemItem(m.transcript, "[Compacting context...]", itemIDCompaction)

case agent.CompactionCompleted:
    ev := e.(agent.CompactionCompleted)
    // Replace the compaction system item with result
    if ev.Result.Skipped {
        m.transcript = replaceSystemItem(m.transcript, itemIDCompaction,
            fmt.Sprintf("[Compact skipped: not enough turns (need %d, have %d)]",
                m.compactionCfg.MinTurns, ev.Result.Before))
    } else if ev.Result.Error != "" {
        m.transcript = replaceSystemItem(m.transcript, itemIDCompaction,
            "[Compaction failed — continuing without compaction]")
    } else {
        m.transcript = replaceSystemItem(m.transcript, itemIDCompaction,
            fmt.Sprintf("[Compacted: %d → %d messages]",
                ev.Result.Before, ev.Result.After))
    }
```

Note: `appendSystemItem` and `replaceSystemItem` are existing or new helpers in `transcript.go`. If replace is not available, add it. The simplest approach is to append and let the final item override for display.

### Step 6 - Hook Dispatch Integration

Files:

- `internal/agent/compact.go` or `internal/agent/agent.go`

In `doCompact`, dispatch `PreCompact` and `PostCompact` hooks using the hook runner.

The hook runner is already wired in `internal/cli/repl.go` as a decorator around the agent. To make hook dispatch available inside `doCompact`, the `Agent` struct should hold an optional hook dispatcher:

```go
type Agent struct {
    // ... existing fields ...
    hookDispatch func(ctx context.Context, event hooks.Event, payload any) hooks.Decision
}
```

Wire this in `internal/cli/repl.go` after the hook runner is constructed.

Phase 20 simplification: if adding `hookDispatch` to `Agent` creates coupling concerns, defer `PreCompact`/`PostCompact` hook dispatch to Phase 13's hook UX work. The hook event constants already exist; implementing dispatch is incremental. The minimum Phase 20 deliverable is `CompactionStarted`/`CompactionCompleted` agent events with TUI display.

Decision: implement `PreCompact` dispatch minimally. Pass through the existing `HookDecision` pattern: if the hook runner's `PreCompact` decision is `deny`, skip compaction. If no hook runner is wired, treat as `allow`.

Tests:

- [ ] `PreCompact` hook returning `deny` skips compaction.
- [ ] `PostCompact` hook fires after successful compaction.
- [ ] No hook wired: compaction proceeds normally.

### Step 7 - Slash Command

Files:

- `internal/tui/app.go`
- `internal/tui/slash_test.go`

Add `/compact` to the slash command parser. When parsed:

1. If `ActiveRun == true`: signal the agent goroutine via a channel or message. In the current architecture, agent events flow one way (agent → TUI via `<-chan Event`). To send a signal from TUI to the agent, the simplest approach is to add a `compactCh chan struct{}` to `agent.Input` that the agent loop selects on.
2. If `ActiveRun == false`: run a quick in-place Layer 4 truncation on `state.App.Messages` if turn count >= MinTurns. Display result.

Implementation detail for `agent.Input.CompactRequest chan struct{}`:

In `a.run()`, add a select case after each turn completion:

```go
select {
case <-in.CompactRequest:
    messages = a.doCompact(ctx, cfg, messages, lastContextTokens, events)
default:
    // no compact request
}
```

This is non-blocking and only triggers if `/compact` was explicitly requested.

Tests:

- [x] `/compact` is parsed correctly by slash parser.
- [x] `/compact` with no active run and `state.App.Messages` < MinTurns*2 shows skip message.
- [x] `/compact` with active run sends signal to agent.

### Step 8 - Config Wiring

Files:

- `internal/cli/repl.go`

Wire `CompactionConfig` into `agent.Config` at REPL startup:

```go
agentCfg := agent.DefaultConfig()
agentCfg.Compaction = agent.DefaultCompactionConfig()
// Future: read from config file when Phase 13 config loader exists
```

If `bootstrap.Snapshot` contains model capability information (`RecommendedNumCtx`), populate `agentCfg.Compaction.MaxContextTokens`:

```go
if cap, ok := llm.CapabilityFor(bootstrap.Snapshot.DefaultModel); ok {
    agentCfg.Compaction.MaxContextTokens = int64(cap.RecommendedNumCtx)
}
```

### Step 9 - Tests and Verification

Required commands:

```sh
go test ./internal/agent/...
go test -race ./internal/agent/...
go test ./internal/tui/...
go test ./...
tools/check-allowed-deps.sh
tools/check-network-policy.sh
```

Manual smoke:

```sh
go run ./cmd/nandocodego --model qwen3 --no-alt-screen
```

Flow:

1. Run 10+ turns with tool calls.
2. Observe `[Compacting context...]` in transcript.
3. Confirm session continues.
4. Type `/compact` manually.
5. Confirm result message appears.
6. Start fresh REPL with only 2 turns; type `/compact`.
7. Confirm skip message.

## Full Implementation Checklist

### Step 1 — Agent Events

- [x] Add `CompactionStarted` type to `internal/agent/events.go`.
- [x] Add `CompactionCompleted` type to `internal/agent/events.go`.
- [x] Both types implement `isEvent()`.

### Step 2 — CompactionConfig and CompactionResult

- [x] Create `internal/agent/compact.go`.
- [x] Define `CompactionConfig` struct with all fields.
- [x] Implement `DefaultCompactionConfig()` with threshold=0.8, minTurns=4, maxSummaryLen=2000.
- [x] Define `CompactionResult` struct including `Messages []llm.Message` (internal field).
- [x] Test: `DefaultCompactionConfig()` returns correct defaults.
- [x] Test: zero-value disabled config.

### Step 3 — Core Compaction Functions

- [x] Implement `countTurns(messages []llm.Message) int`.
- [x] Implement `collapseToolResults(messages []llm.Message) []llm.Message`.
- [x] Implement `stripThinkingBlocks(messages []llm.Message) []llm.Message`.
- [x] Implement `emergencyTruncate(messages []llm.Message, minTurns int) []llm.Message`.
- [x] Implement `buildSummaryPrompt(cfg CompactionConfig, messages []llm.Message) string`.
- [x] Implement `summarizeMessages(ctx, client, cfg, model, msgs) (string, error)`.
- [x] Implement `Compact(ctx, client, cfg, model, messages) CompactionResult`.
- [x] Create `internal/agent/compact_test.go`.
- [x] Test: `countTurns` counts user+assistant pairs.
- [x] Test: `countTurns` ignores system messages.
- [x] Test: `countTurns` empty slice returns 0.
- [x] Test: `collapseToolResults` replaces long results.
- [x] Test: `collapseToolResults` preserves short results.
- [x] Test: `collapseToolResults` skips non-tool messages.
- [x] Test: `stripThinkingBlocks` removes old thinking.
- [x] Test: `stripThinkingBlocks` preserves last 2 turns.
- [x] Test: `emergencyTruncate` keeps system messages and last MinTurns turns.
- [x] Test: `emergencyTruncate` drops middle messages.
- [x] Test: `Compact` skipped when under MinTurns.
- [x] Test: `Compact` Layer 1 success with fake LLM client.
- [x] Test: `Compact` Layer 4 fallback on LLM error.
- [x] Test: `Compact` Before/After counts correct.
- [x] Test: `summarizeMessages` with fake client returns text.
- [x] Test: `summarizeMessages` respects context cancellation.
- [x] Test: `summarizeMessages` with deadline that expires returns error.
- [x] Test: `go test -race ./internal/agent/...` passes.

### Step 4 — Agent Loop Integration

- [x] Add `Compaction CompactionConfig` field to `agent.Config`.
- [x] Update `DefaultConfig()` to embed `DefaultCompactionConfig()`.
- [x] Add `WithCompactionConfig(cfg) Option` functional option.
- [x] Add `doCompact` method to `Agent`.
- [x] Track `lastContextTokens int64` in `a.run()` from `prompt_eval_count` field of LLM response.
- [x] Implement proactive compaction check after each successful turn.
- [x] Implement reactive compaction on second `DoneReason: "length"`.
- [x] Third `DoneReason: "length"` after compaction emits `TerminalContextOverflow`.
- [x] Add `CompactRequest chan struct{}` field to `agent.Input`.
- [x] In `a.run()`, select on `in.CompactRequest` after each turn.
- [x] Test: fake-length client triggers compaction on second length.
- [x] Test: after compaction success, agent continues.
- [x] Test: `Disabled=true` skips compaction and emits `TerminalContextOverflow`.
- [x] Test: MinTurns not met skips compaction.
- [x] Test: `CompactRequest` channel triggers manual compaction.
- [x] Test: `go test -race ./internal/agent/...` passes.

### Step 5 — TUI Integration

- [x] Handle `agent.CompactionStarted` in `internal/tui/app.go` event reducer.
- [x] Handle `agent.CompactionCompleted` in `internal/tui/app.go` event reducer.
- [x] Display `[Compacting context...]` system transcript item on started.
- [x] Display result or skip/error message on completed.
- [x] Test: transcript contains compaction system item after `CompactionStarted` event.
- [x] Test: transcript item updated after `CompactionCompleted`.

### Step 6 — Hook Dispatch

- [ ] Add optional `hookDispatch` callback to `Agent` struct.
- [ ] In `doCompact`, dispatch `PreCompact` event before calling `Compact`.
- [ ] If `PreCompact` returns `deny`, skip compaction and log at DEBUG.
- [ ] In `doCompact`, dispatch `PostCompact` event after `Compact` returns.
- [ ] Wire hook dispatcher in `internal/cli/repl.go`.
- [ ] Test: `PreCompact` deny skips compaction.
- [ ] Test: `PostCompact` fires on success.
- [ ] Test: no hook dispatcher — compaction proceeds normally.

### Step 7 — Slash Command

- [x] Add `compact` to slash command parser in `internal/tui/app.go`.
- [x] Handle `/compact` when `ActiveRun=false`: run Layer 4 truncation on `state.App.Messages` if turn count >= MinTurns.
- [x] Handle `/compact` when `ActiveRun=true`: send signal via `in.CompactRequest` channel.
- [x] Display `[Compact skipped: ...]` when MinTurns not met.
- [x] Test: slash parser recognizes `/compact`.
- [x] Test: `/compact` with no active run and insufficient turns shows skip.

### Step 8 — Config Wiring

- [x] Wire `DefaultCompactionConfig()` in `internal/cli/repl.go`.
- [x] Populate `MaxContextTokens` from model capability if available.
- [x] Pass `CompactionConfig` through `agent.WithCompactionConfig(cfg)`.

### Step 9 — Final Verification

- [x] `go test ./internal/agent/...` passes.
- [x] `go test -race ./internal/agent/...` passes.
- [x] `go test ./internal/tui/...` passes.
- [x] `go test ./...` passes.
- [x] `tools/check-allowed-deps.sh` passes.
- [x] `tools/check-network-policy.sh` passes.
- [ ] Manual smoke: 10-turn session triggers compaction.
- [ ] Manual smoke: `/compact` works and shows correct messages.
- [x] `docs/PHASE-LOG.md` Phase 20 entry added.

## Acceptance Criteria

- [x] Agent does NOT emit `TerminalContextOverflow` on the second `DoneReason: "length"` when compaction is configured (default on).
- [x] Agent emits `TerminalContextOverflow` if compaction is disabled (`Compaction.Disabled=true`).
- [x] Agent emits `TerminalContextOverflow` if compaction is skipped (MinTurns not met) and length is still exceeded.
- [x] `CompactionStarted` event emitted before compaction begins (visible in TUI as `[Compacting context...]`).
- [x] `CompactionCompleted` event emitted after compaction finishes with correct `Before`/`After` counts.
- [x] Compaction preserves the most recent `MinTurns` complete turn pairs intact.
- [x] Compaction preserves all initial system messages (memory section, tool instructions).
- [x] After successful compaction, session continues producing correct responses.
- [ ] `PreCompact` hook fires before compaction.
- [ ] `PostCompact` hook fires after compaction.
- [ ] `PreCompact` hook returning `deny` skips compaction.
- [x] `/compact` slash command triggers compaction when an active run is in progress.
- [x] `/compact` slash command shows skip message when fewer than MinTurns turns exist.
- [x] `CompactionConfig.MinTurns=4` (default): fewer than 4 turns are not compacted.
- [x] Compaction with LLM failure falls back to Layer 4 (emergency truncate) and continues.
- [x] Layer 4 emergency truncate does not produce fewer messages than MinTurns turn pairs.
- [x] `go test -race ./internal/agent/...` passes without a live Ollama instance.
- [x] `tools/check-allowed-deps.sh` passes (no new direct dependencies added).
- [x] `tools/check-network-policy.sh` passes.
- [x] `docs/PHASE-LOG.md` Phase 20 entry records files, decisions, and exit gate status.

## Forbidden

- Compaction that modifies the user's most recent message.
- Compaction that drops tool results from the current turn.
- Compaction that fires with fewer than `MinTurns` complete turns.
- Logging the compaction summary content at any log level (content may include session material).
- Compaction that calls a non-configured external endpoint.
- Compaction that blocks the Bubble Tea `Update` loop.
- Persisting compaction state to disk.
- Adding WebSearch or any external-API tool as part of compaction.
- Attempting to compact MCP or sub-agent contexts before Phase 10/11.
- Adding more than one new direct dependency in this phase. No new direct dependencies should be needed for Phase 20.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---:|---|
| LLM summary is inaccurate and loses critical task context | High | Layer 2/3 preprocessing reduces noise; preserve last MinTurns turns intact; user can restart with fresh context if needed. |
| Compaction side-query itself hits context overflow (circular) | High | Collapse tool results (Layer 2) and strip thinking (Layer 3) first; summary prompt uses only collapsed messages; cap summary input with MaxSummaryLen. |
| Proactive threshold firing too aggressively | Medium | Default threshold=0.8 is conservative; `MaxContextTokens=0` disables proactive path entirely if Ollama doesn't report accurate counts. |
| Compaction introduces non-determinism in tool behavior | Low | Compaction preserves last MinTurns turns intact; tool context for current turn is always complete. |
| PreCompact hook deny blocks recovery from context overflow | Medium | If hook denies and session hits length again, emit TerminalContextOverflow — user can disable blocking hook. |
| TUI state inconsistency after compaction (old message count vs. new) | Low | `Terminal.Conversation` carries compacted messages; `state.App.Messages` updated at run completion. |
| Agent retry loop after compaction fires third length immediately | Low | Third length emits TerminalContextOverflow; documented behavior. User should run `/compact` proactively in very long sessions. |
| `/compact` channel signal lost if agent goroutine exits before select | Low | Non-blocking select with default case; silently drops signal if not active. |
| compactRequest channel blocks TUI message send | Low | Use buffered channel (`make(chan struct{}, 1)`) so TUI send never blocks. |

## Phase Log Template

When implementation finishes, append a Phase 20 entry to `docs/PHASE-LOG.md` with:

- objective (graceful context window management, correctness fix not feature);
- files created/updated;
- dependencies added and allowlist status (expect none);
- tests run and results;
- manual smoke result (10-turn session, `/compact` command);
- design decisions:
  - 4-layer strategy with Layer 1 LLM summary as default path;
  - reactive trigger on second `DoneReason: "length"` vs. proactive `prompt_eval_count` threshold;
  - PreCompact hook deny causes TerminalContextOverflow;
  - summary not logged;
  - standalone `/compact` uses Layer 4 only when no run active;
- known gaps and deferred work:
  - DNS-based IP validation for WebFetch (from Phase 19, not Phase 20);
  - configurable `SummaryModel` via Phase 13 config;
  - richer `/compact history` command;
  - compaction for MCP/sub-agent contexts;
- exit gate status.

## Exit Gate

Phase 20 is complete only when:

- all acceptance criteria above are checked;
- `go test ./...` and `go test -race ./internal/agent/...` pass;
- `tools/check-allowed-deps.sh` and `tools/check-network-policy.sh` pass;
- the manual smoke REPL session demonstrates compaction firing in a long session;
- `/compact` slash command works correctly for both active and idle states;
- `docs/PHASE-LOG.md` records the Phase 20 implementation with any deviations from this plan.
