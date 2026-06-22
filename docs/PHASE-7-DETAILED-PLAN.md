# Phase 7 Detailed Plan - Bubble Tea TUI and REPL

Date: 2026-05-03
Status: Final plan and implementation checklist
Source plans:

- `.codex/go-ollama-plan-AGENTS.md`
- `docs/PHASE-LOG.md`
- `docs/PHASE-1-DETAILED-PLAN.md`
- `docs/PHASE-3-DETAILED-PLAN.md`
- `docs/PHASE-4-DETAILED-PLAN.md`
- `docs/PHASE-5-DETAILED-PLAN.md`
- `docs/PHASE-6-DETAILED-PLAN.md`

## Goal

Phase 7 turns the library layers into a usable terminal REPL.

Running `nandocodego` with no subcommand should open a Bubble Tea interface where the user can type prompts, stream assistant output, see tool calls/results, respond to permission prompts, use a minimal slash-command set, and abort in-flight work with Ctrl-C.

The goal is not to build the full command framework, memory, hooks, MCP, skills, tasks, sub-agents, or polished Phase 13 config UX. Phase 7 should deliver a coherent interactive loop on top of the existing agent, permissions, tools, and state layers.

Deliverables:

- Bubble Tea root model in `internal/tui`.
- Cached Glamour markdown renderer.
- Basic transcript viewport and input composer.
- Agent-event bridge from `agent.Run` to Bubble Tea messages.
- Permission prompt modal with allow/deny/always-allow choices.
- Minimal Vim input mode support.
- Minimal slash commands: `/help`, `/clear`, `/exit`, `/model <name>`.
- `internal/cli/repl.go` wiring no-args execution into the REPL.
- Focused unit tests for model update behavior, slash commands, permission broker, and event reduction.
- Optional manual or scripted smoke flow documentation.

## Baseline Analysis

### Phase 0 - Security and Supply Chain

Implemented:

- Security posture and outbound network policy.
- Dependency allowlist and network policy scripts.
- CI/security workflow baseline.

Phase 7 implications:

- Adding Bubble Tea, Bubbles, Lip Gloss, and Glamour is allowed because they are already listed in `tools/allowed-deps.txt`.
- TUI must not introduce new network destinations. Only the configured Ollama endpoint is used by the LLM client.
- The TUI must not print secrets or full logs by default.
- Permission prompts must be fail-closed on cancellation, timeout, or UI exit.
- Do not execute shell commands from UI code. Tool execution remains inside the agent/tool path.

### Phase 1 - CLI, Paths, Logging

Implemented:

- Signal-aware `cmd/nandocodego/main.go`.
- `cli.Run(ctx, args)` and `ExitCode`.
- `doctor` and `version` subcommands.
- Path helpers and logger helpers.

Phase 7 implications:

- Change root command no-args behavior from help output to REPL launch.
- Preserve `nandocodego --help`, `nandocodego doctor`, and `nandocodego version`.
- Add only minimal flags needed to launch the REPL, such as `--model` and `--ollama-url` if desired.
- CLI owns constructing dependencies: bootstrap snapshot, Ollama client, built-in registry, agent, state store, and TUI program.
- Keep default `doctor` network-free.

### Phase 2 - LLM Client

Implemented:

- `llm.Client` interface.
- Ollama client and streaming events.
- Watchdog and usage counts.

Phase 7 implications:

- REPL uses `ollama.NewClient(snapshot.OllamaBaseURL)`.
- TUI must handle client setup/chat errors as terminal messages without crashing.
- Do not call LLM APIs inside Bubble Tea `Update`.
- Model availability validation is deferred except for optional `/model` local state changes. Full `/models` and `/pull` are Phase 13.

### Phase 3 - Tools

Implemented:

- Built-in registry for Bash, FileRead, FileWrite.
- Tool progress events and render hints.
- Tool context construction.

Phase 7 implications:

- REPL should register the Phase 3 built-in tools.
- Tool panels should display name, ID, summary, progress state, and final error/result summary.
- Do not render unbounded tool output; rely on already bounded agent/tool result messages.
- File snapshots and detailed diff UI are out of scope.

### Phase 4 - Agent Loop

Implemented:

- `agent.Run(ctx, input) <-chan agent.Event`.
- Assistant text/thinking deltas.
- Tool start/progress/result events.
- Retry notices and terminal events.
- Context cancellation support.

Phase 7 implications:

- Agent runs in a goroutine started by a Bubble Tea command or CLI wiring.
- A bridge goroutine drains the agent event channel and sends `agentEventMsg` into the Bubble Tea program.
- Ctrl-C should cancel the active agent run first; if no active run exists, it exits the REPL.
- Agent output should be reduced into `state.App` and transcript items rather than storing raw event slices.

### Phase 5 - Permissions

Implemented:

- Seven permission modes.
- Source-tagged rules.
- Resolver.
- Agent prompt callback field.

Phase 7 implications:

- Permission modal is the first real implementation of `permissions.PromptFunc`.
- Prompt callback must block only the agent goroutine, not Bubble Tea `Update`.
- `allow` approves the current call only.
- `deny` denies the current call.
- `always allow` should add a `permissions.SourceSession` allow rule to `state.App.PermissionRules` and approve the current call.
- Closing the REPL or canceling the active run should deny any outstanding prompt.

### Phase 6 - State Layer

Implemented:

- `bootstrap.State` and global bootstrap singleton.
- `state.Store[state.App]`.
- `state.App`, `ToolSettings`, `PermissionPrompt`, `ToolUse`, `TaskSummary`.
- `state.OnChange`.

Phase 7 implications:

- TUI model should own a `*state.Store[state.App]`.
- All mutations of `state.App` should clone before changing slices, maps, or pointers.
- TUI can keep ephemeral view-only fields, such as viewport dimensions and cached rendered strings, outside `state.App`.
- `state.OnChange` mirrors model, working dir, budgets, and permission state into bootstrap.
- The store does not guarantee every intermediate update to subscribers; Bubble Tea receives explicit event messages and updates the store/model directly.

## Evaluation of the Original Phase 7 Plan

The original Phase 7 plan has the right outcome:

- Bubble Tea root model.
- Agent event bridge.
- Cached Glamour renderer.
- Vim mode.
- Permission modal.
- CLI REPL wiring.
- Minimal slash commands.

It needs more implementation detail:

- It does not define how dependencies are constructed from bootstrap/state.
- It does not define how `permissions.PromptFunc` interacts with the TUI without blocking `Update`.
- It does not define state reduction from `agent.Event` into `state.App`.
- It does not specify transcript representation versus provider `llm.Message`.
- It does not define Ctrl-C behavior when no run is active.
- It does not define how to preserve current CLI subcommands.
- It does not define tests that avoid requiring a live terminal or Ollama.
- It does not define which Bubble Tea dependencies are added and how allowlist checks stay green.

## Final Phase 7 Scope

In scope:

- Add direct dependencies:
  - `github.com/charmbracelet/bubbletea`
  - `github.com/charmbracelet/bubbles`
  - `github.com/charmbracelet/lipgloss`
  - `github.com/charmbracelet/glamour`
- `internal/tui` root model and subcomponents.
- `internal/cli/repl.go`.
- No-args CLI opens REPL.
- Minimal prompt submission and streaming output.
- Tool panel summaries.
- Permission prompt modal and broker.
- Minimal Vim insert/normal mode.
- Minimal slash commands.
- Unit tests using fake agents/clients where possible.

Out of scope:

- Full slash command framework in `internal/commands`.
- `/models`, `/pull`, `/memory`, `/hooks`, `/permissions show`, `/skills`, `/agents`.
- Config file parsing.
- Model list validation through `/api/tags`.
- MCP, hooks, memory, skills, sub-agents, background tasks.
- Persistent transcript/session files.
- Full e2e PTY automation unless added without new dependencies.
- Highly polished visual theme beyond a readable, stable layout.

