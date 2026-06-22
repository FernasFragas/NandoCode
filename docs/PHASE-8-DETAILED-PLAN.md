# Phase 8 Detailed Plan - Memory

Date: 2026-05-03
Status: Final plan and implementation checklist
Source plans:

- `.codex/go-ollama-plan-AGENTS.md`
- `.codex/go-ollama-plan-HUMANS.md`
- `docs/PHASE-LOG.md`
- `docs/PROJECT-STATUS-AND-ONBOARDING.md`
- `docs/PHASE-1-DETAILED-PLAN.md`
- `docs/PHASE-3-DETAILED-PLAN.md`
- `docs/PHASE-4-DETAILED-PLAN.md`
- `docs/PHASE-5-DETAILED-PLAN.md`
- `docs/PHASE-6-DETAILED-PLAN.md`
- `docs/PHASE-7-DETAILED-PLAN.md`
- `docs/REPL-OLLAMA-STREAMING-FIX.md`
- `book/ch01-architecture.md`
- `book/ch03-state.md`
- `book/ch04-api-layer.md`
- `book/ch05-agent-loop.md`
- `book/ch06-tools.md`
- `book/ch11-memory.md`
- `.codex/agent-context/learnings-memory.md`

## Goal

Phase 8 implements file-based, human-editable memory for the REPL.

The user-visible goal is that the agent can remember durable preferences and project facts across sessions without a database, vector store, cloud service, or hidden state. A user should be able to tell the agent a preference in one session, start a new session later, and see that preference influence the answer.

Deliverables:

- `internal/memory` package for paths, metadata scanning, index loading, staleness handling, prompt-section construction, LLM-based recall, and pending extraction drafts.
- Per-project memory directory under `paths.MemoryDir(scopeRoot)`.
- Always-loaded `MEMORY.md` index with hard caps.
- Four memory types only: `user`, `feedback`, `project`, `reference`.
- LLM side-query recall using the existing Ollama `llm.Client`, not embeddings.
- Filename validation so recall cannot load hallucinated files.
- Memory prompt injection behind a clearly marked dynamic section.
- Explicit write instructions so the main agent can save memory via existing file tools.
- Background extraction safety net that writes review drafts to `pending/`, not directly into active memory.
- Minimal CLI/REPL wiring so memory works in normal interactive sessions.
- Focused unit, benchmark, and fake-client integration tests.
- Phase log update after implementation.

## Definition Of Success

The Phase 8 exit gate is a two-session manual flow:

1. Session 1:
   - Run the REPL.
   - Tell the agent: "Remember that I prefer table-driven tests in Go."
   - Confirm it writes or proposes a memory file under the project memory directory.
2. Session 2:
   - Start a fresh REPL in the same project.
   - Ask it to write or describe a Go test.
   - Verify it naturally uses or recommends table-driven style without being told again.

This exit gate must work with the default configured Ollama endpoint and without any network destination other than Ollama.

## Baseline Analysis From Implemented Phases

### Phase 0 - Security and Supply Chain

Implemented:

- `SECURITY.md`
- dependency allowlist
- network policy checker
- CI/security baseline
- no-secrets policy for logs, memory, telemetry, and test fixtures

Phase 8 implications:

- Memory files are local but still a security boundary. They can contain user prompts, model outputs, project details, and accidentally pasted secrets.
- Memory content must never be logged at INFO.
- Memory content must not be treated as executable instruction. It is user-editable context and may be stale or malicious.
- Phase 8 must not add any network destination. Recall and extraction may call only the configured `llm.Client`, which currently means the configured Ollama endpoint.
- Any new direct dependency must already be allowlisted or must update `tools/allowed-deps.txt` and `docs/PHASE-LOG.md` with justification.
- Memory path overrides from committable project settings are out of scope and must not be introduced before the config-source model exists.

### Phase 1 - CLI, Paths, Logging, Scaffold

Implemented:

- `cmd/nandocodego`
- Cobra root command
- `internal/paths`
- `internal/logging`
- `paths.MemoryDir(gitRoot string)`
- empty future package directories, including memory

Phase 8 implications:

- Reuse `internal/paths.MemoryDir`; do not duplicate XDG/home resolution.
- Add memory path resolution on top of the existing path helper because `bootstrap.GitRoot` is currently empty in normal REPL startup.
- Keep logging setup as-is. If memory needs diagnostics, log counts, durations, and redacted filenames only.
- `doctor` remains network-free. Memory health checks in `doctor` can be deferred unless they require only local filesystem inspection.

### Phase 2 - LLM Client

Implemented:

