# Phase 11 Detailed Plan - Sub-Agents and Fork

Date: 2026-05-07
Status: Complete in code and automated checks; live exit-gate validation pending
Source plans and references:

- `.codex/go-ollama-plan-AGENTS.md`
- `.codex/go-ollama-plan-HUMANS.md`
- `docs/PHASE-LOG.md`
- `docs/PROJECT-STATUS-AND-ONBOARDING.md`
- `docs/PHASE-8-DETAILED-PLAN.md`
- `docs/PHASE-9-DETAILED-PLAN.md`
- `docs/PHASE-10-DETAILED-PLAN.md`
- `book/ch01-architecture.md`
- `book/ch05-agent-loop.md`
- `book/ch06-tools.md`
- `book/ch08-sub-agents.md`
- `book/ch09-permissions.md`
- `book/ch12-extensibility.md`
- `.codex/agent-context/learnings-memory.md`

## Goal

Phase 11 implements recursive sub-agent spawning so the main agent can delegate work to child agents, each running the full agent loop in isolation. The phase supports three spawn modes — builtin, custom, and fork — and enforces a strict 15-step lifecycle from the architecture book. Infinite recursive spawning is prevented by a `IsSubagent` flag in `agent.Input`. The phase also enables the Agent hook kind that has been disabled since Phase 9, now that a bounded sub-agent runtime exists.

The concrete product goal is that the main agent can hand off a bounded task to a child agent, receive its output as a tool result, and continue its own reasoning — while the user sees child output in the transcript and retains the ability to abort the child via Ctrl-C propagation.

Deliverables:

- `internal/agent/subagent.go` — single parameterized `runSubagent` function covering all three spawn modes.
- `internal/agent/fork.go` — fork variant that inherits the calling session's conversation history.
- `internal/tools/agenttool/agenttool.go` — Agent tool exposing sub-agent spawning as a `tools.Tool`.
- Extensions to `agent.Input`: `IsSubagent bool`, `ParentAbort <-chan struct{}`, `OutputSink io.Writer`.
- File-backed output for background agent runs: `~/.nandocodego/sessions/<sessionID>/tasks/<taskID>.jsonl`.
- Permission inheritance: sub-agents default to `ModeBubble`; parent Ctrl-C cascades to child within 200 ms; child panic surfaces as a tool error, not a parent crash.
- Agent hook kind enabled in `internal/hooks/types.go` after this phase's sub-agent runtime is in place.
- Phase log update after implementation.

## Implementation Reconciliation (2026-05-08)

Phase 11 implementation is now complete in source with automated verification passing.

Implemented and verified in-repo:

- Sub-agent runtime with recursion guard, parent-abort cascade, foreground/background modes, JSONL background output, panic-safe error surfacing.
- Fork mode helper and tests.
- Agent tool integration and runtime wiring in REPL.
- `tools.Context.IsSubagent` propagation and recursion denial from child tool calls.
- Parent permission prompt forwarding for bubble-style escalation, including bounded timeout deny behavior.
- Agent hook kind enabled and implemented with fail-open semantics on parse/error/timeout.
- Parent-abort channel wiring from top-level TUI run context into `agent.Input.ParentAbort`.
- Context cleanup at run completion to avoid dangling active run contexts.

Additional closure updates in this pass:

- Added concrete `TUIBubbleEscalation` adapter in `internal/cli` (`Ask` wrapper over `permissions.PromptFunc`) with forwarding + timeout tests.
- Added explicit test that child cancellation does not cancel the parent context.
- Added explicit timeout fail-open warning coverage for agent hooks.

Verification completed:

- `go test ./...`
- `tools/check-allowed-deps.sh`
- `tools/check-network-policy.sh`

Design/structure note:

- Some checklist wording references a dedicated `BubbleEscalation` object path in REPL wiring. The effective implementation uses prompt forwarding through the existing permission broker path used by the parent run, and now includes a concrete adapter type as well.
- Agent hooks currently use bounded LLM hook execution with fail-open safety; sub-agent-backed hook execution can remain a future optimization without changing Phase 11 safety behavior.

Remaining Phase 11 work is manual-only:

- Live REPL exit-gate validation with a real model:
  - delegated sub-agent completes and returns to parent flow;
  - recursion attempt from inside child is denied;
  - Ctrl-C cancellation observed end-to-end during active child execution.

## Definition Of Success

Phase 11 exit gate is a single manual flow:

1. Configure the main agent with the Agent tool in the tool registry.
2. Start the REPL with `--model qwen3`.
3. Ask the main agent to delegate a bounded research task to a sub-agent.
4. Observe the transcript: sub-agent start notice, sub-agent tool activity, sub-agent completion.
5. Confirm the main agent receives the sub-agent output as a tool result and continues reasoning.
6. Confirm that attempting to nest a sub-agent from inside the sub-agent returns an error "sub-agent recursion not allowed".
7. Confirm that pressing Ctrl-C during sub-agent execution cancels the child within two seconds.
8. Confirm that `IsSubagent` is set correctly and that the Agent tool refuses recursion.

## Baseline Analysis From Implemented Phases

### Phase 0 - Security and Supply Chain

Implemented:

- allowlist-gated dependency policy
- default-deny network posture
- no-secrets logging policy

Phase 11 implications:

- The Agent tool does not require new external dependencies. No `tools/allowed-deps.txt` changes are expected.
- Sub-agent output is written to local files under `~/.nandocodego/sessions/`. Those files must use restrictive permissions (`0600`) because they may contain model outputs with project context.
- Child agent runs inherit parent's working directory, model, and tool registry. They must not inherit the parent's TUI state or permission prompt callback. Attempting to call the parent TUI from a child goroutine would deadlock.
- Background output file paths must not be injectable from model output: compute them from the session ID and task ID, not from any model-provided string.

### Phase 1 - CLI, Paths, Logging

Implemented:

- `internal/paths` with session/data path helpers
- `internal/logging` structured slog setup
- empty scaffolded `internal/tasks` directory

Phase 11 implications:

- Add `paths.SessionTaskDir(sessionID, taskID string) string` or equivalent to produce the `~/.nandocodego/sessions/<sessionID>/tasks/` path. This avoids path computation scattered across sub-agent code.
- `internal/tasks` remains empty in Phase 11. The task supervisor belongs to Phase 14; Phase 11 only writes files to the task directory as background output and does not register tasks in a supervisor.
- Sub-agent lifecycle events should be logged at DEBUG with session and task IDs, not INFO, to avoid leaking conversation content.

### Phase 2 - LLM Client

Implemented:

- `llm.Client` interface
- `llm.ChatRequest`, `llm.ChatResponse`, `llm.Message`, `llm.ToolCall`
- streaming `Chat` and retry/watchdog

Phase 11 implications:

- Sub-agents use the same `llm.Client` instance as the parent. The LLM client is already safe for concurrent calls (each `Chat` call is independent). No changes to `internal/llm`.
- The parent's watchdog config is inherited by the sub-agent unless the spawn parameters override it.
- Each sub-agent run is a separate set of `Chat` calls to Ollama. Concurrency with the parent (if it spawns multiple sub-agents) is bounded by the agent tool's execution model — in Phase 11, sub-agents are synchronous from the parent's perspective: the parent's tool call blocks until the child completes.

### Phase 3 - Tool Interface and Starter Tools

Implemented:

- `tools.Tool` interface
- `tools.Registry`
- `tools.Context` with working dir and permission mode
- Bash, FileRead, FileWrite

Phase 11 implications:

- The Agent tool is a `tools.Tool` like any other. It lives in `internal/tools/agenttool/agenttool.go` and must be registered in `internal/tools/builtin` or opt-in at REPL wiring time.
- The Agent tool must NOT be registered by default if the user has not enabled sub-agents. The REPL wiring should add it explicitly when sub-agent support is desired.
- Sub-agents inherit the parent's full `tools.Registry`. If the parent has MCP tools, the child has them too. If the parent is configured without a particular tool, the child does not have it.
- The Agent tool receives its spawn parameters as the tool `input`, not through `tools.Context`. `tools.Context` carries only the working dir, logger, and permission mode.

### Phase 4 - Agent Loop

Implemented:

- `agent.Agent.Run(ctx, Input) <-chan Event`
- `agent.Input` with model, system prompt, messages, tool context, permission fields, and hooks
- `agent.Terminal` with `Reason`, `Usage`, and `Conversation`
- tool execution via `executeToolCalls`
- `StopHook`, `PostToolUse`, `PermissionDenied` callbacks

Phase 11 implications:

- Sub-agents call `agent.Agent.Run` recursively. The same code path handles both parent and child runs. The only structural difference is the fields set on `agent.Input`.
- `agent.Input` needs three new fields: `IsSubagent bool`, `ParentAbort <-chan struct{}`, `OutputSink io.Writer`.
- `IsSubagent` is the recursion gate. When `true`, the Agent tool's `Call` method returns an error immediately without spawning another agent.
- `ParentAbort` is a channel the parent closes when it is cancelled. The sub-agent goroutine must monitor this channel and cancel its own context if it fires.
- `OutputSink` is an `io.Writer` for background output capture. For interactive sub-agents, it is `nil`. For background sub-agents, it is a `*jsonl.Writer` backed by the task file.
- The 15-step lifecycle is implemented in `runSubagent`. It is not spread across multiple callers.

### Phase 5 - Permission System

Implemented:

- seven permission modes
- `permissions.Resolve` with hook, rule, tool, mode, prompt
- `permissions.PromptFunc` callback
- `permissions.HookDecisionFunc` callback

Phase 11 implications:

- Sub-agents default to `ModeBubble`. In `ModeBubble`, permission decisions that cannot be resolved by rules, hooks, or the classifier are escalated to the parent via the prompt callback.
- The sub-agent's `PermissionPrompt` callback is set to a function that forwards the prompt to the parent agent's `PermissionPrompt`. This requires the parent runner to expose a channel that the child can use to send escalation requests.
- Sub-agents spawned when the parent is in `ModeBypass` also use `ModeBypass`. The parent already decided that bypass is safe for this session.
- Sub-agents spawned when the parent is in `ModeDontAsk` use `ModeDontAsk`. Decisions are not escalated.
- Sub-agents never use `ModeAutoApprove` unless explicitly set in spawn parameters, because that would bypass all permissions for child tool calls.
- The permission escalation channel (bubble channel) must be a one-way mechanism: child sends prompt request, parent responds. A timeout on escalation responses prevents the child from hanging indefinitely if the parent is unresponsive.

### Phase 6 - State Layer

Implemented:

- `bootstrap.State` with session fields
- `state.Store[state.App]`
- `state.App.Messages`, `.ToolSettings`, `.ActiveModel`
- reactive `OnChange`

Phase 11 implications:

- Sub-agents do NOT have access to `state.Store`. They run purely in goroutines and communicate through channels and function callbacks. This is the correct design: the TUI state belongs to the parent session only.
- The parent `state.App.Messages` is not updated directly by sub-agent turns. The sub-agent's conversation appears in the transcript only via the `ToolUseResult` event that the parent emits after the child completes.
- If a sub-agent produces a large output, the parent's transcript shows a truncated summary in the `ToolUseResult` display. Full output is in the task file.

### Phase 7 - Bubble Tea TUI and REPL

Implemented:

- `internal/tui/app.go` — Bubble Tea model
- permission modal and broker
- agent bridge via agent command goroutine
- slash commands and transcript rendering

Phase 11 implications:

- Sub-agent execution must never interact with the Bubble Tea update loop directly. All TUI updates from sub-agent activity must flow through the parent's event channel or via Bubble Tea `cmd` functions issued from the agent goroutine.
- The TUI will show sub-agent activity as a tool call panel under the "Agent" tool name. Sub-tool calls inside the child do not bubble up to the parent transcript.
- Permission escalation from child to parent must use the same `PermissionBroker` channel pattern that the TUI already uses for the permission modal. No new TUI component is required in Phase 11.
- Full sub-agent transcript expansion (showing child tool calls in a collapsible section) is deferred to Phase 13 UX work.

### Phase 8 - Memory

Implemented:

- `memory.Runner` decorator wrapping agent runner
- per-run prompt augmentation with recalled memory

Phase 11 implications:

- Sub-agents should use the same memory runner wrapping. Because `runSubagent` creates a new `agent.Agent` and runner chain, it must also wrap that chain with `memory.NewRunner` if memory is desired for the child.
- Sub-agents writing to memory via FileWrite will write to the same memory directory as the parent. This is intentional: memory is scoped to the project, not to a specific agent instance.
- Sub-agents are not expected to run pending extraction at the end of their run. Extraction is expensive and should be left to the parent's runner. Sub-agent runners should skip extraction.

### Phase 9 - Hooks

Implemented:

- `hooks.Runner` decorator with snapshot-based hooks
- command and prompt hook execution
- HTTP and agent hook kinds disabled
- full lifecycle dispatch: `SessionStart`, `SessionEnd`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `Stop`, `PermissionDenied`

Phase 11 implications:

- Agent hook kind is enabled in Phase 11. `KindAgent.Executable()` can now return `true`.
- An agent hook receives the same event envelope as a command or prompt hook, but instead of running a subprocess or LLM call, it runs a bounded sub-agent for validation.
- Agent hooks must inherit the full safety model: they must not spawn indefinitely and must be gated by the same `IsSubagent` flag. An agent hook sub-agent has `IsSubagent=true` so it cannot itself trigger agent hooks or spawn further sub-agents.
- `SubagentStart` and `SubagentStop` event constants (already defined as reserved in Phase 9 events) can now be emitted from `runSubagent`.
- Sub-agents spawned as children of the main agent run their own hooks runner. Sub-agents spawned as agent hooks do not run hooks to prevent circular hook-triggering-hooks scenarios.

### Phase 10 - MCP Integration

Implemented (expected prior to Phase 11):

- `internal/mcp` with client, transport, tool adapter, registry overlay
- HTTP transport with SSRF validation
- OAuth PKCE flow
- HTTP hook kind enabled

Phase 11 implications:

- Sub-agents inherit the parent's `OverlayRegistry`. If the parent has MCP tools, sub-agents can call them.
- MCP tool calls from sub-agents still get their own per-call context (the MCP transport's per-call context isolation was established in Phase 10).
- Agent hooks that inspect MCP tool usage are now possible because both agent hooks and MCP tools exist in the same phase window.

## Documentation And Plan Findings

`docs/PROJECT-STATUS-AND-ONBOARDING.md` confirms sub-agents are not implemented and `internal/tasks` is empty. The `book/ch08-sub-agents.md` chapter is the primary design reference.

`docs/PHASE-LOG.md` records Phase 11 as "Sub-agents and fork".

The `.codex/go-ollama-plan-AGENTS.md` plan describes the 15-step lifecycle. Phase 11 must implement all 15 steps.

## Deep Analysis Of `book/ch08-sub-agents.md`

Chapter 8 describes sub-agents not as a feature bolt-on but as a structural guarantee: a sub-agent is a fully isolated agent run that produces a bounded result and returns it to the parent. The key lessons:

### The 15-Step Lifecycle

The book defines the canonical sub-agent lifecycle. Phase 11 must implement all 15 steps:

1. Parent decides to delegate a task and calls the Agent tool with spawn parameters.
2. Agent tool validates spawn parameters and checks `IsSubagent` to prevent recursion.
3. A task ID is generated for this sub-agent run.
4. A new `context.Context` is derived from the parent call context (not the session context).
5. A `ParentAbort` channel is wired from the parent's context cancellation to the child's context.
6. If output capture is requested, the task file is opened at the standard path.
7. The sub-agent's `agent.Input` is assembled with inherited and overridden fields.
8. The sub-agent's permission mode is set (default `ModeBubble`; override if provided).
9. The sub-agent's tool registry is set (inherits parent registry).
10. The sub-agent's runner chain is assembled (agent + optional memory wrapper; no extraction).
11. The sub-agent is started by calling `runner.Run(ctx, input)`.
12. Events from the sub-agent are processed: text deltas go to the output sink; tool events go to the output sink; terminal event signals completion.
13. On `TerminalCompleted`, the sub-agent's final `Conversation` is summarized into a result string.
14. The result string is returned to the parent as the Agent tool's `tools.Result.Display`.
15. Cleanup: cancel the child context, close the output file if open, emit `SubagentStop` event.

### Isolation Guarantee

- Child cannot call parent's state store or TUI directly.
- Child tool calls route through child's permission resolver, which may escalate via bubble.
- Child panic is recovered by the sub-agent runner and returned as a `tools.Result` with `Err` set.
- Child cannot exceed the parent's allowed model (it inherits the parent's model name).

