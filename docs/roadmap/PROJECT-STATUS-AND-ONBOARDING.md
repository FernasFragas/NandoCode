# Project Status

Date: 2026-06-23 (last reviewed 2026-10-06, after the ADR-002 browser-first re-plan)

A snapshot of what is built, what is validated, and what is missing. For the
committed order of upcoming work see
[NEXT-PHASES-IMPLEMENTATION-PLAN.md](NEXT-PHASES-IMPLEMENTATION-PLAN.md). For
ideas and deferred work see [BACKLOG.md](BACKLOG.md). For how the system is
built see [ARCHITECTURE.md](../architecture/ARCHITECTURE.md). New to the project?
Start with [engineers.md](../../engineers.md) or
[product-managers.md](../../product-managers.md).

## Current Implementation Reality Snapshot

- **Code-complete or substantially implemented:** Phases 0-16, Phase 19, Phase 20, Phase 21, Phase 22 core, Phase 24, Ollama Cloud API key support, Phase 26, Phase 27, Phase 28, Phase 29, the response-time refactor, and the evaluation framework (`nandocodego eval`, 2026-06-25).
- **CI:** `.github/workflows/ci.yml` and `security.yml` run on every push and PR (see `docs/architecture/ARCHITECTURE.md` § Security Boundaries).
- **Implemented but still needing manual/live acceptance:** Phases 8-14 (Gate G0) and Workstream CL/PA evidence. Phase 22 was accepted as-is on 2026-10-05; the TUI is in maintenance mode ([ADR-002](../adr/ADR-002-BROWSER-UI-PRIMARY-SURFACE.md)). The 2026-06-23 launch-readiness pass closed the known code gaps for prompt/trace diagnostics, semantic routing/index-status handling, TUI follow-ups, and the served browser P0 UI.
- **Remaining planned implementation:** browser-first steps B1-B2, Phase 25 (rescoped to browser session durability), B3-B4, the G0 and CL/PA evidence gates, then Phase 17 and Phase 18. Order: [NEXT-PHASES-IMPLEMENTATION-PLAN.md](NEXT-PHASES-IMPLEMENTATION-PLAN.md).
- **Release boundary:** Phase 17 and Phase 18 are last. Any new v0.1 feature or runtime requirement belongs before Phase 17, not after Phase 18.


## Verification Status

