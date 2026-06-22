# Phase 16 Detailed Plan - Observability and Metrics

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
- `docs/PHASE-15-DETAILED-PLAN.md`
- `book/ch08-security.md`
- `book/ch16-observability.md`
- `.codex/agent-context/learnings-memory.md`

## Goal

Phase 16 adds production-grade observability using the decorator pattern. Wrappers around stable interfaces (`llm.Client`, `tools.Tool`, and the agent runner) capture timing, counts, and errors without scattering log calls throughout business logic. An optional OpenTelemetry export path is available when the user explicitly enables it with two environment variables. Metrics and logs must never expose prompts, file contents, model responses, or secrets.

Deliverables:

- `internal/observability/` — LLM decorator, Tool decorator, Agent runner decorator, in-memory meter, and OTEL bridge.
- `internal/logging/redact.go` — secret redaction helpers.
- `internal/logging/attrs.go` — safe structured log attributes.
- In-memory metric aggregates available for the `/cost` slash command and TUI status bar.
- OTEL traces, metrics, and optional log bridge wired behind `NANDOCODEGO_TELEMETRY=1` plus `NANDOCODEGO_OTEL_ENDPOINT=<url>`.
- `doctor` command updated to report telemetry status.
- Tests: decorator interface compliance, metric capture accuracy, redaction correctness, OTEL opt-in gating.

## Definition Of Success

The Phase 16 exit gate is a dual automated/manual flow:

1. Automated:
   - Run a full REPL session without OTEL variables set.
   - Confirm: no outbound HTTP except to the configured Ollama endpoint (verified by network policy check or mock transport).
   - Confirm: `Redact("sk-abc123xyz")` returns `"sk-***"`.
   - Confirm: LLM decorator captures first-token latency within 5 ms of ground truth.
   - Confirm: `/cost` shows token totals matching `agent.Usage`.

2. Manual:
   - Set `NANDOCODEGO_TELEMETRY=1` and `NANDOCODEGO_OTEL_ENDPOINT=http://localhost:4318`.
   - Start a local OTEL collector (e.g., `otelcol` or Jaeger).
   - Run one REPL turn.
   - Verify spans appear in the collector for the agent loop, LLM call, and at least one tool execution.

## Baseline Analysis From Implemented Phases

### Phase 0 — Security and Supply Chain

Implemented:

- dependency allowlist
- network policy checker
- no-secrets policy for logs, memory, telemetry, and test fixtures

Phase 16 implications:

- OTEL client libraries (`go.opentelemetry.io/otel`, `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp`, `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc`, `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp`) must be added to the allowlist with justification.
- OTEL adds transitive dependencies. Run `go mod tidy` and re-check the full dependency graph.
- OTEL telemetry must not send to Anthropic, Google, or any non-user-configured endpoint. Network policy checker must verify no hardcoded telemetry endpoint exists in OTEL configuration code.
- `NANDOCODEGO_TELEMETRY=0` (default) means zero OTEL initialization and zero outbound OTEL HTTP connections.
- Redaction helpers must be tested with a corpus of known secret patterns. Redaction must not log the secret being redacted.

### Phase 1 — CLI, Paths, Logging, Scaffold

Implemented:

- `cmd/nandocodego`
- `internal/logging`
- `internal/paths`
- `doctor` subcommand

Phase 16 implications:

- `internal/logging` already exists; add `redact.go` and `attrs.go` to the same package.
- `doctor` command gains a `telemetry:` line: "disabled" or "otel → <endpoint>".
- Logging configuration (`slog.Handler`) is set up in Phase 1. Phase 16 can bridge slog to the OTEL log exporter optionally without replacing the existing slog setup.
- Keep the existing slog handler. The OTEL bridge is additive and activated only when telemetry is enabled.

### Phase 2 — LLM Client

Implemented:

- `llm.Client` interface: `Chat`, `Embed`, `ListModels`, `PullModel`.
- `llm.StreamEvent` with `PromptEvalCount`, `EvalCount`, `TotalDuration`.
- Retry and watchdog helpers.

Phase 16 implications:

- The LLM decorator wraps all four `llm.Client` methods.
- First-token latency is measured as the time from `Chat()` call to the first `StreamEvent` with non-empty content or non-zero counts.
- Stream duration is measured from `Chat()` call to channel close.
- Watchdog timeouts are counted as a separate metric (not conflated with general LLM errors).
- The decorator must not buffer stream events; it only timestamps their passage.
- `Embed`, `ListModels`, `PullModel` capture call count and duration; no streaming metrics.

### Phase 3 — Tool Interface and Starter Tools

Implemented:

- `tools.Tool` interface.
- `Bash`, `FileRead`, `FileWrite`.
- Built-in registry.

Phase 16 implications:

- The Tool decorator wraps individual `tools.Tool` instances at construction time.
- Alternatively, the registry can apply the decorator transparently to all registered tools.
- Phase 16 uses per-tool-name metric labels (not per-call-ID) to keep cardinality bounded.
- Tool decorator captures: call count by name, duration, error count, permission decision counts (via a hook into the permission result).
- The decorator must not log tool arguments or results (these may contain file contents or secrets).
- `CheckPermissions` is not a method on `tools.Tool` in this repo; permission metrics are captured by a separate permission observer rather than the tool decorator.

