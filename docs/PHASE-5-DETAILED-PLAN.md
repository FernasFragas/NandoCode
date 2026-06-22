# Phase 5 Detailed Plan - Permission System

Date: 2026-05-03
Status: Final plan and implementation checklist
Source plans:

- `.codex/go-ollama-plan-AGENTS.md`
- `.codex/go-ollama-plan-HUMANS.md`
- `docs/PHASE-3-DETAILED-PLAN.md`
- `docs/PHASE-4-DETAILED-PLAN.md`
- `docs/PHASE-LOG.md`

## Goal

Phase 5 replaces the temporary Phase 3/4 permission behavior with a central resolver.

The resolver must support seven permission modes, source-tagged rules, deterministic rule precedence, hook and prompt extension points, and per-call classification through the existing tool interface. After Phase 5, the agent loop must no longer decide permissions itself. Every model-requested tool call goes through `permissions.Resolve(...)` before execution.

The goal is not to build the TUI permission modal, config file loading, hook execution, sub-agents, or the future auto-mode LLM classifier. Phase 5 should leave behind a small, well-tested permission package that those later phases can plug into.

Deliverables:

- `internal/permissions` package with modes, decisions, rule sources, pattern matching, rule merging, and resolver.
- Compatibility adapter from existing `tools.PermissionMode` values to the new seven-mode system.
- Stable permission targets for Bash, FileRead, and FileWrite inputs.
- Hard-deny behavior for Bash commands classified as destructive, including `rm -rf /`.
- Agent integration so `internal/agent` calls only `permissions.Resolve(...)` for permission decisions.
- Unit tests for mode behavior, rule precedence, provenance preservation, pattern matching, resolver stages, and agent integration.
- Phase log update after implementation.

## Baseline Analysis

### Phase 0 - Security and Supply Chain

Implemented:

- `SECURITY.md` establishes local-first but not sandboxed behavior.
- Dependency and network policy scripts are active.
- CI, Dependabot, dependency review, and security scan workflows exist.

Phase 5 implications:

- Permission resolution is a security boundary, not a UX helper.
- Fail closed on malformed rules, malformed inputs, unknown modes, nil tools, and unavailable prompts.
- Do not introduce new direct dependencies. Pattern matching can use the standard library.
- Do not log full tool inputs, file contents, command outputs, prompts, or rule files at INFO.
- Rules with `SourcePolicy` must be able to deny operations even when the user selects a permissive mode.

### Phase 1 - Repo Scaffold, CLI, Paths, Logging

Implemented:

- CLI entrypoint is signal-aware.
- `doctor`, `version`, path helpers, logger helpers, Makefile targets, and tests exist.

Phase 5 implications:

- The permissions package should be library-first and should not call `os.Exit`.
- No CLI command should be added yet unless needed for tests. `/permissions` commands belong to Phase 13.
- Tests should use temporary directories and fake tools rather than shelling out unnecessarily.

### Phase 2 - LLM Client

Implemented:

- `llm.Client`, streaming chat, watchdog, retry helpers, model capabilities, and Ollama client exist.

Phase 5 implications:

- The auto-mode LLM classifier is explicitly deferred. Do not call the LLM from permissions in Phase 5.
- The resolver should expose a classifier extension point, but nil classifier must be deterministic and fail closed.
- Permission tests must not require Ollama.

### Phase 3 - Tool Interface and Starter Tools

Implemented:

- `tools.Tool`, `tools.PermissionResult`, `tools.Context`, registry, path safety, Bash, FileRead, FileWrite, and built-ins.
- Starter tools already classify read-only, destructive, and mutating calls.

Phase 5 implications:

- Reuse `Tool.CheckPermissions` as the per-call tool classifier.
- Do not import `internal/permissions` from `internal/tools`; that would invert the dependency direction.
- `tools.Context.PermissionMode` currently encodes four temporary modes. Phase 5 should preserve this for execution compatibility but stop treating it as authoritative.
- Adjust the Bash tool classifier so commands classified as destructive return `tools.PermDeny` in the neutral/default classifier context. This is what makes `Bash("rm -rf /")` denied even in `ModeBypass`.
- Do not infer categorical denial from the generic `Tool.IsDestructive` method in the resolver. The Phase 3 `FileWrite` implementation currently reports destructive for all writes, but Phase 5 should treat safe contained file writes as mutating calls that can be asked or explicitly allowed.
- Add a small optional target interface to tool inputs without coupling them to the permissions package:

```go
type PermissionTargeter interface {
    PermissionTarget() string
}
```

The interface can live in `internal/tools` or be defined locally in `internal/permissions` as a structural interface. The starter inputs should return:

- `bash.Input.PermissionTarget() string` -> command text.
- `fileread.Input.PermissionTarget() string` -> requested path.
- `filewrite.Input.PermissionTarget() string` -> requested path.

### Phase 4 - Agent Loop

Implemented:

- `internal/agent` streams model output, executes tools serially, and appends tool-result messages.
- `executeToolCalls` currently calls `Tool.CheckPermissions` directly.
- `PermAllow` executes; `PermDeny` and `PermAsk` become denied tool-result messages.

Phase 5 implications:

- Replace the direct `Tool.CheckPermissions` call with `permissions.Resolve`.
- Agent should keep Phase 4 behavior for unresolved asks: no interactive prompt yet, so `DecisionAsk` becomes a model-readable permission-denied/permission-required tool result.
- Add permission mode and rules to `agent.Input` rather than overloading `tools.Context`.
- Preserve compatibility by mapping `ToolContext.PermissionMode` to a Phase 5 mode when `agent.Input.PermissionMode` is unset.
- Do not add TUI prompting or hooks yet.

## Evaluation of the Original Phase 5 Plan

The original Phase 5 plan is directionally right:

- Seven modes are the correct product surface.
- Rules need source provenance.
- Rule precedence must be `deny > ask > allow`.
- Resolution must happen per call, not per tool.
- `ModeBubble` must exist for future sub-agents.
- The agent loop must delegate permission decisions to the permission layer.

The plan needs more implementation detail:

- It does not define the public resolver API or result shape.
- It does not explain how to avoid an import cycle with `internal/tools`.
- It does not explain how the new seven modes interact with existing Phase 3/4 `tools.PermissionMode`.
- It does not define rule target extraction for typed Go inputs.
- It references hooks and interactive prompts before those packages exist.
- It lists `Tool(arg-glob)` patterns but does not specify malformed pattern behavior, `**` behavior, or whether matching is case-sensitive.
- It says the auto-mode classifier is deferred but does not define the deterministic Phase 5 fallback.
- It does not define agent tests that prove `internal/agent` no longer makes permission decisions itself.

## Final Phase 5 Scope

In scope:

- `internal/permissions` package.
- Seven permission modes.
- Decision and resolver result types.
- Source-tagged rule sets.
- Deterministic rule merging with provenance preservation.
- Pattern parsing and matching for `Tool(arg-glob)`.
- `**` recursive segment support implemented on top of `path.Match`.
- Resolver order with no-op hook and no-op prompt/classifier extension points.
- Compatibility mapping from `tools.PermissionMode`.
- Stable permission target methods for starter tool inputs.
- Agent integration.
- Unit tests and agent integration tests.
- Phase log update after implementation.

Out of scope:

- Config-file rule loading.
- Persistent user/project/local/session rule stores.
- CLI or slash-command rule editing.
- TUI interactive permission modal.
- Hook runner implementation.
- LLM-based auto classifier.
- Sub-agent spawning.
- Changing tool execution concurrency.
- New direct dependencies.

## Target Package Layout

```text
internal/permissions/
  mode.go
  decision.go
  rules.go
  match.go
  resolver.go
  adapter.go
  mode_test.go
  rules_test.go
  match_test.go
  resolver_test.go

internal/agent/
  tools.go              # updated to call permissions.Resolve
  input.go              # updated with permission fields
  permissions_test.go   # agent integration tests for resolver path
```

