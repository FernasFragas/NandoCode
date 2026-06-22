# Phase 15 Detailed Plan - Concurrency & Speculative Execution

Date: 2026-05-07
Status: ✅ Implemented in code (2026-05-08 reconciliation below)
Source plans and references:

- `.codex/go-ollama-plan-AGENTS.md`
- `.codex/go-ollama-plan-HUMANS.md`
- `docs/PHASE-LOG.md`
- `docs/PROJECT-STATUS-AND-ONBOARDING.md`
- `docs/PHASE-8-DETAILED-PLAN.md`
- `docs/PHASE-9-DETAILED-PLAN.md`
- `docs/PHASE-14-DETAILED-PLAN.md`
- `book/ch05-agent-loop.md`
- `book/ch06-tools.md`
- `book/ch15-concurrency.md`
- `.codex/agent-context/learnings-memory.md`

## Goal

Phase 15 upgrades the agent's tool execution from serial to concurrent while preserving submission-order correctness. A greedy left-to-right partition algorithm identifies safe concurrent batches and serial singletons. Speculative execution starts a tool call while the LLM is still streaming the rest of the turn. A concurrency cap of 10 limits simultaneous tool executions using `errgroup` from `golang.org/x/sync`.

The user-visible goal is that reads, searches, and other concurrency-safe tools complete in the time of the slowest individual call rather than the sum of all calls. A turn that previously required 5 × 800 ms file reads (4 s total) completes in approximately 800 ms.

Deliverables:

- `internal/agent/partition.go` — greedy left-to-right partition algorithm with property tests.
- `internal/agent/speculative.go` — speculative executor that begins tool execution during streaming.
- Modified `internal/agent/stream.go` — `accumulateTurn` wired to speculative executor.
- Modified `internal/agent/tools.go` — `executeToolCalls` replaced by `executeConcurrent`.
- `tools.Tool` interface extended with `IsConcurrencySafe(input any) bool` and `IsDestructive(input any) bool`.
- Benchmark suite: 5 parallel FileRead calls, partition algorithm throughput.
- Property tests and fuzz tests for partition and ordering.

## Definition Of Success

The Phase 15 exit gate is an automated benchmark plus a manual integration check:

1. Benchmark `BenchmarkConcurrentFileRead` starts 5 file-read tool calls in one turn. The measured total duration must be at most `max(individual) + 10%`, not `sum(individual)`.
2. A fuzz-driven property test confirms that, for any permutation of concurrent-safe and non-safe tool calls, submission-order yield holds (results arrive in call order).
3. `go test -race ./internal/agent/...` passes with zero races.
4. Serial tool execution (all tools `IsConcurrencySafe=false`) behaves identically to Phase 14 baseline.

## Baseline Analysis From Implemented Phases

### Phase 0 — Security and Supply Chain

Implemented:

- dependency allowlist
- network policy checker

Phase 15 implications:

- `golang.org/x/sync` must be added to `go.mod` and `tools/allowed-deps.txt`. This is a first-party Google/Go dependency and should be allowlisted without controversy.
- Concurrency must not create new outbound connections. All new goroutines call only local tools.
- Race detector is mandatory for all Phase 15 tests.

### Phase 1 — CLI, Paths, Logging, Scaffold

Implemented: CLI scaffold, logging, `internal/paths`.

Phase 15 implications:

- No path changes required.
- Logging should record batch sizes and partition decisions at DEBUG level, not at INFO.
- Speculative executor should log speculative start and abort at DEBUG.

### Phase 2 — LLM Client

Implemented:

- `llm.Client` with streaming `Chat`
- `llm.StreamEvent` carrying incremental tool call data

Phase 15 implications:

- Speculative execution depends on the stream emitting complete tool call JSON incrementally. The current Ollama client emits tool calls only in the `Done` event (full JSON, not streaming fragments). This is a critical constraint.
- **Speculative execution in Phase 15 is bounded**: a tool call is started speculatively only when its complete `llm.ToolCall` struct has been parsed from the stream. With the Ollama client, this means all tool calls arrive together in the Done event, making speculative execution equivalent to starting concurrent execution immediately after the stream closes.
- If a future provider sends incremental tool call JSON, the speculative executor can start earlier. Phase 15 should structure the speculative path to support this improvement without requiring a rewrite.
- Document the Ollama streaming constraint prominently in `speculative.go`.

### Phase 3 — Tool Interface and Starter Tools

