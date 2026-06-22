# Ollama Cloud API Key Implementation Plan

Date: 2026-05-22

Status: Implemented and reviewed on 2026-05-22. Phase 25 Remote / Bridge Mode is now unblocked.

## Runtime Error Boundaries - 2026-05-24

This project now distinguishes three common failure classes during cloud usage:

- `llm stream watchdog timeout`: stream idle time exceeded the configured watchdog timeout.
- `resolve @docs\`` / similar mention errors: prompt mention parsing/path normalization issues.
- `ollama API error (status 403) ... requires a subscription`: Ollama account/model entitlement issue.

Only the first class is addressed by `llm_stream_idle_timeout` and
`cloud_llm_stream_idle_timeout` settings. Watchdog tuning does not change
subscription or permission errors from Ollama Cloud.

Before the final timeout, long-idle streams can emit an informational idle
warning. TUI shows a system item, server mode emits SSE event
`llm_idle_warning`, and `--print` writes the warning to stderr so stdout/JSON
output remains parseable. These warnings do not mask the final
`llm stream watchdog timeout`, and they do not change provider errors such as
Ollama Cloud subscription failures.

## Implementation Review - 2026-05-22

Implemented:

- Auth-capable Ollama client construction with bearer-token headers for direct Cloud API calls.
- Provider/origin types, model origin resolution, and runtime client routing between local Ollama and direct Ollama Cloud.
- Credential resolution from session memory, `OLLAMA_API_KEY`, and OS keychain, plus TUI masked prompting for interactive use.
- `/model`, `/models --cloud`, `/models --all`, and local-only `/pull` behavior.
- First-prompt TUI preflight so cloud-only configured models request credentials before prompt packing.
- Non-interactive `--print` and server-mode credential-required failures instead of blocking prompts.
- Config, bootstrap state, app state, docs, security redaction, dependency allowlist, and network policy updates.

Validation recorded:

- `go test ./...`
- `tools/check-allowed-deps.sh`
- `tools/check-network-policy.sh`

Review findings and accepted limitations:

- Direct Cloud catalog listing is intentionally unauthenticated because Ollama documents `GET https://ollama.com/api/tags` that way. If Ollama later requires auth for listing, attach an already-resolved key without prompting just to list models.
- Cloud fallback is intentionally conservative when the local Ollama catalog cannot be reached. The resolver does not silently choose a cloud model while local model availability is unknown, preserving local-first behavior.
- Real Ollama Cloud integration remains opt-in with `NANDOCODEGO_RUN_OLLAMA_CLOUD_INTEGRATION=1` and a real `OLLAMA_API_KEY`; normal automated tests do not send project data to cloud services.

## Goal

Add first-class support for Ollama Cloud API access while preserving the app's local-first default.

When the user selects or starts a run with a model that is not available from the configured local Ollama endpoint but is available through Ollama Cloud, the app must ask for an Ollama API key before any model request can send prompts, file context, tool output, memory snippets, or project metadata to `https://ollama.com`.

## Source Facts From Ollama Cloud Docs

Use the official docs as the protocol source of truth: https://docs.ollama.com/cloud

Facts to implement against:

- Ollama has local daemon cloud-offload models, for example `gpt-oss:120b-cloud`, which are accessed through the local daemon after `ollama signin` and `ollama pull`.
- Ollama also supports direct Cloud API access where `https://ollama.com` acts as a remote Ollama host.
- Direct Cloud API access uses an API key from `OLLAMA_API_KEY`.
- Direct Cloud API requests authenticate with `Authorization: Bearer <key>`.
- Direct cloud model listing is `GET https://ollama.com/api/tags`.
- Direct cloud chat is `POST https://ollama.com/api/chat`.
- Direct Cloud API examples use model names without the `-cloud` suffix, for example `gpt-oss:120b`.

## Product Decisions

These decisions close the open questions from the first plan and should be implemented unless product direction changes.

- `/models` remains local-only by default.
- `/models --cloud` lists direct Ollama Cloud API models.
- `/models --all` lists both local and direct cloud models.
- Cloud credentials are global per user through the OS keychain, not project-scoped.
- Direct cloud API model names are canonical without `-cloud`.
- If a user enters a `*-cloud` name that is not local, the resolver may offer the direct API equivalent by trimming only the final `-cloud` suffix. It must display the canonical cloud model name before switching and send the canonical name in API requests.
- If a `*-cloud` model appears in local `/api/tags`, treat it as local Ollama daemon cloud-offload. Do not ask for an app-level API key.
- Server mode only supports process-level or host-level credentials in the first version: `OLLAMA_API_KEY` or OS keychain. Do not accept per-session cloud keys through the HTTP API in this slice.
- The TUI footer/status should identify cloud mode, for example `Model: gpt-oss:120b | Provider: Ollama Cloud`.
- Do not reopen the removed Phase 23 OpenAI-compatible adapter. This work is Ollama Cloud only.

