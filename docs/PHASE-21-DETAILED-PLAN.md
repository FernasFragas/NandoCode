# Phase 21 Detailed Plan - Web Interface and HTTP API

Date: 2026-05-07
Status: Complete 2026-05-19 (automated, security, Docker runtime, and live API validation passed)
Source plans and references:

- `.codex/go-ollama-plan-AGENTS.md`
- `.codex/go-ollama-plan-HUMANS.md`
- `docs/PHASE-LOG.md`
- `docs/PROJECT-STATUS-AND-ONBOARDING.md`
- `docs/PHASE-8-DETAILED-PLAN.md`
- `docs/PHASE-9-DETAILED-PLAN.md`
- `docs/PHASE-18-DETAILED-PLAN.md`
- `docs/PHASE-19-DETAILED-PLAN.md`
- `docs/PHASE-20-DETAILED-PLAN.md`
- `book/ch04-api-layer.md`
- `book/ch05-agent-loop.md`
- `book/ch06-tools.md`
- `book/ch16-remote.md`
- `.codex/agent-context/learnings-memory.md`

## Roadmap Placement

Phase 21 is required after Phase 22 and before Phase 17 and Phase 18 because Phase 25 remote/bridge mode is required for v0.1 and depends on server mode. Phase 22 must land first so the local TUI has the run-state visibility, render performance, modal correctness, and activity display needed before those interaction patterns are reused or mirrored by server/remote workflows.

Phase 17 and Phase 18 are the final release-packaging and hardening phases; do not start them while Phase 21 is unimplemented.

Some baseline references in this document mention Phase 17/18 as earlier numbered phases. Treat those as historical planning context only. The current implementation order is documented in `docs/PHASE-LOG.md` and `docs/PROJECT-STATUS-AND-ONBOARDING.md`.

## Goal

Phase 21 adds a `nandocodego server` subcommand that exposes the existing agent loop over HTTP with Server-Sent Events (SSE) for streaming output and HTTP POST for message input. The result is a running HTTP service on port 8080 (default) that any browser, IDE extension, remote container, or automated pipeline can consume without a local terminal.

The Dockerfile already exposes port 8080 and `docker-compose.yml` already references an HTTP service shape. Phase 21 fulfils that promise. The agent loop, permission system, memory injection, hooks, and tool execution remain unchanged; Phase 21 wraps them in a transport layer.

The user-visible goal is that a developer can run `nandocodego server` in a Docker container, open a browser pointing at `http://localhost:8080`, send messages, see streaming tokens, and approve or deny tool permission requests — all without a local terminal.

## Implementation Status (2026-05-18)

Implemented in code:

- `internal/server` package and session/event transport.
- `nandocodego server` CLI command and root registration.
- SSE stream with heartbeat and replay, message POST, permission POST, health/models endpoints.
- Prompt/context parity wiring through `contextpack.BuildCurrentTurnPrompt(...)`.
- Optional `message_id` dedup and 409 on concurrent runs.
- Embedded `web/index.html` UI.
- Smoke script: `tools/smoke-server.sh`.
- Unit tests for server package and CLI server command wiring.

Validated by automated checks:

- `go test ./internal/server ./internal/cli`
- `go test -race ./internal/server/...`
- `go test ./...`
- `tools/check-allowed-deps.sh`
- `tools/check-network-policy.sh`

Pending manual/live checks:

- Optional interactive browser walk-through on host machine (the API and embedded UI routes are already validated via container/runtime checks).

## Checklist Resolution (2026-05-19)

The checkbox blocks below are retained as historical planning artifacts. Current resolution state:

Completed and validated in this branch:

- Server package, handlers, session lifecycle, SSE, auth, rate limiting, permission broker, and CLI server command.
- Current-turn prompt packing parity path via `contextpack.BuildCurrentTurnPrompt(...)`.
- Optional `message_id` dedup, 409 run-conflict behavior, Last-Event-ID replay support, and structured evidence-too-large handling.
- Embedded UI and CI-style smoke script.
- Automated checks:
  - `go test ./internal/server ./internal/cli`
  - `go test -race ./internal/server/...`
  - `go test -count=3 -race ./internal/server/...`
  - `go test ./...`
  - `go vet ./...`
  - `tools/check-allowed-deps.sh`
  - `tools/check-network-policy.sh`

Previously blocked items now resolved (2026-05-19):

- `gosec ./internal/server/...` ran successfully (`0 issues`) using local `GOCACHE`.
- Docker image build and runtime validation succeeded after fixing Dockerfile builder image/dependency steps.
- In-container live API validation succeeded:
  - `GET /v1/health` returned `{"status":"ok","ollama":"reachable"}`
  - `POST /v1/sessions` returned a session id
  - `POST /v1/sessions/{id}/messages` returned `{"queued":true}`
  - `DELETE /v1/sessions/{id}` returned 204
  - subsequent `GET /v1/sessions/{id}` returned 404
  - `GET /` returned 200 with HTML
  - `--no-ui` container returned 404 for `GET /`

Deliverables:

- `internal/server` package with HTTP server, session lifecycle, SSE writer, auth middleware, rate limiter, and HTTP permission broker.
- `internal/cli/server.go` defining the `server` subcommand and its flags.
- `web/index.html` — a single-file minimal UI embedded via `embed.FS`, served at `GET /`.
- `internal/cli/root.go` wired to register the `server` subcommand through `newServerCmd(...)`.
- All existing agent abstractions (memory runner, hooks dispatcher, permission resolver, tool registry) reused without modification.
- No new external dependencies: stdlib `net/http`, `encoding/json`, `sync`, and `context` only.
- `tools/check-network-policy.sh` updated only if implementation adds policy-relevant patterns. The current checker scans hardcoded URLs, not `net.Listen`; do not document a false existing violation.
- `go test -race ./internal/server/...` passes.
- Phase log update.

## Definition Of Success

The Phase 21 exit gate is a three-step manual flow:

1. Build and start the server:
   ```sh
   go build -o bin/nandocodego ./cmd/nandocodego
   ./bin/nandocodego server --bind 127.0.0.1 --port 8080
   ```
2. Interact via curl:
   ```sh
   SESSION=$(curl -s -X POST http://localhost:8080/v1/sessions | jq -r .session_id)
   curl -N "http://localhost:8080/v1/sessions/$SESSION/events" &
   curl -s -X POST "http://localhost:8080/v1/sessions/$SESSION/messages" \
        -H 'Content-Type: application/json' \
        -d '{"prompt": "What is 2+2?"}'
   ```
3. Confirm that SSE events stream back, including `assistant_text_delta` events and a final `terminal` event. Confirm that the curl in step 2 returned 202 Accepted.

This exit gate must work with the default configured Ollama endpoint and without any network destination other than Ollama and the local loopback interface.

## Baseline Analysis From Implemented Phases

### Phase 0 - Security and Supply Chain

Implemented: SECURITY.md, dependency allowlist, network policy checker, CI baseline.

Phase 21 implications:

- Server mode creates a new attack surface. When `--bind` is a non-loopback address, bearer token authentication becomes mandatory.
- The current network policy checker scans hardcoded HTTP(S) endpoints, not `net.Listen` or `net.Dial`. Phase 21 should add listen-surface checks only if they are implemented intentionally and narrowly. Do not assume the current script already enforces listen-call policy.
- The rate limiter and concurrent-session cap exist to limit resource exhaustion from malicious or runaway clients. These are security controls, not just performance controls.
- No new external dependencies may be introduced. SSE is plain HTTP; rate limiting is hand-rolled with `sync.Map` and `time.Ticker`. Bearer token generation uses `crypto/rand`, which is already stdlib.
- Memory contents, tool inputs, tool outputs, permission decisions, and conversation text must never appear in HTTP response headers or access logs. Only session IDs, status codes, and latency counters may be logged at INFO.

### Phase 1 - CLI, Paths, Logging, Scaffold

Implemented: Cobra CLI, `internal/paths`, `internal/logging`, XDG path helpers.

Phase 21 implications:

- Add `server` as a new Cobra subcommand in `internal/cli/server.go`. Flags: `--bind string` (default `127.0.0.1`), `--port int` (default `8080`), `--token string` (default empty), `--no-ui` (skip serving `web/index.html`).
- Reuse `internal/logging` for structured slog output. Access logs at DEBUG level, startup at INFO.
- `paths.DataDir()` or a new `paths.ServerDir()` can store a persistent server config file if Phase 21 introduces one; for now, all server config comes from flags only.

### Phase 2 - LLM Client

Implemented: provider-neutral `llm.Client`, streaming `Chat`, model list, retry/watchdog.

Phase 21 implications:

- The server's session goroutine calls the agent runner exactly as the REPL does. No new LLM client surface is needed.
- The `GET /v1/models` endpoint proxies `llm.Client.ListModels` and must respect the same Ollama URL that the session uses.
- Streaming LLM tokens arrive as `agent.AssistantTextDelta` events. The SSE writer maps them to `event: assistant_text_delta` frames. The mapping is one-to-one and must not buffer tokens.

### Phase 3 - Tool Interface and Starter Tools

Implemented: `tools.Tool`, `tools.Context`, Bash/FileRead/FileWrite, path safety, registry.

Phase 21 implications:

- Tool execution in server mode is identical to REPL mode. The session goroutine constructs a `tools.Context` from the session's working directory and passes it to the agent.
- Permission requests that arise from tool execution must be forwarded as SSE `permission_request` events, not as interactive terminal prompts. The `PermissionBroker` pattern from the TUI is adapted to an HTTP channel in `internal/server/permission.go`.
- The memory directory, if configured, is still added to `tools.Context.AdditionalWorkingDirs` for server sessions.

### Phase 4 - Agent Loop

Implemented: `agent.Run(ctx, input) <-chan agent.Event`, event channel, terminal events, usage.

Phase 21 implications:

- The session goroutine consumes `<-chan agent.Event` and forwards every event to the SSE connection. The SSE writer must flush after every event to guarantee per-token delivery.
- `agent.Terminal` carries `Conversation []llm.Message`. The server stores this in the session so that subsequent POST messages append to the history.
- Agent cancellation on `DELETE /v1/sessions/{id}` calls the session's `cancelFunc`, which propagates through the agent's `context.Context`.

### Phase 5 - Permission System

Implemented: canonical permission modes, source-tagged rules, central resolver, TUI prompt callback, fail-closed behavior.

Phase 21 implications:

- The HTTP permission broker replaces the TUI `PermissionBroker`. When the resolver calls the "ask" callback, the HTTP broker emits a `permission_request` SSE event and blocks on a `chan permissionDecision` with a 30-second timeout. Auto-deny fires on timeout.
- The `permissionDecision` type and the `always_allow` option are directly analogous to the TUI `permissionResolvedMsg`. Phase 21 must not add new permission modes; it wires existing modes to an HTTP transport.
- In server mode with `--token`, every permission decision endpoint also requires the bearer token. An unauthenticated request to `/v1/sessions/{id}/permissions/{req_id}` returns 401.

### Phase 6 - State Layer

Implemented: `bootstrap.State`, `state.Store[state.App]`, `state.App.Messages`, `state.App.ToolSettings`, reactive store.

Phase 21 implications:

- Each session has its own `state.Store[state.App]`. Sessions are isolated; state from one session does not bleed into another.
- The server does not use a global `state.Store`. The bootstrap snapshot is read once at server startup for global configuration (Ollama URL, model, permission mode), then each session derives a local app state.
- `state.App.ActiveRun` guards concurrent run prevention within a session. A POST to `/v1/sessions/{id}/messages` while a run is already active returns 409 Conflict.

### Phase 7 - Bubble Tea TUI and REPL

Implemented: TUI model, Bubble Tea program, permission modal, transcript rendering, slash commands, Vim modes.

Phase 21 implications:

- The server runs without Bubble Tea. There is no `tea.Program`. The session goroutine is a plain Go goroutine that calls the memory runner, which calls the agent runner.
- The `AgentRunner` interface already defined in `internal/tui/input.go` (or equivalent) is reused unchanged. The server passes the same memory-wrapped runner that the REPL uses.
- Slash commands are not available in server mode. `/help`, `/clear`, `/model`, and other REPL commands are TUI-only. If the client POSTs a prompt beginning with `/`, the server treats it as a plain text prompt (not a command) and forwards it to the agent.

### Phase 8 - Memory

Implemented: `internal/memory` with scan, recall, prompt injection, runner wrapper, pending extraction.

Phase 21 implications:

- Memory injection works identically in server mode. The session's runner is the memory runner wrapping the agent runner. The scope root is derived from the session's working directory at session creation time.
- The memory directory is added to `ToolSettings.AdditionalWorkingDirs` for each session, exactly as in the REPL.
- Pending extraction runs after `agent.Terminal` events in server mode, with the same non-blocking fire-and-forget semantics as the REPL.

### Phase 9 - Hooks

Implemented: `internal/hooks` with snapshot, matcher, command/prompt runners, dispatcher, runner decorator.

Phase 21 implications:

- Hook dispatch runs inside the session goroutine. The hooks snapshot is taken at server startup (not per-session), matching the TUI behavior where hooks are snapshotted at REPL start.
- `SessionStart` and `SessionEnd` hook events fire per HTTP session, not per server process. This gives hook scripts visibility into remote sessions.
- `PreToolUse` and `PostToolUse` hook decisions flow through the agent's existing hook dispatch path. The server does not add a new hook dispatch layer.

### Phase 10 - MCP Integration

Implemented: `internal/mcp` with transport negotiation, tool wrapping, server lifecycle.

Phase 21 implications:

- MCP tools are available in server sessions if MCP is configured. The session constructs the tool registry the same way the REPL does.
- MCP server lifecycle is managed per-process, not per-session. MCP connections established at server startup are shared across sessions with appropriate synchronization.

### Phase 11 - Sub-agents and Fork

Implemented: sub-agent tool, fork lifecycle, child-agent state isolation.

Phase 21 implications:

- Sub-agents spawned from a server session emit their events through the parent session's event channel. The SSE stream carries sub-agent events with a `agent_id` field to distinguish parent from child.
- Child agent cancellation propagates through the session's context. DELETE session cancels all running sub-agents for that session.

### Phase 12 - Skills

Implemented: skill discovery, frontmatter parsing, prompt loading.

Phase 21 implications:

- Skills are available in server sessions via the same prompt-injection mechanism as the REPL.

### Phase 13 - Slash Commands and Config UX

Implemented: command registry, config loader, `/models`, `/pull`, `/permissions`, `/memory`, `/hooks`, `/skills`, `/agents`.

Phase 21 implications:

- The full slash command registry is not exposed over HTTP. POST `/v1/sessions/{id}/messages` is for user prompts only. Administrative commands (listing models, changing permission mode) have dedicated REST endpoints where warranted, or are out of scope for Phase 21.
- `GET /v1/models` covers the most common administrative need: listing available models.

### Phase 14 - Tasks

Implemented: background task supervisor, task state machine, task broadcast.

Phase 21 implications:

- Background tasks run within the session goroutine's context. Task events are forwarded to the SSE stream as `task_start`, `task_progress`, `task_result` events.
- Task visibility is per-session. `GET /v1/sessions/{id}` includes a summary of active tasks.

### Phase 15 - Concurrency and Speculative Execution

Implemented: tool partition algorithm, safe concurrent batch execution, speculative first-turn.

Phase 21 implications:

- Concurrent tool batches in server sessions emit events in concurrent order. The SSE stream must carry tool IDs so the client can associate deltas with the correct tool call.

### Phase 16 - Observability and Metrics

Implemented: logging decorators, metrics package, OTEL exporter, safe metric schema.

Phase 21 implications:

- Server metrics include: active sessions count, request counters, SSE connection duration, and agent run duration through the existing observability meter where practical.
- `GET /v1/health` returns JSON health state in Phase 21. Prometheus-compatible metrics are out of scope unless an existing repo metric endpoint is reused without new dependencies.

### Phase 17 - Distribution and Install

Roadmap note: Phase 17 is not implemented yet under the current roadmap. It remains after Phase 21 because server mode is required for Phase 25 and v0.1.

Phase 21 implications:

- The Dockerfile already exposes port 8080. Phase 21 satisfies the intent of that exposure.
- `docker-compose.yml` already defines a service shape for the HTTP server. Phase 21 wires it fully.
- Phase 17 should package the server subcommand into the same binary after this phase lands.

### Phase 18 - Hardening, Eval Suite, and Docs

Roadmap note: Phase 18 is not implemented yet under the current roadmap. It is the final hardening phase after Phase 17.

Phase 21 implications:

- Phase 18 must cover the server surface added here.
- Post-Phase-21 and before release: run `gosec` on `internal/server/...`, add SSE replay and permission timeout fuzz cases, and update docs with the HTTP API reference.

### Phase 19 and Phase 20

Roadmap note: Phase 19 and Phase 20 are already completed later plans. They do not change the agent runner interface required by Phase 21.

Phase 21 implications: the agent runner `AgentRunner` interface must remain the single composition point. Phase 21 uses it as-is.

## Documentation and Log Findings

The `DOCKER_WEB_GUIDE.md` describes an HTTP API that does not yet exist. Phase 21 is the phase that builds the API that document describes. The guide's endpoint shapes align with the plan below; any discrepancies between the guide and this plan should be resolved in favor of this plan, and `DOCKER_WEB_GUIDE.md` should be updated as part of Phase 21 implementation.

`docs/PROJECT-STATUS-AND-ONBOARDING.md` now marks Phase 22 as required before Phase 21, and Phase 21 as required before Phase 25, Phase 17, and Phase 18. Use that roadmap order if older notes in this document imply numeric ordering.

## Deep Analysis of `book/ch16-remote.md`

Chapter 16 describes four remote-execution topologies that Claude Code supports. Phase 21 implements a simplified version of the Direct Connect topology (the local server with WebSocket), but using SSE + HTTP POST instead of WebSocket. The motivations for that choice:

### Asymmetric Transport: SSE for Reads, HTTP POST for Writes

Chapter 16 explains why the production system uses asymmetric transport: reads are high-frequency and server-initiated (token streaming), while writes are low-frequency and client-initiated (one prompt per user turn). Unifying them on WebSocket creates coupling — if the socket drops during a write, you cannot distinguish "not sent" from "sent but acknowledgment lost."

Phase 21 chooses SSE over WebSocket for the additional reason that SSE is simpler to implement correctly in Go without external dependencies, works through HTTP/1.1 proxies and CDNs without special upgrade handling, and is natively supported by browser `EventSource` API without JavaScript libraries.

The tradeoff: SSE is server-to-client only. Client-to-server messages still use HTTP POST to `/v1/sessions/{id}/messages`. This matches the Chapter 16 pattern exactly: the CCR protocol uses WebSocket (or SSE) for reads and `CCRClient` POST calls for writes.

### BoundedRingBuffer: Fixed-Memory Replay

Chapter 16 describes `BoundedUUIDSet` as a FIFO-bounded circular buffer for deduplication. Phase 21 adapts this pattern for event replay: each session maintains a `RingBuffer[SessionEvent]` of capacity 200. When an SSE client reconnects with `Last-Event-ID`, the server replays events from that ID onward from the ring buffer. This handles the common case where the browser tab briefly loses the SSE connection.

The ring buffer is a circular slice-backed structure with O(1) append and O(n) replay from offset. 200 events × typical event size of 512 bytes = 100KB per session maximum replay memory. For 10 concurrent sessions, the ring buffers consume at most 1MB.

### Session Lifecycle: Five States

