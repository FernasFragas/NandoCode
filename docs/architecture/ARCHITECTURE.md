# Architecture Overview

Last reviewed: 2026-09-28 against `main` (`de64d62`).

This is the narrative entry point to how `nandocodego` is built. For diagrams of
each flow, see [APPLICATION-ARCHITECTURE-FLOWCHART.md](APPLICATION-ARCHITECTURE-FLOWCHART.md).
For how model and embedding calls are routed between local Ollama and Ollama
Cloud, see [EMBEDDING-AND-MODEL-ROUTING.md](EMBEDDING-AND-MODEL-ROUTING.md).

![Whole-application architecture](images/whole-application.png)

## What The System Is

`nandocodego` is a single Go binary (module `github.com/FernasFragas/Nandocode`)
that runs a local-first coding agent. A model served by Ollama is given the
user's prompt plus packed project context. It can call tools (read, edit, search,
run shell commands, spawn sub-agents), and every tool call passes through a
permission system before it runs. The same agent runtime is exposed through
several surfaces:

| Surface | Entry | Notes |
| --- | --- | --- |
| Interactive TUI (REPL) | `nandocodego` with no subcommand | Bubble Tea UI. Primary surface. |
| One-shot | `nandocodego --print "<prompt>"` (`--json` for machine output) | No interactive prompts; fails closed when input is needed. |
| HTTP/SSE server + browser UI | `nandocodego server` (default `127.0.0.1:8080`) | Token-authenticated sessions, embedded web UI, HTTP permission broker. |
| Semantic index | `nandocodego index build\|refresh\|status\|clear [path]` | Builds the local embedding index used for retrieval. |
| Evaluation | `nandocodego eval run\|validate [path]` | Runs the coding-task fixtures in `evals/` through the real agent loop. |
| Utilities | `doctor`, `init`, `version` | Environment diagnostics, project setup, build info. |

Defaults that matter: the local Ollama URL is `http://localhost:11434`, the
default chat model is `qwen3.6:35b` (`internal/llm/defaults.go`), and the default
embedding model is `qwen3-embedding:8b`. Cloud models are used only when the user
selects one and supplies an `OLLAMA_API_KEY`.

## Layers

```text
┌──────────────────────────────────────────────────────────────────────────┐
│ Surfaces        cmd/nandocodego → internal/cli                           │
│                 internal/tui (terminal)   internal/server (+ web/)       │
├──────────────────────────────────────────────────────────────────────────┤
│ Session wiring  internal/bootstrap (immutable session facts)             │
│                 internal/state (reactive UI/app store)                   │
│                 internal/config, internal/commands (slash commands)      │
├──────────────────────────────────────────────────────────────────────────┤
│ Agent runtime   internal/agent  ── event stream ──▶ surfaces             │
│   decorators    internal/memory (recall/extract), internal/hooks         │
│   context       internal/contextpack, internal/mentions,                 │
│                 internal/retrievalroute, internal/semantic,              │
│                 internal/analysis                                        │
│   work units    internal/tasks (background tasks), sub-agents,           │
│                 coordinator mode + mailboxes                             │
├──────────────────────────────────────────────────────────────────────────┤
│ Capabilities    internal/tools/* (registry + built-in tools)             │
│                 internal/permissions (single decision point)             │
│                 internal/mcp (external tool servers), internal/skills    │
├──────────────────────────────────────────────────────────────────────────┤
│ Model access    internal/llm (Client interface, retry, watchdog)         │
│                 internal/llm/ollama, modelresolver, modelruntime         │
│                 internal/credentials (API key: env / keychain / prompt)  │
├──────────────────────────────────────────────────────────────────────────┤
│ Foundations     internal/paths, internal/logging, internal/observability,│
│                 internal/ids, internal/types, internal/version           │
└──────────────────────────────────────────────────────────────────────────┘
```

Dependencies point downward. `internal/agent` depends only on the `llm.Client`
interface, not on Ollama. The surfaces never call tools directly; they submit
input to the agent and render the events it emits.

## Package Map

