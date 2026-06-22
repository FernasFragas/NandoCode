# Phase 4 Detailed Plan - Agent Loop

Date: 2026-05-02
Status: Final plan and implementation checklist
Source plans:

- `.codex/go-ollama-plan-AGENTS.md`
- `.codex/go-ollama-plan-HUMANS.md`
- `docs/PHASE-3-DETAILED-PLAN.md`
- `docs/PHASE-LOG.md`

## Goal

Phase 4 implements the first model-driven agent loop.

Given a system prompt, message history, an `llm.Client`, and a tool registry, the agent streams an assistant response, executes requested tool calls through the Phase 3 tool boundary, appends tool-result messages, and continues until the model finishes, the caller aborts, a turn budget is reached, context overflows, or an unrecoverable error occurs.

The goal is not to build the REPL, TUI, full permission system, hooks, memory, MCP, sub-agents, or async/concurrent tool execution. Phase 4 should leave a small, testable `internal/agent` package that later phases can wrap.

## Current Repository Reality

Phase 0 through Phase 3 are implemented. Phase 1 was recently hardened. Phase 4 has not been implemented yet.

Available building blocks:

- `llm.Client.Chat(ctx, *llm.ChatRequest) (<-chan llm.StreamEvent, error)`
- `llm.WatchStream(ctx, stream, config)`
- `llm.ChatRequest.Options`
- `llm.Message`, `llm.ToolCall`, and `llm.ToolDef`
- `tools.Registry`
- `tools.Tool`
- `tools.ToLLMToolDef`
- `tools.Context`
- `tools.PermissionResult`
- `internal/tools/builtin.NewRegistry()`

Important current constraints:

- `internal/agent` has no implementation files.
- `llm.Client` does not have `ChatOnce` or `Close`.
- `llm.ToolCall` does not include a model-provided ID.
- Tool arguments are stored as `map[string]any`; Phase 3 tool input parsers expect `json.RawMessage`.
- Phase 5 permissions and Phase 9 hooks do not exist yet.
- No Phase 4 CLI or TUI integration exists and should not be added in this phase unless needed for a smoke harness.

## Baseline Analysis From Phases 0-3

### Phase 0 - Security and Supply Chain

Implemented:

- `SECURITY.md`
- dependency allowlist
- network policy checker
- CI baseline
- dependency review
- Phase 0 verifier

Phase 4 implications:

- The agent loop is the first code that lets model output drive local actions. It must route every tool call through the Phase 3 tool interface.
- The loop must not bypass `Tool.CheckPermissions`, even though Phase 5 has not implemented the full permission resolver.
- The loop must not log full prompts, full message history, tool inputs, or tool results at INFO.
- The loop must not introduce new network destinations. Only the configured `llm.Client` may communicate with Ollama.
- New direct dependencies should be avoided. Phase 4 can be implemented with the standard library plus existing internal packages.

### Phase 1 - CLI, Paths, Logging, Tooling

Implemented and hardened:

- runnable `nandocodego` binary,
- signal-aware CLI entrypoint,
- `cli.Run(ctx, args)`,
- `cli.ExitCode(err)`,
- version command,
- path helpers,
- logging tests,
- Makefile targets,
- CI skip cleanup.

Phase 4 implications:

- The agent package should accept `context.Context` and not call `os.Exit`.
- The package should be callable from future CLI/TUI code without global mutable state.
- Tests should use fake clients and temporary working directories.
- Logging should be injected or optional, and default to `slog.Default()` only at the boundary.

### Phase 2 - LLM Client

Implemented:

- streaming chat client,
- stream events,
- tool-call data types,
- watchdog wrapper,
- retry classification helpers,
- model capabilities,
- Ollama examples.

Known Phase 2 debt:

- no `errors.go`,
- no `clientopts.go`,
- no `ollama/errors.go`,
- no `ChatOnce`,
- no `Close`,
- no `doctor --ollama`,
- no NDJSON parser fixture suite.

Phase 4 implications:

- Do not require `ChatOnce` or `Close`.
- Use `Chat` plus `WatchStream`.
- Own retry notices at the agent layer.
- Use fake `llm.Client` tests instead of depending on missing NDJSON fixtures.
- Treat watchdog timeout as retryable only before any tool executes in the current turn.

