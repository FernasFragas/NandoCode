# Inaccurate Listing Response Deep Dive

Date: 2026-05-17
Related report: `docs/INACCURATE-LISTING-RESPONSE-REPORT-2026-05-17.md`
Observed prompt: `list all the files in @docs/`

## Executive Summary

The existing report identifies the main immediate cause correctly: `list all the files in @docs/` is not classified as listing intent, so `@docs/` expands in content mode and sends hundreds of KiB of file bodies. That makes the prompt vulnerable to history/context drift, which explains the unrelated Phase 15 reasoning.

The deeper implementation problem is broader:

- Listing intent is a private boolean inside `mentions.ExpandPrompt`, not durable prompt metadata.
- The TUI only receives file/directory expansion counts, not an explicit "this was a listing request" report.
- Tree mode changes which content is attached. Per the 2026-05-17 user decision, it must not add an answer-style constraint such as "return only the directory listing".
- Prompt packing and prompt dump reporting must prove that the latest listing instruction and tree block survived to the final `llm.ChatRequest` without relying on injected answer constraints.
- The incomplete-response retry can reinforce the wrong task by asking for "the promised answer" after the model has already drifted; this remains a recovery risk to validate in live runs.

The best fix is to turn listing intent into first-class prompt-expansion metadata and enforce it through expansion, TUI notices, prompt construction, tests, and final request inspection.

## Current Verification Status

As of the latest verification pass on 2026-05-17, the code-level fixes described in this report are implemented and covered by automated tests.

Verified by:

```bash
go test ./internal/agent ./internal/mentions ./internal/tui ./internal/commands ./internal/observability
go test ./...
```

Current code behavior:

- `list all the files in @docs/` is classified as listing intent and expands directory mentions as tree-only.
- Tree listing prompts do not include `<file ...>` body blocks.
- Listing prompts expand as tree-only and do not append a listing-only answer contract.
- `review @docs/` and `summarize @docs/` remain content-mode prompts.
- `list all the files in @docs?content` honors the explicit suffix and shows a warning that file bodies were attached despite listing wording.
- Directory blocks include explicit `files_discovered`, `dirs_discovered`, `files_included`, `content_bytes`, `mode`, and `source` attributes.
- `?all` renders as `mode="all"` and uses filesystem-backed expansion.
- Cap/truncation reasons are surfaced in rendered directory metadata, TUI summaries, and `ResolvedDirectory.OmittedReasons`.
- Incomplete-response retry uses the generic preamble recovery prompt; listing-specific retry constraints were removed with the answer-contract decision.
- Prompt packing reports dropped mention blocks and latest-user preservation without listing-constraint-specific metadata.
- `/prompt last` reports dump mode and explains how to enable previews when dump mode is off.
- `/trace last` can show an active current-run trace before terminal completion and includes mention-expansion mode/count metadata.

No known code blocker remains for this listing-response issue after removing the answer-constraint injection. Remaining work is manual live evidence capture and documentation carry-forward before starting Phase 22.

## Original Findings And Current Verification

The findings below document the root cause at the time of the observed failure. They are retained for traceability. The implementation status update near the end of this report records which items are now fixed and which verification tasks remain.

## 1. Intent Detection Is Phrase-Fragile

File: `internal/mentions/expand.go`

Current function:

```go
func shouldPreferTreeOnly(input string) bool
```

Current trigger examples:

```text
list files
list folders
name the files
name all files
name the all the files and folders
show tree
directory tree
what files are in
```

Why the observed prompt misses:

```text
list all the files in @docs/
```

This does not contain the exact substring `list files`. It contains `list all the files`, which is absent from the trigger list.

Result:

- `autoTree == false`
- `MentionModeAuto` remains `content`
- `expandDirectory()` reads file bodies
- TUI shows `21 files included, 52 discovered, 485.1 KiB`

Expected listing behavior would have shown something like:

```text
expanded 1 directories as tree, 52 files discovered
```

## 2. Content Mode Creates Prompt Drift Risk

File: `internal/mentions/expand.go`

When mode remains `auto`, this function makes it content mode:

```go
func effectiveMentionMode(mode MentionMode) MentionMode {
    switch mode {
    case MentionModeAuto, MentionModeAll:
        return MentionModeContent
    default:
        return mode
    }
}
```

Then `expandDirectory()` reads and appends file bodies until caps stop it.

For a listing prompt, this is the wrong failure mode. If intent detection is uncertain, the safer default for `list/name/show/enumerate files` should be tree-only, not content-heavy.

## 3. Tree Mode Has No Answer Contract

Even when tree mode works, the final prompt is still mostly:

