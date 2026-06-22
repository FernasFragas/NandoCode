# Incomplete Response Recovery Report

Date: 2026-05-16

## Purpose

This report captures the runtime issue where the TUI shows that the assistant is about to provide a summary, but the final summary never appears. It also records the patch that was added, the exact interaction context that motivated the fix, and the remaining tasks needed to validate and harden the ask-and-response flow.

The goal is to make this report self-contained for a follow-up agent. A future agent should not need the chat transcript to understand the bug, reproduce it, or continue the work.

## Review Outcome For Large-Project Analysis

The desired product behavior is not just "respond sooner." The application must support reliable deep analysis over large projects with many files and documents, while still avoiding runs that appear to hang or end after a preamble.

The current patch is useful because it catches one concrete failure mode: a completed model turn whose visible answer is only a promise. However, the broader plan must be careful not to solve the symptom by reducing context, interrupting useful reasoning too early, or forcing shallow final answers.

The follow-up work should preserve these constraints:

- Keep enough context capacity for repository-scale work. Do not treat lower `num_ctx` as a default fix for latency or incomplete output.
- Separate "productive long analysis" from "unproductive stall." A long run is acceptable when it is reading files, summarizing evidence, making tool calls, or producing observable checkpoints.
- Require an evidence trail for large-project conclusions. Final reports should be grounded in files read, phase docs checked, and unresolved uncertainty.
- Prefer resumable analysis state over a single huge prompt. Large projects need checkpoints, rolling summaries, and a synthesis stage.
- Make progress visible in the TUI. Users should see whether the agent is reading, synthesizing, waiting on the model, retrying, compacting, or stalled.
- Make token accounting understandable. The status bar must distinguish current prompt/context tokens from cumulative session tokens.

The main weakness in the current report is that `Task A11` originally described "thinking character count exceeds a threshold" as a possible recovery signal. That is dangerous for this product goal. Thinking length alone is not a reliable stall signal for deep analysis. A thinking-heavy model can legitimately spend time planning a complex synthesis. Stall recovery should depend on lack of useful progress, not on reasoning volume alone.

## User-Visible Symptom

The user asked the application to analyze project files and generate a missing-implementation summary. The assistant streamed a preamble that promised the final answer, then stopped without delivering the promised content.

Observed first interaction:

```text
Assistant:

  Based on my thorough analysis of the project structure, implementation status, and all detailed phase plans in docs/, here is the
  comprehensive missing-implementation summary:

You:
continue

Thinking (2482 chars) Ctrl+T to expand

Assistant:

  Now I have the full picture. Let me write the missing-implementation summary:
```

The later detailed interaction showed the same failure shape with visible thinking expanded:

```text
Assistant:

  Now I have the full picture. Let me write the missing-implementation summary:

You:
continue

Thinking (4647 chars) Ctrl+T to collapse
| The user wants me to create a `.md` file summarizing what is missing to be implemented in the `nandocodego` project, based on the analysis
|
| I have already read:
| - `docs/PROJECT-STATUS-AND-ONBOARDING.md` (Executive summary, Current Phase Status, What Is Missing)
| - `docs/PHASE-LOG.md` (Detailed history of phases 0-9, post-phase-16 improvements, phases 27, etc.)
| - `docs/PHASE-10-DETAILED-PLAN.md` through `PHASE-27-DETAILED-PLAN.md` (Detailed plans and statuses)
| - Project file structure (331 files, covering CLI, LLM, Tools, Agent, Permissions, State, Bootstrap, TUI, Memory, Hooks, Config, Commands,
|
| Current implementation status (based on logs and file existence):
| - Phases 0-7: Fully implemented.
| - Phase 8 (Memory): Core implementation landed, exit-gate pending (manual two-session validation, etc.).
| - Phase 9 (Hooks): Core implementation landed, exit-gate pending.
| - Phase 10 (MCP): Implemented in code and automated checks; manual live exit-gate pending.
| - Phase 11 (Sub-agents and fork): Complete in code/automated checks; live exit-gate pending.
| - Phase 12 (Skills): Implemented in code/automated checks; manual live REPL exit-gate pending.
| - Phase 13 (Slash commands and config UX): Implemented in code/automated checks; live exit-gate pending.
| - Phase 14 (Tasks): Implemented in code/automated checks; manual live exit-gate pending.
| - Phase 15 (Concurrency): 100% Complete / Core Concurrency Infrastructure Delivered.
| - Phase 16 (Observability/Metrics): Implemented in code (2026-05-08 reconciliation).
| - Phase 17 (Distribution): Pre-implementation plan. (Not started).
| - Phase 18 (Hardening/Evals/Docs): Pre-implementation plan. (Not started).
| - Phase 19 (Content Compaction): Complete (hook dispatch deferred).
| - Phase 20 / 21 / 22 / 23 / 24 / 25 / 26: mixed planning/checklist status depending on document.
| - Phase 27: Completed (Directory Mention Expansion).
|
| Planned output:
| - Write `/Users/fernando/Desktop/to_sync/ai_projects_etc/go-nandocode-llm/docs/MISSING-IMPLEMENTATION-SUMMARY.md`.
```

