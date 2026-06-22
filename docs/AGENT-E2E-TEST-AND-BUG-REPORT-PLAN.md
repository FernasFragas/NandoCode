# Agent E2E Test And Bug Report Plan

Date: 2026-06-07
Status: Reviewed and strengthened for execution
Project: `nandocodego`
Purpose: provide a detailed, agent-executable end-to-end test plan for the currently implemented application surface, with strict evidence capture and detailed bug reporting.

## Goal

Run a full agent-driven validation pass over the application that:

- exercises all currently implemented functional areas end to end;
- captures reproducible evidence for every test scenario;
- produces structured bug reports for failures, gaps, regressions, and UX defects;
- separates implemented behavior from future-phase behavior so the test effort does not fail on features that are intentionally not shipped yet.

This plan is broader than the existing regression plan. The regression plan focuses on automated gates and load checks. This document defines the full agent-run validation program: automation, live/manual scenarios, evidence collection, defect triage, and final reporting.

## Source Of Truth

Before running this plan, the coordinator agent must read:

- `docs/PROJECT-STATUS-AND-ONBOARDING.md`
- `docs/NEXT-PHASES-IMPLEMENTATION-PLAN.md`
- `docs/PHASE-LOG.md`
- `docs/REGRESSION-AND-LOAD-TEST-PLAN.md`
- `docs/PERFORMANCE-FOLLOW-UP-MULTI-AGENT-PLAN.md`

If a test expectation conflicts with source code, the source code wins and the discrepancy must be logged as documentation drift.

## Accuracy Review Notes

This plan was reviewed against the current source on 2026-06-07. Agents must account for these current facts:

- The root CLI exposes `doctor`, `version`, `init`, `index`, and `server`; there is no `connect` command yet.
- Server routes currently exist under `/v1/health`, `/v1/models`, `/v1/sessions`, `/v1/sessions/{id}`, `/v1/sessions/{id}/events`, `/v1/sessions/{id}/messages`, `/v1/sessions/{id}/permissions/{request_id}`, `/v1/sessions/{id}/model`, and `/v1/sessions/{id}/tree`.
- `tools/run-regression-full.sh` intentionally excludes `internal/tools/webfetch` in sandbox-sensitive environments because local listener binding may be unavailable.
- `doctor` does not probe Ollama by default, but it currently loads MCP config and starts MCP status checks for configured servers. Treat this as current behavior to verify. If the release expectation is "no network probes by default", file a bug or documentation-drift report with exact evidence.
- `make test-e2e` points at `./e2e/...`; if that package does not exist in the current worktree, record that as `blocked` or `future test harness gap`, not as an application runtime failure.
- Installed cloud-backed models may already be usable in a developer environment. Negative credential scenarios must therefore be run in a credential-clean environment or with a dedicated auth-failure seam; do not mutate a user's real credential state just to force the negative path.

## Scope

This plan tests the functionality that is implemented now or explicitly expected to work now:

- CLI bootstrap, config loading, path setup, `doctor`, `version`, `init`, `--print`, and `index`.
- Ollama runtime behavior, model listing/show/pull, model limits, retries, watchdogs, and cloud credential flow where applicable.
- Agent loop behavior: streaming, retries, terminal states, compaction, context policy, output budgets, and tool execution.
- Permission behavior across modes and interactive approval flows.
- Built-in tool ecosystem and tool safety boundaries.
- TUI ask/response workflow, slash commands, prompt expansion, transcript rendering, queue/background behavior, checkpoint flow, semantic controls, and index progress.
- Memory, hooks, MCP, sub-agents, skills, tasks, coordinator mode, speculative execution, and observability.
- HTTP/SSE server mode, browser/API flow, session lifecycle, permission broker, and server-side prompt path.
- Semantic indexing and retrieval, including CLI, TUI, and server integration paths.
- Performance and latency evidence capture for real user flows.

This plan must not mark the run failed solely because these future-phase items are absent:

- Phase 25 remote/bridge mode
- Phase 17 release packaging
- Phase 18 final eval/release hardening

Those items should be recorded as `out_of_scope_future_phase` if encountered.

## Deliverables

The agent test program must produce all of the following artifacts:

1. A master execution report:
   - `docs/reports/E2E-MASTER-REPORT-YYYY-MM-DD.md`
2. A scenario matrix:
   - `docs/reports/E2E-SCENARIO-MATRIX-YYYY-MM-DD.md`
3. One report per agent lane:
   - `docs/reports/E2E-AGENT-<lane>-REPORT-YYYY-MM-DD.md`
4. One report per confirmed defect:
   - `docs/reports/bugs/BUG-YYYYMMDD-<slug>.md`