## Target Package Layout

```text
internal/tui/
  app.go
  messages.go
  transcript.go
  markdown.go
  styles.go
  vim.go
  permission.go
  slash.go
  bridge.go
  app_test.go
  vim_test.go
  permission_test.go
  slash_test.go
  transcript_test.go

internal/cli/
  repl.go
  repl_test.go
```

Do not create `internal/commands` in Phase 7 unless the implementation genuinely needs it. Phase 13 owns the command registry.

## TUI Dependencies

Add to `go.mod`:

```text
github.com/charmbracelet/bubbletea
github.com/charmbracelet/bubbles
github.com/charmbracelet/lipgloss
github.com/charmbracelet/glamour
```

Rules:

- These are already allowlisted.
- Run `go mod tidy`.
- Keep `tools/check-allowed-deps.sh` passing.
- Do not add PTY/e2e libraries in Phase 7 unless explicitly approved and allowlisted.

## REPL Wiring

Define in `internal/cli/repl.go`:

```go
type replOptions struct {
    model     string
    ollamaURL string
}

func newREPLCmd(ctx context.Context) *cobra.Command // optional if root RunE calls runREPL directly
func runREPL(ctx context.Context, cmd *cobra.Command, opts replOptions) error
```

Wiring steps:

1. Build `initial := bootstrap.DefaultInitial(workingDir)`.
2. Apply CLI flags:
   - `--model` overrides `initial.DefaultModel`.
   - `--ollama-url` overrides `initial.OllamaBaseURL`.
3. Call `bootstrap.InitGlobal(initial)` during startup if not already initialized.
4. Build `snap := bootstrap.Global().Snapshot()`.
5. Build `appState := state.DefaultApp(snap)`.
6. Build `store := state.NewStore(appState, state.OnChange)`.
7. Build `client := ollama.NewClient(snap.OllamaBaseURL)`.
8. Build built-in tool registry with `builtin.NewRegistry()`.
9. Build `agent.New(client, registry, agent.WithConfig(...))`.
10. Build `tui.New(...)`.
11. Run `tea.NewProgram(model, tea.WithAltScreen(), tea.WithFPS(60))`.

Root command behavior:

- `nandocodego` opens the REPL.
- `nandocodego --help` prints help.
- `nandocodego doctor` still runs doctor.
- `nandocodego version` still prints version.

## TUI Model Design

Root model fields:

```go
type Model struct {
    store      *state.Store[state.App]
    agent      AgentRunner
    program    ProgramSender // set after program construction or injected bridge
    viewport   viewport.Model
    input      textarea.Model
    renderer   *MarkdownRenderer
    styles     Styles
    width      int
    height     int
    cancelRun  context.CancelFunc
    promptBroker *PermissionBroker
    transcript []TranscriptItem
    err        error
}
```

Interfaces for testability:

```go
type AgentRunner interface {
    Run(context.Context, agent.Input) <-chan agent.Event
}

type ProgramSender interface {
    Send(tea.Msg)
}
```

Rules:

- `Update` mutates the model and store only.
- `Update` never calls `agent.Run`, `llm.Client`, `Tool.Call`, or shell execution directly.
- Long-running work starts in `tea.Cmd` or goroutines owned by bridge code.
- View rendering uses current model state; it should not block on channels.
- Keep layout stable: transcript viewport, input area, status line, optional modal.

## Messages

Define in `internal/tui/messages.go`:

```go
type agentEventMsg struct{ Event agent.Event }
type agentDoneMsg struct{}
type agentStartFailedMsg struct{ Err error }
type permissionPromptMsg struct{ Request permissionRequest }
type permissionResolvedMsg struct{ ID string }
type slashCommandMsg struct{ Command string; Args []string }
type tickMsg time.Time
```

Use Bubble Tea built-ins for:

- `tea.KeyMsg`
- `tea.WindowSizeMsg`

Rules:

- Agent event bridge sends `agentEventMsg` for every event.
- Agent terminal event clears `ActiveRun` and `cancelRun`.
- Permission prompt messages set `state.App.PermissionPrompt`.

## Transcript Model

Provider `llm.Message` is the agent history format. The TUI needs richer display records:

```go
type TranscriptKind string

const (
    TranscriptUser TranscriptKind = "user"
    TranscriptAssistant TranscriptKind = "assistant"
    TranscriptThinking TranscriptKind = "thinking"
    TranscriptTool TranscriptKind = "tool"
    TranscriptSystem TranscriptKind = "system"
)

type TranscriptItem struct {
    Kind      TranscriptKind
    ToolID    string
    ToolName  string
    Content   string
    Collapsed bool
    Error     string
}
```

Rules:

- User submissions append both `llm.Message{Role: llm.RoleUser}` to `state.App.Messages` and a user transcript item.
- Assistant deltas append to the last assistant transcript item or create one.
- Thinking deltas append to a collapsed thinking item.
- Tool starts create a tool item and `state.App.ActiveTools[id]`.
- Tool progress updates the tool item summary, not a full unbounded output stream.
- Tool result marks the tool item done and records error/result summary.
- Terminal detail becomes a system transcript item only for non-completed exits.

## Agent Event Reduction

Create reducer helpers:

```go
func reduceAgentEvent(app state.App, itemState []TranscriptItem, evt agent.Event) (state.App, []TranscriptItem)
```

Event handling:

- `AssistantTextDelta`: append content to transcript assistant item.
- `AssistantThinkingDelta`: append thinking to collapsed thinking item.
- `ToolUseStart`: add `ToolUse` with name and started time.
- `ToolUseProgress`: update summary with latest stream/message.
- `ToolUseResult`: mark tool done, set error if any.
- `RetryNotice`: set `LastRetryNotice` and add a compact system transcript note.
- `Terminal`: set `ActiveRun=false`, store terminal reason/detail/usage, clear active permission prompt.

Rules:

- Use `app.Clone()` before mutating app state.
- Do not append raw `agent.Event` to app state.
- Do not store full unbounded progress data in `state.App`.

## Prompt Submission

Submission steps:

1. User presses Enter in insert mode with non-empty input.
2. If input starts with `/`, route to slash-command handler.
3. Otherwise:
   - append user message to app state and transcript,
   - clear input buffer,
   - set `ActiveRun=true`,
   - create cancellable context,
   - build `agent.Input`:
     - model from `state.App.ActiveModel`,
     - messages from `state.App.Messages`,
     - tool context from `state.App.ToolContext(runCtx)`,
     - permission mode/rules from app state,
     - permission prompt from broker,
   - start agent bridge command.

Important:

- Add the user message exactly once.
- Agent history passed to `agent.Input.Messages` should include the newly appended user message.
- If an agent run is active, Enter should queue the prompt in `QueuedPrompts` or reject with a status message. Phase 7 should prefer queueing one or more prompts because `state.App.QueuedPrompts` already exists.

## Agent Bridge

Bridge design:

```go
func startAgentCmd(ctx context.Context, runner AgentRunner, input agent.Input) tea.Cmd
func drainAgentEvents(ctx context.Context, events <-chan agent.Event, send func(tea.Msg))
```

Rules:

- The bridge owns draining the event channel until closed or context canceled.
- It sends each event through `Program.Send`.
- It must not mutate Bubble Tea model state directly.
- On channel close, send `agentDoneMsg`.
- Context cancellation should stop the bridge promptly.

## Permission Prompt Broker

Define in `internal/tui/permission.go`:

```go
type PermissionBroker struct {
    send func(tea.Msg)
    mu sync.Mutex
    pending map[string]chan permissionDecision
}

func (b *PermissionBroker) PromptFunc() permissions.PromptFunc
func (b *PermissionBroker) Resolve(id string, decision permissionDecision)
func (b *PermissionBroker) CancelAll(reason string)
```

