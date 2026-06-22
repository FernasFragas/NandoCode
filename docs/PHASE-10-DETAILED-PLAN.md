# Phase 10 Detailed Plan - MCP Integration

Date: 2026-05-07
Status: Complete in code and automated checks; manual live exit-gate validation pending
Source plans and references:

- `.codex/go-ollama-plan-AGENTS.md`
- `.codex/go-ollama-plan-HUMANS.md`
- `docs/PHASE-LOG.md`
- `docs/PROJECT-STATUS-AND-ONBOARDING.md`
- `docs/PHASE-8-DETAILED-PLAN.md`
- `docs/PHASE-9-DETAILED-PLAN.md`
- `book/ch01-architecture.md`
- `book/ch06-tools.md`
- `book/ch09-permissions.md`
- `book/ch12-extensibility.md`
- `book/ch15-mcp.md`
- `.codex/agent-context/learnings-memory.md`

## Goal

Phase 10 integrates the Model Context Protocol (MCP) so users can connect external tool servers over stdio and HTTP transports. MCP-provided tools are wrapped in the standard `tools.Tool` interface so they flow through the existing permission system, tool registry, and TUI without any special-casing in the agent loop. This phase also enables the previously-disabled HTTP hook kind now that HTTP transport infrastructure is being built, and it lays groundwork for the Agent hook kind that will be enabled in Phase 11 once sub-agents exist.

The concrete product goal is that a user can declare an MCP server in `~/.nandocodego/config.toml`, start a REPL, and use any tool that server exposes — with the same permission prompts, hook decisions, and transcript rendering that built-in tools already provide.

Deliverables:

- `internal/mcp` package with client, config, tool adapter, transport layer, name sanitizer, and dynamic tool registry overlay.
- `internal/mcp/transport` sub-packages for stdio and HTTP+SSE.
- `internal/mcp/auth` sub-package for RFC 9728/8414/PKCE OAuth flow with OS keyring token storage.
- Per-call context isolation: each MCP tool invocation gets a fresh, independently-cancellable context derived from the parent.
- MCP tool names normalized to `mcp__<server>__<tool>` convention.
- MCP server lifecycle management: start, health check, graceful stop.
- HTTP hook kind enabled after this phase's HTTP transport infrastructure validates destination policy.
- Agent hook kind continues to be disabled until Phase 11 adds a bounded sub-agent runtime.
- `doctor` command extended to report MCP server connection status.
- Phase log update after implementation.

## Implementation Reconciliation (2026-05-08)

All code-level Phase 10 items in this plan are now implemented in-repo, including:

- MCP config parsing, trust defaults, and validation warnings for user/project config sources.
- MCP client lifecycle (`NewClient`, `Connect`, `Tools`, `CallTool`, `Close`) and transport abstraction (`ClientTransport`).
- Stdio + HTTP MCP transports, including shared HTTP destination safety checks and dial-time DNS/IP validation.
- OAuth PKCE flow with keyring token storage and HTTP bearer injection for `auth = "oauth"` MCP servers.
- MCP tool adapter implementing `tools.Tool` with per-call child context and non-text content placeholders.
- Registry overlay implementation with collision handling where built-in tools win.
- REPL MCP wiring with startup diagnostics and non-fatal per-server connection failures.
- Doctor MCP diagnostics including per-server status and connectivity checks.
- HTTP hooks enabled and guarded by destination validation and safe dialer behavior.

Verification completed:

- `go test ./...`
- `tools/check-allowed-deps.sh`
- `tools/check-network-policy.sh`

Deliberate implementation notes:

- The repository currently keeps MCP transport files under `internal/mcp/*.go` instead of `internal/mcp/transport/*`; behavior is equivalent to the plan, package layout differs.
- MCP tool descriptions are truncated more aggressively than the plan text (to satisfy existing tool-description validation constraints in this repo), which is stricter than the 2000-char cap.

Remaining Phase 10 work is manual-only:

- Run the stdio live flow with a real local MCP server and confirm no orphaned process on REPL exit.
- Run the HTTP hook live flow and confirm behavior against a real local endpoint.

## Definition Of Success

Phase 10 exit gate is two manual flows:

Flow 1 — stdio tool call:

1. Declare an MCP server entry in `~/.nandocodego/config.toml` pointing at a locally-installed stdio MCP server binary.
2. Start the REPL.
3. Ask the model to use a tool provided by that server.
4. Confirm the tool name appears as `mcp__<server>__<tool>` in the transcript.
5. Confirm permission prompt appears for first use.
6. Confirm the tool result is rendered in the transcript.
7. Stop the REPL; confirm the server process is not left orphaned.

Flow 2 — HTTP hook now enabled:

1. Configure a user-level HTTP hook targeting a local test server.
2. Start the REPL.
3. Attempt a Bash tool call.
4. Confirm the HTTP hook fires and its decision is honored.
5. Confirm that a hook targeting a private IP outside the explicitly-allowed list is rejected at startup with a clear diagnostic, not silently skipped.

Both flows must work without any network destination other than the configured Ollama endpoint and declared MCP server/hook endpoints.

## Baseline Analysis From Implemented Phases

### Phase 0 - Security and Supply Chain

Implemented:

- `SECURITY.md`
- dependency allowlist in `tools/allowed-deps.txt`
- hardcoded endpoint checker in `tools/check-network-policy.sh`
- CI/security baseline

Phase 10 implications:

- `github.com/modelcontextprotocol/go-sdk` must be added to `tools/allowed-deps.txt` with justification before the import lands in any source file.
- `github.com/zalando/go-keyring` must also be added to the allowlist; it is the OS keyring dependency for OAuth token storage.
- The MCP HTTP transport will add a new outbound network destination. The endpoint must be configurable and must not be hardcoded. `tools/check-network-policy.sh` should not trip on legitimate config-driven MCP server URLs.
- OAuth callback server must bind only to `127.0.0.1` and must close after token exchange.
- MCP server configs loaded from project directories must be treated with the same project-hook caution: parsed and reported, but not executed without a trust gate unless explicitly approved.
- MCP tool names and server-provided descriptions must never be executed as instructions or used to modify the permission resolver in ways not declared in config.

### Phase 1 - CLI, Paths, Logging

Implemented:

- `cmd/nandocodego`
- Cobra root command
- `internal/paths` with XDG/NANDOCODEGO helpers
- `internal/logging` with structured slog setup
- `doctor` command
- empty future package dirs including `internal/mcp`

Phase 10 implications:

- `internal/mcp` already exists as an empty directory. Add files there; do not create a parallel location.
- `paths.ConfigDir()` returns `~/.nandocodego`; use it as the default search location for `config.toml`.
- `doctor` already runs without network calls. Extend it to report MCP server entries from config, their enabled status, and connection results from a lightweight ping. Doctor must not leave server processes running after the health check.
- Logging must not include raw MCP tool inputs/outputs at INFO level. MCP content is user-generated and may contain PII or project secrets.

### Phase 2 - LLM Client

Implemented:

- provider-neutral `llm.Client`
- streaming `Chat` with `ChatRequest.Format` for structured output
- model list/pull/embed API shapes
- retry/watchdog helpers

Phase 10 implications:

- MCP does not interact with `llm.Client` directly. The MCP client calls MCP servers; the LLM client calls Ollama. They are separate I/O paths.
- OAuth prompt hooks can optionally use `llm.Client` if a prompt hook is configured alongside MCP, but that is Phase 9/12 territory, not core MCP.
- The MCP Go SDK's own retry behavior should not conflict with the agent loop watchdog. Be explicit about which timeout layer is responsible for each request.

### Phase 3 - Tool Interface and Starter Tools

Implemented:

- `tools.Tool` interface with `Name()`, `Description()`, `Schema()`, `UnmarshalInput()`, `Call()`
- `tools.Context` with working dir, additional dirs, permission mode, and logger
- `tools.Registry` with `Lookup` and list
- `tools.Result` with `Display` and `Data`
- Bash, FileRead, FileWrite
- Path safety helpers including symlink resolution

Phase 10 implications:

- The MCP tool adapter must implement the full `tools.Tool` interface. The agent loop and permission resolver must not know they are talking to an MCP tool.
- `tools.Registry` must support dynamic overlays so MCP tools can be added at session start after config is parsed. Registry must not become a mutable shared-state problem if future concurrent phases call `Lookup` while MCP is registering tools.
- MCP tool schemas are provided by the server at `tools/list` time. They must be converted to `tools.Schema` compatible with the existing JSON Schema format passed to `llm.ChatRequest.Tools`.
- MCP tool results may be text, image, or embedded resources. Phase 10 should map text results to `tools.Result.Display` and defer image/resource rendering to a later phase.
- Path safety helpers in `internal/tools` are not applicable to MCP tool calls because the tool executes on the remote MCP server, not locally. Permission checks still apply to the decision of whether to invoke the tool, but the local path containment checks are not the right layer.

### Phase 4 - Agent Loop

Implemented:

- `agent.Run(ctx, Input) <-chan Event`
- `agent.Input.SystemPrompt`, `.Messages`, `.ToolContext`, `.PermissionMode`, `.PermissionRules`, `.PermissionPrompt`, `.HookDecision`, `.PostToolUse`, `.PermissionDenied`, `.StopHook`
- tool execution via `executeToolCalls`
- `agent.Terminal` with `Conversation []llm.Message`
- event stream: `AssistantTextDelta`, `ThinkingDelta`, `ToolUseStart`, `ToolUseProgress`, `ToolUseResult`, `HookNotice`, `RetryNotice`, `Terminal`

Phase 10 implications:

- The agent loop calls tools through `tools.Registry.Lookup(name)`. If MCP tools are registered there, they are called transparently.
- Tool call IDs are synthesized (`tool-0`, `tool-1`, etc.) in the current implementation. MCP tool calls may carry a server-generated call ID; that ID should be preserved in the hook event and log metadata.
- Each MCP tool call must use a child context derived from the agent turn context, not the session-level context. This ensures a single slow MCP tool call does not block cancellation of the entire agent run.
- `ToolUseProgress` events can be forwarded from MCP streaming results if the transport supports incremental output. For Phase 10, batch the result and emit a single `ToolUseResult`.

### Phase 5 - Permission System

Implemented:

- seven permission modes: `ModeDefault`, `ModeBypass`, `ModeAsk`, `ModeDontAsk`, `ModeBubble`, `ModeAutoEdit`, `ModeAutoApprove`
- `permissions.Resolve` with `HookDecision` callback, rule matching, tool classifier, mode, and prompt
- `permissions.Request` and `permissions.Result`
- source-tagged rules

Phase 10 implications:

- MCP tools must flow through `permissions.Resolve` with no bypass. The tool name `mcp__<server>__<tool>` is the canonical name used for rule matching.
- Rules can target MCP tools by prefix: `mcp__filesystem__*` can allow all tools from the filesystem server.
- The tool classifier in `permissions.Resolve` is currently nil for most requests. MCP tools should not assume any classifier is present; they rely on mode, rules, hooks, and prompts.
- `HookDecision` is wired in `internal/hooks/runner.go`. HTTP hooks are now enabled in Phase 10, so MCP tool calls may trigger HTTP pre-tool hooks for the first time.

### Phase 6 - State Layer

Implemented:

- `bootstrap.State` with session/config fields
- `state.Store[state.App]` with reactive `OnChange`
- `state.App.Messages`, `.ToolSettings`, `.ActiveModel`, `.Tasks`
- copy-on-write discipline on slice/map mutations

Phase 10 implications:

- MCP server configs should be loaded from `config.toml` at session start and stored in `bootstrap.Snapshot` or passed directly to `internal/mcp` during REPL wiring. Do not add a `sync.Map` of live MCP clients to `state.App`; client lifecycle belongs in the composition root.
- If a MCP server fails to connect at startup, emit a warning-level `HookNotice` or startup diagnostic, but do not abort the session.
- `state.App.ToolSettings.AdditionalWorkingDirs` is for local file tool safety. Do not add MCP server URLs there.

### Phase 7 - Bubble Tea TUI and REPL

Implemented:

- `internal/tui/app.go` — full Bubble Tea model
- permission modal and broker
- transcript rendering with tool panels
- slash commands: `/help`, `/clear`, `/exit`, `/model`
- `internal/cli/repl.go` — REPL composition root

Phase 10 implications:

- MCP tool calls should render in the same transcript tool panels as built-in tools. No new TUI component is needed for Phase 10.
- OAuth browser-open flows should NOT block the Bubble Tea event loop. Open the browser from a goroutine; emit a `HookNotice`-style event notifying the user to complete OAuth in the browser.
- MCP startup diagnostics (connection failures, disabled server entries) should surface as startup system notices using the existing `HookNotice` event path.
- Full `/mcp` slash command is deferred to Phase 13. Phase 10 can add a minimal diagnostic only via `doctor`.

### Phase 8 - Memory

Implemented:

- `memory.Runner` decorator wrapping the agent runner
- recall, frontmatter scan, pending extraction, staleness, and prompt injection
- per-run composition root: `memory.NewRunner(agentRunner, client, memory.DefaultConfig(...))`

Phase 10 implications:

- MCP tools may create or modify project files. If those files are under the memory directory, the memory runner will scan them on the next session. No special interaction between MCP and memory is needed in Phase 10.
- Memory injection in `agent.Input.SystemPrompt` is already in place. If MCP tools produce tool descriptions that collide with memory instructions, the collision is in the system prompt order, not in code. Keep memory section clearly delimited.

### Phase 9 - Hooks

Implemented:

- `hooks.Runner` decorator wrapping the memory runner
- snapshot-based hook config loaded once at session start from `~/.nandocodego/hooks.json`
- command and prompt hooks executable
- HTTP and agent hook kinds recognized but disabled by default
- `PreToolUse`, `PostToolUse`, `SessionStart`, `SessionEnd`, `UserPromptSubmit`, `Stop`, `PermissionDenied` all wired

Phase 10 implications:

- HTTP hook kind should be enabled in Phase 10 now that HTTP transport infrastructure is being built.
- The destination validation logic required to safely enable HTTP hooks can share code with the MCP HTTP transport dial path.
- Enabling HTTP hooks means writing the SSRF-safe dial validator that the original Phase 9 plan deferred.
- Agent hook kind continues to be disabled; Phase 11 provides the bounded sub-agent runtime.
- MCP tool calls will trigger `PreToolUse` and `PostToolUse` exactly like built-in tool calls because they flow through the same permission/hook bridge in `internal/agent/tools.go`.

## Documentation And Plan Findings

The source `.codex` plans agree Phase 10 is MCP. `internal/mcp` is no longer empty; a stdio-first integration slice is implemented, with HTTP/auth/doctor/hook enablement pending.

`docs/PHASE-LOG.md` records the current authoritative order: Phase 10 MCP, Phase 11 Sub-agents and fork.

Phase 10 must add entries to `docs/PHASE-LOG.md` when implementation lands.

## Deep Analysis Of `book/ch15-mcp.md`

Chapter 15 treats MCP as a trust boundary, not merely an API integration. The book's core lessons relevant to Phase 10:

### Transport And Lifecycle

- Each MCP server has its own process or connection lifecycle. A stdio server is a child process started by the CLI and must be terminated cleanly on session end. An HTTP server is a remote endpoint that the CLI dials but does not manage.
- The client must handle server restarts gracefully. If a stdio server process dies unexpectedly, the tool call in flight should fail with a clear error; the agent loop should not hang.
- Connection state must not leak between sessions. Each REPL session gets its own MCP client instances.

### Tool Naming And Discovery

- Tool discovery runs once at session start by calling `tools/list` on each server. Tools can change between sessions but not mid-session unless the client implements a `tools/list_changed` notification, which is an optional capability.
- Phase 10 should implement static discovery at session start and not support mid-session tool changes. That simplifies the registry overlay considerably.
- The name convention `mcp__<server>__<tool>` is load-bearing: it must be consistent across discovery, tool registration, permission rules, hook matchers, and any future skill declarations. Any change to the convention after Phase 10 is a breaking change.

### Content And Trust Boundary

- MCP servers can return not just text but also images, embedded resources, and tool definitions. The book is explicit: resource content returned by a tool call must not auto-execute. It should be rendered as context only.
- MCP servers can also expose prompts (prompt templates). These should be presented as available capabilities, not auto-injected into the system prompt.
- Tool names and descriptions provided by the server are untrusted. They must not be used to modify permission rules, create new hook matchers, or update any security policy at runtime.
- Injection attacks through MCP tool results: a malicious MCP server could return content designed to override the system prompt. The agent loop should continue treating tool results as user-turn content, not as system-level instructions.

### OAuth And Authentication

- The RFC 9728 protected resource metadata standard defines how the client discovers the authorization server for an HTTP MCP endpoint.
- RFC 8414 authorization server metadata tells the client where to redirect for authorization.
- PKCE (RFC 7636) protects the authorization code exchange. The client generates a code verifier, hashes it to a code challenge, and passes the challenge in the authorization request. The verifier is sent with the token request.
- Tokens must be stored in the OS keyring, not in config files. `github.com/zalando/go-keyring` provides the OS keyring abstraction.
- The OAuth callback must be a short-lived local HTTP server on `127.0.0.1` with a random port. It must close immediately after receiving the code and must not serve any other requests.

### Deliberate Adaptations For This Repo