### Phase 4 — Agent Loop

Implemented:

- `agent.Agent.Run(ctx, Input) <-chan Event`
- `agent.Terminal` event with `Usage` and `Conversation`.

Phase 16 implications:

- The Agent runner decorator wraps the `Run` method.
- It starts a span on `Run` entry and ends it on `Terminal` event.
- It increments `agent.turn_count`, `agent.tool_call_count`, and `agent.token_total` from `Terminal.Usage`.
- The decorator must forward all events unchanged; it does not filter or modify the event channel.
- `agent.Usage` already captures `PromptEvalCount`, `EvalCount`, `Turns`, `ToolCalls`, and `TotalDuration`. These are the ground-truth token totals for the `/cost` command.

### Phase 5 — Permission System

Implemented:

- `permissions.Resolve` — central resolver returning `PermissionResult`.
- `permissions.Decision` enum: Allow, Deny, RequiresPrompt.

Phase 16 implications:

- Add a `PermissionObserver` interface (or functional callback) that the resolver calls on each decision.
- Metrics: `permission.decisions` by `mode`, `stage`, `tool_name`, `outcome` (allow/deny/prompt).
- The observer must not log the `input` or `target` of the tool call.
- Phase 16 wires the observer at the REPL composition root alongside the decorators.

### Phase 6 — State Layer

Implemented:

- `state.Store[state.App]`
- `state.App.Tasks`

Phase 16 implications:

- No state changes required for Phase 16.
- In-memory metric aggregates are stored in the `observability.Meter` struct, not in `state.App`. The `/cost` command reads from the meter directly.
- Meter is passed to the TUI at construction time, similar to how the supervisor is passed.

### Phase 7 — Bubble Tea TUI and REPL

Implemented:

- TUI, REPL, status bar, transcript, slash commands.

Phase 16 implications:

- The `/cost` slash command is added or enhanced to read from the in-memory meter.
- The status bar can show `tokens: N` when the meter has data.
- No new modal or dedicated observability pane in Phase 16; text rendering in transcript is sufficient for `/cost`.
- TUI must not import `go.opentelemetry.io/otel` packages directly; only the observability package uses OTEL.

### Phase 8 — Memory

Implemented: `internal/memory` package.

Phase 16 implications:

- Memory recall and extraction call `llm.Client`. These calls flow through the LLM decorator and are captured in the LLM metrics with a label or attribute indicating the caller context (e.g., `caller=memory_recall`).
- Phase 8 deferred telemetry to Phase 16. Phase 16 adds that coverage now.

### Phase 9 — Hooks

Implemented: snapshot hooks, `PreToolUse`, `PostToolUse`.

Phase 16 implications:

- Hook execution duration can be captured by the tool decorator or a separate hook observer.
- Phase 16 adds `hook.decisions` metric counting hook outcomes by event type and decision.

### Phase 10 — MCP Tools

Implemented: MCP tool integration.

Phase 16 implications:

- MCP tool calls flow through the Tool decorator; tool name label distinguishes MCP tools from builtin tools.
- No MCP-specific metrics in Phase 16.

### Phase 11 — AgentTool

Implemented: `AgentTool` for sub-agent spawning.

Phase 16 implications:

- Sub-agent runs initiated by `AgentTool` are wrapped by the Agent runner decorator.
- Nested spans: sub-agent span is a child of the parent agent span.
- Token usage from sub-agents accumulates separately in the meter under `agent.sub_turns`.

### Phase 12 — SkillTool

Implemented: `SkillTool`.

Phase 16 implications:

- Skill executions are captured by the Tool decorator like any other tool call.

### Phase 13 — Slash Commands, Config, --print Mode

Implemented: koanf config, slash commands, `--print` mode.

Phase 16 implications:

- Telemetry configuration (`NANDOCODEGO_TELEMETRY`, `NANDOCODEGO_OTEL_ENDPOINT`) is read from environment variables at startup, not from the config file. This avoids complexity around config reload races.
- The `/cost` slash command is added in Phase 16 (or enhanced if it was stubbed in Phase 13).
- `--print` mode works with in-memory metrics; no OTEL export is required for `--print`.

### Phase 14 — Tasks

Implemented: task supervisor, JSONL output, task tools.

Phase 16 implications:

- Task lifecycle events (created, started, completed, failed, killed) are captured as metric increments in the observability package.
- Task duration is captured from `RunningTask.StartedAt` to `CompletedTask.FinishedAt`.
- Task output files are local; the observability package does not read them.

### Phase 15 — Concurrency and Speculative Execution

Implemented: partition algorithm, concurrent tool batches, `executeConcurrent`.

Phase 16 implications:

- Batch size is captured as a metric histogram: `tool.batch_size`.
- Concurrent tool execution duration (batch wall time) is captured separately from serial tool duration.
- Per-tool duration inside a concurrent batch is still captured by the Tool decorator.
- The speculative executor logs start/abort at DEBUG through the safe logging helpers.

## Evaluation Of The Original Phase 16 Concept

The original concept is correct at the product level:

- decorator pattern
- no prompt/content logging
- in-memory meter for `/cost`
- OTEL opt-in
- redaction helpers
- structured log attributes

