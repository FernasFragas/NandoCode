---
name: nandocodego-tool-interface
description: Tool interface contract, tools.Context fields, permission resolution flow, registry rules, and built-in tool list for nandocodego
type: reference
---

## Tool Interface (`internal/tools/tool.go`)

```go
type Tool interface {
    Name() string
    Description() string        // ≤ 1024 chars; first sentence ≤ 100 chars
    Aliases() []string
    JSONSchema() map[string]any

    IsEnabled(ctx Context) bool
    IsReadOnly(input any) bool         // default false
    IsConcurrencySafe(input any) bool  // default false
    IsDestructive(input any) bool      // default false

    CheckPermissions(ctx Context, input any) PermissionResult
    Call(ctx Context, input any, progress chan<- ProgressEvent) (Result, error)
    Render(input any, result Result) RenderHints
}

type Result struct {
    Data           any
    Display        string
    NewMessages    []llm.Message
    MaxResultChars int  // 0 = use default
}
```

## Permission Resolution Order (fail-closed)

1. Hook decision (`PreToolUse` command/prompt hook)
2. Rule match: `AlwaysDeny` > `AlwaysAsk` > `AlwaysAllow`
3. Tool's `CheckPermissions(input)` — per-call, not per-tool-name
4. Mode-based default
5. Interactive TUI prompt
6. Auto-mode classifier

## Permission Modes

`ModeBypass` | `ModeDontAsk` | `ModeAuto` | `ModeAcceptEdits` | `ModeDefault` | `ModePlan` | `ModeBubble`

Sub-agents always use `ModeBubble` as default.

## tools.Context Key Fields

```go
type Context struct {
    Ctx              context.Context
    Cancel           context.CancelFunc
    Logger           *slog.Logger
    LLM              llm.Client
    AgentID          ids.AgentID
    SessionID        ids.SessionID
    WorkingDir       string
    PermissionMode   permissions.Mode
    PermissionRules  permissions.Rules
    ToolSettings     ToolSettings  // MaxResultChars, MaxReadChars, AdditionalWorkingDirs
    AppState         func() state.App
    SetAppState      func(func(state.App) state.App)
    Hooks            hooks.Snapshot
    TodoList         *todo.TodoList
}
```

## Built-in Tools (current registry)

| Tool | Safe | ReadOnly | Destructive |
|---|---|---|---|
| `Bash` | AST-based | AST-based | yes for writes |
| `Read` | yes | yes | no |
| `Write` | no | no | yes |
| `Edit` | no | no | yes |
| `Glob` | yes | yes | no |
| `Grep` | yes | yes | no |
| `WebFetch` | no (PermAsk) | yes | no |
| `Agent` (subagent) | no | no | — |
| `Skill` | yes | yes | no |

## llm.Client Interface

```go
type Client interface {
    Chat(ctx, req) (<-chan StreamEvent, error)
    ChatOnce(ctx, req) (*StreamEvent, error)
    Embed(ctx, req) ([][]float32, error)
    ListModels(ctx) ([]ModelInfo, error)
    PullModel(ctx, name, progress) error
    ShowModel(ctx, name) (ModelDetails, error)
    Close() error
}
```

`ShowModel` returns: `ContextLength`, `Family`, `ParameterSize`, `QuantizationLevel`, `Parameters map[string]any`
`ComputeLimits(d ModelDetails) ModelLimits` derives `MaxOutputTokens` and `MaxResultChars` from actual model.
