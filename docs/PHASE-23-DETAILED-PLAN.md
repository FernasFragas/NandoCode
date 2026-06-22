# Phase 23 Detailed Plan — OpenAI-Compatible LLM Adapter (Archived)

Date: 2026-05-07
Status: Archived; removed from active v0.1 roadmap
Source plans:

- `.codex/go-ollama-plan-AGENTS.md`
- `.codex/go-ollama-plan-HUMANS.md`
- `docs/PHASE-LOG.md`
- `docs/PROJECT-STATUS-AND-ONBOARDING.md`
- `docs/PHASE-8-DETAILED-PLAN.md`
- `docs/PHASE-9-DETAILED-PLAN.md`
- `book/ch04-api-layer.md`
- `book/ch05-agent-loop.md`

## Roadmap Placement

Phase 23 is no longer part of the active v0.1 roadmap. The current plan keeps v0.1 Ollama/local-provider based and removes the OpenAI-compatible adapter from implementation scope.

Do not implement this phase unless the roadmap decision is explicitly reversed. Phase 24, Phase 25, Phase 17, and Phase 18 no longer depend on this plan.

## Historical Goal

The rest of this document is retained only as historical design material. It is not an active implementation checklist.

Phase 23 adds a second implementation of `llm.Client` that speaks the OpenAI chat completions API. The existing `llm.Client` interface was designed to accommodate multiple providers from Phase 2; this phase instantiates that promise. After Phase 23, users can point `nandocodego` at vLLM, llama.cpp server, LM Studio, Groq, Together AI, Anthropic, and any other service that exposes `/v1/chat/completions` SSE streaming — without modifying the agent loop, tools, memory, hooks, MCP, or TUI.

The Ollama client remains the default and primary supported provider. The OpenAI adapter is an opt-in for users who need a hosted endpoint or prefer a cloud model for a specific session. Configuration lives in `config.toml`; the adapter is selected at startup via a provider factory.

Deliverables:

- `internal/llm/openai/client.go` — provider-neutral OpenAI-compatible HTTP client (~200 lines).
- `internal/llm/openai/stream.go` — SSE line reader and delta accumulator that handles fragmented tool-call argument streaming.
- `internal/llm/openai/convert.go` — bidirectional type conversion between `llm.*` types and OpenAI wire types.
- `internal/llm/openai/errors.go` — HTTP status code mapping to `llm.ErrorClass` values.
- `internal/llm/factory.go` — `NewClient(cfg Config) (llm.Client, error)` provider factory.
- `internal/llm/factory_test.go` — factory unit tests with both provider keys.
- `internal/llm/openai/client_test.go` — client tests with httptest.Server.
- `internal/llm/openai/stream_test.go` — SSE accumulator tests with fixture NDJSON/SSE data.
- `internal/llm/openai/testdata/` — fixture SSE response files.
- `internal/llm/capabilities.go` update — additional known model families.
- `internal/config/` update — `LLMConfig` struct with provider, model, base_url, api_key fields.
- `internal/cli/doctor.go` update — report configured LLM provider.
- `docs/PHASE-LOG.md` update — Phase 23 entry.

## Definition of Success

The Phase 23 exit gate is:

1. Configure `config.toml` with `[llm] provider = "openai"`, `base_url = "https://api.groq.com/openai/v1"`, and `model = "llama3-8b-8192"`.
2. Set `NANDOCODEGO_API_KEY=<groq-key>`.
3. Run `nandocodego` and submit a prompt that uses a file tool.
4. Confirm the tool call round-trip completes and the assistant message streams correctly.
5. `nandocodego doctor --llm` reports `provider: openai-compatible` and a successful endpoint ping.
6. Change `provider` back to `"ollama"` and confirm behavior is identical to pre-Phase-23.

This must work without any code changes to `internal/agent`, `internal/tools`, `internal/memory`, `internal/hooks`, `internal/tui`, or `internal/permissions`.

## Baseline Analysis from Implemented Phases

### Phase 0 — Security and Supply Chain

Implemented:

- `SECURITY.md`, dependency allowlist, network policy checker, CI security baseline.
- No-secrets policy for logs, memory, telemetry, and test fixtures.

Phase 23 implications:

- API keys are a new credential class. They must never be logged, never written to `config.toml`, never appear in memory files, task output JSONL, or error messages.
- API keys arrive only from `NANDOCODEGO_API_KEY` environment variable or, in a future phase, from the OS keyring via `github.com/zalando/go-keyring` (already allowlisted).
- The `tools/check-network-policy.sh` scanner currently allows only Ollama localhost. Phase 23 must not hard-code any non-localhost URL in source files. Base URLs come from config at runtime; the scanner should not flag them.
- `tools/allowed-deps.txt` requires no new direct dependencies for the OpenAI adapter. The standard library `net/http`, `bufio`, `strings`, and `encoding/json` handle SSE streaming. No new deps.
- Any OpenAI provider test that hits a real network endpoint must be gated behind `//go:build integration` and the `NANDOCODEGO_RUN_OPENAI_INTEGRATION=1` env var.

### Phase 1 — CLI, Paths, Logging, Scaffold

Implemented:

- `cmd/nandocodego`, Cobra root, `internal/paths`, `internal/logging`.
- `nandocodego doctor` command, version, scaffold.

Phase 23 implications:

- `nandocodego doctor` should gain a `--llm` flag that tests the configured provider endpoint with a minimal request (e.g., `POST /v1/models` list or a zero-token chat) and reports round-trip latency, status, and model name confirmation.
- Add `llm provider` line to the default `doctor` output reporting the configured provider name (`ollama` or `openai-compatible`).
- Logging should already mask secrets. Verify that the HTTP round-tripper used by the OpenAI client does not log `Authorization` headers at any level. Use a transport middleware that strips auth before any debug log.

### Phase 2 — LLM Client (Ollama)

Implemented:

- Provider-neutral `llm.Client` interface: `Chat`, `Embed`, `ListModels`, `PullModel`.
- `internal/llm/ollama/ollama.go` — hand-rolled HTTP, NDJSON streaming.
- `internal/llm/watchdog.go` — per-chunk idle timeout protection.
- `internal/llm/retry.go` — `RetryWithPolicy`, `ClassifyError`, `ErrorClass`.
- `internal/llm/capabilities.go` — model capability matrix.

