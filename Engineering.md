**I developed it privately and published it as a squashed history, so commits don't show the step-by-step agent work.**
## What I specified

- I directed Claude Code, OpenAI Codex and Ollama models through one loop: phase spec, agent implementation, my review, then `PHASE-LOG.md` evidence.
- A local-first Go coding agent: one binary with TUI, `--print`, HTTP/SSE server, semantic index and evals, all driving one agent loop.
- A dependency allowlist and no hardcoded network endpoints, both enforced in CI.
- Ollama only: local by default, opt-in Ollama Cloud via API key. No OpenAI-compatible or multi-provider work in v0.1.
- Phased delivery: each phase has a detailed plan, exit gates, and a log entry with files, checks and evidence.
- Default model `qwen3.6:35b`, with at least 64K context treated as a floor.

## What the agents implemented

- Phases 0–16, 19–22, 24, 26–29: LLM client, tools, agent loop, permissions, TUI, memory, hooks, MCP, sub-agents, skills and tasks.
- Semantic indexing and a retrieval router with light/full modes, caps, deadlines and stage-level observability.
- A deterministic eval framework: five recorded Go fixtures, scoring, JSON/Markdown reports and a CI job.
- An HTTP/SSE server and embedded browser UI with sessions, permissions, model switching and a file tree.
- Performance work: chat fast path, bounded parallel scanning, a render cache and parallel startup preparation.
- Multi-agent E2E test runs that produced bug and block reports, such as the cloud-model switch bug.

## Where I overrode them

- I rejected the "return only the listing" answer constraint and the intent-specific retry for listing prompts; tree-only expansion with no injected instructions.
- I removed the automatic project-analysis fallback from normal chat; it now runs only through an explicit `/analyze-project`.
- I ruled out repeating the user prompt on every request; an anchor footer appears only when packing risks prompt drift.
- I chose `auto` semantic retrieval with strict bypasses, deferred rerank and caching, and made dimension mismatch skip retrieval instead of blocking.
- I banned lowering `num_ctx` globally to fix latency; the context-budget machinery must handle it instead.
- I made the browser the primary surface (ADR-002): TUI to maintenance mode, remote `connect`/JWT cut, Phase 25 rescoped to session durability.