Implemented:

- `tools.Tool` interface: `Name`, `Description`, `InputSchema`, `UnmarshalInput`, `Call`, `IsEnabled`.
- `Bash`, `FileRead`, `FileWrite`.

Phase 15 implications:

- Extend `tools.Tool` with two new methods:
  - `IsConcurrencySafe(input any) bool` — true if the tool can run in parallel with other concurrent-safe tools.
  - `IsDestructive(input any) bool` — true if the tool modifies external state (files, processes, network).
- Default implementations on the base type (or via embedding) should return `false` for both, preserving safe serial behavior for tools that do not opt in.
- `FileRead`: `IsConcurrencySafe=true`, `IsDestructive=false`.
- `Bash`: `IsConcurrencySafe=false`, `IsDestructive=true` (commands are non-deterministic side-effects).
- `FileWrite`: `IsConcurrencySafe=false`, `IsDestructive=true`.
- `MCPTool`: `IsConcurrencySafe=false` by default; individual MCPs can override.
- `AgentTool`: `IsConcurrencySafe=false` (sub-agents have their own turn; concurrency here creates nesting complexity).
- `SkillTool`: `IsConcurrencySafe=false` (skills may call any tool).
- Task tools (Phase 14): `IsConcurrencySafe=true` for TaskList/TaskGet/TaskOutput; `=false` for TaskCreate/TaskStop.

### Phase 4 — Agent Loop

Implemented:

- `agent.Run(ctx, input) <-chan agent.Event`
- `executeOneTurn` → `accumulateTurn` → `executeToolCalls` (serial)
- `sendEvent` helper

Phase 15 implications:

- `executeToolCalls` is the primary target for replacement.
- `accumulateTurn` calls `executeToolCalls` after the stream closes. Phase 15 restructures this to wire speculative execution during streaming.
- The event channel is buffered (size 16) and serial: Phase 15 must ensure events from concurrent tool executions are serialised before being sent to the channel.
- `ToolUseStart` and `ToolUseResult` events must still appear in call order, not completion order.
- The agent loop's `MaxTurns` and `MaxOutputTokens` are unchanged by Phase 15.

### Phase 5 — Permission System

Implemented:

- `permissions.Resolve` — central resolver
- `permissions.PromptFunc` — TUI prompt callback

Phase 15 implications:

- Permission resolution for each tool call must happen on the correct goroutine. The permission resolver is context-safe, but the TUI prompt callback (`PromptFunc`) may have internal state.
- During concurrent batch execution, each tool resolves permissions independently.
- If any tool in a batch is denied, the batch continues for the remaining tools; the denied tool emits a permission-denied event and a denied message. This matches existing serial behavior.
- `IsConcurrencySafe(input)` is evaluated after input parsing but before permission resolution.

### Phase 6 — State Layer

Implemented:

- `state.Store[state.App]` with copy-on-write semantics.
- `state.App.Messages`, `state.App.ToolSettings`, `state.App.Tasks`.

Phase 15 implications:

- Tool executions in a concurrent batch do not update state.App directly; they return `llm.Message` results to the caller, which appends them in submission order.
- No new state fields are required for Phase 15.
- The submission-order guarantee is enforced by the result aggregator, not by state.

### Phase 7 — Bubble Tea TUI and REPL

Implemented: TUI, REPL, permission modal, transcript rendering.

Phase 15 implications:

- The TUI receives `ToolUseStart` and `ToolUseResult` events in submission order. Concurrent internal execution does not change the event ordering observed by the TUI.
- No new TUI components are required.
- The status bar can optionally show a "concurrency: N" indicator when a batch is active, but this is deferred.

### Phase 8 — Memory

Implemented: `internal/memory` package, runner integration.

Phase 15 implications:

- Memory recall and extraction are sequential and use their own `llm.Client` calls. They are not affected by Phase 15 tool concurrency.
- Memory runner decorates the agent runner; it wraps `Run(ctx, input)` and is unaffected by the internal concurrent execution inside `Run`.

### Phase 9 — Hooks

Implemented: snapshot hooks, `PreToolUse`, `PostToolUse`.

Phase 15 implications:

- `PreToolUse` hooks run before permission resolution. In a concurrent batch, each tool's `PreToolUse` runs in its own goroutine. Hook functions must be goroutine-safe.
- Verify that the hook snapshot (`HookDecision` function) is safe to call concurrently. If it is not, serialize hook calls within a batch.
- `PostToolUse` hooks run after each tool completes, in completion order within the batch, but the agent receives all batch results in submission order.
- Document the concurrent hook call contract in Phase 15.

