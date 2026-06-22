# Regression And Load Test Plan

**Date:** 2026-05-17  
**Project:** `nandocodego`  
**Purpose:** verify that the features already implemented in the application still work after recent context, latency, checkpoint, retrieval, TUI, and reliability changes.

This document is agent-ready. Use it as the regression and load-test checklist before starting Phase 22, before large refactors, and before release hardening.

## Scope

This plan covers behavior that is supposed to work now:

- CLI bootstrap, config loading, `doctor`, `version`, and `--print`.
- Ollama-backed `llm.Client` integration shape, model list/show/pull paths, limits, retry, and stream watchdog behavior.
- Agent loop: streaming, tool calls, tool results, permissions, length retry, incomplete-response retry, compaction, max-turn safety, terminal reasons, and usage accounting.
- Built-in tools, complete tool ecosystem, path safety, snapshots, result truncation, and permission classification.
- State/bootstrap layer, TUI store behavior, queued prompts, permission broker, transcript rendering, thinking visibility, slash commands, inline completion, directory mentions, trace visibility, prompt packing, checkpoint resume, and `/analyze-project` retrieval injection.
- Memory, hooks, MCP, sub-agents/fork, skills, tasks, concurrency/speculative execution, observability, and metrics.
- Workstream CL foundations already landed before Phase 22: trace, adaptive context, history prompt packing, memory recall modes, slow-stage notices, incomplete response recovery, latest checkpoint resume, and `/analyze-project` retrieval ranking.

This plan does **not** require unimplemented future phases to pass:

- Phase 21 HTTP/server/browser mode.
- Phase 22 enhanced TUI work not yet implemented.
- Phase 24 multi-agent coordination.
- Phase 25 remote/bridge mode.
- Phase 17 packaging/release workflow.
- Phase 18 final release eval suite.
- CL-6 full map/reduce project-analysis workflow and summary cache.

## Implementation Status

The following automation from this plan is implemented in the repository:

- `tools/run-regression-fast.sh`: fast regression gate runner.
- `tools/run-regression-full.sh`: full regression gate runner (`go test`, race subset, `go vet`, dependency and network policy checks).
- `tools/run-load-suite.sh`: repeated unit load, race load, render benchmarks, directory expansion load tests, and context/retrieval/checkpoint load tests.
- `Makefile` targets: `regression-fast`, `regression-full`, and `load-suite`.
- New load/benchmark tests:
  - `internal/tui/render_benchmark_test.go` for transcript rendering under large streaming/tool/thinking/system loads.
  - `internal/mentions/expand_test.go` large-directory budget fixture (1000 files).
  - `internal/agent/prompt_packer_test.go` determinism and large-history benchmark.
  - `internal/analysis/retrieval_test.go` determinism and 50k-entry retrieval benchmarks.
  - `internal/analysis/checkpoint_test.go` repeated checkpoint write/load and missing-file behavior.

Current execution note:

- `internal/tools/webfetch` is excluded by the regression scripts in this sandboxed environment because `httptest` local listener bind is not permitted here. This is explicitly reported by the scripts after each run.

## Test Environment

Use isolated state/config/cache dirs so tests do not depend on a developer machine:

```sh
export NANDOCODEGO_CONFIG_HOME="$(mktemp -d)"
export NANDOCODEGO_DATA_HOME="$(mktemp -d)"
export NANDOCODEGO_CACHE_HOME="$(mktemp -d)"
export NANDOCODEGO_STATE_HOME="$(mktemp -d)"
export GOCACHE=/private/tmp/go-nandocodego-gocache
```

For live model checks:

```sh
export NANDOCODEGO_TEST_OLLAMA=1
export OLLAMA_HOST=http://localhost:11434
```

Recommended local models for live tests:

- Fast smoke: any small installed chat model.
- Thinking/TUI validation: a thinking-capable model such as `qwen3`.
- Large-context smoke: an installed model whose `ShowModel` metadata reports a context length above 32k.

## Fast Regression Gate

Run this gate after every feature patch.

```sh
go test ./internal/agent/... \
  ./internal/analysis/... \
  ./internal/bootstrap/... \
  ./internal/cli/... \
  ./internal/commands/... \
  ./internal/config/... \
  ./internal/hooks/... \
  ./internal/memory/... \
  ./internal/observability/... \
  ./internal/permissions/... \
  ./internal/state/... \
  ./internal/tools/... \
  ./internal/tui/...
```

Expected result:

- All packages pass.
- No race between TUI event handling and agent event delivery.
- No test requires an installed Ollama model unless it is explicitly gated by env vars.

## Full Local Regression Gate

