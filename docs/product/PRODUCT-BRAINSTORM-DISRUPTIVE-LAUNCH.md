# Product Brainstorm - Disruptive Launch Opportunities

Date: 2026-06-22
Status: PM brainstorm, not an implementation commitment
Scope: Feature opportunities that could make `nandocodego` more valuable, more differentiated, and more launch-worthy.

## Current Product Read

`nandocodego` is already more than a local chat CLI. The current product has a real agent runtime: local-first Ollama support, optional Ollama Cloud routing, TUI, server mode, permissions, tools, memory, hooks, MCP, skills, background tasks, multi-agent coordination, semantic workspace indexing, prompt packing, traceability, and response-time improvements.

The product is not yet launch-ready. The remaining launch path is:

1. Close live/manual validation gates for Phases 8-14, Workstream CL/PA, and Phase 22.
2. Implement Phase 25 Remote / Bridge Mode.
3. Finish Phase 17 distribution/install.
4. Finish Phase 18 hardening, evals, docs, and release approval.

The most interesting product wedge is this:

> A private, local-first agentic engineer that can run where the code lives, coordinate multiple agents, show its work, and let one human engineer ship with leverage that previously required a team.

That wedge is different from "another AI coding CLI." The launch should make the human engineer feel amplified, not replaced.

## Rating Model

Scores are 1-10.

| Rating | Meaning |
| --- | --- |
| Disruption | How much the feature changes expectations for what a coding tool can be. |
| Product value | How much more valuable the current product becomes to real users. |
| Feasibility | How realistic it is to ship with the existing architecture and roadmap. |
| Launch priority | P0 means launch-critical, P1 means strong launch enhancer, P2 means post-launch, P3 means moonshot. |

## Strategic Principles

1. Lead with agency, privacy, and proof. The product should prove what agentic engineering can do.
2. Keep the engineer in command. Make agent orchestration, permissions, and review visible.
3. Do not compete only on model quality. Compete on workflow, trust, local control, and agent coordination.
4. Make launch demos concrete. "One engineer ships a real project with local agents" is stronger than feature lists.
5. Package the product around outcomes: understand a repo, plan work, modify code, verify behavior, and leave an audit trail.

## Feature Brainstorm

