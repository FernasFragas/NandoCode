# Phase 22 Detailed Plan - Enhanced TUI and Input Handling

Date: 2026-05-07
Status: Core implementation landed with automated verification; not final-complete until manual REPL gates and documented partials are resolved
Source plans and references:

- `.codex/go-ollama-plan-AGENTS.md`
- `.codex/go-ollama-plan-HUMANS.md`
- `docs/PHASE-LOG.md`
- `docs/PROJECT-STATUS-AND-ONBOARDING.md`
- `docs/PHASE-8-DETAILED-PLAN.md`
- `docs/PHASE-9-DETAILED-PLAN.md`
- `docs/PHASE-21-DETAILED-PLAN.md`
- `book/ch13-terminal-ui.md`
- `book/ch14-input-interaction.md`
- `docs/ADR-001-TUI-USER-EXPERIENCE-IMPROVEMENTS.md`
- `docs/TASKS-TUI.md`
- `docs/CONTEXT-LATENCY-OPTIMIZATION-PLAN.md`
- `docs/INACCURATE-LISTING-RESPONSE-DEEP-DIVE-2026-05-17.md`
- `.codex/agent-context/learnings-memory.md`

## Roadmap Placement

Phase 22 is required before Phase 21, Phase 17, and Phase 18. The ADR-001 and TASKS-TUI plans make TUI run-state visibility and perceived response speed part of the core v0.1 ask/response flow. Implementing new server and remote surfaces before the local TUI clearly shows progress would spread an unresolved UX problem across more transports.

Phase 26 already delivered inline completion. When implementing Phase 22, scope it to both:

- the ADR-001/TASKS-TUI responsiveness work: modal correctness, animated run status, tool elapsed time, queue/retry/compaction visibility, permission/help polish, semantic style refactor, hierarchical activity display, and `/btw`;
- the original production TUI gaps: transcript virtualization, sticky scroll, bracketed paste, keybinding context stack, richer Vim behavior, render benchmarks, and live acceptance checks.

`docs/TASKS-TUI.md` is the agent-readable implementation breakdown for the ADR work. Its Phase 0 bug fixes are mostly complete; P0-15 editor invocation suspension remains a backlog item and should be closed in or before Phase 22 unless a fresh implementation review proves it is already handled.

## 2026-05-17 Plan Review And Execution Update

Phase 22 should not be implemented as one large TUI rewrite. It should be executed as a set of narrow agent batches with clear ownership, because the phase touches the same hot files repeatedly: `internal/tui/app.go`, `internal/tui/transcript.go`, `internal/tui/vim.go`, `internal/tui/styles.go`, and `internal/commands/registry.go`.

The current pre-Phase-22 context/latency/prompt-fidelity work is complete in code for the v0.1 gate: tracing, adaptive context, checkpoint hardening, project-analysis workflow, retrieval integration, oversized analysis fallback, and listing prompt accuracy are implemented. Phase 22 can therefore focus on local TUI correctness, visibility, rendering performance, and input behavior.

### 2026-05-17 Implementation Update (P22-A)

P22-A is implemented and test-validated.

Completed in code:

- `/memory edit` TUI-safe execution path:
  - Added TUI-level `/memory edit` handling that runs editor via `tea.ExecProcess` (no Bubble Tea suspension conflict).
  - Kept command-layer memory file safety/template seeding via shared helper.
- Permission modal correctness:
  - `Esc` now denies permission prompts explicitly.
  - `A` now writes literal target-scoped rule (`Tool(target)`), not broad `Tool(*)`.
- `/clear` transient reset hardening:
  - Clears transcript/tool map plus transient render/error/timing flags and closes picker.
- Modal/picker interaction:
  - Picker is closed when permission prompt opens.
  - Modal overlay path updated to center-overlay behavior instead of appending below content.

Validation evidence:

- `go test ./internal/commands ./internal/tui ./internal/tui/picker` pass.
- `go test ./...` pass.

Manual validation still pending (record in P22-I evidence):

- live `/memory edit` behavior with real `$EDITOR` in interactive TUI.
- live permission modal visual behavior across resize and active picker cases.

### Phase 22 Start Rule

Phase 22 may start once the remaining pre-Phase-22 work is live/manual validation only and the user explicitly authorizes starting Phase 22. Do not silently spend Phase 22 implementation time on Gate G0 or CL/PA code unless validation finds a real implementation bug or the user asks for it.

Before starting P22-A, the implementing agent must state whether the remaining pre-Phase-22 work is validation-only. If it is not validation-only, stop and record the implementation blocker.

### Pre-Phase-22 Verification Baseline

Before starting or immediately before authorizing P22-A, capture or confirm the following evidence when available. Missing live/manual evidence should be recorded as pending validation, not silently treated as pass:

- `go test ./...` passes.
- Listing accuracy checks from `docs/INACCURATE-LISTING-RESPONSE-DEEP-DIVE-2026-05-17.md` pass in a live TUI run:
  - `list all the files in @docs/` expands as tree, includes zero file bodies, and carries no listing-only answer contract.
  - `list all the files in @docs?content` warns that file bodies were included despite listing wording.
  - `review @docs/` and `summarize @docs/` remain content mode and do not carry a listing-only answer contract.
- `/prompt last` and `/trace last` show prompt shape and mention-expansion metadata for the listing run.
- Any mismatch in listing intent, prompt packing, prompt dump visibility, trace metadata, or reintroduced listing-answer constraints is fixed before Phase 22 continues.

### Scope Reconciliation

The original Phase 22 plan includes broad Vim, virtual transcript, keybinding, bracketed paste, and search work. ADR-001 and `docs/TASKS-TUI.md` add run visibility, activity hierarchy, queue/retry/compaction visibility, `/bg`, and `/btw`.

### 2026-05-18 Implementation Review

Current code implements the core Phase 22 reliability and visibility slices, but the full original "production Vim/activity" ambition is broader than what has landed. Treat the current status as code-implemented for the v0.1 TUI responsiveness path, with manual REPL evidence and a few explicitly deferred deep-interaction items still open.

Implemented and covered by automated tests:

- P22-A safety cleanup: TUI-safe `/memory edit`, permission modal Esc deny, literal-target always-allow rules, picker/modal cleanup, and `/clear` transient reset.
- P22-B run visibility: `RunPhase`, `RunUIState`, status priority, tick scheduling, retry/compaction/streaming/waiting/permission phases.
- P22-C semantic style roles and status snapshot fixtures at 60/80/120 columns.
- P22-D persistent status details: active tool elapsed time, queued prompt count, retry/compaction status, `/queue`, permission modal mode/Esc hints, and command picker metadata.
- P22-E transcript performance: line-budget transcript virtualization, height estimates/cache, sticky-scroll guard, streaming refresh throttling, markdown render caching, and `BenchmarkView1000Items`.
- P22-F foundations: bracketed paste preprocessing, keybinding context stack primitives, `gg`/`gG` chord interceptor, `/search` command with `n`/`N` navigation, and a full command-state parser union in `vim.go`.
- P22-G visibility slice: `AssistantTurnStarted` event, turn markers in transcript, active task line, tip line, and footer/status markers.
- P22-H side-workflow slice: `/bg` one-slot background status and `/btw <question>` isolated read-only side run that does not mutate main `app.Messages`.
- P22-I automated evidence: `go test ./...`, `go test -race ./internal/tui/...`, and `go test -bench=BenchmarkView1000Items ./internal/tui` pass on the 2026-05-18 local run.

Known partials and deferred follow-ups:

- Vim command-state parsing exists, but editor mutations for the full `3dw`, `ci"`, `da(`, dot-repeat, find-repeat, registers, linewise paste/yank, and indent behavior are not fully integrated into textarea editing.
- The keybinding context stack exists and is synced, but app dispatch still uses direct `handleKeyMsg` branches rather than a centralized binding resolver for every key.
- `P22-FIX-1` is now implemented: modal context is stack-top during prompts and scroll context is suppressed while modal is active.
- `/btw` is isolated and read-only, but if a main run is active it is queued to run after the main run finishes; true concurrent side-question execution remains a later architecture change.
- `P22-FIX-2` is now implemented: `/btw` runs with `ToolsetName: "read_only"` and the model/tool lookup path is restricted to `FileRead`, `Glob`, and `Grep`.
- Hierarchical activity display is a pragmatic visibility layer, not a full collapsible tree for nested sub-agents/tool batches.
- Click-to-expand tool-use panels and mouse lost-release recovery remain unimplemented.
- Manual REPL checks for editor suspension, modal overlay behavior, bracketed paste in a real terminal, run-status transitions, `/bg`, and `/btw` remain pending.

### 2026-05-18 Code Review Correction Tasks (Execution Update 2026-05-18)

These tasks were validated against the current codebase, `book/ch13-terminal-ui.md`, `book/ch14-input-interaction.md`, `docs/ADR-001-TUI-USER-EXPERIENCE-IMPROVEMENTS.md`, and `docs/TASKS-TUI.md`. They are correct follow-ups for the landed code. They are written as implementation tickets for the next agent; do not mark them complete without code and tests.

#### P22-FIX-1 - Make modal keybinding context highest priority
Status: `DONE`

Why:

- `book/ch14-input-interaction.md` says the active modal/confirmation context must take priority over task/chat contexts, with last matching context winning.
- The Phase 22 plan says `ContextModal` blocks all other contexts while a permission prompt is open.
- Current `internal/tui/app.go` appends `ContextModal` before Vim/scroll contexts, so `bindingStack.Top()` can become `ContextScroll` instead of `ContextModal`.

Files:

- `internal/tui/app.go`
- `internal/tui/keybindings_test.go`

Implementation:

- Change `syncBindingContexts()` so the stack order keeps `ContextGlobal` as the base context and puts `ContextModal` last whenever `state.App.PermissionPrompt != nil`.
- Do not push `ContextScroll` while a permission prompt is active; this matches the planned `ContextScroll` predicate.
- Decide whether Vim insert/normal context should remain below modal during prompts or be omitted entirely while modal is active. Prefer the smallest safe change: keep Vim context below modal only if existing tests need it.
- Keep direct `handlePermissionKeyMsg` handling intact; this task fixes the context-stack abstraction and tests, not the whole key-dispatch architecture.

Required tests:

- Update `TestSyncBindingContextsModalPriority` to assert `model.bindingStack.Top() == ContextModal`.
- Add/keep an assertion that `ContextScroll` is absent while a permission prompt is active.
- Keep an assertion that after clearing the prompt and resyncing, the previous Vim/scroll contexts become visible again.

Implemented:

- `internal/tui/app.go`: `syncBindingContexts()` now appends `ContextModal` last and suppresses `ContextScroll` while modal is active.
- `internal/tui/keybindings_test.go`: `TestSyncBindingContextsModalPriority` now asserts top context is `ContextModal`, validates `ContextScroll` absence during prompt, and validates context restoration after prompt close.

