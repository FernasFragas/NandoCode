# Phase 3 Detailed Plan - Tool Interface and Starter Tools

Date: 2026-05-02
Status: Implemented and verified
Source plan: `.codex/go-ollama-plan-AGENTS.md`

## Goal

Phase 3 should introduce the first real action surface for the project: a common tool API plus three starter tools that can read files, write files, and run bounded shell commands.

The goal is not to build the agent loop. Phase 4 owns model-driven tool orchestration. Phase 3 should leave behind a small, tested tool foundation that Phase 4 can call without revisiting safety fundamentals.

Deliverables:

- `internal/tools` core package with a self-describing `Tool` interface.
- Fail-closed defaults for read-only, concurrency, destructive, and permission classification.
- Deterministic tool registry.
- Conversion from tools to `llm.ToolDef`.
- Safe path containment helpers.
- `Bash`, `FileRead`, and `FileWrite` implementations.
- Unit and integration-style tests for classification, permissions, path safety, process cancellation, result limits, and atomic writes.
- `examples/oneshot-tool/main.go` proving the tools work without the Phase 4 agent loop.

## Baseline Analysis

### Phase 0 - Security and Supply Chain

Implemented:

- `SECURITY.md` defines the local-first but not sandboxed security posture.
- `tools/allowed-deps.txt` establishes an allowlist for direct Go dependencies.
- `tools/check-allowed-deps.sh` enforces direct dependency policy.
- `tools/check-network-policy.sh` scans source files for unauthorized hardcoded endpoints.
- GitHub Actions, Dependabot, dependency review, security scan, and security issue template exist.
- `docs/PHASE-LOG.md` records the security baseline and remaining security contact placeholder.

Phase 3 implications:

- Tools are the first model-facing code that can touch shell and filesystem. They must be treated as privileged.
- `Bash` must not become an unreviewed network escape hatch.
- File writes need explicit permission classification and atomic-write guarantees.
- Any new direct dependency must already be allowlisted or intentionally added with rationale.
- `mvdan.cc/sh/v3` is already allowlisted and is the intended Bash parser dependency.

Observed baseline issue, resolved during implementation:

- On macOS, `tools/check-network-policy.sh` printed `grep: invalid option -- P` because it used GNU-only `grep -P`. Phase 3 replaced this with portable extended grep and fixed line parsing.

### Phase 1 - Repository Scaffold and CLI

Implemented:

- `go.mod` and `go.sum` exist with module path `github.com/FernasFragas/Nandocode`.
- `Makefile` includes build, test, race, lint, install, clean, check, and Docker-related targets.
- Cobra CLI exists in `internal/cli`.
- `cmd/nandocodego/main.go` calls `cli.Execute()`.
- `internal/version`, `internal/paths`, and `internal/logging` are implemented.
- Empty future package directories exist for `agent`, `permissions`, `state`, `hooks`, `mcp`, `tui`, and others.

Phase 3 implications:

- Tool packages should follow the project layout from the plan: `internal/tools/<toolname>/<toolname>.go`.
- Tests should be colocated with source as `<source>_test.go`.
- Phase 3 should not add CLI integration beyond the example harness.
- Phase 3 should not create implementations for future phases just to satisfy imports.

Important constraint:

- The original Phase 3 sketch references future packages such as `internal/permissions`, `internal/hooks`, `internal/mcp`, `internal/state`, `internal/ids`, and `internal/fscache`. Those directories are present but have no types. Phase 3 should not depend on them yet.

### Phase 2 - LLM Client

Implemented:

- `internal/llm/types.go` defines `Message`, `ToolCall`, `ToolDef`, `ChatRequest`, streaming events, model info, pull progress, and `Client`.
- `internal/llm/ollama` implements Ollama chat, embeddings, model list, and model pull.
- Retry, watchdog, and model capability packages exist with tests.
- `examples/chat/main.go` demonstrates streaming chat.

Phase 3 implications:

- Tool schemas should convert directly into `llm.ToolDef`.
- Tool results should be shaped so Phase 4 can serialize them into `llm.Message` values with `RoleTool`.
- Phase 3 tests should not require a live Ollama instance.
- Phase 3 should not call the LLM client from tools.

Current verification result:

```bash
env GOCACHE=/private/tmp/nandocodego-gocache go test ./...
tools/check-allowed-deps.sh
```

Both pass.

Resolved implementation caveat:

```bash
tools/check-network-policy.sh
```

It now exits successfully without BSD `grep` errors on macOS.

## Evaluation of the Original Phase 3 Plan

The original Phase 3 plan is directionally correct:

- It chooses the right first tools: `Bash`, `FileRead`, and `FileWrite`.
- It requires input-dependent safety classification.
- It requires `mvdan.cc/sh/v3/syntax` for Bash parsing.
- It requires atomic file writes.
- It requires a Bash permission matrix of at least 30 commands.
- It requires a non-agent example harness.

The plan needs more detail before implementation:

- The `Context` sketch imports future packages that do not exist yet. Phase 3 needs a minimal context that compiles now and can grow later.
- `BuildTool` needs a Go-specific design. Interfaces do not have default methods, so the implementation needs either a `ToolSpec` plus wrapper or explicit helper defaults.
- Registry and built-in registration need an import-cycle-safe layout. A core `tools` package cannot import child packages that already import `tools`.
- Path containment needs exact symlink and missing-parent behavior.
- `Bash` read-only classification needs a concrete command policy and parse-failure behavior.
- `FileRead` needs text/binary/directory/truncation decisions.
- `FileWrite` needs a concrete atomic write algorithm and test strategy for interrupted writes.
- The security scripts need a pre-flight hardening task because the network checker is currently noisy on macOS.

## Final Phase 3 Scope

In scope:

- Core `internal/tools` API.
- Minimal Phase 3 permission modes inside `internal/tools`.
- Registry and built-in registration.
- JSON schema conversion to `llm.ToolDef`.
- Path safety helpers.
- `Bash`, `FileRead`, and `FileWrite`.
- Unit tests and integration-style tests.
- Example harness under `examples/oneshot-tool`.
- Phase log update after implementation.

Out of scope:

- Full agent loop.
- Interactive permission prompts.
- Full `internal/permissions` package.
- Hooks.
- MCP tools.
- TUI rendering.
- Memory.
- File edit tool.
- Grep, glob, web fetch, and web search tools.
- Browser or server UI.
- Ollama end-to-end tests.

## Architecture Decisions

### Package Layout

Use this structure:

```text
internal/tools/
  tool.go
  context.go
  registry.go
  schema.go
  pathsafe.go
  builtin/
    builtin.go
    builtin_test.go
  bash/
    bash.go
    bash_test.go
    classify.go
    classify_test.go
  fileread/
    fileread.go
    fileread_test.go
  filewrite/
    filewrite.go
    filewrite_test.go
    atomic.go
    atomic_test.go
examples/oneshot-tool/
  main.go
```

Reasoning:

- `internal/tools` holds shared contracts only.
- Tool implementations import `internal/tools`.
- `internal/tools/builtin` imports the child tool packages and registers them. This avoids an import cycle between core `tools` and tool implementations.

### Core Tool Interface

Define in `internal/tools/tool.go`:

```go
type Permission int

const (
    PermAllow Permission = iota
    PermDeny
    PermAsk
)

type PermissionResult struct {
    Decision     Permission
    Reason       string
    UpdatedInput any
}

type ProgressEvent struct {
    Tool    string
    Stream  string
    Message string
    Data    any
}

type RenderHints struct {
    Title   string
    Summary string
}

type Result struct {
    Data           any
    Display        string
    NewMessages    []llm.Message
    MaxResultChars int
}

type Tool interface {
    Name() string
    Description() string
    Aliases() []string
    JSONSchema() map[string]any
    UnmarshalInput(raw json.RawMessage) (any, error)

    IsEnabled(ctx Context) bool
    IsReadOnly(input any) bool
    IsConcurrencySafe(input any) bool
    IsDestructive(input any) bool

    CheckPermissions(ctx Context, input any) PermissionResult
    Call(ctx Context, input any, progress chan<- ProgressEvent) (Result, error)

    Render(input any, result Result) RenderHints
}
```