## User Experience

### Local Model Selection

User enters:

```text
/model qwen3.6:35b
```

Behavior:

1. Query configured local Ollama endpoint.
2. If the model exists locally, switch as today.
3. Do not contact Ollama Cloud.
4. Do not prompt for an API key.

Expected transcript:

```text
[Switched to local model: qwen3.6:35b]
```

### Cloud Model Selection

User enters:

```text
/model gpt-oss:120b
```

If the model is not local but is available in the cloud catalog, show a credential modal before switching.

Prompt copy:

```text
Ollama Cloud API key required

Model gpt-oss:120b is available through Ollama Cloud, not your local Ollama server.
Using it may send prompts, file context, tool output, memory snippets, and project metadata to Ollama Cloud.

API key: [masked input]

[Use once] [Save to keychain] [Cancel]
```

Behavior:

- `Use once`: keep the key in process memory only.
- `Save to keychain`: save the key through the OS keychain and also use it for the current process.
- `Cancel`: leave the current model, provider, and client unchanged.
- If `OLLAMA_API_KEY` is already present, skip the modal.
- If a saved key exists in the OS keychain, skip the modal.
- If the key is invalid, show a clear authentication error and leave the current model/provider unchanged.

Expected success transcript:

```text
[Switched to Ollama Cloud model: gpt-oss:120b]
```

Expected alias transcript:

```text
[Model gpt-oss:120b-cloud is local-cloud naming. Using Ollama Cloud API model: gpt-oss:120b]
[Switched to Ollama Cloud model: gpt-oss:120b]
```

### Prompt Submission With A Cloud Default Model

If the app starts with a configured or flag-provided model that is cloud-only, do not send a model request immediately at startup. On the first user prompt:

1. Resolve the active model before prompt packing.
2. If it is cloud-only and no credential is available, show the same credential modal.
3. If the user cancels, do not pack context and do not start an agent run.
4. If the user approves, then pack context and start the run.

This avoids expensive local context work and ensures cloud consent is explicit before the run begins.

### Model Listing

Default local-only behavior:

```text
/models
```

```text
Models:
  Local
    qwen3.6:35b (19 GB)
    llama3.2:3b (2.0 GB)
```

Cloud listing:

```text
/models --cloud
```

```text
Models:
  Ollama Cloud
    gpt-oss:120b
    gpt-oss:20b
```

Combined listing:

```text
/models --all
```

```text
Models:
  Local
    qwen3.6:35b (19 GB)

  Ollama Cloud
    gpt-oss:120b
    gpt-oss:20b
```

If cloud listing fails, local listing must still render for `/models --all`:

```text
Models:
  Local
    qwen3.6:35b (19 GB)

  Ollama Cloud
    [Unavailable: failed to list Ollama Cloud models: ...]
```

### Pull Behavior

`/pull` remains local-only.

- `/pull <model>` always targets the configured local Ollama daemon.
- If the active provider is Ollama Cloud API, explain that `/pull` still operates on local Ollama.
- Do not try to pull from `https://ollama.com`.

### Non-Interactive Behavior

`--print`:

- Resolve provider before prompt packing.
- If the model is local, proceed as today.
- If the model is cloud-only, require `OLLAMA_API_KEY` or a keychain credential.
- If no credential exists, exit with a clear error before prompt packing.
- Do not ask for interactive input.

Server mode:

- Resolve provider before running a message.
- If a cloud model is requested and no credential exists, return a structured credential-required error/event.
- Do not block waiting for terminal input.
- Do not accept per-session API keys in this slice.

## Non-Goals

- Do not implement generic OpenAI-compatible providers.
- Do not add Anthropic, Groq, vLLM, LM Studio, llama.cpp server, or arbitrary provider selection.
- Do not store API keys in `config.toml`, project files, memory files, prompt dumps, telemetry, logs, or task output.
- Do not make cloud model listing part of normal startup.
- Do not require an Ollama Cloud credential for local daemon cloud-offload models that already appear in local `/api/tags`.
- Do not implement a full account/login flow. The app only accepts an Ollama API key.
- Do not make cloud mode the default provider.

## Current Code Facts

The implementation should verify source before editing, but the current expected code shape is:

- `internal/llm/types.go` defines provider-neutral `llm.Client`.
- `internal/llm/ollama/ollama.go` implements local Ollama HTTP calls for `/api/chat`, `/api/tags`, `/api/show`, `/api/embeddings`, and `/api/pull`.
- `internal/cli/repl.go`, `internal/cli/print.go`, and `internal/server/server.go` construct `ollama.NewClient(...)` at startup.
- `internal/commands/registry.go` implements `/model`, `/models`, and `/pull`.
- `internal/tui/app.go` dispatches slash commands synchronously from the Bubble Tea update loop.
- `internal/tui/permission.go` is the existing pattern for a TUI prompt broker, but it is used from agent goroutines, not from synchronous slash command dispatch.
- `internal/config` has `default_model` and `ollama_base_url`.
- `internal/bootstrap` and `internal/state` mirror session and UI state.
- `internal/logging/redact.go` already redacts some secrets.
- `github.com/zalando/go-keyring` is already in `go.mod`.

The synchronous slash-command dispatch is important: do not block inside `handleModel` waiting for a TUI credential prompt. The TUI cloud-selection path must use Bubble Tea messages and commands.

## Target Architecture

Add four small layers:

1. Auth-capable Ollama HTTP client.
2. Ollama Cloud credential resolver.
3. Model origin resolver.
4. Switchable runtime client/router.

Keep the agent loop, tools, permissions, hooks, memory, tasks, and server session code provider-neutral by continuing to call `llm.Client`.

### Provider Model

Add provider/origin constants under `internal/llm` or a dedicated subpackage such as `internal/llm/provider`.

```go
type Provider string

const (
    ProviderOllamaLocal    Provider = "ollama_local"
    ProviderOllamaCloudAPI Provider = "ollama_cloud_api"
)

type ModelOrigin string

const (
    ModelOriginLocal          ModelOrigin = "local"
    ModelOriginOllamaCloudAPI ModelOrigin = "ollama_cloud_api"
)

type ResolvedModel struct {
    RequestedName string
    Model         string
    Origin        ModelOrigin
    Provider      Provider
    BaseURL       string
    AliasUsed     bool
    AliasReason   string
}
```

Recommended constants:

```go
const OllamaCloudBaseURL = "https://ollama.com"
```

### Auth-Capable Ollama Client

Modify `internal/llm/ollama/ollama.go`.

Keep the old constructor:

```go
func NewClient(baseURL string) *Client
```

Add:

```go
type Options struct {
    BaseURL    string
    APIKey     string
    HTTPClient *http.Client
}

func NewClientWithOptions(opts Options) *Client
```

Client fields:

```go
type Client struct {
    baseURL    string
    apiKey     string
    httpClient *http.Client
}
```

Add a helper and use it for every request:

```go
func (c *Client) applyHeaders(req *http.Request, hasJSONBody bool) {
    if hasJSONBody {
        req.Header.Set("Content-Type", "application/json")
    }
    if c.apiKey != "" {
        req.Header.Set("Authorization", "Bearer "+c.apiKey)
    }
}
```

Requirements:

- `NewClient(baseURL)` must behave exactly as before.
- `NewClientWithOptions(Options{BaseURL: "https://ollama.com", APIKey: key})` must authenticate all direct cloud calls.
- Do not include `apiKey` in errors, log fields, request dumps, prompt dumps, or tests that compare raw error text.
- Allow injecting `HTTPClient` for tests.
- Keep response streaming behavior unchanged.

Endpoint-specific behavior:

- `Chat`: works for local and cloud.
- `ListModels`: works for local and cloud.
- `ShowModel`: try the endpoint. If cloud returns unsupported or not found for metadata, return a classified error and let callers fall back to static limits.
- `Embed`: try the endpoint. Do not block the cloud chat feature on cloud embedding support.
- `PullModel`: remains local behavior. Direct cloud pull should not be used by product flows.

### Credential Resolver

Create `internal/credentials/ollama.go` or `internal/ollamacloud/credentials.go`.

Use an interface around keyring so tests do not touch the developer's real OS keychain.

```go
type KeySource string

const (
    KeySourceNone     KeySource = "none"
    KeySourceSession  KeySource = "session"
    KeySourceEnv      KeySource = "env"
    KeySourceKeychain KeySource = "keychain"
)

type Store interface {
    Get(service, account string) (string, error)
    Set(service, account, secret string) error
    Delete(service, account string) error
}

type Resolver struct {
    Store      Store
    Service    string
    Account    string
    SessionKey string
}

type ResolveOptions struct {
    AllowPrompt bool
    Prompt      PromptFunc
}

type PromptFunc func(context.Context, Prompt) (PromptResult, error)
```