Starter tool updates:

```text
internal/tools/bash/bash.go         # add PermissionTarget on Input
internal/tools/fileread/fileread.go # add PermissionTarget on Input
internal/tools/filewrite/filewrite.go # add PermissionTarget on Input
```

## Permission Modes

Define in `internal/permissions/mode.go`:

```go
type Mode string

const (
    ModeBypass      Mode = "bypass"
    ModeDontAsk     Mode = "dontAsk"
    ModeAuto        Mode = "auto"
    ModeAcceptEdits Mode = "acceptEdits"
    ModeDefault     Mode = "default"
    ModePlan        Mode = "plan"
    ModeBubble      Mode = "bubble"
)
```

Mode semantics after hook, rule, and tool-classifier stages:

| Mode | Tool classifier `Allow` | Tool classifier `Ask` | Tool classifier `Deny` |
| --- | --- | --- | --- |
| `ModeBypass` | allow | allow | deny |
| `ModeDontAsk` | allow | deny | deny |
| `ModeAuto` | allow | ask in Phase 5 | deny |
| `ModeAcceptEdits` | allow | allow only for contained file-edit/write tools; otherwise ask | deny |
| `ModeDefault` | allow | ask | deny |
| `ModePlan` | allow | deny | deny |
| `ModeBubble` | ask | ask | deny |

Important rules:

- Hooks and rules are evaluated before mode defaults.
- `ModeBypass` does not override `AlwaysDeny` or intrinsic tool denial.
- `ModeBypass` does not override destructive Bash classifier denial.
- `ModeBubble` must never return `DecisionAllow` from the mode stage. It returns `DecisionAsk` to let future parent-agent or TUI layers decide.
- `ModeAuto` must not silently allow mutating calls until the auto classifier exists. Its Phase 5 fallback is `Ask` for classifier asks.
- Unknown or empty modes normalize to `ModeDefault` unless the caller explicitly requests strict parsing in tests.

Compatibility with `tools.PermissionMode`:

```go
func FromToolsMode(mode tools.PermissionMode) Mode
```

Mapping:

- `tools.PermissionBypassPermissions` -> `ModeBypass`
- `tools.PermissionDontAsk` -> `ModeDontAsk`
- `tools.PermissionPlan` -> `ModePlan`
- `tools.PermissionDefault` or empty -> `ModeDefault`
- unknown -> `ModeDefault`

## Decisions and Resolver Result

Define in `internal/permissions/decision.go`:

```go
type Decision string

const (
    DecisionAllow Decision = "allow"
    DecisionDeny  Decision = "deny"
    DecisionAsk   Decision = "ask"
)

type Stage string

const (
    StageHook       Stage = "hook"
    StageRule       Stage = "rule"
    StageTool       Stage = "tool"
    StageMode       Stage = "mode"
    StagePrompt     Stage = "prompt"
    StageClassifier Stage = "classifier"
)

type Result struct {
    Decision     Decision
    Stage        Stage
    Reason       string
    UpdatedInput any
    Rule         *Rule
}
```

Rules:

- `Result.UpdatedInput` defaults to the parsed input.
- Reasons should be concise and model-readable.
- Do not expose stack traces or full command output in permission reasons.
- Preserve the matching rule pointer or copy in the result for debugging and future `/permissions show`.

## Rules and Sources

Define in `internal/permissions/rules.go`:

```go
type Source int

const (
    SourcePolicy Source = iota
    SourceUser
    SourceProject
    SourceLocal
    SourceCLI
    SourceSession
)

type Rule struct {
    Pattern string
    Source  Source
}

type Rules struct {
    AlwaysAllow []Rule
    AlwaysDeny  []Rule
    AlwaysAsk   []Rule
}
```

Rule behavior:

- Source is provenance, not precedence.
- Effective decision precedence is always `AlwaysDeny > AlwaysAsk > AlwaysAllow`.
- Within a decision bucket, preserve source ordering after merge for later display.
- Malformed rules must not match. The resolver should continue and include a debug-level log path later, but tests should assert malformed patterns fail closed.
- `Merge(a, b Rules) Rules` concatenates buckets and performs stable source ordering. It never deduplicates by pattern.
- A local allow rule does not override a policy deny rule because deny bucket precedence wins.

## Pattern Matching

Pattern syntax:

```text
ToolName(arg-glob)
```

Examples:

- `Bash(ls*)`
- `Bash(npm test*)`
- `FileRead(docs/**)`
- `FileWrite(/tmp/nandocodego-*)`

Matching rules:

- Tool name is case-sensitive and literal.
- Argument glob is matched against the permission target string.
- Empty tool name, missing parentheses, empty glob, or trailing garbage means the pattern is invalid and does not match.
- `*`, `?`, and character classes use `path.Match` semantics.
- A `**` path segment means zero or more path segments. Implement this as a small recursive matcher that uses `path.Match` for non-`**` segments.
- For non-path command targets, `**` has no special practical value but should still behave consistently.
- Normalize Windows path separators in targets and patterns to `/` before matching.
- Do not shell-parse rule patterns.

Permission target extraction:

1. If parsed input implements `interface{ PermissionTarget() string }`, use that.
2. Else if the parsed input is a struct with a string field named `Path`, `Command`, or `Query`, use the first present value in that order.
3. Else fallback to compact JSON for matching only. If JSON fails, target is empty and no arg-glob should match except an explicit empty-glob pattern, which is invalid.

## Resolver API

Define in `internal/permissions/resolver.go`:

```go
type HookDecisionFunc func(context.Context, Request) (Result, bool)
type PromptFunc func(context.Context, Prompt) (Decision, string, error)
type ClassifierFunc func(context.Context, Request, tools.PermissionResult) (Result, bool)

type Prompt struct {
    ToolName string
    Target   string
    Reason   string
}

type Request struct {
    Mode       Mode
    Rules      Rules
    Tool       tools.Tool
    ToolName   string
    Input      any
    ToolContext tools.Context

    HookDecision HookDecisionFunc
    Prompt       PromptFunc
    Classifier   ClassifierFunc
}

func Resolve(ctx context.Context, req Request) Result
```

Resolver order:

1. Validate request. Nil tool or empty tool name returns deny.
2. Hook decision. In Phase 5 this is nil/no-op, but the hook slot must exist.
3. Rule match:
   - first matching deny returns deny;
   - else first matching ask returns ask;
   - else first matching allow returns allow.
4. Tool classifier:
   - call `Tool.CheckPermissions` with a neutral copy of `ToolContext` whose `PermissionMode` is `tools.PermissionDefault`;
   - map `tools.PermDeny` to `DecisionDeny`;
   - map `tools.PermAllow` and `tools.PermAsk` through mode semantics.
5. Mode default.
6. Prompt:
   - if the mode result is `Ask` and `Prompt` is non-nil, call it;
   - if prompt returns allow/deny/ask, return that stage result;
   - if prompt is nil, return ask with reason `interactive permission prompt unavailable`.
7. Auto classifier:
   - Phase 5 does not call an LLM classifier;
   - if `Classifier` is nil, keep the current ask/deny result;
   - if supplied in tests or later phases, it may convert ask to allow/deny.

Implementation note:

- Although the original chain lists prompt before classifier, Phase 5 should keep the classifier hook after deterministic mode handling and before any future UI-specific finalization. With nil classifier and nil prompt, observable behavior remains fail-closed.
- The resolver should be deterministic and side-effect free except calling optional callbacks.

## Agent Integration

Update `agent.Input`:

```go
type Input struct {
    Model        string
    SystemPrompt string
    Messages     []llm.Message
    ToolContext  tools.Context

    PermissionMode  permissions.Mode
    PermissionRules permissions.Rules
    PermissionPrompt permissions.PromptFunc
}
```

Rules:

- If `PermissionMode` is empty, derive it from `ToolContext.PermissionMode`.
- If both are empty, default to `permissions.ModeDefault`.
- `internal/agent/tools.go` must call `permissions.Resolve` exactly once per parsed tool call.
- Unknown tool and malformed input errors stay outside the permission resolver because no valid per-call tool input exists yet.
- For `DecisionAllow`, execute the tool.
- For `DecisionDeny`, append a denied tool-result message.
- For `DecisionAsk`, append a permission-required tool-result message. No interactive prompt exists until Phase 7.
- Use `Result.UpdatedInput` when non-nil.
- Preserve the Phase 4 event shape: emit `ToolUseStart` before resolving permissions, emit `ToolUseResult` for allow/deny/ask outcomes.

Acceptance verification:

```bash
rg "CheckPermissions" internal/agent
```

After Phase 5 this should show only permission-package tests or no direct agent call sites. The production agent path must use `permissions.Resolve`.

## Test Plan

### Permissions Package Tests

Mode tests:

- `ModeDefault` allows classifier allow, asks classifier ask, denies classifier deny.
- `ModeBypass` allows classifier ask but not classifier deny.
- `ModeDontAsk` denies classifier ask.
- `ModePlan` denies classifier ask and allows classifier allow.
- `ModeAuto` asks classifier ask in Phase 5.
- `ModeAcceptEdits` allows `FileWrite` asks and asks Bash non-read-only commands.
- `ModeBubble` never returns allow.
- Unknown mode normalizes to default.

Rule tests:

- Deny beats ask and allow.
- Ask beats allow.
- Source provenance is preserved on merge.
- `SourceLocal` allow does not override `SourcePolicy` deny.
- Duplicate patterns from different sources remain visible after merge.

Pattern tests:

- Literal tool mismatch does not match.
- `Bash(ls*)` matches `Bash` target `ls -la`.
- `Bash(npm test*)` matches `npm test ./...`.
- `FileRead(docs/**)` matches nested docs files.
- Invalid patterns do not match.
- Windows separators normalize for path-like targets.

Resolver tests:

- Nil tool denies.
- Hook deny wins over rules and modes.
- Rule deny wins over bypass mode.
- Rule ask wins over rule allow.
- Rule allow can allow `FileWrite` in `ModeDontAsk`.
- Tool classifier deny wins over bypass mode.
- Destructive Bash classifier denial wins over bypass mode.
- Prompt nil returns ask for default mutating call.
- Prompt callback can convert ask to allow.
- Classifier nil does not auto-allow `ModeAuto`.
- `UpdatedInput` from tool classifier is preserved.

### Agent Tests

- `Bash("ls")` in default mode completes and reaches a second model turn.
- `Bash("rm -rf /")` is denied and is not executed.
- `FileWrite` in default mode becomes a permission-required tool result.
- `FileWrite` with an explicit session allow rule executes.
- `FileWrite` with both allow and deny rules is denied.
- `ModeBubble` does not execute even read-only `FileRead`.
- Existing Phase 4 permission tests are updated to assert resolver-backed behavior.

## Concrete Todos

### A. Pre-Flight Analysis

- [ ] Run `go test ./internal/agent/... ./internal/tools/...`.
- [ ] Run `rg "PermissionMode|CheckPermissions|PermAllow|PermAsk|PermDeny" internal`.
- [ ] Confirm no existing `internal/permissions` files need preserving.
- [ ] Review Bash/FileRead/FileWrite input structs before adding target methods.

### B. Create Permissions Types

- [ ] Add `internal/permissions/mode.go`.
- [ ] Define the seven `Mode` constants.
- [ ] Add mode normalization.
- [ ] Add `FromToolsMode`.
- [ ] Add `internal/permissions/decision.go`.
- [ ] Define `Decision`, `Stage`, and `Result`.
- [ ] Add helpers to map `tools.Permission` to `Decision`.

### C. Implement Rules

- [ ] Add `internal/permissions/rules.go`.
- [ ] Define `Source`, `Rule`, and `Rules`.
- [ ] Add stable source string methods for readable test failures.
- [ ] Implement `Merge`.
- [ ] Preserve duplicates and source provenance.
- [ ] Add rule precedence helper: deny, then ask, then allow.