Important detail: the thinking block contains enough information to produce the answer, but the visible assistant message still stops after a preamble. This means the problem is not only missing context. The model had relevant context in its hidden/visible thinking stream, but the agent accepted an incomplete final answer as a completed turn.

## Additional Screenshot Evidence

The user later provided a screenshot of the same interaction after expanding the detailed thinking block. The screenshot adds two important observations:

1. The status bar still shows:

```text
Model: qwen3.6:35b | Mode: insert | [Running...] | tokens: 2996719
```

2. The expanded thinking block contains a full plan and ends with intent statements like:

```text
I will format this cleanly as a summary document.
Let's generate the file.
I'll make sure to accurately reflect the phase numbers and names from the project docs.
I will output the markdown content directly.
```

This means there are likely two related failure modes:

- completed-too-early: the model emits a preamble, returns `done_reason=stop`, and the agent previously accepted the turn as completed;
- thinking-only or long-running stall: the run remains active, the model continues or stalls in thinking/planning, and no final assistant answer or tool call appears.

The patch described below addresses the first failure mode. The second failure mode needs additional instrumentation and progress-aware timeout controls. The app should not cancel a run just because thinking is long. It should act only when the stream is idle, the run is making no observable progress, or the model repeatedly plans without producing a tool call, checkpoint, or final answer.

The very large token value in the screenshot should also be investigated; it may represent accumulated tokens across the session, duplicated accounting, or an incorrect unit/source being displayed as tokens.

## Root Cause Analysis

### Confirmed Agent Loop Behavior

The agent loop previously ended a run whenever a model turn produced no tool calls:

1. `internal/agent/stream.go` accumulates assistant text, thinking text, tool calls, and final `DoneReason`.
2. `internal/agent/agent.go` handles `DoneReason == "length"` with retry and compaction logic.
3. For any other done reason, including `stop`, the assistant message is appended to history.
4. If `len(result.ToolCallMessages) == 0`, the run emits `TerminalCompleted`.

That behavior is structurally correct for normal answers, but it has a blind spot: it cannot distinguish a real final answer from a preamble-only answer such as:

```text
Here is the summary:
```

or:

```text
Now I have the full picture. Let me write the missing-implementation summary:
```

If the model reports `done_reason=stop`, the app treats the run as successful even though the assistant did not deliver the artifact.

### Why Reducing `num_ctx` Would Not Fix This Specific Failure

This failure is not primarily caused by the model lacking project context. In the second interaction, the thinking text shows the model had already collected and organized the relevant project status.

Reducing `num_ctx` would likely make project-scale analysis worse, because fewer file contents and prior messages can fit in the prompt. It may reduce latency, but it increases the chance that the model loses important file context. For this specific bug, the missing final answer happened after the model had enough context to draft the summary.

The more relevant controls are:

- output completion budget (`num_predict` / `MaxOutputTokens`);
- stop reason handling;
- semantic detection of preamble-only completions;
- TUI visibility for terminal and retry events;
- instrumentation showing final `DoneReason`, prompt tokens, output tokens, and assistant content length.

## Patch Added

The patch added a semantic retry guard for preamble-only assistant responses.

### Implementation Progress

Implemented on 2026-05-16:

- retry notices now carry structured diagnostics: retry kind, done reason, assistant character count, and thinking character count;
- terminal usage now preserves the last model `DoneReason`;
- `watchdog_timeout` and `stream_error:*` done reasons now terminate visibly as unrecoverable runs instead of being accepted as successful empty completions;
- observability records retry counts, retry kind counts, last retry details, and last done reason;
- `/cost` includes last done reason and retry diagnostics;
- the TUI status bar labels cumulative meter tokens as `session tokens`;
- incomplete-response fixture coverage now includes additional preamble variants and normal short-answer counterexamples;
- TUI tests cover the retry transcript shape and final report visibility;
- a manual smoke test exists at `docs/manual-tests/INCOMPLETE-RESPONSE-RECOVERY.md`.