Phase 23 implications:

- The `llm.Client` interface is the integration boundary. The OpenAI adapter must satisfy it exactly. No new methods, no method signature changes.
- `llm.StreamEvent` carries `Role`, `Content`, `Thinking`, `ToolCalls`, `Done`, `DoneReason`, `PromptEvalCount`, `EvalCount`. The OpenAI adapter must map all seven of these from SSE deltas. Unmapped fields (`Thinking` when the model has no reasoning field) should use zero values.
- `llm.WatchStream` is already provider-neutral (wraps any `<-chan llm.StreamEvent`). The OpenAI adapter's `Chat` method must return the same channel type so `WatchStream` applies automatically in `internal/agent/stream.go`.
- `ClassifyError` and `GetRetryPolicy` operate on `error` values. Phase 23 should extend `ClassifyError` to recognize HTTP 401 (→ `ErrorClassPermanent`), 429 (→ `ErrorClassRateLimit`, if that class is added), and 503 (→ `ErrorClassServer`). The Ollama client does not need this change; only the OpenAI errors path does.
- `Embed` for the OpenAI adapter should call `/v1/embeddings` if the provider supports it, or return `ErrNotSupported` if not. The memory recall path uses `llm.Client` for side-queries, not embeddings directly, so this is low priority.
- `ListModels` for the OpenAI adapter calls `GET /v1/models` and returns model IDs. `PullModel` returns `ErrNotSupported` — remote providers do not download weights locally.

### Phase 3 — Tool Interface and Starter Tools

Implemented:

- `tools.Tool`, `tools.Registry`, `tools.Context`.
- `Bash`, `FileRead`, `FileWrite`.
- `internal/tools/builtin`.

Phase 23 implications:

- Tool definitions flow from `tools.ToLLMToolDef` → `llm.ToolDef` → agent loop → `buildToolDefs` → chat request.
- The OpenAI adapter must convert `[]llm.ToolDef` to OpenAI `tools` array format in `convert.go`. The JSON shape differs: Ollama uses `{"type":"function","function":{...}}` inline; OpenAI uses the same shape but expects `"strict": false` and `"additionalProperties": false` on the schema object.
- No changes needed in `internal/tools`.

### Phase 4 — Agent Loop

Implemented:

- `agent.Agent.Run(ctx, input) <-chan agent.Event`.
- `internal/agent/stream.go` — `executeOneTurn`, `accumulateTurn`.
- `internal/agent/tools.go` — `executeToolCalls` via `permissions.Resolve`.

Phase 23 implications:

- `accumulateTurn` already accumulates `llm.StreamEvent` into an `llm.Message`. This logic is provider-neutral and needs no changes.
- `executeToolCalls` reads `msg.ToolCalls []llm.ToolCall`. The OpenAI adapter must produce `llm.ToolCall` values with valid `Name` and `Arguments` (as `map[string]any`). The agent loop does not inspect IDs, so the OpenAI `id` field can be stored in a future extension field or ignored.
- The agent's turn budget, watchdog, retry, context overflow, and stop-hook paths are all provider-neutral. No agent loop changes.

### Phase 5 — Permission System

Implemented:

- `internal/permissions` with seven modes, source-tagged rules, resolver.

Phase 23 implications:

- No permission changes needed. The OpenAI adapter executes tool calls whose inputs are validated the same way regardless of provider.

### Phase 6 — State Layer

Implemented:

- `internal/bootstrap.State` — model, Ollama endpoint, budgets.
- `internal/state.Store[state.App]`.

Phase 23 implications:

- `bootstrap.Initial` currently holds `OllamaBaseURL` and `Model`. Phase 23 adds `LLMProvider string` and `LLMBaseURL string` to `bootstrap.Initial`. The Ollama-specific field should be preserved for backward compatibility.
- `state.OnChange` mirrors model and other infrastructure fields into bootstrap. Add `LLMProvider` and `LLMBaseURL` to mirrored fields.

### Phase 7 — Bubble Tea TUI and REPL

Implemented:

- Full interactive REPL, permission modal, transcript rendering, minimal slash commands.

Phase 23 implications:

- No TUI changes required. The TUI consumes `agent.Event` values which are provider-neutral.
- `nandocodego doctor --llm` output is plain text printed by `internal/cli/doctor.go`. No TUI rendering needed.

### Phase 8 — Memory

Implemented:

- `internal/memory` with recall side-queries via `llm.Client.Chat`.

Phase 23 implications:

- Memory recall uses `llm.Client` directly. When the OpenAI provider is configured, recall side-queries go through the OpenAI adapter. This is correct: the recall model should match the chat model so context token counting is consistent.
- Extraction also uses `llm.Client`. Same pass-through logic applies.
- No memory package changes.

### Phase 9 — Hooks

Implemented:

- `internal/hooks` — command and prompt hook types, snapshot-based dispatch.

Phase 23 implications:

- Prompt hooks use `llm.Client` for structured-output requests. When the OpenAI adapter is active, prompt hooks call OpenAI. This requires the OpenAI adapter's non-streaming path (`stream: false`) to work correctly.
- Non-streaming responses are a degenerate case of the streaming path. The accumulator should handle a single `data: [DONE]` SSE event after one full `data: {...}` event.

### Phases 10–22 Summary

The following phases have been implemented and their artifacts are provider-neutral:

- Phase 10 (MCP): MCP tool results arrive as `llm.Message` tool-result role. Provider-neutral.
- Phase 11 (Sub-agents and fork): Sub-agents use `agent.Agent.Run`. Provider-neutral — a sub-agent can use a different `llm.Client` if configured with one.
- Phase 12 (Skills): Skills are prompt templates that feed into `agent.Input.SystemPrompt`. Provider-neutral.
- Phase 13 (Slash commands and config UX): Config loading reads `config.toml`. Phase 23 adds the `[llm]` section to the schema. The config UX (`/model`, `/config`) should surface the provider field.
- Phase 14 (Tasks and TaskSupervisor): Task output is JSONL. Provider-neutral.
- Phase 15 (Concurrency): Tool partitioning operates on `tools.Tool` metadata. Provider-neutral.
- Phase 16 (Observability): Logging and metric decorators wrap `llm.Client`. The decorator wraps the concrete client returned by the factory, so metrics are provider-aware by label.
- Future Phase 17 (Distribution): binary build. Provider selection is a runtime config choice; no build changes.
- Future Phase 18 (Hardening): evals and docs. Phase 23 adds an OpenAI eval fixture if this provider is in release scope.
- Phases 19–22: HTTP server, session lifecycle, SSE streaming server. These phases expose `agent.Agent` over HTTP. The HTTP server is provider-neutral; it creates an `agent.Agent` using whatever `llm.Client` the factory returns.

