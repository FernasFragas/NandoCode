# Documentation

**Start with the guide for your role:** [engineers.md](../engineers.md) or
[product-managers.md](../product-managers.md). Both are at the repo root. Coding agents follow [AGENTS.md](../AGENTS.md).

## The Four Documents That Answer Most Questions

| Question | Document |
| --- | --- |
| How is the system built? | [architecture/ARCHITECTURE.md](architecture/ARCHITECTURE.md) |
| What is being built next, in what order? | [roadmap/NEXT-PHASES-IMPLEMENTATION-PLAN.md](roadmap/NEXT-PHASES-IMPLEMENTATION-PLAN.md) |
| What ideas, bugs, and follow-ups are waiting? | [roadmap/BACKLOG.md](roadmap/BACKLOG.md) |
| What has been built, and how was it verified? | [phases/PHASE-LOG.md](phases/PHASE-LOG.md) |

## Folders

| Folder | Contents |
| --- | --- |
| [`architecture/`](architecture/) | System overview and package map, Mermaid flowcharts, model/embedding routing, diagrams in `images/`. |
| [`roadmap/`](roadmap/) | Next-phases plan (committed order), backlog, project status snapshot, Gate G0 validation plan. |
| [`product/`](product/) | Product strategy and brainstorms. Ideas here are not commitments until they reach the roadmap. |
| [`phases/`](phases/) | Specs for phases that are still open or still need exit-gate evidence (`PHASE-N-DETAILED-PLAN.md`), and `PHASE-LOG.md` (newest first). |
| [`plans/`](plans/) | Live specs for cross-cutting work: web UI and browser visual revamp, context/latency and prompt accuracy (CL/PA gate), evaluation framework, performance, regression testing, E2E testing. Design mockups in `UX/`. |
| [`adr/`](adr/) | Architecture Decision Records. Start with ADR-002 (browser UI is the primary surface). |
| [`guides/`](guides/) | How-to guides (debug breakpoints). |
| [`reports/`](reports/) | Open `blocks/` from E2E runs and `investigations/` (root-cause reports) still tied to open work. New E2E runs write to `e2e/<date>/` and open bugs to `bugs/`. |
| [`manual-tests/`](manual-tests/) | Manual test checklists. |

## Keeping Docs Honest

- Authority order: `roadmap/NEXT-PHASES-IMPLEMENTATION-PLAN.md` (what and when) > phase/plan docs (how) > `reports/` (evidence only). `roadmap/BACKLOG.md` is for unscheduled work only.
- When code and a doc disagree, the code is right. Fix the doc.
- Finishing a phase or slice means appending to `phases/PHASE-LOG.md` and updating `roadmap/NEXT-PHASES-IMPLEMENTATION-PLAN.md`.
- New ideas go in `roadmap/BACKLOG.md`, not in new standalone files.
- When a phase, plan, or report is finished: update its status line, update `roadmap/PROJECT-STATUS-AND-ONBOARDING.md` if reality changed, close or promote its `BACKLOG.md` items, and delete it (git history keeps it; fix inbound links). The PHASE-LOG entry is the lasting record.
- Resolved bug reports are deleted once the fix is verified in source, covered by a test, and recorded in `phases/PHASE-LOG.md`.
- Older phase docs cite `book/` and `.codex/` files that are not in this repository. Treat those citations as background.
