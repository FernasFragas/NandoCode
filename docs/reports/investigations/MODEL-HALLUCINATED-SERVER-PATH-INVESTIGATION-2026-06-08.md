# Model Hallucinated Server Path Investigation - 2026-06-08

## Scope

This report investigates why the application produced an inaccurate answer to this prompt:

```text
Find the server path that updates session models, verify whether cloud-listed models can actually be selected and summarize the behavioral currently
```

The model answered with a fabricated route, fabricated file path, and an incorrect cloud-model behavior summary. This report compares that answer against the current repository implementation and identifies likely product causes and fixes.

## Executive Summary

The bad answer was most likely caused by the request being routed as a general prompt with no retrieval and no tools. In that mode, the model had no live repository evidence and answered from generic priors about web-server architectures.

The highest-confidence root cause is the retrieval-route classifier in `internal/retrievalroute/route.go`. The prompt is clearly a codebase investigation request to a human, but it does not match the current `isWorkspaceDiscoveryPrompt` vocabulary strongly enough. Terms such as `server`, `route`, `endpoint`, `session`, `model`, `cloud-listed`, `selected`, and `behavior` are not enough today to force workspace discovery. When classified as a general prompt, the route decision can set `ToolModeNone`, which removes tool definitions from the LLM request.

The application also lacks a normal-run grounding system prompt or response validation gate that would prevent confident code claims when no files or tool results were inspected.

## Incorrect Claims In The Model Answer

The model answer claimed:

- HTTP route: `POST /api/model` or `/model`.
- Handler file: `internal/http/handlers/model.go`.
- Flow through `state.Session.SetModel(...)` and `bootstrap.state.modelChannel`.
- `hooks.Snapshot` and TUI `ShowModel` refresh triggered from that server path.
- `ComputeLimits(model)` runs as part of this HTTP server update path.
- Cloud-listed models cannot be selected unless already pulled locally.

These claims do not match the repository.

## Actual Current Implementation

The server route is registered in `internal/server/server.go`:

```go
mux.HandleFunc("/v1/sessions/", s.sessionRoutes)
```

Within `sessionRoutes`, the model update route is:

```go
case "model":
    if r.Method == http.MethodPost {
        s.handleUpdateModel(w, r, id)
        return
    }
```

The actual endpoint is:

```text
POST /v1/sessions/{id}/model
```

The handler is `internal/server/handler.go`:

```go
func (s *Server) handleUpdateModel(w http.ResponseWriter, r *http.Request, id string)
```

Current behavior:

- Returns `404` when the session does not exist.
- Parses `ModelUpdateRequest`.
- Returns `400` for invalid JSON or empty `model`.
- Returns `409` if the session already has an active run.
- If `s.modelRuntime != nil`, calls `s.modelRuntime.Switch(...)`.
- If runtime switching succeeds, persists resolved model/provider/base URL through `sess.applyModelSwitch(...)`.
- If no model runtime exists, validates against `s.client.ListModels(...)` and directly sets `sess.appState.ActiveModel`.
- Responds with `model`, `provider`, and `base_url`.

Cloud behavior:

- `modelruntime.Service.Switch(...)` resolves the requested model via `modelresolver.Resolver`.
- The resolver checks local models first.
- If cloud is enabled, it checks cloud catalog entries.
- The resolver supports cloud aliases including a final `:cloud` suffix and a final `-cloud` suffix.
- Cloud selection requires credentials and validates them by listing models through the cloud client.

Therefore, cloud-resolved models can be selected when the runtime resolver is active, cloud is enabled, and credentials validate.

## Likely Failure Chain

### 1. The prompt can be classified as a general prompt

`internal/retrievalroute/route.go` decides whether a prompt should receive semantic context, explicit context, local search, or no retrieval.

The key branch is:

```go
if !isWorkspaceDiscoveryPrompt(prompt) {
    return skipDecision(ActionSkipAllRetrieval, ReasonSkipGeneralPrompt, "Preparing model request...", "general_prompt", ToolModeNone)
}
```

The current workspace-discovery keywords include terms such as:

```text
fix, debug, bug, broken, error, failing, failure, panic,
implement, refactor, code, codebase, repo, repository, workspace,
project, file, function, method, class, package, module, test,
auth, authentication, handler, service, api, cli, tui
```

The user prompt contains domain-specific repo words, but not the strongest current trigger words:

```text
server path, updates session models, cloud-listed models, selected, behavior
```

