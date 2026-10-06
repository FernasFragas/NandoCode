# Backlog

Last reviewed: 2026-10-06 (docs cleanup after the browser-first re-plan, see [ADR-002](../adr/ADR-002-BROWSER-UI-PRIMARY-SURFACE.md)).

This is the single list of work that is **not** on the committed v0.1 path.
The committed path (what is being built next, in order) lives in
[NEXT-PHASES-IMPLEMENTATION-PLAN.md](NEXT-PHASES-IMPLEMENTATION-PLAN.md).

How to use this file:

- An item here is an idea or a known gap, not a commitment.
- To promote an item, add it to the roadmap with an owning plan, then remove it here or mark it `Promoted`.
- When something ships, record it in [PHASE-LOG.md](../phases/PHASE-LOG.md) and delete it here.
- Priority: **P0** = should block v0.1 release, **P1** = strong v0.1 or immediately-after candidate, **P2** = post-launch, **P3** = moonshot.

## 1. Open Bugs

From the 2026-06-07 E2E run ([master report](../reports/e2e/2026-06-07/E2E-MASTER-REPORT-2026-06-07.md)). Status was re-verified against source on 2026-10-06. Resolved bugs are archived under [archive/reports/bugs/](../archive/reports/bugs/).

| Bug | Severity | Status | Next step |
| --- | --- | --- | --- |
| [Server model endpoint rejects a listed cloud model](../reports/bugs/BUG-20260607-server-model-endpoint-rejects-listed-cloud-model.md) | sev2 high, **P0** | **Still reproducible** (retest 2026-10-05): `kimi-k2.6:cloud` now switches, but a stale local `glm-5.1:cloud` tag is listed by `/v1/models` and rejected with `model not found` because Ollama Cloud retired `glm-5.1` | Fix in roadmap step B1 |
| [Invalid config warning does not fail `--print`](../reports/bugs/BUG-20260607-invalid-config-warning-does-not-fail-print.md) | sev4 low | **Open. Decided 2026-10-06: fail fast** | Roadmap step B1: failing `--print` test with a malformed config first, then exit non-zero with actionable text. The browser should surface the same error. |

Blocked E2E scenarios B007/B008 (cloud credential gate observability) are in [reports/blocks/](../reports/blocks/).

## 2. Decisions Needed

| Decision | Why it matters | Owner input needed |
| --- | --- | --- |
| Default model | Code default is `qwen3.6:35b`, while many docs use `qwen3`. A 35B model is a heavy first-run default. | Product + engineering |
| Launch positioning | [Product brainstorm](../product/PRODUCT-BRAINSTORM-DISRUPTIVE-LAUNCH.md) recommends "the local agentic engineer" plus Proof Mode | Product |
| TUI entry point name | ADR-002 moves the TUI off plain `nandocodego`; proposed `nandocodego tui`. Confirm the name and whether `nandocodego --model X` without a subcommand should warn | Product + engineering |

Resolved 2026-10-06: **Malformed config** - `--print` fails fast (bug above, B1). **Private vulnerability reporting** - `SECURITY.md` now points to GitHub private vulnerability reporting; the maintainer must enable it in the repository settings (Security → Private vulnerability reporting) before public release.

Resolved 2026-10-05 ([ADR-002](../adr/ADR-002-BROWSER-UI-PRIMARY-SURFACE.md)): **Browser UI at launch** - the browser is the primary surface, the TUI stays in maintenance mode, plain `nandocodego` opens the browser, v0.1 is localhost only, and the frontend stays plain JS split into embedded files.

## 3. Product Features (Ideas)

Source: [PRODUCT-BRAINSTORM-DISRUPTIVE-LAUNCH.md](../product/PRODUCT-BRAINSTORM-DISRUPTIVE-LAUNCH.md), which has the scoring rationale. 60-second install from that list is committed (Phase 17). Remote/Bridge Mode was removed from v0.1 by ADR-002 (localhost only; see §6).

