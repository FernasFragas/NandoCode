# Remaining Phases Task Review

**Date:** 2026-06-22  
**Status:** Agent-ready review addendum for remaining active roadmap  
**Scope:** Carry-forward validation gates plus Phase 25, Phase 17, and Phase 18.

## Purpose

This document reviews the remaining implementation plans and adds missing execution detail where the existing phase docs are too broad. Phase 21, Phase 24, Ollama Cloud API key support, Phase 28, Phase 29, and the Go response-time refactor are now implemented. It complements:

- `docs/roadmap/NEXT-PHASES-IMPLEMENTATION-PLAN.md`
- `docs/roadmap/GATE-G0-PHASE-8-14-VALIDATION-PLAN.md`
- `docs/plans/CONTEXT-LATENCY-OPTIMIZATION-PLAN.md`
- `docs/plans/TASKS-TUI.md`
- `docs/phases/PHASE-22-DETAILED-PLAN.md`
- `docs/phases/PHASE-25-DETAILED-PLAN.md`
- `docs/phases/PHASE-17-DETAILED-PLAN.md`
- `docs/phases/PHASE-18-DETAILED-PLAN.md`

Phase 23 OpenAI-compatible adapter work is intentionally excluded from the active v0.1 roadmap.

## Active Order

1. Gate G0 - Phase 8-14 validation
2. Workstream CL - Context, latency, and project-scale analysis reliability
3. Phase 22 - Enhanced TUI and input handling live/manual evidence
4. Phase 25 - Remote / Bridge Mode
5. Phase 17 - Distribution and Install
6. Phase 18 - Hardening, Eval Suite, and Docs

## Cross-Phase Rules

- Do not start Phase 17 until every active phase above it is implemented and accepted.
- Phase 25 may proceed; Phase 21, Phase 24, Ollama Cloud API key support, Phase 28, and Phase 29 are implemented prerequisites.
- Do not add Phase 23/OpenAI adapter work unless the roadmap decision is explicitly reversed.
- Do not globally lower `num_ctx` as a latency shortcut.
- Treat remaining Phase 22 live/manual evidence as release validation. Do not reimplement completed Phase 21 server or Phase 24 coordinator work because of older ordering notes.
- Every phase must update `docs/phases/PHASE-LOG.md` with files changed, tests run, manual checks, known constraints, and exit-gate status.
- For code work, keep changes scoped to the owning phase. If a dependency gap is discovered, either fix it before continuing or record it as a blocker.

## Workstream CL Review - Context, Latency, And Analysis Reliability

### Reviewed Gap

The current plans identify the right problem: the app can support large context, but small prompts can pay too much latency, and huge project analysis can still overload context or end without a final answer. The plan needs explicit implementation boundaries so agents do not mix all CL work into one oversized patch.

### Required Slice Order

1. **CL-0 Trace and baseline**
2. **CL-1 Adaptive context policy**
3. **CL-2 Token-aware prompt assembly**
4. **CL-3 Fast memory recall default**
5. **CL-4 Hook timing and slow-stage visibility**
6. **CL-5 Checkpoint/resume and final-answer reliability**
7. **CL-6 Project-scale analysis workflow**
8. **CL-7 Retrieval before expansion**
9. **CL-8 User controls**

### Updated Status After Current Implementation

The current implementation has completed several foundations but not the full Workstream CL exit gate.

- **Implemented foundations:** CL-0 trace/slow-stage visibility, CL-1 adaptive context mode, CL-2 history-level prompt packing, CL-3 memory recall modes, CL-5 latest-checkpoint resume prompt, CL-7 `/analyze-project` retrieval injection, and most CL-8 control surface.
- **Implemented since last review:** CL-4 hook timing coverage now includes `PreToolUse`, `PostToolUse`, `PermissionDenied`, `Stop`, and `SessionEnd`; CL-5 checkpoint hardening now includes richer schema, stale guard, and `/checkpoint status|clear`; CL-6 foundations now include chunker, summary cache primitives, and evidence ledger persistence.
- **Implemented by the response-time refactor:** common prompts can bypass semantic retrieval and tool schemas, output budget defaults are larger, semantic retrieval has cache/light narrowing, context-pack file reads and index scanning are bounded-parallel, and TUI transcript/picker paths are optimized.
- **Still not fully implemented:** manual bounded-context eval/load evidence collection for CL workflow under realistic small/medium/large repos.

### Corrected Pre-Phase-22 Blocking List

Before Phase 22 starts, finish or explicitly defer the following with documented risk:

1. **Gate G0 manual evidence:** record `pass|fail|blocked` for Phases 8-14 in `docs/phases/PHASE-LOG.md`.
2. **Manual latency/evidence capture:** run small, medium, and large analysis scenarios and record `/trace last`, context mode, checkpoint/retrieval behavior, and final answer completeness.

### Implementation Notes