Run before merging large behavior changes.

```sh
go test ./...
go test -race ./internal/agent/... ./internal/hooks/... ./internal/tasks/... ./internal/tui/... ./internal/state/...
go vet ./...
tools/check-allowed-deps.sh
tools/check-network-policy.sh
```

Expected result:

- `go test ./...` passes in the normal local environment.
- Race tests pass for stateful/concurrent packages.
- Dependency and network policy scripts pass.
- Any known sandbox-only failures must be documented with exact package, reason, and reproduction.

## Agent Task Board

Use these task cards when assigning work to agents. Each task is intentionally bounded and produces evidence that can be pasted into the regression report template.

### Task A0 - Regression Coordinator

**Goal:** run the overall regression effort, collect evidence, and decide whether Phase 22 can start.

**Owns:**

- This document.
- The final regression report.
- Cross-task blocker triage.

**Steps:**

1. Record current commit, Go version, OS, Ollama version, and installed model names.
2. Create isolated `NANDOCODEGO_*` dirs and export `GOCACHE`.
3. Run the fast regression gate.
4. Run the full local regression gate.
5. Assign A1-A14 and L0-L10 tasks to specialized agents.
6. Merge findings into the report template.
7. Mark each finding as `blocker`, `defer`, or `non-blocking`.

**Commands:**

```sh
git rev-parse HEAD
go version
go test ./internal/agent/... ./internal/analysis/... ./internal/bootstrap/... ./internal/cli/... ./internal/commands/... ./internal/config/... ./internal/hooks/... ./internal/memory/... ./internal/observability/... ./internal/permissions/... ./internal/state/... ./internal/tools/... ./internal/tui/...
go test ./...
go vet ./...
tools/check-allowed-deps.sh
tools/check-network-policy.sh
```

**Evidence to collect:**

- Command output summary, not full logs unless a failure occurs.
- Exact failing package/test names.
- Whether failures are deterministic.
- Final recommendation: `phase-22-ready`, `phase-22-ready-with-deferred-risk`, or `blocked`.

**Done when:**

- Every assigned task has `pass|fail|blocked`.
- All blockers have owners and reproduction steps.

### Task A1 - CLI, Config, Bootstrap Agent

**Goal:** verify process startup, config precedence, path setup, and print-mode behavior.

**Owns:** R0.

**Inspect:**

- `internal/cli`
- `internal/config`
- `internal/bootstrap`
- `internal/state`
- `internal/paths`

**Commands:**

```sh
go test ./internal/cli/... ./internal/config/... ./internal/bootstrap/... ./internal/state/...
go run ./cmd/nandocodego version
go run ./cmd/nandocodego doctor
go run ./cmd/nandocodego --help
go run ./cmd/nandocodego init
go run ./cmd/nandocodego --print "say ok"
```

**Extra checks:**

- Confirm `NANDOCODEGO_CONFIG_HOME`, `NANDOCODEGO_DATA_HOME`, `NANDOCODEGO_CACHE_HOME`, and `NANDOCODEGO_STATE_HOME` are respected.
- Confirm generated config comments match current defaults.
- Confirm `--print` does not create a Bubble Tea TUI.

**Done when:**

- R0 pass criteria are satisfied or each failure has exact repro and expected/actual output.

### Task A2 - LLM And Model Limits Agent

**Goal:** verify Ollama client behavior, model metadata parsing, limits, retries, and stream failure handling.

**Owns:** R1 plus live Ollama smoke when available.

**Inspect:**

- `internal/llm`
- `internal/llm/ollama`
- `internal/observability/llm.go`
- `internal/cli/repl.go`

**Commands:**

```sh
go test ./internal/llm/... ./internal/llm/ollama/...
go run ./cmd/nandocodego --model qwen3:latest --print "reply with one short sentence"
```

**Extra checks:**

- If `qwen3:latest` is not installed, record installed model used instead.
- Confirm `ShowModel` updates context/output/result limits.
- Confirm thinking-capable model requests thinking and non-thinking model does not.

**Done when:**

- Automated tests pass.
- Live smoke either passes or is marked blocked with model/Ollama availability details.

### Task A3 - Agent Loop And Recovery Agent

**Goal:** verify the core model/tool loop and terminal-state behavior.

**Owns:** R2.

**Inspect:**

- `internal/agent/agent.go`
- `internal/agent/stream.go`
- `internal/agent/incomplete_response.go`
- `internal/agent/prompt_packer.go`
- `internal/agent/compact.go`

**Commands:**

