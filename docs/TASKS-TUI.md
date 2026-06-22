# TUI Implementation Task Plan

Generated from `ADR-001-TUI-USER-EXPERIENCE-IMPROVEMENTS.md` and
`THINKING-VISIBILITY-PLAN.md` with the design decisions captured below.

## Roadmap Placement

This task plan is required Phase 22 input for v0.1. Phase 22 must land before Phase 21 server work, Phase 25 remote/bridge mode, Phase 17 packaging, and Phase 18 hardening. The purpose is to make the local ask/response loop visibly responsive and diagnosable before new transport surfaces reuse or mirror the interaction model.

The required Phase 22 order from this file is:

1. Close remaining Phase 0 backlog item P0-15 unless a fresh implementation review proves it is already handled.
2. Implement UI-0 modal and permission rendering correctness.
3. Implement UI-1 animated run status.
4. Implement UI-5 semantic style roles before deeper render changes.
5. Implement UI-2, UI-3, and UI-4.
6. Implement UI-6 hierarchical activity display.
7. Implement UI-7 `/btw` side question last.

## 2026-05-17 Agent Execution Addendum

This addendum makes the plan directly executable by agents. It reconciles the ADR task slices with the broader Phase 22 input/render work in `docs/PHASE-22-DETAILED-PLAN.md`.

### Current Preconditions

- Workstream CL code work before Phase 22 is complete for v0.1 scope: trace, adaptive context, checkpoint hardening, retrieval, and bounded project-analysis workflow are implemented.
- Phase 26 inline completion is already delivered and should not be reimplemented here.
- P0-15 remains open unless implementation review proves otherwise: `/memory edit` still launches `$EDITOR` from `commands.Registry`, where Bubble Tea cannot safely release the terminal on its own.

### Implementation Batches

| Batch | Contains | Dependency | Agent ownership |
| --- | --- | --- | --- |
| P22-A Safety fixes | P0-15, UI-0-1 through UI-0-5 | none | one agent for command/editor flow, one agent for modal/picker flow if parallelized |
| P22-B Run status | UI-1-1 through UI-1-4 | P22-A preferred | single owner for `runstate.go` and `app.go` |
| P22-C Style and snapshots | UI-5-1 plus test fixture helper | P22-B types available | single owner for `styles.go` and `testdata` helper |
| P22-D Status details | UI-2, UI-3, UI-4 | P22-B, P22-C | can split `/queue` command from modal/status rendering |
| P22-E Transcript performance | virtual transcript, sticky scroll, render cache, benchmarks | P22-C | single owner for `transcript.go`; integration owner for `app.go` |
| P22-F Input system | bracketed paste, contexts, chords, Vim expansion, transcript search | P22-E for scroll/search integration | split Vim library work from keybinding/app integration |
| P22-G Activity hierarchy | UI-6-1 through UI-6-9 | P22-B, P22-E | sequential within UI-6 |
| P22-H Side workflows | UI-7 plus `/bg` completion | P22-G | single owner because it crosses tools, agent input, commands, and TUI |
| P22-I Verification | docs, user manual, race tests, benchmarks, manual evidence | all prior | verification owner |

### Agent Work Rules

- Each worker must keep edits inside its assigned batch. If a required change crosses into another batch, record a follow-up instead of silently expanding scope.
- `internal/tui/app.go` is the conflict hotspot. Only one active worker should own it at a time unless write areas are explicitly separated.
- Snapshot fixtures must be stable and width-specific. Prefer 60, 80, and 120 columns for status/footer views.
- Tests must cover state transitions without requiring a live terminal. Manual checks are still required for the final gate.
- No Phase 22 task may add a new dependency unless `tools/allowed-deps.txt` is updated intentionally and the dependency checker passes.

### New Quality Gates Per Batch

| Batch | Minimum automated gate |
| --- | --- |
| P22-A | `go test ./internal/commands ./internal/tui ./internal/tui/picker` |
| P22-B | `go test ./internal/tui -run 'RunUI|Status|Tick|Phase'` plus `go test ./internal/tui` |
| P22-C | `go test ./internal/tui -run 'Style|Snapshot|Status'` |
| P22-D | `go test ./internal/commands ./internal/tui ./internal/tui/picker` |
| P22-E | `go test ./internal/tui -run 'Virtual|Transcript|Sticky|Render'` and `go test -bench=BenchmarkView1000Items ./internal/tui` |
| P22-F | `go test ./internal/tui -run 'Vim|Paste|Keybinding|Chord|Search'` |
| P22-G | `go test ./internal/agent ./internal/tui -run 'AssistantTurn|Group|Tree|Tip|Footer'` |
| P22-H | `go test ./internal/tools ./internal/agent ./internal/commands ./internal/tui -run 'ReadOnly|Background|Btw|BG'` |
| P22-I | `go test ./...`, `go test -race ./internal/tui/...`, render benchmarks, manual exit gate |

### Batch Exit Template

Each agent must append or report:

```text
Batch:
Files changed:
Tests run:
Manual checks still pending:
Risks:
Follow-up tasks:
```

## Design Decisions (locked)

| Decision | Choice |
| :--- | :--- |
| Bug-fix sequencing | All bugs (A1–A7, C1–C13) land first as **Phase 0** before any UI phase |
| UI-2 scope | Reduced to elapsed-time-only on existing tool panels; UI-6 supersedes the strip |
| `RunPhase` ownership | TUI-local in `internal/tui.Model` with an exported read-only `RunSnapshot()` accessor |
| Always-allow rule scope | `A` adds a rule matching the literal modal target, not `Tool(*)` |
| Side commands in scope | `/queue`, `/btw`, `/bg` all in scope |
| Theme work | Semantic style refactor only; no high-contrast or mono themes in this pass |
| Spinner / verb / tip config | Full config surface (verb lists, tip catalogue, tick interval, ASCII fallback) |
| Empty-state after `/clear` | Small variant: single-line "transcript cleared — type a new prompt" |
| Background tool slots | One slot |
| Background tool output | Inline one-line summary in transcript + full detail in `/bg view` |
| `AssistantTurnStarted` event | New event added to `internal/agent/events.go` |
| Git branch refresh | TTL-only, 2s cache |
| `/btw` context | Fully isolated, fresh empty history + minimal system prompt |
| Tree connector style | Plain two-space indent (matches screenshot) |
| Render tests | Snapshot fixtures under `internal/tui/testdata/` |

## Phase 0 Status (as of 2026-05-16)

Per `THINKING-VISIBILITY-PLAN.md` Implementation Status section, the following
Phase 0 tasks are **already implemented and tested**:

- ✅ P0-1 (C1 bootstrap config-override)
- ✅ P0-2 (A3 `LengthRetryTokens` = 65536)
- ✅ P0-3 (C2 `NumCtx` default 32768)
- ✅ P0-4 (C3 `MaxTurns` safety cap 200)
- ✅ P0-5 (C4 `ToolContext` permission mode)
- ✅ P0-6 (A1 `req.Think`)
- ✅ P0-7 (A4 queued prompts drain) — with deviation: `agentStartFailedMsg`
   never wired because `AgentRunner.Run` has no start-error return path. Track
   as a future refactor when the runner interface is revisited.
- ✅ P0-8 (A5 dead `realProgramSender`)
- ✅ P0-9 (A6/C8 bootstrap mutex)
- ✅ P0-10 (A7/C5 sub-agent JSONL extended)
- ✅ P0-11 (C6 `sendEventForce` ctx-aware)
- ✅ P0-12 (C7 agenttool prompt cancellation)
- ✅ P0-13 (C10 detached memory extraction) — with deviation: drafts written
   silently rather than via `memoryUpdatedMsg`. Acceptable for v1.
