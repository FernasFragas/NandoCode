# Phase 13 Detailed Plan - Slash Commands and Config UX

Date: 2026-05-07
Status: ✅ Implemented in code and automated checks (2026-05-08); live exit-gate validation pending
Source plans:

- `.codex/go-ollama-plan-AGENTS.md`
- `.codex/go-ollama-plan-HUMANS.md`
- `docs/PHASE-LOG.md`
- `docs/PROJECT-STATUS-AND-ONBOARDING.md`
- `docs/PHASE-8-DETAILED-PLAN.md`
- `docs/PHASE-9-DETAILED-PLAN.md`
- `docs/PHASE-12-DETAILED-PLAN.md`
- `book/ch13-terminal-ui.md`
- `.codex/agent-context/learnings-memory.md`

## Goal

Phase 13 completes the slash command system, implements hierarchical TOML-based configuration loading with source-tagged provenance, adds the `--print` non-interactive one-shot mode, and wires the `/model` command to validate against Ollama's live model list. When Phase 13 is done, the product is fully discoverable and configurable without editing Go files.

Deliverables:

- `internal/config` package: hierarchical config loading with `koanf/v2`, source tagging, and a Config struct that mirrors `bootstrap.Initial` fields.
- `internal/commands` package: one handler file per slash command group. Replaces and extends the minimal Phase 7 slash dispatch.
- `internal/cli/print.go`: non-interactive `--print` mode that runs one agent turn and exits.
- Updated `internal/cli/repl.go`: config loaded at startup before bootstrap defaults; config feeds `bootstrap.Initial`.
- Full slash command surface: 20 commands across model, memory, hooks, permissions, skills, cost, init, and agents namespaces.
- `nandocodego init` subcommand: writes a default `~/.nandocodego/config.toml` if not already present.
- Phase log update after implementation.

## Definition Of Success

Phase 13 is complete when this end-to-end flow works:

1. `nandocodego init` creates `~/.nandocodego/config.toml` with commented defaults.
2. Edit the config file to set `default_model = "qwen3"`.
3. Run `nandocodego` without `--model`. Verify the REPL starts with the model from the config file.
4. Run `nandocodego --print "What is 2+2?"`. Verify the assistant response is printed to stdout and the process exits 0.
5. Inside the REPL, run `/models`. Verify a list of models from Ollama appears.
6. Run `/memory list`. Verify memory files are listed.
7. Run `/permissions show`. Verify current mode and source-tagged rules are shown.
8. Run `/hooks list`. Verify active hooks are listed by event kind.

This exit gate must work against a live Ollama instance for model-dependent commands and without it for config-only tests.

## Baseline Analysis From Implemented Phases

### Phase 0 - Security Baseline

Implemented:

- Dependency allowlist.
- Network policy checker.
- No-secrets log policy.

Phase 13 implications:

- Config files may contain sensitive values (API keys for future MCP servers, custom Ollama URLs).
- Config values must never be logged at INFO. Log only field names and source labels.
- `koanf/v2` and its TOML provider must be added to `tools/allowed-deps.txt` with justification.
- `BurntSushi/toml` or the koanf-bundled TOML parser must also be allowlisted if it becomes a direct dependency.
- `nandocodego init` must not overwrite an existing config file. It is a one-shot bootstrap command.
- Config from project `.nandocodego/config.toml` must not be treated as policy-level authority. It is user-trusted but not operator-level.

### Phase 1 - CLI, Paths, Logging, Scaffold

Implemented:

- `internal/paths` with `ConfigDir()`, `DataDir()`, and all path helpers.
- Cobra root command, `doctor`, `version` subcommands.
- Structured logging.
- Empty `internal/config/` scaffold.
- Empty `internal/commands/` scaffold.

Phase 13 implications:

- `internal/config` is the correct package for the loader.
- Config files live at:
  - `~/.nandocodego/config.toml` (user config, from `paths.ConfigDir()`).
  - `<project>/.nandocodego/config.toml` (project config, from working directory).
- `nandocodego init` is a new Cobra subcommand in `internal/cli/init.go`.
- `--print` mode is a global flag on the root command or a dedicated positional flow in `internal/cli/print.go`.
- `doctor` can be extended later to show config file status; do not change it in Phase 13.

### Phase 2 - LLM Client

Implemented:

- `llm.Client.ListModels()` returns `[]llm.ModelInfo`.
- `llm.Client.PullModel()` streams `PullProgress`.
- Streaming and non-stream chat.

Phase 13 implications:

- `/models` uses `llm.Client.ListModels()`.
- `/pull <model>` uses `llm.Client.PullModel()` with progress streaming to the transcript.
- `/model <name>` validation uses `llm.Client.ListModels()` to confirm the named model is available.
- `--print` mode uses `llm.Client.Chat` via the normal agent loop; no special LLM client path is needed.
- Model list requests must respect the configured `OllamaBaseURL` (which may come from config file, not just CLI flag).

### Phase 3 - Tool Interface and Starter Tools

Implemented:

- `tools.Tool`, `tools.Registry`, built-in tools.
- Path safety, permission classification.

Phase 13 implications:

- No new tools are introduced in Phase 13. All slash commands are TUI/CLI commands, not agent tools.
- `--print` mode uses the same tool registry as the REPL.
- `internal/commands` must not import `internal/tools` directly; slash commands interact with the agent and state, not with tool internals.

### Phase 4 - Agent Loop

Implemented:

- `agent.Run(ctx, input) <-chan agent.Event`.
- Terminal event with conversation payload.
- Turn budget and context overflow handling.

Phase 13 implications:

- `--print` mode calls `agent.Run` once, drains all events, prints `AssistantTextDelta` output, and exits.
- The print mode handler must handle `agent.Terminal` to detect failure reasons and exit with an appropriate code.
- No TUI is instantiated in `--print` mode. Event draining happens directly in `internal/cli/print.go`.
- `--json` flag for `--print` mode must marshal `{content, tool_uses, usage}` to stdout.

### Phase 5 - Permission System

Implemented:

- `permissions.Resolve` with source-tagged rules.
- Session-level allow/deny via `PermissionMode`.

Phase 13 implications:

- `/permissions show` reads `bootstrap.Snapshot().PermissionMode` and `PermissionRules`.
- `/permissions allow <pattern>` and `/permissions deny <pattern>` add session-level rules to `state.App` using `SourceSession`.
- Session rules must survive turn boundaries (they are stored in `state.App.PermissionRules`, not in transient agent input).
- `/permissions show` must display the source label for each rule (policy, user, project, local, cli, session) so users can understand the provenance hierarchy.
- Config-loaded rules should be tagged as `SourceUser` (from user config) or `SourceProject` (from project config) respectively.

### Phase 6 - State Layer

Implemented:

- `bootstrap.State` with `Snapshot()` and `Update()`.
- `state.App` with all session state.
- `state.OnChange` mirrors infrastructure fields.

Phase 13 implications:

- Config loading results should feed `bootstrap.Initial` before `bootstrap.New()` is called, not after.
- Config values that change the permission mode or default model should be set in `bootstrap.Initial.DefaultModel` and `bootstrap.Initial.PermissionMode` before TUI initialization.
- `bootstrap.Initial` already has the correct field surface; Phase 13 just needs to populate it from the config file.
- `state.App.PermissionRules` is the correct place for session-level rules added by `/permissions allow`.
- No new state fields are needed in Phase 13 beyond what Phase 5 and 6 already defined.

