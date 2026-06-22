# ADR-001: Terminal User Interface Experience Improvements

Date: 2026-05-09

Status: Accepted for v0.1 roadmap

## Roadmap Placement

This ADR is required input for Phase 22. The local TUI must expose clear run-state visibility, progress, retry/compaction state, tool activity, queue state, and permission context before Phase 21 server work and before Phase 25 remote/bridge mode. Otherwise the same unclear ask/response experience would be replicated into browser and remote workflows.

Use `docs/TASKS-TUI.md` as the agent-readable implementation breakdown for this ADR. Phase 22 owns the required implementation slices.

## Context

Nandocodego already has a Bubble Tea terminal UI with transcript rendering, slash commands, permission modals, file/command pickers, active tool panels, and basic status information. The current user feedback during model execution is still too thin: when a run is active, the status bar only adds a static `[Running...]` marker. That leaves users guessing whether the app is waiting on Ollama, streaming tokens, executing tools, blocked on permissions, compacting context, retrying, or stalled.

The most urgent UI gap is a more interactive loading and progress experience while the LLM is running and the application is waiting for a response. This ADR also captures adjacent user-interface improvements that should be implemented as independent, agent-readable work packages.

Current implementation touchpoints:

| Area | Current files |
| :--- | :--- |
| TUI root model | `internal/tui/app.go` |
| TUI messages | `internal/tui/messages.go` |
| Styles | `internal/tui/styles.go` |
| Transcript items | `internal/tui/transcript.go` |
| Permission modal | `internal/tui/permission.go`, `internal/tui/app.go` |
| Agent event stream | `internal/agent/events.go` |
| App state | `internal/state/app.go` |
| Observability meter | `internal/observability/metrics.go` |

Current limitations:

- `renderStatusBar` shows only `Model`, Vim mode, optional `[Running...]`, running task count, and token count.
- There is a `tickMsg` type, but the TUI does not use it to animate run state.
- There is no explicit run phase model such as `waiting_for_model`, `streaming`, `running_tool`, or `permission_required`.
- The user cannot see elapsed time for a run or a tool at a glance.
- Tool panels show start/progress/result but do not make active vs completed states visually obvious enough.
- Queued prompts are supported internally, but the status bar does not surface queue length.
- Retry, compaction, hook notices, and permission prompts appear in transcript/system items, but they are not summarized in the persistent status area.

## Decision

Improve the terminal UX through a small internal UI-state model and a phased set of visual improvements. The first deliverable is an animated run indicator and status-line phase summary. Later deliverables add richer tool progress, queue visibility, keyboard discoverability, transcript controls, notification surfacing, and theme/accessibility support.

The UI should remain terminal-native and low-overhead. Avoid turning the TUI into a complex dashboard before the basic interaction loop feels responsive. The priority is to make the user understand what the program is doing right now.

## Goals

- Give immediate, animated feedback when the app is waiting on the LLM.
- Distinguish model wait, model streaming, tool execution, permission wait, retry, compaction, and idle states.
- Show elapsed time for active runs and active tools.
- Surface queue length, token count, running task count, and active tool count in one compact status area.
- Keep rendering deterministic and testable.
- Avoid excessive animation frequency that wastes CPU or causes terminal flicker.
- Keep all changes compatible with `--no-alt-screen`.

## Non-Goals

- Replacing Bubble Tea.
- Building a web UI.
- Changing the agent loop protocol unless a later slice explicitly needs richer events.
- Adding model-side progress estimates that the backend cannot provide reliably.
- Adding external dependencies before checking whether existing Bubble Tea/Bubbles packages already cover the need.
- Implementing the full feature set in one large PR.

## UX Principles

- Always answer the user's implicit question: "Is it working, and what is it waiting on?"
- Use compact persistent status for transient state; use transcript items for important durable events.
- Prefer phase labels over vague spinners. A spinner alone is not enough.
- Show elapsed time, not fake percentages.
- Keep keyboard controls visible when they matter.
- Do not hide permission prompts or errors behind animation.
- Degrade cleanly in narrow terminals and no-color environments.

## Proposed Run Phases

Introduce a TUI-owned run phase model. This can be stored only in `internal/tui.Model` initially; do not expand global `state.App` unless another package needs it.

```go
type RunPhase string

const (
    RunPhaseIdle          RunPhase = "idle"
    RunPhaseQueued        RunPhase = "queued"
    RunPhaseExpanding     RunPhase = "expanding_context"
    RunPhaseWaitingModel  RunPhase = "waiting_for_model"
    RunPhaseStreaming     RunPhase = "streaming_response"
    RunPhaseRunningTool   RunPhase = "running_tool"
    RunPhasePermission    RunPhase = "permission_required"
    RunPhaseRetrying      RunPhase = "retrying"
    RunPhaseCompacting    RunPhase = "compacting_context"
    RunPhaseDone          RunPhase = "done"
    RunPhaseError         RunPhase = "error"
)
```

Track a small UI run snapshot:

```go
type RunUIState struct {
    Phase          RunPhase
    StartedAt      time.Time
    PhaseStartedAt time.Time
    LastEventAt     time.Time
    SpinnerFrame   int
    ActiveToolID    string
    ActiveToolName  string
    LastNotice      string
}
```

Initial phase transitions:

| Trigger | New phase |
| :--- | :--- |
| User submits normal prompt | `waiting_for_model` after mention expansion succeeds. |
| `AssistantTextDelta` | `streaming_response`. |
| `AssistantThinkingDelta` | `streaming_response` or future `thinking` phase if exposed separately. |
| `ToolUseStart` | `running_tool`. |
| `ToolUseProgress` | `running_tool`. |
| `ToolUseResult` | `waiting_for_model` unless another tool remains active. |
| `permissionPromptMsg` | `permission_required`. |
| `permissionResolvedMsg` | `waiting_for_model`. |
| `RetryNotice` | `retrying`. |
| `CompactionStarted` | `compacting_context`. |
| `CompactionCompleted` | `waiting_for_model`. |
| `Terminal` or `agentDoneMsg` | `done`, then `idle`. |
| `agentStartFailedMsg` | `error`. |

