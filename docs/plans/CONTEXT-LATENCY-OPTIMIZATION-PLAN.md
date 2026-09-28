# Context & Latency Optimization Plan

## Roadmap Placement

This plan is required v0.1 work before new transport surfaces. Complete the latency/context foundation before Phase 21 server mode, and carry the TUI-facing render/status pieces into Phase 22.

Implementation relationship to the roadmap:

- Phases 0-3 and 5-8 of this plan belong to the pre-Phase-22 context/latency reliability step.
- Phase 4 streaming render optimization belongs to Phase 22 and should be implemented alongside ADR-001/TASKS-TUI run visibility work.
- The project-scale analysis workflow and retrieval phases must be complete before Phase 17 packaging.
- Do not solve latency by globally lowering `num_ctx`; use adaptive context, token-aware packing, trace data, and workflow-based analysis.

## Goal

Make the ask-and-response flow feel fast for normal prompts while preserving quality for large-context work such as full-project analysis.

This plan is intentionally agent-ready: each phase has a concrete objective, code areas, implementation tasks, acceptance criteria, tests, and risk notes.

## Problem Statement

The application currently has two competing needs:

1. **Fast interaction:** small or medium prompts should show visible progress and first response quickly.
2. **Large context quality:** project analysis must still support many files, memory, history, tool schemas, and enough output budget.

The current behavior is too static. It can use excessive context for small prompts, perform synchronous pre-run work before the main model starts, and re-render too much during streaming. At the same time, simply lowering `num_ctx` would harm large-context tasks.

The correct solution is **adaptive context + token-aware prompt packing + workflow-based project analysis + visible timing instrumentation**.

## Non-Goals

- Do not globally lower context in a way that silently reduces answer quality.
- Do not remove memory, hooks, thinking, or directory expansion.
- Do not require cloud services or external vector databases for the first pass.
- Do not introduce a heavy tokenizer dependency unless the repo already accepts it or a later phase explicitly approves it.

## Current Critical Paths

### Prompt Submission Path

Relevant code:

- `internal/tui/app.go`
  - `handleKeyMsg`
  - prompt submission around `mentions.ExpandPrompt`
  - `startAgentCmd`
- `internal/tui/bridge.go`
  - `startAgentCmd`
  - `drainAgentEvents`
- `internal/hooks/runner.go`
  - session start hooks
  - user prompt hooks
- `internal/memory/runner.go`
  - synchronous pre-run memory recall
- `internal/agent/stream.go`
  - `ChatRequest`
  - `num_ctx`
  - streaming event emission
- `internal/llm/ollama/ollama.go`
  - Ollama `/api/chat`
- `internal/tui/app.go`
  - `handleAgentEvent`
  - `refreshViewportContent`
  - `renderTranscript`

### Primary Suspected Latency Sources

1. **Context sizing is not adaptive enough.**
   - `num_ctx` should match the request and model, not be a giant static default.

2. **Memory recall can block before the main model starts.**
   - `memory.Runner.buildAugmentedPrompt` calls an LLM recall path before `next.Run`.

3. **Hooks can block before the main model starts.**
   - `SessionStart` and `UserPromptSubmit` hooks run synchronously.

4. **Mention expansion blocks before agent start.**
   - Large `@dir` expansion can read/walk many files before any model request.

5. **Streaming render is too expensive.**
   - The TUI currently refreshes the whole transcript for every event and can Glamour-render the growing assistant message repeatedly.

6. **Huge project analysis is treated too much like one giant prompt.**
   - This does not scale as well as chunk/retrieve/summarize workflows.

## Design Principles

1. **Preserve user intent.**
   - Explicit `@file` and `@dir` context should be prioritized over implicit memory and old history.

2. **Measure before optimizing.**
   - Add timings before changing defaults aggressively.

3. **Use adaptive context.**
   - Small prompts should not pay for huge context.
   - Large prompts should automatically get larger context when the model supports it.

4. **Use context-aware degradation.**
   - If content does not fit, summarize, rank, or chunk it visibly.
   - Do not silently drop important files.

5. **Make slow stages visible.**
   - The TUI should explain whether it is expanding files, recalling memory, running hooks, waiting for Ollama, thinking, or rendering.

6. **Project-scale analysis is a workflow.**
   - Do not rely on one enormous prompt for whole-repo analysis.

---

## Phase 0 — Baseline And Guardrails

### Objective

Create a measurable baseline without changing user-facing behavior.

### Tasks

- [ ] Add a per-run trace type.