```sh
go test ./internal/agent/...
go test ./internal/agent/... -run 'TestAgentRunLengthRetry|TestAgentRunIncomplete|TestAgentRunEmitsPromptPackReport|TestInputValidation'
```

**Extra checks:**

- Confirm terminal events are emitted once.
- Confirm `max_turns` does not produce hanging goroutines.
- Confirm prompt packing uses packed history only for the model request and does not mutate stored history.

**Done when:**

- R2 pass criteria are satisfied and recovery regressions are either fixed or documented as blockers.

### Task A4 - Tools And Permission Agent

**Goal:** verify tool safety, path safety, permission gating, snapshots, and result limits.

**Owns:** R3.

**Inspect:**

- `internal/tools`
- `internal/permissions`
- `internal/tools/builtin`

**Commands:**

```sh
go test ./internal/tools/... ./internal/permissions/...
go test ./internal/tools/fileedit/... ./internal/tools/fileread/... ./internal/tools/filewrite/... ./internal/tools/webfetch/...
```

**Extra checks:**

- Confirm dangerous Bash commands are classified.
- Confirm writes are denied in plan/dont-ask modes unless rules allow them.
- Confirm file edit refuses stale snapshots.
- Confirm WebFetch does not allow unexpected external network behavior.

**Done when:**

- R3 pass criteria are satisfied and any safety regression is marked as a blocker.

### Task A5 - TUI Core Agent

**Goal:** verify transcript, event handling, status, queue, permission modal, thinking, checkpoint resume, and `/analyze-project` UI plumbing.

**Owns:** R4.

**Inspect:**

- `internal/tui/app.go`
- `internal/tui/transcript.go`
- `internal/tui/messages.go`
- `internal/tui/permission.go`
- `internal/tui/slash.go`

**Commands:**

```sh
go test ./internal/tui/...
go test ./internal/tui/... -run 'TestIncompleteResponse|TestPromptSubmission|TestContinueUsesCheckpoint|TestAnalyzeProject|TestHandleAgentEvent'
```

**Extra checks:**

- Confirm `handleAgentEvent` recovers from render panic.
- Confirm stage summary and slow-stage notices do not spam the transcript.
- Confirm queued prompt processing cannot leave `ActiveRun=true` forever.

**Done when:**

- R4 pass criteria are satisfied and all event/state regressions have exact reproduction steps.

### Task A6 - Slash Commands Agent

**Goal:** verify command registry behavior and session state mutations.

**Owns:** R5.

**Inspect:**

- `internal/commands/registry.go`
- `internal/commands/registry_test.go`

**Commands:**

```sh
go test ./internal/commands/...
```

**Manual TUI commands:**

```text
/help
/cost
/trace last
/trace threshold 1s
/context status
/context small
/context large
/memory recall
/memory recall off
/permissions show
/hooks list
/skills list
/agents list
/compact
/refresh-index
/analyze-project . summarize project status
```

**Done when:**

- Invalid usage returns clear errors.
- Valid session commands mutate expected app state only.

### Task A7 - Memory Agent

**Goal:** verify memory scan, pending drafts, recall modes, and two-session behavior.

**Owns:** R6 and Gate G0 Phase 8 evidence.

**Inspect:**

- `internal/memory`
- `internal/commands/registry.go` memory command paths
- `internal/cli/repl.go` memory setup

**Commands:**

```sh
go test ./internal/memory/... ./internal/commands/... -run 'Memory|memory'
```

**Manual live flow:**

1. Start TUI with isolated data dir.
2. Ask it to remember a small fact.
3. Exit.
4. Start a new session in the same project.
5. Ask a related question.

**Done when:**

- Memory behavior passes or Phase 8 Gate G0 evidence is marked `fail|blocked` with repro.

### Task A8 - Hooks Agent

**Goal:** verify hook loading, command/prompt hook execution, safety boundaries, and timing coverage.

**Owns:** R7 and Gate G0 Phase 9 evidence.

**Inspect:**

- `internal/hooks`
- hook integration in `internal/cli/repl.go`
- TUI hook notices

**Commands:**

```sh
go test ./internal/hooks/...
```

**Manual live flow:**

1. Configure a user command hook that blocks `Bash(rm -rf*)`.
2. Start TUI.
3. Ask model to run a destructive command.

**Known gap to report separately:**

- Missing timing for pre-tool, post-tool, permission-denied, stop, and session-end hooks.

**Done when:**

- Hook blocking demo is recorded and missing timing work is listed as CL-4 follow-up or fixed.

### Task A9 - MCP Agent

**Goal:** verify MCP config, stdio server lifecycle, tool wrapping, timeout, and untrusted-content behavior.

