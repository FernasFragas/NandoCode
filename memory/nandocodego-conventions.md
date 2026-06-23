---
name: nandocodego-conventions
description: Naming conventions, package layout rules, tooling choices, and ID format for nandocodego
type: reference
---

## Naming Conventions

| Artifact | Convention | Example |
|---|---|---|
| Go module | `github.com/<user>/nandocodego` | `github.com/FernasFragas/Nandocode` |
| Package dirs | lowercase, single word | `internal/agent`, `internal/tools` |
| Tool implementations | `internal/tools/<toolname>/<toolname>.go` | `internal/tools/bash/bash.go` |
| Tool type | `<Toolname>Tool` struct | `type BashTool struct` |
| Tool constructor | `New<Toolname>Tool() Tool` | `func NewBashTool(deps Deps) Tool` |
| Branded IDs | `type AgentID string` + `func NewAgentID() AgentID` | `internal/types/ids.go` |
| Discriminated unions | sealed interface + struct variants | `type TaskState interface{ isTaskState() }` |
| Constants | `SCREAMING_SNAKE_CASE` cross-pkg; `camelCase` pkg-private | `const StreamIdleTimeout` |
| Error vars | `var Err<Cause> = errors.New(...)` | `var ErrPromptTooLong` |
| Slash commands | `internal/commands/<kebab-name>.go` | `internal/commands/clear.go` |
| TUI components | `internal/tui/<component>.go` | `internal/tui/messages.go` |

## ID Format

Single-char prefix + 8 random alnum `[0-9a-z]` → ~2.8T combos:
- `a` = AgentID, `b` = bash task, `t` = task, `r` = remote, `m` = MCP monitor, `d` = dream

## Key Tooling Choices

| Concern | Choice |
|---|---|
| CLI | `github.com/spf13/cobra` |
| TUI | `github.com/charmbracelet/bubbletea` + `lipgloss` + `bubbles` |
| Markdown render | `github.com/charmbracelet/glamour` |
| Logging | `log/slog` stdlib (JSON prod, text dev) |
| Config | stdlib flag + os + TOML files + env overlay |
| HTTP retries | `github.com/cenkalti/backoff/v4` (per-error-class budgets) |
| Bash AST | `mvdan.cc/sh/v3/syntax` |
| MCP client | `github.com/modelcontextprotocol/go-sdk` |
| Ollama client | `github.com/ollama/ollama/api` |
| Git interop | `github.com/go-git/go-git/v5` |
| YAML frontmatter | `gopkg.in/yaml.v3` |
| Subprocess | `os/exec` with `context.Context` |
| Process supervision | `errgroup` or `sync.WaitGroup` + context |

## Package Source Layout

```
cmd/nandocodego/main.go          — entrypoint only; delegates to internal/cli
internal/agent/                  — Run() loop, events, compaction, sub-agents
internal/bootstrap/              — startup singleton state
internal/cli/                    — Cobra commands, REPL wiring
internal/commands/               — slash command handlers
internal/config/                 — TOML loading + env overlay
internal/hooks/                  — snapshot, runner, event constants
internal/llm/                    — Client interface, types, retry, watchdog
internal/llm/ollama/             — Ollama implementation
internal/memory/                 — scan, recall, extract, store
internal/mcp/                    — MCP client wrapper
internal/observability/          — metric/log decorators
internal/paths/                  — XDG path helpers
internal/permissions/            — modes, rules, resolution
internal/skills/                 — file-driven skill loader
internal/state/                  — reactive Store[T], App struct
internal/tasks/                  — supervisor, task kinds
internal/tools/                  — Tool interface, Registry, Context
internal/tools/bash/             — BashTool
internal/tools/builtin/          — NewRegistry() returning all built-in tools
internal/tui/                    — Bubble Tea root model + components
internal/version/                — Version, Commit vars (set via ldflags)
```
