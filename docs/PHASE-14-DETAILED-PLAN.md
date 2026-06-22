# Phase 14 Detailed Plan - Tasks: Background Bash & Async Agents

Date: 2026-05-07
Status: ✅ Implemented in code and automated checks (2026-05-08); manual live exit-gate pending
Source plans and references:

- `.codex/go-ollama-plan-AGENTS.md`
- `.codex/go-ollama-plan-HUMANS.md`
- `docs/PHASE-LOG.md`
- `docs/PROJECT-STATUS-AND-ONBOARDING.md`
- `docs/PHASE-8-DETAILED-PLAN.md`
- `docs/PHASE-9-DETAILED-PLAN.md`
- `book/ch05-agent-loop.md`
- `book/ch06-tools.md`
- `book/ch14-tasks.md`
- `.codex/agent-context/learnings-memory.md`

## Goal

Phase 14 implements a unified task supervisor that manages long-running background operations without blocking the REPL or agent loop. The user-visible goal is that the agent can launch a background bash command or a sub-agent run, receive its task ID immediately, continue the conversation, and later inspect status or output on demand.

Deliverables:

- `internal/tasks/` — supervisor, sealed state interface, JSONL output writer/reader, and kind enum.
- `internal/ids/` — kind-prefixed ID generation based on timestamp hex.
- `internal/types/task.go` — shared TaskSummary and TaskKind types for cross-package use.
- `internal/tools/tasktool/` — TaskCreate, TaskList, TaskGet, TaskOutput, and TaskStop tools, all registered in the builtin registry.
- JSONL output file per task under `~/.nandocodego/sessions/<session-id>/tasks/<task-id>.jsonl`.
- State integration: `state.App.Tasks` updated via `store.Set` on every task state transition.
- `/agents list` TUI slash command wired to TaskList filtered by KindAgent.
- Unit tests: supervisor lifecycle, goroutine non-leak, context cancellation, JSONL output, ID generation.

## Definition Of Success

The Phase 14 exit gate is a manual two-step flow:

1. In the REPL, ask the agent to run a long bash command in the background (e.g., `sleep 30 && echo done`).
2. Verify:
   - TaskCreate returns a task ID immediately without blocking the prompt.
   - `/agents list` or `TaskList` tool shows the task as `running`.
   - TaskStop cancels the task within 200 ms; subsequent TaskGet shows `killed`.
   - The JSONL output file exists in the session tasks directory and is readable during execution.
   - Starting a second REPL session while the first task is still tracked does not inherit or resume the previous task.

## Baseline Analysis From Implemented Phases

### Phase 0 — Security and Supply Chain

Implemented:

- `SECURITY.md`
- dependency allowlist
- network policy checker
- CI/security baseline
- no-secrets policy for logs, memory, telemetry, and test fixtures

Phase 14 implications:

- Task output JSONL files may contain stdout from arbitrary bash commands, including secrets inadvertently echoed. Output files must never be read into memory wholesale for logging.
- Task IDs, session IDs, and output file paths are safe to log at DEBUG; file contents are not.
- Any new direct dependency (e.g., `golang.org/x/sync` for errgroup in Phase 15) must be allowlisted before use. Phase 14 does not require errgroup; a simple `sync.WaitGroup` and goroutines suffice.
- Output files are stored under the user data directory (`NANDOCODEGO_DATA_HOME`). Symlink traversal checks must guard that directory as they do the memory directory.

### Phase 1 — CLI, Paths, Logging, Scaffold

Implemented:

- `cmd/nandocodego`
- Cobra root command
- `internal/paths`
- `internal/logging`
- empty future package directories, including `internal/tasks/` and `internal/ids/`

Phase 14 implications:

- Reuse `internal/paths` for session and task output directory resolution. Add `paths.SessionDir(sessionID string)` and `paths.TaskOutputFile(sessionID, taskID string)` if they do not already exist.
- Keep logging discipline: log task IDs, kinds, durations, exit codes, and error classes; never log task output body or command arguments that might contain secrets.
- The `doctor` command can add a `tasks: <n> running` health check using only local supervisor state.

### Phase 2 — LLM Client

Implemented:

- provider-neutral `llm.Client`
- streaming `Chat`
- `ChatRequest.Format` for structured outputs
- model list/pull/embed APIs
- retry/watchdog helpers

Phase 14 implications:

- KindAgent tasks wrap an `agent.Agent.Run` call. The task receives a pre-built `agent.Input` at creation time; the agent's `llm.Client` is already configured.
- Task-level retries are out of scope for Phase 14. The supervisor records a `FailedTask` immediately on non-zero exit or non-nil error.
- Watchdog behavior is inherited from the inner agent's configuration; the supervisor does not add a separate watchdog.

### Phase 3 — Tool Interface and Starter Tools

Implemented:

- `tools.Tool`
- `tools.Context`
- `Bash`, `FileRead`, and `FileWrite`
- path safety helpers
- built-in registry

Phase 14 implications:

- The five task tools (TaskCreate, TaskList, TaskGet, TaskOutput, TaskStop) are `tools.Tool` implementations registered in the builtin registry.
- Task tools do not perform file I/O on behalf of the user; they manipulate supervisor state and read output files. They must still route through the permission resolver for tool calls the agent makes.
- TaskCreate for `KindBash` accepts a command string and optional working directory; it must not bypass `Bash` tool permission rules. Alternatively, Phase 14 can require that background bash goes through TaskCreate only with explicit permission approval, separate from inline `Bash` calls.

### Phase 4 — Agent Loop

Implemented:

- `agent.Run(ctx, input) <-chan agent.Event`
- `agent.Input.SystemPrompt`
- assistant/thinking deltas
- tool execution loop
- terminal events and usage

Phase 14 implications:

- The agent loop is serial (one tool call at a time). TaskCreate is non-blocking from the agent's perspective: it returns the task ID as a tool result immediately, while the actual goroutine runs in the supervisor.
- The agent cannot await a task result within a single turn. To check a task, the agent must call TaskGet or TaskOutput in a subsequent turn.
- `agent.Terminal.Conversation` already carries full conversation history. KindAgent tasks can copy this into a child `agent.Input`.
- Phase 14 does not add concurrency to the agent loop itself; that is Phase 15.

### Phase 5 — Permission System

Implemented:

- canonical permission modes
- source-tagged rules
- central resolver
- TUI prompt callback extension point
- fail-closed prompt behavior

Phase 14 implications:

- TaskCreate for KindBash must honour the same permission rules as the inline Bash tool. The supervisor must record the permission decision alongside the task record.
- If the permission resolver denies task creation, TaskCreate returns an error result and no task is registered.
- TaskStop is considered a management operation, not a tool call against user files, and can use `DecisionAllow` by default with an optional `denyTaskStop` hook for future phases.
- KindAgent tasks inherit the parent session's permission mode and rules.

### Phase 6 — State Layer

Implemented:

- `bootstrap.State`
- `state.Store[state.App]`
- `state.App.Messages`
- `state.App.ToolSettings`
- `state.App.ToolContext(ctx)`
- `state.OnChange`

Phase 14 implications:

- Add `Tasks map[string]types.TaskSummary` to `state.App`. Key is task ID.
- The supervisor calls `store.Set` on each state transition to publish changes.
- TaskSummary in state is a lightweight snapshot: ID, Kind, Description, Status, StartedAt, FinishedAt, ExitCode, OutputFile. Do not store full output in state.
- Use copy-on-write map semantics: `store.Set(func(a state.App) state.App { ... })`.
- `state.OnChange` subscriptions in the TUI refresh the status bar or agent list pane when tasks change.

### Phase 7 — Bubble Tea TUI and REPL

Implemented:

- real no-args REPL
- prompt submission
- agent bridge
- permission modal
- transcript rendering
- minimal slash commands
- `--model`, `--ollama-url`, `--no-alt-screen`

Phase 14 implications:

- The TUI must not block on task operations. All supervisor interactions happen in tool call goroutines, not in `Update`.
- The status bar can show a running task count from `state.App.Tasks`.
- `/agents list` slash command reads `state.App.Tasks`, filters by `KindAgent`, and renders a table in the transcript area.
- No new modal or task-detail pane is required for Phase 14; text rendering in transcript is sufficient.

### Phase 8 — Memory

Implemented:

- `internal/memory` package with scan, recall, extraction, and runner integration.
- Per-project memory directory under `paths.MemoryDir(scopeRoot)`.
- MEMORY.md index, staleness warnings, LLM side-query recall.

Phase 14 implications:

- Memory recall and extraction run synchronously before and after agent turns. Task supervisor goroutines must not call memory APIs directly; memory is the caller's concern.
- If a KindAgent task runs a full agent turn, the task runner should optionally accept a `memoryWrapper` runner decorator consistent with Phase 8 runner integration.

### Phase 9 — Hooks

Implemented:

- `internal/hooks` snapshot hook system.
- `PreToolUse` and `PostToolUse` fully wired.
- `SessionStart`, `SessionEnd`, `UserPromptSubmit`, `Stop` at framework level.

Phase 14 implications:

- TaskCreate invocations fire `PreToolUse` hooks (the task tool is a tool). `PostToolUse` fires when the task ID is returned, not when the task finishes.
- There is no `TaskComplete` hook type in Phase 14. That is a future extension.
- Hook snapshots from session start apply to all task tool calls within that session.

### Phase 10 — MCP Tools

Implemented:

- MCP tool integration
- go-keyring, oauth2, fsnotify

Phase 14 implications:

- `KindMCP` is defined in the `TaskKind` enum as a reserved kind for future MCP server monitors, but not implemented in Phase 14.
- The supervisor type-switches on `TaskKind` internally; unknown kinds panic in development builds but return an error in production.

### Phase 11 — AgentTool and Sub-Agent Spawning

Implemented:

- `AgentTool` — spawns sub-agents inline within a turn.

Phase 14 implications:

- `KindAgent` tasks in Phase 14 use the same mechanics as `AgentTool` but run asynchronously. The task receives a pre-built `agent.Input` and a dedicated output writer.
- KindAgent task output captures agent events (text deltas, tool starts, tool results) as JSON lines rather than streaming to the TUI.
- Sub-agent runs that complete during a normal inline `AgentTool` call are not affected. Only tasks created via `TaskCreate` with `kind=a` use the supervisor.

