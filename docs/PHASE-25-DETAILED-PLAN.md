# Phase 25 Detailed Plan — Remote / Bridge Mode (Required v0.1)

Date: 2026-05-07
Status: Final plan and implementation checklist; ready to start after Ollama Cloud API key support completion review on 2026-05-22
Source plans:

- `.codex/go-ollama-plan-AGENTS.md`
- `.codex/go-ollama-plan-HUMANS.md`
- `docs/PHASE-LOG.md`
- `docs/PROJECT-STATUS-AND-ONBOARDING.md`
- `docs/PHASE-8-DETAILED-PLAN.md`
- `docs/PHASE-24-DETAILED-PLAN.md`
- `book/ch16-remote.md`

## Roadmap Precondition - 2026-05-22

`docs/OLLAMA-CLOUD-API-KEY-PLAN.md` is complete. Phase 25 should build on the resulting provider/runtime-client shape instead of introducing a second model-routing path.

## Implementation Readiness Review - 2026-05-21

Status: ready to start. Ollama Cloud API key support has landed, and no blocking issues remain in the Phase 22 large-file prompt path for Phase 25 planning.

Current source baseline:

- `nandocodego server` exists and exposes Phase 21 HTTP/SSE routes.
- Existing session routes use `POST /v1/sessions/{id}/messages`, not `/input`; Phase 25 should extend the existing route instead of adding a parallel input endpoint.
- `internal/server/ringbuffer.go` already provides an in-memory per-session event buffer with `Last-Event-ID` replay in tests.
- `internal/server/recentids.go` already provides bounded message-id dedupe for submitted messages.
- `internal/server/auth.go` currently implements optional opaque bearer-token auth; Phase 25 replaces or wraps this with JWT auth.
- `POST /v1/sessions/{id}/permissions/{reqID}` and `DELETE /v1/sessions/{id}` already exist; Phase 25 hardens them with JWT/session-state semantics.
- Sessions are still in-memory and use `ready/running/closing`; Phase 25 adds persisted `starting/running/detached/stopping/stopped` metadata.
- There is no `connect` command, no `remote_bridge.go`, no JWT implementation, no detached session store, no gap-event replay behavior, and no UDS server listener yet.

Plan corrections from this review:

- Token retrieval must happen at server startup. A separate `nandocodego server --print-token` command after the server is already running cannot work with an in-memory signing secret unless a persistent admin channel is added, which is out of scope.
- Keep the existing `/messages` route as the write endpoint. Do not introduce `/input` as a second spelling in Phase 25.
- Reuse and extend existing `RingBuffer` and `RecentIDs` where they fit; add new types only when Phase 25 needs semantics they do not provide, such as replay gap detection or delivered-event dedupe.
- Treat the recent context-packing `--print` and server tests as part of the server-entry-point baseline. Phase 25 should not bypass `contextpack.BuildCurrentTurnPrompt` when routing remote messages.

## Roadmap Placement

Phase 25 is required v0.1 work. It depends on Phase 21 server mode, Phase 24 multi-agent coordination, and the completed Ollama Cloud API key support workstream. It must be implemented before Phase 17 and Phase 18.

Historical baseline sections below may describe Phase 17 and Phase 18 as already implemented because this plan was originally written in numeric order. Under the current roadmap, Phase 17 and Phase 18 are final and must not be started until Phase 25 is complete.

## Goal

Phase 25 enables `nandocodego` to run as a server in a remote environment — a container, CI runner, cloud VM, or a developer's home workstation — while users interact with it through a browser or a separate `nandocodego connect` CLI session. The agent loop runs where the code lives; the user interface runs where the user is.

This builds directly on Phase 21 (HTTP server with SSE streaming) by adding:

- Session persistence: agent goroutines continue running when the client disconnects.
- Reconnection handling: clients replay missed events from a ring-buffer offset on reconnect.
- JWT authentication: short-lived signed tokens with proactive refresh and epoch management.
- Bounded event deduplication during reconnection, reusing `RecentIDs` if it is sufficient.
- `nandocodego connect <url>` CLI command: runs the TUI locally, agent remotely.
- `internal/tui/remote_bridge.go`: TUI adapter that forwards input to the remote server and renders SSE events.

The design follows the ch16 principle: reads are persistent connections (SSE), writes are HTTP POST. Failures degrade gracefully. The agent's core loop is agnostic about transport.

## Definition of Success

The Phase 25 exit gate is an eight-step flow:

1. Start server in a remote environment (or second terminal): `nandocodego server --port 8080 --print-token`.
2. Capture the startup connect token printed by that server process.
3. Connect from client terminal: `nandocodego connect http://localhost:8080 --token <jwt>`.
4. Type a prompt in the client TUI, confirm the agent runs on the server, and the result streams back to the client.
5. Disconnect the client (Ctrl-C), wait 5 seconds, reconnect: `nandocodego connect http://localhost:8080 --token <jwt> --session <id>`.
6. Confirm: agent continued running during disconnection (detached state) and buffered events replay correctly on reconnect.
7. JWT expiry: let token expire (or artificially set short TTL in test), confirm client refreshes transparently.
8. `go test -race ./internal/server/...` passes.

## Baseline Analysis from Implemented Phases

### Phase 0 — Security and Supply Chain

Implemented:

- Dependency allowlist, network policy checker, CI security baseline.
- No-secrets policy.

Phase 25 implications:

- JWT tokens are secrets. They must never be logged, never written to disk in plaintext beyond the temporary token file, and never included in error messages.
- The JWT signing secret is a per-server random 32-byte key generated at startup and stored only in memory. It is never written to disk.
- `prctl(PR_SET_DUMPABLE, 0)` is Linux-only. The Go standard library does not expose it directly; use `syscall.Syscall(syscall.SYS_PRCTL, ...)` on Linux with a `//go:build linux` build tag. On other platforms, use a no-op. Test that the build tag compiles correctly on Linux and is skipped on macOS/Windows.
- The temporary token file (`~/.nandocodego/sessions/<id>/token.tmp`) must be written with `0600` permissions and unlinked immediately after the process reads it into memory. This is for the startup case where the server writes the token for a child process; in the interactive server case, the token is printed to stdout.
- New dependency required: `github.com/golang-jwt/jwt/v5` for JWT signing and verification. Must be added to `tools/allowed-deps.txt`.
- UDS sockets from Phase 24 are now also used by the server to receive messages from `SendMessage` routed via `uds:` (Phase 25 implements the server side that Phase 24 skipped).
- Session metadata JSON files contain no secrets (no tokens); they contain session state, timestamps, and working directory.

### Phase 1 — CLI, Paths, Logging, Scaffold

Implemented:

- `internal/paths`, `nandocodego doctor`.

Phase 25 implications:

- Add `paths.ServerSessionsDir() string` — returns `paths.DataDir() + "/sessions"`.
- Add `paths.SessionMetadataPath(sessionID string) string` — `sessions/<id>/metadata.json`.
- Add `paths.SessionTokenPath(sessionID string) string` — `sessions/<id>/token.tmp` (for startup token handoff only).
- Add `paths.ServerSocketPath(sessionID string) string` — for UDS server socket.
- `nandocodego connect` is a new subcommand in `internal/cli/connect.go`.
- `nandocodego server` already exists from Phase 21; Phase 25 extends it.
- `nandocodego doctor` gains a server section reporting the sessions directory and active session count.

