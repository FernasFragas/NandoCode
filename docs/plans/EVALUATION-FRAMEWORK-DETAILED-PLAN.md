# Evaluation Framework Detailed Plan

Date: 2026-06-25  
Status: Implemented 2026-06-25 (see `docs/phases/PHASE-LOG.md`). Live-model runs and a larger fixture set remain for Phase 18.  
Command: `nandocodego eval run ./evals`

## Goal

Add a repeatable evaluation framework that measures whether NandoCode can
complete coding tasks correctly.

The framework must exercise the real agent loop, tool registry, permission
resolver, and workspace mutation paths. It must produce objective, reviewable
evidence instead of relying only on the model's final answer.

The initial acceptance target is:

- at least five coding-task fixtures;
- JSON and Markdown reports;
- deterministic CI execution with a recorded or mock model;
- live-model execution for local quality measurement;
- a README example showing an eval command and report.

## Relationship To The Existing Roadmap

This plan replaces the narrow eval-runner design inside
`docs/phases/PHASE-18-DETAILED-PLAN.md`, which currently proposes an eval-only Go test
package behind a build tag.

The broader Phase 18 hardening plan remains valid. The change is that evals
become a first-class product command:

```bash
nandocodego eval run ./evals
```

This is preferable to requiring contributors to understand Go build tags or
invoke a package-specific test command. Unit tests for the eval framework still
run through `go test ./...`.

Phase 25 and Phase 17 do not depend on the full live-model eval suite. The
deterministic subset should land early enough to become a regression gate for
those phases.

## Why This Matters

Agentic coding quality cannot be inferred from unit coverage of the runtime
alone. NandoCode needs a feedback loop that answers:

- Did the task finish?
- Did the resulting code pass its tests?
- Did the agent change the intended files?
- Did it make unrelated changes?
- How many tools, approvals, retries, and seconds did it need?
- Did a new runtime change improve or regress task success?
- Does the same fixture behave consistently across models?

The eval framework should make those answers stable enough for CI and detailed
enough for model and runtime comparisons.

## Scope

### In Scope

- A new `eval` Cobra command with a `run` subcommand.
- Directory-based fixtures under `evals/`.
- Isolated temporary workspaces per fixture.
- Live Ollama and recorded-model execution modes.
- Agent-event and permission instrumentation.
- Test-command execution with timeout and captured output.
- Workspace snapshots and file-change classification.
- Scoring with hard gates and weighted checks.
- JSON and Markdown reports.
- Deterministic CI fixtures.
- Five initial coding fixtures.
- README documentation and an example report.

### Out Of Scope For The First Version

- LLM-as-judge scoring.
- Cloud-hosted eval services.
- Distributed execution across multiple machines.
- Automatic benchmark publishing.
- Evaluating arbitrary untrusted fixture repositories.
- Statistical significance analysis across model populations.
- A browser dashboard.
- Cross-language fixture coverage beyond what is needed to prove the framework.
- Full Phase 18's target of 30 or more live-model scenarios.

The architecture must allow those capabilities later without changing the
fixture contract unnecessarily.

## Design Principles

1. **Score artifacts, not claims.** A final answer saying "done" is not proof.
2. **Use the real runtime.** Do not bypass the agent loop or tools in live mode.
3. **Keep CI deterministic.** CI must not depend on model availability or output
   nondeterminism.
4. **Keep fixtures self-contained.** A fixture must not depend on the NandoCode
   source tree after it is copied into its temporary workspace.
5. **Never mutate fixture sources.** Every run works from a copied workspace.
6. **Fail closed on invalid scoring configuration.**
7. **Separate infrastructure errors from task failures.**
8. **Preserve raw evidence.** Reports should link to logs, diff output, and test
   output when available.
9. **Make results comparable.** Stable fixture IDs, schema versions, and metric
   names are required.
10. **Do not silently omit checks.** Every configured check receives
    `passed`, `failed`, `skipped`, or `error`.

## User Experience

### Primary Command

```bash
nandocodego eval run ./evals
```

Default behavior:

- discover all valid fixture directories below `./evals`;
- run fixtures sequentially;
- use the configured live model;
- write JSON and Markdown reports;
- print a concise terminal summary;
- exit non-zero when the required pass threshold is not met.

### Deterministic CI Command

```bash
nandocodego eval run ./evals \
  --provider recorded \
  --tag deterministic \
  --fail-under 1.0 \
  --output-dir .tmp-evals/ci
```

### Useful Flags

