# Project Status and Engineer Onboarding

Date: 2026-06-23

This document consolidates the implementation plans, phase logs, agent context files, Docker docs, README, and the book chapters into one current engineering view. It is meant to be the first status document an engineer reads before changing code.

## Current Documentation Routing

Use these documents in this order for launch-readiness work:

1. `docs/NEXT-PHASES-IMPLEMENTATION-PLAN.md` is the authoritative roadmap order and first-read routing document for active v0.1 work.
2. This document is the current implementation/onboarding snapshot and explains known caveats.
3. `docs/REMAINING-PHASES-TASK-REVIEW.md` expands the active gates and remaining phases into reviewed tasks, blockers, and evidence requirements.
4. `docs/PHASE-LOG.md` is the chronological implementation record and acceptance-evidence log. Do not infer current roadmap order from older entries.
5. Detailed phase/workstream docs in `docs/` own implementation steps only when they are listed by the current routing documents above.

Historical or non-authoritative material:

- `.codex/go-ollama-plan-AGENTS.md` and `.codex/go-ollama-plan-HUMANS.md` are original architecture/phase plans. They are useful reference material, but their roadmap order is superseded by `docs/NEXT-PHASES-IMPLEMENTATION-PLAN.md`.
- `.codex/agent-context/ARCHITECTURE.md` is deprecated for this repo. It references unrelated auth, sqlc, Kafka, and HTTP-service packages.
- `.codex/agent-context/testing-standards.md` has reusable generic testing principles, but its repo-specific TypeScript/Kafka/SQLite/gomock/testify section is stale for `nandocodego`.

## Roadmap Ordering Note

As of 2026-06-22, Phase 17 (`docs/PHASE-17-DETAILED-PLAN.md`) and Phase 18 (`docs/PHASE-18-DETAILED-PLAN.md`) are explicitly the last implementation phases for v0.1.0.

- Phase 17 is the penultimate phase: distribution, install, release workflow, release-facing `doctor`, and packaging verification.
- Phase 18 is the final phase: hardening, evals, docs, security review, and v0.1.0 release approval.
- Phase 24 is complete. Ollama Cloud API key support is complete. Phase 28 semantic workspace indexing is complete as an MVP. Phase 29 TUI semantic index progress observability is complete as an MVP. The Go response-time refactor is implemented and validated.
- Phase 25 Remote / Bridge Mode is the next feature implementation phase.
- Do not start Phase 17 until feature and runtime reliability work, including Phase 25, is implemented and accepted.
- Do not plan any later v0.1.0 implementation phase after Phase 18. Issues discovered in Phase 18 must be fixed, moved back to the appropriate earlier phase before release, or documented as non-blocking known limitations.

Some status details below were written earlier in the project history and may lag behind newer routing docs. Use `docs/NEXT-PHASES-IMPLEMENTATION-PLAN.md`, this document's current roadmap sections, and the listed detailed phase plans as the authoritative roadmap boundary.

## Current Roadmap To Follow

This section supersedes the older phase-status table below when deciding what to implement next.

For detailed work packages, dependencies, and exit criteria, use `docs/NEXT-PHASES-IMPLEMENTATION-PLAN.md`. For the reviewed task breakdown and blockers for each remaining phase, use `docs/REMAINING-PHASES-TASK-REVIEW.md`. This section is the compact ordering source.

As of 2026-06-22, the remaining roadmap order is:

1. **Carry forward validation evidence.** Phases 8-14, Workstream CL/PA, and Phase 22 have substantial implementation, but their live/manual evidence still needs to be reconciled before release packaging. Use `docs/GATE-G0-PHASE-8-14-VALIDATION-PLAN.md`, `docs/CONTEXT-LATENCY-OPTIMIZATION-PLAN.md`, `docs/PROMPT-ACCURACY-AND-CONTEXT-FIDELITY-PLAN.md`, and `docs/PHASE-22-DETAILED-PLAN.md`.
2. **Implement Phase 25 - Remote / Bridge Mode.** This follows Phase 21, Phase 24, Ollama Cloud API key support, and Phase 28/29 so remote/server surfaces inherit the same provider and semantic retrieval behavior.
3. **Implement Phase 17 - Distribution and Install.** This is the penultimate implementation phase and should only package a stable product surface.
4. **Implement Phase 18 - Hardening, Eval Suite, and Docs.** This is the final implementation phase and release-approval gate for v0.1.0.

Completed plans that should not be reimplemented unless a regression is found: Phase 15, Phase 16, Phase 19, Phase 20, Phase 21, Phase 24, Ollama Cloud API key support, Phase 26, Phase 27, Phase 28, Phase 29, and the Go response-time refactor.

## Current Implementation Reality Snapshot