### Phase 3 - Tool Interface and Starter Tools

Implemented:

- core tool API,
- registry,
- tool-to-LLM definition conversion,
- path safety,
- `Bash`,
- `FileRead`,
- `FileWrite`,
- built-in registry,
- one-shot tool example,
- tool tests and integration-style atomic write test.

Phase 4 implications:

- Convert enabled tools to `llm.ToolDef` each turn.
- Look up tool calls by canonical name or alias through `Registry.Lookup`.
- Marshal `llm.ToolCall.Function.Arguments` into JSON and pass to `Tool.UnmarshalInput`.
- Call `Tool.CheckPermissions`.
- In Phase 4, `PermAllow` executes; `PermDeny` becomes a tool-result message; `PermAsk` must not prompt interactively yet and should be treated as denied with an explicit reason.
- Tool execution should be serial. Concurrency is Phase 15.
- Capture tool progress as agent events.
- Append tool results as `llm.RoleTool` messages using bounded display text.

## Evaluation of Original Phase 4 Plan

The original Phase 4 plan has the right shape:

- channel-returning `Run(ctx, input)`,
- sealed event interface,
- terminal reason enum,
- streamed text/thinking deltas,
- tool start/progress/result events,
- turn and output-token budgeting,
- watchdog integration,
- fake-client tests,
- abort latency requirement.

The plan needs more detail before implementation:

- Define concrete `Agent`, `Input`, `Config`, `Usage`, and error-handling types.
- Specify how tool-call IDs are synthesized because `llm.ToolCall` has no ID.
- Specify how `map[string]any` tool arguments become `json.RawMessage`.
- Specify how tool permission results are handled before Phase 5.
- Specify how tool results are serialized back to `llm.Message`.
- Specify retry boundaries so the agent never retries after a tool has executed in that turn.
- Specify deterministic fake-client test fixtures without requiring NDJSON parser work.
- Specify package layout that will not import future phases.

## Final Phase 4 Scope

In scope:

- `internal/agent` package.
- Agent event types.
- Terminal reasons.
- Agent input/config types.
- Agent run loop.
- Streaming assistant text/thinking accumulation.
- Watchdog wrapping.
- Serial tool execution through `tools.Tool`.
- Tool progress forwarding.
- Tool-result messages appended to history.
- Max turn handling.
- Context cancellation handling.
- Output-token length retry handling.
- Retry notices for stream retry paths.
- Fake-client unit tests.
- Optional integration smoke test behind `//go:build integration`.
- Phase log update after implementation.

Out of scope:

- REPL or TUI.
- Interactive permission prompts.
- Full `internal/permissions` resolver.
- Hooks and stop hooks.
- MCP.
- Memory.
- State layer.
- Sub-agents.
- Background tasks.
- Concurrent/speculative tool execution.
- New direct dependencies.
- Default network checks in `doctor`.

## Target Package Layout

```text
internal/agent/
  agent.go
  events.go
  input.go
  loop.go
  stream.go
  tools.go
  usage.go
  errors.go
  agent_test.go
  fake_client_test.go
  tools_test.go
  integration_test.go
```

Recommended responsibilities:

- `events.go`: event interface and event structs.
- `input.go`: `Input`, `Config`, defaults, validation.
- `agent.go`: `Agent` construction and public `Run`.
- `loop.go`: turn loop and terminal transitions.
- `stream.go`: one model turn, stream accumulation, watchdog handling.
- `tools.go`: tool definition conversion, tool-call execution, message serialization.
- `usage.go`: usage aggregation.
- `errors.go`: internal sentinel errors for testable terminal mapping.

## Target Public API

```go
type Agent struct {
    client llm.Client
    tools  *tools.Registry
    config Config
    logger *slog.Logger
}

func New(client llm.Client, registry *tools.Registry, opts ...Option) (*Agent, error)
func (a *Agent) Run(ctx context.Context, in Input) <-chan Event
```

`Run` rules:

- Return the event channel immediately.
- Run the loop in one goroutine.
- Close the channel after emitting exactly one terminal event.
- Honor `ctx.Done()` within 200 ms in tests.
- Never panic on malformed model tool calls.
- Never call `os.Exit`.
- Never mutate caller-owned message slices.

## Input and Config

