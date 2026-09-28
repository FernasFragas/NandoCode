# Next Phases Implementation Plan

**Date:** 2026-06-22 (last reviewed 2026-09-28)  
**Status:** First-read routing document for remaining v0.1 work  
**Scope:** Roadmap order, current implementation reality, source-of-truth map, and pre-start checks.  
**Important:** this file is **not** the detailed implementation guide for each phase. Agents must read this first, then use the detailed phase files in `docs/` as the implementation guide.

## Purpose

Read this document before starting any remaining implementation work. Its job is to answer:

- What should be worked on next?
- Which detailed plan owns the implementation instructions?
- What must be validated before moving to a later phase?
- What current repo facts should an agent know before reading the detailed plan?

Do not implement a phase from this file alone. This document intentionally avoids duplicating the detailed instructions in files such as `docs/phases/PHASE-22-DETAILED-PLAN.md`, `docs/phases/PHASE-21-DETAILED-PLAN.md`, `docs/phases/PHASE-24-DETAILED-PLAN.md`, and the other phase plans.

## Source-Of-Truth Rules

- **Roadmap order:** this file.
- **Detailed implementation steps:** the relevant detailed phase or workstream file in `docs/`.
- **Reviewed task breakdown and blockers:** `docs/roadmap/REMAINING-PHASES-TASK-REVIEW.md`.
- **Implementation history and acceptance evidence:** `docs/phases/PHASE-LOG.md`.
- **Current project status and onboarding context:** `docs/roadmap/PROJECT-STATUS-AND-ONBOARDING.md`.
- **Architecture reference:** `docs/architecture/ARCHITECTURE.md` (overview and package map) and `docs/architecture/APPLICATION-ARCHITECTURE-FLOWCHART.md` (diagrams).
- **Ideas, bugs, and deferred work not on this roadmap:** `docs/roadmap/BACKLOG.md`.
- **Actual implementation reality:** current source code and tests. If docs and source disagree, inspect source and update docs rather than guessing.

Non-authoritative references:

- `book/` chapters (`book/chNN-*.md`) and `.codex/` plans are cited throughout the phase docs as design references, but **neither is in the repository**. `.codex/` is gitignored local material and `book/` is not distributed. Treat those citations as background; the phase plans and source code are self-sufficient.

- `README.md` is a user-facing overview. It should route here for current roadmap details rather than duplicating launch status.
- `.codex/go-ollama-plan-AGENTS.md` and `.codex/go-ollama-plan-HUMANS.md` are historical plans. Use them for architecture context only after checking this file and the project-status document.
- `.codex/agent-context/ARCHITECTURE.md` is deprecated for this repository. `.codex/agent-context/testing-standards.md` is generic guidance only where it agrees with current Go tests and the Makefile.

## Current Implementation Snapshot

Latest recorded implementation review checked the current `docs/`, `book/`, and `internal/` implementation. Automated checks run during that review:

```bash
go test ./...
tools/check-allowed-deps.sh
tools/check-network-policy.sh
```

All three passed in that recorded review. `docs/phases/PHASE-LOG.md` also records targeted validation for later completed slices, including Phase 28, Phase 29, and the response-time refactor.

Current source reality:

- Core local agent stack exists: CLI, TUI, Ollama client, agent loop, tools, permissions, state, memory, hooks, MCP, sub-agents, skills, slash commands/config, background tasks, concurrency, observability, compaction, inline completion, directory mentions, prompt dump/packing, context modes, checkpoint, retrieval, and analysis workflow foundations.
- `internal/analysis` exists with chunking, cache, ledger, checkpoint, retrieval, and `BuildProjectAnalysisPrompt`.
- `internal/tui` has `/analyze-project`, `/trace`, `/prompt`, `/checkpoint`, file picker/indexing, listing safeguards, slow-stage notices, retry/compaction transcript items, and a basic status bar.
- Phase 21 is complete as of 2026-05-19: server package, `nandocodego server`, HTTP/SSE session manager, browser UI, HTTP permission broker, Docker runtime validation, and live API checks are implemented.
- Phase 24 is complete as of 2026-05-19: coordinator mode, `SendMessage`, bounded mailboxes, worker name registration, auto-resume hooks, dream lifecycle, restricted worker/coordinator registries, TUI coordinator status, and server coordinator runtime are implemented.
- Ollama Cloud direct API support with API-key prompting is implemented (2026-05-22).
- Phase 28 semantic workspace indexing is implemented as an MVP.
- Phase 29 TUI semantic index progress observability is implemented as an MVP.
- The Go response-time refactor is implemented and validated: common prompts have a chat-only fast path, semantic retrieval uses cache/light narrowing, context-pack file reads and index scanning are bounded-parallel, and TUI transcript/picker paths are optimized.
- Evaluation framework is implemented (2026-06-25): `nandocodego eval run|validate`, `internal/eval`, five fixtures under `evals/`, `make eval|eval-ci|eval-validate`, and a deterministic eval CI job. It replaces the narrow eval-runner design in the Phase 18 plan; see `docs/plans/EVALUATION-FRAMEWORK-DETAILED-PLAN.md`.
- CI exists in `.github/workflows/ci.yml` (allowlist, network policy, build/vet/race on Linux/macOS/Windows, lint, dependency review, deterministic evals) and `security.yml` (gosec report-only, govulncheck, container scan).
- No Phase 25 `connect` command, `internal/tui/remote_bridge.go`, detached server session, JWT auth, reconnect/replay, or bridge UDS listener was found.