Recommended service/account:

```text
service: nandocodego
account: ollama.com
```

Resolution order:

1. In-memory session key.
2. `OLLAMA_API_KEY`.
3. OS keychain.
4. TUI prompt, only when `AllowPrompt` is true and a prompt function is provided.

Prompt result:

```go
type PromptResult struct {
    Key       string
    Save     bool
    Canceled bool
}
```

Requirements:

- Empty keys are invalid.
- If prompt returns `Save=true`, write to keychain and cache in session memory.
- If keychain is unavailable, `Use once` must still work.
- If keychain save fails, show an error and do not silently pretend persistence succeeded.
- Provide a delete helper for future `/credentials clear` or tests.

### Model Origin Resolver

Create a service under `internal/llm/modelresolver` or `internal/ollamacloud`.

Inputs:

- Local client.
- Cloud catalog client.
- Cloud enabled flag.
- Optional short TTL cache for cloud catalog.

Algorithm for `Resolve(ctx, requested string)`:

1. Trim whitespace and reject empty names.
2. Call local `ListModels`.
3. If exact local name exists, return local result with `Model=requested`.
4. If cloud is disabled, return not found with local-only message.
5. Fetch cloud catalog from `https://ollama.com/api/tags`.
6. If exact cloud name exists, return cloud result with `Model=requested`.
7. If requested ends with the final suffix `-cloud`, trim that suffix and check the cloud catalog again.
8. If alias exists, return cloud result with `RequestedName=requested`, `Model=trimmed`, `AliasUsed=true`.
9. Otherwise return not found.

Catalog cache:

- Cache successful cloud catalog responses for 5 minutes.
- Cache errors for no more than 15 seconds, or do not cache errors.
- Provide a way for tests to bypass time by injecting a clock or by using very small TTLs.

`/models --cloud` can use the same cloud catalog service. It should not require a key because the Ollama docs show unauthenticated model listing. If the endpoint starts requiring auth, support attaching a key when one is already available, but do not prompt just to list models.

### Switchable Runtime Client

Add `internal/llm/router.go` or `internal/llm/runtime.go`.

Purpose: the REPL builds agent/tool/memory/hook/task objects once, but `/model` can switch between local and cloud later. A router lets those existing objects keep one stable `llm.Client`.

```go
type RuntimeClient struct {
    mu       sync.RWMutex
    current  llm.Client
    provider Provider
    baseURL  string
}

func NewRuntimeClient(initial llm.Client, provider Provider, baseURL string) *RuntimeClient
func (r *RuntimeClient) Switch(next llm.Client, provider Provider, baseURL string)
func (r *RuntimeClient) Snapshot() RuntimeSnapshot
```

Implement the `llm.Client` methods by taking the current client under lock and delegating.

Important behavior:

- Do not hold the lock while streaming the full response. Copy the current client pointer, release the lock, then call `Chat`.
- In-flight runs keep using the client that was current when the request began.
- Switching during an active run should be rejected or deferred at the product layer. For v1, reject `/model` changes while `state.App.ActiveRun` is true.
- Observability should wrap the router once, not each provider instance repeatedly.

### Model Runtime Service

Create one small orchestration service used by TUI, CLI print, and server startup. Suggested package: `internal/llm/modelruntime`.

Responsibilities:

- Own local client, cloud catalog client, runtime router, model resolver, and credential resolver.
- Resolve model origin.
- Acquire credentials when needed.
- Validate cloud credentials without sending project data.
- Switch runtime client.
- Update app/bootstrap state through explicit callbacks or returned values.

Suggested API:

```go
type Service struct {
    LocalClient llm.Client
    Runtime    *llm.RuntimeClient
    Resolver   *Resolver
    Creds      *credentials.Resolver
}

type SwitchOptions struct {
    RequestedModel string
    AllowPrompt    bool
    Prompt         credentials.PromptFunc
}

type SwitchResult struct {
    Resolved       ResolvedModel
    CredentialSrc  credentials.KeySource
    UsedCloud      bool
    Message        string
}

func (s *Service) Resolve(ctx context.Context, requested string) (ResolvedModel, error)
func (s *Service) Switch(ctx context.Context, opts SwitchOptions) (SwitchResult, error)
func (s *Service) ListLocal(ctx context.Context) ([]llm.ModelInfo, error)
func (s *Service) ListCloud(ctx context.Context) ([]llm.ModelInfo, error)
func (s *Service) PullLocal(ctx context.Context, name string, progress chan<- llm.PullProgress) error
```

