# Phase 24 Detailed Plan — Multi-Agent Coordination (Required v0.1)

Date: 2026-05-19
Status: Reviewed and expanded implementation checklist; ready for agent execution
Source plans:

- `.codex/go-ollama-plan-AGENTS.md`
- `.codex/go-ollama-plan-HUMANS.md`
- `docs/PHASE-LOG.md`
- `docs/PROJECT-STATUS-AND-ONBOARDING.md`
- `docs/PHASE-8-DETAILED-PLAN.md`
- `docs/PHASE-9-DETAILED-PLAN.md`
- `book/ch08-sub-agents.md`
- `book/ch09-fork-agents.md`
- `book/ch10-coordination.md`
- `https://docs.ollama.com/api/chat`
- `https://docs.ollama.com/capabilities/tool-calling`
- `https://docs.ollama.com/context-length`

## Roadmap Placement

Phase 24 is required v0.1 work. It must be implemented after Phase 21 and before Phase 25, Phase 17, and Phase 18.

Do not implement Phase 17 or Phase 18 while Phase 24 is unimplemented. Phase 18 docs/evals must assume coordinator mode exists and must include the Phase 24 end-to-end coverage.

## Goal

Phase 24 promotes `nandocodego` from a single-agent tool with background helpers (Phase 11 sub-agents, Phase 14 task supervisor) into a true multi-agent coordination system. Agents can now send messages to each other via a mailbox abstraction, a coordinator agent can spawn and supervise worker agents in a structured research-synthesize-implement pattern, and speculative "dream" thinking can run in the background while the user types.

The concrete user-visible changes:

- A new `SendMessage` tool that routes messages to in-process sub-agents, background task agents, or peer agents via Unix Domain Socket.
- A coordinator mode (feature-gated) where the active agent manages a team of worker sub-agents with restricted tools.
- A `Mailbox` per agent task state for queued message delivery at tool-round boundaries.
- Dream tasks (`KindDream = "d"`) that think speculatively and are killed immediately when the user submits a message.
- Auto-resume: `SendMessage` to a completed or killed agent transparently restarts it from its replayable sidechain transcript.

This is the v0.1 multi-agent coordination target. Phase 11 (sub-agent spawning, `IsSubagent` flag, `ParentAbort` channel) and Phase 14 (`tasks.Supervisor`, JSONL output files, `TaskState` sealed interface) are both prerequisites.

## 2026-05-19 Review Addendum — Book and Ollama Constraints

The product direction remains correct, but this plan must be implemented with tighter engineering boundaries than the original draft. The book chapters and current Ollama docs imply these non-negotiable constraints:

- Use one unified task state machine. Do not add a parallel "coordinator runtime" with separate lifecycle rules; coordinator, workers, bash tasks, and dreams must all report through `internal/tasks.Supervisor` and existing task summaries.
- Keep the repo's current task ID format from `internal/ids.New` (`<kind>-<12 hex chars>`). The book's compact examples explain the prefix idea, but Phase 24 should not break existing `b-...`, `a-...`, `m-...`, and `r-...` IDs.
- The coordinator receives exactly three tools: `Agent`, `SendMessage`, and `TaskStop`. If a coordinator can read files or edit files directly, the core "think vs. do" separation is broken.
- Workers receive the normal worker tool pool minus internal coordination tools. Workers must not spawn new workers, call `SendMessage`, or stop sibling tasks.
- Messages are delivered only between complete model turns/tool rounds. The mailbox must never interrupt an in-flight Ollama stream or inject messages while tool calls are being assembled.
- Worker prompts must encode the coordinator's synthesized understanding. "Never delegate understanding" means implementation workers get exact files, facts, constraints, and tests, not vague "based on your findings" prompts.
- `SendMessage` must be idempotent. A caller-provided or generated `message_id` prevents duplicate delivery when a tool call is retried or a UDS write is repeated.
- Mailboxes and resumed histories must be bounded. The book explicitly calls out memory blowups from unbounded agent state; Phase 24 must cap queued messages, message bytes, JSONL replay size, and in-memory task metadata.
- Ollama `/api/chat` supports `tools`, `format`, `stream`, `think`, `keep_alive`, and runtime `options`. Phase 24 must not bypass `llm.Client`; it must preserve the existing request path so `num_ctx`, usage accounting, retry/watchdog behavior, and tool schemas remain consistent.
- Ollama streaming tool calls require accumulation of every streamed `thinking`, `content`, and `tool_calls` chunk before the follow-up request. Phase 24 mailbox draining must happen after the existing stream accumulator emits a complete assistant turn.
- Ollama context length is a runtime resource decision. Coordinator sessions should default to the runtime `num_ctx` selected by Phase 21/22, while worker and dream budgets must be capped so parallel workers do not exhaust VRAM.
- `keep_alive` can pin model memory. Coordinator mode must cap parallel workers and avoid starting dream work while many workers are already active.
- Server mode is now part of the implemented baseline. Coordinator mode and task events must work through `internal/server` SSE sessions, not only the terminal REPL.

## Definition of Success

The Phase 24 exit gate is a multi-step end-to-end test:

1. Start the REPL in coordinator mode (`NANDOCODEGO_COORDINATOR=1`).
2. Submit: "Search the current directory for Go files and count total lines. Use 3 parallel worker agents."
3. Observe: coordinator spawns 3 named worker agents (for example `worker-a`, `worker-b`, `worker-c`), each performs a file scan, all complete.
4. Coordinator receives task-completion notifications for all three workers.
5. Coordinator synthesizes the results and returns a total line count.
6. Entire flow completes without manual intervention — no permission prompts (workers run in `ModeAuto` for read-only Bash).
7. `SendMessage` to a completed agent auto-resumes it from its replayable sidechain transcript and delivers the message.
8. Dream task starts in background, is killed within 100ms when user submits any message.
9. `go test -race ./internal/tasks/... ./internal/tools/sendmessage/...` passes.
10. Start `nandocodego server` with `NANDOCODEGO_COORDINATOR=1`, submit the same coordinator task through the HTTP session API, and observe worker task events over SSE.
11. Verify the recorded Ollama requests still carry the effective `options.num_ctx` and never execute partial streamed tool calls before the assistant turn is complete.

## Baseline Analysis from Implemented Phases

### Phase 0 — Security and Supply Chain

Implemented:

- Dependency allowlist, network policy checker, CI security baseline.

Phase 24 implications:

- Mailbox files are local state but still a security boundary. A mailbox path traversal could deliver messages to arbitrary file paths. All mailbox writes must use path containment checks within the task data directory.
- IPC via Unix Domain Socket is a new attack surface. UDS socket paths must be within `paths.StateDir()/sockets` and permissions must be `0600`. Phase 24 does not add UDS server for external connections (that is Phase 25 bridge mode); Phase 24 uses UDS only for local peer communication between concurrent `nandocodego` instances on the same machine.
- Worker agents must not inherit coordinator's full permission set if the coordinator is running in a more permissive mode. Full permission forwarding follows the `ModeBubble` escalation protocol from Phase 5 in a future hardening phase; Phase 24 v0.1 auto-denies worker `ModeBubble` prompts to avoid deadlock.
- The `NANDOCODEGO_COORDINATOR=1` env var is a session-level feature gate. It must be parsed once at startup; mid-session changes are ignored.
- No new direct dependencies required. All mailbox I/O uses stdlib `os`, `encoding/json`, `sync`. UDS uses stdlib `net`.

### Phase 1 — CLI, Paths, Logging, Scaffold

Implemented:

- `internal/paths` with `StateDir()`, `SessionDir(sessionID)`, `SessionTasksDir(sessionID)`, and `TaskOutputPath(sessionID, taskID)`.

Phase 24 implications:

- Mailbox and replayable transcript files live under the current session task directory: `paths.SessionTasksDir(sessionID) + "/" + <task-id> + "/"`.
- Keep the existing `paths.TaskOutputPath(sessionID, taskID)` event JSONL path for UI/debugging unless a migration is intentionally implemented.
- UDS sockets live at `paths.StateDir() + "/sockets/<agent-id>.sock"`.
- Both paths must be created with `0700` directory permissions and `0600` file permissions.
- Add explicit helpers such as `paths.TaskDir(sessionID, taskID string) string`, `paths.TaskMailboxPath(sessionID, taskID string) string`, `paths.TaskTranscriptPath(sessionID, taskID string) string`, and `paths.AgentSocketPath(agentID string) string`.

### Phase 2 — LLM Client

Implemented:

- `llm.Client` interface, streaming chat, retry, watchdog, capabilities.

Phase 24 implications:

- Coordinator agent and worker agents all use the same `llm.Client` instance (shared at the process level) or separate instances from the factory. Since the Ollama client is stateless (HTTP-based), sharing is safe. Worker agents receive the same client from the coordinator's dependency injection.
- Dream tasks also use `llm.Client`. The dream task can be configured to use a smaller/faster model if the capability matrix indicates a good option, but Phase 24 uses the same model as the coordinator by default.
- All coordinator, worker, auto-resume, and dream calls must go through `llm.Client.Chat` so the existing Ollama adapter continues to set `/api/chat` fields (`tools`, `stream`, `options`, `keep_alive`) and emit final usage (`prompt_eval_count`, `eval_count`, `done_reason`).
- The agent stream accumulator remains the only place that interprets streamed `content`, `thinking`, and `tool_calls`. Mailbox delivery must be added after a complete assistant turn is accumulated, never inside the Ollama stream reader.
- `agent.Config.NumCtx`, `RuntimeNumCtx`, and `context_policy.go` remain the source of truth for `options.num_ctx`. Phase 24 may cap worker/dream effective context, but it must not silently drop `num_ctx` from the request.
- Coordinator mode must cap parallel workers because large-context Ollama sessions consume VRAM. Default cap: 3 concurrent workers; config may raise the cap to 5; a 6th active worker is always rejected.

### Phase 3 — Tool Interface

Implemented:

- `tools.Tool`, `tools.Registry`, `tools.Context`.

Phase 24 implications:

- `SendMessage` is a new tool in `internal/tools/sendmessage/sendmessage.go`.
- `SendMessage` has `IsConcurrencySafe = false` because it modifies shared mailbox state.
- Coordinator-mode workers get a restricted registry (no `SendMessage`, no task-management tools). This restriction is enforced in `internal/agent/coordinator.go` by building a `tools.Registry` that excludes coordinator-only tools.
- The coordinator itself gets a minimal registry: `Agent`, `SendMessage`, `TaskStop` only. The existing `Agent` tool remains the entry point, but coordinator workers must be registered as supervisor-backed `KindAgent` tasks; do not use the legacy 8-hex `RunSubagent` ID format for coordinator addressing.
- The existing Agent tool lives in `internal/tools/agenttool/agenttool.go`. Phase 24 must extend that schema with coordinator-safe naming/background fields instead of adding a duplicate Agent tool.
- Current gap: `Agent(background=true)` still waits for the child `runSubagent` loop to finish before returning the task ID. Phase 24 must make coordinator background worker spawns truly detached: register the task, launch the worker goroutine, return `{status:"async_launched", task_id, output_file, name}` immediately, and let task notifications/reporting deliver completion later.
- Current gap: `agent.RunSubagent` generates an 8-hex local ID, while `tasks.Supervisor` generates repo task IDs like `a-<12 hex chars>`. Phase 24 `SendMessage` and name registration must address supervisor task IDs only.
- Direct synchronous `Agent` calls may keep the existing blocking behavior outside coordinator mode. Coordinator mode must force/default `background=true` and must reject synchronous worker spawns unless an explicit future option is added.
- Dynamic worker tool inventories must not mutate tool descriptions each turn. Pass volatile worker context through prompt/user context assembly, not by rewriting registered tool schemas.