### Phase 10 — MCP Tools

Implemented: MCP tool integration, go-keyring, oauth2, fsnotify.

Phase 15 implications:

- MCP tools default to `IsConcurrencySafe=false`. MCP server calls may have side effects unknown to the client.
- A future extension can mark specific MCP tools as safe via their schema or capability declaration.
- Phase 15 does not introduce special-casing for MCP in the partition algorithm.

### Phase 11 — AgentTool

Implemented: `AgentTool` for inline sub-agent spawning.

Phase 15 implications:

- `AgentTool` is `IsConcurrencySafe=false`. Two sub-agents running concurrently in a batch could deadlock on shared resources or produce non-deterministic conversation state.
- This decision may be revisited in a future phase with explicit isolation guarantees.

### Phase 12 — SkillTool

Implemented: `SkillTool` for embedded skills.

Phase 15 implications:

- `SkillTool` is `IsConcurrencySafe=false` because skills may internally call any tool, including destructive ones.

### Phase 13 — Slash Commands, Config, --print Mode

Implemented: full slash commands, koanf config, `--print` mode.

Phase 15 implications:

- A config option `concurrency.max_batch_size` (default: 10) can be loaded from koanf to override the errgroup limit.
- `--print` mode benefits from concurrency; no special handling needed.

### Phase 14 — Tasks

Implemented: task supervisor, JSONL output, task tools, state integration.

Phase 15 implications:

- Task tools (TaskList, TaskGet, TaskOutput) are `IsConcurrencySafe=true`; they read from the supervisor state without side effects.
- TaskCreate and TaskStop are `IsConcurrencySafe=false`; they mutate supervisor state.
- The task supervisor's internal goroutines are unaffected by Phase 15 tool concurrency.

## Evaluation Of The Original Phase 15 Concept

The original concept is correct at the algorithm level:

- greedy left-to-right partition
- safe batch accumulation
- submission-order yield
- speculative execution during streaming
- errgroup with cap 10

It needs additional detail for this repo:

- It does not account for the Ollama constraint that tool call JSON arrives only in the Done event, which limits the practical benefit of speculative execution with the current client.
- It does not specify how `IsConcurrencySafe` and `IsDestructive` are added to the existing `tools.Tool` interface without breaking all existing tool implementations.
- It does not specify how permission prompts (blocking TUI callbacks) interact with concurrent batch execution.
- It does not define a backward-compatible default for tools that do not implement the new methods.
- It does not specify how `ToolUseStart` and `ToolUseResult` events are serialized for a concurrent batch.

## Final Phase 15 Scope

In scope:

- `tools.Tool` interface extension: `IsConcurrencySafe(any) bool`, `IsDestructive(any) bool`.
- Default implementations in a `BaseToolConcurrencyMixin` struct (embedded or standalone).
- `internal/agent/partition.go` — partition algorithm with property tests.
- `internal/agent/speculative.go` — speculative executor.
- Modified `internal/agent/stream.go` — speculative wiring.
- Modified `internal/agent/tools.go` — `executeConcurrent` replacing `executeToolCalls`.
- Existing tool updates: FileRead, FileWrite, Bash, MCPTool, AgentTool, SkillTool, task tools.
- `golang.org/x/sync` added to go.mod and allowlist.
- Benchmark: concurrent FileRead timing.
- Property tests: partition correctness for all safe/unsafe permutations.
- Fuzz tests: tool call ordering.
- Race detector: all agent tests.

Out of scope:

- Per-tool concurrency timeout (Phase 16 / observability).
- MCP tool concurrency opt-in via schema (future extension).
- AgentTool concurrency with isolation (future extension).
- Dynamic concurrency cap based on system load.
- Streaming incremental tool call JSON (requires provider support; Ollama does not provide it yet).
- Tool call retry within a concurrent batch (existing `retry.go` constraints documented in Phase 2 still apply).

## Target User Experience

### Before Phase 15

A turn with 5 file reads takes approximately 5 × 800 ms = 4 s.

```
Turn 1: [FileRead /a] → 800ms → [FileRead /b] → 800ms → ... → 4000ms
```

### After Phase 15

A turn with 5 file reads takes approximately max(800ms) = 800 ms.