### Phase 12 — SkillTool

Implemented:

- `SkillTool` — embedded skills runner.

Phase 14 implications:

- Skills can call task tools if they need non-blocking background work.
- Phase 14 does not run skills as background tasks.

### Phase 13 — Slash Commands, Config, --print Mode

Implemented:

- Full slash command dispatch in TUI.
- Config file loading with koanf/v2.
- `--print` non-interactive mode.

Phase 14 implications:

- `/agents list` is a Phase 13 slash command already partially wired. Phase 14 completes its data source by wiring it to `state.App.Tasks`.
- Task tool configuration (e.g., maximum concurrent tasks) can be loaded from the config file via koanf.
- `--print` mode does not launch background tasks; TaskCreate in `--print` mode returns an error result immediately.

## Evaluation Of The Original Phase 14 Concept

The original plan is correct at the product level:

- unified task supervisor
- sealed state interface
- non-blocking TaskCreate
- per-task JSONL output file
- kind-prefixed IDs
- five task tools
- store integration

It needs additional detail for this repo:

- It does not specify how TaskCreate for KindBash interacts with the existing permission resolver for Bash commands.
- It does not describe how KindAgent tasks receive their input (pre-built vs. constructed at creation time).
- It does not specify the goroutine lifecycle: what happens if the supervisor itself is garbage collected while a task runs.
- It does not define output file rotation or size caps.
- It does not specify the session directory layout or how session ID is determined.
- It does not define what happens when TaskCreate is called in `--print` mode.
- It does not specify the TaskOutput streaming behavior (tail vs. full read).

## Final Phase 14 Scope

In scope:

- `internal/tasks/supervisor.go` — task supervisor (Start, Stop, Get, List).
- `internal/tasks/state.go` — sealed TaskState interface and concrete types.
- `internal/tasks/output.go` — JSONL output writer and line reader.
- `internal/ids/ids.go` — kind-prefixed ID generation.
- `internal/types/task.go` — TaskSummary, TaskKind for cross-package use.
- `internal/tools/tasktool/tasktool.go` — five task tools.
- State integration: `state.App.Tasks map[string]types.TaskSummary`.
- `/agents list` wired to task supervisor via state.
- JSONL sentinel on task completion.
- Permission integration for KindBash task creation.
- Tests: supervisor lifecycle, goroutine non-leak, cancellation, JSONL, ID uniqueness.
- Phase log update.

Out of scope:

- KindMCP task implementation (type only reserved).
- KindRemote task implementation (type only reserved).
- Task output size caps or rotation (deferred to Phase 16 / operations).
- Task retry logic.
- Task dependency graphs or pipelines.
- Background extraction improvements using task supervisor.
- Web UI or remote task monitoring.
- Cross-session task persistence (tasks are in-memory; output files survive but state does not).
- Task scheduling or cron.
- Metrics and telemetry decorators (Phase 16).

## Target User Experience

### Launching a Background Task

The agent calls `TaskCreate` with a kind and a description:

```json
{
  "kind": "bash",
  "description": "run test suite",
  "command": "go test ./... 2>&1",
  "working_dir": "/home/user/project"
}
```

The tool returns immediately:

```json
{
  "task_id": "b-1a2b3c4d",
  "status": "running",
  "output_file": "~/.nandocodego/sessions/sess-abc123/tasks/b-1a2b3c4d.jsonl"
}
```

The REPL remains responsive. The agent can continue the conversation.

### Checking Task Status

```json
{ "task_id": "b-1a2b3c4d" }
```

Returns:

```json
{
  "task_id": "b-1a2b3c4d",
  "kind": "bash",
  "status": "running",
  "started_at": "2026-05-07T10:00:00Z",
  "pid": 12345,
  "output_tail": [
    {"ts": "2026-05-07T10:00:01Z", "stream": "stdout", "text": "ok  \tgithub.com/FernasFragas/nandocodego/internal/agent"}
  ]
}
```

### Stopping a Task

```json
{ "task_id": "b-1a2b3c4d" }
```

Returns within 200 ms:

```json
{ "task_id": "b-1a2b3c4d", "status": "killed" }
```

### JSONL Output Format

Each line of the output file is a JSON object:

```jsonl
{"ts":"2026-05-07T10:00:00.123Z","stream":"stdout","text":"starting tests\n"}
{"ts":"2026-05-07T10:00:01.456Z","stream":"stderr","text":"warning: deprecated flag\n"}
{"ts":"2026-05-07T10:00:05.789Z","kind":"exit","code":0}
```

The sentinel `{"kind":"exit","code":N}` is always the last line. Readers can tail the file and detect completion by watching for this sentinel.

### /agents list

```
/agents list
```

Renders a table in the transcript:

```
ID             Kind   Status     Description            Started
a-deadbeef     agent  completed  summarise PR #42       10:00:01
b-1a2b3c4d     bash   running    run test suite         10:00:00
```

## Architecture

### Package Layout