Prompt flow:

1. Agent calls `PermissionPromptFunc` from its goroutine.
2. Broker creates a prompt ID and response channel.
3. Broker sends `permissionPromptMsg` to Bubble Tea.
4. Bubble Tea stores `state.App.PermissionPrompt`.
5. User chooses:
   - `a` allow once,
   - `d` deny,
   - `A` always allow.
6. Bubble Tea calls broker resolve through a command.
7. Broker unblocks prompt function.
8. Prompt function returns `DecisionAllow` or `DecisionDeny`.

Always allow behavior:

- Add a session allow rule to `state.App.PermissionRules.AlwaysAllow`.
- Pattern should be conservative and visible:
  - For Bash: `Bash(<exact command>)` when possible.
  - For FileWrite: `FileWrite(<target>)`.
- Use `permissions.SourceSession`.

Failure behavior:

- If context is canceled, return deny.
- If REPL exits, `CancelAll` denies outstanding prompts.
- If an unknown prompt ID is resolved, ignore it.

## Vim Mode

Phase 7 implements a small state machine:

- Insert mode:
  - normal text entry,
  - Enter submits,
  - Esc switches to normal.
- Normal mode:
  - `i` enters insert,
  - `a` enters insert after cursor if supported by textarea,
  - `q` exits if no run active,
  - Ctrl-C aborts active run or exits.
- Visual mode:
  - define enum and no-op/escape handling only; full selection can wait.

Rules:

- Use `state.App.VimMode` as the durable mode.
- Keep cursor mechanics inside Bubbles textarea where possible.
- Do not implement a full editor in Phase 7.

## Slash Commands

Implement minimal command handling in `internal/tui/slash.go`:

| Command | Behavior |
| --- | --- |
| `/help` | Append a compact system transcript item listing Phase 7 commands. |
| `/clear` | Clear transcript and `state.App.Messages`. |
| `/exit` | Exit the REPL, canceling active run first. |
| `/model <name>` | Set `state.App.ActiveModel` and append a system transcript item. |

Rules:

- Unknown slash command appends a system transcript item with an error.
- `/model` does not call Ollama in Phase 7. Validation against `/api/tags` belongs to Phase 13.
- Slash command implementation should be small and easily replaceable by Phase 13.

## Markdown and Rendering

`internal/tui/markdown.go`:

```go
type MarkdownRenderer struct {
    width int
    renderer *glamour.TermRenderer
}

func NewMarkdownRenderer(width int) (*MarkdownRenderer, error)
func (r *MarkdownRenderer) Render(s string) string
func (r *MarkdownRenderer) Resize(width int) error
```

Rules:

- Do not instantiate Glamour renderer per frame.
- Recreate only on width changes.
- Cache rendered transcript strings if needed to avoid full re-render every keypress.
- Keep cards/panels simple; this is a terminal app, not a dashboard.
- Tool panels should be compact and stable.

## CLI Behavior

Update `internal/cli/root.go`:

- Root `RunE` calls `runREPL`.
- Add root flags:
  - `--model`
  - `--ollama-url`
  - optionally `--no-alt-screen` for testing/manual debugging.
- Keep help/version/doctor working.

Testing:

- `nandocodego --help` still returns help.
- `nandocodego version` still works.
- `nandocodego doctor` still works.
- Root command can be tested with an injected `runREPL` function or test option so unit tests do not open a real terminal.

## Test Plan

### TUI Unit Tests

- Initial model builds from `state.DefaultApp`.
- Window resize updates viewport/input dimensions.
- Typing text updates input buffer.
- Enter with normal prompt appends user transcript and starts run command.
- Enter with slash command routes to slash handler.
- Ctrl-C cancels active run before exiting.
- Ctrl-D exits when no modal is active.
- Agent text deltas append to assistant transcript.
- Tool start/progress/result update `state.App.ActiveTools` and transcript items.
- Terminal event clears active run and stores usage.

### Permission Tests