- **Code-complete or substantially implemented:** Phases 0-16, Phase 19, Phase 20, Phase 21, Phase 22 core, Phase 24, Ollama Cloud API key support, Phase 26, Phase 27, Phase 28, Phase 29, and the response-time refactor.
- **Implemented but still needing manual/live acceptance:** Phases 8-14, Workstream CL/PA evidence, and live Phase 22/browser interaction evidence. The 2026-06-23 launch-readiness pass closed the known code gaps for prompt/trace diagnostics, semantic routing/index-status handling, TUI follow-ups, and the served browser P0 UI.
- **Remaining planned implementation:** Phase 25, Phase 17, and Phase 18.
- **Release boundary:** Phase 17 and Phase 18 are last. Any new v0.1 feature or runtime requirement belongs before Phase 17, not after Phase 18.

## Executive Summary

`nandocodego` is now a local-first Go agent CLI with a Bubble Tea REPL, Ollama-backed LLM client, optional direct Ollama Cloud routing, agent loop, tool execution, permission prompts, state layer, memory, hooks, MCP, sub-agents, skills, slash-command/config UX, background tasks, concurrency/speculative execution, observability, content compaction, inline completion, directory mention expansion, incomplete-response recovery foundations, listing-prompt accuracy safeguards, semantic workspace indexing/retrieval, HTTP/SSE server mode with a served rich browser UI, and coordinator-mode multi-agent coordination.

The project is not yet release-ready. The remaining work is less about rebuilding the core loop and more about closing live acceptance gates, implementing required v0.1 remote/bridge mode, then finishing distribution and final hardening.

Current verification status:

- The phase log records passing validation for the completed implementation slices, including `go test ./...`, dependency allowlist, network policy checks, targeted race tests, and semantic/TUI benchmarks where applicable.
- On 2026-06-23, the launch-readiness pass ran targeted tests for TUI, server, command diagnostics, semantic retrieval, retrieval routing, analysis, agent, and context packing; checked the served browser JavaScript with `node --check`; verified the root and embedded browser UI files are synchronized; passed `go test ./...` outside the sandbox; passed `tools/run-load-suite.sh`; and passed a live local server smoke that served the rich UI and created a session.
- In this sandbox, full `go test ./...` can fail for `internal/llm/ollama`, `internal/tools/sendmessage`, and `internal/tools/webfetch` listener-binding tests because local listener binding is restricted; the same suite passed outside the sandbox on 2026-06-23.
- Phase log records Phases 0 through 16 as implemented, plus post-Phase-16 runtime fixes, Phase 21, Phase 24, Ollama Cloud API key support, Phase 28, Phase 29, and the response-time refactor. Some manual exit gates still need live sign-off.

Primary documentation drift and routing corrections:

- `README.md` now routes current roadmap work through `docs/NEXT-PHASES-IMPLEMENTATION-PLAN.md` instead of treating the original `.codex` implementation plan as current.
- `README.Docker.md` and `docker-compose.yml` previously described Ollama environment variables that the current CLI does not read; Docker guidance now points at `--ollama-url`.
- `.codex/agent-context/ARCHITECTURE.md` and part of `testing-standards.md` now carry explicit staleness notices and should not be used as source of truth for this repo.
- `docs/WEB-UI-UX-PRODUCT-PLAN.md` now records that the served embedded UI is the rich browser UI and that `SessionEvent.data` mapping is fixed. P1 management panels and backend tree traversal hardening remain separate follow-ups.
- `USER_MANUAL.md` now reflects the current `/btw` behavior: isolated side run, read-only toolset, read-only permission mode, and queued execution behind an active main run.
- The original phase plan folded telemetry into distribution/install. A review against the book and current code found that observability deserves its own phase before release packaging.
- `docs/PHASE-LOG.md` contains historical "Next Steps" text with old Phase 8/11 numbering. Treat `docs/NEXT-PHASES-IMPLEMENTATION-PLAN.md`, this document's current roadmap sections, and the listed detailed phase plans as authoritative for upcoming work.
- A Chapter 12 review narrowed Phase 9 executable hook scope: command/prompt hooks are Phase 9 runtime scope, while project hooks, HTTP hooks, and agent hooks need later trust/runtime prerequisites before execution.

## Source Material Reviewed

Plans and phase docs:

- `docs/NEXT-PHASES-IMPLEMENTATION-PLAN.md`
- `docs/GATE-G0-PHASE-8-14-VALIDATION-PLAN.md`
- `docs/REMAINING-PHASES-TASK-REVIEW.md`
- `.codex/go-ollama-plan-HUMANS.md`
- `.codex/go-ollama-plan-AGENTS.md`
- `docs/PHASE-1-DETAILED-PLAN.md`
- `docs/PHASE-3-DETAILED-PLAN.md`
- `docs/PHASE-4-DETAILED-PLAN.md`
- `docs/PHASE-5-DETAILED-PLAN.md`
- `docs/PHASE-6-DETAILED-PLAN.md`
- `docs/PHASE-7-DETAILED-PLAN.md`
- `docs/PHASE-8-DETAILED-PLAN.md`
- `docs/PHASE-9-DETAILED-PLAN.md`
- `docs/PHASE-28-DETAILED-PLAN.md`
- `docs/PHASE-29-DETAILED-PLAN.md`
- `docs/GO-RESPONSE-TIME-PERFORMANCE-REFACTOR-REPORT.md`
- `docs/PHASE-LOG.md`

Context and reference docs:

- `.codex/agent-context/api-conventions.md`
- `.codex/agent-context/learnings-memory.md`
- `.codex/agent-context/technical-skills.md`
- `.codex/agent-context/testing-standards.md`
- `.codex/agent-context/ARCHITECTURE.md`
- `book/ch01-architecture.md` through `book/ch18-epilogue.md`
- `README.md`
- `README.Docker.md`
- `DOCKER_WEB_GUIDE.md`
- `SECURITY.md`
- `Makefile`
- `Dockerfile`
- `docker-compose.yml`

## Current Phase Status

| Phase | Status | Current repo reality |
|---|---:|---|
| 0 - Security and supply-chain baseline | Done | Security policy, dependency allowlist, network policy check, CI guardrails, and phase verification scripts exist. |
| 1 - Repo scaffolding and tooling | Done | Go module, Cobra CLI, version/doctor commands, paths, logging, Makefile, tests, and XDG/NANDOCODEGO path overrides exist. |
| 2 - LLM client / Ollama | Done | `internal/llm` and `internal/llm/ollama` provide the provider-neutral interface, Ollama streaming, model list/pull/embed API shape, retry/watchdog, capabilities, and chat example. |
| 3 - Tool interface and starter tools | Done | Self-describing tool interface, registry, path safety, Bash, FileRead, FileWrite, and built-in registry exist. |
| 4 - Agent loop | Done | `internal/agent` can stream model turns, execute tool calls, emit events, track usage, handle errors, and run integration tests behind an explicit Ollama env flag. |
| 5 - Permission system | Done | Central resolver, seven permission modes, rules, command matching, Bash classifier integration, and agent permission integration exist. |
| 6 - State layer | Done | `internal/bootstrap` and `internal/state` implement two-tier state, reactive store, app state, app-to-bootstrap mirroring, tests, race coverage, and benchmark coverage. |
| 7 - Bubble Tea TUI + REPL | Done | `internal/tui` implements transcript rendering, markdown, slash commands, Vim modes, permission broker/modal, agent bridge, CLI no-args REPL wiring, and tests. |
| 8 - Memory | Core implementation landed, exit-gate pending | `internal/memory` now includes root resolution, scan/frontmatter, index caps, staleness warnings, prompt-section builder, recall side-query, pending extraction drafts, and runner integration. Conversation persistence events were also added. Remaining Phase 8 closure work is manual two-session validation and final phase-log exit-gate sign-off. |
| 9 - Hooks | Core implementation landed, exit-gate pending | `internal/hooks` now includes event types, JSON snapshot loading, matcher, command/prompt runners, disabled project/HTTP/agent handling, dispatcher, runner decorator, tests, and REPL integration. Remaining Phase 9 closure work is the live Ollama manual blocking demo and project-hook disabled diagnostic confirmation. |
| 10 - MCP integration | Core implementation landed, exit-gate pending | `internal/mcp` contains config, transports, tool wrapping, server lifecycle, and tests. Remaining work is live MCP server validation and final phase-log sign-off. |
| 11 - Sub-agents and fork | Core implementation landed, exit-gate pending | Sub-agent tools, fork lifecycle, child state isolation, cancellation, and JSONL output exist. Remaining work is live validation of inheritance, cancellation, result return, and recursion prevention. |
| 12 - Skills | Core implementation landed, exit-gate pending | Skill discovery, frontmatter parsing, prompt loading, source handling, and tests exist. Remaining work is live REPL validation and prompt-injection boundary review. |
| 13 - Slash commands and config UX | Core implementation landed, exit-gate pending | Command registry, config loading, and richer slash commands exist. Remaining work is live UX validation and any source-provenance follow-up found during review. |
| 14 - Tasks | Core implementation landed, exit-gate pending | Task supervisor, task lifecycle, output streaming, and task tools exist. Remaining work is manual validation of lifecycle, stop/cleanup, and status rendering. |
| 15 - Concurrency and speculative execution | Done | Tool partitioning, safe concurrent execution, speculative paths, tests, and phase-log closure exist. |
| 16 - Observability and metrics | Done | Logging/metrics decorators, meter state, retry/done-reason diagnostics, `/cost` integration, and tests exist. |
| 17 - Distribution and install | Planned; penultimate | Not started under the current roadmap. Must be implemented after required feature, runtime reliability, server, coordinator, and remote/bridge work is complete. |
| 18 - Hardening, eval suite, docs | Planned; final | Not started under the current roadmap. This is the final v0.1.0 hardening, eval, docs, and release-approval phase after Phase 17. |
| 19 - Complete tool ecosystem | Done | Later tool ecosystem work is recorded as complete in `docs/PHASE-LOG.md`; do not reimplement unless a regression is found. |
| 20 - Content compaction | Done with caveat | Content compaction is complete; hook-dispatch caveats are documented in the phase plan/log. |
| 21 - Web interface and HTTP API | Complete | `nandocodego server`, HTTP/SSE sessions, browser UI, and HTTP permission broker are implemented and validated with automated checks, `gosec`, Docker runtime, and live in-container API checks. |
| 22 - Enhanced TUI and input handling | Core implementation landed; manual/follow-up gate open | Run visibility, status details, transcript performance, bracketed paste preprocessing, keybinding context primitives, chord handling, status snapshots, `/queue`, `/bg`, `/btw`, activity/tip lines, and automated verification are implemented. Remaining gaps: live REPL evidence, context-stack modal priority fix, `/btw` read-only tool manifest restriction, full textarea-integrated Vim mutations/repeat/registers, true concurrent `/btw`, full collapsible hierarchical activity tree, click-to-expand tool panels, and mouse lost-release recovery. |
| 23 - OpenAI-compatible LLM adapter | Removed from active v0.1 plan | No longer wanted for this roadmap. Keep the existing plan document only as historical/archive material unless this decision changes. |
| 24 - Multi-agent coordination | Complete | Coordinator mode, `SendMessage`, bounded mailboxes, worker name registration, auto-resume hooks, dream lifecycle, restricted worker/coordinator registries, TUI coordinator status, and server coordinator runtime are implemented and validated. |
| 25 - Remote / bridge mode | Planned before Phase 17; required for v0.1 | Next feature implementation phase. Implement after Phase 21, Phase 24, Ollama Cloud API key support, and Phase 28/29 semantic index behavior. |
| 26 - Inline completion in TUI input | Done | Inline completion is complete and should not be treated as future work. |
| 27 - Directory mention expansion | Done | Directory mention expansion is complete and should not be treated as future work. |
| 28 - Semantic workspace index and embedding retrieval | Implemented MVP | `internal/semantic`, `nandocodego index`, TUI `/semantic` and `/index`, prompt/server semantic retrieval injection, local cache store, and embedding API modernization are implemented. |
| 29 - TUI semantic index progress observability | Implemented MVP | Semantic build/refresh event progress, TUI status progress rendering, concise transcript completion/error lines, and concurrent index operation guard are implemented. |