```text
internal/
  tasks/
    supervisor.go
    state.go
    output.go
    supervisor_test.go
    state_test.go
    output_test.go
  ids/
    ids.go
    ids_test.go
  types/
    task.go
  tools/
    tasktool/
      tasktool.go
      tasktool_test.go
```

### Core Types

```go
// internal/types/task.go

package types

import "time"

type TaskKind string

const (
    KindBash   TaskKind = "b"
    KindAgent  TaskKind = "a"
    KindMCP    TaskKind = "m"
    KindRemote TaskKind = "r"
)

// TaskStatus is a summary status for state.App.Tasks.
type TaskStatus string

const (
    TaskStatusPending   TaskStatus = "pending"
    TaskStatusRunning   TaskStatus = "running"
    TaskStatusCompleted TaskStatus = "completed"
    TaskStatusFailed    TaskStatus = "failed"
    TaskStatusKilled    TaskStatus = "killed"
)

// TaskSummary is the lightweight snapshot stored in state.App.Tasks.
type TaskSummary struct {
    ID          string
    Kind        TaskKind
    Description string
    Status      TaskStatus
    CreatedAt   time.Time
    StartedAt   time.Time
    FinishedAt  time.Time
    ExitCode    int
    OutputFile  string
    Err         string // serialisable error string; empty on success
}
```

```go
// internal/tasks/state.go

package tasks

import "time"

// TaskState is a sealed interface for the task state machine.
type TaskState interface{ isTaskState() }

type PendingTask struct {
    ID          string
    Kind        TaskKind
    Description string
    CreatedAt   time.Time
}

type RunningTask struct {
    PendingTask
    StartedAt  time.Time
    PID        int    // 0 for KindAgent (no OS PID)
    OutputFile string
}

type CompletedTask struct {
    RunningTask
    FinishedAt time.Time
    ExitCode   int
}

type FailedTask struct {
    RunningTask
    FinishedAt time.Time
    Err        error
}

type KilledTask struct {
    RunningTask
    FinishedAt time.Time
}

func (PendingTask) isTaskState()   {}
func (RunningTask) isTaskState()   {}
func (CompletedTask) isTaskState() {}
func (FailedTask) isTaskState()    {}
func (KilledTask) isTaskState()    {}
```

```go
// internal/tasks/supervisor.go

package tasks

import (
    "context"
    "io"
    "sync"

    "github.com/FernasFragas/nandocodego/internal/state"
    "github.com/FernasFragas/nandocodego/internal/types"
)

// RunFunc is a function that performs work for a task.
// It receives a context (canceled on Stop) and a writer for JSONL output.
type RunFunc func(ctx context.Context, out io.Writer) error

type Supervisor struct {
    mu         sync.RWMutex
    tasks      map[string]TaskState
    cancels    map[string]context.CancelFunc
    sessionDir string
    store      *state.Store[state.App]
}

func NewSupervisor(sessionDir string, store *state.Store[state.App]) *Supervisor

// Start registers a new task, spawns a goroutine, and returns the task ID.
// It is non-blocking; the RunFunc executes asynchronously.
func (s *Supervisor) Start(ctx context.Context, kind types.TaskKind, desc string, run RunFunc) (string, error)

// Stop cancels a running task's context. Returns error if task is not running.
func (s *Supervisor) Stop(id string) error

// Get returns the current state for a task.
func (s *Supervisor) Get(id string) (TaskState, bool)

// List returns a snapshot of all task states, sorted by creation time.
func (s *Supervisor) List() []TaskState
```

```go
// internal/ids/ids.go

package ids

import (
    "fmt"
    "sync/atomic"
    "time"

    "github.com/FernasFragas/nandocodego/internal/types"
)

var counter uint32

// New generates a unique task ID with the given kind prefix.
// Format: "<kind>-<timestamp-hex><counter-hex>"
// Example: "b-0196a3b20042"
func New(kind types.TaskKind) string {
    ts := uint32(time.Now().UnixMilli() & 0xFFFFFFFF)
    c := atomic.AddUint32(&counter, 1)
    return fmt.Sprintf("%s-%08x%04x", kind, ts, c&0xFFFF)
}
```

```go
// internal/tasks/output.go

package tasks

import (
    "bufio"
    "encoding/json"
    "io"
    "os"
    "time"
)

// OutputLine is one line of JSONL task output.
type OutputLine struct {
    Ts     time.Time `json:"ts"`
    Stream string    `json:"stream,omitempty"` // "stdout" or "stderr"
    Text   string    `json:"text,omitempty"`
    Kind   string    `json:"kind,omitempty"` // "exit"
    Code   int       `json:"code,omitempty"`
}

// OutputWriter writes JSONL lines to an output file.
type OutputWriter struct {
    f   *os.File
    enc *json.Encoder
}

// NewOutputWriter opens (or creates) the output file and returns a writer.
func NewOutputWriter(path string) (*OutputWriter, error)

// WriteText writes one line of stdout or stderr output.
func (w *OutputWriter) WriteText(stream, text string) error

// WriteExit writes the terminal sentinel with the exit code.
func (w *OutputWriter) WriteExit(code int) error

// Close flushes and closes the underlying file.
func (w *OutputWriter) Close() error

// TailLines reads the last n lines from a JSONL output file.
// Returns all lines if the file has fewer than n lines.
func TailLines(path string, n int) ([]OutputLine, error)
```