Add helpers:

- `ValidateTool(t Tool) error`
- `ToLLMToolDef(t Tool) (llm.ToolDef, error)`
- `TruncateDisplay(s string, limit int) (string, bool)`

Validation rules:

- Tool name must be non-empty.
- Description must be no more than 1024 characters.
- First sentence must be no more than 100 characters.
- JSON schema must be non-nil.

### Fail-Closed Defaults

Because Go interfaces do not have defaults, implement a wrapper in one of two acceptable forms:

Preferred:

```go
type Spec struct {
    Name              string
    Description       string
    Aliases           []string
    Schema            map[string]any
    Unmarshal         func(json.RawMessage) (any, error)
    CallFunc          func(Context, any, chan<- ProgressEvent) (Result, error)
    IsReadOnlyFunc    func(any) bool
    IsConcurrentFunc  func(any) bool
    IsDestructiveFunc func(any) bool
    CheckPermFunc     func(Context, any) PermissionResult
    RenderFunc        func(any, Result) RenderHints
}

func BuildTool(spec Spec) Tool
```

Default behavior:

- `IsEnabled`: true.
- `IsReadOnly`: false.
- `IsConcurrencySafe`: false.
- `IsDestructive`: true for unknown input.
- `CheckPermissions`: ask in default mode unless read-only, deny in plan/dontAsk unless read-only.
- `Render`: tool name plus a short summary.

Alternative:

- Each concrete tool implements every method explicitly, and tests assert fail-closed behavior for malformed input.

Preferred approach is `BuildTool` because it matches the plan and prevents future tools from accidentally omitting safety methods.

### Minimal Phase 3 Context

Define a context that compiles without future packages:

```go
type PermissionMode string

const (
    PermissionDefault           PermissionMode = "default"
    PermissionBypassPermissions PermissionMode = "bypassPermissions"
    PermissionPlan              PermissionMode = "plan"
    PermissionDontAsk           PermissionMode = "dontAsk"
)

type Context struct {
    Context               context.Context
    Logger                *slog.Logger
    WorkingDir            string
    AdditionalWorkingDirs []string
    Env                   []string
    BashTimeout           time.Duration
    MaxResultChars        int
    MaxReadChars          int
    PermissionMode        PermissionMode
    RecordFileSnapshot    func(path string, content []byte)
}
```

Add:

- `DefaultContext(ctx context.Context, workingDir string) Context`
- `EffectiveContext() context.Context`
- `EffectiveBashTimeout() time.Duration`
- `EffectiveMaxResultChars() int`
- `EffectiveMaxReadChars() int`

Default values:

- Bash timeout: 30 seconds.
- Bash output limit: 30,000 characters.
- File read limit: 100,000 characters.
- Permission mode: `PermissionDefault`.

### Registry

Define `Registry` in `internal/tools/registry.go`.

Required methods:

- `NewRegistry() *Registry`
- `Register(t Tool) error`
- `Lookup(name string) (Tool, bool)`
- `All() []Tool`

Rules:

- Reject nil tools.
- Reject empty names.
- Reject duplicate canonical names.
- Reject duplicate aliases.
- Reject aliases that conflict with canonical names.
- `All()` returns a copy sorted alphabetically by canonical name.
- Lookup is exact and case-sensitive in Phase 3.

### Built-In Registration

Use `internal/tools/builtin/builtin.go`:

```go
func Register(reg *tools.Registry) error
func NewRegistry() (*tools.Registry, error)
```

Register:

- `bash.NewBashTool()`
- `fileread.NewFileReadTool()`
- `filewrite.NewFileWriteTool()`

Do not put built-in registration in the core `internal/tools` package if that creates an import cycle.

## Path Safety Plan

Create shared path helpers in `internal/tools/pathsafe.go`.

Required behavior:

- Relative paths resolve against `Context.WorkingDir`.
- Absolute paths are allowed only if contained in `WorkingDir` or `AdditionalWorkingDirs`.
- Existing paths are evaluated with `filepath.EvalSymlinks`.
- New write paths resolve the deepest existing parent, then join the remaining path.
- Symlink escapes are denied.
- Device-like paths are denied, including `/dev/null`, `/dev/random`, `/dev/zero`, `/dev/stdin`, and `/dev/fd/*`.
- Empty paths are denied.

Suggested API:

```go
func ResolvePath(ctx Context, requested string, mode PathMode) (string, error)

type PathMode int

const (
    PathRead PathMode = iota
    PathWrite
)
```

Testing matrix:

- Relative path inside root allowed.
- Absolute path inside root allowed.
- Path with `..` escaping root denied.
- Symlink inside root pointing outside denied.
- Missing write file under existing parent allowed.
- Missing write file under missing parent denied.
- `/dev/random` denied on Unix-like platforms.
- Additional working directory path allowed.

## FileReadTool Plan

Package: `internal/tools/fileread`

Input:

```go
type Input struct {
    Path   string `json:"path"`
    Offset int    `json:"offset,omitempty"`
    Limit  int    `json:"limit,omitempty"`
}
```

Output:

```go
type Output struct {
    Path      string `json:"path"`
    Content   string `json:"content"`
    Truncated bool   `json:"truncated"`
    SizeBytes int64  `json:"size_bytes"`
}
```

Phase 3 behavior:

- Always read-only for valid input.
- Always concurrency-safe for valid input.
- Permission result is allow for valid input.
- Reject paths outside allowed roots.
- Reject directories in Phase 3.
- Reject non-UTF-8 binary content in Phase 3.
- Apply `Offset`, `Limit`, and context read limit.
- Return clear errors for missing files, denied paths, malformed input, and unsupported binary content.

Optional but recommended:

- Include line-numbered display text for model-facing output.
- Keep raw content in typed output.

Tests:

- Valid read.
- Missing file.
- Directory rejected.
- Outside path rejected.
- Symlink escape rejected.
- UTF-8 binary/malformed content rejected.
- Truncation with `MaxReadChars`.
- Offset and limit.
- Malformed JSON input.
- Classification methods fail closed for wrong input type.

## FileWriteTool Plan

Package: `internal/tools/filewrite`

Input:

```go
type Input struct {
    Path    string `json:"path"`
    Content string `json:"content"`
}
```

Output:

```go
type Output struct {
    Path         string `json:"path"`
    BytesWritten int   `json:"bytes_written"`
    Created      bool  `json:"created"`
}
```

Phase 3 behavior:

- Not read-only.
- Not concurrency-safe.
- Destructive when overwriting an existing file.
- Ask in default mode.
- Deny in plan mode.
- Deny in dontAsk mode.
- Allow in bypassPermissions mode.
- Reject paths outside allowed roots.
- Do not create missing parent directories in Phase 3.
- Record prior content through `RecordFileSnapshot` before overwrite.

Atomic write algorithm:

1. Resolve and validate target path.
2. Stat parent directory and require it to exist.
3. Read prior target content if target exists.
4. Create temp file in the same directory.
5. Write full content to temp file.
6. `Sync` temp file.
7. Close temp file.
8. Rename temp file over target.
9. Best-effort sync parent directory.
10. Remove temp file on error.

Forbidden:

- `os.WriteFile(target, ...)` for final target writes.
- `os.OpenFile(target, os.O_TRUNC, ...)`.
- Writing through symlinks.

Tests:

- Create new file.
- Overwrite existing file.
- Snapshot callback receives prior contents.
- Outside path rejected.
- Missing parent rejected.
- Symlink target rejected.
- Permission classification by mode.
- Temp file cleanup on induced error where practical.
- Atomic stress test with repeated writes and concurrent reads.
- Subprocess `kill -9` test proving target is either old content or new content, never partial.

The `kill -9` test can be an integration-style test if it is too heavy for normal unit tests.

## BashTool Plan

Package: `internal/tools/bash`

Dependency:

- Add direct dependency `mvdan.cc/sh/v3` to `go.mod`.
- It is already allowlisted in `tools/allowed-deps.txt`.

Input:

```go
type Input struct {
    Command     string            `json:"command"`
    Description string            `json:"description,omitempty"`
    TimeoutMS   int               `json:"timeout_ms,omitempty"`
    Env         map[string]string `json:"env,omitempty"`
}
```

Output:

```go
type Output struct {
    Stdout   string `json:"stdout"`
    Stderr   string `json:"stderr"`
    ExitCode int    `json:"exit_code"`
    Duration string `json:"duration"`
}
```

Semantic validation:

- Command must be non-empty.
- Timeout must be positive when provided.
- Timeout must not exceed the context timeout cap.
- Environment variable names must be valid shell identifiers.
- Working directory must be configured.

Execution:

- Use `exec.CommandContext`.
- Use `sh -c` for portability unless the parser/execution strategy later chooses direct argv execution.
- Set `cmd.Dir` to the contained working directory.
- Use context cancellation and timeout.
- Capture stdout and stderr.
- Stream stdout/stderr chunks through `ProgressEvent`.
- Do not block if progress channel is nil.
- Do not deadlock if caller provides an unconsumed progress channel.
- Non-zero exit is a tool result, not a framework error.
- Context cancellation or timeout should return a structured result with a non-zero/sentinel exit code and explanatory stderr.

Classification:

- Parse with `mvdan.cc/sh/v3/syntax`.
- If parsing fails, not read-only, not concurrency-safe, ask or deny depending on permission mode.
- If command includes redirection, treat as not read-only unless explicitly proven safe later.
- If command includes command substitution, process substitution, heredoc, unknown expansions, or unsupported syntax, treat as not read-only in Phase 3.
- A command is read-only only if every simple command is read-only or neutral.
- A pipeline is read-only only if every command in the pipeline is read-only or neutral.
- A compound command is read-only only if every component is read-only or neutral.

Initial neutral commands:

- `echo`
- `printf`
- `true`
- `false`
- `pwd`

Initial read-only commands:

- `ls`
- `cat`
- `head`
- `tail`
- `wc`
- `sort`
- `uniq`
- `cut`
- `grep`
- `egrep`
- `fgrep`
- `rg`
- `find`
- `stat`
- `file`
- `du`
- `df`
- `env`
- `printenv`
- `which`
- `type`
- `date`
- `uname`

Read-only Git subcommands:

- `status`
- `diff`
- `log`
- `show`
- `branch`
- `remote`
- `rev-parse`
- `ls-files`
- `grep`

Read-only Go subcommands:

- `test`
- `list`
- `version`
- `env`

Known ask-required or destructive commands:

- `rm`
- `rmdir`
- `mv`
- `cp`
- `mkdir`
- `touch`
- `chmod`
- `chown`
- `dd`
- `tee`
- `truncate`
- `curl`
- `wget`
- `ssh`
- `scp`
- `sudo`
- `git reset`
- `git checkout -- file`
- `git clean`
- `git apply`
- `go get`
- `go install`
- `go mod tidy`

Permission behavior:

- `bypassPermissions`: allow.
- `plan`: allow read-only, deny non-read-only.
- `dontAsk`: allow read-only, deny non-read-only.
- `default`: allow read-only, ask non-read-only.

Tests:

- At least 30 Bash command cases.
- Every read-only command category.
- At least 10 known unsafe commands.
- Pipelines.
- `&&` and `||`.
- Redirection.
- Parse failure.
- Environment validation.
- Timeout/cancellation.
- Stdout/stderr capture.
- Non-zero exit.
- Working directory behavior.
- Progress events.

## Concrete Todos

### A. Pre-Flight Hardening

- [x] Fix `tools/check-network-policy.sh` to avoid GNU-only `grep -P`.
- [x] Fix `tools/check-network-policy.sh` parsing so filename, line number, and source line are handled separately.
- [x] Decide whether commented YAML examples should be ignored by the network checker.
- [x] Run:

```bash
env GOCACHE=/private/tmp/nandocodego-gocache go test ./...
tools/check-allowed-deps.sh
tools/check-network-policy.sh
```

### B. Core Tool API

- [x] Create `internal/tools/tool.go`.
- [x] Define `Permission`, `PermissionResult`, `ProgressEvent`, `RenderHints`, `Result`, and `Tool`.
- [x] Define `PermissionMode` in Phase 3 without importing future `internal/permissions`.
- [x] Implement `ValidateTool`.
- [x] Implement `ToLLMToolDef`.
- [x] Implement `TruncateDisplay`.
- [x] Implement `BuildTool` using `Spec`, or document why each concrete tool implements all methods explicitly.
- [x] Add `internal/tools/tool_test.go`.

### C. Context

- [x] Create `internal/tools/context.go`.
- [x] Define minimal Phase 3 `Context`.
- [x] Add `DefaultContext`.
- [x] Add effective default helpers for context, timeout, read limit, and result limit.
- [x] Add tests for default values and cancellation propagation.

### D. Registry

- [x] Create `internal/tools/registry.go`.
- [x] Implement `NewRegistry`.
- [x] Implement `Register`.
- [x] Implement `Lookup`.
- [x] Implement `All`.
- [x] Add duplicate-name and duplicate-alias tests.
- [x] Add deterministic sort-order test.

### E. Path Safety

- [x] Create `internal/tools/pathsafe.go`.
- [x] Implement path resolution for read and write modes.
- [x] Resolve symlinks for existing paths.
- [x] Resolve deepest existing parent for write paths.
- [x] Reject paths outside allowed roots.
- [x] Reject special device paths.
- [x] Add table-driven tests for containment and symlink escapes.

### F. FileRead

- [x] Create `internal/tools/fileread/fileread.go`.
- [x] Implement input and output structs.
- [x] Implement JSON schema.
- [x] Implement input parsing.
- [x] Implement classification.
- [x] Implement permission check.
- [x] Implement text read with limits.
- [x] Reject unsupported directories and binary files.
- [x] Add tests for behavior and malformed inputs.

### G. FileWrite

- [x] Create `internal/tools/filewrite/filewrite.go`.
- [x] Create `internal/tools/filewrite/atomic.go`.
- [x] Implement input and output structs.
- [x] Implement JSON schema.
- [x] Implement input parsing.
- [x] Implement classification by permission mode.
- [x] Implement atomic write.
- [x] Implement snapshot callback.
- [x] Add tests for create, overwrite, denial, snapshot, cleanup, and atomicity.
- [x] Add a `kill -9` interrupted-write test, likely under an integration build tag if needed.

### H. Bash

- [x] Add `mvdan.cc/sh/v3` to `go.mod`.
- [x] Create `internal/tools/bash/bash.go`.
- [x] Create `internal/tools/bash/classify.go`.
- [x] Implement input and output structs.
- [x] Implement JSON schema.
- [x] Implement input parsing and semantic validation.
- [x] Implement AST classification.
- [x] Implement permission behavior by mode.
- [x] Implement command execution with `exec.CommandContext`.
- [x] Implement stdout/stderr capture and progress events.
- [x] Implement result truncation.
- [x] Add Bash matrix tests with at least 30 commands.
- [x] Add execution tests for stdout, stderr, non-zero exit, timeout, cancellation, env, and working directory.