```go
type Input struct {
    Model        string
    SystemPrompt string
    Messages     []llm.Message
    ToolContext  tools.Context
}

type Config struct {
    MaxTurns       int
    MaxOutputTokens int
    LengthRetryTokens int
    Watchdog        llm.WatchdogConfig
}
```

Defaults:

- `MaxTurns`: 32.
- `MaxOutputTokens`: 8192.
- `LengthRetryTokens`: 65536.
- `Watchdog`: `llm.DefaultWatchdogConfig()`.

Validation:

- `llm.Client` is required.
- Tool registry may be nil; nil means no tools.
- `Input.Model` is required.
- `ToolContext.Context` should be set to the run context if missing.
- `ToolContext.WorkingDir` should default via `tools.DefaultContext`.

## Events

Define a sealed event interface:

```go
type Event interface{ isEvent() }
```

Event structs:

```go
type AssistantTextDelta struct { Content string }
type AssistantThinkingDelta struct { Thinking string }
type ToolUseStart struct { ID, Name string; Input any }
type ToolUseProgress struct { ID string; Data any }
type ToolUseResult struct { ID string; Result tools.Result; Err error }
type RetryNotice struct { Attempt int; Cause string }
type Terminal struct { Reason TerminalReason; Detail string; Usage Usage }
```

Terminal reasons:

```go
type TerminalReason string

const (
    TerminalCompleted TerminalReason = "completed"
    TerminalAborted TerminalReason = "aborted"
    TerminalMaxTurns TerminalReason = "max_turns"
    TerminalContextOverflow TerminalReason = "context_overflow"
    TerminalStopHook TerminalReason = "stop_hook"
    TerminalUnrecoverable TerminalReason = "unrecoverable"
)
```

Phase 4 note:

- `TerminalStopHook` is defined now for API stability but should not be emitted until hooks exist, except in tests that validate enum stability.

## Usage

```go
type Usage struct {
    PromptEvalCount int64
    EvalCount       int64
    TotalDuration   int64
    Turns           int
    ToolCalls       int
}
```

Rules:

- Sum counts from terminal `llm.StreamEvent` values.
- Increment `Turns` once per model turn attempted.
- Increment `ToolCalls` for every tool call that reaches permission handling, including denied calls.

## Turn Loop

Each turn:

1. Check context cancellation.
2. Build enabled tool definitions:
   - iterate `registry.All()`,
   - skip `!tool.IsEnabled(toolCtx)`,
   - convert with `tools.ToLLMToolDef`.
3. Build `llm.ChatRequest`:
   - model from input,
   - system prompt as an initial system message if non-empty,
   - current history,
   - enabled tools,
   - `Stream: true`,
   - `Options["num_predict"] = current output-token budget`.
4. Call `client.Chat`.
5. Wrap returned stream with `llm.WatchStream`.
6. Accumulate assistant content, thinking, and tool calls.
7. Emit text/thinking deltas as chunks arrive.
8. On done:
   - update usage,
   - inspect `DoneReason`,
   - if length, retry once with `LengthRetryTokens`,
   - if watchdog timeout and no tool has executed in the turn, retry according to policy,
   - if no tool calls, append assistant message and terminate completed,
   - if tool calls exist, append assistant message with tool calls, execute serially, append tool messages, and continue.
9. Stop with `TerminalMaxTurns` if the next turn would exceed `MaxTurns`.

## Stream Handling

Accumulation rules:

- Emit `AssistantTextDelta` only for non-empty `event.Message.Content`.
- Emit `AssistantThinkingDelta` only for non-empty `event.Message.Thinking`.
- Append content and thinking exactly in stream order.
- Treat `event.Message.ToolCalls` as the latest complete set observed for that turn.
- Do not execute tools until the stream is done in Phase 4. The original plan says "as soon as complete tool_calls array is observed"; because `llm.ToolCall` does not expose an explicit completeness signal, Phase 4 should execute after the done event for deterministic behavior.

Done reason handling:

- `""` or `"stop"`: normal completion.
- `"length"`: one retry with expanded `num_predict`; second length becomes `TerminalContextOverflow`.
- `"watchdog_timeout"`: retry if no tool has executed in this turn; otherwise `TerminalUnrecoverable`.
- unknown non-empty reasons: `TerminalUnrecoverable` with detail.

## Retry Behavior

Phase 4 retry rules:

- Retry model stream setup errors according to `llm.ClassifyError` and `llm.GetRetryPolicy`.
- Emit `RetryNotice` before each retry.
- Do not retry after any tool execution started in the current turn.
- Context cancellation always wins and emits `TerminalAborted`.
- Keep retry counts separate from turn counts. A retry of the same model turn should not append duplicate messages.

Concrete first implementation:

- One retry for watchdog timeout before tool execution.
- One retry for `done_reason == "length"` with `LengthRetryTokens`.
- Up to the existing `llm.GetRetryPolicy` count for `client.Chat` setup errors.
- Stream mid-turn non-watchdog errors are represented by stream closure without a done event; treat as unrecoverable unless the fake client exposes an explicit setup error.

## Tool Execution

Tool-call execution steps:

1. Synthesize an ID:
   - `turn-<turn-number>-tool-<index>`.
2. Look up the tool by `call.Function.Name`.
3. If missing:
   - emit `ToolUseStart`,
   - emit `ToolUseResult` with an error,
   - append a tool-result message describing the unknown tool.
4. Marshal `call.Function.Arguments` to JSON.
5. Call `Tool.UnmarshalInput(raw)`.
6. Emit `ToolUseStart` with parsed input when parsing succeeds, or raw arguments when parsing fails.
7. Call `Tool.CheckPermissions(toolCtx, parsedInput)`.
8. Permission behavior in Phase 4:
   - `PermAllow`: execute.
   - `PermDeny`: do not execute; append denied tool result.
   - `PermAsk`: do not prompt yet; append denied tool result saying interactive prompts arrive in Phase 5/7.
9. Execute with a progress channel.
10. Forward each `tools.ProgressEvent` as `ToolUseProgress`.
11. Emit `ToolUseResult`.
12. Append one `llm.RoleTool` message.

Tool-result message shape:

```go
llm.Message{
    Role: llm.RoleTool,
    ToolName: toolName,
    Content: boundedDisplayOrError,
}
```

Result content rules:

- Prefer `result.Display`.
- If empty, JSON-marshal `result.Data`.
- Apply `tools.TruncateDisplay` using `ToolContext.EffectiveMaxResultChars()`.
- For errors and denials, include a concise, model-readable sentence.
- Do not include Go stack traces.

Progress behavior:

- Use a buffered progress channel.
- Drain progress until tool call returns.
- Do not block tool execution if the agent event consumer is slow; event sending should be context-aware.

## Fake Client Testing Strategy

Create a fake `llm.Client` with scripted turns:

```go
type fakeClient struct {
    turns []fakeTurn
}

type fakeTurn struct {
    events []llm.StreamEvent
    err    error
    wait   time.Duration
}
```

Rules:

- Each `Chat` call consumes one `fakeTurn`.
- Events are sent asynchronously.
- Context cancellation stops event emission.
- Tests can inspect received `ChatRequest` values.

Do not use NDJSON parser fixtures in Phase 4 unit tests. That remains Phase 2 debt.

## Concrete Todos

### A. Pre-Flight Verification

- [ ] Run `go mod tidy`.
- [ ] Run `tools/check-allowed-deps.sh`.
- [ ] Run `tools/check-network-policy.sh`.
- [ ] Run `env GOCACHE=/private/tmp/nandocodego-gocache GOMODCACHE=/private/tmp/nandocodego-gomodcache go test ./...`.
- [ ] Run `env GOCACHE=/private/tmp/nandocodego-gocache GOMODCACHE=/private/tmp/nandocodego-gomodcache go vet ./...`.

### B. Create Agent Package Skeleton

- [ ] Create `internal/agent/events.go`.
- [ ] Create `internal/agent/input.go`.
- [ ] Create `internal/agent/usage.go`.
- [ ] Create `internal/agent/agent.go`.
- [ ] Create `internal/agent/loop.go`.
- [ ] Create `internal/agent/stream.go`.
- [ ] Create `internal/agent/tools.go`.
- [ ] Create `internal/agent/errors.go` if sentinel errors are useful.

### C. Define Events and Usage

- [ ] Add sealed `Event` interface.
- [ ] Add assistant delta events.
- [ ] Add tool start/progress/result events.
- [ ] Add retry notice event.
- [ ] Add terminal event.
- [ ] Add `TerminalReason` enum.
- [ ] Add `Usage`.
- [ ] Add compile-time `isEvent()` methods.
- [ ] Add tests for terminal reason string values.