```
Turn 1: [FileRead /a, FileRead /b, FileRead /c, FileRead /d, FileRead /e] → 800ms (concurrent)
```

### Mixed Safe/Unsafe Example

Tool calls: `[FileRead /a, FileRead /b, Bash("git log"), FileRead /c]`

Partition result:

1. Batch: `[FileRead /a, FileRead /b]` — concurrent
2. Singleton: `[Bash("git log")]` — serial
3. Batch: `[FileRead /c]` — single-item batch (still correct)

Timeline:
```
t=0ms:   start FileRead /a, FileRead /b (concurrent)
t=800ms: both complete; start Bash("git log")
t=1600ms: Bash completes; start FileRead /c
t=2400ms: FileRead /c completes
Total: ~2400ms (vs ~3200ms serial)
```

Results are submitted to the conversation in call order: `/a`, `/b`, `git log`, `/c`.

## Architecture

### Tool Interface Extension

```go
// tools/tool.go

type Tool interface {
    Name() string
    Description() string
    InputSchema() json.RawMessage
    UnmarshalInput(raw json.RawMessage) (any, error)
    Call(ctx tools.Context, input any, progress chan<- ProgressEvent) (Result, error)
    IsEnabled(ctx tools.Context) bool

    // New in Phase 15:
    // IsConcurrencySafe returns true if this tool can execute in parallel
    // with other concurrent-safe tools in the same batch.
    // Default: false (safe, serial).
    IsConcurrencySafe(input any) bool

    // IsDestructive returns true if this tool modifies external state.
    // Destructive tools are always isolated as singletons by the partition algorithm.
    // Default: true (safe, serial).
    IsDestructive(input any) bool
}
```

Backward compatibility approach: define a `BaseTool` embedded struct or a `DefaultConcurrencyBehavior` interface with a default method. All existing tools that do not override will return `IsConcurrencySafe=false`, `IsDestructive=true`, preserving serial behavior.

Alternative approach: use a type assertion in the partition algorithm:

```go
type ConcurrencySafeChecker interface {
    IsConcurrencySafe(input any) bool
    IsDestructive(input any) bool
}

func isSafe(t tools.Tool, input any) bool {
    if cc, ok := t.(ConcurrencySafeChecker); ok {
        return cc.IsConcurrencySafe(input) && !cc.IsDestructive(input)
    }
    return false // default: not safe
}
```

This avoids breaking existing tool implementations. The partition algorithm uses `isSafe(tool, input)` throughout.

### Partition Algorithm

```go
// internal/agent/partition.go

package agent

// Batch is a group of tool calls that can execute concurrently.
// A batch with one item is a singleton (serial execution).
type Batch struct {
    Calls []indexedCall
    Safe  bool // true if all items are concurrency-safe non-destructive
}

type indexedCall struct {
    Index int            // original position in the call slice
    Call  llm.ToolCall
    Tool  tools.Tool
    Input any
}

// Partition applies the greedy left-to-right algorithm and returns
// an ordered slice of batches preserving the original call order.
//
// Algorithm:
//  1. Walk calls left-to-right.
//  2. If current call is safe and accumulator is empty or all-safe: add to accumulator.
//  3. If current call is unsafe: emit accumulator as a batch (if non-empty);
//     emit current call as a singleton batch; reset accumulator.
//  4. After all calls: emit remaining accumulator as final batch.
func Partition(calls []indexedCall) []Batch
```

Property guarantees (verified by tests):

- All input calls appear exactly once in the output batches.
- Input call order is preserved across batches and within batches.
- No batch contains a mix of safe and unsafe calls.
- A safe batch always contains only safe calls.
- A singleton batch contains exactly one call (safe or unsafe).
- Safe calls adjacent to each other are grouped in the same batch.
- A single unsafe call between two safe groups splits them into three batches.

### Speculative Executor

```go
// internal/agent/speculative.go

package agent

// SpeculativeExecutor starts tool executions as soon as complete tool call JSON
// is available from the stream. With the Ollama client, this means executions
// start immediately after the stream Done event since Ollama delivers all tool
// calls together in the Done event.
//
// With providers that stream incremental tool call JSON (e.g., Anthropic), the
// executor can start earlier — as soon as each tool call's JSON is complete.
//
// The executor stores pending results by call index; the caller assembles them
// in submission order.
type SpeculativeExecutor struct {
    agent    *Agent
    toolCtx  tools.Context
    // ... permission and hook fields
}

// StartSpeculative begins executing a batch of tool calls concurrently.
// Returns a channel that receives results in submission order.
func (se *SpeculativeExecutor) StartSpeculative(ctx context.Context, batch Batch, events chan<- Event) <-chan batchResult

type batchResult struct {
    Index    int
    Messages []llm.Message
    Count    int
}
```

