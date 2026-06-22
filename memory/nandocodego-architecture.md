---
name: nandocodego-architecture
description: Six core abstractions, layered architecture, and Ollama-specific constraints — no cache TTL, keep_alive management, 32 tool cap, NDJSON streaming, embeddings banned for memory recall
type: project
---

## Six Abstractions (no seventh allowed)

| Abstraction | Go realization |
|---|---|
| Query Loop | `func (a *Agent) Run(ctx context.Context, in Input) <-chan Event` |
| Tool | `Tool` interface + `Registry` map |
| Task | `TaskState` sealed interface + supervisor goroutine per task |
| State | `bootstrap.State` singleton (mutex-guarded) + `state.Store` reactive pub-sub |
| Memory | MD files under `~/.local/share/nandocodego/projects/<slug>/memory/` |
| Hooks | `hooks.Snapshot` frozen at session start; `hooks.Runner` dispatches by event |

## Layered Architecture

```
TUI (Bubble Tea) — REPL / slash commands / vim mode
Agent Loop — goroutine yielding Events on channel
Tool Orchestrator — partition into safe/serial batches
Tool Registry — Bash, FileRead, FileWrite, FileEdit, Grep, Glob, WebFetch, Agent, Todo
llm.Client (Ollama only) — streaming, retries, watchdog
Permission System — modes, rules, hooks, classifiers
State (bootstrap singleton + reactive store)
Memory (MD + LLM recall) | Hooks (snapshot) | MCP clients
```

## Ollama-Specific Constraints (treat as design facts)

- **No prompt cache** — optimize for `keep_alive` warmth and `num_ctx` re-use instead
- **32 active tool cap** — smaller models degrade with too many; use ToolSearch for the rest
- **NDJSON streaming** (not SSE) — `done: true` on final line; official Go client handles it
- **Token ground truth** — use `prompt_eval_count` + `eval_count` from Ollama, not estimates
- **No cache TTL concept** — pass `keep_alive: "30m"` for sessions, `keep_alive: 0` for one-shots
- **Memory recall via Ollama side-query** — embeddings banned for memory recall (learnings §L25)
- **Structured outputs** — pass JSON Schema in `format` field; fall back to JSON-mode + post-validation
- **Thinking models** — `message.thinking` field separate from `message.content`; never feed thinking back
- **Tool calling reliability** varies: qwen3 (excellent), llama3.1/3.2 (good), gemma3 (poor — degrade to JSON prompting)

## Module Path

`github.com/FernasFragas/nandocodego`