5. One report per blocked scenario:
   - `docs/reports/blocks/BLOCK-YYYYMMDD-<slug>.md`
6. A final summary for leadership triage:
   - `docs/reports/E2E-EXECUTIVE-SUMMARY-YYYY-MM-DD.md`

Screenshots, logs, and command outputs should be stored under:

- `docs/reports/artifacts/YYYY-MM-DD/<lane>/`

The coordinator must create these directories before execution:

```sh
mkdir -p docs/reports/bugs docs/reports/blocks docs/reports/artifacts/$(date +%F)
```

## Execution Principles

- Test from the outside first. Use CLI, TUI, and HTTP interfaces before reading implementation internals unless repro investigation requires code inspection.
- Record exact command lines, environment variables, model names, and working directories for every meaningful run.
- Distinguish product defects from environment defects.
- Distinguish product defects from test-harness defects. If a repo test script fails before it reaches the product surface, file a harness bug and continue with an equivalent direct product check when possible.
- Treat documentation drift as a real finding when it affects normal usage.
- Prefer deterministic repros. If a failure is flaky, state that explicitly and record attempt counts.
- Never treat a scenario as passed without evidence.
- When a scenario is not automatable, record manual evidence with timestamps and artifacts.
- Do not mark an end-to-end scenario `pass` based only on unit tests. Unit tests can support evidence, but each E2E scenario must include a user-facing interface check or a justified statement that no user-facing interface exists.
- Redact API keys, bearer tokens, absolute home-directory secrets, prompt bodies containing private code, and generated credentials from reports and artifacts.
- Preserve raw logs as artifacts only when they are needed for diagnosis. The markdown report should summarize the relevant lines and link the artifact.

## Test Quality Bar

Every scenario must meet this quality bar before it can be accepted:

- Preconditions are explicit and reproducible.
- The test uses isolated config/data/cache/state directories unless the scenario is explicitly about persisted user state.
- The command or interaction script is exact enough for another agent to rerun.
- The expected result is specific and externally observable.
- The actual result includes enough evidence to prove the status.
- A failed scenario has a linked bug or blocked report.
- Any live-model scenario records the exact model, provider, and whether the model was local or cloud.
- Any performance scenario records at least wall-clock duration and `/trace last` where available.
- Any permission/security scenario records the decision path without exposing sensitive file contents.

## Test Data Standards

Agents must use controlled fixtures instead of ad hoc files wherever possible.

Required sample workspaces:

- `tiny-go`: one Go module with one package, one test file, and one README.
- `medium-go`: several packages with imports, tests, markdown docs, and nested directories.
- `prompt-fidelity`: files designed to test listing, `@dir/`, `@dir?content`, explicit file references, and large-file truncation.
- `security-paths`: files and symlinks that test path traversal, denied writes, and allowed additional working directories.
- `semantic-sample`: code and docs with intentionally related symbols across directories for semantic retrieval checks.

Fixture rules:

- Keep fixtures under a temporary directory or `docs/reports/artifacts/YYYY-MM-DD/fixtures/`.
- Do not write generated fixtures into source directories unless the lane is testing source-tree behavior.
- Record fixture creation commands or include a fixture manifest in the lane report.
- If a scenario uses this repository itself as the target workspace, state that clearly because results may change as files change.

## Evidence Quality Bar

Use evidence levels:

- `E0`: command/test output only
- `E1`: output plus relevant artifact or screenshot
- `E2`: output plus trace/prompt metadata and artifact
- `E3`: independently rerun and reproduced by a second agent or second environment

Minimum evidence:

- `sev0_release_blocker` and `sev1_critical`: `E3` unless the defect is destructive or unsafe to rerun.
- `sev2_high`: `E2`
- `sev3_medium`: `E1`
- `sev4_low` and `sev5_docs_or_polish`: `E0`

Evidence must avoid content leaks. For prompt dumps, include metadata and short sanitized excerpts only unless the fixture was intentionally public test data.

## Environment Setup

Use isolated directories:

```sh
export NANDOCODEGO_CONFIG_HOME="$(mktemp -d)"
export NANDOCODEGO_DATA_HOME="$(mktemp -d)"
export NANDOCODEGO_CACHE_HOME="$(mktemp -d)"
export NANDOCODEGO_STATE_HOME="$(mktemp -d)"
export GOCACHE=/private/tmp/go-nandocodego-gocache
```

For live Ollama checks:

```sh
export OLLAMA_HOST=http://localhost:11434
export NANDOCODEGO_TEST_OLLAMA=1
```

For negative cloud-auth checks, prepare one of these before execution:

- a credential-clean environment with no working Ollama Cloud auth state; or
- a controllable test seam that can simulate missing or rejected cloud credentials without touching user credentials.

