# Web Browser UI UX Product Plan (Agent-Ready)

**Date reviewed:** 2026-05-24  
**Implementation audit:** 2026-05-28  
**Status:** Partially implemented; next agent must first reconcile the served embedded UI with the richer root UI, then fix browser event payload mapping before adding P1 panels.  
**Scope:** Upgrade the existing Phase 21 browser page into a usable local web UI.  
**Module path:** `github.com/FernasFragas/nandocodego`  

This plan is grounded in the current repository. It intentionally builds on the
implemented `nandocodego server` HTTP/SSE surface instead of introducing a new
transport, build system, or server mode.

## Review Findings Resolved

The previous draft was not ready for an implementation agent because it assumed
infrastructure that is not present in the repo. This version removes those
assumptions:

- Do not add WebSocket transport for this work. The current server uses SSE for
  server-to-browser events and HTTP POST for browser-to-server actions.
- Do not add `github.com/gorilla/websocket`; Phase 21 intentionally avoided new
  external dependencies.
- Do not add root `--server` flags. The current entry point is the
  `nandocodego server` subcommand.
- Do not create `assets/web/*` unless the implementation also updates the
  existing embed path and tests. `internal/server/server.go` currently embeds
  `internal/server/web/index.html` because `//go:embed web/index.html` is
  resolved relative to the `internal/server` package. The richer product UI is
  currently in the repository root `web/index.html` and is not the file served
  by `nandocodego server`.
- Do not expose the slash command registry directly over HTTP. Current server
  message POSTs are user prompts, not command dispatch.
- Do not claim model switching, file tree browsing, memory panels, skills
  panels, hooks panels, or prompt inspection are already exposed over HTTP.
  Add dedicated endpoints only in the slices that need them.

## Product Goal

Turn the current minimal browser page into a first-class local web UI for common
nandocodego workflows:

- chat with the model from a browser;
- see streaming assistant output, thinking, tool calls, run notices, and
  terminal status;
- approve or deny permission requests without using the terminal;
- choose the active model from a picker;
- insert file and directory mentions without memorizing paths;
- view session status, usage, tasks, and diagnostics in panels as follow-up
  work.

The web UI is not a replacement for the TUI. It is a parallel local interface
for browser workflows. The agent loop, permission resolver, prompt packing, tool
execution, memory runner, and hooks remain server-side.

## Current Repo Baseline

Use these facts as the starting point for implementation:

| Area | Current implementation |
| --- | --- |
| Server command | `nandocodego server` in `internal/cli/server.go` |
| Default bind | `127.0.0.1:8080` |
| Server package | `internal/server` |
| Embedded UI | `internal/server/web/index.html`, embedded by `//go:embed web/index.html` in `internal/server/server.go` |
| Rich UI draft | Root `web/index.html`; currently not served by `nandocodego server` |
| Transport | Served minimal UI still uses native `EventSource`; root rich UI uses fetch-stream SSE plus HTTP writes |
| Auth | Bearer token middleware when `--token` is set |
| Non-loopback safety | Non-loopback bind is rejected unless `--token` is set |
| Rate/session limits | `internal/server/ratelimit.go` |
| Session replay | Ring buffer with `Last-Event-ID` support |
| Permission bridge | `internal/server/permission.go` with 30 second timeout |

