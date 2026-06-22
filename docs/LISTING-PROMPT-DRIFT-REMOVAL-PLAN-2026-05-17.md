# Listing Prompt Drift Removal Plan

Date: 2026-05-17
Primary symptom: `list all files and folders in @docs/` starts as a correct listing task, then drifts into unrelated code/spec generation.

## Decision Update

This plan intentionally keeps the LLM in the flow. The goal is not to bypass the model with a local deterministic listing response. The goal is to make the LLM request accurate by controlling exactly what is attached to the query and by fixing retry behavior so a retry cannot replace the user's directory-listing task with an unrelated continuation request.

Removed from the previous plan:

- No local-only listing response.
- No "do not call the LLM" requirement.
- No "no LLM spinner" exit criterion.
- No local prompt dump state that says no prompt was sent.

Still required:

- Listing prompts must send only the latest listing task and listing data needed to answer it.
- Listing prompts must not attach file bodies unless the user explicitly asks for content mode.
- Listing prompts must not attach unrelated dynamic memory.
- Listing prompts must not attach stale session history.
- Retry prompts must preserve the original latest user task and must not use ambiguous "promised answer" wording.

## Implementation Status (2026-05-17)

Status: ✅ Code implementation complete for the planned P0 and P1 drift-removal slices.

Implemented in code and covered by tests:

- First-class prompt intent classification with explicit attachment policy (`directory_listing`, `directory_listing_with_content`, review/analysis blockers).
- Listing-scoped prompt envelope (`User request` + `Directory tree data`) for listing intent.
- Tree-only listing attachment without `<file>` bodies and without XML attachment blocks in listing mode.
- Latest-only outbound history policy for listing requests (state/transcript persistence remains unchanged).
- Listing-aware dynamic memory skip and non-listing fast-recall relevance gate.
- Anchored incomplete-response retry prompt; listing retries preserve original listing request + tree data; `promised answer` wording removed.
- Prompt dump visibility fields for intent, attachment policy, history policy, memory policy, retry policy, included file bodies, and tree attachment.
- `/prompt last` regression coverage for listing runs proving:
  - `intent=directory_listing`
  - `attachment_policy=listing_tree_only`
  - `history_policy=latest_only`
  - `memory_policy=skipped_listing_intent`
  - `retry_policy=anchored_original_request`
  - `included_file_bodies=0`
  - `directory_tree_attached=true`

Validation run:

- `go test ./...`

Remaining operational validation (manual/live evidence):

- Capture live `/prompt last` and `/trace last` evidence for:
  - `list all files and folders in @docs/`
  - `list all files and folders in @docs?content`
  - `review @docs/`
  - `summarize @docs/`

## Executive Summary

The current failure is no longer mainly a listing-intent detection issue. The supplied thinking trace shows that the model initially understands the request and reads the `@docs/` directory tree correctly. The drift happens after unrelated prompt material becomes more salient than the simple listing task.

The relevant code paths show four high-confidence causes:

1. `internal/tui/app.go` appends the expanded listing prompt to the full prior `appState.Messages`, so stale conversation can remain visible when the context budget is large.
2. `internal/memory/runner.go` injects dynamic memory for simple listing turns. In fast mode it selects the first N memory files alphabetically, not the entries relevant to the latest prompt.
3. `internal/agent/incomplete_response.go` builds the retry prompt `You stopped after a preamble. Provide the promised answer now...`, which appears in the bad thinking trace and replaces the concrete listing request with an ambiguous continuation task.
4. `internal/mentions/expand.go` sends a generic "Referenced files and directories" block with XML-like directory metadata. It is tree-only, but the prompt shape is still shared with analysis/content contexts and includes legacy `files`/`bytes` attributes.

The correct fix is an LLM prompt-fidelity fix:

- Classify simple directory listing as a first-class prompt intent.
- Build a listing-scoped LLM request from only the latest user request plus directory tree data.
- Skip dynamic memory for listing-scoped requests.
- Use latest-only/stateless history policy for listing-scoped requests.
- Replace the generic incomplete-response retry with an anchored retry that includes the original user request and original listing context, or disable retry for listing when the first response is already substantive.

## Transcript Analysis

User request in the supplied trace:

```text
list all files and folders in @docs/
```

The first part of the model thinking is healthy:

- It identifies the request as a directory listing.
- It sees directory tree data for `@docs/`.
- It extracts `manual-tests/` and the files under `docs/`.
- It plans to output folders and files.

The failure starts when the model switches to:

```text
The user's actual request is: "You stopped after a preamble. Provide the promised answer now. Do not say you will write it; write it."
```

That sentence is not in the user's listing request. It is the current incomplete-response recovery prompt.

The trace then references:

- `go-sdk`
- package structure
- error handling rules
- config priority
- `cmd/nandocodego/main.go`
- core implementation skeleton/code

Those concepts match dynamic memory files in this repo:

- `memory/nandocodego-conventions.md`
- `memory/nandocodego-error-config.md`
- `memory/nandocodego-architecture.md`
- `memory/MEMORY.md`

Conclusion: the LLM is being induced by attached context and retry mechanics, not by an inability to understand a listing request.

## Current Codebase Findings

## Finding 1 - Listing Prompt Expansion Is Tree-Only But Not Listing-Scoped Enough

