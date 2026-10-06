# Next Phases Implementation Plan

**Date:** 2026-06-22 (last reviewed 2026-10-05: browser-first re-plan, see ADR-002)  
**Status:** First-read routing document for remaining v0.1 work  
**Scope:** Roadmap order, current implementation reality, source-of-truth map, and pre-start checks.  
**Important:** this file is **not** the detailed implementation guide for each phase. Agents must read this first, then use the detailed phase files in `docs/` as the implementation guide.

## Purpose

Read this document before starting any remaining implementation work. Its job is to answer:

- What should be worked on next?
- Which detailed plan owns the implementation instructions?
- What must be validated before moving to a later phase?
- What current repo facts should an agent know before reading the detailed plan?

Do not implement a phase from this file alone. This document intentionally avoids duplicating the detailed instructions in files such as `docs/plans/WEB-UI-UX-PRODUCT-PLAN.md`, `docs/phases/PHASE-25-DETAILED-PLAN.md`, `docs/phases/PHASE-17-DETAILED-PLAN.md`, and the other phase plans. Finished phase plans live in `docs/archive/`.

## Source-Of-Truth Rules

- **Roadmap order:** this file.
- **Primary surface decision:** `docs/adr/ADR-002-BROWSER-UI-PRIMARY-SURFACE.md` (browser UI primary, TUI in maintenance mode, localhost only).
- **Detailed implementation steps:** the relevant detailed phase or workstream file in `docs/`.
- **Evidence, blockers, and exit criteria for the remaining gates:** the "Release Review" sections at the end of `docs/phases/PHASE-17-DETAILED-PLAN.md` and `docs/phases/PHASE-18-DETAILED-PLAN.md`, and the "Workstream CL Gate" section at the end of `docs/plans/CONTEXT-LATENCY-OPTIMIZATION-PLAN.md`. (The former `REMAINING-PHASES-TASK-REVIEW.md` was folded into these and archived on 2026-10-06.)
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
- No detached server session, session persistence, or reconnect gap handling was found. (`connect`, `remote_bridge.go`, JWT, and the bridge UDS listener were removed from scope by ADR-002.)
- Browser server routes today: session create/get/delete, SSE events, message POST, permission resolution, model switch, file tree, model list, health. There is no run cancel route, no cloud-key entry, and no command/panel endpoints.

## Roadmap Order

As of 2026-10-05 the **browser UI is the primary v0.1 surface** ([ADR-002](../adr/ADR-002-BROWSER-UI-PRIMARY-SURFACE.md)). The TUI stays shipped in maintenance mode, plain `nandocodego` will start the server and open the browser, and v0.1 is localhost only.

Implement remaining work in this order:

1. **B0 - Browser-first docs and decisions:** ADR-002, this roadmap, backlog, plan rescopes, stray-file cleanup. (Landed on the `update-documentation` branch.)
2. **B1 - P0 bug and quick hardening:** fix the cloud-model switch bug (P0), add security-header tests, move the file-tree endpoint onto `tools.ResolvePath` + `dirwalk.Walk`, make `--print` fail fast on a malformed config (decided 2026-10-06).
3. **B2 - Browser parity blockers:** split the served page into embedded static files (plain JS, no build tools); run stop/cancel; cloud API key entry; small endpoints for clear, compact, index build/status, and cost; session list.
4. **Phase 25 (rescoped) - Browser Session Durability:** detach/reattach, replay with gap detection, persisted session metadata, detached-session cleanup. No JWT, no `connect`, no TUI bridge.
5. **B3 - Browser management panels:** permissions, tasks, memory, prompt inspector, skills, hooks (one at a time, each with backend tests).
6. **B4 - Browser accessibility and hardening:** ARIA roles, keyboard support, reduced motion, and a CSP without `'unsafe-inline'` once the page is split. (The Host/Origin/JSON request guard already exists in `internal/server/auth.go` `NewRequestGuard`, covered by `TestRequestGuard`.)
7. **Carry-forward validation evidence:** Gate G0 and Workstream CL/PA, run through the browser where the browser exposes the feature; TUI/CLI only where it does not.
8. **Phase 17 - Distribution and Install** (includes the default-command change and browser-first first run).
9. **Phase 18 - Hardening, Eval Suite, and Docs.**

Phase 22 is accepted as-is: the TUI is in maintenance mode, and its remaining deep-interaction follow-ups are parked in the backlog.

Phase 23 OpenAI-compatible adapter work is removed from the active v0.1 roadmap unless the roadmap decision is explicitly reversed. The Ollama Cloud workstream must stay scoped to Ollama's documented direct API at `https://ollama.com`.

## Read-Next Map

Use this table to decide what to read after this file. The detailed plan column owns implementation details.