## First Feature: Animated LLM Loading Graphic

### User Experience

When the agent is waiting for an Ollama/model response, the status bar should show an animated indicator and a specific phase:

```text
◐ Waiting for qwen3 · 00:07 · ctx compact ok · tokens: 12,482
```

While streaming:

```text
● Streaming response · 00:11 · 327 chars received · Ctrl-C abort
```

While a tool is running:

```text
◒ Running Bash · go test ./internal/config · 00:04 · Ctrl-C abort
```

When permission is required:

```text
■ Permission required · Bash · go test ./... · [a] allow [d] deny [A] always
```

When idle:

```text
Model: qwen3 · Mode: insert · /help · @ files · tokens: 12,482
```

Recommended spinner frames:

```text
⠋ ⠙ ⠹ ⠸ ⠼ ⠴ ⠦ ⠧ ⠇ ⠏
```

Fallback ASCII frames for terminals that render Braille poorly:

```text
- \ | /
```

Do not use percentages. Ollama streaming does not provide a reliable total.

### Implementation Notes

Use Bubble Tea ticks:

```go
func tickCmd() tea.Cmd {
    return tea.Tick(120*time.Millisecond, func(t time.Time) tea.Msg {
        return tickMsg(t)
    })
}
```

Behavior:

- Start ticking when `ActiveRun` becomes true.
- Continue ticking while `ActiveRun` or `PermissionPrompt != nil`.
- Stop returning new ticks when idle.
- On each tick, increment `SpinnerFrame`, update elapsed display, and re-render.
- Keep tick interval between 100ms and 200ms. Faster animation is unnecessary.

Minimal files for first slice:

| File | Change |
| :--- | :--- |
| `internal/tui/messages.go` | Reuse existing `tickMsg`; add no new message unless needed. |
| `internal/tui/app.go` | Add `RunUIState`, phase transitions, tick command scheduling, and richer `renderStatusBar`. |
| `internal/tui/styles.go` | Add styles for active, muted, warning, and accent status segments if needed. |
| `internal/tui/app_test.go` | Add tests for phase transitions and status rendering. |

Acceptance criteria:

- Submitting a prompt immediately changes the status from idle to an animated waiting state.
- The spinner advances while waiting for the first assistant delta.
- The phase changes to streaming when text arrives.
- The phase changes to running tool when `ToolUseStart` arrives.
- The phase changes to permission required when a permission prompt opens.
- Ctrl-C still aborts active runs.
- No tick loop continues after the run finishes.
- Existing TUI tests pass.

## Additional UI Features

### 1. Rich Active Tool Panel

Problem:

Tool panels currently show `[started]`, progress text, or `[OK]`, but active tools are not visually distinct enough.

Proposal:

- Show active tools in a compact live panel above the input or inside the transcript tail.
- Include icon/marker, tool name, short ID, elapsed time, and target summary.
- Collapse completed tools by default after success, preserving error output expanded.

Example:

```text
Tools
  ⠹ Bash      go test ./...                         00:12
  ✓ FileEdit  USER_MANUAL.md                        421 B changed
  ! WebFetch  https://example.com                   timeout
```

Agent work package:

- Add `ToolUIState` derived from `state.App.ActiveTools` and transcript items.
- Track `StartedAt`, `Done`, `Error`, and summary already available in `state.ToolUse`.
- Add render tests for active, success, and error states.

Acceptance criteria:

- Active tools remain visible even when transcript scrollback is long.
- Completed successful tools become compact.
- Failed tools remain prominent.
- Tool state works with concurrent tool execution.

### 2. Queued Prompt Visibility

Problem:

The app can queue prompts while a run is active, but the user has little visibility into queue length.

Proposal:

- Status bar displays `queue: N` when queued prompts exist.
- Add `/queue` command later to list, remove, or clear queued prompts.
- Initial implementation can be display-only.

Example:

```text
⠴ Streaming response · queue: 2 · Ctrl-C abort current
```

Agent work package:

- Read `len(appState.QueuedPrompts)` in status rendering.
- Add tests for status bar queue count.
- Optional later command registry extension: `/queue list`, `/queue clear`, `/queue drop <index>`.

Acceptance criteria:

- Queue count appears only when greater than zero.
- Queue count updates when prompts are added or consumed.

### 3. Better Permission Modal Context

Problem:

The permission modal shows tool, target, reason, and action keys. It does not show the active permission mode, matching rule/hook source, or risk category.

Proposal:

- Include permission mode.
- Include rule/hook reason when available.
- Show command/file target in a wrapped, copyable block.
- Add short key help for allow once, deny, and always allow.
- Consider adding `?` help inside the modal.

Example:

```text
Permission Required

Tool: Bash
Mode: default
Reason: command is not classified as read-only
Target:
  go test ./...

[a] allow once  [d] deny  [A] always allow this target  [esc] deny
```

Agent work package:

- Extend modal render only if the required fields are already in state.
- If mode/rule source is not available, do not change permission resolver yet; defer richer provenance.
- Add modal wrapping tests for long targets.

Acceptance criteria:

- Long commands do not overflow the modal.
- Permission action keys remain unchanged.
- Existing permission broker behavior remains unchanged.

### 4. Inline Run Summary Card

Problem:

At the end of a run, users see the final answer but not a concise summary of what happened.

Proposal:

Add a small system summary item after terminal completion:

```text
Run completed in 01:42 · 4 LLM calls · 7 tool calls · 18,240 tokens · completed
```

For failures:

```text
Run stopped after 00:39 · reason: max_turns · 5 tool calls · use /compact or continue with a narrower prompt
```

Agent work package:

- Use `agent.Terminal.Usage` and `Meter.Snapshot()` where appropriate.
- Track run start time in `RunUIState`.
- Append a `TranscriptSystem` item on terminal event.

Acceptance criteria:

- Successful, aborted, max-turn, context-overflow, stop-hook, and unrecoverable terminal reasons render distinct summary lines.
- Summary does not duplicate on `agentDoneMsg` after `Terminal`.

### 5. Transcript Controls

Problem:

Long sessions become hard to navigate.

Proposal:

- Add keyboard shortcuts for jump to latest, jump to previous tool, jump to previous error.
- Add collapse/expand all tool panels.
- Add slash commands: `/transcript save`, `/transcript clear-tools`, `/transcript errors` in a later command UX phase.

Initial minimal feature:

- `End` or `Ctrl-E` jumps to latest transcript.
- `Ctrl-R` toggles whether completed tools are collapsed.

Agent work package:

- Add local TUI key handling.
- Add a boolean `CollapseCompletedTools` in TUI model.
- Do not persist this setting initially.

Acceptance criteria:

- Existing typing behavior is not broken.
- Shortcuts do not conflict with picker navigation or permission modal keys.

### 6. First-Run and Empty-State Help

Problem:

New users may not know about slash commands, `@path` mentions, permissions, or model switching.

Proposal:

When transcript is empty, show a concise help card:

```text
Start with a task, or try:
  Explain @README.md
  /models
  /permissions show
  Use @internal/cli to inspect the CLI implementation

Keys: Enter submit · Ctrl-C abort · /help commands · @ attach files
```

Agent work package:

- Add `renderEmptyState()` when transcript has no items and no active run.
- Hide once the first prompt/system note exists.
- Include current model and working directory.

Acceptance criteria:

- Startup notes still render when present.
- Empty state does not appear after `/clear` if that behavior would be confusing; decide explicitly in implementation.

### 7. Theme and Accessibility Improvements

Problem:

The current style is functional but not configurable. Some colors may be poor in terminals with limited palettes or for color-blind users.

Proposal:

- Introduce semantic style roles: info, success, warning, error, muted, accent, active.
- Add a no-color-safe rendering path using text markers.
- Later config: `tui.theme = "default" | "high-contrast" | "mono"`.

Agent work package:

- Refactor `DefaultStyles()` to semantic fields without changing all render sites at once.
- Add tests or snapshots for no-color output if the test framework supports it.

Acceptance criteria:

- Existing colors remain roughly equivalent in default theme.
- High-contrast or mono mode can be added without invasive render rewrites.

### 8. Model and Context Health Indicator

Problem:

Users do not know whether slow responses are caused by model wait, large context, retries, or compaction.

Proposal:

Show compact health information:

```text
qwen3 · ctx 74% · compact soon · tokens: 74218 · 2 retries
```

Initial version should only show data already available:

- Total tokens from `Meter.Snapshot()`.
- Retry count or last retry notice from app state.
- Compaction phase from events.

Avoid fake context percentage unless accurate context-window data is available.

Agent work package:

- Add retry count to run UI state.
- Render last retry cause in status bar briefly, then move to `LastNotice`.
- Defer context percentage until model context accounting is reliable.

Acceptance criteria:

- Retry status appears immediately on `RetryNotice`.
- Status returns to waiting/streaming phase after retry state clears.

### 9. Command Palette Improvements

Problem:

Slash commands exist and a picker provider exists, but command discoverability can improve.

Proposal:

- When the user types `/`, show command descriptions and examples.
- Add fuzzy matching and stable ordering by command relevance.
- Show command usage errors inline with examples.

Agent work package:

- Extend command provider items with usage and description from the command registry.
- Avoid duplicating command docs in multiple places; command metadata should come from one registry structure.

Acceptance criteria:

- `/per` suggests `/permissions`.
- Selecting a command inserts the command name, not full docs.
- `/help` remains authoritative.

### 10. File Mention Picker Improvements

Problem:

`@path` is powerful, but users need confidence about what is being attached.

Proposal:

- Show file size and directory markers in picker details.
- Show whether a selected directory may be truncated by mention caps.
- After prompt submit, display a compact attachment summary.

Current code already has `directoryExpansionSummary`. Improve its wording and placement after the run-state work.

Acceptance criteria:

- File picker remains fast on large repos.
- Attachment summary shows files, directories, skipped/truncated status.

## Phased Implementation Plan

### Phase UI-1: Animated Run Status

Scope:

- Add `RunUIState` to `internal/tui`.
- Add tick scheduling.
- Add phase transitions from existing agent/TUI messages.
- Replace static `[Running...]` status with animated phase-aware status.

Files:

- `internal/tui/app.go`
- `internal/tui/messages.go`
- `internal/tui/styles.go`
- `internal/tui/app_test.go`

Tests:

- Unit test spinner frame changes on tick while active.
- Unit test no new tick is scheduled when idle.
- Unit test phase transition for assistant delta, tool start, permission prompt, terminal.
- Existing `go test ./internal/tui` passes.

Exit criteria:

- User sees animated, phase-specific feedback while waiting for LLM response.
- No regressions to prompt submission, Ctrl-C abort, permission modal, or picker behavior.

### Phase UI-2: Active Tool Strip and Run Summary

Scope:

- Render active tool strip from `ActiveTools`.
- Add elapsed time for active tools.
- Collapse successful completed tool transcript panels by default or add a clear visual distinction.
- Append run summary card on terminal completion.

Files:

- `internal/tui/app.go`
- `internal/tui/transcript.go`
- `internal/tui/styles.go`
- `internal/tui/app_test.go`

Tests:

- Concurrent tools display deterministically sorted by start time or ID.
- Failed tools render error styling.
- Run summary appears exactly once.