## Roadmap Order

The next implementation phase is Phase 25 Remote / Bridge Mode. Earlier validation gates remain important pre-release evidence work, but they are not new feature phases.

Implement remaining work in this order:

1. **Carry-forward validation evidence:** Gate G0, Workstream CL/PA, and Phase 22 live/manual evidence.
2. **Phase 25 - Remote / Bridge Mode**
3. **Phase 17 - Distribution and Install**
4. **Phase 18 - Hardening, Eval Suite, and Docs**

Phase 17 and Phase 18 are intentionally last. Phase 25 remains required v0.1 work and must land before Phase 17. Phase 24, Phase 28, Phase 29, and Ollama Cloud API key support are complete dependencies for Phase 25.

Phase 23 OpenAI-compatible adapter work is removed from the active v0.1 roadmap unless the roadmap decision is explicitly reversed. The Ollama Cloud workstream must stay scoped to Ollama's documented direct API at `https://ollama.com`.

## Read-Next Map

Use this table to decide what to read after this file. The detailed plan column owns implementation details.

| Step | Status | Detailed plan to use | Extra required inputs |
| --- | --- | --- | --- |
| Gate G0 - Phases 8-14 validation | Next validation gate | `docs/roadmap/GATE-G0-PHASE-8-14-VALIDATION-PLAN.md` | Phase docs `docs/phases/PHASE-8-DETAILED-PLAN.md` through `docs/phases/PHASE-14-DETAILED-PLAN.md`; `docs/phases/PHASE-14-EXIT-GATE.md`; `docs/phases/PHASE-LOG.md` |
| Workstream CL/PA evidence gate | Foundations implemented; live evidence pending | `docs/plans/CONTEXT-LATENCY-OPTIMIZATION-PLAN.md`, `docs/plans/PROMPT-ACCURACY-AND-CONTEXT-FIDELITY-PLAN.md` | `docs/reports/investigations/INCOMPLETE-RESPONSE-RECOVERY-REPORT.md`; `docs/reports/investigations/INACCURATE-LISTING-RESPONSE-DEEP-DIVE-2026-05-17.md`; `docs/plans/LISTING-PROMPT-DRIFT-REMOVAL-PLAN-2026-05-17.md`; `docs/plans/REGRESSION-AND-LOAD-TEST-PLAN.md` |
| Phase 22 - Enhanced TUI and Input Handling | Core implemented; manual/follow-up gate open | `docs/phases/PHASE-22-DETAILED-PLAN.md` | `docs/adr/ADR-001-TUI-USER-EXPERIENCE-IMPROVEMENTS.md`; `docs/plans/TASKS-TUI.md`; `book/ch13-terminal-ui.md`; `book/ch14-input-interaction.md`; `book/ch17-performance.md` |
| Phase 21 - Web Interface and HTTP API | Complete | `docs/phases/PHASE-21-DETAILED-PLAN.md` | `book/ch04-api-layer.md`; `book/ch05-agent-loop.md`; `book/ch16-remote.md`; post-Phase-22 event/status model |
| Phase 24 - Multi-Agent Coordination | Complete | `docs/phases/PHASE-24-DETAILED-PLAN.md` | `book/ch08-sub-agents.md`; `book/ch09-fork-agents.md`; `book/ch10-coordination.md`; current Phase 11/14/15 code |
| Ollama Cloud API key support | Implemented (2026-05-22) | `docs/plans/OLLAMA-CLOUD-API-KEY-PLAN.md` | Official Ollama Cloud docs; current Ollama client, command registry, TUI prompt flow, config/bootstrap/state, print mode, and server mode |
| Phase 28 - Semantic Workspace Index And Embedding Retrieval | Implemented MVP | `docs/phases/PHASE-28-DETAILED-PLAN.md` | Current `llm.Client.Embed`, Ollama `/api/embed`, `internal/analysis`, `internal/tools/dirwalk`, TUI/server prompt paths, local cache/state paths |
| Phase 29 - TUI Semantic Index Progress Observability | Implemented MVP | `docs/phases/PHASE-29-DETAILED-PLAN.md` | Phase 28 semantic service/event sink, `internal/tui` status rendering, `/index build`, `/index refresh` |
| Evaluation framework | Implemented (2026-06-25) | `docs/plans/EVALUATION-FRAMEWORK-DETAILED-PLAN.md` | `internal/eval`, `evals/`, `docs/phases/PHASE-LOG.md` entry; live-model runs and larger fixture sets feed Phase 18 |
| Phase 25 - Remote / Bridge Mode | Not implemented | `docs/phases/PHASE-25-DETAILED-PLAN.md` | `book/ch16-remote.md`; Phase 21 server implementation; Phase 24 coordination implementation; Phase 28/29 semantic index behavior |
| Phase 17 - Distribution and Install | Not implemented; penultimate | `docs/phases/PHASE-17-DETAILED-PLAN.md` | Completed and accepted Gate G0, CL/PA, Phases 22/21/24, Ollama Cloud API key support, Phase 28, Phase 29, and Phase 25 |
| Phase 18 - Hardening, Eval Suite, and Docs | Not implemented; final release gate | `docs/phases/PHASE-18-DETAILED-PLAN.md` | Completed Phase 17; `docs/plans/REGRESSION-AND-LOAD-TEST-PLAN.md`; all phase logs and acceptance evidence |