### Files Changed

- `internal/agent/incomplete_response.go`
- `internal/agent/incomplete_response_test.go`
- `internal/agent/agent.go`
- `internal/agent/agent_test.go`
- `internal/tui/app.go`
- `internal/tui/app_test.go`

### Behavior After Patch

When a model turn:

- has no tool calls;
- ends with empty `DoneReason` or `DoneReason == "stop"`;
- produces only a short preamble/promise;
- matches cues such as `here is`, `let me write`, `now I have`, `summary:`, `report:`, or `analysis:`;

the agent appends a synthetic follow-up instruction to history:

```text
You stopped after a preamble. Provide the promised answer now. Do not say you will write it; write it.
```

It then retries once in the same user-visible turn and emits:

```text
[Retry 1] assistant response looked incomplete, requesting the promised answer
```

The retry is capped at one attempt to avoid runaway loops.

The TUI also now renders non-completed terminal exits such as:

```text
[Run ended: max_turns: exceeded maximum turn count]
```

That makes future `max_turns`, context overflow, aborted, and unrecoverable exits visible in the transcript instead of only storing them in app state.

### Verified Tests

The following command passed after the patch:

```sh
GOCACHE=/private/tmp/go-nandocode-llm-gocache go test ./...
```

## Remaining Risks

### R1. Heuristic False Negatives

The detector can miss other preamble-only variants. Examples:

```text
Certainly. The report follows below.
```

```text
I found the missing implementation work. It is:
```

```text
The answer is as follows.
```

The current heuristic should catch the observed bug, but the phrase space is broader than the initial patch.

### R2. Heuristic False Positives

Some legitimate short answers end with a colon or contain words like `summary` or `analysis`. The patch mitigates this by checking word count, length, and preamble cues, but follow-up tests should cover normal short responses.

### R3. Preamble Remains Visible

The first preamble has already streamed to the TUI before the guard can decide it is incomplete. The retry adds the real answer afterward, but the transcript still contains the preamble.

That is acceptable for now because it preserves streamed output honestly. A later UX pass could visually mark the preamble as superseded, but that requires a transcript item update mechanism tied to assistant message IDs.

### R4. Hidden Thinking Can Be Misleading

The model may have a correct plan in thinking but still produce an incomplete final response. The application should not assume that thinking implies completion. The terminal condition must be based on final assistant content and tool state, not only hidden/visible reasoning.

### R5. No Persistent Trace For This Failure Class

The user can see the retry notice in the transcript, but the app does not yet persist a structured trace record with:

- done reason;
- assistant content length;
- thinking length;
- output token count;
- prompt token count;
- retry trigger reason;
- whether the run completed after the retry.

This makes postmortem analysis harder.

### R6. Thinking-Only Runs Can Stay Active Without Delivering Final Content

The screenshot shows the run still active (`[Running...]`) while the visible thinking block contains enough material to produce the requested report. If the model keeps emitting thinking tokens, or if the stream stays open without content, the semantic retry guard does not run because it only evaluates after a model turn finishes.

This needs separate handling from preamble-only completion.

### R7. Token Counter May Be Misleading Or Broken

The screenshot shows `tokens: 2996719`, which is far beyond the configured `num_ctx` defaults discussed elsewhere. This could be:

- cumulative session token accounting;
- double-counting;
- a duration or nanosecond value incorrectly displayed as token count;
- provider usage metadata being interpreted incorrectly;
- expected cumulative total but displayed without context.

Regardless of the cause, this number makes debugging harder because the user may interpret it as current context size.

## Actionable Task List

### Task A1: Add A Manual Reproduction Script

Priority: High

Create a repeatable manual smoke test that reproduces the original failure shape in the TUI.

Suggested file:

```text
docs/manual-tests/INCOMPLETE-RESPONSE-RECOVERY.md
```

Include:

- initial project state;
- model used;
- exact prompt;
- expected transcript before patch;
- expected transcript after patch;
- expected retry notice;
- expected final summary content.

Acceptance criteria:

- The doc includes the exact transcript from this report.
- A tester can run the TUI and verify whether the retry appears.
- The test explicitly checks that the final answer appears after the retry.

### Task A2: Add A Fake-Model TUI Integration Test

Priority: High