For interactive lanes, verify these execution prerequisites before assigning final lane ownership:

- PTY-capable terminal control for TUI scenarios
- loopback socket access for local server and SSE scenarios
- browser automation or a clearly documented manual browser path for embedded UI checks

Recommended environment record:

- OS and version
- CPU and memory
- `go version`
- `git rev-parse HEAD`
- installed Ollama version
- installed local model names
- whether Ollama Cloud credentials are present

## Agent Topology

Use one coordinator plus specialized execution agents. Every lane owns a disjoint report and a bounded slice of the test matrix.

### Coordinator

Owns:

- test kickoff
- environment record
- execution matrix
- cross-lane blocker triage
- lane checkpoint enforcement
- final merge of findings

Must not do deep functional testing until the other lanes are running.

### Lane A: CLI And Bootstrap

Owns:

- startup paths
- config precedence
- doctor/init/version
- `--print`
- `index` CLI

### Lane B: Runtime And Models

Owns:

- model list/show/pull
- limits and watchdog behavior
- local-vs-cloud credential handling
- retry and stream behavior

### Lane C: Core Agent Loop And Tools

Owns:

- streaming runs
- tool execution
- permission paths
- terminal reasons
- retry and compaction

### Lane D: TUI Workflow

Owns:

- interactive prompt flow
- slash commands
- transcript behavior
- queue, `/bg`, `/btw`
- index progress and semantic UX

### Lane E: Knowledge Systems

Owns:

- memory
- hooks
- skills
- MCP
- prompt fidelity

### Lane F: Tasks And Multi-Agent Coordination

Owns:

- tasks
- sub-agents
- fork
- coordinator mode
- mailbox/`SendMessage`
- dream lifecycle

### Lane G: Server And Browser/API

Owns:

- `nandocodego server`
- session lifecycle
- SSE stream
- permission broker
- model switch API
- tree API
- embedded UI/browser validation

### Lane H: Semantic Index And Retrieval

Owns:

- build/refresh/status/clear
- retrieval routing
- TUI and server semantic behavior
- semantic evidence in prompts and traces

### Lane I: Performance And Reliability

Owns:

- real latency evidence
- render responsiveness
- checkpoint/resume reliability
- incomplete-response recovery
- slow-stage observability

## Global Execution Order

1. Coordinator runs baseline environment setup and automation gates.
2. Lanes A-I execute in parallel.
3. Coordinator merges initial failures and identifies shared blockers.
4. Lanes rerun narrowed scenarios for confirmation.
5. Coordinator issues final disposition:
   - `pass`
   - `pass_with_known_non_blockers`
   - `blocked`

## Agent Checkpoint Protocol

The coordinator must require each lane agent to checkpoint early and predictably. This prevents a lane from running for a long time and leaving only a vague `in_progress` artifact.

Required checkpoints:

- `T+10 minutes`: environment, prerequisites, and first executable scenario status
- `T+30 minutes`: initial scenario table with at least one `pass`, `fail`, or `blocked` row, or a block report explaining why no scenario can run
- every `30 minutes` after that: updated lane report, new bugs/blocks, and remaining scenario count
- before shutdown or handoff: final lane report with all completed and incomplete scenarios clearly marked

Stale-lane rules:

- If a lane misses two consecutive checkpoints, the coordinator must mark the lane `stale_checkpoint_missing` in the master report.
- If a lane cannot produce a report artifact, the coordinator must file a block report for that lane rather than leaving the lane as plain `in_progress`.
- If a lane is still incomplete at final aggregation, the lane report must use scenario statuses `not_tested`, `blocked`, or `partial`, and the master report must explain the confidence impact.
- The coordinator may continue high-priority direct validation for the affected lane, but must not present that lane as fully completed unless every scenario has a final status and evidence.

Checkpoint report minimum:

- current scenario counts by `pass`, `fail`, `blocked`, `not_tested`, and `partial`
- exact blockers preventing progress
- files written so far
- commands currently running or intentionally skipped
- next three scenarios to execute

## Phase 0: Baseline Automation

The coordinator must run the repo-supported gates first:

```sh
make regression-fast
make regression-full
make load-suite
```

The coordinator should also run the raw commands below when the environment permits:

```sh
go test ./...
go test -race ./internal/agent/... ./internal/hooks/... ./internal/tasks/... ./internal/tui/... ./internal/state/...
go vet ./...
tools/check-allowed-deps.sh
tools/check-network-policy.sh
```

Expected output:

- pass, or exact failures with package and repro command

Current sandbox note:

- If raw `go test ./...` fails only in `internal/tools/webfetch` with local listener binding errors, compare against `tools/run-regression-full.sh`, which intentionally excludes that package and reports the skip.
- If server, SSE, local-model, or semantic-index commands fail only because loopback or Unix sockets are blocked by the harness, rerun with an environment that allows local listeners before filing a product bug.