Exit criteria:

- Users can tell which tool is currently active without scrolling.
- Completed runs leave a concise operational summary.

### Phase UI-3: Queue, Retry, and Compaction Visibility

Scope:

- Show queued prompt count in status bar.
- Show retry state in status bar.
- Show compaction state and result summary more clearly.
- Add optional `/queue` command only if command registry metadata is ready; otherwise display-only.

Files:

- `internal/tui/app.go`
- `internal/commands/registry.go` if `/queue` is added
- `internal/tui/app_test.go`
- `internal/commands/registry_test.go` if `/queue` is added

Tests:

- Queue count appears and disappears correctly.
- Retry status transitions back after the next model/tool event.
- Compaction status renders during start and after completion.

Exit criteria:

- Long or interrupted runs no longer look stuck.

### Phase UI-4: Permission Modal and Help Polish

Scope:

- Improve modal layout, wrapping, and help text.
- Add empty-state help card.
- Improve command picker metadata if registry can expose usage strings.

Files:

- `internal/tui/app.go`
- `internal/tui/permission.go`
- `internal/tui/picker/command_provider.go`
- `internal/commands/registry.go`
- Tests in matching packages

Tests:

- Long permission targets wrap safely.
- Empty state renders only when intended.
- Picker remains usable with slash commands.

Exit criteria:

- New users can discover basic controls without reading docs first.
- Permission prompts are easier to evaluate.

### Phase UI-5: Theme and Accessibility Foundation

Scope:

- Refactor styles into semantic roles.
- Add high-contrast and mono theme structures.
- Wire config only if config UX has a stable TUI settings section; otherwise keep theme selection internal.

Files:

- `internal/tui/styles.go`
- `internal/config/config.go` only if settings are persisted
- `internal/config/defaults.go` only if settings are persisted
- `USER_MANUAL.md` after behavior lands

Tests:

- Default style construction remains valid.
- Config load tests only if persisted settings are introduced.

Exit criteria:

- UI can support multiple themes without rewriting render functions.

## Agent Work Package Template

Each agent implementing one UI slice should follow this contract:

1. Read this ADR and the relevant files listed for the phase.
2. Inspect current tests before editing.
3. Make the smallest cohesive implementation for one phase only.
4. Add or update tests that prove the UI state transition or rendering behavior.
5. Run the narrow package tests first, usually `go test ./internal/tui`.
6. If command/config changes are included, run their package tests too.
7. Do not modify unrelated docs or phase plans unless the user explicitly requests it.
8. Do not introduce a new dependency without documenting why the existing Bubble Tea/Bubbles stack is insufficient.

## Detailed Acceptance Checklist

Phase UI-1 checklist:

- `RunUIState` exists and is initialized to idle.
- `tickMsg` updates animation frame only while useful.
- Status bar shows `waiting_for_model` before first stream delta.
- Status bar shows `streaming_response` after assistant content arrives.
- Status bar shows active tool name during `ToolUseStart` and `ToolUseProgress`.
- Status bar shows `permission_required` while permission modal is open.
- Status bar shows retry and compaction states from existing events.
- Terminal completion clears active run animation.
- Ctrl-C behavior is unchanged.
- Permission modal behavior is unchanged.
- Picker behavior is unchanged.

Phase UI-2 checklist:

- Active tool strip renders active tools without requiring scrollback.
- Tool elapsed time updates via the same tick loop.
- Tool display truncation remains UTF-8 safe.
- Failed tools remain visibly distinct.
- Terminal summary appears once.

Phase UI-3 checklist:

- Queue length appears when queued prompts exist.
- Retry cause appears without flooding transcript.
- Compaction status appears in both transcript and status as appropriate.

Phase UI-4 checklist:

- Permission modal wraps long targets.
- Empty-state help appears on a new clean session.
- Command discoverability improves without duplicating command definitions.

Phase UI-5 checklist:

- Semantic style names exist.
- Default theme remains compatible.
- High-contrast/mono path is possible.

## Risks and Mitigations

| Risk | Mitigation |
| :--- | :--- |
| Animation causes high CPU usage. | Use 100-200ms ticks and stop ticking when idle. |
| Flicker in slow terminals. | Keep status updates small and avoid full transcript mutation on every tick. |
| Tests become brittle due to time. | Inject or isolate time formatting helpers; test deterministic status strings with fixed timestamps. |
| Status bar becomes too crowded. | Hide low-priority segments on narrow widths; prioritize phase, elapsed, permission/tool, and abort hint. |
| Unicode spinner renders poorly. | Provide ASCII fallback or config later. |
| More state creates desync with app state. | Derive from existing events and keep `RunUIState` TUI-local. |
| Large PRs become hard to review. | Keep phases separate and merge incrementally. |

## Open Questions

- Should spinner style be configurable immediately, or deferred until theme support?
- Should run phase be stored in `state.App` for external observability, or stay TUI-local?
- Should successful tool panels auto-collapse, or should the user explicitly toggle collapse behavior?
- Should `/queue` be added as part of UI work, or wait for broader command UX improvements?
- Should terminal width determine whether token count or queue count is hidden first?

## Recommended First Agent Task

Implement Phase UI-1 only.

Prompt for an implementation agent:

```text
Implement ADR-001 Phase UI-1: Animated Run Status. Read docs/ADR-001-TUI-USER-EXPERIENCE-IMPROVEMENTS.md and modify only the TUI files needed for the first phase. Add tests for tick scheduling, phase transitions, and status rendering. Keep permission modal, Ctrl-C abort, and picker behavior unchanged. Run go test ./internal/tui.
```

Expected implementation output:

- Changed files list.
- Test command and result.
- Notes on any intentionally deferred ADR items.

## Consequences

Positive:

- Users get immediate feedback that the app is alive while waiting for the LLM.
- Long-running tasks become easier to understand.
- Permission and tool execution states become more visible.
- Future UI improvements have a clear phased roadmap.

Negative:

- The TUI model gains more local state.
- Status rendering becomes more complex.
- Animation must be tested carefully to avoid runaway tick loops.

Neutral:

- The agent loop protocol can remain unchanged for the first phase.
- Future phases may require small event additions if current events are not rich enough.

---

## Review Addendum (additional UX issues found in second-pass review)

The phases above remain correct, but the review surfaced several existing UI
defects and missing acceptance criteria. These should be folded into the matching
phase rather than spun out as new phases.

### UI-1 additions

- **Stop-tick acceptance test**: assert no `tickMsg` is returned for at least
  300ms after `Terminal` arrives. Without this, tick loops can leak quietly.
- **Status priority order**: the status segment must pick a single phase using
  this priority — `permission_required` > `running_tool` > `thinking` >
  `streaming` > `waiting_for_model` > idle. Naive concatenation produces
  misleading output (e.g. `[Thinking...]` shown while a tool is actually
  blocking on stdin).
- **Flicker between runs**: when a queued prompt is drained immediately after
  `agentDoneMsg`, status oscillates idle → running. Add a `RunPhaseDrainingQueue`
  micro-phase, or skip the idle frame when queue length > 0.

### UI-2 additions (active tool panel)

- **Tool progress formatting**: today `app.go:649` renders `fmt.Sprintf("%v",
  e.Data)`. Structured progress payloads (maps/structs) appear as raw Go syntax.
  UI-2 must define a small formatter per known tool (Bash → command tail,
  WebFetch → URL, FileEdit → path + bytes changed) with a fallback.
- **Concurrent tool ordering**: must be deterministic. Sort by `StartedAt`
  ascending; break ties on `ToolID`. Add a test with two tools started on the
  same tick.

### UI-3 additions (queue, retry, compaction)

- **Cross-reference with THINKING-VISIBILITY-PLAN A4**: the queue-drain fix and
  the queue-display feature share state. Land A4 first; otherwise the displayed
  queue length will appear stuck at non-zero while prompts are silently dropped.
- **`/clear` must reset transient state**: today `internal/tui/app.go:499-506`
  clears `Messages` but leaves `LastRetryNotice`, `TerminalReason`,
  `TerminalDetail`, and `Usage`. The status bar and any future run-summary card
  will show stale data. Add explicit reset to the `/clear` handler.

### UI-4 additions (permission modal and help)

- **`overlayModal` is not a real overlay**: `app.go:1271-1273` uses
  `lipgloss.JoinVertical`, so the modal is appended below transcript content,
  not centered on top. The mockup in section 3 implies an overlay. Fix as part
  of UI-4 using a true overlay (e.g. `lipgloss.Place` over a captured snapshot
  of the background view).
- **Esc inside the permission modal**: today Ctrl-C only cancels via
  `activeRunCtx`. If the run was already cancelled but a permission prompt is
  still open, the user has no way to dismiss it. Add Esc → deny inside the modal.
- **"Always allow" rule scope**: `tui/app.go:773` builds the rule as
  `toolName + "(*)"`, matching any future target. This is a security-relevant
  UX bug — users approving `Bash` for `go test ./...` silently approve all
  future Bash commands. UI-4 should either:
  1. Replace `(*)` with the literal modal target (with user confirmation), or
  2. Show explicit text "this will allow ALL `Bash` calls forever" before
     accepting `A`.
- **Picker / modal race**: if the picker is open and a `permissionPromptMsg`
  arrives, `closePicker` must run before the modal renders. Add a deterministic
  test for this case.
- **Slash-command picker re-triggering**: typing `/permissions allow Bash(/path)`
  may re-trigger the command picker on the embedded `/`. The picker provider
  needs to gate on cursor position relative to a leading `/`, not on substring
  matching.

### UI-5 additions

- **Status bar width truncation**: `renderStatusBar` (`app.go:1217-1239`)
  produces unbounded-width output. Define hide-priority on narrow widths: drop
  token count first, then queue length, then elapsed time, never the phase or
  abort hint. Add tests at 60 / 80 / 120 columns.
- **Live token display depends on metrics fix**: `Meter.RecordLLMChat`
  (`internal/observability/metrics.go:89-91`) currently ignores
  `promptTokens`/`completionTokens` and only `RecordAgentRun` updates totals.
  ADR-001 section 8's live `tokens: N` requires fixing this first.

### New work package: UI-0 — Modal & permission rendering correctness

Before any of the visible UX work in UI-1..UI-5, fix the rendering bugs that
will otherwise be re-exposed by every later phase:

| File | Change |
| :--- | :--- |
| `internal/tui/app.go` | Replace `overlayModal` with a true overlay (lipgloss.Place + background snapshot). |
| `internal/tui/app.go` | Add Esc handler in permission modal. |
| `internal/tui/app.go:499` | `/clear` resets `LastRetryNotice`, `TerminalReason`, `TerminalDetail`, `Usage`. |
| `internal/tui/picker/command_provider.go` | Gate command picker on leading `/` at cursor anchor, not substring. |

This phase is small, has no animation, and unblocks the rest of the roadmap.

### Open questions to resolve before UI-1

- Should `RunPhase` live in `state.App` so that headless agent runs (sub-agents,
  memory extraction) can also report their phase, or stay TUI-local? Recommend
  TUI-local for now and revisit if a non-TUI consumer needs it.
- Should "Always allow" be removed entirely until rule scope is configurable
  per-modal? Removing is safer than the current behaviour.

---

## Phase UI-6: Claude-Code-Style Hierarchical Activity Display

Status: Proposed addition (post-UI-1, can land in parallel with UI-2)

### Motivation