- provider-neutral `llm.Client`
- streaming `Chat`
- `ChatRequest.Format` for structured outputs
- model list/pull/embed APIs
- retry/watchdog helpers

Phase 8 implications:

- Recall and extraction should use `llm.Client`, not a new Ollama-specific package.
- There is no `ChatOnce` helper. Phase 8 should add a small internal helper in `internal/memory` that calls `Chat` with `Stream: false` and consumes the returned event channel into one response string.
- Structured output should be requested through `ChatRequest.Format` with a JSON schema.
- Recall failures should be non-fatal. The main prompt should continue with only `MEMORY.md` if recall times out or returns invalid JSON.

### Phase 3 - Tool Interface And Starter Tools

Implemented:

- `tools.Tool`
- `tools.Context`
- `Bash`, `FileRead`, and `FileWrite`
- path safety helpers
- built-in registry

Phase 8 implications:

- The memory write path should use existing tools where practical. The main agent can create memory files and update `MEMORY.md` with `FileRead` plus `FileWrite`.
- There is no FileEdit tool yet. Updating an existing memory means read the full file, edit in-model, then write the full replacement through `FileWrite`.
- Memory directory paths are normally outside the project working directory. The REPL must add the memory directory to `ToolSettings.AdditionalWorkingDirs` or build a tools context that includes it, otherwise `FileWrite` will correctly reject memory writes.
- Permission checks still apply. Phase 8 should not silently grant writes to memory files just because they are under the app data directory.

### Phase 4 - Agent Loop

Implemented:

- `agent.Run(ctx, input) <-chan agent.Event`
- `agent.Input.SystemPrompt`
- assistant/thinking deltas
- tool execution loop
- terminal events and usage

Phase 8 implications:

- Memory prompt injection should use `agent.Input.SystemPrompt`.
- The current agent emits streaming deltas but does not emit a durable "these messages were added to the conversation history" event. Phase 8 needs to add a small conversation persistence event or terminal field so the TUI can append assistant/tool messages back into `state.App.Messages`.
- Without that fix, Phase 8 extraction and later turns would see only user messages, which is not enough for memory-quality decisions.
- Memory recall should run before each agent run, using the latest user message and the current message history.

### Phase 5 - Permission System

Implemented:

- canonical permission modes
- source-tagged rules
- central resolver
- TUI prompt callback extension point
- fail-closed prompt behavior

Phase 8 implications:

- Memory file writes through `FileWrite` must still route through permissions.
- Memory instructions should tell the model to ask or proceed through normal tool calls, not bypass permission policy.
- Background extraction that writes only to `pending/` is an internal process, but it still must validate paths and never write outside the memory directory.
- Future config-backed memory rules belong to Phase 13. Phase 8 should not add a project-config override that can redirect memory writes.

### Phase 6 - State Layer

Implemented:

- `bootstrap.State`
- `state.Store[state.App]`
- `state.App.Messages`
- `state.App.ToolSettings`
- `state.App.ToolContext(ctx)`
- `state.OnChange`

Phase 8 implications:

- Add only small state fields if needed. Do not put full memory contents in bootstrap.
- Bootstrap may hold coarse memory settings in the future, but Phase 8 can wire memory through CLI options/defaults without full config.
- `state.App.ToolSettings.AdditionalWorkingDirs` is the right app-state surface to include the memory directory for tool access.
- Keep memory contents out of `state.OnChange`.
- Continue copy-on-write discipline when mutating slices/maps.

### Phase 7 - Bubble Tea TUI And REPL

Implemented:

- real no-args REPL
- prompt submission
- agent bridge
- permission modal
- transcript rendering
- minimal slash commands
- `--model`, `--ollama-url`, `--no-alt-screen`

Post-Phase-7 fixes already made:

- Submitted user message is now passed to the agent input for the current run.
- Assistant/thinking deltas now start a new transcript item after a new user prompt.
- Viewport refreshes and follows the bottom after transcript changes.

Phase 8 implications:

- Do not run memory recall inside Bubble Tea `Update`; it may call the LLM and must run in the agent command goroutine or a wrapper around the agent runner.
- Memory integration should be mostly invisible in the TUI for Phase 8. Full `/memory list`, `/memory edit`, and review UI are Phase 13.
- Manual review of `pending/` drafts can be filesystem-based in Phase 8.
- Minimal visible system messages are acceptable for memory load failures only if they do not spam normal sessions.

## Documentation And Log Findings

The source `.codex` plans agree that Phase 8 is Memory. `docs/PROJECT-STATUS-AND-ONBOARDING.md` also marks Phase 8 as Memory and not started.