### Phase 7 - Bubble Tea TUI and REPL

Implemented:

- `internal/tui/slash.go` with `/help`, `/clear`, `/exit`, `/model`.
- Agent bridge.
- Transcript rendering.
- `internal/cli/repl.go`.

Phase 13 implications:

- `internal/tui/slash.go` will be substantially extended or replaced by a routing dispatch to `internal/commands` handlers.
- The current `HandleSlashCommand` function signature accepts only `cmd` and `args` strings. Phase 13 must extend it to accept context (store reference, Ollama client, loader references) or replace it with a method on a `CommandRegistry` struct.
- `/help` enhancement: the output should list all 20 commands with their descriptions.
- `/model <name>` enhancement: validate the name against `llm.Client.ListModels()` before accepting.
- The TUI `Model` must hold references to the Ollama client, memory store, hooks snapshot, permissions context, and skills loader to pass them into command handlers.
- All slash commands must be non-blocking in `Update`; async operations use Bubble Tea commands.

### Phase 8 - Memory Core

Implemented:

- `internal/memory` with scan, recall, index loading, pending extraction.
- Memory runner decorator.
- Memory files under `paths.MemoryDir(scopeRoot)`.

Phase 13 implications:

- `/memory list` uses `memory.Scan(dir)` to list all active memory files with their type, name, and modification date.
- `/memory show <name>` reads the full file body and renders it with Glamour.
- `/memory edit <name>` opens the file in `$EDITOR` via `os/exec.Command`. The REPL must pause while the editor is open.
- `/memory promote <name>` moves a file from `<memory-dir>/pending/` to `<memory-dir>/`.
- All memory commands must use the already-resolved `memoryDir` from the REPL startup context.
- The `memory.Scan` function must be accessible to the command handlers.

### Phase 9 - Hooks

Implemented:

- `internal/hooks` with snapshot, matcher, command/prompt runners.
- User hook config from `~/.nandocodego/hooks.json`.
- Snapshot frozen at session start.

Phase 13 implications:

- `/hooks list` reads the hook snapshot (frozen at startup) and displays active hooks grouped by event kind.
- `/hooks reload` re-loads `hooks.json`, builds a new snapshot, and replaces the active snapshot. Requires user confirmation (type `yes` or press `y`) because live hook replacement is a privileged operation.
- The hooks snapshot reference in the REPL must be mutable (a pointer or wrapped in a mutex-guarded container) to support `/hooks reload`.
- Hook config path may eventually come from `config.toml` but in Phase 13 it still reads from the same `~/.nandocodego/hooks.json` path.
- Display of hooks should show each hook's kind, event, matchers, and whether it is enabled or disabled (project hooks are disabled by default).

### Phase 10 - MCP (Treat As Complete)

Implemented:

- `github.com/modelcontextprotocol/go-sdk`.
- `github.com/zalando/go-keyring`.
- `golang.org/x/oauth2`.
- `github.com/fsnotify/fsnotify`.

Phase 13 implications:

- MCP server configuration can live in `config.toml` under an `[mcp]` section.
- Phase 13 should parse and expose MCP server config from the config file, even if Phase 10 is not yet fully wired to it.
- No new MCP server wiring is introduced in Phase 13; that belongs to Phase 10 refinement.
- `keyring` and `oauth2` credentials are not stored in `config.toml`; they remain in OS keyring.

### Phase 11 - Sub-agents and AgentTool (Treat As Complete)

Implemented:

- Sub-agent spawning.
- `AgentTool` for invoking child agents.

Phase 13 implications:

- `/agents list` shows all sub-agents spawned in the current session with their status and a brief description.
- Sub-agent metadata must be tracked in `state.App` (or a parallel session-scoped store); Phase 11 should already expose this; Phase 13 displays it.
- `--print` mode with sub-agents: if the one-shot agent spawns sub-agents, they complete normally before exit. This is existing behavior from Phase 11 and does not require Phase 13 changes.

### Phase 12 - Skills (Treat As Complete)

Implemented:

- `internal/skills` with loader, watcher, embed skills.
- `SkillTool`.
- `/skills list` and `/skills show` in the TUI.

Phase 13 implications:

- Phase 13 enhances `/skills list` to show the source origin label from the skills loader.
- Config file can specify a custom project skills directory under a `[skills]` section; this is optional and deferred if it complicates Phase 13 scope.
- No new skill commands are added in Phase 13; Phase 12 already covers `/skills list` and `/skills show`.

## Documentation and Log Findings

`docs/PHASE-LOG.md` records Phase 13 as "Slash commands and config UX." The Phase 7 log explicitly defers full command framework to Phase 13. The Phase 9 log defers `/hooks` command UX to Phase 13. The Phase 8 log defers `/memory` command UX to Phase 13.

`book/ch13-terminal-ui.md` describes the full command surface, the config hierarchy, and the `--print` flag. Key lessons:

- Commands should be organized into namespaced groups (`/memory *`, `/hooks *`, `/permissions *`).
- Config provenance must be visible to users through `/config show` or source-tagged rule display.
- `--print` is a professional debugging and scripting entry point, not a replacement for the REPL.
- The config hierarchy is explicitly ordered and each layer should be able to override the layer below.

## Evaluation of the Original Phase 13 Plan

The original Phase 13 sketch in `.codex/go-ollama-plan-AGENTS.md` was correct in listing the command surface and config hierarchy. It needs more precision:

- It did not specify which koanf providers to use or how to structure the Config struct.
- It did not define the `--json` output format for `--print` mode.
- It did not specify how `/hooks reload` confirmation works in the TUI.
- It did not specify how `/memory edit` pauses the REPL for `$EDITOR`.
- It did not define source labels for config-loaded permission rules.
- It did not specify how session-level rules from `/permissions allow` persist across turns.
- It did not define the `nandocodego init` subcommand behavior.

This plan addresses all of these gaps.

## Final Phase 13 Scope

In scope:

- `internal/config` package: koanf-based loader, Config struct, source tagging, defaults.
- `nandocodego init` Cobra subcommand.
- `internal/cli/repl.go` updated to load config before bootstrap.
- `internal/cli/print.go`: `--print` mode with `--json` flag.
- `internal/commands` package: command handlers for all 20 slash commands.
- Replacement of Phase 7 slash dispatch with a full `CommandRegistry`.
- TUI `Model` extended with references needed by command handlers.
- `/help` enhanced with full command list.
- `/model` enhanced with Ollama validation.
- All acceptance criteria in this document.

Out of scope:

- Full config editor or TUI config browser.
- Config file hot reload (project config changes require REPL restart).
- TOML schema validation beyond koanf defaults.
- Advanced Vim visual mode.
- Persistent session files (Phase 14).
- Background task supervisor commands (Phase 14).
- Telemetry or metrics commands (Phase 16).
- Remote config or enterprise policy server.
- OAuth-flow MCP server onboarding UX (Phase 10 refinement).

## Config System Design

### Hierarchy (lowest to highest priority)

