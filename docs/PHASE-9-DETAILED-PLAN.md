# Phase 9 Detailed Plan - Hooks

Date: 2026-05-03
Status: Core implementation landed; manual exit-gate validation pending
Source plans and references:

- `.codex/go-ollama-plan-AGENTS.md`
- `.codex/go-ollama-plan-HUMANS.md`
- `docs/PHASE-LOG.md`
- `docs/PROJECT-STATUS-AND-ONBOARDING.md`
- `docs/PHASE-8-DETAILED-PLAN.md`
- `book/ch09-permissions.md`
- `book/ch12-extensibility.md`
- `book/ch13-terminal-ui.md`
- `.codex/agent-context/learnings-memory.md`

## Goal

Phase 9 implements a snapshot-based hook system that can intercept lifecycle events and influence behavior safely.

The concrete product goal is:

- A user can declare hooks in config.
- Hooks are snapshotted at session start.
- Hook decisions can block or modify operations at key lifecycle points.
- Mid-session file edits do not silently change active hook behavior.

Primary target in this phase:

- `PreToolUse` and `PostToolUse` are fully wired.
- `SessionStart`, `SessionEnd`, `UserPromptSubmit`, and `Stop` are wired at the framework level.
- Command and prompt hook types are executable.
- HTTP and agent hook types are represented in config/type validation but remain disabled by default until their extra security/runtime prerequisites are present.

## Definition Of Success

Phase 9 is complete when this end-to-end flow works:

1. A configured command hook matching `Bash(rm -rf*)` exits with code `2` and stderr `"denied by policy"`.
2. User runs REPL with permission mode `dontAsk`.
3. Model attempts matching command.
4. Tool call is blocked before execution by hook decision.
5. Model-visible message includes `"denied by policy"`.
6. Editing the hook file during the same REPL session has no effect until restart.

## Implementation Status

Phase 9 core implementation is now present in `internal/hooks` and wired through the REPL composition root, agent tool path, permission resolver bridge, and TUI transcript notices.

Automated validation completed:

- hook snapshot, matcher, command, prompt, dispatch, and runner tests;
- agent/permission/TUI/CLI integration slices;
- full `go test ./...`;
- dependency allowlist check;
- hardcoded endpoint/network policy check.

Manual REPL validation remains the final exit-gate item because it requires a live model to attempt the blocked `Bash(rm -rf*)` tool call and verify user-visible/model-visible behavior end to end.

## Why This Phase Matters

Hooks are the first control-plane extension that can enforce local policy independently of prompt quality.

Without hooks:

- permission modes are static policy profiles;
- per-tool rules are pattern-only;
- there is no lifecycle-aware policy program.

With hooks:

- teams can encode guardrails around dangerous operations;
- policy can be context-sensitive and event-sensitive;
- the system can require post-checks before claiming completion.

## Deep Analysis Of `book/ch12-extensibility.md`

Chapter 12 is a design warning as much as a feature description. It separates extensibility into two dimensions:

- skills extend what the model can do;
- hooks control when behavior is allowed to proceed.

Phase 9 must implement the second dimension only. Skill loading, skill-declared hooks, dynamic skill discovery, and MCP skill security are Phase 12/15 concerns. Pulling them into Phase 9 would create a mixed extension system before the repo has the command registry, sub-agent runtime, MCP boundary, or config provenance to support it.

The chapter's hook-specific lessons that should directly shape Phase 9 are:

- **Freeze hook config at the trust boundary.** Hooks execute arbitrary code, so active hook config must be a snapshot, not live file reads.
- **Exit code `2` is the only blocking command-hook exit.** Exit `1` is too common and must stay warning-only.
- **Blocking support is asymmetric.** `PreToolUse`, `UserPromptSubmit`, and `Stop` can block. `SessionStart`, `SessionEnd`, and fact-reporting events should not block.
- **Project hooks need explicit trust.** The book assumes a workspace trust dialog. This repo does not have one yet, so project hook execution must not be on by default.
- **Source precedence matters.** Policy sources should eventually override user/project sources, but Phase 9 should not invent the whole Phase 13 config system.
- **Hook output is untrusted model input.** Stderr that becomes model-visible context must be bounded and redacted.
- **HTTP hook validation must happen at connection time.** The book calls out DNS rebinding. Phase 9 should not ship broad HTTP hooks unless it can enforce destination policy in the dial path.
- **Internal callback/function hooks are different from user hooks.** They are useful future optimization points, but user-configurable hooks should not depend on that machinery.