```text
<original user request>

Referenced files and directories:
<directory ... mode="tree">
<tree>
...
</tree>
</directory>
```

There is intentionally no extra answer-style instruction appended to the final user message. Earlier versions of this plan proposed adding one, but that path is now superseded: the prompt should stay faithful to the user's request and attached tree context, while diagnostics prove the correct tree context reached the LLM.

## 4. TUI Summary Does Not Expose Misclassification

File: `internal/tui/app.go`

Current summary logic:

```go
func directoryExpansionSummary(_ []mentions.ResolvedFile, dirs []mentions.ResolvedDirectory) string
```

When mode is not `tree`, it reports:

```text
expanded 1 directories, 21 files included, 52 discovered, 485.1 KiB
```

This is technically accurate, but it does not say:

- The user likely asked for a listing.
- File bodies were attached anyway.
- `@docs?tree` would have avoided the mismatch.

The summary should treat "listing-like prompt + included file bodies" as a warning condition.

## 5. Directory XML Attributes Are Ambiguous

File: `internal/mentions/expand.go`

Current tree-mode directory blocks still use:

```xml
<directory path="docs" files="0" bytes="0" truncated="false" mode="tree">
```

In tree mode, `files="0"` means "zero file bodies included", not "zero files discovered". The `<tree>` contains file paths, but the attribute can mislead both users and the LLM.

Better shape:

```xml
<directory path="docs" mode="tree" files_discovered="52" dirs_discovered="3" files_included="0" content_bytes="0" truncated="false">
```

This reduces ambiguity and gives prompt-pack diagnostics better structure.

## 6. `?all` Loses Its Semantics In Rendered Mode

File: `internal/mentions/expand.go`

`MentionModeAll` forces filesystem source in `expandDirectory()`, but `effectiveMentionMode(MentionModeAll)` returns `content`.

Result:

- Behavior may be correct internally.
- Rendered metadata says `mode="content"`, not `mode="all"`.

This is not the direct cause of the observed listing failure, but it weakens diagnostics.

## 7. Omitted Reasons Are Incomplete For Caps

File: `internal/mentions/expand.go`

`OmittedReasons` collects skipped files already added to `skipped`, plus gitignored count. It does not reliably add reasons when the loop breaks due to:

- `dirwalk.ReasonFileCap`
- `dirwalk.ReasonByteCap`
- `prompt-file-cap`
- `prompt-byte-cap`

Observed run likely hit a byte cap or per-directory cap because only 21 of 52 files were included. The TUI summary did not expose a cap warning.

## 8. Prompt Dump Is Useful But Not Sufficient By Default

File: `internal/agent/prompt_dump.go`

`recordPromptDump()` always records in-memory metadata. But with `prompt_dump_mode = "off"`, message previews and full content are not stored.

That means `/prompt last` can confirm message sizes and roles, but not whether the final request contained:

- file bodies,
- a tree-only block,
- the listing instruction,
- stale Phase 15 context from history.

For this class of bug, `prompt_dump_mode = "metadata"` should be recommended during investigation.

## 9. Prompt Packing Preserves Latest User Message But Not Intent Semantics

File: `internal/agent/prompt_packer.go`

The packer tracks:

- latest user message inclusion,
- mention block counts,
- dropped messages,
- dropped mention blocks.

This helps after the fact. It does not know that a prompt is a listing request, and it does not ensure that a listing guard or tree block survived as an atomic prompt part.

## 10. Incomplete-Response Retry Can Reinforce The Wrong Task

File: `internal/agent/agent.go`

When a response looks incomplete, the agent appends:

```text
You stopped after a preamble. Provide the promised answer now. Do not say you will write it; write it.
```

In the observed failure, the model had already drifted into Phase 15 reasoning. The retry then asks it to continue "the promised answer", which can reinforce the wrong context instead of returning to the user's actual listing prompt.

Earlier versions of this plan proposed constraining the retry by the original turn intent. That path is now superseded for listing prompts because it reintroduces an answer-style instruction.

## Correct Target Behavior

For:

```text
list all the files in @docs/
```

The final request should have these properties:

- Latest user instruction is preserved verbatim.
- `@docs/` expands as tree-only.
- No `<file ...>` body blocks are attached.
- Directory metadata says discovered and included counts clearly.
- No listing-only response contract is attached.
- TUI summary explicitly says `as tree`.
- `/prompt last` can show that the final request shape matches the intent.

Expected TUI summary:

```text
expanded 1 directories as tree, 52 files discovered, 0 file bodies included
```

Expected model behavior:

```text
docs/
docs/ADR-001-TUI-USER-EXPERIENCE-IMPROVEMENTS.md
docs/CONTEXT-LATENCY-OPTIMIZATION-PLAN.md
...
```

