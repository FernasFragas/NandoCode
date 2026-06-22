---
name: nandocodego-constraints
description: Hard rules — what must never be done in nandocodego, anti-patterns list, forbidden patterns per phase
type: feedback
---

## MUST NOT Do (§0.3)

- **No seventh abstraction** — no "workflows", "pipelines", "managers", "controllers" at the top level
- **No hardcoded model name** — models are user-configured in `config.toml`, validated against `/api/tags`
- **No cloud dependencies** — outbound HTTP only to: Ollama endpoint, approved MCP servers, WebFetch/WebSearch tools
- **No custom TUI framework** — use Bubble Tea + Lip Gloss only
- **No provider branching in business logic** — all model traffic through `llm.Client` interface

## Anti-Patterns (§4 — refuse these)

1. Switch-based tool dispatch — use registry + interface; zero edits to dispatcher when adding tools
2. Single-tier state — two tiers (bootstrap + reactive); one-way mirror only
3. `Workflow`/`Pipeline` abstraction — it's a Task with sub-tasks
4. Cyclic imports
5. `context.Background()` in non-main, non-test code — always propagate
6. `panic` for control flow — use error returns; panic only for true invariant violations
7. `init()` doing I/O
8. Reading hook config mid-session — snapshot at start; `/hooks reload` is the only refresh
9. Bare strings as IDs — use branded types (`type AgentID string`)
10. Embeddings for memory recall (< 1000 items) — LLM side-query only
11. `fmt.Sprintf` for SQL/shell — use parser/escaper
12. Cloning parent history into fork without justification
13. Sub-agents in `auto` permission mode — default to `bubble`
14. Goroutine with no `context.Context`
15. `map[string]any` as public API surface — wrap in typed struct
16. Hardcoded model names outside `internal/llm/capabilities.go`
17. "Manager" or "service" suffix on a struct — prefer concrete nouns
18. `go test` in CI without `-race`

## Per-Call Concurrency Rules

- Every long-running goroutine takes `context.Context` — no exceptions
- Every channel has exactly one closer
- No goroutine leaks — CI runs `go test -race -timeout=120s ./...`
- Channel-of-channels is a smell — refactor to struct with explicit fields
