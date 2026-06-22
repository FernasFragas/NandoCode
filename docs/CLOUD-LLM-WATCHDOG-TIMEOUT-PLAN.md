# Cloud LLM Watchdog Timeout Implementation Plan

## Status

Implemented and reviewed on 2026-05-24.

Tasks 0-9 are complete. The "Detailed Agent Tasks" section remains as the
implementation record; the review below is authoritative for what exists now
and what was intentionally left out.

## Implementation Review - 2026-05-24

Implemented:

- Config keys `llm_stream_idle_timeout` and `cloud_llm_stream_idle_timeout`, with defaults of `90s` and `5m`.
- CLI flags `--llm-stream-idle-timeout` and `--cloud-llm-stream-idle-timeout` on root REPL/print mode and the `server` subcommand.
- Provider-aware watchdog selection through `agent.Input.LLMProvider`, `agent.Config.CloudWatchdog`, and `Config.watchdogForProvider`.
- Loaded timeout propagation through REPL, print mode, server mode, TUI runs, delegated agents, task agents, and sub-agents.
- Server `--write-timeout` default of `0`, so long-lived SSE responses are not cut off before the configured LLM stream watchdog.
- Mention normalization for trailing Markdown backticks, so `@docs\`` resolves as `@docs`.
- User-visible idle warning events:
  - agent event: `LLMIdleWarning{Provider, Timeout}`;
  - TUI system item: `[Still waiting for model stream (...); idle ...]`;
  - server SSE event: `llm_idle_warning` with `provider`, `timeout_ms`, and `timeout_str`;
  - print-mode stderr warning, preserving stdout and JSON output.
- Documentation updates in `USER_MANUAL.md`, `DOCKER_WEB_GUIDE.md`, and `docs/OLLAMA-CLOUD-API-KEY-PLAN.md`.

Not implemented:

- No separate config keys or CLI flags for `IdleWarningTimeout`. The warning threshold remains internal/default-derived; users configure the final idle timeout only.
- No per-session HTTP API override for watchdog settings. Server mode reads config and process CLI flags at startup.
- No per-task model resolver/provider switch when a task or delegated agent overrides the model. Those paths pass the active provider metadata; resolving a different provider for an overridden task model is future work.
- No change to Ollama Cloud subscription or entitlement handling. `403 requires a subscription` remains a provider access error and is not affected by watchdog settings.

Validation recorded during implementation:

```bash
go test ./...
```

## Problem

Cloud models can legitimately take longer than local models to emit the next streamed chunk, especially when the prompt includes directory context or asks for large document generation. Before this work the agent used a fixed stream watchdog:

- `llm.DefaultWatchdogConfig()` uses a 90 second idle timeout and 45 second warning threshold.
- REPL and print mode both hard-code that default into `agent.Config`.
- The server path starts from `agent.DefaultConfig()`, which also uses the same default watchdog.

When a cloud response is idle for more than 90 seconds, the watchdog emits `DoneReason: "watchdog_timeout"`. The agent maps that to the unrecoverable detail:

```text
llm stream watchdog timeout
```

That was useful for dead streams, but too aggressive for cloud providers.

There is also a separate mention parsing issue:

```text
resolve @docs`: lstat .../docs`: no such file or directory
```

That is not caused by the watchdog. It happens because the prompt parser treats the trailing Markdown backtick as part of the path. The full fix for the previously observed errors therefore has two tracks:

1. Add configurable local/cloud LLM stream watchdog timeouts.
2. Harden mention normalization so `@docs\`` resolves as `@docs`.

This plan does not fix Ollama Cloud `403 requires a subscription` errors. Those are model/account entitlement failures and must remain explicit provider errors.

## Goals

- Keep current local-model behavior by default.
- Use a longer default stream idle timeout for Ollama Cloud API models.
- Let users override both local and cloud watchdog idle timeout values through config and CLI flags.
- Make print, TUI, server, task, and sub-agent paths use the same effective watchdog policy.
- Keep watchdog failures visible as unrecoverable when the stream is truly idle past the configured limit.
- Fix `@docs\`` path parsing so Markdown punctuation does not break folder mentions.

## Reviewed Approach Notes