## Current Plan Gaps

The original observability gap has been addressed by Phase 16 and later retry/done-reason diagnostics. The major response-time refactor has also landed: common prompts can bypass semantic retrieval and tool schemas, output budget defaults are larger, semantic retrieval has caching/light candidate narrowing, context-pack reads and index scanning are parallelized, and TUI transcript/picker paths are optimized.

Current carry-forward validation priority:

- Preserve deep context quality rather than making prompts smaller by default.
- Record Gate G0 live/manual evidence for Phases 8-14.
- Record realistic CL/PA evidence for small, medium, and large runs using `/trace last`, `/prompt last`, context mode, semantic retrieval, checkpoint/resume behavior, and final-answer completeness.
- Record Phase 22 live REPL evidence and explicitly defer or close the remaining deep-interaction follow-ups.

Distribution/install remains Phase 17 and final hardening/eval/docs remains Phase 18.

## What Is Done

The implemented system now has these working foundations:

- A Go CLI entrypoint in `cmd/nandocodego`.
- Cobra root command with `doctor`, `version`, `init`, `server`, `index`, `--help`, `--model`, `--ollama-url`, `--print`, `--json`, `--num-ctx`, and TUI flags.
- A no-args REPL path that builds bootstrap state, app store, Ollama client, tool registry, agent runner, and Bubble Tea program.
- Local-only default Ollama endpoint: `http://localhost:11434`.
- A provider-neutral `llm.Client` interface with Ollama local and direct Ollama Cloud runtime routing.
- A switchable `llm.RuntimeClient`, local-first model resolver, and credential-gated direct cloud activation flow.
- Model capability lookup in `internal/llm/capabilities.go`.
- Starter tool registry with Bash, FileRead, and FileWrite.
- A central permission resolver with modes, rules, prompts, and tool classifier integration.
- A two-tier state architecture:
  - `internal/bootstrap` for infrastructure/session facts.
  - `internal/state` for reactive UI and session state.