### Concurrent Execution

```go
// internal/agent/tools.go (modified)

// executeConcurrent replaces executeToolCalls.
// It partitions calls into batches, executes each batch with errgroup,
// and assembles results in submission order.
func (a *Agent) executeConcurrent(
    ctx context.Context,
    calls []llm.ToolCall,
    toolCtx tools.Context,
    // ... permission and hook params
    events chan<- Event,
) ([]llm.Message, int)
```

Execution loop:

```
for each batch in Partition(calls):
    if batch is singleton:
        execute serially (same as before Phase 15)
    else:
        eg, egCtx := errgroup.WithContext(ctx)
        eg.SetLimit(concurrencyLimit)  // default 10
        results := make([]batchResult, len(batch.Calls))
        for i, call := range batch.Calls:
            eg.Go(func() { results[i] = executeSingleCall(egCtx, call, ...) })
        eg.Wait()
        append results in batch call order to messages
```

Event serialization: each goroutine sends its `ToolUseStart` and `ToolUseResult` events through a per-goroutine channel. The aggregator drains per-goroutine channels in submission order and forwards to the main events channel. This ensures the TUI observes events in call order.

### Three-Level Abort Hierarchy

```
query ctx → batch eg ctx → per-tool context
```

- Query ctx canceled: `eg.Wait()` returns; outer loop exits; Terminal(Aborted).
- Batch eg ctx canceled: all goroutines in the batch receive cancellation; batch results are partial but still assembled in order.
- Per-tool ctx: individual tool timeout (Phase 16 adds per-tool deadlines; Phase 15 inherits query ctx for per-tool).

## Implementation Plan

### Step 1 — Add `golang.org/x/sync` to Go Module

Files:
- `go.mod`
- `go.sum`
- `tools/allowed-deps.txt`

- [ ] Run `go get golang.org/x/sync@latest` to add the dependency.
- [ ] Add `golang.org/x/sync` to `tools/allowed-deps.txt` with justification: "errgroup for concurrent tool batch execution".
- [ ] Verify `tools/check-allowed-deps.sh` passes.

### Step 2 — Extend `tools.Tool` Interface

Files:
- `internal/tools/tool.go`
- `internal/tools/concurrency.go` (new helper file)

- [ ] Add `IsConcurrencySafe(input any) bool` to the `tools.Tool` interface.
- [ ] Add `IsDestructive(input any) bool` to the `tools.Tool` interface.
- [ ] Define `ConcurrencySafeChecker` as an optional interface using type assertion.
- [ ] Implement `isSafeForBatch(t tools.Tool, input any) bool` helper in `tools` package.
- [ ] Update `FileRead` to implement `IsConcurrencySafe=true`, `IsDestructive=false`.
- [ ] Update `FileWrite` to implement `IsConcurrencySafe=false`, `IsDestructive=true`.
- [ ] Update `Bash` to implement `IsConcurrencySafe=false`, `IsDestructive=true`.
- [ ] Update `MCPTool` to default `IsConcurrencySafe=false`, `IsDestructive=true`.
- [ ] Update `AgentTool` to implement `IsConcurrencySafe=false`, `IsDestructive=true`.
- [ ] Update `SkillTool` to implement `IsConcurrencySafe=false`, `IsDestructive=true`.
- [ ] Update TaskList, TaskGet, TaskOutput to `IsConcurrencySafe=true`, `IsDestructive=false`.
- [ ] Update TaskCreate, TaskStop to `IsConcurrencySafe=false`, `IsDestructive=true`.
- [ ] Test: `isSafeForBatch` returns correct result for each tool type.
- [ ] Test: tool not implementing `ConcurrencySafeChecker` defaults to `isSafeForBatch=false`.

### Step 3 — Partition Algorithm

Files:
- `internal/agent/partition.go`
- `internal/agent/partition_test.go`