Files:

- `internal/mentions/expand.go`
- `internal/mentions/expand_test.go`

Current behavior:

- `ExpandPromptDetailed()` detects listing intent.
- Directory mentions are switched to `MentionModeTree`.
- File bodies are not attached for listing/tree mode.
- The expanded prompt is still rendered as a generic attachment:

```text
<user request>

Referenced files and directories:

<directory path="docs" files="0" bytes="0" files_discovered="54" ... mode="tree">
<tree>
...
</tree>
</directory>
```

Risk:

- The LLM receives a block shape that is also used for content and analysis tasks.
- Legacy `files="0"` and `bytes="0"` attributes are ambiguous beside `files_discovered` and `files_included`.
- The prompt does not clearly separate "original user request" from "directory tree data".

Target:

- Keep the LLM call.
- For listing intent, send a listing-specific prompt envelope:

```text
User request:
list all files and folders in @docs/

Directory tree data:
docs/
manual-tests/
manual-tests/INCOMPLETE-RESPONSE-RECOVERY.md
...
```

- Do not add answer-style constraints such as "return only", "exactly the tree", "no greetings", or "perfect".
- Do not include `<file>` body blocks.
- Do not include generic analysis/review wording.

## Finding 2 - Dynamic Memory Fast Mode Injects Unrelated Project Specs

Files:

- `internal/memory/runner.go`
- `internal/memory/prompt.go`
- `internal/memory/types.go`

Current behavior:

- `memory.Runner.Run()` augments `agent.Input.SystemPrompt` when memory is enabled.
- `buildAugmentedPrompt()` defaults to `fast` recall.
- Fast mode selects the first `MaxSelected` memory entries from `scanRes.Entries`.
- `Scan()` sorts memory entries alphabetically.
- The memory section includes the `MEMORY.md` index and selected file bodies.

Risk:

- For `list all files and folders in @docs/`, project memory is not needed.
- Alphabetical selection can include project-spec memory such as package layout and error/config rules.
- Memory is attached as system prompt context, so it can compete strongly with the user listing task.

Target:

- Listing-scoped requests skip memory injection.
- Fast recall must be relevance-gated for all other requests.
- `MEMORY.md` index must not be injected when no memory entry is selected.

## Finding 3 - Retry Prompt Replaces The Original Task

Files:

- `internal/agent/incomplete_response.go`
- `internal/agent/agent.go`

Current behavior:

The retry path appends this as a new user message:

```text
You stopped after a preamble. Provide the promised answer now. Do not say you will write it; write it.
```

Risk:

- The latest user message no longer says "list files and folders in @docs/."
- "Promised answer" causes the model to search prior messages and memory for a previous task.
- The bad thinking trace shows this exact prompt becoming the model's perceived current task.

Target:

- Retry prompt must be anchored to the original latest user request.
- Retry prompt must include the listing-scoped directory tree context again if it is needed for the answer.
- Retry prompt must never contain "promised answer".
- Listing retries should be conservative: do not retry when the first response contains a substantive listing, even if short.

## Finding 4 - Session History Is Too Broad For Listing Requests

Files:

- `internal/tui/app.go`
- `internal/agent/prompt_packer.go`
- `internal/state/app.go`

Current behavior:

- The TUI sends `updatedMessages := append(appState.Messages, userMessage)`.
- Prompt packing trims only when the prompt exceeds budget.
- A large context window preserves unrelated previous tasks.

Risk:

- Large context can make stale task drift worse.
- Directory listing does not need prior conversation unless the user asks a follow-up such as "same for that folder" or "continue the previous listing".

Target:

- Listing-scoped requests should use a latest-only history policy.
- The current LLM request should include:
  - system prompt, if truly necessary and not memory-derived,
  - latest user listing prompt,
  - listing tree data.
- It should exclude:
  - previous assistant implementation plans,
  - previous retry prompts,
  - dynamic memory sections,
  - old tool outputs,
  - old directory/content attachments.

## Finding 5 - Prompt Dump Does Not Expose Enough Policy Decisions

Files:

- `internal/agent/prompt_dump.go`
- `internal/commands/registry.go`

Current behavior:

- Prompt dumps expose message count, estimated tokens, tools, message previews, and prompt-pack report.
- They do not explicitly say why memory/history/retry were included or skipped for listing prompts.

Target:

- `/prompt last` should show:
  - intent,
  - attachment policy,
  - history policy,
  - memory policy,
  - retry policy,
  - whether `<file>` bodies were attached,
  - whether dynamic memory context was attached.

## Implementation Plan For Agents

## P0 - First-Class Prompt Intent And Attachment Policy

Owner: Agent 1

Files:

- `internal/mentions/expand.go`
- new: `internal/mentions/intent.go`
- `internal/mentions/expand_test.go`
- `internal/agent/input.go`

Tasks:

1. Extract listing/review/analyze detection into a first-class classifier.

```go
type IntentKind string

const (
    IntentUnknown IntentKind = "unknown"
    IntentDirectoryListing IntentKind = "directory_listing"
    IntentDirectoryListingWithContent IntentKind = "directory_listing_with_content"
    IntentReview IntentKind = "review"
    IntentAnalysis IntentKind = "analysis"
)

type AttachmentPolicy string

const (
    AttachDefault AttachmentPolicy = "default"
    AttachListingTreeOnly AttachmentPolicy = "listing_tree_only"
    AttachContent AttachmentPolicy = "content"
)

type IntentReport struct {
    Kind IntentKind
    AttachmentPolicy AttachmentPolicy
    HasMention bool
    ExplicitMode MentionMode
    Reasons []string
}
```