- ✅ P0-14 (C11 token accounting + latency averaging)

Remaining in Phase 0:

- ⏳ **P0-15** (editor invocation suspending TUI) — not addressed.
- ⏳ **Manual verification** with a real thinking-capable Ollama model.

So UI-0 work can start immediately; only P0-15 remains as a Phase 0 backlog
item and it is independent of any UI phase.

## Task Numbering Convention

- `P0-*` — Phase 0 bug fixes (must land before UI phases)
- `UI-0-*` — Modal & permission rendering correctness
- `UI-1-*` — Animated run status
- `UI-2-*` — Tool elapsed-time only
- `UI-3-*` — Queue / retry / compaction visibility
- `UI-4-*` — Permission modal & help polish
- `UI-5-*` — Semantic style refactor
- `UI-6-*` — Hierarchical activity display
- `UI-7-*` — `/btw` side question

Each task lists: **deps**, **files**, **acceptance**, **tests**, **agent prompt**.

---

## Phase 0 — Bug Fixes

### P0-1 — Fix bootstrap config-override discard

**Deps:** none
**Files:** `internal/cli/repl.go`, `internal/bootstrap/state.go`

**Problem:** `repl.go:94` calls `bootstrap.Global().Snapshot()` before
`bootstrap.InitGlobal(initial)` at `:95`. `Global()` invokes `sync.Once.Do` with
`DefaultInitial("")`, which sets `SessionID`; the guard at `:94`
(`SessionID == ""`) is then false, so `InitGlobal(initial)` never runs. **Every
CLI flag and config override is silently discarded.**

**Acceptance:**

- Setting `--num-ctx` via CLI flag is reflected in `bootstrap.Global().Snapshot().NumCtx`.
- Setting `model` via config file is reflected in `Snapshot().ActiveModel`.
- Test in `bootstrap/state_test.go` builds an `Initial`, calls `InitGlobal`, and
  asserts that all fields propagated.

**Fix sketch:** Build `initial` first (lines 51–91), call `bootstrap.InitGlobal(initial)`
unconditionally, then call `Global().Snapshot()`. Remove the
`if SessionID == ""` guard in `repl.go`. Either drop the `sync.Once` gating in
`Global()` or expose `bootstrap.SetGlobal(state)` for explicit replacement.

**Agent prompt:**
```
Implement P0-1 from docs/TASKS-TUI.md. Re-order bootstrap initialization in
internal/cli/repl.go so that InitGlobal applies before any Global() read. Add a
regression test in internal/bootstrap that builds a non-default Initial, calls
InitGlobal, and asserts Snapshot reflects every overridden field. Do not change
public API beyond the necessary minimum. Run go test ./internal/cli ./internal/bootstrap.
```

### P0-2 — Set `LengthRetryTokens` and add regression test

**Deps:** P0-1
**Files:** `internal/bootstrap/state.go:125`, `internal/bootstrap/state_test.go`

**Problem:** `LengthRetryTokens: 512` is too small; every retry on length error
fails and falls through to `TerminalContextOverflow`.

**Acceptance:**

- `DefaultInitial("").LengthRetryTokens == 65536`
- Assertion in `bootstrap_test.go`

**Agent prompt:**
```
Change bootstrap LengthRetryTokens default from 512 to 65536 and add a test in
internal/bootstrap that asserts DefaultInitial("").LengthRetryTokens == 65536.
```

### P0-3 — Set `agent.Config.NumCtx` default

**Deps:** none
**Files:** `internal/agent/input.go:74`

**Problem:** `DefaultConfig().NumCtx == 0`; sub-agents and any caller that
doesn't probe send `num_ctx: 0` to Ollama → server default (often 2048).

**Acceptance:**

- `agent.DefaultConfig().NumCtx == 32768`
- `agent/input_test.go` covers it

**Agent prompt:**
```
Set agent.DefaultConfig().NumCtx default to 32768. Add a test in
internal/agent/input_test.go asserting the value. Verify cli/repl.go still
overrides correctly when the model probe succeeds.
```

### P0-4 — Restore `MaxTurns` safety cap

**Deps:** none
**Files:** `internal/bootstrap/state.go:122`, `internal/agent/input.go:76`,
`internal/agent/agent.go:120`

**Problem:** `MaxTurns: 0` means unlimited; a stuck tool loop never terminates.

**Acceptance:**

- Default `MaxTurns == 200` everywhere it is set.
- Treat `0` as "use default 200" rather than "unlimited" (less surprising).
- Test confirms a synthetic loop terminates with `TerminalMaxTurns` after 200 iterations.

**Agent prompt:**
```
Restore MaxTurns default of 200 in bootstrap/state.go, agent/input.go, and
agent/agent.go. Make 0 mean "use default 200". Add a test in internal/agent
that wires a fake tool emitting infinite ToolUseResult and asserts the run
terminates with TerminalMaxTurns after 200 turns.
```

### P0-5 — Fix `App.ToolContext` PermissionMode

**Deps:** none
**Files:** `internal/state/app.go:212`

**Problem:** Hardcodes `PermissionDefault`, dropping session mode.

**Acceptance:**

- `a.PermissionMode` is assigned to `tc.PermissionMode`.
- Test in `state/app_test.go` covers each `PermissionMode` constant.

**Agent prompt:**
```
In internal/state/app.go ToolContext(), assign a.PermissionMode instead of the
constant. Add a parametric test covering bypass/dontask/restrictive/default.
```

### P0-6 — Activate `req.Think` for capable models

**Deps:** P0-3
**Files:** `internal/agent/stream.go:43-52`, `internal/llm/capabilities.go`

**Problem:** `req.Think` is never set despite the capability matrix existing.

**Acceptance:**

- For a model where `ModelCapabilities(model).SupportsThinking == true`,
  `req.Think == true`.
- Test mocks the Ollama client and asserts `Think` on the request.

**Agent prompt:**
```
In agent/stream.go executeOneTurn, after the req struct literal, set req.Think
= true when llm.ModelCapabilities(model).SupportsThinking is true. Add a unit
test that mocks the Ollama client (use the existing fake if one exists,
otherwise add a minimal one) and asserts Think on the outbound request for
qwen3-thinking and false for llama3.1.
```

### P0-7 — Drain `QueuedPrompts` after run completion

**Deps:** none
**Files:** `internal/tui/app.go` (agentDoneMsg handler, ~line 232)

**Problem:** Prompts typed during active run are appended to `app.QueuedPrompts`
but never replayed. The `agentDoneMsg` handler only clears `ActiveRun`.

**Acceptance:**

- After `agentDoneMsg`, if `len(QueuedPrompts) > 0`, the next prompt starts.
- `startQueuedPrompt` failure (`agentStartFailedMsg`) clears `ActiveRun`,
  `cancelRun`, and surfaces an error transcript item.
- Test simulates queue of 2 prompts, verifies both run sequentially.

**Agent prompt:**
```
Implement queue drain in internal/tui/app.go agentDoneMsg case. Add a
startQueuedPrompt helper (signature in docs/THINKING-VISIBILITY-PLAN.md Part A4)
that re-uses the existing agent start machinery. Also handle agentStartFailedMsg
in the drain path: clear ActiveRun, cancel context, append an error transcript
item. Add tests covering: drain success, drain failure, empty queue, picker
closure on drain.
```

### P0-8 — Remove dead `realProgramSender` in tui/messages.go

**Deps:** none
**Files:** `internal/tui/messages.go:61-69`

**Problem:** Duplicate of the one in `cli/repl.go:270-276`; never instantiated.

**Acceptance:** Lines 61–69 (struct + Send method, including the leading comment)
removed. `ProgramSender` interface at 57–59 retained.