#### P22-FIX-2 - Add a real read-only tool manifest for `/btw`
Status: `DONE`

Why:

- `docs/ADR-001-TUI-USER-EXPERIENCE-IMPROVEMENTS.md` requires `/btw` to use a fresh side run with tools reduced to a read-only subset.
- `docs/TASKS-TUI.md` requires a read-only toolset that exposes read/search/discovery tools and excludes Bash, FileEdit, FileWrite, WebFetch, and other side-effect tools.
- Current `/btw` uses `permissions.ModePlan`, but `agent.buildToolDefs()` still exposes the full agent registry to the model. Permission mode is necessary but not sufficient for the planned UX and safety model.

Files:

- `internal/tools/builtin/builtin.go`
- `internal/agent/input.go`
- `internal/agent/agent.go`
- `internal/agent/stream.go`
- `internal/agent/tools.go`
- `internal/tui/app.go`
- related tests under `internal/tools/builtin`, `internal/agent`, and `internal/tui`

Implementation:

- Add a toolset selector to `agent.Input`, for example `ToolsetName string` with supported values `""`/`"default"` and `"read_only"`.
- Add a read-only built-in registry constructor in `internal/tools/builtin`, for example `NewReadOnlyRegistry()`, containing current repo read-only/discovery tools: `FileRead`, `Grep`, `Glob`. Include `TodoRead` only if the implementation decision treats session todo reads as acceptable for side questions; document the choice in tests.
- Teach `Agent` to derive an effective registry/tool lookup from `Input.ToolsetName`.
- Ensure both model tool definitions (`buildToolDefs`) and tool execution lookup (`lookupTool`/execution path) use the same effective registry for the run. Do not only filter definitions; unknown/mutating tools must also be rejected if a model somehow calls them.
- Keep `permissions.ModePlan` in `/btw` as defense in depth.
- Set `ToolsetName: "read_only"` in `startBTWPrompt`.

Required tests:

- Built-in registry test: `NewReadOnlyRegistry()` includes `FileRead`, `Grep`, `Glob` and excludes `Bash`, `FileWrite`, `FileEdit`, `WebFetch`, `TodoWrite`.
- Agent request test: a run with `ToolsetName: "read_only"` sends only read-only tool definitions to the LLM request.
- Agent execution test: if a fake model asks for `Bash` or `FileWrite` during a read-only run, the tool result is an unknown/rejected tool error and no tool executes.
- TUI test: `/btw` builds an agent input with `HistoryPolicyLatestOnly`, `PermissionMode: plan`, and `ToolsetName: "read_only"`.

Implemented:

- `internal/agent/input.go`: added `ToolsetName` plus constants `ToolsetDefault` and `ToolsetReadOnly`.
- `internal/tools/builtin/builtin.go`: added `NewReadOnlyRegistry()` with `FileRead`, `Glob`, `Grep`.
- `internal/agent/agent.go`, `internal/agent/stream.go`, `internal/agent/tools.go`: agent now resolves an effective per-run registry; both tool defs and tool execution use the same registry.
- `internal/tui/app.go`: `/btw` now sets `ToolsetName: agent.ToolsetReadOnly` (with `ModePlan` retained).
- Tests added:
- `internal/tools/builtin/builtin_test.go`: `TestNewReadOnlyRegistry`.
- `internal/agent/agent_test.go`: `TestAgentReadOnlyToolsetFiltersToolDefs`, `TestAgentReadOnlyToolsetRejectsMutatingToolCalls`.
- `internal/tui/app_test.go`: `TestBTWCommandUsesReadOnlyLatestOnlyPlanInput`.

#### P22-FIX-3 - Align `/btw` behavior with the ADR or explicitly defer concurrency
Status: `DEFERRED`

Why:

- The ADR describes `/btw` as a side question that does not interrupt the main run.
- Current code queues `/btw` until the main run finishes when `ActiveRun` is true.
- This queued behavior is documented now, but Phase 22 cannot claim the full ADR `/btw` behavior unless concurrent side runs are implemented.

Files:

- `internal/tui/app.go`
- `internal/tui/app_test.go`
- `USER_MANUAL.md`
- `docs/PHASE-LOG.md`

Implementation options:

- Option A, implement the ADR behavior: add a separate `/btw` run slot with its own context/cancel function/event buffer, render it in an inset, and ensure the main run `RunUIState` remains unchanged.
- Option B, defer the ADR behavior: keep current queued behavior and create a named follow-up task/phase for true concurrent side conversations. If deferring, keep the user manual wording that active-run `/btw` is queued.

Required tests if implementing Option A:

- Starting `/btw` while a main run is active does not flip the main run state or replace `activeRunCtx`.
- Side-run terminal does not mutate main `app.Messages`.
- Recursion is rejected while a side run exists.
- `/btw done` or empty input in side mode cancels/clears the side run.

Required docs if choosing Option B:

- `docs/PHASE-22-DETAILED-PLAN.md` and `docs/PHASE-LOG.md` name the follow-up phase/task and state that current Phase 22 behavior is queued.

Decision:

- Chose Option B for Phase 22 closure: keep queued `/btw` behavior for active-run cases and defer true concurrent side-run architecture.
- Deferred scope is now tracked as `P23-UX-2` in Phase 23 planning: separate side-run context/event lane, inset rendering, and run-isolation tests.

#### P22-FIX-4 - Decide and execute or defer deep Vim and activity requirements
Status: `DEFERRED`

Why:

- `book/ch14-input-interaction.md` specifies full Vim editor behavior: command-state transitions separated from effects, motions/operators/text objects, dot-repeat, find-repeat, registers, and linewise paste behavior.
- `book/ch13-terminal-ui.md` describes mouse selection and lost-release recovery.
- The current code has a parser union and visibility layer but not full textarea mutations, full collapsible activity trees, click-to-expand tool panels, or mouse lost-release recovery.

Files if implementing now:

- `internal/tui/vim.go`
- `internal/tui/app.go`
- `internal/tui/transcript.go`
- new or existing TUI tests

Implementation policy:

- If these remain Phase 22 scope, split into separate tasks:
  - `P22-FIX-4A`: textarea-integrated Vim mutations/repeat/registers.
  - `P22-FIX-4B`: collapsible hierarchical activity tree and tool-panel click toggles.
  - `P22-FIX-4C`: mouse lost-release recovery.
- If they are deferred, record the deferral in this plan and `docs/PHASE-LOG.md` with a named future phase/task. Do not leave them as ambiguous Phase 22 completion blockers.

Decision:

- Deferred to Phase 23 as explicit follow-up tasks:
- `P23-INPUT-1`: full textarea-integrated Vim mutations/repeat/registers.
- `P23-UX-3`: collapsible hierarchical activity tree and click-to-expand tool panels.
- `P23-INPUT-2`: mouse lost-release recovery.
- Phase 22 is treated as closed for implemented TUI responsiveness/visibility scope, with these deep-interaction items moved out of the gate.

For implementation, the required order is:

1. **P22-A - Safety and state cleanup:** implemented and automated-test validated; live editor/modal checks still pending.
2. **P22-B - Run visibility foundation:** implemented and automated-test validated.
3. **P22-C - Semantic styles and snapshots:** implemented for current status rendering; snapshot fixtures now cover phase states at 60/80/120 columns.
4. **P22-D - Persistent status details:** implemented for tool elapsed time, queue/retry/compaction status, `/queue`, permission modal polish, literal-target allow rules, and command metadata.
5. **P22-E - Transcript performance:** implemented for line-budget virtualization, height estimates/cache, sticky-scroll guard, streaming throttling, and benchmark coverage.
6. **P22-F - Input system:** implemented for bracketed paste, context-stack primitives, chord interceptor, transcript search, and command-state parser. Full textarea-editing Vim motions/repeat/register behavior remains partial.
7. **P22-G - Hierarchical activity display:** implemented as turn markers, active task line, tips, and footer/status bands. Full collapsible nested tree remains partial.
8. **P22-H - Background and side workflows:** implemented as one-slot `/bg` status plus read-only isolated `/btw`; active-run `/btw` is queued, not concurrent.
9. **P22-I - Verification and documentation:** automated tests/benchmark recorded; manual REPL checks remain pending.

### Agent Ownership Matrix

| Batch | Primary files | Agent task | Required tests |
| --- | --- | --- | --- |
| P22-A | `internal/commands/registry.go`, `internal/tui/app.go`, `internal/tui/permission.go`, `internal/tui/picker/*` | Fix editor suspension, modal overlay/Esc, `/clear` transient reset, picker/modal races, slash picker anchor scope. | `go test ./internal/commands ./internal/tui ./internal/tui/picker` |
| P22-B | `internal/tui/runstate.go`, `internal/tui/app.go`, `internal/tui/messages.go` | Add `RunPhase`, `RunUIState`, `RunSnapshot`, tick scheduling, phase transitions, stop-tick guarantee. | Phase transition table tests, tick lifecycle tests. |
| P22-C | `internal/tui/styles.go`, `internal/tui/testdata/*` | Add semantic style roles and snapshot fixture helper; migrate render sites without visual churn. | Style non-zero tests, snapshot tests at 60/80/120 columns. |
| P22-D | `internal/tui/app.go`, `internal/tui/transcript.go`, `internal/commands/registry.go`, `internal/tui/picker/command_provider.go` | Add tool elapsed display, queue/retry/compaction segments, `/queue`, permission modal fields, literal target always-allow, command metadata. | Command tests, status snapshot tests, permission rule tests. |
| P22-E | `internal/tui/transcript.go`, `internal/tui/app.go`, `internal/tui/render_benchmark_test.go` | Add virtual transcript, height cache, sticky scroll, markdown render cache for completed assistant blocks, cheap active-stream rendering. | Virtual window tests, sticky scroll tests, `BenchmarkView1000Items`, existing render benchmarks. |
| P22-F | `internal/tui/vim.go`, `internal/tui/input.go`, `internal/tui/keybindings.go`, `internal/tui/transcript.go` | Add bracketed paste preprocessing, context stack, chord interceptor, transcript search, full Vim command states and repeat behavior. | Vim transition table tests, paste tests, keybinding/chord tests, search tests. |
| P22-G | `internal/agent/events.go`, `internal/agent/agent.go`, `internal/tui/transcript.go`, `internal/tui/app.go`, `internal/tui/footer.go`, `internal/tui/tips.go` | Add `AssistantTurnStarted`, transcript groups, tree render, collapsible batches, active task line, tips, footer bands. | Agent event tests, group helper tests, tree snapshot tests, footer/tip tests. |
| P22-H | `internal/tools/registry.go`, `internal/agent/input.go`, `internal/tui/app.go`, `internal/commands/registry.go` | Add read-only toolset, one-slot background handling, `/bg`, `/btw` isolated side question. | Read-only tool rejection tests, background slot tests, `/btw` isolation tests. |
| P22-I | docs, manual test records | Update `PHASE-LOG`, onboarding status, user manual, manual evidence. | `go test ./...`, `go test -race ./internal/tui/...`, render benchmarks, manual exit gate. |

