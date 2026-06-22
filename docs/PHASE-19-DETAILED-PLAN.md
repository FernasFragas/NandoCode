# Phase 19 Detailed Plan - Complete Tool Ecosystem

Date: 2026-05-07
Status: ✅ Complete — 2026-05-08
Source plans:

- `.codex/go-ollama-plan-AGENTS.md`
- `.codex/go-ollama-plan-HUMANS.md`
- `docs/PHASE-LOG.md`
- `docs/PROJECT-STATUS-AND-ONBOARDING.md`
- `docs/PHASE-8-DETAILED-PLAN.md`
- `docs/PHASE-9-DETAILED-PLAN.md`
- `book/ch06-tools.md`
- `book/ch17-performance.md`
- `.codex/agent-context/learnings-memory.md`

## IMPORTANT ORDERING NOTE

**Phase 19 must be executed BEFORE Phase 18 (hardening/eval).** The eval suite in Phase 18 requires FileEdit and Grep to test realistic coding scenarios. Without this phase, the eval suite can only exercise Bash, FileRead, and FileWrite, which covers fewer than half of the targeted coding workflows. FileEdit is the single most-requested missing tool in any multi-file coding session. Grep is required for any "find all usages of X" eval scenario. Implementing Phase 19 first makes Phase 18 meaningful.

## Goal

Implement all remaining built-in tools referenced in the architecture diagram and `book/ch06-tools.md`. After this phase the agent can do real coding work: make precise targeted edits to existing files, search codebases with regex patterns, find files by glob, fetch web content for research, and track its own in-session task list.

The agent architecture diagram specifies: Bash, FileRead, FileWrite, **FileEdit, Grep, Glob, WebFetch, Agent, Todo** — but FileEdit, Grep, Glob, WebFetch, and Todo have no implementation phase yet. Phase 19 fills that gap. (Agent/sub-agent dispatch is Phase 11; WebSearch is deliberately deferred because it requires an external API key not appropriate for local-first v0.1.)

Deliverables:

- `internal/tools/fileedit` package with atomic, staleness-detected, diff-displaying patch tool.
- `internal/tools/glob` package with `**` recursive pattern matching and auto-exclusions.
- `internal/tools/grep` package with regex search, binary file detection, context lines, and head-limit pagination.
- `internal/tools/webfetch` package with HTML stripping, URL validation, and permission gating.
- `internal/tools/todo` package with session-scoped in-memory task list.
- All 7 new tools (TodoWrite and TodoRead are separate tools) registered in `internal/tools/builtin/builtin.go`.
- `tools/allowed-deps.txt` updated if any new direct dependency is added.
- Tests with `go test -race` for every new package.
- Phase log update after implementation.

## Definition Of Success

The Phase 19 exit gate is a single REPL session that exercises all five new tool families:

1. Open the REPL.
2. Ask the agent to grep for `TODO` comments in the working directory.
3. Ask it to glob all `*_test.go` files.
4. Ask it to edit the first result from step 3 by adding a comment line (uses FileEdit).
5. Ask it to fetch the Ollama API docs URL and summarize the `/api/chat` endpoint.
6. Ask it to create a todo list with the three things it just found.
7. Ask it to mark the first todo as in_progress.
8. Verify each tool call shows in the transcript with a correct render hint.

All six operations must succeed without the agent falling back to Bash workarounds. This exit gate requires a live Ollama model.

## Baseline Analysis From Implemented Phases

### Phase 0 - Security and Supply Chain

Implemented:

- `SECURITY.md` trust boundaries.
- `tools/allowed-deps.txt` with 18 allowed direct runtime deps.
- `tools/check-allowed-deps.sh` and `tools/check-network-policy.sh`.

Phase 19 implications:

- `golang.org/x/net` is already an **indirect** dep (present in `go.sum`). WebFetch can use its `html` tokenizer package. Before importing it directly in a new package, add `golang.org/x/net` to `tools/allowed-deps.txt` with justification: HTML stripping for WebFetch. The allowlist check covers only direct deps, so if `golang.org/x/net` stays indirect the script will not flag it, but adding it explicitly is the correct procedure.
- WebSearch (which would require an external API key or a search engine endpoint) must NOT be added in this phase. The network policy script would flag any hardcoded external endpoint.
- All file-path tools must be validated by `tools/check-network-policy.sh` because they do not introduce new URLs.
- No secret must be logged by any new tool, including WebFetch response bodies.

### Phase 1 - CLI, Paths, Logging, Scaffold

Implemented:

- `internal/paths` with `MemoryDir`, `SanitizePathForDir`, and XDG helpers.
- `internal/logging` with structured slog setup.

Phase 19 implications:

- No new path helpers are needed. File tools reuse `tools.ResolvePath` and `tools.ResolveContained`.
- Glob and Grep tools do their own directory walks; do not add path helpers to `internal/paths` for this phase.
- WebFetch should log only the URL (no response body) at debug level.

### Phase 2 - LLM Client

Implemented:

- Provider-neutral `llm.Client` interface.
- Streaming `Chat` and `ChatRequest.Format`.

Phase 19 implications:

- No LLM calls in Phase 19 tools except potentially a future SmartEdit path. All five tool families in this phase are deterministic: they read/write/search files or make HTTP calls.
- No `llm.Client` injection needed in any new tool for Phase 19.

### Phase 3 - Tool Interface and Starter Tools

Implemented:

- `tools.Tool` interface, `tools.BuildTool`, `tools.Spec`, fail-closed defaults.
- `tools.Context` with `WorkingDir`, `AdditionalWorkingDirs`, `PermissionMode`, `RecordFileSnapshot`.
- `tools.ResolvePath` with `PathRead`/`PathWrite` modes.
- `Bash`, `FileRead`, `FileWrite` tool implementations.
- `internal/tools/builtin/builtin.go` starter registry.
- `tools/pathsafe.go` with `ResolveContained`, symlink guard, special-path rejection.

Phase 19 implications:

- Every new tool should call `tools.ResolvePath` (or `tools.ResolveContained` for the read mode) rather than implementing its own path resolution.
- `tools.BuildTool` factory applies fail-closed defaults. Any new tool that omits `IsConcurrentFunc` defaults to `false` (serial). This is intentional for FileEdit; Glob and Grep should explicitly set `IsConcurrentFunc: func(any) bool { return true }`.
- `tools.RecordFileSnapshot` must be called in FileEdit before writing, exactly as FileWrite does, so the snapshot cache stays consistent.
- The `tools.Context.TodoList` field (added in this phase — see types section below) should be added without breaking existing tools. Existing tools do not use it; they simply ignore it.
- `builtin.go` registers all tools by calling `NewXxx()` constructors. Phase 19 adds 7 new calls there.

### Phase 4 - Agent Loop

Implemented:

- `agent.Run(ctx, input) <-chan agent.Event`.
- `agent.Input.SystemPrompt`.
- Tool execution loop in `internal/agent/tools.go`.
- `TerminalContextOverflow` terminal reason.

Phase 19 implications:

- Agent loop is serial today (Phase 15 adds concurrency). All Phase 19 tools must be safe to run serially without deadlocks. No new synchronization is required in the agent loop itself.
- FileEdit, Glob, and Grep results should respect `tools.Context.MaxResultChars` to avoid context overflow. Use `tools.TruncateDisplay`.
- `TerminalContextOverflow` is distinct from Phase 20 compaction. Phase 19 tools must not try to prevent it; that belongs to Phase 20.

### Phase 5 - Permission System

Implemented:

- Seven permission modes including `PermissionPlan`, `PermissionDontAsk`, `PermissionBypassPermissions`.
- `permissions.Resolve` with `HookDecision` callback.
- Tool-specific `CheckPermissions()` flows through resolver.

Phase 19 implications:

- FileEdit is `IsDestructive=true`, `IsReadOnly=false`. `CheckPermissions` should behave identically to FileWrite: `PermAllow` in `bypassPermissions`, `PermDeny` in `plan`/`dontAsk`, `PermAsk` otherwise.
- Glob and Grep are `IsReadOnly=true`. `CheckPermissions` may return `PermAllow` unconditionally.
- WebFetch is `IsReadOnly=true` from the filesystem perspective but initiates outbound network. `CheckPermissions` returns `PermAsk` for all fetches — even in `bypassPermissions` mode, surface a prompt for network access. This is intentional: the user should know the agent is making external HTTP requests.
- TodoWrite and TodoRead operate only on in-session memory. `CheckPermissions` returns `PermAllow` for both. There are no user-visible side effects outside the session.

### Phase 6 - State Layer

Implemented:

- `internal/bootstrap` session facts.
- `state.App` with `Messages`, `ToolSettings`, `ToolContext(ctx)`.
- `state.App.ToolSettings.AdditionalWorkingDirs`.

Phase 19 implications:

- `tools.Context` gains a `TodoList *todo.TodoList` field in Phase 19. This field is set once at REPL startup in `state.App.ToolContext(ctx)` construction, so it is session-scoped. Do not add `TodoList` to `bootstrap.State` or `state.App`; it belongs only in `tools.Context`.
- File content hash cache for FileEdit staleness detection also lives in `tools.Context` as `FileSnapshots map[string][]byte` (or reuse `RecordFileSnapshot` which already exists). See the FileEdit section for details.

### Phase 7 - Bubble Tea TUI and REPL

Implemented:

- Transcript rendering, tool panels, permission modal.
- `RenderHints` controls what appears in transcript tool panels.

Phase 19 implications:

- Each new tool should return meaningful `RenderHints.Title` and `RenderHints.Summary`. Examples:
  - FileEdit: `Title: "FileEdit"`, `Summary: "path/to/file"`
  - Glob: `Title: "Glob"`, `Summary: "**/*.go (42 files)"`
  - Grep: `Title: "Grep"`, `Summary: "pattern (15 matches)"`
  - WebFetch: `Title: "WebFetch"`, `Summary: "https://example.com"`
  - TodoWrite: `Title: "TodoWrite"`, `Summary: "3 todos"`
  - TodoRead: `Title: "TodoRead"`, `Summary: "3 todos"`

### Phase 8 - Memory

Implemented:

- Memory runner decorator.
- `internal/memory` with scan, recall, prompt injection.

Phase 19 implications:

- No direct interaction between Phase 19 tools and memory. Memory runner does not care which tools are registered.
- After Phase 19 is complete, a future memory prompt instruction can be updated to mention FileEdit for memory file updates, replacing the current "read full file then FileWrite" instruction.

### Phase 9 - Hooks

Implemented:

- `internal/hooks` with PreToolUse, PostToolUse, and lifecycle events.
- Hook snapshot at session start.
- Predefined event constants including `PreCompact`, `PostCompact` (reserved for Phase 20).

Phase 19 implications:

- All new tools flow through the same `PreToolUse`/`PostToolUse` hook dispatch as existing tools because hooks are wired in the agent tool path, not per-tool. No per-tool hook wiring is needed.
- WebFetch outbound URL is available in the hook input envelope (`tool.target = url`). A future user could write a hook that denies specific domains.

## Documentation and Log Findings

`docs/PROJECT-STATUS-AND-ONBOARDING.md` describes Phase 3 as providing only 3 tools (Bash, FileRead, FileWrite) and explicitly calls out that the architecture diagram's FileEdit, Grep, Glob, WebFetch, and Todo have no implementation phase. This plan is the authoritative answer to that gap.

`book/ch06-tools.md` specifies the following for the Go adaptation:

- `IsConcurrencySafe(input any) bool` takes the parsed input. For Grep and Glob this is always `true`. For FileEdit it is always `false` (file mutation is not concurrency-safe until Phase 15 adds lock-based batching).
- `IsDestructive(input any) bool` should be `true` for FileEdit and `false` for all read-only tools.
- Result budgeting: per-tool `MaxResultChars` limits prevent single-tool context explosions. Glob and Grep should cap at 50,000 characters (configurable through `tools.Context.MaxResultChars`). WebFetch defaults to 50,000 chars but allows caller override via `max_length` input field.
- `CheckPermissions` for read-only tools should return `PermAllow` directly rather than going through the ask path. This is performance-relevant: not adding unnecessary permission prompts for safe read-only operations.

`book/ch17-performance.md` specifies:

- Result budgeting applies per-tool and per-conversation. Grep at 250 results with context lines can still produce large output; Glob at 1000 files can be large. Use `tools.TruncateDisplay` with a `<truncated>` suffix to bound output, then let the model paginate if it needs more.
- Token efficiency: Grep and Glob output is highly repetitive (path prefixes repeat). Gzip-style context is handled by the LLM, not the tool. Still, sorted output and clear formatting help the model parse results without wasted tokens.

## Deep Analysis Of `book/ch06-tools.md` For Phase 19

Chapter 6 describes several patterns directly applicable to the Phase 19 tool set.

### FileEdit and Staleness Detection

The chapter describes `FileEditTool`'s staleness mechanism in detail (section "FileEditTool: Staleness Detection"). The key design:

- `readFileState` is an LRU cache of file contents and timestamps, maintained across the conversation.
- Before applying an edit, the tool checks whether the file has been modified since the model last read it.
- If stale, the edit is rejected with a message telling the model to re-read the file first.

In the Go repo, `tools.Context.RecordFileSnapshot` already serves this role for FileWrite. FileEdit should extend the same mechanism:

- When FileRead is called, it should call `ctx.RecordFileSnapshot(path, content)`.
- When FileEdit is called, it should check the current file bytes against the snapshot. If different (hash mismatch), reject the edit with a staleness message.
- The snapshot cache should be a `map[string][]byte` held in `tools.Context`. Currently `RecordFileSnapshot` is a `func(path string, content []byte)` callback. Phase 19 should add a corresponding `ReadFileSnapshot(path string) ([]byte, bool)` callback to `tools.Context` so FileEdit can check staleness without circular package imports.

The Go adaptation does not use an LRU; a plain `map[string][]byte` per-session is sufficient. The session is bounded by the agent run which typically covers dozens of files, not thousands.

### Fuzzy Match Normalization

Chapter 6 describes `findActualString()` for fuzzy matching:

- Normalizes whitespace and quote styles before matching.
- `replace_all` flag enables bulk replacements.
- Without `replace_all`, non-unique matches are rejected (requiring more context to identify a single location).

Phase 19 FileEdit should implement equivalent normalization:

- Trim trailing whitespace from each line of `old_string`.
- Normalize `\r\n` to `\n`.
- Do NOT normalize quote style: Go source uses specific quote characters for correctness; normalizing quotes would corrupt string literals.
- After normalization, require exactly one match unless `replace_all=true`.

### GrepTool Pagination

Chapter 6 describes GrepTool's `head_limit` with default 250 entries and `appliedLimit: true` signal. The Go Grep tool should follow the same contract: default 250, configurable via `head_limit` input, with explicit truncation notice in the output.

### Tool Result Budgeting