2. Classify `IntentDirectoryListing` when:
   - a prompt has listing verbs/nouns,
   - it contains a directory mention,
   - it does not contain review/analysis/summarize/explain/open/read-content wording,
   - explicit mention mode is not `?content` or `?all`.

3. Classify `IntentDirectoryListingWithContent` when listing wording is present but `?content` or `?all` is explicit.
4. Add the intent report to `ExpansionReport`.
5. Add a string version of intent/policy to `agent.Input` to avoid import cycles.

Acceptance tests:

- `list all files and folders in @docs/` => directory listing + tree-only attachment policy.
- `list all files and folders in @docs?content` => listing-with-content + content attachment policy.
- `review @docs/` => review + content/default policy.
- `summarize @docs/` => analysis/review class, not listing.
- `show contents of @docs/a.md` => not directory listing.

## P0 - Build A Listing-Scoped LLM User Message

Owner: Agent 2

Files:

- `internal/mentions/expand.go`
- new: `internal/mentions/listing_prompt.go`
- `internal/tui/app.go`
- `internal/cli/print.go`
- `internal/tui/app_test.go`

Tasks:

1. Keep sending listing requests to the LLM.
2. For `IntentDirectoryListing`, render a listing-specific user message instead of the generic referenced-files block.
3. Use this shape:

```text
User request:
<original user request>

Directory tree data:
<tree from resolved mentions>
```

4. Do not include:
   - `<file>` blocks,
   - file contents,
   - dynamic memory context,
   - previous retry prompts,
   - old assistant responses,
   - generic "Referenced files and directories" heading,
   - answer-style constraints like "return only", "exactly", "no greetings", or "perfect".

5. Keep content-mode and review/analyze mode behavior unchanged.
6. Preserve directory metadata outside the LLM prompt through `ExpansionReport`, trace, and prompt dump metadata.

Acceptance tests:

- `TestListingPromptUsesListingScopedEnvelope`.
- The final LLM request for `list all files and folders in @docs/` contains `User request:` and `Directory tree data:`.
- The final LLM request does not contain `<file path=`.
- The final LLM request does not contain `=== DYNAMIC MEMORY CONTEXT ===`.
- The final LLM request does not contain `You stopped after a preamble`.
- The final LLM request does not contain package-layout memory text.
- `review @docs/` still attaches content according to existing caps.

## P0 - Intent-Aware History Policy For Listing LLM Calls

Owner: Agent 3

Files:

- `internal/tui/app.go`
- `internal/agent/input.go`
- `internal/agent/prompt_packer.go`
- `internal/agent/events.go`
- `internal/commands/registry.go`

Tasks:

1. Introduce history policy:

```go
type HistoryPolicy string

const (
    HistoryDefault HistoryPolicy = "default"
    HistoryLatestOnly HistoryPolicy = "latest_only"
)
```

2. For `IntentDirectoryListing`, set `HistoryLatestOnly`.
3. When `HistoryLatestOnly` is active, build `agent.Input.Messages` from only the latest listing-scoped user message.
4. Keep the transcript and persisted conversation intact after the response; only the outbound LLM request is latest-only.
5. Keep default history for review, analysis, chat, and explicit `continue`.
6. Record the policy in `PromptPackReport` and prompt dump metadata.

Acceptance tests:

- Given stale prior history containing `You stopped after a preamble`, the next listing LLM request excludes it.
- Given stale prior history containing `cmd/nandocodego/main.go` package-layout text, the next listing LLM request excludes it.
- `review @docs/` still includes history according to default policy.
- `/prompt last` shows `history_policy=latest_only` for listing.

## P0 - Skip Dynamic Memory For Listing-Scoped LLM Requests

Owner: Agent 4

Files:

- `internal/memory/runner.go`
- `internal/memory/runner_test.go`
- `internal/agent/input.go`
- `internal/tui/app.go`

Tasks:

1. Add `PromptIntent` and `AttachmentPolicy` fields to `agent.Input`.
2. In `memory.Runner.buildAugmentedPrompt()`, return `in.SystemPrompt` unchanged when:
   - `PromptIntent == directory_listing`, or
   - `AttachmentPolicy == listing_tree_only`.
3. Add a fallback classifier on `latestUserMessage(in.Messages)` so non-TUI frontends also skip memory for listing prompts.
4. Do not inject `MEMORY.md` index for skipped memory.
5. Emit or record a stage/prompt-dump policy value:

```text
memory_policy=skipped_listing_intent
```

Acceptance tests:

- Listing LLM request does not include `=== DYNAMIC MEMORY CONTEXT ===`.
- Listing LLM request does not include `nandocodego-conventions`.
- Listing LLM request does not include `cmd/nandocodego/main.go` unless the listed directory itself contains that filename.
- `review @docs/` can still use memory.
- `analyze project architecture` can still use memory.

## P0 - Replace The Generic Retry Prompt

Owner: Agent 5

Files:

