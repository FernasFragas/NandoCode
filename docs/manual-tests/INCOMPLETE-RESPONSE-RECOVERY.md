# Manual Test: Incomplete Response Recovery

Date: 2026-05-16

## Goal

Verify that a project-scale analysis run does not leave the user with only a preamble such as `Let me write the summary:` and no final answer.

This test also checks that long thinking remains allowed. The expected behavior is not to make the model think less; it is to make progress visible and require a real final answer or artifact.

## Setup

Run the TUI from the repository root:

```sh
go run ./cmd/nandocodego
```

Use a thinking-capable model if available, for example:

```text
/model qwen3.6:35b
```

## Prompt

Submit:

```text
Analyze the project docs and generate a detailed missing-implementation summary. Read the phase plans and produce a markdown report with concrete tasks.
```

## Original Failure Shape

Before the recovery patch, the transcript could stop after a promise:

```text
Assistant:

  Based on my thorough analysis of the project structure, implementation status, and all detailed phase plans in docs/, here is the comprehensive missing-implementation summary:

You:
continue

Thinking (2482 chars) Ctrl+T to expand

Assistant:

  Now I have the full picture. Let me write the missing-implementation summary:
```

The final summary never appeared, or the run stayed active with only planning text visible in the thinking block.

## Expected Behavior

If the model stops after a preamble, the transcript should show a retry notice:

```text
[Retry 1] assistant response looked incomplete, requesting the promised answer
```

After that, the assistant should provide the actual report or perform the requested file edit.

The status bar should label cumulative tokens as:

```text
session tokens: <number>
```

It should not imply that cumulative session tokens are the current context size.

## Pass Criteria

- The model can still perform deep multi-document analysis.
- The TUI remains responsive while thinking is collapsed or expanded.
- If the assistant emits only a preamble and stops, one recovery retry is visible.
- The final answer contains the requested summary or the requested artifact is written.
- Non-completed terminal exits appear in the transcript, for example `max_turns` or `context_overflow`.
- `/cost` shows last done reason and retry diagnostics when available.

## Fail Criteria

- The run completes after only `Here is the summary:` or `Let me write...`.
- The run stays indefinitely active with no new stream events, tool calls, progress, checkpoint, final text, or terminal notice.
- The status bar shows a large unlabeled `tokens:` number.
- Lowering `num_ctx` is required for the test to pass.
