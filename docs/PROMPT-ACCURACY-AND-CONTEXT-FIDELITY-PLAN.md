# Prompt Accuracy And Context Fidelity Plan

Date: 2026-05-17
Status: Ready for implementation

## Objective

Make every LLM request explainable and faithful to the user's prompt. When a user references files or directories, the model should receive the intended context, the TUI should clearly show what was included or omitted, and developers should be able to inspect the final request sent to the LLM when responses look out of context.

This plan addresses the observed failure:

```text
User: name the all the files and folders in @docs

TUI: expanded 1 directories, 7 files, 200.2 KiB

Assistant answers about a truncated Phase 3 document instead of listing docs/
```

Root cause:

- `docs/` contains about 51 files locally.
- `docs/` is ignored by `.gitignore`.
- Directory expansion uses `git ls-files --cached --others --exclude-standard` before filesystem walking.
- Because `docs/` is ignored, only 7 tracked/cached files were expanded.
- The model received file bodies from those 7 files, not the full directory listing the user intended.
- The TUI did not warn that most files were omitted by gitignore.
- There is no `/prompt last` command to inspect the final `ChatRequest.Messages`.

## Desired Behavior

For explicit user context like `@docs`, the app must be transparent:

- The user should know exactly how many files/folders were discovered, included, skipped, ignored, and truncated.
- Explicit `@directory` mentions should not silently omit ignored files without a visible warning.
- Listing-style prompts should prioritize directory tree context over file body context.
- The final prompt sent to the LLM should be inspectable in a safe, opt-in way.
- If prompt packing trims messages or context, the user should see that clearly.

## Accuracy Contract

The main goal of this plan is prompt accuracy: the LLM must receive context that matches the user's actual request, and the TUI must reveal any gap between what the user asked for and what was sent.

These invariants are non-negotiable:

- The latest user instruction must be preserved verbatim in the final `ChatRequest.Messages`.
- Explicit mentions must either include the requested context or produce a visible omission warning with a machine-readable reason.
- Directory listing prompts must not attach large file bodies by default. They should send a directory tree unless the user explicitly asks for content analysis.
- Context packing must never silently remove the latest user request, system instruction, or all mention context without reporting it.
- Git ignore behavior must not silently redefine an explicit user request. If the user asks for `@docs`, the app must not pretend 7 files is the full directory when 51 readable files exist locally.
- Every omitted file or directory must map to a reason category such as `gitignored`, `binary`, `too-large`, `prompt-byte-cap`, `prompt-file-cap`, `permission`, `depth-cap`, or `hard-exclude`.
- Prompt dumps must describe the exact final request boundary, not an earlier intermediate prompt.
- Full prompt content persistence must remain opt-in because prompt bodies can contain private code, secrets, or user data.
- The exact regression prompt `name the all the files and folders in @docs` must produce a directory listing response, not a summary of one document body.

If any implementation choice conflicts with these invariants, the invariant wins.

## Implementation Order

Implement in this order so user-visible prompt accuracy improves before lower-priority diagnostics:

1. PA-1: Add directory expansion diagnostics.
2. PA-3: Add tree-only/list-intent prompt mode.
3. PA-2: Fix explicit `@directory` semantics for gitignored directories.
4. PA-0: Add prompt dump metadata and `/prompt last` inspection.
5. PA-4: Improve prompt packing reports.
6. PA-5: Add regression tests for the exact failure and all prompt accuracy edges.
7. PA-6: Update user docs and troubleshooting.
8. PA-7: Run integration verification against the complete TUI-to-LLM request path.

PA-0 can be developed in parallel with PA-1 to PA-3 if there are multiple agents, but it must be integrated after the final request shape and pack report metadata are stable.

## Agent Implementation Contract

Each agent must make a narrow, verifiable change and leave the repository in a testable state. Do not mix unrelated roadmap work into these tasks.