### Corrections Applied To The Initial Phase 9 Plan

The initial Phase 9 draft was too broad in three places:

- It treated project hook files as executable by default. This is unsafe without workspace trust.
- It treated HTTP hooks as normal Phase 9 runtime scope. That requires SSRF-grade destination validation not yet present in the repo.
- It treated agent hooks as a full hook kind before Phase 11 sub-agents exist.

The corrected plan keeps the public model and enum surface broad enough for future compatibility, but narrows executable Phase 9 behavior:

- user-level command hooks: executable;
- user-level prompt hooks: executable with `llm.Client` and structured output;
- project hooks: parsed and reported, but execution disabled unless a future trust/config gate enables them;
- HTTP hooks: type exists, disabled by default;
- agent hooks: type exists, disabled until Phase 11 provides a bounded agent runtime.

## Analysis Of Implemented Phases And Their Phase 9 Implications

### Phase 0 - Security Baseline

Relevant constraints already in repo:

- default-deny external network posture;
- no silent secret logging;
- explicit trust boundary around user/project-controlled files.

Phase 9 implications:

- hooks execute arbitrary code, so logs must not include hook stdin payloads or raw stderr at INFO;
- HTTP hooks must be explicit opt-in and remain off by default;
- hook output should be treated as untrusted input.

### Phase 1 - CLI, Paths, Logging

Relevant implemented pieces:

- `internal/paths` provides user/data/config roots;
- REPL startup path is centralized in `internal/cli/repl.go`;
- structured logging exists.

Phase 9 implications:

- hook snapshot should be built in REPL startup path (composition root);
- hook config search can use `paths.ConfigDir()` plus project working directory;
- if hook diagnostics are logged, use concise metadata only.

### Phase 2 - LLM Client

Relevant pieces:

- provider-neutral `llm.Client`;
- structured output via `ChatRequest.Format`;
- non-stream and stream chat support.

Phase 9 implications:

- prompt hooks should use `llm.Client`, not provider-specific calls;
- future agent hooks should also use `llm.Client` after Phase 11 provides the bounded runtime;
- prompt hook response contract should require structured JSON output.

### Phase 3 - Tool Surface

Relevant pieces:

- tool registry and metadata;
- tool call path through `executeToolCalls`.

Phase 9 implications:

- hook event payloads should include tool name, resolved target, and parsed input summary;
- `PreToolUse` should run before permission prompt/decision is finalized.

### Phase 4 - Agent Loop

Relevant pieces:

- event stream architecture;
- turn lifecycle boundaries;
- terminal event and now persisted conversation payload.

Phase 9 implications:

- `Stop` hook can be inserted before terminal completion emission;
- hook execution belongs in agent runtime path or decorators, not TUI `Update`.

### Phase 5 - Permission Resolver

Most important integration seam:

- `permissions.Request.HookDecision` already exists and runs before rules/tool classifier/mode.

Phase 9 implications:

- integrate `PreToolUse` through `HookDecision` callback;
- precedence remains deterministic: hook deny overrides later allow;
- `ask` from hook should still route through existing prompt callback.

### Phase 6 - State Layer

Relevant pieces:

- store mirrors infra fields into bootstrap via `OnChange`;
- no heavy I/O in `OnChange`.

Phase 9 implications:

- do not execute hooks from `state.OnChange`;
- state should only reflect hook outcomes (for transcript/system items), not run hook commands.

### Phase 7 - TUI and REPL

Relevant pieces:

- normal prompt submission path exists;
- permission modal and transcript system items exist.

Phase 9 implications:

- hook blocks/warnings can surface via existing system transcript items;
- no network/process execution from Bubble Tea update loop.

### Phase 8 - Memory Core

Relevant pieces:

- runtime decorators are now used (`memory.NewRunner`);
- conversation persistence improved for downstream phases.

Phase 9 implications:

- hook runner should follow the same composition-root/decorator philosophy;
- hook decisions and memory behavior must coexist without circular coupling.

## Original Gaps Addressed By Phase 9

- `internal/hooks` now contains the core config model, parser, matcher, snapshot, dispatcher, runners, and tests.
- Hook lifecycle dispatch exists through the hook runner decorator.
- `PreToolUse` is wired into the permission resolver through `permissions.Request.HookDecision`.
- `/hooks` commands are still intentionally deferred because Phase 13 owns command/config UX.