```text
--provider live|recorded    Model backend; default live
--model <name>              Model used in live mode
--ollama-url <url>          Ollama endpoint override
--recording <name>          Recording variant; default "default"
--tag <tag>                 Run fixtures containing this tag; repeatable
--filter <glob>             Match fixture IDs
--jobs <n>                  Fixture parallelism; default 1
--repeat <n>                Repeat each selected fixture; default 1
--timeout <duration>        Global per-fixture timeout override
--output-dir <path>         Report and artifact directory
--keep-workspaces           Preserve temporary workspaces
--fail-under <0..1>         Required aggregate score
--fail-fast                 Stop after the first failed/error fixture
--json-only                 Suppress Markdown generation
--markdown-only             Suppress JSON generation
```

`--jobs` should initially be capped at a conservative value such as four.
Recorded CI runs should use one worker unless parallelism itself is being
tested.

### Optional Validation Command

Add:

```bash
nandocodego eval validate ./evals
```

This validates fixture structure, YAML, path rules, command definitions,
recordings, and duplicate fixture IDs without running an agent.

## Fixture Layout

Required structure:

```text
evals/
  basic-refactor/
    task.md
    repo/
    expected/
    scoring.yaml
  bugfix-with-tests/
    task.md
    repo/
    expected/
    scoring.yaml
```

Optional deterministic recording:

```text
evals/
  basic-refactor/
    task.md
    repo/
    expected/
    scoring.yaml
    recordings/
      default.json
```

### `task.md`

`task.md` is the exact user task passed to the agent. It is plain Markdown and
must not contain framework-specific frontmatter in version 1.

Example:

```markdown
Refactor the duplicated integer parsing logic into one helper.

Keep the public API unchanged and make sure all tests pass.
```

The evaluator should normalize line endings but otherwise preserve the task
text exactly.

### `repo/`

`repo/` is copied into an isolated workspace before each run.

Rules:

- fixture sources remain read-only from the runner's perspective;
- symlinks escaping `repo/` are rejected during validation;
- file permissions are preserved where practical;
- `.git/`, `.nandocodego/`, caches, and generated result directories are not
  copied unless explicitly allowed by fixture configuration;
- every repetition starts from a fresh copy;
- fixture repositories should be small and purpose-built.

### `expected/`

`expected/` contains golden final files.

Each regular file under `expected/` maps to the same relative path in the final
workspace. The final file must exist and match its golden content according to
the configured comparison mode.

Example:

```text
expected/
  parser.go
  parser_test.go
```

Files that must be deleted are declared in `scoring.yaml`; deletion markers
should not be encoded as special files.

An empty `expected/` directory is allowed when tests and path constraints are
the authoritative checks.

### `scoring.yaml`

`scoring.yaml` defines execution constraints, objective checks, and scoring
weights.

Proposed version 1 schema:

```yaml
version: 1

id: basic-refactor
description: Extract duplicated parsing logic without changing behavior.
tags:
  - deterministic
  - refactor
  - go

execution:
  timeout: 90s
  max_turns: 12
  max_tool_calls: 30
  permission_mode: default
  approval_strategy: allow
  environment:
    CGO_ENABLED: "0"

model:
  live:
    temperature: 0
  recorded:
    recording: default

tests:
  - name: go-test
    command: ["go", "test", "./..."]
    timeout: 30s
    required: true

workspace:
  allowed_changes:
    - "parser.go"
    - "parser_test.go"
  required_changes:
    - "parser.go"
  forbidden_changes:
    - "go.mod"
    - "go.sum"
  must_exist:
    - "parser.go"
    - "parser_test.go"
  must_not_exist: []
  max_changed_files: 2
  max_diff_lines: 80

golden:
  compare: exact
  files:
    - "parser.go"

checks:
  final_answer_contains:
    - "tests"
  required_tools:
    - FileRead
    - FileEdit
  forbidden_tools: []

weights:
  task_completion: 0.15
  tests: 0.40
  expected_files: 0.25
  change_scope: 0.15
  final_answer: 0.05

pass_threshold: 0.90
```

### Schema Rules

- `version` must equal a supported schema version.
- `id` must be unique across the discovered fixture set.
- `id` should match the directory name.
- Weight values must be between `0` and `1`.
- Weight values must sum to `1.0`, allowing a small floating-point tolerance.
- Every command is an argument array, never a shell string.
- Paths must be relative, slash-normalized, and unable to escape the fixture
  workspace.
- Unknown fields are errors in deterministic CI mode.
- Missing optional sections use documented defaults.
- A fixture tagged `deterministic` must include a valid recording.

## Recorded Model Contract

The deterministic provider implements the existing `llm.Client` interface and
returns recorded `llm.StreamEvent` sequences.

Proposed recording format:

```json
{
  "version": 1,
  "model": "recorded/basic-refactor",
  "strict": true,
  "turns": [
    {
      "match": {
        "latest_user_contains": "Refactor the duplicated integer parsing logic",
        "required_tools": ["FileRead", "FileEdit"]
      },
      "events": [
        {
          "message": {
            "role": "assistant",
            "tool_calls": [
              {
                "function": {
                  "name": "FileRead",
                  "arguments": {"path": "parser.go"}
                }
              }
            ]
          }
        },
        {
          "done": true,
          "done_reason": "stop"
        }
      ]
    }
  ]
}
```

The implementation should reuse the public `llm.StreamEvent`, `llm.Message`,
and `llm.ToolCall` JSON shapes where possible.

Strict recording behavior:

- turns are consumed in order;
- the requested model must match the recording model or the evaluator's
  recorded-model alias;
- stable request expectations are checked;
- tool definitions may be checked by name, never by unstable full JSON order;
- an exhausted recording is an infrastructure error;
- unused required turns are an infrastructure error;
- malformed tool arguments fail before tool execution;
- recorded mode performs no LLM network call.

Do not match full system prompts or absolute workspace paths. Those are too
fragile for deterministic CI.

## Runtime Architecture

### Proposed Packages

```text
internal/eval/
  contracts.go
  discover.go
  load.go
  validate.go
  workspace.go
  snapshot.go
  diff.go
  runner.go
  runtime.go
  instrumentation.go
  scoring.go
  checks.go
  command_check.go
  recorded_client.go
  report_json.go
  report_markdown.go
  errors.go

internal/cli/
  eval.go

evals/
  ...
```

### Core Types

```go
type Fixture struct {
    ID          string
    Root        string
    Task        string
    RepoDir     string
    ExpectedDir string
    Config      ScoringConfig
}

type RunOptions struct {
    Provider       Provider
    Model          string
    OllamaURL      string
    Recording      string
    OutputDir      string
    KeepWorkspaces bool
    Jobs           int
    Repeat         int
    Timeout        time.Duration
    FailUnder      float64
}

type FixtureResult struct {
    SchemaVersion         int
    RunID                 string
    FixtureID             string
    Status                Status
    Score                 float64
    TaskCompletionStatus  string
    TestsPassed           int
    TestsFailed           int
    FilesChanged          []FileChange
    UnexpectedFilesChanged []string
    ToolCalls             int
    ApprovalsRequired     int
    Runtime               time.Duration
    ModelUsed             string
    Retries               int
    FinalDiffSize         DiffSize
    FailureReason         string
    Checks                []CheckResult
    Artifacts             ArtifactPaths
}
```

Use explicit JSON tags and a top-level schema version.

### Status Values

```text
passed
failed
error
timeout
canceled
skipped
```

Meaning:

- `passed`: score and all hard gates passed.
- `failed`: the runtime completed, but one or more task-quality checks failed.
- `error`: fixture, runtime, recording, or evaluator infrastructure failed.
- `timeout`: the fixture deadline expired.
- `canceled`: the parent command was canceled.
- `skipped`: the fixture was filtered or explicitly unsupported.

Only `passed` contributes as a successful fixture. `error` must never be
reported as a normal task failure.

## Workspace Isolation And Change Detection

### Workspace Lifecycle

For each fixture repetition:

1. Validate the source fixture.
2. Create a temporary working directory.
3. Copy `repo/` into `<temp>/workspace`.
4. Capture the baseline manifest.
5. Build an isolated `tools.Context` rooted at the workspace.
6. Run the agent.
7. Capture the final manifest.
8. Execute configured test checks.
9. Compare golden files and change scope.
10. Write artifacts.
11. Delete the workspace unless `--keep-workspaces` is set.

### Manifest

The baseline and final manifests should record:

- relative path;
- file type;
- size;
- SHA-256 digest;
- executable bit;
- text/binary classification.

Ignore evaluator-generated files and configured fixture exclusions.

### File Change Types

```text
added
modified
deleted
mode_changed
```

`files_changed` contains stable relative paths sorted lexicographically.

`unexpected_files_changed` contains changed paths that match no
`allowed_changes` pattern. A forbidden path is always unexpected and should
produce a dedicated failed check.

### Final Diff Size

The result must include:

```json
{
  "files": 2,
  "lines_added": 18,
  "lines_deleted": 12,
  "bytes_added": 640,
  "bytes_deleted": 410,
  "binary_files": 0
}
```

Use standard-library workspace snapshots as the source of truth. A textual
line-diff helper may calculate line counts for bounded UTF-8 files. For binary
or oversized files, report byte counts and increment `binary_files`.

Do not require fixture directories to contain Git metadata.