Because `server`, `route`, `endpoint`, `session`, `model`, `cloud-listed`, `select`, and `behavior` are not direct workspace-discovery triggers, the app can classify the prompt as `general_prompt`.

### 2. General prompts remove tools

For `general_prompt`, the route decision uses `ToolModeNone`.

`internal/agent/stream.go` then removes all tools:

```go
func (a *Agent) buildToolDefs(registry *tools.Registry, toolCtx tools.Context) []llm.ToolDef {
    if strings.EqualFold(strings.TrimSpace(toolModeFromContext(toolCtx.Context)), ToolModeNone) {
        return nil
    }
    ...
}
```

That means the model cannot use `Grep`, `FileRead`, `Glob`, or `Bash` to verify the repository.

### 3. Current-turn context only expands explicit mentions

`contextpack.BuildCurrentTurnPrompt(...)` and `PackCurrentTurnPrompt(...)` expand explicit `@path` mentions and pack referenced evidence. The failing prompt had no explicit file or directory mention.

Without a route that enables semantic retrieval or tools, the model receives no relevant repository evidence.

### 4. Normal runs do not add a grounding system prompt

`internal/agent/agent.go` prepends a system prompt only when `agent.Input.SystemPrompt` is set:

```go
if in.SystemPrompt != "" {
    history = append([]llm.Message{{
        Role:    llm.RoleSystem,
        Content: in.SystemPrompt,
    }}, history...)
}
```

In the server flow, `Input.SystemPrompt` is only set for coordinator mode:

```go
if app.CoordinatorMode {
    in.SystemPrompt = agent.BuildCoordinatorSystemPrompt(nil, "")
}
```

For a normal single-agent server run, there is no default instruction requiring the model to inspect files before naming routes, handlers, or behavior.

### 5. There is no evidence/citation gate for code claims

The application records prompt metadata and emits routing events, but it does not block or flag final answers that make concrete repository claims without file evidence, semantic retrieval evidence, or tool results.

That allowed the model to produce a confident answer with:

- nonexistent files,
- nonexistent routes,
- nonexistent state functions,
- incorrect cloud-selection behavior.

## Contributing Product Gaps

### Missing intent triggers

The classifier under-detects repository investigation prompts that use product-specific words:

- `server`
- `route`
- `endpoint`
- `http`
- `session`
- `model`
- `model switch`
- `cloud`
- `cloud-listed`
- `select`
- `selected`
- `current behavior`
- `actual behavior`
- `verify`
- `path` when paired with server/code terms

### No local-search fallback for uncertain code questions

When semantic retrieval is skipped, there is no mandatory fallback to local grep/read tools for codebase questions. This makes classifier false negatives high impact.

### Tool removal is too aggressive for ambiguous technical prompts

`ToolModeNone` is useful for cheap prompts like "reply exactly OK", but it is risky for prompts that ask to "find", "verify", or "summarize current behavior" in an active repository.

### No visible user warning

The UI/server emits `retrieval_route_decided`, but the final answer does not necessarily tell the user:

```text
No repository files were inspected for this answer.
```

Without that, users see a confident answer and must manually audit it.

## Recommended Solutions

### P0: Expand route intent detection and add regression coverage

Update `isWorkspaceDiscoveryPrompt(...)` so this exact prompt routes to workspace discovery, not `general_prompt`.

Recommended approach:

- Add high-signal terms: `server`, `route`, `endpoint`, `session`, `model switch`, `cloud-listed`, `selected`, `current behavior`, `verify`.
- Prefer phrase/pair matching for broad terms like `model` and `path`.
- Treat prompts containing `find` plus server/code nouns as workspace discovery.
- Treat prompts containing `verify` plus repository/product nouns as workspace discovery.

Add tests in `internal/retrievalroute/route_test.go`:

```go
func TestDecideServerModelPathPromptUsesWorkspaceDiscovery(t *testing.T) {
    d := Decide(Input{
        RawPrompt: "Find the server path that updates session models, verify whether cloud-listed models can actually be selected and summarize the behavioral currently",
        ShouldQuery: true,
        SemanticEnabled: true,
        SemanticMode: "auto",
    }, Config{Mode: "auto"})

    if d.Action != ActionSemanticFull || !d.AllowEmbedding {
        t.Fatalf("decision=%+v", d)
    }
    if d.ToolMode != ToolModeDefault {
        t.Fatalf("tool mode=%q", d.ToolMode)
    }
}
```

