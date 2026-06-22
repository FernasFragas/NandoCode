# Phase 6 Detailed Plan - State Layer

Date: 2026-05-03
Status: Final plan and implementation checklist
Source plans:

- `.codex/go-ollama-plan-AGENTS.md`
- `docs/PHASE-LOG.md`
- `docs/PHASE-1-DETAILED-PLAN.md`
- `docs/PHASE-3-DETAILED-PLAN.md`
- `docs/PHASE-4-DETAILED-PLAN.md`
- `docs/PHASE-5-DETAILED-PLAN.md`

## Goal

Phase 6 implements the two-tier state layer:

1. `internal/bootstrap`: a small mutable infrastructure singleton for session-level configuration and runtime facts.
2. `internal/state`: a tiny reactive app store for UI-facing state, transcript state, queued prompts, permission UI state, active tool calls, and future task summaries.

The goal is to make future TUI, REPL, memory, hooks, MCP, and task phases compose around one clear state boundary without turning global state into a transcript database. Phase 6 should be library-first, race-clean, and independent of Bubble Tea.

Deliverables:

- Thread-safe `bootstrap.State`.
- Typed bootstrap snapshot and initialization options.
- Generic reactive `state.Store[T]`.
- `state.App` model with copy helpers.
- Single `state.OnChange(prev, next App)` bridge for app-state-to-bootstrap mirroring.
- Unit, race, and benchmark coverage.
- Phase log update after implementation.

## Baseline Analysis

### Phase 0 - Security and Supply Chain

Implemented:

- `SECURITY.md`
- dependency allowlist
- network policy checker
- CI and security workflows
- phase verification scripts

Phase 6 implications:

- State must not weaken the default-deny network posture.
- Bootstrap may hold effective Ollama endpoint and telemetry settings, but Phase 6 must not initiate network calls.
- State must not persist prompts, model output, tool output, or secrets to disk.
- Do not add dependencies. The standard library is enough for locking, fan-out, and benchmarks.
- Avoid logging state values at INFO; many state fields can contain project paths, user prompts, or tool data.

### Phase 1 - Repo Scaffold, CLI, Paths, Logging

Implemented:

- Module path is `github.com/FernasFragas/nandocodego`.
- Signal-aware CLI entrypoint exists.
- `internal/paths` exposes config, data, cache, state, sessions, memory, and skills paths.
- `internal/logging` exists.
- `internal/config` exists as an empty package directory.

Phase 6 implications:

- Use `internal/paths` for bootstrap directory defaults.
- Do not implement full TOML config loading in Phase 6. `internal/config` is not ready.
- Bootstrap should accept an explicit `Initial`/`Options` struct that future config loading can populate.
- Avoid global logger mutation from state or bootstrap.

### Phase 2 - LLM Client

Implemented:

- `llm.Client`, message types, tool definitions, streaming events, model info, and usage counters.

Phase 6 implications:

- App state can store `llm.Message` history because that is the stable provider-neutral transcript type.
- Bootstrap can store the selected/default model and Ollama base URL as plain strings.
- State must not store an `llm.Client`; clients are infrastructure dependencies owned by later bootstrap/CLI wiring.

### Phase 3 - Tool Interface and Starter Tools

Implemented:

- `tools.Context`
- built-in tools
- path safety
- tool result/progress types

Phase 6 implications:

- App state should not store raw `context.Context`, `slog.Logger`, or callback functions from `tools.Context`.
- Define a serializable/copyable `state.ToolSettings` and provide a method to build a `tools.Context` for an agent run.
- Keep active tool progress in app state, not bootstrap.
- File snapshots and tool result details remain tool/agent concerns, not bootstrap concerns.

### Phase 4 - Agent Loop

Implemented:

- `agent.Input`
- `agent.Event`
- streaming assistant/thinking deltas
- tool start/progress/result events
- terminal events and usage

Phase 6 implications:

- App state should be able to represent an in-flight agent run:
  - active run flag,
  - active tool calls,
  - latest retry notice,
  - terminal reason/detail,
  - aggregate usage.
- Phase 6 should not wire the agent event stream into the store yet. That bridge belongs to Phase 7 REPL/TUI, but the state shape should support it.
- Do not store full `agent.Event` values as an append-only log; store reduced state that the UI can render efficiently.

### Phase 5 - Permission System

Implemented:

- `permissions.Mode`
- `permissions.Rules`
- `permissions.Resolve`
- agent input permission fields