Cloud credential validation:

- Validate with `GET https://ollama.com/api/tags` using the `Authorization` header if the endpoint honors it, or with a minimal non-project metadata request if listing remains public.
- Do not validate with a real chat prompt containing project/user data.
- If validation cannot prove the key is bad but chat may still work, allow switch and surface chat errors normally. Do not send project data during validation.

## State And Config Changes

### Config

Modify `internal/config/config.go`, `internal/config/defaults.go`, loader tests, and default TOML.

Add:

```go
OllamaCloudEnabled bool `koanf:"ollama_cloud_enabled"`
```

Default:

```go
OllamaCloudEnabled: true
```

Default TOML:

```toml
# ollama_cloud_enabled = true
```

Environment:

```text
OLLAMA_API_KEY=...
NANDOCODEGO_OLLAMA_CLOUD=0|1
```

Rules:

- `OLLAMA_API_KEY` is the credential environment variable because Ollama documents it.
- `NANDOCODEGO_OLLAMA_CLOUD=0` disables cloud catalog lookup and direct cloud switching even if config enables it.
- Do not add `api_key` to config.
- If config contains `api_key` or `ollama_api_key`, warn that plaintext key config is unsupported and ignored.

### Bootstrap

Modify `internal/bootstrap/state.go`.

Add to `Initial` and `Snapshot`:

```go
LLMProvider         string
LLMBaseURL          string
OllamaCloudEnabled bool
```

Defaults:

```go
LLMProvider: "ollama_local"
LLMBaseURL:  "http://localhost:11434"
OllamaCloudEnabled: true
```

Keep existing `OllamaBaseURL` for local daemon configuration.

### App State

Modify `internal/state/app.go`, clone tests, and onchange tests.

Add:

```go
LLMProvider string
LLMBaseURL  string
```

Optional UI state:

```go
CredentialPrompt *CredentialPrompt
```

If the credential prompt is kept purely inside the TUI model instead of `state.App`, tests still need to verify prompt state and cancellation behavior. Use state only if it helps render/status consistency.

`state.OnChange` should mirror `ActiveModel`, `LLMProvider`, and `LLMBaseURL` into bootstrap.

## TUI Implementation Details

Do not block the Bubble Tea update loop waiting for credential input.

### Message Types

Add TUI messages, likely in `internal/tui/messages.go`:

```go
type modelSwitchStartedMsg struct {
    Requested string
}

type modelSwitchNeedsCredentialMsg struct {
    Resolved llm.ResolvedModel
}

type modelSwitchCompletedMsg struct {
    Result modelruntime.SwitchResult
}

type modelSwitchFailedMsg struct {
    Requested string
    Err       error
}

type cloudCredentialPromptMsg struct {
    Request credentialRequest
}

type cloudCredentialResolvedMsg struct {
    ID     string
    Key    string
    Save   bool
    Cancel bool
}
```

Names can differ, but the flow must be asynchronous.

### Prompt UI

Use a masked input. `github.com/charmbracelet/bubbles/textinput` is already available through the existing `bubbles` dependency.

Expected key behavior:

- Characters type into the masked key input.
- `Tab` cycles buttons: `Use once`, `Save to keychain`, `Cancel`.
- `Enter` activates focused button.
- `Esc` cancels.
- Normal prompt textarea input is disabled while the credential modal is active.
- The key must not appear in transcript, prompt dump, trace, logs, test snapshots, or panic output.

### `/model` Flow In TUI

Handle `/model` specially in `internal/tui/app.go` before the synchronous command registry dispatch, similar to existing special handling for `/compact`, `/analyze-project`, `/bg`, and `/btw`.

Flow:

1. User submits `/model <name>`.
2. If no name, keep existing current-model display path.
3. If active run is true, show `[Error: cannot switch models while a run is active]`.
4. Return a `tea.Cmd` that resolves local/cloud model origin.
5. If local, switch through the model runtime service/router and send `modelSwitchCompletedMsg`.
6. If cloud and key exists from env/keychain/session, switch and send `modelSwitchCompletedMsg`.
7. If cloud and no key exists, send a credential prompt message.
8. After user resolves the credential modal, run another `tea.Cmd` to validate/switch.
9. On completion, update `state.App.ActiveModel`, `LLMProvider`, `LLMBaseURL`, model limits, and transcript.
10. On failure/cancel, leave previous state unchanged.

Do not call a blocking `PromptFunc` from inside `commands.handleModel` while in TUI mode.

