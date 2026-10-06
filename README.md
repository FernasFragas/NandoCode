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

**Current Version:** v0.0.0-dev (Phase 29 and the evaluation framework complete; next: browser-first parity steps B1-B4 and Phase 25 browser session durability, see [ADR-002](docs/adr/ADR-002-BROWSER-UI-PRIMARY-SURFACE.md))

This project is under active development. **Engineers** should start with [engineers.md](engineers.md); **product managers** with [product-managers.md](product-managers.md). The committed roadmap is in [Next Phases Implementation Plan](docs/roadmap/NEXT-PHASES-IMPLEMENTATION-PLAN.md) and everything else is in the [Backlog](docs/roadmap/BACKLOG.md).

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
- ✅ **Evaluation framework:** `nandocodego eval` with deterministic fixtures in CI
- ⏳ **Next:** browser-first work (the browser UI is now the primary surface, see [ADR-002](docs/adr/ADR-002-BROWSER-UI-PRIMARY-SURFACE.md)), Phase 25 browser session durability, then Phase 17 (distribution) and Phase 18 (hardening and release)

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
Review @docs/phases/PHASE-LOG.md#L3000-L3300
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

## Evaluation Framework

Run the deterministic coding-task fixture suite:

```bash
nandocodego eval run ./evals
```

Validate fixture structure without executing any agent run:

```bash
nandocodego eval validate ./evals
```

Run the same fixture set against a live configured model:

```bash
nandocodego eval run ./evals --provider live --model qwen3.6:35b
```

Eval artifacts are emitted as JSON and Markdown under `.tmp-evals/`.

Example report summary:

```markdown
# NandoCode Evaluation Report

| Metric | Value |
| --- | ---: |
| Fixtures | 5 |
| Passed | 5 |
| Failed | 0 |
| Errors | 0 |
| Aggregate score | 1.00 |

| Fixture | Status | Score | Tests | Files | Tools | Approvals | Runtime |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| add-input-validation | passed | 1.00 | 1/1 | 2 | 5 | 2 | 1586ms |
| add-missing-unit-tests | passed | 1.00 | 1/1 | 1 | 4 | 1 | 666ms |
| basic-refactor | passed | 1.00 | 1/1 | 1 | 3 | 1 | 570ms |
| bugfix-with-tests | passed | 1.00 | 1/1 | 2 | 5 | 2 | 581ms |
| rename-symbol-safely | passed | 1.00 | 1/1 | 2 | 5 | 2 | 587ms |
```

Review third-party fixtures before running them. They can execute code inside a
temporary workspace during evaluation.

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

- [Engineer Guide](engineers.md) - Onboarding: build, test, repo layout, how work is done, what's next
- [Product Guide](product-managers.md) - What the product does today, release status, backlog highlights, open decisions
- [User Manual](USER_MANUAL.md) - Every command, tool, slash command, and configuration option
- [Architecture Overview](docs/architecture/ARCHITECTURE.md) - Layers, package map, request lifecycle, design rules, extension points
- [Application Architecture Flowchart](docs/architecture/APPLICATION-ARCHITECTURE-FLOWCHART.md) - Mermaid diagrams for each flow
- [Next Phases Implementation Plan](docs/roadmap/NEXT-PHASES-IMPLEMENTATION-PLAN.md) - Committed roadmap order and active work routing
- [Backlog](docs/roadmap/BACKLOG.md) - Ideas, open bugs, UX follow-ups, tech debt, parked scope
- [Phase Log](docs/phases/PHASE-LOG.md) - Implementation record and acceptance evidence
- [Security Policy](SECURITY.md) - Trust boundaries and reporting
- [Docker Usage](README.Docker.md) - Docker and container deployment guide
- [All documentation](docs/README.md)

## Architecture

![Whole-application architecture](docs/architecture/images/whole-application.png)

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
- ✅ **Evaluation framework:** `nandocodego eval`
- ⏳ **Phase 25:** Browser session durability (rescoped from Remote / Bridge Mode)
- ⏳ **Phases 17-18:** Distribution, hardening, evals, docs, and release approval

See the [Next Phases Implementation Plan](docs/roadmap/NEXT-PHASES-IMPLEMENTATION-PLAN.md), [Project Status](docs/roadmap/PROJECT-STATUS-AND-ONBOARDING.md), and [Phase Log](docs/phases/PHASE-LOG.md) for detailed implementation progress and current launch-readiness routing.

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
│   ├── eval/               # Evaluation framework
│   └── ...                 # full map: docs/architecture/ARCHITECTURE.md
├── evals/                  # Eval fixtures
├── tools/                  # Build and check scripts
├── docs/                   # Documentation (index: docs/README.md)
└── .github/                # CI/CD workflows
```

## License

MIT License - see [LICENSE](LICENSE) for details.

## Contributing

This project is currently in active pre-v0.1 development. Contributions should follow the current roadmap in `docs/roadmap/NEXT-PHASES-IMPLEMENTATION-PLAN.md` and the detailed phase or validation plan it routes to.

Please see:
- [engineers.md](engineers.md) for the development workflow
- [SECURITY.md](SECURITY.md) for security guidelines
- [Project Status](docs/roadmap/PROJECT-STATUS-AND-ONBOARDING.md) for current status and source-of-truth routing

## Acknowledgments

This project is inspired by and functionally equivalent to the TypeScript `nandocodets` reference implementation, adapted for Go and optimized for local LLMs via Ollama.

Built with:
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - Terminal UI framework
- [Cobra](https://github.com/spf13/cobra) - CLI framework
- Ollama HTTP APIs - Local model, direct cloud, and embedding access

---

**Made with 🤖 for developers who value privacy and local-first AI tooling.**