**Agent prompt:**
```
Delete the unused realProgramSender struct and Send method from
internal/tui/messages.go (lines 61-69). Keep the ProgramSender interface.
Run go build ./... and go test ./internal/tui to confirm nothing referenced it.
```

### P0-9 — Fix `bootstrap.Global()` and `ResetGlobalForTest` races

**Deps:** P0-1
**Files:** `internal/bootstrap/state.go:191`, `:211-214`

**Problem:** `Global()` reads `globalState` without lock; `ResetGlobalForTest`
writes both `globalOnce` and `globalState` without lock.

**Acceptance:**

- `Global()` does not read `globalState` outside `sync.Once.Do`.
- `ResetGlobalForTest` is mutex-guarded (or replaced with explicit
  `SetGlobal` if P0-1 went that route).
- `go test -race ./internal/bootstrap` passes under stress.

**Agent prompt:**
```
Remove the fast-path nil check in bootstrap.Global(). Guard ResetGlobalForTest
with a sync.Mutex (or remove if SetGlobal exists from P0-1). Add a stress test
spawning N goroutines calling Global concurrently with -race.
```

### P0-10 — Add missing event kinds to sub-agent JSONL

**Deps:** none
**Files:** `internal/agent/subagent.go:171-205`

**Problem:** Loop misses `AssistantThinkingDelta`, `ToolUseProgress`,
`RetryNotice`, `CompactionStarted`, `CompactionCompleted`, `HookNotice`.

**Acceptance:**

- All listed event kinds produce a JSONL line with `kind` set appropriately.
- Test wires a fake event stream and asserts JSONL output.

**Agent prompt:**
```
Extend the event-loop switch in internal/agent/subagent.go to handle
AssistantThinkingDelta, ToolUseProgress, RetryNotice, CompactionStarted,
CompactionCompleted, HookNotice. Write each as a JSONL line with kind, ts, and
event-specific fields. Add a subagent_test.go case driving each event kind and
asserting the JSONL output.
```

### P0-11 — Make `sendEventForce` context-aware

**Deps:** none
**Files:** `internal/agent/agent.go:485-487`

**Problem:** Unconditional channel send deadlocks if consumer has exited.

**Acceptance:**

- `sendEventForce` returns early on ctx cancel; logs a single drop notice via
  the existing logger.
- Test deliberately closes the consumer and asserts no goroutine leak after
  ctx cancel (use `runtime.NumGoroutine()` snapshot).

**Agent prompt:**
```
Replace the unconditional channel send in agent/agent.go sendEventForce with
a select that includes ctx.Done. Log a single dropped event when cancelled.
Add a test that exercises the cancel-during-send case.
```

### P0-12 — Cancel agenttool broker prompt on timeout

**Deps:** none
**Files:** `internal/tools/agenttool/agenttool.go:141-150`

**Problem:** On wrapper timeout, the user-facing permission modal stays open;
the user's eventual choice is sent to a dead channel.

**Acceptance:**

- Timeout path calls a new `broker.Cancel(promptID)` that closes the modal.
- Test simulates timeout and asserts modal closure.

**Agent prompt:**
```
Add Cancel(promptID) to the permission broker (or equivalent dismissal). In
agenttool wrapPermissionPrompt, call it on timeout before returning deny. Test
covers the timeout path and the broker cleanup.
```

### P0-13 — Detach memory extraction from run lifecycle

**Deps:** none
**Files:** `internal/memory/runner.go:76`

**Problem:** `extractPending` runs synchronously between `Terminal` and channel
close; TUI sees `ActiveRun=true` for many seconds after the answer streamed.

**Acceptance:**

- `Terminal` event emitted and channel closed before extraction starts.
- Extraction posts a `memoryUpdatedMsg` on the TUI program when complete.
- Test verifies `ActiveRun` clears before extraction completes.

**Agent prompt:**
```
Refactor memory/runner.go so extraction runs in a detached goroutine after the
Terminal event is emitted and the events channel closed. Post a memoryUpdatedMsg
to the TUI when extraction finishes. Add a test using a slow extractPending stub
that asserts the events channel is closed first.
```

### P0-14 — Fix `Meter.RecordLLMChat` token accounting

**Deps:** none
**Files:** `internal/observability/metrics.go:89-91`

**Problem:** Discards `promptTokens`/`completionTokens`; latency summed not
averaged. Status bar `tokens: N` only updates post-run.

**Acceptance:**

- `Meter.Snapshot()` reflects token deltas immediately after each `RecordLLMChat`.
- Latency stored as either "last" or "moving average", not running sum.
- Test calls `RecordLLMChat` 3x and asserts snapshot totals match input.

**Agent prompt:**
```
In observability/metrics.go RecordLLMChat, accumulate promptTokens and
completionTokens into TotalTokens. Replace the running-sum latency with a moving
average over the last 8 samples (ring buffer). Update Snapshot accordingly. Add
metrics_test.go cases for token accumulation and latency averaging.
```

### P0-15 — Editor invocation must suspend the TUI

**Deps:** none
**Files:** `internal/commands/registry.go`, `internal/tui/app.go`

**Problem:** Spawns `$EDITOR` while Bubble Tea owns the TTY in alt-screen mode.
Screen corrupts.

**Acceptance:**

- `commands.Registry` must not import Bubble Tea. It should return an editor intent to the TUI layer.
- TUI receives that intent and returns `tea.ExecProcess` so the alt-screen is released while the editor runs.
- On editor success, TUI appends a system transcript item such as `[Memory edited: <name>]`.
- On editor failure, TUI appends a clear error transcript item.
- Manual test: open editor, edit, exit, verify TUI redraws cleanly in alt-screen and no-alt-screen modes.
- Automated test: dispatching `/memory edit <name>` produces an editor intent; TUI handling returns a non-nil command and preserves transcript state.

**Agent prompt:**
```
Implement P0-15 from docs/TASKS-TUI.md. Do not import Bubble Tea into
internal/commands. Instead, extend commands.Output with an optional editor
intent containing the command path, args, display name, and target file. Change
/memory edit so it seeds the memory file as it does today, then returns that
intent rather than running exec.CommandContext directly. In internal/tui/app.go,
when command dispatch returns an editor intent, issue tea.ExecProcess and append
a system transcript item after completion. Add tests for command output intent
and TUI command creation. Document manual checks for alt-screen and no-alt-screen.
```

---

## UI-0 — Modal & Permission Rendering Correctness

### UI-0-1 — Replace `overlayModal` with a real overlay

**Deps:** none (but logically before UI-4)
**Files:** `internal/tui/app.go` (overlayModal helper, ~line 1271)

**Problem:** Current implementation uses `lipgloss.JoinVertical`, appending the
modal below the transcript instead of overlaying.

**Acceptance:**

- Permission modal renders centered on top of dimmed transcript snapshot.
- Snapshot test in `testdata/permission-modal-overlay.txt`.

**Agent prompt:**
```
Replace overlayModal in internal/tui/app.go with a true overlay using
lipgloss.Place over a captured snapshot of the background view. Dim the
background by 50% via a muted-style render. Add a snapshot fixture.
```

### UI-0-2 — Esc handler on permission modal

**Deps:** UI-0-1
**Files:** `internal/tui/app.go` permission modal key handling, `internal/tui/permission.go`

**Acceptance:** Esc inside modal → deny + close modal. Works regardless of
`activeRunCtx` state.

**Agent prompt:**
```
Add Esc → deny + close to the permission modal in internal/tui. Update the
modal's help text. Add a test asserting Esc dismisses the modal even when
activeRunCtx is nil.
```

### UI-0-3 — `/clear` resets transient run state

**Deps:** none
**Files:** `internal/tui/app.go` (`/clear` handler ~line 499)