- [ ] Implement `Partition(calls []indexedCall) []Batch`.
- [ ] Handle empty input: return empty slice.
- [ ] Handle single safe call: return one single-item batch marked safe.
- [ ] Handle single unsafe call: return one singleton batch marked unsafe.
- [ ] Handle all-safe calls: return one batch with all calls.
- [ ] Handle all-unsafe calls: return N singleton batches.
- [ ] Handle safe/unsafe/safe: return three batches.
- [ ] Property test: for all permutations of N safe + M unsafe calls (N+M <= 8), all calls appear exactly once in output.
- [ ] Property test: output call indices are in ascending order across all batches.
- [ ] Property test: no batch contains both safe and unsafe calls.
- [ ] Property test: batch marked safe contains only safe calls.
- [ ] Fuzz test: random safe/unsafe bitmask → partition → reorder → assert sorted indices.
- [ ] Benchmark: `BenchmarkPartition1000Calls` partitions 1000 mixed calls under 1 ms.

### Step 4 — Event Serializer

Files:
- `internal/agent/eventsync.go` (new)
- `internal/agent/eventsync_test.go`

- [ ] Implement `OrderedEventEmitter` that accepts out-of-order per-goroutine events and re-emits them in submission order.
- [ ] Use a simple slice indexed by call position; each goroutine appends to its slot.
- [ ] After all goroutines complete, drain slots in index order to the main events channel.
- [ ] Test: 3 goroutines write events in reverse order → events emitted in original order.
- [ ] Test: events from a singleton batch pass through unchanged.
- [ ] Test: cancellation does not deadlock the emitter.

### Step 5 — Concurrent Tool Execution

Files:
- `internal/agent/tools.go`
- `internal/agent/tools_test.go`

- [ ] Implement `executeConcurrent` replacing `executeToolCalls`.
- [ ] Parse all inputs and look up all tools before partitioning (fail fast on unknown tools).
- [ ] For each singleton batch: call existing single-tool execution path.
- [ ] For each concurrent batch: use `errgroup.WithContext` with `SetLimit(10)`.
- [ ] Each goroutine in the batch executes one tool call using the existing `executeTool` helper.
- [ ] Permission resolution runs per-goroutine (each call resolves independently).
- [ ] After `eg.Wait()`, assemble messages in submission order.
- [ ] Events (ToolUseStart, ToolUseResult) are collected per-goroutine and emitted in submission order via `OrderedEventEmitter`.
- [ ] Test: 3 concurrent FileRead calls → messages appended in call order.
- [ ] Test: concurrent batch where middle call fails → other calls still complete; error result in correct position.
- [ ] Test: context cancellation during batch → all goroutines return; no goroutine leak.
- [ ] Test: errgroup limit of 10 is respected when 15 concurrent calls are partitioned (only 10 run at once).
- [ ] Test: serial tool (IsConcurrencySafe=false) runs as singleton even when surrounded by safe tools.
- [ ] Test: `go test -race ./internal/agent/...` passes.

### Step 6 — Speculative Executor Integration

Files:
- `internal/agent/speculative.go`
- `internal/agent/stream.go`

- [ ] Implement `SpeculativeExecutor` struct with `StartSpeculative` method.
- [ ] In `accumulateTurn`: after the stream closes and tool calls are collected, call the speculative executor instead of `executeConcurrent` directly.
- [ ] Document in `speculative.go` that with the Ollama client, speculative execution is equivalent to post-stream concurrent execution since Ollama delivers all tool calls in the Done event.
- [ ] Add a `SpeculativeStartCallback` type that fires when a tool call begins speculatively (for testing).
- [ ] Test: speculative executor starts tool within 50 ms of receiving the Done event (mock stream with instant close).
- [ ] Test: context canceled before Done event → speculative tasks never start.
- [ ] Test: context canceled after Done event but before tools complete → tasks receive cancellation within 200 ms.
- [ ] Test: speculative results match non-speculative results for the same input.

### Step 7 — Benchmarks

Files:
- `internal/agent/concurrent_bench_test.go`

- [ ] Implement `BenchmarkConcurrentFileRead` with 5 fake file read tools (each with 10 ms artificial delay).
- [ ] Assert benchmark duration < `max(individual) + 10%` (i.e., < 11 ms for 5 × 10 ms tools).
- [ ] Implement `BenchmarkSerialFileRead` as baseline for comparison.
- [ ] Implement `BenchmarkPartition1000Calls` as algorithm throughput test.
- [ ] Document benchmark expectations in a comment: "concurrent should be ~5x faster than serial for IO-bound tools".

### Step 8 — Backward Compatibility and Regression Tests

