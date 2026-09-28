# Thinking Visibility & Bug Fix Implementation Plan

## Overview

This document covers two things:

1. **Bug fixes** — critical defects found during a full implementation review
2. **Thinking visibility** — a detailed plan to surface the LLM's reasoning process in the TUI

They are ordered by dependency: the bug fixes in Part A must land before Part B is
meaningful, because two of the bugs (`req.Think` never set, `LengthRetryTokens` too
small) directly undermine the thinking feature.

---

## Implementation Status — 2026-05-16

**Status:** Implemented and verified with `GOCACHE=/private/tmp/go-nandocode-llm-gocache go test ./...`.

### Completed

- **A1:** `req.Think` is set for models whose capability matrix reports `SupportsThinking`.
- **A2 / Part B:** Thinking blocks are visible, styled, tracked as streaming, and toggled with `Ctrl+T`.
- **A3:** Bootstrap `LengthRetryTokens` now defaults to `65536`.
- **A4:** Queued prompts are drained after a run completes.
- **A5:** Dead TUI `realProgramSender` was removed; the CLI-owned sender remains.
- **A6 / C8:** Bootstrap singleton access and test reset are guarded by a mutex.
- **A7 / C5:** Sub-agent JSONL now records thinking plus progress, retry, compaction, hook, and richer tool events.
- **C1:** REPL initializes bootstrap with config/flag overrides before the first `Global()` read.
- **C2:** `agent.DefaultConfig()` now has a safe `NumCtx` default of `32768`.
- **C3:** Default `MaxTurns` is a high safety cap (`200`) instead of unlimited.
- **C4:** `state.App.ToolContext` converts and propagates session permission mode.
- **C6:** `sendEventForce` is context-aware while still delivering buffered abort terminals when possible.
- **C7:** Permission prompt cancellation now clears the active TUI modal, covering agent-tool timeout cancellation paths.
- **C9 corrections:** Thinking finalizes on `Terminal` / `agentDoneMsg`, active thinking coalesces across interleaved transcript items, picker key handling keeps priority, and status priority prefers permission/tool state over thinking.
- **C10:** Memory extraction is detached after terminal events so the runner channel closes without waiting on the extraction LLM call.
- **C11:** LLM token counts are recorded by `RecordLLMChat`, first-token latency is averaged, and agent-run accounting avoids double-counting observed LLM tokens.
- **C12 tests added:** Retry token default, sub-agent thinking JSONL, empty-transcript `Ctrl+T`, permission cancellation modal clearing, detached memory extraction lifecycle, live token accounting, and token double-count avoidance.

### Deliberate Deviations / Follow-ups

- `C10` proposed a `memoryUpdatedMsg`; the implemented behavior detaches extraction and writes pending drafts silently because the runner event channel is intentionally closed after `Terminal`.
- `C9` mentions `agentStartFailedMsg`, but the current `AgentRunner.Run` interface returns only an event channel and has no start-error return path. The unused message type remains a future cleanup item.
- Manual model-level verification with a real thinking-capable Ollama model is still recommended.

---

## Part A — Bug Fixes

### A1. `req.Think` is never set — thinking models never reason

**Severity:** Critical (the entire thinking feature is dead without this)

**Root cause:**  
`internal/agent/stream.go:43-52` builds the `ChatRequest` but never checks
`llm.ModelCapabilities(model).SupportsThinking` or sets `req.Think`. The field
and the capability matrix exist but are never connected.

**Fix — `internal/agent/stream.go`**

Inside `executeOneTurn`, after the `req` struct literal is built (after line 52):

```go
// Activate thinking for capable models
if cap := llm.ModelCapabilities(model); cap.SupportsThinking {
    req.Think = true
}
```