Chapter 6 Table: GrepTool has `maxResultSizeChars: 100,000`. The Go tools should use `ctx.EffectiveMaxResultChars()` which defaults to 30,000 chars in `tools.Context`. For Grep specifically, 30,000 chars may be too tight for context-heavy results. Phase 19 can set a tool-specific default of 50,000 chars by checking `ctx.MaxResultChars` and applying `max(ctx.MaxResultChars, 50_000)` as the Grep floor. This keeps it tunable.

## Evaluation Of The Original Phase 19 Plan

The original Phase 19 description in the prompt is correct at the product level. It specifies:

- FileEdit with staleness detection, fuzzy normalization, diff display, atomic write.
- Glob with `**` patterns, auto-exclusions, 1000-file limit.
- Grep with regex, binary detection, head-limit 250, context lines.
- WebFetch with HTML stripping, URL validation, 30s timeout.
- TodoWrite/TodoRead with session-scoped in-memory list.

The plan needs supplementation in these areas:

- It does not specify how the file content hash cache integrates with the existing `RecordFileSnapshot` callback in `tools.Context`. This plan adds `ReadFileSnapshot` to complete the bidirectional contract.
- It says `golang.org/x/net/html` is "already an indirect dep" — confirmed in `go.sum`. The plan must specify the `tools/allowed-deps.txt` update procedure.
- It does not specify how `TodoList` is initialized in the REPL startup path. This plan adds it to `tools.Context` construction in `state.App.ToolContext`.
- The plan says "no new dep for WebSearch". WebSearch is out of scope entirely and should not be mentioned in the code.
- Auto-excludes for Glob and Grep should be consistent. This plan specifies: `.git/`, `node_modules/`, `.svn/`, `vendor/`, `dist/`.

## Final Phase 19 Scope

In scope:

- `internal/tools/fileedit` package: patch-based editing with staleness detection, fuzzy normalization, diff display, atomic write, `replace_all` mode.
- `internal/tools/glob` package: pattern matching with `**` support, auto-excludes, 1000-file limit, sorted output.
- `internal/tools/grep` package: regex search with binary detection, head-limit 250, context lines 0-5, auto-excludes.
- `internal/tools/webfetch` package: URL validation, HTML stripping, 30s timeout, 3-redirect limit, `PermAsk` permission gating, truncation.
- `internal/tools/todo` package: session-scoped `TodoList`, `TodoWrite` replaces full list, `TodoRead` returns current list, checklist display.
- `tools.Context` additions: `ReadFileSnapshot` callback, `TodoList *todo.TodoList`.
- `FileRead` patch: call `ctx.RecordFileSnapshot` so FileEdit can detect staleness.
- `internal/tools/builtin/builtin.go`: register all 7 new tools.
- `tools/allowed-deps.txt`: add `golang.org/x/net` if importing it directly.
- Test files for all new packages, all passing with `go test -race`.
- `docs/PHASE-LOG.md` Phase 19 entry.

Out of scope:

- WebSearch (external API key required, out of scope for local-first v0.1).
- Agent/sub-agent tool (Phase 11).
- MCP tool wrapping (Phase 10).
- FileEdit smart/AI-assisted editing.
- Glob/Grep result streaming via `ProgressEvent`.
- Persistent todo list (cross-session persistence is Phase 14 task storage).
- WebFetch with authentication headers.
- Grep `--files-with-matches` mode.
- Config-backed Glob exclude lists beyond the hardcoded defaults.

## Target User Experience

### FileEdit

The model calls:

```json
{
  "path": "internal/foo/foo.go",
  "old_string": "func OldName()",
  "new_string": "func NewName()"
}
```

If the file has not been read since the last edit (staleness), it returns:

```
FileEdit: staleness detected — file has changed since last read. Re-read the file before editing.
```

If `old_string` is not found after normalization:

```
FileEdit: old_string not found in file. Check the exact content with FileRead first.
```

On success, it returns a diff-style display:

```
--- internal/foo/foo.go
+++ internal/foo/foo.go
@@ -12,3 +12,3 @@
-func OldName()
+func NewName()
```

### Glob

```json
{"pattern": "**/*_test.go"}
```

Returns sorted paths, one per line:

```
internal/agent/agent_test.go
internal/agent/tools_test.go
internal/memory/runner_test.go
...
42 files matched.
```

### Grep

```json
{"pattern": "TODO", "include": "*.go", "context_lines": 1}
```

Returns matches in `filename:line:content` format:

```
internal/foo/foo.go:42:    // TODO(nando): remove deprecated path
internal/foo/foo.go:43:    old := legacyFn()
internal/bar/bar.go:17:// TODO: add tests
...
[15 matches; head_limit=250 applied]
```

### WebFetch

```json
{"url": "https://ollama.com/docs/api"}
```

Permission prompt appears:
```
WebFetch wants to fetch https://ollama.com/docs/api — allow?
```

Returns stripped text content:
```
Ollama API Documentation
...
[truncated at 50000 chars]
```

### TodoWrite

```json
{
  "todos": [
    {"id": "1", "content": "Grep for TODO comments", "status": "completed", "priority": "high"},
    {"id": "2", "content": "Fix the flagged TODOs", "status": "in_progress", "priority": "high"},
    {"id": "3", "content": "Run tests", "status": "pending", "priority": "medium"}
  ]
}
```

Display:

```
- [x] Grep for TODO comments [high]
- [~] Fix the flagged TODOs [high]
- [x] Run tests [medium]
```

### TodoRead

Returns the current todo list in the same checklist format.

## Architecture

### Package Layout

```text
internal/tools/fileedit/
  fileedit.go
  fileedit_test.go

internal/tools/glob/
  glob.go
  glob_test.go

internal/tools/grep/
  grep.go
  grep_test.go

internal/tools/webfetch/
  webfetch.go
  webfetch_test.go

internal/tools/todo/
  todo.go
  todo_test.go
```

### Core Types

#### FileEdit

```go
// Input is the FileEdit tool input.
type Input struct {
    Path       string `json:"path"`
    OldString  string `json:"old_string"`
    NewString  string `json:"new_string"`
    ReplaceAll bool   `json:"replace_all,omitempty"`
}

// Output is the FileEdit tool output.
type Output struct {
    Path         string `json:"path"`
    OccurrencesReplaced int `json:"occurrences_replaced"`
    BytesBefore  int    `json:"bytes_before"`
    BytesAfter   int    `json:"bytes_after"`
}
```

#### Glob

```go
// Input is the Glob tool input.
type Input struct {
    Pattern  string `json:"pattern"`
    BasePath string `json:"base_path,omitempty"`
}

// Output is the Glob tool output.
type Output struct {
    Paths        []string `json:"paths"`
    TotalMatched int      `json:"total_matched"`
    Truncated    bool     `json:"truncated"`
}
```

#### Grep

```go
// Input is the Grep tool input.
type Input struct {
    Pattern      string `json:"pattern"`
    Path         string `json:"path,omitempty"`
    Include      string `json:"include,omitempty"`
    Exclude      string `json:"exclude,omitempty"`
    ContextLines int    `json:"context_lines,omitempty"`
    HeadLimit    int    `json:"head_limit,omitempty"`
}

// Match is a single grep result.
type Match struct {
    File    string `json:"file"`
    Line    int    `json:"line"`
    Content string `json:"content"`
    IsContext bool `json:"is_context,omitempty"`
}

// Output is the Grep tool output.
type Output struct {
    Matches      []Match `json:"matches"`
    AppliedLimit bool    `json:"applied_limit"`
    HeadLimit    int     `json:"head_limit"`
}
```

#### WebFetch

