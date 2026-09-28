# Debug Breakpoints Guide

This guide maps the critical execution paths in the system and tells you exactly where to place breakpoints to observe each stage of a request's lifecycle — from user keypress to terminal event.

---

## Architecture Overview

```
User types prompt
       ↓
TUI (Bubble Tea) captures input
       ↓
Agent Input built → Agent.Run() spawned in goroutine
       ↓
Agent loop: executeOneTurn() × N turns
       ↓
LLM streaming via Ollama HTTP client
       ↓
Tool calls extracted → executeToolCallsConcurrent()
       ↓
Permission resolved → Tool.Call() executed
       ↓
Results fed back to LLM → next turn or Terminal event
       ↓
Events drained async → TUI Update() re-renders
```

---

## 1. Process Startup

**File:** `cmd/nandocodego/main.go`

| Line | What to inspect |
|------|-----------------|
| `main()` entry | Signal context creation (`os.Interrupt`, `SIGTERM`) |
| `cli.Run(ctx, os.Args[1:])` call | All CLI arguments before parsing |
| `cli.ExitCode(err)` | Exit code mapping when CLI returns an error |

**When to use:** Verify the binary receives the expected arguments, or when the process exits unexpectedly without a visible error.

---

## 2. CLI Parsing and REPL Bootstrap

**File:** `internal/cli/root.go`

| Breakpoint | What to inspect |
|------------|-----------------|
| Root `RunE` function body | Parsed flags: `--model`, `--ollama-url`, `--log-level`, `--print`, `--permission-mode` |
| Config merge logic | How file config + flag overrides are combined |
| Dispatch to REPL vs print vs subcommand | Which path is taken based on args |

**File:** `internal/cli/repl.go`

| Breakpoint | What to inspect |
|------------|-----------------|
| `bootstrap.InitGlobal(initial)` | The resolved initial config (model name, Ollama URL, turn limits) |
| `state.DefaultApp(snap)` call | The initial `App` state snapshot |
| Ollama client creation | `baseURL`, whether observability wrapper is applied |
| Tool registration loops | Which tools get registered: builtin, skills, MCP, task tools |
| `agent.New(client, registry, ...)` | Agent config: `MaxTurns`, token budgets, watchdog settings |
| `tea.NewProgram(tuiModel, ...)` call | TUI options (alt screen, mouse support) |
| `program.Run()` | Last synchronous call before the event loop takes over |

**When to use:** Debugging startup failures, wrong model selection, missing tools, or config not being read correctly.

---

## 3. User Input → Agent Submission

**File:** `internal/tui/app.go`

| Breakpoint | What to inspect |
|------------|-----------------|
| `Update()` — `tea.KeyMsg` case, `"enter"` branch | Raw input string before processing |
| Input validation / slash-command check | Whether input is routed as `/command` or user prompt |
| `agent.Input{...}` construction | `Messages` slice (full history), `ToolContext`, `PermissionMode`, `PermissionRules` |
| `startAgentCmd(ctx, runner, input, send)` | The fully assembled `agent.Input` before it's handed off |
| Inside `startAgentCmd`: `runner.Run(ctx, input)` | The event channel returned by the agent |

**When to use:** Input isn't reaching the agent, history is malformed, wrong permission mode applied, or agent never starts.

---

## 4. Agent Main Loop

**File:** `internal/agent/agent.go`

| Breakpoint | What to inspect |
|------------|-----------------|
| `Run()` — goroutine launch | Confirm the agent goroutine starts |
| `run()` — top of loop (`for turn := 1; turn <= maxTurns`) | `turn` counter, `messages` slice length |
| `executeOneTurn(...)` call site | Arguments: model name, message count, tool count, token budget |
| After `executeOneTurn` returns | `turnResult`: assistant message content, tool call count |
| `executeToolCallsConcurrent(...)` call site | Tool calls list (names + raw JSON inputs) |
| After tool calls: append to `messages` | Updated message history length |
| Terminal condition check (`len(toolCalls) == 0`) | Whether the loop exits cleanly or continues |
| `events <- Terminal{...}` | Final `Reason` field: `completed`, `max_turns`, `context_overflow`, `aborted` |