1. System defaults (`config.DefaultConfig()` in Go code).
2. User config (`~/.nandocodego/config.toml`).
3. Project config (`<working-dir>/.nandocodego/config.toml`).
4. CLI flags (e.g., `--model`, `--ollama-url`).
5. Session overrides (e.g., `/model <name>`, `/permissions allow`).

Later layers override earlier layers for the same field.

### Config Struct

```go
// Config is the merged, source-tagged configuration for a session.
// Field sources are tracked separately in ConfigSources.
type Config struct {
    DefaultModel    string
    OllamaBaseURL   string
    PermissionMode  permissions.Mode
    MaxTurns        int
    BashTimeout     time.Duration
    MaxReadChars    int
    MaxResultChars  int
    LogLevel        string
    LogFormat       string
    MemoryEnabled   bool
    SkillsDir       string // optional custom user skills directory
    ProjectSkillsDir string // optional custom project skills directory
    MCPServers      []MCPServerConfig
}

// MCPServerConfig describes one MCP server from config.
type MCPServerConfig struct {
    Name     string
    Command  string
    Args     []string
    Env      map[string]string
}

// ConfigSources records which layer provided each field.
type ConfigSources struct {
    DefaultModel    string // "default", "user", "project", "flag", "session"
    OllamaBaseURL   string
    PermissionMode  string
    MaxTurns        string
    // ... one string per Config field
}

// LoadResult holds the merged Config and its source map.
type LoadResult struct {
    Config  Config
    Sources ConfigSources
}
```

### koanf Setup

Use `github.com/knadh/koanf/v2` with:

- `koanf/providers/confmap` for system defaults.
- `koanf/providers/file` with `koanf/parsers/toml` for user and project config files.
- `koanf/providers/env` or manual CLI flag merging for flag overrides.

The TOML parser to use: `github.com/knadh/koanf/parsers/toml` which wraps `github.com/BurntSushi/toml`. Both `knadh/koanf/v2` and `knadh/koanf/parsers/toml` must be added to `tools/allowed-deps.txt`.

### Loading Flow in `internal/config/loader.go`

```go
func Load(userConfigPath, projConfigPath string, flags FlagOverrides) (LoadResult, error)
```

Steps:

1. Load system defaults into koanf.
2. Load user config from `userConfigPath` if the file exists; log WARN if it exists but fails to parse; never crash.
3. Load project config from `projConfigPath` if the file exists; same error policy.
4. Apply CLI flag overrides via `koanf.Set`.
5. Unmarshal merged koanf into `Config` struct.
6. Build `ConfigSources` by checking which layer last set each field.
7. Validate: `PermissionMode.Normalize()`, `MaxTurns >= 1`, `BashTimeout >= 1s`.
8. Log unknown config keys at WARN but do not crash.
9. Never log raw config values; log only field names and source labels.
10. Return `LoadResult`.

### Default Config File Template

`nandocodego init` writes this file if `~/.nandocodego/config.toml` does not already exist:

```toml
# nandocodego configuration
# Generated by `nandocodego init`
# See `nandocodego --help` for flag descriptions.

# default_model = "qwen3"
# ollama_base_url = "http://localhost:11434"
# permission_mode = "default"
# max_turns = 32
# bash_timeout = "30s"
# max_read_chars = 200000
# max_result_chars = 100000
# log_level = "info"
# log_format = "text"
# memory_enabled = true

# [[mcp_servers]]
# name = "my-server"
# command = "/usr/local/bin/my-mcp-server"
# args = []
# [mcp_servers.env]
# MY_VAR = "value"
```

Every default line is commented out so the file documents options without silently overriding them.

## Slash Command Architecture

### CommandRegistry

Replace the current `HandleSlashCommand(cmd, args string) (TranscriptItem, bool)` approach with a `CommandRegistry`:

```go
// Handler is the function signature for a slash command.
// ctx is the Bubble Tea context-like dispatch bundle.
// args are the space-split tokens after the command name.
// Returns one or more TranscriptItems to append, a quit flag, and any Bubble Tea command to run.
type Handler func(ctx HandlerContext, args []string) ([]tui.TranscriptItem, bool, tea.Cmd)

// HandlerContext provides all references a slash command handler needs.
type HandlerContext struct {
    Store       *state.Store[state.App]
    Bootstrap   *bootstrap.State
    LLMClient   llm.Client
    MemoryDir   string
    HooksRef    *hooks.SnapshotRef  // mutable, supports /hooks reload
    SkillLoader *skills.Loader
    Renderer    *tui.MarkdownRenderer
    Ctx         context.Context
}

// CommandRegistry holds all registered slash command handlers.
type CommandRegistry struct {
    handlers map[string]Handler
    aliases  map[string]string
}

func (r *CommandRegistry) Register(name string, h Handler)
func (r *CommandRegistry) Alias(alias, target string)
func (r *CommandRegistry) Dispatch(cmd string, args []string, ctx HandlerContext) ([]tui.TranscriptItem, bool, tea.Cmd)
func (r *CommandRegistry) Descriptions() []CommandDescription
```

### Command Table

| Command | Package | Description |
|---------|---------|-------------|
| `/help` | `commands/help.go` | List all commands with descriptions |
| `/clear` | `commands/session.go` | Clear transcript and message history |
| `/exit` | `commands/session.go` | Exit the REPL |
| `/model [name]` | `commands/model.go` | Show current model or switch with Ollama validation |
| `/models` | `commands/model.go` | List available Ollama models with sizes |
| `/pull <model>` | `commands/model.go` | Pull a model with a progress transcript item |
| `/memory list` | `commands/memory.go` | List active memory files with type and date |
| `/memory show <name>` | `commands/memory.go` | Display full memory file content |
| `/memory edit <name>` | `commands/memory.go` | Open memory file in $EDITOR |
| `/memory promote <name>` | `commands/memory.go` | Move pending draft to active memory dir |
| `/hooks list` | `commands/hooks.go` | Show active hooks grouped by event kind |
| `/hooks reload` | `commands/hooks.go` | Reload hooks.json with confirmation |
| `/permissions show` | `commands/permissions.go` | Display current mode and source-tagged rules |
| `/permissions allow <pattern>` | `commands/permissions.go` | Add session-level allow rule |
| `/permissions deny <pattern>` | `commands/permissions.go` | Add session-level deny rule |
| `/skills list` | `commands/skills.go` | List available skills with source (delegates to Phase 12 impl) |
| `/skills show <name>` | `commands/skills.go` | Show full skill content (delegates to Phase 12 impl) |
| `/cost` | `commands/cost.go` | Show token usage for the current session |
| `/init` | `commands/init.go` | Create default config.toml if not present |
| `/agents list` | `commands/agents.go` | List sub-agents spawned this session |

### Handler Context Plumbing

The TUI `Model` must hold:

```go
type Model struct {
    // ... existing fields ...
    cmdRegistry *commands.CommandRegistry
    cmdCtx      commands.HandlerContext
}
```

`cmdCtx` is built once at TUI construction and holds all the references command handlers need. It does not hold a `tea.Program` reference; handlers return `tea.Cmd` values instead of sending directly.

## --print Mode Design

### Flag

Add `--print` as a string flag on the root Cobra command:

```
nandocodego --print "describe the repo in one sentence"
nandocodego --print "list the main packages" --json
nandocodego --print "what is 2+2?" --model qwen3
```

### Flow

`runPrint(ctx, printInput, jsonMode, opts)` in `internal/cli/print.go`:

1. Load config (same as REPL).
2. Build bootstrap initial state from config + flags.
3. Construct Ollama client.
4. Build tool registry.
5. Build `agent.Input` with `printInput` as the sole user message.
6. Call `agent.Run(ctx, input)`.
7. Drain events:
   - Accumulate `AssistantTextDelta` into a string buffer.
   - Track tool uses in a slice.
   - Capture usage from `agent.Terminal`.
   - On `agent.Terminal` with non-completed reason: print an error to stderr, exit non-zero.
8. In JSON mode: print `{"content": "...", "tool_uses": [...], "usage": {...}}` to stdout.
9. In text mode: print only the accumulated assistant content to stdout.
10. Exit 0 on success.

### JSON Output Schema

```json
{
  "content": "The main packages are...",
  "tool_uses": [
    {"name": "Bash", "input": {"command": "ls"}, "output": "..."}
  ],
  "usage": {
    "prompt_eval_count": 120,
    "eval_count": 45,
    "total_duration_ms": 1234
  }
}
```

### Exit Codes

| Condition | Exit code |
|-----------|-----------|
| Success | 0 |
| Agent completed with `aborted` reason | 1 |
| Agent completed with `unrecoverable` reason | 2 |
| Config load error | 3 |
| Ollama unreachable | 4 |

## Architecture

### Package Layout

```text
internal/config/
  config.go        — Config struct, MCPServerConfig, ConfigSources, LoadResult
  loader.go        — Load(userPath, projPath string, flags FlagOverrides) (LoadResult, error)
  defaults.go      — DefaultConfig(), default TOML template string
  loader_test.go   — hierarchy, override, unknown keys, never-crash

internal/commands/
  registry.go      — CommandRegistry, Handler, HandlerContext
  help.go          — /help
  session.go       — /clear, /exit
  model.go         — /model, /models, /pull
  memory.go        — /memory list, show, edit, promote
  hooks.go         — /hooks list, reload
  permissions.go   — /permissions show, allow, deny
  skills.go        — /skills list, show (thin wrappers around Phase 12)
  cost.go          — /cost
  init.go          — /init
  agents.go        — /agents list
  registry_test.go — dispatch, unknown command, alias
  model_test.go    — /model validation, /models output
  memory_test.go   — list, show, promote
  permissions_test.go — show, allow, deny
  hooks_test.go    — list, reload confirmation

internal/cli/
  print.go         — runPrint, --print flag, --json flag, exit codes
  print_test.go    — success, failure, json mode
  init.go          — nandocodego init subcommand
  repl.go          — updated to load config before bootstrap
```

### Config Struct in Detail

```go
package config

import (
    "time"
    "github.com/FernasFragas/nandocodego/internal/permissions"
)

type Config struct {
    DefaultModel     string            `koanf:"default_model"`
    OllamaBaseURL    string            `koanf:"ollama_base_url"`
    PermissionMode   permissions.Mode  `koanf:"permission_mode"`
    MaxTurns         int               `koanf:"max_turns"`
    BashTimeout      time.Duration     `koanf:"bash_timeout"`
    MaxReadChars     int               `koanf:"max_read_chars"`
    MaxResultChars   int               `koanf:"max_result_chars"`
    LogLevel         string            `koanf:"log_level"`
    LogFormat        string            `koanf:"log_format"`
    MemoryEnabled    bool              `koanf:"memory_enabled"`
    SkillsDir        string            `koanf:"skills_dir"`
    ProjectSkillsDir string            `koanf:"project_skills_dir"`
    MCPServers       []MCPServerConfig `koanf:"mcp_servers"`
}

type MCPServerConfig struct {
    Name    string            `koanf:"name"`
    Command string            `koanf:"command"`
    Args    []string          `koanf:"args"`
    Env     map[string]string `koanf:"env"`
}
```

### FlagOverrides

```go
// FlagOverrides holds values explicitly set via CLI flags.
// Zero values mean "not set"; use pointers for optional fields.
type FlagOverrides struct {
    Model      *string
    OllamaURL  *string
    LogLevel   *string
    LogFormat  *string
    Print      *string
    JSONOutput bool
}
```

### SnapshotRef (for /hooks reload)

The hooks snapshot must be held in a mutable container so `/hooks reload` can replace it:

```go
// SnapshotRef holds the active hooks snapshot in a thread-safe container.
type SnapshotRef struct {
    mu  sync.RWMutex
    ss  hooks.Snapshot
}

func (r *SnapshotRef) Get() hooks.Snapshot
func (r *SnapshotRef) Set(ss hooks.Snapshot)
```

`SnapshotRef` is defined in `internal/hooks/snapshot.go` or a new `internal/hooks/ref.go`. The runner must read the snapshot from `SnapshotRef.Get()` on every dispatch rather than holding a snapshot copy.

## Implementation Plan

### Step 1 - Config Package: Defaults and Config Struct

Files:

- `internal/config/config.go`
- `internal/config/defaults.go`

Implement:

- `Config` struct with `koanf` struct tags.
- `MCPServerConfig` struct.
- `ConfigSources` struct mirroring all Config field names as strings.
- `LoadResult` struct with `Config` and `Sources`.
- `DefaultConfig() Config` returning hardcoded safe defaults.
- `DefaultConfigTOML() string` returning the commented TOML template for `nandocodego init`.

Tests:

- `DefaultConfig()` returns non-zero values for all required fields.
- `DefaultConfigTOML()` contains all field names.

### Step 2 - Config Package: Loader

Files:

- `internal/config/loader.go`
- `internal/config/loader_test.go`

Add `github.com/knadh/koanf/v2` and `github.com/knadh/koanf/parsers/toml` to `go.mod` and `tools/allowed-deps.txt`.

Implement `Load(userConfigPath, projConfigPath string, flags FlagOverrides) (LoadResult, error)`:

1. Initialize koanf with confmap defaults from `DefaultConfig()`.
2. Load user config file if it exists; WARN on parse error, continue.
3. Load project config file if it exists; WARN on parse error, continue.
4. Apply `FlagOverrides` non-nil fields via `k.Set`.
5. Unmarshal into `Config`.
6. Build `ConfigSources` by checking which layer last set each field.
7. Validate: normalize `PermissionMode`, clamp `MaxTurns` to `>= 1`, clamp `BashTimeout` to `>= 1s`.
8. Log unknown top-level keys at WARN.
9. Return `LoadResult, nil`.

Tests:

- System defaults are used when no files exist.
- User config overrides system defaults for set fields.
- Project config overrides user config for set fields.
- CLI flag overrides project config.
- Missing config file is not an error.
- Config file with parse error logs WARN but returns default values.
- Config file with unknown keys logs WARN but does not crash.
- `ConfigSources.DefaultModel` is `"default"` when only system defaults apply.
- `ConfigSources.DefaultModel` is `"user"` when set in user config.
- `ConfigSources.DefaultModel` is `"project"` when set in project config.
- `ConfigSources.DefaultModel` is `"flag"` when set via `FlagOverrides`.
- Sensitive values (hypothetical password field) are never logged.