**Acceptance:** `/clear` resets `LastRetryNotice`, `TerminalReason`,
`TerminalDetail`, `Usage`, and `m.transcript`, leaving `Messages` empty.

**Agent prompt:**
```
Extend the /clear command in internal/tui/app.go to reset LastRetryNotice,
TerminalReason, TerminalDetail, Usage in app state, in addition to clearing
Messages. Add a test asserting all fields are zero after /clear.
```

### UI-0-4 — Picker / modal race fix

**Deps:** none
**Files:** `internal/tui/app.go` (Update handler)

**Acceptance:**

- When picker is open and `permissionPromptMsg` arrives, picker closes before
  the modal renders.
- Test orders the messages and asserts picker is closed in the resulting state.

**Agent prompt:**
```
In internal/tui/app.go Update, ensure permissionPromptMsg handling calls
closePicker before showing the modal. Add a test that submits an `@` (opens
picker) and then sends a permissionPromptMsg; assert picker closed and modal
visible.
```

### UI-0-5 — Slash-command picker scope by cursor anchor

**Deps:** none
**Files:** `internal/tui/picker/command_provider.go`

**Problem:** `/permissions allow Bash(/path)` re-triggers the command picker on
the embedded `/`.

**Acceptance:**

- Command picker activates only when `/` is the first non-whitespace char from
  cursor.
- Test covers `/help`, `/permissions allow Bash(/x)`, and ` /help` (leading space).

**Agent prompt:**
```
In picker/command_provider.go, gate command-picker activation on cursor anchor:
the `/` must be at start of input or after whitespace, not any substring. Add
tests for the three example inputs above.
```

---

## UI-1 — Animated Run Status

### UI-1-1 — Add `RunUIState`, `RunPhase`, and `RunSnapshot()` accessor

**Deps:** P0-* (all)
**Files:** `internal/tui/app.go`, `internal/tui/runstate.go` (new)

**Acceptance:**

- New file `internal/tui/runstate.go` defines `RunPhase`, `RunUIState`,
  `RunSnapshot` (read-only copy), and `(m *Model) RunSnapshot() RunSnapshot`.
- Initialized to `RunPhaseIdle`.
- No effect on rendering yet — just plumbing.

**Agent prompt:**
```
Create internal/tui/runstate.go with the types defined in ADR-001 lines 75-106.
Add a RunSnapshot value type (read-only copy of RunUIState) and a (*Model)
RunSnapshot() accessor for tests and future consumers. Add a unit test asserting
the initial snapshot is RunPhaseIdle.
```

### UI-1-2 — Wire phase transitions

**Deps:** UI-1-1
**Files:** `internal/tui/app.go` (Update handler)

**Acceptance:** Phase transitions per ADR-001 lines 108–124 table. Priority
order on conflict: `permission_required` > `running_tool` > `streaming` >
`waiting_for_model` > idle.

**Test matrix:** one test per row of the transition table, plus a test verifying
that simultaneous tool-active + thinking yields `running_tool`.

**Agent prompt:**
```
In internal/tui/app.go Update, transition m.runUI.Phase per the ADR-001
transition table. Implement the priority order from the addendum. Add tests
covering each transition and a multi-state priority test.
```

### UI-1-3 — Tick scheduling + stop-tick guarantee

**Deps:** UI-1-1
**Files:** `internal/tui/app.go` (tickCmd, Init, Update)

**Acceptance:**

- Tick interval read from config (`tui.tick_ms`, default 120).
- Ticks only while `ActiveRun || PermissionPrompt != nil`.
- After `Terminal`, no further `tickMsg` returned for ≥300ms (test).
- Spinner frames cycle through both Unicode and ASCII fallback (config flag).

**Agent prompt:**
```
Implement tickCmd in internal/tui/app.go reading interval from config (default
120ms). Schedule ticks while ActiveRun or PermissionPrompt is set; stop
otherwise. Add a stop-tick acceptance test using a fake clock that asserts no
tickMsg for 300ms after Terminal. Add ASCII fallback gated by config.
```

### UI-1-4 — Status-bar render with priority

**Deps:** UI-1-2, UI-1-3, P0-14
**Files:** `internal/tui/app.go` (`renderStatusBar`), `internal/tui/styles.go`

**Acceptance:**

- Status bar renders the active phase per the mockups in ADR-001 lines 132–158.
- Width-aware: at narrow widths, drop token count first, then queue length,
  then elapsed time. Phase and abort hint always visible.
- Snapshot tests at 60, 80, 120 columns.

**Agent prompt:**
```
Rewrite renderStatusBar in internal/tui/app.go to render a single line per the
priority order and mockups in ADR-001. Use semantic styles (active, muted,
warning, accent) — these will be defined in UI-5-1 but seed stubs now. Add
snapshot fixtures at 60/80/120 columns for each phase.
```

---

## UI-2 — Tool Elapsed Time (reduced)

### UI-2-1 — Per-tool elapsed time

**Deps:** UI-1-3
**Files:** `internal/tui/app.go` (tool panel render), `internal/tui/transcript.go`

**Acceptance:**

- Active tool transcript items display `Mm:SSs` elapsed time, updated each tick.
- Completed tools display final elapsed time once.
- Snapshot fixtures.

**Agent prompt:**
```
Add an Elapsed accessor to TranscriptItem for tool kinds, derived from StartedAt
on the transcript item (added if missing). Update the tool panel render to
include elapsed time. Use the tick loop to refresh. Snapshot fixture.
```

---

## UI-3 — Queue, Retry, and Compaction Visibility

### UI-3-1 — Queue length in status bar

**Deps:** P0-7, UI-1-4
**Files:** `internal/tui/app.go`

**Acceptance:**

- `queue: N` segment appears when `len(QueuedPrompts) > 0`.
- Disappears when queue drains.
- Snapshot tests.

**Agent prompt:**
```
Extend renderStatusBar in internal/tui/app.go to render a `queue: N` segment
when len(app.QueuedPrompts) > 0. Place it between phase and abort hint per the
ADR-001 mockup. Add snapshot fixtures under internal/tui/testdata/ for
queue-0, queue-1, queue-3. Run go test ./internal/tui.
```

### UI-3-2 — Retry and compaction segments

**Deps:** UI-1-4
**Files:** `internal/tui/app.go`

**Acceptance:**

- On `RetryNotice`, status shows `retrying (N) cause` for `LastNoticeTTL` (config
  default 5s) then reverts to prior phase.
- On `CompactionStarted/Completed`, status shows `compacting ctx` and updates.

**Agent prompt:**
```
Add retry and compaction handling in renderStatusBar. On RetryNotice, store
LastNotice with TTL = config.tui.last_notice_ttl_ms (default 5000). Tick loop
clears the notice when expired and reverts to previous phase. On
CompactionStarted/Completed, render `compacting ctx` until Completed arrives.
Tests: retry segment appears on event, clears after TTL, prior phase restored;
compaction segment appears and clears. Snapshot fixtures.
```

### UI-3-3 — `/queue` slash command

**Deps:** P0-7
**Files:** `internal/commands/registry.go`, `internal/tui/app.go`

**Sub-commands:** `/queue list`, `/queue clear`, `/queue drop <index>`.

**Acceptance:**

- `/queue list` prints numbered queued prompts as a system transcript item.
- `/queue clear` empties the queue and prints how many removed.
- `/queue drop N` removes index N (1-based) with bounds check.
- Picker autocompletion includes the sub-commands.
- Tests cover each sub-command and bounds errors.

**Agent prompt:**
```
Add /queue slash command in commands/registry.go with list/clear/drop
sub-commands operating on app state QueuedPrompts. Surface output as system
transcript items. Register usage strings so the picker autocompletes. Tests in
commands/registry_test.go and tui/app_test.go.
```