- The book's reference implementation is TypeScript; this is Go. Use `github.com/modelcontextprotocol/go-sdk` instead of the TypeScript SDK.
- The book discusses prompt caching for MCP tool descriptions. Ollama does not support prompt caching. Phase 10 can cache the tool list in memory for the session duration without worrying about prompt cache TTL.
- The book mentions MCP sampling requests (the server asks the client to make an LLM call on the server's behalf). This is an advanced capability that opens significant security questions. Phase 10 should reject sampling requests explicitly with a clear error.
- Team/shared MCP configs and policy-server-managed MCP allowlists are out of scope for Phase 10.

## Evaluation Of The Original Phase 10 Plan

The original Phase 10 entries in `.codex/go-ollama-plan-AGENTS.md` are correct at the product level but need more precision:

- They do not specify that `github.com/modelcontextprotocol/go-sdk` must be added to `tools/allowed-deps.txt` before use.
- They do not specify how OAuth tokens are scoped to server identity in the keyring.
- They do not address the MCP sampling capability rejection.
- They do not specify that HTTP hook enablement is gated on the same destination validation logic as HTTP transport.
- They do not specify the exact tool name sanitization rules for server names containing hyphens, spaces, or uppercase letters.
- They do not specify how the dynamic tool registry overlay interacts with the existing `tools.Registry.Lookup` thread-safety model.

## Final Phase 10 Scope

In scope:

- `internal/mcp/config.go` — config structs for MCP server entries and `config.toml` minimal parsing.
- `internal/mcp/client.go` — MCP client lifecycle, session start discovery, graceful stop.
- `internal/mcp/tool.go` — MCP → `tools.Tool` adapter.
- `internal/mcp/registry.go` — dynamic tool registry overlay that layers MCP tools on top of the built-in registry.
- `internal/mcp/transport/stdio.go` — stdio transport: start child process, pipe stdin/stdout, handle process death.
- `internal/mcp/transport/http.go` — HTTP+SSE transport: dial, send requests, receive events, SSRF-safe destination validation.
- `internal/mcp/auth/oauth.go` — OAuth 2.0 with PKCE: RFC 9728/8414 discovery, local callback server, token storage in OS keyring.
- `internal/mcp/sanitize.go` — name sanitization: lowercase, replace disallowed chars, produce canonical `mcp__<server>__<tool>`.
- REPL wiring in `internal/cli/repl.go`: load MCP config, build clients, register tools, then wire into agent.
- `doctor` extension: report MCP server status.
- HTTP hook enablement: add SSRF-safe dial validator and flip `KindHTTP.Executable()` to true.
- Tests for all above packages.
- `tools/allowed-deps.txt` updates for `go-sdk` and `go-keyring`.
- Phase log update.

Out of scope:

- Sub-agent spawning (Phase 11).
- Agent hook kind enablement (Phase 11).
- MCP sampling requests (rejected explicitly but full support is future).
- MCP resource subscriptions and live notifications.
- MCP image/embedded resource rendering beyond text (deferred to Phase 13+ UX work).
- MCP prompt templates auto-injection.
- `/mcp` slash command and full command UX (Phase 13).
- Team/policy-server MCP allowlists.
- Multi-provider LLM backends.
- Skills (Phase 12).
- Config file full UI and source-tagged overrides (Phase 13).
- WebFetch/WebSearch tools (Phase 13).
- Observability/metrics decorators (Phase 16).

## Architecture

### Package Layout

```text
internal/mcp/
  config.go
  config_test.go
  client.go
  client_test.go
  tool.go
  tool_test.go
  registry.go
  registry_test.go
  sanitize.go
  sanitize_test.go

internal/mcp/transport/
  stdio.go
  stdio_test.go
  http.go
  http_test.go
  transport.go

internal/mcp/auth/
  oauth.go
  oauth_test.go
  keyring.go
  keyring_test.go
```

### Core Types

```go
// ServerConfig describes a single MCP server entry from config.
type ServerConfig struct {
    Name      string            // canonical server name; used in tool name prefix
    Transport string            // "stdio" or "http"
    Command   string            // for stdio: path to binary
    Args      []string          // for stdio: arguments
    URL       string            // for http: base URL
    Env       map[string]string // additional environment for stdio processes
    Enabled   bool              // default true
    Trusted   bool              // project configs require explicit trust in Phase 10
}

// Config holds all MCP server entries loaded from config.toml.
type Config struct {
    Servers []ServerConfig
}

// ToolDescriptor is the MCP wire-format tool definition.
type ToolDescriptor struct {
    Name        string
    Description string
    InputSchema json.RawMessage
}

// CallResult is the MCP wire-format tool call result.
type CallResult struct {
    Content []ContentBlock
    IsError bool
}

// ContentBlock is one item in a CallResult.
type ContentBlock struct {
    Type string // "text", "image", "resource"
    Text string // populated when Type is "text"
}
```

### Transport Interface

```go
// Transport is the low-level MCP wire protocol abstraction.
type Transport interface {
    // Initialize sends the MCP initialize request and returns server capabilities.
    Initialize(ctx context.Context) (ServerCapabilities, error)
    // ListTools returns the tool list from the server.
    ListTools(ctx context.Context) ([]ToolDescriptor, error)
    // CallTool sends a tool call and returns the result.
    // Each call must use a fresh context scoped to this call only.
    CallTool(ctx context.Context, name string, input json.RawMessage) (CallResult, error)
    // Close shuts down the transport cleanly.
    Close() error
}
```

### MCP Tool Adapter

```go
// MCPTool adapts a remote MCP tool into the tools.Tool interface.
type MCPTool struct {
    canonicalName string        // mcp__<server>__<tool>
    descriptor    ToolDescriptor
    transport     Transport
    serverName    string
}

func (t *MCPTool) Name() string
func (t *MCPTool) Description() string
func (t *MCPTool) Schema() tools.Schema
func (t *MCPTool) UnmarshalInput(raw []byte) (any, error)
func (t *MCPTool) Call(ctx tools.Context, input any, progress chan<- tools.ProgressEvent) (tools.Result, error)
```

The `Call` implementation must:

1. Extract a child context from `ctx.Context` scoped to this single call.
2. Marshal `input` back to JSON (it was parsed through `UnmarshalInput`).
3. Call `transport.CallTool` with the scoped context.
4. Map `CallResult.Content` text blocks to `tools.Result.Display`.
5. If `CallResult.IsError` is true, return an error with the content text as the message.
6. Close or cancel the child context after the call completes, success or failure.

### Registry Overlay

```go
// OverlayRegistry layers MCP tools on top of a base registry.
// It is built once at session start after all servers have listed their tools.
// It is read-only after construction and safe for concurrent Lookup calls.
type OverlayRegistry struct {
    base *tools.Registry
    mcp  map[string]tools.Tool // keyed by canonical name
}

func NewOverlayRegistry(base *tools.Registry, mcpTools []tools.Tool) *OverlayRegistry
func (r *OverlayRegistry) Lookup(name string) (tools.Tool, bool)
func (r *OverlayRegistry) List() []tools.Tool
```

The overlay is immutable after construction. Thread safety comes from never mutating `mcp` after `NewOverlayRegistry` returns.

### Name Sanitization Rules

```go
// SanitizeName converts a raw MCP server name or tool name to a safe segment.
// Rules:
// 1. Lowercase entire string.
// 2. Replace any character that is not [a-z0-9_] with an underscore.
// 3. Collapse consecutive underscores to one.
// 4. Trim leading/trailing underscores.
// 5. If empty after sanitization, return "unknown".
func SanitizeName(raw string) string

// CanonicalToolName returns mcp__<server>__<tool> from sanitized parts.
func CanonicalToolName(serverName, toolName string) string
```

### OAuth Flow

```go
// TokenStore persists and retrieves OAuth tokens for an MCP server.
type TokenStore interface {
    Save(serverID, tokenJSON string) error
    Load(serverID string) (tokenJSON string, err error)
    Delete(serverID string) error
}

// KeyringStore implements TokenStore using the OS keyring.
type KeyringStore struct {
    service string // e.g. "nandocodego-mcp"
}

// PKCEFlow performs a full RFC 9728/8414/PKCE authorization flow.
type PKCEFlow struct {
    ServerID    string
    ResourceURL string // MCP server base URL
    Store       TokenStore
}

func (f *PKCEFlow) EnsureToken(ctx context.Context) (accessToken string, err error)
```

`EnsureToken` must:

1. Check `Store` for a non-expired token. If found, return it.
2. Fetch RFC 9728 protected resource metadata from `/.well-known/oauth-protected-resource` on the resource URL.
3. Follow the `authorization_servers[0]` link to fetch RFC 8414 authorization server metadata.
4. Generate a PKCE verifier and challenge (`S256` method).
5. Build the authorization URL.
6. Open the URL in the system browser via `os/exec`.
7. Start a short-lived HTTP listener on `127.0.0.1:0` (random port).
8. Wait for the callback with code and state, with a bounded timeout.
9. Exchange the code for tokens using the PKCE verifier.
10. Persist the token JSON in the OS keyring.
11. Return the access token.

### HTTP Transport Destination Validation

```go
// ValidateHTTPDestination checks that a given URL is safe to dial.
// It rejects:
// - non-HTTPS URLs unless localhost/127.0.0.1/::1
// - RFC 1918 private addresses unless explicitly in an allowlist
// - loopback addresses in unexpected contexts
// - DNS names that resolve to private addresses (checked at connection time via custom Dialer)
func ValidateHTTPDestination(rawURL string, allowPrivate bool) error
```

This same function is used by both the MCP HTTP transport and the HTTP hook runner in `internal/hooks/http.go`. It must run at dial time via a custom `net.Dialer`, not just as a preflight URL check.

## Implementation Plan

### Step 1 - Dependency Allowlist Updates

Files:

- `tools/allowed-deps.txt`

Actions:

- [x] Add `github.com/modelcontextprotocol/go-sdk` with justification: MCP protocol implementation.
- [x] Add `github.com/zalando/go-keyring` with justification: OS keyring for OAuth token storage.
- [x] Run `tools/check-allowed-deps.sh` and confirm both pass before any import is added to source.

### Step 2 - Name Sanitization

Files:

- `internal/mcp/sanitize.go`
- `internal/mcp/sanitize_test.go`

Implement:

- [x] Name sanitization implemented as `sanitizeName(raw)` with lowercase + replacement + collapse + trim.
- [x] Canonical tool naming implemented as `toolName(serverName, toolName)` in `mcp__<server>__<tool>` format.
- [x] Canonical naming test exists (`internal/mcp/naming_test.go`).
- [ ] Remaining parity: fallback-to-`"unknown"` behavior and edge-case tests from this checklist.

### Step 3 - Config Structs And Minimal Parser

Files:

- `internal/mcp/config.go`
- `internal/mcp/config_test.go`

Implement:

- [ ] `ServerConfig` struct with `Name`, `Transport`, `Command`, `Args`, `URL`, `Env`, `Enabled`, `Trusted`.
- [x] `Config` struct with `Servers []ServerConfig`.
- [x] Config loading implemented as `LoadConfig(userConfigPath, projectConfigPath)` with missing-file tolerance.
- [ ] Use `gopkg.in/yaml.v3` for initial TOML-like minimal parsing, or add `BurntSushi/toml` to the allowlist if TOML-specific parsing is required. Decision: evaluate whether `gopkg.in/yaml.v3` covers the config format; if not, add TOML dependency with justification.
- [x] Validate: `Transport` must be `"stdio"` or `"http"`.
- [x] Validate: `stdio` entries require non-empty `Command`.
- [ ] Validate: `http` entries require non-empty `URL`.
- [ ] Validate: `Name` must be a non-empty string; names must be unique within a config file.
- [x] `Enabled` defaults to `true` if absent.
- [ ] `Trusted` defaults to `false` for project-level configs; user-level configs default to `true` unless explicitly set.
- [x] Test: missing file returns empty config.
- [x] Test: valid stdio entry parsed correctly.
- [ ] Test: valid http entry parsed correctly.
- [ ] Test: duplicate names produce a validation error.
- [ ] Test: invalid transport produces a validation error.
- [ ] Test: `Enabled: false` entry is parsed but skipped when building clients.

### Step 4 - Transport Interface And Stdio Transport

Files:

- `internal/mcp/transport/transport.go`
- `internal/mcp/transport/stdio.go`
- `internal/mcp/transport/stdio_test.go`

Implement:

- [ ] `Transport` interface: `Initialize`, `ListTools`, `CallTool`, `Close`.
- [ ] `StdioTransport` struct: holds `*exec.Cmd`, reader/writer, JSON-RPC message loop.
- [ ] `NewStdioTransport(cfg ServerConfig) (*StdioTransport, error)` — build but do not start process.
- [ ] `(*StdioTransport).Start(ctx context.Context) error` — start process, initialize message loop.
- [ ] `(*StdioTransport).Initialize(ctx context.Context) (ServerCapabilities, error)`.
- [ ] `(*StdioTransport).ListTools(ctx context.Context) ([]ToolDescriptor, error)`.
- [ ] `(*StdioTransport).CallTool(ctx context.Context, name string, input json.RawMessage) (CallResult, error)`.
- [ ] `(*StdioTransport).Close() error` — send shutdown notification, then kill process if not stopped within bounded timeout (2 seconds).
- [ ] Per-call timeout: derive child context from `ctx` when calling `CallTool`; do not use the session-level context.
- [ ] Handle unexpected process death: detect EOF on stdout pipe; return error on all in-flight calls.
- [ ] Do not inherit full parent process environment. Pass explicit `Env` from config plus a minimal safe subset.
- [ ] Test: process start and stop with a fake echo server.
- [ ] Test: `CallTool` with a fake server that returns a text result.
- [ ] Test: process death mid-call returns an error.
- [ ] Test: context cancellation on `CallTool` cancels the in-flight request.
- [ ] Test: `Close` kills a non-responding process after bounded timeout.

### Step 5 - HTTP Transport And SSRF Validation

Files:

- `internal/mcp/transport/http.go`
- `internal/mcp/transport/http_test.go`
- `internal/hooks/http.go` (update to use shared validation)

Implement:

- [ ] `ValidateHTTPDestination(rawURL string, allowPrivate bool) error` in a shared location accessible by both `internal/mcp/transport` and `internal/hooks`. Candidate: `internal/mcp/transport/http.go` exported as `ValidateDestination`, or a small `internal/netutil` package.
- [ ] Validation rules: reject non-HTTPS URLs unless localhost/loopback; reject RFC 1918 addresses unless `allowPrivate` is true; use a custom `net.Dialer` that resolves the hostname and re-checks the resolved IP against the same rules at dial time.
- [ ] `HTTPTransport` struct: holds `*http.Client` with custom dialer, base URL, auth token getter.
- [ ] `NewHTTPTransport(cfg ServerConfig, tokenGetter func(ctx context.Context) (string, error)) (*HTTPTransport, error)` — validate destination, build client.
- [ ] `(*HTTPTransport).Initialize(ctx context.Context) (ServerCapabilities, error)`.
- [ ] `(*HTTPTransport).ListTools(ctx context.Context) ([]ToolDescriptor, error)`.
- [ ] `(*HTTPTransport).CallTool(ctx context.Context, name string, input json.RawMessage) (CallResult, error)` — use SSE streaming or JSON-RPC as supported by the server.
- [ ] `(*HTTPTransport).Close() error` — cancel any in-flight requests.
- [ ] Update `internal/hooks/http.go` to call `ValidateDestination` at hook registration time. Return a clear diagnostic if the destination is rejected.
- [ ] Enable `KindHTTP.Executable()` returning `true` in `internal/hooks/types.go` only after the validation is in place.
- [ ] Test: valid HTTPS external URL passes validation.
- [ ] Test: plain HTTP non-localhost URL fails validation.
- [ ] Test: RFC 1918 address fails without `allowPrivate`.
- [ ] Test: RFC 1918 address passes with `allowPrivate`.
- [ ] Test: DNS resolution to private IP fails at dial time even if original URL was a hostname.
- [ ] Test: `CallTool` with a fake HTTP test server.
- [ ] Test: HTTP hook now fires when `KindHTTP.Executable()` is true.

### Step 6 - OAuth PKCE Flow

Files:

- `internal/mcp/auth/oauth.go`
- `internal/mcp/auth/oauth_test.go`
- `internal/mcp/auth/keyring.go`
- `internal/mcp/auth/keyring_test.go`

Implement:

- [ ] `KeyringStore` with `Save`, `Load`, `Delete` using `github.com/zalando/go-keyring`.
- [ ] `PKCEFlow` struct with `ServerID`, `ResourceURL`, `Store`.
- [ ] `(*PKCEFlow).EnsureToken(ctx context.Context) (string, error)`.
- [ ] `fetchResourceMetadata(ctx context.Context, resourceURL string) (ResourceMetadata, error)` — GET `/.well-known/oauth-protected-resource`.
- [ ] `fetchAuthServerMetadata(ctx context.Context, issuerURL string) (AuthServerMetadata, error)` — GET `/.well-known/oauth-authorization-server`.
- [ ] `generatePKCE() (verifier, challenge string, err error)` — generate 32-byte verifier, SHA-256 hash, base64url encode.
- [ ] `startCallbackServer(ctx context.Context) (addr string, codeCh <-chan string, stop func(), err error)` — listen on `127.0.0.1:0`, single request handler.
- [ ] `exchangeCode(ctx context.Context, tokenEndpoint, code, verifier, redirectURI string) (TokenResponse, error)`.
- [ ] Token expiry check before returning cached token. Refresh if `refresh_token` is available.
- [ ] Emit a user-visible notice (via caller, not direct TUI access) instructing user to complete browser flow.
- [ ] `KeyringStore` service name: `"nandocodego-mcp"`.
- [ ] Keyring key: `"token:<serverID>"`.
- [ ] Test: `EnsureToken` returns cached token without network call.
- [ ] Test: expired token triggers refresh if refresh token exists.
- [ ] Test: full PKCE flow with fake OAuth server using `net/http/httptest`.
- [ ] Test: callback server closes after first request.
- [ ] Test: `generatePKCE` produces distinct verifier/challenge pairs.

### Step 7 - MCP Client Lifecycle

Files:

- `internal/mcp/client.go`
- `internal/mcp/client_test.go`

Implement:

- [ ] `Client` struct: holds `ServerConfig`, `Transport`, discovered `[]ToolDescriptor`, and lifecycle state.
- [ ] `NewClient(cfg ServerConfig) (*Client, error)` — validate config, select transport.
- [ ] `(*Client).Connect(ctx context.Context) error` — start transport, call `Initialize`, call `ListTools`, populate tool descriptors.
- [ ] `(*Client).Tools() []ToolDescriptor` — return discovered tools (read-only after `Connect`).
- [ ] `(*Client).CallTool(ctx context.Context, name string, input json.RawMessage) (CallResult, error)` — delegate to transport with per-call context.
- [ ] `(*Client).Close() error` — stop transport gracefully.
- [ ] Connection failure during `Connect` is non-fatal: log warning, return error, caller decides whether to skip or abort.
- [ ] Reject sampling requests from server during `Initialize` capabilities exchange by not advertising the sampling capability.
- [ ] Test: `Connect` succeeds with fake stdio transport.
- [ ] Test: `Connect` failure returns error; session continues without this server.
- [ ] Test: `Close` after `Connect` terminates transport.
- [ ] Test: `Tools` returns empty slice before `Connect`.

### Step 8 - MCP Tool Adapter

Files:

- `internal/mcp/tool.go`
- `internal/mcp/tool_test.go`

Implement:

- [ ] `MCPTool` struct implementing `tools.Tool`.
- [ ] `NewMCPTool(serverName string, desc ToolDescriptor, client *Client) *MCPTool`.
- [ ] `(*MCPTool).Name() string` — returns canonical `mcp__<server>__<tool>`.
- [ ] `(*MCPTool).Description() string` — returns server-provided description; truncate at 2000 chars to avoid prompt overflow.
- [ ] `(*MCPTool).Schema() tools.Schema` — convert `desc.InputSchema` to `tools.Schema`. If invalid JSON Schema, return a permissive schema with a single `input` string parameter and a warning.
- [ ] `(*MCPTool).UnmarshalInput(raw []byte) (any, error)` — unmarshal into `map[string]any`.
- [ ] `(*MCPTool).Call(ctx tools.Context, input any, progress chan<- tools.ProgressEvent) (tools.Result, error)` — marshal input, call `client.CallTool` with child context, map result.
- [ ] Child context derives from `ctx.Context`; cancel it after call returns.
- [ ] Map `CallResult.IsError=true` to `tools.Result{}` plus a non-nil error.
- [ ] Map text `ContentBlock` to `tools.Result.Display`.
- [ ] Non-text content blocks: log at DEBUG, include a `[non-text content omitted]` placeholder in `Display`.
- [ ] Test: `Name()` returns correct canonical name.
- [ ] Test: `Call` with text result produces `Result.Display`.
- [ ] Test: `Call` with `IsError=true` returns error.
- [ ] Test: context cancellation propagates to transport.
- [ ] Test: invalid input schema produces permissive fallback.

### Step 9 - Registry Overlay

Files:

- `internal/mcp/registry.go`
- `internal/mcp/registry_test.go`

Implement:

- [ ] `OverlayRegistry` struct with `base *tools.Registry` and `mcp map[string]tools.Tool`.
- [ ] `NewOverlayRegistry(base *tools.Registry, mcpTools []tools.Tool) *OverlayRegistry`.
- [ ] `(*OverlayRegistry).Lookup(name string) (tools.Tool, bool)` — check `mcp` map first, then `base.Lookup`.
- [ ] `(*OverlayRegistry).List() []tools.Tool` — return base tools plus MCP tools sorted by name.
- [ ] MCP tools do not shadow built-in tools: if a canonical MCP tool name collides with a built-in name (unlikely but possible), return a startup warning and keep the built-in.
- [ ] Immutable after construction; no mutex needed.
- [ ] Test: built-in tool found via `Lookup`.
- [ ] Test: MCP tool found via `Lookup`.
- [ ] Test: unknown name returns `false`.
- [ ] Test: `List` includes both built-in and MCP tools.
- [ ] Test: MCP tool shadowing built-in emits warning but built-in wins.

### Step 10 - REPL Wiring

Files:

- `internal/cli/repl.go`
- `internal/cli/repl_test.go` (if not already present)

Actions:

- [x] Load MCP config in REPL composition root from user/project `config.toml`.
- [x] For enabled stdio servers, start MCP sessions and list tools.
- [x] Register discovered MCP tools into the shared registry used by the agent.
- [x] On REPL exit, close MCP manager/clients.
- [x] Connection failures are warnings and do not abort REPL.
- [ ] Overlay registry abstraction not implemented yet (direct registration used as interim approach).
- [ ] Surface startup warnings in transcript/system messages (currently stderr warnings).
- [ ] Test: REPL composition root builds overlay registry when MCP config has entries.
- [ ] Test: missing MCP config file does not abort REPL start.

### Step 11 - Doctor Extension

Files:

- `internal/cli/doctor.go`
- `internal/cli/doctor_test.go`

Actions:

- [ ] Add an MCP section to `doctor` output.
- [ ] For each server entry in config: report name, transport, enabled status, trusted status.
- [ ] Attempt a `Connect` + `ListTools` + `Close` to verify connectivity. Use a short timeout (3 seconds).
- [ ] Report: connected (and number of tools), connection failed (with redacted error), disabled.
- [ ] Doctor must not leave server processes running after the check.
- [ ] Redact server URLs in logs at INFO level; allow them in doctor output since the user declared them.
- [ ] Test: doctor with empty MCP config reports no servers.
- [ ] Test: doctor with valid server reports tool count.
- [ ] Test: doctor with failing server reports failure without aborting doctor run.

### Step 12 - Hook HTTP Enablement

Files:

- `internal/hooks/types.go`
- `internal/hooks/http.go`
- `internal/hooks/http_test.go`

Actions:

- [ ] `KindHTTP.Executable()` returns `true` after this step (was `false` in Phase 9).
- [ ] `internal/hooks/http.go` calls `mcp/transport.ValidateDestination` (or the shared `netutil` function) before making any HTTP request.
- [ ] Configured HTTP hooks targeting private/loopback addresses that are not localhost and not explicitly allowed produce a startup warning in `Snapshot.Warnings` and are disabled with a clear reason.
- [ ] HTTP hook execution uses `net/http` with the custom safe dialer.
- [ ] Response timeout: honor `hook.Timeout()` with a 10-second maximum for HTTP hooks.
- [ ] Test: HTTP hook fires and returns a JSON decision body.
- [ ] Test: HTTP hook targeting 10.0.0.1 is disabled with a diagnostic.
- [ ] Test: HTTP hook targeting `localhost` is allowed.
- [ ] Test: HTTP hook timeout returns a warning result, not a block.

### Step 13 - Tests, Benchmarks, And Manual Smoke

Required commands:

```sh
go test ./internal/mcp/...
go test ./internal/hooks/...
go test ./internal/agent/... ./internal/permissions/... ./internal/tui/... ./internal/cli/...
go test ./...
tools/check-allowed-deps.sh
tools/check-network-policy.sh
```

If adding TOML dependency:

```sh
go mod tidy
tools/check-allowed-deps.sh
```

Manual smoke — stdio:

```sh
go run ./cmd/nandocodego --model qwen3 --no-alt-screen
```

1. Set up a local MCP server (e.g., `npx @modelcontextprotocol/server-filesystem /tmp`).
2. Add entry to `~/.nandocodego/config.toml`.
3. Start REPL.
4. Ask model to list files using the MCP filesystem tool.
5. Confirm `mcp__filesystem__list_directory` appears in transcript with permission prompt.
6. Exit REPL.
7. Confirm no orphaned server process.

Manual smoke — HTTP hook:

1. Start a local HTTP server that returns `{"decision":"allow"}`.
2. Add HTTP hook to `~/.nandocodego/hooks.json`.
3. Start REPL.
4. Trigger Bash tool call.
5. Confirm HTTP hook fires.
6. Confirm decision is honored.

## Acceptance Criteria

- [ ] `internal/mcp` package exists with config, client, tool adapter, registry overlay, sanitizer, transports, and auth.
- [ ] MCP tool names follow `mcp__<server>__<tool>` format with sanitized lowercase segments.
- [ ] Each MCP tool call uses a child context; cancellation is per-call and does not affect the parent run.
- [x] `tools/allowed-deps.txt` updated for `go-sdk` and `go-keyring` before any import.
- [ ] `go.mod` and `go.sum` updated with `go mod tidy`; allowlist check passes.
- [ ] MCP tools flow through `permissions.Resolve` with mode, rules, and hooks identical to built-in tools.
- [ ] `PreToolUse` and `PostToolUse` hooks fire for MCP tool calls.
- [ ] HTTP hooks are now enabled with SSRF-safe dial-time destination validation.
- [ ] HTTP hooks targeting private IP ranges without explicit allowance are disabled at startup with a clear diagnostic.
- [ ] OAuth tokens are stored in OS keyring, not in config files or environment variables.
- [ ] OAuth callback server binds only to `127.0.0.1` and closes after single use.
- [ ] MCP sampling requests are rejected explicitly during `Initialize`.
- [x] Stdio server process is terminated when the REPL session ends; no orphan processes.
- [x] Connection failure for one MCP server does not abort the REPL session; emits startup warning (currently stderr-based).
- [ ] Tool descriptions from MCP servers are truncated at 2000 characters to protect context.
- [ ] MCP tool names do not shadow built-in tool names; built-in wins on collision.
- [ ] `doctor` reports MCP server status including tool count or failure reason.
- [ ] `OverlayRegistry` is immutable after construction and safe for concurrent read.
- [ ] Agent hook kind remains disabled; Phase 11 enables it.
- [ ] MCP resource content returned by tool calls is context only; it does not modify permission rules or hook config.
- [ ] MCP server config from project directories is parsed and reported but not executed without trust gate.
- [ ] No MCP content is logged at INFO level.
- [ ] Tests do not require a live MCP server (fake transports used for all unit tests).
- [ ] Tests do not require live Ollama for unit/integration suites.
- [ ] `go test ./...` passes.
- [ ] `tools/check-allowed-deps.sh` passes.
- [ ] `tools/check-network-policy.sh` passes (MCP server URLs are config-driven, not hardcoded).
- [x] `docs/PHASE-LOG.md` has a Phase 10 entry recording files, decisions, and deferred work.
- [ ] Manual stdio exit gate flow (two flows above) passes with a real local MCP server.

## Forbidden

- Hardcoding any MCP server URL or OAuth endpoint URL in source files.
- Adding `github.com/modelcontextprotocol/go-sdk` or `github.com/zalando/go-keyring` to `go.mod` before `tools/allowed-deps.txt` is updated and checked.
- Using a shared session-level context for multiple concurrent MCP tool calls.
- Auto-executing MCP sampling requests.
- Injecting MCP prompt templates directly into the agent system prompt.
- Using MCP-provided tool descriptions to modify permission rules or hook matchers at runtime.
- Logging raw MCP tool inputs, outputs, OAuth tokens, or server-provided content at INFO level.
- Leaving stdio server child processes running after session end.
- Enabling HTTP hooks before `ValidateDestination` is integrated into the dial path.
- Enabling agent hook kind (reserved for Phase 11).
- Adding `/mcp` slash command or full config UX (Phase 13).

## Risks And Mitigations

| Risk | Impact | Mitigation |
|---|---:|---|
| MCP server injects prompt-override content into tool results | High | Treat all MCP tool result content as user-turn context, not system instructions. Never allow MCP content to modify system prompt sections. |
| OAuth token stored in plaintext config file | High | Enforce OS keyring via `TokenStore` interface; `KeyringStore` is the only implementation. |
| Stdio server process not cleaned up on crash | High | Use `defer client.Close()` in REPL wiring; `Close` kills process after bounded timeout. |
| SSRF via misconfigured HTTP MCP server URL | High | Dial-time `ValidateDestination` rejects private/loopback addresses. |
| MCP sampling requests grant server unintended LLM access | High | Reject sampling capability in `Initialize`; return explicit error to server. |
| Tool schema from MCP server causes JSON Schema parsing errors | Medium | Fall back to permissive schema with warning; never crash agent loop on malformed schema. |
| Name collision between MCP tools and built-in tools | Low | Built-in always wins; emit startup warning. |
| MCP tool names contain prompt-injection content | Medium | Sanitize to `[a-z0-9_]` only; names are never interpolated into shell commands. |
| HTTP hook latency degrades REPL responsiveness | Medium | Hook timeout from config; 10-second maximum; fail-open for non-blocking hooks. |
| `go-keyring` unavailable on headless Linux environments | Medium | Surface a clear error; fall back to no token (session proceeds without OAuth MCP servers). |
| Phase 10 adds multiple new dependencies at once | Medium | Add both to `tools/allowed-deps.txt` and `go.mod` with `go mod tidy`; run all checks before first commit. |

## Phase Log Template

When implementation finishes, append a Phase 10 entry to `docs/PHASE-LOG.md` with:

- objective;
- files created/updated;
- new dependencies and allowlist status;
- transport and auth design decisions;
- SSRF validation approach;
- HTTP hook enablement decision;
- tests run and manual smoke results;
- deferred work and known constraints;
- exit gate status.

## Exit Gate

Phase 10 is complete only when:

- all acceptance criteria above are met;
- `go test ./...` and security checks pass;
- manual stdio MCP server flow works end-to-end with a real local server;
- manual HTTP hook enabled flow works with a local test endpoint;
- SSRF validation test is confirmed to reject private IP ranges at dial time;
- OAuth PKCE flow test covers token caching, refresh, and full exchange;
- no orphan processes remain after REPL exit;
- phase log records the implementation, decisions, and any deviations from this plan.