**Owns:** R8 and Gate G0 Phase 10 evidence.

**Inspect:**

- `internal/mcp`
- `internal/tools/mcptool`
- MCP config docs/examples

**Commands:**

```sh
go test ./internal/mcp/... ./internal/tools/mcptool/...
```

**Manual live flow:**

1. Configure a simple local stdio MCP server.
2. Start TUI.
3. Ask model to call one MCP tool.
4. Exit and confirm process cleanup.

**Done when:**

- MCP live tool call is recorded or blocked due to missing server fixture.

### Task A10 - Sub-Agent And Fork Agent

**Goal:** verify sub-agent lifecycle, model inheritance, cancellation, JSONL output, and recursion prevention.

**Owns:** R9 and Gate G0 Phase 11 evidence.

**Inspect:**

- `internal/agent/subagent.go`
- `internal/agent/fork.go`
- `internal/tools/agenttool`

**Commands:**

```sh
go test ./internal/agent/... ./internal/tools/agenttool/...
go test ./internal/agent/... -run 'TestRunSubagent|TestRunFork'
```

**Manual live flow:**

1. Ask the main agent to delegate a bounded read-only analysis.
2. Ask for background execution.
3. Cancel parent run and confirm child cancellation.

**Done when:**

- Gate G0 Phase 11 evidence has `pass|fail|blocked` and model inheritance is verified.

### Task A11 - Skills Agent

**Goal:** verify skill discovery, parsing, hot reload, source handling, and prompt boundary behavior.

**Owns:** R10 and Gate G0 Phase 12 evidence.

**Inspect:**

- `internal/skills`
- `internal/tools/skilltool`
- project/user skills dirs

**Commands:**

```sh
go test ./internal/skills/... ./internal/tools/skilltool/...
```

**Manual live flow:**

1. Add a project skill with valid frontmatter.
2. Start TUI and run `/skills list`.
3. Modify skill file.
4. Confirm reload notice appears.

**Done when:**

- Gate G0 Phase 12 evidence has `pass|fail|blocked`.

### Task A12 - Tasks Agent

**Goal:** verify background task lifecycle, output, stop, and session isolation.

**Owns:** R11 and Gate G0 Phase 14 evidence.

**Inspect:**

- `internal/tasks`
- `internal/tools/tasktool`
- task output path handling in `internal/paths`

**Commands:**

```sh
go test ./internal/tasks/... ./internal/tools/tasktool/...
```

**Manual live flow:**

1. Start a background task.
2. List tasks.
3. Fetch output.
4. Stop task.

**Done when:**

- Task lifecycle has manual evidence and no cross-session data leakage.

### Task A13 - Observability And Trace Agent

**Goal:** verify metrics, trace, cost, retry diagnostics, and no prompt-content leakage.

**Owns:** R13.

**Inspect:**

- `internal/observability`
- `/trace` and `/cost` command handlers
- TUI stage summary handling

**Commands:**

```sh
go test ./internal/observability/... ./internal/commands/...
```

**Manual smoke:**

```text
/trace last
/cost
```

**Done when:**

- Trace/cost output includes timing and retry diagnostics without logging prompt/file bodies.

### Task A14 - Context, Checkpoint, Retrieval Agent

**Goal:** verify adaptive context, prompt packing, latest checkpoint resume, and `/analyze-project` retrieval ranking.

**Owns:** R14 and Workstream CL foundation evidence.

**Inspect:**

- `internal/agent/context_policy.go`
- `internal/agent/prompt_packer.go`
- `internal/analysis`
- TUI `/analyze-project` and `continue` paths

**Commands:**

```sh
go test ./internal/agent/... ./internal/analysis/... ./internal/tui/... -run 'Context|PromptPack|Checkpoint|Retrieve|AnalyzeProject|Continue'
```

**Extra checks:**

- Verify explicit user mentions remain visible and are not replaced by retrieval.
- Verify checkpoint resume only fires for pending checkpoint state.
- Verify prompt packer is deterministic.

**Done when:**

- R14 pass criteria are satisfied or each CL regression is listed as pre-Phase-22 blocker.

### Task L0 - Load Test Harness Agent

**Goal:** turn load scenarios L0-L6 into repeatable tests or benchmarks where missing.

**Owns:** L0-L6 automation.

**Inspect:**

- Existing benchmark/test coverage in `internal/tui`, `internal/mentions`, `internal/agent`, `internal/analysis`.

**Implementation tasks:**

- Add or improve benchmarks for transcript rendering and assistant delta handling.
- Add fake large directory fixture tests for mention expansion caps.
- Add prompt-packer determinism/load test with hundreds of messages.
- Add retrieval load test with tens of thousands of entries.
- Add checkpoint repeated-write/load test.