Chapter 16's Direct Connect sessions have five states (`starting`, `running`, `detached`, `stopping`, `stopped`). Phase 21 uses four states: `idle` (session created, no run active), `running` (agent run in progress), `stopped` (session cleanly terminated), `expired` (idle for longer than 30 minutes). State transitions are protected by a session-level mutex. Concurrent state reads from multiple SSE connections and write requests are safe.

### Reconnection Strategy

Chapter 16 discriminates reconnection strategy by failure type. Phase 21 is simpler (it is a local server, not a multi-datacenter cloud service) but applies the same principle: transient connection drops retry with a short delay, while explicit session deletion or session expiry does not retry. The SSE client receives a `event: terminal` with `reason: session_not_found` on a reconnect to a deleted session, which tells the client to stop retrying.

### FlushGate Equivalent: In-order SSE Events

Chapter 16's `FlushGate` prevents out-of-order writes during a history flush. Phase 21's equivalent is simpler: the ring buffer is append-only and the SSE writer reads from it sequentially. A reconnecting client presents `Last-Event-ID`, the server finds that ID in the ring buffer, and replays from the next position. There is no concurrent write path that races with the replay — the session goroutine appends to the ring buffer, and SSE readers only read. The ring buffer is safe for one writer and multiple readers under the session mutex.

## Evaluation of the Original Phase 21 Goal Statement

The goal statement above is correct at the product level. It needs elaboration in the following areas:

- It specifies SSE as the read transport but does not specify the exact event naming convention. This plan resolves that with a defined event taxonomy (see API Endpoints and SSE Event Format below).
- It mentions ring buffer replay but does not specify the buffer capacity or memory model. This plan uses 200 events per session.
- It mentions bearer token auth but does not specify the token format. This plan uses `crypto/rand`-generated 32-byte tokens encoded as hex strings.
- It mentions `tools/check-network-policy.sh` updates as if listener usage is already scanned. The current checker only scans hardcoded outbound HTTP(S) endpoints; any listener-surface check must be added deliberately and scoped to `internal/server`.
- It mentions an embedded minimal web UI but leaves its scope ambiguous. This plan specifies a single-file `web/index.html` with EventSource API, no build step, and no JavaScript framework.
- It does not specify session timeout or cleanup behavior. This plan uses 30-minute idle timeout with a background sweeper goroutine.

## Final Phase 21 Scope

In scope:

- `internal/server` package: `server.go`, `session.go`, `sse.go`, `handler.go`, `auth.go`, `ratelimit.go`, `permission.go`, `ringbuffer.go`.
- `internal/cli/server.go` — Cobra subcommand.
- `web/index.html` — minimal SSE-capable UI (embedded via `//go:embed`).
- REST API: POST sessions, GET events (SSE), POST messages, POST permissions, GET session, DELETE session, GET models, GET health.
- Ring buffer replay via `Last-Event-ID`.
- Bearer token auth (required for non-loopback bind).
- Rate limit: 10 concurrent sessions, 100 requests/minute per IP.
- Session idle timeout: 30 minutes.
- `tools/check-network-policy.sh` update only if implementation adds policy-relevant listener checks.
- `GET /v1/health` with Ollama reachability check.
- `go test -race ./internal/server/...`.
- Phase log update.

Out of scope:

- WebSocket transport.
- Multi-node session routing or session migration across server instances.
- Persistent session storage across server restarts.
- OAuth flows (a server token is a static pre-shared secret for now).
- Full web UI beyond the minimal single-file page.
- gRPC or Protobuf-based API.
- Rate limiting by user identity (only by IP in Phase 21).
- Session sharing or multi-user collaboration on a single session.
- TLS termination (expect a reverse proxy in production; document this clearly).

## Target User Experience

### Starting the Server

```
$ nandocodego server --bind 127.0.0.1 --port 8080
2026-05-07T12:00:00Z INFO server started bind=127.0.0.1 port=8080 auth=none
```

With token:

```
$ nandocodego server --bind 0.0.0.0 --port 8080 --token mysecret
2026-05-07T12:00:00Z INFO server started bind=0.0.0.0 port=8080 auth=bearer
```

Binding to non-loopback without `--token` is an error:

```
$ nandocodego server --bind 0.0.0.0 --port 8080
Error: --token is required when binding to a non-loopback address
```

### Creating a Session

```
POST /v1/sessions
Content-Type: application/json
{}

201 Created
{
  "session_id": "a3f7...",
  "created_at": "2026-05-07T12:00:01Z",
  "state": "idle"
}
```

### Streaming Events

```
GET /v1/sessions/a3f7.../events
Accept: text/event-stream

HTTP/1.1 200 OK
Content-Type: text/event-stream
Cache-Control: no-cache

id: 1
event: session_ready
data: {"session_id":"a3f7...","state":"idle"}

id: 2
event: assistant_text_delta
data: {"content":"4","session_id":"a3f7..."}

id: 5
event: terminal
data: {"reason":"completed","session_id":"a3f7...","usage":{"input_tokens":12,"output_tokens":3}}
```

### Submitting a Message

```
POST /v1/sessions/a3f7.../messages
Content-Type: application/json
{"prompt": "What is 2+2?"}

202 Accepted
{"queued": true}
```

### Permission Request Flow

1. Agent emits a permission request for a tool call.
2. Server forwards as SSE event:
   ```
   id: 8
   event: permission_request
   data: {"request_id":"req_001","tool_name":"Bash","input_summary":"rm -rf /tmp/test","session_id":"a3f7..."}
   ```
3. Client approves:
   ```
   POST /v1/sessions/a3f7.../permissions/req_001
   {"decision": "allow"}
   204 No Content
   ```
4. Agent proceeds. If 30 seconds pass with no decision, the broker auto-denies.

### Health Check

```
GET /v1/health

{"status":"ok","ollama":"reachable","active_sessions":2}
```

## Architecture

### Package Layout

```
internal/server/
  server.go        — HTTP server setup, route registration, graceful shutdown
  session.go       — session struct, state machine, lifecycle, idle timer
  sse.go           — SSE writer, event serialization, flush
  handler.go       — HTTP handler functions for all endpoints
  auth.go          — bearer token middleware
  ratelimit.go     — per-IP request rate limiter and concurrent-session cap
  permission.go    — HTTP-backed permission broker
  ringbuffer.go    — generic ring buffer for event replay

  server_test.go
  session_test.go
  sse_test.go
  handler_test.go
  auth_test.go
  ratelimit_test.go
  permission_test.go
  ringbuffer_test.go

internal/cli/
  server.go        — Cobra `server` subcommand, flag parsing, composition root

web/
  index.html       — single-file embedded UI

cmd/nandocodego/
  main.go          — registers `server` subcommand (one line addition)
```

### Core Types

```go
// SessionState is the lifecycle state of a server session.
type SessionState string

const (
    SessionIdle    SessionState = "idle"
    SessionRunning SessionState = "running"
    SessionStopped SessionState = "stopped"
    SessionExpired SessionState = "expired"
)

// Session holds all state for one HTTP session.
type Session struct {
    ID          string
    CreatedAt   time.Time
    State       SessionState
    mu          sync.Mutex
    cancel      context.CancelFunc
    events      *RingBuffer[SessionEvent]
    subscribers []chan SessionEvent
    lastActive  time.Time
    messages    []llm.Message
    permBroker  *HTTPPermissionBroker
}

// SessionEvent is one event emitted to SSE clients.
type SessionEvent struct {
    ID    int64
    Type  string
    Data  any
}

// RingBuffer is a fixed-capacity circular buffer for replay.
type RingBuffer[T any] struct {
    buf      []T
    head     int
    size     int
    capacity int
    mu       sync.Mutex
    nextID   int64
}

// HTTPPermissionBroker resolves permission requests via HTTP.
type HTTPPermissionBroker struct {
    pending map[string]chan permissionDecision
    mu      sync.Mutex
    timeout time.Duration
}

// permissionDecision carries the allow/deny/always_allow decision.
type permissionDecision struct {
    Decision string // "allow", "deny", "always_allow"
}

// RateLimiter tracks per-IP request counts with a sliding window.
type RateLimiter struct {
    windows  sync.Map // IP -> *ipWindow
    limit    int
    window   time.Duration
    sessions sync.Map // sessionID -> struct{}
    maxSess  int
}

// ServerConfig holds parsed flag values for the server command.
type ServerConfig struct {
    Bind      string
    Port      int
    Token     string
    NoUI      bool
    MaxSessions int
    IdleTimeout time.Duration
    ReadTimeout time.Duration
    WriteTimeout time.Duration
    OllamaURL string
    Model     string
    NumCtx    int
}
```

### SSE Event Taxonomy

