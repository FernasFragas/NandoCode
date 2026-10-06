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

None open as of 2026-10-06. Add a row here (decision, why it matters, owner) when one comes up.

Resolved 2026-10-06: **launch positioning** "the local agentic engineer" with Proof Mode (roadmap B5) as the differentiator. Also resolved 2026-10-06 (recorded in [ADR-002](../adr/ADR-002-BROWSER-UI-PRIMARY-SURFACE.md) "Follow-Up Decisions"): **TUI entry point** `nandocodego tui`; **default model** chosen by the user in the browser first run (Phase 17), `qwen3.6:35b` stays the config default; **browser JS tests** via `node --test` in CI, no npm; **permission-rule `*`** matches `/`. **Malformed config** - `--print` fails fast (bug above, B1). **Private vulnerability reporting** - `SECURITY.md` now points to GitHub private vulnerability reporting; the maintainer must enable it in the repository settings (Security → Private vulnerability reporting) before public release.

Resolved 2026-10-05 ([ADR-002](../adr/ADR-002-BROWSER-UI-PRIMARY-SURFACE.md)): **Browser UI at launch** - the browser is the primary surface, the TUI stays in maintenance mode, plain `nandocodego` opens the browser, v0.1 is localhost only, and the frontend stays plain JS split into embedded files.

## 3. Product Features (Ideas)

Source: [PRODUCT-BRAINSTORM-DISRUPTIVE-LAUNCH.md](../product/PRODUCT-BRAINSTORM-DISRUPTIVE-LAUNCH.md), which has the scoring rationale. Scheduled on 2026-10-06: Proof Mode (roadmap B5), Local Trust Center and Agent Mission Control (B3 Trust and Activity panels), Prompt Inspector (B3), and the "ask the codebase" demo (Phase 18 docs); 60-second install is Phase 17. **Everything still listed below is post-v0.1.** Remote/Bridge Mode was removed from v0.1 by ADR-002 (localhost only; see §6).

| Idea | Priority | Builds on |
| --- | --- | --- |
| **Verification Ledger:** per-run evidence of commands, tests, files, model, permissions, open risks | P1 | Analysis ledger, observability |
| **Agentic Code Review Board:** role agents (security, perf, tests, docs) review a change and produce one prioritized review | P1 | Coordinator mode |
| **Issue-to-PR workflow:** branch, plan, edit, test, summarize, PR body | P1 | Needs strong safeguards |
| **Repo Understanding Map:** browsable structure, hotspots, dependencies, stale docs from the semantic index | P1 | Phase 28 index |
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

- Mobile-first responsive layout (post-launch).
- Theme engine beyond a basic light/dark toggle (post-launch).
- Editing MCP servers from the browser (post-launch; v0.1 shows them read-only in the B3 Trust panel).

Scheduled 2026-10-06: model pull progress in the browser is part of the Phase 17 first run.

## 5. Engineering And Tech Debt

### Code review 2026-10-06

From [CODE-COMPLEXITY-AND-ARCHITECTURE-REVIEW-2026-10-06.md](../reports/investigations/CODE-COMPLEXITY-AND-ARCHITECTURE-REVIEW-2026-10-06.md); IDs refer to that report. Fix test-first (RED reproduction, then fix). On 2026-10-06 the P0 items moved to roadmap step B1, the extraction work and agent event invariants to B1.5, and the server event-log fix to Phase 25 slice 0 (see `NEXT-PHASES-IMPLEMENTATION-PLAN.md`). What remains here is not yet scheduled. Everything else from the review was scheduled on 2026-10-06 (B1, B1.5, C1, B2, B3, Phase 25 slice 0, CL/PA gate, Phase 18).

| Item | Priority | Notes |
| --- | --- | --- |
| Fix-when-touched cleanups: `dirwalk` walkers share one admit policy and honor ctx; `grep` uses `fs.SkipAll` and honors ctx; `webfetch.stripHTML` simplification; `permissions.applyMode` unreachable deny branches; `selfinfo` nondeterministic output order; `hooks.Runner` named wrappers; duplicated semantic/route profile defaults; memory's duplicated filename sanitizer | P2/P3 | Fix the listed issue when you are already changing that file; write the test first. Details in the review report |

## 6. Parked / Out Of Scope For v0.1

- **Phase 23: OpenAI-compatible / generic multi-provider adapter.** Explicitly removed from v0.1 (its plan was deleted 2026-09-28; recover it from git history if the decision is reversed). Ollama Cloud support must stay Ollama-only.
- **Homebrew/Scoop publishing.** Optional in Phase 17, and only if publishing credentials exist.
- **Remote access, JWT auth, and `nandocodego connect` (former Phase 25 bridge).** Removed by ADR-002; v0.1 is localhost only. The bridge UDS listener for `SendMessage` is parked with it.
- **Multi-user collaboration and cloud hosting.** v0.1 is localhost only (ADR-002).
- **Workspace trust flow** (lets project, HTTP, and agent hooks execute). Post-v0.1 feature; until then project-controlled hooks stay parsed but disabled (decided 2026-10-06).
- **Deferred TUI features (from Phase 22):** collapsible hierarchical activity tree, click-to-expand tool panels, mouse lost-release recovery, full textarea-integrated Vim (mutations, dot-repeat, find-repeat, registers, paste/yank), true concurrent `/btw`, `/btw` read-only tool manifest restriction and context-stack modal priority fix.