---

## UI-4 — Permission Modal & Help Polish

### UI-4-1 — Modal context fields

**Deps:** UI-0-1, UI-0-2
**Files:** `internal/tui/permission.go`, `internal/tui/app.go`

**Acceptance:**

- Modal shows: tool, mode, reason (if available), wrapped target block, key help.
- Wrapping safe for ≥120-char targets.
- Snapshot fixture.

**Agent prompt:**
```
Extend the permission modal render in internal/tui/permission.go to show
PermissionMode, Reason (if populated on state.PermissionPrompt), and a
soft-wrapped target block (use lipgloss width-aware wrapping at modal_width-4).
Key help row stays as the bottom line. Add snapshot fixtures for short and
long-target cases.
```

### UI-4-2 — Fix Always-allow rule scope to literal target

**Deps:** UI-4-1
**Files:** `internal/tui/app.go` (~line 773), permission rule plumbing

**Acceptance:**

- Pressing `A` adds a rule matching the **literal** modal target (e.g. exact
  command for Bash, exact path for FileEdit), not `Tool(*)`.
- Modal text reads `[A] always allow this exact target` to set expectation.
- Test asserts the rule string in app state after pressing `A`.

**Agent prompt:**
```
In internal/tui/app.go, change the A-key handler so the appended rule matches
the literal modal target rather than the wildcard pattern Tool(*). Update modal
help text. Add a test that simulates pressing A on a Bash modal with target
"go test ./..." and asserts the resulting PermissionRules contains that exact
target, not "Bash(*)".
```

### UI-4-3 — First-run help and `/clear` mini help

**Deps:** none
**Files:** `internal/tui/app.go` (renderEmptyState helper)

**Acceptance:**

- Empty session (no messages, no startup notes, no active run): render the full
  help card from ADR-001 section 6.
- After `/clear`: single-line `transcript cleared — type a new prompt` (small
  variant decision).
- Snapshot fixtures for both states.

**Agent prompt:**
```
Add renderEmptyState in internal/tui/app.go. Track whether the session has had
any user message via a model field `everHadInput bool` (set on first submit).
If !everHadInput && len(transcript) == 0: render full help card.
If everHadInput && len(transcript) == 0 (post-/clear): render the small variant.
Inject into View() before the input box. Snapshot fixtures: empty-firstrun.txt,
empty-cleared.txt.
```

### UI-4-4 — Command picker metadata

**Deps:** none
**Files:** `internal/commands/registry.go`, `internal/tui/picker/command_provider.go`

**Acceptance:**

- Each command exposes `Usage` and `Description` from a single source in
  `registry.go`.
- Picker shows description and inserts only the command name on selection.
- `/per` → suggests `/permissions` (substring + prefix match, prefix wins).
- Tests.

**Agent prompt:**
```
In internal/commands/registry.go define each command with Name, Usage,
Description, Examples []string. Expose Registry.All() []CommandMeta. Update
picker/command_provider.go to render description and to rank by prefix match
first, substring second. Selection inserts only the name + trailing space.
Tests: rank order for /per, /clear, /he; selection inserts only the name;
description appears in picker row.
```

---

## UI-5 — Semantic Style Refactor (no themes)

### UI-5-1 — Semantic style roles

**Deps:** none
**Files:** `internal/tui/styles.go`

**Acceptance:**

- `Styles` struct exposes semantic roles: `Info`, `Success`, `Warning`, `Error`,
  `Muted`, `Accent`, `Active`, `Thinking{Collapsed,Expanded,Box}`.
- Default values match current visual output (no perceptible color change in
  golden screenshots).
- All render sites in `app.go` migrate to semantic roles.

**Agent prompt:**
```
Refactor DefaultStyles() in internal/tui/styles.go to expose semantic role
fields (Info, Success, Warning, Error, Muted, Accent, Active). Migrate
renderStatusBar, renderTranscript, and renderToolPanel to use semantic fields.
Do not change visible colors. Add a styles_test.go asserting each field is
non-zero.
```

---

## UI-6 — Claude-Code-Style Hierarchical Activity Display

### UI-6-1 — `AssistantTurnStarted` event

**Deps:** UI-1-2
**Files:** `internal/agent/events.go`, `internal/agent/agent.go`

**Acceptance:**

- New event type `AssistantTurnStarted{TurnID string, StartedAt time.Time}`.
- Emitted at the start of each assistant turn (first user→assistant transition
  and every model continuation).
- Sub-agent emits its own `AssistantTurnStarted` events with a distinct turn ID.
- Tests in agent package.

**Agent prompt:**
```
Add a new event in internal/agent/events.go:

  type AssistantTurnStarted struct {
      TurnID    string    // unique per turn; uuid or monotonic+sessionID
      StartedAt time.Time
      ParentTurnID string // empty for top-level; non-empty for sub-agent turns
  }
  func (AssistantTurnStarted) isEvent() {}

Emit from internal/agent/agent.go at the start of each iteration in the agent
loop (turn counter increment site). For sub-agents, also emit on the sub-agent's
event channel with ParentTurnID set. Tests in agent_test.go: single user prompt
produces ≥1 turn started; tool→continuation produces 2 turn-started events with
the same parent.
```

### UI-6-2 — Transcript groups data model

**Deps:** UI-6-1
**Files:** `internal/tui/transcript.go`

**Acceptance:**

- Add `GroupID`, `ParentID`, `Depth` to `TranscriptItem`.
- New `TranscriptGroup` struct stored in `m.groups`.
- Helpers `BeginGroup`, `AddToGroup`, `CollapseGroup`, `ExpandGroup`.
- Unit tests for each helper.

**Agent prompt:**
```
Extend TranscriptItem in internal/tui/transcript.go:

  type TranscriptItem struct {
      // ... existing ...
      GroupID  string
      ParentID string   // for nested sub-agent tool calls; empty for top-level
      Depth    int      // 0 for top-level; +1 per nested level; capped at 4
  }

  type TranscriptGroup struct {
      ID             string
      Header         string
      StartedAt      time.Time
      Collapsed      bool
      ToolIDs        []string
      ExpandedRunIDs map[string]bool
  }

In Model add `groups map[string]*TranscriptGroup` and helpers:
  BeginGroup(id, header) *TranscriptGroup
  AddToGroup(groupID, item TranscriptItem)
  CollapseGroup(id) / ExpandGroup(id)

Tests in transcript_test.go cover each helper. No render changes here.
```

### UI-6-3 — Tree render with plain indent

**Deps:** UI-6-2
**Files:** `internal/tui/app.go` (`renderTranscriptTree`)

**Acceptance:**

- Plain two-space indent per depth level.
- Nested sub-agents render one level deeper.
- Visual depth capped at 4; deeper items show `↳ …` marker.
- Snapshot fixture for a 3-level deep tree.

**Agent prompt:**
```
Add renderTranscriptTree(items []TranscriptItem, groups map[string]*TranscriptGroup) string
in internal/tui/app.go. Algorithm:

  for each top-level group (sorted by StartedAt):
    render group header at depth 0
    for each tool in group:
      render with indent = min(item.Depth+1, 4) * 2 spaces
      if Depth >= 4: prefix with "↳ … "

Replace the current tool-panel render call site in renderTranscript with the
new tree. Add snapshot fixtures: tree-single-turn.txt, tree-nested-subagent.txt,
tree-depth-overflow.txt (5 levels deep, last shown as ↳ …).
```

### UI-6-4 — Collapsible batches and `ctrl+o`

**Deps:** UI-6-3
**Files:** `internal/tui/app.go`

**Acceptance:**