- `internal/agent/incomplete_response.go`
- `internal/agent/agent.go`
- `internal/agent/agent_test.go`

Tasks:

1. Remove the text `You stopped after a preamble. Provide the promised answer now...` from retry prompts.
2. Replace `buildIncompleteAssistantRetryPrompt(history)` with a retry builder that receives:
   - original latest user request,
   - prompt intent,
   - attachment policy,
   - the exact listing-scoped prompt content already sent to the LLM.

3. For listing intent, retry with the original task and tree data:

```text
Original user request:
<original user request>

Directory tree data:
<same tree data sent in the first request>
```

4. Do not add "return only", "no greetings", or other answer-style constraints.
5. Do not retry listing responses that already contain a substantive list:
   - at least 3 path-like lines, or
   - at least one directory and one file path from the tree data, or
   - more than 80 visible words and no code/spec drift marker.

6. For non-listing prompts, retry with an anchored generic prompt:

```text
Your previous response looked incomplete. Answer the original user request below without continuing any unrelated prior task.

Original user request:
<latest original user request>
```

Acceptance tests:

- Retry prompt never contains `promised answer`.
- Retry prompt for listing contains the original listing request.
- Retry prompt for listing contains the directory tree data.
- Retry prompt for listing does not contain old history.
- Retry prompt for listing does not contain dynamic memory.
- Non-listing incomplete preambles still retry once.

## P1 - Relevance-Gate Fast Memory Recall

Owner: Agent 6

Files:

- `internal/memory/runner.go`
- `internal/memory/runner_test.go`
- optional new helper: `internal/memory/score.go`

Tasks:

1. Replace alphabetical fast recall with lexical scoring against:
   - latest user prompt,
   - memory filename,
   - memory name,
   - memory description,
   - memory type.
2. Add a minimum score threshold.
3. If no memory passes threshold, inject no memory section.
4. Do not include `MEMORY.md` index when no memory entry is selected.
5. Add direct-filesystem negative intent terms:
   - `list files`
   - `list folders`
   - `directory tree`
   - `enumerate files`
   - `show folder`

Acceptance tests:

- `list all files and folders in @docs/` selects zero memories.
- `what are the project package conventions` can select conventions memory.
- `review the TUI implementation plan` can select TUI/phase memory if relevant.

## P1 - Simplify Tree Attachment Shape

Owner: Agent 7

Files:

- `internal/mentions/expand.go`
- `internal/mentions/expand_test.go`

Tasks:

1. For listing-scoped prompts, do not render XML-like `<directory>` tags.
2. Remove legacy `files` and `bytes` attributes from listing prompt text.
3. Keep structured metadata in `ExpansionReport`, not inside the LLM prompt.
4. Keep `<directory>`/`<file>` blocks available for content/review modes if existing tests or features require them.
5. Make the tree renderer deterministic:
   - stable ordering,
   - directories with `/`,
   - skipped entries marked consistently,
   - no duplicate root/subdirectory entries.

Acceptance tests:

- Listing prompt tree data is plain text.
- Listing prompt tree data contains nested files.
- Listing prompt tree data contains no `<directory`, `<file`, `files="0"`, or `bytes="0"`.
- Content mode remains compatible.

## P1 - Prompt Dump And Trace Policy Visibility

Owner: Agent 8

Files:

- `internal/agent/prompt_dump.go`
- `internal/commands/registry.go`
- `internal/observability/metrics.go`
- `internal/tui/app.go`
- `USER_MANUAL.md`

Tasks:

1. Add prompt dump fields:
   - `intent`
   - `attachment_policy`
   - `history_policy`
   - `memory_policy`
   - `retry_policy`
   - `included_file_bodies`
   - `directory_tree_attached`
2. Show these fields in `/prompt last`.
3. Add trace metadata for:
   - memory skipped reason,
   - history policy,
   - retry prompt kind.
4. Add TUI system summary:

```text
[Listing prompt: tree data attached, file bodies=0, history=latest_only, memory=skipped]
```

Acceptance tests:

- `/prompt last` proves listing request used latest-only history.
- `/prompt last` proves memory was skipped.
- `/prompt last` shows no file bodies for listing.

## P1 - Drift Regression Suite

Owner: Agent 9

Files:

- `internal/tui/app_test.go`
- `internal/memory/runner_test.go`
- `internal/agent/agent_test.go`
- `docs/REGRESSION-AND-LOAD-TEST-PLAN.md`

Tasks:

1. Build a fake session with stale history:

```text
User: continue
Assistant: Now I have the full picture...
User: You stopped after a preamble...
Assistant: I will generate cmd/nandocodego/main.go...
```

2. Submit:

```text
list all files and folders in @docs/
```

3. Assert the final LLM request:
   - includes the original listing request,
   - includes directory tree data,
   - excludes stale prior history,
   - excludes dynamic memory,
   - excludes file bodies,
   - excludes generic retry text,
   - excludes package-layout memory text.

4. Simulate an incomplete first answer and assert retry:
   - preserves original listing request,
   - reattaches the same tree data,
   - does not include "promised answer",
   - does not include stale history or memory.

5. Add control tests:
   - `review @docs/` still sends content context.
   - `analyze @docs/` still uses project-analysis workflow or content workflow.
   - `list all files and folders in @docs?content` attaches file bodies and warns.

