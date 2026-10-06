# Product Guide

Start here if you are shaping what `nandocodego` becomes. It covers what it is,
what works today, what ships next, and what is in the backlog. There's no code
in this guide. Last reviewed 2026-09-28.

## What It Is

`nandocodego` is a **local-first AI coding agent**. A developer points it at a
code repository and asks for something: explain this module, fix this bug, add
tests, refactor this. A language model reads the relevant code, proposes and makes
changes, runs commands, and reports back. The developer approves every risky
action.

What makes it different from typical AI coding tools:

- **Runs on your machine.** By default the model runs locally through [Ollama](https://ollama.com), so code never leaves the laptop. Cloud models are opt-in, per model, and need an API key.
- **Shows its work.** Every tool call is visible and permission-gated. Traces, prompt dumps, and cost/usage views make runs inspectable.
- **Multi-agent.** It can spawn sub-agents, run background tasks, and coordinate several agents on one job.
- **Built by agents.** The project itself was built through agentic engineering, one engineer directing AI agents. That is part of the story.

Launch positioning (decided 2026-10-06): *"the local agentic engineer: a
private, inspectable coding agent that runs where your code lives, coordinates
specialized agents, and leaves proof of the work."* Proof Mode (a shareable
run report, roadmap step B5) is the launch differentiator. See the
[product brainstorm](docs/product/PRODUCT-BRAINSTORM-DISRUPTIVE-LAUNCH.md).

**Who it's for:** individual engineers and small teams who want AI leverage
without sending code to a third party. That includes privacy-sensitive
companies, offline and air-gapped setups, and solo builders.

## What Users Can Do Today

Status: pre-release (`v0.0.0-dev`). Everything below is built and covered by
automated tests. Several areas still need recorded manual sign-off (see
"Release status").

| Capability | In plain terms |
| --- | --- |
| Terminal app | Chat-style interface in the terminal, with live streaming, tool activity, a status bar, and keyboard shortcuts including Vim mode |
| One-shot mode | Run a single request from a script or CI (`--print`) |
| Browser app + API | Run it as a local server and use it from a browser or other programs |
| Reference files | `@file`, `@folder/`, `@file#L10-L20` pull exact code into the request |
| Code search index | Builds a local "meaning" index of the repo so it can find relevant code without being told where |
| Tools | Read, write, and edit files, search, run shell commands, fetch web pages, keep a to-do list |
| Permissions | Several safety modes, from asking before everything to trusted auto-approve, plus allow/deny rules |
| Memory | Remembers project facts across sessions in readable files |
| Skills & hooks | Reusable instructions (skills) and automation triggers on events (hooks) |
| Integrations | Connects to external tool servers through MCP, an open standard |
| Background work & multi-agent | Long tasks run in the background; a coordinator can direct several worker agents |
| Model choice | Any local Ollama model; Ollama Cloud models with an API key |
| Quality measurement | Built-in eval suite (`nandocodego eval`) scores the agent on real coding tasks |

The full user reference is [USER_MANUAL.md](USER_MANUAL.md).

## Release Status And What Ships Next

Target: **v0.1.0**, the first public release. The remaining path, in order:

Since 2026-10-05 the **browser app is the primary product surface**. The terminal app keeps shipping in maintenance mode, and v0.1 runs on your own machine only (localhost). Decision record: [ADR-002](docs/adr/ADR-002-BROWSER-UI-PRIMARY-SURFACE.md).

| Step | What it means for users | Status |
| --- | --- | --- |
| 1. Fix and harden | Fix the bug where a listed cloud model can't be selected in the browser; security checks | **Next** |
| 2. Browser parity | Stop a run, enter a cloud API key, clear/compact/index/cost, list sessions | Not started |
| 3. Sessions that last | Close the tab or restart the server without losing the session; the agent keeps working meanwhile | Not started |
| 4. Browser panels | Memory, skills, hooks, permissions, tasks, prompt inspector; accessibility | Not started |
| 5. Validation evidence | Recorded manual runs proving the features behave as specified, done in the browser | Code done; evidence pending |
| 6. Distribution & install | One-command install; running `nandocodego` opens the browser | Not started |
| 7. Hardening & release approval | Broader evals, security review, performance checks, docs site, go/no-go | Not started (eval framework already done) |

Detail for engineers: [docs/roadmap/NEXT-PHASES-IMPLEMENTATION-PLAN.md](docs/roadmap/NEXT-PHASES-IMPLEMENTATION-PLAN.md).

Explicitly **not** in v0.1: support for OpenAI or other non-Ollama model providers.

## Backlog Highlights

The full, prioritized list is [docs/roadmap/BACKLOG.md](docs/roadmap/BACKLOG.md).
Scheduled for v0.1 on 2026-10-06:

| Idea | Why it matters | Where |
| --- | --- | --- |
| **Run report / "Proof Mode"** | Export a shareable, redacted record of what the agent did and verified. Builds trust and is the launch differentiator. | Roadmap step B5 |
| **Trust panel** | One screen that answers "what can the agent do right now?" (permissions, hooks, MCP servers, credentials, network policy). | Browser panels (B3) |
| **Activity view** ("mission control") | One view of the run, tools, sub-agents, tasks, queue, and approvals. | Browser panels (B3) |
| Context inspector | Shows what the model saw, so users can debug bad answers. | Browser panels (B3) |
| "Ask the codebase" demo | Cited answers about a repo, as launch material. | Release docs (Phase 18) |

Post-launch ideas (not in v0.1): code review board, issue-to-PR workflow, repo map, skills gallery, IDE plugins, team memory sync, marketplace, and the rest of [BACKLOG.md](docs/roadmap/BACKLOG.md) §3.

## Decisions That Need A Product Call

None open as of 2026-10-06. Decided that day: positioning ("the local agentic engineer" with Proof Mode); Proof Mode, Trust, and Activity views are in v0.1; the first run asks users to pick a model (the 35B default stays in config only); `--print` fails fast on a broken config; security reports go through GitHub private vulnerability reporting. Track new decisions in [BACKLOG.md](docs/roadmap/BACKLOG.md) §2.

## Known Risks

| Risk | Mitigation in plan |
| --- | --- |
| Sounds like "another AI coding CLI" | Lead with local-first in the browser, proof reports (Proof Mode), multi-agent |
| Setup friction (Ollama + models) | Phase 17 installer, `doctor`, model guidance, quickstart |
| Safety concerns about shell/file access | Permission modes today; Trust panel in v0.1 (B3); security fixes in B1 |
| Local model quality varies | Eval suite to measure it; cloud models as opt-in |
| Scope creep before v0.1 | v0.1 proves the core workflow; everything else stays in the backlog |

## Where To Go Deeper

- [README.md](README.md): public overview and quickstart.
- [docs/product/](docs/product/): product thinking and brainstorms.
- [docs/plans/WEB-UI-UX-PRODUCT-PLAN.md](docs/plans/WEB-UI-UX-PRODUCT-PLAN.md): browser app product plan.
- [docs/reports/e2e/2026-06-07/E2E-MASTER-REPORT-2026-06-07.md](docs/reports/e2e/2026-06-07/E2E-MASTER-REPORT-2026-06-07.md): latest end-to-end quality run.
- [docs/architecture/ARCHITECTURE.md](docs/architecture/ARCHITECTURE.md): how it's built, if you want the technical picture.
