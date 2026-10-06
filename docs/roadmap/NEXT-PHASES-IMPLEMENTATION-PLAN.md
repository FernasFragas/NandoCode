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

- **Document hierarchy (2026-10-06):** this file decides *what* is built and in *which order*; the detailed phase or plan file (for browser work, `docs/plans/WEB-UI-UX-PRODUCT-PLAN.md`) decides *how*; investigation reports (for example the 2026-10-06 code review) are *evidence* only and are never followed directly; `docs/roadmap/BACKLOG.md` holds only work that is not scheduled here. If two documents disagree, this order wins and the lower document gets fixed.
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
2. **B1 - P0 bugs and security hardening.** Every item is test-first: write the failing test named here, then fix. IDs refer to `docs/reports/investigations/CODE-COMPLEXITY-AND-ARCHITECTURE-REVIEW-2026-10-06.md`, which holds the evidence.
   - Cloud-model switch bug (`/v1/models` lists a model the switch rejects).
   - Per-session model runtime (P0-3): each server session gets its own `llm.RuntimeClient` and switches only when its model changes. Test: two sessions on different models.
   - Browser keeps the conversation across turns (P0-1): append each run's messages, including the user prompt, instead of replacing history. Test: two `StartRun` calls; the second sees `[Q1, A1, Q2]`.
   - Markdown XSS (P0-2): escape quotes, allow only http/https/mailto links. Test: rendered output has no `on*=` attributes and no other link schemes. This needs the renderer as a DOM-free module tested with `node --test`; add that CI step here (no npm, decided 2026-10-06).
   - Embeddings stay local (P1-5): bind `semantic.LLMEmbedder` to the local Ollama client, not the runtime router. Test: after a switch to a fake cloud client, zero cloud embed calls.
   - Browser "Always Allow" persists (P1-6): add a rule that `permissions` actually matches (shared `permissions.SessionAllowRule` with the TUI). Test: a second run with the same tool and target emits no `permission_request`.
   - `@dir/` evidence cannot follow symlinks outside the workspace (P0-4): per-file `tools.ResolvePath` in `contextpack`. Test: `TestPackCurrentTurnPromptDirectoryMentionDoesNotFollowSymlinkOutsideRoots`.
   - Bash read-only classification checks arguments (P0-6): `find -delete`, `env rm`, `sort -o`, `git branch -D`, `go env -w` must not be auto-allowed; classify the inner command of wrapper commands. Test: new rows in `TestBashPermissionMatrix`.
   - Sub-agents can never be looser than their parent (P0/P1-7a): clamp the requested permission mode and pass the parent's rules and hooks. Test: parent in plan mode, sub-agent requests bypass, Bash write is denied or prompted.
   - Project MCP config cannot mark itself trusted (P0/P1-7b): ignore `trusted` from `.nandocodego/config.toml` and warn. Test: project config `trusted = true` gives `Trusted=false` plus a warning.
   - Permission-rule `*` matches any characters including `/` (decided 2026-10-06; P0/P1-7c), and `**` gets tests. Test: `Bash(cat *)` deny rule blocks `cat a/b`. Document that allow rules become broader.
   - Codebase questions keep their tools (P0-5): split the tool-mode decision from the retrieval decision in `retrievalroute`, match whole words. Test: the 11-row `TestDecideToolModeAndAction` table plus the never-`ToolModeNone` invariant.
   - Shared `internal/netguard` for outbound HTTP safety: block `0.0.0.0` and unmapped IPv6 forms, dial the validated IP (no DNS rebinding), re-check redirects, block `localhost` names, cap HTML bodies; used by MCP, HTTP hooks, and webfetch (removes the `hooks` → `mcp` dependency). Test: `0.0.0.0`, a redirect to `127.0.0.1`, and a `localhost` URL are all rejected.
   - Security-header tests; file-tree endpoint onto `tools.ResolvePath` + `dirwalk.Walk`; `--print` fails fast on a malformed config (decided 2026-10-06).