Write a unified patch artifact when a safe bounded text diff can be generated.

## Agent Execution

### Live Mode

Live mode should:

- load the model and Ollama URL through the same config precedence used by
  print mode;
- use the existing model resolver/runtime for local and direct Ollama Cloud
  behavior;
- construct the built-in tool registry;
- root all tool operations in the temporary fixture workspace;
- use the real `agent.Agent`;
- disable unrelated project memory, project config, and hooks unless the
  fixture explicitly enables them;
- avoid semantic index state from the NandoCode source repository;
- set deterministic model options where supported, including temperature zero;
- capture all agent events until one terminal event is received.

The first implementation should extract or reuse shared non-interactive runtime
assembly rather than copy all of `internal/cli/print.go`.

### Recorded Mode

Recorded mode uses the same:

- agent loop;
- tool registry;
- permission resolver;
- workspace;
- scoring;
- report generation.

Only the `llm.Client` implementation changes. This proves that deterministic CI
still exercises tool parsing, tool execution, retries, permissions, terminal
states, and file scoring.

### Permission Strategy

Fixtures configure one of:

```text
allow
deny
scripted
unavailable
```

- `allow`: every permission prompt is approved.
- `deny`: every permission prompt is denied.
- `scripted`: decisions are consumed from fixture configuration.
- `unavailable`: no prompt callback is installed.

The prompt callback increments `approvals_required` every time it is invoked.
Record allow/deny counts separately in the detailed result.

This metric is distinct from all permission resolutions. Read-only tool calls
that never prompt do not count as approvals required.

## Instrumentation

Collect metrics from existing events:

| Metric | Source |
| --- | --- |
| Task completion status | `agent.Terminal.Reason` plus scoring result |
| Tool calls | Count `agent.ToolUseStart` |
| Tool names | `agent.ToolUseStart.Name` |
| Tool errors | `agent.ToolUseResult.Err` |
| Retries | Count `agent.RetryNotice` |
| Turns | `agent.Terminal.Usage.Turns` |
| Model token counts | `agent.Terminal.Usage` |
| Runtime | Wall-clock duration around the complete fixture run |
| Model used | Resolved runtime model or recording model |
| Approvals required | Wrapped `permissions.PromptFunc` |
| Permission outcomes | Wrapped `permissions.ObserverFunc` |
| Final answer | Accumulated `agent.AssistantTextDelta` |
| Failure reason | Terminal detail, timeout, failed hard gate, or infrastructure error |

The evaluator should also retain:

- ordered tool-call summaries;
- retry reasons;
- terminal reason;
- test stdout/stderr;
- check results;
- final assistant text;
- optional bounded event JSONL.

Thinking content must not be emitted to reports by default.

## Test Command Execution

Test commands are trusted fixture configuration, but execution still needs
strict boundaries:

- command and arguments are provided as a YAML list;
- do not invoke a shell;
- working directory is the fixture workspace;
- environment starts from a minimal allowlist plus fixture overrides;
- apply a per-command timeout;
- capture bounded stdout and stderr;
- record exit code and duration;
- kill the process group on timeout where supported;
- redact known credential environment variables;
- never execute test commands during `eval validate`.

Each test produces:

```go
type TestResult struct {
    Name       string
    Command    []string
    Status     string
    ExitCode   int
    Duration   time.Duration
    StdoutPath string
    StderrPath string
    Required   bool
    Failure    string
}
```

## Scoring Model

### Hard Gates

The following fail a fixture regardless of weighted score:

- evaluator or recording infrastructure error;
- fixture timeout;
- required test command fails;
- forbidden file changes;
- unexpected file changes when `allow_unexpected_changes` is false;
- missing required path;
- present `must_not_exist` path;
- terminal reason is unrecoverable, context overflow, or max turns;
- configured maximum tool calls, changed files, or diff size is exceeded.

### Weighted Checks

After hard gates, calculate:

```text
score = sum(category_score × category_weight)
```

Initial categories:

- `task_completion`
- `tests`
- `expected_files`
- `change_scope`
- `final_answer`

Category behavior:

- `task_completion`: 1 only when the agent reaches `TerminalCompleted`.
- `tests`: passed required and optional tests divided by configured tests,
  with required-test failure also acting as a hard gate.
- `expected_files`: matched golden checks divided by configured golden checks.
- `change_scope`: required changes, allowed changes, and size limits.
- `final_answer`: configured stable text checks only; keep this weight low.

The fixture passes when:

- no hard gate failed; and
- `score >= pass_threshold`.

### Task Completion Status

Report both runtime completion and eval verdict:

```json
{
  "task_completion_status": "completed",
  "status": "failed",
  "failure_reason": "required test go-test failed"
}
```