**Commands after implementation:**

```sh
go test ./internal/tui/... -bench 'Render|Transcript|AssistantDelta' -benchmem
go test ./internal/mentions/... ./internal/tools/dirwalk/... -run 'Large|Directory|Budget'
go test ./internal/agent/... ./internal/analysis/... -run 'Load|Large|Deterministic|Checkpoint|Retrieve'
```

**Done when:**

- Missing load tests are implemented or explicitly listed as future test debt.

### Task L1 - Live Ollama Evaluation Agent

**Goal:** run live L7-L10 scenarios and collect `/trace last` evidence.

**Owns:** L7-L10 live validation.

**Prerequisites:**

- Ollama running.
- At least one chat model installed.
- Thinking-capable model installed if validating thinking behavior.

**Scenarios:**

```text
reply with exactly "ok"
inspect README.md and docs/PHASE-LOG.md, then summarize current roadmap in 5 bullets
/analyze-project . identify missing pre-phase-22 implementation work and cite the files used
analyze the architecture and list risks, tradeoffs, and prioritized fixes
```

**Evidence to collect after each run:**

- Prompt used.
- Model used.
- `/trace last` output.
- `/cost` output.
- First visible response observation.
- Terminal reason and done reason.
- Whether final answer was complete.

**Done when:**

- Small, medium, and large scenarios have pass/fail evidence.
- Any incomplete response is checked against retry/checkpoint behavior.

### Task G0 - Manual Phase Gate Agent

**Goal:** close or explicitly block the manual acceptance gates for Phases 8-14.

**Owns:** Gate G0 evidence.

**Source doc:**

- `docs/GATE-G0-PHASE-8-14-VALIDATION-PLAN.md`

**Outputs:**

- Add a `Gate G0 Evidence` entry to `docs/PHASE-LOG.md`.
- Each phase 8-14 gets `pass`, `fail`, or `blocked`.
- Each fail/blocker includes exact reproduction and owner.

**Done when:**

- Phase 22 readiness no longer depends on vague "manual validation pending" wording.

## Regression Matrix

### R0 - CLI, Config, Bootstrap

Automated checks:

- `go test ./internal/cli/... ./internal/config/... ./internal/bootstrap/... ./internal/state/...`

Manual smoke:

```sh
go run ./cmd/nandocodego version
go run ./cmd/nandocodego doctor
go run ./cmd/nandocodego --help
go run ./cmd/nandocodego init
go run ./cmd/nandocodego --print "say ok"
```

Pass criteria:

- `doctor` prints config/data/cache/state dirs and writable status.
- `init` creates default config without overwriting unexpectedly.
- Config values propagate to bootstrap and app state.
- `--print` expands mentions and exits without starting the TUI.

### R1 - LLM Client And Model Limits

Automated checks:

- `go test ./internal/llm/... ./internal/llm/ollama/...`

Live checks:

```sh
go run ./cmd/nandocodego --model qwen3:latest --print "reply with one short sentence"
```

Pass criteria:

- `ShowModel` metadata is parsed when available.
- `ComputeLimits` applies model-specific output/result/context limits.
- Watchdog timeout and stream error done reasons produce unrecoverable terminal behavior, not silent success.
- Thinking mode is requested only for models marked as thinking-capable.

### R2 - Agent Loop And Recovery

Automated checks:

- `go test ./internal/agent/...`

Required cases:

- No-tool streaming completes.
- Tool-call turn executes and feeds tool results back to the model.
- Unknown/malformed tool calls return controlled errors.
- Permission-denied tool calls are surfaced.
- Length done reason retries once with expanded output budget and then compacts or exits cleanly.
- Incomplete assistant response such as `Let me write the summary:` triggers one recovery retry.
- `max_turns` exits with `TerminalMaxTurns`, not a hang.
- Prompt packing emits `PromptPackReport` only when trimming occurs.

Pass criteria:

- Terminal event is always emitted exactly once.
- Usage counters preserve prompt tokens, eval tokens, tool calls, turns, and done reason.
- Agent does not mutate caller message slices.

### R3 - Tools And Permissions

Automated checks:

- `go test ./internal/tools/... ./internal/permissions/...`

Required tool coverage:

- Bash classification and timeout.
- FileRead/FileWrite/FileEdit snapshots and path safety.
- Glob/Grep directory traversal caps.
- WebFetch policy and local-fetch gating.
- TodoRead/TodoWrite state path.
- Agent/task/self-info tools do not hardcode model names.

Pass criteria:

- Path traversal is blocked.
- Writes require appropriate permission mode/rule.
- Tool results are truncated to model-facing limits without corrupting display.
- File snapshots protect edit preconditions.

### R4 - TUI Core Behavior

Automated checks:

- `go test ./internal/tui/...`

Required cases:

- Assistant text appears in transcript.
- Thinking deltas create a collapsed thinking block and `Ctrl+T` toggles it.
- Non-completed terminal reasons appear in transcript.
- Tool progress/results update the correct transcript item even when tools interleave.
- Permission modal state clears on decision/cancel.
- Queued prompts drain after the active run finishes.
- Status bar labels cumulative tokens as `session tokens`.
- Slow-stage notices dedupe by stage per run.
- Terminal stage summary includes only stages above threshold.
- Prompt submission expands `@file` and `@dir` mentions while keeping the user's visible prompt.
- `/analyze-project` injects retrieval-ranked file mentions when the index is available.
- `continue` uses the checkpoint resume prompt only when a pending checkpoint exists.

Pass criteria:

- No panic in `handleAgentEvent`.
- Transcript refresh survives malformed/large tool output.
- Active run state returns to idle after terminal/channel close.

### R5 - Slash Commands

Automated checks:

- `go test ./internal/commands/...`

Manual command smoke in TUI:

```text
/help
/cost
/trace last
/trace threshold 1s
/context status
/context small
/context large
/memory recall
/memory recall off
/permissions show
/hooks list
/skills list
/agents list
/compact
/refresh-index
/analyze-project . summarize project status
```

Pass criteria:

- Commands return user-facing errors for invalid usage.
- Session-only commands mutate session state without writing unrelated config.
- `/trace last` shows retry/done reason/stage timing when available.
- `/context` updates app state and next agent run input.
- `/memory recall` updates app state and memory runner behavior.

### R6 - Memory

Automated checks:

- `go test ./internal/memory/...`

Manual live validation:

1. Start TUI with isolated data dir.
2. Ask it to remember a small project fact.
3. End session.
4. Start a new session in the same project.
5. Ask a related question.

Pass criteria:

- Pending memory drafts are created and visible with `/memory list`.
- Valid frontmatter files are scanned.
- Invalid memory files are reported as skipped, not silently ignored.
- `recall_mode=fast` does not make a recall-side LLM call.
- `recall_mode=llm` preserves the previous LLM recall behavior.

### R7 - Hooks

Automated checks:

- `go test ./internal/hooks/...`

Manual live validation:

1. Configure a user command hook that blocks `Bash(rm -rf*)`.
2. Start TUI.
3. Ask the model to run a destructive command.

Pass criteria:

- Hook blocks before execution.
- TUI shows hook notice/terminal stop reason.
- Hook snapshot is frozen for the run.
- Project-controlled hooks are parsed/reported but not executed without trust support.
- Stage timings exist for session-start and user-prompt hooks.

Known current gap to test after implementation:

- Pre-tool, post-tool, permission-denied, stop, and session-end hook timing.

### R8 - MCP

Automated checks:

- `go test ./internal/mcp/... ./internal/tools/mcptool/...`

Manual live validation:

1. Configure a simple stdio MCP server.
2. Start TUI.
3. Ask the model to call the MCP tool.
4. Exit the session.

Pass criteria:

- MCP server starts and stops cleanly.
- Wrapped MCP tool schema is visible to the model.
- Tool result returns through normal agent events.
- Timeout/cancel does not orphan the MCP process.
- Untrusted MCP content is not treated as trusted system instruction.

### R9 - Sub-Agents And Fork

Automated checks:

- `go test ./internal/agent/... ./internal/tools/agenttool/...`

Manual live validation:

1. Ask the main agent to delegate a bounded read-only analysis.
2. Ask for background execution.
3. Cancel the parent run.

Pass criteria:

- Child uses current active model, not a hardcoded fallback.
- Child receives bounded context and isolated state.
- Recursive sub-agent spawning is prevented.
- Parent abort cascades to child.
- Background JSONL includes thinking, retries, tool events, compaction, hook notices, and terminal status.

### R10 - Skills

Automated checks:

- `go test ./internal/skills/... ./internal/tools/skilltool/...`

Manual live validation:

1. Add a project skill with valid frontmatter.
2. Start TUI and list skills.
3. Modify the skill file.
4. Confirm reload notice appears.

Pass criteria:

- User and project skill directories are loaded.
- Invalid frontmatter is rejected with a clear diagnostic.
- Hot reload updates the available skill content.
- Skill prompt text does not bypass permission policy.

### R11 - Tasks