## Detailed Agent Implementation Task Packets

Use this section as the delegation-ready task list. Each agent should implement only its assigned write scope and must not reintroduce local-only listing behavior. Listing requests must still be sent to the LLM.

## Agent 1 Task Packet - Prompt Intent And Attachment Policy

Priority: P0
Dependency: none

Owned files:

- `internal/mentions/intent.go` (new)
- `internal/mentions/expand.go`
- `internal/mentions/expand_test.go`
- `internal/agent/input.go` only for simple string constants/fields if needed

Do not edit:

- `internal/tui/app.go`
- `internal/memory/runner.go`
- `internal/agent/agent.go`

Implementation steps:

1. Create `internal/mentions/intent.go`.
2. Move the word normalization helpers currently used by `shouldPreferTreeOnly()` into this file or keep wrappers in `expand.go` if smaller.
3. Define:

```go
type IntentKind string
type AttachmentPolicy string

const (
    IntentUnknown                     IntentKind = "unknown"
    IntentDirectoryListing            IntentKind = "directory_listing"
    IntentDirectoryListingWithContent IntentKind = "directory_listing_with_content"
    IntentReview                      IntentKind = "review"
    IntentAnalysis                    IntentKind = "analysis"
)

const (
    AttachDefault         AttachmentPolicy = "default"
    AttachListingTreeOnly AttachmentPolicy = "listing_tree_only"
    AttachContent         AttachmentPolicy = "content"
)
```

4. Define `IntentReport` with `Kind`, `AttachmentPolicy`, `HasMention`, `ExplicitMode`, and `Reasons`.
5. Implement `ClassifyPromptIntent(input string, mentions []parsedMention, resolved []resolvedMention) IntentReport`.
6. Keep detection conservative:
   - listing verbs: `list`, `name`, `show`, `enumerate`, `print`, `display`;
   - listing nouns: `file`, `files`, `folder`, `folders`, `directory`, `directories`, `tree`, `project`;
   - blockers: `review`, `summarize`, `summary`, `analyze`, `analysis`, `audit`, `compare`, `explain`, `inspect contents`, `show contents`, `read`, `open`, `bugs`.
7. Only classify `IntentDirectoryListing` when at least one resolved mention is a directory and every explicit directory mention is auto/tree.
8. Classify `IntentDirectoryListingWithContent` when listing wording exists but the directory mention explicitly uses `?content` or `?all`.
9. Add `Intent IntentReport` to `ExpansionReport`.
10. Keep `ExpansionReport.ListingIntent` populated for existing callers, but derive it from `Intent.Kind`.
11. Add optional string fields to `agent.Input` if needed by later agents:

```go
PromptIntent     string
AttachmentPolicy string
OriginalUserText string
```

Tests to add/update:

- `TestClassifyPromptIntentDirectoryListing`.
- `TestClassifyPromptIntentDirectoryListingWithContent`.
- `TestClassifyPromptIntentReviewBlocksListing`.
- `TestClassifyPromptIntentAnalyzeBlocksListing`.
- `TestClassifyPromptIntentShowContentsBlocksListing`.
- Existing listing tree tests continue to pass.

Acceptance:

- Listing prompt detection is available as structured metadata, not only a private boolean.
- No behavior changes outside metadata and existing tree-only listing expansion.
- `go test ./internal/mentions ./internal/agent` passes.

## Agent 2 Task Packet - Listing-Scoped LLM User Message

Priority: P0
Dependency: Agent 1

Owned files:

- `internal/mentions/listing_prompt.go` (new)
- `internal/mentions/expand.go`
- `internal/mentions/expand_test.go`
- `internal/tui/app_test.go`
- `internal/cli/print.go` only if print mode uses a separate expansion path

Do not edit:

- `internal/memory/runner.go`
- `internal/agent/agent.go`
- `internal/agent/prompt_packer.go`

Implementation steps:

1. Add a listing prompt renderer that receives the original user request plus resolved directory tree text.
2. For `IntentDirectoryListing`, render this user message:

```text
User request:
<original user request>

Directory tree data:
<plain tree data>
```

3. The renderer must not emit:
   - `<directory`;
   - `<file`;
   - `Referenced files and directories`;
   - `Listing response constraint`;
   - `return only`;
   - `exactly`;
   - `no greetings`;
   - `perfect`.
4. Reuse the existing tree walk and existing gitignore/source rules.
5. Preserve all file-count, dir-count, skipped, truncation, and mode metadata in `ExpansionReport`/`ResolvedDirectory`, not in prose instructions.
6. Keep existing generic rendering for content/review/analyze prompts.
7. Update TUI prompt submission test to inspect `runner.input.Messages` and prove the final message uses the listing envelope.
8. Update print mode if it calls `mentions.ExpandPrompt()` without access to `ExpansionReport`; it should use `ExpandPromptDetailed()` so listing-scoped rendering is consistent.

Tests to add/update:

- `TestExpandPromptDetailedListingScopedEnvelope`.
- `TestExpandPromptDetailedListingScopedEnvelopeIsPlainTree`.
- `TestPromptSubmissionListingSendsLLMWithListingEnvelope`.
- `TestPrintListingUsesListingEnvelope`.
- Control test: `review @docs/` still uses content blocks.

Acceptance:

- The LLM is still called for listing prompts.
- The last user message sent to the LLM contains `User request:` and `Directory tree data:`.
- The final LLM request excludes file bodies for listing prompts.
- `go test ./internal/mentions ./internal/tui ./internal/cli` passes.

## Agent 3 Task Packet - Latest-Only History For Listing LLM Calls

Priority: P0
Dependency: Agent 1, Agent 2

Owned files:

- `internal/tui/app.go`
- `internal/tui/app_test.go`
- `internal/agent/input.go`
- `internal/agent/events.go`
- `internal/agent/prompt_dump.go`
- `internal/commands/registry.go`

Do not edit:

- `internal/memory/runner.go`
- `internal/agent/incomplete_response.go`

Implementation steps:

1. Add a history policy field to `agent.Input`:

```go
HistoryPolicy string // "default" or "latest_only"
```

2. Define constants in `internal/agent/input.go`:

```go
const (
    HistoryPolicyDefault    = "default"
    HistoryPolicyLatestOnly = "latest_only"
)
```

3. In TUI prompt submission, when `ExpansionReport.Intent.Kind == directory_listing`, build the outbound `agent.Input.Messages` using only the listing-scoped latest user message.
4. Keep `app.Messages` persistence behavior separate:
   - visible transcript still shows prior turns;
   - after terminal, conversation may still be appended to state;
   - only the outbound LLM request is latest-only.
5. Ensure queued prompts also preserve the policy. If queued prompt handling cannot preserve metadata yet, record a P1 follow-up and add a guard that listing prompts are expanded again when dequeued.
6. Add `HistoryPolicy` to prompt dump metadata and `/prompt last`.
7. Add a TUI system summary line:

```text
[Listing prompt: tree data attached, file bodies=0, history=latest_only]
```

Tests to add/update:

- `TestPromptSubmissionListingDropsStaleHistoryFromLLMRequest`.
- `TestPromptSubmissionListingKeepsTranscriptState`.
- `TestPromptSubmissionReviewKeepsDefaultHistory`.
- `/prompt last` command test showing `history_policy=latest_only`.

Acceptance:

- Stale prior messages containing `You stopped after a preamble` are absent from listing LLM requests.
- Stale prior messages containing `cmd/nandocodego/main.go` are absent from listing LLM requests.
- Non-listing prompts retain current behavior.
- `go test ./internal/tui ./internal/agent ./internal/commands` passes.

## Agent 4 Task Packet - Skip Memory For Listing LLM Calls

Priority: P0
Dependency: Agent 1, Agent 3

Owned files:

- `internal/memory/runner.go`
- `internal/memory/runner_test.go`
- `internal/agent/input.go`
- `internal/tui/app.go` only to pass fields if Agent 3 did not already do it

Do not edit:

- `internal/mentions/expand.go` except for using exported classifier only if unavoidable.

Implementation steps:

1. Ensure `agent.Input` carries:
   - `PromptIntent`;
   - `AttachmentPolicy`;
   - `OriginalUserText`, if needed for fallback classification.
2. In `memory.Runner.Run()` or `buildAugmentedPrompt()`, skip augmentation when:

```go
in.PromptIntent == "directory_listing" ||
in.AttachmentPolicy == "listing_tree_only"
```

3. Add fallback detection for non-TUI callers:
   - inspect latest user message;
   - if it contains `Directory tree data:` and listing words, skip memory.
4. When memory is skipped, do not build the `MEMORY.md` index and do not read selected memory file bodies.
5. Emit a stage timing or policy event if existing event types support it; otherwise add prompt dump policy metadata in Agent 8.
6. Preserve memory behavior for review, analysis, and regular chat.

Tests to add/update:

- `TestRunnerSkipsMemoryForDirectoryListingIntent`.
- `TestRunnerSkipsMemoryForListingTreeAttachmentPolicy`.
- `TestRunnerSkipsMemoryForListingEnvelopeFallback`.
- `TestRunnerKeepsMemoryForReview`.
- `TestRunnerKeepsMemoryForAnalysis`.

Acceptance:

- Listing LLM request excludes `=== DYNAMIC MEMORY CONTEXT ===`.
- Listing LLM request excludes contents of `memory/nandocodego-conventions.md`.
- Memory still works for non-listing requests.
- `go test ./internal/memory ./internal/tui` passes.

## Agent 5 Task Packet - Anchored Retry Mechanism

Priority: P0
Dependency: Agent 1, Agent 2, Agent 3

Owned files:

- `internal/agent/incomplete_response.go`
- `internal/agent/agent.go`
- `internal/agent/agent_test.go`
- `internal/observability/agent_test.go` only if retry notice wording assertions need updates
- `internal/tui/app_test.go` only if retry notice string assertions need updates

Do not edit:

- `internal/memory/runner.go`
- `internal/mentions/expand.go`

Implementation steps:

1. Delete the retry prompt constant containing `You stopped after a preamble...`.
2. Add a retry builder:

```go
type incompleteRetryInput struct {
    PromptIntent string
    AttachmentPolicy string
    OriginalUserText string
    LastUserContent string
}

func buildIncompleteAssistantRetryPrompt(in incompleteRetryInput) (string, bool)
```