| Step | Status | Detailed plan to use | Extra required inputs |
| --- | --- | --- | --- |
| B0 - Browser-first docs | Done (2026-10-05) | `docs/adr/ADR-002-BROWSER-UI-PRIMARY-SURFACE.md` | This file; `docs/roadmap/BACKLOG.md` |
| B1 - P0 bug and quick hardening | Next | `docs/reports/bugs/BUG-20260607-server-model-endpoint-rejects-listed-cloud-model.md`; `docs/plans/WEB-UI-UX-PRODUCT-PLAN.md` slices UI-5 and UI-8 | `internal/server`, `internal/llm/modelresolver`, `internal/modelruntime`, `internal/tools/dirwalk` |
| B2 - Browser parity blockers | Not started | `docs/plans/WEB-UI-UX-PRODUCT-PLAN.md` section "Browser-First P0 Additions (2026-10-05)" | `docs/plans/OLLAMA-CLOUD-API-KEY-PLAN.md` for credential consent rules |
| Phase 25 - Browser Session Durability (rescoped) | Not implemented | `docs/phases/PHASE-25-DETAILED-PLAN.md` (read the 2026-10-05 rescope section first) | `internal/server/ringbuffer.go`, `recentids.go`, `session.go` |
| B3 - Browser management panels | Not started | `docs/plans/WEB-UI-UX-PRODUCT-PLAN.md` slice UI-7 | Owning packages per panel |
| B4 - Browser accessibility and hardening | Partially implemented | `docs/plans/WEB-UI-UX-PRODUCT-PLAN.md` slice UI-8 | `internal/server/server.go` `securityHeaders` |
| Gate G0 - Phases 8-14 validation | Pending evidence | `docs/roadmap/GATE-G0-PHASE-8-14-VALIDATION-PLAN.md` | Phase docs 8-14; `docs/phases/PHASE-LOG.md` |
| Workstream CL/PA evidence gate | Foundations implemented; live evidence pending | `docs/plans/CONTEXT-LATENCY-OPTIMIZATION-PLAN.md`, `docs/plans/PROMPT-ACCURACY-AND-CONTEXT-FIDELITY-PLAN.md` | `docs/reports/investigations/INCOMPLETE-RESPONSE-RECOVERY-REPORT.md`; `docs/reports/investigations/INACCURATE-LISTING-RESPONSE-DEEP-DIVE-2026-05-17.md`; `docs/plans/LISTING-PROMPT-DRIFT-REMOVAL-PLAN-2026-05-17.md`; `docs/plans/REGRESSION-AND-LOAD-TEST-PLAN.md` |
| Phase 22 - Enhanced TUI and Input Handling | Accepted; TUI in maintenance mode | `docs/archive/phases/PHASE-22-DETAILED-PLAN.md` | `docs/adr/ADR-001-TUI-USER-EXPERIENCE-IMPROVEMENTS.md` (historical) |
| Phase 21 - Web Interface and HTTP API | Complete | `docs/phases/PHASE-21-DETAILED-PLAN.md` | Server baseline for B1-B4 and Phase 25 |
| Phase 24 - Multi-Agent Coordination | Complete | `docs/archive/phases/PHASE-24-DETAILED-PLAN.md` | Current Phase 11/14/15 code |
| Ollama Cloud API key support | Implemented (2026-05-22) | `docs/plans/OLLAMA-CLOUD-API-KEY-PLAN.md` | Browser key entry is new work in B2 |
| Phase 28 - Semantic Workspace Index And Embedding Retrieval | Implemented MVP | `docs/phases/PHASE-28-DETAILED-PLAN.md` | Browser index controls are new work in B2 |
| Phase 29 - TUI Semantic Index Progress Observability | Implemented MVP | `docs/archive/phases/PHASE-29-DETAILED-PLAN.md` | Browser already renders semantic events; index build/status endpoints are B2 |
| Evaluation framework | Implemented (2026-06-25) | `docs/plans/EVALUATION-FRAMEWORK-DETAILED-PLAN.md` | Live-model runs and larger fixture sets feed Phase 18 |
| Phase 17 - Distribution and Install | Not implemented; penultimate | `docs/phases/PHASE-17-DETAILED-PLAN.md` (read the 2026-10-05 note) | All steps above accepted |
| Phase 18 - Hardening, Eval Suite, and Docs | Not implemented; final release gate | `docs/phases/PHASE-18-DETAILED-PLAN.md` (read the 2026-10-05 note) | Completed Phase 17; `docs/plans/REGRESSION-AND-LOAD-TEST-PLAN.md` |

## Cross-Phase Rules

- The browser UI is the primary surface. New user-facing capability lands in the browser first; the TUI only gets bug fixes and regression fixes.
- Do not add a generic slash-command HTTP bridge. Each browser capability gets a small, tested endpoint that reuses the owning package.
- v0.1 server mode is localhost only and keeps the generated opaque bearer token. Do not add JWT, remote access, or `nandocodego connect`.
- The browser frontend stays plain JS with no build tools; split it into embedded static files under `internal/server/web/`. Do not add an npm toolchain.
- Do not start Phase 17 until B1-B4, Phase 25 (rescoped), Gate G0, and Workstream CL/PA are complete and accepted.
- Do not start Phase 18 until Phase 17 is complete and accepted.
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