Add a test where the TUI receives an agent event sequence matching the failure:

1. assistant text delta with `Now I have the full picture... summary:`;
2. thinking delta with a long plan;
3. retry notice;
4. assistant text delta with the actual report;
5. terminal completed.

Acceptance criteria:

- The TUI view includes the retry notice.
- The TUI view includes the final report.
- The thinking block finalizes correctly.
- The run ends as completed.

Notes:

- Unit tests already cover the agent retry behavior.
- This task should verify the complete user-facing transcript shape.

### Task A3: Persist Structured Retry Diagnostics

Priority: High

Extend observability for semantic retries.

Capture:

- retry kind: `incomplete_assistant_response`;
- `done_reason`;
- assistant content character count;
- thinking character count;
- prompt token count;
- output token count;
- retry attempt;
- final terminal reason.

Acceptance criteria:

- A completed run with one incomplete-response retry increments a metric or records a trace field.
- Existing observability tests are updated.
- The data is accessible in whatever metrics/debug surface the project already uses.

### Task A4: Add Done-Reason Visibility In Debug Output

Priority: Medium

Expose the last model `DoneReason` somewhere useful for debugging. Options:

- a debug transcript item when debug mode is enabled;
- a metrics snapshot;
- a `/debug` or self-info tool field;
- structured logs.

Acceptance criteria:

- A developer can distinguish `stop`, `length`, `watchdog_timeout`, and stream errors without instrumenting code manually.
- Normal TUI output remains clean by default.

### Task A5: Expand Incomplete-Response Fixture Coverage

Priority: Medium

Add more table-driven test cases to `internal/agent/incomplete_response_test.go`.

Recommended cases:

- `The report follows below.`
- `I found the issue. The fix is:`
- `Certainly. Here are the tasks:`
- `I will provide the complete plan now.`
- normal answer: `Summary: nothing is missing.`
- normal answer: `Analysis complete. No changes needed.`
- normal answer with code block ending in a colon-like line.

Acceptance criteria:

- The detector catches likely preamble-only failures.
- It does not retry normal short answers.
- It does not retry substantial markdown summaries.

### Task A6: Decide Whether To Replace Or Keep Preamble Transcript Items

Priority: Medium

Make a product decision for what should happen to the streamed preamble after the retry succeeds.

Options:

1. Keep it as-is.
2. Append a system note after it: `[Assistant stopped early; retrying]`.
3. Mark the assistant item as superseded.
4. Remove it from final persisted conversation but keep it in visible transcript.

Recommendation:

Start with option 2. It is transparent and does not require message IDs or transcript mutation beyond the existing retry notice.

Acceptance criteria:

- The chosen behavior is documented.
- Tests cover transcript output for the chosen behavior.
- Conversation persistence does not accumulate confusing synthetic retry prompts as user messages.

### Task A7: Add A Model-Specific Regression Prompt Suite

Priority: Medium

Create a small eval set for local models that previously exhibit this behavior, especially thinking models.

Suggested prompts:

```text
Analyze docs/ and generate a detailed missing-implementation summary as a markdown document.
```

```text
Read the phase plans and produce a prioritized task list. Do not stop after a preamble.
```

```text
Generate docs/MISSING-IMPLEMENTATION-SUMMARY.md based on the repository status.
```

Acceptance criteria:

- The eval records model name, prompt tokens, output tokens, final text length, retry count, and terminal reason.
- It can be run manually without network access.
- It catches regressions where the model repeatedly emits only preambles.

### Task A8: Add A Hard Completion Contract For Artifact Requests

Priority: Medium

For prompts that ask the assistant to create or update a file, prefer tool-backed completion over text-only promises.

Potential behavior:

- If the model says it will write a file but no file tool call occurs, retry with an instruction to either use the file tool or explain why it cannot.
- If the prompt explicitly requests a `.md` file, require a file write or an explicit refusal/error.

Acceptance criteria:

- File/artifact requests do not terminate successfully after only a promise.
- Tests cover a model turn that says `I will write docs/X.md:` without a tool call.
- The retry remains capped.

### Task A9: Improve Prompting To Avoid Preamble-Only Final Answers

Priority: Low

Add or revise the agent system prompt to discourage "I will now..." final answers.

Candidate instruction:

```text
When you have enough information, provide the requested answer or perform the requested file edit directly. Do not end a turn by saying you will write, summarize, analyze, or continue unless you are about to call a tool.
```

Acceptance criteria:

- The instruction is placed in the appropriate central prompt path.
- Tests or manual evals show fewer preamble-only completions.
- The instruction does not reduce useful planning behavior before tool calls.

### Task A10: Keep `num_ctx` Large For Project Analysis, Optimize Elsewhere

Priority: High

Do not reduce `num_ctx` as the primary fix for this issue. Large project analysis needs enough prompt context to include relevant files and prior tool outputs.

Instead, continue the context-latency plan:

- token-aware prompt assembly;
- file summarization before full inclusion;
- retrieval over raw bulk context;
- context compaction;
- faster first-token observability;
- clearer prompt/output budget separation.

Acceptance criteria:

- The docs explain that `num_ctx` controls maximum context capacity, not just latency.
- Project-scale analysis remains possible.
- Speed improvements come from smarter context selection and streaming/rendering improvements, not blindly shrinking context.

### Task A11: Detect And Recover Unproductive Thinking-Only Stalls

Priority: High

Add watchdog logic for runs that stream thinking or stay active without producing final assistant content, tool calls, checkpoints, or other useful progress.

Do not use thinking length alone as a cancellation trigger. Long thinking can be valid for deep project analysis. A stall detector must combine multiple signals.

Safer stall signals:

- no stream events arrive for the existing watchdog interval;
- no assistant content, tool call, progress event, checkpoint, or file write appears for N seconds;
- the model repeats intent statements such as `I will write`, `let me generate`, or `let's create the file` without taking the corresponding action;
- the run exceeds a configurable wall-clock budget for the current phase and has no new evidence/checkpoint to show;
- the same phase restarts or retries repeatedly without adding new evidence.

Potential recovery behavior:

- emit a visible retry/system notice;
- cancel the current stream;
- retry once with an instruction such as:

```text
You have already reasoned through the task. Stop planning and provide the final answer or perform the required file edit now.
```

Acceptance criteria:

- A fake stream that emits only thinking and then stalls does not leave the TUI permanently running.
- A fake stream that emits excessive but active/productive thinking is not cancelled solely because of length.
- A fake stream that repeatedly promises output without producing content or tool calls is cancelled or retried.
- The user sees a clear transcript notice explaining what happened.
- Normal long but productive thinking is not interrupted too aggressively.

### Task A12: Fix Or Clarify The TUI Token Counter

Priority: High

Investigate the status bar token value shown as:

```text
tokens: 2996719
```

Determine exactly what this value represents.

Acceptance criteria:

- The displayed token number is traced to its source field.
- Tests cover token accounting for at least one completed agent run.
- If the value is cumulative, label it clearly, for example `session tokens`.
- If the value is current-context tokens, ensure it cannot exceed reported prompt/context values without explanation.
- If the value is wrong due to double-counting or unit confusion, fix the accounting.

### Task A13: Add A Project-Scale Analysis State Machine

Priority: High

Large project analysis should not be a single unstructured prompt followed by a single final answer. Add an explicit workflow for repository-scale requests.

Recommended phases:

1. inventory: identify relevant files and documents;
2. triage: rank files by relevance and decide read depth;
3. evidence collection: read selected files and record facts;
4. per-area summary: summarize by subsystem, phase, or feature area;
5. synthesis: produce the final report from the evidence ledger;
6. validation: check for contradictions, missing files, and unsupported claims;
7. output: write or stream the final answer/artifact.

Acceptance criteria:

- The agent can expose the current phase in the TUI.
- Each phase can emit a compact progress/checkpoint event.
- A stalled run can be diagnosed by phase, not just by generic `[Running...]`.
- The final answer is based on collected evidence, not only raw chat history.

### Task A14: Build An Evidence Ledger For Deep Analysis

Priority: High

Add a structured evidence ledger for project-analysis runs. This should record what the agent actually inspected and what conclusions were derived from it.

Suggested fields:

- file path;
- byte or line range inspected when available;
- reason the file was selected;
- extracted facts;
- confidence or freshness note;
- open questions;
- link from final claim to supporting evidence.

Acceptance criteria:

- A final large-project report can list the main files used.
- The agent can identify which conclusions are unsupported or weakly supported.
- Compaction preserves the ledger instead of flattening it into vague prose.
- Tests cover a synthesis generated from multiple ledger entries.

### Task A15: Add Token-Aware Context Packing For Large Projects

Priority: High

Keep `num_ctx` large, but avoid filling it with low-value or duplicate content. Add a context packer that builds model prompts from prioritized evidence.