### First Prompt With Cloud Active Model

Before current prompt-packing code in `internal/tui/app.go`, call an async "ensure active model provider" path when the provider is unknown/local but active model is not local.

Implementation approach:

- When normal prompt submission starts, check whether the active model is resolved and current provider can serve it.
- If not resolved, store the pending user input in TUI state and start the same model-resolution flow.
- After successful switch, resume prompt submission with the saved input.
- If canceled or failed, discard the pending run and leave the input available for editing.

This should be implemented carefully to avoid duplicate transcript user messages.

## Command Registry Changes

`internal/commands/registry.go` can still own non-interactive command behavior and shared rendering helpers, but it should not own TUI credential prompting.

Add to `HandlerContext`:

```go
ModelRuntime *modelruntime.Service
```

Possible optional fields:

```go
CredentialPrompt credentials.PromptFunc
Interactive bool
```

Rules:

- In TUI, `/model` can bypass registry for switch logic.
- In tests and non-TUI command usage, `handleModel` should use `ModelRuntime.Switch` with `AllowPrompt=false`.
- `/models` should parse `--cloud` and `--all`.
- `/pull` should call `ModelRuntime.PullLocal`, not the current runtime client.
- Existing command tests should keep working for local-only fake clients or be updated with fake model runtime services.

## CLI Startup Wiring

### REPL

Modify `internal/cli/repl.go`.

Build:

1. Local Ollama client from `snap.OllamaBaseURL`.
2. Runtime router initialized with the local client.
3. Observability wrapper around the runtime router.
4. Model runtime service with local client, runtime router, cloud resolver, credentials resolver, and cloud-enabled flag.

Pass the observed runtime client to:

- Agent runner.
- Task tools.
- Agent tool.
- Memory runner.
- Hook dispatcher.
- Self-info tool.

Pass the model runtime service to:

- TUI model constructor or setter.
- Command handler context.

Startup model limits:

- If the default model is local, keep current `ShowModel` limits behavior.
- If `ShowModel` fails because the model is not local, do not block REPL startup.
- Use static `llm.ModelCapabilities` and default context/output limits until the model is resolved.

### Print

Modify `internal/cli/print.go`.

Before `buildPrintInput(...)`:

1. Build model runtime service.
2. Resolve `initial.DefaultModel`.
3. If cloud-only, resolve credentials with `AllowPrompt=false`.
4. If missing, return a user-facing error before prompt packing.
5. Switch runtime to cloud if needed.
6. Then run prompt packing and agent execution.

### Server

Modify `internal/server/server.go` and handlers/session code.

Startup:

- Build local client, runtime router, and model runtime service.
- Use process/keychain credentials only.

Message handling:

- Before building prompt/context for a session message, ensure the session model can be served.
- If cloud credential is missing, emit or return:

```json
{
  "error": "requires_credential",
  "provider": "ollama_cloud_api",
  "credential": "OLLAMA_API_KEY"
}
```

Do not include an API key field in request/response schemas in this first version.

## Error Handling

Add classified errors where useful. Keep user messages short.

Recommended classes:

- `ErrModelNotFound`
- `ErrCloudDisabled`
- `ErrCredentialRequired`
- `ErrCredentialCanceled`
- `ErrUnauthorized`
- `ErrForbidden`
- `ErrRateLimited`
- `ErrProviderUnavailable`

HTTP mapping for Ollama Cloud:

- `401`: invalid or missing Ollama API key.
- `403`: key is valid but not allowed to access the model.
- `404`: model not found for selected provider.
- `429`: rate limited by Ollama Cloud.
- `5xx`: Ollama Cloud service error.

Do not include raw response bodies at INFO level. If DEBUG logs include response snippets, pass them through `logging.Redact`.

## Security Requirements

- Never store keys in TOML, memory files, task output, prompt dumps, trace output, telemetry, transcripts, or logs.
- Mask credential input in the TUI.
- Redact:
  - `OLLAMA_API_KEY=...`
  - `Authorization: Bearer ...`
  - `api_key`
  - `ollama_api_key`
  - bearer-token-like substrings.
- Prompt dumps must not capture credential modal state.
- Telemetry labels may include provider name and model name, but not credentials.
- Do not send validation chat prompts that contain user/project data.
- Do not query Ollama Cloud at process startup.
- Do not make cloud selection implicit when a local exact match exists.

## File-Level Implementation Checklist

Expected new files:

- `internal/credentials/ollama.go`
- `internal/credentials/ollama_test.go`
- `internal/llm/router.go`
- `internal/llm/router_test.go`
- `internal/llm/modelresolver/resolver.go` or equivalent
- `internal/llm/modelresolver/resolver_test.go`
- `internal/llm/modelruntime/service.go` or equivalent
- `internal/llm/modelruntime/service_test.go`
- `docs/manual-tests/OLLAMA-CLOUD-API-KEY.md`

Expected modified files:

- `internal/llm/ollama/ollama.go`
- `internal/llm/ollama/ollama_test.go`
- `internal/llm/types.go` or new provider type file
- `internal/logging/redact.go`
- `internal/logging/redact_test.go`
- `internal/config/config.go`
- `internal/config/defaults.go`
- `internal/config/loader.go`
- `internal/config/loader_test.go`
- `internal/bootstrap/state.go`
- `internal/bootstrap/state_test.go`
- `internal/state/app.go`
- `internal/state/app_test.go`
- `internal/state/onchange.go`
- `internal/state/onchange_test.go`
- `internal/commands/registry.go`
- `internal/commands/registry_test.go`
- `internal/tui/app.go`
- `internal/tui/messages.go`
- `internal/tui/styles.go` if the modal needs styles
- `internal/tui/app_test.go`
- `internal/tui/snapshot_status_test.go` if provider appears in the footer
- `internal/cli/repl.go`
- `internal/cli/print.go`
- `internal/cli/print_test.go`
- `internal/server/server.go`
- `internal/server/session.go`
- `internal/server/handler.go`
- `internal/server/session_test.go`
- `USER_MANUAL.md`
- `README.md`
- `docs/PHASE-LOG.md`
- `docs/PROJECT-STATUS-AND-ONBOARDING.md`
- `docs/NEXT-PHASES-IMPLEMENTATION-PLAN.md`

The exact package names can change if the implementation finds a cleaner local pattern, but the responsibilities above should remain separated.

## Test Plan

### Unit Tests

Ollama client:

- `NewClient` sends no `Authorization` header.
- `NewClientWithOptions` sends `Authorization: Bearer <key>` on chat, list, show, embed, and pull requests.
- Error messages never include the configured API key.
- Streaming chat still decodes events.

Credentials:

- Env var key resolves before keychain.
- Session key resolves before env var.
- Keychain resolves when env/session are empty.
- Prompt is called only when allowed.
- Prompt cancel returns `ErrCredentialCanceled`.
- `Use once` does not write keychain.
- `Save to keychain` writes keychain and caches session key.
- Keychain unavailable still allows `Use once`.

Model resolver:

- Exact local match wins over cloud.
- Cloud exact match resolves to `ollama_cloud_api`.
- `gpt-oss:120b-cloud` local exact match remains local.
- `gpt-oss:120b-cloud` absent locally resolves to cloud alias `gpt-oss:120b` if the cloud catalog contains it.
- Cloud disabled returns local-only not found.
- Cloud catalog failures do not hide local matches.
- Catalog cache avoids repeated network calls.

Router:

- Delegates all methods to current client.
- Switch changes subsequent calls.
- In-flight chat uses the old client after switch.
- Snapshot returns provider/base URL.

Commands:

- `/model` local switch works without credential prompt.
- `/model` cloud without non-interactive credential returns credential-required.
- `/models` lists local only by default.
- `/models --cloud` lists cloud only.
- `/models --all` groups both and tolerates cloud failure.
- `/pull` always uses local client.

TUI:

- Cloud `/model` starts async resolution and does not block update loop.
- Credential modal masks input.
- `Esc` cancels and leaves model/provider unchanged.
- `Use once` switches provider without keychain write.
- `Save to keychain` switches provider and writes keychain.
- Invalid key leaves model/provider unchanged.
- Active-run `/model` is rejected.
- First prompt with cloud-only active model prompts before context packing.
- Canceling that prompt does not append duplicate user messages.
- Footer/status shows cloud provider after switch.

Print:

- Cloud model without env/keychain fails before prompt packing.
- Cloud model with env key switches and runs.
- Local model remains unchanged.

Server:

- Cloud model without process/keychain credential emits/returns `requires_credential`.
- Local model behavior remains unchanged.

Security:

- Redaction covers `OLLAMA_API_KEY`, `Authorization`, `Bearer`, `api_key`, and `ollama_api_key`.
- Prompt dump and trace tests do not contain credential text.

### Integration Tests

Gate real cloud tests behind both:

```text
NANDOCODEGO_RUN_OLLAMA_CLOUD_INTEGRATION=1
OLLAMA_API_KEY=...
```