### Phase 2 — LLM Client

Implemented:

- `llm.Client`, streaming chat, watchdog, retry.

Phase 25 implications:

- Remote sessions use the same `llm.Client` as local sessions. The server's `llm.Client` is configured at server startup via `config.toml`; the client does not specify or influence the model.
- Network policy: the server's LLM traffic goes to the configured Ollama endpoint for v0.1. OpenAI-compatible provider work is not part of the active roadmap.

### Phase 3–9 (Tools, Agent, Permissions, State, TUI, Memory, Hooks)

Phase 25 implications for each:

- **Tools**: All tool execution runs on the server. The client `connect` command does NOT execute tools. This is the fundamental asymmetry of remote mode.
- **Agent**: `agent.Agent.Run` executes entirely on the server. Events are serialized as SSE and streamed to the client.
- **Permissions**: Permission prompts generated on the server are SSE-streamed to the client as a special event type. The client's `remote_bridge.go` renders the permission modal locally and sends the user's decision as an HTTP POST to the server. The server's `permissions.PromptFunc` blocks until the decision arrives.
- **State**: The server maintains `state.Store[state.App]` per session. The client does not have its own state store; it renders from SSE event stream only.
- **TUI**: `internal/tui/remote_bridge.go` replaces the local agent runner with an HTTP+SSE transport layer. From the Bubble Tea model's perspective, it receives the same `agentEventMsg` types, but they originate from SSE deserialization rather than direct goroutine events.
- **Memory**: Memory runs on the server (where the files are). The client is unaware of memory operations.
- **Hooks**: Hooks run on the server. Command hooks execute in the server's process environment.

### Phase 10–13 (MCP, Sub-agents, Skills, Commands)

All execute on the server. The client is a thin rendering layer.

### Phase 14 — Tasks and TaskSupervisor

Implemented:

- Task supervisor, JSONL output files.

Phase 25 implications:

- Background tasks (from Phase 14) continue running in the server during client disconnection (detached state). The `TaskSupervisor` is per-session; it stays alive as long as the session is alive.
- Session detach/reattach does NOT affect running background tasks.
- Task notifications continue to be buffered in the event ring buffer during detachment.

### Phase 15 — Concurrency

Implemented:

- Tool partitioning, concurrent tool execution.

Phase 25 implications:

- No changes. Concurrent tool execution happens on the server.

### Phase 16 — Observability

Implemented:

- Logging and metric decorators.

Phase 25 implications:

- Add server-side metrics: `server_sessions_active`, `server_sessions_detached`, `server_sse_reconnects`, `server_jwt_refreshes`, `server_jwt_rejections`.
- Connection log events at INFO: `session_created`, `session_connected`, `session_detached`, `session_reconnected`, `session_stopped`.
- Do NOT log JWT token values, session tokens, or auth headers at any level.

### Phase 17 — Distribution

Roadmap note: Phase 17 is not implemented yet under the current roadmap. It must package Phase 25 because remote/bridge mode is required for v0.1.

Phase 25 implications:

- `prctl` build tag requires that the Linux binary compiles correctly. Add `GOOS=linux GOARCH=amd64` cross-compilation test to CI for the server package.
- `nandocodego connect` is part of the same binary; no separate artifact needed.

### Phase 18 — Hardening

Roadmap note: Phase 18 is not implemented yet under the current roadmap. It must harden and document Phase 25 because this phase is required for v0.1.

Phase 25 implications:

- Phase 25 adds an end-to-end test that exercises the full connect-detach-reconnect cycle.

### Phases 19–22 (HTTP Server, SSE, Session Lifecycle)

Implemented:

- `nandocodego server` HTTP server.
- SSE event streaming.
- Session creation, input handling.
- Phase 21 bearer token authentication (basic).

Phase 25 extends Phase 21:

- Phase 21 sessions are in-memory only. Phase 25 persists to disk.
- Phase 21 bearer tokens are simple opaque strings. Phase 25 replaces with signed JWTs.
- Phase 21 already has basic per-session event buffering and `Last-Event-ID` replay. Phase 25 adds persisted replay metadata, gap detection, detached session state, and client reconnect policy.
- Phase 21 sessions remain alive while the server process is alive, but there is no explicit detached lifecycle or persisted metadata. Phase 25 makes detachment first-class.
- Phase 21 has no durable reconnect protocol. Phase 25 defines reconnect behavior across client disconnects, ring-buffer gaps, token expiry, and server restarts.

### Provider Scope

Phase 25 implications:

- The remote server owns LLM configuration and uses the existing Ollama-backed client path for v0.1.
- The client's `nandocodego connect` command does not need any LLM configuration.
- OpenAI-compatible adapter work is intentionally excluded from the active v0.1 roadmap.

### Phase 24 — Multi-Agent Coordination

Implemented:

- Mailbox, SendMessage, coordinator mode, UDS client.

Phase 25 implications:

- Phase 24 implemented the `SendMessage` client side for `uds:` routing. Phase 25 implements the server side: `internal/server/session_store.go` registers UDS socket paths for active sessions.
- `bridge:` routing in `SendMessage` (Phase 24 returned `ErrNotSupported`) now routes to the Phase 25 bridge relay for cross-session communication on the same server.
- Phase 25 does NOT implement cross-machine bridge (Anthropic Remote Control). Track that as a future remote-control follow-up if needed.

## Deep Analysis of ch16-remote.md

### Asymmetric Transport Design

Chapter 16 establishes the core design principle: reads are persistent connections (SSE), writes are HTTP POST. Phase 25 follows this exactly:

- Server → Client: SSE stream (`GET /v1/sessions/{id}/events`).
- Client → Server: HTTP POST to `/v1/sessions/{id}/messages`.
- Permission decisions: HTTP POST to `/v1/sessions/{id}/permissions/{requestID}`.
- Token refresh: POST to `/v1/sessions/{id}/token/refresh`.

### Session State Machine

Five states from ch16, adapted for this repo:

```
starting → running → detached → running (on reconnect)
                  ↘ stopping → stopped
running → stopping → stopped
```

| State | Description |
|-------|-------------|
| `starting` | Session created, agent goroutine launching |
| `running` | Client connected, agent active |
| `detached` | Client disconnected, agent goroutine still running |
| `stopping` | Session received stop signal, goroutine winding down |
| `stopped` | Session terminal, goroutine exited |

Metadata persists to disk on every state transition. On server restart, sessions in `running` or `detached` state are loaded but cannot be resumed (agent goroutine is gone). They are moved to `stopped` and reported as "interrupted" to reconnecting clients.

### Delivered-Event Deduplication

Chapter 16 uses a bounded UUID set for event deduplication. This repo already has `internal/server/recentids.go` for bounded FIFO dedupe of submitted message IDs. Phase 25 should reuse or extend that helper unless reconnect delivery needs a distinct API.