## Recommended Implementation Plan

## P0 - Replace Substring Intent Detection With Token-Based Listing Detection

Files:

- `internal/mentions/expand.go`
- `internal/mentions/expand_test.go`

Tasks:

1. Add a small tokenizer for intent detection:
   - lowercase,
   - strip punctuation,
   - collapse whitespace,
   - preserve word order.
2. Recognize listing verbs:
   - `list`
   - `name`
   - `show`
   - `enumerate`
   - `print`
   - `display`
3. Recognize listing nouns:
   - `file`
   - `files`
   - `folder`
   - `folders`
   - `directory`
   - `directories`
   - `tree`
4. Ignore filler words between verb and noun:
   - `all`
   - `the`
   - `every`
   - `each`
5. Keep negative intent guards:
   - `review`
   - `summarize`
   - `analyze`
   - `inspect contents`
   - `find bugs`
   - `compare`

Acceptance:

- `list all the files in @docs/` -> tree mode.
- `list every file in @docs/` -> tree mode.
- `show folders in @docs/` -> tree mode.
- `review @docs/` -> content mode.
- `summarize @docs/` -> content mode.

## P0 - Make Listing Intent A Returned Expansion Report

Files:

- `internal/mentions/expand.go`
- `internal/tui/app.go`
- `internal/cli/print.go`

Current API:

```go
ExpandPrompt(input, ctx) (string, []ResolvedFile, []ResolvedDirectory, error)
```

Recommended addition:

```go
type ExpansionReport struct {
    ListingIntent bool
    ModeByMention map[string]MentionMode
    IncludedFileBodies int
    DiscoveredFiles int
    Warnings []string
}
```

Then add:

```go
ExpandPromptDetailed(input, ctx) (string, []ResolvedFile, []ResolvedDirectory, ExpansionReport, error)
```

Keep `ExpandPrompt()` as a compatibility wrapper.

Acceptance:

- TUI can warn when listing intent exists but file bodies were included.
- Agent prompt construction preserves the user's listing request and tree block without appending an answer contract.
- Tests do not infer mode only by string-scanning generated XML.

## P0 - Do Not Add Listing-Only Answer Contract

Files:

- `internal/mentions/expand.go` or `internal/tui/app.go`

Task:

Supersedes the earlier proposal to append a compact listing-only instruction to listing prompts. Do not add that instruction to the expanded user message. The prompt should contain the original user request plus the rendered directory tree context only.

Acceptance:

- The instruction is absent from `/prompt last` for `list all the files in @docs/`.
- The instruction is absent for `review @docs/`, `summarize @docs/`, and `@docs?content`.
- Tree-mode directory expansion still attaches zero file bodies for listing intent.

## P0 - Improve Directory Block Metadata

Files:

- `internal/mentions/expand.go`

Tasks:

1. Replace ambiguous attributes:
   - `files`
   - `bytes`
2. Add explicit attributes:
   - `files_discovered`
   - `dirs_discovered`
   - `files_included`
   - `content_bytes`
   - `source`
   - `mode`
3. Preserve backwards readability of the XML-like block.

Acceptance:

- Tree-mode blocks do not say `files="0"` without also saying discovered count.
- `?all` renders mode/source clearly.

## P1 - Add Misclassification Warnings

Files:

- `internal/tui/app.go`

Tasks:

1. If `ListingIntent == true` and `IncludedFileBodies > 0`, show:

```text
[Warning: listing prompt expanded file bodies; use @path?tree or check listing-intent detection]
```

2. If `Mode == tree`, show:

```text
expanded 1 directories as tree, 52 files discovered, 0 file bodies included
```

3. If caps truncated tree/content, include the cap reason.

Acceptance:

- The observed bad summary cannot pass silently.

## P1 - Keep Retry Prompt Free Of Listing Constraints

Files:

- `internal/agent/agent.go`
- possibly `internal/agent/input.go`

Tasks:

1. Keep incomplete-response recovery from appending listing-only answer instructions.
2. The earlier proposal to use an intent-specific listing retry instruction is superseded and must not be implemented.
3. Continue improving off-task detection separately, but do not solve it by injecting answer constraints into listing prompts.

Acceptance:

- An incomplete listing response does not add a listing-only retry constraint to the next LLM request.

## P1 - Add Final Request Shape Tests

Files:

- `internal/mentions/expand_test.go`
- `internal/tui/app_test.go`
- `internal/agent/*_test.go`

Required tests:

1. `list all the files in @docs/`
   - final prompt contains `mode="tree"`,
   - final prompt contains no `<file path=...>` body blocks,
   - listing answer constraint is absent.