Reference UI (Claude Code TUI, see screenshot in repo `docs/Screenshot 2026-05-16
at 16.38.42.png`) demonstrates an information-dense yet readable activity area
that solves several problems the current nandocodego TUI still has even after
UI-1 lands:

1. Long tool sequences scroll the screen and lose visual structure. There is no
   way to tell at a glance which tool calls belong to which agent turn, which
   are still running, and which have completed.
2. The user cannot collapse noisy tool output without losing the parent context.
3. There is no way to "fire and forget" a long-running tool (e.g. a long
   `go test`, a multi-minute web fetch) and continue the conversation.
4. The single `[Running...]` indicator does not communicate *which phase of
   thinking/working* the model is in, nor a sense of "how much effort is being
   spent."
5. Users cannot ask a quick side question without aborting and losing the in-flight
   context.
6. The status footer does not surface the git branch, queued sub-agents, or
   currently-applicable shortcuts.

### Target Visual

```text
  Read docs/ADR-001-TUI-USER-EXPERIENCE-IMPROVEMENTS.md (750 lines)
  Read docs/THINKING-VISIBILITY-PLAN.md (603 lines)

  Listed 1 directory (ctrl+o to expand)

  Agent(Deep code review for bugs and improvements)
  └ Read(internal/commands/registry.go)
    Bash(git diff --stat)
    Running…
    Bash(git diff internal/agent/agent.go internal/agent/input.go …)
    Running…
    … +19 tool uses (ctrl+o to expand)
    (ctrl+b to run in background)

* Bebopping…  (4m 26s · ↓ 4.8k tokens · almost done thinking with medium effort)
  └ Tip: Use /btw to ask a quick side question without interrupting Claude's current work

──────────────────────────────────────────────────────────────────────────────
>
──────────────────────────────────────────────────────────────────────────────
  esc to interrupt · ↓ to manage

  ● main
  ○ general-purpose  Deep code review for bugs and improvements          ↑/↓ select · Enter view
```

The display has five distinct regions, each with explicit responsibilities:

| Region | Purpose | Source of truth |
| :--- | :--- | :--- |
| Transcript | Hierarchical, collapsible record of completed tool calls and assistant messages | `m.transcript` (extended) |
| Active task | One-line animated status for the currently running model turn | `RunUIState` (from UI-1) |
| Inline tip | Context-sensitive hint anchored under the active task | `m.activeTip` (new) |
| Input box | The composer | existing |
| Footer strip | Multi-row footer with current-context shortcuts, git branch, and queued sub-agents | `m.footerState` (new) |

### Hierarchical Tool Tree

Replace the current flat tool-panel list with a tree where:

- Each *agent turn* introduces a logical group. The group root is rendered with
  a `┐` glyph and a header line (e.g. `Agent(reason summary)`); the root may
  itself be implicit and elided for the top-level turn so the screen does not
  feel over-decorated.
- Each tool call within the turn is rendered as a child with a leading `│ `
  spacer and `└` for the last child. The current connector for completed
  intermediate calls is a thin space (the screenshot shows no `│ ` runner; the
  child list is indented two spaces) — implementation should match the simpler
  indented form to keep terminal width usable.
- A tool call that contains nested sub-agent activity (e.g. `Agent(...)` tool)
  renders the sub-agent's own tool calls one further indentation level. This
  is the same recursion `internal/agent/subagent.go` already produces in JSONL.
- Collapsed tool batches show `… +N tool uses (ctrl+o to expand)` and persist
  this collapsed/expanded state per group across renders.

Data model addition in `internal/tui/transcript.go`:

```go
type TranscriptItem struct {
    // ... existing fields ...
    GroupID    string // shared by sibling tool calls of one assistant turn
    ParentID   string // for nested sub-agent tool calls
    Depth      int    // pre-computed indent depth for rendering
    BatchedInto string // when this item has been folded into a "+N tool uses" entry
}

type TranscriptGroup struct {
    ID         string
    Header     string    // e.g. "Agent(Deep code review for bugs)"
    StartedAt  time.Time
    Collapsed  bool
    ToolIDs    []string
}
```

The TUI maintains `m.groups map[string]*TranscriptGroup` keyed by `GroupID`
and rebuilds the visible transcript from `m.transcript` + `m.groups` on every
`refreshViewportContent`.

Group creation triggers:

- A new turn begins when `assistantTurnStartedMsg` arrives (a new event the
  agent layer must emit; today we infer from the first delta after a user
  message — make it explicit).
- A new nested group begins on `agenttool.Start` (already emitted via
  `ToolUseStart` with `tool == "Agent"`); the group header uses the agent's
  task summary.

### Collapsible Tool Batches (`ctrl+o`)

When a group contains more than `MaxVisibleToolsPerGroup` (default 4)
completed tools, the middle of the list collapses to a single placeholder line
`… +N tool uses (ctrl+o to expand)`. The first and last tool of the group
remain visible so the user retains beginning/end anchors.

- `ctrl+o` while focus is on the group expands all hidden children for that
  group only.
- `ctrl+shift+o` expands all groups in the visible transcript.
- The placeholder is itself focusable via `↑/↓` so the keybind is discoverable.

State lives in `TranscriptGroup.Collapsed` and a per-group
`ExpandedRunIDs map[string]bool` so re-collapsing the same group preserves any
explicit user choice.

### Background Execution (`ctrl+b`)

When the active task is in `RunPhaseRunningTool` or `RunPhaseWaitingModel` and
the tool has been running longer than `BackgroundEligibleAfter` (default 20s),
the active-task line gains the `(ctrl+b to run in background)` hint.

Pressing `ctrl+b`:

1. Marks the current tool's `state.ToolUse.Background = true`.
2. Detaches the corresponding run goroutine from the foreground status (it
   continues, its events still update transcript items).