It needs additional detail for this repo:

- It does not specify how the LLM decorator captures first-token latency when the Ollama client delivers all tool calls in the Done event.
- It does not specify how the permission observer is wired without modifying the central resolver's signature.
- It does not define how the OTEL bridge integrates with the existing `slog` setup.
- It does not specify which OTEL exporters are included (HTTP vs gRPC; metrics vs traces vs logs).
- It does not define how tool decorators are applied: per-tool at construction time or via registry-wide wrapping.
- It does not specify how in-memory metric aggregates are exposed to the TUI without creating a circular import.
- It does not account for the Phase 15 concurrency changes that add batch-level timing.

## Final Phase 16 Scope

In scope:

- `internal/observability/llm.go` — LLM client decorator.
- `internal/observability/tool.go` — Tool decorator.
- `internal/observability/agent.go` — Agent runner decorator.
- `internal/observability/metrics.go` — in-memory meter and OTEL bridge.
- `internal/observability/permission.go` — permission observer.
- `internal/logging/redact.go` — secret redaction helpers.
- `internal/logging/attrs.go` — safe structured log attributes.
- `/cost` slash command reading from in-memory meter.
- `doctor` telemetry status line.
- Status bar token count.
- OTEL traces and metrics (HTTP exporter only; gRPC is optional stretch goal).
- OTEL slog bridge (optional; activated only when OTEL is enabled).
- Tests: decorator compliance, metric accuracy, redaction, OTEL gating.
- Phase log update.

Out of scope:

- gRPC OTEL exporter (HTTP only in Phase 16).
- Profiling (pprof) integration.
- Log sampling or log rate limiting.
- OTEL baggage propagation.
- Remote metric scraping (Prometheus).
- Dashboard or visualization.
- Structured error aggregation service.
- Automated alerting.

## Target User Experience

### Default (No OTEL)

Normal REPL session: no behavior change. All metrics captured in memory. After a few turns:

```
/cost
Session usage:
  Prompt tokens:     1,240
  Completion tokens:   380
  Total tokens:      1,620
  LLM calls:            4
  Tool calls:           7
  Turns:                3
  Session duration:  42.3s
```

### OTEL Enabled

```sh
NANDOCODEGO_TELEMETRY=1 NANDOCODEGO_OTEL_ENDPOINT=http://localhost:4318 \
  go run ./cmd/nandocodego --model qwen3
```

Spans appear in the collector:

```
nandocodego.agent.run (42.3s)
  └── nandocodego.llm.chat (1.2s)
  └── nandocodego.tool.call name=FileRead (0.8ms)
  └── nandocodego.tool.call name=FileRead (0.7ms)
  └── nandocodego.tool.call name=Bash (0.3s)
  └── nandocodego.llm.chat (0.9s)
```

### OTEL Missing Endpoint

```sh
NANDOCODEGO_TELEMETRY=1 go run ./cmd/nandocodego
```

Output includes a warning at startup:

```
WARN telemetry enabled but NANDOCODEGO_OTEL_ENDPOINT not set; OTEL export disabled
```

No crash. In-memory metrics still captured.

### Doctor Output

```
$ nandocodego doctor
model:      qwen3
endpoint:   http://localhost:11434
telemetry:  otel → http://localhost:4318
memory:     enabled (42 entries)
tasks:      0 running
```

## Architecture

### Decorator Interfaces

```go
// internal/observability/agent.go

// AgentRunner is the interface both the real agent and decorators implement.
type AgentRunner interface {
    Run(ctx context.Context, in agent.Input) <-chan agent.Event
}

// NewAgentRunnerDecorator wraps an AgentRunner with metrics and tracing.
func NewAgentRunnerDecorator(inner AgentRunner, meter *Meter, logger *slog.Logger) AgentRunner
```

```go
// internal/observability/llm.go

// NewLLMClientDecorator wraps an llm.Client with metrics and tracing.
func NewLLMClientDecorator(inner llm.Client, meter *Meter, logger *slog.Logger) llm.Client
```

```go
// internal/observability/tool.go

// NewToolDecorator wraps a tools.Tool with metrics and tracing.
// The returned Tool implements tools.Tool fully, including IsConcurrencySafe and IsDestructive.
func NewToolDecorator(inner tools.Tool, meter *Meter, logger *slog.Logger) tools.Tool
```

### In-Memory Meter