## Cross-Phase Rules

- Phase 25 is the next P0 feature implementation stage.
- Phase 25 can proceed; Phase 28, Phase 29, and Ollama Cloud API key support are complete.
- Do not start Phase 17 until Gate G0, Workstream CL/PA, Phase 22, Phase 21, Phase 24, Phase 28, Phase 29, and Phase 25 are complete and accepted.
- Do not start Phase 18 until Phase 17 is complete and accepted.
- Treat remaining Phase 22 manual/deep-interaction items as release validation/follow-up work; do not use them to reimplement already-complete server or coordinator foundations.
- Do not add Phase 23/OpenAI-compatible adapter work unless the roadmap explicitly changes.
- Do not generalize the Ollama Cloud workstream into a multi-provider adapter. It is Ollama-only and must keep local models as the default.
- Do not solve latency by globally lowering `num_ctx`; use the context/latency plan's adaptive context, prompt packing, trace data, retrieval, cache, checkpoint, and workflow mechanisms.
- Do not add listing-only answer constraints to listing prompts. The current prompt-fidelity decision is to preserve the user's request and attach tree/content context accurately.
- Every phase must update `docs/phases/PHASE-LOG.md` with files changed, tests run, manual checks, known constraints, and exit-gate status.
- If a detailed phase plan conflicts with the current source code, inspect the implementation and update the docs. Do not hallucinate missing behavior.

## Current Blockers And Carry-Forward Notes

These notes are here so agents enter the detailed plans with the current repo reality in mind. They do not replace the detailed plans.

### Gate G0

Phases 8-14 have substantial implementation and automated coverage, but the documented live/manual exit gates still need pass/fail/blocked evidence. Use `docs/roadmap/GATE-G0-PHASE-8-14-VALIDATION-PLAN.md`.

### Workstream CL/PA

Most foundations are implemented, but live evidence is still required before Phase 22:

- small/medium/large `/trace last` evidence;
- context mode and model-limit behavior evidence;
- memory recall `fast` versus `llm` comparison;
- checkpoint and final-answer completeness evidence;
- large analysis evidence with cache/ledger behavior;
- listing prompt evidence for `@docs/`, `@docs?content`, `review @docs/`, and `summarize @docs/`.

Known implementation nuance: `BuildProjectAnalysisPrompt` currently uses heuristic local signal-line summaries, not true LLM map/reduce summarization. Treat it as a bounded workflow foundation unless a later implementation upgrades it and records evidence.

Known implementation risk: the analysis workflow currently ignores summary-cache and evidence-ledger write errors in the prompt-building path. The detailed CL/PA or Phase 18 work should decide whether to surface, test, or explicitly accept this behavior.

### Open E2E Bugs