3. **B1.5 - Surface-neutral extraction.** Move the logic B2 needs out of the TUI and the duplicated composition roots, so B2 endpoints are thin. Characterization tests first for every move; behavior must not change. Details: review report, "Composition Roots And Duplication".
   - `internal/turnprep`: one turn-preparation function (pack, route, semantic retrieval, `agent.Input`) used by the TUI, server, `--print`, and eval. Test first: a prompt table capturing today's `agent.Input` from each path.
   - `bootstrap.ApplyConfig`: one config-to-runtime mapping for all surfaces (the server maps 13 of 26 fields today). Test: every config field lands.
   - `modelruntime.Activate`: model switch plus limits refresh, returning a credential-required error instead of prompting (needed for B2 cloud key entry).
   - `runctl`: run cancel and compact control (needed for B2 Stop).
   - `ApplyTerminal` (end-of-run history, usage, checkpoint) and reusable `/clear` and `/compact` state operations.
   - Agent event invariants: an `assertEventInvariants` test helper, then fixes so every tool result has an earlier start, results keep call order, there is exactly one `Terminal`, and `ToolUseProgress` is emitted (review P1-2). Make `TestAgentRunProgressEvents` and `TestSupervisorStop` real tests (today they cannot fail).
   - Runtime bugs found by the review, each with a RED test: the watchdog timeout cancels the upstream Ollama request (P1-3); `Supervisor.Start` rollback leaves no phantom task and cancels its context; coordinator workers can use tools; the prompt packer never keeps a tool result without its tool call; the server closes the MCP manager on shutdown.
   - Grounding for normal runs (after the B1 routing fix): a default grounding system prompt in `turnprep`, a local-search fallback for uncertain code questions, and visible evidence state. Measure with a new eval fixture using the hallucination-investigation prompt.
   - Dispatch the `PreCompact` / `PostCompact` hook events from the shared compact operation (a `PreCompact` deny skips compaction).
   - Move `internal/tui/fileindex` to a surface-neutral package (pure move; `analysis` imports it today, and browser `@` completion will need it).
   - Characterization tests for `internal/memory` recall, prompt section, store, and scan (40% coverage; runs on every prompt) while wiring it through `turnprep`.
   - Small bugs, each with a RED test: `grep` silently stops at lines over 64 KB and searches nested `node_modules`; MCP config treats `#` inside quoted values as a comment; `sendmessage` has an unchecked type assertion that can panic the agent goroutine.
4. **C1 - Core cleanup (after B1.5; may run in parallel with B2; must land before Phase 25).** Behavior-preserving refactors on top of the B1.5 safety net.
   - `agent.run` (review P1-1): a `runState` struct, a `loopAction` enum instead of `turn--; continue`, and a `turnRequest` struct instead of 21 positional parameters. About 350 lines; three PRs.
   - Context pipeline: one `@`-mention tokenizer (four copies today), split `contextpack.buildEvidenceParts`, run eval fixtures through `turnprep`, wire the observability run trace into the server.
   - Token estimation calibration (decided 2026-10-06): after each run, compare the 4-chars/token estimate with Ollama's `prompt_eval_count` and keep a per-model ratio that budgets use. Test: a fake client reporting actual counts shifts the next run's estimate.
5. **B2 - Browser parity blockers:** split the served page into embedded static files (plain JS, no build tools); run stop/cancel; cloud API key entry; small endpoints for clear, compact, index build/status, and cost; session list.
   - Go 1.22 pattern routing for `/v1/sessions/...` with a route-table test through `routes()`, so each new endpoint is one line.
   - Browser session bugs: "New Session" deletes or reuses the old session (no hitting the 10-session cap); the permission modal closes when the broker times out.
   - With the index endpoints: `semantic.Refresh` re-embeds after an embedding-model change (P1-9); `Status` reads only the manifest (P1-8).
   - Small parser bugs, each with a RED test: `@main.go?` resolves the file; skill bodies with lines over 4 KB load; excerpts stay valid UTF-8.
6. **Phase 25 (rescoped) - Browser Session Durability:** slice 0 reliable server event log (atomic replay+subscribe, gap event, typed event DTOs), then detach/reattach, replay with gap detection, persisted session metadata, detached-session cleanup. No JWT, no `connect`, no TUI bridge.
7. **B3 - Browser management panels:** permissions, tasks, memory, prompt inspector, skills, hooks (one at a time, each with backend tests).
   - Decided 2026-10-06: the permissions and hooks panels form one **Trust** panel (permission mode and rules, hook sources, MCP servers read-only with trust and connection status, cloud credential status, network policy, writable roots; summary also in `doctor`). The tasks panel becomes an **Activity** view (run phase, tools, sub-agents, tasks, queue, permission waits, index activity) together with the status bar. MCP server editing from the browser is post-launch.
   - First split `internal/commands` into typed query functions plus text formatters (a pure-move file split first, over 500 lines), so panels JSON-encode the same data the TUI prints.