**When to use:** Agent loops forever, hits `max_turns` unexpectedly, exits too early, or emits wrong terminal reason.

---

## 5. Single LLM Turn (Streaming)

**File:** `internal/agent/stream.go`

| Breakpoint | What to inspect |
|------------|-----------------|
| `executeOneTurn()` entry | Full `ChatRequest` before it's sent: model, message count, tool definitions |
| `buildToolDefs()` result | Which tools are included vs filtered out |
| `llm.Chat(ctx, req)` call | The actual HTTP request payload |
| `accumulateTurn()` entry | Stream channel received |
| Inside `for evt := range stream` | Each `StreamEvent`: delta content, tool call chunks, stop reason |
| `AssistantTextDelta` emit | Streaming text as it arrives |
| Tool call accumulation (partial chunks assembled) | Assembled `ToolCall` with name + arguments |
| Watchdog timer reset | Whether the watchdog fires (stream stall detection) |
| `accumulateTurn()` return | Final `turnResult`: assistant message + tool calls |

**When to use:** LLM not responding, partial tool calls, stream stalling, wrong tools sent to model.

---

## 6. Ollama HTTP Client

**File:** `internal/llm/ollama/ollama.go`

| Breakpoint | What to inspect |
|------------|-----------------|
| `Chat()` entry | `req.Model`, `len(req.Messages)`, `len(req.Tools)` |
| `json.Marshal(ollamaReq)` | Exact JSON being sent to Ollama |
| `http.Post(url, ...)` | URL and request body |
| HTTP response status check | Non-2xx status code and body |
| `json.NewDecoder(resp.Body).Decode(&evt)` in goroutine | Each raw event chunk decoded from the stream |
| Channel send `out <- StreamEvent{...}` | The `StreamEvent` being pushed to the agent |
| Context cancellation check | Whether cancellation is handled mid-stream |

**When to use:** Ollama connectivity issues, malformed requests, unexpected model responses, stream errors.

---

## 7. Tool Execution

**File:** `internal/agent/tools.go`

| Breakpoint | What to inspect |
|------------|-----------------|
| `executeToolCallsConcurrent()` entry | `toolCalls` slice: names, raw JSON inputs |
| Tool registry lookup (`registry.Get(name)`) | Whether the tool is found; log name if nil |
| `tool.UnmarshalInput(rawArgs)` | Parsed input struct before execution |
| `permissions.Resolve(ctx, req)` call | `Request`: tool name, mode, rules, input |
| `permissions.Resolve()` return | `Decision` (Allow/Deny/Prompt) and `Reason` |
| `events <- ToolUseStart{...}` | Tool ID, tool name emitted to TUI |
| `tool.Call(ctx, input, progress)` | Tool name and parsed input just before execution |
| Progress channel reads | Intermediate output while tool runs |
| `tool.Call()` return | `Result.Data`, `Result.Display`, error if any |
| `events <- ToolUseResult{...}` | Final result or error sent to TUI |

**When to use:** Tool not found, permission denied unexpectedly, wrong input parsed, tool crashes, result not shown in TUI.

---

## 8. Permission Resolution

**File:** `internal/permissions/resolver.go`

| Breakpoint | What to inspect |
|------------|-----------------|
| `Resolve()` entry | `Request`: `Mode`, `Tool.Name()`, `Input`, `Rules` |
| AlwaysAllow rule matching | Which rule matched, or none |
| AlwaysDeny rule matching | Which rule triggered a deny |
| Hook invocation (if configured) | Hook input and the decision it returns |
| Interactive prompt path (`PromptFunc` call) | Whether UI prompt is triggered |
| `Resolve()` return | Final `Decision` enum value and human-readable `Reason` |

**When to use:** Tools being silently blocked, permission prompt not appearing, hook not firing, wrong mode applied.

---

## 9. Event Drain → TUI

**File:** `internal/tui/app.go` (bridge/drain goroutine)