| Slice | Primary Goal | Main Files | Must Prove |
| --- | --- | --- | --- |
| PA-1 | Users can see what directory expansion discovered, included, and omitted. | `internal/tools/dirwalk/walk.go`, `internal/mentions/expand.go`, `internal/tui/app.go` | `@docs`-style ignored directories show discovered/included/ignored counts. |
| PA-3 | Listing prompts send tree context, not file bodies. | `internal/mentions/expand.go`, `internal/tui/app.go` | `name the all the files and folders in @docs` expands as tree-only. |
| PA-2 | Explicit `@directory` respects user intent even when Git ignores the directory. | `internal/tools/dirwalk/walk.go`, `internal/mentions/expand.go`, config/state files | `@docs` can include readable ignored docs files subject to caps and hard excludes. |
| PA-0 | Developers can inspect the exact final prompt safely. | `internal/agent/stream.go`, new prompt dump package, command/TUI files | `/prompt last` shows final request metadata and optional full content. |
| PA-4 | Prompt packing cannot hide dropped context. | `internal/agent/prompt_packer.go`, event/TUI/command files | Dropped mention blocks and forced latest-user inclusion are reported. |
| PA-5 | Regressions are caught automatically. | Tests in mention, dirwalk, agent, TUI packages | The exact failure and related variants are covered. |
| PA-6 | Users know how to inspect and control prompt context. | `USER_MANUAL.md`, status docs | Mention modes, prompt dump, and omission warnings are documented. |
| PA-7 | The full TUI-to-LLM path is verified. | TUI, mentions, agent, prompt packer | The original failure no longer reproduces. |

Agent handoff format after each slice:

```text
Implemented:
- ...

Files changed:
- ...

Tests run:
- ...

Prompt accuracy behavior proven:
- ...

Known remaining gaps:
- ...
```

Do not mark a slice complete unless the acceptance checks for that slice pass or the remaining blocker is documented with a concrete next task.


## 2026-05-17 Review Improvements

The first version of this plan identified the right failure but needed tighter implementation boundaries. The following decisions are now part of the plan:

- Prompt dump settings should follow the repo's existing flat config model, not a new nested `[debug]` table, unless a larger config refactor is intentionally done.
- `/prompt last` should work in the current session even when file persistence is disabled. Metadata can be stored in memory safely; full content persistence remains opt-in.
- The final prompt dump must be captured after prompt packing, at the final `llm.ChatRequest` boundary, but pack metadata is produced earlier in `agent.Run`. The implementation must explicitly pass/record the latest pack report so `/prompt last` can correlate both.
- Tree-only listing behavior should land before changing ignored-file semantics. It fixes the observed "model latched onto file body" failure even when a directory is large.
- `@docs?tree`, `@docs?content`, and `@docs?all` require parser changes because current mention tokens stop only on whitespace and `NormalizeMentionPath` trims `?` as punctuation. Do not implement suffix modes as ad hoc string trimming.
- Explicit filesystem fallback must preserve hard excludes and caps. It must never turn `@.` into an accidental full-home or dependency-cache dump.
- The TUI summary must distinguish three counts: filesystem-discovered, prompt-eligible, and actually-included.

---

## PA-0 - Prompt Dump Infrastructure

### Goal

Capture the exact final request sent to the LLM, after mention expansion, memory, system prompt injection, compaction, and prompt packing.

### Files

- `internal/agent/stream.go`
- `internal/analysis` or new package `internal/promptlog`
- `internal/commands/registry.go`
- `internal/tui/app.go`
- `USER_MANUAL.md`

### Tasks

- [ ] Add a `PromptDump` struct:

```go
type PromptDump struct {
    CreatedAt         time.Time        `json:"created_at"`
    Model             string           `json:"model"`
    Options           map[string]any   `json:"options"`
    MessageCount      int              `json:"message_count"`
    ToolCount         int              `json:"tool_count"`
    ToolNames         []string         `json:"tool_names"`
    Messages          []PromptMessage  `json:"messages"`
    EstimatedTokens   int              `json:"estimated_tokens"`
    PromptPackReport  *PromptPackMeta  `json:"prompt_pack_report,omitempty"`
}

type PromptMessage struct {
    Index          int    `json:"index"`
    Role           string `json:"role"`
    Bytes          int    `json:"bytes"`
    EstimatedTokens int   `json:"estimated_tokens"`
    Content        string `json:"content,omitempty"`
    ContentPreview string `json:"content_preview,omitempty"`
}
```

- [ ] Store prompt dumps under:

```text
<state-dir>/prompt-dumps/latest.json
<state-dir>/prompt-dumps/<timestamp>-<short-id>.json
```

- [ ] Default to metadata + previews only.
- [ ] Require explicit config/env to store full content:

```toml
prompt_dump_mode = "off"   # off | metadata | full
prompt_dump_keep = 10
prompt_preview_chars = 600
```

or:

```bash
NANDOCODEGO_PROMPT_DUMP=metadata
NANDOCODEGO_PROMPT_DUMP=full
```

- [ ] Add flat config fields:

```go
PromptDumpMode   string `koanf:"prompt_dump_mode"`
PromptDumpKeep   int    `koanf:"prompt_dump_keep"`
PromptPreviewChars int  `koanf:"prompt_preview_chars"`
```

- [ ] Never dump API keys, environment variables, HTTP headers, or permission secrets.
- [ ] Keep an in-memory `LatestPromptDump` metadata snapshot for `/prompt last` even when persistence is off.
- [ ] Add `/prompt last` command to show metadata and previews.
- [ ] Add `/prompt save last` command to print the path of the latest dump.
- [ ] Add `/prompt show last full` only when full dumps are enabled.
- [ ] Add an agent-level prompt observer or prompt recorder so `agent.Run` can attach the latest `PromptPackReport` to the dump created in `executeOneTurn`.
- [ ] Include tool names but not full tool JSON schemas by default. Full schema dump is optional and controlled by full mode.

### Acceptance

- User can run `/prompt last` after any request and see final message roles, bytes, token estimates, tool count, `num_ctx`, `num_predict`, and previews.
- With full dump enabled, the exact final `ChatRequest.Messages` content is persisted.
- With dump disabled, no prompt body is written, but in-memory metadata for the latest prompt remains visible during the current session.
- Tests verify full content is not persisted unless explicitly enabled.
- Tests verify prompt pack metadata is attached to the same final request dump that was sent to the LLM.

### Agent Prompt

```text
Implement PA-0 from docs/PROMPT-ACCURACY-AND-CONTEXT-FIDELITY-PLAN.md.
Add prompt dump infrastructure at the final LLM request point in
internal/agent/stream.go. Use flat config fields prompt_dump_mode,
prompt_dump_keep, and prompt_preview_chars plus NANDOCODEGO_PROMPT_DUMP override.
Keep latest prompt metadata in memory even when persistence is off. Persist full
message content only in full mode. Add /prompt last and /prompt save last
commands. Do not log prompt content through normal logs. Add tests for disabled,
metadata, and full modes, and verify PromptPackReport metadata is attached.
Run go test ./internal/agent ./internal/commands ./internal/tui.
```

---

## PA-1 - Directory Expansion Diagnostics

### Goal

Make `@directory` expansion explain what was discovered and what was omitted.

### Files

- `internal/tools/dirwalk/walk.go`
- `internal/mentions/expand.go`
- `internal/tui/app.go`
- `internal/mentions/expand_test.go`

### Tasks

- [ ] Extend `dirwalk.Stats`:

```go
type Stats struct {
    FileCount       int
    DirCount        int
    ByteCount       int64
    Truncated       bool
    Reason          string
    Source          string // git | filesystem
    IgnoredByGit    int
    TotalFilesystemFiles int
    TotalFilesystemDirs  int
}
```

- [ ] When git walk is used, compare against a bounded filesystem walk for explicit directory mentions to detect ignored files.
- [ ] Comparison walk must count only readable text-candidate files after hard excludes, not binary files or dependency/cache directories.
- [ ] Avoid double-reading all file contents during diagnostics. Count file paths and stat sizes first; content reads remain in expansion.
- [ ] Add skipped reasons:
  - `gitignored`
  - `prompt-file-cap`
  - `prompt-byte-cap`
  - `dir-file-cap`
  - `dir-byte-cap`
  - `binary`
  - `too-large`
  - `permission`
  - `depth-cap`
- [ ] Extend `ResolvedDirectory` with:

```go
DiscoveredFiles int
DiscoveredDirs  int
IncludedFiles   int
IncludedDirs    int
SkippedCount    int
IgnoredByGit    int
ExpansionSource string
OmittedReasons  map[string]int
```