## Deep Analysis of the OpenAI Chat Completions Streaming Protocol

The OpenAI streaming protocol differs from Ollama's NDJSON in three important ways that the adapter must handle correctly.

### SSE Framing

Ollama sends one JSON object per line (NDJSON). OpenAI sends Server-Sent Events with the format:

```
data: {"id":"...","object":"chat.completion.chunk","choices":[{"delta":{...}}]}\n\n
data: [DONE]\n\n
```

Each event is prefixed by `data: `. The termination sentinel is the literal string `[DONE]`, not a JSON object. The accumulator must:

1. Read lines with `bufio.Scanner`.
2. Skip lines that do not start with `data: `.
3. Stop on `data: [DONE]`.
4. Strip the `data: ` prefix and unmarshal the remaining JSON.

### Tool Call Fragment Accumulation

This is the most complex difference from Ollama. In Ollama, the tool call arrives as a complete object in the final `message` of the response. In OpenAI streaming, tool call arguments arrive as `arguments` string fragments spread across multiple deltas:

```
delta: {"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"Bash","arguments":""}}]}
delta: {"tool_calls":[{"index":0,"function":{"arguments":"{\"command\""}}]}
delta: {"tool_calls":[{"index":0,"function":{"arguments":":\"ls -la\"}"}}]}
delta: {}  (content delta, no tool_calls)
```

The accumulator must:

1. Keep a `map[int]*accumulatedToolCall` keyed by the `index` field.
2. On each delta with `tool_calls`, merge: first delta that has an `id` and `name` initializes the entry; subsequent deltas append to `Arguments`.
3. After `[DONE]`, unmarshal each accumulated `Arguments` string as JSON to produce the `map[string]any` expected by `llm.ToolCall.Function.Arguments`.
4. If `Arguments` is not valid JSON (model error), return an error event rather than silently producing a nil arguments map.

### Thinking / Reasoning Content

Some OpenAI-compatible providers return reasoning in a non-standard field:

- Deepseek: `delta.reasoning_content` (string, streamed alongside or before `content`)
- OpenAI o1/o3: `delta.refusal` or injected into `content` as a special marker (provider-dependent; handle as content for now)
- Anthropic: thinking comes in a separate content block type (out of scope for v1)

Phase 23 maps `reasoning_content` to `llm.StreamEvent.Thinking`. Other providers that do not have this field produce `Thinking: ""` (zero value), which the TUI renders identically to Ollama models without thinking.

## Evaluation of the Original Phase 23 Concept

The original concept is correct at the product level:
- `llm.Client` interface was designed for this; the adapter is ~200 lines.
- SSE streaming is straightforward with `bufio.Scanner`.
- The factory pattern is the right integration point.
- No agent-loop changes required.

It needs more implementation detail for this repo:

- It does not specify how `PullModel` and `Embed` behave for providers that do not support them.
- It does not specify the exact `config.toml` section structure or the bootstrap integration path.
- It does not specify how the `NANDOCODEGO_API_KEY` env var interacts with `config.toml`'s `api_key` field (env var wins).
- It does not specify the non-streaming code path needed by prompt hooks.
- It does not define how the `doctor --llm` flag tests the endpoint.
- It does not address the `tools/check-network-policy.sh` scanner impact.

## Final Phase 23 Scope

In scope:

- `internal/llm/openai` package with client, stream accumulator, type converter, and errors.
- `internal/llm/factory.go` provider factory.
- Config schema `[llm]` section with provider, model, base_url, api_key fields.
- API key from env var `NANDOCODEGO_API_KEY`, never logged, never written to config file.
- `nandocodego doctor` updated to report LLM provider.
- `nandocodego doctor --llm` endpoint health check.
- `internal/llm/capabilities.go` extended with OpenAI-compatible model families.
- Unit tests and httptest.Server-based integration tests (no live network in CI).
- Phase log update.

Out of scope:

- OS keyring storage for API keys (future credential-management follow-up).
- Per-provider retry budgets beyond the existing `GetRetryPolicy` extension (future provider-hardening follow-up).
- OpenAI function-calling `strict` mode (future schema/tool-definition follow-up).
- Anthropic-specific content block format (not OpenAI-compatible; separate adapter if needed).
- Streaming token count from OpenAI's `usage` chunk (optional field in streaming; parse if present, ignore if absent).
- Provider-specific model capability discovery via API (use static matrix for now).
- Config hot-reload for provider switch (Phase 13 config watching must trigger factory re-initialization; stub the hook but leave implementation to a future config-runtime follow-up).

## Target Configuration UX

### config.toml

```toml
[llm]
provider = "ollama"      # "ollama" | "openai" — default: "ollama"
model    = "qwen3:14b"   # model name; validated against provider on startup
base_url = "http://localhost:11434"  # Ollama default; override for cloud

# api_key: never set here; use NANDOCODEGO_API_KEY env var
# For non-local providers, set NANDOCODEGO_API_KEY before starting.
```

### Environment Variables

```
NANDOCODEGO_API_KEY   — API key for OpenAI-compatible provider (read-only from env; never written)
```

### Doctor Output (default)

```
LLM Provider:
  Provider:  ollama
  Base URL:  http://localhost:11434
  Model:     qwen3:14b
  Status:    ✓ reachable (42ms)
```

### Doctor Output (--llm flag)

```
LLM Provider Extended:
  Provider:     openai-compatible
  Base URL:     https://api.groq.com/openai/v1
  Model:        llama3-8b-8192
  Auth:         key set (not shown)
  Ping:         POST /v1/models → 200 OK (138ms)
  Stream test:  1-token chat → streamed correctly (212ms)
  Capabilities: tools=yes  thinking=no  images=no
```