| Breakpoint | What to inspect |
|------------|-----------------|
| `drainAgentEvents()` goroutine | Confirm it's running and receiving events |
| `send(agentEventMsg{Event: evt})` | Which event type is being forwarded to TUI |
| `Update()` — `agentEventMsg` case | Event type received by the TUI message loop |
| `handleAgentEvent(evt)` dispatch | Event type switch — which case is matched |

**File:** `internal/tui/app.go` — `handleAgentEvent()`

| Breakpoint | What to inspect |
|------------|-----------------|
| `AssistantTextDelta` case | `delta.Content` string appended to transcript |
| `ToolUseStart` case | Tool ID, tool name, initial display |
| `ToolUseProgress` case | Progress text content |
| `ToolUseResult` case | Result content, error flag, tool ID lookup |
| `Terminal` case | `Reason`, `Usage` (prompt/completion tokens) |
| `store.Set(...)` calls | State mutations: `ActiveTools`, `TerminalReason`, `Usage` |

**When to use:** Events arrive from agent but TUI doesn't update, tool results not shown, token counts wrong, terminal state not cleared.

---

## 10. State Management

**File:** `internal/state/app.go` + `internal/state/store.go`

| Breakpoint | What to inspect |
|------------|-----------------|
| `store.Set(func(app) App {...})` calls | Before/after snapshots of `App` state |
| `store.Get()` calls | What state the caller reads at that moment |
| `OnChange` callback | When and why subscribers are notified |
| `App.ToolContext()` construction | `tools.Context` built from settings: limits, paths, permission mode |

**When to use:** State appears stale, concurrent read/write races, wrong tool limits applied.

---

## 11. Context Compaction

**File:** `internal/agent/agent.go`

| Breakpoint | What to inspect |
|------------|-----------------|
| Reactive compaction trigger | Context overflow error classification |
| `doCompact()` entry | `messages` slice length and token count before compaction |
| `CompactionStarted` event emit | Confirm TUI receives it |
| `doCompact()` return | Compacted `messages` slice length |
| `CompactionCompleted` event emit | Summary tokens, new message count |
| Retry after compaction | Whether the same turn is retried with shorter history |

**When to use:** `context_overflow` terminal reason, compaction loop, messages disappearing from history.

---

## Quick Breakpoint Cheatsheet

```
Startup         cmd/nandocodego/main.go           → main()
CLI parsing     internal/cli/root.go              → RunE()
REPL init       internal/cli/repl.go              → agent.New(), program.Run()
User input      internal/tui/app.go               → Update() "enter" branch
Agent start     internal/agent/agent.go           → Run(), run() loop top
LLM request     internal/agent/stream.go          → executeOneTurn(), accumulateTurn()
HTTP to Ollama  internal/llm/ollama/ollama.go     → Chat(), decode loop
Tool dispatch   internal/agent/tools.go           → executeToolCallsConcurrent()
Permissions     internal/permissions/resolver.go  → Resolve()
TUI update      internal/tui/app.go               → handleAgentEvent()
State mutation  internal/state/store.go           → Set()
```

---

## Recommended Debugging Workflow

### For a full request trace:
1. Set breakpoints at `agent.Run()`, `executeOneTurn()`, `llm.Chat()`, `tool.Call()`, `handleAgentEvent()`
2. Submit a prompt with one tool call (e.g. a file read)
3. Step through in order: agent loop → HTTP → stream drain → tool execute → TUI update

### For permission issues:
1. Break at `permissions.Resolve()` entry and return
2. Inspect `Request.Mode`, `Request.Rules`, `Decision`

### For TUI not updating:
1. Break at `drainAgentEvents()` send call
2. Then break at `Update()` `agentEventMsg` case
3. Verify the event type is handled in `handleAgentEvent()`

### For LLM not responding / timeout:
1. Break at `ollama.Chat()` just before `http.Post`
2. Check the JSON payload
3. Break in the stream decode goroutine to see if any events arrive
4. Check watchdog timer reset logic in `accumulateTurn()`

### For wrong tool inputs:
1. Break at `tool.UnmarshalInput(rawArgs)` and inspect `rawArgs`
2. Then break at `tool.Call()` to see the parsed struct