`docs/PHASE-LOG.md` has old "Next Steps" wording after Phase 6/7 that refers to "Phase 8 - Tasks" and "Phase 11 - Memory". That wording is stale after the current phase plan. The authoritative current order is:

- Phase 8: Memory
- Phase 9: Hooks
- Phase 10: MCP
- Phase 11: Sub-agents and fork
- Phase 14: Tasks

Phase 8 implementation should update `docs/PHASE-LOG.md` with the completed memory phase and should avoid copying the stale task numbering forward.

## Deep Analysis Of `book/ch11-memory.md`

Chapter 11 is not just a feature description. It is a set of design constraints for memory as an agent behavior. The important lesson is that storage is intentionally simple, while behavior is intentionally strict. The memory system should not become a database, a vector index, a hidden cache, or a second source of truth for project facts.

### Principles To Preserve

Phase 8 should preserve these chapter-level principles:

- Memory files are user-visible notes, not authoritative application state. A memory records an observation that may be wrong or stale.
- Markdown files are the storage format because they are inspectable, editable, diffable, and recoverable with normal filesystem tools.
- The four-type taxonomy is a filter, not a label set. If information can be re-derived from the current repo, it should usually not become memory.
- Exclusions still apply when the user says "remember this." If the requested memory is a raw PR list, code pattern, command output, git history, or task scratchpad, the agent should ask what was surprising or durable instead of saving the raw material.
- `description` frontmatter is the recall index. It needs to be specific enough for an LLM selector to make a relevance decision without reading the body.
- Scan should read only frontmatter, not full memory bodies. The body remains private until a file is selected.
- The active write path is two-step: write or update the topic file, then update `MEMORY.md` as a short table of contents.
- `MEMORY.md` is not authoritative for existence. It orients the model and the user; the scanner must trust actual files on disk.
- Recall should use an LLM side-query over a manifest, not keyword matching and not embeddings.
- Recalled filenames are untrusted model output and must be validated against the scanned manifest before reading.
- Staleness warnings are action cues. They should make the model verify current code before relying on an old observation.
- Background extraction is a safety net, not the primary write path. The main agent still receives explicit memory-saving instructions.
- Memory path handling is security-sensitive because memory access can create a permission carve-out if implemented casually.

### Deliberate Adaptations For This Repo

The book describes a mature TypeScript/Claude Code implementation. Phase 8 must adapt it to the current Go/Ollama repo:

- The book's write path uses `FileWriteTool` and `FileEditTool`; this repo currently has only `FileRead` and `FileWrite`. Phase 8 should instruct full-file replacement for updates rather than adding `FileEdit` early.
- The book benefits from prompt-cache-aware async recall. Ollama has no equivalent prompt cache, and the current TUI has no attachment/collapse model for late-arriving memory. Phase 8 should run bounded pre-run recall in the runner wrapper first. Later phases can add async prefetch or cache last-turn recall if local-model latency requires it.
- The book's background extraction uses a forked agent with constrained tools. Sub-agents and task supervision are not implemented yet, so Phase 8 should produce bounded `pending/` drafts directly through `internal/memory` instead of spawning a tool-using child agent.
- The book has team memory, KAIROS append-only logs, `/dream` consolidation, and a memory enable/disable chain. These are explicitly out of scope for Phase 8, but their security lessons still apply to path containment and future planning.
- The book has telemetry for selection rates. This repo has a dedicated future Phase 16 for decorator-based logs and metrics, so Phase 8 should return recall counts and durations for tests/debug logs only. Do not add metrics sinks or scattered counters in this phase.
- The book's default path is `~/.claude/projects/<slug>/memory/`; this repo uses `paths.MemoryDir(scopeRoot)`, currently under `NANDOCODEGO_DATA_HOME` or XDG data fallback.

### Resolved Plan Differences

The sparse `.codex` Phase 8 plan and the book disagree on staleness threshold. The sparse plan said warnings appear for entries older than 30 days. Chapter 11 says memories from today or yesterday get no warning and everything older gets a caveat. Phase 8 should follow the book: no warning for today/yesterday; action-cue warning for memories two or more calendar days old.

The sparse `.codex` Phase 8 plan also mentions `internal/memory/paths.go` and direct sanitization. The current repo already has `internal/paths.MemoryDir` and `paths.SanitizePathForDir`, so Phase 8 should add scope-root resolution in `internal/memory/root.go` and reuse `internal/paths`.

## Evaluation Of The Original Phase 8 Plan

The original Phase 8 plan is correct at the product level:

- file-based memory
- Markdown and YAML frontmatter
- four memory types
- always-loaded index
- LLM side-query recall
- no embeddings
- staleness warnings
- pending extraction

It needs more implementation detail for this repo:

- It assumes a memory package does not already have a path helper, but `internal/paths.MemoryDir` already exists.
- It assumes a canonical git root is available, but `bootstrap.GitRoot` is currently empty.
- It does not specify how memory writes are allowed through current path-safe tools.
- It does not account for the missing FileEdit tool.
- It sets a 30-day staleness threshold, while Chapter 11 calls for warnings on anything older than yesterday.
- It does not specify how recall is triggered without blocking Bubble Tea `Update`.
- It does not define how recalled content is injected into `agent.Input.SystemPrompt`.
- It does not cover conversation-history persistence needed for future turns and extraction.
- It does not define tests that avoid a live Ollama instance.
- It does not define symlink/path traversal protections for internal writes to `pending/`.
- It does not explain how pending drafts are reviewed before Phase 13 command UX exists.

## Final Phase 8 Scope

In scope:

- Standard per-project memory under `paths.MemoryDir(scopeRoot)`.
- Top-level memory files and `MEMORY.md`.
- `pending/` draft extraction directory.
- Four memory types: `user`, `feedback`, `project`, `reference`.
- YAML frontmatter parsing.
- Index cap enforcement.
- Staleness warnings.
- LLM side-query recall with structured output and filename validation.
- System prompt memory section construction.
- Agent runner wrapper or equivalent composition layer that injects memory before a run.
- Tool context update so the agent can write memory files through existing file tools.
- Pending extraction drafts after completed runs.
- Tests and benchmarks.
- Phase log update.

Out of scope:

- Team memory.
- KAIROS append-only daily logs and `/dream` consolidation.
- Memory sync.
- Embeddings or vector stores for recall.
- Full `/memory` slash command UI.
- Config file support for enabling/disabling memory or choosing a recall model.
- Project settings memory path overrides.
- Automatic merging of pending extraction drafts into active memory.
- Background task supervision for extraction.
- Metrics, telemetry export, or new logging/metrics decorators.
- MCP, hooks, sub-agents, skills, and task integration.
- Full docs site pages.

## Target User Experience

### Session Start

When the REPL starts:

1. Resolve the memory scope root:
   - use `bootstrap.Snapshot.GitRoot` when non-empty;
   - otherwise find the nearest git root from `WorkingDir`;
   - otherwise use `WorkingDir`.
2. Compute memory directory with `paths.MemoryDir(scopeRoot)`.
3. Ensure the directory exists.
4. Ensure `MEMORY.md` exists or treat a missing file as an empty index.
5. Add the memory directory to `ToolSettings.AdditionalWorkingDirs` so FileRead/FileWrite can operate there through normal path safety and permission checks.
6. Tell the model in the prompt that the directory already exists so it does not waste turns on `mkdir` or exploratory `ls` calls before writing.

### Prompt Submission

When a user submits a normal prompt:

1. TUI appends the user message as it does today.
2. The agent command goroutine starts.
3. The memory wrapper identifies the latest user message.
4. Memory loads the capped `MEMORY.md` index.
5. Memory scans top-level memory files for frontmatter.
6. Memory asks the configured recall model to select relevant filenames from descriptions only.
7. Returned filenames are validated against the scanned set.
8. Selected files are read, wrapped with staleness warnings when needed, and turned into a memory prompt section.
9. The wrapped runner calls the real agent with `Input.SystemPrompt` extended by the memory section.

### Memory Writes During A Run

The main agent receives memory instructions in the system prompt:

- Save only durable, non-derivable information.
- Use exactly four types.
- Create or update a memory file under the memory directory.
- Update `MEMORY.md` with one short index entry.
- Do not save code facts that can be re-derived by reading the repo.
- Do not save raw PR lists, git history, command output, task scratchpads, or anything already documented in project files. Ask what durable lesson should be saved instead.
- Do not save secrets.
- Use existing FileRead/FileWrite tools and normal permission flow.

### Run Completion

After a completed run:

1. The runner wrapper may launch a short, bounded extraction pass.
2. Extraction uses the recent conversation and current memory manifest.
3. It proposes candidate memories as draft Markdown files under `pending/`.
4. Drafts are not active until the user manually reviews and moves/merges them.
5. Failures are debug-level only and never block the user-visible run completion.

## Architecture

### Package Layout