## Architecture

### Package Layout

```text
internal/llm/
  factory.go
  factory_test.go
  openai/
    client.go
    stream.go
    convert.go
    errors.go
    client_test.go
    stream_test.go
    testdata/
      stream_text_only.sse
      stream_tool_call.sse
      stream_tool_call_fragmented.sse
      stream_reasoning_content.sse
      stream_empty.sse
      stream_error_400.json
```

### Core Types

```go
// internal/llm/openai/client.go

type Client struct {
    baseURL    string
    apiKey     string
    model      string
    httpClient *http.Client
    logger     *slog.Logger
}

func NewClient(baseURL, apiKey, model string, logger *slog.Logger) (*Client, error)

// Satisfies llm.Client
func (c *Client) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error)
func (c *Client) Embed(ctx context.Context, model string, texts []string) ([][]float32, error)
func (c *Client) ListModels(ctx context.Context) ([]llm.ModelInfo, error)
func (c *Client) PullModel(ctx context.Context, model string, progress chan<- llm.PullProgress) error
```

```go
// internal/llm/openai/stream.go

// OpenAI wire types for SSE deltas
type openAIChunk struct {
    ID      string          `json:"id"`
    Object  string          `json:"object"`
    Choices []openAIChoice  `json:"choices"`
    Usage   *openAIUsage    `json:"usage"`
}

type openAIChoice struct {
    Index        int         `json:"index"`
    Delta        openAIDelta `json:"delta"`
    FinishReason string      `json:"finish_reason"`
}

type openAIDelta struct {
    Role             string           `json:"role"`
    Content          string           `json:"content"`
    ReasoningContent string           `json:"reasoning_content"` // Deepseek R1
    ToolCalls        []openAIToolCall  `json:"tool_calls"`
}

type openAIToolCall struct {
    Index    int    `json:"index"`
    ID       string `json:"id"`
    Type     string `json:"type"`
    Function struct {
        Name      string `json:"name"`
        Arguments string `json:"arguments"` // Accumulated across deltas
    } `json:"function"`
}

type openAIUsage struct {
    PromptTokens     int `json:"prompt_tokens"`
    CompletionTokens int `json:"completion_tokens"`
}

// Accumulator state per streaming response
type streamAccumulator struct {
    toolCalls map[int]*accumulatedToolCall
    content   strings.Builder
    thinking  strings.Builder
    promptTok int
    evalTok   int
}

type accumulatedToolCall struct {
    id        string
    name      string
    argsJSON  strings.Builder
}

// ReadStream reads SSE from r, converts each event to llm.StreamEvent,
// and closes out when data: [DONE] is received.
func ReadStream(ctx context.Context, r io.Reader, out chan<- llm.StreamEvent)
```

```go
// internal/llm/openai/convert.go

// ToOpenAIMessages converts []llm.Message to OpenAI messages array.
func ToOpenAIMessages(msgs []llm.Message) []openAIMessage

// ToOpenAITools converts []llm.ToolDef to OpenAI tools array.
func ToOpenAITools(tools []llm.ToolDef) []openAITool

// FromOpenAIDelta converts an openAIDelta to a partial llm.StreamEvent.
func FromOpenAIDelta(delta openAIDelta) llm.StreamEvent

// FromAccumulator converts a finalized streamAccumulator to a terminal llm.StreamEvent.
func FromAccumulator(acc *streamAccumulator) (llm.StreamEvent, error)
```

```go
// internal/llm/factory.go

type Config struct {
    Provider string // "ollama" | "openai"
    BaseURL  string
    APIKey   string // from env only; never from config file
    Model    string
}

// NewClient returns an llm.Client for the configured provider.
// Returns an Ollama client when Provider is "" or "ollama" (backward-compatible default).
func NewClient(cfg Config, logger *slog.Logger) (llm.Client, error)
```

### Request Serialization

```go
// openAIRequest is the JSON body sent to /v1/chat/completions
type openAIRequest struct {
    Model    string           `json:"model"`
    Messages []openAIMessage  `json:"messages"`
    Tools    []openAITool     `json:"tools,omitempty"`
    Stream   bool             `json:"stream"`
    // Options forwarded from llm.ChatRequest.Options:
    MaxTokens   int     `json:"max_tokens,omitempty"`
    Temperature float64 `json:"temperature,omitempty"`
    TopP        float64 `json:"top_p,omitempty"`
}
```

### Retry and Watchdog Integration

The OpenAI adapter's `Chat` method must integrate with existing infrastructure identically to the Ollama client:

1. Return a `<-chan llm.StreamEvent` immediately.
2. Start a goroutine that reads SSE, converts events, and sends them on the channel.
3. Close the channel after the terminal event (done=true or error).
4. The caller in `internal/agent/stream.go` wraps the returned channel with `llm.WatchStream`.
5. Retry logic in `internal/agent/agent.go` calls `Chat` again on watchdog timeout.

The OpenAI adapter does NOT need its own retry loop inside `Chat`. The agent loop handles retries.

### Error Mapping

```go
// internal/llm/openai/errors.go

// HTTPStatusToErrorClass maps OpenAI HTTP status codes to llm.ErrorClass.
func HTTPStatusToErrorClass(status int) llm.ErrorClass
// 200: no error
// 400: ErrorClassBadRequest (0 retries)
// 401: ErrorClassPermanent (0 retries; bad key)
// 404: ErrorClassNotFound (1 retry; model may not exist)
// 429: ErrorClassRateLimit (3 retries, linear backoff)
// 5xx: ErrorClassServer (5 retries, exponential backoff)
```

`ClassifyError` in `internal/llm/retry.go` should be extended to check for a new `openAIError` sentinel type that carries the HTTP status.

## Implementation Plan

### Step 1 — Config Schema Extension

Files:

- `internal/config/config.go` (update `LLMConfig` struct)
- `internal/bootstrap/state.go` (add `LLMProvider`, `LLMBaseURL` fields to `Initial`)
- `internal/state/onchange.go` (mirror new fields)

Implement:

- `LLMConfig` struct with `Provider`, `BaseURL`, `APIKey`, `Model` fields.
- Default: `Provider = "ollama"`, `BaseURL = "http://localhost:11434"`.
- `APIKey` is populated at startup from `os.Getenv("NANDOCODEGO_API_KEY")` and never written back to config.
- Bootstrap `Initial` gains `LLMProvider string` and `LLMBaseURL string`.
- `state.OnChange` mirrors `LLMProvider` and `LLMBaseURL` into bootstrap.

Tests:

- Config parses TOML `[llm]` section.
- Missing `[llm]` section produces Ollama defaults.
- `api_key` field in TOML is rejected with an error message ("use NANDOCODEGO_API_KEY env var").
- Bootstrap mirrors `LLMProvider` change correctly.

### Step 2 — Provider Factory

Files:

- `internal/llm/factory.go`
- `internal/llm/factory_test.go`

Implement:

- `NewClient(cfg Config, logger *slog.Logger) (llm.Client, error)`.
- `cfg.Provider == "" || cfg.Provider == "ollama"`: return `ollama.NewClient(cfg.BaseURL)`.
- `cfg.Provider == "openai"`: return `openai.NewClient(cfg.BaseURL, cfg.APIKey, cfg.Model, logger)`.
- Unknown provider: return descriptive error.
- API key redacted in all error strings and logs.

Tests:

- Factory returns Ollama client for default config.
- Factory returns Ollama client for explicit `provider = "ollama"`.
- Factory returns OpenAI client for `provider = "openai"`.
- Factory returns error for unknown provider.
- Factory test stubs `openai.NewClient` via interface to avoid live network.

### Step 3 — OpenAI Wire Types and Converter

Files:

- `internal/llm/openai/convert.go`

Implement:

- `ToOpenAIMessages`: map `llm.Message` roles to OpenAI role strings; serialize tool-result messages with `tool_call_id`; handle multi-content messages.
- `ToOpenAITools`: convert `llm.ToolDef` to `{"type":"function","function":{...}}` with schema.
- `fromOpenAIFinishReason`: `"stop"` → `"stop"`, `"tool_calls"` → `"tool_calls"`, `"length"` → `"length"`, `"content_filter"` → `"stop"`.

Tests:

- `ToOpenAIMessages` round-trips all four roles.
- Tool-result messages get correct `tool_call_id`.
- System messages map correctly.
- Multi-turn with thinking produces no `reasoning_content` in outgoing messages (thinking is not fed back).

### Step 4 — SSE Stream Reader and Accumulator

Files:

- `internal/llm/openai/stream.go`
- `internal/llm/openai/stream_test.go`
- `internal/llm/openai/testdata/`

Implement:

- `ReadStream(ctx context.Context, r io.Reader, out chan<- llm.StreamEvent)` — runs in a goroutine.
- Line-by-line reading with `bufio.Scanner`.
- Skip blank lines, `event:` lines, and `: keepalive` comment lines.
- Parse `data: {...}` chunks into `openAIChunk`.
- Stop on `data: [DONE]`.
- Use `streamAccumulator` to merge tool call fragments across deltas.
- Emit one `llm.StreamEvent` per content delta (for streaming UX responsiveness).
- On `[DONE]`: finalize accumulator, unmarshal accumulated tool-call arguments, emit final event with `Done: true`.
- Context cancellation: exit cleanly, close channel, no goroutine leak.

Tool call fragment accumulation:

- `accumulatedToolCall` per index in `map[int]*accumulatedToolCall`.
- First delta with non-empty `id` and `name` initializes the entry.
- Subsequent deltas append `Function.Arguments` string.
- On finalization: `json.Unmarshal(argsJSON, &map[string]any{})` per tool call.
- If unmarshal fails: emit error event rather than nil arguments.

Tests (using `testdata/` fixture files):

- `stream_text_only.sse` — text-only response produces correct `AssistantTextDelta` events.
- `stream_tool_call.sse` — single complete tool call (non-fragmented) produces correct `ToolCall`.
- `stream_tool_call_fragmented.sse` — arguments spread across 5 deltas produce correct `map[string]any`.
- `stream_reasoning_content.sse` — `reasoning_content` deltas map to `Thinking` field.
- `stream_empty.sse` — empty response (`[DONE]` only) produces single terminal event.
- Context cancellation mid-stream exits without goroutine leak (verified with `goleak` or manual goroutine count).
- Malformed JSON in arguments delta produces error event.

### Step 5 — OpenAI HTTP Client

Files:

- `internal/llm/openai/client.go`
- `internal/llm/openai/client_test.go`

Implement:

- `NewClient(baseURL, apiKey, model string, logger *slog.Logger) (*Client, error)`.
- Validate `baseURL` is non-empty.
- Redact `apiKey` from any logger calls.
- `Chat(ctx, req) (<-chan llm.StreamEvent, error)`:
  - Build `openAIRequest` from `req` using `convert.ToOpenAIMessages`, `convert.ToOpenAITools`.
  - Set `"stream": true`.
  - POST to `baseURL + "/v1/chat/completions"` with `Authorization: Bearer <apiKey>`.
  - On non-2xx: read body, return `openAIError` with HTTP status.
  - On 2xx: create channel, start goroutine calling `ReadStream`, return channel.
- `Embed(ctx, model, texts) ([][]float32, error)`:
  - POST to `baseURL + "/v1/embeddings"`.
  - Return `ErrNotSupported` if provider returns 404 or 501.
- `ListModels(ctx) ([]llm.ModelInfo, error)`:
  - GET `baseURL + "/v1/models"`.
  - Map `model.id` to `llm.ModelInfo.Name`.
- `PullModel(ctx, model, progress) error`:
  - Return `ErrNotSupported` immediately.

HTTP client configuration:

- Default `http.Client` with `Timeout: 0` (streaming requires no global timeout; watchdog handles per-chunk idle).
- Set `Accept: text/event-stream` header on streaming requests.
- Set `Content-Type: application/json` on all POST requests.
- Strip `Authorization` header before any debug log of outgoing requests.

Tests (using `httptest.NewServer`):