The packer should:

- reserve output tokens before filling input context;
- include the user objective and current phase;
- include the evidence ledger summary;
- include only the most relevant raw excerpts;
- include recent conversation turns only when needed;
- avoid re-sending unchanged large file contents every turn;
- record what was omitted because of budget.

Acceptance criteria:

- Project-scale analysis uses stable context budgets instead of unbounded prompt growth.
- Reducing latency does not require lowering `num_ctx`.
- The final report can still cite or summarize many files through the ledger.
- Tests cover packing behavior when evidence exceeds the available budget.

### Task A16: Add Checkpoint And Resume Support For Long Analyses

Priority: High

Long analysis should survive retries, compaction, model stalls, and user `continue` messages. Persist enough structured state to resume without rereading everything.

Checkpoint contents:

- original user objective;
- current analysis phase;
- files inventoried;
- files read;
- evidence ledger;
- partial summaries;
- outstanding tasks;
- last model/tool event timestamps;
- retry/stall history.

Acceptance criteria:

- If a run is interrupted after evidence collection, a follow-up `continue` can resume synthesis from checkpoints.
- Checkpoints are compact and do not require storing all raw file contents.
- The TUI can show that the run is resuming from a checkpoint.

### Task A17: Add Large-Project Reliability Evals

Priority: High

Add evals that measure whether the system can analyze many project documents without losing quality.

Eval dimensions:

- number of files discovered;
- number of files read;
- prompt/context tokens per turn;
- output tokens;
- first-token latency;
- total wall time;
- retry count;
- compaction count;
- final report completeness;
- evidence coverage;
- unsupported claim count.

Acceptance criteria:

- At least one eval uses the repo's own `docs/PHASE-*-DETAILED-PLAN.md` files.
- The eval checks that final output includes conclusions from early, middle, and late documents.
- The eval fails if the model only emits a preamble.
- The eval fails if the final answer lacks evidence coverage.

### Task A18: Add A Final-Answer Quality Gate For Deep Reports

Priority: Medium

Before accepting `TerminalCompleted` for large-analysis tasks, run a lightweight quality gate over the final assistant message.

The gate should check:

- the answer is not only a preamble;
- required artifact/file was produced when requested;
- the answer has enough structure for the requested report;
- major evidence areas are represented;
- unresolved uncertainty is explicitly stated;
- output is not empty after a long thinking/tool phase.

Acceptance criteria:

- Quality gate failures emit a visible retry notice.
- The retry prompt includes missing requirements, not a generic "continue."
- The gate is only active for analysis/report/artifact tasks, not every short chat answer.

## Suggested Implementation Order

1. A12: Fix or clarify the TUI token counter so debugging starts from trustworthy numbers.
2. A3: Persist structured retry and run diagnostics.
3. A4: Add done-reason debug visibility.
4. A5: Expand detector tests while the patch is fresh.
5. A2: Add TUI integration coverage for the visible transcript.
6. A1: Add the manual smoke test.
7. A10: Preserve large `num_ctx` capacity and optimize context selection instead.
8. A13: Add a project-scale analysis state machine.
9. A14: Build an evidence ledger for deep analysis.
10. A15: Add token-aware context packing for large projects.
11. A16: Add checkpoint and resume support for long analyses.
12. A11: Detect and recover unproductive thinking-only stalls without interrupting productive analysis.
13. A18: Add a final-answer quality gate for deep reports.
14. A8: Add artifact-request completion contract.
15. A17: Add large-project reliability evals.
16. A7: Build a small local model-specific prompt suite.
17. A6: Decide transcript handling for superseded preambles.
18. A9: Tune central prompting.

## Agent Handoff Notes

If a future agent continues this work, start by reading:

- `internal/agent/incomplete_response.go`
- `internal/agent/agent.go`
- `internal/agent/agent_test.go`
- `internal/tui/app.go`
- `internal/tui/app_test.go`
- `docs/CONTEXT-LATENCY-OPTIMIZATION-PLAN.md`
- this report

Then run:

```sh
GOCACHE=/private/tmp/go-nandocode-llm-gocache go test ./...
```

Do not treat the observed failure as only a context-window problem. The detailed interaction proves the model had enough internal context to outline the answer. The failure was that the final assistant message stopped at a promise, and the application accepted that as a completed run.

Also do not treat the desired fix as "make the model think less." The target behavior is deep analysis with visible progress, trustworthy context management, and reliable final delivery.