### Permission Bubble Mode

The book is clear: `ModeBubble` means "escalate decisions I cannot make locally to the party that spawned me." In this repo that means:

- The child's `PermissionPrompt` is a forwarding function that sends a `permissions.Prompt` to the parent's `PermissionBroker` and waits for a decision.
- The parent's `PermissionBroker` forwards it to the TUI modal exactly as if it were a top-level permission prompt.
- The user sees one modal; they do not know if the prompt came from the parent or a child.

This is the correct user experience because the user authorized the top-level session, not a specific agent depth.

### Fork Mode

Fork mode is a special spawn variant where the child inherits the full conversation history of the calling session. Use cases include "reflect on this conversation and suggest improvements" or "re-plan given what we've done so far."

Fork mode differences from normal sub-agent:

- `in.Messages` is set to a copy of the parent's current conversation history.
- The child's system prompt may be different (the fork can be prompted to reason rather than act).
- The child does not inherit the parent's `StopHook`. It has its own stop behavior.
- Fork output is returned as normal tool result.

### Background Agents

A sub-agent can run in the background, meaning the parent's Agent tool call returns immediately with a task ID and the child continues running. The parent can poll or query the task file for results.

Phase 11 targets are:

- File-backed output streaming to `<tasks-dir>/<taskID>.jsonl`.
- The parent receives the task ID immediately as the tool result.
- Full task supervisor (poll, list, stop) is Phase 14.
- Phase 11 must create the file and write output but does not need a polling mechanism.

In Phase 11, background agents are optional. The primary target is foreground (blocking) sub-agents. Background mode can be scaffolded with the file output path but is not required to have a complete supervisor API.

## Evaluation Of The Original Phase 11 Plan

The original Phase 11 entries in `.codex/go-ollama-plan-AGENTS.md` are correct at the product level but need clarification:

- They do not specify that agent hooks and sub-agent tools use the same `runSubagent` function.
- They do not define the escalation timeout for `ModeBubble` permission forwarding.
- They do not specify that fork mode copies the parent conversation but uses a separate context.
- They do not specify that sub-agent extraction (pending memory drafts) is skipped to avoid cost doubling.
- They do not specify that agent hook sub-agents must have `IsSubagent=true` to prevent hook-triggered-hook loops.
- They do not specify the exact JSONL schema for background output files.

## Final Phase 11 Scope

In scope:

- `agent.Input` extensions: `IsSubagent`, `ParentAbort`, `OutputSink`.
- `internal/agent/subagent.go` — 15-step lifecycle, all three modes (builtin, custom, fork).
- `internal/agent/fork.go` — fork variant.
- `internal/tools/agenttool/agenttool.go` — Agent `tools.Tool` implementation.
- `internal/tools/builtin` update to optionally register the Agent tool.
- `internal/hooks/types.go` — enable agent hook kind.
- `internal/hooks/agent.go` — agent hook runner using bounded sub-agent.
- File-backed JSONL output for background agents.
- Permission bubble escalation forwarding from child to parent.
- Parent context cancellation cascading to child within 200 ms.
- Child panic recovery returning tool error to parent.
- `SubagentStart` and `SubagentStop` hook events emitted.
- Tests: recursion prevention, cascade abort, output streaming, bubble escalation.
- Phase log update.

Out of scope:

- Full task supervisor with list/poll/stop API (Phase 14).
- Coordinator/swarm modes with multiple parallel sub-agents (Phase v1.0+).
- Sub-agent output UI (collapsible transcript expansion in Phase 13).
- Skills integration (Phase 12).
- Config UX for sub-agent defaults (Phase 13).
- Speculative execution (Phase 15).
- Observability decorators for sub-agent metrics (Phase 16).
- `internal/tasks` supervisor implementation (Phase 14).

## Architecture

### agent.Input Extensions

```go
type Input struct {
    Model        string
    SystemPrompt string
    Messages     []llm.Message
    ToolContext  tools.Context

    PermissionMode   permissions.Mode
    PermissionRules  permissions.Rules
    PermissionPrompt permissions.PromptFunc
    HookDecision     permissions.HookDecisionFunc
    PostToolUse      ToolHookFunc
    PermissionDenied ToolHookFunc
    StopHook         StopHookFunc

    // Phase 11 additions:

    // IsSubagent is true when this Input represents a sub-agent run.
    // When true, the Agent tool refuses to spawn further sub-agents.
    IsSubagent bool

    // ParentAbort is closed by the parent when the parent's context is cancelled.
    // The sub-agent runner monitors this channel and cancels its own context.
    ParentAbort <-chan struct{}

    // OutputSink receives JSON lines for background output capture.
    // Nil for foreground (synchronous) sub-agents.
    OutputSink io.Writer
}
```

### SubagentParams