| Event type             | When emitted                                          | Data fields |
|------------------------|-------------------------------------------------------|-------------|
| `session_ready`        | SSE connection established                            | `session_id`, `state` |
| `assistant_text_delta` | Each `agent.AssistantTextDelta`                       | `content`, `session_id` |
| `thinking_delta`       | Each `agent.AssistantThinkingDelta`                   | `thinking`, `session_id` |
| `tool_use_start`       | Each `agent.ToolUseStart`                             | `tool_id`, `tool_name`, `input`, `session_id` |
| `tool_use_progress`    | Each `agent.ToolUseProgress`                          | `tool_id`, `data`, `session_id` |
| `tool_use_result`      | Each `agent.ToolUseResult`                            | `tool_id`, `result`, `error`, `session_id` |
| `retry_notice`         | Each `agent.RetryNotice`                              | `attempt`, `cause`, `session_id` |
| `hook_notice`          | Each `agent.HookNotice`                               | `message`, `session_id` |
| `permission_request`   | When HTTP permission broker fires                     | `request_id`, `tool_name`, `input_summary`, `session_id` |
| `permission_resolved`  | When HTTP permission broker resolves                  | `request_id`, `decision`, `session_id` |
| `terminal`             | Each `agent.Terminal`                                 | `reason`, `usage`, `session_id` |
| `error`                | Internal server error during event dispatch           | `message`, `session_id` |

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/sessions` | Create a new session; returns `{session_id, created_at, state}` |
| `GET` | `/v1/sessions/{id}/events` | SSE stream of session events; honors `Last-Event-ID` for replay |
| `POST` | `/v1/sessions/{id}/messages` | Queue a user prompt for the agent; returns 202 Accepted or 409 if run active |
| `POST` | `/v1/sessions/{id}/permissions/{req_id}` | Resolve a pending permission request |
| `GET` | `/v1/sessions/{id}` | Session metadata: state, created_at, last_active, usage summary |
| `DELETE` | `/v1/sessions/{id}` | Cancel and remove session; returns 204 |
| `GET` | `/v1/models` | List models available from the configured Ollama instance |
| `GET` | `/v1/health` | Health check; returns `{status, ollama, active_sessions}` |
| `GET` | `/` | Serve `web/index.html` (unless `--no-ui`) |

### Concurrency Model

The server uses a simple concurrency model:

1. The main goroutine owns the HTTP listener via `net/http`'s `http.Server`.
2. Each HTTP handler runs in its own goroutine (standard `net/http` behavior).
3. Each session has one "agent goroutine" that drives the agent run. It is the only writer to the session's ring buffer and the only writer to `session.State`.
4. SSE connections are reader goroutines. They block on a `chan SessionEvent` that the agent goroutine broadcasts to. Multiple SSE connections to the same session each get their own channel; the agent goroutine sends to all channels under the session mutex.
5. HTTP permission decisions arrive on a handler goroutine, write to a `chan permissionDecision`, and return. The agent goroutine blocks on that channel.

This is a single-writer, multiple-reader model per session. The session mutex protects state transitions and subscriber list mutations. No shared mutable state exists between sessions.

Graceful shutdown:

1. `http.Server.Shutdown(ctx)` stops accepting new connections and waits for active handlers.
2. Before Shutdown, the server cancels all active sessions via their cancel functions.
3. Active agent goroutines receive context cancellation, emit `agent.Terminal{Reason: TerminalAborted}`, and exit.
4. SSE readers see the terminal event and close.
5. Shutdown returns after all goroutines exit or the shutdown timeout (10 seconds) expires.

## 2026-05-18 Review: Book-Derived Implementation Invariants

This section supersedes any ambiguous earlier wording. It turns the book guidance into concrete engineering constraints for Phase 21 agents.

### One Agent Loop, Multiple Transports

Book source: Chapter 5, agent loop.

Invariant:

- Server mode must call the same `agent.Run(ctx, agent.Input)` path as REPL and `--print`.
- Server mode must not implement a second tool loop, model loop, retry loop, permission pipeline, memory pipeline, or compaction pipeline.
- Session handlers may translate HTTP requests into `agent.Input` and translate `agent.Event` into SSE events, but they must not inspect model text to drive tool execution.

Implementation consequences:

- Build a small shared composition helper if needed, for example `internal/runtime/runtime.go` or `internal/cli/runtime.go`, that creates the same Ollama client, observed registry, memory runner, hooks runner, MCP manager, and agent config currently assembled in `runREPL`.
- Keep server-specific code at the transport edge: session lifecycle, permission broker, SSE, JSON handlers, and auth.
- Agent goroutine state transitions must be total and explicit: every successful run, error, panic recovery, cancellation, and terminal event returns the session to a non-running state or removes it.

Tests:

- Fake runner emits a sequence of `agent.Event`; server forwards them without model/tool special-casing.
- Panic in fake runner path emits an `error` event and returns session to `idle`.
- `DELETE` during a running fake runner cancels the context and yields a terminal/cancel state.

### Asymmetric Transport And Idempotent Writes

Book source: Chapter 16, asymmetric reads and writes.

Invariant:

- SSE is the read stream. HTTP POST is the write path.
- Writes must have clear acknowledgment semantics. A POST either starts a run and returns 202, fails validation, or reports a conflict.
- Phase 21 should support an optional client-supplied idempotency key for message POSTs to avoid duplicate turns after client retry.

Implementation consequences:

- Add optional `message_id` to `POST /v1/sessions/{id}/messages`.
- Maintain a bounded per-session recent message ID set (capacity 200) using the same fixed-memory pattern as ring-buffer replay.
- Duplicate `message_id` while a run is active returns the original accepted response shape if known, or 202 with `duplicate: true`; it must not start another agent run.
- If no `message_id` is supplied, behavior remains non-idempotent and simple.

Tests:

- Two POSTs with the same `message_id` start only one run.
- Duplicate `message_id` after completion does not append a second user message.
- Distinct IDs can start distinct turns sequentially.

### Bounded Memory Everywhere

Book source: Chapter 16, bounded UUID set; Chapter 6, tool result budgets; Chapter 11, memory indexing.

Invariant:

- Every per-session buffer is fixed-capacity or timeout-bound.
- No SSE subscriber, permission request, event replay buffer, recent message ID set, or session map can grow without bound.

Implementation consequences:

- Ring buffer capacity defaults to 200 events and is configurable only by server config, not per request.
- Subscriber channel capacity is small and fixed, for example 64.
- Permission pending map is bounded by active tool prompts; timeout cleanup is mandatory.
- Session registry is bounded by `--max-sessions`.
- Idle sweeper must clean stopped/expired sessions and release the session cap.

Tests:

- Slow subscriber does not block `Broadcast`.
- Ring buffer evicts oldest events.
- Permission timeout removes pending request.
- Session cap is released on `DELETE` and expiry.

### Prompt Fidelity Parity

Book source: Chapter 4, prompt assembly; Phase 22 follow-up.

Invariant:

- Server prompt assembly must use the same current-turn context packing behavior as TUI and `--print`.
- Normal HTTP prompts with large `@file` or `@dir` evidence must not silently become `/analyze-project`.
- Explicit project analysis is out of scope for `/v1/sessions/{id}/messages`; server message POST treats leading slash text as plain prompt unless a dedicated endpoint is later added.

Implementation consequences:

- Before constructing the user message, call `contextpack.BuildCurrentTurnPrompt(...)` using the same runtime `agent.Config`, runtime `NumCtx`, `MaxOutputTokens`, context mode, model, and session history that the final agent run uses.
- Apply file-scoped status latest-only history policy just like TUI.
- Carry `EvidencePack`, `PromptIntent`, `AttachmentPolicy`, `OriginalUserText`, and `HistoryPolicy` into `agent.Input`.
- Return a 413 or 422 structured JSON error for `contextpack.ErrEvidenceTooLarge`; do not start an agent run.

Tests:

- Server and `contextpack.BuildCurrentTurnPrompt` produce the same final prompt shape for small file, large file, multi-file, and directory prompt fixtures.
- Large file status prompt returns latest-only history metadata in prompt dump and agent input.
- Too-large evidence returns a split-guidance error and no fake runner call.

### Permission Flow Fails Closed

Book source: Chapter 6, fail-closed tool and permission pipeline.

Invariant:

- Missing, invalid, timed-out, duplicate, or unknown permission decisions deny by default.
- Server mode must not introduce any permission bypass beyond existing configured `permissions.Mode`.

Implementation consequences:

- HTTP permission broker returns deny on timeout and context cancellation.
- `always_allow` must update the session permission rules using existing permission rule types and source semantics; it must not mutate global config files.
- Permission request SSE data should include enough for a client UI to decide, but must avoid leaking full huge tool inputs by default. Include `input_summary`, tool name, target when available, and a bounded preview.
- Permission decision endpoint must require auth whenever auth is configured.

Tests:

- Timeout denies.
- Context cancellation denies.
- Unknown request ID returns 404.
- `always_allow` affects a second equivalent request in the same session only.
- Large tool input is truncated in `permission_request`.

### Streaming With Watchdogs And Heartbeats

Book source: Chapter 4, streaming watchdog.

Invariant:

- A connected SSE client must receive periodic heartbeat comments so proxies and browsers do not silently stall.
- Server-side agent execution should not depend on SSE clients staying connected.

Implementation consequences:

- SSE handler sends `: heartbeat\n\n` every 15 seconds while connected.
- Agent run continues if the SSE client disconnects; reconnect can replay buffered events.
- Handler detects request context cancellation and unsubscribes cleanly.
- Write errors close only that SSE connection, not the session.

Tests:

- Heartbeat is emitted with no events.
- SSE disconnect removes subscriber.
- Run continues after subscriber disconnect and events are replayable.

### Observability Without Content Leakage

Book source: Chapter 4 API layer, Chapter 16 remote, Phase 16 observability.

Invariant:

- Logs and metrics may include session ID, event type, status code, duration, model name, and counts.
- Logs and metrics must not include prompts, assistant text, memory contents, tool inputs, tool outputs, permission decisions with raw input, or file bodies.

Implementation consequences:

- Add access logging middleware that records method, route template, status, latency, remote IP hash or raw IP only at DEBUG if needed, and session ID.
- Avoid logging request/response bodies.
- `/v1/health` should return operational state only: server status, Ollama reachability, active session count, version if available.

Tests:

- Access log test with prompt body confirms body text is absent.
- Permission request with sensitive text logs only request ID/tool name/status.

## Agent-Ready Implementation Slices

The flat todo list below remains as a checklist, but implementation agents should work in these slices. Each slice has a bounded write scope and verification command.

### P21-0 - Runtime Composition Extraction

Goal:

Create reusable runtime builders so REPL, `--print`, and server do not diverge in model config, context policy, tool registry, memory, hooks, MCP, and observability.

Files:

- `internal/cli/repl.go`
- `internal/cli/print.go`
- New candidate: `internal/runtime/runtime.go` or `internal/cli/runtime.go`
- Tests under the chosen package

Tasks:

- Extract config/bootstrap loading helpers used by REPL and print into a shared function.
- Extract agent config construction, including `NumCtx`, `MaxOutputTokens`, watchdog, context mode, and reserve.
- Extract tool registry construction for built-ins, skills, MCP, task/agent tools, and observability wrapping where possible.
- Keep process-owned resources clear: MCP manager and skill loader still need explicit close hooks.
- Do not change behavior of existing REPL and print flows.

Acceptance:

- Existing `go test ./internal/cli ./internal/tui ./internal/agent` passes.
- A test proves print and server/reusable config paths produce the same `agent.Config` for `--num-ctx 131072`.

Agent prompt:

```text
Implement P21-0 from docs/PHASE-21-DETAILED-PLAN.md.
Extract shared runtime composition helpers for config loading and agent config/tool registry setup without changing REPL or --print behavior. Add tests proving NumCtx/MaxOutputTokens/context mode parity.
Run go test ./internal/cli ./internal/tui ./internal/agent.
```

### P21-1 - Server Core Primitives

Goal:

Implement low-level primitives with no agent dependency: ring buffer, bounded recent ID set, auth, rate limiter, SSE writer.

Files:

- `internal/server/ringbuffer.go`
- `internal/server/recentids.go`
- `internal/server/auth.go`
- `internal/server/ratelimit.go`
- `internal/server/sse.go`
- Matching tests

Tasks:

- Implement fixed-capacity replay buffer.
- Implement fixed-capacity recent message ID dedup set.
- Implement bearer middleware with constant-time compare.
- Implement rate limiter and exact session cap.
- Implement SSE writer with JSON encoding, flush, heartbeat support, and `Last-Event-ID` replay support helpers.

Acceptance:

- `go test -race ./internal/server` passes for primitive tests.
- No new external dependencies.

Agent prompt:

```text
Implement P21-1 from docs/PHASE-21-DETAILED-PLAN.md.
Add internal/server primitives: ring buffer, recent ID dedup set, auth middleware, rate limiter/session cap, and SSE writer with heartbeat support. Keep it independent from agent runtime.
Run go test -race ./internal/server.
```

### P21-2 - Session Registry And Lifecycle

Goal:

Implement session state, subscribers, idle expiry, cancellation, and lifecycle invariants.

Files:

- `internal/server/session.go`
- `internal/server/session_test.go`

Tasks:

- Implement `Session`, `SessionRegistry`, lifecycle states, allowed transitions, cancellation, subscriber management, and sweeper.
- Include `messages []llm.Message`, `lastUsage`, `lastActive`, pending permission broker, recent message IDs, and event ring.
- Ensure `DELETE` and expiry release session cap through a registry callback or explicit server coordination.
- Ensure no goroutine sends on a closed subscriber channel.

Acceptance:

- State transition, subscriber, expiry, cancellation, and race tests pass under `go test -race ./internal/server`.

Agent prompt:

```text
Implement P21-2 from docs/PHASE-21-DETAILED-PLAN.md.
Add session registry and lifecycle management with bounded subscribers, cancellation, idle expiry, recent message IDs, and race-safe transitions.
Run go test -race ./internal/server.
```

### P21-3 - HTTP Permission Broker

Goal:

Adapt existing permission prompting to HTTP/SSE while preserving fail-closed semantics.

Files:

- `internal/server/permission.go`
- `internal/server/permission_test.go`
- If needed: minimal additions to `internal/permissions`

Tasks:

- Implement blocking `permissions.PromptFunc` adapter.
- Broadcast bounded `permission_request` events.
- Accept `allow`, `deny`, and `always_allow` decisions.
- Deny on timeout, cancellation, malformed decision, duplicate resolution, or missing request.
- Apply `always_allow` to session-local permission rules only.

Acceptance:

- Permission happy path, timeout, cancellation, double resolve, and session-local always-allow tests pass.

Agent prompt:

```text
Implement P21-3 from docs/PHASE-21-DETAILED-PLAN.md.
Add HTTP-backed permission broker that emits SSE permission_request events and resolves through POST decisions. Preserve fail-closed behavior and session-local always_allow rules.
Run go test -race ./internal/server.
```

### P21-4 - Handler Skeleton And API Contract

Goal:

Expose REST/SSE endpoints with fake runner support before wiring the real agent runtime.

Files:

- `internal/server/handler.go`
- `internal/server/server.go`
- `internal/server/handler_test.go`
- `internal/server/server_test.go`

Tasks:

- Implement route parsing without third-party router dependencies.
- Implement all endpoint status codes and JSON error shapes.
- Implement auth, rate limit, no-payload access logging, session cap, and bind validation.
- Use injected fake runner/client in tests.

Acceptance:

- Handler tests cover success and error paths for every endpoint.
- No Ollama server required for tests.

Agent prompt:

```text
Implement P21-4 from docs/PHASE-21-DETAILED-PLAN.md.
Add HTTP server and handlers for sessions, events, messages, permissions, models, health, and UI route using injected fake clients/runners. Keep real agent runtime wiring out of this slice.
Run go test -race ./internal/server.
```

### P21-5 - Agent Run Integration And Prompt Packing Parity

Goal:

Wire real agent runner into sessions while preserving Phase 22 prompt packing and existing tool/memory/hooks behavior.

Files:

- `internal/server/session.go`
- `internal/server/handler.go`
- Runtime composition helper from P21-0
- Tests in `internal/server`

Tasks:

- On message POST, use `contextpack.BuildCurrentTurnPrompt(...)` before appending the user message.
- Carry prompt metadata into `agent.Input`.
- Apply latest-only history for listing/file-status intents.
- Start exactly one agent goroutine per session run.
- Map every current `agent.Event` type to a stable SSE event.
- Store terminal conversation back into session history.
- Return structured 413/422 on too-large evidence before starting a run.

Acceptance:

- Fake runner tests prove prompt metadata arrives in `agent.Input`.
- Too-large evidence returns error and runner is not called.
- Event mapping tests cover all current agent event types.

Agent prompt:

```text
Implement P21-5 from docs/PHASE-21-DETAILED-PLAN.md.
Wire session message POST into real/fake AgentRunner with Phase 22 context packing parity, latest-only history policy, event mapping, terminal history storage, and too-large evidence errors.
Run go test -race ./internal/server ./internal/contextpack ./internal/agent.
```

### P21-6 - CLI Server Command And Graceful Shutdown

Goal:

Add the `server` subcommand and process lifecycle.

Files:

- `internal/cli/server.go`
- `internal/cli/root.go`
- `internal/cli/server_test.go`
- Potential runtime helper files

Tasks:

- Add `newServerCmd(...)` and register it in `newRootCommand`.
- Flags: `--bind`, `--port`, `--token`, `--no-ui`, `--max-sessions`, `--idle-timeout`, `--read-timeout`, `--write-timeout`.
- Reuse root `--model`, `--ollama-url`, `--num-ctx`, logging flags.
- Validate non-loopback bind requires token.
- Handle SIGINT/SIGTERM with 10-second graceful shutdown.

Acceptance:

- CLI tests cover flag parsing, bind validation, root registration, and graceful shutdown with a fake server.

Agent prompt:

```text
Implement P21-6 from docs/PHASE-21-DETAILED-PLAN.md.
Add nandocodego server CLI command wired through internal/cli/root.go, reuse root model/ollama/num-ctx flags, validate auth on non-loopback bind, and support graceful shutdown.
Run go test ./internal/cli ./internal/server.
```

### P21-7 - Embedded Web UI

Goal:

Serve a minimal functional browser client without adding a frontend build system.

Files:

- `web/index.html`
- `internal/server/server.go` or `internal/server/ui.go`
- UI route tests

Tasks:

- Add single HTML file with vanilla JS `EventSource`.
- Create session, open SSE stream, send message, append streaming text, render tool blocks as `<details>`, render permission prompt modal.
- Store bearer token only in memory or sessionStorage if user enters it; do not put token in URL.
- Do not write conversation/tool data to localStorage.
- Keep design utilitarian and responsive.

Acceptance:

- `GET /` serves UI unless `--no-ui`.
- No npm, no bundler, no new dependency.
- Manual browser smoke can create session and stream response.

Agent prompt:

```text
Implement P21-7 from docs/PHASE-21-DETAILED-PLAN.md.
Add embedded single-file web UI with EventSource streaming, message submit, tool result display, and permission decisions. No npm or frontend dependencies.
Run go test ./internal/server and perform a browser/curl smoke if possible.
```

### P21-8 - Docker, Docs, Smoke Scripts, And Security Checks

Goal:

Close documentation, Docker, and verification.

Files:

- `DOCKER_WEB_GUIDE.md`
- `docs/PHASE-LOG.md`
- `docs/PROJECT-STATUS-AND-ONBOARDING.md`
- `tools/smoke-server.sh`
- `tools/check-network-policy.sh` only if needed
- `Dockerfile`, `docker-compose.yml` if current commands are stale

Tasks:

- Update docs to match actual endpoint shapes and auth behavior.
- Add smoke script that creates session, opens SSE, sends message, waits for terminal, deletes session.
- Verify Docker command starts `nandocodego server` on port 8080.
- Run dependency and network policy checks.
- Run `gosec` only if available; document skipped if not installed.

Acceptance:

- `go test ./...` passes.
- `tools/check-allowed-deps.sh` passes.
- `tools/check-network-policy.sh` passes.
- Smoke script passes with live Ollama or is documented as blocked.

Agent prompt:

```text
Implement P21-8 from docs/PHASE-21-DETAILED-PLAN.md.
Update Docker/web docs, phase log, project status, and add a curl smoke script for the implemented server API. Run repo tests and policy checks; run gosec if available.
```

## Implementation Plan

### Step 1 - Ring Buffer

File: `internal/server/ringbuffer.go`, `internal/server/ringbuffer_test.go`

Implement a generic ring buffer `RingBuffer[T]`:

- Fixed capacity, set at construction time.
- `Append(item T) int64` returns the assigned event ID (monotonically increasing int64).
- `Since(id int64) []T` returns all items with ID greater than `id`.
- `All() []T` returns all currently stored items in order.
- Thread-safe under a `sync.Mutex`.

Rules:

- When the buffer is full, oldest items are evicted (circular overwrite).
- `Since` with an ID older than the oldest retained item returns all retained items (partial replay is better than no replay).
- `Since` with an ID equal to the most recent returns empty slice.
- `Since` with ID `-1` (or any negative value) returns all retained items.

Tests:

- Append up to capacity, verify all items retained.
- Append past capacity, verify oldest items evicted.
- `Since` at boundary, before beginning, after end.
- Race detector passes with concurrent `Append` and `Since`.

### Step 2 - Session

File: `internal/server/session.go`, `internal/server/session_test.go`

Implement `Session` and `SessionRegistry`:

- `NewSession(id string) *Session`
- `Session.AddSubscriber() (chan SessionEvent, func())` — returns channel and an unsubscribe function.
- `Session.Broadcast(event SessionEvent)` — sends to all subscriber channels; drops events for full channels (non-blocking send) to prevent slow SSE clients from blocking the agent goroutine.
- `Session.Transition(to SessionState) error` — validates allowed transitions.
- `Session.Close()` — cancels context, drains and closes all subscriber channels.
- `SessionRegistry` — thread-safe map of `sessionID -> *Session` with `Create`, `Get`, `Delete`, `All` methods.
- Background sweeper: a goroutine that scans sessions every 5 minutes and expires sessions idle for more than 30 minutes.

Allowed state transitions:

- `idle -> running` (on message POST starting a run)
- `running -> idle` (on Terminal event with any reason)
- `idle -> stopped` (on DELETE)
- `running -> stopped` (on DELETE while run active, cancel is called)
- `stopped -> expired` is not a valid transition; stopped sessions are deleted from registry.
- `idle -> expired` (by background sweeper after idle timeout)

Tests:

- Create session, verify default state.
- Transition through valid states.
- Invalid transitions return error.
- AddSubscriber; Broadcast delivers to all subscribers.
- Broadcast to full subscriber channel does not block.
- Sweeper expires idle sessions after mock clock advances past timeout.
- Race detector passes with concurrent Broadcast and AddSubscriber.

### Step 3 - SSE Writer

File: `internal/server/sse.go`, `internal/server/sse_test.go`

Implement `SSEWriter`:

- `NewSSEWriter(w http.ResponseWriter) (*SSEWriter, error)` — sets `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `X-Accel-Buffering: no` (for nginx).
- `WriteEvent(id int64, eventType string, data any) error` — encodes data as JSON, writes the SSE frame, flushes.
- SSE frame format:
  ```
  id: <id>\n
  event: <type>\n
  data: <json>\n
  \n
  ```
