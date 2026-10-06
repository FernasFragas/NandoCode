# Engineer Guide

Start here if you are going to change code in `nandocodego`. Last reviewed 2026-09-28.

## In One Paragraph

`nandocodego` is a local-first coding agent written in Go. One binary gives you
a terminal UI, a one-shot `--print` mode, an HTTP/SSE server with a browser UI,
a semantic code index, and an eval runner. All of them drive the same agent loop.
That loop sends the user's prompt plus packed project context to an Ollama model
(local by default, Ollama Cloud when a user opts in with an API key). It executes
the tool calls the model makes (files, grep, shell, sub-agents, MCP) only after
the permission system approves them. The core runtime is built. What remains for
v0.1 is P0 security fixes and browser-first parity (roadmap steps B1-B4, with
B1.5 and C1), browser session durability (Phase 25),
validation evidence, packaging, and hardening.

## First Hour

```bash
# prerequisites: Go (version in go.mod), Ollama running locally
ollama serve
ollama pull qwen3.6:35b                 # the code default; any tool-capable model works

make build                              # -> bin/nandocodego
./bin/nandocodego doctor                # environment check
./bin/nandocodego server --model qwen3.6:35b   # browser UI (primary surface): open the printed URL
./bin/nandocodego --model qwen3.6:35b          # interactive TUI (maintenance mode)
./bin/nandocodego --model qwen3.6:35b --print "summarize @README.md"

make test                               # go test ./...
make check                              # build + vet + race tests + lint + phase-0 policy checks (needs golangci-lint)
make eval                               # deterministic coding-task evals (no model needed)
```

The examples use the code default, `qwen3.6:35b`. It is a large download; if you
use another installed model, pass it with `--model` (from Phase 17 the browser first run
will ask users to pick a model; see ADR-002 "Follow-Up Decisions").

Tests that need live services are opt-in, e.g.
`NANDOCODEGO_RUN_OLLAMA_INTEGRATION=1 OLLAMA_MODEL=qwen3.6:35b make test-integration`.
Some listener-binding tests fail inside restricted sandboxes; they pass on a
normal machine and in CI.

## Repository Layout

```text
cmd/nandocodego/     entrypoint (main.go → internal/cli)
internal/            all application code; see the package map
evals/               eval fixtures (task, starting repo, expected result, scoring, recorded model)
examples/            small standalone programs using the llm client and tools
tools/               policy and regression scripts used by make and CI
docs/                architecture, roadmap, phase specs, plans, reports (see docs/README.md)
.github/workflows/   CI (ci.yml) and security scans (security.yml)
USER_MANUAL.md       end-user reference for every command, tool, and slash command
SECURITY.md          trust boundaries and reporting
```

## How The System Is Organized

Read [docs/architecture/ARCHITECTURE.md](docs/architecture/ARCHITECTURE.md). It
has the layer diagram, a package-by-package map, the lifecycle of one prompt, the
design rules to preserve, where state is stored, and how to add a tool, slash
command, model family, or eval fixture.

The short version:

- **Surfaces** (`internal/cli`, `internal/tui`, `internal/server`) build a runtime and render the agent's event stream.
- **Agent runtime** (`internal/agent`) runs the model ⇄ tools loop, decorated by memory and hooks, fed by context packing, mentions, and semantic retrieval.
- **Capabilities** (`internal/tools/*`, `internal/mcp`, `internal/skills`) are all gated by `internal/permissions`.
- **Model access** (`internal/llm/...`) hides Ollama local vs cloud behind one `llm.Client`.

Diagrams: [APPLICATION-ARCHITECTURE-FLOWCHART.md](docs/architecture/APPLICATION-ARCHITECTURE-FLOWCHART.md).
Source files to read first, in order: `internal/cli/repl.go`, `internal/agent/agent.go`,
`internal/tools/tool.go`, `internal/permissions/resolver.go`, `internal/tui/app.go`,
`internal/server/server.go`.

## What Is Next