```go
// Input is the WebFetch tool input.
type Input struct {
    URL       string `json:"url"`
    MaxLength int    `json:"max_length,omitempty"`
}

// Output is the WebFetch tool output.
type Output struct {
    URL         string `json:"url"`
    StatusCode  int    `json:"status_code"`
    ContentType string `json:"content_type"`
    Text        string `json:"text"`
    Truncated   bool   `json:"truncated"`
}
```

#### Todo

```go
// TodoStatus is an enum for todo item status.
type TodoStatus string

const (
    TodoPending    TodoStatus = "pending"
    TodoInProgress TodoStatus = "in_progress"
    TodoCompleted  TodoStatus = "completed"
)

// TodoPriority is an enum for todo item priority.
type TodoPriority string

const (
    PriorityHigh   TodoPriority = "high"
    PriorityMedium TodoPriority = "medium"
    PriorityLow    TodoPriority = "low"
)

// TodoItem is a single item in the todo list.
type TodoItem struct {
    ID       string       `json:"id"`
    Content  string       `json:"content"`
    Status   TodoStatus   `json:"status"`
    Priority TodoPriority `json:"priority"`
}

// TodoList is a session-scoped, mutex-protected todo list.
type TodoList struct {
    mu    sync.RWMutex
    items []TodoItem
}

// TodoWriteInput is the input for TodoWrite.
type TodoWriteInput struct {
    Todos []TodoItem `json:"todos"`
}

// TodoReadInput is the input for TodoRead (empty).
type TodoReadInput struct{}
```

### tools.Context Additions

Add to `internal/tools/context.go`:

```go
// ReadFileSnapshot returns the snapshot content recorded by RecordFileSnapshot.
// Returns nil, false if no snapshot exists for the path.
ReadFileSnapshot func(path string) ([]byte, bool)

// TodoList is the session-scoped todo list for TodoWrite/TodoRead.
// Nil when todo tools are not registered.
TodoList *todo.TodoList
```

The import cycle is avoided because `tools.Context` holds a `*todo.TodoList` pointer. The `todo` package imports `tools` for `tools.Context`, but `tools` imports `todo` only through the `context.go` field — this would be a cycle. To break it:

- Define `TodoList` as an interface in `tools/context.go`:

```go
// TodoStore is the interface satisfied by todo.TodoList.
type TodoStore interface {
    Replace(items []any) error
    All() []any
}
```

- Or use `any` and type-assert in todo tool `call()`.
- Recommended: use `any` for `TodoList` in `tools.Context`, type-assert to `*todo.TodoList` in todo tool implementations. This avoids any import cycle.

### Auto-Excludes

Both Glob and Grep auto-exclude these directory prefixes by default:

```go
var defaultExcludes = []string{
    ".git",
    "node_modules",
    ".svn",
    "vendor",
    "dist",
}
```

These are checked against the first path component of each relative file path. They cannot be overridden via `tools.Context` in Phase 19; config-backed overrides belong to Phase 13.

### Binary File Detection (Grep)

Before searching a file, Grep reads the first 512 bytes. If any null byte (`\x00`) is present, the file is classified as binary and skipped. The display output notes how many binary files were skipped.

### HTML Stripping (WebFetch)

Use `golang.org/x/net/html` tokenizer (already an indirect dependency):

- Walk the token stream.
- Collect `TextToken` content from tokens whose ancestor elements are not `<script>`, `<style>`, or `<head>`.
- Join with spaces, collapse runs of whitespace, and trim.
- Preserve paragraph breaks by inserting `\n\n` after `</p>`, `</div>`, `</li>`, `</h1>`-`</h6>`.

This produces readable prose without requiring a full DOM parser or external HTML-to-text library.

### Private IP Validation (WebFetch)

Reject URLs whose resolved IP falls in private/loopback ranges unless `tools.Context` has `AllowLocalFetch=true` (not implemented in Phase 19 — add the flag to `tools.Context` as a reserved field with zero-value `false`):

Blocked ranges:

- `127.0.0.0/8` (loopback)
- `10.0.0.0/8` (private)
- `172.16.0.0/12` (private)
- `192.168.0.0/16` (private)
- `::1/128` (IPv6 loopback)
- `fc00::/7` (IPv6 unique local)

The check is performed on the URL host before dialing. DNS resolution is NOT performed for the check — the host string is checked with `net.ParseIP`. If the host is a domain name (not an IP), the check is skipped and the actual dial may still fail for other reasons. A Phase 20+ hardening task can add DNS-based validation.

### Diff Display (FileEdit)

FileEdit produces a minimal unified-diff-style display:

```
--- path/to/file
+++ path/to/file
@@ context @@
-old line 1
-old line 2
+new line 1
+new line 2
```

For `replace_all=true`, each replacement site is shown as a separate `@@` hunk. The diff display uses only standard library (`strings.Split`, line comparison). Do not import a diff library.

### Staleness Detection (FileEdit)

In `tools.Context`:

- `RecordFileSnapshot func(path string, content []byte)` already exists.
- Add `ReadFileSnapshot func(path string) ([]byte, bool)`.

In FileRead's `call()` function, after reading the file successfully, call:

```go
if ctx.RecordFileSnapshot != nil {
    ctx.RecordFileSnapshot(path, content)
}
```

This is a one-line patch to `internal/tools/fileread/fileread.go`.

In FileEdit's `call()`:

```go
if ctx.ReadFileSnapshot != nil {
    if snap, ok := ctx.ReadFileSnapshot(path); ok {
        current, _ := os.ReadFile(path)
        if !bytes.Equal(snap, current) {
            return Result{}, errors.New("FileEdit: staleness detected — file has changed since last read; re-read the file before editing")
        }
    }
}
```

The snapshot callbacks are wired in `state.App.ToolContext(ctx)` in `internal/state/app.go` using a session-scoped `map[string][]byte` protected by a `sync.RWMutex`.

## Implementation Plan

### Step 1 - tools.Context Extensions

Files:

- `internal/tools/context.go`
- `internal/tools/context_test.go`

Add to `tools.Context` struct:

```go
// ReadFileSnapshot returns the previously recorded snapshot for a path.
ReadFileSnapshot func(path string) ([]byte, bool)

// AllowLocalFetch permits WebFetch to reach loopback/private addresses.
AllowLocalFetch bool

// TodoList holds the session-scoped todo list (type any to avoid import cycle).
TodoList any
```

Update `DefaultContext` to leave these fields nil/zero (their zero values are correct).

Wire snapshot callbacks in `internal/state/app.go`:

```go
// In ToolContext(ctx):
snaps := make(map[string][]byte)
var snapMu sync.RWMutex
tc.RecordFileSnapshot = func(path string, content []byte) {
    snapMu.Lock()
    snaps[path] = append([]byte(nil), content...)
    snapMu.Unlock()
}
tc.ReadFileSnapshot = func(path string) ([]byte, bool) {
    snapMu.RLock()
    defer snapMu.RUnlock()
    b, ok := snaps[path]
    return b, ok
}
```

Tests:

- [x] `RecordFileSnapshot` stores bytes and `ReadFileSnapshot` retrieves them.
- [x] Overwriting a snapshot with new bytes replaces old bytes.
- [x] `ReadFileSnapshot` returns `false` for unknown path.
- [x] Concurrent read and write using `go test -race` passes.

### Step 2 - FileRead Snapshot Patch

Files:

- `internal/tools/fileread/fileread.go`
- `internal/tools/fileread/fileread_test.go`

After a successful file read, call `ctx.RecordFileSnapshot(path, content)`.