### P0: Never use `ToolModeNone` for "find/verify current behavior" prompts

Add a guard before `ReasonSkipGeneralPrompt`:

- if prompt includes `find`, `verify`, `trace`, `where`, `current behavior`, or `actual behavior`
- and includes a repository/product noun such as `server`, `route`, `endpoint`, `session`, `model`, `handler`, `api`, `tui`, `cli`
- then keep `ToolModeDefault` and route to workspace discovery or local search.

This protects against false negatives even if semantic retrieval is disabled or unavailable.

### P1: Add a local-search fallback profile

For code-investigation prompts where semantic retrieval is disabled, index-missing, or classifier confidence is low, use `ActionLocalSearchOnly` with tools enabled instead of `ActionSkipAllRetrieval`.

Expected behavior:

- The model receives `Grep`, `FileRead`, and `Glob`.
- It can inspect route registration and handler files.
- It avoids fabricating architecture from priors.

### P1: Add a default grounding system prompt for normal runs

Add a concise default system prompt for non-coordinator agent runs:

```text
When answering questions about this repository, inspect the relevant files or tool results before naming routes, files, functions, behavior, or tests. If no repository evidence was inspected, say so explicitly and do not present guesses as facts.
```

This should be appended carefully so it does not override user instructions or existing coordinator prompts.

### P1: Surface evidence state in final responses or UI

When route decision is `skip_all_retrieval` with `ToolModeNone`, expose a visible notice before or after the assistant response:

```text
No repository context or tools were used for this response.
```

For codebase prompts, this should be treated as a warning, not a hidden event.

### P2: Add an answer-quality evaluation

Add an end-to-end or integration evaluation for prompts that require repo grounding:

1. Ask the exact failing prompt.
2. Assert route decision is not `general_prompt`.
3. Assert tool definitions are present or semantic context is attached.
4. Assert the final answer mentions the real route:

```text
POST /v1/sessions/{id}/model
```

5. Assert the final answer does not mention known fabricated values:

```text
POST /api/model
internal/http/handlers/model.go
state.Session.SetModel
bootstrap.state.modelChannel
```

### P2: Use prompt-dump metadata as a diagnostic

Prompt dumps already record prompt and tool metadata. Add a small diagnostic command or report section that shows:

- route action,
- route reason,
- tool mode,
- attached evidence count,
- tool schema count.

That would make this failure obvious after one run.

## Proposed Implementation Order

1. Add route tests for the exact failing prompt and adjacent variants.
2. Update `isWorkspaceDiscoveryPrompt(...)` with code-investigation phrase matching.
3. Add a guard that prevents `ToolModeNone` for `find/verify/current behavior` prompts about repository entities.
4. Add server/TUI test coverage that verifies route events and `agent.Input.ToolMode`.
5. Add default grounding instructions for normal non-coordinator runs.
6. Add one E2E answer-quality check that blocks fabricated route/file claims.

## Suggested Regression Prompts

Use these as route and E2E fixtures:

```text
Find the server path that updates session models, verify whether cloud-listed models can actually be selected and summarize the current behavior.
```

```text
Where is the HTTP endpoint that changes a session model, and what does it do today?
```

```text
Trace model switching from the browser/server API to runtime resolution.
```

```text
Verify whether a cloud-listed model can be selected without being pulled locally.
```

## Confidence

High confidence:

- The model answer is incorrect against the current repository.
- The actual HTTP path is `POST /v1/sessions/{id}/model`.
- The actual handler is `internal/server/handler.go`.
- Current runtime-backed behavior supports cloud model selection with enabled cloud config and valid credentials.
- `ToolModeNone` removes tool definitions from the LLM request.

Medium-high confidence:

- The failing prompt can be routed as `general_prompt` because the classifier lacks the relevant server/session/model-route vocabulary.
- The bad answer was produced without repository evidence.

Lower confidence:

- Whether the exact user run used server mode, TUI mode, or `--print`. All three paths use the same broad routing concepts, but print mode currently does not execute semantic retrieval itself.

## Bottom Line

The application did not fail because the code path is impossible to find. It failed because the prompt was not reliably recognized as a repository investigation task, so the model likely answered without tools, semantic context, or grounding instructions.

The immediate fix is to expand retrieval-route detection for server/session/model behavior prompts and add a regression test for this exact prompt. The durable fix is to prevent concrete repository claims unless the model inspected repository evidence or clearly discloses that it did not.