The committed order for v0.1 is in
[docs/roadmap/NEXT-PHASES-IMPLEMENTATION-PLAN.md](docs/roadmap/NEXT-PHASES-IMPLEMENTATION-PLAN.md).
Since 2026-10-05 the **browser UI is the primary surface** and the TUI is in
maintenance mode ([ADR-002](docs/adr/ADR-002-BROWSER-UI-PRIMARY-SURFACE.md)):

1. **B1: P0 bug and quick hardening.** Fix the server cloud-model switch bug, add security-header tests, harden the file-tree endpoint.
2. **B2: Browser parity.** Split the served page into embedded static files (plain JS, no build tools), stop/cancel, cloud key entry, clear/compact/index/cost endpoints, session list.
3. **Phase 25 (rescoped): Browser session durability.** Detach/reattach, replay with gap detection, sessions that survive a restart. No JWT, no `connect`. Spec: [PHASE-25-DETAILED-PLAN.md](docs/phases/PHASE-25-DETAILED-PLAN.md).
4. **B3/B4: Browser panels, accessibility, hardening.**
5. **Validation evidence** (Gate G0, CL/PA), run through the browser where possible.
6. **Phase 17: Distribution and install**, including plain `nandocodego` opening the browser.
7. **Phase 18: Hardening, evals, docs.** Final release gate.

The browser UI lives only in `internal/server/web/`. New user-facing features
land there first; the TUI gets bug fixes only.

Everything else (ideas, UX follow-ups, tech debt, open decisions) is in
[docs/roadmap/BACKLOG.md](docs/roadmap/BACKLOG.md).

## How Work Gets Done Here

1. Find the owning spec. Every phase has `docs/phases/PHASE-N-DETAILED-PLAN.md`; cross-cutting work has a plan in `docs/plans/`. Reviewed task breakdowns and blockers are in [REMAINING-PHASES-TASK-REVIEW.md](docs/archive/roadmap/REMAINING-PHASES-TASK-REVIEW.md).
2. Check the spec against the code. If they disagree, the code is the truth; update the doc.
3. Keep changes inside the owning phase. If you find a dependency gap, fix it first or record it as a blocker.
4. Run `make check` (it includes race tests), and `make eval` for agent-loop changes.
5. Append an entry to [docs/phases/PHASE-LOG.md](docs/phases/PHASE-LOG.md) with files changed, checks run, manual evidence, and known constraints.
6. If you close or add a backlog item, update [BACKLOG.md](docs/roadmap/BACKLOG.md).

## Rules That Are Easy To Break

- Don't add a direct dependency without adding it to `tools/allowed-deps.txt` and justifying it. CI enforces this.
- Don't hardcode network endpoints. `tools/check-network-policy.sh` enforces this.
- Don't run tools outside `permissions.Resolve`. New tools default to unsafe and destructive.
- Don't block the Bubble Tea update loop. Block the agent goroutine instead (see the permission broker).
- Don't fix latency by lowering `num_ctx` globally. Use the context-budget machinery.
- Don't add answer-shaping constraints to listing prompts (see the prompt-fidelity decision in the CL/PA plans).
- Don't start OpenAI-compatible or multi-provider work. It's out of scope for v0.1.
- Never log tokens, API keys, or auth headers.

## Documentation Map

| Need | Go to |
| --- | --- |
| How to use the product | [USER_MANUAL.md](USER_MANUAL.md), [README.md](README.md) |
| Architecture | [docs/architecture/](docs/architecture/) |
| What's next / backlog | [docs/roadmap/](docs/roadmap/) |
| Current status snapshot and caveats | [PROJECT-STATUS-AND-ONBOARDING.md](docs/roadmap/PROJECT-STATUS-AND-ONBOARDING.md) |
| Design specs and history | [docs/phases/](docs/phases/) |
| Debugging a request | [docs/guides/DEBUG-BREAKPOINTS.md](docs/guides/DEBUG-BREAKPOINTS.md) |
| Docker / server deployment | [README.Docker.md](README.Docker.md) |
| Everything in `docs/` | [docs/README.md](docs/README.md) |

Phase docs sometimes cite `book/chNN-*.md` and `.codex/` files. Those are not in
the repository; treat the citations as background reading you can skip.