### Final Agent Execution Contract

The table above defines ownership. The checklist below defines what each batch must actually deliver. If an agent discovers source reality differs from this plan, it must report the mismatch and update the relevant docs instead of guessing.

#### P22-A - Safety And State Cleanup

Start here. Own only the editor/modal/picker safety surface.

- Verify whether P0-15 still exists: `/memory edit` currently launches `$EDITOR` through `commands.Registry`; if that still cannot safely suspend/release Bubble Tea, fix it here.
- Fix permission modal rendering so it behaves as a real blocking modal in the TUI and does not leave picker/input state active underneath.
- Make Escape behavior explicit for permission/modal flows.
- Ensure `/clear` resets transient TUI state that would confuse the next prompt.
- Fix picker/modal races and slash-picker anchor scope.
- Done when `go test ./internal/commands ./internal/tui ./internal/tui/picker` passes and remaining manual modal/editor checks are listed.

#### P22-B - Run Visibility Foundation

Single owner because this is the central run-state path.

- Add TUI-local `RunPhase`, `RunUIState`, and a read-only snapshot helper for tests/rendering.
- Derive phase transitions from existing TUI and agent events.
- Use deterministic status priority: permission first, then running tool, retry/compaction, streaming/thinking/waiting, queued, idle.
- Add tick scheduling and elapsed time support.
- Prove ticks stop after terminal, abort, and idle.
- Done when run-phase transition tests, status tests, and tick lifecycle tests pass.

#### P22-C - Semantic Styles And Snapshots

Prepare stable rendering tests before larger visual changes.

- Add semantic style roles in `internal/tui/styles.go`.
- Keep visual churn minimal; do not add full theme variants in this phase.
- Add stable snapshot/render helpers and width-specific fixtures for 60, 80, and 120 columns.
- Done when style non-zero tests and snapshot tests pass.

#### P22-D - Persistent Status Details

Build on P22-B and P22-C. Do not implement the full hierarchy here.

- Show active tool elapsed time.
- Show queued prompt count, retry state, and compaction state.
- Add `/queue` if command metadata is ready.
- Improve permission modal fields/context and command picker metadata.
- Ensure `A` always-allow creates a literal-target rule, not a broad `Tool(*)` rule.
- Done when command tests, status snapshots, and permission rule tests pass.

#### P22-E - Transcript Performance

Make long sessions fast before adding hierarchy.

- Implement virtual transcript rendering, height cache, sticky scroll, and render benchmarks.
- Preserve full transcript navigation: virtualization is rendering-only and must not truncate history.
- Keep scroll/search/inspection working across old messages, tool panels, and thinking blocks.
- Render active streaming text cheaply; render/cache final markdown after completion.
- Done when virtual window tests, sticky scroll tests, existing render benchmarks, and `BenchmarkView1000Items` pass.

#### P22-F - Input System

Pure Vim library/test work may start earlier if it stays inside `internal/tui/vim.go` and tests. App/keybinding integration waits until transcript and status integration are stable.

- Add bracketed paste preprocessing; pasted bytes must not enter the Vim command state machine.
- Add keybinding context stack and chord interceptor.
- Add substring transcript search, not regex search.
- Expand Vim command states, motions, text objects, repeat, find-repeat, registers, and paste behavior.
- Done when Vim transition, paste, keybinding, chord, and search tests pass.

#### P22-G - Hierarchical Activity Display

Wait for P22-B and P22-E.

- Add assistant turn grouping and any needed `AssistantTurnStarted`-style event or equivalent grouping signal.
- Render transcript groups/tree structure clearly.
- Support collapsible tool batches and clearer nested sub-agent/tool activity.
- Add active task line, tips, and footer bands.
- Prepare display affordances for background activity only; actual `/bg` behavior belongs to P22-H.
- Done when agent event tests, group helper tests, tree snapshots, footer/tip tests, and activity-display snapshots pass.

#### P22-H - Background And Side Workflows

Crosses tools, agent input, commands, and TUI state; keep this single-owner.

- Implement actual one-slot `/bg` behavior and output inspection.
- Implement `/btw <question>` as an isolated side question.
- Enforce read-only/minimal context for `/btw`.
- Ensure `/btw` does not mutate the main conversation history.
- Add read-only toolset restrictions and tests.
- Done when read-only tool rejection, background slot, `/bg`, `/btw`, isolation, and cleanup tests pass.

#### P22-I - Verification And Documentation

This is the only batch that may mark Phase 22 complete.

- Run `go test ./...`.
- Run `go test -race ./internal/tui/...`.
- Run render benchmarks, including `BenchmarkView1000Items`.
- Run the manual REPL checks from this plan and ADR/TASKS-TUI.
- Update `docs/PHASE-LOG.md`.
- Update onboarding/status docs and user manual if user-visible behavior changed.
- Done only when automated, benchmark, and manual evidence is recorded.

### Parallelization Guidance

- P22-A can be split across two agents if they do not edit the same files: one owns `internal/commands/registry.go`, one owns modal/picker files.
- P22-B should be single-owner because it changes the central run-state path in `app.go`.
- P22-C can run after P22-B types exist and before P22-D/P22-G render work.
- P22-E and P22-F should not run in parallel against `app.go` unless one agent owns only library files and another only integration. Pure Vim library/test work may start earlier only if it stays isolated from `app.go`.
- P22-G must wait for P22-B and P22-E, because it depends on run phases and transcript rendering.
- P22-G may prepare display affordances for background activity, but P22-H owns actual `/bg` and `/btw` behavior.
- P22-H must wait for P22-G unless `/btw` is intentionally deferred.

### Required Agent Handoff Format

Every Phase 22 worker should end with:

- files changed;
- tests run;
- unchecked manual checks;
- known risks;
- any follow-up tasks created for the next batch.

Do not mark Phase 22 complete until P22-I has evidence for automated tests, race tests, benchmarks, and manual REPL checks.

## ADR/TASKS-TUI Integration

The following slices are required Phase 22 scope:

1. **UI-0 - Modal and permission rendering correctness**
   - Replace fake overlays with safe rendering, handle Escape correctly, reset transient state on `/clear`, fix picker/modal races, and scope slash-command picker anchors.
2. **UI-1 - Animated run status**
   - Add TUI-local `RunUIState`, `RunPhase`, tick scheduling, phase transitions, status priority, and tests.
3. **UI-2 - Tool elapsed time**
   - Show elapsed time on existing tool panels without introducing a separate strip; UI-6 owns the richer hierarchy.
4. **UI-3 - Queue, retry, and compaction visibility**
   - Show queued prompt count, retry state, and compaction state in the status/footer; add `/queue` if command metadata is ready.
5. **UI-4 - Permission modal and help polish**
   - Improve modal context, fix always-allow literal target scope, add first-run and `/clear` help, and improve command picker metadata.
6. **UI-5 - Semantic style refactor**
   - Introduce semantic style roles without adding full theme variants in this pass.
7. **UI-6 - Hierarchical activity display**
   - Add `AssistantTurnStarted`, transcript groups, plain-indent tree rendering, collapsible batches, background execution and `/bg`, animated active task line, tips, and footer bands.
8. **UI-7 - `/btw` side question**
   - Add a read-only isolated side-question path after UI-6 is stable.

The latency plan's Phase 4 streaming render optimization is also Phase 22-relevant: streamed assistant content should render as cheap plain text while active, throttle viewport refreshes, and cache final markdown rendering on terminal completion.

## Goal

Phase 22 upgrades the Bubble Tea TUI from its Phase 7 foundation to production-quality input handling and rendering, based on the deep lessons of Chapters 13 and 14. The Phase 7 TUI established the structure: Bubble Tea model, vim mode, permission broker, transcript rendering, and slash command handling. Phase 22 fills the gaps that separate a working prototype from a polished, fast, and keyboard-complete REPL.

The user-visible goals are:

- Full vim motion vocabulary: `3dw`, `ci"`, `gg`, `G`, `.` (dot-repeat), `;`/`,` (find-repeat), and all twelve `CommandState` variants.
- Bracketed paste that never triggers vim commands from pasted escape sequences.
- A transcript that stays under 16ms render time even with 1000 messages (60fps budget).
- Sticky scroll that follows the bottom during streaming and stops when the user scrolls up.
- Five keybinding contexts with a context stack; modal dialogs block global bindings.
- Chord shortcuts (`gg`, `gG`) using a 1-second chord timeout and a `ChordInterceptor`.
- Click-to-expand collapsed tool-use panels in the transcript.
- Mouse lost-release recovery preventing the input from getting stuck in drag state.

Deliverables:

- `internal/tui/vim.go` — extended to all twelve `CommandState` variants, full operator/motion/text-object set, dot-repeat, find-repeat, registers.
- `internal/tui/input.go` — bracketed paste detection; `isPasted` flag on key events; pasted text routes to insert mode regardless of current vim mode.
- `internal/tui/transcript.go` — height cache, virtual viewport, sticky scroll, `scrollOffset` tracking.
- `internal/tui/keybindings.go` — five-context stack, chord interceptor, chord timeout, context push/pop.
- `internal/tui/app.go` — wired to all new components.
- `internal/tui/vim_test.go` — full `CommandState` transition table tests.
- `internal/tui/keybindings_test.go` — context stack, chord, resolution tests.
- `go test -race ./internal/tui/...` passes.
- Phase log update.

## Definition Of Success

The original Phase 22 exit gate is a set of five manual REPL checks:

1. Vim motions: in normal mode, type `3dw` and confirm three words are deleted. Type `.` and confirm it repeats.
2. Find-repeat: type `fa` (find 'a'), then `;` to find the next 'a' forward, then `,` to reverse. Confirm direction reversal.
3. Bracketed paste: paste a shell snippet containing escape sequences (e.g., `echo $'\x1b[1m'`). Confirm that no vim command is triggered and the text appears verbatim.
4. Virtual scroll: in a session with 100+ transcript messages, scroll to the middle. Confirm render is smooth and the frame budget (target ≤ 16ms) is met by inspecting the debug frame counter.
5. Chord shortcut: in normal mode, press `g` then `g` within 1 second. Confirm the transcript scrolls to the top.

All five must pass. All existing tests must still pass.

The reviewed Phase 22 gate adds ADR-001 checks that must also pass before the phase is considered complete:

6. Run visibility: submit a prompt and confirm the status bar moves through waiting, streaming, tool, permission, retry, compaction, and done/idle states as applicable.
7. Status details: confirm queue length, active tool elapsed time, retry notice, compaction notice, active tool count, and token count appear without overlapping at 60, 80, and 120 columns.
8. Permission modal: confirm the modal overlays the transcript, Escape denies, `A` creates a literal-target rule, and picker state is closed while the modal is active.
9. Hierarchical activity: run a prompt with multiple tools and confirm assistant turns, tool groups, nested sub-agent entries, collapsed batches, and active task line render clearly.
10. Background and side workflows: confirm one long-running eligible tool can be backgrounded and inspected with `/bg`, and `/btw <question>` runs in isolated read-only context without mutating the main conversation.

Automated Phase 22 completion additionally requires:

- `go test ./...`
- `go test -race ./internal/tui/...`
- `go test -bench=BenchmarkView1000Items ./internal/tui`
- existing render benchmarks in `internal/tui/render_benchmark_test.go`
- dependency and network policy checks, if the scripts are available in the checkout

## Baseline Analysis From Implemented Phases

### Phase 0 - Security and Supply Chain

Implemented: SECURITY.md, dependency allowlist, network policy checker.

Phase 22 implications:

- Phase 22 does not add new dependencies. The chord interceptor, bracketed paste handler, and virtual scroll are all implemented in pure Go without new packages.
- The key parsing pipeline handles raw bytes including escape sequences. Bracket paste handling means the TUI processes `\x1b[200~` and `\x1b[201~` markers. These are well-defined terminal sequences and do not create new injection surfaces if `isPasted` is correctly set and respected by the vim handler.
- No memory contents, prompts, or model outputs are introduced into the keybinding or input layers.

### Phase 1 - CLI, Paths, Logging, Scaffold

Implemented: Cobra CLI, logging, paths.

Phase 22 implications:

- A `--debug-fps` flag can be added to the REPL command to enable per-frame timing output. This is a diagnostic aid for verifying the 16ms frame budget in the exit gate.
- No new log messages are added to the keybinding hot path. Key events at DEBUG level are acceptable only in debug mode.

### Phase 2 - LLM Client

No direct implications. Phase 22 does not touch the LLM client.

### Phase 3 - Tool Interface and Starter Tools

No direct implications. Phase 22 does not touch tools.

### Phase 4 - Agent Loop

Implemented: `agent.Run`, event channel, `agent.Event` types.

Phase 22 implications:

- Virtual scroll must handle rapid transcript growth from streaming `AssistantTextDelta` events. The height cache must invalidate correctly when a transcript item grows (partial streaming text becoming a completed paragraph).
- Sticky scroll must resume auto-scroll-to-bottom as new events arrive, unless the user has manually scrolled up.

### Phase 5 - Permission System

Implemented: central resolver, TUI prompt callback, `PermissionBroker`.

Phase 22 implications:

- The permission modal is the primary driver of the `ContextModal` keybinding context. When the modal is visible, `ContextModal` is the top-of-stack context. The modal's `y`/`n` keybindings take absolute priority over all other contexts.
- Popping `ContextModal` when the modal closes must re-expose the previous context stack state.

### Phase 6 - State Layer

Implemented: `state.Store[state.App]`, reactive store.

Phase 22 implications:

- `state.App` does not need new fields for the keybinding context stack. The context stack lives entirely in the TUI model; it does not need to survive REPL restarts.
- The scroll offset and `atBottom` flag live in the TUI model's transcript component, not in `state.App`.
- Height cache entries could be stored in `state.App` for future persistence, but Phase 22 keeps them in-memory only.

### Phase 7 - Bubble Tea TUI and REPL

Implemented: Bubble Tea model, textarea, viewport, Vim state (Insert/Normal/Visual with basic motions), permission broker/modal, transcript rendering, slash commands.

Phase 22 implications: this phase directly extends Phase 7.

Current vim implementation gaps:

- Only three `CommandState` variants: `idle`, basic operator, basic motion. Missing: `count`, `operatorCount`, `operatorFind`, `operatorTextObj`, `find`, `g`, `replace`, `indent`.
- No text objects (`iw`, `aw`, `i"`, `a"`, etc.).
- No dot-repeat (`RecordedChange` union).
- No find-repeat (`;`/`,`).
- No register with linewise/characterwise awareness.
- No `gg`/`G` motions.
- No `>>`/`<<` indent operators.

Current transcript gaps:

- Renders all items every frame regardless of viewport position.
- No height cache.
- No sticky scroll (always auto-scrolls regardless of user scroll position).
- No virtual viewport (all items mounted).

Current keybinding gaps:

- Global keybindings only. No context stack. No modal priority.
- No chord support.
- No bracketed paste detection in the key handling path.

### Phase 8 - Memory

No direct implications. Phase 22 does not touch memory.

### Phase 9 - Hooks

Implemented: hook notices forwarded to TUI as `HookNotice` transcript items.

Phase 22 implications:

- `HookNotice` transcript items must participate in the virtual scroll height cache. If a hook fires during a run, a new transcript item is appended and must be included in the height calculation.

### Phase 10 - MCP Integration

No direct implications. MCP tool events already flow through the agent event channel and appear as transcript items.

### Phase 11 - Sub-agents and Fork

No direct implications. Sub-agent events appear as transcript items; the virtual scroll handles them uniformly.

### Phase 12 - Skills

No direct implications.

### Phase 13 - Slash Commands and Config UX

Implemented: full command registry, `/models`, `/memory`, `/hooks`, etc.

Phase 22 implications:

- Slash command invocation is typed in the input box. With the extended vim mode, the `/` key in Normal mode starts a search (basic substring filter on transcript). Phase 22 must not shadow the slash command flow: `/` at the start of a line in Insert mode still triggers slash command completion, not vim search. Vim search (`/`) is a Normal mode only binding.
- The `ContextScroll` context must not capture `/` when the input is focused in Insert mode.

### Phase 14 - Tasks

Implemented: background tasks, task broadcast events.

Phase 22 implications:

- Task progress events produce new transcript items. The virtual scroll append path must be cheap (O(1) append + height cache miss on new items, not O(n) full re-render).

### Phase 15 through Phase 21

No direct implications for the input/render subsystem. Phase 22 is a TUI-only phase.

## Documentation and Log Findings

Chapter 13 (`book/ch13-terminal-ui.md`) provides the production lessons for rendering:

- Intern characters and styles into integer pools (not directly applicable to Bubble Tea, but the principle of avoiding per-frame allocation applies).
- Track a height cache per transcript item, keyed by item ID and terminal column width.
- Damage rectangles: only diff the rows that changed.
- `OffscreenFreeze`: freeze React subtrees for off-screen messages. In Bubble Tea, the equivalent is not mounting the `viewport` content for items outside the visible window.
- Sticky scroll: Bubble Tea's `viewport` does not natively support sticky scroll. Phase 22 must implement it explicitly by tracking `atBottom bool` and calling `viewport.GotoBottom()` on new content unless the user has scrolled up.

Chapter 14 (`book/ch14-input-interaction.md`) provides the production lessons for input:

- The twelve `CommandState` variants form an exhaustive discriminated union. Go sealed interfaces enforce this at the type level.
- Transitions are pure functions: `(CommandState, Key) -> (CommandState, optional side-effect)`. The side-effect is a closure capturing the editor state, not a direct mutation.
- Bracketed paste: `\x1b[200~` / `\x1b[201~` delimiters. Set `isPasted = true`. Skip keybinding resolution for pasted events.
- Context stack: context priority determines binding resolution. The last context added wins for a given key.
- Chord state machine with a 1-second timeout and a `ChordInterceptor` component.
- `PersistentState`: `lastChange` for dot-repeat, `lastFind` for find-repeat, `register` for yank buffer.

The Phase 7 vim implementation used a simpler three-variant state machine. Phase 22 replaces it with the full twelve-variant union, preserving the existing `VimMode` enum (Insert/Normal/Visual) as the top-level discriminant.

## Evaluation of the Original Phase 22 Goal Statement

The goal statement is architecturally correct. It needs clarification in the following areas:

- It lists `CmdOperatorFind` as one of the twelve variants. The authoritative set from Chapter 14 is: `idle`, `count`, `operator`, `operatorCount`, `operatorFind`, `operatorTextObj`, `find`, `g`, `operatorG`, `replace`, `indent`. Phase 22 implements all eleven transitions (the twelfth state `idle` is the reset state, so there are eleven named non-idle states). The Go union will have twelve total variants including `Idle`.
- The goal statement says the virtual scroll should render within 16ms for 1000 items. Chapter 13 clarifies that the expensive part is Yoga layout (not applicable in Bubble Tea), markdown re-parse (applicable), and the diff (Bubble Tea handles this). Phase 22's target is: the `View()` function for a 1000-item transcript completes in under 16ms when only 15-25 items are in the visible window. This is achievable by not rendering off-screen items.
- The goal statement mentions `PageUp`/`PageDown`, `Home`/`End` in Normal mode as Jump handles. These are straightforward viewport scroll commands and belong in the `ContextScroll` context.
- The goal statement mentions `/` in Normal mode starts search. Phase 22 implements basic substring filter on transcript content, not a full regex search. Full search is a future TUI follow-up, not part of the active v0.1 roadmap.

## Final Phase 22 Scope

In scope:

- All twelve `CommandState` variants in `internal/tui/vim.go`.
- Full operator set: `d`, `c`, `y`, `>`, `<`.
- Full motion set: `w/W/b/B/e/E`, `f/F/t/T`, `0/$`, `^`, `gg/G`, `h/j/k/l`.
- Text objects: `iw`, `aw`, `i"`, `a"`, `i'`, `a'`, `` i` ``, `` a` ``, `i(`, `a(`, `i[`, `a[`, `i{`, `a{`.
- Dot-repeat via `RecordedChange` sealed interface.
- Find-repeat via `;`/`,` using `PersistentState.lastFind`.
- Unnamed register with linewise/characterwise flag.
- Count prefix (`3dw`, `5w`, `2dd`).
- Bracketed paste in `internal/tui/input.go` with `isPasted` flag.
- Paste in Normal vim mode enters Insert mode, pastes text, returns to Normal.
- Height cache in `internal/tui/transcript.go` (keyed by item ID + column width).
- Virtual viewport: only render items in window ± 2-item buffer.
- Sticky scroll with `atBottom` tracking.
- `PageUp`/`PageDown`/`Home`/`End` scroll bindings in `ContextScroll`.
- Five keybinding contexts in `internal/tui/keybindings.go`.
- Context stack push/pop.
- Chord interceptor with 1-second timeout.
- Default chords: `g g` (go to top), `g G` (go to bottom).
- `/` in Normal mode: basic transcript search (substring highlight, `n`/`N` next/prev match).
- Click to expand/collapse tool-use panels (mouse click on collapsed tool item).
- Lost-release recovery for mouse drag state.
- `internal/tui/vim_test.go` with full transition table coverage.
- `internal/tui/keybindings_test.go` with context stack and chord tests.
- `go test -race ./internal/tui/...` passes.