- [ ] Update TUI expansion summary:

```text
[Expanded @docs: 7 files included, 51 discovered, 44 ignored by gitignore, 200.2 KiB]
```

- [ ] If omitted files exist, append a warning:

```text
[Warning: @docs omitted 44 gitignored files. Use @docs?all or /context mentions all to include ignored files.]
```

### Acceptance

- The exact `@docs` failure shape shows a warning instead of silently saying only "7 files".
- Tests create an ignored directory with tracked and ignored files and assert diagnostics.
- Directory expansion summary is deterministic and concise.
- Diagnostics do not require reading every file body twice.

### Agent Prompt

```text
Implement PA-1 from docs/PROMPT-ACCURACY-AND-CONTEXT-FIDELITY-PLAN.md.
Extend dirwalk and mention expansion diagnostics so explicit @directory mentions
report discovered, included, skipped, ignored-by-git counts, and expansion
source. Update the TUI expansion summary. Add tests with a temporary git repo
where a directory is gitignored but explicitly mentioned.
Run go test ./internal/tools/dirwalk ./internal/mentions ./internal/tui.
```

---

## PA-2 - Explicit Directory Mention Semantics

### Goal

Explicit `@directory` mentions should mean "include the directory I asked for," not "include only files Git would list."

### Files

- `internal/tools/dirwalk/walk.go`
- `internal/mentions/expand.go`
- `internal/config/config.go`
- `internal/config/defaults.go`
- `internal/bootstrap/state.go`
- `internal/state/app.go`
- `USER_MANUAL.md`

### Design

Add mention expansion policy using flat config keys:

```toml
mention_directory_source = "auto" # auto | git | filesystem
mention_include_gitignored_on_explicit = true
```

Policy behavior:

- `auto`: use git for normal indexed/project discovery, but use filesystem walk for explicit `@directory` when git would omit files due to ignore rules.
- `git`: current behavior, but with warnings.
- `filesystem`: always use filesystem walk for explicit directory mentions.

### Tasks

- [ ] Add config fields and state propagation.
- [ ] Add `dirwalk.Options.Source`:

```go
type SourceMode string
const (
    SourceAuto SourceMode = "auto"
    SourceGit SourceMode = "git"
    SourceFilesystem SourceMode = "filesystem"
)
```

- [ ] In `mentions.ExpandPrompt`, pass an explicit-mention option that can override git ignore behavior.
- [ ] Preserve default excludes like `.git`, `node_modules`, `vendor`, `dist`, build outputs, caches.
- [ ] Treat default excludes as hard excludes even in `?all` mode unless a future explicit unsafe mode is added.
- [ ] Never include binary files.
- [ ] Keep prompt caps enforced.
- [ ] Emit warning when switching from git to filesystem due to explicit ignored directory.
- [ ] For large ignored directories, include the tree and cap content reads; do not block the TUI on unbounded filesystem walking.

### Acceptance

- `@docs` includes all readable text files under `docs/` up to configured caps, even though `docs/` is gitignored.
- `.git`, `node_modules`, and other hard excludes still do not expand.
- Config can force old git-only behavior.
- Tests cover auto, git, and filesystem modes.

### Agent Prompt

```text
Implement PA-2 from docs/PROMPT-ACCURACY-AND-CONTEXT-FIDELITY-PLAN.md.
Add directory expansion source policy and make explicit @directory mentions use
filesystem fallback when git ignore rules would omit requested files. Preserve
hard excludes and prompt caps. Add config/state propagation and tests for auto,
git, and filesystem modes.
Run go test ./internal/config ./internal/bootstrap ./internal/state ./internal/tools/dirwalk ./internal/mentions.
```

---

## PA-3 - Tree-Only Mode And Intent-Aware Directory Expansion

### Goal

For prompts that ask to list files/folders, send the tree, not massive file bodies.

### Files

- `internal/mentions/expand.go`
- `internal/tui/app.go`
- `internal/mentions/expand_test.go`
- `USER_MANUAL.md`

### Design

Add explicit suffixes. These suffixes are part of mention syntax, not path names:

```text
@docs?tree      # tree only
@docs?content   # tree + file contents, current behavior
@docs?all       # include gitignored readable files, subject to caps
```

