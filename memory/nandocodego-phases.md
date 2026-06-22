---
name: nandocodego-phases
description: Delivery phases status for nandocodego — which phases are complete, what each phase ships, exit gate summary
type: project
---

## Phase Status

| Phase | Name | Milestone | Status |
|---|---|---|---|
| 0 | Security & supply-chain baseline | Threat model + dep allowlist | ✅ Done |
| 1 | Repo scaffolding & tooling | `nandocodego --version` runs | ✅ Done |
| 2 | LLM client (Ollama) | Streaming chat with watchdog | ✅ Done |
| 3 | Tool interface + 3 starter tools | Bash / FileRead / FileWrite | ✅ Done |
| 4 | Agent loop | One-shot agent answers a prompt | ✅ Done |
| 5 | Permission system | 7 modes + rule resolution | ✅ Done |
| 6 | State layer (two-tier) | Bootstrap + reactive store | ✅ Done |
| 7 | Bubble Tea TUI + REPL | Full interactive REPL | ✅ Done |
| 8 | Memory | Recall + write across sessions | ✅ Done |
| 9 | Hooks | Command/prompt; HTTP/agent reserved | ✅ Done |
| 10 | MCP integration | stdio + http transports | ✅ Done |
| 11 | Sub-agents and fork | Recursive agent spawning | ✅ Done |
| 12 | Skills (file-driven tools) | Bundled + project-level skills | ✅ Done |
| 13 | Slash commands & config UX | `/help`, `/model`, `/memory`, etc. | ✅ Done |
| 14 | Tasks: background bash & async agents | Unified task supervisor | ✅ Done |
| 15 | Concurrency & speculative execution | Partition + per-call safety | ✅ Done |
| 16 | Observability and metrics | Log + metric decorators | ✅ Done |
| 17 | Distribution and install | Static binaries + brew/deb | 🔲 Pending |
| 18 | Hardening, eval suite, docs | Release candidate | 🔲 Pending |
| 19 | Complete tool ecosystem | FileEdit, Glob, Grep, WebFetch, Todo | 🔲 Pending |
| 20 | Content compaction | 4-layer compaction strategy | 🔲 Pending |
| 21 | Web interface & HTTP API | SSE + embedded UI | 🔲 Pending |

## Phase 17 Exit Gate
`goreleaser release --snapshot` produces 5 binaries; `brew install` works on macOS.

## Phase 19 Exit Gate (run before 18)
`builtin.NewRegistry()` returns 9 tools; FileEdit + Grep work in eval runner.

## Phase 18 Exit Gate
Eval suite ≥ 80% on qwen3; `gosec`/`govulncheck`/`golangci-lint` zero findings; `v0.1.0` tagged.

## Post-Phase-16 Improvements (2026-05-16) ✅

All applied and validated:

| # | Change | Key files |
|---|---|---|
| 1 | **Dynamic model limits** — `ShowModel` + `ComputeLimits()` applied at startup and on `/model` switch; `MaxOutputTokens`/`MaxResultChars` now derived from actual model | `internal/llm/limits.go`, `internal/llm/ollama/ollama.go`, `internal/cli/repl.go`, `internal/commands/registry.go` |
| 2 | **Self-info tool** — `GetConfig` tool reads live model config from Ollama at call time via `func() string` closure | `internal/tools/selfinfo/selfinfo.go` |
| 3 | **Sub-agent model fix** — `agenttool` + `tasktool` use `func() string` getter (live `ActiveModel`); no more hardcoded `"qwen3"` / `"llama2"` fallback | `internal/tools/agenttool/agenttool.go`, `internal/tools/tasktool/tasktool.go` |
| 4 | **Memory system** — directory created at startup; `/memory list` shows warnings + pending + path; `/memory edit` seeds frontmatter template; `extractPending` emits HookNotice | `internal/cli/repl.go`, `internal/commands/registry.go`, `internal/memory/runner.go` |
| 5 | **Unlimited turns** — `MaxTurns = 0` means unlimited; default changed from 10→0; `Agent` tool can still pass explicit `max_turns` | `internal/agent/agent.go`, `internal/agent/input.go`, `internal/bootstrap/state.go` |

## Next Pending Phases

| Phase | Name | Notes |
|---|---|---|
| 17 | Distribution and install | `goreleaser`, brew formula, 5-platform binaries |
| 18 | Hardening, eval suite, docs | Release candidate v0.1.0 |
| 19 | Complete tool ecosystem | FileEdit, Glob, Grep, WebFetch, Todo (run before 18) |
| 20 | Content compaction | 4-layer strategy, `CompactionStarted`/`CompactionCompleted` events |
| 21 | Web interface & HTTP API | SSE + `nandocodego serve` + embedded UI |