```go
// internal/observability/metrics.go

package observability

import (
    "sync"
    "sync/atomic"
    "time"
)

// Meter collects in-memory metrics. It is safe for concurrent use.
type Meter struct {
    mu sync.RWMutex

    // LLM metrics
    LLMCalls          int64
    LLMErrors         int64
    LLMWatchdogHits   int64
    FirstTokenLatency DurationHistogram
    StreamDuration    DurationHistogram
    PromptTokens      int64
    CompletionTokens  int64

    // Tool metrics
    ToolCalls    map[string]int64 // by tool name
    ToolErrors   map[string]int64
    ToolDuration map[string]DurationHistogram

    // Agent metrics
    AgentTurns       int64
    AgentToolCalls   int64
    AgentSessions    int64
    TerminalReasons  map[string]int64

    // Permission metrics
    PermissionDecisions map[string]int64 // key: "mode:stage:outcome"

    // Task metrics (Phase 14)
    TaskCreated   map[string]int64 // by kind
    TaskCompleted map[string]int64
    TaskFailed    map[string]int64
    TaskKilled    map[string]int64

    // Batch metrics (Phase 15)
    BatchSizes DurationHistogram // reusing histogram for size distribution

    otelBridge OTELBridge
}

// DurationHistogram tracks min, max, mean, and p99 duration samples.
type DurationHistogram struct {
    Count int64
    Sum   time.Duration
    Min   time.Duration
    Max   time.Duration
}

// OTELBridge is the interface for forwarding metrics to an OTEL exporter.
// A no-op implementation is used when OTEL is disabled.
type OTELBridge interface {
    RecordLLMCall(ctx context.Context, model string, latency, duration time.Duration, promptTokens, completionTokens int64, err error)
    RecordToolCall(ctx context.Context, toolName string, duration time.Duration, err error)
    RecordAgentTurn(ctx context.Context, reason string, turns, toolCalls int64, tokens int64)
    RecordPermissionDecision(ctx context.Context, mode, outcome string)
    Shutdown(ctx context.Context) error
}

// NoopBridge is the zero-allocation default when OTEL is disabled.
type NoopBridge struct{}

func (NoopBridge) RecordLLMCall(context.Context, string, time.Duration, time.Duration, int64, int64, error) {}
func (NoopBridge) RecordToolCall(context.Context, string, time.Duration, error)                             {}
func (NoopBridge) RecordAgentTurn(context.Context, string, int64, int64, int64)                             {}
func (NoopBridge) RecordPermissionDecision(context.Context, string, string)                                 {}
func (NoopBridge) Shutdown(context.Context) error                                                           { return nil }
```

### OTEL Bridge Implementation

```go
// internal/observability/otelbridge.go

// RealOTELBridge implements OTELBridge using go.opentelemetry.io/otel.
type RealOTELBridge struct {
    tracer   trace.Tracer
    meter    otelmetric.Meter
    shutdown func(context.Context) error
}

// NewOTELBridge initialises the OTEL provider with an OTLP HTTP exporter.
// It returns a NoopBridge if the endpoint is empty or telemetry is disabled.
func NewOTELBridge(ctx context.Context, endpoint string) (OTELBridge, error)
```

### Redaction Helpers

```go
// internal/logging/redact.go

package logging

import "regexp"

var secretPatterns = []*regexp.Regexp{
    regexp.MustCompile(`(?i)(sk|key|token|secret|password|credential|auth)[_-]?[a-zA-Z0-9]{8,}`),
    regexp.MustCompile(`(?i)bearer\s+[a-zA-Z0-9._-]+`),
    regexp.MustCompile(`[a-zA-Z0-9]{20,}`), // long opaque strings
}

// Redact replaces recognized secret patterns in s with "<kind>-***".
// Redact("sk-abc123xyz") → "sk-***"
// Redact("Bearer eyJhbGc...") → "Bearer ***"
// It is safe to call on non-secret strings; non-matching input is returned unchanged.
func Redact(s string) string

// RedactAttr returns an slog.Attr whose value is redacted.
func RedactAttr(key, val string) slog.Attr
```

```go
// internal/logging/attrs.go

package logging

import "log/slog"

// SafeAttr returns an slog.Attr that is safe to include in structured logs.
// It redacts the value before creating the attribute.
func SafeAttr(key, val string) slog.Attr

// OpAttr returns an slog.Attr for the operation name.
func OpAttr(op string) slog.Attr

// DurationAttr returns an slog.Attr for a duration value.
func DurationAttr(key string, d time.Duration) slog.Attr

// ErrorClassAttr returns an slog.Attr for an error class string.
// Error messages themselves must not be logged; only their class.
func ErrorClassAttr(class string) slog.Attr

// RetryableAttr returns an slog.Attr for a boolean retryable flag.
func RetryableAttr(retryable bool) slog.Attr
```

## Implementation Plan

### Step 1 — Redaction Helpers

Files:
- `internal/logging/redact.go`
- `internal/logging/redact_test.go`
- `internal/logging/attrs.go`
- `internal/logging/attrs_test.go`

- [ ] Implement `Redact(s string) string` with a compiled pattern set.
- [ ] Pattern: `sk-<alphanumeric>` → `sk-***`.
- [ ] Pattern: `Bearer <token>` → `Bearer ***`.
- [ ] Pattern: `password=<value>` → `password=***`.
- [ ] Pattern: `token=<value>` → `token=***`.
- [ ] Pattern: long opaque alphanumeric strings (≥ 20 chars) → `<redacted>`.
- [ ] Non-secret strings are returned unchanged.
- [ ] `Redact` must not log internally.
- [ ] Implement `RedactAttr`, `SafeAttr`, `OpAttr`, `DurationAttr`, `ErrorClassAttr`, `RetryableAttr`.
- [ ] Test: `Redact("sk-abc123xyz")` → `"sk-***"`.
- [ ] Test: `Redact("Bearer eyJhbGciOiJSUzI1NiJ9.abc")` → `"Bearer ***"`.
- [ ] Test: `Redact("hello world")` → `"hello world"` (unchanged).
- [ ] Test: `Redact("")` → `""`.
- [ ] Test: `Redact` is safe to call concurrently (compiled patterns are read-only).
- [ ] Test: `SafeAttr("api_key", "sk-secret")` produces attr with value `"sk-***"`.
- [ ] Test: `ErrorClassAttr` does not include the error message text.
- [ ] Benchmark: `BenchmarkRedact` on a 1 KB string completes under 10 µs.