Existing HTTP API:

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/` | Serve the embedded UI when `--no-ui` is false |
| `GET` | `/v1/health` | Health check and Ollama reachability |
| `GET` | `/v1/models` | List local models via the current LLM/model runtime path |
| `POST` | `/v1/sessions` | Create a server session |
| `GET` | `/v1/sessions/{id}` | Return a session view |
| `DELETE` | `/v1/sessions/{id}` | Stop and delete a session |
| `GET` | `/v1/sessions/{id}/events` | SSE event stream with replay |
| `POST` | `/v1/sessions/{id}/messages` | Queue one user prompt |
| `POST` | `/v1/sessions/{id}/permissions/{request_id}` | Resolve a permission request |
| `POST` | `/v1/sessions/{id}/model` | Update the session model |
| `GET` | `/v1/sessions/{id}/tree` | Return a session-scoped file tree |

Existing SSE event types emitted by the server:

```text
session_ready
heartbeat
run_started
assistant_text_delta
assistant_thinking_delta
tool_use_start
tool_use_progress
tool_use_result
hook_notice
retry_notice
llm_idle_warning
stage_timing
prompt_pack_report
permission_request
task_lifecycle
terminal
error
retrieval_route_decided
semantic_query_embed_started
semantic_stage_timing
semantic_query_embed_finished
semantic_search_finished
semantic_retrieval
semantic_skipped
llm_request_started
llm_stream_opened
first_token_received
```

Important browser auth constraint: native `EventSource` cannot send custom
`Authorization` headers. The served minimal UI currently uses `EventSource`, so
protected SSE does not work there. The root rich UI uses `fetch(...)` and parses
the SSE stream manually so the optional bearer token can be sent as a header.
Keep the fetch-stream approach unless a later auth design replaces it
deliberately.

## Implementation Audit

Current code has progressed beyond the original plan baseline:

| Area | Status | Evidence / notes |
| --- | --- | --- |
| HTTP/SSE server baseline | Implemented | `internal/server` exposes sessions, message POST, SSE events, model list, permissions, replay, auth, rate limiting, and embedded UI. |
| Served browser UI | Minimal only | `internal/server/web/index.html` is the embedded UI and still shows the older manual session/EventSource page. |
| Root rich browser shell | Implemented but unserved | Root `web/index.html` has header, sidebar, main chat area, status bar, modal shell, model picker, mentions panel, and fetch-stream SSE, but is not embedded by the server. |
| Session lifecycle UI | Implemented only in unserved root UI | Root UI creates a session on load and reconnects the SSE stream with exponential backoff; served UI requires manual session creation and manual SSE connect. |
| Chat input | Implemented in both UIs with different fidelity | Served UI posts prompts without `message_id`; root UI posts prompts with `message_id`. |
| Permission modal UI | Implemented only in unserved root UI and blocked by event payload mismatch | Root UI posts `allow`, `deny`, and `always_allow` to the existing permission endpoint, but reads request fields from the wrong object. Served UI has no permission modal. |
| Model picker | Backend implemented; browser picker unserved | Backend `POST /v1/sessions/{id}/model` exists. Root browser picker exists but is not served. |
| Mention helper | Backend implemented; browser helper unserved; backend safety caveat remains | Backend `GET /v1/sessions/{id}/tree` exists. Root UI autocomplete/panel exists but is not served. Backend currently uses manual `filepath.WalkDir`; refactor to `tools.ResolvePath` and `dirwalk.Walk` before treating this as complete. |
| Status/tasks/diagnostics from events | Partially implemented only in unserved root UI | Root UI has status fields, but some event shape assumptions are wrong and transcript search is still missing. |
| Security headers | Implemented, tests still needed | CSP, frame denial, nosniff, referrer policy, permissions policy, and no-store API headers are set in `securityHeaders`. |
| P1 management panels | Not implemented | Memory, Skills, Hooks, Permissions table, Tasks endpoint/panel, Trace/Cost endpoints, and Prompt Inspector remain future work. |

Critical correctness gap:

- The server is not serving the richer browser UI described by most of this
  plan. Either move the root `web/index.html` into `internal/server/web/index.html`
  or update the embed layout deliberately with route/tests. Do this before
  judging browser UX slices as complete.
- Server SSE frames encode a `SessionEvent` envelope as JSON:
  `{"id":"...","type":"assistant_text_delta","session_id":"...","data":{...}}`.
  Root browser event handling switches on `msg.type` correctly, but many
  renderers read payload fields from the top level (`msg.delta`,
  `msg.tool_call_id`, `msg.request_id`, `msg.usage`). They must read from
  `msg.data` instead.
- Expected mappings include:
  - `assistant_text_delta`: `msg.data.content`
  - `assistant_thinking_delta`: `msg.data.thinking`
  - `tool_use_start`: `msg.data.id`, `msg.data.name`, `msg.data.input`
  - `tool_use_progress`: `msg.data.id`, `msg.data.data`
  - `tool_use_result`: `msg.data.id`, `msg.data.result`, `msg.data.error`
  - `permission_request`: `msg.data.request_id`, `msg.data.tool_name`,
    `msg.data.target`, `msg.data.reason`
  - `stage_timing`: `msg.data.stage`, `msg.data.duration_ms`
  - `prompt_pack_report`: `msg.data.estimated_included`,
    `msg.data.estimated_skipped`
  - `task_lifecycle`: derive task counts from task status changes, not
    nonexistent `count` or `delta` fields.
  - `terminal`: `msg.data.reason`, `msg.data.detail`, `msg.data.usage`
- `agent.Usage` currently serializes as `PromptEvalCount`, `EvalCount`,
  `TotalDuration`, `Turns`, `ToolCalls`, and `DoneReason`. The browser must use
  those names or the server should add explicit JSON DTO fields.

## UX Requirements

### P0: Must Have

| ID | Requirement | Current status | Implementation notes |
| --- | --- | --- | --- |
| P0-1 | Auto-create a session on page load | Implemented in root UI, not served | `POST /v1/sessions`; keep session id in memory, not as source-of-truth localStorage |
| P0-2 | Chat input and transcript | Partially implemented | Served UI has basic send/log only. Root UI has richer transcript but must be served and must read event payloads from `msg.data`. |
| P0-3 | Streaming assistant output | Partially implemented | Render `assistant_text_delta` from `msg.data.content`; finalize on `terminal` from `msg.data` |
| P0-4 | Thinking and tool panels | Partially implemented in root UI, not served | Render `assistant_thinking_delta`, `tool_use_*`, `hook_notice`, `retry_notice`, `llm_idle_warning`, `stage_timing` from `msg.data` |
| P0-5 | Permission modal | Implemented in root UI, not served; needs event payload fix | Render `permission_request` from `msg.data`; POST `allow`, `deny`, or `always_allow` |
| P0-6 | Model picker | Backend implemented; UI unserved | Lists `/v1/models`; updates `/v1/sessions/{id}/model` |
| P0-7 | Mention helper | Endpoint implemented; UI unserved; backend should be refactored for plan-level safety | Tree endpoint exists; refactor to `tools.ResolvePath` + `dirwalk.Walk`; insert plain `@path` text |
| P0-8 | Status bar | Partially implemented in root UI, not served | Use session view, run events, terminal usage, prompt pack report, and task lifecycle events from `msg.data` |

### P1: Should Have

| ID | Requirement | Implementation notes |
| --- | --- | --- |
| P1-1 | Session history and clear | Server currently has in-memory sessions only; do not promise restart persistence |
| P1-2 | Prompt inspector | Add dedicated prompt metadata endpoints only if prompt dump data is available server-side |
| P1-3 | Memory panel | Use `internal/memory`; add endpoints for list/show/promote with path checks |
| P1-4 | Skills panel | Use `internal/skills`; list/show first, invoke only through existing agent/tool paths |
| P1-5 | Hooks panel | Use current hook snapshot; reload requires explicit confirmation |
| P1-6 | Permissions table | Read and update session rules without bypassing `permissions.Resolve` |
| P1-7 | Tasks, trace, and cost panels | Prefer existing session events and `lastTerminal`; add endpoints only for missing data |

### P2: Defer

- Mobile-first responsive layout.
- Theme engine beyond a basic light/dark CSS toggle.
- MCP server manager.
- Model pull progress UI.
- Browser token flow for protected SSE.
- Multi-user collaboration.
- Cloud hosting.

## Architecture Rules

- Keep the agent loop server-side.
- Keep prompt packing in `contextpack.BuildCurrentTurnPrompt(...)`.
- Keep permission decisions flowing through the existing HTTP permission broker.
- Prefer stdlib HTTP and vanilla browser APIs. Do not add npm, TypeScript, or a
  frontend build step for this phase.
- Keep frontend state derived from server responses and SSE events. Browser
  storage may cache convenience values, but it must not be the source of truth.
- Do not expose raw prompt dumps, API keys, OAuth tokens, environment variables,
  or unrestricted filesystem data.
- Add backend endpoints under `/v1/...` to match the existing API shape.
- Preserve `GET /v1/sessions/{id}/events` replay semantics.

## P0 API Additions Status

Both P0 backend additions now exist in code. Keep this section as the contract
for fixing gaps and avoiding incompatible API drift.

### Set Session Model

Status: implemented.

```text
POST /v1/sessions/{id}/model
Content-Type: application/json