This is a one-line addition. The function reads the file into `[]byte` already; just pass it to the snapshot callback before converting to string for display.

Tests:

- [x] After FileRead call, `ReadFileSnapshot` returns the file contents.
- [x] If `RecordFileSnapshot` is nil, no panic occurs.

### Step 3 - FileEdit Tool

Files:

- `internal/tools/fileedit/fileedit.go`
- `internal/tools/fileedit/fileedit_test.go`

Key behaviors:

- [x] Read file contents on call entry.
- [x] If `ReadFileSnapshot` is set and snapshot differs from current file, return staleness error.
- [x] Normalize `old_string` whitespace before matching (trim trailing whitespace per line, normalize `\r\n` to `\n`).
- [x] Count occurrences of normalized `old_string` in normalized file content.
- [x] If count == 0: return "old_string not found" error.
- [x] If count > 1 and `replace_all=false`: return "old_string is not unique; use replace_all=true or provide more context" error.
- [x] Reject no-op edits where `old_string == new_string`.
- [x] Perform replacement (one occurrence or all) in the original (non-normalized) content to preserve original whitespace in surrounding code.
- [x] Write result atomically using `filewrite.AtomicWrite`.
- [x] Call `ctx.RecordFileSnapshot(path, newContent)` after successful write.
- [x] Return `Output` with `OccurrencesReplaced`, `BytesBefore`, `BytesAfter`.
- [x] Return diff-style `Display` string.
- [x] `IsDestructive=true`, `IsReadOnly=false`, `IsConcurrencySafe=false`.
- [x] `CheckPermissions` follows FileWrite precedent: `PermAllow`/`PermDeny`/`PermAsk` by mode.

Tests:

- [x] Simple replacement succeeds and diff shows correct change.
- [x] Staleness error returned when file modified externally after last read.
- [x] No-op rejection (`old_string == new_string`).
- [x] `old_string` not found returns clear error.
- [x] Non-unique `old_string` without `replace_all` returns ambiguity error.
- [x] `replace_all=true` replaces all occurrences.
- [x] Trailing whitespace normalization allows match despite minor formatting differences.
- [x] Atomic write: partial write failure does not corrupt file (integration test with a temp file).
- [x] Path outside working directory is rejected.
- [x] Symlink traversal outside working directory is rejected.
- [x] `go test -race` passes.

### Step 4 - Glob Tool

Files:

- `internal/tools/glob/glob.go`
- `internal/tools/glob/glob_test.go`

Key behaviors:

- [x] Resolve `base_path` (or use `ctx.WorkingDir` if empty) through `tools.ResolvePath`.
- [x] If `pattern` contains `**`, use `filepath.WalkDir` with manual pattern matching.
- [x] For patterns without `**`, use `filepath.Glob` relative to base path.
- [x] Auto-exclude `.git/`, `node_modules/`, `.svn/`, `vendor/`, `dist/` by path component.
- [x] Return sorted file paths (not directories, only files).
- [x] Apply 1000-file result limit; include `Truncated=true` and the count of omitted files in display if truncated.
- [x] Return paths relative to the base path for concise output.
- [x] `IsReadOnly=true`, `IsConcurrencySafe=true`, `IsDestructive=false`.
- [x] `CheckPermissions` returns `PermAllow` unconditionally (read-only).

Tests:

- [x] Simple `*.go` pattern matches Go files in base dir.
- [x] `**/*.go` matches files in subdirectories.
- [x] `.git/` directory is excluded from results.
- [x] `node_modules/` directory is excluded from results.
- [x] `vendor/` directory is excluded from results.
- [x] 1001 matching files results in truncation and correct count.
- [x] Empty pattern returns a clear error.
- [x] `base_path` outside working directory is rejected.
- [x] Results are sorted alphabetically.
- [x] Symlink inside working directory is followed.
- [x] `go test -race` passes.

### Step 5 - Grep Tool

Files:

- `internal/tools/grep/grep.go`
- `internal/tools/grep/grep_test.go`

Key behaviors:

- [x] Compile `pattern` as `regexp.MustCompile` — return error on invalid regex.
- [x] Walk files in `path` (default: `ctx.WorkingDir`) using `filepath.WalkDir`.
- [x] If `include` is provided, match file names against it using `filepath.Match`.
- [x] If `exclude` is provided, skip matching file names.
- [x] Before searching a file, read first 512 bytes and skip if any null byte is present (binary detection).
- [x] Auto-exclude `.git/`, `node_modules/`, `.svn/`, `vendor/`, `dist/` directories.
- [x] Read matching files line by line; emit matches with `ContextLines` before/after lines.
- [x] Apply `head_limit` (default 250 match-lines, not context lines); set `AppliedLimit=true` in output when truncated.
- [x] Output format: `filename:line_number:content` per match line (ripgrep-compatible).
- [x] `IsReadOnly=true`, `IsConcurrencySafe=true`, `IsDestructive=false`.
- [x] `CheckPermissions` returns `PermAllow` unconditionally.
- [x] Respect `ctx.EffectiveMaxResultChars()` for final display truncation.

Tests:

- [x] Literal pattern finds exact matches.
- [x] Regex pattern `[A-Z]{3,}` finds all-caps words.
- [x] Binary file detection: file with null bytes is skipped.
- [x] `include: "*.go"` restricts results to Go files.
- [x] `exclude: "*_test.go"` skips test files.
- [x] `.git/` directory excluded from walk.
- [x] `context_lines=2` returns 2 lines before and after each match.
- [x] `head_limit=5` stops at 5 match-lines and sets `AppliedLimit=true`.
- [x] Invalid regex returns an error, not a panic.
- [x] Empty directory returns empty result gracefully.
- [x] 0-byte file does not panic.
- [x] `go test -race` passes.

### Step 6 - WebFetch Tool

Files:

- `internal/tools/webfetch/webfetch.go`
- `internal/tools/webfetch/webfetch_test.go`

Key behaviors:

- [x] Validate URL: must be `http://` or `https://`; reject `file://`, `ftp://`, etc.
- [x] If URL host is an IP address, reject if it falls in private/loopback ranges (see architecture section).
- [x] Create `http.Client` with 30-second timeout, follow up to 3 redirects.
- [x] Set `User-Agent: nandocodego/0.1` on requests.
- [x] On success, inspect `Content-Type`:
  - If `text/html`, strip HTML using `golang.org/x/net/html` tokenizer.
  - If `application/json` or `text/plain`, use content as-is.
  - Other content types: return error "unsupported content type".
- [x] Truncate to `input.MaxLength` (default 50,000 chars). Set `Output.Truncated=true` when truncated.
- [x] `IsReadOnly=true`, `IsConcurrencySafe=true`, `IsDestructive=false`.
- [x] `CheckPermissions`: return `PermAsk` for all fetches regardless of permission mode. Exception: `PermissionBypassPermissions` — return `PermAllow`.
- [x] Do NOT log response body at any log level.

Tests:

- [x] `http://` URL with a test HTTP server returns stripped text.
- [x] HTML stripping removes `<script>` and `<style>` content.
- [x] HTML stripping preserves paragraph breaks.
- [x] `https://` scheme accepted; `file://` scheme rejected.
- [x] Private IP `127.0.0.1` rejected.
- [x] Private IP `192.168.1.1` rejected.
- [x] Domain names pass IP check (DNS not resolved in check).
- [x] Truncation at `max_length` sets `Truncated=true`.
- [x] `CheckPermissions` returns `PermAsk` in default mode.
- [x] `CheckPermissions` returns `PermAllow` in `bypassPermissions` mode.
- [x] 30-second timeout respected (use test server that delays).
- [x] Redirect limit of 3 is respected.
- [x] `go test -race` passes.
- [x] Test uses a local `httptest.Server`; no external network calls.

