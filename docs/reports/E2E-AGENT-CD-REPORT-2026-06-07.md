# E2E Agent CD Report - 2026-06-07

## Scope

- Lane: `C` and `D`
- Owner agent: `Lane CD worker`
- Functional areas:
  - Lane C: core agent loop, tool execution, permissions, retries, compaction, cheap-prompt routing
  - Lane D: TUI prompt flow, slash commands, transcript behavior, backgrounding, permissions modal, semantic index UX
- Source commit: `cd6743c7968ea6c809859a9a1f8a8f30ea930e88`
- Start/end time:
  - Start: `2026-06-07T11:24:00+01:00`
  - Last updated: `2026-06-07T11:34:00+01:00`

## Environment

- OS: `Darwin 25.5.0 arm64` / `macOS 26.5.1 (25F80)`
- Go version: `go version go1.26.2 darwin/arm64`
- Ollama status: `ollama --version` reported client `0.30.5`; localhost runtime reachable only outside sandbox; `ollama list` succeeded with escalation
- Model/provider: local Ollama model `qwen3.6:35b`; local embedding model `qwen3-embedding:8b`; cloud model also listed: `kimi-k2.6:cloud`
- Isolated config/data/cache/state paths: `/private/tmp/nandocodego-lane-cd.Bk2o0Z/{config,data,cache,state}`
- Browser/terminal details where relevant: `TTY execution pending for live TUI scenarios`; non-interactive runs executed from `/private/tmp/nandocodego-lane-cd.Bk2o0Z/workspace`

## Scenario Results

Checkpoint status. Live execution is in progress; only executed scenarios are listed here with final evidence, and the remaining lane scenarios are still pending active execution.

| Scenario | Priority | Automation | Evidence | Status | Bug/Block | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| C-001 plain chat prompt with no tools | p0 | automated | E2 | pass |  | `--print 'Respond with exactly OK.' --json` returned `{"content":"OK","tool_uses":[]}` |
| C-002 tool call execution with successful result | p0 | automated | E2 | pass |  | `--print 'Summarize @note.txt in five words.' --json` produced one `FileRead` tool call with successful output |
| C-012 `ToolModeNone` path for cheap prompts | p1 | automated | E2 | fail | [BUG-20260607-print-cheap-prompt-includes-tool-schemas](./bugs/BUG-20260607-print-cheap-prompt-includes-tool-schemas.md) | Two trivial `--print` prompts persisted prompt dumps with `tool_count: 9` instead of omitting tool schemas |
| C-013 prompt evidence pack generation for explicit file context | p1 | automated | E2 | pass |  | Prompt dump recorded `FilesReferenced: 1`, `FilesRaw: 1`, `RawBytesIncluded: 17`, and user message preview included expanded `@note.txt` content |

## Coverage Notes

- Functional paths covered:
  - Live `--print` entrypoint against local Ollama
  - Prompt dump persistence in isolated state
  - Explicit file-context packing path with public fixture data
  - Successful tool execution path via `FileRead`
- Positive paths covered:
  - Plain non-tool chat completion
  - Successful tool invocation
  - Prompt evidence-pack metadata for explicit file context
- Negative/error paths covered:
  - Cheap-prompt request-shaping defect in print mode
  - Initial sandbox-localhost denial from `ollama list`; confirmed as environment boundary, not product failure, by rerunning with escalation
- Performance or reliability evidence captured:
  - `C-001` wall-clock from usage: `11.10s`
  - `C-002` wall-clock from usage: `8.45s`
  - Cheap-prompt repro rerun wall-clock: `11.38s`
- Known coverage gaps:
  - No TUI PTY evidence yet in this checkpoint
  - Permission modal, slash-command, queue, `/bg`, `/btw`, and resize coverage still in progress
  - Retry/compaction/max-turn/unknown-tool/parse-error scenarios still need either live repro or block classification

## Findings

| Finding | Severity | Disposition | Impacted Scenarios | Evidence |
| --- | --- | --- | --- | --- |
| `--print` cheap prompts still include all tool schemas in persisted prompt dumps | sev3_medium | confirmed | C-012 | Two independent `--print` runs plus prompt-dump JSON |

## Bugs

- [BUG-20260607-print-cheap-prompt-includes-tool-schemas](./bugs/BUG-20260607-print-cheap-prompt-includes-tool-schemas.md): cheap `--print` prompts do not omit tool schemas, contradicting the `ToolModeNone` expectation for low-cost requests.

## Blocks

- None filed yet in this checkpoint. Remaining scenarios are still under active execution rather than formally blocked.

## Risk Assessment

- Top user-facing risks:
  - Print-mode cheap prompts may spend unnecessary context budget on tool schemas
  - TUI permission and background workflows are not yet evidenced in this checkpoint
- Top release risks:
  - Request-shaping divergence between print mode and TUI/route-based paths
  - Several release-critical TUI/permission scenarios remain unproven until PTY execution completes
- Top test-confidence risks:
  - This checkpoint is strong for non-interactive print mode and weak for interactive TUI flows
  - No second-environment reproduction yet for the current bug

## Rerun Recommendation

- Scenarios to rerun immediately:
  - C-012 after a fix in the print path
  - TUI cheap-prompt equivalent to compare print and TUI behavior
- Scenarios to rerun after fixes:
  - Any print-mode prompt-shaping regression around explicit context and tool routing
- Scenarios that need a different environment:
  - None declared yet; TUI PTY execution is expected to run in the current environment

## Lane Recommendation

- `blocked`

Checkpoint rationale: execution is active and already produced one confirmed defect plus three completed Lane C scenarios, but most Lane C and all Lane D scenarios still need live evidence before this lane can recommend pass or pass_with_known_non_blockers.