Suffix semantics:

- `?tree`: force tree-only context for that directory. It uses the configured explicit mention source policy, so by default it should include readable gitignored files for explicit directories while still respecting hard excludes and caps.
- `?content`: force tree plus bounded file bodies for that directory. It does not disable ignore diagnostics or hard excludes.
- `?all`: force filesystem source plus tree and bounded file bodies for that directory. It still excludes `.git`, dependency directories, caches, binary files, unreadable files, and files beyond caps.
- Natural-language listing intent has lower precedence than explicit suffixes. For example, `review @docs?tree` is tree-only because the suffix is explicit.
- Safety caps have higher precedence than all suffixes. No suffix may bypass hard excludes, file count caps, byte caps, depth caps, or binary filtering.

Add conservative intent detection for natural-language listing prompts:

Trigger tree-only default when prompt contains:

- `list files`
- `list folders`
- `name the files`
- `name all files`
- `show tree`
- `directory tree`
- `what files are in`

And all mentions are directories.

Do not trigger tree-only if prompt asks to:

- summarize
- review
- analyze implementation
- inspect contents
- find bugs
- compare docs

### Tasks

- [ ] Extend mention token parsing to support query suffixes.
- [ ] Add a parsed mention type:

```go
type MentionMode string
const (
    MentionModeAuto MentionMode = "auto"
    MentionModeTree MentionMode = "tree"
    MentionModeContent MentionMode = "content"
    MentionModeAll MentionMode = "all"
)

type ParsedMention struct {
    Raw  string
    Path string
    Mode MentionMode
}
```

- [ ] `NormalizeMentionPath` should no longer silently discard `?tree`/`?all`; parsing must split suffix first, then normalize path.
- [ ] If a real path contains `?`, require escaping or leave it unsupported with a clear error. Do not guess.
- [ ] Add `MentionModeTree`, `MentionModeContent`, `MentionModeAll`.
- [ ] `@docs?tree` renders:

```xml
<directory path="docs" mode="tree" files="51" dirs="1" truncated="false">
<tree>
docs/
PHASE-1-DETAILED-PLAN.md
...
</tree>
</directory>
```

- [ ] No `<file>` content blocks in tree mode.
- [ ] Add TUI summary:

```text
[Expanded @docs as tree: 51 files, 1 folders]
```

- [ ] Add tests for explicit `?tree`, explicit `?content`, `?all`, and listing-intent auto tree mode.
- [ ] Add tests for negative listing detection: `review @docs`, `analyze @docs`, and `find bugs in @docs` must not become tree-only.
- [ ] Add tests for mixed mentions: if prompt has one directory and one file, listing-intent tree mode applies only to directory mentions; explicit file mentions remain file blocks.
- [ ] Add tests for suffix precedence: explicit `?tree` beats analysis intent, explicit `?content` beats listing intent, and caps beat all suffixes.

### Acceptance

- Prompt `name the all the files and folders in @docs` and the corrected prompt `name all files and folders in @docs` both send only tree context by default.
- Model cannot latch onto file body content because none is sent.
- Prompt `review @docs` still sends bounded file content unless caps require truncation.
- User can override with `@docs?content`.
- User can override ignored-file behavior with `@docs?all`.
- TUI summaries clearly say whether the expansion mode was `tree`, `content`, or `all`.

### Agent Prompt

```text
Implement PA-3 from docs/PROMPT-ACCURACY-AND-CONTEXT-FIDELITY-PLAN.md.
Add mention suffix modes ?tree, ?content, ?all and conservative listing-intent
detection. Directory listing prompts should expand as tree-only context. Keep
content mode available for review/analysis prompts. Add tests for suffix parsing,
tree-only rendering, and the exact prompt "name the all the files and folders in
@docs".
Run go test ./internal/mentions ./internal/tui.
```

---

## PA-4 - Prompt Packing Transparency

### Goal

When context is trimmed, users must know what happened.

### Files

- `internal/agent/prompt_packer.go`
- `internal/agent/events.go`
- `internal/tui/app.go`
- `internal/commands/registry.go`

### Tasks

- [ ] Extend `PromptPackReport`:

```go
type PromptPackReport struct {
    InputBudgetTokens int
    EstimatedIncluded int
    EstimatedSkipped  int
    IncludedMessages  int
    SkippedMessages   int
    ForcedIncludeLast bool
    DroppedRoles      map[string]int
    DroppedBytes      int
    DroppedMentionBlocks int
    IncludedMentionBlocks int
    LastUserMessageIncluded bool
    SystemMessageIncluded bool
}
```

- [ ] Treat mention blocks as structured prompt sections by detecting `<file ` and `<directory ` tags in message content.
- [ ] Keep the current invariant that the latest user message is forced in; report if it exceeds budget by itself.
- [ ] Add a warning when the latest user message is larger than `inputBudgetTokens` and is force-included anyway.
- [ ] TUI notice should say:

```text
[Prompt packed: kept 4/12 messages, skipped ~18k tokens, dropped 2 mention blocks]
```

- [ ] `/trace last` should include prompt packing details.
- [ ] Prompt dump should include the latest pack report.

### Acceptance

- If directory context is dropped, the TUI says so.
- Tests verify prompt pack reports mention block drops.
- Tests verify the latest user message is always included and reported as forced when needed.

### Agent Prompt

```text
Implement PA-4 from docs/PROMPT-ACCURACY-AND-CONTEXT-FIDELITY-PLAN.md.
Extend PromptPackReport with dropped role/byte/mention-block metadata and surface
it in TUI notices, /trace last, and prompt dumps. Add tests for history containing
directory/file blocks that get packed out.
Run go test ./internal/agent ./internal/commands ./internal/tui.
```

---

## PA-5 - Prompt Accuracy Regression Tests

### Goal

Prevent regressions where the model receives the wrong context for explicit mentions.

### Files

- `internal/mentions/expand_test.go`
- `internal/tui/app_test.go`
- `internal/agent/prompt_packer_test.go`
- `internal/tools/dirwalk/walk_test.go`

### Test Cases

- [ ] `@docs` in gitignored directory includes explicit files or warns in git-only mode.
- [ ] `name all files and folders in @docs` produces tree-only expansion.
- [ ] `name the all the files and folders in @docs` produces tree-only expansion.
- [ ] `review @docs` produces content expansion.
- [ ] `summarize @docs` produces content expansion.
- [ ] `what files are in @docs` produces tree-only expansion.
- [ ] `@docs?tree` never includes `<file>` bodies.
- [ ] `@docs?content` includes bounded `<file>` bodies.
- [ ] `@docs?all` includes gitignored readable text files subject to caps.
- [ ] `review @docs?tree` remains tree-only because explicit suffixes beat intent detection.
- [ ] `name all files in @docs?content` remains content mode because explicit suffixes beat intent detection.
- [ ] Prompt packing reports dropped mention blocks.
- [ ] `/prompt last` shows final message metadata.
- [ ] Full prompt dump is disabled by default.

### Prompt Accuracy Evals

Add lightweight eval fixtures that assert final prompt shape, not model prose quality. These can be regular Go tests that inspect expanded prompt messages before a network LLM call.

| User Prompt | Required Final Prompt Shape | Forbidden Shape |
| --- | --- | --- |
| `name the all the files and folders in @docs` | latest user text preserved; one `<directory ... mode="tree">`; `<tree>` contains local readable docs paths | `<file path=...>` bodies from docs |
| `review @docs` | latest user text preserved; directory metadata plus bounded `<file>` bodies | silent omission of gitignored docs files |
| `name all files in @docs?content` | latest user text preserved; `mode="content"`; bounded `<file>` bodies present | auto tree-only override |
| `review @docs?tree` | latest user text preserved; `mode="tree"` only | file bodies included |
| `summarize @docs` with tiny prompt budget | latest user text preserved; prompt pack report says mention context dropped or truncated | dropped context with no report |

These evals should fail if the final prompt no longer matches the user's requested operation.

### Agent Prompt

```text
Implement PA-5 from docs/PROMPT-ACCURACY-AND-CONTEXT-FIDELITY-PLAN.md.
Add regression tests for explicit directory mentions, tree-only listing prompts,
gitignored directories, prompt packing visibility, and prompt dump behavior.
Use temporary repos and temporary state/cache dirs. Run go test ./internal/...
for affected packages and then go test ./...
```