### Phase 4 — Agent Loop

Implemented:

- `agent.Agent.Run(ctx, input) <-chan agent.Event`.
- `agent.Input` with `SystemPrompt`, `Messages`, `ToolContext`.

Phase 24 implications:

- `agent.Input` gains `IsCoordinator bool` flag. When true, the system prompt is augmented with the coordinator-mode instructions (research/synthesize/implement methodology, worker tool awareness, `"never delegate understanding"` principle).
- `agent.Input` gains `CoordinatorID string` for workers running in `ModeBubble` — they need to know the coordinator's agent ID to forward permission escalations via `SendMessage`.
- Pending messages from the mailbox are injected as user-role messages at tool-round boundaries in `agent.go`'s main loop. Add a narrow helper such as `drainPendingMessages(ctx, taskID, supervisor)` rather than spreading mailbox reads across the loop.
- The agent loop does not change structurally. The mailbox drain is an additional step between tool execution completion and the next model call.

### Phase 5 — Permission System

Implemented:

- Seven permission modes, `ModeBubble` for worker escalation.

Phase 24 implications:

- Worker agents in `ModeBubble` that hit an `PermAsk` decision must escalate to the coordinator instead of prompting the user directly.
- This requires the `PermissionPromptFunc` passed to workers to send a `SendMessage` permission-request to the coordinator agent, wait for a response message, and parse the `plan_approval_response` structured type.
- The coordinator, when it receives a permission escalation via its mailbox, may either approve/deny itself (if it is in `ModeBubble` relative to the user) or prompt the TUI user.
- Phase 24 implements a basic version: workers in `ModeBubble` auto-deny rather than escalate. Document full coordinator-to-user escalation as a future multi-agent hardening follow-up.
- Workers that are spawned with `ModeAuto` or `ModeBypass` do not need escalation — they operate autonomously within their permission scope.

### Phase 6 — State Layer

Implemented:

- `internal/bootstrap.State`, `internal/state.Store[state.App]`.
- `state.App.Tasks` holds `map[string]types.TaskSummary`.

Phase 24 implications:

- `state.App.Tasks` is already designed to show task summaries in the TUI. Phase 24 continues to populate this map through `tasks.Supervisor.publish`.
- Add `CoordinatorMode bool` and `CoordinatorID string` to `state.App` for UI/status use, and to `bootstrap.Initial` plus `bootstrap.Snapshot` only if session-level bootstrap access is required by the implementation.
- `state.App` does not hold full mailbox contents (too large); if the TUI needs a count, add a derived `WorkerCount int` or compute it from `Tasks`.
- `state.OnChange` must not mirror coordinator fields to bootstrap unless matching fields are added to `bootstrap.Snapshot`; otherwise keep coordinator mode as session/app state.

### Phase 7 — Bubble Tea TUI

Implemented:

- Full interactive REPL, transcript rendering.

Phase 24 implications:

- When coordinator mode is active, the status bar should show `[COORDINATOR]` indicator.
- Worker task completions arrive as task notifications (from Phase 14) and are rendered as system transcript items.
- Dream task activity is not shown in the transcript unless it produces a result injected before the user's next message.
- No major TUI changes required. The existing task panel rendering handles workers naturally.

### Phase 8 — Memory

Implemented:

- `internal/memory` with recall, extraction, runner decorator.

Phase 24 implications:

- Worker agents in coordinator mode do NOT get the memory runner decorator. They receive only a narrow system prompt with their specific task. Adding memory recall to workers wastes tokens and adds latency.
- The coordinator agent retains the memory runner for its own recall (user preferences, project facts).
- Dream tasks also skip memory recall — they are speculative and short-lived.

### Phase 9 — Hooks

Implemented:

- `internal/hooks` — command and prompt hooks, snapshot-based dispatch.

Phase 24 implications:

- Worker agents fire `PreToolUse` and `PostToolUse` hooks independently. Each worker uses a copy of the session hook snapshot (from `hooks.Snapshot`).
- `StopHook` fires for the coordinator's terminal event, not for individual workers. Worker terminal events do not fire `StopHook` unless the worker is also a foreground session.
- No hooks changes required.

### Phase 10 — MCP

Implemented:

- MCP client, tool wrapping, stdio and HTTP transports.

Phase 24 implications:

- MCP tools are available to coordinator workers. The coordinator system prompt can inform workers which MCP tools are available (via `getCoordinatorUserContext` analogue).
- MCP tool exclusion from the worker restricted registry is the same as other internal coordination tools — workers cannot use `SendMessage` or `Agent` but can use all file/bash/MCP tools.

### Phase 11 — Sub-agents and Fork

Implemented:

- `agent.Input.IsSubagent bool`, `agent.Input.ParentAbort <-chan struct{}`.
- `internal/tools/agenttool/agenttool.go` — the `Agent` tool for spawning sub-agents.

Phase 24 implications:

- Phase 24 builds directly on Phase 11. The `Agent` tool is the coordinator's primary worker-spawning mechanism.
- Coordinator mode extends Phase 11 by: (a) restricting the coordinator's tool registry, (b) setting `IsCoordinator` on the coordinator's `agent.Input`, (c) setting worker agents' `agent.Input.CoordinatorID` for `ModeBubble` escalation.
- Fork sub-agents are mutually exclusive with coordinator mode (same as in the TypeScript reference). `NANDOCODEGO_COORDINATOR=1` must disable fork if it is also enabled.

### Phase 12 — Skills

Implemented:

- File-driven tools, bundled and project-level skills.

Phase 24 implications:

- Worker agents can use skill-based tools. No changes required.
- The coordinator's skill awareness can include skill tool names in the worker context summary.

### Phase 13 — Slash Commands and Config UX

Implemented:

- Full slash command registry, config file loading and watching.

Phase 24 implications:

- Phase 13 config watching triggers factory re-initialization. If `NANDOCODEGO_COORDINATOR` changes in env (unlikely but possible via re-exec), it is read at startup only.
- A future `/coordinator` slash command is out of scope for Phase 24. Document it as a future coordinator-UX follow-up if needed.

### Phase 14 — Tasks and Supervisor

Implemented:

- `TaskState` sealed interface, `KindBash`, `KindAgent` task kinds.
- `tasks.Supervisor` with JSONL output files.
- `TaskOutputTool`, `TaskListTool`, `TaskStopTool`.

Phase 24 implications:

- Phase 24 adds `KindDream TaskKind = "d"` to the task kind enum.
- Dream task state type: `DreamTaskState` implementing `TaskState`.
- Dream tasks are killed immediately (via `context.CancelFunc`) when the user submits a message — the REPL `handleKeyMsg` or the agent bridge must call `supervisor.KillDream()` on message submission.
- Agent task state metadata gains a `Mailbox *Mailbox` field (the bounded in-memory queue) plus a mailbox JSONL file path for durable replay/debugging.
- `Supervisor.DrainMessages(taskID string) []PendingMessage` atomically extracts queued messages and resets the queue.
- JSONL output files already exist for `KindAgent` tasks. Phase 24 must add replayable transcript records alongside those event logs before auto-resume can reconstruct agent history safely.
- Current gap: existing agent task JSONL is an event log (`text_delta`, `tool_start`, `tool_result`, `terminal`), not a replayable `[]llm.Message` transcript. Phase 24 must either add a sidechain transcript file containing complete `llm.Message` records or extend task output JSONL with explicit `message` records that can be reconstructed losslessly.
- Do not base auto-resume on text deltas alone. A resumed worker needs role order, assistant tool calls, matching tool results, thinking content where valid, model, system prompt, permission mode, and original worker metadata.
- Existing `TaskState` variants (`PendingTask`, `RunningTask`, `CompletedTask`, `FailedTask`, `KilledTask`) should be extended by composition or new agent-specific state only where required. Do not fork a second task status enum.

### Phase 15 — Concurrency

Implemented:

- Tool partitioning, per-call safety, speculative execution foundations.

Phase 24 implications:

- `SendMessage` has `IsConcurrencySafe = false`. It must run in the serial partition of tool execution.
- Coordinator spawning multiple workers via `Agent` tool calls can be batched as concurrent tools (all have `IsConcurrencySafe = true` because each spawns an independent goroutine). Phase 15 partitioning already handles this.

### Phase 16 — Observability

Implemented:

- Logging and metric decorators around `llm.Client`.

Phase 24 implications:

- Add metrics for: `coordinator_workers_spawned`, `coordinator_workers_completed`, `coordinator_workers_failed`, `sendmessage_routed_inprocess`, `sendmessage_routed_mailbox`, `sendmessage_autoresume_count`, `dream_tasks_killed`.
- These counters feed the Phase 16 metric decorator, not scattered `fmt.Println` calls.

### Related Phase Summary

- Future Phase 17 (Distribution): no build changes. Feature gate via env var.
- Future Phase 18 (Hardening): Phase 24 adds coordinator end-to-end eval fixtures because this phase is required for v0.1.
- Phase 19–22 (HTTP server, SSE, session lifecycle): Coordinator mode sessions expose the coordinator's event stream via SSE. Workers run in-process; their output is accessible through task notifications, async launch metadata, and JSONL files.
- Current gap: server mode currently constructs only the built-in/MCP registry path, while REPL registers task tools and `Agent`. Phase 24 must add coordinator-mode server registry construction that includes exactly the coordinator tools and the supporting supervisor plumbing, without changing normal server sessions when the feature gate is off.
- Phase 23 (OpenAI adapter): removed from the active v0.1 plan. Coordinator and workers use the existing Ollama-backed `llm.Client` path.

## Ollama API Review Notes

Official Ollama docs for `/api/chat`, tool calling, and context length add implementation requirements that should be tested explicitly:

- `/api/chat` is the only model endpoint Phase 24 should use. The request shape includes `model`, `messages`, optional `tools`, optional `format`, optional `options`, `stream`, `think`, and `keep_alive`.
- The response includes `message.tool_calls`, `done`, `done_reason`, `prompt_eval_count`, `eval_count`, and timing fields. Phase 24 progress and completion notifications should rely on the existing `llm.StreamEvent`/`agent.Usage` mapping rather than parsing Ollama JSON directly in new coordinator code.
- Tool calls can be single, parallel, multi-turn, and streamed. When streaming, the client must gather all chunks of `thinking`, `content`, and `tool_calls` and send the fully assembled assistant message plus tool results in the next request.
- `format` can be `json` or a JSON schema. Phase 24 does not require structured Ollama outputs for normal coordinator turns, but tests that parse coordinator/dream protocol messages should prefer explicit JSON schema or strict internal structs over free-form text parsing.
- Ollama's default context length is hardware dependent. Agent and coding tasks should use large context when available, but larger context increases memory usage. Phase 24 worker/dream spawning must respect runtime `num_ctx` and cap concurrency.
- If `OLLAMA_CONTEXT_LENGTH` or the app context slider provides a larger runtime window, Phase 24 should use the live runtime limit exposed through existing config instead of hard-coding static model recommendations.
- `keep_alive` keeps a model loaded. Dreams should not extend keep-alive pressure during active coordinator work; either inherit the default request value or explicitly use a short value only if the existing config already supports it.

## Deep Analysis of ch10-coordination.md

### Task State Machine Extension

Chapter 10 defines seven task types. Phase 24 adds `KindDream` to the existing `KindBash` and `KindAgent`. The full list for this repo after Phase 24:

| Kind | Prefix | Description |
|------|--------|-------------|
| `KindBash` | `b` | Background shell command (Phase 14) |
| `KindAgent` | `a` | Background sub-agent (Phase 11/14) |
| `KindMCP` | `m` | Existing MCP monitor/task kind |
| `KindRemote` | `r` | Existing remote task kind |
| `KindDream` | `d` | Speculative background thinking (Phase 24) |

`KindInProcessTeammate` (`t`) and `KindWorkflow` (`w`) are reserved for future phases. Phase 24 does not implement them.

### Communication Channels

Phase 24 implements three of the four communication channels from ch10:

1. **In-process sub-agent routing** (direct channel): `SendMessage` to a running sub-agent queues the message in the task's agent mailbox via the supervisor. Messages are drained at tool-round boundaries.
2. **Background agent mailbox**: `SendMessage` to a background task appends to the in-memory mailbox and mirrors the message to `<task-dir>/mailbox.jsonl` via `Supervisor.QueueMessage`.
3. **Unix Domain Socket** (`uds:<path>`): `SendMessage` to a `uds:` address sends a JSON-encoded `PendingMessage` to a listening UDS server. Phase 24 implements the client (send) side only. Phase 25 (bridge) implements the server side.

The fourth channel — **bridge messages** (`bridge:<session-id>`) — is Phase 25 / Remote mode. Phase 24 routes any `bridge:` address to an `ErrNotSupported` error with a user-friendly message.

### Coordinator Mode Implementation

The coordinator agent is an `agent.Agent` with:

- `agent.Input.IsCoordinator = true`.
- A minimal `tools.Registry` containing only `Agent`, `SendMessage`, and `TaskStop`.
- An extended system prompt from `coordinator.BuildSystemPrompt(workerToolNames, scratchpadDir)`.
- MCP tool names injected into the worker context summary.

Workers are `agent.Agent` instances with:

- `agent.Input.IsSubagent = true`.
- `agent.Input.CoordinatorID` set to the coordinator's agent ID.
- Full worker-safe tool registry minus internal coordination and task-observation tools. Exclude at least `Agent`, `SendMessage`, `TaskCreate`, `TaskGet`, `TaskList`, `TaskOutput`, and `TaskStop` for v0.1 workers.
- `agent.Input.PermissionMode = ModeAuto` by default for worker spawns; coordinator can specify otherwise via `Agent` tool parameters.
- A generated or coordinator-provided worker name registered in `Supervisor.RegisterName`, allowing future `SendMessage` calls to use role names like `researcher-auth` instead of raw task IDs.
- An isolated tool context: fresh worker permission state, no shared TUI `setAppState`, same session `tasks.Supervisor`/state-store notification path, same `llm.Client`.

The existing Agent tool must support coordinator use directly:

- `task` remains the required worker prompt.
- `name` is optional but strongly encouraged in coordinator mode.
- `background` remains supported and coordinator mode must default workers to background execution.
- Add an optional supervisor/coordinator configuration dependency to `agenttool.Tool` (constructor option or setter is acceptable) so coordinator mode can create supervisor-backed `KindAgent` tasks without changing non-coordinator behavior.
- Background worker launch must be truly asynchronous. The Agent tool should create/register the task and return immediately after the goroutine starts; it must not consume the worker event stream on the coordinator's tool-call path.
- The detached worker goroutine owns event consumption and sidechain recording. It updates the supervisor state and emits task notifications when terminal.
- `model`, `permission_mode`, and `max_turns` keep their existing behavior.
- If `run_in_background` is added as a compatibility alias, it must map to `background` internally and not duplicate behavior.

The coordinator system prompt encodes:
- Research / Synthesize / Implement / Verify four-phase workflow.
- "Never delegate understanding" principle.
- Parallelism guidance (read tasks in parallel, write tasks per disjoint file set).
- Continue-vs-spawn decision matrix.
- Worker prompt writing best practices and anti-patterns.
- Available worker tools list.
- Scratchpad directory path (if feature-gated `NANDOCODEGO_SCRATCHPAD=1`).

### Dream Tasks

Dream tasks are the most speculative feature in Phase 24. They must be implemented conservatively:

- Feature gate: `NANDOCODEGO_DREAM=1` env var. Off by default.
- One dream task per session at most. If a dream is already running when the user is idle, do not start another.
- No dream starts while coordinator mode has active workers. Avoid competing with useful worker calls for Ollama VRAM.
- Dream spawns immediately after the current agent run completes, before the next user message.
- Dream prompt: "The user just completed a session turn. Think about what they might ask next and prepare a brief analysis." — generic speculative prompt.
- Dream result: if the dream completes before the next user message, inject its output as a `[dream]` system message in the agent input.
- Kill path: `supervisor.KillDream()` called in the REPL before starting the next agent run. Kill must complete within 100ms (the dream's `context.CancelFunc` is called and the goroutine exits).
- Dream task output is NOT written to the main transcript, event JSONL, or replayable sidechain transcript unless it completes before the next user message and is explicitly injected as a `[dream]` system message.
- Dream task does NOT use the memory runner (no recall side-query).
- Dream task uses the same `llm.Client` and model as the main session by default, but with lower `MaxTurns` and a smaller effective context target when the existing config permits it.
- Dream task cancellation means "cancel requested and supervisor state updated within 100ms"; the Ollama HTTP goroutine may finish cleanup after the cancellation signal, but it must not keep the dream task marked running.

### Auto-Resume Pattern

When `SendMessage` targets a task in a terminal state (`completed`, `failed`, `killed`):

1. Look up the task by ID in `Supervisor`.
2. Find the replayable transcript path from agent task metadata. If only event JSONL exists, return a clear resume error rather than guessing from deltas.
3. Read replayable JSONL `message` records, reconstruct `[]llm.Message` history.
4. Filter out orphaned thinking blocks and unresolved tool-use messages.
5. Create a new `agent.Input` with the reconstructed history and the `SendMessage` content as the next user message.
6. Register a new background task (new ID, same `agentType`, `selectedAgent`, `model`).
7. Call `agent.Agent.Run` in a goroutine, linked to the new task state.
8. Return `SendMessage` result: `"Agent '<name>' was completed; resumed with your message"`.

This makes `SendMessage` idempotent with respect to agent liveness from the coordinator's perspective.

Auto-resume is bounded:

- Store replayable sidechain messages during worker execution. A recommended format is one JSONL record per complete chat message: `{ "kind": "message", "message": llm.Message, "turn": n, "ts": ... }`; event records may remain for UI/debugging but are not sufficient for resume.
- Preserve metadata needed to restart the worker: model, system prompt, toolset/worker registry policy, permission mode, original task prompt, agent name, and output/transcript paths.
- Reconstruct only the newest valid history that fits the effective context budget. Prefer existing `agent.BuildAssemblyBudget`/prompt packing helpers; if they are not directly reusable, add a small resume budget helper in `internal/agent`.
- Preserve role ordering and matching tool-call/tool-result pairs. Drop incomplete streamed assistant turns and unmatched tool results.
- Preserve the original agent name mapping by moving the name to the new task ID.
- Keep the old task state terminal and evictable; do not mutate completed task history into running state.
- Treat corrupted JSONL as a recoverable `SendMessage` error. Do not spawn a half-resumed worker.

## Evaluation of the Original Phase 24 Concept

The original concept is correct at the product level. It needs more implementation detail for this repo:

- It does not specify which `ModeBubble` escalation behavior applies in v0.1 (auto-deny vs. full escalation chain). Phase 24 uses auto-deny for worker `ModeBubble` as documented.
- It does not specify how the coordinator system prompt is constructed or stored (inline in `coordinator.go` vs. embedded file). Phase 24 uses a `coordinator.BuildSystemPrompt` function with the text inline, consistent with how Phase 8's memory prompt section was built.
- It does not specify how dream tasks interact with the queued prompt system in `state.App.QueuedPrompts`. Phase 24: dream tasks run after the completion event of the previous turn, before the next queued prompt is processed.
- It does not specify whether the coordinator shares the session memory runner. Phase 24: coordinator uses the memory runner; workers do not.
- It does not specify the scratchpad directory location. Phase 24: `paths.StateDir() + "/scratchpad/" + sessionID` when `NANDOCODEGO_SCRATCHPAD=1`.
- It does not specify Ollama streaming semantics. Phase 24: preserve the existing stream accumulator and drain mailboxes only after a complete assistant turn is assembled.
- It does not specify backpressure. Phase 24: mailbox capacity and message byte limits are mandatory, not optional.
- It does not specify server behavior. Phase 24: coordinator mode must be available in both terminal REPL and `internal/server` sessions.
- It does not specify the current blocking behavior of `Agent(background=true)`. Phase 24: coordinator worker launch must detach and return immediately after the worker task is registered.
- It does not distinguish event JSONL from replayable transcripts. Phase 24: add replayable `llm.Message` sidechain records for auto-resume; do not reconstruct history from text deltas.
- It does not list all worker-internal tools. Phase 24: workers exclude `Agent`, `SendMessage`, `TaskCreate`, `TaskGet`, `TaskList`, `TaskOutput`, and `TaskStop`.
- It does not state that server sessions currently lack task/agent tools. Phase 24: coordinator server sessions must build/register the coordinator registry and supervisor plumbing explicitly.

## Final Phase 24 Scope

In scope:

- `Mailbox` type in `internal/tasks/mailbox.go`.
- Agent task mailbox metadata and `Supervisor.QueueMessage` / `Supervisor.DrainMessages`.
- Durable mailbox append log at `<task-dir>/mailbox.jsonl` for debuggability and delivery audit. Auto-resume uses the separate replayable sidechain transcript, not mailbox-only records.
- `KindDream` task type and `DreamTaskState`.
- `supervisor.KillDream()` method and dream lifecycle.
- `internal/tools/sendmessage/sendmessage.go` with three routing modes (in-process, mailbox, UDS client).
- `internal/agent/coordinator.go` with coordinator mode activation, system prompt, tool restriction.
- `internal/tools/agenttool/agenttool.go` schema extension for coordinator worker names and default-background behavior.
- True detached background launching for coordinator workers; `Agent(background=true)` must not block on worker completion in coordinator mode.
- REPL and server wiring so coordinator sessions emit task/worker events consistently.
- Server-side coordinator registry/supervisor wiring because the current server registry path does not include `Agent` or task tools.
- Coordinator mode feature gate via `NANDOCODEGO_COORDINATOR=1`.
- Dream feature gate via `NANDOCODEGO_DREAM=1`.
- Auto-resume from replayable sidechain transcript in `SendMessage`.
- Replayable worker sidechain transcript records containing complete `llm.Message` values and restart metadata.
- Agent name registry (simple `map[string]string` in `Supervisor`).
- Bounded mailbox capacity, message byte limit, duplicate `message_id` suppression, and safe UDS path containment.
- Ollama-safe context/concurrency defaults: default 3 workers, configurable cap up to 5, no dream during active worker fan-out.
- Worker context isolation (fresh worker permission state, no TUI state sharing, no `setAppState` sharing). Do not reference permission-state symbols from other implementations unless a concrete Go implementation introduces them.
- `ModeBubble` workers auto-deny in this phase; full escalation chain is a future multi-agent hardening follow-up.
- Tests and Phase log update.

Out of scope:

- Full `ModeBubble` escalation chain through coordinator to user.
- `bridge:` address routing (Phase 25).
- In-process teammate (`KindInProcessTeammate`) with plan approval protocol.
- Swarm teams with named teammates and broadcast (`*`) addressing.
- Team file persistence (`team.json`).
- Full scratchpad directory workflows for cross-worker knowledge sharing. Phase 24 may pass a feature-gated scratchpad path in prompts, but should not build a complete scratchpad protocol.
- New task-management or todo-list tools beyond the existing background task tools and existing todo tools. `TaskCreate` already exists for background tasks, but it is not part of the coordinator's minimal tool registry and must be excluded from worker registries.
- Coordinator `/coordinator` slash command.
- Telemetry for SendMessage routing modes beyond the metrics listed in this phase.
- Blocking `wait_for_reply=true` semantics for `SendMessage`. Phase 24 accepts the schema field but returns a clear `ErrNotSupported` if it is true.
- UDS server/listener discovery and cross-process peer listing. Phase 24 implements only the secure client send path.

## Architecture

### Package Layout

```text
internal/tasks/
  mailbox.go
  mailbox_test.go
  (existing supervisor.go, state.go, output.go, etc.)

internal/types/
  task.go

internal/tools/sendmessage/
  sendmessage.go
  sendmessage_test.go

internal/tools/agenttool/
  agenttool.go
  agenttool_test.go

internal/agent/
  coordinator.go
  coordinator_test.go
  (existing agent.go, stream.go, tools.go, etc.)

internal/server/
  session.go
  handler.go
  sse.go
```

### Core Types

```go
// internal/tasks/mailbox.go

type PendingMessage struct {
    ID        string    `json:"id"`
    FromAgent string    `json:"from_agent"`
    ToAgent   string    `json:"to_agent"`
    Summary   string    `json:"summary,omitempty"`
    Content   string    `json:"content"`
    SentAt    time.Time `json:"sent_at"`
}

type Mailbox struct {
    mu       sync.Mutex
    messages []PendingMessage
    seenIDs  map[string]struct{} // bounded duplicate suppression
    notify   chan struct{} // closed+replaced on each new message
}

const DefaultMailboxCapacity = 128
const DefaultMailboxMessageBytes = 64 * 1024

func NewMailbox() *Mailbox
func (m *Mailbox) Enqueue(msg PendingMessage) error
func (m *Mailbox) Drain() []PendingMessage       // atomic: returns all, resets queue
func (m *Mailbox) Notify() <-chan struct{}         // closed when message arrives
func (m *Mailbox) Len() int
```

```go
// Agent task metadata extension.
// The current repo uses generic status states in internal/tasks/state.go.
// Implement this as optional metadata embedded in RunningTask/terminal states
// or as a supervisor side map, but keep TaskState summaries compatible.

type AgentTaskMetadata struct {
    Mailbox      *Mailbox          // message inbox
    MailboxFile  string            // append-only mailbox JSONL mirror
    AgentName    string            // human-readable name (optional)
    EvictAfter   *time.Time        // GC deadline for terminal tasks
}
```

```go
// Extension to tasks.Supervisor

func (s *Supervisor) QueueMessage(taskID string, msg PendingMessage) error
func (s *Supervisor) DrainMessages(taskID string) ([]PendingMessage, error)
func (s *Supervisor) RegisterName(name, taskID string) error
func (s *Supervisor) LookupByName(name string) (string, bool)  // returns task ID
func (s *Supervisor) UnregisterName(name string)
func (s *Supervisor) KillDream() error
```

```go
// internal/tasks/dream.go

type DreamTaskState struct {
    ID          string
    Status      types.TaskStatus
    StartTime   time.Time
    EndTime     *time.Time
    Cancel      context.CancelFunc  // not serialized
    Result      string              // injected as system context if available
}

func (d *DreamTaskState) Kind() types.TaskKind  { return types.KindDream }
func (d *DreamTaskState) TaskID() string  { return d.ID }
// implements TaskState sealed interface
func (d *DreamTaskState) isTaskState()    {}
```

```go
// internal/tools/sendmessage/sendmessage.go

type Input struct {
    To           string `json:"to"`
    Message      string `json:"message"`
    Summary      string `json:"summary,omitempty"`
    MessageID    string `json:"message_id,omitempty"`
    WaitForReply bool   `json:"wait_for_reply,omitempty"`
    // "to" formats:
    //   "agent-name"           — in-process name registry lookup
    //   "a-1a2b3c4d0001"      — direct task ID lookup
    //   "uds:/path/to.sock"   — Unix domain socket
    //   "bridge:<session-id>" — returns ErrNotSupported (Phase 25)
}

type SendMessageTool struct {
    supervisor *tasks.Supervisor
    logger     *slog.Logger
}

func NewSendMessageTool(supervisor *tasks.Supervisor, logger *slog.Logger) *SendMessageTool

// IsConcurrencySafe returns false.
// IsReadOnly returns false.
// IsDestructive returns false.
```

```go
// internal/agent/coordinator.go

// IsCoordinatorMode returns true when NANDOCODEGO_COORDINATOR=1.
func IsCoordinatorMode() bool

// IsDreamEnabled returns true when NANDOCODEGO_DREAM=1.
func IsDreamEnabled() bool

// BuildCoordinatorSystemPrompt builds the ~370-line coordinator methodology prompt.
func BuildCoordinatorSystemPrompt(workerToolNames []string, scratchpadDir string) string

// BuildCoordinatorRegistry returns a registry containing only Agent, SendMessage, TaskStop.
func BuildCoordinatorRegistry(
    agentTool tools.Tool,
    sendMessageTool tools.Tool,
    taskStopTool tools.Tool,
) *tools.Registry

// BuildWorkerRegistry returns the full registry minus internal coordination tools.
func BuildWorkerRegistry(full *tools.Registry) *tools.Registry

// CoordinatorInternalTools is the set of tool names excluded from worker registries.
var CoordinatorInternalTools = map[string]bool{
    "SendMessage": true,
    "Agent":       true,
    "TaskCreate":  true,
    "TaskGet":     true,
    "TaskStop":    true,
    "TaskList":    true,
    "TaskOutput":  true,
}
```

### SendMessage Routing Logic

```
SendMessage(to: X, message: M)
  ↓
  if starts with "bridge:" → return ErrNotSupported
  if starts with "uds:"    → sendToUDS(path, M)   → return result
  ↓
  Look up X in supervisor.LookupByName(X) → taskID (or X is already a taskID)
  ↓
  task = supervisor.Get(taskID)
  ↓
  if task == nil           → return error "agent not found"
  if task summary status == running → supervisor.QueueMessage(taskID, M) → return "queued"
  if task is terminal      → autoResume(task, M)  → return "resumed"
```

### Coordinator System Prompt Structure

The coordinator system prompt is approximately 370 lines and encodes:

**Section 1: Role Definition** (~20 lines)
- You are a coordinator agent. Your job is to plan, delegate, and synthesize.
- You do NOT edit files, run shell commands, or read code directly.
- You spawn worker agents and synthesize their results.

**Section 2: Tool Inventory** (~15 lines)
- Agent: spawn a worker with a specific prompt and tool set.
- SendMessage: send a message to a running or completed worker.
- TaskStop: kill a running worker if it needs to be redirected.

**Section 3: Four-Phase Workflow** (~80 lines)
- Research phase: spawn 3–5 workers in parallel to read files, run tests, gather facts.
- Synthesis phase: coordinator synthesizes all research results — do NOT delegate this.
- Implementation phase: spawn workers with precise, specific instructions derived from synthesis.
- Verification phase: spawn workers to run tests, verify changes, check consistency.

**Section 4: Never Delegate Understanding** (~60 lines)
- Anti-pattern examples with corrections.
- Specific file paths, line numbers, and exact changes in worker prompts.
- The cost of vague prompts vs. specific prompts.

**Section 5: Parallelism Model** (~40 lines)
- Read-only tasks run freely in parallel.
- Write tasks on disjoint file sets can run in parallel.
- Write tasks on overlapping file sets must serialize.
- Wait for all research workers before synthesis.

**Section 6: Continue vs. Spawn Decision** (~50 lines)
- High context overlap, same files: continue (SendMessage).
- Low overlap, different domain: spawn fresh.
- Failed worker: spawn fresh with failure context.
- Follow-up on worker's own output: continue.

**Section 7: Worker Prompt Best Practices** (~60 lines)
- Include: file paths, line numbers, exact changes, test commands.
- Include: what NOT to change (scope boundaries).
- Include: success criteria (how to know the task is done).
- Avoid: vague instructions, missing file paths, delegating understanding.

**Section 8: Worker Tool Inventory** (~20 lines)
- Dynamically inserted from `workerToolNames`.
- Scratchpad directory if available.

**Section 9: Result Collection** (~25 lines)
- Use task-completion notifications and Agent tool async launch metadata to track worker output.
- Worker results arrive as task notifications in your conversation.
- After all workers complete, synthesize before spawning next phase.
- Do not call `TaskOutput` directly in coordinator mode; the coordinator registry is intentionally limited to `Agent`, `SendMessage`, and `TaskStop`.

### Mailbox Message Drain Protocol

Messages from the mailbox are drained at tool-round boundaries in the agent loop. In `internal/agent/agent.go`:

```
agent main loop:
  1. Call LLM → stream events → accumulate message
  2. If assistant message has tool calls:
     a. Execute tool calls
     b. Append tool-result messages
     c. Drain task notifications for completed workers
     d. Drain mailbox: messages = supervisor.DrainMessages(taskID)
     e. For each notification/pending message: append as user-role llm.Message
     f. Continue loop
  3. If no tool calls (terminal model turn):
     a. Drain task notifications and mailbox one final time
     b. If messages found: continue loop (deliver messages)
     c. If no messages: emit Terminal event, return
```

This guarantees messages never interrupt a model mid-generation, only between complete turns. It also preserves Ollama streaming semantics: streamed `thinking`, `content`, and `tool_calls` chunks are accumulated before any mailbox or task-notification injection is considered.

## Implementation Plan

### Step 1 — Mailbox

Files:

- `internal/tasks/mailbox.go`
- `internal/tasks/mailbox_test.go`

Implement:

- `NewMailbox() *Mailbox`.
- `Enqueue(msg PendingMessage) error` — thread-safe append, validates non-empty ID, enforces message byte limit, deduplicates by `msg.ID`, enforces capacity, close+replace notify channel.
- `Drain() []PendingMessage` — atomic swap of messages slice, reset to nil.
- `Notify() <-chan struct{}` — returns current notify channel (replaced on each enqueue).
- `Len() int` — returns current queue length without draining.
- Default limits: 128 queued messages per agent, 64 KiB UTF-8 content per message, bounded duplicate-ID set aligned with queue capacity.
- Backpressure behavior: when the mailbox is full, return `ErrMailboxFull`; do not drop oldest messages silently.
- Duplicate behavior: if `msg.ID` has already been accepted, return a success-equivalent duplicate result from `SendMessage` without enqueuing a second copy.
- Persistence hook: `Supervisor.QueueMessage` mirrors accepted messages to `<task-dir>/mailbox.jsonl`; the `Mailbox` type itself remains an in-memory concurrency primitive.

Concurrency:

- `mu sync.Mutex` protects `messages` and `notify`.
- `Drain` holds the lock only during the swap (does not hold it while processing messages).
- `Notify` returns the channel reference under the lock; callers select on it without the lock.

Tests:

- Empty mailbox `Drain` returns nil (not empty slice).
- Enqueue and drain returns all messages in order.
- Enqueue over capacity returns `ErrMailboxFull` and preserves existing messages.
- Oversized message returns validation error and is not queued.
- Duplicate `message_id` is accepted once and not returned twice by `Drain`.
- Concurrent enqueue + drain under `-race` produces no races.
- `Notify` channel is closed when message arrives.
- Second `Notify` call after drain returns new open channel.
- `Len` returns correct count before and after drain.

### Step 2 — Task State Extensions

Files:

- `internal/tasks/state.go`
- `internal/tasks/supervisor.go`
- `internal/types/task.go`

Implement:

- The repo type is `tasks.Supervisor`; keep that name in code.
- Add an agent task state representation that can carry `Mailbox *tasks.Mailbox`, `MailboxFile string`, `AgentName string`, `EvictAfter *time.Time`, `Notified bool`, `OutputOffset int64`, and lightweight progress counters.
- Preserve the existing public task summaries in `types.TaskSummary`; do not expose mailbox contents in app state.
- `Supervisor.QueueMessage(taskID string, msg PendingMessage) error`.
- `Supervisor.DrainMessages(taskID string) ([]PendingMessage, error)`.
- `Supervisor.RegisterName(name, taskID string) error` — stores in `agentNameRegistry map[string]string`, rejects empty names, duplicate live names, and path-like names.
- `Supervisor.LookupByName(name string) (string, bool)`.
- `Supervisor.UnregisterName(name string)`.
- `Supervisor.KillDream() error` — cancels current dream task, sets status to `killed`.
- `Supervisor.ActiveWorkerCount() int` or equivalent helper for coordinator/dream concurrency gates.
- State transitions must stay under the supervisor mutex: pending → running → terminal, never terminal → running. Auto-resume creates a new task ID.
- Task IDs must keep the repo pattern generated by `internal/ids.New`: one kind prefix, a hyphen, and 12 lowercase hex chars. `KindDream` uses `d-...`.

Tests:

- `QueueMessage` on non-existent task returns error.
- `QueueMessage` on running task appends to mailbox.
- `QueueMessage` mirrors accepted messages to `mailbox.jsonl` under the task directory with `0600` file permissions.
- `DrainMessages` returns queued messages and resets queue.
- `RegisterName` and `LookupByName` round-trip.
- `RegisterName` rejects duplicate live names and invalid names.
- `LookupByName` returns false for unknown name.
- `KillDream` with no active dream is a no-op (no error).
- Terminal tasks are never mutated back to running.

### Step 3 — Dream Task

Files:

- `internal/tasks/dream.go`
- `internal/tasks/dream_test.go`

Implement:

- `DreamTaskState` struct implementing `TaskState` sealed interface.
- `Supervisor.SpawnDream(ctx context.Context, client llm.Client, model string, systemPrompt string) error`.
- Dream runs `agent.Agent.Run` in a goroutine with a child context.
- Dream stores its `context.CancelFunc` in `DreamTaskState.Cancel`.
- Dream stores its output (terminal assistant message content) in `DreamTaskState.Result`.
- `KillDream()` calls `Cancel()` and sets status to `killed`.
- Dream uses no tools, no memory runner, low `MaxTurns`, and the current model through `llm.Client`.
- Dream does not start if any coordinator worker is active or if a previous dream result is still fresh and unused.
- Dream result carries a `CompletedAt` timestamp; inject only if fresh (default max age 30s).
- Dream killed within 100ms deadline: test with `time.After(100 * time.Millisecond)` assertion.

Tests:

- `SpawnDream` starts a goroutine that completes normally.
- `KillDream` cancels the dream context within 100ms.
- `DreamTaskState.Result` is set after natural completion.
- Supervisor does not spawn a second dream if one is already running.
- Supervisor does not spawn dream while active coordinator workers exist.
- Dream task does not appear in `Supervisor.List()` result (it is ephemeral).

### Step 4 — SendMessage Tool

Files:

- `internal/tools/sendmessage/sendmessage.go`
- `internal/tools/sendmessage/sendmessage_test.go`

Implement routing modes:

**In-process routing** (name or task ID):

1. `supervisor.LookupByName(to)` → taskID or `to` is already a taskID.
2. Validate direct task IDs with the actual repo ID format: `^[a-z]-[0-9a-f]{12}$`. `SendMessage` only accepts agent task IDs (`a-...`) in v0.1.
   - Legacy bare 8-hex sub-agent IDs from `agent.RunSubagent` are not valid `SendMessage` addresses unless they are migrated into supervisor-backed `a-...` tasks first.
3. `supervisor.Get(taskID)` → agent task state.
4. If running: `supervisor.QueueMessage(taskID, msg)` → return `"queued"` or `"duplicate"`.
5. If terminal: call auto-resume (Step 5).
6. If not found: return error.

**UDS routing** (`uds:<path>`):

1. Parse socket path from `to` field.
2. Clean and validate path containment under `paths.StateDir()/sockets`. Reject relative traversal, symlinks that escape the socket directory, and non-socket paths where detectable.
3. `net.Dialer{Timeout: 1 * time.Second}.DialContext(ctx, "unix", path)`.
4. Write one JSON-encoded `PendingMessage` followed by newline, then close.
5. Return success or dial error.

**Bridge routing** (`bridge:<session-id>`):

1. Return `ErrNotSupported` with message "bridge mode requires Phase 25 remote server".

Input schema:

```json
{
  "type": "object",
  "properties": {
    "to":           {"type": "string", "description": "Agent name, task ID, uds:<path>, or bridge:<session-id>"},
    "summary":      {"type": "string", "description": "Short routing summary shown in task notifications"},
    "message_id":   {"type": "string", "description": "Optional idempotency key; generated if omitted"},
    "message":      {"type": "string", "description": "Message content to deliver"},
    "wait_for_reply": {"type": "boolean", "description": "Block until agent replies (in-process only)"}
  },
  "required": ["to", "message"]
}
```

`IsConcurrencySafe = false` — modifies shared mailbox state.
`IsReadOnly = false`.
`WaitForReply = true` returns `ErrNotSupported` in v0.1 with a clear message; do not implement blocking waits in this phase.
`message` is capped at 64 KiB and must be valid UTF-8.

Tests:

- `SendMessage` to running in-process agent queues message within 10ms.
- `SendMessage` duplicate `message_id` returns duplicate/success without enqueuing twice.
- `SendMessage` with `wait_for_reply=true` returns `ErrNotSupported`.
- `SendMessage` rejects message content over the byte limit.
- `SendMessage` to completed agent calls auto-resume.
- `SendMessage` to unknown agent returns error.
- `SendMessage` to `uds:` address writes to httptest-style fake UDS listener.
- `SendMessage` to `uds:` path outside `paths.StateDir()/sockets` is rejected before dialing.
- `SendMessage` to `bridge:` returns `ErrNotSupported`.
- Concurrent `SendMessage` calls to same agent do not race (`-race` clean).

### Step 5 — Auto-Resume

Files:

- `internal/agent/coordinator.go` (or `internal/tasks/resume.go`)

Implement `AutoResume(ctx context.Context, taskID string, newMessage string, client llm.Client, registry *tools.Registry, supervisor *tasks.Supervisor) (string, error)`:

1. Resolve the terminal task by `taskID` and read JSONL from its output file — each line is a `llm.Message` JSON.
2. Filter orphaned messages:
   - Remove `thinking` content blocks if the following message does not reference them.
   - Remove tool-use messages without a corresponding tool-result message.
   - Remove tool-result messages without corresponding tool-use IDs.
   - Remove incomplete streamed assistant turns that never reached `done`.
3. Enforce the effective context budget before appending the new message. Keep newest complete turns, preserve system/developer instructions when present, and log what was truncated.
4. Append `newMessage` as a user-role `llm.Message`.
5. Build `agent.Input` with reconstructed history, original model, same `agentType`, same coordinator/worker restrictions, and current `NumCtx` policy.
6. Register new background task in supervisor with a new task ID and fresh mailbox.
7. Move the old agent name mapping to the new task ID.
8. Call `agent.Agent.Run(ctx, input)` in a goroutine linked to the new task.
9. Return new task ID as resume confirmation string.

Tests:

- Auto-resume reads JSONL and reconstructs 5-message history correctly.
- Orphaned thinking block removed from reconstructed history.
- Unmatched tool-use message removed from reconstructed history.
- Unmatched tool-result message removed from reconstructed history.
- Over-budget history is truncated at complete turn boundaries.
- New message appended as final user message.
- Auto-resume registers new task in supervisor.
- Auto-resume preserves agent name mapping on the new task ID.
- Auto-resume with corrupted JSONL returns error.

### Step 6 — Coordinator Mode

Files:

- `internal/agent/coordinator.go`
- `internal/agent/coordinator_test.go`
- `internal/tools/agenttool/agenttool.go`
- `internal/cli/repl.go`
- `internal/server/session.go`
- `internal/server/handler.go`

Implement:

- `IsCoordinatorMode() bool` — returns `isEnvTruthy(os.Getenv("NANDOCODEGO_COORDINATOR"))`.
- `IsDreamEnabled() bool` — returns `isEnvTruthy(os.Getenv("NANDOCODEGO_DREAM"))`.
- `BuildCoordinatorSystemPrompt(workerToolNames []string, scratchpadDir string) string`.
- `BuildCoordinatorRegistry(agentTool, sendMessageTool, taskStopTool tools.Tool) *tools.Registry`.
- `BuildWorkerRegistry(full *tools.Registry) *tools.Registry` — excludes `CoordinatorInternalTools`.
- `CoordinatorConfig` snapshot parsed once at session startup: coordinator enabled, dream enabled, max workers, scratchpad enabled, effective `NumCtx`.
- `BuildCoordinatorUserContext(workerToolNames []string, mcpNames []string, scratchpadDir string) []llm.Message` or equivalent. Volatile tool/MCP/scratchpad info should be added as context messages, not by changing tool descriptions.
- Agent tool schema additions: `name` and `run_in_background` alias if needed. `background` remains the canonical Go field.
- Background worker spawning must register the name with `Supervisor.RegisterName` before returning the async launch result.
- Coordinator `Agent(background=true)` path must be detached. Implementation direction:
  - Create/register a `KindAgent` task through `Supervisor.Start` or an equivalent helper.
  - Start the worker goroutine from the supervisor run function.
  - Return from the Agent tool as soon as the task is running.
  - Consume worker events in the goroutine and write both event JSONL and replayable message sidechain records.
  - Emit task notifications on terminal status; do not stream worker internals directly into the coordinator transcript.
  - Keep existing foreground/blocking `Agent` behavior for non-coordinator sessions unless explicitly changed by a separate task.
- Coordinator worker cap enforcement lives in the Agent tool or coordinator wrapper, not only in the prompt.
- Coordinator mode disables fork mode and recursive sub-agent spawning.

Wiring in `internal/cli/repl.go`:

- If `IsCoordinatorMode()`:
  - Build coordinator registry (3 tools only).
  - Set `agent.Input.IsCoordinator = true`.
  - Replace system prompt with `BuildCoordinatorSystemPrompt(workerToolNames, scratchpadDir)`.
  - Log warning: `[coordinator] mode active — direct file tools disabled`.
- If not coordinator mode: behavior unchanged.

Wiring in `internal/server`:

- Server session creation reads the same coordinator config as REPL.
- When coordinator mode is off, preserve the current server registry behavior.
- When coordinator mode is on, construct a session-local `tasks.Supervisor`, register/instantiate the coordinator `Agent`, `SendMessage`, and `TaskStop` tools, and use that registry for the session runner.
- Do not globally mutate the server's shared built-in registry when adding coordinator tools; build a per-session or per-mode registry to avoid cross-session leakage.
- SSE emits worker task lifecycle events with task ID, optional agent name, status, output file path, and usage summary.
- Server sessions use the coordinator registry when coordinator mode is active.
- Existing auth, rate limit, and session lifecycle behavior must not change when coordinator mode is off.

Agent loop changes in `internal/agent/agent.go`:

- After tool execution, before next model call: `drainMailbox(taskID, supervisor)`.
- `drainMailbox` appends drained messages as user-role `llm.Message` entries to the conversation, with stable XML or bracketed metadata (`from`, `sent_at`, `message_id`) and plain text content.
- Drain task completion notifications in the same boundary pass so coordinator sees worker completions before deciding the next action.
- If task ID is empty (session not tracked by supervisor), skip drain silently.
- Do not drain while the stream accumulator is collecting Ollama tool calls.

Tests:

- `IsCoordinatorMode` returns true for `NANDOCODEGO_COORDINATOR=1`.
- `IsCoordinatorMode` returns false for empty, `0`, `false`.
- Coordinator registry contains exactly 3 tools.
- Worker registry excludes all `CoordinatorInternalTools`.
- Agent tool schema includes worker `name` in coordinator mode and maps `run_in_background` alias to `background` if implemented.
- Agent tool `background=true` returns before worker completion in coordinator mode; test with a blocking fake worker and assert the tool returns promptly with task metadata.
- Coordinator worker cap defaults to 3; config may raise to 5; 6th active worker is always rejected.
- `BuildCoordinatorSystemPrompt` output contains "never delegate understanding" phrase.
- `BuildCoordinatorSystemPrompt` output contains worker tool names list.
- Mailbox drain in agent loop: queued messages appear as user messages in next turn.
- Server session in coordinator mode exposes the same restricted registry and emits worker lifecycle SSE events.

### Step 7 — Dream Integration in REPL

Files:

- `internal/tui/app.go` (or `internal/cli/repl.go`)
- `internal/server/session.go`

Implement:

- After each agent run terminal event, if `IsDreamEnabled()` and no dream is currently running: `supervisor.SpawnDream(...)`.
- Before starting each new agent run: `supervisor.KillDream()`.
- If `DreamTaskState.Result` is non-empty at run start, prepend as a `[dream]` system message in `agent.Input.Messages`.
- Apply the same kill-before-new-input rule to server sessions before accepting a new user message.
- Do not spawn dreams while active coordinator workers exist or when the session is already processing queued prompts.

Tests:

- Dream is spawned after terminal event when enabled.
- Dream is killed before next run start.
- Kill completes within 100ms deadline.
- Dream result injected as system message when available.
- No dream spawned when `NANDOCODEGO_DREAM` is not set.
- No dream spawned while coordinator workers are active.
- Server session kills dream before starting a new submitted message.

### Step 8 — Coordinator Status in TUI

Files:

- `internal/tui/app.go`
- `internal/tui/styles.go` (or status bar rendering)
- `internal/state/app.go`
- `internal/server/types.go`

Implement:

- Status bar shows `[COORDINATOR]` badge when `IsCoordinatorMode()`.
- Worker task completions are already rendered as system transcript items via Phase 14 task notifications.
- `state.App` exposes coordinator mode and active worker count for TUI/server read models.
- Server status/session snapshots include `coordinator_mode` and `worker_count` fields without breaking existing JSON clients.

Tests (TUI unit tests, no live model):

- Status bar view contains "[COORDINATOR]" when coordinator mode active.
- Status bar view does not contain "[COORDINATOR]" when coordinator mode inactive.
- Server session snapshot includes coordinator mode and worker count.

### Step 9 — Tests and Race Checks

Required commands:

```sh
go test -race ./internal/tasks/...
go test -race ./internal/tools/sendmessage/...
go test -race ./internal/agent/...
go test -race ./internal/server/...
go test ./internal/tui/...
go test ./...
go vet ./...
tools/check-allowed-deps.sh
tools/check-network-policy.sh
```

End-to-end coordinator test (integration, requires Ollama):

```sh
go test -tags=integration -run TestCoordinatorEndToEnd ./internal/agent
```

Manual smoke test:

```sh
NANDOCODEGO_COORDINATOR=1 go run ./cmd/nandocodego --no-alt-screen
```

Submit: "Use 3 worker agents to count Go files in this repo. Each worker should scan a different directory."
Verify: coordinator spawns 3 workers, all complete, coordinator synthesizes total.

Server smoke test:

```sh
NANDOCODEGO_COORDINATOR=1 go run ./cmd/nandocodego server
```

Submit the same task through the HTTP session API and verify SSE includes worker task events plus the final coordinator response.

Ollama request validation:

- In fake-client tests, assert `ChatRequest.Options["num_ctx"]` is present for coordinator and worker requests when runtime config sets it.
- In stream tests, assert mailbox messages are not drained until after a complete assistant turn with streamed tool calls is assembled.

## Agent-Ready Implementation Slices

Use these slices for parallel implementation agents. Each slice has an owned write scope to reduce merge conflicts.

### Slice P24-0 — Preflight and Existing Contracts

Owner scope: tests and small documentation notes only.

- Verify current type names and file paths: `tasks.Supervisor`, `internal/tools/agenttool/agenttool.go`, `internal/agent/stream.go`, `internal/server/session.go`.
- Add failing characterization tests for current agent tool schema, stream accumulation, and task summary behavior before changing them.
- Confirm Phase 21/22 server tests are green before coordinator changes.
- Acceptance: no production behavior changes; tests document the baseline that later slices extend.

### Slice P24-1 — Mailbox and Task State

Owner scope: `internal/tasks/*`, `internal/types/*` only.

- Implement bounded `Mailbox`, `PendingMessage`, duplicate suppression, notify channel behavior, and mailbox JSONL append helper.
- Extend supervisor task state with agent mailbox metadata without changing public task summary shape except optional coordinator fields.
- Add name registry with validation and duplicate-live-name rejection.
- Add active worker count helper and terminal-state guards.
- Acceptance: `go test -race ./internal/tasks/...` passes with capacity, duplicate, JSONL permission, and state-transition tests.

### Slice P24-2 — SendMessage Tool

Owner scope: `internal/tools/sendmessage/*` and registry wiring files only.

- Implement input schema with `to`, `message`, `summary`, `message_id`, and `wait_for_reply`.
- Implement in-process routing, terminal auto-resume hook point, secure UDS client routing, and `bridge:` rejection.
- Validate message size, direct task ID format, UDS path containment, and unsupported blocking waits.
- Acceptance: `go test -race ./internal/tools/sendmessage/...` passes; duplicate messages are not queued twice.

### Slice P24-3 — Auto-Resume

Owner scope: `internal/agent/resume.go`, `internal/agent/resume_test.go`, and narrow supervisor hooks.

- Reconstruct newest valid history from JSONL, filtering incomplete streamed turns and unmatched tool pairs.
- Add or consume replayable sidechain transcript records; event-only JSONL is insufficient for resume.
- Apply context-budget truncation before appending the new user message.
- Spawn a new background agent task with a fresh ID and mailbox, then move the agent name mapping.
- Acceptance: corrupted JSONL fails safely; over-budget history truncates at complete turn boundaries; old terminal task remains terminal.

### Slice P24-4 — Coordinator Registry and Prompt

Owner scope: `internal/agent/coordinator.go`, `internal/agent/coordinator_test.go`, `internal/tools/agenttool/agenttool.go`.

- Build coordinator config parsed once per session.
- Build coordinator registry with exactly `Agent`, `SendMessage`, `TaskStop`.
- Build worker registry excluding all internal coordination/task-observation tools not allowed to workers.
- Extend Agent tool with worker names, coordinator default-background behavior, and true detached launch.
- Build the coordinator prompt from book teachings: four phases, never delegate understanding, parallelism model, continue-vs-spawn matrix, prompt anti-patterns, and worker output synthesis.
- Acceptance: registry tests prove exactly three coordinator tools; worker cap enforced; prompt contains required methodology sections.

### Slice P24-5 — Agent Loop Drain and Notifications

Owner scope: `internal/agent/agent.go`, `internal/agent/stream.go`, related tests only.

- Add mailbox/task-notification drain at safe turn boundaries.
- Preserve Ollama streaming behavior by draining only after complete assistant turns.
- Inject pending messages with stable metadata and plain text content.
- Acceptance: streamed tool-call test proves no partial tool call execution or mid-stream message injection; coordinator sees worker completion before terminal response.

### Slice P24-6 — Dream Lifecycle

Owner scope: `internal/tasks/dream.go`, dream tests, narrow REPL/server hooks.

- Add `KindDream`, `DreamTaskState`, spawn, cancel, result freshness, and no-list behavior.
- Kill dream before every new user message in REPL and server sessions.
- Block dreams while coordinator workers are active.
- Acceptance: cancellation state update occurs within 100ms; dream output is injected only when fresh and unused.

### Slice P24-7 — REPL, Server, TUI, and SSE Wiring

Owner scope: `internal/cli/*`, `internal/server/*`, `internal/tui/*`, `internal/state/*`.

- Wire coordinator config into REPL and server session creation.
- Add server coordinator registry/supervisor construction because normal server sessions currently do not register task tools or `Agent`.
- Expose coordinator mode and worker count in state/status snapshots.
- Emit worker task lifecycle events over SSE.
- Add `[COORDINATOR]` TUI badge.
- Acceptance: coordinator mode has no effect when env is off; server and TUI tests cover visible status and task events.

### Slice P24-8 — End-to-End, Race, and Docs

Owner scope: integration tests, `docs/PHASE-LOG.md`, and final verification only.

- Add fake-client coordinator E2E test for three workers and synthesis.
- Add optional Ollama integration test guarded by tags/env.
- Run race suites, full tests, dependency/network checks, and manual REPL/server smoke tests.
- Update phase log with implementation deviations and test results.
- Acceptance: all exit-gate commands pass or documented Ollama-only checks are skipped because no local Ollama endpoint is available.

## Implementation Checklist

### 2026-05-19 Implementation Update (Current)

Completed in code:
- `internal/agent/coordinator.go` added with `IsCoordinatorMode`, `IsDreamEnabled`, `CoordinatorConfig`, `BuildCoordinatorRegistry`, `BuildWorkerRegistry`, and `CoordinatorInternalTools`.
- `internal/tools/agenttool` now supports coordinator worker fields (`name`, `run_in_background`) and coordinator-mode supervisor-backed async worker launches with worker-cap enforcement.
- `internal/tools/sendmessage` now supports terminal-task resume hook injection (`WithResumeFunc`) and keeps in-process + UDS routing behavior.
- REPL wiring now provides a concrete resume hook that relaunches a supervisor-backed worker task when `SendMessage` targets a terminal worker.
- `internal/tasks` now includes dream lifecycle primitives (`SpawnDream`, `ConsumeDreamResult`, `KillDream` integration) and excludes `KindDream` from `Supervisor.List()`.
- `internal/agent` run loop now supports safe turn-boundary pending-message injection through `agent.Input.PendingMessagesProvider` (never mid-stream).
- REPL wiring now registers `SendMessage`, builds worker registry in coordinator mode, and switches active runner registry to coordinator-only tools (`Agent`, `SendMessage`, `TaskStop`).
- `state.App` and UI now expose coordinator status (`CoordinatorMode`, `WorkerCount`) and TUI status bar shows `[COORDINATOR]`.
- Server session view now includes `coordinator_mode` and `worker_count` fields.
- New/updated tests:
  - `internal/agent/coordinator_test.go`
  - `internal/tasks/dream_test.go`
  - `internal/tools/agenttool/agenttool_test.go`
  - `internal/tools/sendmessage/sendmessage_test.go`
  - `internal/tui/app_test.go`
  - `internal/server/handler_test.go`

Verification run:
- `go test ./...` passed on 2026-05-19.

Phase 24 exit-gate status (2026-05-19, complete):
- Replayable sidechain records are persisted as `kind=message` JSONL records in agent task outputs and are consumed by resume reconstruction helpers.
- Auto-resume routes for coordinator `SendMessage` now relaunch supervisor-backed workers with reconstructed replay context from structured replay records (with safe fallback to output tail text).
- Mailbox delivery is wired at safe turn boundaries via `agent.Input.PendingMessagesProvider` and consumed by worker runs through supervisor mailbox drains.
- Coordinator mode is wired in both REPL and server session paths with restricted registry behavior.
- Server sessions emit coordinator worker lifecycle over SSE as `task_lifecycle` events driven by session-local task-state watchers.
- Dream lifecycle is wired in REPL and server flows:
  - kill-before-next-input,
  - spawn-after-terminal-when-idle,
  - fresh dream result injection as `[dream]` system context before the next run.
- Verification command set passed:
  - `go test ./...`
  - `go test -race ./internal/tasks/... ./internal/tools/sendmessage/... ./internal/agent/... ./internal/server/...`
  - `go vet ./...`
  - `tools/check-allowed-deps.sh`
  - `tools/check-network-policy.sh`
- Live server smoke validation passed in coordinator mode (`NANDOCODEGO_COORDINATOR=1`):
  - `GET /v1/health` returned `{"ollama":"reachable","status":"ok"}`
  - session create succeeded
  - message post accepted (`{"queued":true}`)

### Mailbox

- [ ] Create `internal/tasks/mailbox.go` with `PendingMessage`, `Mailbox`, `NewMailbox`.
- [ ] Add `PendingMessage` fields: `ID`, `FromAgent`, `ToAgent`, `Summary`, `Content`, `SentAt`.
- [ ] Implement `Mailbox.Enqueue(msg PendingMessage) error` — thread-safe, validates ID/content, closes+replaces notify channel.
- [ ] Enforce `DefaultMailboxCapacity = 128`.
- [ ] Enforce `DefaultMailboxMessageBytes = 64 * 1024`.
- [ ] Implement duplicate `message_id` suppression with bounded `seenIDs`.
- [ ] Return `ErrMailboxFull` instead of dropping queued messages.
- [ ] Implement `Mailbox.Drain() []PendingMessage` — atomic swap, returns nil on empty.
- [ ] Implement `Mailbox.Notify() <-chan struct{}` — returns notify channel under lock.
- [ ] Implement `Mailbox.Len() int`.
- [ ] Write `mailbox_test.go` with concurrent enqueue/drain race test.
- [ ] Test capacity overflow does not lose already queued messages.
- [ ] Test oversized message rejection.
- [ ] Test duplicate message ID is drained once.
- [ ] Test `Notify` channel is closed when message arrives.
- [ ] Test second `Notify` after drain returns new open channel.

### Task State Extensions

- [ ] Extend agent task state with `Mailbox *Mailbox`.
- [ ] Extend agent task state with `MailboxFile string`.
- [ ] Extend agent task state with `AgentName string`.
- [ ] Extend agent task state with `EvictAfter *time.Time`.
- [ ] Extend agent task state with `Notified bool` and `OutputOffset int64` if not already represented.
- [ ] Initialize `Mailbox: tasks.NewMailbox()` in agent task spawn path.
- [ ] Create task directories with `0700` and mailbox JSONL files with `0600`.
- [ ] Implement `Supervisor.QueueMessage(taskID, msg)`.
- [ ] Implement `Supervisor.DrainMessages(taskID) ([]PendingMessage, error)`.
- [ ] Append accepted queued messages to `<task-dir>/mailbox.jsonl`.
- [ ] Add `agentNameRegistry map[string]string` field to `Supervisor`.
- [ ] Implement `Supervisor.RegisterName(name, taskID) error`.
- [ ] Implement `Supervisor.LookupByName(name) (string, bool)`.
- [ ] Implement `Supervisor.UnregisterName(name)`.
- [ ] Implement `Supervisor.ActiveWorkerCount() int`.
- [ ] Implement `Supervisor.KillDream() error`.
- [ ] Write tests for `QueueMessage` on non-existent task (error).
- [ ] Write tests for `QueueMessage` + `DrainMessages` round-trip.
- [ ] Write tests for mailbox JSONL append and file permissions.
- [ ] Write tests for `RegisterName` + `LookupByName`.
- [ ] Write tests for duplicate/invalid agent names.
- [ ] Write test for `KillDream` with no active dream (no-op).
- [ ] Write test that terminal task state is not mutated back to running during auto-resume.

### Dream Task

- [ ] Add `KindDream TaskKind = "d"` to task kind constants.
- [ ] Create `internal/tasks/dream.go` with `DreamTaskState` struct.
- [ ] Implement `DreamTaskState.Kind()`, `DreamTaskState.TaskID()`, `DreamTaskState.isTaskState()`.
- [ ] Implement `Supervisor.SpawnDream(ctx, client, model, systemPrompt) error`.
- [ ] Dream uses child context derived from session context; cancel stored in `DreamTaskState.Cancel`.
- [ ] Dream goroutine stores final assistant message in `DreamTaskState.Result` on completion.
- [ ] `KillDream` calls `DreamTaskState.Cancel()` and sets status to `killed`.
- [ ] Dream uses no tools and no memory runner.
- [ ] Dream uses low `MaxTurns` and bounded effective context.
- [ ] Dream stores `CompletedAt` and result freshness state.
- [ ] Dream is not started while coordinator workers are active.
- [ ] Write test: `SpawnDream` completes normally.
- [ ] Write test: `KillDream` completes within 100ms.
- [ ] Write test: supervisor does not spawn second dream if one is running.
- [ ] Write test: dream does not appear in `Supervisor.List()` output.
- [ ] Write test: dream does not start while active coordinator workers exist.

### SendMessage Tool

- [ ] Create `internal/tools/sendmessage/sendmessage.go`.
- [ ] Implement `Input` struct with `To`, `Message`, `Summary`, `MessageID`, `WaitForReply` fields.
- [ ] Implement `SendMessageTool` with `supervisor *tasks.Supervisor` dependency.
- [ ] Define JSON input schema with required fields.
- [ ] Set `IsConcurrencySafe = false`, `IsReadOnly = false`.
- [ ] Implement in-process routing: `LookupByName` + `QueueMessage` path.
- [ ] Generate `message_id` when omitted.
- [ ] Implement direct task ID routing (bypass name registry if `to` matches `a-[0-9a-f]{12}` pattern).
- [ ] Implement terminal-state routing: call `AutoResume`.
- [ ] Implement `wait_for_reply=true` rejection with `ErrNotSupported`.
- [ ] Implement message byte limit validation.
- [ ] Implement secure `uds:` routing: path containment under `paths.StateDir()/sockets`, `net.Dialer` timeout, write JSON line, close.
- [ ] Implement `bridge:` routing: return `ErrNotSupported`.
- [ ] Write test: message to running agent queues within 10ms.
- [ ] Write test: duplicate `message_id` queues once.
- [ ] Write test: `wait_for_reply=true` returns `ErrNotSupported`.
- [ ] Write test: oversized message is rejected.
- [ ] Write test: message to completed agent calls auto-resume.
- [ ] Write test: message to unknown agent returns error.
- [ ] Write test: `uds:` routing with fake UDS listener.
- [ ] Write test: `uds:` path outside state socket directory rejected before dial.
- [ ] Write test: `bridge:` returns `ErrNotSupported`.
- [ ] Write concurrent routing test under `-race`.

### Auto-Resume

- [ ] Implement `AutoResume(ctx, task, newMessage, client, registry, supervisor)` function.
- [ ] Add replayable sidechain transcript records for worker runs: complete `llm.Message` values plus turn number and timestamp.
- [ ] Persist worker restart metadata: model, system prompt, permission mode, toolset/worker policy, original prompt, agent name, output path, transcript path.
- [ ] Keep event JSONL for UI/debugging, but do not use event-only `text_delta`/`tool_start` records as resume history.
- [ ] Read replayable JSONL message records line by line into `[]llm.Message`.
- [ ] Filter orphaned thinking blocks (no following message referencing them).
- [ ] Filter unmatched tool-use messages (no corresponding tool-result).
- [ ] Filter unmatched tool-result messages.
- [ ] Filter incomplete streamed assistant turns that never reached done.
- [ ] Apply effective context budget before appending the new user message.
- [ ] Truncate only at complete turn boundaries.
- [ ] Append `newMessage` as user-role `llm.Message`.
- [ ] Register new background task in supervisor (new ID).
- [ ] Move agent name registry entry from old terminal task to new task ID.
- [ ] Keep old task state terminal and evictable.
- [ ] Start `agent.Agent.Run` goroutine linked to new task.
- [ ] Return new task ID string.
- [ ] Write test: 5-message JSONL reconstructs correctly.
- [ ] Write test: orphaned thinking block removed.
- [ ] Write test: unmatched tool-use removed.
- [ ] Write test: unmatched tool-result removed.
- [ ] Write test: over-budget transcript truncates at a complete turn.
- [ ] Write test: new message appended as final user message.
- [ ] Write test: corrupted JSONL returns error.
- [ ] Write test: event-only JSONL without replayable message records fails with a clear resume error.

### Coordinator Mode

- [ ] Implement `IsCoordinatorMode() bool` in `internal/agent/coordinator.go`.
- [ ] Implement `IsDreamEnabled() bool`.
- [ ] Implement `CoordinatorConfig` parsed once per session.
- [ ] Implement `BuildCoordinatorSystemPrompt(workerToolNames, scratchpadDir)` — full ~370-line prompt.
- [ ] Prompt must contain "never delegate understanding" phrase.
- [ ] Prompt must contain four-phase workflow (Research/Synthesize/Implement/Verify).
- [ ] Prompt must contain continue-vs-spawn decision guidance.
- [ ] Prompt must contain anti-pattern examples.
- [ ] Prompt must contain dynamically inserted worker tool names.
- [ ] Keep volatile worker/MCP/scratchpad inventory out of tool descriptions.
- [ ] Implement `BuildCoordinatorRegistry(agentTool, sendMessageTool, taskStopTool)`.
- [ ] Implement `BuildWorkerRegistry(full)` excluding `CoordinatorInternalTools`.
- [ ] Extend `internal/tools/agenttool/agenttool.go` schema with optional worker `name`.
- [ ] Add optional supervisor/coordinator dependency to `agenttool.Tool` so coordinator `Agent` calls create supervisor-backed `KindAgent` tasks.
- [ ] Add `run_in_background` alias only if needed for compatibility; map it to existing `background`.
- [ ] Default coordinator worker spawns to background execution.
- [ ] Make coordinator background worker launch truly detached: return task metadata immediately after worker task registration.
- [ ] Keep non-coordinator foreground/blocking Agent behavior unless covered by a separate explicit change.
- [ ] Register named workers with `Supervisor.RegisterName`.
- [ ] Enforce default max 3 active workers, configurable max 5, and unconditional rejection of a 6th active worker.
- [ ] Disable fork mode when coordinator mode is active.
- [ ] Wire coordinator mode in `internal/cli/repl.go`.
- [ ] Wire coordinator mode in `internal/server/session.go`.
- [ ] Add server coordinator registry construction with `Agent`, `SendMessage`, `TaskStop`, and a session-local supervisor.
- [ ] Preserve existing server built-in/MCP registry behavior when coordinator mode is off.
- [ ] Emit worker lifecycle SSE events from server sessions.
- [ ] Wire coordinator-aware `drainMailbox` in `internal/agent/agent.go` main loop.
- [ ] Drain task completion notifications and mailbox messages at the same safe turn boundary.
- [ ] Write test: `IsCoordinatorMode` correctly parses env var.
- [ ] Write test: coordinator registry has exactly 3 tools.
- [ ] Write test: worker registry excludes `SendMessage`, `Agent`, `TaskCreate`, `TaskGet`, `TaskStop`, `TaskList`, `TaskOutput`.
- [ ] Write test: Agent tool schema supports coordinator worker names.
- [ ] Write test: coordinator Agent background launch returns before a blocking worker completes.
- [ ] Write test: worker cap defaults to 3 and always rejects a 6th active worker.
- [ ] Write test: coordinator system prompt contains required phrases.
- [ ] Write test: mailbox drain injects messages as user-role conversation entries.
- [ ] Write test: server coordinator session uses restricted registry and emits worker events.

### Dream REPL Integration

- [ ] Wire `SpawnDream` after agent terminal event when `IsDreamEnabled()`.
- [ ] Wire `KillDream` before starting next agent run.
- [ ] Wire `KillDream` before server session starts a new user message.
- [ ] Inject dream result as system message when non-empty.
- [ ] Skip dream spawn while coordinator workers are active.
- [ ] Skip stale dream result injection after 30s.
- [ ] Write TUI test: dream spawned after terminal event.
- [ ] Write TUI test: dream killed before next run start within 100ms.
- [ ] Write TUI test: dream result injected as system message.
- [ ] Write server test: dream killed before new user message.
- [ ] Write test: dream skipped while active workers exist.

### Coordinator TUI Status

- [ ] Add `[COORDINATOR]` badge to TUI status bar when coordinator mode active.
- [ ] Add coordinator mode and active worker count to state snapshot.
- [ ] Add coordinator mode and active worker count to server session/status JSON.
- [ ] Write TUI unit test: status bar contains badge in coordinator mode.
- [ ] Write TUI unit test: status bar does not contain badge in normal mode.
- [ ] Write server unit test: coordinator fields appear without breaking existing clients.

### Integration and Final Checks

- [ ] Run `go test -race ./internal/tasks/...` — clean.
- [ ] Run `go test -race ./internal/tools/sendmessage/...` — clean.
- [ ] Run `go test -race ./internal/agent/...` — clean.
- [ ] Run `go test -race ./internal/server/...` — clean.
- [ ] Run `go test ./...` — no regressions.
- [ ] Run `tools/check-allowed-deps.sh` — no new deps.
- [ ] Run `tools/check-network-policy.sh` — clean.
- [ ] Run `go vet ./...` — clean.
- [ ] Manual coordinator smoke test with real Ollama.
- [ ] Manual server coordinator smoke test with real Ollama.
- [ ] Fake-client test verifies coordinator/worker requests preserve `options.num_ctx`.
- [ ] Stream test verifies mailbox drain waits for complete streamed tool-call turn.
- [ ] `SendMessage` to completed agent auto-resume test.
- [ ] Dream task kill-within-100ms confirmed.
- [ ] Update `docs/PHASE-LOG.md` with Phase 24 entry.

## Acceptance Criteria

- [ ] `SendMessage` tool exists in `internal/tools/sendmessage` and is registered in the coordinator's tool registry.
- [ ] `SendMessage` to an in-process running sub-agent delivers the message within 10ms (queued delivery at next tool-round boundary).
- [ ] `SendMessage` to a background task agent appends to its `Mailbox` queue atomically.
- [ ] `SendMessage` persists accepted mailbox messages to `mailbox.jsonl` under the task directory.
- [ ] `SendMessage` suppresses duplicate `message_id` deliveries.
- [ ] `SendMessage` enforces mailbox capacity and message byte limit with explicit errors.
- [ ] `SendMessage wait_for_reply=true` returns `ErrNotSupported` in v0.1.
- [ ] `SendMessage` to a completed agent transparently restarts it from its replayable sidechain transcript.
- [ ] Auto-resume reconstructs conversation history correctly with orphaned blocks filtered.
- [ ] Auto-resume uses replayable `llm.Message` sidechain records, not event-only JSONL deltas.
- [ ] Auto-resume truncates over-budget history at complete turn boundaries.
- [ ] Auto-resume keeps the original task terminal and moves name mapping to the new task ID.
- [ ] `SendMessage` to `bridge:` address returns `ErrNotSupported` with a descriptive message.
- [ ] `SendMessage` to `uds:` address connects to a Unix Domain Socket and delivers the message.
- [ ] `SendMessage` rejects `uds:` paths outside `paths.StateDir()/sockets`.
- [ ] Coordinator mode activated by `NANDOCODEGO_COORDINATOR=1`; no behavior change without the env var.
- [ ] Coordinator registry contains exactly 3 tools: `Agent`, `SendMessage`, `TaskStop`.
- [ ] Worker registry excludes all `CoordinatorInternalTools` entries.
- [ ] Coordinator mode is wired in both REPL and server sessions.
- [ ] Server SSE emits worker lifecycle events during coordinator sessions.
- [ ] Coordinator system prompt contains "never delegate understanding" principle and four-phase workflow.
- [ ] Agent tool supports coordinator worker names and registers them with the supervisor.
- [ ] Coordinator Agent background launch returns immediately with task metadata and does not block until worker completion.
- [ ] Coordinator worker cap defaults to 3 active workers; config may raise to 5; a 6th active worker is always rejected.
- [ ] Worker agents in coordinator mode do not share mutable permission-denial or permission-prompt state with the coordinator.
- [ ] Worker agents do NOT auto-inherit coordinator's TUI `setAppState` (workers use no-op state function).
- [ ] Workers share the model, tool registry, and `llm.Client` with the coordinator.
- [ ] Pending messages are drained at tool-round boundaries — never mid-generation.
- [ ] Streamed Ollama tool calls are fully accumulated before mailbox/task notifications are injected.
- [ ] Coordinator and worker requests preserve effective `options.num_ctx` when runtime config sets it.
- [ ] Dream task is feature-gated by `NANDOCODEGO_DREAM=1`.
- [ ] Dream task is killed within 100ms of `KillDream()` call.
- [ ] Dream result is injected as a system message when it completes before the next user message.
- [ ] Dream does not start while coordinator workers are active.
- [ ] `go test -race ./internal/tasks/... ./internal/tools/sendmessage/...` passes.
- [ ] `go test -race ./internal/server/...` passes.
- [ ] `tools/check-allowed-deps.sh` passes — no new direct dependencies.
- [ ] All pre-Phase-24 tests pass without modification.
- [ ] Coordinator end-to-end test: 3 parallel workers complete and coordinator synthesizes result.
- [ ] Server coordinator smoke test: 3 parallel workers complete and final response arrives over session/SSE.
- [ ] Phase log updated with Phase 24 entry.
- [ ] `ModeBubble` workers auto-deny permission escalation in v0.1 (behavior documented in code comments).
- [ ] `KindDream` does not appear in `Supervisor.List()` output.
- [ ] Mailbox `Drain` is atomic — concurrent enqueue during drain does not lose messages.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---:|---|
| Mailbox drain races with coordinator shut-down | High | `Mailbox.Drain` uses mutex; supervisor `KillDream` and message drain are separate non-racing paths. |
| Mailbox grows without bound | High | Enforce 128-message queue cap, 64 KiB message cap, duplicate suppression, and explicit backpressure errors. |
| Auto-resume reconstructs history with orphaned tool-use | High | Filter pass removes unmatched tool-use messages before appending new message. |
| Auto-resume tries to replay event-only JSONL | High | Store complete `llm.Message` sidechain records and fail clearly if only event deltas are available. |
| Auto-resume replays too much transcript | High | Apply effective `num_ctx` budget and truncate at complete turn boundaries before spawning resumed worker. |
| Dream task goroutine leaks if `KillDream` is never called | Medium | Dream context is derived from session context; session context cancel kills dream on REPL exit. |
| Coordinator spawns too many workers, exhausting model VRAM | High | Enforce default 3 active workers, configurable max 5, and 6th-worker rejection in code; do not rely only on prompt guidance. |
| `SendMessage` `uds:` path allows socket path traversal | Medium | Validate socket path is within `paths.StateDir()/sockets`; reject traversal, symlink escapes, and absolute paths outside the socket dir. |
| Worker permission escalation to coordinator blocks coordinator | Medium | Auto-deny `ModeBubble` workers in this phase; defer full escalation to a future multi-agent hardening follow-up to avoid deadlock. |
| JSONL resume reconstructs > 100K-token history | Medium | Cap reconstructed history at the effective runtime `num_ctx` before appending new message; log warning. |
| Coordinator system prompt takes up too many tokens | Medium | Cap total system prompt at 8192 tokens; truncate worker tool list if needed. |
| Dream result is stale by the time it is injected | Low | Result freshness timestamp checked; if dream ran > 30s ago, skip injection. |
| Multiple concurrent `SendMessage` calls to same agent race | Medium | `Mailbox.Enqueue` is mutex-protected; concurrent callers serialize without data loss. |
| Mailbox drain executes partial streamed Ollama tool calls | High | Keep drain after `stream.go` completes assistant turn assembly; add regression test with streamed tool calls plus queued message. |
| Server coordinator mode diverges from REPL mode | Medium | Parse one coordinator config type and share registry/session construction tests across REPL/server paths. |
| Server coordinator mode lacks Agent/task tools | High | Build a coordinator-specific server registry and session-local supervisor when the feature gate is on; preserve normal server behavior when off. |
| Agent background mode still blocks the coordinator | High | Implement detached coordinator worker launch and add a blocking-worker test proving Agent returns before completion. |
| Workers can recurse through task-management tools | High | Exclude `TaskCreate`, `TaskGet`, `TaskList`, `TaskOutput`, `TaskStop`, `Agent`, and `SendMessage` from worker registries. |
| Dynamic worker tool inventory busts prompt/tool cache behavior | Low | Keep tool descriptions static; inject volatile worker/MCP/scratchpad context as normal messages/context. |

## Exit Gate

Phase 24 is complete only when:

- All acceptance criteria above are met.
- `go test -race ./internal/tasks/... ./internal/tools/sendmessage/... ./internal/agent/... ./internal/server/...` passes.
- `go test ./internal/tui/...` passes.
- `go test ./...` passes.
- `tools/check-allowed-deps.sh` passes.
- `tools/check-network-policy.sh` passes.
- `go vet ./...` passes.
- Manual coordinator smoke test with real Ollama: 3 parallel workers complete and coordinator synthesizes result.
- Manual server coordinator smoke test with real Ollama: worker task events arrive over SSE and final response is returned.
- `SendMessage` to completed agent auto-resume confirmed manually.
- Dream task kill-within-100ms confirmed via test assertion.
- Fake-client tests confirm effective `options.num_ctx` survives coordinator/worker requests.
- Fake-client test confirms coordinator `Agent(background=true)` returns promptly while the worker remains running.
- Auto-resume tests confirm replayable sidechain transcripts are used and event-only JSONL is rejected.
- Stream tests confirm mailbox delivery waits until after complete Ollama streamed tool-call turns.
- Phase log records implementation, test results, deviations from plan, and manual smoke test result.

## Phase Log Template

When implementation finishes, append a Phase 24 entry to `docs/PHASE-LOG.md` with:

- objective;
- files created and updated;
- dependencies added and allowlist status (expected: none);
- tests and checks run;
- manual coordinator smoke test result;
- manual server coordinator smoke test result;
- design decisions (especially mailbox drain protocol, auto-resume filtering, dream kill deadline);
- known constraints and deferred work (full `ModeBubble` escalation chain, bridge routing, swarm teams, full scratchpad protocol, blocking `wait_for_reply`);
- exit gate status.