Phase 6 implications:

- `state.App` should store the active permission mode and rules.
- `bootstrap.State` may mirror effective permission mode/rules for infrastructure consumers.
- Interactive permission prompt state belongs in `state.App`, not bootstrap.
- `ModeBubble` and prompt callback remain future integration points; Phase 6 should not implement permission UI.

## Evaluation of the Original Phase 6 Plan

The original Phase 6 plan is correct at the architectural level:

- one mutable bootstrap singleton;
- one tiny reactive store;
- one-way mirror from app state into bootstrap;
- race detector clean;
- no messages/progress in bootstrap.

It needs more implementation detail:

- It assumes `internal/config` can load bootstrap state, but the config package is empty.
- It does not define the bootstrap API or how to reset it in tests.
- It does not define which fields belong in bootstrap versus app state.
- It does not define subscriber fan-out semantics.
- It does not define whether subscribers receive every intermediate state or only the latest state.
- It does not define copy rules for slices and maps inside `state.App`.
- It does not define placeholders for tasks, permission prompts, or active tool calls before those phases exist.
- It does not define how Phase 5 permission mode/rules are represented.

## Final Phase 6 Scope

In scope:

- `internal/bootstrap/state.go`
- `internal/state/store.go`
- `internal/state/app.go`
- `internal/state/onchange.go`
- Unit tests for bootstrap, store, app copy helpers, and onchange mirroring.
- Race tests for concurrent store writes/subscribers.
- Store benchmark for the exit gate.
- Phase log update after implementation.

Out of scope:

- Full config loading or TOML parsing.
- CLI/TUI wiring.
- Agent-event reducer implementation.
- Persistent session files.
- Memory, hooks, MCP, skills, or tasks implementation.
- Telemetry export.
- Additional dependencies.

## Architecture Boundary

### Bootstrap State

Bootstrap state is infrastructure state. It is allowed to hold:

- directory paths,
- session identifiers,
- selected model and Ollama endpoint,
- permission mode/rules,
- execution budgets,
- telemetry flags,
- coarse runtime facts.

Bootstrap state must not hold:

- message transcript,
- queued prompts,
- streaming assistant text,
- tool progress/output,
- permission modal contents,
- task output,
- memory contents,
- MCP responses.

### App State

App state is UI/session state. It is allowed to hold:

- transcript messages,
- queued prompts,
- input buffer,
- active run flag,
- active tool summaries,
- permission prompt state,
- active model,
- permission mode/rules,
- reduced task summaries,
- aggregate usage.

App state must not hold:

- `llm.Client`,
- tool registry,
- `context.Context`,
- `slog.Logger`,
- open files,
- process handles,
- OS keyring values,
- full task output files.

### One-Way Mirror

The only allowed mirror direction is:

```text
state.App --OnChange--> bootstrap.State
```

There must be no bootstrap subscription back into app state in Phase 6. Future CLI/config code can initialize both tiers, but reactive changes flow one way.

## Target Package Layout

```text
internal/bootstrap/
  state.go
  state_test.go

internal/state/
  store.go
  store_test.go
  app.go
  app_test.go
  onchange.go
  onchange_test.go
  store_benchmark_test.go
```

## Bootstrap Design

Define in `internal/bootstrap/state.go`:

```go
type Initial struct {
    WorkingDir        string
    GitRoot           string
    ConfigDir         string
    DataDir           string
    CacheDir          string
    StateDir          string
    SessionID         string
    SessionDir        string
    DefaultModel      string
    OllamaBaseURL     string
    KeepAlive         string
    LogLevel          string
    LogFormat         string
    MaxTurns          int
    MaxOutputTokens   int
    LengthRetryTokens int
    MaxResultChars    int
    MaxReadChars      int
    BashTimeout       time.Duration
    PermissionMode    permissions.Mode
    PermissionRules   permissions.Rules
    TelemetryEnabled  bool
    TelemetryEndpoint string
}

type Snapshot struct {
    // same fields as Initial, plus CreatedAt/UpdatedAt if useful
}

type State struct {
    mu sync.RWMutex
    snapshot Snapshot
}
```

Public API:

```go
func DefaultInitial(workingDir string) Initial
func New(initial Initial) *State
func Global() *State
func InitGlobal(initial Initial)
func ResetGlobalForTest(initial Initial)
func (s *State) Snapshot() Snapshot
func (s *State) Update(f func(*Snapshot))
```

Rules:

- `DefaultInitial` uses `paths.ConfigDir`, `paths.DataDir`, `paths.CacheDir`, `paths.StateDir`, and `paths.SessionDir`.
- If `workingDir` is empty, default to `os.Getwd()`.
- If `SessionID` is empty, use a simple generated fallback in `DefaultInitial`, such as `session-<unix-nano>`; tests should pass an explicit session ID. Later ID branding can replace this in Phase 14.
- `New` normalizes empty/unknown permission modes to `permissions.ModeDefault`.
- `Snapshot` returns copies of slices/maps inside `permissions.Rules`.
- `Update` holds the write lock while updating the snapshot, then normalizes permission mode and defensively copies rules.
- `Global` is the single bootstrap singleton.
- `ResetGlobalForTest` exists only for tests and must be clearly documented.

Forbidden in bootstrap:

- A `Messages` field.
- `[]agent.Event`.
- `[]tools.ProgressEvent`.
- A second package-level mutable singleton.

## Store Design

Define in `internal/state/store.go`:

```go
type Store[T any] struct {
    mu          sync.RWMutex
    value       T
    onChange    func(prev, next T)
    subscribers map[int]chan T
    nextID      int
}

func NewStore[T any](initial T, onChange func(prev, next T)) *Store[T]
func (s *Store[T]) Get() T
func (s *Store[T]) Set(f func(prev T) T)
func (s *Store[T]) Subscribe() (<-chan T, func())
```

Semantics:

- `Get` returns the current value by value.
- `Set` calls `f(prev)` exactly once.
- `Set` calls `onChange(prev, next)` exactly once after releasing the store lock.
- `Set` notifies subscribers after `onChange`.
- `Subscribe` returns a receive-only channel and an idempotent unsubscribe function.
- Subscriber channels are buffered with capacity 1.
- Subscribers receive the latest state, not a guaranteed stream of every intermediate state.
- Slow subscribers must not block `Set`; if a subscriber buffer is full, replace the queued value with the latest value.
- Closing a subscriber channel is owned by `unsubscribe`.
- `Set` should be safe under concurrent writers and race-clean.

Implementation note:

- For generic `T`, the store cannot deep-copy arbitrary values. `state.App` must provide copy helpers and callers must use copy-on-write update patterns for slices/maps.

## App State Design

Define in `internal/state/app.go`:

```go
type VimMode string
const (
    VimInsert VimMode = "insert"
    VimNormal VimMode = "normal"
    VimVisual VimMode = "visual"
)

type ToolSettings struct {
    WorkingDir            string
    AdditionalWorkingDirs []string
    Env                   []string
    BashTimeout           time.Duration
    MaxResultChars        int
    MaxReadChars          int
}

type ToolUse struct {
    ID        string
    Name      string
    Summary   string
    StartedAt time.Time
    Done      bool
    Error     string
}

type PermissionPrompt struct {
    ID       string
    ToolName string
    Target   string
    Reason   string
}

type TaskSummary struct {
    ID          string
    Kind        string
    Status      string
    Description string
    OutputFile  string
}

type App struct {
    Messages          []llm.Message
    QueuedPrompts     []string
    InputBuffer       string
    VimMode           VimMode
    ActiveModel       string
    ToolSettings      ToolSettings
    PermissionMode    permissions.Mode
    PermissionRules   permissions.Rules
    PermissionPrompt  *PermissionPrompt
    ActiveRun         bool
    ActiveTools       map[string]ToolUse
    Tasks             []TaskSummary
    LastRetryNotice   string
    TerminalReason    agent.TerminalReason
    TerminalDetail    string
    Usage             agent.Usage
}
```

Helpers:

```go
func DefaultApp(bootstrap.Snapshot) App
func (a App) Clone() App
func (a App) ToolContext(ctx context.Context) tools.Context
```

Rules:

- `DefaultApp` initializes mode/model/tool settings from bootstrap snapshot.
- `Clone` deep-copies all slices, maps, and pointer fields.
- `ToolContext` builds a fresh `tools.Context` without storing `context.Context` inside app state.
- `PermissionRules` must be copied through a helper to avoid aliasing.
- `ActiveTools` is keyed by `agent.ToolUseStart.ID`.
- Task summaries are placeholders only. Do not import `internal/tasks`.

## OnChange Design

Define in `internal/state/onchange.go`:

```go
func OnChange(prev, next App)
```

Mirrored from app state to bootstrap:

- active/default model,
- working directory,
- tool budget settings,
- permission mode,
- permission rules.

Not mirrored:

- messages,
- queued prompts,
- input buffer,
- active tool calls,
- permission prompt modal state,
- retry notices,
- terminal detail,
- usage,
- tasks.

Rules:

- `OnChange` is the only production `Store[App]` callback that writes to bootstrap.
- `OnChange` must avoid expensive work and must not block on I/O.
- `OnChange` must not call model clients, tools, hooks, or telemetry exporters.
- `OnChange` should compare `prev` and `next` to avoid unnecessary bootstrap updates, but correctness is more important than micro-optimizing.

## Test Plan

### Bootstrap Tests

- `DefaultInitial` fills directory paths from `internal/paths`.
- Empty working directory falls back to current directory.
- Empty permission mode normalizes to `ModeDefault`.
- `Snapshot` returns copies, not mutable aliases.
- Concurrent `Snapshot` and `Update` calls are race-clean.
- `ResetGlobalForTest` isolates tests.

### Store Tests

- `Get` returns the initial value.
- `Set` updates the value.
- `Set` calls the updater exactly once.
- `Set` calls `onChange` exactly once with correct `prev` and `next`.
- `Subscribe` receives the current/latest value.
- Unsubscribe is idempotent and closes the subscriber channel.
- Concurrent writers do not race.
- Slow subscriber does not block `Set`.

### App Tests

- `DefaultApp` copies model, working directory, budgets, and permission state from bootstrap snapshot.
- `App.Clone` deep-copies messages, queued prompts, env, additional dirs, permission rules, active tools, task summaries, and permission prompt.
- Mutating a clone does not mutate the source.
- `ToolContext(ctx)` uses supplied context and app tool settings.

### OnChange Tests

- Changing active model mirrors to bootstrap.
- Changing working directory mirrors to bootstrap.
- Changing permission mode/rules mirrors to bootstrap.
- Changing messages does not mirror transcript into bootstrap.
- Changing active tools does not write tool progress into bootstrap.
- `OnChange` does not panic with zero-value `App`.

### Benchmark

Add:

```go
func BenchmarkStoreSetFiveSubscribers(b *testing.B)
```

Benchmark requirements:

- 5 subscribers.
- At least 10,000 `Set` calls.
- No allocations that grow with number of historical updates.
- Record enough timing to support the exit-gate target: 10 K `Set` calls/sec with 5 subscribers and p99 set latency at or below 1 ms on the reference machine.

## Concrete Todos

### A. Pre-Flight Analysis

- [ ] Run `env GOCACHE=/private/tmp/nandocodego-gocache GOMODCACHE=/private/tmp/nandocodego-gomodcache go test ./internal/permissions/... ./internal/agent/... ./internal/tools/...`.
- [ ] Confirm `internal/bootstrap` and `internal/state` contain no existing files to preserve.
- [ ] Review `internal/paths` defaults for config/data/cache/state/session paths.
- [ ] Review `agent.Event`, `agent.Usage`, and `agent.TerminalReason` before defining app state.

### B. Implement Bootstrap State

- [ ] Create `internal/bootstrap/state.go`.
- [ ] Define `Initial`.
- [ ] Define `Snapshot`.
- [ ] Define `State`.
- [ ] Implement `DefaultInitial`.
- [ ] Implement `New`.
- [ ] Implement `Global`.
- [ ] Implement `InitGlobal`.
- [ ] Implement `ResetGlobalForTest`.
- [ ] Implement `Snapshot`.
- [ ] Implement `Update`.
- [ ] Add rule-copy helpers or private copy functions.
- [ ] Normalize permission mode on initialization and update.

### C. Test Bootstrap State

- [ ] Create `internal/bootstrap/state_test.go`.
- [ ] Test path defaults with environment overrides.
- [ ] Test permission mode normalization.
- [ ] Test snapshot copy behavior.
- [ ] Test concurrent snapshot/update with race detector.
- [ ] Test global reset isolation.

### D. Implement Generic Store

- [ ] Create `internal/state/store.go`.
- [ ] Define `Store[T]`.
- [ ] Implement `NewStore`.
- [ ] Implement `Get`.
- [ ] Implement `Set`.
- [ ] Implement `Subscribe`.
- [ ] Implement nonblocking latest-value fan-out.
- [ ] Make unsubscribe idempotent.
- [ ] Document subscriber semantics.

### E. Test Generic Store