- Do not infer provider from model names inside the agent. The model resolver can turn `kimi-k2.6:cloud` into canonical model `kimi-k2.6`, so model-name heuristics are not reliable.
- Do not infer provider from `llm.Client` inside the agent. The runtime client has provider metadata, but it is wrapped by observability before it reaches the agent, and the `llm.Client` interface intentionally has no provider method.
- Pass provider metadata through `agent.Input`. `state.App`, `bootstrap.Initial`, and `bootstrap.Snapshot` already carry `LLMProvider`; `agent.Input` is the missing boundary.
- Do not add watchdog timeout fields to `state.App` for the first implementation. The agent config already carries runtime behavior, and `state.App` only needs the active provider for per-run selection.
- Expose only idle timeout settings as config/CLI. `WatchdogConfig.IdleWarningTimeout` is now wired to user-visible idle warnings, but it remains internal/default-derived rather than separately configurable.
- Server mode needs explicit write-timeout handling. A 5 minute cloud watchdog is ineffective for SSE/web clients if `http.Server.WriteTimeout` is shorter than the configured cloud watchdog.

## Proposed User-Facing Settings

Add these config keys:

```toml
# Local/default stream watchdog behavior.
llm_stream_idle_timeout = "90s"

# Used when the active provider is Ollama Cloud API.
cloud_llm_stream_idle_timeout = "5m"
```

Add these CLI flags to root REPL/print mode and the `server` subcommand:

```bash
--llm-stream-idle-timeout 90s
--cloud-llm-stream-idle-timeout 5m
```

No warning-timeout config keys or flags are exposed. Idle warnings are emitted
from the watchdog's internal/default warning threshold; users tune the final
idle timeout with the two idle-timeout settings above.

Precedence should match the existing config model:

```text
defaults < user config < project config < CLI flags
```

Recommended defaults:

- Local/default idle timeout: `90s`, preserving current behavior.
- Cloud idle timeout: `5m`, long enough for slow cloud first-token gaps without hiding truly stuck streams forever.

## Implementation Steps

### 1. Extend Config

Update `internal/config/config.go`:

- Add `LLMStreamIdleTimeout time.Duration` with `koanf:"llm_stream_idle_timeout"`.
- Add `CloudLLMStreamIdleTimeout time.Duration` with `koanf:"cloud_llm_stream_idle_timeout"`.
- Add matching `ConfigSources` fields.
- Add matching optional fields to `FlagOverrides` for the CLI timeout overrides.

Update `internal/config/defaults.go`:

- Default local idle timeout to `90s`.
- Default cloud idle timeout to `5m`.
- Include the new keys in `DefaultConfigTOML()`.

Update `internal/config/loader.go`:

- Add the new keys to the default `confmap`.
- Mark sources in `markSourcesFromKoanf`.
- Add keys to the known-key allowlist.
- Apply CLI overrides after config files load.
- Validate durations:
  - idle timeout below `1s` becomes `1s`;
  - cloud timeout below `1s` becomes the local idle timeout or `1s`.
  - invalid duration syntax should return a config load error, matching the current `time.Duration` config behavior.

### 2. Carry Settings Through Bootstrap

Update `internal/bootstrap/state.go`:

- Add `LLMStreamIdleTimeout` and `CloudLLMStreamIdleTimeout` to `Initial`.
- Add `LLMStreamIdleTimeout` and `CloudLLMStreamIdleTimeout` to `Snapshot`.
- Populate defaults in `DefaultInitial`.
- Copy them in `New`.

Do not add these duration settings to `state.App` unless a UI needs to display
or mutate them at runtime. The current fix only needs `state.App.LLMProvider`,
which already exists.

### 3. Build Watchdog Configs Once

Add a helper, preferably in `internal/llm/watchdog.go` or `internal/agent`:

```go
func buildWatchdog(idle time.Duration, fallback llm.WatchdogConfig) llm.WatchdogConfig {
    wd := fallback
    wd.IdleTimeout = idle
    if wd.IdleWarningTimeout <= 0 || wd.IdleWarningTimeout >= idle {
        wd.IdleWarningTimeout = idle / 2
    }
    return wd
}
```

Then set:

- `agentCfg.Watchdog` from local/default settings.
- `agentCfg.CloudWatchdog` from cloud settings.

Add `CloudWatchdog llm.WatchdogConfig` to `agent.Config` in `internal/agent/input.go`. Keep `Watchdog` as the local/default field to minimize churn.

### 4. Select the Watchdog Per Run

Add `LLMProvider string` to `agent.Input`.

In `internal/agent/agent.go`, pass `in.LLMProvider` into `executeOneTurn`.

Prefer adding an internal helper on agent config:

```go
func (c Config) watchdogForProvider(provider string) llm.WatchdogConfig {
    if provider == string(llm.ProviderOllamaCloudAPI) && c.CloudWatchdog.IdleTimeout > 0 {
        return c.CloudWatchdog
    }
    if c.Watchdog.IdleTimeout > 0 {
        return c.Watchdog
    }
    return llm.DefaultWatchdogConfig()
}
```

In `internal/agent/stream.go`, choose the effective watchdog:

```go
watchdog := a.config.watchdogForProvider(inProvider)
```

Then call:

```go
watchedStream, cancelWatchdog := llm.WatchStream(ctx, stream, watchdog)
```

This avoids provider inference from model names, which is important because `kimi-k2.6:cloud` can be resolved to the canonical model name `kimi-k2.6` after switching.

### 5. Propagate Provider at Every Entry Point

Update TUI prompt submission in `internal/tui/app.go`:

- Main prompt `agent.Input`: `LLMProvider: appState.LLMProvider`.
- Resume prompt path: `LLMProvider: appState.LLMProvider`.
- BTW prompt path: `LLMProvider: appState.LLMProvider`.

Update print mode in `internal/cli/print.go`:

- After `modelRuntimeSvc.Switch`, use `switchRes.Resolved.Provider`.
- Pass the provider into `buildPrintInput`.
- Set `agent.Input.LLMProvider`.

Update server mode:

- `internal/server/handler.go` already calls `sess.applyModelSwitch(switchRes)` before `StartRun`.
- `internal/server/session.go` should set `agent.Input.LLMProvider = app.LLMProvider`.

Update task/sub-agent paths:

- `internal/agent/subagent.go` should copy `parentInput.LLMProvider` into `childInput`.
- `internal/tools/agenttool/agenttool.go` should accept or derive the active provider, not just active model, so delegated agents use the same cloud watchdog.
- `internal/tasks/supervisor.go` should receive provider metadata in the run function or infer it from the parent input/session state.

If task/provider plumbing becomes too broad, phase it:

1. Main TUI, print, and server runs first.
2. Sub-agent/task propagation second, before claiming full completion.

### 6. Add CLI Flags

Update `internal/cli/root.go`:

- Add `llmStreamIdleTimeout string`.
- Add `cloudLLMStreamIdleTimeout string`.
- Register:

```go
rootCmd.Flags().StringVar(&opts.llmStreamIdleTimeout, "llm-stream-idle-timeout", "", "LLM stream idle watchdog timeout")
rootCmd.Flags().StringVar(&opts.cloudLLMStreamIdleTimeout, "cloud-llm-stream-idle-timeout", "", "Ollama Cloud stream idle watchdog timeout")
```

Pass those values into both REPL and print options, then into `config.FlagOverrides`.

Update `internal/cli/server.go`:

- Add matching server options and flags.
- Pass them into `server.Config`.
- Include them in the `config.Load` flag overrides inside `server.New`.

Use empty string to mean "not overridden" so config source tracking remains accurate.

### 7. Align Server Write Timeout

The server currently defaults `WriteTimeout` to `120s`. If `cloud_llm_stream_idle_timeout` is `5m`, an SSE client may still be disconnected before the agent watchdog fires.

Update server behavior in one of these ways:

- Preferred: set server `WriteTimeout` default to `0` for SSE-friendly long streaming responses.
- Alternative: if `WriteTimeout > 0 && WriteTimeout < effectiveCloudIdleTimeout`, emit a startup warning telling the user to set `--write-timeout 0` or a larger value.

The preferred option matches long-lived streaming semantics better.

### 8. Harden Mention Normalization

Update `internal/mentions/expand.go`:

- Extend `NormalizeMentionPath` trailing trim characters to include backticks.

Expected behavior:

```go
NormalizeMentionPath("docs`") == "docs"
```

Add a test in `internal/mentions/expand_test.go`.

This fixes the `resolve @docs\`` error. It is independent of the cloud watchdog work.

### 9. Documentation Updates

Update `USER_MANUAL.md`:

- Add config table rows for the new timeout settings.
- Add CLI examples:

```bash
nandocodego --model kimi-k2.6:cloud --cloud-llm-stream-idle-timeout 5m
nandocodego --print "Generate a docs plan in @docs" --model kimi-k2.6:cloud --cloud-llm-stream-idle-timeout 5m
nandocodego server --model kimi-k2.6:cloud --cloud-llm-stream-idle-timeout 5m --write-timeout 0
```

Update `docs/OLLAMA-CLOUD-API-KEY-PLAN.md` or create a short cloud runtime note explaining:

- A watchdog timeout means the stream was idle too long.
- A `403 requires a subscription` response is an Ollama account/model entitlement issue and is not changed by these timeout settings.
- `@docs\`` mention errors are parser/path issues, not model errors.

## Detailed Agent Tasks

These tasks are ordered to minimize merge conflicts and keep the system
buildable after each task. Agents should work in order unless explicitly
coordinated otherwise.

### Task 0: Baseline and Worktree Safety

Owner: any agent before starting implementation.

Files to inspect:

- `git status --short`
- `docs/CLOUD-LLM-WATCHDOG-TIMEOUT-PLAN.md`
- `internal/llm/watchdog.go`
- `internal/agent/input.go`
- `internal/agent/stream.go`
- `internal/config/config.go`
- `internal/config/loader.go`

Steps:

1. Check the worktree before editing. This repository currently has many unrelated modified and added files; do not reset or revert them.
2. Confirm whether the `@docs\`` mention normalization fix is already present:
   - `internal/mentions/expand.go` should trim a trailing backtick in `NormalizeMentionPath`.
   - `internal/mentions/expand_test.go` should include a `NormalizeMentionPath("docs`") == "docs"` assertion.
3. If the mention fix is already present, do not rewrite it. If it is missing, implement Task 1.

Acceptance:

- The agent can state which relevant files were already dirty before its own edits.
- No unrelated modified files are reverted.

### Task 1: Harden Mention Normalization

Owner: mention/context agent.

Purpose:

Fix the path parsing error:

```text
resolve @docs`: lstat .../docs`: no such file or directory
```

Files:

- `internal/mentions/expand.go`
- `internal/mentions/expand_test.go`

Implementation:

1. In `NormalizeMentionPath`, extend the trailing trim character set to include the Markdown backtick.
2. Preserve existing behavior for punctuation, quotes, root references, slash normalization, and trailing slash trimming.
3. Add or keep this focused test:

```go
if got := NormalizeMentionPath("docs`"); got != "docs" {
    t.Fatalf("expected docs without trailing markdown backtick, got %q", got)
}
```

Tests:

```bash
go test ./internal/mentions
```

Acceptance:

- `@docs\`` resolves as `docs`.
- Existing mention tests still pass.

### Task 2: Add Config Fields and Loader Support

Owner: config agent.

Purpose:

Expose two durable settings:

- `llm_stream_idle_timeout`
- `cloud_llm_stream_idle_timeout`

Files:

- `internal/config/config.go`
- `internal/config/defaults.go`
- `internal/config/loader.go`
- `internal/config/loader_test.go`
- `internal/config/defaults_test.go` if it asserts generated config keys

Implementation:

1. In `Config`, add:

```go
LLMStreamIdleTimeout      time.Duration `koanf:"llm_stream_idle_timeout"`
CloudLLMStreamIdleTimeout time.Duration `koanf:"cloud_llm_stream_idle_timeout"`
```

2. In `ConfigSources`, add:

```go
LLMStreamIdleTimeout      string
CloudLLMStreamIdleTimeout string
```

3. In `FlagOverrides`, add string-pointer overrides:

```go
LLMStreamIdleTimeout      *string
CloudLLMStreamIdleTimeout *string
```

Use string pointers because root/server flags are parsed as optional strings and
`koanf` already unmarshals duration strings into `time.Duration`.

4. In `DefaultConfig()`, set:

```go
LLMStreamIdleTimeout:      90 * time.Second,
CloudLLMStreamIdleTimeout: 5 * time.Minute,
```

5. In `DefaultConfigTOML()`, add commented examples near the other model/runtime settings:

```toml
# llm_stream_idle_timeout = "90s"
# cloud_llm_stream_idle_timeout = "5m"
```

6. In `Load`, add both keys to the base `confmap` using `.String()` values, matching existing duration keys like `bash_timeout`.
7. Initialize both source fields to `"default"`.
8. Apply flag overrides after config files load:

```go
if flags.LLMStreamIdleTimeout != nil {
    _ = k.Set("llm_stream_idle_timeout", *flags.LLMStreamIdleTimeout)
    src.LLMStreamIdleTimeout = "flag"
}
if flags.CloudLLMStreamIdleTimeout != nil {
    _ = k.Set("cloud_llm_stream_idle_timeout", *flags.CloudLLMStreamIdleTimeout)
    src.CloudLLMStreamIdleTimeout = "flag"
}
```

9. In `markSourcesFromKoanf`, mark both keys.
10. In `markUnknownKeys`, add both keys to `knownTop`.
11. After unmarshal, normalize valid-but-too-small durations:

```go
if cfg.LLMStreamIdleTimeout < time.Second {
    cfg.LLMStreamIdleTimeout = time.Second
}
if cfg.CloudLLMStreamIdleTimeout < time.Second {
    cfg.CloudLLMStreamIdleTimeout = cfg.LLMStreamIdleTimeout
}
```

Do not silently accept invalid duration syntax. If `koanf.Unmarshal` returns an
error for an invalid duration, preserve the load error behavior.

Tests:

Add or update tests for:

- Defaults are `90s` and `5m`.
- User config overrides both fields and sources become `"user"`.
- Project config overrides user config and source becomes `"project"`.
- Flag override wins and source becomes `"flag"`.
- Valid values below `1s` are clamped.
- Unknown-key warning does not fire for the two new keys.

Run:

```bash
go test ./internal/config
```

Acceptance:

- `config.Load` exposes both durations with correct precedence and source tracking.
- Invalid duration strings still fail loudly.

### Task 3: Add Watchdog Defaults and Agent Selection

Owner: agent/LLM agent.

Purpose:

Allow one `agent.Config` to carry both local/default and cloud watchdog policies.

Files:

- `internal/llm/watchdog.go`
- `internal/llm/watchdog_test.go`
- `internal/agent/input.go`
- `internal/agent/agent.go`
- `internal/agent/stream.go`
- `internal/agent/agent_test.go`

Implementation:

1. Keep `llm.DefaultWatchdogConfig()` unchanged at `90s` idle and `45s` warning.
2. Add a cloud default helper:

```go
func DefaultCloudWatchdogConfig() WatchdogConfig {
    return WatchdogConfig{
        IdleTimeout:        5 * time.Minute,
        IdleWarningTimeout: 60 * time.Second,
        OnIdleWarning:      nil,
    }
}
```

The warning value is used only when a caller sets `OnIdleWarning`. The agent
stream path now sets that callback so users can see an informational idle
warning before the final timeout.

3. Add to `agent.Config`:

```go
CloudWatchdog llm.WatchdogConfig
```

4. Update `agent.DefaultConfig()` to set:

```go
Watchdog:      llm.DefaultWatchdogConfig(),
CloudWatchdog: llm.DefaultCloudWatchdogConfig(),
```

5. Add to `agent.Input`:

```go
LLMProvider string
```

Place it near `Model` so run identity fields stay together.

6. Add an unexported helper on `agent.Config`:

```go
func (c Config) watchdogForProvider(provider string) llm.WatchdogConfig
```

Rules:

- if `provider == string(llm.ProviderOllamaCloudAPI)` and `c.CloudWatchdog.IdleTimeout > 0`, return `c.CloudWatchdog`;
- otherwise, if `c.Watchdog.IdleTimeout > 0`, return `c.Watchdog`;
- otherwise return `llm.DefaultWatchdogConfig()`.

7. Thread provider into one-turn execution:

- Change `executeOneTurn` signature in `internal/agent/stream.go` to accept `llmProvider string`.
- In `internal/agent/agent.go`, pass `in.LLMProvider` from the call site that currently starts with `a.executeOneTurn(ctx, in.Model, ...)`.
- In `executeOneTurn`, use `a.config.watchdogForProvider(llmProvider)` when calling `llm.WatchStream`.

Tests:

Add focused tests for `watchdogForProvider`:

- empty provider uses `Watchdog`;
- local provider uses `Watchdog`;
- cloud provider uses `CloudWatchdog`;
- zero-value config falls back to `llm.DefaultWatchdogConfig()`.

Add one behavioral stream test if practical:

- local idle timeout is short;
- cloud idle timeout is longer;
- fake stream delays longer than local but shorter than cloud;
- a cloud input completes without synthetic `watchdog_timeout`.

Run:

```bash
go test ./internal/llm ./internal/agent
```

Acceptance:

- Existing local behavior remains default.
- Cloud runs can use a longer watchdog without changing the LLM client interface.

### Task 4: Build Agent Config From Loaded Timeouts

Owner: CLI/server integration agent.

Purpose:

Ensure REPL, print, and server modes put config-loaded watchdog values into `agent.Config`.

Files:

- `internal/bootstrap/state.go`
- `internal/cli/repl.go`
- `internal/cli/print.go`
- `internal/server/server.go`

Implementation:

1. In `bootstrap.Initial` and `bootstrap.Snapshot`, add:

```go
LLMStreamIdleTimeout      time.Duration
CloudLLMStreamIdleTimeout time.Duration
```

2. In `bootstrap.DefaultInitial`, set `90s` and `5m`.
3. In `bootstrap.New`, copy both fields into the snapshot.
4. In REPL config loading (`internal/cli/repl.go`), copy:

```go
initial.LLMStreamIdleTimeout = cfgRes.Config.LLMStreamIdleTimeout
initial.CloudLLMStreamIdleTimeout = cfgRes.Config.CloudLLMStreamIdleTimeout
```

5. In print config loading (`internal/cli/print.go`), copy the same fields.
6. In server config loading (`internal/server/server.go`), copy the same fields into `init`.
7. When constructing `agentCfg`, set:

```go
agentCfg.Watchdog = watchdogFromIdle(initialOrSnap.LLMStreamIdleTimeout, llm.DefaultWatchdogConfig())
agentCfg.CloudWatchdog = watchdogFromIdle(initialOrSnap.CloudLLMStreamIdleTimeout, llm.DefaultCloudWatchdogConfig())
```

If the helper is shared from `internal/agent`, export it; otherwise keep it
local to each package. It must preserve `OnIdleWarning` from the fallback config
and only adjust idle and warning durations.

Tests:

- Update bootstrap/state tests if they assert full snapshots.
- Add or update print/server tests if fake hooks can inspect constructed config.

Run:

```bash
go test ./internal/bootstrap ./internal/cli ./internal/server
```

Acceptance:

- Loaded config values reach every top-level agent configuration path.

### Task 5: Add CLI Flag Plumbing

Owner: CLI agent.

Purpose:

Expose runtime overrides without requiring config file edits.

Files:

- `internal/cli/root.go`
- `internal/cli/repl.go`
- `internal/cli/print.go`
- `internal/cli/server.go`
- `internal/server/server.go`
- `internal/cli/root_test.go`
- server CLI tests if present

Implementation:

1. In root command options, add:

```go
llmStreamIdleTimeout      string
cloudLLMStreamIdleTimeout string
```

2. Register root flags:

```go
rootCmd.Flags().StringVar(&opts.llmStreamIdleTimeout, "llm-stream-idle-timeout", "", "LLM stream idle watchdog timeout")
rootCmd.Flags().StringVar(&opts.cloudLLMStreamIdleTimeout, "cloud-llm-stream-idle-timeout", "", "Ollama Cloud stream idle watchdog timeout")
```

3. Add the same fields to `replOptions` and `printOptions`.
4. Pass these options from `root.go` into `runREPL` and `runPrintFn`.
5. Include them in `config.FlagOverrides` inside REPL and print.
6. In `serverOptions` and `server.Config`, add the same two string fields.
7. Register the same two flags on the `server` subcommand.
8. Pass them from `cli/server.go` into `server.Config`.
9. Include them in `config.Load` inside `server.New`.

Tests:

- Extend `TestRootPrintPassesNumCtxOption` or add a new root test that captures `printOptions` and verifies both timeout strings are passed through.
- Add a similar test for REPL if the codebase has a REPL option-capture pattern.
- Add server command parsing coverage if existing tests make it practical.

Run:

```bash
go test ./internal/cli ./internal/server
```

Acceptance:

- `nandocodego --cloud-llm-stream-idle-timeout 5m ...` overrides config.
- `nandocodego --print ... --cloud-llm-stream-idle-timeout 5m` passes the override into print mode.
- `nandocodego server --cloud-llm-stream-idle-timeout 5m` passes the override into server config loading.

### Task 6: Propagate Provider Into Agent Inputs

Owner: runtime integration agent.

Purpose:

Make each run choose the correct watchdog by provider.

Files:

- `internal/tui/app.go`
- `internal/cli/print.go`
- `internal/server/session.go`
- `internal/agent/subagent.go`
- `internal/tools/agenttool/agenttool.go`
- `internal/tools/tasktool/tasktool.go`
- `internal/tasks/supervisor.go`
- related tests in `internal/tui`, `internal/server`, `internal/tasks`, and `internal/tools/agenttool`

Implementation:

1. TUI main prompt:
   - In the `agent.Input` built around `internal/tui/app.go` main prompt submission, set `LLMProvider: appState.LLMProvider`.
2. TUI resume prompt:
   - In the resume path `agent.Input`, set `LLMProvider: appState.LLMProvider`.
3. TUI BTW prompt:
   - In the BTW `agent.Input`, set `LLMProvider: appState.LLMProvider`.
4. Print mode:
   - After `modelRuntimeSvc.Switch`, `initial.LLMProvider` is already set from `switchRes.Resolved.Provider`.
   - Set `in.LLMProvider = initial.LLMProvider` before running the agent.
   - Optionally extend `buildPrintInput` to accept provider, but do not pass provider into context packing unless context packing actually needs it.
5. Server mode:
   - `handlePostMessage` already calls `sess.applyModelSwitch(switchRes)` before `StartRun`.
   - In `Session.runAgent`, set `LLMProvider: app.LLMProvider` on `agent.Input`.
6. Sub-agent copying:
   - In `internal/agent/subagent.go`, copy `parentInput.LLMProvider` into `childInput.LLMProvider`.
7. Agent tool:
   - Add provider plumbing alongside the existing model callback.
   - Either extend `agenttool.New` to accept `getProvider func() string` and update all call sites, or add a `SetProviderFunc` method. Prefer the simpler change with explicit constructor argument if all call sites are in-repo.
   - Set `parentInput.LLMProvider` before `agent.RunSubagent`.