If a command is too heavy for the environment, record:

- command
- reason not completed
- whether it is a test-environment limit or a product defect

## Scenario Matrix

Each lane must execute all scenarios in its slice. Every scenario must end in one of:

- `pass`
- `fail`
- `blocked`
- `out_of_scope_future_phase`
- `not_tested`
- `partial`

Each scenario must include:

- scenario ID
- title
- interface under test
- preconditions
- commands or interaction script
- expected result
- actual result
- artifacts
- bug/block report link if not pass
- automation level: `automated`, `manual`, or `hybrid`
- evidence level: `E0`, `E1`, `E2`, or `E3`
- priority: `p0`, `p1`, `p2`, or `p3`

The final master report may include `not_tested` or `partial` only when the full program is explicitly marked `blocked`. A `ready_for_next_phase` recommendation is not allowed while any `p0` or `p1` scenario remains `not_tested`, `partial`, or stale.

Priority definitions:

- `p0`: must pass before release or before building later phases on this surface
- `p1`: important release confidence scenario
- `p2`: broad regression coverage
- `p3`: useful exploratory or polish coverage

## Scenario Record Template

Every scenario entry in the scenario matrix must use this structure so results are comparable across agents:

```md
## <SCENARIO-ID> <Title>

- Lane:
- Owner agent:
- Priority: `p0|p1|p2|p3`
- Automation level: `automated|manual|hybrid`
- Evidence level achieved: `E0|E1|E2|E3`
- Status: `pass|fail|blocked|out_of_scope_future_phase|not_tested|partial`
- Attempts:
- Start time:
- End time:

### Preconditions

- Isolated config/data/cache/state paths:
- Fixture or workspace:
- Model/provider requirements:
- Server/TUI/browser requirements:

### Steps

1. Exact command or interaction.
2. Exact command or interaction.
3. Exact command or interaction.

### Expected Result

Specific externally observable result.

### Actual Result

Specific observed result.

### Evidence

- Artifact path:
- Command output excerpt:
- Screenshot or trace path:
- Sanitization notes:

### Defects Or Blocks

- Bug:
- Block:

### Retest Notes

What should be rerun after a fix or environment change.
```

## Reproducibility And Independence Rules

- A second agent must be able to rerun any `p0` or `p1` scenario from the scenario record alone.
- A scenario that depends on another scenario must name that dependency and explain why it cannot be isolated.
- A scenario must clean up after itself unless the scenario is explicitly testing persisted state.
- A lane must not reuse a dirty fixture after a failed scenario unless the dirty state is part of the repro.
- When behavior differs between local model, cloud model, TUI, server, or `--print`, record the split instead of averaging the result.
- Timing measurements must include at least three runs for performance claims unless the first run already proves a severe regression or hang.
- Manual checks must record terminal size, browser viewport, keystrokes or clicks, and timestamps.

## Detailed Lane Plans

### Lane A: CLI And Bootstrap

Scenarios:

1. `A-001` `version` prints build metadata.
2. `A-002` `doctor` verifies local environment and records current MCP-status behavior.
3. `A-003` `doctor` reports path roots from overridden env vars.
4. `A-004` `init` creates default config once and handles rerun cleanly.
5. `A-005` `--help` and root help text are accurate.
6. `A-006` `--print` runs without TUI.
7. `A-007` `--print --json` returns machine-readable output.
8. `A-008` config precedence works across defaults, file, and flags.
9. `A-009` invalid config behavior is explicit and actionable.
10. `A-010` `index build`, `refresh`, `status`, and `clear` work on a sample workspace.

Evidence:

- command transcript
- resulting files in config/state/cache dirs
- any generated config

Minimum quality:

- `A-002` must record whether `doctor` starts MCP connectivity checks when MCP servers are configured.
- `A-006` and `A-007` must prove no Bubble Tea alternate-screen interaction is required.
- `A-009` must distinguish between a true runtime defect and documentation drift. If malformed config is warning-only by current contract, the scenario fails only if the warning is unclear or unsafe.
- `A-010` must inspect semantic index files under cache/state paths, not just command text.

### Lane B: Runtime And Models

Scenarios:

1. `B-001` model list succeeds with Ollama available.
2. `B-002` model list failure is clear when Ollama is unavailable.
3. `B-003` `ShowModel`-derived limits affect runtime state.
4. `B-004` small prompt on local model streams successfully.
5. `B-005` long prompt triggers retry or length handling as expected.
6. `B-006` watchdog timeout is surfaced clearly.
7. `B-007` cloud-only model selection requests credentials before sending context.
8. `B-008` `--print` with unavailable credential fails non-interactively and clearly.
9. `B-009` server-mode credential requirement is explicit and non-blocking.
10. `B-010` model switch back to local model clears cloud-only dependency path.
11. `B-011` installed cloud-backed model behavior is distinguished from missing-credential behavior.