```text
internal/memory/
  types.go
  root.go
  store.go
  frontmatter.go
  scan.go
  index.go
  staleness.go
  prompt.go
  recall.go
  extract.go
  runner.go

  types_test.go
  root_test.go
  store_test.go
  frontmatter_test.go
  scan_test.go
  index_test.go
  staleness_test.go
  prompt_test.go
  recall_test.go
  extract_test.go
  runner_test.go
  scan_benchmark_test.go
```

Names may change if implementation reveals a simpler split, but keep these responsibilities separate.

### Core Types

```go
type Type string

const (
	TypeUser      Type = "user"
	TypeFeedback  Type = "feedback"
	TypeProject   Type = "project"
	TypeReference Type = "reference"
)

type Entry struct {
	Filename    string
	Path        string
	Name        string
	Description string
	Type        Type
	UpdatedAt   time.Time
	SizeBytes   int64
}

type LoadedEntry struct {
	Entry
	Content          string
	StalenessWarning string
}

type Config struct {
	Enabled        bool
	Model          string
	MaxSelected    int
	RecallTimeout  time.Duration
	ExtractTimeout time.Duration
	IndexMaxLines  int
	IndexMaxBytes  int
	StaleAfterDays int
}
```

Defaults:

- `Enabled`: true for REPL.
- `Model`: active model until Phase 13 adds config for a small recall model.
- `MaxSelected`: 5 for prompt injection. The source plan allows up to 10; start conservative to protect context.
- `RecallTimeout`: 3 seconds.
- `ExtractTimeout`: 5 seconds.
- `IndexMaxLines`: 200.
- `IndexMaxBytes`: 25000.
- `StaleAfterDays`: 2 calendar days; today and yesterday are not stale, anything older receives an action-cue warning.

### Storage Layout

```text
<memory-dir>/
  MEMORY.md
  user_preferences.md
  feedback_testing.md
  project_release_context.md
  reference_dashboard.md
  pending/
    20260503T213000Z-feedback-testing.md
```

Rules:

- Active memory files live directly under `<memory-dir>`.
- `MEMORY.md` is the always-loaded index.
- `pending/` drafts are not scanned as active memory.
- Nested `team/`, `logs/`, and KAIROS structures are reserved for future phases.
- Memory filenames must be relative basenames ending in `.md`.
- Reject names containing path separators, `..`, null bytes, or absolute paths.

### Frontmatter Contract

Every active memory file must use YAML frontmatter:

```markdown
---
name: Testing Policy
description: Integration tests should use real DB instances, not mocks
type: feedback
---

Body text...
```

Required fields:

- `name`
- `description`
- `type`

Validation:

- `type` must be one of the four allowed values.
- `description` must be non-empty because recall depends on it.
- Files with malformed frontmatter are skipped and returned as warnings from scan.
- Scan reads only the first 30 lines or a small byte cap for frontmatter extraction.

### Index Contract

`MEMORY.md` is always loaded but must remain small.

Caps:

- maximum 200 lines;
- maximum 25,000 bytes;
- whichever limit is hit first triggers a capped result and warning.

Index entries should be one-line links:

```markdown
- [Testing Policy](feedback_testing.md) - integration tests should use real DB instances
```

The index is not authoritative for scan. Scan uses actual files. The index is context for the model and a human table of contents.

### Recall Contract

Recall input:

- latest user prompt;
- compact recent conversation summary or recent user turns;
- manifest of memory entries containing only filename, type, name, updated date, and description;
- set of already loaded filenames.

Recall output schema:

```json
{
  "selected": ["feedback_testing.md"]
}
```

Rules:

- Use `ChatRequest.Format` with a JSON schema.
- Return at most `MaxSelected` filenames.
- Validate every filename against the scanned manifest.
- Drop unknown filenames silently or return a warning; never read them.
- On invalid JSON, timeout, cancellation, or model error, return no recalled entries and continue.

### Staleness Contract

For memory files older than yesterday, prepend an action cue:

```text
Before recommending from memory: confirm this is still current. This memory was last updated 47 days ago.
```

Do not expire memories automatically. Old memories are hypotheses, not facts.

Use calendar-day behavior, not raw duration math:

- memory updated today: no warning;
- memory updated yesterday: no warning;
- memory updated two or more calendar days ago: warning.

### Prompt Injection Contract

Memory content must be injected as contextual notes, not instructions with higher priority than the current user and system prompt.

The memory prompt section must include:

- the memory directory path;
- the four allowed memory types;
- explicit exclusions for derivable code facts, secrets, raw task logs, and ephemeral details;
- explicit instruction to push back on "remember this" requests when the requested content is derivable or ephemeral;
- write protocol using FileRead/FileWrite;
- warning that memories may be stale or user-edited;
- `MEMORY.md` index;
- recalled entries with filenames and staleness warnings.