8. Task tool and task supervisor:
   - Add provider callback to `tasktool.NewWithAgent`.
   - Pass provider into `tasks.AgentRunFunc` and `tasks.AgentRunFuncWithMailbox`.
   - Add `LLMProvider: provider` to the `agent.Input` built in `agentRunFuncInternal`.
9. Explicit task or agent model override:
   - Do not add model-resolution logic in tasktool/agenttool as part of this work. If a task overrides the model, it should still use the active provider metadata unless a future task adds per-task model resolution.

Tests:

- Server: fake runner captures `agent.Input` and verifies cloud provider is present after `applyModelSwitch`.
- Print: capture or inspect input path if practical.
- Agent subagent: parent input with cloud provider produces child input with cloud provider. If existing tests cannot inspect child input directly, add a small fake client assertion around watchdog choice.
- Task supervisor: extend existing mailbox/task tests to verify provider is passed.

Run:

```bash
go test ./internal/tui ./internal/server ./internal/agent ./internal/tasks ./internal/tools/agenttool ./internal/tools/tasktool
```

Acceptance:

- Main runs, print runs, server runs, sub-agents, and task agents all pass provider metadata into `agent.Input`.
- Cloud provider inputs select `CloudWatchdog`.

### Task 7: Fix Server Write Timeout Interaction

Owner: server agent.

Purpose:

Prevent HTTP/SSE server write timeout from ending a web run before the cloud
watchdog timeout.

Files:

- `internal/cli/server.go`
- `internal/server/server.go`
- `DOCKER_WEB_GUIDE.md`
- `USER_MANUAL.md`
- server tests if present

Implementation:

1. Change the server CLI default write timeout from `120s` to `0`.
2. Change `parseDurationDefault(opts.writeTimeout, "120s")` to use `"0"` for write timeout.
3. In `server.New`, remove or change the block that turns `cfg.WriteTimeout == 0` into `120 * time.Second`.
4. Document that `--write-timeout 0` disables the write timeout, which is appropriate for long-lived SSE streams.
5. Keep `ReadTimeout` default at `30s`; it protects request header/body reads and does not limit response streaming after the request is accepted.

Alternative if maintainers reject a default change:

- Keep `120s`, but log a startup warning when `WriteTimeout > 0 && WriteTimeout < CloudLLMStreamIdleTimeout`.
- Still document `--write-timeout 0` as the recommended server setting for cloud models.

Tests:

- Verify default server config uses `WriteTimeout == 0`.
- Verify explicit `--write-timeout 120s` still sets `120s`.

Run:

```bash
go test ./internal/cli ./internal/server
```

Acceptance:

- Server mode no longer disconnects default SSE streams before the default `5m` cloud watchdog.

### Task 8: Documentation Updates

Owner: docs agent.

Files:

- `USER_MANUAL.md`
- `DOCKER_WEB_GUIDE.md`
- `docs/OLLAMA-CLOUD-API-KEY-PLAN.md` or a new short cloud runtime doc
- `docs/CLOUD-LLM-WATCHDOG-TIMEOUT-PLAN.md`

Implementation:

1. Add config table rows for:
   - `llm_stream_idle_timeout`
   - `cloud_llm_stream_idle_timeout`
2. Add CLI examples:

```bash
nandocodego --model kimi-k2.6:cloud --cloud-llm-stream-idle-timeout 5m
nandocodego --print "Generate a docs plan in @docs" --model kimi-k2.6:cloud --cloud-llm-stream-idle-timeout 5m
nandocodego server --model kimi-k2.6:cloud --cloud-llm-stream-idle-timeout 5m --write-timeout 0
```

3. Explain error boundaries:
   - `llm stream watchdog timeout`: idle stream exceeded configured watchdog.
   - `resolve @docs\``: mention parser/path issue.
   - `403 requires a subscription`: Ollama account/model entitlement issue; timeout config does not fix it.
4. Explain that idle warnings are user-visible status events, but the warning threshold itself is not a user-configurable setting.

Acceptance:

- Docs distinguish timeout, mention, and subscription errors accurately.

### Task 9: User-Visible Idle Warning

Owner: UI/agent agent.

Status: Implemented.

Purpose:

Make `IdleWarningTimeout` visible to users.

Implemented behavior:

1. `internal/agent/events.go` defines `LLMIdleWarning{Provider string, Timeout time.Duration}`.
2. `executeOneTurn` wraps `watchdog.OnIdleWarning` and emits `LLMIdleWarning`.
3. TUI renders a system item like `[Still waiting for model stream (...); idle ...]`.
4. Server emits SSE event `llm_idle_warning` with `provider`, `timeout_ms`, and `timeout_str`.
5. Print mode writes the warning to stderr, not stdout or JSON output.