### Step 7 - Todo Tool

Files:

- `internal/tools/todo/todo.go`
- `internal/tools/todo/todo_test.go`

Key behaviors:

- `TodoList` struct with `sync.RWMutex` and `[]TodoItem` items.
- `Replace(items []TodoItem) error` — validates all items, then atomically replaces the list.
- `All() []TodoItem` — returns a copy of the current list.
- `TodoWrite` tool calls `Replace`; `TodoRead` tool calls `All`.
- Validation in `Replace`:
  - [x] Every item must have a non-empty `ID`.
  - [x] Every item must have a non-empty `Content`.
  - [x] `Status` must be one of `pending`, `in_progress`, `completed`.
  - [x] `Priority` must be one of `high`, `medium`, `low`.
  - [x] Duplicate `ID` values within the list are rejected.
- `TodoWrite`:
  - [x] `IsReadOnly=false`, `IsConcurrencySafe=false`, `IsDestructive=false`.
  - [x] `CheckPermissions` returns `PermAllow` unconditionally.
  - [x] Replaces entire list (not append-only).
  - [x] Returns checklist display.
- `TodoRead`:
  - [x] `IsReadOnly=true`, `IsConcurrencySafe=true`, `IsDestructive=false`.
  - [x] `CheckPermissions` returns `PermAllow` unconditionally.
  - [x] Returns checklist display of current list.

Session wiring:

In `internal/cli/repl.go` (REPL startup), construct a `*todo.TodoList` and set `toolCtx.TodoList = todoList`. The two tools cast `ctx.TodoList.(interface{ ... })` to access it.

Checklist display format:

```
- [x] Task content [high]
- [~] In-progress task [medium]
- [x] Pending task [low]
```

Where `[x]` = completed, `[~]` = in_progress, `[ ]` = pending.

Tests:

- [x] `TodoWrite` with valid items stores them; `TodoRead` returns same items.
- [x] `TodoWrite` replaces entire list (old items gone after replace).
- [x] Duplicate ID in `TodoWrite` returns error.
- [x] Invalid `Status` value returns error.
- [x] Invalid `Priority` value returns error.
- [x] Empty item `Content` returns error.
- [x] `TodoRead` on empty list returns empty display gracefully.
- [x] Concurrent `TodoWrite` and `TodoRead` via `go test -race` passes.
- [x] `TodoList` is nil in `tools.Context` by default — `TodoWrite` returns a clear "todo list not initialized" error rather than panicking.

### Step 8 - Register All 7 New Tools in builtin.go

Files:

- `internal/tools/builtin/builtin.go`
- `internal/tools/builtin/builtin_test.go`

Add to `New()` function:

```go
fileedit.NewFileEditTool(),
glob.NewGlobTool(),
grep.NewGrepTool(),
webfetch.NewWebFetchTool(),
todo.NewTodoWriteTool(),
todo.NewTodoReadTool(),
```

Import the six new packages.

Tests:

- [x] Registry after `New()` contains exactly the expected tool names.
- [x] All 9 registered tools (3 original + 6 new; WebFetch and Todo are 2 tools) pass `tools.ValidateTool`.
- [x] No duplicate tool names in registry.

Wait — counting: Bash, FileRead, FileWrite (3 original) + FileEdit, Glob, Grep, WebFetch, TodoWrite, TodoRead (6 new) = 9 total. The prompt says "7 new tools" — FileEdit, Glob, Grep, WebFetch, TodoWrite, TodoRead = 6 tool instances, but TodoWrite and TodoRead are separate tool registrations. With a `NewWebFetchTool` and separate `NewTodoWriteTool`/`NewTodoReadTool`, the total new registrations are 6. The plan description says "register all 7 new tools" counting the two Todo tools as two separate tools plus FileEdit, Glob, Grep, WebFetch = 4, total 6. The exact count is 6 new tool registrations. Align the test to verify 9 total (3 + 6).

### Step 9 - allowed-deps.txt Update

If `golang.org/x/net` is imported as a direct dependency from `internal/tools/webfetch`, add it to `tools/allowed-deps.txt`:

```
golang.org/x/net
```

Justification to record in `docs/PHASE-LOG.md`: HTML stripping for WebFetch tool uses `golang.org/x/net/html` tokenizer. Package was already an indirect dependency; Phase 19 promotes it to a direct import.

Run after adding:

```sh
go mod tidy
tools/check-allowed-deps.sh
```

### Step 10 - Tests, Race, and Verification

Required commands:

```sh
go test ./internal/tools/fileedit/...
go test ./internal/tools/glob/...
go test ./internal/tools/grep/...
go test ./internal/tools/webfetch/...
go test ./internal/tools/todo/...
go test ./internal/tools/...
go test ./internal/tools/builtin/...
go test -race ./internal/tools/...
go test ./...
tools/check-allowed-deps.sh
tools/check-network-policy.sh
```

Manual smoke:

```sh
go run ./cmd/nandocodego --model qwen3 --no-alt-screen
```

Flow:

1. Ask: "Grep for TODO in this directory."
2. Verify Grep tool panel appears with results.
3. Ask: "Glob all `*_test.go` files."
4. Verify Glob tool panel appears with sorted paths.
5. Ask: "Edit the first test file to add a comment `// Phase 19 smoke test` at the top."
6. Verify FileEdit tool panel with diff display appears.
7. Verify the file is actually changed.
8. Ask: "Fetch https://ollama.com and summarize the page."
9. Approve the WebFetch permission prompt.
10. Verify stripped text appears in transcript.
11. Ask: "Create a todo list with what you just did."
12. Verify TodoWrite tool panel with checklist appears.
13. Ask: "Show me your todo list."
14. Verify TodoRead panel shows same items.

## Full Implementation Checklist

### Step 1 — tools.Context Extensions

- [x] Add `ReadFileSnapshot func(path string) ([]byte, bool)` to `tools.Context`.
- [x] Add `AllowLocalFetch bool` to `tools.Context`.
- [x] Add `TodoList any` to `tools.Context`.
- [x] Update `DefaultContext` comment to document new fields.
- [x] Wire `RecordFileSnapshot` and `ReadFileSnapshot` in `state.App.ToolContext` using session-scoped `map[string][]byte` protected by `sync.RWMutex`.
- [x] Test: `RecordFileSnapshot` stores; `ReadFileSnapshot` retrieves.
- [x] Test: unknown path returns `false`.
- [x] Test: concurrent access passes `go test -race`.

### Step 2 — FileRead Snapshot Patch

- [x] After successful read in `fileread.call()`, call `ctx.RecordFileSnapshot(path, rawBytes)`.
- [x] Guard: `if ctx.RecordFileSnapshot != nil`.
- [x] Test: snapshot is set after `FileRead.Call`.
- [x] Test: nil `RecordFileSnapshot` does not panic.

### Step 3 — FileEdit Tool