Files:
- `internal/agent/agent_test.go`
- `internal/agent/integration_test.go`

- [ ] Run existing agent tests to confirm no regressions.
- [ ] Test: agent with only non-concurrent tools behaves identically to Phase 14 baseline (same event sequence, same message order).
- [ ] Test: agent with `MaxTurns=1` and one unsafe tool call → normal serial execution.
- [ ] Test: empty tool call list → no batches, no errors.
- [ ] Test: `go test ./internal/agent/...` passes.

### Step 9 — Config Integration

Files:
- `internal/config/config.go` (or config types file)

- [ ] Add `Concurrency.MaxBatchSize int` to config struct (default: 10).
- [ ] Wire config value to errgroup limit in `executeConcurrent`.
- [ ] Test: `MaxBatchSize=1` forces serial execution (all batches become singletons).
- [ ] Test: `MaxBatchSize=5` limits concurrent batch to 5 even when 8 safe calls are available.

### Step 10 — Documentation and Phase Log

Files:
- `docs/PHASE-LOG.md`

- [ ] Update `docs/PHASE-LOG.md` with Phase 15 entry.
- [ ] Document the Ollama speculative execution constraint (all tool calls in Done event).
- [ ] Document concurrent hook call contract.
- [ ] Document concurrency safety classification for each tool.

### Step 11 — Full Test and Check Suite

Required commands:

```sh
go test ./internal/agent/...
go test ./internal/tools/...
go test -race ./internal/agent/...
go test -race ./internal/tools/...
go test -fuzz=FuzzPartition ./internal/agent/ -fuzztime=30s
go test -bench=. ./internal/agent/
tools/check-allowed-deps.sh
tools/check-network-policy.sh
```

## Acceptance Criteria

- [ ] `internal/agent/partition.go` implements the greedy left-to-right partition algorithm.
- [ ] Partition property test: for all input permutations (N+M <= 8 calls), all calls appear exactly once in output in original order.
- [ ] Partition property test: no batch contains a mix of safe and unsafe calls.
- [ ] `internal/agent/speculative.go` implements the speculative executor.
- [ ] Speculative executor starts tool within 50 ms of receiving the Done event (measured in test with mock stream).
- [ ] `executeConcurrent` replaces `executeToolCalls` in `tools.go`.
- [ ] 5 concurrent FileRead calls complete in `max(individual) ± 10%`, not `sum(individual)` (benchmark verified).
- [ ] Submission-order yield: `ToolUseResult` events and `llm.Message` results arrive in call order, not completion order.
- [ ] `IsConcurrencySafe=false` tool is never placed in a batch with other tools.
- [ ] Partition is purely based on `IsConcurrencySafe` and `IsDestructive` flags; no model name, tool name, or argument special-casing.
- [ ] `errgroup.SetLimit(10)` (or config override) caps concurrent goroutines within a batch.
- [ ] Context cancellation aborts all goroutines in a concurrent batch within 200 ms.
- [ ] No goroutine leaks after batch cancellation (`go test -race` passes, goroutine count stable).
- [ ] Serial tool execution (all `IsConcurrencySafe=false`) produces identical event sequence to Phase 14 behavior.
- [ ] `go test -race ./internal/agent/...` passes with zero races.
- [ ] `go test ./internal/tools/...` passes after interface extension.
- [ ] All existing tool tests pass (no regression from interface extension).
- [ ] `golang.org/x/sync` is in `go.mod` and `tools/allowed-deps.txt`.
- [ ] `tools/check-allowed-deps.sh` passes.
- [ ] `tools/check-network-policy.sh` passes.
- [ ] Fuzz test `FuzzPartition` finds no ordering violations in 30 s.
- [ ] `docs/PHASE-LOG.md` has a Phase 15 entry with Ollama constraint documented.

## Forbidden