The section should be clearly marked as dynamic:

```text
=== DYNAMIC MEMORY CONTEXT ===
...
=== END DYNAMIC MEMORY CONTEXT ===
```

## Implementation Plan

### Step 1 - Conversation Persistence Precondition

Problem:

- `state.App.Messages` currently receives submitted user messages.
- The agent internally builds assistant/tool messages, but TUI state is not guaranteed to receive final assistant/tool messages for future turns.
- Memory extraction and future recall should not operate on user-only history.

Plan:

1. Extend `agent.Terminal` or add a new `agent.ConversationDelta` event with generated assistant/tool messages for the completed run.
2. In `agent.run`, track messages added after `Input.Messages`:
   - assistant message for each turn;
   - tool result messages when tools run.
3. In `internal/tui/app.go`, append those new messages to `state.App.Messages` on terminal/conversation event.
4. Add tests proving:
   - a completed text turn persists the assistant message;
   - a tool turn persists assistant tool-call message and tool result messages;
   - submitted user message is not duplicated.

This is Phase 8 support work, not a new product feature.

### Step 2 - Memory Root Resolution

Files:

- `internal/memory/root.go`
- `internal/memory/root_test.go`

Implement:

- `ScopeRoot(workingDir, gitRoot string) (string, error)`
- `DirForScope(scopeRoot string) string`

Rules:

- Use explicit `gitRoot` if non-empty.
- Otherwise walk upward from `workingDir` looking for `.git` directory or file.
- If no git root exists, use `workingDir`.
- Clean paths with `filepath.Clean`.
- Do not follow arbitrary user-provided memory path overrides in Phase 8.

Tests:

- explicit git root wins;
- nested working dir finds parent `.git`;
- non-git temp dir falls back to working dir;
- worktree-style `.git` file does not panic;
- path sanitization produces stable memory dir through `paths.MemoryDir`.

### Step 3 - Filesystem Store

Files:

- `internal/memory/store.go`
- `internal/memory/store_test.go`

Implement a small filesystem store:

- `Ensure(ctx) error`
- `ReadIndex(ctx) (Index, error)`
- `WritePending(ctx, draft Draft) (string, error)`
- `ReadActive(ctx, filename string) (string, error)`
- `ReadSelected(ctx, filenames []string) ([]LoadedEntry, error)`

Security:

- All internal writes use path containment checks.
- `pending/` writes use atomic temp-file-and-rename.
- Reject symlinks that resolve outside memory dir.
- Use restrictive file permissions where practical: directories `0700`, files `0600`.

### Step 4 - Frontmatter And Scan

Files:

- `internal/memory/types.go`
- `internal/memory/frontmatter.go`
- `internal/memory/scan.go`
- tests and benchmark

Dependency:

- Prefer `gopkg.in/yaml.v3`, already allowlisted, for frontmatter parsing.
- If adding it as a direct dependency, record the justification in the Phase 8 log.

Implement:

- `ParseFrontmatter(filename string, r io.Reader, modTime time.Time, size int64) (Entry, error)`
- `Scan(ctx, dir string) (ScanResult, error)`

`ScanResult` should contain:

- valid entries;
- skipped files with reasons;
- duration/counts for debug/testing.

Scan rules:

- Include only top-level `*.md` files.
- Exclude `MEMORY.md`.
- Exclude `pending/`.
- Read at most first 30 lines or a small byte cap for metadata.
- Sort entries by filename for deterministic tests.

Benchmark:

- Generate 1000 memory files with frontmatter.
- Assert scan target is under 50 ms on normal local disk.
- Keep benchmark independent from live Ollama.

### Step 5 - Index Loading And Caps

Files:

- `internal/memory/index.go`
- `internal/memory/index_test.go`

Implement:

- `LoadIndex(path string, limits Limits) (Index, error)`
- `Index.Capped bool`
- `Index.Warning string`

Rules:

- Missing `MEMORY.md` is an empty index, not fatal.
- Cap by both lines and bytes.
- Return warning text instructing the model/user to consolidate if capped.
- Tests cover exactly-at-limit and over-limit cases.

### Step 6 - Staleness Warnings

Files:

- `internal/memory/staleness.go`
- `internal/memory/staleness_test.go`

Implement:

- `AgeDescription(now, updatedAt time.Time) string`
- `StalenessWarning(now, updatedAt time.Time) string`

Rules:

- today and yesterday: no warning.
- two or more calendar days old: action cue warning.
- use human-readable age like "47 days ago".
- tests use fixed dates.

### Step 7 - Prompt Section Builder

Files:

- `internal/memory/prompt.go`
- `internal/memory/prompt_test.go`

Implement:

- `BuildSection(input SectionInput) string`

Input:

- memory dir;
- index content and warning;
- recalled entries;
- pending draft note if present.

Output constraints:

- Clearly marked dynamic section.
- States memory content is context, not higher-priority instruction.
- Includes write protocol and exclusions.
- Does not include every scanned description unless needed.
- Deterministic output for tests.

Tests:

- empty memory returns small section or no section depending config;
- index included and capped warning included;
- recalled memory content included with filename labels;
- stale warnings included;
- secrets are not generated by templates.

### Step 8 - LLM Recall

Files:

- `internal/memory/recall.go`
- `internal/memory/recall_test.go`

Implement:

- `Recall(ctx, client llm.Client, cfg Config, query Query, entries []Entry, alreadyLoaded map[string]bool) (RecallResult, error)`

Use fake `llm.Client` tests for:

- valid selection;
- hallucinated filename dropped;
- more than max selected truncated;
- invalid JSON returns empty non-fatal result;
- timeout/cancel returns context error or empty result according to caller needs;
- manifest contains descriptions only, not full memory bodies.

Prompt guidance:

- Be conservative.
- Select only memories useful for the current query.
- Skip if uncertain.
- Do not select memories already loaded.
- Do not select reference docs for tools already active unless they are warnings/gotchas.

### Step 9 - Runner Integration

Files:

- `internal/memory/runner.go`
- `internal/memory/runner_test.go`
- `internal/cli/repl.go`

Implement a decorator around the agent runner:

```go
type Runner interface {
	Run(context.Context, agent.Input) <-chan agent.Event
}
```

The memory runner:

1. receives `agent.Input`;
2. resolves memory dir;
3. loads index;
4. scans entries;
5. recalls relevant entries using latest user message;
6. builds memory section;
7. appends it to `Input.SystemPrompt`;
8. delegates to the real runner;
9. forwards all events unchanged;
10. optionally triggers pending extraction after terminal completion.

Why this shape:

- It keeps Bubble Tea `Update` free of network calls.
- It keeps `agent.Agent` provider-neutral and mostly unaware of memory.
- It keeps memory as one of the six abstractions without inventing a new command framework.
- It is testable with fake runners and fake clients.
- It gives Phase 15 a clean place to add async prefetch later if sequential local-model recall is too slow.

CLI wiring:

- In `runREPL`, after memory dir resolution, add it to `appState.ToolSettings.AdditionalWorkingDirs`.
- Wrap `agentRunner` before passing it to `tui.New`.
- Use active model as recall/extraction model until Phase 13 adds config.

### Step 10 - Explicit Memory Write Instructions

Files:

- `internal/memory/prompt.go`
- tests

The memory prompt should tell the model:

- Write durable memory only when explicitly asked or when a durable preference/correction is clearly established.
- Use one of four types.
- Use semantic filenames: `<type>_<topic>.md`.
- Before creating a new memory, prefer updating an existing related file.
- Keep `MEMORY.md` entries one line and under about 150-200 characters.
- Do not save:
  - code patterns derivable from the repo;
  - raw command output;
  - raw PR lists or git history;
  - secrets;
  - ephemeral task lists;
  - anything already in project docs or source.
- If the user asks to remember excluded material, ask what durable, surprising, or non-obvious lesson should be saved instead.

Because FileEdit does not exist yet, instructions should say:

- Read the existing memory file.
- Write the full updated replacement with `FileWrite`.

### Step 11 - Pending Extraction

Files:

- `internal/memory/extract.go`
- `internal/memory/extract_test.go`

Implement:

- `ExtractDrafts(ctx, client llm.Client, cfg Config, conversation []llm.Message, manifest []Entry) ([]Draft, error)`
- `WritePending` stores drafts under `pending/`.

Rules:

- Extraction runs after completed agent runs only.
- Use a short timeout.
- Never block terminal completion.
- Never auto-merge drafts into active memory.
- Draft files include frontmatter and a clear review marker.
- If the main agent wrote active memory during the run, extraction may skip or produce no drafts.
- Extraction writes proposal files only. It must not update active topic files or `MEMORY.md`.

Phase 8 can keep detection simple:

- If the conversation includes recent tool writes under the memory dir, skip extraction for that run.
- Otherwise ask the extractor model whether anything is worth remembering.

Full background task supervision belongs to Phase 14.