{"model":"qwen3"}
```

Behavior:

- Return `404` if the session does not exist.
- Return `409` if a run is active.
- Trim and validate the model name is non-empty.
- Validate against `modelRuntime.ListLocal(...)` when `modelRuntime` exists;
  otherwise validate against `client.ListModels(...)`.
- Update `Session.appState.ActiveModel`.
- If using `modelruntime.Service` and provider/base URL resolution is available,
  update `LLMProvider` and `LLMBaseURL` consistently with existing
  `applyModelSwitch(...)` behavior.
- Return the updated session view or a small JSON object:
  `{"model":"...","provider":"...","base_url":"..."}`.

Tests:

- Valid model updates session state.
- Unknown model should return `400`.
- Active run returns `409`.
- Missing session returns `404`.

### Safe File Tree

Status: implemented, but not yet aligned with the planned safety implementation.
The current handler uses manual path prefix checks and `filepath.WalkDir`.
Before considering this endpoint complete, refactor it to use `tools.ResolvePath`
and `internal/tools/dirwalk.Walk` as specified below.

```text
GET /v1/sessions/{id}/tree?path=.&depth=2
```

Behavior:

- Return `404` if the session does not exist.
- Resolve `path` with `tools.ResolvePath` and the session `ToolContext`.
- Reject paths outside the working directory and additional allowed dirs.
- Use `internal/tools/dirwalk.Walk` for traversal and default excludes.
- Cap `depth` to a small value, default `2`, maximum `4`.
- Cap files per response, default `300`, maximum `500`.
- Return entries plus walk stats so the UI can display truncation:

```json
{
  "root": ".",
  "entries": [
    {"path":"docs","name":"docs","is_dir":true},
    {"path":"README.md","name":"README.md","is_dir":false}
  ],
  "stats": {"truncated": false, "reason": "", "source": "git"}
}
```

Tests:

- Current working directory returns JSON.
- `..` traversal is rejected.
- `.git` and default excludes are omitted.
- Depth and file caps are enforced.

## Implementation Slices

Each slice must leave the repo testable. Do not mix later panels into P0 slices.

Recommended next implementation order:

1. Reconcile the embedded UI first: either move the root rich `web/index.html`
   into `internal/server/web/index.html`, or intentionally change the embed path
   and tests so `nandocodego server` serves the rich UI.
2. Fix browser SSE payload handling in UI-2/UI-3/UI-6 so the served rich UI
   reads `SessionEvent.data` correctly.
3. Refactor the tree endpoint to use `tools.ResolvePath` and `dirwalk.Walk`.
4. Add security header tests and close remaining accessibility gaps.
5. Start P1 sidebar panels one panel at a time.

### Slice UI-0: Serve the Rich Browser UI

**Status:** Not complete; required before UI-1 through UI-6 can be considered
served product functionality.

**Goal:** Make the richer product UI the actual page served by
`nandocodego server`.

Files:

- `internal/server/web/index.html`
- root `web/index.html`
- `internal/server/server.go` only if changing the embed layout
- `internal/server/handler_test.go` or a new route/embed test

Tasks:

- Decide whether the canonical UI file lives under `internal/server/web/` or
  whether `server.go` should embed another directory.
- Ensure `GET /` serves the rich browser shell, not the old manual
  EventSource/debug page.
- Preserve the fetch-stream SSE implementation so bearer-token auth can work.
- Remove or clearly mark any stale duplicate UI file so future edits do not land
  in the wrong place.
- Add a small server test that checks the served HTML contains a stable marker
  from the rich UI and does not contain the old manual `EventSource` UI.

Acceptance:

- `go run ./cmd/nandocodego server --bind 127.0.0.1 --port 8080` serves the rich
  app shell at `/`.
- The served page uses fetch-stream SSE, not native `EventSource`.
- `go test ./internal/server ./internal/cli` passes.

Agent prompt:

```text
Fix Slice UI-0 from docs/WEB-UI-UX-PRODUCT-PLAN.md. The rich UI currently lives
at root web/index.html, but internal/server/server.go embeds
internal/server/web/index.html. Make nandocodego server serve the rich UI,
preserve fetch-stream SSE for bearer-token auth, and add a route/embed test so
this cannot drift again. Run go test ./internal/server ./internal/cli.
```

### Slice UI-1: Browser Shell and Session Lifecycle

**Status:** Implemented in root `web/index.html`, but not currently served.

**Goal:** Replace the current raw event log page with a stable app shell that
creates a session and connects to SSE.

Files:

- `internal/server/web/index.html` after UI-0 reconciliation
- root `web/index.html` only if it remains the canonical source
- `internal/server/handler_test.go` only if route behavior changes

Tasks:

- Build a header, left nav, main chat area, right details panel, and status bar
  using plain HTML/CSS/JS.
- On load, call `POST /v1/sessions`.
- Connect to `/v1/sessions/{id}/events` using `fetch` streaming, preserving the
  ability to send an optional bearer token header.
- Show connection state: creating session, connected, reconnecting, failed.
- Keep the session id in JS memory. If caching it for convenience, verify it
  still exists with `GET /v1/sessions/{id}` before reuse.

Acceptance:

- `go run ./cmd/nandocodego server --bind 127.0.0.1 --port 8080` serves the UI.
- Opening `/` creates a session and receives `session_ready`.
- Existing server tests still pass.

Agent prompt:

```text
Audit Slice UI-1 from docs/WEB-UI-UX-PRODUCT-PLAN.md after completing UI-0. The
shell and session lifecycle are implemented in the rich UI, but must be the
served embedded UI before this slice is complete. Preserve fetch-stream SSE, no
WebSocket, npm, or new dependencies. Run go test ./internal/server ./internal/cli.
```

### Slice UI-2: Chat Transcript and Event Rendering

**Status:** Partially implemented; next fix is required here.

**Goal:** Users can send prompts and read structured streaming responses.

Files:

- `internal/server/web/index.html` after UI-0 reconciliation

Tasks:

- Normalize SSE handling first:
  ```js
  const payload = msg.data || {};
  ```
  Switch on `msg.type`, but pass `payload` to renderers.
- Add textarea input with Enter to send and Shift+Enter for newline.
- Send prompts with `POST /v1/sessions/{id}/messages`.
- Generate a client `message_id` for duplicate-safe retry.
- Render user messages immediately after a successful `202`.
- Render assistant text from `assistant_text_delta` using `payload.content`.
- Render thinking blocks from `assistant_thinking_delta` using
  `payload.thinking`.
- Render tool cards from `tool_use_start`, `tool_use_progress`, and
  `tool_use_result` using `payload.id`, `payload.name`, `payload.input`,
  `payload.data`, `payload.result`, and `payload.error`.
- Render notices for hooks, retries, idle warnings, stage timings, prompt pack
  reports, and errors.
- Finalize the turn on `terminal` and update usage from `payload.usage`.

Acceptance:

- Prompt submission returns `202` and disables input while the run is active.
- Streaming text updates one assistant message rather than appending raw log
  lines.
- Tool and error events are visible without breaking the chat flow.

Agent prompt:

```text
Fix Slice UI-2 from docs/WEB-UI-UX-PRODUCT-PLAN.md. After UI-0, the served rich
UI must normalize SessionEvent payloads and read event data from msg.data.
Update assistant deltas, thinking, tool cards, notices, errors, and terminal
usage accordingly. Run go test ./internal/server.
```

### Slice UI-3: Permission Modal

**Status:** Implemented in root UI shape, but not served; blocked by same
`msg.data` payload fix.

**Goal:** Browser users can resolve tool permission requests.

Files:

- `internal/server/web/index.html` after UI-0 reconciliation
- `internal/server/permission_test.go` if backend behavior changes

Tasks:

- On `permission_request`, open a modal showing `payload.tool_name`,
  `payload.target`, and `payload.reason`.
- Provide Allow, Deny, and Always Allow actions.
- POST `{"decision":"allow"}`, `{"decision":"deny"}`, or
  `{"decision":"always_allow"}` to
  `/v1/sessions/{id}/permissions/{request_id}`.
- Treat Escape as Deny only after the user has focused the modal.
- Show timeout/resolve state if the server emits a terminal/error afterward.

Acceptance:

- Permission request blocks visible UI progress until the user responds or the
  server timeout fires.
- Always Allow maps to the existing server `always_allow` decision.
- No new permission modes are introduced.

Agent prompt:

```text
Fix Slice UI-3 from docs/WEB-UI-UX-PRODUCT-PLAN.md. After UI-0, the served rich
UI permission modal must read permission_request details from msg.data before
posting allow, deny, or always_allow to the existing permission endpoint. Do not
change the permission resolver semantics. Run go test ./internal/server.
```

### Slice UI-4: Model Picker

**Status:** Backend implemented; browser picker exists in root UI but is not
currently served.

**Goal:** Users can see available local models and switch the session model.

Files:

- `internal/server/types.go`
- `internal/server/handler.go`
- `internal/server/server.go`
- `internal/server/handler_test.go`
- `internal/server/web/index.html` after UI-0 reconciliation

Tasks:

- Current code already has `ModelUpdateRequest` and a handler for
  `POST /v1/sessions/{id}/model`.
- Current code already routes the endpoint in `sessionRoutes`.
- Validate model names as described in "P0 API Additions".
- Add a model picker in the header that fetches `/v1/models`.
- Show the active session model from the session view or update response.
- Disable switching while a run is active.

Acceptance:

- Model list is populated from `/v1/models`.
- Choosing a valid model updates the session state.
- Invalid model and active-run cases return clear errors.
- `go test ./internal/server` passes.

Agent prompt:

```text
Audit Slice UI-4 from docs/WEB-UI-UX-PRODUCT-PLAN.md after UI-0. The
session-scoped model endpoint exists and the rich UI has a header picker, but
the picker must be served before this slice is complete. Verify tests cover
valid, missing, unknown, and active-run cases; fix only gaps. Do not add model
pull UI in this slice. Run go test ./internal/server.
```

### Slice UI-5: Mentions and Safe File Tree

**Status:** Endpoint implemented; root UI helper exists but is not served;
backend safety implementation must be refactored before this is complete.

**Goal:** Users can insert `@path` mentions from a safe browser picker.

Files:

- `internal/server/types.go`
- `internal/server/handler.go`
- `internal/server/server.go`
- `internal/server/tree_test.go`
- `internal/server/web/index.html` after UI-0 reconciliation

Tasks:

- Current code has `GET /v1/sessions/{id}/tree`.
- Refactor the handler to use `tools.ResolvePath` and `dirwalk.Walk`; do not
  keep the raw `filepath.WalkDir` implementation as the final version.
- Return flat entries plus stats. The UI can render a tree from paths.
- In the chat input, typing `@` opens an autocomplete list from the tree data.
- Selecting an entry inserts plain `@path` text.
- Add a left nav "Mentions" panel that shows the same tree and inserts selected
  paths into the input.

Acceptance:

- `@` autocomplete works for files and directories.
- Path traversal and excluded directories are blocked by tests.
- Sending a prompt with inserted `@path` uses the existing prompt packing path.

Agent prompt:

```text
Fix Slice UI-5 from docs/WEB-UI-UX-PRODUCT-PLAN.md. The tree endpoint exists and
the rich UI has mentions support, but UI-0 must make it served. Refactor the
backend to use tools.ResolvePath and dirwalk.Walk instead of raw
filepath.WalkDir. Keep plain @path insertion behavior and update traversal/cap
tests. Run go test ./internal/server.
```

### Slice UI-6: Status, Tasks, and Diagnostics From Existing Events

**Status:** Partially implemented in root UI, but not served; event payload
shape must be corrected and transcript search is missing.

**Goal:** Make the UI useful for long runs without adding management endpoints.

Files:

- `internal/server/web/index.html` after UI-0 reconciliation

Tasks:

- Show active run state from `run_started` and `terminal`.
- Show token usage from `terminal` payload usage fields.
- Show prompt packing information from `prompt_pack_report` payload fields.
- Show stage timing timeline from `stage_timing` payload fields.
- Show task changes from `task_lifecycle` payload fields.
- Add transcript search in the browser.

Acceptance:

- Long-running sessions show visible progress without checking terminal logs.
- The status bar reflects active run, latest terminal reason, token usage, and
  task count.
- No backend changes are needed for this slice.

Agent prompt:

```text
Fix Slice UI-6 from docs/WEB-UI-UX-PRODUCT-PLAN.md. After UI-0, status, task,
prompt packing, and stage timing UI must read fields from msg.data and match the
server's emitted event shapes. Add transcript search. Do not add backend
endpoints in this slice. Run go test ./internal/server.
```

### Slice UI-7: P1 Sidebar Panels

**Status:** Not implemented.

**Goal:** Add management panels after P0 browser chat is stable.

Files depend on the panel. Prefer small endpoint groups instead of a generic
command-dispatch endpoint.

Panel endpoint guidance:

| Panel | Endpoint shape | Backend source |
| --- | --- | --- |
| Memory | `GET /v1/sessions/{id}/memory`, `GET /v1/sessions/{id}/memory/{name}`, optional promote endpoint | `internal/memory` |
| Skills | `GET /v1/skills`, `GET /v1/skills/{name}` | `internal/skills` |
| Hooks | `GET /v1/hooks`, optional reload with `{"confirm":"yes"}` | current hook snapshot/reload path |
| Permissions | `GET /v1/sessions/{id}/permissions`, allow/deny session-rule endpoints | `internal/permissions`, `Session.permRules` |
| Tasks | `GET /v1/sessions/{id}/tasks`, optional stop endpoint | `state.App.Tasks`, `tasks.Supervisor` |
| Trace/cost | Start with event-derived data; add endpoints only if event data is insufficient | terminal usage and observability data |
| Prompt inspector | Add only after confirming prompt dump data is retained server-side | `agent/prompt_dump` and session state |

Acceptance:

- Each panel ships with its own backend tests.
- Each endpoint returns `404` for missing sessions and safe errors for missing
  files/items.
- No endpoint returns secrets, env vars, or full prompt content by default.

Agent prompt:

```text
Implement one P1 sidebar panel from Slice UI-7 in docs/WEB-UI-UX-PRODUCT-PLAN.md.
Do not implement a generic slash-command HTTP bridge. Add only the endpoint(s)
needed for the selected panel, with tests, and update the served browser UI for
that panel. Run the relevant go test command plus go test ./internal/server.
```

### Slice UI-8: Accessibility, Security Headers, and Final Hardening

**Status:** Partially implemented.

**Goal:** Finish the browser UI without weakening the local-server security
model.

Files:

- `internal/server/server.go`
- `internal/server/auth_test.go` or new server security tests
- `internal/server/web/index.html` after UI-0 reconciliation

Tasks:

- Security headers are implemented in `securityHeaders`; add tests for:
  `Content-Security-Policy`, `X-Frame-Options`, and `Referrer-Policy`.
- Keep CSP compatible with the chosen frontend shape. If inline script/style is
  retained, document the tradeoff in a comment or convert to embedded static
  files and update `//go:embed`.