- `Chat` returns channel that streams events from fixture SSE response.
- `Chat` returns error on HTTP 401 with `ErrorClassPermanent`.
- `Chat` returns error on HTTP 429 with `ErrorClassRateLimit`.
- `Chat` returns error on HTTP 500 with `ErrorClassServer`.
- `ListModels` parses OpenAI model list response.
- `PullModel` returns `ErrNotSupported`.
- `Embed` calls `/v1/embeddings` and parses float vectors.
- Context cancellation on `Chat` closes the response body (no goroutine leak).
- Auth header is stripped from request logs (verify logger output does not contain `Bearer`).

### Step 6 — Error Classification Extension

Files:

- `internal/llm/openai/errors.go`
- `internal/llm/retry.go` (extend `ClassifyError`)

Implement:

- `openAIError` type implementing `error` with `StatusCode int` and `Message string`.
- `HTTPStatusToErrorClass(status int) llm.ErrorClass`.
- Extend `ClassifyError` to handle `*openAIError` before the generic fallback.

Tests:

- 401 → `ErrorClassPermanent`.
- 429 → `ErrorClassRateLimit` (new class) or `ErrorClassServer` if rate-limit class not added.
- 500 → `ErrorClassServer`.
- 400 → `ErrorClassBadRequest`.
- Existing Ollama-specific error cases still classified correctly.

### Step 7 — Capabilities Matrix Extension

Files:

- `internal/llm/capabilities.go`

Add entries for known OpenAI-compatible model families:

```go
// OpenAI hosted
"gpt-4o": {Tools: true, Thinking: false, Images: true, RecommendedCtx: 128000},
"gpt-4":  {Tools: true, Thinking: false, Images: false, RecommendedCtx: 8192},
"gpt-3.5": {Tools: true, Thinking: false, Images: false, RecommendedCtx: 4096},
// OpenAI reasoning
"o1": {Tools: false, Thinking: true, Images: false, RecommendedCtx: 128000},
"o3": {Tools: true,  Thinking: true, Images: false, RecommendedCtx: 200000},
// Deepseek
"deepseek-r1": {Tools: true, Thinking: true, Images: false, RecommendedCtx: 64000},
"deepseek-v3": {Tools: true, Thinking: false, Images: false, RecommendedCtx: 128000},
// Mistral (via Mistral API or vLLM)
"mistral-large":  {Tools: true, Thinking: false, Images: false, RecommendedCtx: 32000},
"mistral-nemo":   {Tools: true, Thinking: false, Images: false, RecommendedCtx: 128000},
// Meta Llama (via Groq, Together, vLLM)
"llama3-70b": {Tools: true, Thinking: false, Images: false, RecommendedCtx: 8192},
"llama3-8b":  {Tools: true, Thinking: false, Images: false, RecommendedCtx: 8192},
```

`normalizeModelName` must handle OpenAI naming conventions: `gpt-4o-2024-11-20` → `gpt-4o`, `deepseek-r1:70b` → `deepseek-r1`.

Tests:

- Seven new model name normalization cases.
- Capability lookup for all new families returns expected struct.

### Step 8 — Doctor Command Updates

Files:

- `internal/cli/doctor.go`

Implement:

- Default `doctor` output: add "LLM Provider" section showing `provider`, `base_url` (redacted if cloud), `model`, and reachability status.
- `--llm` flag: perform endpoint health check:
  1. `GET /v1/models` (or equivalent for Ollama: `GET /api/tags`).
  2. If successful, report latency and model list size.
  3. Optionally: 1-token streaming chat to confirm streaming works.
- Auth key status: "key set" or "no key" — never print key value.

Tests:

- `doctor` output contains "LLM Provider" section.
- `doctor --llm` with httptest server returns success report.
- `doctor --llm` with unreachable server returns failure report (non-zero exit).

### Step 9 — REPL Wiring

Files:

- `internal/cli/repl.go`

Update `runREPL` to:

1. Read `[llm]` section from config.
2. Populate `llm.factory.Config` from config + env var.
3. Call `llm.NewClient(factoryCfg, logger)` instead of `ollama.NewClient(baseURL)`.
4. Pass the resulting `llm.Client` to `agent.New`.

This is a small change because the factory is the new integration point. The REPL does not need to know which provider is active.

Tests:

- `runREPL` with `provider = "ollama"` config creates Ollama client (verified via mock factory).
- `runREPL` with `provider = "openai"` config creates OpenAI client (verified via mock factory).

### Step 10 — Integration and Race Tests

Required commands:

```sh
go test -race ./internal/llm/...
go test -race ./internal/cli/...
go test ./internal/agent/...
tools/check-allowed-deps.sh
tools/check-network-policy.sh
```

Optional integration test (requires `NANDOCODEGO_RUN_OPENAI_INTEGRATION=1` and valid `NANDOCODEGO_API_KEY`):

```sh
go test -tags=integration -run TestOpenAIClientIntegration ./internal/llm/openai
```

Manual smoke test with Groq (or any OpenAI-compatible provider):

```sh
cat > ~/.nandocodego/config.toml << 'EOF'
[llm]
provider = "openai"
base_url = "https://api.groq.com/openai/v1"
model    = "llama3-8b-8192"
EOF
export NANDOCODEGO_API_KEY=<groq-key>
go run ./cmd/nandocodego --no-alt-screen
```

Submit: "List files in the current directory" and confirm `Bash ls` tool call completes.

## Implementation Checklist

### Config and Factory

- [ ] Add `LLMConfig` struct to `internal/config/config.go` with `Provider`, `BaseURL`, `APIKey`, `Model` fields.
- [ ] Add TOML parsing for `[llm]` section in config loader.
- [ ] Return error if `api_key` appears in TOML config ("use NANDOCODEGO_API_KEY env var").
- [ ] Populate `APIKey` from `os.Getenv("NANDOCODEGO_API_KEY")` at startup, not from config.
- [ ] Add `LLMProvider string` and `LLMBaseURL string` to `bootstrap.Initial`.
- [ ] Update `bootstrap.DefaultInitial` to set `LLMProvider = "ollama"` and `LLMBaseURL = "http://localhost:11434"`.
- [ ] Update `state.OnChange` to mirror `LLMProvider` and `LLMBaseURL` changes into bootstrap.
- [ ] Implement `internal/llm/factory.go` with `Config` struct and `NewClient(cfg, logger)` function.
- [ ] Factory returns Ollama client for provider `""` and `"ollama"`.
- [ ] Factory returns OpenAI client for provider `"openai"`.
- [ ] Factory returns descriptive error for unknown provider (does not include api_key value).
- [ ] Write `internal/llm/factory_test.go` with four test cases.
- [ ] Update `internal/cli/repl.go` to call `llm.NewClient` instead of `ollama.NewClient`.