- `Flush()` — calls `http.Flusher.Flush()` if available.

Rules:

- Never buffer multiple events before flushing; flush after every `WriteEvent` call.
- If `data` marshals to a JSON object with a `session_id` field already, pass it through. Otherwise, wrap: `{"session_id": ..., "payload": ...}`.
- Errors from `w.Write` are returned; the caller (handler) closes the SSE connection.

Tests:

- Single event writes correct frame format.
- JSON marshaling errors return error, do not write partial frame.
- Flush is called after each write (use `httptest.NewRecorder` with a custom flusher).

### Step 4 - Auth Middleware

File: `internal/server/auth.go`, `internal/server/auth_test.go`

Implement bearer token middleware:

- `NewAuthMiddleware(token string) func(http.Handler) http.Handler`
- If `token` is empty string, the middleware is a no-op pass-through.
- If `token` is non-empty, check `Authorization: Bearer <token>` header.
- On mismatch: respond 401 with `{"error": "unauthorized"}`.
- Token comparison uses `crypto/subtle.ConstantTimeCompare` to prevent timing attacks.
- Token generation helper: `GenerateToken() (string, error)` uses `crypto/rand` to generate 32 bytes encoded as hex.

Tests:

- Empty token: all requests pass.
- Correct token: request passes.
- Wrong token: 401.
- Missing header: 401.
- Timing-safe comparison: verify no early return on prefix match.