```go
// internal/server/recentids.go or internal/server/dedup.go

type EventDeduper interface {
    SeenOrAdd(id string) bool
}
```

Default capacity: 2000. Use separate dedupe state for sent IDs and delivered IDs if reconnect behavior needs both.

### JWT Authentication

Phase 25 uses `github.com/golang-jwt/jwt/v5` for HS256 signing:

```go
// internal/server/jwt.go

type ServerClaims struct {
    SessionID string `json:"sid"`
    Epoch     int64  `json:"epoch"`
    jwt.RegisteredClaims
}

// Sign creates a new JWT for the session.
// TTL defaults to 1 hour.
func Sign(sessionID string, epoch int64, secret []byte, ttl time.Duration) (string, error)

// Verify validates the token and returns claims.
// Returns ErrExpired, ErrEpochMismatch, or ErrInvalidToken on failure.
func Verify(tokenString string, secret []byte, expectedEpoch int64) (*ServerClaims, error)
```

Epoch management:
- Server stores current epoch per session in `SessionMetadata.Epoch`.
- Token refresh increments the epoch atomically.
- Old tokens (stale epoch) return 409 Conflict.
- The 409 response body includes the message "epoch mismatch — re-authenticate".
- On 409, the client must stop the current remote session attempt and ask the user to run `nandocodego connect` again with a fresh startup token.

Server secret:
- Generated at startup: `crypto/rand.Read(32 bytes)`.
- Stored only in memory (never written to disk).
- Server restart invalidates all existing tokens (sessions move to `stopped` on reconnect).

### Event Ring Buffer

Each session has a ring buffer of recent SSE events for replay:

```go
// internal/server/ringbuffer.go

type EventRingBuffer struct {
    mu     sync.RWMutex
    buf    []SSEEvent
    head   int
    count  int
    cap    int
    lastID int64
}

type SSEEvent struct {
    ID      int64
    Type    string
    Data    string  // JSON-encoded agent event
}

func NewEventRingBuffer(capacity int) *EventRingBuffer

func (r *EventRingBuffer) Append(eventType, data string) int64  // returns assigned ID
func (r *EventRingBuffer) Since(lastID int64) []SSEEvent        // events with ID > lastID
func (r *EventRingBuffer) LastID() int64
```

Default ring buffer capacity: 500 events per session. This covers approximately 5 minutes of typical streaming output.

Clients send `Last-Event-ID` header on reconnect. The server calls `ring.Since(lastID)` and replays the events before live streaming resumes.

### Reconnection Protocol

Client-side reconnect logic (in `internal/tui/remote_bridge.go`):

| Failure type | Strategy |
|---|---|
| 4003 Unauthorized | Stop immediately, no retries — invalid or expired token |
| 4001 Not Found | Max 3 retries with 1s/2s/4s backoff — session may have expired |
| HTTP 409 Conflict | Stop, re-authenticate — epoch mismatch |
| Other transient (5xx, network) | Exponential backoff, max 5 retries (1s→2s→4s→8s→16s cap) |
| 200 SSE disconnected (EOF) | Immediate reconnect with Last-Event-ID, max 10 times |

The client always sends `Last-Event-ID` on reconnect (the ID of the last event it successfully received). If the server's ring buffer no longer has events that old, it sends a `gap` event notifying the client that some events were missed.

### Permission Forwarding

Server-side permission prompts flow to the client as SSE events:

```json
{
  "type": "permission_request",
  "requestID": "perm-abc123",
  "toolName": "Bash",
  "description": "Run: ls -la /etc",
  "permissionSuggestions": ["allow", "deny", "always_allow"]
}
```

The client renders a permission modal (same as local mode) and sends the decision as HTTP POST:

```
POST /v1/sessions/{id}/permissions/{requestID}
{"decision": "allow"}
```

The server's `permissions.PromptFunc` blocks the agent goroutine until the decision arrives (same pattern as Phase 7's TUI permission broker, but over HTTP instead of Bubble Tea messages).

## Evaluation of the Original Phase 25 Concept

The original concept is correct at the product level. It needs more implementation detail:

- It does not specify how the server secret is generated or how server restart invalidates tokens. Phase 25: random 32-byte key in memory, restart = new key, all sessions become `stopped`.
- It does not specify the ring buffer capacity or what happens when the buffer overflows (client is too far behind). Phase 25: 500-event capacity; overflow sends a `gap` event to the client.
- It does not specify the JWT library. Phase 25: `github.com/golang-jwt/jwt/v5` (add to allowlist).
- It does not specify whether `nandocodego connect` needs its own config file or reuses the main config. Phase 25: reuses main config for LLM settings but ignores them (server owns the model); connect command only needs `--token` and `--session` flags.
- It does not specify how permission prompts are forwarded. Phase 25: SSE event + HTTP POST decision.
- It does not specify how the client discovers its session ID for `--session` flag. Phase 25: printed to stdout on first connect.

## Final Phase 25 Scope

In scope:

- `internal/server/session_store.go` — persistent session metadata, state machine.
- `internal/server/jwt.go` — JWT sign/verify with epoch management.
- `internal/server/dedup.go` or extension of `recentids.go` — delivered-event dedupe with bounded memory.
- Extension of `internal/server/ringbuffer.go` — SSE event ring buffer with oldest/latest IDs and gap detection.
- `internal/server/reconnect.go` — reconnection protocol constants and client-side retry logic (shared by `remote_bridge.go`).
- `internal/cli/connect.go` — `nandocodego connect` command.
- `internal/tui/remote_bridge.go` — TUI adapter for remote sessions.
- `prctl(PR_SET_DUMPABLE, 0)` call on Linux server startup.
- Startup token printing via `nandocodego server --print-token`; token file write+unlink only if a child-process token handoff is implemented.
- UDS server socket registration for Phase 24 `SendMessage` `uds:` routing.
- Phase 21 HTTP server extensions: detached session handling, ring buffer integration, JWT middleware.
- Tests and Phase log update.

Out of scope:

- Cross-machine bridge relay (`bridge:` routing to another machine; future remote-control follow-up if needed).
- WebSocket transport (SSE is sufficient for Phase 25; WebSocket is a future optimization).
- Multi-user auth (one server secret, one session per server instance for Phase 25).
- OAuth2 for client authentication.
- Session sharing between multiple server instances, which requires an external state store.
- Upstream proxy for credential injection in containers (ch16 section 4).
- Mobile client.
- Browser-based client UI beyond the Phase 21 minimal UI.

## Target UX

### Server

```sh
# Start server and print a startup connect token
nandocodego server --port 8080 --print-token

# Server output on start:
# [server] listening on :8080
# [server] connect token: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
# [server] run `nandocodego connect http://localhost:8080 --token <above>` to connect
```

### Client

```sh
# Connect to remote server
nandocodego connect http://remote-host:8080 --token <jwt>

# Reconnect to existing session
nandocodego connect http://remote-host:8080 --token <jwt> --session <session-id>