| Package | Responsibility |
| --- | --- |
| `cmd/nandocodego` | Process entrypoint; signal handling; calls `cli.Run`. |
| `internal/cli` | Cobra commands (`root`, `repl`, `print`, `server`, `index`, `eval`, `doctor`, `init`) and runtime assembly for each surface. |
| `internal/tui` | Bubble Tea model: transcript, markdown, input, Vim mode, keybindings, permission modal, run state/status bar, index progress. `fileindex/` and `picker/` power `@` file completion. |
| `internal/server` | HTTP/SSE API (`/v1/health`, `/v1/models`, `/v1/sessions/...`), token auth, rate limiting, session manager with event ring buffer, HTTP permission broker, embedded browser UI in `web/`. |
| `internal/commands` | Slash-command registry: `/help /clear /exit /model /models /pull /memory /context /hooks /permissions /skills /cost /trace /prompt /init /agents /queue /compact /refresh-index /analyze-project /checkpoint /bg /btw /semantic /index`. |
| `internal/config` | Layered TOML config (user `config.toml` + project `.nandocodego/config.toml` + flags), with source tagging and warnings. |
| `internal/bootstrap` | Thread-safe snapshot of session-level facts (model, URLs, working dir, context limits). |
| `internal/state` | Reactive store for UI-facing app state (active model/provider, run flags, usage). |
| `internal/agent` | The query loop: builds `llm.ChatRequest`, streams the model turn, partitions and executes tool calls (concurrently when safe), handles retries/incomplete responses, compaction, sub-agents, fork, coordinator mode, prompt dumps. Ends each run with a `Terminal` reason (`completed`, `aborted`, `max_turns`, `context_overflow`, `stop_hook`, `unrecoverable`). |
| `internal/contextpack` | Packs current-turn evidence (mentioned files, ranges, directory trees) into a token budget and records what was omitted. |
| `internal/mentions` | Parses and expands `@path`, `@dir/`, `@file#L10-L20`, `?content` mentions; detects listing intent. |
| `internal/retrievalroute` | Decides per prompt whether to run semantic retrieval and which tool mode to use (e.g. chat-only fast path for trivial prompts). |
| `internal/semantic` | Local workspace index: scan, extract, embed via Ollama `/api/embed`, vector store, hybrid search, evidence rendering, progress events. |
| `internal/analysis` | Project-analysis workflow foundations: chunking, summary cache, evidence ledger, checkpoints, retrieval helpers. |
| `internal/memory` | File-based project memory: scan, recall into the prompt, extract pending drafts after a run. Wraps the agent as a runner decorator. |
| `internal/hooks` | Lifecycle hooks (session, prompt, pre/post tool, stop). Command and prompt hooks run; project/HTTP/agent hooks are parsed but disabled until a trust flow exists. |
| `internal/tasks` | Background task supervisor (bash and agent tasks), JSONL output, lifecycle and stop. |
| `internal/tools` | `Tool` interface, registry, schemas, path safety, execution context. |
| `internal/tools/*` | Built-in tools: `bash`, `fileread`, `filewrite`, `fileedit`, `glob`, `grep`, `webfetch`, `todo`, plus `agenttool` (sub-agents), `tasktool`, `sendmessage` (coordinator mailboxes), `skilltool`, `selfinfo`, `dirwalk` (shared walker). `builtin` registers the default and read-only sets. |
| `internal/permissions` | Permission modes, rule matching, and the central resolver every tool call goes through. |
| `internal/mcp` | MCP config, stdio/HTTP transports, server lifecycle, tool wrapping; `auth/` for MCP HTTP auth. |
| `internal/skills` | Skill discovery (user, project, bundled `assets/`), frontmatter, prompt loading. |
| `internal/llm` | Provider-neutral types, `Client` interface, `RuntimeClient` switcher, retry, stream watchdog, model capabilities. |
| `internal/llm/ollama` | Ollama HTTP client (chat streaming, list/pull, embed). |
| `internal/llm/modelresolver` | Resolves a model name to local or cloud, local first. |
| `internal/llm/modelruntime` | Switches the active model/provider; requires credentials before any cloud call. |
| `internal/credentials` | Ollama Cloud API key from session, `OLLAMA_API_KEY`, or OS keychain. |
| `internal/eval` | Eval fixture loading/validation, isolated workspaces, recorded-model client, checks, scoring, JSON/Markdown reports. |
| `internal/observability` | Metrics decorators, meters, retry/done-reason diagnostics feeding `/cost` and `/trace`. |
| `internal/paths` | XDG-aware config/data/cache/state/memory/session/skills directories (overridable with `NANDOCODEGO_*`). |
| `internal/logging`, `internal/ids`, `internal/types`, `internal/version` | slog setup, kind-prefixed IDs, shared task types, build metadata. |

## How One Prompt Flows (TUI)

1. `cmd/nandocodego/main.go` calls `cli.Run`. With no subcommand, `internal/cli/repl.go` runs.
2. The REPL loads config, builds the `bootstrap` snapshot and `state` store, creates the Ollama client wrapped in `llm.RuntimeClient`, the model runtime, the tool registry, MCP servers, the memory and hook runners, and the semantic service.
3. It creates `agent.Agent` and the `tui.Model`, then starts Bubble Tea.
4. The user submits a prompt. If the active model is cloud-only, the model runtime obtains an API key first, before any project context leaves the machine.
5. The TUI expands `@` mentions (`internal/mentions`), packs current-turn evidence (`internal/contextpack`), asks `internal/retrievalroute` whether to add semantic retrieval, and builds `agent.Input`.
6. Hook and memory decorators run `UserPromptSubmit`/session hooks and inject recalled memory.
7. The agent streams the model turn. Tool calls are partitioned: consecutive concurrency-safe calls run in parallel, others serially. Each call goes through hook-aware permission resolution, and the TUI modal blocks the agent goroutine, never the UI loop.
8. Tool results are appended and the loop repeats until the model stops, a limit is hit, or the user aborts. The run ends with a `Terminal` event.
9. The TUI reduces every event into transcript items, status bar, usage, and notices. Memory extraction may queue pending drafts.