### Step 3 - Wire Config Into REPL Startup

Files:

- `internal/cli/repl.go`

Changes:

1. At the start of `runREPL`, call `config.Load(userConfigPath, projConfigPath, flags)`.
2. Build `bootstrap.Initial` from the merged config and flag overrides.
3. Use `bootstrap.Initial.DefaultModel` and all other fields to initialize the session.
4. Pass config-loaded values into the agent, memory runner, and hooks runner.
5. Config-loaded Ollama URL takes precedence over the compile-time default but is overridden by `--ollama-url` flag.

### Step 4 - nandocodego init Subcommand

Files:

- `internal/cli/init.go`
- `internal/cli/root.go` (register `init` subcommand)

Implement `runInit(ctx, cmd, args)`:

1. Compute target path: `paths.ConfigDir() + "/config.toml"`.
2. If the file exists, print "Config already exists at <path>" and exit 0.
3. Ensure the directory exists with `os.MkdirAll`.
4. Write `config.DefaultConfigTOML()` to the file.
5. Print "Created config at <path>".
6. Exit 0.

Tests:

- Running `init` creates the file.
- Running `init` twice does not overwrite the existing file.
- Output includes the file path.

### Step 5 - --print Mode

Files:

- `internal/cli/print.go`
- `internal/cli/print_test.go`
- `internal/cli/root.go` (add `--print` and `--json` flags)

Implement `runPrint(ctx context.Context, input string, jsonOutput bool, opts printOptions) error`:

1. Load config.
2. Build agent input with `input` as the sole user message.
3. Drain `agent.Run` event channel:
   - Accumulate `AssistantTextDelta.Text` into `contentBuf`.
   - Collect `ToolUseStart` and `ToolUseResult` into `toolUses`.
   - On `Terminal`: capture usage; check reason; set exit code.
4. If `jsonOutput`:
   - Marshal `printOutput{Content, ToolUses, Usage}` to JSON.
   - Write to `cmd.OutOrStdout()`.
5. Else:
   - Write `contentBuf.String()` to `cmd.OutOrStdout()`.
6. Return error based on terminal reason and exit code.

Tests (use fake agent runner):

- Text mode prints assistant content to stdout.
- JSON mode prints valid JSON with content, tool_uses, usage fields.
- Aborted terminal exits with code 1.
- Unrecoverable terminal exits with code 2.
- Tool uses are captured in JSON mode.
- Empty input is rejected before calling the agent.

### Step 6 - CommandRegistry

Files:

- `internal/commands/registry.go`
- `internal/commands/registry_test.go`

Implement:

- `Handler` function type.
- `HandlerContext` struct with all required references.
- `CommandRegistry` struct.
- `Register(name string, h Handler)`.
- `Alias(alias, target string)`.
- `Dispatch(cmd string, args []string, ctx HandlerContext) ([]TranscriptItem, bool, tea.Cmd)`.
- `Descriptions() []CommandDescription` for `/help` listing.
- Return a system transcript item for unknown commands that lists available commands.

Tests:

- Registered command dispatches correctly.
- Unknown command returns a system item with error.
- Alias resolves to target.
- Duplicate registration panics (programming error, not a user error).

### Step 7 - /help Handler

Files:

- `internal/commands/help.go`

Implement:

- Lists all registered commands sorted alphabetically by name.
- Each entry shows: `  /name    description`.
- Groups commands by prefix for readability (`/memory *`, `/permissions *`, etc.).
- Returns one system transcript item.

### Step 8 - /clear and /exit Handlers

Files:

- `internal/commands/session.go`

Move from `internal/tui/slash.go`. Behavior unchanged:

- `/clear`: clear `state.App.Messages` and the transcript. Return transcript item.
- `/exit`: return quit=true.

### Step 9 - /model, /models, /pull Handlers

Files:

- `internal/commands/model.go`
- `internal/commands/model_test.go`

Implement:

`/model [name]`:

- If no name given: show `state.App.ActiveModel` and its source from `ConfigSources`.
- If name given: call `llm.Client.ListModels()` asynchronously via a `tea.Cmd`.
  - If the model is in the list: call `store.Set` to update `ActiveModel`; return success item.
  - If not in the list: return error item suggesting `/pull <name>` to download it.
  - If `ListModels` fails: return error item with the reason.

`/models`:

- Call `llm.Client.ListModels()` via `tea.Cmd`.
- Return transcript item with a formatted table: `Name   Size   Modified`.
- Sort by name alphabetically.
- If Ollama is unreachable: return error item.

`/pull <model>`:

- Call `llm.Client.PullModel(name)` via `tea.Cmd`.
- Stream progress events back as transcript item updates (or a single summary item on completion).
- On success: return success item.
- On failure: return error item.

Tests (using fake `llm.Client`):

- `/model qwen3` with `qwen3` in the model list updates `ActiveModel`.
- `/model unknown` with `unknown` not in the list returns error item suggesting `/pull`.
- `/model` with no args returns current model name.
- `/models` returns a formatted table.
- `/pull qwen3` success returns a success item.

### Step 10 - /memory Handlers

Files:

- `internal/commands/memory.go`
- `internal/commands/memory_test.go`

Implement:

`/memory list`:

- Call `memory.Scan(ctx, memoryDir)` to list active memory files.
- For each entry: show filename, type, name, and modification date.
- If no memory files: show a message.
- Exclude `pending/` entries.

`/memory show <name>`:

- Resolve `<name>` to a file path under `memoryDir`.
- Validate the path is inside `memoryDir`.
- Read the file and render with `MarkdownRenderer`.
- If not found: return error item.

`/memory edit <name>`:

- Resolve and validate the file path as above.
- Read `$EDITOR` environment variable; fall back to `vi`.
- Return a `tea.Cmd` that:
  1. Restores the terminal (exits alt-screen if active).
  2. Runs `exec.Command($EDITOR, filePath)` with stdin/stdout/stderr connected.
  3. Waits for the editor to exit.
  4. Re-enters alt-screen if it was active.
  5. Sends a system transcript item confirming the file was edited.

`/memory promote <name>`:

- Resolve `<pending-dir>/<name>` to a file path.
- Validate it is inside `pending/`.
- Move to `memoryDir/<name>`.
- Return success item.
- If the file does not exist: return error item.

Tests (use temp directory with real files):

- `/memory list` with no files returns empty message.
- `/memory list` with two active files lists them.
- `/memory show` with a valid name renders the file body.
- `/memory show` with an unknown name returns error.
- `/memory promote` moves the file from pending to active directory.
- `/memory promote` with unknown name returns error.
- Path traversal (`/memory show ../../etc/passwd`) is rejected.

### Step 11 - /hooks Handlers

Files:

- `internal/commands/hooks.go`
- `internal/commands/hooks_test.go`

Implement `SnapshotRef` in `internal/hooks/ref.go` (if not already present):

- `SnapshotRef` with mutex-guarded `hooks.Snapshot`.
- `Get() hooks.Snapshot`, `Set(ss hooks.Snapshot)`.
- The runner reads from `ref.Get()` on every dispatch.

`/hooks list`:

- Call `ref.Get()` to get the current snapshot.
- Group hooks by event kind.
- For each hook: show kind, matchers, enabled/disabled status.
- Show a note that project hooks are disabled by default.