### Step 2 — In-Memory Meter

Files:
- `internal/observability/metrics.go`
- `internal/observability/metrics_test.go`

- [ ] Implement `Meter` with all metric fields.
- [ ] Implement `DurationHistogram.Record(d time.Duration)` updating count, sum, min, max atomically.
- [ ] Implement `Meter.Snapshot() MeterSnapshot` returning a copy safe for reading.
- [ ] Implement `NoopBridge` satisfying `OTELBridge`.
- [ ] Thread-safety: use `sync/atomic` for counter increments; `sync.Mutex` for map updates.
- [ ] Test: concurrent `RecordToolCall` from 100 goroutines → final count == 100.
- [ ] Test: `DurationHistogram` min/max are correct after 10 samples.
- [ ] Test: `Meter.Snapshot()` returns a deep copy (original not mutated after snapshot).
- [ ] Test: `NoopBridge` methods do not panic when called.
- [ ] Test: Meter with nil OTEL bridge uses NoopBridge without panic.

### Step 3 — LLM Client Decorator

Files:
- `internal/observability/llm.go`
- `internal/observability/llm_test.go`

- [ ] Implement `llmDecorator` wrapping `llm.Client`.
- [ ] `Chat`: record start time; forward call; record time-to-first-token as stream event arrives; record stream close time; increment `LLMCalls`; on error increment `LLMErrors`.
- [ ] Detect first token: intercept the returned channel; proxy events through; record `FirstTokenLatency` on first non-empty content event.
- [ ] `Embed`: record call count and duration.
- [ ] `ListModels`: record call count and duration.
- [ ] `PullModel`: record call count and duration.
- [ ] Do not log request bodies, prompts, or response content.
- [ ] Log at DEBUG: operation name, duration, token counts (from Done event), error class (not message).
- [ ] `NewLLMClientDecorator` returns `llm.Client`; compile error if interface not fully implemented.
- [ ] Test: `Chat` increments `LLMCalls` by 1 per call.
- [ ] Test: `Chat` captures `FirstTokenLatency` within 5 ms of actual first event (using fake stream with timed events).
- [ ] Test: `Chat` on error increments `LLMErrors` and does not panic.
- [ ] Test: decorator forwards all stream events unchanged (none dropped).
- [ ] Test: decorator does not buffer events (each event forwarded before next arrives).
- [ ] Test: decorator implements full `llm.Client` interface (all methods).
- [ ] Test: `go test -race ./internal/observability/...` passes.

### Step 4 — Tool Decorator

Files:
- `internal/observability/tool.go`
- `internal/observability/tool_test.go`

- [ ] Implement `toolDecorator` wrapping `tools.Tool`.
- [ ] `Call`: record start time; forward call; record duration; increment `ToolCalls[name]`; on error increment `ToolErrors[name]`.
- [ ] Forward `IsConcurrencySafe` and `IsDestructive` from inner tool.
- [ ] Forward `IsEnabled`, `Name`, `Description`, `InputSchema`, `UnmarshalInput` unchanged.
- [ ] Do not log tool input, tool result, or tool error message.
- [ ] Log at DEBUG: tool name, duration, error class (not message).
- [ ] `NewToolDecorator` returns `tools.Tool`; compile error if interface not fully implemented.
- [ ] Test: `Call` increments `ToolCalls["FileRead"]` by 1.
- [ ] Test: `Call` on error increments `ToolErrors["Bash"]`.
- [ ] Test: `Call` captures duration within 1 ms of ground truth.
- [ ] Test: decorator forwards `IsConcurrencySafe` from inner tool.
- [ ] Test: decorator implements full `tools.Tool` interface (all methods including Phase 15 additions).
- [ ] Test: `go test -race ./internal/observability/...` passes.

### Step 5 — Agent Runner Decorator

Files:
- `internal/observability/agent.go`
- `internal/observability/agent_test.go`

- [ ] Define `AgentRunner` interface: `Run(ctx, agent.Input) <-chan agent.Event`.
- [ ] Implement `agentRunnerDecorator` wrapping `AgentRunner`.
- [ ] `Run`: start span (or record start time); forward to inner; drain events; on `agent.Terminal` update meter with `Usage`; end span.
- [ ] Increment `AgentTurns`, `AgentToolCalls`, `PromptTokens`, `CompletionTokens` from `Terminal.Usage`.
- [ ] Increment `TerminalReasons[reason]` on `Terminal`.
- [ ] Do not log `Terminal.Conversation`, `Input.SystemPrompt`, or `Input.Messages`.
- [ ] Log at INFO: session start and end, terminal reason, turn count (no content).
- [ ] Test: decorator increments `AgentTurns` by `Terminal.Usage.Turns`.
- [ ] Test: decorator increments `PromptTokens` from `Terminal.Usage.PromptEvalCount`.
- [ ] Test: decorator increments `TerminalReasons["completed"]` on TerminalCompleted.
- [ ] Test: all events are forwarded unchanged (none dropped).
- [ ] Test: decorator implements `AgentRunner` interface.
- [ ] Test: cancellation before Terminal event → decorator cleans up span/timer without panic.