Automated checks:

- `go test ./internal/tasks/... ./internal/tools/tasktool/...`

Manual live validation:

1. Start a background task.
2. List tasks.
3. Fetch task output.
4. Stop task.

Pass criteria:

- Task runs without blocking the TUI.
- Status transitions are persisted.
- Output JSONL is readable.
- Stop cancels the active task and closes output.
- Session isolation prevents one session reading another session's task output.

### R12 - Concurrency And Speculative Execution

Automated checks:

- `go test ./internal/agent/... -run 'Test.*Concurrent|Test.*Speculative|Test.*Partition'`
- `go test -race ./internal/agent/...`

Pass criteria:

- Safe tools can run concurrently.
- Unsafe tools stay sequential.
- Batch progress is reported.
- Race tests are clean.
- Partition fuzz/property tests continue to pass when run.

### R13 - Observability, Trace, And Cost

Automated checks:

- `go test ./internal/observability/... ./internal/commands/...`

Manual smoke:

```text
/trace last
/cost
```

Pass criteria:

- Latest trace records terminal reason, done reason, retry count, first event/assistant latency, and stage latencies.
- `/trace last` includes slow-stage threshold and source.
- `/cost` shows session token totals and retry diagnostics.
- Prompt/file contents are not logged into metrics.

### R14 - Context, Prompt Packing, Checkpoint, Retrieval

Automated checks:

- `go test ./internal/agent/... ./internal/analysis/... ./internal/tui/...`

Required cases:

- `effectiveNumCtx` respects `auto|small|large|max`.
- Prompt packer keeps system anchor and latest user message.
- Prompt packer drops older messages first when over budget.
- `PromptPackReport` reaches TUI when trimming happens.
- Checkpoint save/load round-trips.
- `continue` with pending checkpoint creates a resume prompt.
- `continue` without pending checkpoint behaves as a normal user prompt.
- `/analyze-project` retrieval ranks likely relevant files and injects explicit mentions.
- Retrieval does not replace user-provided explicit mentions.

Pass criteria:

- Small prompts do not always request max context.
- Large prompts can scale context up without globally shrinking quality.
- Checkpoint resume does not trigger from stale/completed checkpoints once hardening is implemented.

## Load And Performance Tests

### L0 - Fast Unit Load

Command:

```sh
go test ./internal/agent/... ./internal/tui/... ./internal/analysis/... -count=20
```

Pass criteria:

- No flakes.
- No goroutine leaks reported by tests that assert channel closure.
- Runtime is stable across repeated runs.

### L1 - Race And Concurrent Tool Load

Command:

```sh
go test -race ./internal/agent/... ./internal/tasks/... ./internal/state/... ./internal/tui/...
```

Pass criteria:

- No data races.
- Tool batches do not corrupt event ordering or transcript state.

### L2 - Large Transcript Render Load

Recommended automated benchmark target:

```sh
go test ./internal/tui/... -bench 'Render|Transcript|AssistantDelta' -benchmem
```

Until benchmarks exist, add tests that build transcripts with:

- 1,000 assistant deltas.
- 10,000 assistant deltas.
- 100 interleaved tool events.
- 50 thinking blocks.
- 100 system notices.

Pass criteria:

- No panic.
- Render time growth is bounded enough for interactive TUI use.
- Markdown render cache invalidates only changed items.

### L3 - Directory Mention Expansion Load

Fixture:

- Create fake repo with 1,000 files across nested dirs.
- Include text, binary, too-large, hidden, and excluded paths.

Command target:

```sh
go test ./internal/mentions/... ./internal/tools/dirwalk/... -run 'Large|Directory|Budget'
```

Pass criteria:

- Directory walk respects max depth, file cap, byte cap, and default excludes.
- Expansion reports included/skipped/truncated counts.
- Prompt expansion stays under configured prompt byte/file caps.
- Binary and too-large files are skipped with reasons.

### L4 - Prompt Packing Load

Fixture:

- 1 system message.
- 300 alternating user/assistant messages.
- Several large tool-result messages.
- Latest user request with explicit file context.

Pass criteria:

- Packing completes quickly.
- Latest user message is always preserved.
- System anchor is preserved when present.
- Skipped/included estimates are reported.
- Packed prompt is deterministic for the same input.

### L5 - Checkpoint/Resume Load

Fixture:

- Repeatedly run fake terminal events with pending and completed checkpoints.
- Alternate `continue`, normal prompts, and terminal failures.

Pass criteria:

- Checkpoint file remains valid JSON.
- Completed checkpoints do not force resume after hardening lands.
- Pending checkpoint resume prompt includes original task and partial assistant output.
- Concurrent TUI event handling does not corrupt checkpoint writes.