`/hooks reload`:

- Display: "This will replace the active hook snapshot. Type `yes` to confirm."
- If the user types `yes` (via a confirmation tea.Cmd that waits for a line of input): reload `hooks.json` from the user config dir, build a new snapshot, call `ref.Set(newSnapshot)`.
- Return success or failure transcript item.
- If the user types anything other than `yes`: return a cancellation item.

Tests:

- `/hooks list` with no hooks returns empty message.
- `/hooks list` with hooks shows them grouped by event kind.
- `/hooks reload` with confirmation reloads and updates the snapshot.
- `/hooks reload` without confirmation is cancelled.

### Step 12 - /permissions Handlers

Files:

- `internal/commands/permissions.go`
- `internal/commands/permissions_test.go`

`/permissions show`:

- Read `bootstrap.Snapshot().PermissionMode` and its source.
- Read `state.App.PermissionRules`.
- For each rule: show pattern, decision, and source label.
- Source labels map to: `policy`, `user`, `project`, `local`, `cli`, `session`.

`/permissions allow <pattern>`:

- Parse `<pattern>` as a permission pattern (validate via `permissions.ParsePattern`).
- If invalid: return error item.
- Add rule `{Pattern: pattern, Decision: DecisionAllow, Source: SourceSession}` to `state.App.PermissionRules`.
- Return success item showing what was added.

`/permissions deny <pattern>`:

- Same as `/permissions allow` but with `DecisionDeny`.

Tests:

- `/permissions show` lists mode and all rules with source labels.
- `/permissions allow "Bash(ls *)"` adds a session allow rule.
- `/permissions deny "Bash(rm *)"` adds a session deny rule.
- Invalid pattern returns error item.
- Session rules appear in `/permissions show` after being added.
- Session rules survive turn boundaries (stored in `state.App`, not in `agent.Input`).

### Step 13 - /skills Handlers (thin wrappers)

Files:

- `internal/commands/skills.go`

Phase 12 already implements the core skill logic. Phase 13 adds these as thin wrappers in the command registry:

- `/skills list` → delegates to Phase 12 slash handler or reimplements using `loader.List()`.
- `/skills show <name>` → delegates to Phase 12 or reimplements using `loader.ReadBody`.

These can be copied from Phase 12 slash.go if Phase 12 implemented them inline, or directly call `commands.SkillListHandler` and `commands.SkillShowHandler`.

### Step 14 - /cost Handler

Files:

- `internal/commands/cost.go`

`/cost`:

- Read `state.App.Usage` (accumulated from all `agent.Terminal` events in the session).
- Display:
  - Prompt tokens: `N`
  - Generated tokens: `N`
  - Total tokens: `N`
  - Turns: `N`
  - Tool calls: `N`
- Return one system transcript item.

### Step 15 - /init Handler (for in-session use)

Files:

- `internal/commands/init.go`

`/init`:

- Same logic as `nandocodego init` subcommand but callable from inside the REPL.
- If config already exists: return info item.
- If not: write the template, return success item.

### Step 16 - /agents Handler

Files:

- `internal/commands/agents.go`

`/agents list`:

- Read sub-agent records from `state.App` (Phase 11 should have added this field).
- If no sub-agents spawned: return info item.
- For each sub-agent: show name, status (running / completed), brief description.

### Step 17 - Replace Phase 7 Slash Dispatch in TUI

Files:

- `internal/tui/app.go`
- `internal/tui/slash.go` (keep for Phase 7 test compatibility or migrate tests)

Changes:

1. Construct `commands.NewRegistry()` with all handlers registered.
2. Build `commands.HandlerContext` at TUI model construction time.
3. In `handleKeyMsg`, route slash command input to `registry.Dispatch(cmd, args, cmdCtx)` instead of the Phase 7 `HandleSlashCommand` function.
4. Append returned `TranscriptItems` to the transcript.
5. If `tea.Cmd` returned, return it from `Update`.
6. If quit flag, call `tea.Quit`.
7. Retain the Phase 7 tests or migrate them to use the new registry dispatch.

### Step 18 - Tests, Checks, and Manual Smoke

Required test commands:

```sh
go test ./internal/config/...
go test ./internal/commands/...
go test ./internal/cli/...
go test ./internal/tui/...
go test ./...
go test -race ./internal/config/...
go test -race ./internal/commands/...
tools/check-allowed-deps.sh
tools/check-network-policy.sh
```

Manual smoke (Ollama running):

```sh
go run ./cmd/nandocodego init
cat ~/.nandocodego/config.toml
go run ./cmd/nandocodego --print "what is the Go module name for this project?" --no-alt-screen
go run ./cmd/nandocodego --model qwen3 --no-alt-screen
# Inside REPL:
# /models
# /model qwen3
# /memory list
# /permissions show
# /permissions allow "Bash(ls *)"
# /permissions show
# /hooks list
# /cost
# /agents list
# /help
```

## Acceptance Criteria

- [ ] `nandocodego --print "hello"` exits 0 and writes the assistant response to stdout.
- [ ] `nandocodego --print "hello" --json` exits 0 and writes valid JSON with `content`, `tool_uses`, and `usage` fields.
- [ ] `nandocodego --print` with a failing agent exits with code 1 or 2 (not 0).
- [ ] `nandocodego init` creates `~/.nandocodego/config.toml` if not present.
- [ ] `nandocodego init` does not overwrite an existing config file.
- [ ] Config loading follows the hierarchy: system defaults < user config < project config < CLI flags.
- [ ] User config file with parse error logs WARN but does not crash.
- [ ] User config file with unknown keys logs WARN but does not crash.
- [ ] Config values are never logged at INFO.
- [ ] `ConfigSources` shows correct source label for each field.
- [ ] All 20 slash commands are registered and reachable via `/command`.
- [ ] `/help` lists all 20 commands with descriptions.
- [ ] `/models` lists models sorted by name with sizes.
- [ ] `/model <name>` validates against `llm.Client.ListModels()` before accepting.
- [ ] `/model <name>` with a name not in the model list returns an error suggesting `/pull`.
- [ ] `/memory list` lists active memory files with type and modification date.
- [ ] `/memory show <name>` renders the full memory file body.
- [ ] `/memory edit <name>` opens `$EDITOR` (or `vi`) and waits for it to exit.
- [ ] `/memory promote <name>` moves a file from `pending/` to the active memory directory.
- [ ] `/memory promote` with path traversal is rejected.
- [ ] `/hooks list` shows active hooks grouped by event kind.
- [ ] `/hooks reload` requires `yes` confirmation before replacing the snapshot.
- [ ] `/hooks reload` without confirmation is safely cancelled.
- [ ] `/permissions show` displays the current mode and all source-tagged rules.
- [ ] `/permissions allow "Bash(ls *)"` adds a session rule visible in `/permissions show`.
- [ ] `/permissions deny "Bash(rm *)"` adds a session deny rule.
- [ ] Session-level permission rules survive turn boundaries.
- [ ] Invalid permission pattern in `/permissions allow` returns an error item.
- [ ] `/cost` shows token usage for the current session.
- [ ] `/init` from within the REPL creates `config.toml` if not present.
- [ ] `/agents list` shows spawned sub-agents (or "no agents" if none).
- [ ] `/skills list` and `/skills show` continue to work (delegates to Phase 12).
- [ ] All 20 commands appear in `/help` output.
- [ ] Unknown slash command returns a helpful error listing available commands.
- [ ] `go test -race ./internal/config/...` passes.
- [ ] `go test -race ./internal/commands/...` passes.
- [ ] `go test ./...` passes.
- [ ] `tools/check-allowed-deps.sh` passes (koanf and toml parser added to allowlist).
- [ ] `tools/check-network-policy.sh` passes.
- [ ] `docs/PHASE-LOG.md` has a Phase 13 entry.