### Step 6 — Permission Observer

Files:
- `internal/observability/permission.go`
- `internal/observability/permission_test.go`
- `internal/permissions/resolver.go` (modified to accept observer)

- [ ] Define `PermissionObserver` interface or functional type: `func(mode, stage, toolName, outcome string)`.
- [ ] Modify `permissions.Resolve` to accept an optional observer (nil = no-op).
- [ ] Observer is called after the resolution decision with mode, tool name, and outcome (not input).
- [ ] Implement `observability.NewPermissionObserver(meter *Meter) PermissionObserver`.
- [ ] Increment `PermissionDecisions[key]` where key = `"<mode>:<outcome>"`.
- [ ] Test: observer increments count for allow decision.
- [ ] Test: observer increments count for deny decision.
- [ ] Test: nil observer does not panic.
- [ ] Test: observer is not called with tool input or tool arguments.

### Step 7 — OTEL Bridge

Files:
- `internal/observability/otelbridge.go`
- `internal/observability/otelbridge_test.go`
- `go.mod`
- `go.sum`
- `tools/allowed-deps.txt`

- [ ] Add OTEL dependencies: `go.opentelemetry.io/otel`, `go.opentelemetry.io/otel/trace`, `go.opentelemetry.io/otel/metric`, `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp`, `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp`, `go.opentelemetry.io/otel/sdk`.
- [ ] Add all OTEL dependencies to `tools/allowed-deps.txt`.
- [ ] Implement `NewOTELBridge(ctx context.Context, endpoint string) (OTELBridge, error)`.
- [ ] Return `NoopBridge` if endpoint is empty.
- [ ] Configure OTLP HTTP exporter pointing to `endpoint`.
- [ ] Register trace and metric providers with the global OTEL registry.
- [ ] Instrument spans: `nandocodego.agent.run`, `nandocodego.llm.chat`, `nandocodego.tool.call`.
- [ ] Instrument metrics: `nandocodego.llm.calls` (counter), `nandocodego.llm.first_token_latency` (histogram), `nandocodego.tool.calls` (counter by name), `nandocodego.agent.tokens` (counter by type).
- [ ] `Shutdown(ctx)` flushes and shuts down providers.
- [ ] Test: `NewOTELBridge` with empty endpoint returns `NoopBridge`.
- [ ] Test: `NewOTELBridge` with endpoint returns non-noop bridge.
- [ ] Test: `RecordLLMCall` does not panic when bridge is a NoopBridge.
- [ ] Test: `RealOTELBridge.Shutdown` can be called multiple times without panic.
- [ ] Test: zero OTEL spans created when `NANDOCODEGO_TELEMETRY` is not set (integration test with span counter).

### Step 8 — Telemetry Initializer

Files:
- `internal/observability/init.go`
- `internal/cli/repl.go` (modified)

- [ ] Implement `InitTelemetry(ctx context.Context) (*Meter, func(context.Context) error, error)`.
- [ ] Read `NANDOCODEGO_TELEMETRY` and `NANDOCODEGO_OTEL_ENDPOINT` from environment.
- [ ] If `NANDOCODEGO_TELEMETRY` is unset or `"0"`: use `NoopBridge`; no OTEL initialization.
- [ ] If `NANDOCODEGO_TELEMETRY=1` but endpoint empty: log warning; use `NoopBridge`.
- [ ] If both set: call `NewOTELBridge`; return shutdown func.
- [ ] In `runREPL`: call `InitTelemetry`; wrap `llm.Client`, tools, and agent runner with decorators.
- [ ] Defer `shutdown(ctx)` at REPL exit.
- [ ] Test: `NANDOCODEGO_TELEMETRY` unset → `NoopBridge` returned.
- [ ] Test: `NANDOCODEGO_TELEMETRY=1` without endpoint → warning logged; `NoopBridge` returned.
- [ ] Test: `NANDOCODEGO_TELEMETRY=1` with endpoint → `RealOTELBridge` returned.
- [ ] Test: `InitTelemetry` called twice does not panic.

### Step 9 — /cost Slash Command

Files:
- `internal/tui/app.go` or slash command handler
- `internal/observability/metrics.go`

- [ ] Implement `/cost` slash command (or enhance stub from Phase 13).
- [ ] Read `Meter.Snapshot()` to build the usage summary.
- [ ] Render: prompt tokens, completion tokens, total tokens, LLM calls, tool calls, turns, session duration.
- [ ] Verify token totals match the last `agent.Terminal.Usage` values (consistency check in test).
- [ ] Test: `/cost` with no activity renders "No usage data yet." or zero values.
- [ ] Test: `/cost` after one turn renders correct token totals.
- [ ] Test: `/cost` does not render prompt bodies or file contents.

### Step 10 — Doctor Update

Files:
- `internal/commands/doctor.go` (or equivalent)