### Type Conversion

- [ ] Implement `internal/llm/openai/convert.go` with `ToOpenAIMessages`, `ToOpenAITools`, `fromOpenAIFinishReason`.
- [ ] Map `llm.RoleSystem` → `"system"`, `llm.RoleUser` → `"user"`, `llm.RoleAssistant` → `"assistant"`, `llm.RoleTool` → `"tool"`.
- [ ] Serialize tool-result messages with `tool_call_id` field populated from `llm.Message.ToolCallID` (add field if missing).
- [ ] Thinking content is NOT included in outgoing messages to the model.
- [ ] `ToOpenAITools` outputs `{"type":"function","function":{...}}` array.
- [ ] Add `"additionalProperties": false` to schema objects in tool definitions.
- [ ] Write `convert_test.go` with round-trip tests for all four roles and multi-turn conversations.

### SSE Stream Reader

- [ ] Implement `internal/llm/openai/stream.go` with `ReadStream`, `streamAccumulator`, `accumulatedToolCall`.
- [ ] `ReadStream` uses `bufio.Scanner` with default buffer size.
- [ ] Skip blank lines and lines not starting with `data: `.
- [ ] Stop on `data: [DONE]`.
- [ ] Parse each `data: {...}` line into `openAIChunk`.
- [ ] Emit one `llm.StreamEvent` per content character run for streaming responsiveness.
- [ ] On each delta with `tool_calls`, update `streamAccumulator.toolCalls` map by index.
- [ ] First delta with non-empty `id` initializes `accumulatedToolCall.id` and `name`.
- [ ] Subsequent deltas append to `accumulatedToolCall.argsJSON`.
- [ ] Map `reasoning_content` deltas to `llm.StreamEvent.Thinking`.
- [ ] On `[DONE]`: for each accumulated tool call, unmarshal `argsJSON` to `map[string]any`.
- [ ] If argument unmarshal fails, emit `llm.StreamEvent` with `Error` field set.
- [ ] Parse `usage` chunk when present and emit token counts in terminal event.
- [ ] Context cancellation causes clean exit — no goroutine leak.
- [ ] Create `testdata/stream_text_only.sse` fixture.
- [ ] Create `testdata/stream_tool_call.sse` fixture.
- [ ] Create `testdata/stream_tool_call_fragmented.sse` fixture (5 argument-fragment deltas).
- [ ] Create `testdata/stream_reasoning_content.sse` fixture.
- [ ] Create `testdata/stream_empty.sse` fixture.
- [ ] Write `stream_test.go` with one test per fixture file.
- [ ] Write goroutine leak test for context cancellation.

### HTTP Client

- [ ] Implement `internal/llm/openai/client.go` with `Client` struct and `NewClient`.
- [ ] `NewClient` validates non-empty `baseURL`.
- [ ] `Chat` builds `openAIRequest` using `convert.ToOpenAIMessages` and `convert.ToOpenAITools`.
- [ ] `Chat` sets `"stream": true`.
- [ ] `Chat` sets `Authorization: Bearer <apiKey>` header; header is stripped before any debug log.
- [ ] `Chat` sets `Content-Type: application/json` and `Accept: text/event-stream`.
- [ ] On non-2xx response: read body, return `openAIError` with `StatusCode` and `Message`.
- [ ] On 2xx: create buffered channel, start `ReadStream` goroutine, return channel.
- [ ] `Embed` calls `/v1/embeddings` and returns `[][]float32`.
- [ ] `Embed` returns `ErrNotSupported` on 404 or 501.
- [ ] `ListModels` calls `GET /v1/models` and parses `{"data":[{"id":"..."},...]}`.
- [ ] `PullModel` returns `ErrNotSupported`.
- [ ] Write `client_test.go` using `httptest.NewServer`.
- [ ] Test `Chat` happy path with fixture SSE server.
- [ ] Test `Chat` HTTP 401 → `ErrorClassPermanent`.
- [ ] Test `Chat` HTTP 429 → `ErrorClassRateLimit` or `ErrorClassServer`.
- [ ] Test `Chat` HTTP 500 → `ErrorClassServer`.
- [ ] Test `ListModels` with sample OpenAI model list JSON.
- [ ] Test `PullModel` returns `ErrNotSupported`.
- [ ] Test `Embed` with sample embeddings JSON.
- [ ] Test context cancellation on `Chat` closes response body.
- [ ] Verify `Authorization` header does not appear in any log line (scan logger output in test).

### Error Classification

- [ ] Implement `internal/llm/openai/errors.go` with `openAIError` type and `HTTPStatusToErrorClass`.
- [ ] 200 range: no error.
- [ ] 400: `ErrorClassBadRequest`.
- [ ] 401: `ErrorClassPermanent`.
- [ ] 403: `ErrorClassPermanent`.
- [ ] 404: `ErrorClassNotFound`.
- [ ] 429: `ErrorClassRateLimit` (add new class if not present, or reuse `ErrorClassServer`).
- [ ] 5xx: `ErrorClassServer`.
- [ ] Extend `ClassifyError` in `internal/llm/retry.go` to handle `*openAIError`.
- [ ] Existing Ollama error classification tests still pass.

### Capabilities

- [ ] Add `gpt-4o`, `gpt-4`, `gpt-3.5` entries to capabilities matrix.
- [ ] Add `o1`, `o3` with `Thinking: true`.
- [ ] Add `deepseek-r1` with `Thinking: true`, `deepseek-v3` without thinking.
- [ ] Add `mistral-large`, `mistral-nemo`.
- [ ] Add `llama3-70b`, `llama3-8b`.
- [ ] Update `normalizeModelName` to strip version date suffixes (`-2024-11-20` → `""`).
- [ ] Write tests for all new normalization patterns.
- [ ] Write tests for all new capability lookups.