### D. Define Agent Construction

- [ ] Add `Config` with defaults.
- [ ] Add `Option` functions:
  - `WithConfig(Config)`,
  - `WithLogger(*slog.Logger)`,
  - optional `WithWatchdog(llm.WatchdogConfig)`.
- [ ] Add `New(client llm.Client, registry *tools.Registry, opts ...Option)`.
- [ ] Validate nil client.
- [ ] Allow nil registry as no tools.
- [ ] Ensure defaults are applied.
- [ ] Add constructor tests.

### E. Implement Run Contract

- [ ] `Run` returns immediately.
- [ ] `Run` starts exactly one goroutine.
- [ ] Event channel closes after terminal.
- [ ] Exactly one `Terminal` event is emitted.
- [ ] Context cancellation emits `TerminalAborted`.
- [ ] Abort path completes within 200 ms in test.
- [ ] No `os.Exit`, sleeps without context, or package-level mutable state.

### F. Build Chat Requests

- [ ] Copy input message history before mutation.
- [ ] Prepend system prompt as `RoleSystem` when non-empty.
- [ ] Filter disabled tools via `Tool.IsEnabled`.
- [ ] Convert tools via `tools.ToLLMToolDef`.
- [ ] Set `Stream: true`.
- [ ] Set `Options["num_predict"]`.
- [ ] Preserve existing `Options` behavior if the input later grows an options field.
- [ ] Add request-building tests.

### G. Stream One Model Turn

- [ ] Call `client.Chat`.
- [ ] Wrap stream with `llm.WatchStream`.
- [ ] Emit text deltas.
- [ ] Emit thinking deltas.
- [ ] Accumulate final assistant content.
- [ ] Accumulate final assistant thinking.
- [ ] Capture latest tool calls.
- [ ] Aggregate usage from done events.
- [ ] Detect missing done event.
- [ ] Add stream tests for text, thinking, and tool-call accumulation.

### H. Implement Retry and Length Handling

- [ ] Emit `RetryNotice` for setup retries.
- [ ] Emit `RetryNotice` for watchdog retry.
- [ ] On first `done_reason == "length"`, retry with `LengthRetryTokens`.
- [ ] On second length, emit `TerminalContextOverflow`.
- [ ] Never retry after a tool execution starts in the current turn.
- [ ] Add tests for length retry and context overflow.
- [ ] Add test for watchdog retry before tools.

### I. Implement Tool Execution Bridge

- [ ] Synthesize deterministic tool IDs.
- [ ] Lookup tools by model name.
- [ ] Marshal tool arguments to JSON.
- [ ] Parse with `Tool.UnmarshalInput`.
- [ ] Emit `ToolUseStart`.
- [ ] Call `Tool.CheckPermissions`.
- [ ] Execute only on `PermAllow`.
- [ ] Treat `PermDeny` as denied result.
- [ ] Treat `PermAsk` as denied result in Phase 4.
- [ ] Forward progress events.
- [ ] Emit `ToolUseResult`.
- [ ] Append bounded `RoleTool` messages.
- [ ] Add tests for unknown tool, malformed input, denied tool, ask tool, allowed tool, progress forwarding, and result truncation.

### J. Implement Turn Loop

- [ ] Stop completed when no tool calls are returned.
- [ ] If tool calls exist, append assistant message and tool messages, then continue.
- [ ] Enforce `MaxTurns`.
- [ ] Emit `TerminalMaxTurns` when budget is exhausted.
- [ ] Preserve message ordering:
  - prior history,
  - assistant message with tool calls,
  - one tool message per call in call order.
- [ ] Add tests:
  - no tools, one turn,
  - one tool call, two turns,
  - max turns reached.

### K. Integration Smoke Test

- [ ] Add `internal/agent/integration_test.go` with `//go:build integration`.
- [ ] Skip unless explicit environment is set, for example `NANDOCODEGO_RUN_OLLAMA_INTEGRATION=1`.
- [ ] Use `internal/tools/builtin.NewRegistry()`.
- [ ] Use a real Ollama model from env, defaulting to `qwen3`.
- [ ] Ask for a harmless `Bash` read-only command such as `ls`.
- [ ] Assert a `ToolUseStart` or `ToolUseResult` for `Bash`.
- [ ] Assert terminal completed.
- [ ] Document that the Phase 4 exit gate requires running the smoke test 10 times against local Ollama.