3. In `agent.run()`, derive retry input from:
   - `agent.Input.PromptIntent`;
   - `agent.Input.AttachmentPolicy`;
   - `agent.Input.OriginalUserText`;
   - the latest user message in current history.
4. For listing intent, retry by resending the same listing-scoped content:

```text
Original user request:
<original user request>

Directory tree data:
<same tree data>
```

5. For non-listing intent, retry with:

```text
Your previous response looked incomplete. Answer the original user request below without continuing any unrelated prior task.

Original user request:
<original user request>
```

6. Do not include:
   - `promised answer`;
   - `return only`;
   - `no greetings`;
   - old assistant messages;
   - dynamic memory.
7. Add listing-specific no-retry guard:
   - if assistant content has at least 3 path-like lines, do not retry;
   - if assistant content mentions one known directory and one known file from the tree data, do not retry;
   - if assistant content is longer than 80 visible words and has no drift markers, do not retry.
8. Add drift markers for retry diagnostics only:
   - `cmd/nandocodego/main.go`;
   - `Package Source Layout`;
   - `go-sdk`;
   - `provide the promised answer`;
   - `core implementation skeleton`.

Tests to add/update:

- `TestBuildIncompleteAssistantRetryPromptDoesNotUsePromisedAnswer`.
- `TestListingRetryResendsOriginalRequestAndTreeData`.
- `TestListingRetryExcludesHistoryAndMemory`.
- `TestListingSubstantiveShortAnswerDoesNotRetry`.
- `TestNonListingIncompletePreambleStillRetries`.
- `TestRetryNoticeCauseDoesNotSayPromisedAnswer`.

Acceptance:

- The string `promised answer` no longer appears in any live retry prompt or retry notice.
- Listing retry prompt preserves the original listing task.
- Non-listing retry still recovers incomplete preambles.
- `go test ./internal/agent ./internal/tui ./internal/observability` passes.

## Agent 6 Task Packet - Relevance-Gated Fast Memory Recall

Priority: P1
Dependency: Agent 4

Owned files:

- `internal/memory/runner.go`
- `internal/memory/runner_test.go`
- `internal/memory/score.go` (new, optional)

Implementation steps:

1. Replace fast recall's alphabetical first-N selection with lexical scoring.
2. Score memory entries using:
   - latest user words;
   - filename words;
   - `Entry.Name`;
   - `Entry.Description`;
   - `Entry.Type`.
3. Ignore stopwords and path-only tokens such as `docs`, `file`, `files`, `folder`, `folders` unless paired with semantically useful terms.
4. Add a minimum score threshold.
5. If no entry passes threshold:
   - do not read any memory file bodies;
   - do not include `MEMORY.md` index;
   - return the original system prompt.
6. Keep `llm` recall mode behavior unchanged except for respecting listing skip from Agent 4.

Tests to add/update:

- `TestFastRecallDoesNotSelectAlphabeticalMemoryForListing`.
- `TestFastRecallSelectsConventionsForConventionsQuestion`.
- `TestFastRecallSelectsPhaseMemoryForPhaseQuestion`.
- `TestFastRecallNoSelectionSkipsMemoryIndex`.

Acceptance:

- Simple filesystem listing does not pull project memories.
- Relevant project questions can still pull useful memories.
- `go test ./internal/memory` passes.

## Agent 7 Task Packet - Plain Tree Attachment Shape

Priority: P1
Dependency: Agent 2

Owned files:

- `internal/mentions/listing_prompt.go`
- `internal/mentions/expand.go`
- `internal/mentions/expand_test.go`

Implementation steps:

1. Ensure listing-scoped prompt tree data is plain text.
2. Ensure the tree renderer has stable sorting.
3. Represent directories with trailing `/`.
4. Represent skipped entries as:

```text
path/to/file [skipped: reason]
```

5. Remove duplicate directory entries where a root and child are both rendered redundantly.
6. Keep XML-like blocks only for content/default attachment policies if still needed.
7. Keep all counts in metadata, not in LLM prompt text.

Tests to add/update:

- `TestListingPromptPlainTreeNoXML`.
- `TestListingPromptIncludesNestedDirectories`.
- `TestListingPromptMarksSkippedEntries`.
- `TestContentModeStillUsesFileBlocks`.

Acceptance:

- Listing prompt tree data contains no XML tags.
- Listing prompt remains deterministic across runs.
- `go test ./internal/mentions` passes.

## Agent 8 Task Packet - Prompt Dump And Trace Policy Visibility

Priority: P1
Dependency: Agents 1-5

Owned files:

- `internal/agent/prompt_dump.go`
- `internal/commands/registry.go`
- `internal/observability/metrics.go`
- `internal/observability/agent.go`
- `internal/tui/app.go`
- `internal/commands/registry_test.go`
- `internal/observability/metrics_test.go`
- `USER_MANUAL.md`

Implementation steps:

1. Extend `PromptDump` with:

```go
Intent string
AttachmentPolicy string
HistoryPolicy string
MemoryPolicy string
RetryPolicy string
IncludedFileBodies int
DirectoryTreeAttached bool
```

2. Thread these fields from `agent.Input` and/or prompt pack report to `recordPromptDump()`.
3. Add `/prompt last` output lines:

```text
Intent:               directory_listing
Attachment policy:    listing_tree_only
History policy:       latest_only
Memory policy:        skipped_listing_intent
Retry policy:         anchored_original_request
File bodies:          0
Directory tree:       attached
```