- [ ] Add `telemetry:` line to `doctor` output.
- [ ] If OTEL disabled: `telemetry: disabled`.
- [ ] If OTEL enabled: `telemetry: otel → <endpoint>`.
- [ ] If telemetry env set but endpoint missing: `telemetry: warning (endpoint not configured)`.
- [ ] Test: doctor output contains `telemetry:` line.
- [ ] Test: doctor output does not contain the OTEL endpoint when telemetry is disabled.

### Step 11 — Status Bar Token Count

Files:
- `internal/tui/app.go`

- [ ] Add token count to status bar: `tokens: N` where N is `PromptTokens + CompletionTokens`.
- [ ] Update on each `state.OnChange` event (or subscribe to meter updates).
- [ ] Show `tokens: 0` or hide when no activity.
- [ ] Test: status bar view renders token count correctly.

### Step 12 — Tests, Checks, and Manual Smoke

Required commands:

```sh
go test ./internal/observability/...
go test ./internal/logging/...
go test -race ./internal/observability/...
go test -bench=BenchmarkRedact ./internal/logging/
go test ./internal/tui/...
go test ./internal/cli/...
tools/check-allowed-deps.sh
tools/check-network-policy.sh
```

Manual smoke (no OTEL):

```sh
go run ./cmd/nandocodego --model qwen3 --no-alt-screen
```

Flow:

1. Run one turn.
2. Type `/cost`.
3. Confirm token totals, LLM calls, tool calls match expectations.
4. Run `doctor`.
5. Confirm `telemetry: disabled`.
6. Confirm no outbound requests except to Ollama.

Manual smoke (OTEL):

```sh
# Start a local OTEL collector (e.g., docker run --rm -p 4318:4318 otel/opentelemetry-collector)
NANDOCODEGO_TELEMETRY=1 NANDOCODEGO_OTEL_ENDPOINT=http://localhost:4318 \
  go run ./cmd/nandocodego --model qwen3 --no-alt-screen
```

Flow:

1. Run one turn with a tool call.
2. Exit REPL.
3. Check collector output for spans: `nandocodego.agent.run`, `nandocodego.llm.chat`, `nandocodego.tool.call`.
4. Confirm no span contains prompt body, file content, or tool arguments.

## Acceptance Criteria

- [ ] `internal/observability/` exists with `llm.go`, `tool.go`, `agent.go`, `metrics.go`, `permission.go`.
- [ ] `internal/logging/redact.go` and `internal/logging/attrs.go` exist.
- [ ] `Redact("sk-abc123xyz")` → `"sk-***"`.
- [ ] `Redact("hello world")` → `"hello world"` (unchanged).
- [ ] LLM decorator captures first-token latency to within 5 ms of the actual first stream event.
- [ ] Tool decorator captures call duration within 1 ms of ground truth.
- [ ] Agent runner decorator captures `PromptTokens` and `CompletionTokens` matching `agent.Terminal.Usage`.
- [ ] `/cost` shows token totals matching `agent.Usage` from the last terminal event.
- [ ] `NANDOCODEGO_TELEMETRY=0` (default) → zero OTEL spans created; zero outbound HTTP except Ollama.
- [ ] `NANDOCODEGO_TELEMETRY=1` without `NANDOCODEGO_OTEL_ENDPOINT` → warning logged; no crash; in-memory metrics still captured.
- [ ] `NANDOCODEGO_TELEMETRY=1` with endpoint → OTEL bridge initialized; spans and metrics sent to endpoint.
- [ ] LLM decorator implements full `llm.Client` interface (compile-time check).
- [ ] Tool decorator implements full `tools.Tool` interface including Phase 15 additions (compile-time check).
- [ ] Agent runner decorator implements `AgentRunner` interface (compile-time check).
- [ ] No decorator logs prompt bodies, message content, tool arguments, tool results, or file contents.
- [ ] `go test -race ./internal/observability/...` passes with zero races.
- [ ] Concurrent `RecordToolCall` from 100 goroutines → final count == 100 (thread-safety test).
- [ ] `DurationHistogram` min/max are correct after 10 samples (including edge cases of single sample and all-equal samples).
- [ ] Permission observer does not log tool input or target.
- [ ] Doctor `telemetry:` line reflects actual OTEL configuration.
- [ ] Status bar shows token count updated after each turn.
- [ ] All OTEL dependencies in `go.mod` are in `tools/allowed-deps.txt`.
- [ ] `tools/check-allowed-deps.sh` passes.
- [ ] `tools/check-network-policy.sh` passes (no hardcoded telemetry endpoint).
- [ ] `docs/PHASE-LOG.md` has a Phase 16 entry.
- [ ] Manual OTEL smoke flow produces spans in the collector.

## Forbidden