### State Integration

Add to `state.App`:

```go
Tasks map[string]types.TaskSummary
```

The supervisor calls `store.Set` using a helper:

```go
func (s *Supervisor) publishState(id string, summary types.TaskSummary) {
    s.store.Set(func(a state.App) state.App {
        tasks := make(map[string]types.TaskSummary, len(a.Tasks)+1)
        for k, v := range a.Tasks {
            tasks[k] = v
        }
        tasks[id] = summary
        a.Tasks = tasks
        return a
    })
}
```

### Tool Implementations

All five tools live in `internal/tools/tasktool/tasktool.go` and share a `*tasks.Supervisor` reference injected at construction.

**TaskCreate**

Input:
```json
{
  "kind": "bash",
  "description": "run tests",
  "command": "go test ./...",
  "working_dir": "/project"
}
```

- Validate kind is one of "bash" or "agent" (MCP and Remote are reserved).
- For KindBash: build a RunFunc that shells out using `mvdan.cc/sh/v3` (already a dependency), writes stdout/stderr as JSONL lines, and writes exit sentinel.
- For KindAgent: require an `agent_input_json` field containing a serialised `agent.Input`; build a RunFunc that constructs the agent and drains its event channel, writing events as JSONL lines.
- Call `supervisor.Start` with the RunFunc.
- Return task ID and output file path as tool result.
- Non-blocking: returns before task execution begins.

**TaskList**

Input: optional `{ "kind": "bash" }` filter.

- Call `supervisor.List()`.
- Return array of TaskSummary objects.
- Default to all kinds if no filter.

**TaskGet**

Input: `{ "task_id": "b-1a2b3c4d", "tail_lines": 20 }`

- Call `supervisor.Get(task_id)`.
- If running: call `output.TailLines` for the last `tail_lines` lines (default 20, cap 100).
- Return full TaskSummary plus output tail.

**TaskOutput**

Input: `{ "task_id": "b-1a2b3c4d", "max_lines": 100 }`

- Open the JSONL output file directly.
- Read up to `max_lines` lines from the file.
- Return lines as a structured array.
- Does not stream; returns a snapshot.

**TaskStop**

Input: `{ "task_id": "b-1a2b3c4d" }`

- Call `supervisor.Stop(task_id)`.
- Block for up to 200 ms for state to transition to Killed or Completed.
- Return new status.

### JSONL for KindAgent Output

When a KindAgent task runs, its RunFunc drains the `agent.Event` channel and writes events as JSONL lines:

```jsonl
{"ts":"...","kind":"text_delta","content":"Here is the summary..."}
{"ts":"...","kind":"tool_start","tool_name":"Bash","tool_id":"tool-0"}
{"ts":"...","kind":"tool_result","tool_id":"tool-0","ok":true}
{"ts":"...","kind":"terminal","reason":"completed","turns":2}
{"ts":"...","kind":"exit","code":0}
```

Exit code 0 means `TerminalCompleted`; non-zero codes map to other terminal reasons.

## Implementation Plan

### Step 1 — Add `Tasks` to `state.App`

Files:
- `internal/state/app.go` (or wherever `state.App` is defined)
- `internal/types/task.go`

- [ ] Define `TaskKind`, `TaskStatus`, and `TaskSummary` in `internal/types/task.go`.
- [ ] Add `Tasks map[string]types.TaskSummary` field to `state.App`.
- [ ] Ensure zero value is a nil map (handled in copy-on-write helpers).
- [ ] Write a test proving `state.OnChange` fires when Tasks map changes.
- [ ] Write a test proving Tasks map uses copy-on-write semantics (old snapshot not mutated).

### Step 2 — ID Generation

Files:
- `internal/ids/ids.go`
- `internal/ids/ids_test.go`

- [ ] Implement `ids.New(kind types.TaskKind) string` using timestamp hex plus atomic counter.
- [ ] Test: 10,000 generated IDs have no duplicates.
- [ ] Test: ID prefix matches the kind prefix exactly.
- [ ] Test: ID format matches regex `^[bam r]-[0-9a-f]{12}$` (12 hex chars).
- [ ] Test: IDs generated in concurrent goroutines are unique (race detector enabled).
- [ ] Benchmark: 1,000,000 ID generations complete under 1 s.

### Step 3 — Sealed TaskState

Files:
- `internal/tasks/state.go`
- `internal/tasks/state_test.go`

- [ ] Define `TaskState` sealed interface with `isTaskState()` private method.
- [ ] Implement `PendingTask`, `RunningTask`, `CompletedTask`, `FailedTask`, `KilledTask`.
- [ ] Add `ToSummary() types.TaskSummary` method on each concrete type.
- [ ] Test: `ToSummary` maps each concrete type to the correct `TaskStatus`.
- [ ] Test: type switch over TaskState covers all five cases without panic.

### Step 4 — JSONL Output Writer and Reader

Files:
- `internal/tasks/output.go`
- `internal/tasks/output_test.go`