- [ ] Create `internal/state/store_test.go`.
- [ ] Test initial `Get`.
- [ ] Test `Set`.
- [ ] Test updater called once.
- [ ] Test `onChange` called once with correct values.
- [ ] Test subscribe and unsubscribe.
- [ ] Test slow subscriber behavior.
- [ ] Test concurrent writers and subscribers.

### F. Implement App State

- [ ] Create `internal/state/app.go`.
- [ ] Define `VimMode`.
- [ ] Define `ToolSettings`.
- [ ] Define `ToolUse`.
- [ ] Define `PermissionPrompt`.
- [ ] Define `TaskSummary`.
- [ ] Define `App`.
- [ ] Implement `DefaultApp`.
- [ ] Implement `App.Clone`.
- [ ] Implement `App.ToolContext`.
- [ ] Add helper functions for copying `permissions.Rules`.

### G. Test App State

- [ ] Create `internal/state/app_test.go`.
- [ ] Test `DefaultApp`.
- [ ] Test deep-copy behavior for every slice/map/pointer field.
- [ ] Test `ToolContext` conversion.
- [ ] Test zero-value app clone safety.

### H. Implement OnChange Bridge

- [ ] Create `internal/state/onchange.go`.
- [ ] Implement `OnChange(prev, next App)`.
- [ ] Mirror only model, working dir, budgets, permission mode, and permission rules.
- [ ] Do not mirror messages, active tools, queued prompts, tasks, terminal detail, or usage.
- [ ] Keep `OnChange` side-effect-light and I/O-free.

### I. Test OnChange

- [ ] Create `internal/state/onchange_test.go`.
- [ ] Use `bootstrap.ResetGlobalForTest`.
- [ ] Test mirrored fields update bootstrap.
- [ ] Test non-mirrored fields do not add transcript/progress fields to bootstrap.
- [ ] Test zero-value behavior.

### J. Benchmark and Verification

- [ ] Create `internal/state/store_benchmark_test.go`.
- [ ] Add `BenchmarkStoreSetFiveSubscribers`.
- [ ] Run `go test ./internal/bootstrap/... ./internal/state/...`.
- [ ] Run `go test -race ./internal/bootstrap/... ./internal/state/...`.
- [ ] Run `go test ./...`.
- [ ] Run `go vet ./...`.
- [ ] Run `tools/check-allowed-deps.sh`.
- [ ] Run `tools/check-network-policy.sh`.
- [ ] Run `go test -bench=BenchmarkStoreSetFiveSubscribers ./internal/state`.
- [ ] Run `rg "bootstrap\\.(Global|InitGlobal|ResetGlobalForTest|State|Update)" internal/state internal/bootstrap internal | sort` and verify only `internal/state/onchange.go`, bootstrap implementation/tests, and explicit tests write to bootstrap.

### K. Documentation and Phase Log

- [ ] Update `docs/PHASE-LOG.md` with Phase 6 implementation details.
- [ ] Record benchmark output.
- [ ] Record race-test output.
- [ ] Record any deviations from this plan.

## Acceptance Criteria

- [ ] `internal/bootstrap.State` exists and is mutex-guarded.
- [ ] `internal/state.Store[T]` exists with `Get`, `Set`, and `Subscribe`.
- [ ] `state.App` exists and keeps transcript/progress out of bootstrap.
- [ ] `state.OnChange` is the only production app-state callback that writes to bootstrap.
- [ ] `OnChange` mirrors only selected infrastructure fields.
- [ ] Subscribe/unsubscribe tests pass under concurrent writers.
- [ ] `OnChange` is called exactly once per `Set` with correct `prev` and `next`.
- [ ] Race detector is clean for bootstrap and state packages.
- [ ] Store benchmark demonstrates 10 K `Set` calls/sec with 5 subscribers and p99 set latency at or below 1 ms.
- [ ] No new direct dependency is added.

## Exit Gate

Phase 6 is complete when:

- `env GOCACHE=/private/tmp/nandocodego-gocache GOMODCACHE=/private/tmp/nandocodego-gomodcache go test -race ./internal/bootstrap/... ./internal/state/...` passes.
- `go test -bench=BenchmarkStoreSetFiveSubscribers ./internal/state` meets the target.
- A static search shows no package other than `internal/state/onchange.go` writes app-derived values into bootstrap state.
- Bootstrap snapshots contain infrastructure fields only, while app state contains messages, queued prompts, active tools, permission prompts, and task summaries.