- `docs/phases/PHASE-LOG.md` records passing validation for each completed slice: `go test ./...`, dependency allowlist, network policy checks, targeted race tests, and benchmarks where applicable.
- The 2026-06-23 launch-readiness pass ran targeted tests across TUI, server, commands, semantic retrieval, routing, analysis, agent, and context packing; `go test ./...`; `tools/run-load-suite.sh`; and a live local server smoke.
- CI now runs build, vet, race tests on three OSes, lint, security scans, and the deterministic eval suite on every push.
- Inside restricted sandboxes, listener-binding tests in `internal/llm/ollama`, `internal/tools/sendmessage`, and `internal/tools/webfetch` can fail; they pass on a normal machine.

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
| 7 - Bubble Tea TUI + REPL | Done; maintenance mode (ADR-002) | `internal/tui` implements transcript rendering, markdown, slash commands, Vim modes, permission broker/modal, agent bridge, CLI no-args REPL wiring, and tests. |
| 8 - Memory | Core implementation landed, exit-gate pending | `internal/memory` now includes root resolution, scan/frontmatter, index caps, staleness warnings, prompt-section builder, recall side-query, pending extraction drafts, and runner integration. Conversation persistence events were also added. Remaining Phase 8 closure work is manual two-session validation and final phase-log exit-gate sign-off. |
| 9 - Hooks | Core implementation landed, exit-gate pending | `internal/hooks` now includes event types, JSON snapshot loading, matcher, command/prompt runners, disabled project/HTTP/agent handling, dispatcher, runner decorator, tests, and REPL integration. Remaining Phase 9 closure work is the live Ollama manual blocking demo and project-hook disabled diagnostic confirmation. |
| 10 - MCP integration | Core implementation landed, exit-gate pending | `internal/mcp` contains config, transports, tool wrapping, server lifecycle, and tests. Remaining work is live MCP server validation and final phase-log sign-off. |
| 11 - Sub-agents and fork | Core implementation landed, exit-gate pending | Sub-agent tools, fork lifecycle, child state isolation, cancellation, and JSONL output exist. Remaining work is live validation of inheritance, cancellation, result return, and recursion prevention. |
| 12 - Skills | Core implementation landed, exit-gate pending | Skill discovery, frontmatter parsing, prompt loading, source handling, and tests exist. Remaining work is live REPL validation and prompt-injection boundary review. |
| 13 - Slash commands and config UX | Core implementation landed, exit-gate pending | Command registry, config loading, and richer slash commands exist. Remaining work is live UX validation and any source-provenance follow-up found during review. |
| 14 - Tasks | Core implementation landed, exit-gate pending | Task supervisor, task lifecycle, output streaming, and task tools exist. Remaining work is manual validation of lifecycle, stop/cleanup, and status rendering. |
| 15 - Concurrency and speculative execution | Done | Tool partitioning, safe concurrent execution, speculative paths, tests, and phase-log closure exist. |
| 16 - Observability and metrics | Done | Logging/metrics decorators, meter state, retry/done-reason diagnostics, `/cost` integration, and tests exist. |
| 17 - Distribution and install | Planned; penultimate | Not started. Runs after B1-B4, Phase 25 (rescoped), and the G0/CL evidence gates. Also owns the default-command change: plain `nandocodego` opens the browser, the TUI moves to `nandocodego tui`. |
| 18 - Hardening, eval suite, docs | Planned; final | The deterministic eval framework it needs landed early (2026-06-25, `docs/plans/EVALUATION-FRAMEWORK-DETAILED-PLAN.md`); the rest is not started. This is the final v0.1.0 hardening, eval, docs, and release-approval phase after Phase 17. |
| 19 - Complete tool ecosystem | Done | Later tool ecosystem work is recorded as complete in `docs/phases/PHASE-LOG.md`; do not reimplement unless a regression is found. |
| 20 - Content compaction | Done with caveat | Content compaction is complete; hook-dispatch caveats are documented in the phase plan/log. |
| 21 - Web interface and HTTP API | Complete | `nandocodego server`, HTTP/SSE sessions, browser UI, and HTTP permission broker are implemented and validated with automated checks, `gosec`, Docker runtime, and live in-container API checks. |
| 22 - Enhanced TUI and input handling | Accepted as-is 2026-10-05; TUI in maintenance mode | Run visibility, status details, transcript performance, bracketed paste preprocessing, keybinding context primitives, chord handling, status snapshots, `/queue`, `/bg`, `/btw`, activity/tip lines, and automated verification are implemented. Unfinished deep-interaction items are parked in [BACKLOG.md](BACKLOG.md) §6; only the P22-A safety slice still needs manual REPL checks. |
| 23 - OpenAI-compatible LLM adapter | Removed from active v0.1 plan | No longer wanted for this roadmap. The plan document was deleted; recover it from git history if this decision changes. |
| 24 - Multi-agent coordination | Complete | Coordinator mode, `SendMessage`, bounded mailboxes, worker name registration, auto-resume hooks, dream lifecycle, restricted worker/coordinator registries, TUI coordinator status, and server coordinator runtime are implemented and validated. |
| 25 - Browser session durability (rescoped from Remote / bridge mode) | Planned; required for v0.1 | Rescoped by ADR-002: detach/reattach, replay with gap detection, persisted session metadata. Runs after B2, before B3. `connect`, JWT, and the TUI bridge are out of scope. |
| 26 - Inline completion in TUI input | Done | Inline completion is complete and should not be treated as future work. |
| 27 - Directory mention expansion | Done | Directory mention expansion is complete and should not be treated as future work. |
| 28 - Semantic workspace index and embedding retrieval | Implemented MVP | `internal/semantic`, `nandocodego index`, TUI `/semantic` and `/index`, prompt/server semantic retrieval injection, local cache store, and embedding API modernization are implemented. |
| 29 - TUI semantic index progress observability | Implemented MVP | Semantic build/refresh event progress, TUI status progress rendering, concise transcript completion/error lines, and concurrent index operation guard are implemented. |


## What Is Missing

The highest-impact missing product features are:

- Exit-gate closure for Phases 8-14: memory, hooks, MCP, sub-agents, skills, command/config UX, and tasks need live/manual validation recorded against their phase docs.
- Manual evidence capture for context, latency, project-scale analysis, and listing prompt accuracy: `/trace last`, `/prompt last`, effective context mode, retrieval/checkpoint behavior, final-answer completeness, and listing/tree-mode prompt shape need recorded live evidence before release packaging.
- Phase 25 (rescoped 2026-10-05 by ADR-002): browser session durability: detached sessions, reconnect/replay with gap detection, session persistence across restarts. `connect`, JWT, and the remote TUI bridge are out of scope.
- Browser-first parity (roadmap steps B1-B4): run cancel, cloud key entry, command endpoints, session list, management panels, accessibility.
- Phase 17: release builds, installer, checksums, release workflow, changelog, and release-facing `doctor`.
- Phase 18: final eval suite, realistic REPL smoke tests, model matrix testing, end-to-end permission scenarios, docs, performance gates, and security review.


## Known Risks

The current code is test-green, but an engineer should account for these risks:

- Docker and web docs should stay aligned with the existing `nandocodego server` command, port 8080 defaults, and `--bind` / `--port` flags.
- Provider support remains local-first Ollama plus direct Ollama Cloud API for cloud-only models. Generic OpenAI-compatible provider work has been removed from the active roadmap and should not be revived as part of Ollama Cloud support.
- Default model naming in docs and code should be rechecked before Phase 17/18. Engineers should pass `--model` explicitly when validating behavior across machines.
- Model switching validates through the model runtime when that service is wired. Release validation should cover the browser model picker, the TUI, and `--print`.
- Hook core is implemented, but project-controlled hook config is deliberately parsed and reported without execution because there is no workspace trust flow or full config provenance model yet.
- Project-scale analysis can still fail by spending too much context on raw file contents and then stopping before the promised final report. Use the reliability roadmap before reducing `num_ctx`.
- The browser lacks run cancel, cloud key entry, command endpoints, a session list, and management panels until B2/B3 land; until then some validation must fall back to the TUI.
- Open P0 bug: the server model switch rejects a listed stale `:cloud` model (owner: B1).
