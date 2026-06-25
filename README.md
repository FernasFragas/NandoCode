# nandocodego

A Go-language agentic coding CLI powered by local LLMs via [Ollama](https://ollama.com).

## What is nandocodego?

`nandocodego` is a local-first AI coding assistant that brings the power of large language models to your development workflow without sending your code to cloud services. It:

- **Runs locally** using Ollama-served models on your machine
- **Respects your privacy** - your code never leaves your computer
- **Provides rich terminal UI** with streaming responses, tool execution, and interactive permissions
- **Provides agentic foundations** with an agent loop, starter tools, state, and permission management
- **Provides advanced capabilities** including memory, hooks, MCP, sub-agents, skills, task supervision, HTTP/SSE server mode, and coordinator-mode multi-agent work

## Built With Agentic Engineering

This project was built 100% through an agentic engineering workflow: a human engineer directed the product vision, architecture, review, and release decisions while AI coding agents helped implement, test, document, and iterate inside the repository.

The point is not to hide the engineer behind the tools. It is to show what one engineer can ship by orchestrating agents with clear specifications, local-first tooling, review discipline, and steady technical judgment.

`nandocodego` is both an agentic coding tool and a proof point for agentic engineering as a serious software-development practice.

## Status

**Current Version:** v0.0.0-dev (Phase 29 complete; Phase 25 remote/bridge mode next)

This project is under active development. For current roadmap order, read [Next Phases Implementation Plan](docs/NEXT-PHASES-IMPLEMENTATION-PLAN.md) first, then [Project Status and Engineer Onboarding](docs/PROJECT-STATUS-AND-ONBOARDING.md) and [Phase Log](docs/PHASE-LOG.md). The original [.codex implementation plan](.codex/go-ollama-plan-AGENTS.md) is historical reference material, not the current launch-status source.

### Completed Phases

- ✅ **Phase 0:** Security baseline, CI/CD, dependency management
- ✅ **Phase 1:** Repository scaffolding, build system, `--version` and `doctor` commands
- ✅ **Phase 2:** Ollama LLM client with streaming, watchdog, retry logic, and capability detection
- ✅ **Phase 3:** Tool interface with Bash, FileRead, and FileWrite starter tools
- ✅ **Phase 4:** Model-driven agent loop
- ✅ **Phase 5:** Central permission system
- ✅ **Phase 6:** Bootstrap and reactive state layer
- ✅ **Phase 7:** Bubble Tea TUI and interactive REPL
- ✅ **Phases 8-16:** Memory, hooks, MCP, sub-agents, skills, commands/config UX, tasks, concurrency, and observability
- ✅ **Phases 19-22:** Tool ecosystem, content compaction, web/server mode, and enhanced TUI/input handling
- ✅ **Phase 24:** Multi-agent coordination
- ✅ **Phases 26-27:** Inline completion and directory mention expansion
- ✅ **Phases 28-29:** Semantic workspace indexing/retrieval and TUI index-progress observability
- ✅ **Ollama Cloud API key support:** complete
- ⏳ **Next:** Phase 25 Remote / Bridge Mode

## Prerequisites

- **Go version matching `go.mod`** - [Download](https://go.dev/dl/)
- **Ollama** running locally - [Install Ollama](https://ollama.com)
- A local model pulled (e.g., `ollama pull qwen3.6:35b`)

## Installation

### From Source

```bash
git clone https://github.com/FernasFragas/Nandocode.git
cd Nandocode
make build
sudo make install  # Optional: installs to /usr/local/bin
```

### Using Go Install

```bash
go install github.com/FernasFragas/Nandocode/cmd/nandocodego@latest
```

### Using Docker

```bash
# Build Docker image
make docker-build

# Run interactively
make docker-run ARGS="doctor"

# Or use docker-compose
docker-compose up
```

See [README.Docker.md](README.Docker.md) for detailed Docker usage instructions.

## Quick Start

```bash
# Verify installation
nandocodego --version
# Output: nandocodego 0.0.0-dev (9241798)

# Run system diagnostics
nandocodego doctor
# Outputs:
# - Version and build information
# - Go runtime version (OS, Arch, CPUs)
# - Configuration directory paths
# - Directory status and permissions
# - Environment variables (XDG_CONFIG_HOME, XDG_DATA_HOME, NANDOCODEGO_DEBUG)

# Start interactive REPL
nandocodego
```

### File and Directory Mentions

You can reference local files directly in prompts with `@path/to/file`:

```text
Explain the startup flow in @internal/cli/root.go and @internal/cli/repl.go
```

For large files, you can request a specific line range:

```text
Review @docs/PHASE-LOG.md#L3000-L3300
```

Range syntax supports only `@file#Lstart-Lend`.

You can also mention directories with `@path/to/dir` (or a trailing slash). The prompt expander inlines UTF-8 files and adds a directory tree block:

```text
Summarize @docs @internal/mentions/
```

Directory expansion is capped to protect context size. Current defaults:

- Per-directory files: `200`
- Per-prompt files across all mentions: `400`
- Per-directory bytes: `512 KiB`
- Per-prompt bytes across all mentions: `2 MiB`
- Max walk depth: `8`

In `--print` mode the same syntax works:

```bash
nandocodego --print "Summarize @README.md"
```

### Test the LLM Client (Phase 2)

```bash
# Run the chat example (requires Ollama running)
cd examples/chat
go run main.go --model qwen3.6:35b --prompt "Hello!"
```

### Semantic Workspace Index

Build or inspect the local semantic index when you want prompt-time retrieval
for broader project questions:

```bash
nandocodego index build .
nandocodego index status .
```

In the TUI, use:

```text
/semantic status
/semantic on
/index build
```

The index uses the configured embedding model (`qwen3-embedding:8b` by default)
and stores cache data outside the project source tree.

## Ollama Cloud API Support

`nandocodego` stays local-first by default. Local models and `/pull` continue to use your configured local Ollama daemon.

Cloud-specific behavior:

- `/models` lists local models.
- `/models --cloud` lists direct Ollama Cloud API models.
- `/models --all` lists both local and cloud catalogs.
- `/model <name>` resolves local-first, then Ollama Cloud when needed.
- `*-cloud` names are normalized to canonical direct-cloud model names when switching to direct cloud API.

Credential behavior:

- Uses `OLLAMA_API_KEY` when present.
- Otherwise checks OS keychain (`service: nandocodego`, `account: ollama.com`).
- TUI can prompt for cloud key (`Use once` or `Save to keychain`).
- `--print` and server mode are non-interactive and return credential-required errors when key is missing.

Privacy behavior:

- No cloud key prompt for local models.
- Cloud-only model use prompts for credentials before context packing and before run start.
- API keys are not persisted in plaintext config and are redacted from logs.

Cloud stream watchdogs:

- Local/default streams use `llm_stream_idle_timeout` (`90s` by default).
- Direct Ollama Cloud streams use `cloud_llm_stream_idle_timeout` (`5m` by default).
- Override at runtime with `--llm-stream-idle-timeout` or `--cloud-llm-stream-idle-timeout`.
- Long-idle streams emit an informational warning before the final `llm stream watchdog timeout`; server mode sends this as `llm_idle_warning`.

## Security

**Important:** `nandocodego` is a powerful development tool with filesystem and shell access. While it runs locally and uses permission controls, you should understand its security model before use.

See [SECURITY.md](SECURITY.md) for:
- Security posture and threat model
- Permission system details
- Credential handling
- Network policy
- Vulnerability reporting

## Documentation

- [Security Policy](SECURITY.md) - Comprehensive security documentation
- [Next Phases Implementation Plan](docs/NEXT-PHASES-IMPLEMENTATION-PLAN.md) - Authoritative roadmap order, active work routing, and pre-start checklist
- [Project Status and Engineer Onboarding](docs/PROJECT-STATUS-AND-ONBOARDING.md) - Current progress, gaps, local setup, Docker notes, and multi-model guidance
- [Application Architecture Flowchart](docs/APPLICATION-ARCHITECTURE-FLOWCHART.md) - Detailed Mermaid flowcharts for the CLI, TUI, server, agent, tools, context, retrieval, storage, and observability architecture
- [Remaining Phases Task Review](docs/REMAINING-PHASES-TASK-REVIEW.md) - Reviewed task detail, blockers, and evidence requirements for active gates
- [Phase Log](docs/PHASE-LOG.md) - Historical implementation record and acceptance evidence
- [Historical Implementation Plan](.codex/go-ollama-plan-AGENTS.md) - Original phase architecture; superseded for current roadmap order by the Next Phases plan
- [Docker Usage](README.Docker.md) - Docker and container deployment guide

## Architecture

![ Whole-Application.png](%20Whole-Application.png)

`nandocodego` is built around six core abstractions:

1. **Query Loop** - Drives the agent's reasoning and tool execution cycle
2. **Tools** - Self-describing, permission-aware operations (file I/O, shell, web, etc.)
3. **Tasks** - Background operations (shell commands, sub-agents)
4. **State** - Two-tier reactive state management
5. **Memory** - File-based, human-editable session memory
6. **Hooks** - Lifecycle event interception

## Features (By Phase)

- ✅ **Phase 0:** Security baseline, CI, dependency management
- ✅ **Phase 1:** Basic scaffolding, `--version`, `doctor` command
- ✅ **Phase 2:** Ollama LLM client with streaming, watchdog, retry logic, capability detection
- ✅ **Phase 3:** Tool interface + Bash/FileRead/FileWrite
- ✅ **Phase 4:** Agent loop
- ✅ **Phase 5:** Permission system (7 modes)
- ✅ **Phase 6:** State management
- ✅ **Phase 7:** Bubble Tea TUI + REPL
- ✅ **Phases 8-16:** Memory, hooks, MCP, sub-agents, skills, commands/config UX, tasks, concurrency, and observability
- ✅ **Phases 19-22:** Tool ecosystem, compaction, web/server mode, and enhanced TUI/input handling
- ✅ **Phase 24:** Multi-agent coordination
- ✅ **Phases 26-27:** Inline completion and directory mention expansion
- ✅ **Phases 28-29:** Semantic workspace indexing/retrieval and TUI index-progress observability
- ✅ **Ollama Cloud API key support:** complete
- ⏳ **Phase 25:** Remote / Bridge Mode
- ⏳ **Phases 17-18:** Distribution, hardening, evals, docs, and release approval

See the [Next Phases Implementation Plan](docs/NEXT-PHASES-IMPLEMENTATION-PLAN.md), [Project Status and Engineer Onboarding](docs/PROJECT-STATUS-AND-ONBOARDING.md), and [Phase Log](docs/PHASE-LOG.md) for detailed implementation progress and current launch-readiness routing.

## Development

### Local Development

```bash
# Build the binary
make build

# Run unit tests
make test

# Run tests with race detector
make test-race

# Run integration tests (requires Ollama)
make test-integration

# Lint code
make lint

# Format code
make fmt

# Run all checks (like CI)
make check

# Install locally to /usr/local/bin
make install
```

### Docker Development

```bash
# Build Docker image
make docker-build

# Run in container
make docker-run ARGS="doctor"

# Get a shell in container
make docker-shell

# Docker Compose
make docker-compose-up        # Start stack
make docker-compose-logs      # Follow logs
make docker-compose-down      # Stop stack
```

## Project Structure

```
nandocodego/
├── cmd/nandocodego/        # Main entrypoint
├── internal/
│   ├── agent/              # Query loop
│   ├── analysis/           # Project-analysis workflow, cache, ledger, checkpoints
│   ├── cli/                # CLI commands
│   ├── llm/                # Ollama local/cloud client interfaces and runtime routing
│   ├── semantic/           # Workspace semantic index and retrieval
│   ├── server/             # HTTP/SSE server mode and embedded web UI
│   ├── tools/              # Tool implementations
│   ├── permissions/        # Permission system
│   ├── state/              # State management
│   ├── tui/                # Terminal UI
│   └── ...
├── tools/                  # Build and check scripts
├── docs/                   # Documentation
└── .github/                # CI/CD workflows
```

## License

MIT License - see [LICENSE](LICENSE) for details.

## Contributing

This project is currently in active pre-v0.1 development. Contributions should follow the current roadmap in `docs/NEXT-PHASES-IMPLEMENTATION-PLAN.md` and the detailed phase or validation plan it routes to.

Please see:
- [SECURITY.md](SECURITY.md) for security guidelines
- [Project Status and Engineer Onboarding](docs/PROJECT-STATUS-AND-ONBOARDING.md) for current status and source-of-truth routing

## Acknowledgments

This project is inspired by and functionally equivalent to the TypeScript `nandocodets` reference implementation, adapted for Go and optimized for local LLMs via Ollama.

Built with:
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - Terminal UI framework
- [Cobra](https://github.com/spf13/cobra) - CLI framework
- Ollama HTTP APIs - Local model, direct cloud, and embedding access

---

**Made with 🤖 for developers who value privacy and local-first AI tooling.**