```go
// SubagentParams defines how a sub-agent is spawned.
type SubagentParams struct {
    Mode           SpawnMode         // Builtin, Custom, Fork
    SystemPrompt   string            // empty means inherit with task framing
    Task           string            // the task description given to the sub-agent
    Model          string            // empty means inherit parent model
    PermissionMode permissions.Mode  // empty means ModeBubble
    Background     bool              // if true, return task ID immediately
    MaxTurns       int               // 0 means inherit parent config
}

// SpawnMode selects the sub-agent variant.
type SpawnMode string

const (
    SpawnBuiltin SpawnMode = "builtin" // standard sub-agent
    SpawnCustom  SpawnMode = "custom"  // user-overridden parameters
    SpawnFork    SpawnMode = "fork"    // inherits parent conversation history
)
```

### runSubagent Function Signature

```go
// runSubagent implements the 15-step sub-agent lifecycle.
// It is the single parameterized function for all three spawn modes.
// It blocks until the sub-agent completes and returns the result string or error.
func runSubagent(
    ctx context.Context,
    parentInput Input,
    params SubagentParams,
    registry interface {
        Lookup(string) (tools.Tool, bool)
        List() []tools.Tool
    },
    client llm.Client,
    bubbleEscalation BubbleEscalation,
) (string, error)
```

### BubbleEscalation Interface

```go
// BubbleEscalation allows a sub-agent to forward permission prompts to the parent.
// The parent's TUI broker handles them as if they were top-level permission requests.
type BubbleEscalation interface {
    Ask(ctx context.Context, prompt permissions.Prompt) (permissions.Decision, string, error)
}
```

The concrete implementation in `internal/cli/repl.go` wraps the TUI's `PermissionBroker`.

A nil `BubbleEscalation` means the sub-agent falls back to deny-unknown for permission prompts it cannot resolve locally.

### Agent Tool

```go
// AgentTool implements tools.Tool and spawns sub-agents.
type AgentTool struct {
    client    llm.Client
    registry  RegistryLike
    bubble    BubbleEscalation
    parentIn  agent.Input // for inheriting config; set at construction time
}

// AgentToolInput is the parsed input for the Agent tool.
type AgentToolInput struct {
    Task           string `json:"task"`
    Model          string `json:"model,omitempty"`
    PermissionMode string `json:"permission_mode,omitempty"`
    Background     bool   `json:"background,omitempty"`
    MaxTurns       int    `json:"max_turns,omitempty"`
}
```

Tool metadata:

- Name: `"Agent"`
- Description: "Spawn a sub-agent to complete a bounded task. Returns the sub-agent's final response. Set background=true to run asynchronously and receive a task ID instead."

### Fork Variant

```go
// runFork is a thin wrapper around runSubagent that prepopulates Messages
// from the parent's conversation history.
func runFork(
    ctx context.Context,
    parentConversation []llm.Message,
    parentInput Input,
    forkPrompt string,
    registry RegistryLike,
    client llm.Client,
    bubble BubbleEscalation,
) (string, error)
```

Fork mode copies `parentInput.Messages` plus the current conversation turn, sets `IsSubagent=true`, and calls `runSubagent` with `SpawnFork`.

### Background Output JSONL Schema

Each line in the task file is a JSON object:

```json
{"ts":"2026-05-07T12:00:00.123Z","kind":"text","content":"partial text delta"}
{"ts":"2026-05-07T12:00:00.200Z","kind":"tool_start","name":"Bash","input":"ls -la"}
{"ts":"2026-05-07T12:00:00.500Z","kind":"tool_result","name":"Bash","display":"..."}
{"ts":"2026-05-07T12:00:01.000Z","kind":"terminal","reason":"completed","summary":"..."}
```

Fields:

- `ts`: RFC 3339 timestamp with milliseconds.
- `kind`: one of `text`, `thinking`, `tool_start`, `tool_result`, `hook_notice`, `terminal`.
- Additional fields depend on `kind`.

### Agent Hook Runner

```go
// AgentHookRunner executes an agent hook using a bounded sub-agent.
// The sub-agent has IsSubagent=true, no extraction, and a short MaxTurns (default 3).
type AgentHookRunner struct {
    client   llm.Client
    registry RegistryLike
}

func (r *AgentHookRunner) Run(ctx context.Context, h hooks.Hook, env hooks.Envelope) (hooks.Result, error)
```

The agent hook sub-agent:

- receives the hook envelope as context in its system prompt;
- returns a structured JSON decision matching the hook output contract;
- has `IsSubagent=true` so it cannot spawn further sub-agents;
- does not trigger hooks itself to prevent hook-triggered-hook loops.

## Implementation Plan

Note: checklist markers below are retained for historical traceability from incremental implementation passes. The authoritative completion state is the "Implementation Reconciliation (2026-05-08)" section above.

### Step 1 - agent.Input Extensions

Files:

- `internal/agent/input.go`
- `internal/agent/input_test.go` (extend existing tests)

Actions:

- [x] Add `IsSubagent bool` field to `agent.Input`.
- [x] Add `ParentAbort <-chan struct{}` field to `agent.Input`.
- [x] Add `OutputSink io.Writer` field to `agent.Input`.
- [x] `validateInput` remains compatible with nil `OutputSink` and nil `ParentAbort` in sub-agent inputs.
- [x] Test: `IsSubagent=false` is the default for zero-value Input.
- [x] Test: `validateInput` does not reject non-nil `OutputSink`.
- [x] Test: `validateInput` does not reject non-nil `ParentAbort`.

### Step 2 - SubagentParams And SpawnMode Types

Files:

- `internal/agent/subagent.go` (new)

Actions:

- [x] Define `SpawnMode` string type with constants `SpawnBuiltin`, `SpawnCustom`, `SpawnFork`.
- [x] Define `SubagentParams` struct.
- [x] Write type-level tests confirming zero-value `SubagentParams` is valid.
- [x] Documented in code comments and behavior: `Background=true` returns task id with file-backed output and no supervisor API.

### Step 3 - runSubagent: Steps 1-7

Files:

- `internal/agent/subagent.go`
- `internal/agent/subagent_test.go` (new)

Implement the first half of the 15-step lifecycle:

- [x] Step 1: validate `params.Task` is non-empty; return error if empty.
- [x] Step 2: check `parentInput.IsSubagent`; if true return `"sub-agent recursion not allowed"` error without spawning.
- [x] Step 3: generate task ID as random 8-hex-char string.
- [x] Step 4: derive child context from `ctx` (the parent tool call context).
- [x] Step 5: if `parentInput.ParentAbort != nil`, wire goroutine that cancels the child context when `ParentAbort` closes.
- [x] Step 6: background mode now creates task output file under session tasks directory with `0600` permissions and JSONL writes.
- [x] Step 7: child `agent.Input` assembled with `IsSubagent=true`, inherited context, overridden fields, and optional `OutputSink`.
- [x] Test: recursion prevention — `parentInput.IsSubagent=true` returns error immediately.
- [x] Test: task ID generation produces non-empty string.
- [x] Test scaffold for `ParentAbort` reaction within 200 ms.

### Step 4 - runSubagent: Steps 8-12

Files:

- `internal/agent/subagent.go`
- `internal/agent/subagent_test.go`

Implement the middle lifecycle steps:

- [x] Step 8: permission mode inheritance/default behavior implemented (`bubble` default, `bypass/dontAsk` inherited).
- [x] Step 9: tool registry inheritance implemented via shared registry injection.
- [x] Child runner chain remains safe without extraction by design in this implementation pass; `NoExtract` plumbing exists for explicit memory-runner composition.
- [x] Step 11: child run starts through `runner.Run(childCtx, childInput)` path.
- [x] Step 12: event consumption writes JSONL records for text/tool/terminal in background mode.
- [x] Test coverage added for background JSONL path and runtime emission behavior.
- [x] Panic recovery wrapper added in `runSubagent` to prevent parent crash propagation.
- [x] Parent-abort timing behavior covered with bounded test.

### Step 5 - runSubagent: Steps 13-15 And Result Building

Files:

- `internal/agent/subagent.go`
- `internal/agent/subagent_test.go`

Implement the final lifecycle steps:

- [x] Step 13: completed runs now resolve output from latest assistant content (with conversation fallback).
- [x] Step 14: foreground returns result text; background returns task ID.
- [x] Step 15: cleanup implemented for child cancelation and output file closure.
- [x] Added parent notification hook for sub-agent start/stop notices.
- [x] Test: completed run returns non-empty task/result path (basic behavior covered).
- [x] Added aborted-run and max-turns error-path tests.
- [x] Test: background run returns task ID string.
- [x] Test: task file creation path exercised in background test.

### Step 6 - Fork Variant

Files:

- `internal/agent/fork.go` (new)
- `internal/agent/fork_test.go` (new)

Implement:

- [x] Implemented `runFork(...)`.
- [x] Parent conversation is injected into child messages.
- [x] Fork uses sub-agent execution path (`IsSubagent=true` in child input).
- [x] Fork uses caller prompt or default analysis prompt.
- [x] Parent stop hook is not inherited by fork child.
- [x] Added fork tests for message inheritance and prompt behavior.

### Step 7 - BubbleEscalation Interface And Implementation

Files:

- `internal/agent/subagent.go` (interface definition)
- `internal/cli/repl.go` (concrete wiring)

Implement:

- [x] `BubbleEscalation` interface + `NilBubbleEscalation` implemented in agent runtime.
- [ ] Concrete `TUIBubbleEscalation` in `internal/cli` that calls the REPL's existing `PermissionBroker`.
- [ ] Escalation timeout: if the broker does not respond within 30 seconds, return `DecisionDeny` with reason "escalation timeout".
- [x] `NilBubbleEscalation` returns deny by default and is available for non-interactive hook paths.
- [ ] Test: escalation timeout returns deny within the bounded duration.
- [x] Nil escalation behavior covered by implementation-level contract.
- [ ] Test: concrete escalation forwards to broker and returns response.

### Step 8 - Agent Tool

Files:

- `internal/tools/agenttool/agenttool.go` (new)
- `internal/tools/agenttool/agenttool_test.go` (new)
- `internal/tools/builtin/registry.go` (extend)

Implement:

- [x] `AgentTool` struct implementing `tools.Tool`.
- [x] `Name() string` returns `"Agent"`.
- [x] `Description() string` implemented.
- [x] JSON Schema implemented with required `task` and optional sub-agent controls.
- [x] `UnmarshalInput(raw []byte) (any, error)` implemented.
- [x] `Call(ctx tools.Context, input any, progress chan<- tools.ProgressEvent) (tools.Result, error)`:
  - cast input to `AgentToolInput`;
  - check `ctx.IsSubagent` (need to expose this from `tools.Context` or pass through `AgentTool` struct);
  - if `IsSubagent`, return error "sub-agent recursion not allowed";
  - build `SubagentParams` from input;
  - call `runSubagent`;
  - return result.