# On first connect, session ID is printed:
# [connect] connected to session s7f3k9p2
# [connect] to reconnect: nandocodego connect http://remote-host:8080 --token <jwt> --session s7f3k9p2
```

### Doctor Output

```
Server Sessions:
  Sessions dir: /Users/fernando/.local/share/nandocodego/sessions
  Active sessions: 2
  Detached sessions: 1
```

## Architecture

### Package Layout

```text
internal/server/
  session_store.go     — session metadata persistence and state machine
  jwt.go               — JWT sign/verify, epoch management
  dedup.go             — delivered-event dedupe if RecentIDs is not sufficient
  ringbuffer.go        — existing SSE event buffer, extended for gap detection
  reconnect.go         — reconnect protocol constants, retry helper
  session_store_test.go
  jwt_test.go
  dedup_test.go
  ringbuffer_test.go
  reconnect_test.go
  prctl_linux.go       — prctl(PR_SET_DUMPABLE, 0) implementation
  prctl_other.go       — no-op for non-Linux

internal/cli/
  connect.go           — nandocodego connect command

internal/tui/
  remote_bridge.go     — TUI adapter for remote sessions
```

### Core Types

```go
// internal/server/session_store.go

type SessionState string

const (
    SessionStarting SessionState = "starting"
    SessionRunning  SessionState = "running"
    SessionDetached SessionState = "detached"
    SessionStopping SessionState = "stopping"
    SessionStopped  SessionState = "stopped"
)

type SessionMetadata struct {
    SessionID  string       `json:"session_id"`
    State      SessionState `json:"state"`
    CreatedAt  time.Time    `json:"created_at"`
    LastActive time.Time    `json:"last_active"`
    AgentID    string       `json:"agent_id,omitempty"`
    Model      string       `json:"model,omitempty"`
    WorkingDir string       `json:"working_dir,omitempty"`
    Epoch      int64        `json:"epoch"`
}

type SessionStore struct {
    mu       sync.RWMutex
    sessions map[string]*sessionEntry
    dir      string
    logger   *slog.Logger
}

type sessionEntry struct {
    metadata SessionMetadata
    ring     *EventRingBuffer
    sentIDs        EventDeduper
    deliveredIDs   EventDeduper
    agentCancel    context.CancelFunc
    permBroker     *server.PermissionBroker
    connectedCh    chan struct{}  // closed when client connects
    detachCh       chan struct{}  // closed when client disconnects
}

func NewSessionStore(dir string, logger *slog.Logger) (*SessionStore, error)
func (s *SessionStore) Create(ctx context.Context, model, workingDir string) (*SessionMetadata, error)
func (s *SessionStore) Get(sessionID string) (*SessionMetadata, bool)
func (s *SessionStore) Transition(sessionID string, to SessionState) error
func (s *SessionStore) BumpEpoch(sessionID string) (int64, error)
func (s *SessionStore) LoadFromDisk(ctx context.Context) error  // called on server start
func (s *SessionStore) AppendEvent(sessionID, eventType, data string) (int64, error)
func (s *SessionStore) EventsSince(sessionID string, lastID int64) ([]SSEEvent, error)
func (s *SessionStore) AddDedup(sessionID, uuid string)
func (s *SessionStore) HasDedup(sessionID, uuid string) bool
```

```go
// internal/server/jwt.go

type ServerClaims struct {
    SessionID string `json:"sid"`
    Epoch     int64  `json:"epoch"`
    jwt.RegisteredClaims
}

const DefaultTokenTTL = 1 * time.Hour

func Sign(sessionID string, epoch int64, secret []byte, ttl time.Duration) (string, error)
func Verify(tokenString string, secret []byte, expectedEpoch int64) (*ServerClaims, error)

var (
    ErrExpired      = errors.New("jwt: token expired")
    ErrEpochMismatch = errors.New("jwt: epoch mismatch")
    ErrInvalidToken  = errors.New("jwt: invalid token")
)
```

```go
// internal/server/recentids.go or internal/server/dedup.go

type EventDeduper interface {
    SeenOrAdd(id string) bool
}
```

```go
// internal/server/ringbuffer.go

type SSEEvent struct {
    ID   int64  `json:"id"`
    Type string `json:"type"`
    Data string `json:"data"`
}

type EventRingBuffer struct {
    mu     sync.RWMutex
    buf    []SSEEvent
    head   int
    count  int
    cap    int
    lastID int64
}

func NewEventRingBuffer(capacity int) *EventRingBuffer
func (r *EventRingBuffer) Append(eventType, data string) int64
func (r *EventRingBuffer) Since(lastID int64) []SSEEvent
func (r *EventRingBuffer) LastID() int64
```

```go
// internal/server/reconnect.go

const (
    MaxRetriesUnauthorized = 0
    MaxRetriesNotFound     = 3
    MaxRetriesTransient    = 5
    MaxRetriesEOF          = 10
)

type RetryPolicy struct {
    Code       int
    MaxRetries int
    Backoff    BackoffStrategy
}

type BackoffStrategy int
const (
    BackoffNone        BackoffStrategy = iota
    BackoffLinear      // 1s, 2s, 4s
    BackoffExponential // 1s, 2s, 4s, 8s, 16s cap
)

func ReconnectPolicyFor(httpStatus int) RetryPolicy
```

```go
// internal/tui/remote_bridge.go

// RemoteBridge implements the AgentRunner interface from Phase 7 but
// forwards input to a remote server via HTTP POST and receives events
// via SSE, converting them to agent.Event values for the TUI.

type RemoteBridge struct {
    serverURL  string
    sessionID  string
    token      string
    httpClient *http.Client
    logger     *slog.Logger
    lastEventID int64
    sentIDs     server.EventDeduper
}

func NewRemoteBridge(serverURL, sessionID, token string, logger *slog.Logger) *RemoteBridge

// Run satisfies the AgentRunner interface.
// It opens an SSE connection, converts events, and sends them to the channel.
// It does NOT call the local agent — it receives agent events from the server.
func (b *RemoteBridge) Run(ctx context.Context, input agent.Input) <-chan agent.Event
```

```go
// internal/cli/connect.go

// connectOptions holds connect command configuration.
type connectOptions struct {
    serverURL string
    token     string
    sessionID string
}

