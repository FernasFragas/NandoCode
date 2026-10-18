# Browser Visual Revamp Plan (BV)

**Date:** 2026-10-18
**Status:** BV-0 done; BV-1 to BV-8 not started
**Roadmap step:** BV in [`NEXT-PHASES-IMPLEMENTATION-PLAN.md`](../roadmap/NEXT-PHASES-IMPLEMENTATION-PLAN.md)
**Visual spec:** [`docs/plans/UX/DESIGN-SPEC.md`](UX/DESIGN-SPEC.md) and the two mockups in `docs/plans/UX/`
**Idea one-pager:** [`docs/ideas/browser-visual-revamp.md`](../ideas/browser-visual-revamp.md)
**Supersedes:** slice UI-6 of [`WEB-UI-UX-PRODUCT-PLAN.md`](WEB-UI-UX-PRODUCT-PLAN.md). UI-7 panels stay in B3 and render inside the BV shell.

## Goal

Make the browser UI look like the two mockups (same layout, palette, density
and components) and work like an IDE-grade control room for the local agent.
Show real data only.

The mockups are the visual spec, not the functional spec. Every value on screen
comes from a real event or endpoint. A region without a backend shows an honest
empty state, never stub data. Each slice waits for the roadmap step that
delivers its backend, so nothing is built twice.

## Rules (in addition to AGENTS.md)

- Plain ES modules, no build tools, no npm. Render functions are DOM-free and
  return `{tag, attrs, children}` trees. A small builder (`web/js/dom.js`) turns
  them into DOM in the browser. `node --test` asserts on the trees.
- Text is always a text node. The builder has no raw-HTML path except
  `markdown.js` output, which is already sanitized (B1-1).