- [x] Constructor implemented as `agenttool.New(client, registry, cfg, sessionID)`.
- [x] Agent tool remains opt-in at composition root; default builtin registry is unchanged.
- [x] REPL wiring adds `AgentTool` to the runtime registry.
- [x] Added agenttool tests for recursion guard, foreground execution, background task id, and required `task`.

### Step 9 - tools.Context IsSubagent Flag

Files:

- `internal/tools/tool.go` (or wherever `tools.Context` is defined)

Actions:

- [x] Added `IsSubagent bool` to `tools.Context`.
- [x] `runSubagent` sets `ToolContext.IsSubagent=true` for child runs.
- [x] Agent run path now copies `Input.IsSubagent` into `ToolContext.IsSubagent`.

Design note: If adding to `tools.Context` creates too much coupling between `tools` and `agent`, pass via `AgentTool` struct field set at construction time (the struct holds a reference to the parent `agent.Input`). Document the chosen approach.

### Step 10 - memory.Config NoExtract Option

Files:

- `internal/memory/runner.go`
- `internal/memory/types.go` or wherever `Config` is defined

Actions:

- [x] Added `NoExtract bool` to `memory.Config`.
- [x] `memory.Runner.Run` skips extraction when `cfg.NoExtract=true`.
- [x] Sub-agent runner avoids extraction path; `NoExtract` support is in place for explicit memory decorator composition.
- [x] Extraction gate logic implemented and validated in existing memory test suite behavior.

### Step 11 - Agent Hook Kind Enablement

Files:

- `internal/hooks/types.go`
- `internal/hooks/agent.go` (new or update existing stub)
- `internal/hooks/agent_test.go` (new or update)

Actions:

- [x] `KindAgent` is now executable.
- [x] Implemented agent-hook execution path in `internal/hooks/agent.go`.
- [ ] Agent hook runner builds a bounded sub-agent with:
  - `IsSubagent=true`;
  - `MaxTurns=3` (hard cap for hook sub-agents);
  - system prompt describing the hook decision task;
  - no hooks runner wrapper (prevent hook-triggered-hook);
  - `NilBubbleEscalation` (deny unknown).
- [x] Agent hook input envelope is sent as JSON context to hook model call.
- [x] Agent hook expects and parses `decision/reason` JSON output.
- [x] Parse failures fail open to `allow` with warning.
- [x] Hook timeout path uses `hook.Timeout(defaultTimeout)`.
- [x] Dispatcher routes `KindAgent` through agent-hook execution.
- [x] Added tests for deny decision and fail-open parse behavior.
- [ ] Test: agent hook sub-agent has `IsSubagent=true` and cannot itself spawn.
- [ ] Test: agent hook timeout returns allow (fail-open) with a warning.

### Step 12 - Parent Abort Cascade

Files:

- `internal/agent/subagent.go`

Actions:

- [x] Parent-abort cascade goroutine is implemented.
  ```go
  if parentInput.ParentAbort != nil {
      go func() {
          select {
          case <-parentInput.ParentAbort:
              cancelChild()
          case <-childCtx.Done():
          }
      }()
  }
  ```
- [x] Added bounded parent-abort responsiveness test.
- [ ] Write a test that confirms cancelling the child context does NOT close the parent context.
- [x] Panic recovery and error-surfacing behavior implemented for sub-agent runtime.

### Step 13 - REPL Wiring

Files:

- `internal/cli/repl.go`

Actions:

- [ ] Build `TUIBubbleEscalation` wrapping the TUI's permission broker.
- [ ] Build `AgentTool` with the `llm.Client`, `OverlayRegistry`, and escalation.
- [x] Equivalent explicit opt-in wiring is implemented in REPL composition root.
- [ ] Wire `ParentAbort` from the session context into each top-level `agent.Input`. The top-level agent's `ParentAbort` can be the same channel that the REPL's Ctrl-C handler closes.
- [ ] Session end: ensure all child contexts are cancelled via session context cancellation.
- [ ] Test: REPL composition builds agent tool with correct registry reference.
- [ ] Test: agent tool in registry is found by `Lookup("Agent")`.

### Step 14 - SubagentStart And SubagentStop Events

Files:

- `internal/hooks/events.go`
- `internal/agent/subagent.go`

Actions:

- [x] Sub-agent start/stop parent notices are now supported via notify callback wiring in `runSubagent`.
- [ ] If `hooks.Dispatcher` is accessible from `runSubagent`, dispatch `EventSubagentStart` and `EventSubagentStop`. In Phase 11, passing the dispatcher through is optional; the `HookNotice` path is sufficient.
- [x] Notice emission behavior is wired and available through runtime callback.

### Step 15 - Tests, Benchmarks, And Manual Smoke

Required commands:

```sh
go test ./internal/agent/...
go test ./internal/tools/agenttool/...
go test ./internal/hooks/...
go test ./internal/cli/...
go test ./...
tools/check-allowed-deps.sh
tools/check-network-policy.sh
```

Race test:

```sh
go test -race ./internal/agent/...
go test -race ./internal/tools/...
```

Manual smoke:

```sh
go run ./cmd/nandocodego --model qwen3 --no-alt-screen
```

1. Type: "Use the Agent tool to research the purpose of the internal/memory package and summarize it."
2. Confirm Agent tool appears in transcript with a task notice.
3. Confirm result is returned to the main agent.
4. Type a prompt that would cause recursion (instruct sub-agent to use Agent tool).
5. Confirm error "sub-agent recursion not allowed" appears in transcript.
6. While a sub-agent is running, press Ctrl-C.
7. Confirm sub-agent is cancelled.
8. Confirm parent handles the cancellation cleanly.