Valid task completion values:

```text
completed
aborted
max_turns
context_overflow
stop_hook
unrecoverable
timeout
not_started
```

## Result Schema

Minimum JSON result fields:

```json
{
  "schema_version": 1,
  "run_id": "20260625T210000Z-a1b2c3d4",
  "fixture_id": "basic-refactor",
  "status": "passed",
  "score": 1.0,
  "task_completion_status": "completed",
  "tests_passed": 1,
  "tests_failed": 0,
  "files_changed": [
    {"path": "parser.go", "change": "modified"}
  ],
  "unexpected_files_changed": [],
  "tool_calls": 4,
  "approvals_required": 1,
  "runtime_ms": 1834,
  "model_used": "recorded/basic-refactor",
  "provider": "recorded",
  "retries": 0,
  "final_diff_size": {
    "files": 1,
    "lines_added": 8,
    "lines_deleted": 12,
    "bytes_added": 241,
    "bytes_deleted": 330,
    "binary_files": 0
  },
  "failure_reason": ""
}
```

Additional recommended fields:

- fixture description and tags;
- repetition index;
- started/finished timestamps;
- NandoCode version and commit;
- host OS and architecture;
- terminal reason and usage;
- approval allow/deny counts;
- tool-call counts by name;
- checks;
- test results;
- artifact paths;
- recording name and digest;
- fixture source digest.

## Report Layout

Default output:

```text
.tmp-evals/
  20260625T210000Z-a1b2c3d4/
    results.json
    report.md
    fixtures/
      basic-refactor/
        result.json
        final-answer.md
        diff.patch
        events.jsonl
        tests/
          go-test.stdout.txt
          go-test.stderr.txt
```

### JSON Report

The top-level JSON report contains:

- schema version;
- run metadata;
- selection options;
- aggregate counts;
- aggregate score;
- fixture results;
- model/provider metadata;
- report artifact paths.

Write JSON atomically.

### Markdown Report

Recommended sections:

1. Run summary.
2. Aggregate score and pass threshold.
3. Environment and model.
4. Fixture results table.
5. Failure summary.
6. Per-fixture details.
7. Test results.
8. Changed and unexpected files.
9. Tool, approval, retry, and timing metrics.
10. Links to diff and raw artifacts.

Example summary:

```markdown
# NandoCode Evaluation Report

| Metric | Value |
| --- | ---: |
| Fixtures | 5 |
| Passed | 5 |
| Failed | 0 |
| Errors | 0 |
| Aggregate score | 1.00 |
| Runtime | 8.4s |

| Fixture | Status | Score | Tests | Files | Tools | Approvals | Runtime |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| basic-refactor | passed | 1.00 | 1/1 | 1 | 4 | 1 | 1.8s |
```

## Initial Five Fixtures

All five initial fixtures should be small Go modules so framework behavior is
tested without introducing language-specific setup variability.

### 1. `basic-refactor`

Task:

- extract duplicated parsing logic into one private helper;
- preserve public behavior;
- run existing tests.

Scoring:

- `go test ./...` passes;
- only parser implementation and test files may change;
- golden helper implementation matches;
- no dependency files change.

### 2. `bugfix-with-tests`

Task:

- fix an off-by-one range bug;
- add a regression test.

Scoring:

- the previously failing test passes;
- a new test file or expected test case exists;
- production and test files are required changes;
- no unrelated file changes.

### 3. `add-input-validation`

Task:

- reject an empty identifier with a typed error;
- preserve valid behavior.

Scoring:

- valid and invalid-path tests pass;
- expected error declaration exists;
- API file and tests are within allowed scope.

### 4. `rename-symbol-safely`

Task:

- rename an internal symbol across two files;
- keep exported API unchanged.

Scoring:

- tests pass;
- old internal symbol no longer appears;
- expected files changed;
- exported signatures remain golden.

### 5. `add-missing-unit-tests`

Task:

- add table-driven tests for an existing pure function;
- do not modify production behavior.

Scoring:

- test command passes;
- production files must remain unchanged;
- test file matches required assertions;
- final answer mentions tests run.

Every fixture includes a `recordings/default.json` sequence for deterministic
CI and supports live mode without changes to its task or scoring contract.

## CLI Implementation

Add `internal/cli/eval.go`:

```go
func newEvalCmd() *cobra.Command
func newEvalRunCmd() *cobra.Command
func newEvalValidateCmd() *cobra.Command
```

Register the command in `newRootCommand`.

CLI responsibilities:

- parse flags;
- resolve paths;
- load normal model configuration;
- create the eval service;
- stream concise progress to stderr;
- write final artifact paths and summary to stdout;
- map eval outcomes to stable exit codes.