### L6 - Retrieval Load

Fixture:

- 50,000 file-index entries.
- Queries for subsystem names: `tui`, `agent`, `memory`, `hooks`, `config`, `analysis`.

Pass criteria:

- Retrieval returns within an interactive budget.
- Ranking is deterministic.
- Explicit root scope is respected.
- Frecency boost can promote recently selected files.
- No directory entries are injected as file mentions unless explicitly intended.

### L7 - Live Ollama Small Prompt Latency

Scenario:

```text
User: reply with exactly "ok"
```

Collect:

- `/trace last`
- effective context mode
- first assistant latency
- terminal latency
- prompt/eval token counts

Pass criteria:

- Small prompt uses small/auto context tier, not max context by default.
- First visible response is fast enough for interactive use on the local machine.
- No memory recall LLM call occurs in default `fast` mode.

### L8 - Live Medium Prompt With Tools

Scenario:

```text
User: inspect README.md and docs/PHASE-LOG.md, then summarize current roadmap in 5 bullets.
```

Pass criteria:

- FileRead/File mention path works.
- Tool use events appear in TUI.
- Final answer includes requested summary.
- `/trace last` shows tool-start and terminal timing.

### L9 - Live Large Project Analysis Foundation

Scenario:

```text
/analyze-project . identify missing pre-phase-22 implementation work and cite the files used
```

Current pass criteria:

- Retrieval selection notice appears.
- Prompt expansion includes selected ranked files.
- Prompt packing report appears if history is trimmed.
- Latest checkpoint is written at terminal.
- If output is preamble-only, incomplete-response retry triggers.

Current expected limitation:

- This is not yet the full CL-6 map/reduce workflow. The test should record whether the final answer is complete, but failure to use cached chunk summaries is expected until CL-6 lands.

### L10 - Live Long Thinking / Stall Recovery Observation

Scenario:

```text
User: analyze the architecture and list risks, tradeoffs, and prioritized fixes.
```

Pass criteria now:

- Thinking deltas appear when the model supports thinking.
- Thinking can be collapsed/expanded.
- Watchdog timeout is reported as unrecoverable, not silent success.

Future pass criteria after CL-5 hardening:

- Productive long thinking is not cancelled prematurely.
- Stuck streams are diagnosed with clear terminal detail.

## Manual Gate G0 Validation

Before Phase 22, record `pass|fail|blocked` evidence in `docs/PHASE-LOG.md` for:

- Phase 8 memory two-session behavior.
- Phase 9 hooks live blocking behavior.
- Phase 10 MCP live tool call and process cleanup.
- Phase 11 sub-agent lifecycle/cancellation.
- Phase 12 skills discovery/hot reload.
- Phase 13 slash commands/config UX.
- Phase 14 task lifecycle/output/stop.

Use `docs/GATE-G0-PHASE-8-14-VALIDATION-PLAN.md` for the detailed procedure.

## Pre-Phase-22 Release Readiness Checklist

The app is ready to start Phase 22 when:

- Fast and full local regression gates pass.
- Gate G0 evidence is recorded or blockers are explicitly listed.
- CL-4 missing hook timings are implemented or explicitly deferred with risk.
- CL-5 checkpoint hardening is implemented or explicitly scoped.
- CL-6 has a minimum viable project-analysis workflow or a documented decision that Phase 22 can start before it.
- CL-7 retrieval priority tests prove explicit mentions outrank automatic retrieval.
- At least one small, one medium, and one large live Ollama scenario has `/trace last` evidence recorded.

## Regression Report Template

Use this format when recording a run:

```md
## Regression Run - YYYY-MM-DD

Environment:
- Commit:
- OS:
- Go version:
- Ollama version:
- Model:
- Config/data/cache/state dirs isolated: yes/no

Automated:
- Fast gate: pass/fail
- Full gate: pass/fail
- Race gate: pass/fail
- Policy scripts: pass/fail

Manual:
- R0 CLI/config: pass/fail
- R4 TUI core: pass/fail
- R6 memory: pass/fail
- R7 hooks: pass/fail
- R8 MCP: pass/fail
- R9 sub-agents: pass/fail
- R10 skills: pass/fail
- R11 tasks: pass/fail
- L7 small live prompt: pass/fail
- L8 medium tool prompt: pass/fail
- L9 large analysis foundation: pass/fail

Findings:
- Severity:
- Area:
- Repro:
- Expected:
- Actual:
- Logs/trace:
- Owner:
- Blocker for Phase 22: yes/no
```