| Idea | Priority | Builds on |
| --- | --- | --- |
| **Agentic Engineering Proof Mode / Shareable Run Report:** `/run-report last` exports a redacted Markdown record of prompt, plan, tools, files, tests, permissions, risks | P0/P1 | Prompt dumps, trace, observability |
| **Local Trust Center:** `/trust` shows permission mode, rules, network policy, MCP servers, hook sources, cloud/credential status, writable roots; summary in `doctor` | P0/P1 | Permissions, hooks, MCP, credentials |
| **Agent Mission Control:** one view of active run phase, tools, sub-agents, tasks, queue, permission waits, index activity | P0/P1 | Run state, tasks, coordinator |
| **Prompt/Context Inspector:** user-facing view of what was included, summarized, skipped, or retrieved | P1 | `/prompt`, contextpack reports |
| **Verification Ledger:** per-run evidence of commands, tests, files, model, permissions, open risks | P1 | Analysis ledger, observability |
| **Agentic Code Review Board:** role agents (security, perf, tests, docs) review a change and produce one prioritized review | P1 | Coordinator mode |
| **Issue-to-PR workflow:** branch, plan, edit, test, summarize, PR body | P1 | Needs strong safeguards |
| **Repo Understanding Map:** browsable structure, hotspots, dependencies, stale docs from the semantic index | P1 | Phase 28 index |
| **"Ask the codebase" demo** with cited answers | P1 | Phase 28/29 |
| **Skills gallery:** curated local skills (Go maintainer, security review, release notes, triage) | P1 | Skills |
| **One-Engineer Startup Mode:** idea → roadmap → scaffold → tasks → launch checklist | P1 | Multiple |
| **Offline builder kit:** bundled model guidance, sample tasks, offline docs | P1/P2 | Phase 17 |
| **Local Eval Arena:** compare local models on your own repo tasks | P2 | Eval framework (`nandocodego eval`) |
| **Agentic education mode:** agent explains reasoning and trade-offs as it works | P2 | Prompting |
| **Public-good maintainer mode:** OSS issue triage, stale docs, release notes | P2 | Multiple |
| **IDE bridges** (VS Code, Zed, Neovim) to the same session | P2 | Phase 25 |
| **Team memory vault:** encrypted, opt-in, bring-your-own-storage sync | P2 | Memory |
| **Agent marketplace with signed manifests** | P2/P3 | High supply-chain risk |
| **Voice-to-agent** | P3 | None |

## 4. UX Follow-Ups

### Terminal UI

The TUI is in maintenance mode (ADR-002): bug and regression fixes only. The deferred Phase 22 features are now in §6 Parked.

### Browser UI

Promoted to the committed roadmap on 2026-10-05 (steps B2, B3, B4 in [NEXT-PHASES](NEXT-PHASES-IMPLEMENTATION-PLAN.md); detail in [WEB-UI-UX-PRODUCT-PLAN.md](../plans/WEB-UI-UX-PRODUCT-PLAN.md)). Still not committed:

- Mobile-first responsive layout.
- Theme engine beyond a basic light/dark toggle.
- MCP server manager panel.
- Model pull progress UI.
- Multi-user collaboration and cloud hosting.

## 5. Engineering And Tech Debt

| Item | Source | Notes |
| --- | --- | --- |
| Remaining performance follow-up: real latency evidence, production-like semantic benchmarks, better `/trace last`, long-transcript render benchmarks, fast-path/startup/hook tests | [PERFORMANCE-FOLLOW-UP-MULTI-AGENT-PLAN.md](../plans/PERFORMANCE-FOLLOW-UP-MULTI-AGENT-PLAN.md) | Plan status is still "In progress" |
| `BuildProjectAnalysisPrompt` uses heuristic signal lines, not true LLM map/reduce summarization | [NEXT-PHASES](NEXT-PHASES-IMPLEMENTATION-PLAN.md) CL/PA notes | Upgrade or accept as a documented limitation |
| Analysis workflow ignores summary-cache and ledger write errors | Same | Surface, test, or explicitly accept |
| Workspace trust flow, so project/HTTP/agent hooks can execute | [PHASE-9-DETAILED-PLAN.md](../phases/PHASE-9-DETAILED-PLAN.md) | Security-sensitive |
| **P0** Retrieval route misclassifies codebase-investigation prompts as general, so the model answers with no tools (`ToolModeNone`) and hallucinates paths: expand `isWorkspaceDiscoveryPrompt` vocabulary (server, route, endpoint, session, verify, behavior, ...) and never use `ToolModeNone` for "find/verify current behavior" prompts, with a table-driven regression test | [Hallucinated server path investigation](../reports/investigations/MODEL-HALLUCINATED-SERVER-PATH-INVESTIGATION-2026-06-08.md) | Not implemented (checked 2026-10-06, `internal/retrievalroute/route.go`) |
| **P1** Grounding for normal runs: default grounding system prompt, local-search fallback profile for uncertain code questions, and visible evidence state in answers | Same investigation | Not implemented |
| `PreCompact` / `PostCompact` hook events are defined in `internal/hooks/events.go` but never dispatched by compaction | [Phase 20 plan](../archive/phases/PHASE-20-DETAILED-PLAN.md) ("hook dispatch deferred") | Wire through `hooks.Dispatcher`; a `PreCompact` deny should skip compaction |
| Token estimation is character-based (4 chars/token), not tokenizer-aware | [Context-packing review](../archive/reports/investigations/CONTEXT-PACKING-LARGE-FILE-REVIEW-2026-05-20.md) | Fine for now; revisit if budgets misfire |