- Groups with >4 completed tools collapse middle to `… +N tool uses (ctrl+o to expand)`.
- First and last child stay visible.
- `ctrl+o` toggles when focus is on the placeholder; `ctrl+shift+o` expands all.
- `ExpandedRunIDs` preserves explicit user expansions across collapses.
- Snapshot fixtures for collapsed, partially expanded, fully expanded.

**Agent prompt:**
```
In renderTranscriptTree, when a group has > config.tui.max_visible_tools_per_group
(default 4) completed tools and Collapsed=true, render first tool, then a
single placeholder TranscriptItem with content "… +N tool uses (ctrl+o to
expand)" and TranscriptKind=TranscriptBatched (new kind), then last tool. The
placeholder is focusable. Track focus index in m.focusedTranscriptID.

Wire keys (insert mode, after picker handler, before vim switch):
  ctrl+o      → toggle CollapseGroup for the focused group
  ctrl+shift+o → toggle every group

Preserve per-tool explicit-expand state in ExpandedRunIDs. Snapshot fixtures:
batched-collapsed.txt, batched-partial-expand.txt, batched-full-expand.txt.
```

### UI-6-5 — Background execution (`ctrl+b`) + footer indicator

**Deps:** UI-6-3, UI-6-1
**Files:** `internal/tui/app.go`, `internal/agent/events.go`, `internal/state/app.go`

**Acceptance:**

- `state.ToolUse` gains `Background bool`.
- `ToolUseStart` event carries `Backgrounded bool`.
- `ctrl+b` available when current tool elapsed > `tui.background_eligible_after`
  (config, default 20s).
- Only one bg slot; second `ctrl+b` shows transient tip.
- Backgrounded tool not allowed on `Agent(...)` or permission-blocked tools.
- On `ToolUseResult` for background tool: append one-line summary transcript
  item; full output goes to `/bg view <id>`.
- Tests for the eligibility timer, the slot guard, the disallowed cases, and
  the completion path.

**Agent prompt:**
```
Add Background bool to state.ToolUse (internal/state/app.go) and Backgrounded
bool to agent.ToolUseStart (internal/agent/events.go).

In internal/tui/app.go add m.bgSlot *backgroundedTool (only one). The
backgroundedTool struct holds: ToolID, ToolName, StartedAt, eventCh chan agent.Event,
buffer []string (collected output), cancel context.CancelFunc.

Key handler (after picker, before vim switch) when m.runUI.Phase ==
RunPhaseRunningTool, elapsed > config.tui.background_eligible_after_ms, current
tool != "Agent", current tool not in permission gating:
  ctrl+b → if m.bgSlot != nil: render transient tip "Background slot busy"
           else: transfer the active tool's event subscription into m.bgSlot,
                  detach it from the foreground RunUIState (set Phase=Idle),
                  clear m.activeRunCtx but keep the run context alive so the
                  tool continues.

On ToolUseResult for m.bgSlot.ToolID: append a one-line system transcript item
"Background <ToolName> finished in <elapsed> · run /bg view <ID> for output";
clear m.bgSlot.

Tests: eligibility timer, slot guard (second ctrl+b shows tip), Agent disallowed,
permission-blocked disallowed, completion path appends summary.
```

### UI-6-6 — `/bg` slash command

**Deps:** UI-6-5
**Files:** `internal/commands/registry.go`, `internal/tui/app.go`

**Sub-commands:** `/bg list`, `/bg view <id>`, `/bg cancel <id>`.

**Acceptance:**

- `/bg list` shows running and completed background tools.
- `/bg view <id>` opens a detail pane (transcript-style) with full output.
- `/bg cancel <id>` aborts a running background tool (sends to its cancel chan).

**Agent prompt:**
```
Register /bg in internal/commands/registry.go with sub-commands list/view/cancel.
list: print one row per backgrounded tool from m.bgSlot history (keep last 16
in a ring). view: open a modal-style overlay containing the buffered output;
Esc closes. cancel: call the stored cancel func and remove from m.bgSlot. Tests
in commands/registry_test.go and tui/app_test.go.
```

### UI-6-7 — Animated active task line with verbs and effort

**Deps:** UI-1-3, UI-1-4, P0-14, P0-6
**Files:** `internal/tui/phaseverbs.go` (new), `internal/tui/app.go` (`renderActiveTask`)

**Acceptance:**

- `phaseverbs.go` exposes verb lists per phase (configurable via
  `config.tui.verbs`) and a deterministic sampler keyed by
  `(phase, runID, floor(elapsed/30s))`.
- Active task line format: `<spinner> <Verb>…  (<elapsed> · <tokens> · <effortClause>)`.
- Elapsed format: `<60s` → `M:SSs`; `≥60s` → `Hh Mm`.
- Token format: `↓ Nk tokens` for streaming, `↑/↓ N/M` when both meaningful.
- Effort clause derived from `ThinkLevel` on capabilities; v1 uses coarse
  labels (`thinking…`, `streaming…`, `running Bash…`).
- `config.tui.serious = true` disables verbs (shows phase name verbatim).
- Snapshot fixtures per phase.

**Agent prompt:**
```
Create internal/tui/phaseverbs.go with verb maps and a deterministic Sample(phase,
runID, elapsed) function. Add renderActiveTask in internal/tui/app.go matching
the format above. Read tui.verbs and tui.serious from config; defaults from
ADR-001. Snapshot fixtures for each phase and for serious mode.
```

### UI-6-8 — Inline tip strip

**Deps:** UI-6-7
**Files:** `internal/tui/tips.go` (new), `internal/tui/app.go`

**Acceptance:**

- Tip catalogue loaded from config (`tui.tips`) with defaults from ADR-001.
- Each trigger evaluated per tick.
- A tip shown at most once per session per trigger.
- Never overlays permission modal.
- Wraps to single line with `…` if too long.
- Test matrix: each trigger fires; only longest-triggered shows when multiple
  qualify; modal suppresses.

**Agent prompt:**
```
Create internal/tui/tips.go:

  type TipTrigger string
  const (
      TipRunLong          TipTrigger = "run_long"           // run > tui.tip_run_after_ms (45000)
      TipToolLong         TipTrigger = "tool_long"          // active tool > tui.tip_tool_after_ms (60000)
      TipManyTools        TipTrigger = "many_tools"         // group has >4 tools
      TipPermissionDenied TipTrigger = "permission_denied"  // 2 denials in a row
      TipContextHigh      TipTrigger = "context_high"       // usage > 70% of NumCtx
  )

  type TipCatalogue map[TipTrigger]string
  func DefaultTipCatalogue() TipCatalogue { ... }
  func SelectTip(state RunSnapshot, seen map[TipTrigger]bool, c TipCatalogue) (TipTrigger, string, bool)

SelectTip: evaluate every trigger; pick the one whose trigger has been true
longest (record TriggerStartedAt in RunUIState). Mark `seen` after first display.

Wire into View() under renderActiveTask. Skip when PermissionPrompt != nil.
Truncate to width-4 with "…". Tests cover each trigger and the modal suppression.
```

### UI-6-9 — Footer Band A and Band B

**Deps:** UI-6-7
**Files:** `internal/tui/footer.go` (new), `internal/git/branch.go` (new),
`internal/tui/app.go`

**Acceptance:**

- Band A shortcut row per state table in ADR-001 lines 1127–1133.
- Band B shows git branch with color-coded dot and queued/backgrounded
  sub-agents.
- Branch read cached 2s; never blocks render; falls back to `?` if read times out.
- `↑/↓` on empty input box selects a sub-agent in Band B; `Enter` opens its
  transcript pane.
- Snapshot fixtures for: idle, running, permission modal, picker open, with and
  without queued sub-agents.