The 2026-06-07 E2E run filed four bugs. Three appear fixed in code but have no recorded retest, and one needs a product decision. Retest them as part of carry-forward validation; status is tracked in `docs/roadmap/BACKLOG.md` §1.

### Phase 22

Use `docs/phases/PHASE-22-DETAILED-PLAN.md` as the implementation guide. Current TUI reality from source review:

- P22-A safety slice is implemented (editor invocation safety path, permission `Esc` deny, literal-target always-allow rule scope, `/clear` transient reset hardening, modal/picker interaction fixes);
- P22-B run-visibility foundation is implemented in current branch (`RunPhase`/`RunUIState`, priority logic, tick lifecycle tests);
- P22-C semantic style roles are implemented;
- P22-D partials are implemented in current branch (visible elapsed tool time, `/queue list|clear|drop`);
- transcript render caching/windowing and picker prefilter optimization are implemented;
- `/bg` and `/btw` command surfaces are implemented, with `/btw` queued behind active runs rather than true concurrent side execution;
- hierarchical activity display is not implemented;
- full click-to-expand tool panels and mouse lost-release recovery remain follow-ups.

Phase 22 should not redo already-landed inline completion from Phase 26 unless a regression is found.

### Phase 21

Complete as of 2026-05-19. Use `docs/phases/PHASE-21-DETAILED-PLAN.md` and `docs/phases/PHASE-LOG.md` for validation evidence before building Phase 25 remote mode on top of server mode.

### Phase 24

Phase 24 is complete. Source now includes coordinator mode, mailbox routing, `SendMessage`, dream lifecycle primitives, worker registry restrictions, coordinator TUI status, server coordinator runtime, and automated tests. Use `docs/phases/PHASE-24-DETAILED-PLAN.md` and `docs/phases/PHASE-LOG.md` for implementation details and validation evidence.

### Ollama Cloud API Key Support

Implemented as of 2026-05-22. Use `docs/plans/OLLAMA-CLOUD-API-KEY-PLAN.md` for scope and `docs/phases/PHASE-LOG.md` for validation evidence.

Implementation review:

- Local Ollama remains the default.
- Selecting a cloud-only Ollama model prompts for an API key before any cloud model call can send project context.
- `OLLAMA_API_KEY` and OS keychain credentials are supported.
- `--print` and server mode fail with explicit credential-required behavior instead of blocking for input.
- Phase 23 generic OpenAI-compatible provider work remains out of scope.
- Validation evidence is recorded in `docs/plans/OLLAMA-CLOUD-API-KEY-PLAN.md` and `docs/phases/PHASE-LOG.md`.

### Phase 25

No remote bridge implementation was found in source. Use `docs/phases/PHASE-25-DETAILED-PLAN.md`. Phase 25 depends on Phase 21 server mode, Phase 24 coordination, the completed Ollama Cloud API key support slice, and the completed Phase 28/29 semantic retrieval and index-progress behavior that remote/server surfaces should expose.

### Phase 17 And Phase 18

Phase 17 packages a stable product surface. It must not absorb unfinished feature work. Phase 18 is the final hardening, eval, docs, security, and release-approval gate. No later v0.1 implementation phase should be planned after Phase 18.

## Design Rules To Keep In Mind

The detailed phase files contain deeper analysis, and `docs/architecture/ARCHITECTURE.md` lists these rules with context. Cross-phase reminder:

- Keep the agent event/generator loop as the integration spine for TUI, server, and remote work.
- Keep bootstrap/session facts separate from reactive UI state.
- Convert external input into typed internal events early: terminal input, HTTP requests, remote messages, MCP payloads, hook decisions, and mailbox entries.
- Make tool, permission, hook, MCP, and remote boundaries fail closed.
- Make context management explicit through budgets, prompt packing, evidence, summaries, caches, and checkpoints.
- Keep streaming render paths cheap and bounded.
- Restrict worker agents by role and tool access; do not let coordinator privileges leak into workers.

## Standard Pre-Start Checklist

Before implementing a remaining phase:

1. Read this file.
2. Read the detailed phase/workstream plan listed in the Read-Next Map.
3. Read `docs/roadmap/REMAINING-PHASES-TASK-REVIEW.md` for reviewed blockers and evidence requirements.
4. Inspect current source in the packages the detailed plan names.
5. Run or confirm baseline tests relevant to the phase.
6. Update `docs/phases/PHASE-LOG.md` when the phase or validation slice completes.

For source review and test work, prefer current code and tests over stale prose. The detailed plans guide implementation, but the repo decides what is already true.