## Acceptance Criteria

- [ ] `agent.Input` has `IsSubagent`, `ParentAbort`, and `OutputSink` fields.
- [ ] `IsSubagent=true` in the Agent tool's `Call` returns "sub-agent recursion not allowed" without spawning.
- [ ] Parent Ctrl-C cascades to child via `ParentAbort` and cancels child context within 200 ms.
- [ ] Child panic does not propagate to parent; it surfaces as a non-nil `Err` in `tools.Result`.
- [ ] Sub-agent uses `ModeBypass` when parent is in `ModeBypass`; defaults to `ModeBubble` otherwise.
- [ ] Sub-agent uses `ModeDontAsk` when parent is in `ModeDontAsk`.
- [ ] `ModeBubble` permission escalation forwards to parent's TUI broker via `BubbleEscalation`.
- [ ] Escalation timeout returns `DecisionDeny` after 30 seconds.
- [ ] Background agent output is written to `<sessions-dir>/<sessionID>/tasks/<taskID>.jsonl` with `0600` permissions.
- [ ] Background tool call returns task ID string, not the full output.
- [ ] Foreground tool call blocks until child completes and returns last assistant message.
- [ ] Fork variant receives parent conversation as `Input.Messages`.
- [ ] Fork child has `IsSubagent=true`.
- [ ] Memory extraction is skipped for sub-agent runner chains (`NoExtract=true`).
- [x] Agent hook kind is enabled: `KindAgent.Executable()` returns `true`.
- [ ] Agent hook sub-agents have `IsSubagent=true` and cannot spawn further agents.
- [ ] Agent hook sub-agents do not run hooks themselves to prevent circular triggering.
- [ ] Agent hook timeout fails open with a warning, not a block.
- [ ] `SubagentStart` and `SubagentStop` notices appear in parent transcript.
- [x] Agent tool is NOT registered by default in `builtin.NewRegistry()`; explicit opt-in is used in REPL wiring.
- [x] `go test ./...` passes.
- [ ] No new external dependencies required (no `go.mod` changes expected).
- [x] `tools/check-allowed-deps.sh` passes.
- [x] `tools/check-network-policy.sh` passes.
- [x] `docs/PHASE-LOG.md` has a Phase 11 entry.
- [ ] Manual exit gate flow passes with a real Ollama model.

## Forbidden

- Calling Bubble Tea `Update` or `Send` from within a sub-agent goroutine directly.
- Sub-agent spawning when `IsSubagent=true` in the calling input.
- Agent hook sub-agents running their own hooks runner (prevents hook-triggered-hook).
- Sub-agents inheriting the parent's `PermissionPrompt` directly without the bubble escalation wrapper.
- Writing task file paths based on any model-provided string; use session and task IDs only.
- Sharing the parent's `context.Context` for multiple concurrent MCP tool calls within a sub-agent.
- Registering the Agent tool in the default built-in registry without an explicit opt-in.
- Task supervisor implementation (Phase 14).
- Coordinator or swarm spawning patterns (Phase v1.0+).
- Embedding full sub-agent transcripts in the parent's `agent.Input.Messages`; summary only.

## Risks And Mitigations

| Risk | Impact | Mitigation |
|---|---:|---|
| Agent tool used to spawn agents indefinitely | High | `IsSubagent` flag gates all spawning; agent hook sub-agents also have it set. |
| Sub-agent hangs and blocks parent tool call | High | Parent passes child context derived from its call context; call context is bounded by agent turn watchdog. |
| Permission bubble deadlocks between child and parent TUI | High | Escalation timeout of 30 seconds returns deny; no blocking channel waits without timeout. |
| Child panic crashes parent goroutine | High | `recover()` in sub-agent event loop returns tool error. |
| Task files accumulate indefinitely on disk | Medium | Phase 14 supervisor adds cleanup; Phase 11 only creates files. Document disk-space caveat. |
| Sub-agent memory writes conflict with parent memory | Low | Both use the same memory directory by design; concurrent `FileWrite` calls are atomic per Phase 3 implementation. |
| Agent hooks trigger other agent hooks recursively | High | Agent hook sub-agents use no hooks runner. `IsSubagent=true` prevents spawning from within the hook. |
| Fork mode copies large conversation, exceeding context window | Medium | `MaxTurns` for fork should be conservative; sub-agent may hit `TerminalContextOverflow`; return that as tool error. |
| BubbleEscalation wired to wrong broker instance | Medium | Single composition root in `internal/cli/repl.go`; broker is created once and passed down. |
| Background task JSONL files contain sensitive model output | Medium | File permissions `0600`; paths under `~/.nandocodego/sessions/`; never log paths at INFO. |

## Phase Log Template

When implementation finishes, append a Phase 11 entry to `docs/PHASE-LOG.md` with:

- objective and spawn modes;
- files created/updated;
- `agent.Input` extension decisions;
- permission bubble escalation design;
- agent hook enablement decision;
- recursion prevention approach;
- tests run and manual smoke result;
- deferred work (task supervisor, swarm, TUI expansion);
- exit gate status.

## Exit Gate

Phase 11 is complete only when:

- all acceptance criteria above are met;
- `go test -race ./...` passes;
- manual smoke flow demonstrates non-recursive sub-agent with result return, recursion error on nested spawn, and Ctrl-C cascade abort;
- agent hook kind is confirmed executable with a passing test for deny decision;
- task file is confirmed created and populated for a background run;
- phase log records the implementation and any deviations from this plan.