### Step 5 - Rate Limiter

File: `internal/server/ratelimit.go`, `internal/server/ratelimit_test.go`

Implement two limits:

1. Per-IP request rate: sliding window counter, max 100 requests per minute.
2. Global concurrent session cap: max 10 active sessions.

```go
type RateLimiter struct {
    // per-IP windows
    windows sync.Map // string -> *ipWindow
    limit   int
    window  time.Duration
    // session cap
    sessions sync.Map // sessionID -> struct{}
    maxSess  int
}

func (r *RateLimiter) Allow(remoteAddr string) bool
func (r *RateLimiter) SessionAllow(sessionID string) bool
func (r *RateLimiter) SessionRelease(sessionID string)
```

`ipWindow` uses an atomic counter and a reset timestamp. On each request, if `now - resetAt >= window`, reset counter to 1 and update `resetAt`; otherwise increment counter. Allow if counter <= limit.

Rules:

- Remote address parsing strips port with `net.SplitHostPort`.
- Expired IP windows are not cleaned up eagerly; the `sync.Map` grows slowly. A 5-minute sweeper deletes windows where `now - resetAt > 2*window`.
- Session cap is exact: `SessionAllow` fails atomically if count would exceed `maxSess`.

Tests:

- Allow under limit; deny over limit.
- Counter resets after window expires.
- Session cap: allow up to max, deny at max+1, release frees a slot.
- Race detector passes with concurrent Allow calls from multiple goroutines.

### Step 6 - HTTP Permission Broker

File: `internal/server/permission.go`, `internal/server/permission_test.go`

Implement `HTTPPermissionBroker`:

```go
type HTTPPermissionBroker struct {
    pending map[string]chan permissionDecision
    mu      sync.Mutex
    timeout time.Duration
    send    func(SessionEvent) // broadcast function from session
}

// Ask is called by the permission resolver when a tool needs approval.
// It broadcasts a permission_request SSE event and blocks until resolved or timeout.
func (b *HTTPPermissionBroker) Ask(ctx context.Context, req PermissionRequest) (PermissionDecision, error)

// Resolve is called by the POST handler when the client sends a decision.
func (b *HTTPPermissionBroker) Resolve(reqID string, decision permissionDecision) error
```

Rules:

- `Ask` generates a unique `request_id` using `crypto/rand` (8 hex bytes).
- `Ask` creates a `chan permissionDecision` with buffer 1, stores it under `request_id`, broadcasts the `permission_request` SSE event, then selects on the channel and a timeout timer (default 30 seconds).
- On timeout, `Ask` returns `PermissionDecisionDeny` and logs a debug message.
- On context cancellation, `Ask` returns `PermissionDecisionDeny` without broadcasting `permission_resolved`.
- `Resolve` looks up the channel, sends the decision (non-blocking with select-default to avoid blocking if the Ask already timed out), returns error if `request_id` not found.
- Cleanup: `Ask` removes the channel from the map in a `defer`, regardless of how it returns.

Tests:

- Happy path: `Ask` broadcasts event, `Resolve` delivers decision, `Ask` returns it.
- Timeout: `Ask` returns deny after timeout fires.
- Context cancel: `Ask` returns deny immediately.
- Double resolve: second `Resolve` call returns not-found error.
- Race detector passes with concurrent `Ask` and `Resolve`.

### Step 7 - HTTP Handlers

File: `internal/server/handler.go`, `internal/server/handler_test.go`

Implement handlers for all nine endpoints. Each handler:

- Parses and validates the request.
- Looks up or creates the relevant session.
- Executes the action.
- Returns a JSON response with an appropriate status code.

POST `/v1/sessions`:

- Check session cap via `RateLimiter.SessionAllow`.
- Create session in registry.
- Return 201 Created with `{session_id, created_at, state: "idle"}`.

GET `/v1/sessions/{id}/events`:

- Look up session (404 if not found).
- Subscribe to session events: `session.AddSubscriber()`.
- Replay events since `Last-Event-ID` from ring buffer.
- Loop: read from subscriber channel, write SSE event, flush.
- On subscriber channel close (session stopped), write terminal event and return.

POST `/v1/sessions/{id}/messages`:

- Look up session (404).
- Decode `{prompt: string, message_id?: string}`.
- If `message_id` is present and was already accepted in this session, return 202 with `{"queued": true, "duplicate": true}` and do not start a second run.
- If `session.State == SessionRunning`, return 409 Conflict.
- Transition session to `running`.
- Launch agent goroutine (see Step 8).
- Return 202 Accepted `{"queued": true}`.

POST `/v1/sessions/{id}/permissions/{req_id}`:

- Look up session (404).
- Decode `{decision: "allow"|"deny"|"always_allow"}`.
- Call `session.permBroker.Resolve(req_id, decision)`.
- Return 204 No Content or 404 if request_id not found.

GET `/v1/sessions/{id}`:

- Look up session (404).
- Return `{session_id, created_at, last_active, state, usage}`.

DELETE `/v1/sessions/{id}`:

- Look up session (404).
- Call `session.cancel()` to abort any running agent.
- `session.Transition(SessionStopped)`.
- Remove from registry.
- `RateLimiter.SessionRelease(id)`.
- Return 204 No Content.

GET `/v1/models`:

- Call `llm.Client.ListModels(ctx)`.
- Return `{models: [{name, size, modified_at}]}`.

GET `/v1/health`:

- Ping Ollama with a short-timeout context.
- Return `{status: "ok"|"degraded", ollama: "reachable"|"unreachable", active_sessions: N}`.

GET `/`:

- If `--no-ui`, return 404.
- Otherwise, serve embedded `web/index.html`.

Tests (use `net/http/httptest`):

- POST sessions: success, cap exceeded.
- GET events: verify SSE frame format with a test session.
- POST messages: success 202, 409 on active run, 404 on unknown session.
- POST permissions: success, unknown request_id, unknown session.
- GET session: success, 404.
- DELETE session: success, 404, cancels running agent.
- GET health: Ollama reachable, Ollama unreachable (mock client).