**Agent prompt:**
```
Create internal/git/branch.go with a TTL-cached (2s) reader using git
symbolic-ref HEAD and git status --porcelain=v2. Create internal/tui/footer.go
with two render functions taking FooterState (current state, branch info,
sub-agents). Wire into renderView in internal/tui/app.go. Snapshot fixtures
under internal/tui/testdata/footer-*.txt.
```

---

## UI-7 — `/btw` Side Question

### UI-7-1 — Read-only sub-agent toolset

**Deps:** P0-* complete
**Files:** `internal/tools/registry.go`, `internal/agent/agent.go`

**Acceptance:**

- A toolset variant `ReadOnlyTools` exposes Read, Grep, Glob; excludes Bash,
  Edit, Write, WebFetch.
- Selectable via `agent.Input.ToolsetName`.
- Tests verify Bash is rejected.

**Agent prompt:**
```
In internal/tools/registry.go expose a ReadOnlyTools registry constructor that
includes only Read, Grep, Glob, and List. Add a ToolsetName field to
agent.Input; agent.Run consults a tools-registry-resolver mapping name to
registry. Default name "" resolves to existing full registry. Tests: building
an agent with ToolsetName="read_only" rejects Bash, Edit, Write tool requests
with a clear error.
```

### UI-7-2 — `/btw` command and side-conversation pane

**Deps:** UI-7-1, UI-6-9
**Files:** `internal/commands/registry.go`, `internal/tui/app.go`

**Acceptance:**

- `/btw <question>` snapshots no message history (fully isolated), spawns a
  fresh agent with the read-only toolset and a minimal system prompt.
- Side conversation rendered in a dimmed inset above the input box; not added
  to main `Messages`.
- `/btw done` (or Enter on empty input while side conv is focused) closes it.
- No recursion: `/btw` inside a `/btw` rejected with a tip.
- Main run's `RunUIState` unchanged throughout.
- Tests verify isolation (no messages added to `app.Messages`), recursion guard,
  and dismissal.

**Agent prompt:**
```
Register /btw <question> in internal/commands/registry.go. Behavior:

  1. If m.btw != nil: append transient tip "Already in a side conversation"; return.
  2. Build a fresh agent.Input with:
       Messages: []llm.Message{}
       SystemPrompt: "Answer the following side question succinctly. Do not
         modify any files or run tools that have side effects."
       ToolsetName: "read_only"
       Model: appState.ActiveModel
     Append the user's question as a user message.
  3. Start the agent and stash its event channel in m.btw.
  4. Render the side conversation in a dimmed inset above the input box using
     a `renderBtwInset` helper; do not modify app.Messages or the main transcript.
  5. Enter on empty input while m.btw != nil OR `/btw done` closes the inset
     (cancel its ctx, clear m.btw).

The main run's RunUIState is unchanged. Tests cover: app.Messages length is
unchanged before/after a /btw; recursion guard fires; dismissal cancels the
sub-context.
```

---

## Cross-Cutting Tasks

### X-1 — Snapshot testing infrastructure

**Deps:** none
**Files:** `internal/tui/snapshot_test.go`, `internal/tui/testdata/`

**Acceptance:**

- Helper `assertSnapshot(t, name, got)` reads `testdata/<name>.txt`, compares,
  optionally updates with `-update` flag.
- Documented in package doc comment.

**Agent prompt:**
```
Create internal/tui/snapshot_test.go with:

  var updateGolden = flag.Bool("update", false, "update snapshot fixtures")
  func assertSnapshot(t *testing.T, name, got string) {
      t.Helper()
      path := filepath.Join("testdata", name+".txt")
      if *updateGolden {
          require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
          return
      }
      want, err := os.ReadFile(path)
      require.NoError(t, err)
      if string(want) != got {
          t.Fatalf("snapshot mismatch for %s:\nwant:\n%s\n\ngot:\n%s", name, want, got)
      }
  }

Document usage in package doc comment of app.go: "Run `go test -update
./internal/tui` to regenerate golden fixtures." Add an initial test that
exercises the helper with a trivial fixture.
```

### X-2 — Configuration surface additions

**Deps:** none (lands as new keys land)
**Files:** `internal/config/config.go`, `internal/config/defaults.go`,
`USER_MANUAL.md`

**New keys** (all under `tui.`):

- `tick_ms` (int, default 120) — animation tick interval
- `spinner` (string, default "braille") — braille | ascii
- `serious` (bool, default false) — disable whimsical verbs and tips
- `verbs` (map[string][]string) — phase → verbs override
- `tips` (map[string]string) — trigger key → tip text override
- `background_eligible_after_ms` (int, default 20000)
- `last_notice_ttl_ms` (int, default 5000)
- `max_visible_tools_per_group` (int, default 4)

**Acceptance:** each new key documented with default in `USER_MANUAL.md` and
unit-tested in `config_test.go`.

**Agent prompt (per-key incremental):**
```
For each new key added by a UI task, in the same PR:
  1. Add the field to internal/config/config.go (under a TUI sub-struct if not
     already present).
  2. Add the default value in internal/config/defaults.go.
  3. Add one row to the TUI Configuration section of USER_MANUAL.md.
  4. Add a config_test.go case verifying the default loads when unset and the
     override applies when set.
Do not introduce keys speculatively — only when consumed by code in the same PR.
```

### X-3 — `/help` text update

**Deps:** UI-6-* complete
**Files:** `internal/commands/registry.go` (`handleHelp`)

Document new keybinds and slash commands: `Ctrl+T`, `Ctrl+B`, `Ctrl+O`,
`Ctrl+Shift+O`, `/queue`, `/btw`, `/bg`.

**Agent prompt:**
```
Extend handleHelp in internal/commands/registry.go to append a "Keys" section
listing every new keybind introduced by UI-1..UI-7 and a "Commands" section
listing /queue, /btw, /bg with their sub-commands. Pull descriptions from the
CommandMeta added in UI-4-4. Test asserts the help string contains every new
key and command name.
```

---

## Appendix A — Existing Types Reference (verified against source)

Agents must read these files first to confirm current shapes, since line
numbers in this doc may drift.

### `internal/state/app.go`

```go
type App struct {
    // Identity & context
    SessionID    string
    ActiveModel  string

    // Conversation
    Messages       []llm.Message
    QueuedPrompts  []string
    PermissionMode permissions.Mode
    PermissionRules permissions.Rules

    // Run lifecycle
    ActiveRun        bool
    PermissionPrompt *PermissionPrompt
    ActiveTools      map[string]ToolUse
    TodoList         []Todo

    // Output budgets
    MaxOutputTokens  int
    LengthRetryTokens int

    // Run telemetry
    Usage         llm.Usage
    LastRetryNotice string
    TerminalReason  agent.TerminalReason
    TerminalDetail  string

    // Tool settings (caps)
    ToolSettings ToolSettings

    // ... see source for full list
}

type ToolUse struct {
    ID, Name string
    StartedAt time.Time
    LastProgress string
    // (UI-6-5 adds `Background bool`)
}

type PermissionPrompt struct {
    ID, Tool, Target, Reason string
    // ...
}
```

### `internal/agent/events.go`

```go
type Event interface{ isEvent() }

type AssistantTextDelta struct { Content string }
type AssistantThinkingDelta struct { Thinking string }
type ToolUseStart struct { ID, Name string; Input map[string]any /* + Backgrounded bool in UI-6-5 */ }
type ToolUseProgress struct { ID string; Data any }
type ToolUseResult struct { ID string; Output string; Error error }
type RetryNotice struct { Attempt int; Cause string }
type HookNotice struct { Source, Message string }
type CompactionStarted struct { Before int }
type CompactionCompleted struct { Before, After int; Summary string }
type Terminal struct {
    Reason TerminalReason
    Detail string
    Usage  llm.Usage
}

// New in UI-6-1:
// type AssistantTurnStarted struct { TurnID, ParentTurnID string; StartedAt time.Time }
```