Most foundations are implemented, but live evidence is still required before Phase 17:

- small/medium/large `/trace last` evidence;
- context mode and model-limit behavior evidence;
- memory recall `fast` versus `llm` comparison;
- checkpoint and final-answer completeness evidence;
- large analysis evidence with cache/ledger behavior;
- listing prompt evidence for `@docs/`, `@docs?content`, `review @docs/`, and `summarize @docs/`.

Known implementation nuance: `BuildProjectAnalysisPrompt` currently uses heuristic local signal-line summaries, not true LLM map/reduce summarization. Treat it as a bounded workflow foundation unless a later implementation upgrades it and records evidence.

Known implementation risk: the analysis workflow currently ignores summary-cache and evidence-ledger write errors in the prompt-building path. The detailed CL/PA or Phase 18 work should decide whether to surface, test, or explicitly accept this behavior.

### Open E2E Bugs

The 2026-06-07 E2E run filed four bugs; status is tracked in `docs/roadmap/BACKLOG.md` §1. The server cloud-model switch bug was retested on 2026-10-05 and is **still reproducible** for stale local `:cloud` tags; it is P0 and owned by B1. Of the other three, two were verified fixed in source on 2026-10-06 and archived; the malformed-config bug is decided as fail-fast and owned by B1.

### Phase 22

Accepted as-is on 2026-10-05; the TUI is in maintenance mode (ADR-002). Landed: P22-A safety slice, P22-B run visibility, P22-C style roles, P22-D partials (tool elapsed time, `/queue`), transcript render caching, `/bg` and `/btw`. Not implemented and now parked in the backlog: hierarchical activity display, click-to-expand tool panels, mouse lost-release recovery, full textarea Vim, concurrent `/btw`.

### Phase 21

Complete as of 2026-05-19. Use `docs/phases/PHASE-21-DETAILED-PLAN.md` and `docs/phases/PHASE-LOG.md` for validation evidence. It is the baseline for all browser-first work (B1-B4, Phase 25).

### Phase 24

Phase 24 is complete. Source now includes coordinator mode, mailbox routing, `SendMessage`, dream lifecycle primitives, worker registry restrictions, coordinator TUI status, server coordinator runtime, and automated tests. Use `docs/archive/phases/PHASE-24-DETAILED-PLAN.md` and `docs/phases/PHASE-LOG.md` for implementation details and validation evidence.

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

Rescoped on 2026-10-05 to **Browser Session Durability** (ADR-002). Keep: detach/reattach, event replay with gap detection (reuse `RingBuffer` and `RecentIDs`), persisted session metadata so a session list survives a server restart, detached-session cleanup. Removed: `nandocodego connect`, `internal/tui/remote_bridge.go`, JWT, the bridge UDS listener, and `prctl` remote hardening.

### Phase 17 And Phase 18

Phase 17 packages a stable product surface. It must not absorb unfinished feature work. It now also owns the default-command change (plain `nandocodego` starts the server and opens the browser; the TUI moves behind `nandocodego tui`) and the browser-first first-run flow. Phase 18 is the final hardening, eval, docs, security, and release-approval gate. No later v0.1 implementation phase should be planned after Phase 18.

## Design Rules To Keep In Mind

The detailed phase files contain deeper analysis, and `docs/architecture/ARCHITECTURE.md` lists these rules with context. Cross-phase reminder:

- Keep the agent event/generator loop as the integration spine for the browser/server, TUI, and `--print` surfaces.
- Keep bootstrap/session facts separate from reactive UI state.
- Convert external input into typed internal events early: terminal input, HTTP requests, MCP payloads, hook decisions, and mailbox entries.
- Make tool, permission, hook, MCP, and HTTP server boundaries fail closed.
- Make context management explicit through budgets, prompt packing, evidence, summaries, caches, and checkpoints.
- Keep streaming render paths cheap and bounded.
- Restrict worker agents by role and tool access; do not let coordinator privileges leak into workers.

## Standard Pre-Start Checklist

Before implementing a remaining phase:

1. Read this file.
2. Read the detailed phase/workstream plan listed in the Read-Next Map.
3. Read the evidence, blocker, and exit-criteria section of that plan (for Phase 17/18 the "Release Review" section; for CL/PA the "Workstream CL Gate" section).
4. Inspect current source in the packages the detailed plan names.
5. Run or confirm baseline tests relevant to the phase.
6. Update `docs/phases/PHASE-LOG.md` when the phase or validation slice completes.

For source review and test work, prefer current code and tests over stale prose. The detailed plans guide implementation, but the repo decides what is already true.