- [ ] Implement `OutputWriter` with `NewOutputWriter`, `WriteText`, `WriteExit`, `Close`.
- [ ] Create output file with mode `0600`; create parent directory with `0700`.
- [ ] Flush after each line write (no buffering that could lose output on kill).
- [ ] Implement `TailLines(path string, n int) ([]OutputLine, error)`.
- [ ] Test: write 10 lines + exit; TailLines(path, 5) returns last 5 plus sentinel.
- [ ] Test: TailLines on an empty file returns empty slice.
- [ ] Test: TailLines on non-existent file returns error.
- [ ] Test: WriteExit writes `"kind":"exit"` with correct code.
- [ ] Test: concurrent writes to the same OutputWriter do not interleave partial JSON lines (use mutex or single-goroutine contract — document the contract).

### Step 5 — Supervisor Core

Files:
- `internal/tasks/supervisor.go`
- `internal/tasks/supervisor_test.go`

- [ ] Implement `NewSupervisor(sessionDir string, store *state.Store[state.App]) *Supervisor`.
- [ ] Implement `Start`: generate ID, create OutputWriter, transition to PendingTask then RunningTask, spawn goroutine, return ID.
- [ ] Goroutine lifecycle: defer transition to Completed/Failed/Killed; always call `publishState`; always close OutputWriter; never leak.
- [ ] Implement `Stop(id string) error`: look up cancel func, call it, return error if task not found or not running.
- [ ] Implement `Get(id string) (TaskState, bool)`: acquire read lock, return copy.
- [ ] Implement `List() []TaskState`: acquire read lock, return sorted copy.
- [ ] Test: Start returns ID immediately (non-blocking: task not finished before return).
- [ ] Test: goroutine that completes normally → CompletedTask with correct ExitCode.
- [ ] Test: goroutine that returns error → FailedTask with correct Err.
- [ ] Test: Stop cancels context → goroutine returns → KilledTask within 200 ms.
- [ ] Test: Stop on non-existent ID returns error.
- [ ] Test: Stop on already-completed task returns error.
- [ ] Test: List returns tasks sorted by creation time ascending.
- [ ] Test: supervisor does not leak goroutines after task completion (use goleak or manual WaitGroup).
- [ ] Test: 100 concurrent Start calls all register unique IDs.
- [ ] Test: store.Set is called on each state transition (Pending→Running, Running→Completed/Failed/Killed).

### Step 6 — KindBash RunFunc

Files:
- `internal/tools/tasktool/tasktool.go`

- [ ] Implement `bashRunFunc(command, workingDir string) tasks.RunFunc`.
- [ ] Use `mvdan.cc/sh/v3` (already in go.mod) to run the command in a subprocess.
- [ ] Wire stdout and stderr to `OutputWriter.WriteText` with stream tags "stdout"/"stderr".
- [ ] Write exit sentinel on completion.
- [ ] Context cancellation: the subprocess receives SIGTERM/SIGKILL via `sh` runner's built-in context handling.
- [ ] Test: simple `echo hello` task writes one stdout line and exit sentinel.
- [ ] Test: `sleep 10` task stopped via Stop transitions to KilledTask within 200 ms.
- [ ] Test: command with non-zero exit code → CompletedTask with ExitCode != 0 (not FailedTask).

### Step 7 — KindAgent RunFunc

Files:
- `internal/tools/tasktool/tasktool.go`

- [ ] Implement `agentRunFunc(ag *agent.Agent, in agent.Input, out *tasks.OutputWriter) tasks.RunFunc`.
- [ ] Drain `ag.Run(ctx, in)` event channel, writing each event as a JSONL line.
- [ ] Map `agent.Terminal.Reason` to exit code: `TerminalCompleted=0`, all others `=1`.
- [ ] Test: KindAgent task with a fake agent that emits two text deltas and Terminal(Completed) → JSONL has two text_delta lines + terminal line + exit(0).
- [ ] Test: KindAgent task with Terminal(Aborted) → exit code 1.
- [ ] Test: context cancellation of KindAgent task propagates to the inner agent's context.

### Step 8 — Task Tool Implementations

Files:
- `internal/tools/tasktool/tasktool.go`
- `internal/tools/tasktool/tasktool_test.go`

- [ ] Implement `NewTaskTools(sup *tasks.Supervisor) []tools.Tool` returning all five tools.
- [ ] TaskCreate: validate kind; build RunFunc; call supervisor.Start; return tool result with task_id and output_file.
- [ ] TaskCreate: return error result (not panic) for unknown kind.
- [ ] TaskCreate: return error result in `--print` mode (check toolCtx for non-interactive flag).
- [ ] TaskList: call supervisor.List; marshal to JSON; optionally filter by kind parameter.
- [ ] TaskGet: call supervisor.Get; call TailLines; marshal to JSON with tail.
- [ ] TaskOutput: open output file; read up to max_lines; return as JSON array.
- [ ] TaskStop: call supervisor.Stop; poll up to 200 ms for terminal state; return new status.
- [ ] Register all five tools in the builtin registry during REPL bootstrap.
- [ ] Test: TaskCreate with kind="bash" → non-nil task ID in result.
- [ ] Test: TaskCreate with kind="unknown" → error result.
- [ ] Test: TaskList with no filter → includes all test tasks.
- [ ] Test: TaskList with kind filter → returns only matching tasks.
- [ ] Test: TaskGet for running task → includes tail lines.
- [ ] Test: TaskGet for unknown task ID → error result.
- [ ] Test: TaskOutput for completed task → all lines.
- [ ] Test: TaskStop for running task → status transitions to killed.
- [ ] Test: tool IsEnabled returns true for all five tools.
- [ ] Test: tool definitions are valid (Name, Description, InputSchema all non-empty).