Evidence:

- exact model names
- prompt used
- latency timing if relevant
- stderr/stdout summaries

Minimum quality:

- Any cloud scenario must prove no project context is sent before credential acceptance.
- Any unavailable-Ollama scenario must record exit code and error text.
- Any model-limit scenario must record the reported context length and effective runtime context when available.
- Any cloud-negative scenario must state whether the environment was credential-clean. If cloud access is already working, record the scenario as blocked or use the dedicated negative-path environment instead of inferring behavior.
- Any server cloud-model scenario must compare the advertised model catalog with the server's actual model-selection behavior.

### Lane C: Core Agent Loop And Tools

Scenarios:

1. `C-001` plain chat prompt with no tools.
2. `C-002` tool call execution with successful result.
3. `C-003` unknown tool handling.
4. `C-004` tool parse error handling.
5. `C-005` permission allow path.
6. `C-006` permission deny path.
7. `C-007` interactive permission path through TUI or server.
8. `C-008` incomplete-response retry.
9. `C-009` compaction trigger and recovery.
10. `C-010` max-turn safety.
11. `C-011` tool concurrency safe-path behavior.
12. `C-012` `ToolModeNone` path for cheap prompts.
13. `C-013` prompt evidence pack generation for explicit file context.

Tools to exercise:

- `Bash`
- `FileRead`
- `FileWrite`
- `FileEdit`
- `Glob`
- `Grep`
- `Todo`
- `Task`
- `Skill`
- `SendMessage`
- `WebFetch` where environment permits

Evidence:

- prompt
- trace
- tool results
- permission behavior

Minimum quality:

- Tool scenarios must include both success and denied/error paths.
- Permission scenarios must include mode, source-tagged rule state, target, decision, and final tool result.
- `C-012` must verify that cheap prompts omit tool schemas in the model request shape, not merely that routing returned `ToolModeNone`.

### Lane D: TUI Workflow

Scenarios:

1. `D-001` initial render and empty state.
2. `D-002` normal prompt submission and stream.
3. `D-003` thinking display behavior.
4. `D-004` transcript stability during long stream.
5. `D-005` `/help`
6. `D-006` `/model`
7. `D-007` `/models`
8. `D-008` `/memory` commands
9. `D-009` `/hooks` commands
10. `D-010` `/permissions` commands
11. `D-011` `/skills` commands
12. `D-012` `/cost`
13. `D-013` `/trace last`
14. `D-014` `/prompt last`
15. `D-015` `/queue list|clear|drop`
16. `D-016` `/bg`
17. `D-017` `/btw`
18. `D-018` `/analyze-project`
19. `D-019` `/index build|refresh|status|clear`
20. `D-020` permission modal keyboard flow
21. `D-021` file mention completion and picker flow
22. `D-022` resize behavior and viewport stability

Evidence:

- screenshots or screen recording clips
- transcript snapshots
- prompt/trace output

Minimum quality:

- TUI screenshots must include terminal size.
- Long-stream checks must record whether render latency grows over time.
- Permission modal checks must verify keyboard focus, `Esc` handling, and that inactive UI layers do not consume modal keys.
- `/btw` checks must verify read-only behavior and whether the result mutates main conversation history.

### Lane E: Knowledge Systems

Scenarios:

1. `E-001` memory list/show/edit/promote flow.
2. `E-002` memory recall mode switching.
3. `E-003` fast recall versus llm recall behavior.
4. `E-004` hooks list and reload flow.
5. `E-005` command hook execution.
6. `E-006` prompt hook execution.
7. `E-007` disabled project/HTTP/agent hook diagnostics.
8. `E-008` skills list/show.
9. `E-009` skill hot reload or watcher behavior if enabled.
10. `E-010` MCP server config load and tool exposure.
11. `E-011` MCP live tool call where environment permits.
12. `E-012` prompt fidelity for listing prompts:
    - `@docs/`
    - `@docs?content`
    - `review @docs/`
    - `summarize @docs/`

Evidence:

- memory files
- hook output
- MCP status
- prompt dumps showing fidelity behavior

Minimum quality:

- Prompt-fidelity checks must compare `/prompt last` or equivalent prompt metadata against the user-visible request.
- Hook checks must record enabled/disabled/trusted status and whether a hook was skipped, executed, or blocked.
- MCP live tests must clearly separate config parsing from live process/connectivity behavior.

