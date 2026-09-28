# Documentation

**Start with the guide for your role:** [engineers.md](../engineers.md) or
[product-managers.md](../product-managers.md). Both are at the repo root.

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
| [`roadmap/`](roadmap/) | Next-phases plan (committed order), backlog, project status snapshot, remaining-phases task review, Gate G0 validation plan. |
| [`product/`](product/) | Product strategy and brainstorms. Ideas here are not commitments until they reach the roadmap. |
| [`phases/`](phases/) | One detailed spec per delivery phase (`PHASE-N-DETAILED-PLAN.md`), exit-gate notes, and the chronological `PHASE-LOG.md`. |
| [`plans/`](plans/) | Specs for cross-cutting work: evaluation framework, performance, TUI tasks, cloud API keys, regression testing, web UI, E2E testing. |
| [`adr/`](adr/) | Architecture Decision Records. |
| [`guides/`](guides/) | How-to and reference guides (debug breakpoints, file/folder context pipeline). |
| [`fixes/`](fixes/) | Write-ups of specific bug fixes. |
| [`reports/`](reports/) | E2E test reports (top level), `bugs/` and `blocks/` from E2E runs, `investigations/` for latency, accuracy, and root-cause reports. |
| [`manual-tests/`](manual-tests/) | Manual test checklists. |

## Keeping Docs Honest

- When code and a doc disagree, the code is right. Fix the doc.
- Finishing a phase or slice means appending to `phases/PHASE-LOG.md` and updating `roadmap/NEXT-PHASES-IMPLEMENTATION-PLAN.md`.
- New ideas go in `roadmap/BACKLOG.md`, not in new standalone files.
- Phase docs cite `book/` and `.codex/` files that are not in this repository. Treat those citations as background.