| Feature | Concept | Disruption | Product value | Feasibility | Launch priority | Why it matters |
| --- | --- | ---: | ---: | ---: | --- | --- |
| Agentic Engineering Proof Mode | A first-class mode that records how a task was planned, delegated, edited, tested, and reviewed by agents under human direction. | 9 | 9 | 8 | P0 | Turns the project philosophy into a product artifact. Makes "built with agentic engineering" credible and inspectable. |
| Remote / Bridge Mode | `nandocodego connect` lets the UI run locally while the agent runs on the machine/container/VM where code lives. | 8 | 9 | 7 | P0 | This is already the next roadmap phase and unlocks serious remote, container, home-lab, and server workflows. |
| Agent Mission Control | A dashboard/TUI view showing active agents, tasks, tools, permissions, token use, checkpoints, and status in one place. | 8 | 9 | 7 | P0/P1 | Multi-agent work becomes understandable. Users need to see what the system is doing to trust it. |
| 60-Second Local Launch | One install command, `doctor`, model guidance, sample repo task, and first successful agent run in under one minute. | 6 | 10 | 8 | P0 | Reduces adoption friction. A disruptive product still fails if setup is painful. |
| Shareable Run Report | Export a sanitized Markdown/HTML report of a run: prompt, plan, files touched, tests run, decisions, warnings, and final summary. | 8 | 9 | 8 | P0/P1 | Creates trust, reviewability, and a social sharing loop. Also helps teams evaluate agentic work. |
| Local Trust Center | A visible permissions, network, file access, hooks, MCP, and credential dashboard with "what can the agent do right now?" | 8 | 9 | 7 | P0/P1 | Local-first is only valuable if users understand the boundary. This makes safety a product feature. |
| Repo Understanding Map | Semantic index UI that shows project structure, hotspots, dependencies, stale docs, TODOs, and likely ownership areas. | 8 | 8 | 6 | P1 | Converts semantic retrieval into something users can inspect, not just invisible prompt context. |
| One-Engineer Startup Mode | A guided workflow from idea/spec to roadmap, scaffolding, implementation plan, tasks, tests, docs, and launch checklist. | 9 | 8 | 6 | P1 | Positions the product as leverage for solo builders and small teams, not just code editing. |
| Agentic Code Review Board | Multiple role agents review a change as security, performance, product, docs, and test reviewers, then produce one prioritized review. | 8 | 9 | 7 | P1 | Uses the existing coordinator/multi-agent foundation for a clear high-value workflow. |
| Issue-to-PR Workflow | Give it a GitHub issue or local task; it creates a branch, plans, edits, tests, summarizes, and prepares a PR body. | 9 | 10 | 5 | P1 | This is the most commercially legible agentic engineering workflow. Needs strong safeguards. |
| Verification Ledger | Every run stores evidence: commands executed, tests passed/failed, files read/written, model used, permissions granted, and unresolved risks. | 8 | 9 | 7 | P1 | Makes agent output auditable. This is valuable for open source, teams, and enterprise adoption. |
| Prompt/Context Inspector | A user-facing inspector for what context was included, summarized, skipped, or retrieved before the model call. | 7 | 9 | 8 | P1 | Current context work is strong. Exposing it increases trust and helps users debug bad answers. |
| "Ask the Codebase" Public Demo | A polished demo where users run semantic index, ask architectural questions, and get cited answers from their repo. | 7 | 8 | 9 | P1 | Easy to demo and validates local semantic retrieval without requiring code writes. |
| Agent Skills Gallery | Curated local skills for common workflows: Go maintainer, security review, release notes, refactor plan, bug triage. | 7 | 8 | 8 | P1 | Makes the product feel immediately useful while keeping skills local and inspectable. |
| Browser UI Parity | Serve the richer browser UI, fix event payload mapping, and support chat, permissions, model picker, mentions, and status. | 7 | 8 | 7 | P1 | Gives the product a more approachable surface for demos and non-terminal workflows. |
| Offline Builder Kit | Bundle recommended local model setup, sample tasks, eval prompts, and docs for fully offline use. | 7 | 8 | 6 | P1/P2 | Differentiates from cloud-first coding tools and supports privacy-sensitive users. |
| Team Memory Vault | Encrypted, opt-in, bring-your-own-storage memory sync for teams. | 8 | 8 | 4 | P2 | High value for teams, but trust, conflict, and security design should wait until local memory is proven. |
| Agent Marketplace With Audit | Share skills, hooks, MCP configs, and task recipes with signed manifests and safety metadata. | 9 | 8 | 3 | P2/P3 | Big ecosystem potential, but high supply-chain risk. Needs strict sandboxing and provenance. |
| Local Eval Arena | Built-in eval suite where users compare local models on their own repo tasks. | 7 | 7 | 6 | P2 | Helps users choose models and makes product quality measurable. |
| IDE Bridges | VS Code, Zed, and Neovim bridges that connect to the same local/remote agent session. | 8 | 9 | 4 | P2 | Increases daily usage, but should come after TUI/server/remote flows are stable. |
| Voice-to-Agent | Offline voice input for creating tasks, asking questions, and approving low-risk actions. | 7 | 6 | 3 | P3 | Interesting accessibility and mobility story, but not central to v0.1 launch. |
| Agentic Education Mode | The agent explains the engineering reasoning, tradeoffs, and codebase concepts as it works. | 7 | 8 | 6 | P2 | Helps learners and teams adopt agentic workflows, but must not slow expert users by default. |
| Public Good Maintainer Mode | Run triage on open-source repos: issue grouping, stale docs, quick fixes, release notes, contributor onboarding. | 8 | 8 | 5 | P2 | Strong world-impact angle: one maintainer can manage more software with local agents. |

## Highest-Leverage Launch Bets

### 1. Agentic Engineering Proof Mode

This should be the signature launch feature. The project can say:

> This tool was built through agentic engineering, and it can show you the same workflow on your code.

MVP:

- `/run-report last` exports Markdown.
- Include user prompt, generated plan, tool calls, files changed, tests run, permission decisions, model/provider, semantic retrieval summary, and unresolved risks.
- Redact secrets and sensitive paths by default.
- Add a short README section linking to an example report.

Why this improves value:

- Gives users confidence that the agent did real work.
- Helps engineers review agent-created changes.
- Creates launch assets: demo reports, blog snippets, and proof artifacts.

### 2. Remote / Bridge Mode as the Launch Headline

Remote mode can turn the product from "local CLI" into "agentic engineer that lives with your code."

MVP:

- `nandocodego server --port 8080 --print-token`
- `nandocodego connect <url> --token <jwt>`
- Detach/reconnect with event replay.
- Remote permission prompts in local TUI.
- Clear docs: tools run on the server, UI runs on the client.

Why this improves value:

- Works for containers, dev boxes, home servers, cloud VMs, and CI runners.
- Keeps code execution near the source tree.
- Makes agent sessions resilient instead of tied to a single terminal process.

### 3. Agent Mission Control

The product already has tasks, sub-agents, coordinator mode, semantic indexing, trace events, and permissions. The missing product moment is a unified "what is happening right now?" view.

MVP:

- TUI panel or slash command showing active run phase, active tools, sub-agents, background tasks, queued prompts, permission waits, semantic index activity, and last terminal reason.
- Compact status for normal use, expanded detail on demand.
- Same event model reused by browser and remote client.

Why this improves value:

- Makes multi-agent work comprehensible.
- Reduces panic during long runs.
- Helps users trust the system without reading logs.

### 4. Shareable Run Report

This overlaps with Proof Mode but is more explicitly social and review-oriented.

MVP:

- `nandocodego report export --last --format markdown`
- `--redact` on by default.
- Include "human decisions" separately from "agent actions."
- Include "verification" section with commands/tests and whether they passed.

Why this improves value:

- Great for PR descriptions.
- Great for launch storytelling.
- Lets the engineer show the value of agentic engineering without hand-waving.

### 5. Local Trust Center

Safety should be a differentiator, not a buried policy.

MVP:

- `/trust` shows permission mode, allowed/denied rules, network policy, MCP servers, hook sources, cloud model status, credential source, and writable roots.
- `doctor` includes a launch-facing trust summary.
- Browser UI gets a simple Trust panel later.

Why this improves value:

- Makes the local-first promise tangible.
- Helps security-conscious users adopt the tool.
- Reduces fear around shell/file access.

## Positioning Options

### Option A: "The Local Agentic Engineer"

Best for launch.

Message:

> A local-first agentic coding tool that gives one engineer the leverage of a coordinated engineering team, while keeping code and control on their machine.

Strengths:

- Matches current architecture.
- Highlights privacy and agent orchestration.
- Avoids overpromising enterprise features.

Weakness:

- Needs strong demos to avoid sounding like another coding assistant.

### Option B: "Proof-of-Work AI Coding"

Message:

> Every agent run leaves evidence: what it read, changed, tested, and still doubts.

Strengths:

- Highly differentiated.
- Speaks to trust, review, and serious engineering.

Weakness:

- Less emotionally broad than "local agentic engineer."

### Option C: "The Solo Builder Operating System"

Message:

> Plan, build, test, document, and launch with a private team of local agents.

Strengths:

- More ambitious and founder-friendly.
- Good for demos and storytelling.

Weakness:

- Requires broader workflows to feel true.

Recommended launch positioning:

> `nandocodego` is the local agentic engineer: a private, inspectable coding agent that runs where your code lives, coordinates specialized agents, and leaves proof of the work.

## Launch Demo Ideas

| Demo | Why it works | Required product surface |
| --- | --- | --- |
| "Built With Agentic Engineering" demo | Shows the project built itself through the workflow it sells. | Run report, README attribution, launch blog. |
| Find and fix top repo TODOs | Concrete multi-agent workflow with visible delegation. | Coordinator mode, tasks, run status, report export. |
| Remote container session | Shows agent running inside a container while the user drives TUI locally. | Phase 25 connect, permissions, replay. |
| Ask the codebase | Fast, low-risk demo of semantic index and cited answers. | Phase 28/29, prompt/context inspector. |
| Trust boundary walkthrough | Shows local-first, permissions, cloud gating, and network controls. | Trust Center, doctor, security docs. |
| One issue to reviewed patch | Most valuable end-to-end engineering workflow. | Planning, edits, tests, report export, optional GitHub later. |

## Prioritized Launch Roadmap Overlay

This overlay does not replace the engineering roadmap. It reframes what to emphasize.

### Before v0.1

1. Finish Gate G0 and manual evidence capture.
2. Finish Phase 25 Remote / Bridge Mode.
3. Add Proof Mode / Run Report if scope allows.
4. Add launch-facing Trust Center or at least `/trust` plus `doctor` output.
5. Finish distribution/install and 60-second quickstart.
6. Finish hardening, evals, docs, and one polished launch demo.

### Immediately After v0.1

1. Serve and fix the richer browser UI.
2. Add Agent Mission Control panel.
3. Add Prompt/Context Inspector.
4. Add curated Skills Gallery.
5. Add Code Review Board workflow.

### Later Bets

1. Team Memory Vault.
2. IDE bridges.
3. Agent Marketplace with signed/audited packages.
4. Local Eval Arena.
5. Public Good Maintainer Mode.

## Product Risks

| Risk | Severity | Mitigation |
| --- | ---: | --- |
| Product sounds like every other AI coding CLI | High | Lead with local-first, remote-where-code-lives, proof reports, and multi-agent orchestration. |
| Agentic claim feels vague | High | Ship run reports and demo artifacts that show actual agent work. |
| Setup friction kills adoption | High | Prioritize 60-second quickstart, model recommendations, and clear `doctor` output. |
| Safety concern blocks serious users | High | Make Trust Center and permission model visible, not hidden in docs. |
| Multi-agent workflows feel chaotic | Medium | Build Mission Control before expanding more swarm features. |
| Browser UI looks unfinished during launch | Medium | Either polish and serve it or explicitly position TUI as the primary launch surface. |
| Too many moonshots dilute v0.1 | High | Treat v0.1 as proof of the core workflow, not the full platform. |

## Recommendation

For first launch, do not try to present `nandocodego` as a generic "AI coding assistant." Present it as a new way to engineer software:

1. Human-led.
2. Agent-powered.
3. Local-first.
4. Inspectable.
5. Capable of multi-agent work.
6. Able to run where the code lives.

The most valuable launch package is:

- Remote / Bridge Mode
- Agentic Engineering Proof Mode
- Shareable Run Report
- Trust Center
- 60-second install and quickstart
- One polished demo showing a real repo task completed with visible agent orchestration

That combination is disruptive because it shifts the story from "AI autocomplete" to "one engineer operating a transparent local engineering system."
