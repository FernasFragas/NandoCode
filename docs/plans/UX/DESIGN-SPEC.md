# Browser UI Design Spec (BV-0)

**Date:** 2026-10-18
**Owner plan:** [`BROWSER-VISUAL-REVAMP-PLAN.md`](../BROWSER-VISUAL-REVAMP-PLAN.md)
**Sources:** `Local AI Coding Agents Dashboard.png` (canonical, "A") and `Local AI Coding Session Dashboard.png` ("B"), both 1448x1086.
**Code:** tokens in [`internal/server/web/css/tokens.css`](../../../internal/server/web/css/tokens.css), icons in [`internal/server/web/icons.svg`](../../../internal/server/web/icons.svg).

The mockups are the **visual** spec, not the functional spec. Copy their look,
density, and layout. Never copy their data: every value on screen comes from a
real event or endpoint, and a region without a backend shows an honest empty
state (see "Empty states").

## Tokens

Colours are the median of a 5x5 pixel patch in mockup A. Where A and B differ
(B renders slightly warmer), A wins. The values are exact samples by decision
(2026-10-18); tune only after a side-by-side check in the BV-1 gallery.

| Token | Value | Used for |
| --- | --- | --- |
| `--bg` | `#0b1520` | header, nav, footer |
| `--bg-center` | `#0a141d` | transcript, editor, agent canvas |
| `--bg-rail` | `#0b141f` | right status rail |
| `--surface` | `#0e1a26` | project tree |
| `--card` | `#111e2e` | thinking, tool, chip cards |
| `--card-raised` | `#142235` | tool results, code blocks, inputs |
| `--chip` | `#101c2a` | stage / retrieval / pack chips |
| `--border` | `#0d1823` | 1px dividers |
| `--primary` | `#0067fe` | New session, links, active tab underline |
| `--primary-pressed` | `#0061fc` | Allow button |
| `--primary-soft` | `#032a65` | active nav item, active session row |
| `--info` | `#006aff` | running state, info event dot |
| `--success` / `--success-soft` | `#1efb82` / `#06221e` | Connected, Completed, OK |
| `--warning` | `#ffb201` | pending permission |
| `--danger` / `--danger-soft` | `#ff181d` / `#281720` | Stop run, failed events |

Not sampled (anti-aliased or too small to sample reliably), set by eye and to be
checked in BV-1: text colours, `--agent-1..6`, syntax colours.

**Recheck first:** `--border` is almost identical to the backgrounds. It is
faithful to the mockup but dividers may disappear on some monitors.

Typography: system font stack (`--font-ui`), no web fonts (the network policy
rules out a CDN). Sizes: 11 / 12 / 13 / 15 px, brand 22 px semibold.

Spacing: 4 px grid (`--sp-1`..`--sp-6`). Radii: 4 / 6 / 8 px, pill.

## Layout

Measured at 1448 px:

```
┌──────────────────────── header 68px ────────────────────────┐
│ brand · Local server mode      Session: id ⧉  model▾  ●Connected  ■Stop run  ⚙ │
├──────────┬──────────┬──────────────────────────┬───────────┤
│ nav      │ tree     │ transcript + composer    │ right     │
│ 252px    │ 258px    │ ─── drag handle ───      │ rail      │
│          │          │ tab strip + tab content  │ 278px     │
├──────────┴──────────┴──────────────────────────┴───────────┤
│ footer 34px: ● server · http://127.0.0.1:port · ✓ nandocodego vX │
└──────────────────────────────────────────────────────────────┘
```

Decided changes from the mockup:

- **New session** appears only in the sidebar. The header slot holds Stop run and Connected.
- The center column is a **resizable split**: transcript and composer on top, tab area below. The tab area collapses to zero height when no tab is open. There is no lower "Chat" tab.
- A **composer** sits at the bottom of the transcript pane. The mockups show none.
- The bearer token arrives through the `#token=` URL fragment (BF-7), not through a header field. The settings gear keeps a paste fallback.
- Mentions are not a nav item. A file or folder row in the tree has an `@` hover action that inserts the mention into the composer.

Breakpoints:

| Width | Change |
| --- | --- |
| < 1440 px | nav collapses to a 56 px icon rail |
| < 1200 px | project tree becomes a toggle; permission popover falls back to inline |
| < 960 px | right rail becomes a drawer |

## Nav