### Doctor Updates

- [ ] Add "LLM Provider" section to default `doctor` output.
- [ ] Report `provider`, `base_url` (full for localhost, host-only for cloud), `model`.
- [ ] Report reachability status (latency ms or error).
- [ ] Add `--llm` flag to `doctor` command.
- [ ] `--llm`: call `ListModels` and report success + latency.
- [ ] `--llm`: perform 1-token streaming chat, confirm stream completes.
- [ ] `--llm`: report API key status (`"set"` or `"not set"`) without printing the key.
- [ ] `--llm`: report capabilities for configured model.
- [ ] `doctor --llm` exits non-zero if endpoint unreachable.
- [ ] Tests for doctor output with mocked providers.

### Integration and Final Checks

- [ ] Run `go test -race ./internal/llm/...` — clean.
- [ ] Run `go test -race ./internal/cli/...` — clean.
- [ ] Run `go test ./internal/agent/...` — no regressions.
- [ ] Run `tools/check-allowed-deps.sh` — no new dependencies added.
- [ ] Run `tools/check-network-policy.sh` — no hardcoded URLs in source.
- [ ] Run `go vet ./...` — clean.
- [ ] Confirm no `Authorization` header value appears in any log output at any level.
- [ ] Confirm API key never written to disk, never included in error messages.
- [ ] Smoke test: Ollama provider works identically to pre-Phase-23.
- [ ] Smoke test (optional): OpenAI provider with Groq endpoint completes a tool call.
- [ ] Update `docs/PHASE-LOG.md` with Phase 23 entry.

## Acceptance Criteria

- [ ] `llm.NewClient(cfg)` returns Ollama client when `provider` is `"ollama"` or empty — no behavior change for existing users.
- [ ] `llm.NewClient(cfg)` returns OpenAI-compatible client when `provider` is `"openai"`.
- [ ] OpenAI client satisfies `llm.Client` interface — compile-time verified via interface assertion.
- [ ] SSE stream reader handles text-only responses, tool call responses, and fragmented tool call argument responses.
- [ ] Fragmented tool call arguments are accumulated and correctly unmarshaled across five+ delta events.
- [ ] `reasoning_content` field is mapped to `llm.StreamEvent.Thinking`.
- [ ] `llm.WatchStream` and `llm.RetryWithPolicy` work identically with the OpenAI client (no code changes needed in those packages).
- [ ] API key is sourced only from `NANDOCODEGO_API_KEY` env var — never from config file, never logged, never in error strings.
- [ ] Attempting to set `api_key` in `config.toml` produces a user-friendly error with redirect to env var.
- [ ] `nandocodego doctor` reports LLM provider section.
- [ ] `nandocodego doctor --llm` tests the configured provider endpoint and exits non-zero on failure.
- [ ] Capabilities matrix includes `gpt-4o`, `deepseek-r1`, `mistral-large`, and `llama3-70b` entries.
- [ ] `go test -race ./internal/llm/...` passes with zero race detector reports.
- [ ] `tools/check-allowed-deps.sh` passes — no new direct dependencies.
- [ ] `tools/check-network-policy.sh` passes — no hardcoded non-localhost URLs in source.
- [ ] All pre-Phase-23 tests pass without modification.
- [ ] `PullModel` on OpenAI client returns `ErrNotSupported` (not a panic or network call).
- [ ] HTTP 401 from OpenAI provider produces `ErrorClassPermanent` — no retry.
- [ ] HTTP 429 from OpenAI provider produces at most 3 retries.
- [ ] Context cancellation on a streaming `Chat` call closes the HTTP response body and exits the goroutine cleanly.
- [ ] `go vet ./...` clean.
- [ ] `Authorization` header value does not appear in any log output at `DEBUG` level or below (verified in test).
- [ ] Phase log updated with Phase 23 completion entry.
- [ ] Existing Ollama client tests unchanged and still passing.
- [ ] `nandocodego doctor --llm` with unreachable endpoint exits non-zero.
- [ ] Factory rejects unknown `provider` value with a descriptive error.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---:|---|
| API key logged accidentally | High | Strip `Authorization` header before all debug logs; test log output in unit tests. |
| Tool call argument fragmentation causes invalid JSON | High | Accumulate all fragments before unmarshal; emit error event on invalid JSON rather than silently nil. |
| `[DONE]` sentinel not handled on all providers | Medium | Treat any non-JSON `data:` value as stream end; log unknown sentinels at DEBUG. |
| Provider-specific fields break stream parser | Medium | Use lenient JSON with `omitempty`; unknown fields silently ignored by `json.Unmarshal`. |
| Ollama behavior regresses with factory refactor | High | Ensure factory default is Ollama; all pre-Phase-23 tests run in CI without config changes. |
| Rate limit retries exhaust context or budget | Medium | Rate-limit class gets 3 retries max with linear backoff; agent turn budget still applies. |
| Non-streaming path (prompt hooks) broken | Medium | Add explicit test for `stream: false` path in client_test.go; hooks integration test. |
| network-policy scanner flags cloud URLs in config examples | Low | Config example URLs appear only in comments and docs, not in Go source; scanner ignores those. |
| Slow SSE line scanner on large responses | Low | Use `bufio.Scanner` default buffer; increase if > 64KB lines seen in practice. |

## Exit Gate

Phase 23 is complete only when:

- All acceptance criteria above are met.
- `go test -race ./internal/llm/...` passes with zero races.
- `tools/check-allowed-deps.sh` passes — no new deps.
- `tools/check-network-policy.sh` passes.
- `go vet ./...` passes.
- Manual smoke test: default Ollama session works identically to Phase 22.
- The phase log records the implementation, test results, any deviations from this plan, and the manual smoke test result.

## Phase Log Template

When implementation finishes, append a Phase 23 entry to `docs/PHASE-LOG.md` with:

- objective;
- files created and updated;
- dependencies added and allowlist status;
- tests and checks run;
- manual smoke test result (Ollama unchanged; OpenAI provider status);
- design decisions (especially around API key handling, tool call accumulation);
- known constraints and deferred work (keyring, strict mode, provider hot-reload);
- exit gate status.