```go
type RunTrace struct {
    RunID string

    SubmitAt              time.Time
    MentionExpandStartAt  time.Time
    MentionExpandDoneAt   time.Time
    HookPreflightStartAt   time.Time
    HookPreflightDoneAt    time.Time
    MemoryRecallStartAt    time.Time
    MemoryRecallDoneAt     time.Time
    ChatRequestStartAt     time.Time
    ChatResponseHeadersAt  time.Time
    FirstStreamEventAt     time.Time
    FirstVisibleRenderAt   time.Time
    TerminalAt             time.Time

    EffectiveNumCtx        int
    ModelContextLimit      int
    EstimatedInputTokens   int
    ReservedOutputTokens   int
    PromptBytes            int
    MessageCount           int
    ToolSchemaTokens       int
    MemoryTokens           int
    MentionFileCount       int
    MentionDirCount        int
}
```

- [ ] Store the latest trace in app state or observability meter.
- [ ] Add `/trace last` slash command, or extend `/cost` with last-run timing.
- [ ] Add debug status messages only when a stage exceeds a threshold:
  - `Expanding mentions...`
  - `Running hooks...`
  - `Recalling memory...`
  - `Waiting for model...`
  - `Rendering response...`
- [ ] Add unit tests for trace recording with fake delayed runner/client.

### Files To Inspect / Modify

- `internal/tui/app.go`
- `internal/tui/messages.go`
- `internal/observability/metrics.go`
- `internal/commands/registry.go`
- `internal/memory/runner.go`
- `internal/hooks/runner.go`
- `internal/llm/ollama/ollama.go`

### Acceptance Criteria

- A slow request can be explained from trace data.
- Tests can simulate a delayed memory/hook/model stage and assert trace durations.
- Normal output is not noisy unless a debug command/status is requested.

### Risks

- Avoid adding high-frequency `store.Set` calls during streaming.
- Avoid logging prompt bodies or file contents in metrics.

---

## Phase 1 — Token Estimation And Context Policy

### Objective

Right-size `num_ctx` per run without sacrificing large-context quality.

### Current Issue

Using a very high `num_ctx` can slow prompt evaluation and memory allocation, but lowering it globally can reduce context quality. The fix is adaptive sizing.

### Design

Add a rough token estimator first. It does not need exact tokenizer parity to be useful for budgeting.

```go
func EstimateTokens(s string) int {
    // Conservative heuristic, e.g. chars / 3.5 plus structure overhead.
}
```

Add context policy:

```go
type ContextPolicy struct {
    Mode             string  // auto, fixed, max
    FixedNumCtx      int
    MinNumCtx        int
    MaxNumCtx        int
    ReserveOutputPct float64
    ReserveOutputMin int
}
```

Recommended defaults:

```toml
[context]
mode = "auto"
min_num_ctx = 8192
max_num_ctx = 0 # 0 means model reported limit
reserve_output_pct = 0.25
reserve_output_min = 4096
```

Effective context:

```text
estimated_input = system + tools + history + memory + current prompt
reserved_output = max(estimated_input * reserve_output_pct, reserve_output_min)
needed = estimated_input + reserved_output
effective_num_ctx = clamp(nextTier(needed), min_num_ctx, model_limit_or_config_max)
```

Suggested tiers:

```text
8192, 16384, 32768, 65536, 131072, 262144, model_limit
```

### Tasks

- [ ] Add `internal/llm/tokens.go`.
- [ ] Add `internal/llm/context_policy.go`.
- [ ] Add config fields under `[context]`.
- [ ] Load context config through `internal/config`.
- [ ] Add fields to bootstrap/app state:
  - effective/max/min context
  - context mode
  - reserve output policy
- [ ] Compute effective `num_ctx` per run after prompt assembly.
- [ ] Stop sending an arbitrary huge static `num_ctx` when mode is `auto`.
- [ ] Use live `ShowModel` context length as model limit when available.
- [ ] Add visible warning when estimated context is close to limit.

### Files To Inspect / Modify

- `internal/llm/limits.go`
- `internal/llm/types.go`
- `internal/config/config.go`
- `internal/config/defaults.go`
- `internal/config/loader.go`
- `internal/bootstrap/state.go`
- `internal/state/app.go`
- `internal/cli/repl.go`
- `internal/agent/stream.go`

### Acceptance Criteria

- Small prompt uses a small/medium `num_ctx`.
- Large prompt uses larger `num_ctx` up to model limit.
- Explicit CLI/config override still works.
- Trace reports `EffectiveNumCtx`, `ModelContextLimit`, and estimated input tokens.

### Tests