### `internal/tui/messages.go`

```go
type agentEventMsg struct{ event agent.Event }
type agentDoneMsg struct{}
type agentStartFailedMsg struct{ err error }   // EXISTS — usable for P0-7 follow-up
type permissionPromptMsg struct{ ... }
type permissionCancelledMsg struct{ ID string }
type permissionResolvedMsg struct{ ... }
type slashCommandMsg struct{ ... }
type skillChangedMsg struct{ ... }
type tickMsg time.Time
type fileIndexRefreshedMsg struct{ ... }
type ProgramSender interface{ Send(msg tea.Msg) }
```

> Note: the THINKING-VISIBILITY status section claims `agentStartFailedMsg` is
> "unused — runner has no start-error return path." The message type still
> exists in `messages.go:19`; reviving the failure path is a separate refactor
> tracked outside this plan but a UI-1-2 follow-up can wire it once the runner
> interface exposes start errors.

### `internal/tui/transcript.go`

```go
type TranscriptKind string

type TranscriptItem struct {
    Kind      TranscriptKind
    ToolID    string
    ToolName  string
    Content   string
    Collapsed bool
    Error     string
    Rendered  string  // markdown cache
    CharCount int     // thinking only
    Streaming bool    // thinking only
    // UI-6-2 will add: GroupID, ParentID, Depth
    // UI-2-1 will add: StartedAt time.Time (if not already present)
}

// Helpers: AppendAssistantDelta, AppendThinkingDelta, FinalizeThinkingItem,
// CreateToolItem, CreateSystemItem, CreateUserItem
```

## Appendix B — Test Command Reference

| Scope | Command |
| :--- | :--- |
| All tests | `go test ./...` |
| TUI package | `go test ./internal/tui` |
| Agent package | `go test ./internal/agent` |
| Bootstrap | `go test ./internal/bootstrap` |
| Commands | `go test ./internal/commands` |
| Observability | `go test ./internal/observability` |
| Race detector (P0-9 stress) | `go test -race -count=20 ./internal/bootstrap` |
| Update snapshots | `go test -update ./internal/tui` |
| Single test | `go test ./internal/tui -run TestRunUIState_PhaseTransition` |

If a fresh checkout fails with file-lock errors, prefix with a tmp build cache:

```bash
GOCACHE=/private/tmp/go-nandocode-llm-gocache go test ./internal/tui
```

(THINKING-VISIBILITY-PLAN status section uses this idiom.)

## Appendix C — Common Pitfalls

1. **Stale line numbers**. Every line-number reference in this plan was true
   at writing. Phase 0 changes have shifted many of them. Always `grep` for the
   relevant symbol before patching by line.

2. **`agentStartFailedMsg` plumbing**. The type exists in `tui/messages.go`
   but the runner never emits it. Any task whose acceptance mentions it (P0-7
   follow-up, A4 deviation) must either revive the runner's error path or
   document that the message stays unused.

3. **Bubble Tea command vs. message**. UI changes should return a `tea.Cmd`
   from `Update`, not perform IO inline. The tick scheduler in UI-1-3 must
   return a fresh `tea.Tick` only when state warrants it; returning ticks
   unconditionally is the leak failure mode.

4. **State mutation via store**. `m.store.Set(func(app state.App) state.App { ... })`
   is the only legal way to mutate app state. Direct field writes on the
   snapshot have no effect.

5. **`PermissionPrompt` cancellation**. `permissionCancelledMsg` is a
   distinct message from `permissionResolvedMsg`. UI tasks that change the
   modal flow must handle both.

6. **Sub-agent vs. main-agent events**. `subagent.go` consumes its own
   event channel and writes JSONL; sub-agent events are NOT delivered to the
   TUI. UI-6 grouping for sub-agent activity must read from
   `state.App.ActiveTools` (where the `Agent(...)` tool is recorded) and the
   sub-agent's own progress events via the main `ToolUseProgress` flow, not by
   intercepting sub-agent internal events.

7. **Markdown cache invalidation**. Setting `item.Rendered = ""` forces a
   re-render. Forgetting to do this after a Collapse toggle leaves stale
   markdown on screen.

8. **`refreshViewportContent(true)` vs `false`**. Pass `false` when the user
   is mid-read (e.g. expanding a thinking block) so the viewport does not
   auto-scroll to bottom. Pass `true` only for new content arrival.

9. **Snapshot fixtures are byte-exact**. Trailing newline, ANSI escape
   codes, and width-padding all count. Use the `-update` flag and review the
   diff visually before committing.

10. **Tests share global state via `bootstrap.Global()`**. After P0-9 the
    test reset is mutex-guarded; still, prefer `t.Parallel()` only after
    confirming the test does not call `Global()`.

## Appendix D — Verification Checklist Per PR

Each agent should produce a report with:

1. **Files changed**: bullet list with one-line summary per file.
2. **Test command and result**: copy-pasted `go test ./<pkg>` output tail.
3. **Snapshot updates**: list of any `testdata/*.txt` files created or
   regenerated and a one-line description of each.
4. **Acceptance items**: checklist of the task's acceptance criteria
   ticked off.
5. **Intentionally deferred**: any acceptance items punted with reason.
6. **Regression smoke**: confirm `go vet ./...` clean and `go build ./...`
   succeeds.

## Appendix E — Glossary of New Types Introduced

| Type | Defined in | Introduced by |
| :--- | :--- | :--- |
| `RunPhase` | `internal/tui/runstate.go` | UI-1-1 |
| `RunUIState` | `internal/tui/runstate.go` | UI-1-1 |
| `RunSnapshot` | `internal/tui/runstate.go` | UI-1-1 |
| `AssistantTurnStarted` | `internal/agent/events.go` | UI-6-1 |
| `TranscriptGroup` | `internal/tui/transcript.go` | UI-6-2 |
| `TranscriptKind = TranscriptBatched` | `internal/tui/transcript.go` | UI-6-4 |
| `backgroundedTool` | `internal/tui/app.go` | UI-6-5 |
| `TipTrigger`, `TipCatalogue` | `internal/tui/tips.go` | UI-6-8 |
| `FooterState` | `internal/tui/footer.go` | UI-6-9 |
| `BranchInfo` | `internal/git/branch.go` | UI-6-9 |
| `CommandMeta` | `internal/commands/registry.go` | UI-4-4 |

---

## Suggested Implementation Order

1. **Phase 0** (P0-1 through P0-15) — land in numbered order; P0-1 first, P0-2/P0-3/P0-4 can be parallel afterwards.
2. **UI-0** (UI-0-1 through UI-0-5) — small, no dependencies between most; can be parallelized.
3. **UI-1** (UI-1-1 through UI-1-4) — sequential.
4. **X-1** snapshot helper — required before UI-1-4 lands its fixtures.
5. **UI-5-1** — semantic styles. Land before deep render changes in UI-3/UI-6.
6. **UI-2-1**, **UI-3-***, **UI-4-*** — independent, can parallelize.
7. **UI-6** — sequential within (6-1 → 6-2 → 6-3 → 6-4 → 6-5 → 6-6 → 6-7 → 6-8 → 6-9).
8. **UI-7** — last; depends on stable UI-6 and read-only toolset.
9. **X-2** added continuously as keys are introduced; **X-3** at the very end.

## How to Dispatch Tasks

Each task's **Agent prompt** is self-contained and may be pasted into a fresh
agent. Always include:

```
Read docs/TASKS-TUI.md task <ID>, and the referenced ADR sections. Do not
modify other tasks' files. Run go test ./<package> after the change. Report
back: files changed, tests added, test result, intentionally-deferred items.
```