### I. Built-In Registration

- [x] Create `internal/tools/builtin/builtin.go`.
- [x] Register `Bash`, `FileRead`, and `FileWrite`.
- [x] Add tests proving all three are discoverable.
- [x] Add tests proving all three convert to `llm.ToolDef`.

### J. Example Harness

- [x] Create `examples/oneshot-tool/main.go`.
- [x] Use a temporary working directory.
- [x] Register built-in tools.
- [x] Run `FileWrite` in bypass mode to create a file.
- [x] Run `FileRead` to read it.
- [x] Run `Bash("ls")` and verify permission allow.
- [x] Check `Bash("rm -rf .")` returns ask or deny and is not executed.
- [x] Print concise demo output.

### K. Documentation and Phase Log

- [x] Update `docs/PHASE-LOG.md` after implementation.
- [x] Update `README.md` status after implementation.
- [x] Record deviations from `.codex/go-ollama-plan-AGENTS.md`.
- [x] Record known debt and confirm the `kill -9` FileWrite test is implemented under the integration build tag.

## Verification Plan

Required:

```bash
env GOCACHE=/private/tmp/nandocodego-gocache go test ./...
env GOCACHE=/private/tmp/nandocodego-gocache go test -race ./internal/tools/...
env GOCACHE=/private/tmp/nandocodego-gocache go vet ./...
tools/check-allowed-deps.sh
tools/check-network-policy.sh
go run ./examples/oneshot-tool
```

If tools are installed:

```bash
make lint
make check
```

No required Phase 3 test should need:

- Running Ollama.
- Network access.
- User interaction.
- A browser.

## Acceptance Criteria

Phase 3 is complete when:

- [x] `Bash`, `FileRead`, and `FileWrite` exist under `internal/tools`.
- [x] All three expose JSON schemas convertible to `llm.ToolDef`.
- [x] All three are registered and discoverable through a deterministic registry.
- [x] Permission classification is input-dependent.
- [x] Fail-closed behavior is covered by tests.
- [x] Bash classification uses `mvdan.cc/sh/v3/syntax`.
- [x] Bash read-only matrix covers at least 30 commands and at least 10 unsafe commands.
- [x] Bash uses `exec.CommandContext` and respects cancellation.
- [x] FileRead cannot read outside allowed roots.
- [x] FileWrite cannot write outside allowed roots.
- [x] FileWrite uses temp-file plus rename and never truncates the target directly.
- [x] FileWrite interrupted-write testing proves no partial target corruption.
- [x] `examples/oneshot-tool` proves safe command execution and blocks or asks for `rm -rf`.
- [x] `go test -race ./internal/tools/...` passes.
- [x] Phase 0 security checks pass without misleading warnings.

## Implementation Order

1. Fix network policy script portability.
2. Implement core `internal/tools` API and context.
3. Implement registry.
4. Implement path safety.
5. Implement `FileRead`.
6. Implement `FileWrite`.
7. Implement Bash classification.
8. Implement Bash execution.
9. Implement built-in registration.
10. Add example harness.
11. Run verification.
12. Update phase log and README.

## Risks

- Bash classification becomes too permissive. Mitigation: parse with `mvdan.cc/sh/v3`, deny/ask on unsupported syntax, and keep a broad command matrix.
- Path containment misses a symlink escape. Mitigation: resolve existing paths and deepest existing parents, then test escapes explicitly.
- Atomic write tests give false confidence. Mitigation: test repeated writes, concurrent reads, and subprocess kill behavior.
- Tool context grows too early. Mitigation: keep Phase 3 context minimal and defer hooks, MCP, state, permissions, and TUI integration.
- Registry creates import cycles. Mitigation: keep built-in registration in `internal/tools/builtin`.
- Phase 3 bleeds into Phase 4. Mitigation: no model loop, no interactive prompts, no Ollama calls, no TUI.