// runConnect opens the TUI connected to a remote session.
func runConnect(ctx context.Context, cmd *cobra.Command, opts connectOptions) error
```

### HTTP Server Extensions (Phase 21 additions)

New endpoints added to the Phase 21 server:

```
GET  /v1/sessions/{id}/events              — SSE stream (auth required)
POST /v1/sessions/{id}/token/refresh       — token refresh, bumps epoch
POST /v1/sessions/{id}/permissions/{reqID} — permission decision from client
GET  /v1/sessions                          — list sessions (auth required)
GET  /v1/sessions/{id}                     — session metadata (auth required)
DELETE /v1/sessions/{id}                   — stop session (auth required)
```

Existing Phase 21 endpoints extended:

```
POST /v1/sessions                          — create session; now returns JWT
POST /v1/sessions/{id}/messages            — input message; now auth-required
```

JWT middleware:

```go
func JWTMiddleware(store *SessionStore, secret []byte) func(http.Handler) http.Handler
```

Applied to all `/v1/sessions/*` routes except `POST /v1/sessions` (session creation authenticates differently).

### prctl on Linux

```go
// internal/server/prctl_linux.go
//go:build linux

package server

import "syscall"

const prSetDumpable = 4

// DisableCoreDump calls prctl(PR_SET_DUMPABLE, 0) to prevent ptrace heap inspection.
// Returns an error if the syscall fails; callers should log and continue (fail open).
func DisableCoreDump() error {
    _, _, errno := syscall.Syscall(syscall.SYS_PRCTL, prSetDumpable, 0, 0)
    if errno != 0 {
        return errno
    }
    return nil
}
```

```go
// internal/server/prctl_other.go
//go:build !linux

package server

// DisableCoreDump is a no-op on non-Linux platforms.
func DisableCoreDump() error { return nil }
```

Called once at server startup in `nandocodego server` command, before any tokens are generated.

## Implementation Plan

### Step 1 — Paths Helpers

Files:

- `internal/paths/paths.go` (extend existing)

Implement:

- `paths.ServerSessionsDir() string`.
- `paths.SessionMetadataPath(sessionID string) string`.
- `paths.SessionTokenPath(sessionID string) string`.
- `paths.ServerSocketPath(sessionID string) string`.

Tests:

- All new path helpers return paths under `DataDir()`.
- `SessionMetadataPath` and `SessionTokenPath` include `sessionID` in the path.

### Step 2 — Delivered-Event Deduplication

Files:

- `internal/server/recentids.go` or `internal/server/dedup.go`
- `internal/server/recentids_test.go` or `internal/server/dedup_test.go`

Implement delivered-event dedupe with bounded memory. Prefer extending the existing `RecentIDs` helper if its FIFO semantics are sufficient; create a separate `BoundedUUIDSet` only if reconnect delivery needs a different API or independent capacities for sent and delivered event IDs.

Tests:

- Empty set `Has` returns false.
- `Add` then `Has` returns true.
- Adding `capacity + 1` items evicts the oldest.
- After eviction, oldest item `Has` returns false.
- New item `Has` returns true after eviction.
- Concurrent `Add` and `Has` are race-free (requires external mutex if not self-protecting).
- If a new `BoundedUUIDSet` is created and is not internally synchronized, document that callers hold the session-level lock.
- Test total memory usage with 2000-capacity set contains exactly 2000 map entries after filling.

### Step 3 — JWT Signing and Verification

Files:

- `internal/server/jwt.go`
- `internal/server/jwt_test.go`

Implement `Sign` and `Verify` with `github.com/golang-jwt/jwt/v5`.

Tests:

- `Sign` produces a parseable HS256 JWT.
- `Verify` accepts a valid, non-expired token.
- `Verify` returns `ErrExpired` for expired token (TTL = 1ms, sleep 2ms).
- `Verify` returns `ErrEpochMismatch` for correct token with wrong epoch.
- `Verify` returns `ErrInvalidToken` for tampered token.
- `Verify` returns `ErrInvalidToken` for token signed with wrong secret.
- Token value does not appear in any log output (scan logger in test).
- `Sign` with empty `sessionID` returns error.

### Step 4 — Event Ring Buffer

Files:

- `internal/server/ringbuffer.go`
- `internal/server/ringbuffer_test.go`

Extend the existing generic `RingBuffer` or wrap it with Phase 25 replay metadata.

Tests:

- `Append` returns monotonically increasing IDs.
- `Since(0)` returns all events.
- `Since(lastID)` returns only events with ID > lastID.
- After capacity overflow, oldest events are evicted.
- `Since` with an evicted lastID returns all remaining events (gap scenario).
- `LastID` returns ID of most recent event or 0 if empty.
- Concurrent `Append` and `Since` are race-free (RWMutex internal).
- 500-capacity buffer evicts correctly after 501 appends.

### Step 5 — Session Store

Files:

- `internal/server/session_store.go`
- `internal/server/session_store_test.go`

Implement `SessionStore` with disk persistence.

State transitions allowed:

```
starting → running (on SSE connect)
running → detached (on SSE disconnect)
detached → running (on SSE reconnect)
running → stopping (on DELETE /v1/sessions/{id} or context cancel)
detached → stopping (on DELETE or server shutdown)
stopping → stopped (on agent goroutine exit)
```

Disk persistence:

- Write `metadata.json` atomically (temp file + rename) on every state transition.
- `LoadFromDisk`: scan sessions directory on server start; sessions in `running`/`detached` state are transitioned to `stopped` with a note that the server restarted.
- Corrupt or unreadable `metadata.json` files are skipped with a WARN log.

Tests:

- `Create` writes `metadata.json` to disk.
- `Get` returns metadata from in-memory map.
- `Transition` updates metadata and writes to disk.
- `Transition` rejects illegal state transitions (e.g., `stopped → running`).
- `BumpEpoch` increments epoch and writes to disk.
- `LoadFromDisk` loads existing sessions.
- `LoadFromDisk` moves `running`/`detached` sessions to `stopped`.
- `LoadFromDisk` skips corrupt JSON files.
- Concurrent `Transition` and `Get` are race-free.

### Step 6 — Reconnect Protocol

Files:

- `internal/server/reconnect.go`
- `internal/server/reconnect_test.go`

Implement `ReconnectPolicyFor(httpStatus int) RetryPolicy`.

Tests:

- HTTP 4003 (mapped from bearer auth failure) → `MaxRetries: 0`.
- HTTP 4001 (mapped from 404 Not Found) → `MaxRetries: 3, Backoff: BackoffLinear`.
- HTTP 409 → `MaxRetries: 0` (re-authenticate required).
- HTTP 500 → `MaxRetries: 5, Backoff: BackoffExponential`.
- SSE EOF → reconnect immediately with Last-Event-ID (not covered by `ReconnectPolicyFor`; handled in `RemoteBridge`).

### Step 7 — prctl Build Tags

Files:

- `internal/server/prctl_linux.go`
- `internal/server/prctl_other.go`
- `internal/server/prctl_test.go`

Tests:

- `DisableCoreDump()` returns no error on Linux (verify in CI with `GOOS=linux`).
- `DisableCoreDump()` is a no-op and returns nil on macOS/Windows.
- Test that the build compiles on all three platforms (cross-compile check in CI).

### Step 8 — Server HTTP Extensions

Files:

- `internal/server/` (Phase 21 server files, extended)

Implement JWT middleware and new endpoints.

JWT middleware:

- Extracts `Authorization: Bearer <token>` header.
- Calls `jwt.Verify(token, secret, session.Epoch)`.
- Returns 401 on `ErrExpired` or `ErrInvalidToken`.
- Returns 409 on `ErrEpochMismatch`.
- Injects `*ServerClaims` into request context on success.

SSE endpoint (`GET /v1/sessions/{id}/events`):

- Set headers: `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `X-Accel-Buffering: no`.
- Read `Last-Event-ID` header from request; default to 0.
- Call `store.EventsSince(sessionID, lastID)` and replay buffered events.
- If ring buffer does not have events from `lastID`, send a `gap` SSE event.
- Transition session to `running` state.
- Subscribe to session's event channel for live events.
- On client disconnect: transition to `detached` state.

Token refresh endpoint (`POST /v1/sessions/{id}/token/refresh`):

- Verify existing token.
- Call `store.BumpEpoch(sessionID)`.
- Sign new token with new epoch.
- Return `{"token": "<new-jwt>"}`.

Permission decision endpoint (`POST /v1/sessions/{id}/permissions/{reqID}`):

- Parse `{"decision": "allow" | "deny" | "always_allow"}`.
- Look up pending permission request by `reqID`.
- Call `permBroker.Resolve(reqID, decision)`.
- Return 200 on success, 404 if `reqID` not found.

Tests:

- JWT middleware rejects missing auth header with 401.
- JWT middleware rejects expired token with 401.
- JWT middleware rejects epoch mismatch with 409.
- SSE endpoint replays buffered events after `Last-Event-ID`.
- SSE endpoint sends `gap` event when ring buffer overflowed.
- Session transitions to `detached` on SSE disconnect.
- Token refresh bumps epoch and returns new token.
- Old token rejected after epoch bump (409).
- Permission endpoint calls `permBroker.Resolve`.
- Permission endpoint returns 404 for unknown `reqID`.

### Step 9 — RemoteBridge TUI Adapter

Files:

- `internal/tui/remote_bridge.go`

Implement `RemoteBridge` satisfying the `AgentRunner` interface from Phase 7.

`RemoteBridge.Run(ctx, input)`:

1. Open SSE connection to `GET /v1/sessions/{id}/events?token=<jwt>` (or `Authorization` header).
2. Set `Last-Event-ID: <lastEventID>` header.
3. Start goroutine reading SSE events and converting to `agent.Event` values.
4. Return the `agent.Event` channel.

Input forwarding (`SendInput(ctx, content string) error`):

- POST to `POST /v1/sessions/{id}/messages` with JSON body `{"prompt": "<message>", "message_id": "<uuid>"}`.
- On failure, retry per `ReconnectPolicyFor(httpStatus)`.

Permission handling:

- When SSE event type is `permission_request`: extract fields, create local `agent.PermissionPrompt`, display via TUI permission modal.
- When user makes decision: POST to `/v1/sessions/{id}/permissions/{reqID}`.

Reconnect on SSE EOF:

- Send immediate reconnect with `Last-Event-ID` header.
- On 401: show error and stop.
- On 409: show "session epoch mismatch — please reconnect" and stop.
- On 404 (after 3 retries with linear backoff): show "session not found" and stop.
- On 5xx (up to 5 retries with exponential backoff): retry silently, show "reconnecting..." in status bar.

Proactive token refresh:

- When current token has less than 5 minutes until expiry, call token refresh endpoint.
- On 409 during refresh: surface error to user, stop reconnect attempts.

SSE event to `agent.Event` conversion:

- `assistant_delta` → `agent.AssistantTextDelta{Text: delta.text}`.
- `thinking_delta` → `agent.AssistantThinkingDelta{Text: delta.text}`.
- `tool_start` → `agent.ToolUseStart{...}`.
- `tool_progress` → `agent.ToolUseProgress{...}`.
- `tool_result` → `agent.ToolUseResult{...}`.
- `retry_notice` → `agent.RetryNotice{...}`.
- `terminal` → `agent.Terminal{...}`.
- `permission_request` → handled internally, not forwarded to TUI as a generic event.
- `gap` → inject a `agent.RetryNotice`-like system notice ("some events were missed during reconnection").

Tests (using httptest SSE server):

- `Run` returns a channel that receives events from SSE stream.
- `Run` replays buffered events on reconnect with correct `Last-Event-ID`.
- `Run` handles reconnect after EOF correctly.
- `Run` stops on 401.
- `Run` stops on 409 with epoch error.
- `SendInput` POSTs to correct endpoint.
- Permission request event triggers modal resolution.
- Permission decision is POSTed to correct endpoint.
- Proactive token refresh called when token near expiry.
- Goroutine exits cleanly on context cancel (no leak).

### Step 10 — nandocodego connect Command

Files:

- `internal/cli/connect.go`

Implement `nandocodego connect <server-url> [--token <jwt>] [--session <id>]`:

1. Parse flags: `serverURL`, `--token`, `--session`.
2. If `--session` not provided: POST to `POST <server-url>/v1/sessions` to create a new session.
3. Create `RemoteBridge` with `serverURL`, `sessionID`, `token`.
4. Build `state.App` (no LLM config needed; remote bridge owns the model).
5. Wire `RemoteBridge` as `AgentRunner` in TUI `tui.New`.
6. Print session ID and reconnect hint on first connect.
7. Start Bubble Tea program with remote bridge.

Tests:

- Command parses `--token` and `--session` flags.
- Command creates new session when `--session` not provided.
- Command uses provided session ID when `--session` is given.
- Command exits non-zero if server unreachable.
- Session ID printed on first connect.

### Step 11 — Session Metadata Doctor Report

Files:

- `internal/cli/doctor.go`

Add server sessions section to `doctor` output:

```
Server Sessions:
  Sessions dir:     /Users/fernando/.local/share/nandocodego/sessions
  Active sessions:  2
  Detached sessions: 1
```

Tests:

- Doctor output contains "Server Sessions" section.
- Active and detached counts reflect `SessionStore` state.

### Step 12 — UDS Server for SendMessage

Files:

- `internal/server/uds_server.go`
- `internal/server/uds_server_test.go`

Implement the server-side UDS listener for Phase 24's `SendMessage uds:` routing:

- When a session starts, register a UDS socket at `paths.ServerSocketPath(sessionID)`.
- Listen for incoming `PendingMessage` JSON objects from `net.Conn`.
- Deliver received messages to the session's agent via `supervisor.QueueMessage`.
- Clean up socket on session stop.

Tests:

- UDS listener accepts connection and delivers message to agent mailbox.
- UDS listener cleans up socket file on session stop.
- Two simultaneous connections are handled correctly.
- Corrupt JSON input is logged and connection is closed gracefully.

### Step 13 — Integration Tests and Race Checks

Required commands:

```sh
go test -race ./internal/server/...
go test -race ./internal/cli/...
go test -race ./internal/tui/...
go test ./...
tools/check-allowed-deps.sh
tools/check-network-policy.sh
GOOS=linux GOARCH=amd64 go build ./internal/server  # verify prctl build tag
```

End-to-end test (requires two processes, integration tag):

```sh
go test -tags=integration -run TestRemoteConnectDetachReconnect ./internal/server
```

Manual smoke test:

```sh
# Terminal 1 (server)
go run ./cmd/nandocodego server --port 8080 --print-token

# Terminal 2 (client)
go run ./cmd/nandocodego connect http://localhost:8080 --token "$TOKEN_FROM_TERMINAL_1"
# Submit a prompt, disconnect, reconnect
```

## Implementation Checklist

### Paths

- [ ] Add `paths.ServerSessionsDir()` returning `DataDir() + "/sessions"`.
- [ ] Add `paths.SessionMetadataPath(sessionID)`.
- [ ] Add `paths.SessionTokenPath(sessionID)`.
- [ ] Add `paths.ServerSocketPath(sessionID)`.
- [ ] Write path helper tests.

### Delivered-Event Deduplication

- [ ] Decide whether to extend `RecentIDs` or create `internal/server/dedup.go`.
- [ ] Deduper uses bounded FIFO storage plus `map[string]struct{}`.
- [ ] `Add` evicts oldest when full.
- [ ] `Has` is O(1) map lookup.
- [ ] Default capacity: 2000.
- [ ] Synchronization behavior is explicit in code comments and tests.
- [ ] Write capacity overflow test.
- [ ] Write test: 2000-capacity deduper has exactly 2000 entries after filling.

### JWT

- [ ] Add `github.com/golang-jwt/jwt/v5` to `tools/allowed-deps.txt` with justification.
- [ ] Run `go get github.com/golang-jwt/jwt/v5` and update `go.mod`.
- [ ] Create `internal/server/jwt.go` with `ServerClaims`, `Sign`, `Verify`.
- [ ] `Sign` uses HS256 signing method.
- [ ] `Sign` default TTL: 1 hour.
- [ ] `Verify` returns `ErrExpired` for expired tokens.
- [ ] `Verify` returns `ErrEpochMismatch` when epoch does not match `expectedEpoch`.
- [ ] `Verify` returns `ErrInvalidToken` for tampered or wrong-key tokens.
- [ ] Token value never logged (test log output).
- [ ] Write `jwt_test.go` with all 6 scenarios.

### Event Ring Buffer

- [ ] Extend `internal/server/ringbuffer.go` or add a small replay wrapper around it.
- [ ] `Append` assigns monotonically increasing integer IDs starting at 1.
- [ ] `Since(lastID)` returns events with `ID > lastID` in order.
- [ ] `Since` with overflowed `lastID` reports a gap and returns all available events.
- [ ] Internal `sync.RWMutex` protects concurrent `Append` and `Since`.
- [ ] Default capacity: 500.
- [ ] Write `ringbuffer_test.go` with overflow test.
- [ ] Write concurrent `Append`+`Since` race test.

### Session Store

- [ ] Create `internal/server/session_store.go` with `SessionMetadata`, `SessionState`, `SessionStore`.
- [ ] `Create` generates a new session ID (prefixed `s` + 8 random chars, same alphabet as task IDs).
- [ ] `Create` writes `metadata.json` atomically to `paths.SessionMetadataPath(id)`.
- [ ] `Create` initializes replay buffer with capacity 500.
- [ ] `Create` initializes delivered-event dedupe with capacity 2000.
- [ ] `Transition` validates allowed state transitions.
- [ ] `Transition` writes updated `metadata.json` atomically.
- [ ] `BumpEpoch` increments `Epoch` and writes `metadata.json`.
- [ ] `LoadFromDisk` reads all `metadata.json` files in sessions directory.
- [ ] `LoadFromDisk` transitions `running`/`detached` to `stopped` (server restarted).
- [ ] `LoadFromDisk` skips corrupt files with WARN log.
- [ ] `AppendEvent` stores SSE event in ring buffer and returns ID.
- [ ] `EventsSince` delegates to ring buffer `Since`.
- [ ] Session store is safe for concurrent access.
- [ ] Write `session_store_test.go` with 12+ test cases.

### Reconnect Protocol

- [ ] Create `internal/server/reconnect.go` with `RetryPolicy`, `BackoffStrategy`, `ReconnectPolicyFor`.
- [ ] 401 → `MaxRetries: 0`.
- [ ] 404 → `MaxRetries: 3, Backoff: BackoffLinear`.
- [ ] 409 → `MaxRetries: 0`.
- [ ] 500-599 → `MaxRetries: 5, Backoff: BackoffExponential`.
- [ ] Write `reconnect_test.go`.

### prctl

- [ ] Create `internal/server/prctl_linux.go` with `//go:build linux` and `DisableCoreDump`.
- [ ] Create `internal/server/prctl_other.go` with `//go:build !linux` and no-op `DisableCoreDump`.
- [ ] Call `DisableCoreDump()` in `nandocodego server` command startup.
- [ ] Log WARN (not error) if `DisableCoreDump` returns an error.
- [ ] Write `prctl_test.go` verifying `DisableCoreDump` returns nil.
- [ ] Add `GOOS=linux GOARCH=amd64 go build ./internal/server` to CI cross-compile check.

### Server HTTP Extensions

- [ ] Implement JWT middleware `JWTMiddleware(store, secret)`.
- [ ] Middleware rejects missing `Authorization` header with 401.
- [ ] Middleware rejects expired token with 401.
- [ ] Middleware rejects epoch mismatch with 409 and body `"epoch mismatch — re-authenticate"`.
- [ ] Add `GET /v1/sessions/{id}/events` SSE endpoint.
- [ ] SSE endpoint sets correct headers.
- [ ] SSE endpoint replays `EventsSince(lastID)` before live stream.
- [ ] SSE endpoint sends `gap` event when `lastID` is before oldest buffered event.
- [ ] SSE endpoint transitions session to `running` on connect.
- [ ] SSE endpoint transitions session to `detached` on disconnect.
- [ ] Add `POST /v1/sessions/{id}/token/refresh` endpoint.
- [ ] Token refresh bumps epoch and returns new JWT.
- [ ] Old token rejected after epoch bump.
- [ ] Add `POST /v1/sessions/{id}/permissions/{reqID}` endpoint.
- [ ] Permission endpoint calls `permBroker.Resolve`.
- [ ] Permission endpoint returns 404 for unknown `reqID`.
- [ ] Add `GET /v1/sessions` listing endpoint.
- [ ] Add `DELETE /v1/sessions/{id}` stop endpoint.
- [ ] Write server handler tests using `httptest.NewRecorder`.

### RemoteBridge

- [ ] Create `internal/tui/remote_bridge.go` with `RemoteBridge`.
- [ ] `RemoteBridge` satisfies `tui.AgentRunner` interface.
- [ ] `Run` opens SSE connection with correct auth and `Last-Event-ID` header.
- [ ] `Run` converts SSE events to `agent.Event` values and sends on channel.
- [ ] `Run` handles `permission_request` SSE event: shows modal, posts decision.
- [ ] `Run` handles `gap` SSE event: emits system notice.
- [ ] `Run` reconnects on EOF with `Last-Event-ID`.
- [ ] `Run` stops on 401.
- [ ] `Run` stops on 409 with user-friendly error.
- [ ] `Run` retries on 5xx per `ReconnectPolicyFor`.
- [ ] `SendInput` POSTs to `/v1/sessions/{id}/messages`.
- [ ] Proactive token refresh when < 5 minutes until expiry.
- [ ] No goroutine leak on context cancel.
- [ ] Write `remote_bridge_test.go` using `httptest.NewServer` SSE fixture.

### connect Command

- [ ] Create `internal/cli/connect.go` with `connectOptions` and `runConnect`.
- [ ] Register `connect` as a Cobra subcommand in root command.
- [ ] `--token` flag required (or error).
- [ ] `--session` flag optional; creates new session if absent.
- [ ] Prints session ID and reconnect hint on first connect.
- [ ] Wires `RemoteBridge` as `AgentRunner` in TUI.
- [ ] Exits non-zero if server unreachable.
- [ ] Write `connect_test.go`.

### UDS Server

- [ ] Create `internal/server/uds_server.go` with `UDSServer`.
- [ ] `UDSServer.Start(sessionID)` listens on `paths.ServerSocketPath(sessionID)`.
- [ ] Accepts JSON-encoded `PendingMessage` objects from connections.
- [ ] Delivers messages to session agent via `supervisor.QueueMessage`.
- [ ] `UDSServer.Stop(sessionID)` closes listener and removes socket file.
- [ ] Write `uds_server_test.go`.
- [ ] Integrate UDS server start/stop into session lifecycle in HTTP server.

### Doctor and Final Checks

- [ ] Add "Server Sessions" section to `nandocodego doctor` output.
- [ ] Report active and detached session counts.
- [ ] Write doctor test for sessions section.
- [ ] Run `go test -race ./internal/server/...` — clean.
- [ ] Run `go test -race ./internal/tui/...` — clean.
- [ ] Run `go test -race ./internal/cli/...` — clean.
- [ ] Run `go test ./...` — no regressions.
- [ ] Run `tools/check-allowed-deps.sh` — only `github.com/golang-jwt/jwt/v5` added.
- [ ] Run `tools/check-network-policy.sh` — clean.
- [ ] Run `go vet ./...` — clean.
- [ ] Cross-compile check: `GOOS=linux GOARCH=amd64 go build ./internal/server`.
- [ ] Verify JWT secret never written to disk (grep test in CI).
- [ ] Verify token never logged at any level (grep `Authorization` in test logs).
- [ ] Manual smoke test: connect, detach, reconnect cycle.
- [ ] Update `docs/PHASE-LOG.md` with Phase 25 entry.

## Acceptance Criteria

- [ ] Client disconnect causes session to enter `detached` state; agent goroutine continues running.
- [ ] Client reconnect within ring buffer range replays buffered events from `Last-Event-ID` offset.
- [ ] Replayed events are deduplicated — no duplicate events delivered on reconnect.
- [ ] JWT expiry returns 401; client calls refresh endpoint and retries transparently.
- [ ] JWT epoch bump (via token refresh) causes old token to return 409.
- [ ] Delivered-event dedupe with capacity 2000 provides O(1) `Has` and bounded memory.
- [ ] `nandocodego connect <url> --token <jwt>` opens TUI connected to remote session.
- [ ] `nandocodego connect --session <id>` reconnects to existing session.
- [ ] Session ID printed to stdout on first connect with reconnect hint.
- [ ] Permission prompts forwarded to client via SSE; decisions returned via HTTP POST.
- [ ] `prctl(PR_SET_DUMPABLE, 0)` called on Linux server startup (verified in test).
- [ ] `prctl` is a no-op on macOS and Windows (no compile error).
- [ ] Server JWT signing secret is never written to disk.
- [ ] Session metadata (`metadata.json`) survives server restart; interrupted sessions move to `stopped`.
- [ ] `nandocodego server --print-token` prints a valid startup JWT to stdout from the running server process.
- [ ] Ring buffer capacity: 500 events; overflow emits `gap` SSE event to client.
- [ ] `go test -race ./internal/server/...` passes with zero race detector reports.
- [ ] `tools/check-allowed-deps.sh` passes — only `golang-jwt/jwt/v5` added.
- [ ] All pre-Phase-25 tests pass without modification.
- [ ] UDS server accepts messages from Phase 24 `SendMessage uds:` routing.
- [ ] Session state machine rejects illegal transitions (e.g., `stopped → running`).
- [ ] Token value never appears in any log output at any level.
- [ ] `nandocodego doctor` reports server sessions section with active and detached counts.
- [ ] End-to-end: 3-step connect, submit prompt, disconnect, reconnect cycle completes successfully.
- [ ] Phase log updated with Phase 25 entry.
- [ ] `nandocodego connect` exits non-zero with helpful error if server unreachable.
- [ ] 5xx server errors trigger exponential-backoff reconnect (up to 5 retries), not immediate failure.
- [ ] `go vet ./...` clean.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---:|---|
| JWT secret lost on server restart invalidates all tokens | Medium | Expected behavior; document clearly. On restart, sessions become `stopped` and clients must reconnect. |
| Ring buffer overflows for very long sessions | Medium | 500-event capacity covers ~5 minutes of streaming. `gap` event notifies client. Future: configurable capacity. |
| Token logged accidentally | High | Test log output in unit tests; grep for `Authorization` and `Bearer` patterns. |
| `prctl` fails silently on containerized Linux | Low | Fail open (log WARN, continue). Token is still only in-memory; failure only affects ptrace protection. |
| Client reconnects with stale session ID after server restart | Medium | Server `LoadFromDisk` moves old sessions to `stopped`; client receives 404 and the 3-retry linear backoff exhausts cleanly. |
| Race in SSE event delivery during detach/reattach transition | High | Session store uses RWMutex; ring buffer is internally synchronized; agent event channel is buffered. Test with `-race`. |
| UDS socket file left on disk after crash | Medium | `LoadFromDisk` at server start checks for and removes orphaned socket files before starting listeners. |
| Permission prompt times out while client is disconnected | Medium | `permBroker.PromptFunc` should have a configurable timeout (default: 30s); if client does not reconnect in time, auto-deny. |
| Multiple concurrent reconnects for same session | Medium | `SSE endpoint → Transition(running)` is idempotent; `SessionStore` mutex prevents concurrent transition races. Last writer wins. |
| `golang-jwt/jwt/v5` supply chain risk | Low | Already widely used; add to allowlist with license and version justification in Phase log. |

## Exit Gate

Phase 25 is complete only when:

- All acceptance criteria above are met.
- `go test -race ./internal/server/...` passes with zero races.
- `tools/check-allowed-deps.sh` passes — only `golang-jwt/jwt/v5` added.
- `tools/check-network-policy.sh` passes.
- `go vet ./...` passes.
- Cross-compile: `GOOS=linux GOARCH=amd64 go build ./internal/server` succeeds.
- Manual smoke test: connect, detach (Ctrl-C), wait 5s, reconnect — buffered events replay.
- Phase log records implementation, test results, JWT library justification, deviations from plan, and manual smoke test result.

## Phase Log Template

When implementation finishes, append a Phase 25 entry to `docs/PHASE-LOG.md` with:

- objective;
- files created and updated;
- dependencies added and allowlist status (`golang-jwt/jwt/v5` with license, version, and justification);
- tests and checks run;
- manual connect-detach-reconnect smoke test result;
- design decisions (especially JWT secret lifecycle, ring buffer capacity, prctl fail-open);
- known constraints and deferred work (cross-machine bridge, WebSocket, OAuth2, upstream proxy);
- exit gate status.