### Step 9 — /agents list Integration

Files:
- `internal/tui/app.go` or slash command handler

- [ ] Wire `/agents list` to read `state.App.Tasks` and filter by `KindAgent`.
- [ ] Render a markdown table with columns: ID, Status, Description, Started, Finished.
- [ ] If no agent tasks exist, render "No agent tasks." message.
- [ ] Test: slash command with empty Tasks map renders no-tasks message.
- [ ] Test: slash command with two KindAgent tasks renders table with two rows.
- [ ] Test: slash command with mixed kinds renders only KindAgent rows.

### Step 10 — Session Directory Wiring

Files:
- `internal/cli/repl.go`
- `internal/paths/paths.go`

- [ ] Generate a session ID at REPL start (e.g., `ids.New("sess")` or UUID-based helper).
- [ ] Add `paths.SessionDir(sessionID string) string` if it does not exist.
- [ ] Add `paths.TaskOutputFile(sessionID, taskID string) string`.
- [ ] Pass session directory to `NewSupervisor` at REPL composition root.
- [ ] Ensure session directory is created with mode `0700` before first task creation.
- [ ] Test: paths are stable across multiple calls with the same session ID.
- [ ] Test: task output file path contains both session ID and task ID.

### Step 11 — Status Bar Integration

Files:
- `internal/tui/app.go`

- [ ] Subscribe to `state.OnChange` in TUI to detect task count changes.
- [ ] Show `[N tasks]` badge in the status bar when N > 0 running tasks.
- [ ] Clear badge when all tasks are in terminal state.
- [ ] Test: status bar view renders task count correctly for 0, 1, 3 running tasks.

### Step 12 — Tests, Benchmarks, and Manual Smoke

Required commands:

```sh
go test ./internal/tasks/...
go test ./internal/ids/...
go test ./internal/types/...
go test ./internal/tools/tasktool/...
go test ./internal/tui/...
go test ./internal/agent/...
go test -race ./internal/tasks/...
go test -race ./internal/tools/tasktool/...
go test -bench=BenchmarkIDGen ./internal/ids/
tools/check-allowed-deps.sh
tools/check-network-policy.sh
```

Manual smoke:

```sh
go run ./cmd/nandocodego --model qwen3 --no-alt-screen
```

Flow:

1. Ask the agent to run `sleep 10 && echo done` in the background.
2. Verify task ID returned immediately.
3. Ask the agent to list tasks. Verify running status.
4. Ask the agent to stop the task.
5. Verify `killed` status in TaskGet output.
6. Inspect the JSONL output file at the returned path.
7. Start a new REPL session. Verify tasks from previous session are not visible.

## Acceptance Criteria

- [ ] `internal/tasks/supervisor.go` exists with `NewSupervisor`, `Start`, `Stop`, `Get`, `List`.
- [ ] `internal/ids/ids.go` exists and generates IDs with correct kind prefix.
- [ ] 10,000 generated IDs in sequential calls have no duplicates.
- [ ] IDs generated concurrently from 100 goroutines have no duplicates (`go test -race`).
- [ ] `state.App.Tasks` is updated on every task state transition via `store.Set`.
- [ ] `store.Set` uses copy-on-write map semantics; old snapshots are not mutated.
- [ ] TaskCreate returns a task ID without blocking (verified by timing test: < 50 ms).
- [ ] TaskCreate for `kind="bash"` launches a subprocess; JSONL output file contains at least one line within 500 ms.
- [ ] TaskStop cancels a running task's context within 200 ms and transitions it to KilledTask.
- [ ] KilledTask and FailedTask both appear in TaskList output with distinct status values.
- [ ] TaskList without filter returns all tasks sorted by creation time.
- [ ] TaskGet for a running task returns output tail of at most 100 lines.
- [ ] TaskOutput returns lines read directly from the JSONL file.
- [ ] JSONL output file exists and is readable during task execution (concurrent file access from reader and writer).
- [ ] JSONL exit sentinel `{"kind":"exit","code":N}` is always the last line written.
- [ ] Supervisor goroutines do not leak after task completion (verified by goroutine count or goleak).
- [ ] Supervisor handles 100 concurrent `Start` calls without data races (`go test -race`).
- [ ] Output files are created with mode `0600`; session directories with mode `0700`.
- [ ] TaskCreate returns an error result (not panic) for reserved kinds `"mcp"` and `"remote"`.
- [ ] TaskCreate returns an error result in `--print` (non-interactive) mode.
- [ ] `/agents list` renders only KindAgent tasks and shows the correct row count.
- [ ] Status bar shows running task count when tasks are active.
- [ ] All targeted tests pass: `go test ./internal/tasks/... ./internal/ids/... ./internal/types/... ./internal/tools/tasktool/...`.
- [ ] `go test -race ./internal/tasks/...` passes.
- [ ] `tools/check-allowed-deps.sh` passes (no new dependencies required for Phase 14).
- [ ] `tools/check-network-policy.sh` passes (task output writes are local; no new network calls).
- [ ] `docs/PHASE-LOG.md` has a Phase 14 entry recording files, decisions, and any deferred work.
- [ ] Exit gate manual flow works end-to-end with a real Ollama model.