Suggested exit codes:

```text
0   selected fixtures passed threshold
1   one or more fixtures failed quality checks
2   invalid command arguments or fixture configuration
3   evaluator/model/runtime infrastructure error
130 canceled
```

Do not put fixture execution logic in the Cobra command.

## Implementation Slices

### Slice E0: Contracts And Validation

Files:

- `internal/eval/contracts.go`
- `internal/eval/load.go`
- `internal/eval/validate.go`
- tests and YAML fixtures

Tasks:

- define schema-versioned config types;
- parse `task.md` and `scoring.yaml`;
- validate paths, weights, commands, IDs, and tags;
- reject duplicate IDs;
- reject escaping symlinks;
- implement `eval validate`.

Exit gate:

- valid fixture loads;
- malformed and unsafe fixtures fail with path-specific diagnostics;
- no workspace is executed during validation.

### Slice E1: Workspace Snapshot And Diff

Files:

- `internal/eval/workspace.go`
- `internal/eval/snapshot.go`
- `internal/eval/diff.go`

Tasks:

- copy fixture repositories;
- compute SHA-256 manifests;
- classify added/modified/deleted/mode-changed files;
- calculate diff metrics;
- render bounded patches;
- clean or retain workspaces.

Exit gate:

- fixture source remains unchanged;
- repeated workspace creation is deterministic;
- change lists and sizes are stable across runs.

### Slice E2: Recorded Client

Files:

- `internal/eval/recorded_client.go`
- recording fixtures and tests

Tasks:

- implement `llm.Client`;
- parse ordered recorded turns;
- validate stable request expectations;
- stream events through normal agent paths;
- fail on exhausted, malformed, or unused required turns.

Exit gate:

- a recorded tool-use scenario modifies a temporary repo through the real tool;
- no network request occurs;
- replay is deterministic.

### Slice E3: Runtime And Instrumentation

Files:

- `internal/eval/runtime.go`
- `internal/eval/instrumentation.go`
- `internal/eval/runner.go`

Tasks:

- assemble live and recorded runtimes;
- root tools in the temporary workspace;
- isolate memory/config/cache state;
- collect agent events;
- count tool calls, approvals, retries, and usage;
- enforce timeout and cancellation.

Exit gate:

- terminal and timeout paths produce complete metrics;
- exactly one fixture result is produced per repetition;
- no goroutine or temporary-workspace leak.

### Slice E4: Checks And Scoring

Files:

- `internal/eval/checks.go`
- `internal/eval/command_check.go`
- `internal/eval/scoring.go`

Tasks:

- execute test commands without a shell;
- compare golden files;
- enforce allowed, required, and forbidden changes;
- enforce size/tool/turn limits;
- calculate weighted scores;
- classify failure reasons.

Exit gate:

- hard gates cannot be overridden by a high weighted score;
- every configured check appears in output;
- task failures and infrastructure errors remain distinct.

### Slice E5: Reports

Files:

- `internal/eval/report_json.go`
- `internal/eval/report_markdown.go`

Tasks:

- define schema-versioned JSON;
- render Markdown;
- write atomically;
- retain bounded evidence artifacts;
- redact credentials and sensitive environment values.

Exit gate:

- JSON round-trips through its Go type;
- Markdown contains every minimum scoring field;
- deterministic runs produce stable normalized report content apart from
  timestamps and run IDs.

### Slice E6: CLI

Files:

- `internal/cli/eval.go`
- `internal/cli/eval_test.go`
- `internal/cli/root_test.go`

Tasks:

- register `eval run` and `eval validate`;
- implement flags and exit codes;
- inject the eval runner for CLI tests;
- print concise progress and report locations.

Exit gate:

- the recommended command works from repository root;
- invalid fixtures exit 2;
- failed fixtures exit 1;
- infrastructure failures exit 3.

### Slice E7: Initial Fixtures

Files:

- `evals/basic-refactor/...`
- `evals/bugfix-with-tests/...`
- `evals/add-input-validation/...`
- `evals/rename-symbol-safely/...`
- `evals/add-missing-unit-tests/...`

Tasks:

- build five minimal source repositories;
- define expected files and scoring;
- add deterministic recordings;
- verify live compatibility;
- assign stable fixture tags.

Exit gate:

- all five pass in recorded mode;
- each fixture can fail for the intended reason when its expected solution is
  intentionally broken.

### Slice E8: CI And Documentation

Files:

- `.github/workflows/ci.yml`
- `Makefile`
- `README.md`
- optional checked-in `docs/examples/EVAL-REPORT.md`

Tasks:

- add deterministic eval CI job;
- upload JSON, Markdown, and failure artifacts;
- add `make eval` and `make eval-ci`;
- document live and deterministic commands;
- include one example report;
- update Phase 18 and phase-log routing.

Exit gate:

- CI runs the deterministic tag without Ollama;
- artifacts are available on pass and failure;
- README example matches actual command output.

## Testing Strategy

### Unit Tests

- YAML validation and defaults.
- Duplicate fixture IDs.
- Path traversal and symlink rejection.
- Weight validation.
- Command-array validation.
- Workspace copy and cleanup.
- Snapshot change classification.
- Text and binary diff sizing.
- Recorded request matching.
- Recording exhaustion and unused turns.
- Event metric collection.
- Approval counting.
- Timeout and cancellation.
- Golden comparison modes.
- Glob-based change rules.
- Hard gates.
- Weighted score calculation.
- JSON encoding.
- Markdown rendering.
- Exit-code mapping.

### Integration Tests

- Recorded model calls FileRead and FileEdit against a temporary fixture.
- Required test command passes after agent changes.
- Unexpected file change fails the fixture.
- A denied permission leaves the workspace unchanged.
- A retry is reflected in result metrics.
- A fixture timeout kills agent and test processes.
- Two repetitions start from identical repository state.

### Race Tests

Run:

```bash
go test -race ./internal/eval/... ./internal/cli/...
```

Include parallel fixture execution, event collection, result aggregation, and
cancellation.

### Live Smoke Test

With Ollama running:

```bash
nandocodego eval run ./evals \
  --provider live \
  --model qwen3.6:35b \
  --filter basic-refactor \
  --keep-workspaces
```

Record:

- model;
- fixture;
- score;
- tests;
- changed files;
- tool calls;
- approvals;
- retries;
- runtime;
- diff size;
- failure reason.

## CI Plan

Add a deterministic job after build/test:

```yaml
eval-deterministic:
  needs: [go, lint]
  runs-on: ubuntu-latest
  steps:
    - checkout
    - setup Go from go.mod
    - build nandocodego
    - run:
        ./bin/nandocodego eval run ./evals \
          --provider recorded \
          --tag deterministic \
          --fail-under 1.0 \
          --output-dir .tmp-evals/ci
    - upload .tmp-evals/ci
```

Use the repository-approved pinned action versions when implementing the
workflow.

CI requirements:

- no Ollama service;
- no cloud credential;
- no external network dependency after Go dependencies are restored;
- artifacts uploaded even when evals fail;
- job fails for invalid fixtures, infrastructure errors, or score below 1.0;
- deterministic fixtures complete within a small fixed budget, initially five
  minutes for the whole job.

Live-model evals should run manually or on a dedicated self-hosted workflow.
They must not block normal pull requests until model provisioning and
repeatability are operationally stable.

## Makefile Targets

```make
eval:
	./bin/nandocodego eval run ./evals

eval-ci:
	./bin/nandocodego eval run ./evals \
		--provider recorded \
		--tag deterministic \
		--fail-under 1.0 \
		--output-dir .tmp-evals/ci

eval-validate:
	./bin/nandocodego eval validate ./evals
```

Targets should depend on `build` or clearly document that the binary must
already exist.

## Security And Privacy

- Treat fixture commands and repositories as trusted code.
- State clearly that running third-party eval fixtures may execute code.
- Reject fixture path traversal and escaping symlinks.
- Keep workspaces under the OS temp directory by default.
- Never include API keys, auth headers, or full environment dumps in reports.
- Do not store model thinking by default.
- Bound stdout, stderr, event logs, final answers, and patches.
- Recorded CI mode must not access the network through the model provider.
- Network-capable tools should be disabled unless a fixture explicitly enables
  them.
- Live cloud-model runs must show the normal cloud privacy implications.
- Test commands must use argument arrays rather than shell parsing.

## Performance And Reliability Constraints

- Fixture discovery and validation should complete in under one second for the
  initial suite.
- Recorded five-fixture CI execution should target under one minute.
- Reports must be written even when individual fixtures fail.
- Parent cancellation must stop active agent runs and test commands.
- One fixture panic must be recovered as an evaluator error without losing
  already completed fixture results.
- Parallel execution must keep output deterministic by sorting final results by
  fixture ID and repetition.
- Artifact writes must be atomic where partial files would mislead CI.

## Documentation Changes

README additions:

````markdown
## Evaluations

Run coding-task evaluations against your configured model:

```bash
nandocodego eval run ./evals
```

Run the deterministic CI subset without Ollama:

```bash
nandocodego eval run ./evals --provider recorded --tag deterministic
```
````

Include an example report containing:

- fixture status and score;
- tests passed/failed;
- files and unexpected files changed;
- tool calls;
- approvals;
- runtime;
- model;
- retries;
- diff size;
- failure reason.

Update:

- `docs/phases/PHASE-18-DETAILED-PLAN.md` to route eval implementation here;
- `docs/roadmap/NEXT-PHASES-IMPLEMENTATION-PLAN.md` if implementation timing changes;
- `docs/phases/PHASE-LOG.md` when each eval slice lands.

## Risks And Mitigations

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Recorded responses become coupled to prompt formatting | Frequent fixture breakage | Match only stable request properties and ordered turns |
| Evals score final prose instead of code | False confidence | Keep final-answer weight low and use tests/golden files as primary checks |
| Fixtures mutate their source directories | Non-repeatable results | Always copy into a temporary workspace and verify source digests in tests |
| Test commands execute unsafe fixture code | Host compromise | Treat fixtures as trusted, document risk, use no-shell commands and future sandboxing |
| Live results are nondeterministic | Flaky CI | Keep live runs outside blocking CI; use recorded provider for PR gates |
| Scoring weights hide critical failures | False passes | Define hard gates for required tests, forbidden changes, timeout, and runtime errors |
| Reports leak secrets or thinking | Privacy failure | Redact environment values and omit thinking by default |
| Runtime assembly drifts from normal NandoCode behavior | Invalid measurements | Reuse shared non-interactive assembly and real agent/tools |
| Diff computation is expensive on large files | Slow or memory-heavy runs | Bound text diff size and fall back to byte metrics |
| Fixture schemas drift | Broken historical comparisons | Version configs and result JSON; reject unsupported versions |

## Acceptance Criteria

### Command And Discovery

- [ ] `nandocodego eval run ./evals` is registered and documented.
- [ ] `nandocodego eval validate ./evals` validates without executing tasks.
- [ ] Fixtures are discovered recursively and sorted by stable ID.
- [ ] Duplicate IDs and invalid schema versions fail validation.

### Fixtures

- [ ] At least five evaluation fixtures exist.
- [ ] Every fixture contains `task.md`, `repo/`, `expected/`, and
      `scoring.yaml`.
- [ ] Every deterministic fixture includes a valid recording.
- [ ] Fixture source directories remain unchanged after execution.

### Execution

- [ ] Live mode uses the real agent loop and configured model runtime.
- [ ] Recorded mode uses the real agent loop and tools without model network
      calls.
- [ ] Each fixture executes in a fresh isolated workspace.
- [ ] Timeouts and cancellation stop active work.
- [ ] Permission prompts and decisions are instrumented.

### Scoring

- [ ] Task completion status is reported.
- [ ] Tests passed and failed are reported.
- [ ] Files changed are reported.
- [ ] Unexpected files changed are reported.
- [ ] Tool-call count is reported.
- [ ] Approval count is reported.
- [ ] Runtime is reported.
- [ ] Model used is reported.
- [ ] Retry count is reported.
- [ ] Final diff size is reported.
- [ ] Failure reason is reported.
- [ ] Required tests, forbidden changes, timeouts, and infrastructure errors are
      hard gates.

### Reports

- [ ] Eval results are emitted as JSON.
- [ ] Eval results are emitted as Markdown.
- [ ] Result JSON has a schema version.
- [ ] Reports are written when fixtures fail.
- [ ] Per-fixture test logs and diff artifacts are retained.
- [ ] Reports omit thinking and redact credentials by default.

### CI

- [ ] CI runs a deterministic subset with a mock or recorded model.
- [ ] Deterministic CI requires no Ollama service or model credential.
- [ ] CI fails below the configured threshold.
- [ ] JSON and Markdown reports are uploaded as artifacts.
- [ ] Deterministic fixture execution passes under the race detector in targeted
      tests.

### Documentation

- [ ] README includes live and deterministic eval commands.
- [ ] README includes an example eval report.
- [ ] Phase 18 routes its eval work to this detailed plan.
- [ ] Phase log records implementation files, checks, known limitations, and
      live evidence.

## Definition Of Done

The framework is done for its first release when:

1. A clean checkout can build NandoCode and run the five deterministic fixtures
   without Ollama.
2. The same five fixtures can run against a real configured Ollama model.
3. Each run produces complete JSON and Markdown evidence.
4. CI detects an intentionally broken fixture solution.
5. CI detects an unexpected file mutation.
6. Re-running a recorded fixture produces the same verdict and normalized
   metrics.
7. The README report example is generated from an actual eval run.
8. The framework is ready to expand toward Phase 18's larger model-quality
   suite without changing the version 1 fixture contract.
