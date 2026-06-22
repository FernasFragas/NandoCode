# Inaccurate Listing Response Report

Date: 2026-05-17  
Scope: `list all the files in @docs/` returning unrelated analysis output

## Symptom

Observed run:

- User prompt: `list all the files in @docs/`
- TUI summary: `expanded 1 directories, 21 files included, 52 discovered, 485.1 KiB`
- Assistant output: long unrelated "Phase 15 / executeToolCalls" reasoning block

This is incorrect behavior: the user requested a directory listing, not analysis/planning text.

## What The Current Code Is Doing

1. Mention expansion:
   - `internal/mentions/expand.go` uses `shouldPreferTreeOnly(input)` to detect listing intent.
   - Tree-only mode is only enabled when hardcoded trigger substrings match exactly.
   - Current trigger list includes `list files`, but not `list all the files`.

2. Because trigger matching fails:
   - Expansion stays in content mode (`effectiveMentionMode(auto) => content`).
   - Directory block includes file bodies for a bounded subset (`21 included`, `485.1 KiB`).

3. Model input quality degradation:
   - The prompt becomes large and content-heavy.
   - The user intent ("just list files") is weak relative to large attached bodies/history.
   - Model may anchor to prior conversation themes (e.g., Phase 15 task text) instead of performing listing.

4. Fallback workflow:
   - `tryProjectWorkflowFallback()` in `internal/tui/app.go` only triggers for analysis intents (`analy`, `audit`, `review`, `summary`, `report`).
   - It is not the source of this listing failure.

## Root Causes

1. Fragile intent detection:
   - Listing detection is phrase-fragile and misses common variants (`list all the files`, `list every file`, `show all files`).

2. Wrong default for missed detection:
   - If detection misses, default mode remains content-heavy, which is high-risk for intent drift.

3. No durable prompt-shape proof in final prompt:
   - The final prompt must preserve the user's listing request and tree-only context.
   - Per the 2026-05-17 user decision, the fix must not append a separate answer-style constraint such as "return only the listing".

4. No assertion that listing prompt produced tree-only expansion:
   - TUI reports counts but does not warn when a likely listing prompt still expanded file bodies.

## Why The Retry Message Appeared

`[Retry 1] assistant response looked incomplete...` is from the existing incomplete-response recovery (`internal/agent/agent.go`).  
It is a downstream effect: model produced low-quality/off-target response, then retry logic attempted continuation.

## Gaps In Current Tests

Existing tests cover:

- `name the all the files and folders in @docs` -> tree mode
- explicit `?tree` and `?content`

Missing high-value variants:

- `list all the files in @docs`
- `list every file in @docs`
- `show all files in @docs`
- `list folders in @docs`

## Priority Fix Plan

## P0 - Make Listing Detection Robust

Files:

- `internal/mentions/expand.go`
- `internal/mentions/expand_test.go`

Tasks:

1. Replace simple substring checks with normalized phrase matching:
   - Tokenize, collapse whitespace/punctuation, and match intent patterns.
2. Add listing synonyms:
   - `list all files`
   - `list all the files`
   - `list every file`
   - `show all files`
   - `enumerate files`
3. Keep negative guard list (`review`, `summarize`, `analyze`, etc.).

Acceptance:

- `list all the files in @docs/` must force tree mode.
- No `<file ...>` blocks included in expanded prompt for that case.

## P0 - Do Not Add Listing-Mode Output Constraint

Files:

- `internal/tui/app.go` or `internal/mentions/expand.go` (where prompt is finalized)

Tasks:

1. Preserve listing intent by expanding `@path` as tree-only.
2. Do not append a strict short instruction such as:
   - "Return only the file/folder listing for requested paths."
3. Ensure review/analyze prompts remain content mode unless the user explicitly requests `?tree`.

Acceptance:

- The final prompt for `list all the files in @docs/` contains the original user request and `mode="tree"` directory block.
- The final prompt contains zero `<file ...>` body blocks.
- The final prompt contains no listing-only answer constraint.

## P1 - Add TUI Misclassification Warning

Files:

- `internal/tui/app.go`

Tasks:

1. If prompt looks listing-like but expansion mode is content, add warning:
   - `[Warning: listing intent detected late or ambiguous; use @path?tree to force tree mode]`

Acceptance:

- User receives immediate correction path without debugging internals.

## P1 - Expand Regression Coverage

Files:

- `internal/mentions/expand_test.go`
- optional `internal/tui/app_test.go`

Tasks:

1. Add tests for all phrase variants above.
2. Add test that `list all the files in @docs/` produces tree mode.
3. Add test that `review @docs` remains content mode.

Acceptance:

- Listing-intent regressions are caught in CI.

## P2 - Prompt-Pack Diagnostics For Intent Drift

Files:

- `internal/agent/prompt_packer.go`
- `internal/tui/app.go`

Tasks:

1. Surface whether latest user message and mention blocks survived packing.
2. Show this in `/prompt last` and TUI notices.

Acceptance:

- Operator can quickly prove whether wrong output came from packing vs model behavior.

## Immediate Workaround

Use explicit mode in user prompt:

```text
list all the files in @docs?tree
```

This bypasses phrase-detection ambiguity and forces tree-only expansion now.

## Recommended Next Implementation Order

1. P0 detection robustness
2. P0 no listing output constraint
3. P1 regression tests
4. P1 TUI warning
5. P2 diagnostics polish