`Think` is typed `any` (to support Ollama's `bool | "low"|"medium"|"high"`). Using
`true` activates the default thinking level. If a per-model level is desired later,
the capability struct can be extended with a `ThinkLevel string` field.

---

### A2. Thinking is always collapsed with no way to expand it

**Severity:** Critical (users can never see the thinking block)

**Root cause (two parts):**

1. `internal/tui/transcript.go:47` — `AppendThinkingDelta` creates items with
   `Collapsed: true`
2. `internal/tui/app.go:1109-1115` — `renderTranscript` only renders thinking when
   `!item.Collapsed`
3. No key binding anywhere toggles `Collapsed`

This is fixed in full in Part B below.

---

### A3. `LengthRetryTokens` = 512 in bootstrap defaults

**Severity:** High — every REPL session retries with a 512-token budget, which is
too small for any real content, so the retry always fails and falls through to
`TerminalContextOverflow`

**Root cause:**  
`internal/bootstrap/state.go:125` sets `LengthRetryTokens: 512`.  
`internal/agent/input.go:77` (DefaultConfig) correctly sets `65536`.  
`internal/cli/repl.go:149` uses `snap.LengthRetryTokens` (bootstrap value, not DefaultConfig).

**Fix — `internal/bootstrap/state.go`**, line 125:

```go
// Before
LengthRetryTokens: 512,

// After
LengthRetryTokens: 65536,
```

---

### A4. `QueuedPrompts` are silently dropped

**Severity:** High — prompts typed during an active run are accepted by the UI ("queued") but
never sent to the model

**Root cause:**  
`internal/tui/app.go:539-545` appends to `app.QueuedPrompts` when `ActiveRun` is
true. The `agentDoneMsg` handler (lines 232-245) only sets `ActiveRun = false` and
`refreshFileIndex`. Nothing ever drains the queue.

**Fix — `internal/tui/app.go`**, in the `agentDoneMsg` case block

Replace:

```go
case agentDoneMsg:
    if m.cancelRun != nil {
        m.cancelRun()
        m.cancelRun = nil
    }
    if m.activeRunCtx != nil {
        m.activeRunCtx = nil
    }
    m.compactCh = nil
    m.store.Set(func(app state.App) state.App {
        app.ActiveRun = false
        return app
    })
    m.closePicker()
    cmds = append(cmds, m.refreshFileIndexCmd("post-run"))
```

With:

```go
case agentDoneMsg:
    if m.cancelRun != nil {
        m.cancelRun()
        m.cancelRun = nil
    }
    if m.activeRunCtx != nil {
        m.activeRunCtx = nil
    }
    m.compactCh = nil
    m.closePicker()
    cmds = append(cmds, m.refreshFileIndexCmd("post-run"))

    // Drain one queued prompt if available; otherwise mark idle.
    appState := m.store.Get()
    if len(appState.QueuedPrompts) > 0 {
        next := appState.QueuedPrompts[0]
        m.store.Set(func(app state.App) state.App {
            app.QueuedPrompts = app.QueuedPrompts[1:]
            app.ActiveRun = true
            return app
        })
        cmds = append(cmds, m.startQueuedPrompt(next))
    } else {
        m.store.Set(func(app state.App) state.App {
            app.ActiveRun = false
            return app
        })
    }
```

Add helper method to `Model`:

```go
// startQueuedPrompt builds an agent run from a previously queued prompt string.
// The message was already appended to app.Messages when it was queued.
func (m *Model) startQueuedPrompt(prompt string) tea.Cmd {
    appState := m.store.Get()
    runCtx, cancel := context.WithCancel(context.Background())
    parentAbort := make(chan struct{})
    go func() {
        <-runCtx.Done()
        close(parentAbort)
    }()
    m.activeRunCtx = runCtx
    m.cancelRun = cancel
    m.compactCh = make(chan struct{}, 1)

    agentInput := agent.Input{
        Model:            appState.ActiveModel,
        Messages:         append([]llm.Message(nil), appState.Messages...),
        ToolContext:      appState.ToolContext(runCtx),
        PermissionMode:   appState.PermissionMode,
        PermissionRules:  appState.PermissionRules,
        PermissionPrompt: m.broker.PromptFunc(),
        ParentAbort:      parentAbort,
        CompactRequest:   m.compactCh,
        MaxOutputTokens:  appState.MaxOutputTokens,
    }
    return startAgentCmd(runCtx, m.agent, agentInput, func(msg tea.Msg) {
        if m.program != nil {
            m.program.Send(msg)
        }
    })
}
```

---

### A5. Dead `realProgramSender` in `tui/messages.go`

**Severity:** Low — unused code, no runtime impact

**Root cause:**  
`internal/tui/messages.go:63-69` defines `realProgramSender` and its `Send` method.
The same struct is also defined in `internal/cli/repl.go:270-276` in the `cli`
package. The CLI's version is the one actually instantiated. The TUI version is
never used within the TUI package (since `New()` accepts a `ProgramSender` interface
passed in from outside).

**Fix — `internal/tui/messages.go`**: delete lines 63-69 (the `realProgramSender`
struct and its `Send` method). The `ProgramSender` interface on line 57-59 must be
kept — it is used by production and test code.

---

### A6. Potential data race in `bootstrap.Global()`

**Severity:** Low in practice (startup is single-threaded), but technically undefined behavior

**Root cause:**  
`internal/bootstrap/state.go:191`: the `if globalState == nil` check reads a
package-level variable without holding any lock or using an atomic. A concurrent
goroutine writing `globalState` inside `globalOnce.Do()` and another goroutine
reading it on this line are a data race per Go's memory model.

**Fix — `internal/bootstrap/state.go`**

Remove the fast-path nil check. `sync.Once.Do()` is already cheap (a single atomic
load when initialized), so there is no meaningful performance loss:

```go
// Before
func Global() *State {
    if globalState == nil {
        globalOnce.Do(func() {
            globalState = New(DefaultInitial(""))
        })
    }
    return globalState
}

// After
func Global() *State {
    globalOnce.Do(func() {
        globalState = New(DefaultInitial(""))
    })
    return globalState
}
```

---

### A7. Sub-agent discards thinking deltas

**Severity:** Low — thinking from sub-agents is never recorded anywhere

**Root cause:**  
`internal/agent/subagent.go:171-206`: the event loop handles `AssistantTextDelta`,
`ToolUseStart`, `ToolUseResult`, and `Terminal`, but has no case for
`AssistantThinkingDelta`. Thinking text is silently dropped.

**Fix — `internal/agent/subagent.go`**, inside the event loop:

```go
case AssistantThinkingDelta:
    writeJSONL(outputSink, map[string]any{
        "ts":       time.Now().UTC().Format(time.RFC3339Nano),
        "kind":     "thinking",
        "thinking": e.Thinking,
    })
```

The thinking text is intentionally not added to `lastAssistant` — the sub-agent's
return value should remain the final answer, not the reasoning trace.

---

## Part B — Thinking Visibility Feature

### Goal

Give users clear, low-noise visibility into the model's reasoning process:

- **While streaming**: a live indicator in the status bar shows the model is thinking
- **After completion**: a collapsed one-liner shows the thinking block exists and its
  size, without overwhelming the chat view
- **On demand**: `Ctrl+T` expands/collapses any thinking block inline
- **Visual distinction**: thinking is styled differently from the assistant's final answer

### B1. Add `CharCount` and `Streaming` to `TranscriptItem`

**File: `internal/tui/transcript.go`**

Extend `TranscriptItem`:

```go
type TranscriptItem struct {
    Kind      TranscriptKind
    ToolID    string
    ToolName  string
    Content   string
    Collapsed bool
    Error     string
    Rendered  string // cached rendered markdown
    CharCount int    // total chars accumulated (thinking blocks only)
    Streaming bool   // true while deltas are still arriving
}
```

Update `AppendThinkingDelta` to track char count and mark as streaming:

```go
func AppendThinkingDelta(items []TranscriptItem, content string) []TranscriptItem {
    if len(items) > 0 && items[len(items)-1].Kind == TranscriptThinking {
        last := &items[len(items)-1]
        last.Content += content
        last.CharCount += len(content)
        last.Rendered = "" // invalidate render cache
        return items
    }
    return append(items, TranscriptItem{
        Kind:      TranscriptThinking,
        Content:   content,
        Collapsed: true,
        CharCount: len(content),
        Streaming: true,
    })
}

// FinalizeThinkingItem marks the most recent thinking item as no longer streaming.
func FinalizeThinkingItem(items []TranscriptItem) []TranscriptItem {
    for i := len(items) - 1; i >= 0; i-- {
        if items[i].Kind == TranscriptThinking {
            items[i].Streaming = false
            return items
        }
    }
    return items
}
```

---

### B2. Track `thinkingActive` on the TUI model

**File: `internal/tui/app.go`** — `Model` struct (around line 29)

Add one field:

```go
thinkingActive bool // true while AssistantThinkingDelta events are arriving
```

Update `handleAgentEvent` to maintain this field and call `FinalizeThinkingItem`
when thinking ends:

```go
case agent.AssistantThinkingDelta:
    m.thinkingActive = true
    m.transcript = AppendThinkingDelta(m.transcript, e.Thinking)

case agent.AssistantTextDelta:
    if m.thinkingActive {
        m.thinkingActive = false
        m.transcript = FinalizeThinkingItem(m.transcript)
    }
    m.transcript = AppendAssistantDelta(m.transcript, e.Content)

case agent.Terminal:
    m.thinkingActive = false
    m.transcript = FinalizeThinkingItem(m.transcript)
    m.store.Set(func(app state.App) state.App {
        // ... existing terminal handling ...
    })
```

---

### B3. Add thinking styles

**File: `internal/tui/styles.go`**

Add three styles to the `Styles` struct:

```go
ThinkingCollapsed lipgloss.Style
ThinkingExpanded  lipgloss.Style
ThinkingBox       lipgloss.Style
```

Initialize them in `DefaultStyles()`:

```go
ThinkingCollapsed: lipgloss.NewStyle().
    Foreground(lipgloss.Color("243")).
    Italic(true),

ThinkingExpanded: lipgloss.NewStyle().
    Foreground(lipgloss.Color("111")).
    Italic(true),

ThinkingBox: lipgloss.NewStyle().
    Foreground(lipgloss.Color("244")).
    BorderLeft(true).
    BorderStyle(lipgloss.ThickBorder()).
    BorderForeground(lipgloss.Color("237")).
    PaddingLeft(1),
```

---

### B4. Render thinking blocks

**File: `internal/tui/app.go`** — `renderTranscript()`, the `TranscriptThinking` case
(lines 1109-1115)

Replace the existing block:

```go
// Before
case TranscriptThinking:
    if !item.Collapsed {
        output.WriteString(m.styles.Help.Render("Thinking:"))
        output.WriteString("\n")
        output.WriteString(item.Content)
        output.WriteString("\n\n")
    }
```

With:

```go
// After
case TranscriptThinking:
    if item.Collapsed {
        label := "▶ Thinking"
        if item.Streaming {
            label = "▶ Thinking…"
        } else if item.CharCount > 0 {
            label = fmt.Sprintf("▶ Thinking  (%d chars)  Ctrl+T to expand", item.CharCount)
        }
        output.WriteString(m.styles.ThinkingCollapsed.Render(label))
        output.WriteString("\n\n")
    } else {
        header := fmt.Sprintf("▼ Thinking  (%d chars)  Ctrl+T to collapse", item.CharCount)
        output.WriteString(m.styles.ThinkingExpanded.Render(header))
        output.WriteString("\n")
        // Wrap in the left-border box style; do not run through markdown renderer
        // since thinking is raw prose from the model, not markdown.
        output.WriteString(m.styles.ThinkingBox.Render(item.Content))
        output.WriteString("\n\n")
    }
```

The thinking block is intentionally **not** passed through the markdown renderer.
Thinking content from Ollama models is raw prose with no guaranteed markdown
structure, and rendering it through glamour can produce garbled output on partial
content.

---

### B5. Add `Ctrl+T` key binding

**File: `internal/tui/app.go`** — `handleKeyMsg`

**Insert mode** — add before the picker-visible block (around line 398), as a
top-level check inside `if m.vim.IsInsert()`:

```go
// Ctrl+T: toggle most recent thinking block
if msg.Type == tea.KeyCtrlT {
    m.toggleLastThinkingItem()
    return nil, true
}
```

**Normal mode** — add inside the `switch msg.String()` block (around line 375):

```go
case "ctrl+t":
    m.toggleLastThinkingItem()
    return nil, true
```

Add the helper method:

```go
// toggleLastThinkingItem flips Collapsed on the most recent thinking transcript item.
func (m *Model) toggleLastThinkingItem() {
    for i := len(m.transcript) - 1; i >= 0; i-- {
        if m.transcript[i].Kind == TranscriptThinking {
            m.transcript[i].Collapsed = !m.transcript[i].Collapsed
            m.transcript[i].Rendered = "" // invalidate cache
            m.refreshViewportContent(false) // don't scroll; user is reading
            return
        }
    }
}
```

`refreshViewportContent(false)` is used (not `true`) so that toggling open a large
thinking block doesn't jump the viewport to the bottom while the user is reading.

---

### B6. Update status bar with thinking indicator

**File: `internal/tui/app.go`** — `renderStatusBar` (around line 1217)

```go
func (m *Model) renderStatusBar(appState state.App) string {
    status := fmt.Sprintf("Model: %s | Mode: %s", appState.ActiveModel, m.vim.Mode)
    if appState.ActiveRun {
        if m.thinkingActive {
            status += " | [Thinking...]"
        } else {
            status += " | [Running...]"
        }
    }
    // ... rest of existing status bar logic unchanged ...
```

---

### B7. Update `/help` text

**File: `internal/commands/registry.go`** — `handleHelp`, add one line to the help
string:

```
  Ctrl+T                         - Expand/collapse thinking block
```

---

### B8. Capability matrix — ensure `qwen3:thinking` normalizes correctly

The current `normalizeModelName` already handles arbitrary size tags correctly (the
`strings.Index(name, ":")` strip on line 95 removes everything after the first colon).
However, Ollama ships a dedicated thinking variant under the name `qwen3` with
`/think` invocation, not a separate model tag. The matrix already has `"qwen3-thinking"`.

Verify this covers actual Ollama model names in use. If the thinking variant is
invoked as `qwen3:latest` with a system prompt prefix, `SupportsThinking` should be
`false` for `qwen3` (the base model does not expose a `think` response field without
the flag — Ollama ignores `Think: true` for models that don't support it, so setting
it on non-thinking models is safe but wastes a tiny bit of serialization).

No code change required here; the existing matrix is correct. Document this as a
known limitation: the capability matrix is static and must be updated when new
thinking models are added.

---

## Implementation Order

Execute in this sequence to avoid regressions at each step:

| Step | Change | File | Risk |
|------|--------|------|------|
| 1 | Fix `LengthRetryTokens` 512 → 65536 | `bootstrap/state.go:125` | None |
| 2 | Fix bootstrap data race | `bootstrap/state.go:191` | None |
| 3 | Fix dead `realProgramSender` | `tui/messages.go:63-69` | None |
| 4 | Fix sub-agent thinking JSONL | `agent/subagent.go` | None |
| 5 | Set `req.Think` in `executeOneTurn` | `agent/stream.go` | Low |
| 6 | Add `CharCount`, `Streaming` to `TranscriptItem` + update helpers | `tui/transcript.go` | Low |
| 7 | Add `ThinkingCollapsed/Expanded/Box` styles | `tui/styles.go` | None |
| 8 | Add `thinkingActive` to `Model`; update `handleAgentEvent` | `tui/app.go` | Medium |
| 9 | Update `renderTranscript` thinking case | `tui/app.go` | Low |
| 10 | Add `toggleLastThinkingItem` + `Ctrl+T` key binding | `tui/app.go` | Low |
| 11 | Update `renderStatusBar` with thinking indicator | `tui/app.go` | None |
| 12 | Update `/help` text | `commands/registry.go` | None |
| 13 | Fix `QueuedPrompts` drain + `startQueuedPrompt` helper | `tui/app.go` | Medium |

Steps 1–4 are isolated single-line or small changes. Steps 5–12 are the thinking
feature. Step 13 (QueuedPrompts) is independent and can be done in a separate commit.

---

## Testing Strategy

### Manual test: thinking visibility
1. Start the REPL with a thinking-capable model (e.g. `qwen3-thinking`)
2. Ask a complex reasoning question: `"Analyse this project: what could go wrong?"`
3. Verify: status bar shows `[Thinking...]` during the reasoning phase
4. Verify: after response, a collapsed `▶ Thinking (N chars) Ctrl+T to expand` line appears
5. Press `Ctrl+T` — block expands in-place without scrolling to bottom
6. Press `Ctrl+T` again — block collapses
7. Press `Ctrl+T` in Normal mode — same behavior

### Manual test: large document analysis
1. Run: `analyse this project what could go wrong` — includes many file reads via tools
2. Verify no `TerminalContextOverflow` occurs prematurely (validates A3 fix)
3. Verify that if you type a follow-up while the run is active, it is queued and sent
   automatically when the run completes (validates A4 fix)

### Manual test: thinking on non-thinking model
1. Switch to a non-thinking model (e.g. `llama3.1`)
2. Ask a question — no `[Thinking...]` indicator should appear, no thinking block
   should appear in the transcript

---

## Known Limitations Not In Scope

- **Per-model think level** (`"low"|"medium"|"high"` vs `true`) — exposing this as a
  config or `/think-level` command is desirable but out of scope for this change
- **Thinking in sub-agents** — sub-agents now write thinking to the JSONL output file
  (A7) but the thinking is not surfaced to the parent agent's TUI transcript; that
  would require a new event type propagation chain
- **Persistent thinking toggle** — toggling a thinking block collapsed/expanded is
  session-only; on `/clear` all transcript items are wiped, as expected
- **Thinking in compaction** — the compaction prompt currently ignores `Thinking`
  fields in message history; this is acceptable since thinking is ephemeral reasoning,
  not conversation content

---

## Part C — Review Addendum (additional bugs and plan corrections)

A second-pass review surfaced more defects and plan issues. These should land in the
same PR family because several change the same files and several invalidate fixes
already in Parts A and B.

### C1. Bootstrap config-override is silently discarded

**Severity:** Critical — this defeats A3 entirely.

`internal/cli/repl.go:94` calls `bootstrap.Global().Snapshot()` *before*
`bootstrap.InitGlobal(initial)` runs. `Global()` invokes `sync.Once.Do` and
constructs the singleton from `DefaultInitial("")`. The `InitGlobal(initial)` call
at line 95 is guarded by `if SessionID == ""`, and `DefaultInitial` populates
`SessionID`, so the guard is true → `InitGlobal` never runs. **Every CLI flag and
config-derived override built into `initial` at `repl.go:51-91` is dropped.**

Fix options:
- Re-order: build `initial` first, then call `bootstrap.InitGlobal(initial)`
  *before* any `Global()` call.
- Or expose `bootstrap.SetGlobal(state)` for explicit replacement and drop the
  `sync.Once` gating. Document that the singleton is initialized exactly once at
  the top of `main`.

Pair this fix with A3 — otherwise the bootstrap default for `LengthRetryTokens`
becomes a hardcoded permanent value, not a default.

### C2. `agent.Config.NumCtx` has no safe default

`internal/agent/input.go:74` (`DefaultConfig`) leaves `NumCtx = 0`. Sub-agents and
any non-REPL caller that doesn't probe the model send `num_ctx: 0` to Ollama.
Ollama interprets this as "use server default", which is often 2048 — undermining
long-context reasoning silently.

Fix: set `NumCtx` to a sane default (e.g. `32768`) in `DefaultConfig`, and have
`cli/repl.go:151` overwrite it only when the model probe succeeds.

### C3. `MaxTurns: 0` ("unlimited") removes the runaway-loop safety net

Uncommitted changes set `MaxTurns: 0` everywhere (`bootstrap/state.go:122`,
`agent/input.go:76`, `agent/agent.go:120`). A stuck tool-call loop will now never
terminate on its own. Either restore a high default (e.g. `200`) or add a
soft-limit warning at, say, 100 turns.

### C4. `App.ToolContext` ignores `PermissionMode`

`internal/state/app.go:212` hardcodes `PermissionMode: tools.PermissionDefault`.
Session permission mode (`bypass`, `dontask`, `restrictive`) is dropped before
tools see it. Fix: assign `a.PermissionMode` instead of the constant.

### C5. Sub-agent JSONL is missing most event kinds

Plan A7 only adds `AssistantThinkingDelta`. Also missing: `ToolUseProgress`,
`RetryNotice`, `CompactionStarted`, `CompactionCompleted`, `HookNotice`. Adding
them in the same edit keeps sub-agent observability symmetric with the parent.

### C6. `sendEventForce` is a deadlock primitive

`internal/agent/agent.go:485-487` sends on `events` with no `select { case ...
case <-ctx.Done(): }`. If the consumer has already exited (e.g. `runSubagent`
early-returned on a non-completed Terminal), the send blocks forever. Add a
ctx-aware select with a logged drop on cancel.

### C7. `agenttool.wrapPermissionPrompt` orphans the modal on timeout

`internal/tools/agenttool/agenttool.go:141-150`: on timeout the wrapper returns
`deny` to the sub-agent, but the goroutine that opened the broker prompt keeps
the modal on screen. The user's subsequent choice is sent to a dead channel and
silently dropped. Fix: have the timeout path call a `broker.Cancel(promptID)` or
close the modal before returning.

### C8. `bootstrap.ResetGlobalForTest` races (companion to A6)

`state.go:211-214` resets both `globalOnce` and `globalState` without a lock.
Fix in the same commit as A6: guard the reset with a mutex, or replace the whole
singleton pattern with `SetGlobal`.

### C9. Plan errors in Parts A and B

- **A5 line range is off**: `realProgramSender` is at `internal/tui/messages.go:61-69`
  (declaration starts line 62, leading comment line 61). Plan says `63-69`.
  Update to "delete lines 61-69" so the orphaned comment is removed.
- **A4 omits the failure path**: `startQueuedPrompt` does not handle
  `agentStartFailedMsg`. If start fails, `m.activeRunCtx`, `m.cancelRun`, and
  `app.ActiveRun` are left dangling. Add: on agent-start failure, clear those
  fields and append a transcript error item.
- **A4 leaks the parentAbort goroutine** if `cancel` is never invoked (e.g.
  panic before submit returns). Both the new helper and the existing path at
  `app.go:558-561` share this — wrap the goroutine in a `defer close(parentAbort)`
  driven off `runCtx.Done()` only, and ensure `cancel` is always called via
  `defer` in `Update`.
- **B2 ordering bug**: A single Ollama stream event can carry both `Content` and
  `Thinking` simultaneously (`agent/stream.go:88-99` emits two deltas per event).
  The plan's "on AssistantTextDelta, finalize thinking" logic prematurely
  finalizes thinking when text and thinking interleave. Replace with: only
  finalize thinking when `Terminal` fires, OR when text arrives AND no thinking
  delta has arrived in the last 250ms (debounce). Simplest: finalize only on
  `Terminal` / `agentDoneMsg`.
- **B2 missing `agentDoneMsg` handler**: if `Terminal` is never emitted (panic,
  ctx cancel mid-stream), `thinkingActive` stays true and the spinner runs
  forever. Always clear `thinkingActive` and call `FinalizeThinkingItem` on
  `agentDoneMsg`.
- **B4 multiple thinking blocks**: `AppendThinkingDelta` only coalesces when the
  *last* item is `TranscriptThinking`. If any non-thinking item lands between two
  thinking deltas, a second collapsed block appears below. Change coalescing to
  "find most recent thinking item with `Streaming == true`", not "last item".
- **B5 ordering vs picker**: the insert-mode `Ctrl+T` handler must be inserted
  **after** the `if m.picker.Visible { ... }` block (so picker keys win) and
  **before** the `switch msg.Type` block — not "before the picker-visible block"
  as the plan states.
- **B6 status priority**: the proposed `[Thinking...]` segment is appended
  unconditionally. When a tool is also active, the more specific tool name
  should win. Priority: `permission_required` > `running_tool` > `thinking` >
  `streaming` > `waiting_for_model` > idle.
- **B7 normal-mode `ctrl+t` shadow check**: confirm `internal/tui/vim.go` doesn't
  consume `t` (Vim's `t<char>` "till" motion). If it does, gate Ctrl+T through a
  higher-priority handler before vim mode handling.

### C10. `memory.Runner.Run` keeps the run "active" during extraction

`internal/memory/runner.go:76` runs `extractPending` (an LLM call gated by
`cfg.ExtractTimeout`) **between** the `Terminal` event and channel close. The
TUI sees `ActiveRun = true` for many seconds after the answer finishes streaming,
and worse, this LLM call shares the same Ollama connection.

Fix: emit `Terminal` and close the events channel first, then run extraction in
a detached goroutine that posts a `memoryUpdatedMsg` when done.

### C11. `observability.Meter.RecordLLMChat` ignores token counts

`internal/observability/metrics.go:89-91` discards `promptTokens` and
`completionTokens`; `LLMFirstTokenLatency` is summed instead of averaged. Status
bar token display only updates after `RecordAgentRun` fires (post-run). For
ADR-001's live `tokens: N` plan, fix this first or accept that the count is
stale until completion.

### C12. Acceptance / test gaps

- Add an assertion in `bootstrap_test.go` that `DefaultInitial("").LengthRetryTokens == 65536`
  so A3 cannot regress silently.
- Add a `subagent_test.go` case verifying thinking deltas land in JSONL (A7).
- Add a TUI test that toggling `Ctrl+T` on an empty transcript is a no-op (B5).
- Add a TUI test that opening the picker, then receiving a `permissionPromptMsg`,
  closes the picker before showing the modal.
- Add a test for the queue-drain failure path (A4 + C9): `startQueuedPrompt`
  failure must clear `ActiveRun` and surface an error transcript item.

### C13. Revised implementation order

Insert before existing step 1:

| Step | Change | File |
|------|--------|------|
| 0a | Fix bootstrap init ordering so `InitGlobal` actually applies overrides | `cli/repl.go:94-95` |
| 0b | Set `NumCtx` default in `DefaultConfig` | `agent/input.go:74` |
| 0c | Restore a `MaxTurns` safety cap | `bootstrap/state.go:122`, `agent/input.go:76`, `agent/agent.go:120` |
| 0d | Use `a.PermissionMode` in `App.ToolContext` | `state/app.go:212` |

After existing steps 1–4, insert:

| Step | Change | File |
|------|--------|------|
| 4a | Make `sendEventForce` ctx-aware | `agent/agent.go:485` |
| 4b | Extend sub-agent JSONL with progress/retry/compaction/hook | `agent/subagent.go` |
| 4c | Detach memory extraction from run lifecycle | `memory/runner.go:76` |
| 4d | Cancel agenttool broker prompt on timeout | `tools/agenttool/agenttool.go:141` |

Step 13 (QueuedPrompts drain) gains a 13a: handle `agentStartFailedMsg` in the
drain path and add the matching test.