- [ ] Unit test token estimator.
- [ ] Unit test context tier selection.
- [ ] Unit test config loading.
- [ ] Agent request test verifies expected `num_ctx` for small/large prompts.

---

## Phase 2 — Token-Aware Prompt Assembly

### Objective

Make prompt composition aware of available context budget.

### Problem

File/directory expansion currently uses byte and file caps. It should also respect token/context budget and reserve enough room for answer/thinking output.

### Design

Introduce prompt parts:

```go
type PromptPartKind string

const (
    PromptPartSystem PromptPartKind = "system"
    PromptPartUser PromptPartKind = "user"
    PromptPartToolSchema PromptPartKind = "tool_schema"
    PromptPartHistory PromptPartKind = "history"
    PromptPartMemory PromptPartKind = "memory"
    PromptPartFile PromptPartKind = "file"
    PromptPartDirectoryTree PromptPartKind = "directory_tree"
    PromptPartFileSummary PromptPartKind = "file_summary"
)

type PromptPart struct {
    Kind     PromptPartKind
    Source   string
    Priority int
    Content  string
    Tokens   int
    Required bool
}
```

Priority order:

1. Current user request.
2. Explicit file mentions.
3. Explicit directory tree and user-selected paths.
4. Relevant file chunks from explicit directories.
5. Recent conversation.
6. Memory.
7. Older conversation summaries.
8. Low-relevance file chunks.

### Tasks

- [ ] Split `mentions.ExpandPrompt` into:
  - resolve mentions
  - build candidate prompt parts
  - pack parts into budget
- [x] Add a `PromptPacker` package or module.
- [ ] Return an expansion report:

```go
type PromptAssemblyReport struct {
    IncludedFiles int
    SummarizedFiles int
    SkippedFiles int
    EstimatedTokens int
    BudgetTokens int
    Skipped []SkippedPart
}
```

- [ ] Add user-visible summary:
  - `expanded 47 files: included 18, summarized 21, skipped 8, estimated 52k tokens`
- [ ] Do not silently drop explicit files; if explicit files cannot fit, show warning and ask for a narrower request or summarize.

### Files To Inspect / Modify

- `internal/mentions/expand.go`
- `internal/tools/dirwalk/walk.go`
- `internal/tui/app.go`
- `internal/tools/context.go`
- new package candidate: `internal/promptpack`

### Acceptance Criteria

- Explicit small `@file` prompts remain unchanged.
- Large `@dir` prompts no longer blindly overfill context.
- Skipped/truncated/summarized content is reported.
- Prompt assembly respects output reserve.

### Tests

- [x] Prompt packer keeps required parts.
- [x] Prompt packer drops lower-priority parts first.
- [ ] Directory expansion report includes skipped reasons.
- [ ] Large fake directory stays under token budget.

---

## Phase 3 — Faster Memory Recall

### Objective

Avoid a pre-run LLM call on every prompt while preserving useful memory context.

### Current Issue

`memory.Runner.buildAugmentedPrompt` performs LLM recall before the main agent starts. This can add up to `RecallTimeout` before first response.

### Design

Add memory recall modes:

```toml
[memory]
recall_mode = "fast" # off, fast, llm
recall_timeout = "1s"
```

Modes:

- `off`: load only `MEMORY.md`.
- `fast`: keyword/frecency selection over memory metadata. No LLM call.
- `llm`: current LLM recall behavior.

Default should be `fast`.

### Tasks

- [ ] Add `RecallMode` to memory config.
- [ ] Implement lexical/frecency scorer.
- [ ] Use LLM recall only when mode is `llm`.
- [ ] Trace memory scan and recall separately.
- [ ] Surface slow memory recall in TUI if >150ms.

### Files To Inspect / Modify

- `internal/memory/types.go`
- `internal/memory/runner.go`
- `internal/memory/recall.go`
- `internal/config/*`
- `internal/commands/registry.go`

### Acceptance Criteria

- Default memory recall no longer performs an LLM call.
- `memory.recall_mode = "llm"` preserves current behavior.
- Trace shows memory stage duration and selected memory files.

### Tests

- [ ] Fast recall selects by keyword.
- [ ] Fast recall respects `MaxSelected`.
- [ ] LLM recall mode calls client.
- [ ] Off mode does not call client.

---

## Phase 4 — Streaming Render Optimization

### Objective

Make streamed responses appear smooth and immediate without expensive full markdown rendering on every token.

### Current Issue

Every agent event calls `refreshViewportContent(true)`, which calls `renderTranscript`. Assistant rendering calls Glamour on the full growing assistant content every time.

### Design

During active streaming:

- Render assistant deltas as plain text.
- Throttle viewport refresh to every 33-50ms.
- Glamour-render finalized assistant blocks on terminal.
- Cache rendered markdown in `TranscriptItem.Rendered`.

### Tasks

- [ ] Add streaming render throttle state to TUI model.
- [ ] Add `requestViewportRefresh(gotoBottom bool)` that batches refreshes.
- [ ] Mark assistant items as `Streaming`.
- [ ] During `Streaming`, bypass Glamour.
- [ ] On `Terminal`, render finalized assistant markdown once and cache it.
- [ ] Add render benchmarks.

### Files To Inspect / Modify

- `internal/tui/app.go`
- `internal/tui/transcript.go`
- `internal/tui/markdown.go`
- `internal/tui/app_test.go`

### Acceptance Criteria

- Streaming output appears within one render interval after first delta.
- Large streamed answers do not slow down progressively.
- Final rendered markdown still looks correct after completion.

### Tests / Benchmarks

- [ ] Unit test cache invalidation.
- [ ] Unit test streaming uses plain content.
- [ ] Benchmark `renderTranscript` for 1k, 10k, 50k chars.
- [ ] Benchmark repeated delta handling.

---

## Phase 5 — Hook Timing And Slow-Stage Visibility

### Objective

Make hook latency transparent and avoid surprising pre-model delays.

### Status

Partially implemented:
- `SessionStart` and `UserPromptSubmit` hook dispatch now emit `StageTiming` events.
- TUI slow-stage notices and `/trace last` can surface these stage timings.

Still missing:
- Stage timing for `PreToolUse`, `PostToolUse`, `PermissionDenied`, `Stop`, and `SessionEnd`.
- Trace-safe metadata for hook event kind/source so users can identify which hook caused delay without logging sensitive payloads.

### Tasks

- [x] Add timing around:
  - `SessionStart`
  - `UserPromptSubmit`
- [ ] Add timing around:
  - `PreToolUse`
  - `PostToolUse`
  - `PermissionDenied`
  - `Stop`
- [ ] Add trace entries for hook kind/event/source.
- [x] Emit slow hook notices when a timed hook stage exceeds threshold.
- [ ] Consider non-blocking mode for hooks that cannot deny.

### Files To Inspect / Modify

- `internal/hooks/runner.go`
- `internal/hooks/dispatch.go`
- `internal/hooks/types.go`
- `internal/tui/app.go`

### Acceptance Criteria

- Slow hooks are visible in trace.
- Blocking hooks remain blocking.
- Non-blocking hooks, if added, cannot deny or mutate permission decisions.

---

## Phase 6 — Project-Scale Analysis Workflow

### Objective

Support huge project analysis beyond single-context limits.

### Status

Partially implemented foundation:
- `internal/analysis` package now includes retrieval, checkpoint, token-bounded chunking, summary-cache primitives, and evidence-ledger persistence primitives.
- `/analyze-project` exists and now uses retrieval-ranked mention injection before expansion.
- Full chunk/map/reduce synthesis orchestration and progress events are still pending.
- Evidence-ledger persistence exists, but final answer citation flow is not yet fully wired end-to-end in synthesis prompts.

### Design

When a prompt asks for broad project analysis or when prompt assembly exceeds context budget, use a multi-step workflow:

```text
1. Index files
2. Select candidate files
3. Chunk files
4. Summarize chunks
5. Merge file/package summaries
6. Synthesize final answer
```

Add command:

```text
/analyze-project [path] [question]
```

Or automatic fallback:

```text
Prompt too large for one context. Run project analysis workflow? [y/N]
```

### Tasks

- [x] Add file chunker with token limits and path/offset metadata.
- [x] Add summary cache keyed by:
  - path
  - file hash
  - model
  - summary prompt version
- [ ] Add map prompt for file/chunk summaries.
- [ ] Add reduce prompt for module/package summaries.
- [ ] Add final synthesis prompt.
- [ ] Store intermediate summaries under cache dir, not project files.
- [x] Add evidence ledger containing selected files, chunks, cache hits, summaries, and final synthesis inputs.
- [ ] Add progress events:
  - indexing
  - cache hit/miss
  - summarizing N/M
  - reducing package/module summaries
  - synthesizing

### Files / New Packages

- New package candidate: `internal/analysis`
- `internal/commands/registry.go`
- `internal/tasks` may be reused for background analysis
- `internal/tools/dirwalk`
- `internal/llm`
- `internal/tui/app.go`

### Acceptance Criteria

- Can analyze this repository without requiring every file in one prompt.
- Re-running after no file changes reuses summaries.
- User sees progress.
- Final answer cites files/summaries used.