### D. Implement Pattern Matching

- [ ] Add `internal/permissions/match.go`.
- [ ] Parse `Tool(arg-glob)` patterns.
- [ ] Reject malformed patterns.
- [ ] Normalize path separators.
- [ ] Implement `**` segment matching using `path.Match` for normal segments.
- [ ] Implement permission target extraction.
- [ ] Add target methods to Bash, FileRead, and FileWrite inputs.
- [ ] Update Bash permission classification so destructive commands return `tools.PermDeny` in neutral/default mode.
- [ ] Add table tests for valid, invalid, path, and command patterns.

### E. Implement Resolver

- [ ] Add `internal/permissions/resolver.go`.
- [ ] Define `Request`, `Prompt`, and callback types.
- [ ] Validate nil and empty request fields fail closed.
- [ ] Implement hook stage.
- [ ] Implement rule stage.
- [ ] Build neutral tool context for `Tool.CheckPermissions`.
- [ ] Implement mode semantics.
- [ ] Implement nil prompt behavior.
- [ ] Implement optional prompt callback behavior.
- [ ] Add classifier extension point with nil fail-closed fallback.
- [ ] Preserve `UpdatedInput`.
- [ ] Add resolver matrix tests.

### F. Integrate Agent

- [ ] Update `agent.Input` with `PermissionMode`, `PermissionRules`, and `PermissionPrompt`.
- [ ] Apply permission defaults in `validateInput`.
- [ ] Update `executeToolCalls` to call `permissions.Resolve`.
- [ ] Use resolver `UpdatedInput` for execution.
- [ ] Convert `DecisionDeny` and `DecisionAsk` to concise tool-result messages.
- [ ] Keep unknown-tool and malformed-input behavior unchanged.
- [ ] Update existing agent tests.
- [ ] Add agent tests for rule allow/deny, default ask, default read-only allow, and bubble mode.
- [ ] Verify `rg "CheckPermissions" internal/agent` shows no production direct call.

### G. Documentation and Phase Log

- [ ] Update `docs/PHASE-LOG.md` with Phase 5 implementation details after coding.
- [ ] Record any deviations from this plan.
- [ ] Record verification commands and local optional tooling gaps.

### H. Final Verification

- [ ] `go mod tidy`
- [ ] `go test ./internal/permissions/...`
- [ ] `go test ./internal/agent/...`
- [ ] `go test ./internal/tools/...`
- [ ] `go test ./...`
- [ ] `go test -race ./internal/permissions/... ./internal/agent/...`
- [ ] `go vet ./...`
- [ ] `tools/check-allowed-deps.sh`
- [ ] `tools/check-network-policy.sh`
- [ ] `rg "CheckPermissions" internal/agent`

## Acceptance Criteria

- [ ] Matrix test passes with documented `deny > ask > allow` rule precedence.
- [ ] Seven modes are implemented and tested.
- [ ] `ModeBubble` never returns `DecisionAllow`.
- [ ] Provenance-preserving merge keeps duplicate patterns from different sources.
- [ ] `SourceLocal` allow does not override `SourcePolicy` deny.
- [ ] Agent production code routes permission decisions through `permissions.Resolve`.
- [ ] `Bash("ls")` is allowed in `ModeDefault`.
- [ ] `Bash("rm -rf /")` is denied and not executed, including in `ModeBypass`.
- [ ] `FileWrite` in `ModeDefault` does not execute without an explicit allow or prompt approval.
- [ ] No new direct dependency is added.

## Exit Gate

End-to-end fake-client tests prove:

- `Bash("ls")` is allowed in `ModeDefault` through deterministic read-only classification.
- `Bash("rm -rf /")` is denied before execution, including in `ModeBypass`.
- An explicit deny rule beats `ModeBypass`.
- No production call site in `internal/agent` calls `Tool.CheckPermissions` directly.