Notes:

- Event ordering during streaming should remain deterministic enough for tests.
- Print JSON output must remain parseable; warning text belongs on stderr.
- This task did not add user-facing warning-timeout config. Only final idle timeout settings are configurable.

## Tests

### Config Tests

Add/extend `internal/config/loader_test.go`:

- Defaults include `90s` and `5m`.
- User config can override both new keys.
- Project config overrides user config.
- CLI flags override config values.
- Valid but too-small durations are normalized.
- Invalid duration syntax fails config loading.
- Unknown-key detection accepts the new keys.

### Agent Tests

Add tests in `internal/agent`:

- Local/default provider uses `Config.Watchdog`.
- Cloud provider uses `Config.CloudWatchdog`.
- Empty provider falls back to `Config.Watchdog`.
- Cloud watchdog prevents a synthetic timeout when the delay is greater than local idle timeout but below cloud idle timeout.

Use small durations, for example:

- local idle timeout: `10ms`
- cloud idle timeout: `100ms`
- fake stream emits after `40ms`

### CLI/Server Tests

Update CLI tests where root options are asserted:

- `--cloud-llm-stream-idle-timeout 5m` reaches config loading.
- print mode sets `agent.Input.LLMProvider` from the switch result.

Update server tests:

- server config loads cloud watchdog override.
- session `StartRun` passes `app.LLMProvider` into agent input.
- default server write timeout is `0`.

### Idle Warning Tests

Keep coverage for:

- `llm.WatchStream` calls `OnIdleWarning` at the configured warning threshold.
- `llm.DefaultCloudWatchdogConfig()` keeps the cloud warning threshold at `60s`.
- `llm.WithIdleTimeout` clamps warning timeout to half the idle timeout when the existing warning would be invalid.
- print-mode collection returns idle warning text separately from assistant content and JSON/stdout output.

### Mention Tests

Add:

```go
if got := NormalizeMentionPath("docs`"); got != "docs" {
    t.Fatalf("expected docs, got %q", got)
}
```

## Manual Verification

1. Local model still uses the old behavior:

```bash
nandocodego --model qwen3.6:35b
```

2. Cloud model accepts a longer idle window:

```bash
nandocodego --model kimi-k2.6:cloud --cloud-llm-stream-idle-timeout 5m
```

3. Print mode:

```bash
nandocodego --print "Generate a short implementation note in @docs" \
  --model kimi-k2.6:cloud \
  --cloud-llm-stream-idle-timeout 5m
```

4. Server mode:

```bash
nandocodego server \
  --model kimi-k2.6:cloud \
  --cloud-llm-stream-idle-timeout 5m \
  --write-timeout 0
```

5. Mention parsing:

```text
Generate a document in @docs`
```

Expected: the app resolves `docs`, not `docs\``.

6. Subscription failure:

Use a cloud model that the API key cannot access.

Expected: the app still reports a provider `403` error. The watchdog should not mask entitlement failures.

7. Idle warning visibility:

Use a test stream or slow cloud stream that crosses the warning threshold but not the final idle timeout.

Expected:

- TUI shows `[Still waiting for model stream (...); idle ...]`.
- Server clients receive `llm_idle_warning`.
- `--print` writes the warning to stderr and keeps stdout/JSON output parseable.

## Acceptance Criteria

- `llm stream watchdog timeout` no longer occurs for cloud responses that are idle for less than the configured cloud timeout.
- Local model watchdog behavior remains unchanged by default.
- Users can raise the cloud watchdog timeout with a CLI flag or config entry.
- Server/SSE mode does not disconnect earlier than the configured cloud stream idle timeout.
- Idle warnings are visible in TUI, server SSE, and print stderr without changing the final timeout behavior.
- `@docs\`` resolves to `docs`.
- Ollama Cloud `403 requires a subscription` remains visible as a provider access error.
- Focused tests pass:

```bash
go test ./internal/config ./internal/llm ./internal/agent ./internal/mentions ./internal/cli ./internal/server
```

## Suggested Implementation Order

1. Land the mention normalization fix and test.
2. Add config fields, defaults, loader support, and config tests.
3. Add `CloudWatchdog` to `agent.Config` and provider-aware watchdog selection.
4. Add `LLMProvider` to `agent.Input` and propagate it through TUI, print, and server.
5. Add CLI flags.
6. Set server `WriteTimeout` default to `0`.
7. Propagate provider into sub-agent/task paths.
8. Add user-visible idle warning events.
9. Update docs.
10. Run focused tests, then full `go test ./...` if time allows.