## Todos

### Config Package

- [ ] Create `internal/config/config.go` with `Config`, `MCPServerConfig`, `ConfigSources`, `LoadResult`.
- [ ] Add `koanf` struct tags to every `Config` field.
- [ ] Create `internal/config/defaults.go` with `DefaultConfig() Config`.
- [ ] Implement `DefaultConfigTOML() string` with fully commented TOML template.
- [ ] Add `github.com/knadh/koanf/v2` to `go.mod`.
- [ ] Add `github.com/knadh/koanf/parsers/toml` to `go.mod`.
- [ ] Add both packages to `tools/allowed-deps.txt` with justification.
- [ ] Run `go mod tidy`.
- [ ] Create `internal/config/loader.go` with `Load(...)`.
- [ ] Implement confmap defaults loading step.
- [ ] Implement user config TOML file loading step.
- [ ] Implement project config TOML file loading step.
- [ ] Implement `FlagOverrides` merging step.
- [ ] Implement `Config` unmarshaling.
- [ ] Implement `ConfigSources` population.
- [ ] Implement validation: `PermissionMode.Normalize()`, `MaxTurns >= 1`, `BashTimeout >= 1s`.
- [ ] Log unknown top-level keys at WARN (do not crash).
- [ ] Never log raw config field values.
- [ ] Write loader tests for all hierarchy combinations.
- [ ] Write loader test for parse error → WARN, continues with defaults.
- [ ] Write loader test for unknown keys → WARN, no crash.
- [ ] Write loader test for source label correctness.

### nandocodego init Subcommand

- [ ] Create `internal/cli/init.go` with `runInit(ctx, cmd, args)`.
- [ ] Implement "exists → skip" logic.
- [ ] Implement `MkdirAll` for config dir.
- [ ] Write `DefaultConfigTOML()` content to the file.
- [ ] Print the file path after creation.
- [ ] Register `init` subcommand in `internal/cli/root.go`.
- [ ] Write tests: creates file, skip on existing, prints path.

### --print Mode

- [ ] Add `--print` string flag to root Cobra command.
- [ ] Add `--json` bool flag to root Cobra command.
- [ ] In `RunE`, if `--print` is set, call `runPrint` instead of `runREPL`.
- [ ] Create `internal/cli/print.go` with `runPrint(...)`.
- [ ] Implement event draining loop accumulating `AssistantTextDelta`.
- [ ] Track `ToolUseStart`/`ToolUseResult` pairs for JSON mode.
- [ ] Capture usage from `agent.Terminal`.
- [ ] Implement text output mode.
- [ ] Implement JSON output mode with `printOutput` struct.
- [ ] Implement exit codes for each terminal reason.
- [ ] Write `print_test.go` with fake agent runner.
- [ ] Test text mode output.
- [ ] Test JSON mode output structure.
- [ ] Test aborted agent exit code 1.
- [ ] Test unrecoverable agent exit code 2.
- [ ] Test empty input rejection.

### CommandRegistry

- [ ] Create `internal/commands/registry.go`.
- [ ] Define `Handler` function type.
- [ ] Define `HandlerContext` struct with all fields.
- [ ] Define `CommandDescription` struct (name, description).
- [ ] Implement `CommandRegistry` with `handlers` and `aliases` maps.
- [ ] Implement `Register(name, h Handler)`.
- [ ] Implement `Alias(alias, target string)`.
- [ ] Implement `Dispatch(cmd, args, ctx)`.
- [ ] Implement `Descriptions() []CommandDescription` sorted by name.
- [ ] Return system error item for unknown command (listing suggestions).
- [ ] Panic on duplicate `Register` calls (programming error).
- [ ] Write `registry_test.go` for dispatch, unknown command, alias.

### /help Handler

- [ ] Create `internal/commands/help.go`.
- [ ] Implement handler that calls `registry.Descriptions()`.
- [ ] Group commands by prefix (`/memory *`, etc.).
- [ ] Return one system transcript item with the full list.

### /clear and /exit Handlers

- [ ] Create `internal/commands/session.go`.
- [ ] Migrate `/clear` from `internal/tui/slash.go`.
- [ ] Migrate `/exit` from `internal/tui/slash.go`.
- [ ] Ensure existing test coverage is preserved.

### /model, /models, /pull Handlers

- [ ] Create `internal/commands/model.go`.
- [ ] Implement `/model` with no args (show current model and source).
- [ ] Implement `/model <name>` as async tea.Cmd calling `ListModels`.
- [ ] Return success item on valid model.
- [ ] Return error item with `/pull` suggestion on unknown model.
- [ ] Implement `/models` as async tea.Cmd listing models sorted by name.
- [ ] Implement `/pull <model>` as async tea.Cmd calling `PullModel`.
- [ ] Stream pull progress as a series of transcript updates or a final summary.
- [ ] Write `model_test.go` with fake LLM client.

### /memory Handlers

- [ ] Create `internal/commands/memory.go`.
- [ ] Implement `/memory list` using `memory.Scan`.
- [ ] Implement `/memory show <name>` with path validation and Glamour rendering.
- [ ] Implement `/memory edit <name>` with `$EDITOR` and alt-screen management.
- [ ] Implement `/memory promote <name>` with path validation and file move.
- [ ] Reject path traversal in all memory commands.
- [ ] Write `memory_test.go` with temp directory.
- [ ] Test list, show, promote, and error cases.
- [ ] Test path traversal rejection.

### /hooks Handlers and SnapshotRef

- [ ] Create or update `internal/hooks/ref.go` with `SnapshotRef`.
- [ ] Implement `SnapshotRef.Get()` and `Set()` with mutex.
- [ ] Update the hooks runner to call `ref.Get()` per dispatch rather than holding a frozen copy.
- [ ] Create `internal/commands/hooks.go`.
- [ ] Implement `/hooks list` showing hooks grouped by event kind.
- [ ] Implement `/hooks reload` with confirmation flow.
- [ ] Write `hooks_test.go`.
- [ ] Test list with no hooks.
- [ ] Test list with hooks.
- [ ] Test reload with confirmation.
- [ ] Test reload without confirmation (cancellation).

### /permissions Handlers

- [ ] Create `internal/commands/permissions.go`.
- [ ] Implement `/permissions show` reading mode and rules.
- [ ] Map `permissions.Source` values to display labels.
- [ ] Implement `/permissions allow <pattern>`.
- [ ] Validate pattern with `permissions.ParsePattern` or equivalent.
- [ ] Add `{Source: SourceSession}` rule to `state.App.PermissionRules` via `store.Set`.
- [ ] Implement `/permissions deny <pattern>`.
- [ ] Write `permissions_test.go`.
- [ ] Test show output includes mode and all rules.
- [ ] Test allow adds a rule visible in subsequent show.
- [ ] Test deny adds a deny rule.
- [ ] Test invalid pattern returns error.
- [ ] Test session rules survive a turn (persist in `state.App`).