### Code review 2026-10-06

From [CODE-COMPLEXITY-AND-ARCHITECTURE-REVIEW-2026-10-06.md](../reports/investigations/CODE-COMPLEXITY-AND-ARCHITECTURE-REVIEW-2026-10-06.md); IDs refer to that report. Fix test-first (RED reproduction, then fix). Recommended: fold the P0 items into roadmap step B1.

| Item | Priority | Notes |
| --- | --- | --- |
| P0-1 Browser session drops earlier turns (history replaced by each run's added messages) | **P0** | `server/session.go` Terminal handling; needs a multi-turn server test |
| P0-2 XSS in browser markdown (`href` and quotes unescaped, CSP `unsafe-inline`) | **P0** | `internal/server/web/index.html` `renderMarkdown` |
| P0-3 One model runtime shared by all server sessions | **P0** | per-session runtime; switch only when the model changes |
| P0-4 `@dir/` evidence follows symlinks outside the workspace | **P0** | `contextpack.selectDirectoryEvidence`: per-file `tools.ResolvePath` |
| P0-6 Bash read-only classification ignores destructive arguments | **P0** | `tools/bash/classify.go` |
| P0/P1-7 Sub-agent permission mode can exceed parent's; project MCP config can self-trust; `*` in rules does not cross `/` | **P0/P1** | `agenttool`, `mcp/config.go`, `permissions/match.go` |
| P1-2 Agent event invariants (start before result, call order, exactly one Terminal, progress events) | P1 | shared `assertEventInvariants` helper first |
| P1-4 Server event log: replay/subscribe gap, silent drops, no gap event, browser never sends `Last-Event-ID` | P1 | base for Phase 25 |
| P1-5 Embeddings follow the active (cloud) model | P1 | bind embedder to local client |
| P1-6 Browser "Always Allow" works only once | P1 | `*(*)` rule never matches |
| Extract surface-neutral logic before B2 (`turnprep`, `modelruntime.Activate`, `runctl`, `ApplyTerminal`, `bootstrap.ApplyConfig`) | P1 | removes 3-4 drifted copies |
| Remaining P1 bugs (watchdog leak, supervisor rollback, Refresh model change, Status full read, smaller bugs) and test hygiene (sleep-based tests; the memory-test home-dir leak was fixed 2026-10-06; two tests that cannot fail) | P1/P2 | see report |

## 6. Parked / Out Of Scope For v0.1

- **Phase 23: OpenAI-compatible / generic multi-provider adapter.** Explicitly removed from v0.1 (its plan was deleted 2026-09-28; recover it from git history if the decision is reversed). Ollama Cloud support must stay Ollama-only.
- **Homebrew/Scoop publishing.** Optional in Phase 17, and only if publishing credentials exist.
- **Remote access, JWT auth, and `nandocodego connect` (former Phase 25 bridge).** Removed by ADR-002; v0.1 is localhost only. The bridge UDS listener for `SendMessage` is parked with it.
- **Deferred TUI features (from Phase 22):** collapsible hierarchical activity tree, click-to-expand tool panels, mouse lost-release recovery, full textarea-integrated Vim (mutations, dot-repeat, find-repeat, registers, paste/yank), true concurrent `/btw`, `/btw` read-only tool manifest restriction and context-stack modal priority fix.