### L. Documentation and Phase Log

- [ ] Update `docs/PHASE-LOG.md` with Phase 4 files, tests, checks, decisions, and open questions.
- [ ] Record any skipped integration checks with exact reason.
- [ ] Update this plan's checklist after implementation.

## Required Tests

Unit tests:

- [ ] no tools, one turn,
- [ ] assistant thinking and content deltas,
- [ ] one tool call, two turns,
- [ ] unknown tool produces tool error and continues,
- [ ] malformed tool args produce tool error and continues,
- [ ] denied tool becomes tool-result message,
- [ ] ask-required tool is not executed in Phase 4,
- [ ] progress events are forwarded,
- [ ] result display is truncated,
- [ ] watchdog timeout recovers in retry before tools,
- [ ] length retry expands `num_predict`,
- [ ] second length becomes context overflow,
- [ ] abort mid-stream emits terminal aborted within 200 ms,
- [ ] max turns reached,
- [ ] setup error retries emit retry notices,
- [ ] no retry after tool execution starts.

Integration tests:

- [ ] built-in tools with fake client,
- [ ] optional real Ollama smoke under integration tag.

## Acceptance Criteria

Phase 4 is complete when:

- [ ] `internal/agent` exists with the public `Agent.Run(ctx, input) <-chan Event` API.
- [ ] `Run` returns immediately and closes the event channel on terminal state.
- [ ] Exactly one terminal event is emitted for every run.
- [ ] Context cancellation emits `TerminalAborted`.
- [ ] Abort path takes <= 200 ms after cancellation in tests.
- [ ] Assistant text and thinking deltas stream as events.
- [ ] Enabled tools are passed to `llm.ChatRequest`.
- [ ] Tool calls execute serially through `tools.Tool`.
- [ ] Tool permissions are checked before execution.
- [ ] `PermAsk` does not execute tools in Phase 4.
- [ ] Tool results are appended as `llm.RoleTool` messages.
- [ ] Max turn budget emits `TerminalMaxTurns`.
- [ ] First `done_reason == "length"` retries with expanded output tokens.
- [ ] Second `done_reason == "length"` emits `TerminalContextOverflow`.
- [ ] Watchdog timeout can retry before any tool execution.
- [ ] The agent does not import future permissions, hooks, state, TUI, MCP, memory, tasks, or sub-agent packages.
- [ ] `go test -race ./internal/agent/...` passes.
- [ ] `go test ./...` passes.
- [ ] `go vet ./...` passes.
- [ ] `tools/check-allowed-deps.sh` passes.
- [ ] `tools/check-network-policy.sh` passes.
- [ ] Optional real Ollama smoke is implemented and documented.
- [ ] `docs/PHASE-LOG.md` has a Phase 4 entry after implementation.

## Recommended Execution Order

1. Run pre-flight checks.
2. Create event, input, config, and usage types.
3. Add fake client test helper.
4. Implement `Agent.New` and `Run` channel lifecycle.
5. Implement one-turn streaming without tools.
6. Add request-building and enabled-tool conversion.
7. Add tool-call bridge.
8. Add full turn loop.
9. Add retry, watchdog, and length handling.
10. Add max-turn and abort tests.
11. Add optional integration smoke.
12. Run verification.
13. Update phase log.

## Risks

- Tool-call completeness is ambiguous in the current `llm.StreamEvent` model. Mitigation: execute tools only after the done event in Phase 4.
- `PermAsk` has no prompt UI yet. Mitigation: fail closed and return a model-readable denial result until Phase 5/7.
- Retrying after a tool executes can duplicate side effects. Mitigation: never retry after tool execution starts in the current turn.
- Unknown model done reasons may hide provider behavior. Mitigation: treat unknown non-empty done reasons as unrecoverable and record detail.
- Fake tests can overfit to implementation details. Mitigation: assert event sequences and request shapes, not private helpers.
- Real Ollama smoke can be flaky or unavailable. Mitigation: keep it behind an integration tag and env opt-in, but document the manual exit gate.
