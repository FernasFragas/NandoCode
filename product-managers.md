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

Working positioning (proposed, not decided): *"the local agentic engineer: a
private, inspectable coding agent that runs where your code lives, coordinates
specialized agents, and leaves proof of the work."* See the
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

| Step | What it means for users | Status |
| --- | --- | --- |
| 1. Validation evidence | Prove, with recorded manual runs, that memory, hooks, integrations, sub-agents, skills, commands, background tasks, and the terminal app behave as specified. Retest four known bugs. | Code done; evidence pending |
| 2. **Remote / Bridge Mode** | Run the agent on a server, container, or dev box where the code lives, and drive it from your laptop. Disconnect and reconnect without losing the session. | **Next feature to build** |
| 3. Distribution & install | One-command install on macOS/Linux/Windows, verified downloads, release notes | Not started |
| 4. Hardening & release approval | Broader evals, security review, performance checks, docs site, go/no-go | Not started (eval framework already done) |

Detail for engineers: [docs/roadmap/NEXT-PHASES-IMPLEMENTATION-PLAN.md](docs/roadmap/NEXT-PHASES-IMPLEMENTATION-PLAN.md).

Explicitly **not** in v0.1: support for OpenAI or other non-Ollama model providers.

## Backlog Highlights

The full, prioritized list is [docs/roadmap/BACKLOG.md](docs/roadmap/BACKLOG.md).
Launch-relevant ideas that are not yet committed:

| Idea | Why it matters | Suggested priority |
| --- | --- | --- |
| **Run report / "Proof Mode"** | Export a shareable, redacted record of what the agent did and verified. Builds trust and gives us launch material. | P0/P1 |
| **Trust Center** (`/trust`) | One screen that answers "what can the agent do right now?" Makes the privacy promise tangible. | P0/P1 |
| **Mission Control** | One view of all running agents, tasks, and approvals, so multi-agent work is understandable | P0/P1 |
| Context inspector | Shows what the model saw, so users can debug bad answers | P1 |
| Code review board | Several specialist agents review a change together | P1 |
| Issue-to-PR workflow | From a ticket to a reviewed patch | P1 |
| Browser app panels | Memory, skills, permissions, tasks, and cost panels in the web UI | P1 |
| IDE plugins, team memory sync, marketplace | Post-launch bets | P2–P3 |

## Decisions That Need A Product Call

1. **Launch positioning.** Adopt "the local agentic engineer", or choose one of the alternatives in the brainstorm.
2. **Proof Mode / Trust Center.** Pull them into v0.1 (before packaging) or ship right after.
3. **Browser app at launch.** Polish it, or position the terminal app as the primary surface.
4. **Default model.** The current default is a large 35B model. A smaller default would make first run easier.
5. **Malformed config.** When config is broken, warn and continue, or stop?
6. **Security reporting.** Set up a private vulnerability-reporting channel before going public.

## Known Risks

| Risk | Mitigation in plan |
| --- | --- |
| Sounds like "another AI coding CLI" | Lead with local-first, remote-where-code-lives, proof reports, multi-agent |
| Setup friction (Ollama + models) | Phase 17 installer, `doctor`, model guidance, quickstart |
| Safety concerns about shell/file access | Permission modes today; Trust Center proposed |
| Local model quality varies | Eval suite to measure it; cloud models as opt-in |
| Scope creep before v0.1 | v0.1 proves the core workflow; everything else stays in the backlog |

## Where To Go Deeper

- [README.md](README.md): public overview and quickstart.
- [docs/product/](docs/product/): product thinking and brainstorms.
- [docs/plans/WEB-UI-UX-PRODUCT-PLAN.md](docs/plans/WEB-UI-UX-PRODUCT-PLAN.md): browser app product plan.
- [docs/reports/E2E-EXECUTIVE-SUMMARY-2026-06-07.md](docs/reports/E2E-EXECUTIVE-SUMMARY-2026-06-07.md): latest end-to-end quality run.
- [docs/architecture/ARCHITECTURE.md](docs/architecture/ARCHITECTURE.md): how it's built, if you want the technical picture.