Out of scope:

- Visual block mode (`ctrl+v`).
- Macros (`q`/`@`).
- Multi-line text objects.
- Multiple named registers.
- Ex-mode (`:` commands).
- Regex search in transcript (basic substring only in Phase 22).
- Window splits or tab pages.
- Full mouse selection (character/word/line selection drag).
- Kitty keyboard protocol negotiation (Bubble Tea handles terminal protocol internally).
- Custom user keybinding configuration file (Phase 13 scope for the slash command config UX).
- 60fps rendering target via custom renderer (Bubble Tea's renderer is used as-is; the 16ms target is achieved by reducing `View()` complexity, not by replacing the renderer).

## Target User Experience

### Extended Vim Motions

In Normal mode, the following now work correctly:

- `3dw` — delete 3 words forward.
- `ci"` — change inside double-quote text object; enters Insert mode with quote content deleted.
- `da(` — delete around parentheses (including the parens).
- `gg` — jump to top of input buffer.
- `G` — jump to end of input buffer.
- `.` — repeat the last change.
- `fa` — find next 'a' in the line.
- `;` — repeat the last find forward.
- `,` — repeat the last find backward.
- `>>` — indent the current line.
- `r<char>` — replace character under cursor.
- `2dd` — delete 2 lines (in multi-line input mode).
- `p` — paste register content after cursor; if register is linewise, paste on the line below.
- `P` — paste register content before cursor.
- `yy` or `Y` — yank current line into register (linewise).

### Bracketed Paste

When bracketed paste mode is active (the default on modern terminals), pasting text never triggers vim commands. A paste of `\x1b[1m bold \x1b[m` is treated as literal text, not as an escape sequence. In Normal mode, pasting text automatically transitions to Insert mode, inserts the pasted text, and returns to Normal mode.

### Virtual Scroll and Sticky Scroll

With a 200-message conversation:

- The transcript renders only the 15-25 messages visible in the viewport.
- The full transcript remains available; virtualization is a rendering optimization only, not history truncation.
- Users can still scroll through the whole conversation, jump to top/bottom, inspect older messages and tool panels, and use transcript search.
- As the agent streams new tokens, the transcript auto-scrolls to the bottom.
- If the user presses the Up arrow or scroll wheel to read earlier messages, auto-scroll stops.
- Pressing `End` or `ctrl+end` or the `g G` chord re-enables auto-scroll.

### Keybinding Contexts

Five contexts, in priority order (highest first):

1. `ContextModal` — active when permission dialog is open. `y`/`n`/`a`/`Enter`/`Escape` are modal-only. No other key fires.
2. `ContextVimNormal` — active when vim is in Normal mode. All vim commands, `gg`, `/`, scroll.
3. `ContextVimInsert` — active when vim is in Insert mode. Mostly passthrough; `Escape` exits to Normal.
4. `ContextScroll` — active when transcript is scrollable and no modal. `PageUp`/`PageDown`/`Home`/`End`.
5. `ContextGlobal` — always active. `/help`, `/clear`, `ctrl+c`, `ctrl+d`.

When a modal opens, `ContextModal` is pushed onto the stack. When it closes, it is popped. Resolution walks the stack from top to bottom; first match wins.

### Chord Shortcuts

Default chords (in ContextVimNormal):

- `g g` (press `g`, then `g` within 1 second) — go to top of transcript.
- `g G` (press `g`, then `G` within 1 second) — go to bottom of transcript.
- `d d` (press `d`, then `d` within 1 second) — delete current line (also handled by vim as `dd` operator self-repeat).

The `ChordInterceptor` arms a 1-second timer after the first key of a potential chord. If the second key arrives within 1 second, the chord fires. If the timer expires, the first key is passed through as a normal key event (it does not fall on the floor).

## Architecture

### Extended CommandState Union

```go
// CommandState is the current parsing state inside vim Normal mode.
// It is a sealed interface. Add new variants only with full transition tests.
type CommandState interface{ isCommandState() }

// CmdIdle is the initial state. Any normal-mode command starts here.
type CmdIdle struct{}

// CmdCount accumulates a numeric prefix before an operator or motion.
// Example: typing "3" enters CmdCount{Digits:"3"}.
type CmdCount struct{ Digits string }

// CmdOperator waits for a motion or text object after an operator key.
// Example: typing "d" enters CmdOperator{Op: OpDelete, Count: 1}.
type CmdOperator struct {
    Op    Operator
    Count int
}

// CmdOperatorCount accumulates a second count after operator+count.
// Example: typing "d2" enters CmdOperatorCount{Op: OpDelete, Count: 1, Digits: "2"}.
type CmdOperatorCount struct {
    Op    Operator
    Count int
    Digits string
}

// CmdOperatorFind waits for a target character after operator+find key.
// Example: typing "df" enters CmdOperatorFind{Op: OpDelete, Count: 1, FindDir: FindForward}.
type CmdOperatorFind struct {
    Op     Operator
    Count  int
    FindDir FindDirection
}

// CmdOperatorTextObj waits for a text-object selector after operator+"i"/"a".
// Example: typing "di" enters CmdOperatorTextObj{Op: OpDelete, Count: 1, Scope: TextObjInner}.
type CmdOperatorTextObj struct {
    Op    Operator
    Count int
    Scope TextObjScope
}

// CmdFind waits for a target character after a standalone find key.
// Example: typing "f" enters CmdFind{FindDir: FindForward, Count: 1}.
type CmdFind struct {
    FindDir FindDirection
    Count   int
}

// CmdGPrefix waits for the second key of a g-compound command.
// Example: typing "g" enters CmdGPrefix{Count: 1}.
type CmdGPrefix struct{ Count int }

// CmdOperatorG waits for the second key of an operator+g compound.
// Example: typing "dg" enters CmdOperatorG{Op: OpDelete, Count: 1}.
type CmdOperatorG struct {
    Op    Operator
    Count int
}

// CmdReplace waits for the replacement character after "r".
// Example: typing "r" enters CmdReplace{Count: 1}.
type CmdReplace struct{ Count int }

// CmdIndent waits for the second indent key after ">"/"<".
// Example: typing ">" enters CmdIndent{Dir: IndentRight, Count: 1}.
type CmdIndent struct {
    Dir   IndentDir
    Count int
}

func (CmdIdle) isCommandState()           {}
func (CmdCount) isCommandState()          {}
func (CmdOperator) isCommandState()       {}
func (CmdOperatorCount) isCommandState()  {}
func (CmdOperatorFind) isCommandState()   {}
func (CmdOperatorTextObj) isCommandState(){}
func (CmdFind) isCommandState()           {}
func (CmdGPrefix) isCommandState()        {}
func (CmdOperatorG) isCommandState()      {}
func (CmdReplace) isCommandState()        {}
func (CmdIndent) isCommandState()         {}

// Operator identifies which mutation to apply to a range.
type Operator int
const (
    OpDelete Operator = iota
    OpChange
    OpYank
    OpIndentRight
    OpIndentLeft
)

// FindDirection is the direction for f/F/t/T motions.
type FindDirection int
const (
    FindForward  FindDirection = iota
    FindBackward
)

// TextObjScope distinguishes "inner" (i) from "around" (a) text objects.
type TextObjScope int
const (
    TextObjInner TextObjScope = iota
    TextObjAround
)

// IndentDir is the direction for >> and << operators.
type IndentDir int
const (
    IndentRight IndentDir = iota
    IndentLeft
)
```

### RecordedChange Union (Dot-Repeat)

```go
// RecordedChange is a sealed interface for repeatable vim mutations.
// The "." command replays the last RecordedChange.
type RecordedChange interface{ isRecordedChange() }

// RCInsert records a complete Insert-mode session.
type RCInsert struct{ Text string }

// RCOperatorMotion records an operator applied to a motion.
type RCOperatorMotion struct {
    Op     Operator
    Count  int
    Motion MotionKey
}

// RCOperatorTextObj records an operator applied to a text object.
type RCOperatorTextObj struct {
    Op    Operator
    Count int
    Scope TextObjScope
    Delim rune
}

// RCOperatorFind records an operator applied to a find target.
type RCOperatorFind struct {
    Op      Operator
    Count   int
    FindDir FindDirection
    Char    rune
}

// RCReplace records a single-character replacement.
type RCReplace struct {
    Count int
    Char  rune
}

// RCDeleteChar records an "x" (delete character).
type RCDeleteChar struct{ Count int }

// RCIndent records an indent or outdent.
type RCIndent struct {
    Dir   IndentDir
    Count int
}

// RCOpenLine records "o" or "O" (open line).
type RCOpenLine struct {
    Above bool
    Text  string
}

// RCJoinLines records "J" (join lines).
type RCJoinLines struct{ Count int }

func (RCInsert) isRecordedChange()          {}
func (RCOperatorMotion) isRecordedChange()  {}
func (RCOperatorTextObj) isRecordedChange() {}
func (RCOperatorFind) isRecordedChange()    {}
func (RCReplace) isRecordedChange()         {}
func (RCDeleteChar) isRecordedChange()      {}
func (RCIndent) isRecordedChange()          {}
func (RCOpenLine) isRecordedChange()        {}
func (RCJoinLines) isRecordedChange()       {}
```

### PersistentState

```go
// PersistentState survives across vim commands and carries memory for repeat operations.
type PersistentState struct {
    LastChange         RecordedChange
    LastFind           LastFindState
    Register           string
    RegisterIsLinewise bool
}

// LastFindState records the most recent f/F/t/T command for ; and , repeat.
type LastFindState struct {
    FindDir FindDirection
    Char    rune
    // Till is true if the original command was t/T (stop before char, not on char).
    Till    bool
}
```

### TranscriptItem Height Cache

```go
// TranscriptItem is one entry in the conversation transcript.
// Each item has a stable ID used as the height cache key.
type TranscriptItem struct {
    ID       string
    Kind     TranscriptItemKind
    Content  string
    // ... existing fields
}

// heightCache stores the rendered height of each transcript item.
// Key: itemID + ":" + strconv.Itoa(terminalColumns).
// Value: number of terminal rows the item occupies.
type heightCache struct {
    mu    sync.Mutex
    cache map[string]int
}

func (c *heightCache) Get(itemID string, cols int) (int, bool)
func (c *heightCache) Set(itemID string, cols int, height int)
func (c *heightCache) Invalidate(itemID string) // Called when item content changes
func (c *heightCache) InvalidateAll()           // Called on terminal resize
```

### VirtualTranscript

```go
// VirtualTranscript manages the visible subset of transcript items.
type VirtualTranscript struct {
    items        []TranscriptItem
    heights      *heightCache
    scrollOffset int  // current scroll position in lines from top
    atBottom     bool // true if auto-scroll to bottom is active
    viewportH    int  // current terminal height for the transcript area
    cols         int  // current terminal width
}

// View returns the rendered string for the current viewport window.
// Only renders items that fall within [scrollOffset, scrollOffset+viewportH].
// Maintains a ±2-item buffer above and below the visible window.
func (vt *VirtualTranscript) View() string

// Append adds a new item and, if atBottom, scrolls to show it.
func (vt *VirtualTranscript) Append(item TranscriptItem)

// UpdateLast updates the last item's content and invalidates its height cache entry.
func (vt *VirtualTranscript) UpdateLast(content string)

// ScrollUp scrolls up by n lines; sets atBottom=false.
func (vt *VirtualTranscript) ScrollUp(n int)

// ScrollDown scrolls down by n lines; sets atBottom=true if at end.
func (vt *VirtualTranscript) ScrollDown(n int)

// GotoBottom scrolls to the end and re-enables auto-scroll.
func (vt *VirtualTranscript) GotoBottom()

// GotoTop scrolls to the beginning.
func (vt *VirtualTranscript) GotoTop()

// Resize recalculates visible window after terminal resize; invalidates height cache.
func (vt *VirtualTranscript) Resize(cols, viewportH int)
```

### Keybinding Context Stack

```go
// BindingContext identifies a keybinding priority context.
type BindingContext string

const (
    ContextModal      BindingContext = "modal"
    ContextVimNormal  BindingContext = "vim_normal"
    ContextVimInsert  BindingContext = "vim_insert"
    ContextScroll     BindingContext = "scroll"
    ContextGlobal     BindingContext = "global"
)

// ContextStack is the ordered list of active contexts.
// Earlier entries have higher priority.
type ContextStack struct {
    stack []BindingContext
    mu    sync.Mutex
}

func (cs *ContextStack) Push(ctx BindingContext)
func (cs *ContextStack) Pop(ctx BindingContext)
func (cs *ContextStack) Active() []BindingContext // returns a snapshot copy

// Binding associates a key with an action name.
type Binding struct {
    Key     string
    Action  string
    Context BindingContext
}

// KeybindingResolver resolves a key event to an action given active contexts.
type KeybindingResolver struct {
    bindings []Binding
    chords   map[string][]Binding // prefix -> potential chord bindings
}

// Resolve returns the action for the given key in the given contexts.
// Returns empty string if no binding matches.
func (r *KeybindingResolver) Resolve(key string, contexts []BindingContext) string

// ResolveChord returns chord state for a potential multi-key sequence.
type ChordResult int
const (
    ChordNone    ChordResult = iota
    ChordStarted             // first key matches a chord prefix
    ChordMatch               // complete chord matched an action
    ChordAborted             // prefix matched nothing; first key falls through
)
func (r *KeybindingResolver) ResolveChord(keys []string, contexts []BindingContext) (ChordResult, string)
```

### ChordInterceptor

```go
// ChordInterceptor manages the multi-key chord state machine.
type ChordInterceptor struct {
    resolver  *KeybindingResolver
    pending   []string        // accumulated chord keys
    timer     *time.Timer
    timeout   time.Duration   // default 1 second
    onAction  func(string)    // called when chord matches
    onFallthrough func(string) // called when chord aborts, forwarding the first key
}

func NewChordInterceptor(r *KeybindingResolver, timeout time.Duration) *ChordInterceptor

// HandleKey processes one key event. Returns true if the key was consumed by the chord
// state machine (either as part of a chord, or as the chord-aborted fallthrough).
func (ci *ChordInterceptor) HandleKey(key string, contexts []BindingContext) bool
```

### Bracketed Paste Types

```go
// ParsedKeyEvent wraps a Bubble Tea KeyMsg with additional metadata.
type ParsedKeyEvent struct {
    Key      tea.KeyMsg
    IsPasted bool
}

// BracketedPasteState tracks whether we are inside a bracketed paste sequence.
type BracketedPasteState int
const (
    PasteIdle    BracketedPasteState = iota
    PasteActive                       // between \x1b[200~ and \x1b[201~
)
```

## Implementation Plan

### Step 1 - Extend CommandState and Transition Function

File: `internal/tui/vim.go`

Replace the existing partial `CommandState` union with the full eleven-non-idle-variant union defined above. Update the `transition` function to dispatch to one handler function per state variant.

Rules:

- The `transition` function signature: `func transition(state CommandState, key string, ps *PersistentState, edit EditContext) (next CommandState, effect func())`.
- `effect` is nil if no immediate action is needed (state transition only).
- `effect` is a closure over `edit EditContext` that captures all needed editor state at transition time.
- All state transitions are pure: the `transition` function reads `state`, `key`, and `edit`, and returns `next` and `effect`. It does not mutate any field.
- Add the `fromIdle`, `fromCount`, `fromOperator`, `fromOperatorCount`, `fromOperatorFind`, `fromOperatorTextObj`, `fromFind`, `fromGPrefix`, `fromOperatorG`, `fromReplace`, `fromIndent` handler functions.

`EditContext` is an interface the vim state machine calls for editor operations:

```go
type EditContext interface {
    CursorPos() int
    BufferLen() int
    TextAt(start, end int) string
    Delete(start, end int)
    Insert(pos int, text string)
    SetCursor(pos int)
    Lines() []string
    CurrentLineRange() (start, end int)
}
```

This interface decouples the vim state machine from the Bubble Tea textarea implementation and makes the state machine testable without a live TUI.

### Step 2 - Text Objects

File: `internal/tui/vim.go` (or `internal/tui/textobjects.go`)

Implement `resolveTextObject(ec EditContext, scope TextObjScope, delim rune) (start, end int)`:

- Word objects (`w`/`W`): classify characters as word, whitespace, or punctuation; expand to word boundary. `a` variant includes trailing whitespace.
- Quote objects (`"`, `'`, `` ` ``): scan current line for paired quotes; find the pair containing the cursor. `a` variant includes the quote characters.
- Bracket objects (`(`, `[`, `{`): depth-tracking outward search from cursor. `a` variant includes the delimiters.
- Return `(-1, -1)` if the text object cannot be found at the current cursor position.

Tests:

- Word object: cursor in middle of word, at start, at end.
- Word object: `aw` includes trailing whitespace.
- Quote object: cursor inside first pair, inside second pair.
- Quote object: cursor outside any pair returns (-1,-1).
- Bracket object: nested brackets; depth tracking selects innermost.
- Bracket object: cursor before open bracket returns (-1,-1).

### Step 3 - Dot-Repeat

File: `internal/tui/vim.go`

The dot-repeat mechanism:

- Every mutating command records itself as a `RecordedChange` into `PersistentState.LastChange`.
- The `.` handler calls `replayChange(ps.LastChange, ps, ec)`.
- `replayChange` is a pure function dispatching on the `RecordedChange` variant.
- For `RCInsert`: re-insert `Text` at the current cursor position.
- For `RCOperatorMotion`: re-apply the operator to the motion from the current cursor position.
- For `RCOperatorTextObj`: re-apply the operator to the text object at the current cursor position.
- If `LastChange` is nil (no change yet), `.` is a no-op.

### Step 4 - Find-Repeat

File: `internal/tui/vim.go`

Implement `findChar(ec EditContext, dir FindDirection, char rune, till bool, count int) (newPos int, found bool)`:

- Scans the current line in `dir` for `char`.
- If `till` is true, stops one position before `char` (for `t`/`T`).
- Applies `count` repetitions.
- Returns the new cursor position and whether the character was found.

The `;` handler: call `findChar` with `ps.LastFind.FindDir`, `ps.LastFind.Char`, `ps.LastFind.Till`.
The `,` handler: call `findChar` with reversed direction, same `Char` and `Till`.

### Step 5 - Bracketed Paste

File: `internal/tui/input.go`

Current state: Bubble Tea handles raw key events via its own input processing. Phase 22 adds a pre-processing layer on the key event path.

Bubble Tea passes raw key events as `tea.KeyMsg`. The bracketed paste delimiters `\x1b[200~` and `\x1b[201~` appear as `tea.KeyMsg` values. Phase 22 adds:

- A `bracketedPasteBuffer` in the TUI model that activates between the open and close markers.
- When `KeyMsg.String() == "\x1b[200~"`: set `pasteActive = true`, clear `pasteBuffer`.
- When `KeyMsg.String() == "\x1b[201~"`: set `pasteActive = false`, process `pasteBuffer` as a single `ParsedKeyEvent{IsPasted: true}`.
- When `pasteActive == true` and any other key arrives: append to `pasteBuffer`.

In the vim handler, when `ParsedKeyEvent.IsPasted`:

- If in Normal mode: transition to Insert mode, insert pasted text, stay in Insert mode.
- If in Insert mode: insert pasted text at cursor.
- Do not run through the vim command state machine for pasted keys.

Tests:

- Paste sequence `\x1b[200~hello\x1b[201~` produces a single insert of "hello".
- Paste sequence containing `\x1b[1m` (bold escape) does not trigger any vim command.
- Paste in Normal mode transitions to Insert mode.
- Paste in Insert mode inserts at cursor position.
- Partial paste (open marker without close) does not hang or corrupt state.

### Step 6 - VirtualTranscript

File: `internal/tui/transcript.go`

Replace the current all-items-rendered transcript with the `VirtualTranscript` implementation defined in the Architecture section above.

Height calculation per item:

- For text items: count newlines in rendered text + 1.
- For tool-use items in collapsed state: always 1 (collapsed panel is one line).
- For tool-use items in expanded state: count lines in tool input + output summary + 2 (header and footer).
- Cache key: `itemID + ":" + strconv.Itoa(cols)`.

Sticky scroll:

- `atBottom` defaults to `true`.
- `Append` calls `GotoBottom()` if `atBottom == true`.
- `ScrollUp(n)` sets `atBottom = false`.
- `GotoBottom()` sets `atBottom = true` and sets `scrollOffset` to max.

Virtual window:

- `View()` computes cumulative heights to find the first item index at `scrollOffset`.
- Renders items from `firstVisible - 2` (clamped to 0) to `lastVisible + 2` (clamped to len-1).
- Items outside the window are not rendered (not even as empty strings).
- Items outside the window remain stored in `VirtualTranscript.items`; scrolling, top/bottom jumps, transcript search, collapsed/expanded tool panels, and older thinking/message inspection must continue to work across the complete conversation.

Tests:

- Height cache: same item+cols returns cached value without recomputing.
- Height cache: invalid item ID returns cache miss.
- Virtual window: 1000 items, viewport 25 rows, `View()` calls render on at most 29 items.
- Sticky scroll: Append while `atBottom=true` keeps scroll at bottom.
- Sticky scroll: `ScrollUp` stops auto-scroll.
- `GotoBottom` re-enables auto-scroll.
- Resize: `Resize` invalidates all cache entries and recalculates window.
- Race detector: concurrent `Append` and `View` do not race.

Benchmark:

- `BenchmarkView1000Items` with 1000 items, viewport 25 rows. Target: under 2ms per `View()` call (well within the 16ms frame budget).

### Step 7 - Keybinding Contexts

File: `internal/tui/keybindings.go`

Implement the five-context stack and the `KeybindingResolver`:

Default bindings:

```go
var DefaultBindings = []Binding{
    // ContextGlobal
    {Key: "ctrl+c",  Action: "app:interrupt", Context: ContextGlobal},
    {Key: "ctrl+d",  Action: "app:exit",      Context: ContextGlobal},
    {Key: "ctrl+l",  Action: "app:redraw",    Context: ContextGlobal},
    // ContextModal
    {Key: "y",       Action: "modal:approve",        Context: ContextModal},
    {Key: "n",       Action: "modal:deny",           Context: ContextModal},
    {Key: "a",       Action: "modal:approve_always", Context: ContextModal},
    {Key: "enter",   Action: "modal:approve",        Context: ContextModal},
    {Key: "escape",  Action: "modal:deny",           Context: ContextModal},
    // ContextVimNormal
    {Key: "i",       Action: "vim:insert_before", Context: ContextVimNormal},
    {Key: "a",       Action: "vim:insert_after",  Context: ContextVimNormal},
    {Key: "escape",  Action: "vim:already_normal",Context: ContextVimNormal},
    {Key: "/",       Action: "transcript:search", Context: ContextVimNormal},
    // ContextVimInsert
    {Key: "escape",  Action: "vim:normal_mode",   Context: ContextVimInsert},
    // ContextScroll
    {Key: "pgup",    Action: "scroll:page_up",   Context: ContextScroll},
    {Key: "pgdown",  Action: "scroll:page_down", Context: ContextScroll},
    {Key: "home",    Action: "scroll:top",       Context: ContextScroll},
    {Key: "end",     Action: "scroll:bottom",    Context: ContextScroll},
}
```

Context is active when:

- `ContextModal`: `state.App.PermissionPrompt != nil`.
- `ContextVimNormal`: `vim.Mode == Normal`.
- `ContextVimInsert`: `vim.Mode == Insert`.
- `ContextScroll`: transcript is scrollable (items exceed viewport height) and `ContextModal` is not active.
- `ContextGlobal`: always.

Resolution: build active context list each keypress, walk from highest-priority to lowest, return first matching binding action.

Tests:

- Modal context blocks global `ctrl+c` when both are active (modal wins).
- Vim normal context: `i` triggers `vim:insert_before`.
- Vim insert context: `escape` triggers `vim:normal_mode`.
- Both vim contexts inactive: key falls to global.
- Unknown key in all contexts: return empty string (unbound).

### Step 8 - Chord Interceptor

File: `internal/tui/keybindings.go`

Implement `ChordInterceptor` as described in the Architecture section.

Default chord bindings:

```go
var DefaultChords = []Binding{
    {Key: "g g", Action: "transcript:top",    Context: ContextVimNormal},
    {Key: "g G", Action: "transcript:bottom", Context: ContextVimNormal},
}
```

Integration with `Update`:

- Before passing a key to the vim state machine, pass it to `ChordInterceptor.HandleKey`.
- If `ChordInterceptor` returns true (consumed), the chord is in progress or fired an action. Do not pass the key to vim.
- If `ChordInterceptor` returns false (not consumed), pass the key to vim normally.

The `g` key in Normal mode is special: it starts both vim's `CmdGPrefix` state and potentially a chord. The chord interceptor takes priority when chord bindings exist for `g`; the vim `CmdGPrefix` state handles cases not covered by chords (e.g., `gj`, `gk`).

Tests:

- `g` then `g` within timeout: `transcript:top` action fires.
- `g` then `G` within timeout: `transcript:bottom` action fires.
- `g` then timeout expires: `g` falls through to vim as a key event.
- `g` then `j` (no chord): chord aborts, `g` falls through, `j` processed normally.
- Race detector: concurrent `HandleKey` calls do not race.

### Step 9 - Transcript Search

File: `internal/tui/transcript.go`

`/` in Normal mode:

- Transitions to `searchMode = true`.
- Renders a search input line at the bottom of the viewport.
- As the user types, highlights matching items with a simple case-insensitive substring match.
- `n` moves to the next match. `N` moves to the previous match.
- `Escape` clears the search and returns to Normal mode.

Implementation:

- `searchQuery string` and `searchActive bool` on `VirtualTranscript`.
- `searchMatches []int` (indices into `items` that contain the query).
- `searchCursor int` (current match index in `searchMatches`).
- On each search query character: recompute `searchMatches` and scroll to first match.
- Matching uses `strings.Contains(strings.ToLower(item.Content), strings.ToLower(query))`.
- Highlighted items use a distinct style (e.g., yellow background on matching text).

Tests:

- Empty query: no matches, no highlights.
- Query with matches: `searchMatches` populated.
- `n` advances `searchCursor`, scrolls to match.
- `N` reverses `searchCursor`.
- `Escape` clears query and highlights.

### Step 10 - Mouse Improvements

File: `internal/tui/app.go`

Click to expand/collapse tool-use panels:

- Each `TranscriptItem` of `KindToolUse` has `collapsed bool`.
- On mouse click at the item's row: toggle `collapsed`.
- `VirtualTranscript.View()` renders collapsed tool items as a single summary line.
- Height cache entry for the item is invalidated on toggle.

Lost-release recovery:

- Track `mouseDragging bool` in the model.
- On `tea.MouseMsg` with button press: set `mouseDragging = true`.
- On `tea.MouseMsg` with button release: set `mouseDragging = false`.
- On `tea.MouseMsg` with no button (motion): if `mouseDragging == true` and no buttons reported as pressed, infer release and set `mouseDragging = false`.

Tests:

- Click on collapsed tool item toggles to expanded.
- Click on expanded tool item toggles to collapsed.
- Lost-release: drag starts, motion with no button clears drag state.

### Step 11 - Wire App.go

File: `internal/tui/app.go`

- Replace inline transcript rendering with `VirtualTranscript`.
- Wire `ContextStack` and `KeybindingResolver` into the `Update` method.
- Wire `ChordInterceptor` before vim key processing.
- Wire bracketed paste pre-processing before all key handling.
- Push `ContextModal` when `state.App.PermissionPrompt != nil`; pop when nil.
- Push `ContextVimNormal`/`ContextVimInsert` based on vim mode.
- Handle `transcript:top`, `transcript:bottom`, `scroll:page_up`, `scroll:page_down`, `scroll:top`, `scroll:bottom` action strings from the resolver.

### Step 12 - Tests and Verification

Required commands:

```sh
go test -race ./internal/tui/...
go test -bench=BenchmarkView1000Items ./internal/tui/
go test ./...
tools/check-allowed-deps.sh
tools/check-network-policy.sh
```

Manual exit-gate verification:

```sh
go run ./cmd/nandocodego --model qwen3 --no-alt-screen
```

Then perform the five manual checks described in the Definition of Success.

## Implementation Todos

- [ ] Add `CmdCount`, `CmdOperatorCount`, `CmdOperatorFind`, `CmdOperatorTextObj`, `CmdFind`, `CmdGPrefix`, `CmdOperatorG`, `CmdReplace`, `CmdIndent` types to `internal/tui/vim.go`.
- [ ] Implement `func (CmdXxx) isCommandState()` for all eleven non-idle variants.
- [ ] Add `Operator`, `FindDirection`, `TextObjScope`, `IndentDir` types.
- [ ] Implement `EditContext` interface.
- [ ] Refactor `transition()` to dispatch to per-state handlers.
- [ ] Implement `fromIdle()` handler covering full vim vocabulary.
- [ ] Implement `fromCount()` handler.
- [ ] Implement `fromOperator()` handler.
- [ ] Implement `fromOperatorCount()` handler.
- [ ] Implement `fromOperatorFind()` handler.
- [ ] Implement `fromOperatorTextObj()` handler.
- [ ] Implement `fromFind()` handler.
- [ ] Implement `fromGPrefix()` handler.
- [ ] Implement `fromOperatorG()` handler.
- [ ] Implement `fromReplace()` handler.
- [ ] Implement `fromIndent()` handler.
- [ ] Implement `RecordedChange` sealed interface and all nine variant types.
- [ ] Implement `PersistentState` struct with `LastChange`, `LastFind`, `Register`, `RegisterIsLinewise`.
- [ ] Implement `resolveTextObject()` for word, quote, and bracket objects.
- [ ] Write text-object tests: word inner/around, quote inner/around, bracket depth-tracking.
- [ ] Implement `replayChange()` for dot-repeat.
- [ ] Write dot-repeat tests: insert, operator+motion, operator+text-obj, replace, delete-char.
- [ ] Implement `findChar()` for f/F/t/T motions.
- [ ] Implement `;` handler using `ps.LastFind`.
- [ ] Implement `,` handler with reversed direction.
- [ ] Write find-repeat tests: forward, backward, till variant.
- [ ] Implement register save/load in Delete, Change, Yank handlers.
- [ ] Implement `p`/`P` paste with linewise/characterwise awareness.
- [ ] Write register tests: characterwise paste after cursor, linewise paste below line.
- [ ] Write full `CommandState` transition table in `vim_test.go`.
- [ ] Write exhaustive `fromIdle` transition tests covering all keys.
- [ ] Write count-prefix tests: `3`, `3d`, `3dw` chain.
- [ ] Add `bracketedPasteBuffer` and `pasteActive` fields to TUI model.
- [ ] Implement bracketed paste pre-processing in `Update` for `\x1b[200~` and `\x1b[201~`.
- [ ] Implement paste routing: paste in Normal mode → Insert mode + insert + stay Insert.
- [ ] Implement paste routing: paste in Insert mode → insert at cursor.
- [ ] Write bracketed paste tests: plain text, escape sequences, paste in Normal mode.
- [ ] Write partial paste test (no close marker).
- [ ] Implement `heightCache` struct with `Get`, `Set`, `Invalidate`, `InvalidateAll`.
- [ ] Implement `VirtualTranscript` struct with all fields.
- [ ] Implement `View()` with virtual window (visible ± 2 items).
- [ ] Implement `Append()` with sticky scroll check.
- [ ] Implement `UpdateLast()` with cache invalidation.
- [ ] Implement `ScrollUp()` and `ScrollDown()`.
- [ ] Implement `GotoBottom()` and `GotoTop()`.
- [ ] Implement `Resize()` with full cache invalidation.
- [ ] Write height cache tests: cache hit, miss, invalidation.
- [ ] Write virtual window tests: 1000 items, only 29 rendered.
- [ ] Write sticky scroll tests: Append while atBottom, ScrollUp stops auto-scroll.
- [ ] Write resize test: cache cleared, window recalculated.
- [ ] Write `BenchmarkView1000Items` in `internal/tui/transcript_benchmark_test.go`.
- [ ] Confirm benchmark is under 2ms per `View()` call.
- [ ] Implement click-to-expand: `collapsed bool` on tool-use `TranscriptItem`.
- [ ] Wire mouse click handler to toggle `collapsed` on matching item.
- [ ] Implement lost-release recovery: `mouseDragging` flag, motion-with-no-button detection.
- [ ] Write click-to-expand tests.
- [ ] Write lost-release recovery tests.
- [ ] Add `BindingContext` type and five constants to `internal/tui/keybindings.go`.
- [ ] Implement `ContextStack` with `Push`, `Pop`, `Active`.
- [ ] Implement `Binding` type and `DefaultBindings` slice.
- [ ] Implement `KeybindingResolver` with `Resolve`.
- [ ] Implement chord tracking in `KeybindingResolver`: `chords` map.
- [ ] Implement `ResolveChord` returning `ChordResult`.
- [ ] Implement `DefaultChords` with `g g` and `g G`.
- [ ] Implement `ChordInterceptor` with `pending`, `timer`, `timeout`.
- [ ] Implement `ChordInterceptor.HandleKey`.
- [ ] Write context stack tests: push, pop, active snapshot.
- [ ] Write resolver tests: modal wins over global, vim normal wins over scroll.
- [ ] Write chord tests: `g g` fires action, `g` + timeout falls through, `g j` aborts chord.
- [ ] Implement transcript search: `searchQuery`, `searchActive`, `searchMatches`, `searchCursor`.
- [ ] Implement search input rendering at bottom of viewport.
- [ ] Implement `n`/`N` navigation through `searchMatches`.
- [ ] Implement `Escape` to clear search.
- [ ] Write transcript search tests: empty query, query with matches, `n`/`N`, Escape.
- [ ] Wire `VirtualTranscript` into `internal/tui/app.go`.
- [ ] Wire `ContextStack` and `KeybindingResolver` into `Update`.
- [ ] Wire `ChordInterceptor` before vim key dispatch in `Update`.
- [ ] Wire bracketed paste pre-processing before all key handling.
- [ ] Push/pop `ContextModal` on permission prompt state change.
- [ ] Push/pop `ContextVimNormal`/`ContextVimInsert` on vim mode change.
- [ ] Handle all action strings from resolver in `Update`.
- [ ] Run `go test -race ./internal/tui/...` and fix all races.
- [ ] Run `go test ./...` and confirm no regressions.
- [ ] Run `tools/check-allowed-deps.sh`.
- [ ] Run `tools/check-network-policy.sh`.
- [ ] Run `go test -bench=BenchmarkView1000Items ./internal/tui/` and confirm under 2ms.
- [ ] Perform manual exit-gate check 1: `3dw` + `.` (dot-repeat).
- [ ] Perform manual exit-gate check 2: `fa` + `;` + `,` (find-repeat).
- [ ] Perform manual exit-gate check 3: bracketed paste with escape sequences.
- [ ] Perform manual exit-gate check 4: 100+ message session, smooth scroll.
- [ ] Perform manual exit-gate check 5: `g g` chord goes to transcript top.
- [ ] Update `docs/PHASE-LOG.md` with Phase 22 entry.
- [ ] Update `docs/PROJECT-STATUS-AND-ONBOARDING.md` to reflect Phase 22 complete.
- [ ] Confirm `go vet ./internal/tui/...` is clean.
- [ ] Confirm `go test -count=3 -race ./internal/tui/...` is stable (no intermittent failures).
- [ ] Add `--debug-fps` flag to REPL command that logs per-frame view duration.
- [ ] Confirm that `ci"` inside a double-quoted string deletes the content and enters Insert mode.
- [ ] Confirm that `da(` deletes surrounding parentheses and their content.
- [ ] Confirm `>>` indents the current line by 2 spaces.
- [ ] Confirm `r<char>` replaces the character under the cursor.
- [ ] Confirm `yy` yanks a line and `p` pastes it below (linewise).
- [ ] Confirm `2dd` deletes two lines.
- [ ] Confirm `PageUp`/`PageDown` scroll the transcript in scroll context.
- [ ] Confirm `End` key re-enables sticky scroll.

## Acceptance Criteria

- [ ] `3dw` in Normal mode deletes 3 words forward.
- [ ] `.` (dot-repeat) replays the last mutating command at the current cursor position.
- [ ] `ci"` deletes the content inside double quotes and enters Insert mode.
- [ ] `da(` deletes a parenthesized expression including the parentheses.
- [ ] `fa` finds the next 'a'; `;` repeats forward; `,` repeats backward.
- [ ] `gg` (typed fast) jumps to the top of the input buffer.
- [ ] `G` jumps to the end of the input buffer.
- [ ] `>>` indents the current line; `<<` outdents.
- [ ] `r<char>` replaces the character under the cursor.
- [ ] Bracketed paste: pasted escape sequences do not trigger vim commands.
- [ ] Paste in Normal mode transitions to Insert mode, inserts text, returns to Normal.
- [ ] `VirtualTranscript.View()` with 1000 items renders in under 2ms (benchmark passes).
- [ ] Transcript with 1000 messages renders only the visible window ± 2 items.
- [ ] Sticky scroll: new agent streaming output auto-scrolls to bottom when `atBottom = true`.
- [ ] Sticky scroll: manual `ScrollUp` stops auto-scroll.
- [ ] `End` key (or `g G` chord) re-enables sticky scroll.
- [ ] `PageUp`/`PageDown` scroll the transcript by viewport height.
- [ ] `ContextModal` keybindings block all other contexts when permission dialog is open.
- [ ] `y`/`n` in the permission dialog resolve the permission request.
- [ ] `g g` chord (within 1 second) scrolls the transcript to the top.
- [ ] `g G` chord (within 1 second) scrolls the transcript to the bottom.
- [ ] `g` followed by timeout (no second key) falls through to vim as a normal key.
- [ ] `/` in Normal mode opens transcript search input.
- [ ] `n`/`N` in search mode navigate between matches.
- [ ] `Escape` in search mode clears the search.
- [ ] Click on a collapsed tool-use panel expands it.
- [ ] Click on an expanded tool-use panel collapses it.
- [ ] Lost-release recovery: drag state clears on mouse motion with no buttons pressed.
- [ ] `go test -race ./internal/tui/...` passes with zero race conditions.
- [ ] `go test ./...` passes with no regressions in any existing package.
- [ ] `tools/check-allowed-deps.sh` passes (no new dependencies added).
- [ ] `docs/PHASE-LOG.md` has a Phase 22 entry with files, decisions, and exit gate status.

## Forbidden

- Adding a seventh abstraction (no `InputManager`, `ViewManager`, `RenderController`).
- Replacing Bubble Tea with a custom rendering engine. Bubble Tea is the framework.
- Implementing a custom terminal protocol parser. Bubble Tea handles terminal input; Phase 22 adds a pre-processing layer only.
- Storing height cache or scroll state in `state.App`. These are TUI-only and must not leak into the state layer.
- Running any I/O (file reads, LLM calls, network calls) inside the vim state machine or the keybinding resolver.
- Implementing Visual block mode, macros, or ex-mode (deferred to future phases).
- Custom user keybinding configuration files (Phase 13 scope).
- Rendering off-screen transcript items in `VirtualTranscript.View()`.
- Any new direct dependency that is not already in `tools/allowed-deps.txt`.
- Implementing regex transcript search in Phase 22 (substring only).

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Chord `g` key conflicts with vim `CmdGPrefix` state | High | ChordInterceptor takes priority; vim handles `g j`/`g k` as fallthrough after chord aborts. |
| Virtual scroll height estimation errors | Medium | Height cache invalidation on content change; `Resize` clears all entries; fallback to rendered height. |
| Bracketed paste markers not sent by older terminals | Medium | Fall back gracefully: without the markers, all input is treated as non-pasted. Degraded but correct. |
| `EditContext` interface mismatch with Bubble Tea textarea | High | Define `EditContext` as a narrow interface; implement a `TextareaEditContext` adapter that wraps `textarea.Model`. Test the adapter separately from vim logic. |
| Chord timeout of 1 second feels too long | Low | Make timeout configurable via `--chord-timeout` flag; default 1 second matching Chapter 14. |
| Context stack grows unbounded if push/pop are unbalanced | Medium | Assert in tests that push and pop are balanced for each context transition. Log error and skip pop if context not found. |
| Sticky scroll and virtual scroll interact incorrectly during rapid streaming | High | Test case: 50 `Append` calls in rapid succession while `atBottom=true`. Confirm scroll stays at bottom. |
| Transcript search highlighting slows `View()` | Low | Substring search on plain text is O(n). With 1000 items and a 25-row viewport, search only runs on the ± 2 buffer window during `View()`. Full scan for match indices runs only when query changes. |
| Dot-repeat with text objects at line boundaries | Medium | `resolveTextObject` returns (-1,-1) on failure; `replayChange` no-ops on out-of-bounds. |

## Phase Log Template

When implementation finishes, append a Phase 22 entry to `docs/PHASE-LOG.md` with:

- objective;
- files created and updated;
- dependencies added (expected: none);
- tests, benchmarks, and checks run;
- manual five-point exit-gate result;
- design decisions (twelve-variant union, EditContext adapter, chord interceptor, virtual scroll algorithm);
- known constraints and deferred work (Visual block mode, macros, regex search, custom keybindings file);
- exit gate status.

## Exit Gate

Current evidence as of 2026-05-18:

- `go test ./...` passed.
- `go test -race ./internal/tui/...` passed.
- `go test -bench=BenchmarkView1000Items ./internal/tui` passed at roughly 0.26ms/op on the local run.
- `docs/PHASE-LOG.md`, `docs/PROJECT-STATUS-AND-ONBOARDING.md`, and `USER_MANUAL.md` have been updated to reflect implemented behavior and remaining limitations.

Phase 22 is complete only when:

- all acceptance criteria above are checked off;
- `go test -race ./internal/tui/...` and `go test ./...` pass;
- `BenchmarkView1000Items` completes in under 2ms per call;
- `tools/check-allowed-deps.sh` and `tools/check-network-policy.sh` pass;
- all manual exit-gate checks above pass with a live Ollama model in the REPL;
- `docs/PHASE-LOG.md` and `docs/PROJECT-STATUS-AND-ONBOARDING.md` are updated.

Open completion blockers:

- Manual REPL checks are not recorded yet.
- Dependency/network policy scripts were not rerun in the 2026-05-18 review pass.
- Deep-interaction follow-ups listed in the 2026-05-18 Implementation Review either need implementation or explicit phase deferral before Phase 22 can be marked fully complete.