- Logging prompt bodies, system prompts, message content, tool arguments, tool results, or file contents at any log level.
- Hardcoding any telemetry endpoint (e.g., `api.honeycomb.io`, `ingest.signalsciences.net`).
- Activating OTEL unless both `NANDOCODEGO_TELEMETRY=1` and `NANDOCODEGO_OTEL_ENDPOINT` are set.
- Sending telemetry to Anthropic, Google, or any non-user-configured endpoint.
- Importing `go.opentelemetry.io/otel` packages outside `internal/observability/`.
- Using `panic` in a decorator method (decorators must never be the cause of a crash).
- Adding pprof endpoints, Prometheus scrape endpoints, or any inbound HTTP server.
- Metric label cardinality explosion (e.g., using full file paths or user input as label values).

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---:|---|
| OTEL transitive dependencies add significant build size | Medium | Audit `go mod why` after adding OTEL; document size delta in phase log. |
| LLM decorator introduces channel indirection overhead | Low | Benchmark decorator overhead on stream throughput; target < 1 µs per event. |
| Redaction regex has false positives (redacts non-secret tokens) | Medium | Test with representative non-secret strings; tune patterns; document limitations. |
| Redaction regex misses a new secret pattern | High | Maintain a test corpus; document that redaction is best-effort; never log sensitive fields even with redaction. |
| OTEL bridge initialization fails silently | Medium | Always log OTEL init error at WARN; continue with NoopBridge. |
| Tool decorator breaks `ConcurrencySafeChecker` type assertion | High | Decorator must forward `IsConcurrencySafe` and `IsDestructive` from inner tool; test type assertions on wrapped tools. |
| Permission observer receives tool input containing secrets | High | Observer callback signature must not include input or target; pass only mode, tool name, outcome. |
| OTEL SDK adds goroutine leak | Medium | Call `Shutdown` at REPL exit with a timeout; add goroutine count stability test. |

## Phase Log Template

When implementation finishes, append a Phase 16 entry to `docs/PHASE-LOG.md` with:

- objective;
- files created/updated;
- dependencies added and allowlist status;
- tests/benchmarks/checks run;
- manual smoke results (no-OTEL and OTEL);
- design decisions (NoopBridge default, OTEL HTTP only, permission observer contract, decorator interface approach);
- known constraints and deferred work (gRPC exporter, Prometheus scrape, pprof);
- exit gate status.

## Exit Gate

Phase 16 is complete only when:

- all acceptance criteria above are met;
- `go test -race ./internal/observability/...` passes with zero races;
- a full REPL session without OTEL produces no outbound HTTP except to Ollama (verified by mock transport or network policy check);
- `/cost` output matches `agent.Usage` for a test session;
- the OTEL manual smoke flow produces at least three spans in a local collector;
- the phase log records the implementation and any deviations from this plan.

## Implementation Reconciliation (2026-05-08)

### Delivered

- Added new observability package:
  - `internal/observability/metrics.go` (thread-safe in-memory meter)
  - `internal/observability/llm.go` (LLM client decorator)
  - `internal/observability/tool.go` (tool decorator + registry wrapper)
  - `internal/observability/agent.go` (runner decorator)
  - `internal/observability/permission.go` (permission observer integration)
  - `internal/observability/bridge.go` (telemetry env gating + bridge interface)
- Added log-safety helpers:
  - `internal/logging/redact.go`
  - `internal/logging/attrs.go`
- Wired permission decision observation into resolver:
  - `internal/permissions/resolver.go` (`Request.Observer`, final-decision callback)
- Wired Phase 15 batch metrics callbacks:
  - `agent.Config.ToolBatchObserver`
  - `internal/agent/speculative.go` batch timing/size callback
- Wired observability at composition root:
  - `internal/cli/repl.go`
  - `internal/cli/print.go`
- `/cost` upgraded to meter-backed session summary:
  - `internal/commands/registry.go`
- TUI status bar shows cumulative token total when available:
  - `internal/tui/app.go`
- `doctor` now reports telemetry status from env:
  - `internal/cli/doctor.go`

### Verification Commands Run

- `env GOCACHE=/private/tmp/go-build-cache go test ./internal/observability/...`
- `env GOCACHE=/private/tmp/go-build-cache go test ./internal/logging/...`
- `env GOCACHE=/private/tmp/go-build-cache go test ./internal/permissions/...`
- `env GOCACHE=/private/tmp/go-build-cache go test ./internal/commands/...`
- `env GOCACHE=/private/tmp/go-build-cache go test ./internal/tui/...`
- `env GOCACHE=/private/tmp/go-build-cache go test ./internal/cli/...`
- `env GOCACHE=/private/tmp/go-build-cache go test ./internal/agent/...`
- `env GOCACHE=/private/tmp/go-build-cache go test ./internal/tools/...`
- `env GOCACHE=/private/tmp/go-build-cache go test -race ./internal/observability/... ./internal/agent/... ./internal/permissions/... ./internal/commands/... ./internal/cli/... ./internal/tui/...`
- `env GOCACHE=/private/tmp/go-build-cache go test ./...`
- `tools/check-allowed-deps.sh`
- `tools/check-network-policy.sh`

All passed.

### Implemented-But-Different Notes

- OTEL dependency/exporter packages were **not** added in this pass; bridge wiring and env gating are implemented with a no-op bridge by default.
- Telemetry env behavior (`NANDOCODEGO_TELEMETRY`, `NANDOCODEGO_OTEL_ENDPOINT`) and doctor/status integration are implemented, but actual OTLP export remains a follow-up.
- In-memory metrics, redaction, decorators, permission observation, `/cost`, and status-bar token accumulation are complete and active.