- [x] Create `internal/tools/fileedit/fileedit.go`.
- [x] Implement `Input`, `Output` types with JSON tags.
- [x] Implement `unmarshalInput` with validation (non-empty path, non-empty old_string).
- [x] Implement `checkPermissions` (mirrors FileWrite behavior).
- [x] Implement `normalizeForMatch(s string) string`: trim trailing whitespace per line, normalize CRLF to LF.
- [x] Implement `countOccurrences(haystack, needle string) int`.
- [x] Implement `call`: resolve path, read file, check staleness, normalize, count, reject no-op/not-found/ambiguous, replace, atomic write, record snapshot, build diff display, return Output.
- [x] Implement `buildDiffDisplay(path, before, after string) string`.
- [x] Implement `NewFileEditTool()` returning `tools.BuildTool(spec)`.
- [x] Set `IsConcurrentFunc: func(any) bool { return false }`.
- [x] Set `IsDestructiveFunc: func(any) bool { return true }`.
- [x] Create `internal/tools/fileedit/fileedit_test.go`.
- [x] Test: simple replacement success with diff display.
- [x] Test: staleness error when file externally modified.
- [x] Test: no-op rejection.
- [x] Test: old_string not found error.
- [x] Test: non-unique old_string without replace_all error.
- [x] Test: replace_all replaces all occurrences.
- [x] Test: whitespace normalization allows match despite trailing spaces.
- [x] Test: atomic write (partial failure does not corrupt).
- [x] Test: path outside working dir rejected.
- [x] Test: symlink escape rejected.
- [x] Test: `go test -race` passes.

### Step 4 — Glob Tool

- [x] Create `internal/tools/glob/glob.go`.
- [x] Implement `Input`, `Output` types.
- [x] Implement `unmarshalInput` with validation.
- [x] Implement `call`: resolve base path, dispatch to walkGlob or simpleGlob, apply excludes, sort, apply 1000-file limit, build display.
- [x] Implement `walkGlob(base, pattern string, excludes []string) ([]string, error)` for `**` patterns.
- [x] Implement `simpleGlob(base, pattern string, excludes []string) ([]string, error)` for simple patterns.
- [x] Implement `shouldExclude(path string, excludes []string) bool`.
- [x] Implement `matchDoubleStarPattern(pattern, path string) bool` using `filepath.Match` on path segments.
- [x] Set `IsConcurrentFunc: func(any) bool { return true }`.
- [x] Set `IsDestructiveFunc: func(any) bool { return false }`.
- [x] Set `IsReadOnlyFunc: func(any) bool { return true }`.
- [x] Implement `NewGlobTool()`.
- [x] Create `internal/tools/glob/glob_test.go`.
- [x] Test: `*.go` matches root Go files.
- [x] Test: `**/*.go` matches files in subdirs.
- [x] Test: `.git/` excluded.
- [x] Test: `node_modules/` excluded.
- [x] Test: `vendor/` excluded.
- [x] Test: 1001 files truncated to 1000 with `Truncated=true`.
- [x] Test: empty pattern returns error.
- [x] Test: base_path outside working dir rejected.
- [x] Test: results sorted alphabetically.
- [x] Test: `go test -race` passes.

### Step 5 — Grep Tool

- [x] Create `internal/tools/grep/grep.go`.
- [x] Implement `Input`, `Match`, `Output` types.
- [x] Implement `unmarshalInput`.
- [x] Implement `call`: validate pattern, compile regex, resolve search path, walk files, apply include/exclude filters, binary detect, search lines, apply context lines, apply head_limit, build display.
- [x] Implement `isBinaryFile(path string) bool`: read first 512 bytes, check for null byte.
- [x] Implement `searchFile(path string, re *regexp.Regexp, contextLines, headLimit int, matches *[]Match, count *int) bool`.
- [x] Implement `matchesInclude(filename, include string) bool`.
- [x] Implement `matchesExclude(filename, exclude string) bool`.
- [x] Implement `shouldExcludePath(relPath string, excludes []string) bool`.
- [x] Implement `buildGrepDisplay(matches []Match, appliedLimit bool, headLimit int) string`.
- [x] Default `HeadLimit` = 250 when input `HeadLimit` == 0.
- [x] Cap `ContextLines` at 5.
- [x] Set `IsConcurrentFunc: func(any) bool { return true }`.
- [x] Set `IsReadOnlyFunc: func(any) bool { return true }`.
- [x] Implement `NewGrepTool()`.
- [x] Create `internal/tools/grep/grep_test.go`.
- [x] Test: literal pattern finds exact match.
- [x] Test: regex finds pattern.
- [x] Test: binary file skipped.
- [x] Test: `include: "*.go"` restricts results.
- [x] Test: `exclude: "*_test.go"` skips test files.
- [x] Test: `.git/` excluded from walk.
- [x] Test: context_lines=2 includes surrounding lines.
- [x] Test: head_limit=5 truncates and sets `AppliedLimit=true`.
- [x] Test: invalid regex returns error.
- [x] Test: 0-byte file no panic.
- [x] Test: `go test -race` passes.

### Step 6 — WebFetch Tool

- [x] Create `internal/tools/webfetch/webfetch.go`.
- [x] Implement `Input`, `Output` types.
- [x] Implement `unmarshalInput` with URL required validation.
- [x] Implement `validateURL(u string) error`: scheme check, private IP check.
- [x] Implement `isPrivateIP(host string) bool`: parse IP, check ranges.
- [x] Implement `fetchURL(ctx context.Context, u string, maxLen int) (Output, error)`: client, request, read, detect content type, strip HTML or use as-is, truncate.
- [x] Implement `stripHTML(r io.Reader) string` using `golang.org/x/net/html` tokenizer.
- [x] Implement `checkPermissions`: `PermAsk` in default mode, `PermAllow` in `bypassPermissions`, `PermDeny` in `plan`/`dontAsk`.
- [x] Set `IsConcurrentFunc: func(any) bool { return true }`.
- [x] Set `IsReadOnlyFunc: func(any) bool { return true }`.
- [x] Implement `NewWebFetchTool()`.
- [x] Create `internal/tools/webfetch/webfetch_test.go`.
- [x] Test: local httptest.Server with HTML returns stripped text.
- [x] Test: HTML with `<script>` content strips script.
- [x] Test: `file://` scheme rejected.
- [x] Test: `127.0.0.1` rejected.
- [x] Test: `192.168.1.1` rejected.
- [x] Test: domain name not rejected by IP check.
- [x] Test: truncation at `max_length`.
- [x] Test: `CheckPermissions` default = `PermAsk`.
- [x] Test: `CheckPermissions` bypassPermissions = `PermAllow`.
- [x] Test: redirect limit.
- [x] Test: `go test -race` passes.
- [x] No `httptest` calls to external URLs.

### Step 7 — Todo Tool

- [x] Create `internal/tools/todo/todo.go`.
- [x] Implement `TodoStatus`, `TodoPriority` types and constants.
- [x] Implement `TodoItem` struct with JSON tags.
- [x] Implement `TodoList` struct with `sync.RWMutex` and `items []TodoItem`.
- [x] Implement `TodoList.Replace(items []TodoItem) error` with validation.
- [x] Implement `TodoList.All() []TodoItem` returning a copy.
- [x] Implement `TodoWriteInput`, `TodoReadInput` types.
- [x] Implement `buildChecklistDisplay(items []TodoItem) string`.
- [x] Implement `NewTodoWriteTool()` with `IsConcurrentFunc=false`, `CheckPermFunc=PermAllow`.
- [x] Implement `NewTodoReadTool()` with `IsConcurrentFunc=true`, `IsReadOnlyFunc=true`, `CheckPermFunc=PermAllow`.
- [x] Both tools cast `ctx.TodoList` to `*TodoList` with type assertion; return error if nil.
- [x] Wire `TodoList` in `internal/cli/repl.go`: create `todo.NewTodoList()`, set `toolCtx.TodoList = todoList`.
- [x] Create `internal/tools/todo/todo_test.go`.
- [x] Test: Write valid items; Read returns same items.
- [x] Test: Write replaces all items (old items disappear).
- [x] Test: Duplicate ID rejected.
- [x] Test: Invalid status rejected.
- [x] Test: Invalid priority rejected.
- [x] Test: Empty content rejected.
- [x] Test: Read on empty list returns empty display.
- [x] Test: Concurrent Write and Read passes `go test -race`.
- [x] Test: nil TodoList in context returns clear error.