- Add or verify keyboard support for core actions: focus input, send, close
  modal, open model picker, open mentions panel, search transcript.
- Add ARIA roles for navigation, main chat, dialogs, status, and live updates.
- Respect `prefers-reduced-motion`.
- Run the verification commands below.

Acceptance:

- Security header tests pass.
- Permission modal is keyboard accessible.
- Text does not overlap at common desktop widths.
- Verification commands pass or failures are documented.

Agent prompt:

```text
Fix Slice UI-8 from docs/WEB-UI-UX-PRODUCT-PLAN.md. Security headers already
exist; add tests for them, then close keyboard accessibility, ARIA, and
reduced-motion gaps in the served browser UI. Keep the existing localhost/token
safety model intact. Run go test ./..., go test -race ./internal/server/...,
tools/check-allowed-deps.sh, and tools/check-network-policy.sh.
```

## Final Exit Gate

Run:

```sh
go test ./internal/server ./internal/cli
go test ./...
go test -race ./internal/server/...
tools/check-allowed-deps.sh
tools/check-network-policy.sh
```

Manual smoke:

```sh
go run ./cmd/nandocodego server --bind 127.0.0.1 --port 8080
```

Then open `http://127.0.0.1:8080/` and verify:

- UI creates a session and connects to SSE.
- Chat prompt sends and streams assistant output.
- Thinking, tool, notice, error, and terminal events render cleanly.
- Permission requests can be allowed or denied from the modal.
- Model picker lists local models and can switch when no run is active.
- Mention autocomplete inserts `@path` tokens.
- Status bar reflects run state and terminal usage.
- Browser refresh does not corrupt server session state.

## Forbidden

- Do not add WebSocket for this plan.
- Do not add npm, TypeScript, React, or a frontend build step.
- Do not add new external Go dependencies unless a later decision record
  explicitly approves them.
- Do not bind to `0.0.0.0` by default.
- Do not bypass `permissions.Resolve`.
- Do not expose API keys, OAuth tokens, env vars, unrestricted files, or full
  prompt dumps.
- Do not treat slash commands posted to `/messages` as administrative commands.
- Do not make browser localStorage the source of truth for session state.

## Notes For Future Agents

- The route and event names in this document are current as of 2026-05-24.
  If code changes first, update this plan before implementing a dependent slice.
- Prefer narrow, tested endpoint additions over broad RPC abstractions.
- When in doubt, preserve the existing Phase 21 server contract and enhance the
  browser client around it.