### Step 12 - Tests, Benchmarks, And Manual Smoke

Required commands:

```sh
go test ./internal/memory/...
go test ./internal/agent/... ./internal/tui/... ./internal/cli
go test -bench=BenchmarkScan1000 ./internal/memory
tools/check-allowed-deps.sh
tools/check-network-policy.sh
```

If adding `gopkg.in/yaml.v3` as a direct dependency:

```sh
go mod tidy
tools/check-allowed-deps.sh
```

Manual smoke:

```sh
go run ./cmd/nandocodego --model qwen3 --no-alt-screen
```

Flow:

1. Ask the agent to remember a preference.
2. Approve memory file writes if prompted.
3. Exit.
4. Inspect memory dir:
   - `MEMORY.md`
   - one active memory file
5. Start a new REPL in the same working tree.
6. Ask a question that should use the memory.
7. Confirm behavior.

## Acceptance Criteria

- [ ] `internal/memory` exists with path/root, scan, index, staleness, prompt, recall, extraction, and runner integration.
- [ ] `MEMORY.md` is loaded with 200-line and 25,000-byte caps.
- [ ] Scan handles 1000 entries in under 50 ms in benchmark conditions.
- [ ] Recall uses `llm.Client` and structured JSON output.
- [ ] Recall validates filenames against scanned entries and never reads hallucinated names.
- [ ] Recall returns at most `MaxSelected` entries.
- [ ] Recalled entries older than yesterday include action-cue staleness warnings.
- [ ] Memory prompt section marks memory as dynamic, stale-possible context rather than higher-priority instruction.
- [ ] The agent can write memory files with existing FileRead/FileWrite tools through normal permission flow.
- [ ] Memory directory is added to tool additional working dirs without broadening access outside the memory directory.
- [ ] Pending extraction writes drafts only under `pending/`.
- [ ] Pending drafts are not loaded as active memory.
- [ ] No embeddings are used for memory recall.
- [ ] No fifth memory type is introduced.
- [ ] Exclusion prompts tell the model to reject or narrow derivable/ephemeral "remember this" requests.
- [ ] Tests do not require live Ollama.
- [ ] Dependency and network policy checks pass.
- [ ] `docs/PHASE-LOG.md` has a Phase 8 entry recording files, checks, decisions, and any deferred work.

## Forbidden

- Vector embeddings or vector stores as the memory recall mechanism.
- A fifth memory type.
- Treating memory content as authoritative system instructions.
- Auto-merging pending extraction drafts into active memory.
- Project settings that can redirect memory paths.
- Silent permission bypass for memory writes.
- Logging raw memory contents, prompts, model outputs, file contents, or raw extraction results.
- Running recall or extraction inside Bubble Tea `Update`.
- Adding `/memory` command UX beyond tiny debug-only hooks; Phase 13 owns full commands.
- Team memory, KAIROS logs, or dream consolidation.

## Risks And Mitigations

| Risk | Impact | Mitigation |
|---|---:|---|
| Memory captures secrets | High | Prompt exclusions, no auto-merge extraction, pending review, never log memory bodies. |
| Memory becomes stale but sounds authoritative | High | Action-cue staleness warnings for memories older than yesterday; frame memory as context, not truth. |
| Recall slows every prompt | Medium | Side-query timeout, small manifest, max selected entries, continue without recall on failure. |
| Recall hallucinated filenames | Medium | Validate against scanned set before reading. |
| Memory dir widens file tool access too much | High | Add only exact memory dir to additional roots; keep path safety and permissions. |
| Agent saves derivable code facts | Medium | Strong taxonomy/exclusion instructions; pending extraction review. |
| No FileEdit tool causes clumsy updates | Low | Use FileRead plus full FileWrite replacement in Phase 8; FileEdit can improve later. |
| Incomplete conversation state weakens extraction | Medium | Add conversation persistence event/terminal field before extraction. |
| Pending extraction feels invisible | Low | Phase 8 docs and filesystem review; Phase 13 adds `/memory` UX. |

## Phase Log Template

When implementation finishes, append a Phase 8 entry to `docs/PHASE-LOG.md` with:

- objective;
- files created/updated;
- dependencies added and allowlist status;
- tests/benchmarks/checks run;
- manual two-session smoke result;
- design decisions;
- known constraints and deferred work;
- exit gate status.

## Exit Gate

Phase 8 is complete only when:

- all acceptance criteria above are met;
- targeted tests and security checks pass;
- memory works across two REPL sessions with a real Ollama model;
- the phase log records the implementation and any deviations from this plan.