---

## PA-6 - User Documentation And Troubleshooting

### Goal

Make prompt/context behavior understandable to users.

### Files

- `USER_MANUAL.md`
- `docs/PHASE-LOG.md`
- `docs/PROJECT-STATUS-AND-ONBOARDING.md`

### Tasks

- [ ] Document mention modes:

```text
@path
@dir?tree
@dir?content
@dir?all
```

- [ ] Document `/prompt last`.
- [ ] Document what `expanded N directories, X files` means.
- [ ] Add troubleshooting section:

```text
Model answered from the wrong file or ignored my directory listing.
```

- [ ] Explain gitignored directory behavior and explicit mention policy.
- [ ] Update phase/status docs with files changed and tests run.

### Agent Prompt

```text
Implement PA-6 from docs/PROMPT-ACCURACY-AND-CONTEXT-FIDELITY-PLAN.md.
Update USER_MANUAL.md and project status docs to explain prompt inspection,
mention modes, gitignored directory behavior, tree-only listing prompts, and
how to diagnose out-of-context answers. Keep examples aligned with implementation.
```

---

## PA-7 - Integration Verification

### Goal

Verify the full ask-and-response path from TUI prompt entry to final LLM request construction. This slice should not add new features unless a verification failure exposes a missing integration step.

### Files

- `internal/tui/app.go`
- `internal/mentions/expand.go`
- `internal/agent/agent.go`
- `internal/agent/stream.go`
- `internal/agent/prompt_packer.go`
- `docs/PROMPT-ACCURACY-AND-CONTEXT-FIDELITY-PLAN.md`

### Tasks

- [ ] Add or update an integration test that simulates a TUI prompt containing `@docs` and captures the final `llm.ChatRequest`.
- [ ] Verify the final request contains the latest user prompt verbatim.
- [ ] Verify mention expansion happens before prompt packing, and prompt dump metadata is captured after prompt packing.
- [ ] Verify TUI notices mention expansion mode, included/discovered counts, and omitted reasons.
- [ ] Verify prompt pack notices and prompt dump metadata refer to the same request.
- [ ] Run a manual REPL test for:

```text
name the all the files and folders in @docs
```

- [ ] Confirm the assistant response is a directory listing, not a summary of a phase document.
- [ ] Record the manual test result in `docs/PHASE-LOG.md` or the relevant status document.

### Acceptance

- The original failure cannot be reproduced in normal config.
- The original failure is explainable in git-only compatibility config because the warning shows omitted files.
- The prompt dump, TUI notices, and prompt pack report agree on what was sent.

### Agent Prompt

```text
Implement PA-7 from docs/PROMPT-ACCURACY-AND-CONTEXT-FIDELITY-PLAN.md.
Add end-to-end verification for the prompt path from TUI mention expansion to
final llm.ChatRequest construction. Prove that the exact prompt "name the all
the files and folders in @docs" preserves the user request, sends directory tree
context, reports omitted context, and no longer sends unrelated file bodies by
default. Update the phase/status log with the verification result.
Run go test ./internal/tui ./internal/mentions ./internal/agent and any broader
tests needed by touched packages.
```

---

## Final Exit Gate

The plan is complete only when:

- [ ] `/prompt last` shows final request metadata after a run.
- [ ] Full prompt dump can be enabled explicitly and is disabled by default.
- [ ] The latest user prompt is preserved verbatim in every final request, including typo-heavy prompts.
- [ ] `name all files and folders in @docs` expands as tree-only and includes all readable docs paths up to caps.
- [ ] `name the all the files and folders in @docs` expands as tree-only and includes all readable docs paths up to caps.
- [ ] `review @docs` still includes bounded content.
- [ ] Explicit gitignored directory mentions do not silently omit most files.
- [ ] TUI warns when anything is omitted by gitignore, caps, binary detection, permissions, or depth.
- [ ] Prompt packing visibly reports dropped messages/context.
- [ ] Prompt dump metadata, TUI notices, and prompt pack reports agree on the same final LLM request.
- [ ] `go test ./...` passes.
- [ ] Manual REPL test confirms the observed failure no longer reproduces.