| Item | Action |
| --- | --- |
| Chat | center shows chat; tree hidden |
| Project | tree + chat |
| Agents | opens or focuses the Agents tab |
| Transcript | full raw event log view (with search, from UI-6) |
| Tasks | focuses the Tasks section of the right rail (drawer below 960 px) |
| Diagnostics | focuses the Diagnostics section of the right rail |

## Component inventory

Each component is a DOM-free render function that returns a `{tag, attrs, children}` tree (BV-1). Its data source is listed here.

| Component | Mockup region | Data source | Slice |
| --- | --- | --- | --- |
| Header (session id + copy, model picker, connection pill, Stop run, settings) | top bar | session view, `/v1/models`, SSE state, `runctl` cancel | BV-2, BV-5 |
| Nav item / icon rail | left | static | BV-2 |
| Session row (radio, id, time, title, hover delete) | Sessions | session list (BF-6), `session_titled` | BV-5 |
| Tree row (chevron, icon, name, `@` action) | Project | files API (BV-6) | BV-2, BV-6 |
| Message row (U / AI avatar, bubble, time) | transcript | user input, `assistant_text_delta` | BV-4 |
| Thinking card (collapsible, duration) | transcript | `assistant_thinking_delta`, timing | BV-4 |
| Stage chip / retrieval chip / files chip / pack chip | transcript | `stage_timing`, `semantic_retrieval`, `prompt_pack_report` | BV-4 |
| Tool card (icon, name, status pill, duration, params, results) | transcript | `tool_use_start`, `tool_use_progress`, `tool_use_result` | BV-4 |
| Permission card (tool, target, reason, Allow / Deny / Always allow, X) | popover | `permission_request` | BV-4 |
| Composer | transcript bottom | POST message | BV-4 |
| Tab strip (icon, title, close, +) | center bottom | client state | BV-6 |
| Code view (breadcrumb, gutter, highlighted code, status line) | `handler.go` tab | files API | BV-6 |
| Save confirm card | code tab | save endpoint | BV-7 |
| Status row (label, value, dot) | Session status, Diagnostics | session view, SSE health, index status | BV-3 |
| Usage table | Usage | `usage_update`, `terminal.usage` | BV-3 |
| Todo row (radio, content, status) | Tasks (main list) | `todo_updated` (agent `TodoWrite`) | BV-3 |
| Background task row | Tasks (below todos) | `task_lifecycle` | BV-3 |
| Event row (dot, time, label) | Recent events | client event store | BV-3 |
| Worker list row | Agents left panel ("Agents in this session") | `subagent_*` | BV-8b |
| Agent node (icon, name, status pill, time, step, files, tokens) | Agents graph | `subagent_*` | BV-8b |
| Aggregation node | Agents graph | worker completion count | BV-8b |
| Activity log row (dot, time, agent, text) | Agent activity log | agents event store | BV-8b |

## Icons

Hand-drawn 24x24 line icons, stroke `currentColor`, width 1.75, in one sprite
(`icons.svg`): `plus, x, check, chevron-down, chevron-right, chevron-up, trash,
compact, chat, folder, folder-plus, file, file-plus, file-code, agents,
transcript, tasks, diagnostics, settings, copy, stop, shield, terminal, search,
circle, circle-dot, more-vertical, logs, database, alert, refresh, save, at,
panel-left, share`. Each is referenced as `icons.svg#i-<name>`.

## Empty states

- Usage before the first turn shows `—` for each row, never `0`.
- Tasks shows the agent's todo list (`TodoWrite`) first and background tasks below. If both are empty it reads "No tasks yet". "Add task" (the user appending a todo) is hidden; it is in BACKLOG.
- Index status reads "Unavailable" until the B2 index-status endpoint exists.
- Before the first worker, the Agents tab reads "No sub-agents in this session yet" and the graph is hidden.
- When a worker field is unknown, its node omits that line. It never shows a placeholder number.

## Mockup noise (do not copy)

- The footer reads "Sendace" and "Candocodego v0.4.2". Use the real server label and `version.String()`.
- "Agents" is missing from the nav in mockup B. It is present in the real nav.
- Every number, timestamp, session id, and file name is invented.
- The registry of agent *types* (Code Analysis, Test Runner…) does not exist. The panel lists this session's workers instead.
- "Skill: retrieval" on worker nodes: workers have no skill link today. Omit it.
- There are two "New session" buttons. Keep only the sidebar one.