### Lane F: Tasks And Multi-Agent Coordination

Scenarios:

1. `F-001` task create/list/status path.
2. `F-002` task stop/cleanup.
3. `F-003` sub-agent spawn and result return.
4. `F-004` fork behavior and isolation.
5. `F-005` coordinator mode spawn of multiple workers.
6. `F-006` worker restriction enforcement.
7. `F-007` `SendMessage` mailbox path.
8. `F-008` auto-resume behavior on completed worker.
9. `F-009` dream lifecycle start and kill on new prompt.
10. `F-010` task output replay and transcript visibility.

Evidence:

- task IDs
- worker IDs
- message routing proof
- transcript and state snapshots

Minimum quality:

- Multi-agent scenarios must record task IDs, worker names, message IDs, and terminal states.
- Worker-restriction checks must prove restricted workers cannot access coordinator-only tools.
- Dream lifecycle checks must prove the previous dream is killed when a new user prompt starts.

### Lane G: Server And Browser/API

Scenarios:

1. `G-001` `nandocodego server` startup on loopback.
2. `G-002` auth-required behavior on non-loopback bind.
3. `G-003` health endpoint.
4. `G-004` models endpoint.
5. `G-005` create session.
6. `G-006` connect SSE and receive live events.
7. `G-007` post message and stream response.
8. `G-008` permission request/resolve round trip.
9. `G-009` update session model.
10. `G-010` tree endpoint and path safety.
11. `G-011` session deletion cancels run.
12. `G-012` replay via `Last-Event-ID`.
13. `G-013` browser embedded UI prompt flow.
14. `G-014` browser permission flow.
15. `G-015` browser reconnect behavior.
16. `G-016` repo smoke harness reaches the first product request on the default supported shell.

Evidence:

- `curl` scripts
- SSE event excerpts
- browser screenshots
- request/response summaries

Minimum quality:

- HTTP checks must record method, path, status code, and sanitized response body.
- SSE checks must prove events stream incrementally, not only after terminal.
- Auth checks must include loopback and non-loopback bind expectations.
- Replay checks must include the `Last-Event-ID` used and the IDs received after reconnect.
- Browser checks should use Playwright or another repeatable browser automation path when available; otherwise record manual steps and screenshots.
- `G-004` and `G-009` together must verify catalog-to-selection consistency: any model listed by `/v1/models` must either be selectable or fail with a precise credential/access reason, not a generic `model not found`.
- If `tools/smoke-server.sh` or another official harness fails before the first product request, file a harness bug and continue with equivalent direct `curl` or browser validation.

### Lane H: Semantic Index And Retrieval

Scenarios:

1. `H-001` semantic index build on medium sample repo.
2. `H-002` semantic index refresh on changed repo.
3. `H-003` semantic index status.
4. `H-004` semantic index clear.
5. `H-005` general prompt skip path.
6. `H-006` explicit-file prompt skip path.
7. `H-007` related-context prompt light retrieval path.
8. `H-008` broad workspace discovery full retrieval path.
9. `H-009` server prompt path semantic events.
10. `H-010` TUI progress visibility during index build.
11. `H-011` retrieval behavior with stale/missing index.
12. `H-012` retrieval dimensions mismatch handling.
13. `H-013` retrieval cache warm-vs-cold evidence.

Evidence:

- route decision details
- trace output
- semantic event summaries
- prompt excerpts without leaking private content

Minimum quality:

- Route checks must record `route_action`, `route_reason`, `route_profile`, `tool_mode`, and whether embedding was allowed.
- Retrieval checks must include cold and warm cache evidence where the scenario touches repeated retrieval.
- Index progress checks must prove progress appears during work and does not spam the transcript per file.

### Lane I: Performance And Reliability

Scenarios:

1. `I-001` small prompt latency evidence.
2. `I-002` medium explicit-file prompt latency evidence.
3. `I-003` broad semantic prompt latency evidence.
4. `I-004` `/analyze-project` latency and final-answer completeness.
5. `I-005` checkpoint resume after interrupted analysis.
6. `I-006` incomplete-response recovery.
7. `I-007` TUI long-transcript responsiveness.
8. `I-008` repeated semantic retrieval warm-cache benefit.
9. `I-009` hook-induced slow-stage diagnosis.
10. `I-010` context mode differences: `auto`, `small`, `large`, `max`.

Evidence:

- `/trace last`
- prompt metadata
- first visible output time
- terminal latency
- semantic timings
- whether the promised artifact was actually delivered

Minimum quality:

- Latency checks must record first visible output time, first assistant event, terminal latency, and slowest stages.
- Final-answer completeness checks must define the promised artifact before the run and verify the final answer against that promise.
- Context-mode checks must record effective context and any explicit user override.