4. Add trace fields where the observability model already supports run metadata.
5. Update manual docs with a debugging recipe:

```bash
NANDOCODEGO_PROMPT_DUMP=metadata go run ./cmd/nandocodego --model qwen3
```

Tests to add/update:

- `TestPromptLastShowsListingPolicies`.
- `TestPromptDumpRecordsListingPolicies`.
- `TestTraceRecordsListingPolicies` if trace fields are implemented.

Acceptance:

- A user can prove from `/prompt last` that no memory/history/file bodies were attached.
- `go test ./internal/commands ./internal/agent ./internal/observability` passes.

## Agent 9 Task Packet - End-To-End Drift Regression Tests

Priority: P1, but should run immediately after P0 implementation
Dependency: Agents 1-5

Owned files:

- `internal/tui/app_test.go`
- `internal/agent/agent_test.go`
- `internal/memory/runner_test.go`
- `docs/REGRESSION-AND-LOAD-TEST-PLAN.md`

Implementation steps:

1. Create a fake TUI session with prior stale messages:

```text
User: continue
Assistant: Now I have the full picture. Let me write the missing-implementation summary:
User: You stopped after a preamble. Provide the promised answer now.
Assistant: I will generate cmd/nandocodego/main.go and config code.
```

2. Submit `list all files and folders in @docs/`.
3. Capture the fake runner's `agent.Input`.
4. Assert exactly one LLM run is requested and it is a listing LLM request.
5. Assert the final request includes:
   - original listing request;
   - `Directory tree data:`;
   - known file path from the fake docs tree;
   - no file body blocks.
6. Assert the final request excludes:
   - `You stopped after a preamble`;
   - `promised answer`;
   - `cmd/nandocodego/main.go` unless present in the fake docs tree;
   - `=== DYNAMIC MEMORY CONTEXT ===`;
   - `Package Source Layout`;
   - `<file path=`.
7. Add a fake agent retry test where first model response is a preamble and second request is inspected.
8. Add non-listing controls:
   - `review @docs/` keeps content-cap behavior;
   - `list all files and folders in @docs?content` includes content and warning;
   - `analyze @docs/` still reaches project/content analysis path.
9. Update `docs/REGRESSION-AND-LOAD-TEST-PLAN.md` with the new drift regression suite.

Acceptance:

- The original supplied failure is represented as a regression test.
- Tests fail before the P0 implementation and pass after it.
- `go test ./internal/tui ./internal/agent ./internal/memory` passes.

## Recommended Agent Execution Order

1. Agent 1: first-class intent and attachment policy.
2. Agent 2: listing-scoped LLM user message.
3. Agent 3: latest-only history for listing LLM calls.
4. Agent 4: skip memory for listing LLM calls.
5. Agent 5: anchored retry prompt and listing retry guards.
6. Agent 9: drift regression suite.
7. Agent 6: relevance-gated fast memory recall.
8. Agent 7: simplified tree attachment shape.
9. Agent 8: prompt dump and trace visibility.

The first five are the P0 batch. The bug is not fixed until all five are complete, because the observed drift can come from any of these inputs: attached memory, old history, generic retry prompt, or ambiguous tree prompt shape.

## Exit Gate

Automated checks:

```bash
go test ./internal/mentions ./internal/tui ./internal/agent ./internal/memory ./internal/commands ./internal/observability
go test ./...
```

Manual live checks:

```text
list all files and folders in @docs/
list all the files in @docs/
list folders in @docs/
list all files and folders in @docs?content
review @docs/
analyze @docs/
```

Required live behavior for listing-scoped prompts:

- LLM is still called.
- Prompt contains the original listing request.
- Prompt contains directory tree data.
- Prompt does not contain file bodies.
- Prompt does not contain `=== DYNAMIC MEMORY CONTEXT ===`.
- Prompt does not contain `You stopped after a preamble`.
- Prompt does not contain `promised answer`.
- Prompt does not contain package-layout/code-generation memory unless that text is an actual listed filename.
- Retry, if triggered, preserves the original listing request and tree data.
- `/prompt last` shows `intent=directory_listing`, `attachment_policy=listing_tree_only`, `history_policy=latest_only`, and `memory_policy=skipped_listing_intent`.

Required live behavior for non-listing prompts:

- `review @docs/` and `analyze @docs/` still receive enough content/context for deep analysis.
- Dynamic memory remains available when relevant.
- Conversation history remains available for normal chat and explicit continuation.
- Incomplete-response recovery still works for genuine non-listing incomplete preambles.

## Non-Goals

- Do not bypass the LLM for directory listing.
- Do not reintroduce hard answer constraints such as `return only`, `exactly the tree`, `no greetings`, or `perfect`.
- Do not disable memory globally.
- Do not remove history globally.
- Do not weaken large project analysis/review flows.
- Do not make `?content` listing behave like tree-only listing.

## Final Target State

For a pure directory/folder/project listing request, the application should still ask the LLM, but the final LLM request must be narrow and faithful:

1. Latest user listing request.
2. Plain directory tree data.
3. No unrelated memory.
4. No stale history.
5. No file bodies unless explicitly requested.
6. Retry anchored to the same original request and same tree data.
