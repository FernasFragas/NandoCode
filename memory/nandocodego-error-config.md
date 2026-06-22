---
name: nandocodego-error-config
description: Error handling rules, config source priority order, logging policy, security constraints, and credential handling for nandocodego
type: feedback
---

## Error Handling Rules

- Errors are part of the type — use `errors.Is`/`errors.As` for classification
- Stable sentinel errors per package: `var ErrFoo = errors.New("foo: ...")`
- Wrap with `fmt.Errorf("...: %w", err)` to preserve chain
- **Never log raw errors to telemetry** — classify first via `llm.Classify(err)`
- Tool errors returned as `tools.Result{Err: ...}`, not panicked
- `Error()` strings must not include prompts, response bodies, tokens, env vars, or headers

## Error Classes (`internal/llm/errors.go`)

`canceled` | `deadline` | `network` | `server` | `rate_limited` | `model_missing` | `bad_request` | `unauthorized` | `forbidden` | `decode` | `watchdog_timeout` | `unknown`

Retry policy defaults:
- `network`/`deadline`: 3 retries, 500ms base, 5s max
- `server`: 5 retries, 1s base, 20s max
- `rate_limited`: 3 retries, 2s base, 30s max
- `model_missing`: 1 retry
- `canceled`/`bad_request`/`unauthorized`/`forbidden`/`decode`: 0 retries

## Config Source Priority (tagged at parse time)

1. Enterprise policy (`/etc/nandocodego/policy.toml`)
2. User (`~/.config/nandocodego/config.toml`)
3. Project (`.nandocodego/config.toml`, version-controlled)
4. Local (`.nandocodego/config.local.toml`, gitignored)
5. CLI flags
6. Session (`/permissions allow ...`)

"User-only" overrides intentionally **exclude** project sources to defeat malicious-repo redirects.

## Path Resolution (`internal/paths`)

- `ConfigDir()` → `NANDOCODEGO_CONFIG_HOME` or `XDG_CONFIG_HOME/nandocodego` or `~/.config/nandocodego`
- `DataDir()` → `NANDOCODEGO_DATA_HOME` or `XDG_DATA_HOME/nandocodego` or `~/.local/share/nandocodego`
- `CacheDir()` → `NANDOCODEGO_CACHE_HOME` or `XDG_CACHE_HOME/nandocodego` or `~/.cache/nandocodego`
- `StateDir()` → `NANDOCODEGO_STATE_HOME` or `XDG_STATE_HOME/nandocodego` or `~/.local/state/nandocodego`
- `MemoryDir(gitRoot)` → `DataDir()/projects/<sanitized-git-root>/memory`
- `SessionDir(id)` → `StateDir()/sessions/<id>`

## Logging Policy

- `slog.Logger` only — no `fmt.Println`, no `log.Print`
- Two channels: TUI-visible (Bubble Tea messages) vs. structured logs (slog)
- Structured logs always INFO unless `NANDOCODEGO_DEBUG=1`
- **Never log**: prompts, model outputs, file contents, embeddings, tokens, env vars, raw response bodies at INFO

## Security Constraints

- Outbound HTTP: only configured Ollama base URL by default (`http://localhost:11434`)
- WebFetch/WebSearch: only when invoked as tools and approved by permissions
- MCP servers: only when user explicitly registers them
- Telemetry: off by default; requires `NANDOCODEGO_TELEMETRY=1` + explicit endpoint env var
- Credentials: env vars preferred; OS keyring only where specified; never in TOML/logs/memory
- Sub-agents must not inherit stronger permissions than parent