2. `review @docs/`
   - content mode,
   - file body blocks present subject to caps,
   - listing answer constraint absent.
3. `list all the files in @docs?content`
   - explicit suffix wins,
   - content mode,
   - TUI warning explains that file bodies were included despite listing wording.
4. Tiny budget case:
   - latest user prompt survives,
   - prompt pack report says whether mention blocks were dropped.

Acceptance:

- The original failure is a regression test, not only a manual scenario.

## P2 - Improve Prompt Inspection Defaults For Debug Sessions

Files:

- `USER_MANUAL.md`
- `internal/commands/registry.go`
- optional config docs

Tasks:

1. Document that out-of-context answer debugging should use:

```bash
NANDOCODEGO_PROMPT_DUMP=metadata
```

2. `/prompt last` should explicitly say when previews are unavailable because dump mode is `off`.
3. Add a hint:

```text
[Prompt preview disabled. Set prompt_dump_mode="metadata" or NANDOCODEGO_PROMPT_DUMP=metadata.]
```

Acceptance:

- Users can verify final prompt shape without needing full content dumps.

## P2 - Add Running Trace Availability

Files:

- `internal/observability`
- `internal/commands/registry.go`
- `internal/tui/app.go`

Observed issue:

```text
[No run trace recorded yet]
[stage summary] slowest: first_visible_render 12.7s
```

This suggests trace data is only finalized after terminal events, while the user may request trace data before the run is fully recorded.

Tasks:

1. Add "current run trace" or pending trace snapshot.
2. `/trace last` should say whether no terminal trace exists because the run is still active.
3. Include prompt expansion mode/counts in trace metadata.

Acceptance:

- Trace output helps debug live slow/off-context runs instead of returning an empty result.

## Verification Checklist

## Implementation Status Update (2026-05-17)

The following plan slices are now implemented:

- P0 token-based listing detection with broader phrase coverage and negative guards.
- P0 expansion report plumbing (`ExpandPromptDetailed`) + explicit no-answer-constraint behavior for listing prompts.
- P0/P1 directory metadata clarification (`files_discovered`, `dirs_discovered`, `files_included`, `content_bytes`, `mode`, `source`) and explicit `mode="all"` rendering for `?all`.
- P0/P1 truncation reason propagation into `ResolvedDirectory.OmittedReasons`, rendered directory attributes, and TUI summaries.
- P1 listing misclassification warning and tree-mode transcript summary clarity.
- P1 generic incomplete-response retry remains free of listing-answer constraints.
- P1 final-shape regression tests across mentions, TUI, and agent prompt-packing behavior.
- P2 prompt inspection polish (`dump_mode` visibility and off-mode preview hint in `/prompt last`).
- P2 live trace visibility (`Current run trace (active)` path in `/trace last`) with mention expansion metadata included in trace output.

## Remaining Agent Tasks Before Phase 22

No code task remains open for the listing-response fix. The remaining work is verification and evidence capture so Phase 22 starts from a known baseline:

1. Run a live TUI prompt with `NANDOCODEGO_PROMPT_DUMP=metadata`:

```text
list all the files in @docs/
```

Record the transcript expansion line, `/prompt last`, and `/trace last` output in `docs/PHASE-LOG.md`.

2. Run the explicit override case:

```text
list all the files in @docs?content
```

Confirm the warning appears and the prompt dump shows content-mode file bodies.

3. Run the negative guard cases:

```text
review @docs/
summarize @docs/
```

Confirm both remain content mode and do not include a listing-only answer contract.

4. If any live run differs from the automated behavior, create a follow-up bug before Phase 22. Treat regressions in listing intent, prompt dump visibility, prompt packing reports, or unexpected listing-answer constraints as pre-Phase-22 blockers.

After fixes, run:

```bash
go test ./internal/mentions ./internal/tui ./internal/agent ./internal/commands ./internal/config
go test ./...
```

Manual verification prompt:

```text
list all the files in @docs/
```

Expected transcript:

```text
[System] expanded 1 directories as tree, 52 files discovered, 0 file bodies included
```

Expected answer:

- file/folder listing only,
- no Phase 15 analysis,
- no "I will write" preamble,
- no unrelated implementation plan.

## Immediate Workarounds

The main fix has landed. These prompts remain useful as explicit debugging controls:

```text
list all the files in @docs?tree
```

For debugging:

```bash
NANDOCODEGO_PROMPT_DUMP=metadata
```

Then inspect:

```text
/prompt last
```

For a fresh listing run with less history contamination:

```text
/clear
list all the files in @docs?tree
```