- Broker prompt sends `permissionPromptMsg`.
- Allow returns `permissions.DecisionAllow`.
- Deny returns `permissions.DecisionDeny`.
- Context cancellation returns deny.
- Always allow appends `SourceSession` allow rule and returns allow.
- Outstanding prompt is denied on REPL exit.

### Slash Tests

- `/help` appends help.
- `/clear` clears messages and transcript.
- `/exit` returns quit command or sets exit flag.
- `/model qwen3:14b` updates active model without network.
- Unknown command appends an error transcript item.

### Vim Tests

- Insert -> Esc -> normal.
- Normal `i` -> insert.
- Normal `q` exits only when no active run.
- Visual enum exists and Esc returns normal/insert according to chosen behavior.

### CLI Tests

- Root no-args dispatches REPL through injected test runner.
- `doctor` and `version` still bypass REPL.
- `--model` and `--ollama-url` are passed into REPL options.

### Manual Smoke Test

Manual smoke should verify:

- `nandocodego` opens the REPL.
- Typing a prompt streams a response.
- A read-only tool call renders a compact tool panel.
- A mutating tool call opens a permission prompt.
- Ctrl-C aborts an in-flight run.
- Ctrl-D or `/exit` exits.

## Concrete Todos

### A. Pre-Flight

- [ ] Run `env GOCACHE=/private/tmp/nandocodego-gocache GOMODCACHE=/private/tmp/nandocodego-gomodcache go test ./internal/bootstrap/... ./internal/state/... ./internal/permissions/... ./internal/agent/... ./internal/tools/...`.
- [ ] Confirm `internal/tui` has no existing files to preserve.
- [ ] Confirm Bubble Tea dependencies are listed in `tools/allowed-deps.txt`.
- [ ] Review `state.App`, `agent.Event`, and `permissions.PromptFunc`.

### B. Add Dependencies

- [ ] Add Bubble Tea dependency.
- [ ] Add Bubbles dependency.
- [ ] Add Lip Gloss dependency.
- [ ] Add Glamour dependency.
- [ ] Run `go mod tidy`.
- [ ] Run `tools/check-allowed-deps.sh`.

### C. Build TUI Skeleton

- [ ] Create `internal/tui/messages.go`.
- [ ] Create `internal/tui/app.go`.
- [ ] Define `Model`.
- [ ] Define `AgentRunner` interface.
- [ ] Define `ProgramSender` interface or bridge send function.
- [ ] Initialize viewport and textarea.
- [ ] Implement `Init`.
- [ ] Implement `Update` for window size, keys, agent messages, and permission messages.
- [ ] Implement `View`.

### D. Transcript and Rendering

- [ ] Create `internal/tui/transcript.go`.
- [ ] Define `TranscriptItem`.
- [ ] Implement assistant delta append helper.
- [ ] Implement thinking item helper.
- [ ] Implement tool item helpers.
- [ ] Create `internal/tui/markdown.go`.
- [ ] Create cached Glamour renderer.
- [ ] Create `internal/tui/styles.go`.
- [ ] Render transcript into viewport without per-frame renderer allocation.

### E. Agent Bridge

- [ ] Create `internal/tui/bridge.go`.
- [ ] Implement start-agent command.
- [ ] Implement event-drain bridge.
- [ ] Ensure bridge sends `agentEventMsg`.
- [ ] Ensure bridge sends done/failure messages.
- [ ] Ensure context cancellation stops draining promptly.

### F. Prompt Submission

- [ ] Implement Enter submission in insert mode.
- [ ] Append user `llm.Message`.
- [ ] Append user transcript item.
- [ ] Clear input buffer.
- [ ] Build `agent.Input` from app state.
- [ ] Set active run and cancel function.
- [ ] Queue or reject prompts submitted during active run.

### G. Permission Modal