- CL-0 must land first. Without trace data, later latency changes are guesswork.
- CL-1 must preserve explicit user overrides. `auto` context mode should be default; `fixed` or explicit CLI/config should remain respected.
- CL-2 must model prompt parts explicitly. Avoid string-concatenation heuristics that cannot explain why content was included or skipped.
- CL-3 should default to non-LLM recall but keep `llm` mode available for quality comparison.
- CL-5 should build on the existing latest-checkpoint foundation but store enough state to resume a promised report, not merely conversation text.
- CL-6 should use cache files under cache/state dirs, never project source files, and should not require every source file to fit into one prompt.
- CL-7 should remain lexical/frecency first for v0.1. Do not introduce a vector database before the cache/evidence workflow works.
- CL-8 should expose controls through slash commands and config, but defaults must remain quality-preserving.

### Evidence To Collect

- Before/after timing for a small prompt.
- Before/after timing for a medium prompt with memory and hooks enabled.
- Effective `num_ctx`, estimated input tokens, prompt bytes, and first visible render time.
- Large `@dir` expansion report showing included/summarized/skipped files.
- Full project-analysis run showing cached summaries and cited final answer.

### Blockers

- Trace cannot be generated without leaking prompt/file content.
- Adaptive context breaks explicit large-context prompts.
- Explicit files are silently dropped.
- `continue` cannot resume the promised final artifact.
- Project analysis still requires too much raw prompt context because the map/reduce summary workflow is not implemented.

### Exit Criteria

- `/trace last` or equivalent shows stage timings and context budget.
- Small prompts use smaller context tiers.
- Large prompts preserve context up to model limit.
- Default memory recall avoids a pre-run LLM call.
- Large project analysis has a workflow path with cached summaries.
- Final-answer quality gate catches preamble-only completions.

## Phase 22 Review - Enhanced TUI And Input Handling

### Reviewed Gap

The original Phase 22 plan focused on Vim, keybindings, bracketed paste, and transcript virtualization. The updated roadmap now also requires ADR-001 and TASKS-TUI. Phase 22 must therefore be treated as the local ask/response UX stabilization phase, not only an input-refactor phase.

### Required Slice Order

1. Close or verify `P0-15` editor invocation suspension.
2. UI-0 modal and permission rendering correctness.
3. UI-1 animated run status.
4. UI-5 semantic style roles.
5. UI-2 tool elapsed time.
6. UI-3 queue, retry, and compaction visibility.
7. UI-4 permission modal and help polish.
8. Streaming render optimization from the context-latency plan.
9. Original Phase 22 input/render scope.
10. UI-6 hierarchical activity display.
11. UI-7 `/btw` side question.

### Implementation Notes

- Keep `RunUIState` TUI-local unless another package truly needs it.
- Status priority should be deterministic: permission > running tool > retry/compaction > streaming/waiting > queued > idle.
- Tick scheduling must stop when idle. A stuck tick loop is a bug.
- Streamed assistant text should render cheaply while streaming and markdown-render once when finalized.
- Modal fixes must not alter permission resolver semantics.
- `/btw` must use a read-only isolated context and must not mutate the active run history unless explicitly designed to do so.

### Evidence To Collect

- Snapshot/render tests for status phases.
- Tests proving no tick loop remains after terminal/abort.
- Tests for permission modal wrapping and Escape behavior.
- Benchmarks or tests for long streamed responses.
- Manual transcript showing waiting, streaming, tool, permission, retry, compaction, queued, and done states.

### Remaining Risks

- Live REPL evidence still needs to prove the status bar covers waiting, streaming, tool, permission, retry, compaction, queued, background, and done states.
- Permission modal priority and wrapping need live evidence across modal/picker/context-stack combinations.
- Full textarea-integrated Vim mutations, dot-repeat, find-repeat, registers, and paste/yank semantics remain partial.
- Hierarchical activity display, full click-to-expand tool panels, and mouse lost-release recovery remain follow-ups.
- `/btw` is implemented as isolated read-only side-question flow, but true concurrent side execution and a reduced read-only model-visible tool manifest remain follow-ups.

### Exit Criteria

- Users can tell what the app is doing without reading logs.
- Long streaming responses stay responsive.
- Modal, picker, queue, retry, compaction, and activity display are tested.
- Original Phase 22 manual checks still pass.

## Phase 25 Review - Remote / Bridge Mode

### Reviewed Gap

Phase 25 must remain Ollama/local-provider based for v0.1. It depends on Phase 21 server mode and Phase 24 coordination, not on Phase 23. The core risk is security and event replay correctness.

### Required Slice Order

1. Extend server sessions for detach/reattach.
2. Add per-session event ring buffer and event IDs.
3. Add reconnect protocol using `Last-Event-ID` or explicit offset.
4. Add JWT auth and refresh.
5. Add `nandocodego connect`.
6. Add `internal/tui/remote_bridge.go`.
7. Add remote permission prompt round trip.
8. Add server-side UDS listener for Phase 24 routing.
9. Add detached-session cleanup and lifecycle docs.

### Implementation Notes