- A TUI transcript with assistant streaming, thinking output, tool panels, retry notices, and terminal status.
- A permission prompt broker that blocks the agent goroutine, not the Bubble Tea update loop.
- Memory recall/extraction runner wiring with file-based project memory and pending drafts.
- Hook snapshot loading, command/prompt hook execution, pre-tool permission gating, lifecycle dispatch, and TUI hook notices.
- MCP, sub-agent, skill, command/config, task, concurrency, observability, compaction, inline-completion, directory-mention, semantic index, and index-progress foundations recorded in the phase log.
- Incomplete-response retry diagnostics and TUI visibility improvements for preamble-only responses.
- Common-prompt response-time improvements across request shape, semantic retrieval, context pack, startup, transcript rendering, and picker/index paths.
- Unit tests across implemented packages and integration tests gated by explicit env vars.
- Security scripts that enforce dependency allowlisting and prevent accidental hardcoded external endpoints.

## What Is Missing

The highest-impact missing product features are:

- Exit-gate closure for Phases 8-14: memory, hooks, MCP, sub-agents, skills, command/config UX, and tasks need live/manual validation recorded against their phase docs.
- Manual evidence capture for context, latency, project-scale analysis, and listing prompt accuracy: `/trace last`, `/prompt last`, effective context mode, retrieval/checkpoint behavior, final-answer completeness, and listing/tree-mode prompt shape need recorded live evidence before release packaging.
- Phase 22 closure: automated implementation is mostly landed, but manual evidence and the explicit deep-interaction follow-ups in `docs/PHASE-22-DETAILED-PLAN.md` still need resolution before treating the phase as fully complete.
- Phase 25: remote/bridge mode, including `nandocodego connect`, detached sessions, reconnect/replay, JWT auth, and remote TUI bridge.
- Phase 17: release builds, installer, checksums, release workflow, changelog, and release-facing `doctor`.
- Phase 18: final eval suite, realistic REPL smoke tests, model matrix testing, end-to-end permission scenarios, docs, performance gates, and security review.

## Known Risks And Documentation Drift

The current code is test-green, but an engineer should account for these risks:

- Docker and web docs should stay aligned with the existing `nandocodego server` command, port 8080 defaults, and `--bind` / `--port` flags.
- Provider support remains local-first Ollama plus direct Ollama Cloud API for cloud-only models. Generic OpenAI-compatible provider work has been removed from the active roadmap and should not be revived as part of Ollama Cloud support.
- Default model naming in docs and code should be rechecked before Phase 17/18. Engineers should pass `--model` explicitly when validating behavior across machines.
- Model switching validates through the model runtime when that service is wired. Release validation should cover both TUI and non-TUI command paths.
- Hook core is implemented, but project-controlled hook config is deliberately parsed and reported without execution because there is no workspace trust flow or full config provenance model yet.
- Project-scale analysis can still fail by spending too much context on raw file contents and then stopping before the promised final report. Use the reliability roadmap before reducing `num_ctx`.
- TUI response visibility foundations have landed, but live REPL evidence and deep-interaction follow-ups still need release-gate disposition.
- `.codex/agent-context/ARCHITECTURE.md` is not valid for this repo. It references auth, sqlc, Kafka, and HTTP packages that do not exist here.
- `.codex/agent-context/testing-standards.md` has useful generic testing advice, but its repo-specific section references TypeScript, Kafka, SQLite, gomock, and testify patterns that are not current for `nandocodego`.
- The book chapters describe a mature Claude-Code-style architecture. They are reference material, not an exact statement of what this Go/Ollama repo currently implements.

## Architecture Onboarding

The product plan and book organize the system around six abstractions:

1. Query Loop: the agent run loop that turns user messages into model calls, tool calls, events, and terminal states.
2. Tools: self-describing, permission-aware operations exposed to the model.
3. Tasks: long-running or background work. Core implementation exists; live acceptance remains.
4. State: bootstrap/session facts plus reactive UI state.
5. Memory: cross-session, human-readable knowledge. Core implementation landed; manual validation remains.
6. Hooks: lifecycle interception. Core implementation landed; manual REPL validation and future trust/UX work remain.

Current package map:

| Package | Responsibility |
|---|---|
| `cmd/nandocodego` | Process entrypoint. |
| `internal/cli` | Cobra commands, `doctor`, `version`, and REPL startup wiring. |
| `internal/bootstrap` | Thread-safe session/config snapshot and global bootstrap singleton. |
| `internal/state` | Reactive store and UI-facing app state. |
| `internal/llm` | Provider-neutral LLM types, retry, watchdog, and capability matrix. |
| `internal/llm/ollama` | Ollama client implementation. |
| `internal/llm/modelresolver` | Local-first local/cloud model origin resolution. |
| `internal/llm/modelruntime` | Credential-gated runtime switching between local Ollama and direct Ollama Cloud API. |
| `internal/agent` | Model-driven agent loop, event stream, tool execution bridge, and usage tracking. |
| `internal/contextpack` | Current-turn prompt evidence packing, budgets, and omission metadata. |
| `internal/analysis` | Project analysis workflow foundations, cache, ledger, checkpoint, and retrieval helpers. |
| `internal/semantic` | Local semantic workspace index, embeddings, retrieval, rendering, and progress events. |
| `internal/tools` | Tool contracts, schemas, registry, path safety, and execution context. |
| `internal/tools/bash` | Bash tool and command safety classification. |
| `internal/tools/fileread` | Read-only file tool. |
| `internal/tools/filewrite` | Atomic file writing tool. |
| `internal/tools/builtin` | Built-in starter tool registry. |
| `internal/permissions` | Modes, rule matching, resolver, decisions, and prompt contract. |
| `internal/memory` | Project-scoped memory scan, recall, prompt injection, pending extraction, and runner decorator. |
| `internal/hooks` | Snapshot-based hook config, command/prompt hook execution, lifecycle dispatch, and agent/permission bridge. |
| `internal/tui` | Bubble Tea model, transcript, markdown rendering, permission modal, slash parsing, and agent bridge. |
| `internal/server` | HTTP/SSE server mode, session manager, embedded UI, permission broker, and server-side agent runtime. |
| `internal/paths` | XDG and `NANDOCODEGO_*` path helpers. |
| `internal/logging` | Structured slog setup. |
| `internal/version` | Build/version metadata. |

Golden path for one prompt today:

1. `nandocodego` starts in `internal/cli/root.go`.
2. No subcommand calls `runREPL` in `internal/cli/repl.go`.
3. REPL builds `bootstrap.DefaultInitial`, applies `--model` and `--ollama-url`, and creates `state.Store[state.App]`.
4. REPL creates the local Ollama client, runtime router, model resolver/runtime service, built-in tools, memory runner, hook runner, and semantic service.
5. REPL creates `agent.Agent` and `tui.Model`.
6. User submits a prompt in the TUI.
7. If the active or requested model is cloud-only, the model runtime obtains `OLLAMA_API_KEY` from session memory, env, keychain, or TUI prompt before context packing.
8. TUI appends the user message, sets `ActiveRun=true`, builds current-turn packed evidence and optional semantic retrieval context, builds `agent.Input`, and launches the agent command.
9. Hook runner dispatches session/user-prompt events and injects pre/post tool callbacks.
10. Memory runner can add recalled memory context before the model turn and extract pending drafts afterward.
11. Agent streams events back into Bubble Tea messages.
12. TUI reduces events into transcript items, active tool state, hook notices, retry notices, semantic/index notices, terminal status, and usage.
13. Tool calls flow through hook-aware permission resolution before execution.

## Recommended Engineer Read Order

For a new engineer, read in this order:

1. `README.md` for the product summary and commands.
2. `docs/NEXT-PHASES-IMPLEMENTATION-PLAN.md` for current roadmap order and active-doc routing.
3. This document for current reality and caveats.
4. `docs/REMAINING-PHASES-TASK-REVIEW.md` for reviewed active work details.
5. `docs/PHASE-LOG.md` for implementation history and acceptance evidence.
6. `SECURITY.md` for trust boundaries and local-first limitations.
7. `.codex/go-ollama-plan-HUMANS.md` and `.codex/go-ollama-plan-AGENTS.md` only as historical architecture context.
8. Current source in this order:
   - `internal/cli/repl.go`
   - `internal/bootstrap/state.go`
   - `internal/state/app.go`
   - `internal/agent/agent.go`
   - `internal/memory/runner.go`
   - `internal/hooks/runner.go`
   - `internal/tools/tool.go`
   - `internal/permissions/resolver.go`
   - `internal/tui/app.go`
9. Book chapters for future-phase design:
   - `book/ch01-architecture.md`
   - `book/ch05-agent-loop.md`
   - `book/ch06-tools.md`
   - `book/ch07-concurrency.md`
   - `book/ch08-sub-agents.md`
   - `book/ch11-memory.md`
   - `book/ch12-extensibility.md`
   - `book/ch13-terminal-ui.md`
   - `book/ch14-input-interaction.md`
   - `book/ch15-mcp.md`

## Local Development

Prerequisites:

- Go matching `go.mod` (`go 1.26.2` at the time of this document).
- Ollama installed and running for real model interaction.
- At least one local Ollama model pulled.
- Optional: `golangci-lint`, `gosec`, `govulncheck`, and `gofumpt` for full local parity with `make check` / `make security`.

Start Ollama and pull a model:

```bash
ollama serve
ollama pull qwen3
```

Build:

```bash
make build
./bin/nandocodego --version
./bin/nandocodego doctor
```

Run the REPL:

```bash
./bin/nandocodego --model qwen3
```

Run without alt-screen for easier terminal/debug capture:

```bash
./bin/nandocodego --model qwen3 --no-alt-screen
```

Point at a non-default Ollama endpoint:

```bash
./bin/nandocodego --model qwen3 --ollama-url http://localhost:11434
```

Run the standalone chat example:

```bash
go run ./examples/chat --model qwen3 --prompt "Hello from nandocodego"
```

Core checks:

```bash
go test ./...
tools/check-allowed-deps.sh
tools/check-network-policy.sh
```

Race and integration checks:

```bash
go test -race ./internal/bootstrap/... ./internal/state/... ./internal/tui/...
NANDOCODEGO_RUN_OLLAMA_INTEGRATION=1 OLLAMA_MODEL=qwen3 go test -tags=integration ./internal/agent
```

Full Make targets:

```bash
make test
make test-race
make test-integration
make vet
make fmt-check
make check
```

`make lint` and `make security` require external tools to be installed. The security target also requires `gosec` and `govulncheck`.

## Docker Usage

Docker supports both one-shot CLI/REPL usage and the Phase 21 HTTP/SSE server command. The default image command still prints help unless an explicit command is supplied.

Build the image:

```bash
make docker-build
```

Run simple commands:

```bash
make docker-run ARGS="--help"
make docker-run ARGS="doctor"
make docker-run ARGS="version"
```

Run the REPL from Docker against host Ollama on macOS/Windows:

```bash
make docker-run ARGS="--model qwen3 --ollama-url http://host.docker.internal:11434"
```

On Linux, the current `make docker-run` target does not add `host.docker.internal`. Use plain Docker with host networking or add an `extra_hosts` mapping:

```bash
docker run --rm -it --network host nandocodego:latest --model qwen3 --ollama-url http://localhost:11434
```

Docker Compose:

```bash
make docker-compose-up
make docker-compose-logs
make docker-compose-down
```

Compose currently runs the Dockerfile default command (`--help`) unless you set `command:`. Useful overrides today are `["doctor"]`, `["version"]`, or a REPL command with `--model` and `--ollama-url`.

For server mode, run `nandocodego server --bind 0.0.0.0 --port 8080` in the container and publish port 8080. If using direct Ollama Cloud models, pass `OLLAMA_API_KEY` through the container environment or rely on a configured keychain where available.

## Adding Multiple LLMs

The current system supports multiple local Ollama models plus direct Ollama Cloud API models. It does not support generic non-Ollama provider backends in the active v0.1 roadmap.

Pull models into Ollama:

```bash
ollama pull qwen3
ollama pull llama3.2
ollama pull mistral
ollama pull gemma3
```

Start the REPL with a specific model:

```bash
./bin/nandocodego --model qwen3
./bin/nandocodego --model llama3.2
```

Switch model inside the REPL:

```text
/model mistral
/model kimi-k2.6:cloud
```

Current behavior of `/model`:

- It resolves local models first through the local Ollama daemon.
- If the requested model is cloud-only, it resolves against the direct Ollama Cloud catalog.
- `:cloud` and `-cloud` suffixes can force cloud intent and normalize to the canonical cloud model name.
- Cloud-only model use requires `OLLAMA_API_KEY`, a saved keychain credential, or a TUI prompt before context is sent.
- Canceling or failing the cloud credential flow leaves the previous model/provider active.
- `/pull <model>` always targets the local Ollama daemon.

Model flow in code:

1. CLI flag `--model` sets `bootstrap.Initial.DefaultModel`.
2. `state.DefaultApp` copies `bootstrap.Snapshot.DefaultModel` into `state.App.ActiveModel`.
3. Model runtime resolution can update `state.App.ActiveModel` and `state.App.LLMProvider`.
4. TUI prompt submission copies `ActiveModel` and `LLMProvider` into `agent.Input`.
5. Agent builds `llm.ChatRequest.Model`.
6. The runtime client routes the chat to the active local or direct-cloud Ollama client.

To add or tune an Ollama model family:

1. Update `internal/llm/capabilities.go`.
2. Add or adjust tests in `internal/llm/capabilities_test.go`.
3. Document any expected tool-calling limitations.
4. Run:

```bash
go test ./internal/llm/...
go test ./...
```

Capability fields are:

- `SupportsTools`
- `SupportsThinking`
- `SupportsImages`
- `RecommendedNumCtx`

Generic non-Ollama provider work is not active. If that roadmap decision changes later:

1. Keep `internal/agent` provider-agnostic by depending only on `llm.Client`.
2. Implement the `llm.Client` interface in a new provider package, for example `internal/llm/openai` or `internal/llm/vllm`.
3. Extend the existing config/source-tagging path for the new provider.
4. Preserve the network policy and make any new endpoints explicit.
5. Add provider-specific tests with fake HTTP servers.
6. Extend `doctor` to report provider configuration without leaking secrets.
7. Update `tools/allowed-deps.txt` only if a new direct dependency is justified.

## Book Chapter Status Map

The `book/` directory is a design reference for a mature agentic CLI. Current implementation coverage is:

| Book area | Current repo status |
|---|---|
| Architecture and six abstractions (`ch01`) | Substantially implemented. Query loop, tools, permissions, state, TUI, memory, hooks, MCP, sub-agents, skills, tasks, concurrency, and observability foundations exist; live acceptance remains for some areas. |
| Bootstrap (`ch02`) | Implemented in Go through `internal/cli`, `internal/bootstrap`, `internal/state`, and config wiring; review release-time config defaults before Phase 17. |
| Messages/context (`ch03`) | Message model exists in `internal/llm`; context packing, prompt dumps, semantic retrieval evidence, checkpoint foundations, and current-turn budget handling are implemented. Final large-project eval evidence remains carry-forward work. |
| API layer (`ch04`) | Ollama local and direct Ollama Cloud implementation exists; generic multi-provider support remains out of active scope. |
| Agent loop (`ch05`) | Phase 4 agent loop exists and Phase 9 stop hooks are wired; response-time request-shape optimizations and incomplete-response diagnostics are implemented. |
| Tools (`ch06`) | Tool registry and later tool ecosystem work exist; validate any remaining tool gaps against Phase 19 before adding more. |
| Concurrency (`ch07`) | Phase 15 concurrency/speculative foundations are implemented. |
| Sub-agents (`ch08`) | Phase 11 sub-agent/fork foundations are implemented; live validation remains. |
| Permissions/security (`ch09`) | Central resolver, hook integration, and command/config UX exist; final trust/security review remains Phase 18 work. |
| Tasks/coordination (`ch10`) | Phase 14 task supervisor and Phase 24 multi-agent coordination are implemented; keep coordinator eval and live evidence in the Phase 18 hardening scope. |
| Memory (`ch11`) | Core implementation landed and integrated; exit-gate validation still pending against the detailed plan. |
| Extensibility/skills/hooks (`ch12`) | Hooks and skills foundations are implemented; project hook trust, HTTP hooks, and agent hooks remain constrained by later trust/runtime work. |
| Terminal UI (`ch13`) | Implemented with Bubble Tea rather than custom React/Ink. Phase 22 added run visibility, status details, virtualized transcript rendering, sticky-scroll safeguards, activity/tip lines, and render benchmarks. Advanced selection, full collapsible hierarchy, and mouse edge-case recovery remain follow-ups. |
| Input interaction (`ch14`) | Inline completion, bracketed paste preprocessing, keybinding context primitives, chord handling, transcript search, and a Vim command-state parser are implemented. Full textarea-integrated Vim mutations, dot-repeat, find-repeat, registers, and paste/yank semantics remain partial. |
| MCP (`ch15`) | MCP foundation exists; live server validation remains. |
| Remote/cloud (`ch16`) | Phase 21 server mode is complete; Ollama Cloud API key support is complete; Phase 28/29 semantic retrieval and index progress are implemented; Phase 25 remote/bridge mode follows before Phase 17. |
| Build/distribution (`ch17`) | Basic Makefile and Docker files exist; release/install pipeline is not done. |
| Lessons/epilogue (`ch18`) | Useful as future design guidance, not an implementation status source. |

Observability is cross-chapter rather than isolated to one book chapter. The relevant guidance appears in `ch03` for process-level counters, `ch04` for API wrapper correlation and watchdog diagnostics, `ch06` for telemetry-safe tool error classification, and `ch08` for shared response metrics across agent variants. Phase 16 and post-Phase-16 retry diagnostics now cover that foundation; remaining reliability work is focused on large-project context and final-answer completion.

## Recommended Next Engineering Priorities

1. Start Phase 25 Remote / Bridge Mode from `docs/PHASE-25-DETAILED-PLAN.md`.
2. Keep Gate G0, CL/PA, and Phase 22 manual/live evidence as carry-forward validation work before release packaging.
3. Track the non-blocking context-packing follow-ups from `docs/CONTEXT-PACKING-LARGE-FILE-REVIEW-2026-05-20.md`.
4. Preserve correction notes for historical phase-numbering drift in `docs/PHASE-LOG.md`.
5. Keep stale `.codex/agent-context` notices in place unless those files are rewritten for this repo.
6. Keep repo-specific testing guidance aligned with current Go tests, the Makefile, and active validation plans.