## Forbidden

- Blocking the REPL or Bubble Tea `Update` on task operations.
- Writing task output to `state.App.Tasks` (only lightweight summaries go in state).
- Running recall, memory extraction, or LLM calls inside the supervisor goroutines without an explicit opt-in from the caller.
- Logging task output body, command arguments containing secrets, or raw file contents.
- Auto-merging task output into the conversation history without an explicit agent tool call.
- Cross-session task persistence (tasks are bound to the supervisor's lifetime; output files survive but state does not reload on restart).
- Implementing KindMCP or KindRemote task execution in Phase 14.
- Adding task scheduling, dependency graphs, or retry logic.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---:|---|
| Goroutine leak on task panic | High | Recover panics in the supervisor goroutine wrapper; always close OutputWriter. |
| Task output file contains secrets | High | Never log output body; file mode 0600; document in SECURITY.md. |
| Race condition in state transitions | High | `sync.RWMutex` on supervisor task map; copy-on-write in `store.Set`. |
| TaskStop does not return within 200 ms | Medium | Subprocess SIGKILL fallback after 150 ms; document in KindBash RunFunc. |
| Session directory grows unbounded | Low | Phase 14 defers rotation; add cleanup script note in Phase 16 ops work. |
| KindBash bypasses Bash tool permissions | High | TaskCreate must call permission resolver before spawning; deny on DecisionDeny. |
| TaskGet for very large output file is slow | Low | TailLines reads only the last n lines; cap at 100. |
| JSONL writer and reader concurrent access | Medium | Writer owns file exclusively; reader opens a separate file descriptor. |

## Phase Log Template

When implementation finishes, append a Phase 14 entry to `docs/PHASE-LOG.md` with:

- objective;
- files created/updated;
- dependencies added and allowlist status;
- tests/benchmarks/checks run;
- manual smoke result;
- design decisions (especially permission integration for KindBash);
- known constraints and deferred work (KindMCP, KindRemote, rotation, retry);
- exit gate status.

## Implementation Reconciliation (2026-05-08)

Phase 14 has now been reconciled in source with focused closure of the remaining gaps.

Implemented in this closure pass:

- Task model + app-state alignment:
  - `types.TaskKind` now includes reserved `mcp` and `remote` kinds;
  - `state.App.Tasks` moved to copy-on-write map semantics (`map[string]types.TaskSummary`) with deep-clone coverage.
- Supervisor + runner hardening:
  - panic recovery in task goroutines with terminal failed-state publication;
  - start-path cleanup when output writer initialization fails;
  - lock-safe publication flow and map-based state publishing;
  - non-zero bash command exits now map to `CompletedTask` with non-zero exit code (not failed).
- JSONL output correctness:
  - output writer now creates parent dirs, enforces `0700` dir and `0600` file modes, and serializes concurrent writes via mutex;
  - tail-reading and concurrent read/write behavior covered by tests.
- TaskTool behavior completion:
  - `TaskList` supports optional kind filtering;
  - reserved task kinds (`mcp`, `remote`) return explicit errors;
  - `TaskStop` polling window tightened to 200 ms target;
  - stop permission classification aligned to allow-by-default management semantics.
- `/agents list` UX completion:
  - filters only `KindAgent` tasks;
  - deterministic sort and expanded table columns (started/finished);
  - explicit no-agent-task message.
- Test coverage expansion:
  - added/expanded tests for IDs, task state summaries, output writer/tailer, supervisor lifecycle/concurrency/state publication, task tools, and `/agents list` filtering.

Automated checks run successfully:

- `env GOCACHE=/private/tmp/go-build-cache go test ./internal/tasks/... ./internal/ids/... ./internal/types/... ./internal/tools/tasktool/... ./internal/commands/... ./internal/state/... ./internal/tui/...`
- `env GOCACHE=/private/tmp/go-build-cache go test -race ./internal/tasks/... ./internal/tools/tasktool/... ./internal/ids/...`
- `env GOCACHE=/private/tmp/go-build-cache go test ./...`
- `tools/check-allowed-deps.sh`
- `tools/check-network-policy.sh`

Remaining item (manual/live):

- Live REPL/Ollama exit-gate validation for end-to-end background task creation/stop and per-session task isolation behavior.

## Exit Gate

Phase 14 is complete only when:

- all acceptance criteria above are met;
- targeted tests and race-detector checks pass;
- the manual two-step smoke flow works end-to-end with a real Ollama model;
- the phase log records the implementation and any deviations from this plan.