3. Returns the TUI to idle state so the user can type a new prompt.
4. When the backgrounded tool eventually emits `ToolUseResult`, a system
   transcript item (`Background tool Bash completed in 3m 12s — output below`)
   is appended, and the footer strip shows `● bg: 1 running` until cleared.

Constraints:

- Only one background tool slot in v1. Attempting `ctrl+b` while one is already
  backgrounded triggers a transient tip `Background slot busy — wait for it to
  finish or run /bg list`.
- Background mode cannot be set on permission-blocked tools or on the
  `Agent(...)` tool (sub-agents); both have well-defined exit channels already.

Agent-layer changes required:

- Add `Backgrounded bool` to `agent.ToolUseStart` event so re-renders can
  distinguish.
- `agent.Run` must be re-entrant on the same `RunUIState` when the user submits
  a new prompt while a background tool is alive; in practice this just means
  the TUI starts a *new* `agent.Run` and treats the background tool as orphaned
  observability data, not part of the current turn.

### Animated Status with Whimsical Phase Verbs

The active-task line in the screenshot reads:

```text
* Bebopping…  (4m 26s · ↓ 4.8k tokens · almost done thinking with medium effort)
```

This communicates:

- A spinner + a non-clinical verb that varies (`Bebopping`, `Thinking`,
  `Pondering`, `Brewing`, …). The verb is sampled from a phase-specific list
  every 30 seconds so it changes during long runs without becoming distracting.
- Elapsed time at minute granularity once over 60 seconds, second granularity
  below.
- Token rate or total in the form `↓ N tokens` for streaming, `↑ N tokens` for
  prompt, or both `↑/↓ N/M` when both are meaningful.
- A phase-detail clause: `almost done thinking with medium effort`. This requires
  exposing the Ollama "think level" (`low`/`medium`/`high`, see
  THINKING-VISIBILITY-PLAN A1/C2) and a heuristic for "almost done" — currently
  Ollama does not provide this, so v1 must use a coarser label
  (`thinking…`, `streaming…`, `running Bash…`).

Implementation:

- Add `internal/tui/phaseverbs.go` with a `map[RunPhase][]string` of verbs and
  a deterministic sampler keyed by `(phase, runID, floor(elapsed/30s))` so
  tests are stable.
- Add a `ThinkEffort` field to `RunUIState` populated from
  `llm.ModelCapabilities(model).ThinkLevel` (new field, see C2 follow-on).
- Render via a single `renderActiveTask` helper, not inline string-building in
  `renderStatusBar`.

Verb lists (intentionally low-key, no marketing-ese):

| Phase | Verbs |
| :--- | :--- |
| `waiting_for_model` | Thinking, Pondering, Considering, Brewing, Cogitating |
| `streaming_response` | Composing, Writing, Streaming, Drafting |
| `running_tool` | Working, Crunching, Inspecting, Bebopping |
| `compacting_context` | Compacting, Tidying, Summarising |
| `retrying` | Retrying, Reconnecting, Re-rolling |

### Inline Tip

The line `└ Tip: Use /btw to ask a quick side question without interrupting
Claude's current work` is rendered immediately under the active-task line as
a child of it, using the same left-indent connector as the tool tree.

Tip selection rules:

- Tips are short, single-line, and context-aware.
- A tip is shown only when the run has been active for `TipAfter` (default 45s)
  *and* the user has not seen this tip within the same session.
- Tips never replace the spinner; they appear below it.
- If multiple tips qualify, pick the one whose trigger has been true longest.

Initial tip catalogue:

| Trigger | Tip |
| :--- | :--- |
| run > 45s | `Use /btw to ask a quick side question without interrupting the current run.` |
| tool > 60s | `ctrl+b runs this tool in the background so you can keep typing.` |
| > 4 tool calls in current group | `ctrl+o collapses or expands batched tool calls.` |
| permission denied 2x in a row | `Run /permissions show to see why these are being asked.` |
| context > 70% | `/compact summarises history to free context.` |

### `/btw` — Side Question Without Interruption

Add a slash command `/btw <question>` that:

1. Snapshots the in-flight conversation state (current `Messages`, the partial
   assistant message if streaming, the current tool stack) into a sidebar
   conversation.
2. Spawns a *new* short-lived agent run on a fresh context, using the same
   model and a minimal system prompt: `"Answer the following side question
   succinctly. Do not modify any files or run tools that have side effects."`
   Sub-agent tools are reduced to a read-only subset (Read, Grep, Glob, no
   Bash, no Edit, no Write).
3. Renders the side-conversation in a dimmed inset above the input box, so the
   main transcript is not perturbed.
4. When the side conversation closes (Enter on empty, or `/btw done`), the
   main run continues uninterrupted.

Restrictions:

- A `/btw` cannot itself spawn `/btw` (no recursion).
- A `/btw` cannot be backgrounded; it is always foreground.
- The main run's `RunUIState` is *not* paused; the side question is purely
  conversational and does not affect tool execution.

Implementation notes:

- This is a non-trivial slice. Treat as a UI-6 *optional* deliverable. The
  hierarchical tree + collapsible batches + background execution + animated
  status + tip strip are the must-haves; `/btw` can land in a follow-up phase
  UI-7 if velocity requires.

### Footer Strip

Replace the single status line with a two-band footer:

Band A (above input divider) — context-sensitive shortcut row:

```text
  esc to interrupt · ↓ to manage
```

The content depends on the current state:

| State | Band A |
| :--- | :--- |
| Idle | `enter submit · @ files · / commands` |
| Running, foregrounded | `esc to interrupt · ctrl+b background · ↓ to manage` |
| Running with background slot used | `esc to interrupt · ↓ to manage · bg active` |
| Permission modal open | `a allow · d deny · A always · esc cancel` |
| Picker open | `↑/↓ select · enter pick · esc cancel` |

Band B (below input) — persistent session metadata:

```text
  ● main                                                       2 modified, 1 untracked
  ○ general-purpose  Deep code review for bugs                 ↑/↓ select · Enter view
```

Band B shows:

- The current git branch with a `●` indicator coloured by repo state (green if
  clean, yellow if modified, red if conflicted). Read once at startup, then
  on `fs` notifications or after any successful tool that may have changed the
  worktree (FileEdit, Write, Bash with mutating intent).
- A list of queued or backgrounded sub-agents (`○ name  short summary`),
  navigable with `↑/↓` when input is empty. `Enter` opens a detail pane with
  that sub-agent's transcript so far.
- Optional right-aligned hints when something is selectable.

Implementation requires adding:

- `internal/tui/footer.go` — pure render functions taking a `FooterState`.
- `internal/git/branch.go` — minimal helper to read branch and dirty-state via
  `git symbolic-ref` and `git status --porcelain=v2` (cache 2s).
- A new `subagentSummary` accessor on `agenttool` so the footer can list them.

### Files Changed (Phase UI-6)

| File | Change |
| :--- | :--- |
| `internal/tui/transcript.go` | Add `GroupID`, `ParentID`, `Depth`, `BatchedInto`; add `TranscriptGroup`. |
| `internal/tui/app.go` | Build/maintain `m.groups`; new render pipeline `renderTranscriptTree`. |
| `internal/tui/footer.go` | New file: two-band footer rendering and state. |
| `internal/tui/phaseverbs.go` | New file: verb sampling and active-task formatting. |
| `internal/tui/tips.go` | New file: tip catalogue and trigger evaluation. |
| `internal/git/branch.go` | New file: branch + dirty-state reader with TTL cache. |
| `internal/agent/events.go` | Add `AssistantTurnStarted`, `Backgrounded bool` on `ToolUseStart`. |
| `internal/agent/agent.go` | Emit `AssistantTurnStarted` on first user→assistant transition per turn. |
| `internal/tools/agenttool/agenttool.go` | Expose `SubagentSummary` accessor for footer. |
| `internal/commands/registry.go` | (Optional UI-7) add `/btw` and `/bg` commands. |
| `internal/tui/app_test.go` | Tree rendering, collapse/expand, background path, tip selection, footer state tests. |

### Acceptance Criteria

Tree:

- Two tool calls under one assistant turn render with shared indentation and
  the same group header.
- Sub-agent tool calls render one level deeper.
- A 12-tool group renders as `first + … +10 tool uses + last`, expanding fully
  when `ctrl+o` is pressed while focus is on the group.
- Re-collapsing preserves the user's explicit expansions (so re-expanding shows
  the same expansion, not the default first/last only).

Background:

- A `Bash` tool running 25s shows the `(ctrl+b to run in background)` hint.
- Pressing `ctrl+b` returns the TUI to idle, lets the user submit another
  prompt, and the background tool's eventual completion appends a system note.
- Attempting `ctrl+b` on the `Agent(...)` tool does nothing visible and shows
  a tip explaining why.

Animated status:

- Verb changes deterministically at 30s boundaries and is stable across
  test runs (seeded sampler).
- For runs <60s, time format is `M:SSs`; for runs >=60s it is `Hh Mm`.
- Token counts come from `Meter` (requires the metrics fix in C11) and update
  on the same tick cadence as the spinner.

Tip strip:

- A tip appears at most once per session per trigger.
- Tip never appears in front of a permission modal; modal wins.
- Tip text never wraps; if it would exceed `width-4`, it is truncated with `…`.

Footer Band A:

- Shortcut row matches the current state table within one render.
- Modal-open state's shortcut row matches the modal's actual key bindings.

Footer Band B:

- Branch read is cached; rendering the footer never blocks on a git syscall.
- Queued sub-agents listed in start-time order; selection moves with `↑/↓`
  only when input box is empty.

### Risks & Mitigations (UI-6 specific)

| Risk | Mitigation |
| :--- | :--- |
| Tree rendering becomes slow with large transcripts. | Render-cache per `TranscriptItem`; rebuild only on items whose `Rendered` is empty. |
| Background tool state diverges from foreground events. | Background is purely a *display* flag; the agent goroutine and event channel are unchanged, so observability is preserved. |
| Verb sampling becomes annoying. | Sample only at 30s boundaries; provide `config.tui.serious = true` to disable verbs entirely. |
| Footer git read blocks on slow disks. | Read in a goroutine on a 2s TTL; if the read times out, show `?` for the branch. |
| `/btw` increases complexity for marginal benefit. | Defer to UI-7. UI-6 ships without `/btw`. |
| Tree depth degenerates with deep sub-agent chains. | Cap visual depth at 4 levels; deeper items render with a `↳ …` continuation marker. |

### Phase Ordering

UI-6 depends on:

- UI-0 (modal/permission rendering correctness) — for the picker/modal race
  fixes that the focusable footer would otherwise reintroduce.
- UI-1 (animated run status) — provides `RunUIState` and the tick scheduler.

UI-6 is independent of UI-2 (active tool *strip*); in fact UI-6 *replaces* the
UI-2 strip with the tree. If UI-6 is on the roadmap, UI-2 should be either
skipped or reduced to "render in-progress tool elapsed time" — the strip itself
becomes redundant.

### Open Questions

- Should the tree connector use heavy box-drawing (`├─ └─`) or the simpler
  indented-only form shown in the screenshot? Recommend the simpler form for
  legibility in narrow terminals.
- Should verb sampling be deterministic per session or per run? Recommend per
  run (so re-runs of the same prompt feel fresh).
- Should the footer git-status indicator update on every `FileEdit`, or only
  on TTL expiry? Recommend TTL-only for v1 to keep the implementation small.
- Should `/btw` be exposed before sub-agents have an isolated read-only tool
  manifest? Recommend no — ship `/btw` only after the read-only sub-agent
  toolset is wired.