Integration checks:

- `GET https://ollama.com/api/tags` parses at least one model.
- `POST https://ollama.com/api/chat` streams a minimal non-project prompt.
- No project files are read or sent in the integration test.

### Recommended Verification Commands

Run targeted tests during implementation:

```bash
go test ./internal/llm/... ./internal/credentials/... ./internal/config/... ./internal/bootstrap/... ./internal/state/...
go test ./internal/commands/... ./internal/tui/... ./internal/cli/... ./internal/server/...
go test ./...
tools/check-allowed-deps.sh
tools/check-network-policy.sh
```

Cloud integration is optional and must be explicit:

```bash
NANDOCODEGO_RUN_OLLAMA_CLOUD_INTEGRATION=1 OLLAMA_API_KEY=... go test ./internal/llm/ollama -run OllamaCloud
```

## Manual Test Script

Create `docs/manual-tests/OLLAMA-CLOUD-API-KEY.md` with this checklist:

1. Start with no `OLLAMA_API_KEY` and no saved key.
2. Run local model selection and confirm no prompt.
3. Run `/models` and confirm local-only output.
4. Run `/models --cloud` and confirm cloud list or clear unavailable message.
5. Run `/model gpt-oss:120b` and confirm credential modal.
6. Press `Esc` and confirm current local model remains active.
7. Run `/model gpt-oss:120b` again, choose `Use once`, and confirm provider switches.
8. Send a harmless prompt and confirm streaming response.
9. Restart the app and confirm `Use once` did not persist.
10. Run `/model gpt-oss:120b`, choose `Save to keychain`, restart, and confirm no prompt.
11. Run with `NANDOCODEGO_OLLAMA_CLOUD=0` and confirm cloud lookup is disabled.
12. Run `--print` with cloud model and no key, confirm it fails before prompt packing.
13. Run server mode with cloud model and no key, confirm structured `requires_credential`.

## Rollout Slices

Implement in this order:

1. Auth-capable Ollama client and redaction tests.
2. Credential resolver with env/keychain/session behavior.
3. Cloud catalog and model-origin resolver.
4. Runtime client router.
5. Model runtime service that ties resolver, credentials, and router together.
6. Config/bootstrap/state provider fields.
7. Non-interactive `/model`, `/models`, `/pull`, and `--print` support.
8. TUI async model switch and credential modal.
9. First-prompt active-model resolution in TUI.
10. Server credential-required behavior.
11. User/manual docs and phase log.
12. Full test pass and optional cloud integration evidence.

Each slice should leave local-only behavior working.

## Acceptance Criteria

- Local model selection behaves as before.
- Local daemon cloud-offload models that appear in local `/api/tags` do not ask for an app-level key.
- Cloud-only model selection asks for an API key before switching.
- `OLLAMA_API_KEY` skips the interactive prompt.
- Saved keychain credentials skip the interactive prompt.
- Canceling the credential prompt keeps the previous provider/model active.
- Invalid credentials do not switch provider/model.
- Successful cloud switch sends chat requests to `https://ollama.com/api/chat` with `Authorization: Bearer <key>`.
- No project data is sent during credential validation.
- First prompt with a cloud-only default model prompts before context packing and before agent run.
- `--print` and server mode never block for interactive input.
- `/models`, `/models --cloud`, and `/models --all` behave as specified.
- `/pull` remains local-only.
- TUI status shows when the active provider is Ollama Cloud.
- API keys are never logged, stored in config, shown in transcripts, included in prompt dumps, emitted in telemetry, or committed to docs/tests.
- `go test ./...`, dependency allowlist, and network policy checks pass.

## Documentation Updates Required During Implementation

When implementation lands, update:

- `README.md`: describe optional Ollama Cloud API support and keep local-first positioning.
- `USER_MANUAL.md`: add setup, `/model`, `/models --cloud`, credential storage, privacy, and non-interactive behavior.
- `README.Docker.md` and `DOCKER_WEB_GUIDE.md` if server/container flows gain `OLLAMA_API_KEY` behavior.
- `SECURITY.md`: add cloud credential handling, keychain storage, and cloud data-flow warning.
- `docs/PHASE-LOG.md`: record files changed, tests run, manual checks, and remaining limitations.
- `docs/PROJECT-STATUS-AND-ONBOARDING.md`: mark Ollama Cloud API key support complete when accepted and restore Phase 25 as next.
- `docs/NEXT-PHASES-IMPLEMENTATION-PLAN.md`: remove the "next workstream" insertion after completion and resume Phase 25 ordering.