## Scope

In scope:

- hook event constants and typed payloads;
- hook config parsing and snapshot loading at session start;
- command hook execution;
- prompt hook execution through `llm.Client`;
- HTTP and agent hook type definitions with disabled-by-default validation paths;
- pre-tool hook decision bridge to permission resolver;
- post-tool and stop event dispatch;
- deterministic precedence and exit-code semantics;
- tests for snapshot freeze, command blocking, and ask propagation.

Out of scope:

- hot reload via file watcher;
- `/hooks reload` slash command UX;
- advanced trust UI/consent flow;
- enterprise policy server;
- default execution of project-controlled hook files;
- broad HTTP hook execution;
- full agent hook execution before Phase 11 sub-agent runtime exists;
- hooks for MCP/sub-agents not yet implemented;
- telemetry pipeline (Phase 16).

## Hook Model

### Hook Kinds

- `command`: local subprocess receives event JSON on stdin.
- `prompt`: one bounded LLM call returning decision JSON.
- `agent`: reserved for bounded multi-turn validation after Phase 11.
- `http`: reserved for remote policy/audit hooks after SSRF-safe HTTP validation is implemented.

Phase 9 executable kinds:

- `command`
- `prompt`

Phase 9 recognized-but-disabled kinds:

- `agent`
- `http`

### Event Model

Phase 9 should define the full event enum now for future compatibility, even if only a subset is emitted immediately.

Core events to wire now:

- `SessionStart`
- `SessionEnd`
- `UserPromptSubmit`
- `PreToolUse`
- `PostToolUse`
- `Stop`
- `PermissionDenied`

Additional event constants reserved for later phases:

- `SubagentStart`, `SubagentStop`
- `PreCompact`, `PostCompact`
- `MemoryRead`, `MemoryWrite`
- notification/task/config lifecycle events

### Decision Model

```go
type Decision string

const (
    DecisionAllow Decision = "allow"
    DecisionDeny  Decision = "deny"
    DecisionAsk   Decision = "ask"
)
```

Decision precedence when multiple hooks match the same event:

- `deny` wins over `ask` and `allow`;
- `ask` wins over `allow`;
- if no explicit decision, keep existing behavior.

### Command Hook Exit Codes

- `0`: pass, optional structured stdout parsed for enrichments.
- `2`: blocking decision; stderr becomes model-visible/system-visible reason.
- any other non-zero: warning only; do not block.

This is non-negotiable in this phase to avoid accidental blocking on generic script failures.

## Proposed Package Layout

```text
internal/hooks/
  events.go
  types.go
  snapshot.go
  matcher.go
  input.go
  result.go
  runner.go
  command.go
  prompt.go
  http.go
  agent.go
  dispatch.go
  integration.go

  events_test.go
  snapshot_test.go
  matcher_test.go
  command_test.go
  prompt_test.go
  http_test.go
  runner_test.go
  integration_test.go
```

Keep boundaries:

- parsing/snapshot are pure data;
- runners execute effects;
- integration package wires into agent/permissions.

## Config and Snapshot Strategy

Phase 13 will own full config provenance. Phase 9 still needs safe loading now.

Interim loading strategy for Phase 9:

1. Read hook config once at REPL start from:
   - `~/.nandocodego/hooks.json` (user, executable)
   - `<project>/.nandocodego/hooks.json` (project, parsed but disabled by default)
2. Merge executable hooks from trusted sources only.
3. Preserve disabled project hooks in snapshot diagnostics so users can see why they did not run.
4. Freeze snapshot in memory for session lifetime.

Rules:

- no implicit reload;
- parsing errors are non-fatal but surfaced as startup warnings;
- malformed hooks are dropped with reason.
- project hook execution requires a future workspace-trust/config gate and must not be enabled just because a repo contains `.nandocodego/hooks.json`.

When Phase 13 config loader lands, swap source loader only, not execution core.

Format note:

- `.codex` references `hooks.toml`, but this repo does not yet have the Phase 13 config loader or TOML dependency.
- Phase 9 should use JSON for the interim hook config because it is supported by the standard library and keeps dependency policy stable.
- Phase 13 may migrate hook config into the final TOML/config model.

## Integration Architecture

### Composition Root

In `internal/cli/repl.go`:

1. Build base agent.
2. Wrap with memory runner (already present).
3. Wrap with hooks runner (new).
4. Pass wrapped runner to TUI.

Order recommendation:

- memory wraps the base agent to inject memory prompt context;
- hooks wrap the memory runner for lifecycle visibility;
- tool gating itself still enters through the permission resolver callback inside the agent tool path.

### Permission Resolver Bridge

Inside `internal/agent/tools.go`, `permissions.Resolve` is already called with `HookDecision: nil`.

Phase 9 action:

- inject callback from hooks runtime:
  - on `PreToolUse`, execute matching hooks;
  - convert hook decision into `permissions.Result` when decisive;
  - map hook `ask` into permission ask path.

### PostToolUse and Stop

- after each tool completion, dispatch `PostToolUse` hooks asynchronously with bounded timeout.
- before `TerminalCompleted`, dispatch `Stop` hooks; blocking result can force continuation path.

For initial Phase 9, if `Stop` hook denies completion:

- emit system event in transcript;
- terminate with explicit `stop_hook` reason unless the implementation also adds a tested bounded continuation loop.
- do not invent an unbounded continuation loop in Phase 9.

## Data Contracts

### Hook Input Envelope

All hook kinds should receive a common envelope:

```json
{
  "event": "PreToolUse",
  "timestamp": "2026-05-03T23:30:00Z",
  "session_id": "session-...",
  "tool": {
    "name": "Bash",
    "target": "rm -rf build",
    "input_summary": "..."
  },
  "conversation_summary": "...",
  "metadata": {
    "model": "qwen3",
    "permission_mode": "dontAsk"
  }
}
```

### Hook Output Contract

Normalized result shape:

```json
{
  "decision": "allow|deny|ask",
  "reason": "optional human-readable reason",
  "updated_input": null,
  "additional_context": "optional"
}
```

Not every hook kind must support `updated_input` in Phase 9. If not supported, ignore with warning.

## Security and Safety Requirements

- command hooks run with bounded timeout and inherited environment subset only.
- never log full hook stdin payload at INFO.
- redact common secret patterns in stderr/system messages before rendering.
- project hooks are disabled by default until a trust gate exists.
- HTTP hooks remain disabled unless an implementation adds DNS-rebinding-safe destination validation.
- HTTP destination validation must reject private/loopback exceptions unless explicitly allowed by policy and must validate in the dial path, not only as a preflight URL check.
- hook parse/exec failures must fail closed for `PreToolUse` only when hook explicitly blocks; generic execution failures should not become silent allow for matched blocking policies.

## Detailed Implementation Steps

### Step 1 - Hook Core Types and Events

Create:

- `events.go`: event enum and validation.
- `types.go`: hook kind, matcher, config structures.
- `result.go`: normalized execution results.

Acceptance:

- invalid kind/event rejected in unit tests.

### Step 2 - Snapshot Loader

Create:

- `snapshot.go` with `LoadSnapshot(...)`.

Behavior:

- loads once at startup;
- merges sources;
- freezes immutable runtime view.

Acceptance:

- modifying source files mid-session does not affect active snapshot.

### Step 3 - Matcher Engine

Create:

- `matcher.go`.

Behavior:

- event match required;
- optional tool matcher supports glob-like patterns (`Bash(rm -rf*)` style).

Acceptance:

- deterministic matcher tests for positive/negative cases.

### Step 4 - Command Hook Runner

Create:

- `command.go`.

Behavior:

- stdin receives hook input JSON;
- parse exit code semantics (`0`, `2`, others);
- bounded timeout.

Acceptance:

- exit `2` blocks with stderr reason.

### Step 5 - Prompt Hook Runner

Create:

- `prompt.go`.

Behavior:

- one `llm.Client.Chat` call with structured JSON output;
- timeout and JSON parse guard.

Acceptance:

- invalid JSON becomes warning, not panic.

### Step 6 - HTTP Hook Runner

Create:

- `http.go`.

Behavior:

- define type and validation contract;
- return disabled-kind warning by default;
- do not perform network calls until destination validation is implemented.

Acceptance:

- configured HTTP hook is rejected or skipped with a clear disabled-kind reason.

### Step 7 - Agent Hook Runner

Create:

- `agent.go`.

Behavior for Phase 9:

- define type and validation contract;
- return disabled-kind warning by default;
- defer execution until Phase 11 provides a bounded agent runtime.

Acceptance:

- configured agent hook is rejected or skipped with a clear disabled-kind reason.

### Step 8 - Dispatch and Precedence

Create:

- `dispatch.go`, `runner.go`.

Behavior:

- execute matched hooks (parallel where safe, then aggregate);
- apply precedence `deny > ask > allow`;
- preserve richest blocking reason.

Acceptance:

- precedence tests pass.

### Step 9 - Agent/Permission Integration

Patch:

- `internal/agent/tools.go` to pass `HookDecision` callback into `permissions.Resolve`.
- optional lifecycle instrumentation in agent loop for `SessionStart`, `SessionEnd`, `Stop`.

Acceptance:

- matched blocking hook prevents tool execution before tool call.

### Step 10 - TUI/UX Surface

Patch:

- append concise system transcript item when hook blocks or warns.

Acceptance:

- user can see why a call was blocked without debug logs.

### Step 11 - Tests and Verification

Required:

```sh
go test ./internal/hooks/...
go test ./internal/agent/... ./internal/permissions/... ./internal/tui/... ./internal/cli
go test ./...
tools/check-allowed-deps.sh
tools/check-network-policy.sh
```

Manual:

1. Create hook config that blocks `rm -rf*` for Bash.
2. Run REPL with `--no-alt-screen`.
3. Trigger blocked command through model/tool call.
4. Verify deny reason appears.
5. Edit user hook config mid-session to allow and retry; verify still blocked.
6. Restart REPL and verify new snapshot behavior applies.
7. Add project hook config and verify it is reported but not executed by default.

## Acceptance Checklist

- [x] `internal/hooks` package exists with events, types, snapshot, matcher, runners, dispatch, and integration seams.
- [x] Hook snapshot loaded once at session start.
- [x] Mid-session hook file edits do not alter active session behavior.
- [x] Project hook files are not executable by default without a trust gate.
- [x] `PreToolUse` integration through `permissions.Request.HookDecision` is active.
- [x] Command hook exit code `2` blocks and forwards stderr reason.
- [x] Prompt hook `ask` decision triggers existing permission prompt path.
- [x] HTTP and agent hook kinds are disabled with deterministic warnings until their prerequisites exist.
- [x] Hook warning/failure behavior is deterministic and tested.
- [x] No hook logic runs in Bubble Tea `Update`.
- [x] Tests pass without live Ollama for core hook unit/integration suites.
- [ ] Manual REPL blocking demo with live Ollama is complete.
- [ ] Manual project-hook disabled diagnostic is confirmed in the REPL.

## Forbidden In Phase 9

- implicit hook hot reload;
- using exit code `1` as blocking signal;
- writing hook stdout/stderr to INFO logs verbatim;
- bypassing existing permission resolver flow;
- introducing full config subsystem in hooks package;
- coupling hook execution to UI thread/update path.
- executing project-controlled hooks without an explicit trust gate;
- enabling HTTP hooks without socket-level destination validation;
- implementing multi-turn agent hooks before Phase 11.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---:|---|
| Hook scripts become arbitrary remote code gateway | High | Snapshot-only loading, explicit file sources, no implicit reload, strict logging redaction. |
| Project hook config executes from an untrusted repo | High | Parse but disable project hooks until workspace trust/config provenance exists. |
| Blocking semantics accidentally trigger on generic script errors | High | Reserve block strictly for exit `2`; treat other non-zero as warning. |
| Hook latency degrades REPL responsiveness | Medium | Per-hook timeout, bounded parallelism, fail-open warnings where policy does not explicitly block. |
| Decision conflicts across multiple hooks | Medium | Deterministic precedence and aggregate-reason policy with tests. |
| HTTP hook SSRF risk | High | Keep HTTP disabled until destination validation happens in the dial path. |
| Stop hooks create infinite continuation loops | Medium | Max continuation budget and terminal fallback reason. |

## Phase Log Update Template (When Implementing)

When Phase 9 lands, append an entry to `docs/PHASE-LOG.md` with:

- objective and scope;
- files added/changed;
- hook sources and precedence;
- exit code behavior confirmation;
- test matrix and manual demo notes;
- deferred work and known limits.

## Exit Gate

Phase 9 closes only when:

- acceptance checklist above is complete;
- blocking demo (`rm -rf` denied in `dontAsk`) is validated;
- snapshot immutability test is passing;
- phase log is updated with implementation evidence.