The server (`internal/server`) runs the same steps per HTTP session, with SSE
events instead of Bubble Tea messages and an HTTP permission broker instead of the
modal. `--print` runs them once, without prompting.

## Design Rules To Preserve

- **One event stream.** The agent emits typed events; every surface (TUI, server, future remote client) is a consumer. New surfaces should reuse it, not add side channels.
- **Separate session facts from UI state.** `bootstrap` holds immutable session facts; `state` holds reactive UI state.
- **Fail closed at boundaries.** Tools, permissions, hooks, MCP, credentials, and server auth deny by default. Tools are unsafe and destructive unless they declare otherwise.
- **Permissions have one decision point.** Nothing executes a tool without `permissions.Resolve`.
- **Local first.** Local Ollama is the default. Cloud is opt-in per model and credential-gated. Network endpoints are enforced by `tools/check-network-policy.sh`.
- **Control context explicitly.** Budgets, packing, retrieval, summaries, checkpoints. Don't lower `num_ctx` globally to fix latency.
- **Keep dependencies small.** New direct dependencies must be added to `tools/allowed-deps.txt` and justified.
- **Workers get less power.** Sub-agents and coordinator workers use restricted tool registries.

## State And Storage

| What | Where |
| --- | --- |
| User config | `paths.ConfigDir()/config.toml` (XDG config dir) |
| Project config | `.nandocodego/config.toml` in the repo |
| Project memory | `paths.MemoryDir(gitRoot)` |
| Sessions, task output (JSONL) | `paths.DataDir()/sessions/<session>/...` |
| Semantic index cache | Under `paths.CacheDir()`, outside the source tree |
| Skills | `paths.SkillsDir()` (user), project skills dir, bundled `internal/skills/assets` |
| Eval artifacts | `.tmp-evals/` (gitignored) |

Server sessions are in memory; they do not survive a server restart (Phase 25 adds detach/reconnect).

## Security Boundaries

See [SECURITY.md](../../SECURITY.md) for the full policy. In short:

- Shell and file tools run with the user's permissions. The permission modes and rules are the guard.
- Project-controlled hooks are not executed until a workspace trust flow exists.
- Cloud API keys come from env, keychain, or an interactive prompt, and are never logged.
- The server binds to loopback by default and requires a bearer token.
- CI (`.github/workflows/ci.yml`) enforces the dependency allowlist, network policy, build/vet/race tests on Linux/macOS/Windows, formatting and golangci-lint, dependency review, and the deterministic eval suite. `security.yml` runs gosec (report-only), govulncheck, and a container image scan that fails on fixable critical/high vulnerabilities.

## Extension Points

| To add... | Do this |
| --- | --- |
| A tool | Implement `tools.Tool` in `internal/tools/<name>`, declare `IsConcurrencySafe`/`IsDestructive`, register in `internal/tools/builtin`, add tests. |
| A slash command | Register it in `internal/commands/registry.go`; the TUI dispatches through the registry. |
| A model family | Update `internal/llm/capabilities.go` and its tests. |
| An eval fixture | Add `evals/<id>/` with `task.md`, `repo/`, `expected/`, `scoring.yaml`, `recordings/`; run `make eval-validate`. |
| An MCP server or skill | Configuration only. See `USER_MANUAL.md` §12 and §14. |
| A new LLM provider | Out of scope for v0.1. If revived, implement `llm.Client` in a new `internal/llm/<provider>` package. |

## Deeper References

- [APPLICATION-ARCHITECTURE-FLOWCHART.md](APPLICATION-ARCHITECTURE-FLOWCHART.md): Mermaid diagrams per flow.
- [EMBEDDING-AND-MODEL-ROUTING.md](EMBEDDING-AND-MODEL-ROUTING.md): local vs cloud chat and embedding routing.
- [../guides/file-and-folder-context-pipeline.md](../guides/file-and-folder-context-pipeline.md): how files and folders become model context.
- [../guides/DEBUG-BREAKPOINTS.md](../guides/DEBUG-BREAKPOINTS.md): where to break in each stage of a request.
- [../adr/](../adr/): architecture decision records.
- `docs/phases/PHASE-N-DETAILED-PLAN.md`: the design spec each subsystem was built from.