### Step 8 - Agent Goroutine

File: `internal/server/session.go` (agentRun function)

The agent goroutine is started by the POST messages handler. It:

1. Packs current-turn evidence with `contextpack.BuildCurrentTurnPrompt(...)`.
2. Applies listing/file-status history policy exactly as TUI does.
3. Appends the packed user message to `session.messages`.
4. Constructs `agent.Input` with prompt metadata (`EvidencePack`, intent, attachment policy, original user text, history policy), system prompt, tool settings, and session-local permission fields.
5. Injects memory via the memory runner wrapper.
6. Calls `runner.Run(ctx, input)`, receiving `<-chan agent.Event`.
7. For each event:
   a. Maps `agent.Event` to a `SessionEvent` using the SSE event taxonomy.
   b. Appends the `SessionEvent` to the ring buffer.
   c. Broadcasts to all session subscribers.
   d. On `agent.Terminal`: appends conversation to `session.messages`, transitions to `idle`, exits loop.
8. On goroutine panic: recover, broadcast `error` event, transition to `idle`.

The agent goroutine respects context cancellation from `DELETE /v1/sessions/{id}`.

### Step 9 - HTTP Server Setup

File: `internal/server/server.go`

Implement `Server` struct:

```go
type Server struct {
    cfg      ServerConfig
    registry *SessionRegistry
    limiter  *RateLimiter
    auth     func(http.Handler) http.Handler
    runner   AgentRunner
    llm      llm.Client
    httpSrv  *http.Server
}

func New(cfg ServerConfig, runner AgentRunner, llmClient llm.Client) *Server
func (s *Server) Start(ctx context.Context) error
func (s *Server) Shutdown(ctx context.Context) error
```

Route registration in `Start`:

- Build `http.ServeMux`.
- Wrap all routes with `auth` middleware and `loggingMiddleware`.
- Register the rate-limit check middleware on session-creation routes.
- Register all handlers.
- Start the background session sweeper goroutine.
- Start `http.Server.ListenAndServe`.
- On context cancel: `http.Server.Shutdown(shutdownCtx)`.

Bind validation in `Start`:

- Parse `cfg.Bind` with `net.ParseIP`.
- If IP is not loopback (`!ip.IsLoopback()`) and `cfg.Token == ""`, return error.

### Step 10 - Server CLI Command

File: `internal/cli/server.go`

Cobra command:

```go
var serverCmd = &cobra.Command{
    Use:   "server",
    Short: "Start the nandocodego HTTP server",
    Long:  "...",
    RunE:  runServer,
}

func init() {
    serverCmd.Flags().String("bind", "127.0.0.1", "address to bind")
    serverCmd.Flags().Int("port", 8080, "port to listen on")
    serverCmd.Flags().String("token", "", "bearer token for auth")
    serverCmd.Flags().Bool("no-ui", false, "disable web UI")
    serverCmd.Flags().Int("max-sessions", 10, "maximum concurrent server sessions")
    serverCmd.Flags().Duration("idle-timeout", 30*time.Minute, "idle session timeout")
    serverCmd.Flags().Duration("read-timeout", 30*time.Second, "HTTP read timeout")
    serverCmd.Flags().Duration("write-timeout", 0, "HTTP write timeout; 0 disables timeout for SSE")
}
```

`runServer`:

1. Read flags into `ServerConfig`.
2. Compose `llm.Client` from `--ollama-url`, `--model`, and `--num-ctx` (reuse root command flags).
3. Compose memory runner wrapping agent runner.
4. Call `server.New(cfg, runner, llmClient).Start(ctx)`.
5. On SIGINT/SIGTERM: call `Shutdown` with 10-second context.

Add `rootCmd.AddCommand(newServerCmd(...))` in `internal/cli/root.go`. `cmd/nandocodego/main.go` should remain the thin process entry point and should not own command registration.

### Step 11 - Embedded Web UI

File: `web/index.html`

The minimal web UI is a single HTML file with no build step:

- Uses browser `EventSource` API for SSE.
- Has a text input and submit button for sending messages.
- Displays streaming assistant text in a scrollable div.
- Collapses tool use blocks into `<details>` elements.
- Shows permission request prompts as modal overlays with Allow/Deny buttons.
- Uses vanilla JavaScript only — no npm, no bundler, no framework.
- Is embedded via `//go:embed web/index.html` in `internal/server/server.go`.
- Is served at `GET /` behind the auth middleware.

The UI is intentionally minimal. Full-featured web UIs are a community contribution surface; Phase 21's embedded UI demonstrates the API and provides a functional fallback.

### Step 12 - Network Policy Update

File: `tools/check-network-policy.sh`

The current script scans hardcoded HTTP(S) endpoints and does not flag `net.Listen`. Update it only if the implementation adds listen-surface policy checks:

- Allow one server listener construction in `internal/server/server.go` or `internal/server/server.go`'s `http.Server` startup path (marked with a `# server-listen` comment in the source if the script checks listen calls).
- Continue to flag any `net.Listen` calls outside `internal/server/`.
- Continue to flag outbound `net.Dial`/`http.Get`/`http.Post` calls outside the allowlist.

The distinction: listening for inbound connections is not an outbound network policy violation. The script must not conflate the two.

### Step 13 - Tests and Verification

Required commands:

```sh
go test -race ./internal/server/...
go test ./internal/cli/...
go test ./...
tools/check-allowed-deps.sh
tools/check-network-policy.sh
```

Manual smoke:

```sh
go run ./cmd/nandocodego server --bind 127.0.0.1 --port 8080
# In another terminal:
SESSION=$(curl -s -X POST http://localhost:8080/v1/sessions | jq -r .session_id)
curl -N "http://localhost:8080/v1/sessions/$SESSION/events" &
curl -s -X POST "http://localhost:8080/v1/sessions/$SESSION/messages" \
     -H 'Content-Type: application/json' \
     -d '{"prompt": "What is 2+2?"}'
```

Verify:

- SSE stream produces `assistant_text_delta` events.
- Final `terminal` event with `reason: completed`.
- `GET /v1/health` returns `{"status":"ok","ollama":"reachable"}`.
- `DELETE /v1/sessions/$SESSION` returns 204.
- Second `GET /v1/sessions/$SESSION` returns 404.

## Implementation Todos

Resolved status (2026-05-19):

- `done`: server package/files, CLI command wiring, embedded UI, smoke script, replay/dedup/permission/cancellation behaviors, and automated test coverage added.
- `done`: `go test ./...`, `go test -race ./internal/server/...`, `go test -count=3 -race ./internal/server/...`, `go vet ./...`, `tools/check-allowed-deps.sh`, `tools/check-network-policy.sh`.
- `blocked`: `gosec ./internal/server/...` (tool not installed in this environment).
- `blocked`: Docker runtime validation (`docker build/run`) because Docker daemon is not running in this environment.
- `blocked`: live Ollama/browser manual flow requires interactive runtime with Ollama endpoint.

Legacy planning checklist (kept for traceability):