## Bug Finding Rules

Create a bug report when any of the following are true:

- implemented functionality does not match expected behavior;
- error text is missing, misleading, or unsafe;
- the application hangs, crashes, leaks state, or leaves corrupted files;
- permission boundaries can be bypassed;
- prompt fidelity silently drops critical explicit context;
- server sessions leak or share state;
- a server/API endpoint advertises a selectable resource that another official endpoint rejects without a precise reason;
- the UI misrepresents what the app is doing;
- a feature works only under undocumented conditions;
- docs materially mislead an engineer or user.

## Severity Model

Use these severities:

- `sev0_release_blocker`: data loss, security boundary bypass, auth bypass, persistent hang/crash in core flows, wrong-file writes, or impossible-to-use release-critical flow
- `sev1_critical`: major implemented feature broken, severe permission or server-session bug, deterministic crash in common flow, or cloud credential leak risk
- `sev2_high`: important workflow broken with workaround, serious prompt-fidelity issue, major performance regression, or server/TUI state inconsistency
- `sev3_medium`: feature works partially, confusing UX with clear workaround, non-critical docs drift, or limited-scope regression
- `sev4_low`: minor issue with obvious workaround
- `sev5_docs_or_polish`: wording, reporting, or cosmetic issue that does not affect behavior

Use these dispositions:

- `confirmed`
- `likely`
- `needs_more_evidence`
- `environmental`
- `documentation_drift`
- `future_phase_gap`

## Bug Report Template

Every confirmed or likely defect must use this structure:

```md
# BUG-YYYYMMDD-<slug>

## Summary

One-paragraph description of the defect.

## Severity

- Severity: `sevX_*`
- Disposition: `confirmed|likely|...`
- Area: `cli|llm|agent|tui|server|semantic|...`

## Environment

- Commit:
- OS:
- Go version:
- Ollama version:
- Model:
- Relevant env vars:

## Preconditions

- State directory setup
- Config setup
- Model availability
- Sample workspace

## Reproduction Steps

1. Exact step.
2. Exact step.
3. Exact step.

## Expected Result

What should happen.

## Actual Result

What actually happened.

## Evidence

- command output summary
- screenshots
- traces
- prompt dump reference
- log reference
- artifact paths
- sanitization notes

## Frequency

- always
- intermittent
- once
- attempt count:

## Evidence Level

- `E0|E1|E2|E3`

## Impacted Scenarios

- Scenario IDs affected by this bug.

## Regression Risk

What nearby systems may also be affected.

## Suspected Root Cause

Optional, evidence-based only.

## Recommended Fix Direction

Short engineering direction, not a full implementation plan.

## Related Files

- relevant code paths
- relevant docs

## Retest Plan

Exact steps to verify the fix.

## Closure Criteria

What must be true before this bug can be considered closed.
```

## Blocked Scenario Template

Use this when a scenario cannot be completed due to environment, missing dependency, or future-phase boundary:

```md
# BLOCK-YYYYMMDD-<slug>

## Scenario

Scenario ID and title.

## Blocking Condition

Exact condition preventing execution.

## Category

- environment
- sandbox
- missing model
- missing credential
- future phase
- dependency outage
- stale agent
- missing harness capability

## Evidence

- commands attempted
- output summary
- screenshots or logs

## What Was Still Verified

Any partial validation completed before the block.

## Next Step Needed

What must change to unblock this scenario.

## Owner

Who can unblock it: test coordinator, environment owner, product owner, or implementation agent.
```

## Retest And Closure Rules

- Every `sev0_release_blocker`, `sev1_critical`, and `sev2_high` fix must rerun the original failing scenario plus at least one adjacent regression scenario.
- Every permission, auth, path-safety, prompt-fidelity, or state-sharing fix must rerun both the positive and negative path.
- A bug cannot be closed from code inspection alone. Closure requires command output, trace, screenshot, or automated test evidence.
- If a fix changes expected behavior, update the scenario expectation and link the source-of-truth documentation or code change that justifies the new expectation.
- If a defect is downgraded, the master report must explain the downgrade with evidence.
- A blocked scenario can be closed only when it is rerun to a final `pass`, `fail`, or `out_of_scope_future_phase` state.

## Lane Report Template

Each lane report must include these sections:

```md
# E2E Agent <Lane> Report - YYYY-MM-DD

## Scope

- Lane:
- Owner agent:
- Functional areas:
- Source commit:
- Start/end time:

## Environment

- OS:
- Go version:
- Ollama status:
- Model/provider:
- Isolated config/data/cache/state paths:
- Browser/terminal details where relevant:

## Scenario Results

| Scenario | Priority | Automation | Evidence | Status | Attempts | Bug/Block | Notes |
| --- | --- | --- | --- | --- | --- | --- | --- |

## Coverage Notes

- Functional paths covered:
- Positive paths covered:
- Negative/error paths covered:
- Performance or reliability evidence captured:
- Known coverage gaps:

## Checkpoint History

| Time | Completed | Failed | Blocked | Not Tested | Partial | Notes |
| --- | --- | --- | --- | --- | --- | --- |

## Findings

| Finding | Severity | Disposition | Impacted Scenarios | Evidence |
| --- | --- | --- | --- | --- |

## Bugs

- Linked bug reports with one-line impact.

## Blocks

- Linked block reports with required unblock action.

## Risk Assessment

- Top user-facing risks:
- Top release risks:
- Top test-confidence risks:

## Rerun Recommendation

- Scenarios to rerun immediately:
- Scenarios to rerun after fixes:
- Scenarios that need a different environment:

## Lane Recommendation

- `pass`
- `pass_with_known_non_blockers`
- `blocked`
```

Lane reports must not only list counts. They must explain what was actually proven and what remains unproven.

## Master Report Template

The master report must include these sections:

```md
# E2E Master Report - YYYY-MM-DD

## Executive Summary

- Final recommendation:
- Commit tested:
- Overall status:
- Highest severity found:
- Release-impact summary:

## Environment Summary

- OS and machine:
- Go version:
- Ollama version and models:
- Browser automation availability:
- Isolated state paths:
- Known environment limitations:

## Coverage Summary

| Lane | Passed | Failed | Blocked | Future Phase | Evidence Quality | Confidence |
| --- | --- | --- | --- | --- | --- | --- |

## Scenario Matrix Summary

- Total scenarios:
- `p0` status:
- `p1` status:
- `p2` status:
- `p3` status:

## Release Blockers

| Bug | Severity | Area | Impact | Evidence | Required Action |
| --- | --- | --- | --- | --- | --- |

## High And Medium Risks

| Finding | Severity | Area | User Impact | Workaround | Recommendation |
| --- | --- | --- | --- | --- | --- |

## Blocked Coverage

| Scenario | Category | Reason | Risk | Required Unblock |
| --- | --- | --- | --- | --- |

## Documentation Drift

| Finding | Source Of Truth | Current Docs | Impact |
| --- | --- | --- | --- |

## Performance And Reliability

- First visible output findings:
- Terminal latency findings:
- Slow-stage findings:
- Warm-cache/cold-cache findings:
- Reliability or resume findings:

## Security And Permission Findings

- Permission boundary results:
- Path-safety results:
- Auth and server-session results:
- Credential-safety results:

## Test Confidence Assessment

- Strongest evidence:
- Weakest evidence:
- Scenarios requiring second-agent reproduction:
- Remaining blind spots:

## Final Recommendation

- `ready_for_next_phase`
- `ready_with_known_risks`
- `blocked`

Rationale with links to lane reports and bugs.
```

The executive summary must be shorter than the master report and should contain only the final recommendation, critical risks, blocker count, and next actions.

## Exit Criteria

The full program is complete only when:

- every scenario in the matrix has a final status;
- every non-pass scenario has a linked bug or block report;
- evidence artifacts exist for all high-risk flows;
- the master report and executive summary are written;
- open defects are triaged by severity and disposition;
- the final recommendation is explicit and supported by evidence.
- every `p0` scenario has at least `E2` evidence, unless blocked by environment with a linked block report;
- every `sev0_release_blocker` or `sev1_critical` has either a fix with retest evidence or an explicit `blocked` final recommendation;
- every server/auth/permission/path-safety scenario includes a negative-path check;
- every performance claim is backed by measurements, not only subjective observation;
- every documentation-drift finding cites the conflicting source and expected correction.

The program may recommend `ready_for_next_phase` only when:

- all `p0` scenarios pass;
- no `sev0_release_blocker` or `sev1_critical` remains open;
- blocked scenarios do not hide a release-critical surface;
- the coordinator can explain the remaining risk in concrete user-impact terms.

## Notes For Agents

- Do not rewrite this plan while executing it unless the coordinator explicitly decides the plan is wrong.
- Keep bug reports factual and compact. Save long logs as artifacts and summarize them in the markdown.
- Do not conflate missing future-phase functionality with regressions.
- When uncertain whether a failure is real, rerun the scenario at least once before filing a `confirmed` bug.
- Do not mark a scenario `pass` because a related lower-level unit test passed.
- Do not hide missing evidence behind phrases like "appears to work"; include the observable proof.
- Do not file one large bug for unrelated failures. Split by root cause or user-visible defect.
- Prefer smaller, exact artifacts over broad raw logs that contain irrelevant or sensitive data.