- Model name or tool name special-casing in the partition algorithm.
- Emitting `ToolUseResult` events out of submission order to the main events channel.
- Modifying `llm.Client` or the streaming layer to add concurrency.
- Running permission prompt callbacks (`PromptFunc`) concurrently if they have internal TUI state (serialize if uncertain).
- Adding per-tool retry within concurrent batches (existing retry.go constraints from Phase 2 still apply).
- Skipping race detector in any Phase 15 test.
- Using `go test -count=1 -timeout=...` to paper over intermittent races.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---:|---|
| Ollama delivers all tool calls in Done event, limiting speculative benefit | Medium | Document constraint clearly; structure code for future incremental streaming; concurrent batch execution still provides speedup for multi-read turns. |
| Permission prompt (blocking TUI callback) called concurrently | High | Detect if PromptFunc is provided and serialize permission resolution within a batch; document in Phase 15. |
| Tool events emitted out of order observed by TUI | High | `OrderedEventEmitter` collects events per goroutine and re-emits in submission order after batch completes. |
| Data race in errgroup goroutines | High | Mandatory race detector; per-goroutine result slots indexed by call position (no shared slice writes). |
| Interface extension breaks existing tool implementations | Medium | Use optional type assertion (`ConcurrencySafeChecker`) rather than required interface extension; default to serial. |
| Benchmark flakiness on CI | Low | Use artificial delays in benchmark tools rather than real file I/O; document expected ratio not absolute time. |
| Context cancellation leaves orphaned goroutines | High | `errgroup.WithContext` propagates cancellation to all goroutines; test goroutine count stability. |

## Phase Log Template

When implementation finishes, append a Phase 15 entry to `docs/PHASE-LOG.md` with:

- objective;
- files created/updated;
- dependencies added and allowlist status;
- tests/benchmarks/checks run;
- benchmark results (concurrent vs serial ratio);
- design decisions (optional interface extension, event serialization approach, Ollama streaming constraint);
- known constraints and deferred work (per-tool timeout, MCP concurrency opt-in, incremental streaming);
- exit gate status.

## Exit Gate

Phase 15 is complete only when:

- all acceptance criteria above are met;
- `go test -race ./internal/agent/...` passes with zero races;
- benchmark confirms concurrent FileRead batch is at least 3× faster than serial for 5 calls with equal individual durations;
- property and fuzz tests pass;
- serial tool execution regression tests pass;
- the phase log records the implementation and any deviations from this plan.

## Implementation Reconciliation (2026-05-08)

### Delivered

- Concurrent/serial partitioning is implemented in `internal/agent/partition.go`.
- Property + fuzz coverage is implemented in `internal/agent/partition_test.go`:
  - `TestPartitionPropertiesRandom`
  - `FuzzPartition`
  - `BenchmarkPartition1000Calls`
- Speculative/concurrent execution is implemented in `internal/agent/speculative.go` using `errgroup.SetLimit(...)`.
- Tool-call ordering is preserved in submission order for:
  - emitted `ToolUseStart` events,
  - emitted `ToolUseResult` events,
  - returned `llm.RoleTool` messages.
- Concurrency cap is configurable via `concurrency.max_batch_size` in config and wired to `agent.Config.MaxConcurrentTools` (default 10):
  - `internal/config/config.go`
  - `internal/config/defaults.go`
  - `internal/config/loader.go`
  - `internal/bootstrap/state.go`
  - `internal/cli/repl.go`
  - `internal/cli/print.go`
- `SkillTool` classification is conservative (`IsConcurrencySafe=false`, `IsDestructive=true`).
- Explicit concurrent-vs-serial file-read style benchmarks are present in `internal/agent/concurrent_bench_test.go`:
  - `BenchmarkConcurrentFileRead`
  - `BenchmarkSerialFileRead`

### Measured Results

From `go test -bench=. ./internal/agent/` on 2026-05-08:

- `BenchmarkConcurrentFileRead`: `10.91 ms/op`
- `BenchmarkSerialFileRead`: `54.13 ms/op`

Concurrent is ~4.96x faster than serial for 5 equal-duration read-like calls.

### Verification Commands Run

- `go test ./internal/agent/...`
- `go test ./internal/tools/...`
- `go test -race ./internal/agent/...`
- `go test -race ./internal/tools/...`
- `go test -fuzz=FuzzPartition ./internal/agent/ -fuzztime=30s`
- `go test -bench=. ./internal/agent/`
- `tools/check-allowed-deps.sh`
- `tools/check-network-policy.sh`

All passed.

### Implemented-But-Different Notes

- The plan proposed a dedicated `internal/agent/eventsync.go` ordered emitter. Implementation keeps ordering guarantees directly inside `internal/agent/speculative.go` by running calls concurrently, then emitting/assembling results in index order.
- A separate `SpeculativeStartCallback` hook was not added; existing tests assert ordering and concurrency semantics directly at executor level.
- The Ollama constraint remains: tool calls arrive at stream `Done`, so speculative start is effectively immediate post-stream concurrent execution for the current provider.