8. **B4 - Browser accessibility and hardening:** ARIA roles, keyboard support, reduced motion, and a CSP without `'unsafe-inline'` once the page is split. (The Host/Origin/JSON request guard already exists in `internal/server/auth.go` `NewRequestGuard`, covered by `TestRequestGuard`.)
9. **B5 - Proof Mode / shareable run report (launch differentiator, decided 2026-10-06).** Export a redacted Markdown record of a run (prompt, plan, tools, files changed, tests run, permission decisions, open risks) from the browser and via `/run-report last`, built on prompt dumps, the run trace, and the B1.5 event invariants. **Write `docs/plans/PROOF-MODE-RUN-REPORT-PLAN.md` before starting** (scope, report contents, redaction rules, test-first slices, acceptance).
10. **Carry-forward validation evidence:** Gate G0 and Workstream CL/PA, run through the browser where the browser exposes the feature; TUI/CLI only where it does not.
   - CL/PA also records two analysis decisions (2026-10-06): heuristic `BuildProjectAnalysisPrompt` summaries are an accepted v0.1 limitation (documented in Phase 18), and summary-cache/ledger write errors get a test and a logged warning instead of being ignored.
11. **Phase 17 - Distribution and Install** (includes the default-command change to the browser with the TUI at `nandocodego tui`, and a browser first run that asks the user to pick a model).
12. **Phase 18 - Hardening, Eval Suite, and Docs.**

Phase 22 is accepted as-is: the TUI is in maintenance mode, and its remaining deep-interaction follow-ups are parked in the backlog.

Phase 23 OpenAI-compatible adapter work is removed from the active v0.1 roadmap unless the roadmap decision is explicitly reversed. The Ollama Cloud workstream must stay scoped to Ollama's documented direct API at `https://ollama.com`.

## Read-Next Map

Use this table to decide what to read after this file. The detailed plan column owns implementation details.

| Step | Status | Detailed plan to use | Extra required inputs |
| --- | --- | --- | --- |
| B0 - Browser-first docs | Done (2026-10-05) | `docs/adr/ADR-002-BROWSER-UI-PRIMARY-SURFACE.md` | This file; `docs/roadmap/BACKLOG.md` |
| B1 - P0 bugs and security hardening | Next | `docs/reports/investigations/CODE-COMPLEXITY-AND-ARCHITECTURE-REVIEW-2026-10-06.md` (P0 section); `docs/reports/bugs/BUG-20260607-server-model-endpoint-rejects-listed-cloud-model.md`; `docs/plans/WEB-UI-UX-PRODUCT-PLAN.md` slices UI-5 and UI-8 | `internal/server`, `internal/llm/modelresolver`, `internal/modelruntime`, `internal/tools/dirwalk` |
| B1.5 - Surface-neutral extraction | Not started; after B1 | `docs/reports/investigations/CODE-COMPLEXITY-AND-ARCHITECTURE-REVIEW-2026-10-06.md` (Composition Roots And Duplication; P1-2) | `internal/tui/app.go`, `internal/server/session.go`, `internal/cli/{repl,print}.go`, `internal/agent` |
| C1 - Core cleanup | Not started; after B1.5, before Phase 25 | `docs/reports/investigations/CODE-COMPLEXITY-AND-ARCHITECTURE-REVIEW-2026-10-06.md` (P1-1; context pipeline) | `internal/agent`, `internal/contextpack`, `internal/mentions`, `internal/eval` |
| B2 - Browser parity blockers | Not started; after B1.5 | `docs/plans/WEB-UI-UX-PRODUCT-PLAN.md` section "Browser-First P0 Additions (2026-10-05)" | `docs/plans/OLLAMA-CLOUD-API-KEY-PLAN.md` for credential consent rules |
| Phase 25 - Browser Session Durability (rescoped) | Not implemented | `docs/phases/PHASE-25-DETAILED-PLAN.md` (read the 2026-10-05 rescope section first) | `internal/server/ringbuffer.go`, `recentids.go`, `session.go` |
| B3 - Browser management panels | Not started | `docs/plans/WEB-UI-UX-PRODUCT-PLAN.md` slice UI-7 | Owning packages per panel |
| B4 - Browser accessibility and hardening | Partially implemented | `docs/plans/WEB-UI-UX-PRODUCT-PLAN.md` slice UI-8 | `internal/server/server.go` `securityHeaders` |
| B5 - Proof Mode / run report | Not started; plan to be written | `docs/plans/PROOF-MODE-RUN-REPORT-PLAN.md` (to write first) | prompt dumps, `internal/observability`, agent event invariants |
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
- When you modify a test file, replace its `time.Sleep` synchronization and wall-clock assertions with channels or the existing wait helpers (`waitForStatus`, `blockingRunner`).
- Do not start Phase 25 until C1 is accepted.
- Do not start B2 until B1.5 is accepted: B2 endpoints call the extracted packages instead of copying TUI logic.
- Do not start Phase 17 until B1-B5 (including B1.5 and C1), Phase 25 (rescoped), Gate G0, and Workstream CL/PA are complete and accepted.
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