- [ ] Create `internal/server/` directory.
- [ ] Implement `RingBuffer[T]` in `ringbuffer.go`.
- [ ] Write `RingBuffer` tests: capacity, eviction, `Since`, race.
- [ ] Implement bounded recent message ID set in `recentids.go`.
- [ ] Write recent message ID tests: dedup, eviction, duplicate POST semantics.
- [ ] Implement `Session` struct with state machine in `session.go`.
- [ ] Implement `SessionRegistry` with Create/Get/Delete/All.
- [ ] Implement session background sweeper (idle timeout 30 min).
- [ ] Write session state transition tests.
- [ ] Write subscriber broadcast tests.
- [ ] Write sweeper tests with mock clock.
- [ ] Implement `SSEWriter` in `sse.go` with frame format.
- [ ] Write SSE frame format tests.
- [ ] Write SSE flush tests using httptest with custom flusher.
- [ ] Implement `NewAuthMiddleware` in `auth.go`.
- [ ] Implement `GenerateToken` using `crypto/rand`.
- [ ] Write auth middleware tests: pass, wrong token, missing header.
- [ ] Implement `RateLimiter` in `ratelimit.go` with per-IP and session-cap.
- [ ] Write rate limiter tests: allow, deny, reset, session cap, release.
- [ ] Implement `HTTPPermissionBroker` in `permission.go`.
- [ ] Write permission broker tests: happy path, timeout, cancel, double-resolve.
- [ ] Implement all nine HTTP handlers in `handler.go`.
- [ ] Write handler tests for every endpoint using `httptest`.
- [ ] Implement `agentRun` goroutine in `session.go`.
- [ ] Wire `contextpack.BuildCurrentTurnPrompt(...)` into message POST before user message append.
- [ ] Carry evidence pack, prompt intent, attachment policy, original text, and history policy into `agent.Input`.
- [ ] Return structured too-large evidence error without starting a run.
- [ ] Map every `agent.Event` type to a `SessionEvent` in `session.go`.
- [ ] Implement `Server` struct and `Start`/`Shutdown` in `server.go`.
- [ ] Implement bind-validation: non-loopback without token returns error.
- [ ] Add access log middleware (DEBUG level only, no payload logging).
- [ ] Implement `//go:embed web/index.html` in `server.go`.
- [ ] Write `web/index.html` with EventSource, message input, tool panel, permission dialog.
- [ ] Create `internal/cli/server.go` with Cobra command.
- [ ] Wire `--bind`, `--port`, `--token`, `--no-ui` flags.
- [ ] Compose runner (memory runner wrapping agent runner) in `runServer`.
- [ ] Handle SIGINT/SIGTERM in `runServer` with graceful shutdown.
- [ ] Add `rootCmd.AddCommand(newServerCmd(...))` in `internal/cli/root.go`.
- [ ] Update `tools/check-network-policy.sh` only if implementation adds listen-surface checks.
- [ ] Add `# server-listen` comment to the listener startup path only if the network policy script is extended to check listen calls.
- [ ] Implement `GET /v1/health` with Ollama ping.
- [ ] Implement `GET /v1/models` proxying `llm.Client.ListModels`.
- [ ] Implement `Last-Event-ID` replay in GET events handler.
- [ ] Implement 409 Conflict for POST messages when session is running.
- [ ] Implement optional `message_id` idempotency for POST messages.
- [ ] Implement permission request SSE event in HTTPPermissionBroker.
- [ ] Implement permission auto-deny on 30-second timeout.
- [ ] Implement `always_allow` decision type updating permission rules.
- [ ] Write handler test for 409 on concurrent POST messages.
- [ ] Write handler test for `Last-Event-ID` replay.
- [ ] Write integration test that creates session, sends message, reads SSE to terminal.
- [ ] Run `go test -race ./internal/server/...` and fix any races.
- [ ] Run `go test ./...` and confirm no regressions.
- [ ] Run `tools/check-allowed-deps.sh` and confirm no new deps.
- [ ] Run `tools/check-network-policy.sh` and confirm pass.
- [ ] Run manual smoke flow (create session, send message, read SSE, delete).
- [ ] Confirm `GET /v1/health` returns correct Ollama status.
- [ ] Confirm session idle timeout expires sessions correctly.
- [ ] Confirm concurrent-session cap (start 11 sessions, 11th returns error).
- [ ] Confirm bearer token auth rejects wrong token.
- [ ] Confirm bind validation error on non-loopback without token.
- [ ] Confirm graceful shutdown cancels running agent sessions.
- [ ] Update `docs/PHASE-LOG.md` with Phase 21 entry.
- [ ] Update `DOCKER_WEB_GUIDE.md` to match the implemented API.
- [ ] Verify embedded web UI works in browser (open `/`, send message, see streaming).
- [ ] Confirm web UI permission dialog works end to end.
- [ ] Confirm `go vet ./...` clean.
- [ ] Confirm `gosec ./internal/server/...` produces zero findings, or document that `gosec` is not installed.
- [ ] Add Phase 21 server entry to `docs/PROJECT-STATUS-AND-ONBOARDING.md`.
- [ ] Confirm Dockerfile builds and `server` command starts on port 8080.
- [ ] Write `curl` smoke script in `tools/smoke-server.sh` for CI use.
- [ ] Review `web/index.html` for information leakage (no memory contents, no tool inputs visible in browser storage).
- [ ] Confirm SSE stream does not include raw permission decisions in plain text.
- [ ] Write test: DELETE session while agent goroutine is running cancels the run cleanly.
- [ ] Confirm all `sync.Map` usages are race-detector clean in extended test run.
- [ ] Add `--read-timeout` and `--write-timeout` flags for `http.Server` with sensible defaults (30s read, 120s write for SSE).
- [ ] Document API endpoints in inline Go doc comments on handlers.
- [ ] Document session lifecycle state machine in `session.go` with ASCII art or mermaid comment.
- [ ] Verify `go test -count=3 -race ./internal/server/...` is stable (no intermittent failures).

## Acceptance Criteria

Resolution summary (2026-05-19):

- `done`: all criteria that can be proven via unit/integration tests, race checks, vet checks, and direct CLI behavior checks in this environment.
- `blocked`: criteria requiring live Ollama output in SSE/browser.
- `blocked`: criteria requiring Docker daemon runtime validation.
- `blocked`: criterion requiring `gosec` availability.

Legacy acceptance checklist (kept for traceability):

- [ ] `nandocodego server` starts on port 8080 by default with a log line confirming the bind address.
- [ ] `POST /v1/sessions` creates a new session and returns 201 with a unique `session_id`.
- [ ] `GET /v1/sessions/{id}/events` opens an SSE stream and emits `session_ready` immediately.
- [ ] `POST /v1/sessions/{id}/messages` with `{"prompt": "What is 2+2?"}` returns 202 Accepted.
- [ ] `POST /v1/sessions/{id}/messages` with duplicate `message_id` does not start a duplicate run.
- [ ] Server message POST uses the same current-turn context packing and evidence-pack metadata as TUI/`--print`.
- [ ] Too-large `@file`/`@dir` evidence returns a structured split-guidance error and does not call the runner.
- [ ] SSE stream emits at least one `assistant_text_delta` event in response to the above prompt.
- [ ] SSE stream emits a `terminal` event with `reason: completed` when the agent finishes.
- [ ] `GET /v1/sessions/{id}/events` with `Last-Event-ID: 2` replays events 3 onward from the ring buffer.
- [ ] `POST /v1/sessions/{id}/messages` returns 409 Conflict when a run is already active.
- [ ] Permission requests from tool use emit `permission_request` SSE events.
- [ ] `POST /v1/sessions/{id}/permissions/{req_id}` with `{"decision": "allow"}` unblocks the agent.
- [ ] Permission auto-deny fires after 30 seconds with no client response.
- [ ] `DELETE /v1/sessions/{id}` cancels any running agent, removes the session, returns 204.
- [ ] `GET /v1/sessions/{id}` after DELETE returns 404.
- [ ] `GET /v1/health` returns `{"status":"ok","ollama":"reachable"}` when Ollama is up.
- [ ] `GET /v1/models` returns a list of available models from Ollama.
- [ ] Bearer token auth: requests without correct `Authorization: Bearer` return 401 when a token is configured.
- [ ] Binding to a non-loopback address without `--token` produces a startup error.
- [ ] Concurrent-session cap: the 11th session creation attempt returns an appropriate error.
- [ ] Per-IP rate limit: more than 100 requests per minute from the same IP returns 429 Too Many Requests.
- [ ] Sessions idle for 30 minutes are expired and removed from the registry.
- [ ] `go test -race ./internal/server/...` passes with zero race conditions.
- [ ] `tools/check-allowed-deps.sh` passes (no new external dependencies added).
- [ ] `tools/check-network-policy.sh` passes; if listener-surface checks were added, they are scoped to the intentional `internal/server` listen path.
- [ ] Graceful shutdown: SIGINT causes all active agent goroutines to cancel within 10 seconds.
- [ ] `web/index.html` is served at `GET /` and displays a functional message input.
- [ ] Embedded web UI receives SSE events and renders streaming text.
- [ ] `--no-ui` flag causes `GET /` to return 404.
- [ ] `go test ./...` passes with no regressions in any existing package.
- [ ] `docs/PHASE-LOG.md` has a Phase 21 entry with files, decisions, and exit gate status.
- [ ] `DOCKER_WEB_GUIDE.md` documents the implemented API endpoints.
- [ ] Docker container (`docker build . && docker run -p 8080:8080 <image> server`) serves the API on port 8080.
- [ ] `gosec ./internal/server/...` produces zero findings if `gosec` is available; otherwise the phase log records that it was skipped.

## Forbidden

- WebSocket as the primary streaming transport. Use SSE.
- New external dependencies in `go.mod`. Use stdlib only for `internal/server`.
- A second agent loop, tool execution loop, permission resolver, or prompt-packing path in `internal/server`.
- Logging memory contents, tool inputs, tool outputs, or conversation text at INFO level or above.
- Serving raw model output in HTTP response headers.
- Auto-approving permission requests server-side without client input.
- Sessions that can outlive the server process (no persistent session storage in Phase 21).
- TLS termination in the Go server (document that reverse proxy handles TLS).
- Any `net.Listen` call outside `internal/server/server.go`.
- Sharing state between sessions (each session is fully isolated).
- Treating the embedded web UI as a security boundary (it is convenience, not security).
- Treating leading slash prompts as TUI slash commands over `/v1/sessions/{id}/messages`.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| SSE connection drops cause event loss | High | Ring buffer replay via `Last-Event-ID` (200 events). |
| Slow SSE clients block agent goroutine | High | Non-blocking broadcast to subscriber channels; slow clients miss events (can replay). |
| Session goroutines leak on unclean client disconnect | High | Session sweeper with 30-minute idle timeout; context cancel on DELETE. |
| Bearer token in URL query parameter (accidental) | Medium | Require token in `Authorization` header only; never accept it as query param. |
| Permission auto-deny surprises user | Medium | Document 30-second timeout prominently; emit `permission_request` with countdown data. |
| Memory content visible in browser (embedded UI) | Medium | UI displays response text only; tool inputs and memory context never rendered. |
| Concurrent-session cap too low for CI/CD automation | Low | Make cap configurable via `--max-sessions` flag; default 10. |
| SSE stream content-type rejected by some proxies | Low | Set `X-Accel-Buffering: no` for nginx; document proxy configuration. |
| `check-network-policy.sh` update breaks false-positive suppression for other packages | Medium | Scope the allowlist narrowly to `internal/server/server.go`; review script diff carefully. |

## Phase Log Template

When implementation finishes, append a Phase 21 entry to `docs/PHASE-LOG.md` with:

- objective;
- files created and updated;
- dependencies added (expected: none);
- tests and checks run;
- manual smoke result;
- design decisions (SSE vs WebSocket, ring buffer capacity, auth model);
- known constraints and deferred work (TLS, persistent sessions, full web UI);
- exit gate status.

## Exit Gate

Phase 21 is complete only when:

- all acceptance criteria above are checked off;
- `go test -race ./internal/server/...` and `go test ./...` pass;
- `tools/check-allowed-deps.sh` and `tools/check-network-policy.sh` pass;
- `gosec ./internal/server/...` is clean if `gosec` is available, or the phase log records that it was not installed;
- the manual smoke flow (create session, send message, observe SSE, delete session) completes end to end with a live Ollama model;
- `DOCKER_WEB_GUIDE.md` and `docs/PHASE-LOG.md` are updated;
- `docs/PROJECT-STATUS-AND-ONBOARDING.md` reflects Phase 21 as complete.