### Step 8 — builtin.go Registration

- [x] Import 5 new packages in `internal/tools/builtin/builtin.go`.
- [x] Add 6 new `NewXxx()` calls to `New()` function.
- [x] Test: registry contains expected 9 tool names.
- [x] Test: all tools pass `tools.ValidateTool`.
- [x] Test: no duplicate names.

### Step 9 — Dependency Update

- [x] Add `golang.org/x/net` to `tools/allowed-deps.txt` with comment.
- [x] Run `go mod tidy`.
- [x] Run `tools/check-allowed-deps.sh` and confirm pass.
- [x] Run `tools/check-network-policy.sh` and confirm pass.

### Step 10 — Final Verification

- [x] `go test ./...` passes.
- [x] `go test -race ./internal/tools/...` passes.
- [x] `go test -race ./internal/...` passes.
- [x] `go vet ./...` passes.
- [x] `tools/check-allowed-deps.sh` passes.
- [x] `tools/check-network-policy.sh` passes.
- [x] Manual smoke session completes all 7 exit-gate steps.
- [x] `docs/PHASE-LOG.md` Phase 19 entry added.

## Acceptance Criteria

- [x] `internal/tools/fileedit` exists with atomic write, staleness detection, fuzzy normalization, diff display, no-op rejection, and pathsafe containment.
- [x] FileEdit `replace_all=true` replaces all occurrences; without it, ambiguous matches return an error.
- [x] FileEdit staleness error fires when the file has changed since last `FileRead` call.
- [x] FileEdit rejects `old_string == new_string` (no-op).
- [x] FileEdit rejects paths outside the working directory.
- [x] FileEdit diff display shows `---`/`+++` header and `-`/`+` line markers.
- [x] `internal/tools/glob` exists with `**` support, auto-excludes, 1000-file limit, and sorted output.
- [x] Glob auto-excludes `.git/`, `node_modules/`, `.svn/`, `vendor/`, `dist/`.
- [x] Glob `**/*.go` correctly matches Go files in all subdirectories.
- [x] Glob 1001 files truncates with correct count and `Truncated=true`.
- [x] `internal/tools/grep` exists with regex, binary detection, context lines, head-limit pagination.
- [x] Grep `include`/`exclude` filters apply to file names.
- [x] Grep `context_lines` works from 0 to 5.
- [x] Grep `head_limit` defaults to 250 and surfaces `AppliedLimit=true` when used.
- [x] Grep skips binary files (null bytes in first 512 bytes).
- [x] Grep output format is `filename:line:content` (ripgrep-compatible).
- [x] `internal/tools/webfetch` exists with URL validation, HTML stripping, 30s timeout, truncation, permission gating.
- [x] WebFetch rejects `127.0.0.1`, `192.168.x.x`, `10.x.x.x`, `172.16-31.x.x` IP addresses.
- [x] WebFetch `CheckPermissions` returns `PermAsk` in default mode.
- [x] WebFetch HTML stripping removes `<script>`, `<style>` content.
- [x] WebFetch truncates at `max_length` (default 50,000 chars).
- [x] `internal/tools/todo` exists with `TodoList`, `TodoWrite`, `TodoRead`.
- [x] `TodoWrite` replaces entire list atomically.
- [x] `TodoRead` returns a copy (not the internal slice).
- [x] TodoList validation rejects invalid status, priority, duplicate IDs, empty content.
- [x] TodoList checklist display uses `[x]`/`[~]`/`[ ]` notation.
- [x] All 6 new tools registered in `internal/tools/builtin/builtin.go`.
- [x] All tools pass `tools.ValidateTool` invariant checks.
- [x] `go test -race ./internal/tools/...` passes.
- [x] `go test ./...` passes.
- [x] `tools/check-allowed-deps.sh` passes.
- [x] `tools/check-network-policy.sh` passes.
- [x] `docs/PHASE-LOG.md` Phase 19 entry records files, decisions, and exit gate.

## Forbidden

- WebSearch or any tool that calls an external search API endpoint.
- Agent/sub-agent tool (Phase 11).
- LLM calls inside any Phase 19 tool implementation.
- Logging response bodies from WebFetch at any log level.
- Auto-merging or persisting TodoList to disk.
- Importing a third-party diff library (use stdlib `strings` for diff display).
- Importing a third-party HTML parser beyond `golang.org/x/net/html`.
- Enabling WebFetch for private/loopback IPs without `AllowLocalFetch=true`.
- Bypassing `tools.ResolvePath` in any file-path tool.
- Adding Grep or Glob tools that use `exec.Command("grep")` or `exec.Command("find")` — use stdlib `filepath` and `regexp` only.
- Infinite recursion or unbounded memory in Glob `WalkDir` (apply directory depth limit or rely on OS).

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---:|---|
| FileEdit normalizes whitespace incorrectly and corrupts code | High | Normalize only trailing whitespace and CRLF; run full test suite on Go source files. |
| FileEdit staleness check false-positive after same tool writes | Medium | After a successful write, update the snapshot via `RecordFileSnapshot(path, newContent)`. |
| Glob `**` matching is O(n*m) on large directories | Medium | Apply 1000-file hard cap; WalkDir returns early once limit is hit. |
| Grep reads huge files into memory line by line | Medium | Read files in chunks; skip files above a size threshold (e.g., 10 MB). |
| WebFetch response body is massive (e.g., JSON dump) | High | Hard truncation at `MaxLength` before returning. |
| WebFetch DNS rebinding: domain resolves to private IP at request time | Medium | Document the gap; Phase 20+ can add DNS-based validation. Current check is IP-only for literal IP URLs. |
| TodoList nil panic if not initialized | High | Guard with type assertion and return clear error; never panic. |
| golang.org/x/net import cycle | Low | Only WebFetch imports it; no circular dependency with other tool packages. |
| Import cycle between `tools` package and `todo` package | Medium | Use `any` type for `TodoList` field in `tools.Context`; type-assert in todo tools. |
| Binary file detection misses some binaries | Low | Null-byte heuristic covers ELF, PE, PDF, etc. Edge cases (UTF-16 without null bytes) accepted for v0.1. |

## Phase Log Template

When implementation finishes, append a Phase 19 entry to `docs/PHASE-LOG.md` with:

- objective and ordering rationale (before Phase 18);
- files created;
- new direct dependencies and allowlist status;
- test commands run and results;
- manual smoke session result;
- design decisions (diff format choice, null-byte binary detection, IP-only loopback check, `any` TodoList in Context);
- known gaps and deferred work (WebSearch deferred, DNS-based WebFetch validation deferred, config-backed Glob excludes deferred);
- exit gate status.

## Exit Gate

Phase 19 is complete only when:

- all acceptance criteria above are checked;
- `go test ./...` and `go test -race ./internal/tools/...` pass;
- `tools/check-allowed-deps.sh` and `tools/check-network-policy.sh` pass;
- the manual smoke REPL session exercises all 6 new tool families successfully;
- `docs/PHASE-LOG.md` records the Phase 19 implementation with any deviations from this plan.

Phase 19 must be marked complete before Phase 18 (hardening/eval) begins.