### /skills Handlers

- [ ] Create `internal/commands/skills.go` as thin wrappers.
- [ ] Register `/skills list` delegating to Phase 12 loader.
- [ ] Register `/skills show` delegating to Phase 12 loader and renderer.

### /cost, /init, /agents Handlers

- [ ] Create `internal/commands/cost.go` with `/cost` handler.
- [ ] Create `internal/commands/init.go` with `/init` handler (same logic as CLI subcommand).
- [ ] Create `internal/commands/agents.go` with `/agents list` handler.

### TUI Model Integration

- [ ] Add `cmdRegistry *commands.CommandRegistry` to `internal/tui.Model`.
- [ ] Add `cmdCtx commands.HandlerContext` to `internal/tui.Model`.
- [ ] Extend `tui.New(...)` to accept `HandlerContext` or individual components.
- [ ] In `handleKeyMsg`, route slash commands to `registry.Dispatch`.
- [ ] Append returned transcript items to the transcript.
- [ ] Handle returned `tea.Cmd`.
- [ ] Handle returned quit flag.
- [ ] Hold `*skills.Loader` on the model and include it in `HandlerContext`.
- [ ] Hold `*hooks.SnapshotRef` on the model and include it in `HandlerContext`.
- [ ] Ensure existing Phase 7 slash tests pass or are migrated.

### REPL Startup Wiring

- [ ] In `runREPL`, call `config.Load(...)` before `bootstrap.New(...)`.
- [ ] Feed `LoadResult.Config` into `bootstrap.Initial` fields.
- [ ] Build `HandlerContext` with all required references.
- [ ] Pass `HandlerContext` to `tui.New`.
- [ ] Update `tui.New` signature accordingly.
- [ ] Ensure `SnapshotRef` is constructed and passed to both the hooks runner and `HandlerContext`.

### Phase Log

- [ ] Append Phase 13 entry to `docs/PHASE-LOG.md`.
- [ ] Record objective, files created/updated, dependencies added (koanf, toml parser), checks run, manual smoke result, design decisions, known constraints, and exit gate status.

## Forbidden

- Logging raw config values, field values, or session overrides at INFO or higher.
- Overwriting an existing config file in `nandocodego init` or `/init`.
- Making `/hooks reload` execute without user confirmation.
- Session rules (`/permissions allow`) modifying the `bootstrap` snapshot directly; they belong only in `state.App.PermissionRules`.
- Running LLM calls, file I/O, or process spawning directly in Bubble Tea `Update`; all blocking operations use `tea.Cmd`.
- Adding slash commands that bypass the permission system.
- `/memory edit` that writes outside the memory directory.
- Config file loading that crashes on unknown keys.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---:|---|
| koanf adds transitive dependencies that violate the allowlist | Medium | Run `tools/check-allowed-deps.sh` after `go mod tidy`; add all transitive direct deps explicitly if needed. |
| `--print` mode output is non-deterministic (tool use ordering) | Low | Print assistant text only in text mode; JSON mode includes tool uses for debugging. |
| `/model` Ollama call hangs if Ollama is offline | Medium | Use a short timeout (5 seconds) for the validation ListModels call; return an error item on timeout. |
| `/memory edit` with $EDITOR that never exits leaks the process | Low | Set a generous timeout (10 minutes); document that `$EDITOR` must exit normally. |
| `/hooks reload` replaces snapshot while a hook is executing | Medium | `SnapshotRef.Set` uses a write lock; the runner holds a read lock during execution; Go's sync.RWMutex ensures safe serialization. |
| Session permission rules added by `/permissions allow` do not affect in-flight agent runs | Low | Rules in `state.App` are read at the start of each new turn's `agent.Input`; in-flight turns use the snapshot from when they started. Document this behavior. |
| Config hierarchy silently overrides user intent | Low | `ConfigSources` makes provenance explicit; `/permissions show` and `/model` show the source. |
| koanf TOML parser rejects valid TOML | Low | Add parser integration tests with real TOML files; test all Config field types. |
| Two commands with the same name panic (programming error) | Low | The registry panics on duplicate `Register` calls; this is caught at startup, not at user interaction time. |
| Large memory files crash `/memory show` renderer | Low | Set a reasonable render input size cap (64 KB) before calling Glamour. |

## Phase Log Template

When implementation finishes, append a Phase 13 entry to `docs/PHASE-LOG.md` with:

- objective;
- files created and updated;
- dependencies added (`koanf/v2`, `koanf/parsers/toml`) and allowlist status;
- tests run and results;
- manual smoke result (eight-step exit gate);
- design decisions (config hierarchy, SnapshotRef, HandlerContext, --print exit codes);
- known constraints and deferred work;
- exit gate status.

## Implementation Reconciliation (2026-05-08)

Phase 13 is now reconciled in code against the detailed plan, with only manual/live validation remaining.

Implemented in this closure pass:

- Strengthened config loader diagnostics and provenance behavior:
  - parse-error warnings without crashing;
  - unknown top-level key warnings without crashing;
  - source tracking for `skills_dir` and `project_skills_dir`.
- Exposed config warnings to runtime surfaces:
  - REPL startup notices include config warnings;
  - `--print` emits config warnings to stderr while continuing with safe defaults.
- Hardened `--print` behavior:
  - explicit terminal-reason-to-exit-code mapping (`aborted` => 1, `unrecoverable` => 2);
  - typed CLI exit error plumbing into root `ExitCode` resolution;
  - deterministic event-drain helpers and dedicated print-mode tests.
- Added test coverage for previously untested Phase 13 expectations:
  - config parse/unknown-key warning behavior and source labels;
  - print-mode text/json output contract and terminal-event requirements;
  - root exit-code handling for typed CLI exit errors.

Automated checks run successfully:

- `env GOCACHE=/private/tmp/go-build-cache go test ./internal/config/... ./internal/commands/... ./internal/cli/... ./internal/tui/...`
- `env GOCACHE=/private/tmp/go-build-cache go test -race ./internal/config/... ./internal/commands/...`
- `env GOCACHE=/private/tmp/go-build-cache go test ./...`
- `tools/check-allowed-deps.sh`
- `tools/check-network-policy.sh`

Remaining item (manual/live):

- Run the live Ollama exit-gate flow (`/models`, `/model <name>`, and `--print`) in a real interactive environment.

## Exit Gate

Phase 13 is complete only when:

- all acceptance criteria above are checked;
- `go test -race ./...` passes;
- `tools/check-allowed-deps.sh` and `tools/check-network-policy.sh` pass;
- `nandocodego init` creates a commented config file without overwriting an existing one;
- `nandocodego --print "hello"` exits 0 and writes assistant text to stdout;
- `nandocodego --print "hello" --json` outputs valid JSON;
- all 20 slash commands dispatch correctly in unit tests (no live Ollama needed);
- `/model <name>` validates against a live Ollama model list in the manual smoke;
- the phase log records the implementation and any deviations from this plan.