- Client never executes tools; server executes all tools.
- Client does not configure LLM; server owns model/config.
- Tokens and auth headers must never be logged.
- Server restart can invalidate in-memory JWT secrets; this must be documented.
- Ring buffer overflow must produce a clear gap event or reconnect failure.
- Permission prompt timeout behavior should match local TUI expectations.

### Evidence To Collect

- Connect prompt streams from server to client.
- Disconnect/reconnect replays missed events.
- Agent continues running while detached.
- Permission prompt appears in client TUI and decision reaches server.
- JWT expiry/refresh behavior is tested.

### Blockers

- Client can execute tools locally.
- Reconnect duplicates or drops events silently.
- JWT/token leaks to logs or disk.
- Detached sessions leak forever.
- Permission prompt cannot recover after disconnect.

### Exit Criteria

- Connect, detach, reconnect, permission, and replay flow works.
- Server-side tasks/sub-agents continue while client detached.
- Auth tests and race tests pass.
- Remote docs clearly state v0.1 provider scope is Ollama/local.

## Phase 17 Review - Distribution And Install

### Reviewed Gap

Phase 17 should package the complete product surface only. It must not absorb unfinished feature work from Workstream CL or Phases 22/21/24/25.

### Required Slice Order

1. Confirm all previous gates/phases are accepted.
2. Add GoReleaser snapshot builds.
3. Embed version metadata.
4. Generate checksums for uploaded artifacts.
5. Add direct installer with checksum verification before install.
6. Add release workflow after CI/pre-release checks.
7. Enhance `doctor` for release readiness.
8. Update changelog through Phase 17.
9. Add optional Homebrew/Scoop templates only if publishing prerequisites exist.

### Implementation Notes

- `doctor` must not do network probes by default.
- `doctor --ollama` and `doctor --mcp` may perform opt-in network checks.
- Do not fail releases because Homebrew/Scoop tokens are absent; gate those publishers.
- Checksums must cover every uploaded artifact.
- Release workflow must not call a non-reusable CI workflow unless `workflow_call` exists.

### Evidence To Collect

- Snapshot build output for target platforms.
- Checksum file and verification.
- Installer dry run or temp-dir install.
- `nandocodego --version`.
- `nandocodego doctor`, `doctor --ollama`, `doctor --mcp`.
- Release workflow syntax validation.

### Blockers

- Installer executes downloaded code before checksum verification.
- Release artifacts lack checksums.
- Default doctor fails because local network service is down.
- Build misses server/connect/coordinator features.
- CI release flow can publish without tests.

### Exit Criteria

- Five-platform snapshot builds run.
- Installer verifies before install.
- Doctor behavior is release-ready.
- Changelog and Phase 17 log entry are complete.

## Phase 18 Review - Hardening, Eval Suite, And Docs

### Reviewed Gap

Phase 18 must be a final release gate, not a new feature bucket. It should harden everything already implemented, including Workstream CL, TUI, server, coordinator, and remote/bridge.

### Required Slice Order

1. Build eval scenario suite.
2. Add large-project analysis evals.
3. Add TUI/ask-response reliability checks where automatable.
4. Add server/remote/coordinator end-to-end evals.
5. Add fuzz/property tests.
6. Run security tooling and fix findings.
7. Verify performance gates.
8. Complete docs site/user docs/security docs.
9. Finalize changelog.
10. Record release approval or blockers.

### Implementation Notes

- Evals should include local Ollama model matrix where feasible.
- Security review must include hooks, MCP HTTP, server auth, remote JWT, installer, file writes, and task/sub-agent paths.
- Docs must clearly state v0.1 provider scope: Ollama/local-provider based, no OpenAI adapter.
- Known limitations must be specific and non-blocking.
- Any release trust issue is a blocker by default.

### Evidence To Collect

- `make eval` or equivalent output.
- `go test -race ./...`.
- fuzz/property test output.
- `gosec` and `govulncheck` output.
- link/doc check output.
- binary size/performance measurements.
- external/manual smoke sign-off notes.

### Blockers

- Security findings without documented mitigation.
- Eval suite cannot pass minimum threshold.
- Docs claim unsupported OpenAI/provider behavior.
- Server/remote/coordinator flows lack any release validation.
- Phase 17 packaging blocker remains open.

### Exit Criteria

- No further v0.1 implementation phase is needed.
- All release blockers closed or explicitly accepted as non-blocking limitations.
- v0.1.0 release approval is recorded in `docs/phases/PHASE-LOG.md`.

## Documentation Updates Required For Every Phase

For each phase/workstream completion:

- Update `docs/phases/PHASE-LOG.md`.
- Update the phase plan status line if the plan is in `docs/phases/PHASE-*.md`.
- Update `docs/roadmap/PROJECT-STATUS-AND-ONBOARDING.md` if the implementation reality changes.
- Update `docs/roadmap/BACKLOG.md` if a backlog item is closed or promoted.
- Add or update manual test docs when live validation is required.