- [ ] Create `internal/tui/permission.go`.
- [ ] Implement `PermissionBroker`.
- [ ] Implement `PromptFunc`.
- [ ] Implement prompt message handling in model update.
- [ ] Render modal overlay or reserved modal block.
- [ ] Implement allow once.
- [ ] Implement deny.
- [ ] Implement always allow with `SourceSession` rule.
- [ ] Cancel outstanding prompts on abort/exit.

### H. Vim Mode

- [ ] Create `internal/tui/vim.go`.
- [ ] Implement insert/normal/visual mode transitions.
- [ ] Wire Esc, i, a, q, Ctrl-C behavior.
- [ ] Keep text editing delegated to textarea.
- [ ] Add tests for transitions.

### I. Slash Commands

- [ ] Create `internal/tui/slash.go`.
- [ ] Implement `/help`.
- [ ] Implement `/clear`.
- [ ] Implement `/exit`.
- [ ] Implement `/model <name>`.
- [ ] Add unknown command handling.
- [ ] Add tests.

### J. CLI REPL Wiring

- [ ] Create `internal/cli/repl.go`.
- [ ] Add `runREPL`.
- [ ] Build bootstrap initial state.
- [ ] Build app store.
- [ ] Build Ollama client.
- [ ] Build built-in registry.
- [ ] Build agent runner.
- [ ] Build Bubble Tea program.
- [ ] Update root command no-args to run REPL.
- [ ] Add `--model`, `--ollama-url`, and optional `--no-alt-screen`.
- [ ] Preserve `doctor`, `version`, and help behavior.

### K. Tests

- [ ] Add TUI model tests.
- [ ] Add permission broker tests.
- [ ] Add slash command tests.
- [ ] Add Vim mode tests.
- [ ] Add CLI REPL dispatch tests.
- [ ] Use fake agent runner; do not require Ollama for unit tests.

### L. Verification

- [ ] `go mod tidy`
- [ ] `go test ./internal/tui/...`
- [ ] `go test ./internal/cli/...`
- [ ] `go test ./...`
- [ ] `go test -race ./internal/tui/... ./internal/state/...`
- [ ] `go vet ./...`
- [ ] `tools/check-allowed-deps.sh`
- [ ] `tools/check-network-policy.sh`
- [ ] Manual smoke: `nandocodego --help`
- [ ] Manual smoke: `nandocodego version`
- [ ] Manual smoke: `nandocodego doctor`
- [ ] Manual smoke: `nandocodego` opens REPL

### M. Documentation and Phase Log

- [ ] Update `docs/PHASE-LOG.md` with Phase 7 implementation details.
- [ ] Record dependencies added.
- [ ] Record test and manual smoke results.
- [ ] Record known gaps deferred to Phase 13.

## Acceptance Criteria

- [ ] `nandocodego` with no args opens the REPL.
- [ ] `nandocodego --help`, `nandocodego doctor`, and `nandocodego version` still work.
- [ ] Typing a prompt and pressing Enter starts an agent run without blocking Bubble Tea `Update`.
- [ ] Assistant text streams into the transcript.
- [ ] Tool start/progress/result events render as compact tool panels.
- [ ] Ctrl-C aborts an active run; Ctrl-C exits only when no run is active.
- [ ] Ctrl-D exits the REPL.
- [ ] Permission prompt appears for non-read-only tool calls and allow/deny choices are honored.
- [ ] Always-allow adds a session-scoped allow rule.
- [ ] `/help`, `/clear`, `/exit`, and `/model <name>` work.
- [ ] Markdown renderer is cached and not recreated per frame.
- [ ] Tests do not require a live Ollama instance.
- [ ] Dependency and network policy checks pass.

## Exit Gate

Phase 7 is complete when:

- A local manual demo can run for 5 minutes using the REPL with prompt submission, streaming output, one tool panel, one permission prompt, Ctrl-C abort, and `/exit`.
- `go test ./...` and relevant race tests pass.
- `tools/check-allowed-deps.sh` and `tools/check-network-policy.sh` pass.
- The root command behavior has changed to REPL-on-no-args without regressing `doctor`, `version`, or help.