- No inline `style=` or inline `<script>` in new markup (prepares B4's CSP).
- No web fonts and no CDN. Use the system font stack and the hand-drawn `icons.svg`.
- Every slice's PR includes a 1448x1086 screenshot next to the matching mockup
  crop (manual mockup check). After approval, that gallery screenshot becomes
  the baseline for `make visual-check`.
- Tests never touch the real home directory. Any test file you modify loses its
  `time.Sleep` synchronization.

## Decisions (2026-10-18)

| Topic | Decision |
| --- | --- |
| Success | Same look as the mockups, real data only; honest empty states |
| Palette | Exact median samples from mockup A (`tokens.css`) |
| Font / icons | System font stack / ~35 hand-drawn SVG line icons |
| Breakpoints | < 1440 nav → icon rail; < 1200 tree → toggle; < 960 rail → drawer |
| Gallery | `web/dev/gallery.html`, served only with `nandocodego server --dev` |
| Fixtures | Golden files written by a Go test from a fake-LLM session (`-update`) |
| Visual check | `make visual-check`: Chrome CLI screenshots + stdlib Go diff against approved baselines; local only, not CI |
| Token | `#token=` fragment handoff (BF-7); paste fallback in the settings gear |
| Nav | Chat, Project, Agents, Transcript, Tasks, Diagnostics (see DESIGN-SPEC "Nav") |
| New session | Sidebar only |
| Mentions | `@` hover action in the tree; no nav item |
| Usage | Live: `usage_update` after every LLM turn |
| Tasks | Agent todos (`TodoWrite`) first, background tasks below; no "Add task" |
| Permission pill | Scrolls to the pending card and focuses Allow |
| Recent events | Every event except text/thinking deltas and `heartbeat`; last 5 |
| Event stores | Separate stores for Recent events, Transcript view and Agents log, with a consistency test |
| Center | Resizable split: transcript and composer on top, tabs below |
| Permission card | Popover beside the tool card (inline below 1200 px); X dismisses, the request stays pending |
| Thinking | Open while streaming, collapses when the answer starts |
| Sessions | In memory until Phase 25; model-generated title after the first `terminal`; confirm Clear only; hover delete; evict the oldest idle session at the cap |
| Files API | `GET /v1/sessions/{id}/files?path=` replaces `/tree`; 2 MB cap; no highlighting above 256 KB |
| Languages | Go, JS, JSON, HTML, CSS, Markdown (via `markdown.js`), YAML, TOML, shell |
| Agent writes an open file | Auto-reload if clean; banner if dirty |
| Open tabs | Restored from `sessionStorage` (paths only) |
| Save | Same decision path as the agent's `FileWrite`; Always allow = session rule `FileWrite(path)`; Write hooks run; blocked only while the run writes that file; content hash → 409 |
| Agents | Left panel = this session's workers; forward lifecycle, current step, tokens + files, mailbox; graph whenever ≥ 1 worker |

## Slices

Order and gates. A slice may start only when every step it waits for is accepted.

| Slice | Waits for | Summary |
| --- | --- | --- |
| BV-0 | — | Spec, tokens, icons. **Done 2026-10-18.** |
| BV-1 | B1-1 | Fixtures, render modules, gallery, visual-check tool |
| BV-2 | B2 (BF-1 page split) | Shell: header, grid, nav, footer, token handoff |
| BV-3 | BV-2 | Right rail: status, live usage, todos and tasks, diagnostics, recent events |
| BV-4 | BV-2 | Transcript cards, composer, permission popover |
| BV-5 | B2 (BF-2, BF-5, BF-6) | Sessions list, Stop run, Clear, Compact, titles |
| BV-6 | B2 routing, B1-4 tree walk | Files API, tabs, read-only code viewer, highlighter |
| BV-7 | BV-6 | Save |
| BV-8a | B1.5 agent event invariants | `subagent_*` events from the backend |
| BV-8b | BV-8a, BV-2 | Agents tab UI |

BV-3, BV-4, BV-5 and BV-6 may run in parallel once BV-2 has merged.

---

### BV-0: Spec, tokens, icons (done)

Files: `docs/plans/UX/DESIGN-SPEC.md`, `internal/server/web/css/tokens.css`,
`internal/server/web/icons.svg`.

Acceptance: the spec lists tokens, layout, breakpoints, nav, the component
inventory with data sources, empty states, and the mockup noise not to copy.
`tokens.css` and `icons.svg` exist and are not yet linked; BV-2 links them.

### BV-1: Component gallery and fixtures

Tasks:

1. `internal/server/web/js/dom.js`: `h(tag, attrs, ...children)` returns a
   plain tree, and `mount(tree)` builds DOM with `createElement` /
   `createElementNS` for SVG. `on*` attributes, `javascript:` URLs, and
   non-allowlisted attributes are rejected.
   - The SVG namespace string `http://www.w3.org/2000/svg` would fail
     `tools/check-network-policy.sh`. Add that exact string to
     `ALLOWED_PATTERNS` with a comment saying it is an XML namespace identifier,
     not an endpoint. Record this in PHASE-LOG.
2. Render modules under `internal/server/web/js/ui/` (one file per group):
   `header.js`, `nav.js`, `sessions.js`, `tree.js`, `transcript.js` (message,
   thinking, chips, tool card), `permission.js`, `rail.js` (status row, usage,
   todo row, task row, event row), `tabs.js`, `agents.js` (worker row, node,
   aggregation, log row). Each is a pure function: data → tree.
3. Fixture recorder: `internal/server/fixtures_test.go`
   `TestRecordGalleryFixtures` drives a session with a scripted fake LLM client
   through thinking, `search_files`, a `permission_request` for
   `run_command`, a `TodoWrite`, and `terminal`. It writes the events to
   `internal/server/web/dev/fixtures/*.json` when run with `-update`, and
   compares them otherwise. Paths are relative and times are fixed.
4. `internal/server/web/dev/gallery.html` + `gallery.js`: renders every
   component from the fixtures, with width buttons 1280 / 1440 / 1920.
5. Serve `/dev/` only with `--dev`: add a `Dev bool` option in the server
   options and the `server --dev` flag.
6. `tools/visualdiff` (Go, stdlib `image/png`): compares two PNGs with a
   per-image threshold, and `make visual-check` screenshots the gallery
   with `chrome --headless --screenshot --window-size=1448,1086`. Baselines
   go in `internal/server/web/dev/baselines/`. Local only.

RED tests first:

- `dom.test.js`: `mount` of `h('a', {href: 'javascript:alert(1)'})` drops the
  href. `h('div', {onclick: 'x'})` drops the attribute. Text containing
  `<img onerror>` stays text.
- `ui/*.test.js`: each component renders from its fixture, and a hostile
  string (`"><script>`) in any field stays a text child.
- `TestDevGalleryRequiresFlag`: `GET /dev/gallery.html` is 404 without
  `--dev` and 200 with it.
- `TestRecordGalleryFixtures`: fails when an event shape drifts from the
  committed fixture.
- `tools/visualdiff` `TestDiffIdenticalAndChanged`.

Acceptance: the gallery at 1448 px is approved side by side with both
mockups. The `--border` and text colours are rechecked and the spec is
updated if they change. Baselines are committed.

### BV-2: Shell

Waits for B2's BF-1 page split. Build the new shell as part of that split,
not as a reskin of the monolithic `index.html`.

Tasks:

1. `index.html` holds only the layout skeleton and links `css/tokens.css`,
   `css/app.css`, and `js/app.js`. No inline style or script.
2. Header: brand, "Local server mode" pill, session id with a copy button,
   model picker, connection pill, Stop run slot (wired in BV-5), and settings
   gear (token paste fallback).
3. Four-column CSS grid using the token widths and the three breakpoints;
   the nav icon rail has tooltips.
4. Nav mapping from DESIGN-SPEC; Mentions panel removed, `@` hover action on
   tree rows.
5. Footer: server status, URL, `version.String()`.
6. Token handoff: read `#token=`, store it in `sessionStorage` (wrapped in
   try/catch), and remove it from the URL with `history.replaceState`.
7. Move today's features into the new layout without changing their
   behaviour.

RED tests first:

- `token.test.js`: `extractToken('#token=abc&x=1')` returns `abc`, and the
  cleaned hash no longer contains it.
- `TestRoutesServeRichEmbeddedUI` (updated): `index.html` contains no
  `style="` and no inline `<script>` body.
- `layout.test.js`: `navMode(width)` returns `full` / `rail` and
  `treeMode(width)` and `railMode(width)` match the breakpoints.

Acceptance: the shell matches the mockup frame at 1448 px. At 1280 px the
nav is an icon rail with no horizontal scroll. Every existing browser feature
still works (manual run through the B2 checklist).

### BV-3: Right rail

Tasks:

1. Session status: id, model, run state, last event, permissions pending
   (click scrolls to the card and focuses Allow).
2. Usage: emit `usage_update` from the server after each LLM turn with the
   `agent.Usage` fields. Render PromptEvalCount, EvalCount, TotalDuration,
   Turns, ToolCalls, DoneReason (`running` while active).
3. Tasks:
   - Wire a `todo.TodoList` into every server session (today only `cli/repl.go` does).
   - Emit `todo_updated` with the full list after each `TodoWrite`.
   - Render todos first, then background tasks from `task_lifecycle`.
4. Diagnostics: server health (`/v1/health`), SSE state with age, last event,
   and index status ("Unavailable" until the B2 index-status endpoint exists).
5. Recent events: a separate store that takes every event except
   `assistant_text_delta`, `assistant_thinking_delta` and `heartbeat`, and
   shows the last 5. Failures get a red dot.

RED tests first:

- `TestServerRunEmitsUsageUpdatePerTurn`: a two-turn fake run emits two
  `usage_update` events with growing counts before `terminal`.
- `TestServerSessionTodoWriteEmitsTodoUpdated`: a `TodoWrite` call in a server
  run succeeds and emits `todo_updated` with the items.
- `eventstore.test.js`: the Recent-events store excludes deltas and
  heartbeats. Feeding one fixture stream into the Recent-events store and the
  Transcript store gives the same non-delta event IDs in the same order
  (separate-store consistency).
- `rail.test.js`: usage before the first turn renders `—`, not `0`.

Acceptance: during a live run the rail updates without a reload. It matches
the mockup's right column at 1448 px.

### BV-4: Transcript

Tasks:

1. Message rows with U / AI avatars and timestamps.
2. Thinking card: open while streaming with a live timer, collapses when the
   first `assistant_text_delta` arrives, and can be reopened.
3. Chips row from `stage_timing`, `semantic_retrieval` and `prompt_pack_report`.
4. Tool card: name, status pill (Running / Completed / Failed), duration,
   parameters, and results. Results are capped with a "show all" toggle.
5. Permission popover anchored beside the pending tool card, falling back to
   inline below 1200 px. Allow / Deny / Always allow; X hides the card while
   the request stays pending and the rail pill still shows it. Keyboard: focus
   moves to Allow; Escape is the same as X.
6. Composer at the bottom of the transcript pane: `@` autocomplete and
   Shift+Enter as today.
7. Resizable split with a drag handle. The tab area collapses to 0 when no
   tab is open.

RED tests first:

- `transcript.test.js`: the thinking card reports `collapsed: true` after a
  text delta. A tool card for a failed `tool_use_result` has the Failed pill.
- `permission.test.js`: dismissing the card leaves the request in the
  pending store. Resolving it removes it from both.
- `TestPermissionCardClosesOnBrokerTimeout` (with B2's broker-timeout fix):
  the server emits a resolution event on timeout, and the card closes.

Acceptance: the transcript matches the mockup's upper center region at 1448 px.

### BV-5: Sessions and header actions

Waits for B2's BF-2 (cancel via `runctl`), BF-5 (clear, compact) and BF-6
(session list).

Tasks:

1. Sessions list: radio, short id, time, title, active highlight, hover
   delete. Switching sessions re-subscribes to SSE.
2. Stop run (header), Clear (confirm dialog), Compact (immediate, with a toast).
3. At the 10-session cap, New session evicts the oldest **idle** session
   (never a running one) and shows a toast.
4. Titles: after the first run's `terminal`, call the session's model
   asynchronously for a short title and emit `session_titled`. The fallback is
   the truncated first prompt. For a cloud model, the credential gate applies.
   The call never runs during a run.

RED tests first:

- `TestNewSessionAtCapEvictsOldestIdle`: 10 sessions with the oldest one
  running → the oldest idle session is evicted and the running one survives.
- `TestSessionTitleAfterFirstTerminal`: a fake client emits `session_titled`
  once, after `terminal`.
- `TestSessionTitleFallbackOnError`: a title-call error keeps the truncated
  first prompt.
- `TestSessionTitleNeedsCloudCredential`: a cloud model without a credential
  makes no title call.

Acceptance: the left column matches the mockup. Stop run cancels within one
event loop and emits `terminal`.

### BV-6: Files API, tabs, read-only code viewer

Tasks:

1. `GET /v1/sessions/{id}/files?path=` replaces `/tree`. It builds on B1-4's
   `tools.ResolvePath` + `dirwalk.Walk` and keeps B1-4's tests.
   - For a directory it returns `{kind:"dir", entries}`.
   - For a file it returns `{kind:"file", content, sha256, size, lang}`.
   - Above 2 MB it returns `{kind:"file", too_large:true, size}`.
   - It refuses binary files (a NUL byte in the first 8 KB).
   - Update the page's tree calls in the same PR.
2. Tab strip: icon, title, close, `+`. Open tabs (paths and the active tab
   only) are restored from `sessionStorage`.
3. Code view: breadcrumb, gutter, highlighted code, and a status line (language,
   Ln/Col, spaces, UTF-8). Highlighting is off above 256 KB. `.md` files render
   through `markdown.js`.
4. `web/js/highlight.js`: a tokenizer per language (Go, JS, JSON, HTML, CSS,
   YAML, TOML, shell). It returns `{type, text}` tokens; the view maps them to
   spans with text nodes.
5. Live reload: when a `tool_use_result` for `FileWrite` / file edit targets an
   open path, a clean tab reloads with a brief highlight. A dirty tab shows
   "Changed on disk — Reload / Keep mine".

RED tests first:

- `TestFilesRejectsTraversalAndSymlinkEscape` (moved from the `/tree` tests).
- `TestFilesTooLargeAndBinary`: a 2 MB + 1 byte file gives `too_large`, and a
  file with a NUL byte is refused.
- `TestFilesReturnsHash`: the `sha256` matches the content.
- `TestTreeRouteRemoved`: `/tree` returns 404.
- `highlight.test.js`: Go raw strings, comments in strings, and unterminated
  strings tokenize without throwing. `</script>` in a string stays one token.
- `tabs.test.js`: restore works when `sessionStorage` throws.

Acceptance: opening `handler.go` matches mockup B's lower center region.

### BV-7: Save

Tasks:

1. The code view becomes editable: a transparent textarea over the highlighted
   `<pre>`, with a dirty marker on the tab.
2. `PUT /v1/sessions/{id}/files?path=` with `{content, base_sha256}`:
   - 409 if the file on disk no longer matches `base_sha256`.
   - Blocked (423) while a pending or in-flight `FileWrite`/edit call in the
     current run targets the same path.
3. The permission decision goes through the same function as an agent
   `FileWrite`. Plan or read-only mode blocks; a deny rule blocks; an allow rule
   or acceptEdits/bypass saves. Otherwise the response is `needs_confirm` and
   the browser shows a confirm card: Allow once / Always allow (adds the session
   rule `FileWrite(path)` via the shared `permissions.SessionAllowRule` from
   B1-4, and the card says it also lets the agent write this file) / Cancel.
4. `PreToolUse` and `PostToolUse` hooks run as for `FileWrite`. A Pre hook can
   block; Post hook output is shown in a toast.
5. Emit a `file_saved` audit event (path, size, hash, decision source) that
   Proof Mode (B5) can include.
6. Writes go through `filewrite`'s atomic write.

RED tests first:

- `TestSaveConflictReturns409`.
- `TestSaveBlockedInPlanMode`.
- `TestSaveDenyRuleBlocks` / `TestSaveAllowRuleSavesWithoutConfirm`.
- `TestSaveAlwaysAllowAddsFileWriteRule`: a later agent `FileWrite` to that
  path emits no `permission_request`.
- `TestSavePreToolUseHookCanBlock`.
- `TestSaveBlockedWhileRunWritesSamePath`.
- `TestSaveEmitsFileSavedEvent`.
- `TestSaveOutsideWorkspaceRejected` (traversal and symlink escape).

Acceptance: every path in the permission matrix behaves the same for a user
save as for an agent `FileWrite`. A conflict never overwrites.

### BV-8a: Sub-agent events (backend)

Waits for the B1.5 agent event invariants (`assertEventInvariants`). Line the
DTOs up with Phase 25 slice 0 typed events if that has landed.

Tasks:

1. `subagent_started` / `subagent_finished`: task id, name, task description,
   model, parent, start/finish, status, error.
2. `subagent_progress`: forward the worker's `tool_use_start` / `tool_use_result`
   (tool name, short target, status).
3. `subagent_usage`: the worker's `agent.Usage` and the count of distinct paths
   in its tool calls. This needs usage plumbing from the worker loop through
   `tasks.Supervisor`.
4. `subagent_message`: mailbox traffic (from, to, summary, time). Never the
   full content by default.
5. A tasks endpoint or session-view field listing the current workers, for
   page reloads.

RED tests first:

- `TestSubagentLifecycleEvents`: a fake worker run emits started → progress →
  finished in order, and `assertEventInvariants` holds.
- `TestSubagentUsageForwarded`: worker usage reaches the session as
  `subagent_usage`.
- `TestSubagentMessageEventOmitsContent`.
- `TestCoordinatorWorkersListedInSessionView`.

### BV-8b: Agents tab (UI)

Tasks:

1. Left panel "Agents in this session": one row per worker (colour, name,
   task, status). Clicking a row selects its node.
2. Graph with a fixed layout: the root node ("Main agent", or "Coordinator" in
   coordinator mode) on top, a row of workers, and an aggregation node only
   when 2 or more workers have finished. Connectors are plain SVG paths.
   Unknown fields are omitted.
3. Header: Running pill, View logs (switches to the Transcript view filtered
   to sub-agent events), Stop all (the same `runctl` cancel as Stop run).
4. Activity log: its own store, with an "All agents" filter.
5. Sub-agent spawns also appear as nested cards in the transcript.

RED tests first:

- `agents.test.js`: no aggregation node with one finished worker; one with two.
  A node without usage renders no tokens line.
- `agentslog.test.js`: the filter keeps only the selected agent's rows.
  Feeding one fixture stream into the agents log store and the Recent-events
  store gives the same sub-agent event IDs (consistency).

Acceptance: a real coordinator run with 2 or more workers matches the
mockup's Agents region, apart from fields the backend does not have.

## Final exit gate

- Every slice is accepted with the roadmap Step Exit Check.
- The gallery and the live app at 1448 px match both mockups by eye, and the
  baselines pass `make visual-check`.
- No value on screen is invented.
- `index.html` has no inline style or script, which unblocks B4's CSP.

## Not doing

- "Add agent" / agent-type presets, a user "Add task", light theme, mobile
  layout: see BACKLOG.
- Monaco, CodeMirror or any highlighting library (they need bundling).
- Editor extras: LSP, find/replace, multiple cursors.
- TUI parity (ADR-002).
- Pixel-diffing against the mockup PNGs (their content is invented).