### Tests

- [ ] Chunker preserves file path metadata.
- [ ] Cache invalidates on file hash change.
- [ ] Evidence ledger preserves file path and summary provenance.
- [ ] Fake LLM map/reduce workflow produces expected synthesis.
- [ ] Large fake repo completes under bounded context.

---

## Phase 7 — Retrieval Before Expansion

### Objective

For broad questions, include relevant context before dumping everything.

### First-Pass Retrieval

Use lexical scoring:

- path match
- filename match
- extension/type priority
- symbol/header match where cheap
- recency/frecency
- memory metadata match

Later:

- embeddings over file summaries/chunks
- hybrid lexical + vector score

### Tasks

- [x] Add `ContextRetriever`.
- [x] Rank file index entries against latest user request.
- [ ] Feed ranked entries into prompt packer.
- [x] Add transparent report:
  - `selected 23 of 940 files by relevance`

### Acceptance Criteria

- Broad prompts include relevant files without explicit mentions.
- Explicit mentions always outrank retrieval.
- Retrieval decisions are inspectable.

Current behavior note:
- Retrieval is currently wired in the `/analyze-project` workflow.
- Global broad-prompt retrieval (outside `/analyze-project`) remains pending.
- Retrieval currently ranks path/filename/extension and frecency. Cheap symbol/header extraction and memory metadata scoring remain pending.

---

## Phase 8 — User Controls

### Objective

Give users explicit speed/depth controls without requiring code changes.

### Commands

```text
/context status
/context auto
/context small
/context large
/context max
/memory recall off|fast|llm
/thinking auto|off|on
/trace last
/analyze-project [path] [question]
```

### Config

```toml
[context]
mode = "auto"
min_num_ctx = 8192
max_num_ctx = 0
reserve_output_pct = 0.25
reserve_output_min = 4096

[memory]
recall_mode = "fast"
recall_timeout = "1s"

[tui]
stream_render_interval_ms = 40
slow_stage_notice_ms = 150

[analysis]
cache_enabled = true
chunk_tokens = 6000
summary_tokens = 1000
```

### Acceptance Criteria

- Users can choose fast mode or max-context mode.
- Current effective settings are visible.
- Defaults remain quality-preserving but faster than current behavior.

---

## Recommended Implementation Order

1. **Trace instrumentation**
   - Gives evidence and protects future work from guessing.

2. **Adaptive context policy**
   - Fixes core `num_ctx` correctness.

3. **Streaming render optimization**
   - Improves perceived speed with limited behavioral risk.

4. **Fast memory recall default**
   - Removes a common pre-model LLM call.

5. **Token-aware prompt packer**
   - Protects quality as context becomes adaptive.

6. **Context-aware mention expansion**
   - Prevents large `@dir` prompts from overfilling context.

7. **Project-scale analysis workflow**
   - Enables huge-context analysis safely.

8. **Retrieval ranking**
   - Improves relevance and reduces unnecessary prompt bulk.

9. **User controls**
   - Exposes speed/depth tradeoffs.

## Success Metrics

Track these before and after:

- Time from Enter to user prompt visible.
- Time from Enter to main model request start.
- Time from main model request start to first stream event.
- Time from first stream event to first visible TUI render.
- Effective `num_ctx`.
- Estimated input tokens.
- Prompt bytes.
- Memory recall duration.
- Hook preflight duration.
- Render duration per streamed event.

Target outcomes:

- Small prompt first visible model output: materially faster than baseline.
- Large explicit-context prompt: context preserved up to model limit.
- Huge project analysis: completes via workflow instead of failing or hanging.
- Streaming output remains smooth for long answers.

## Risk Register

| Risk | Mitigation |
|---|---|
| Lower context harms quality | Use adaptive context, not fixed low context |
| Token estimator inaccurate | Use conservative estimates and warning margins |
| Explicit file context dropped silently | Mark explicit parts required and report overflow |
| Memory quality drops | Allow `memory.recall_mode = "llm"` and compare traces |
| Streaming markdown looks worse | Use plain text only while streaming, render markdown on final |
| Project analysis costs too many LLM calls | Cache summaries by file hash and prompt version |
| Too many status messages | Show only stage changes or slow-stage notices |

## Definition Of Done

- `go test ./...` passes.
- `/trace last` or equivalent shows stage timings.
- Effective `num_ctx` is adaptive and visible.
- TUI streaming render is throttled/cached.
- Memory recall no longer performs a blocking LLM call by default.
- Large `@dir` prompt produces a context report.
- Full project analysis has a workflow path with cached summaries.
