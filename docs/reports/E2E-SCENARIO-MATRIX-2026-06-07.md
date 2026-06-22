# E2E Scenario Matrix - 2026-06-07

## Coordinator Baseline

## BASE-001 Regression Fast

- Lane: `Coordinator`
- Owner agent: `Coordinator`
- Priority: `p0`
- Automation level: `automated`
- Evidence level achieved: `E0`
- Status: `pass`
- Attempts: `1`
- Start time: `2026-06-07 local`
- End time: `2026-06-07 local`

### Preconditions

- Workspace at commit `cd6743c7968ea6c809859a9a1f8a8f30ea930e88`
- Repo make target `regression-fast` available

### Steps

1. Run `make regression-fast`.

### Expected Result

Repo-supported fast regression gate passes.

### Actual Result

Passed. Reported one expected sandbox skip for `./internal/tools/webfetch/...`.

### Evidence

- Command output excerpt: `Fast regression gate passed.`
- Sanitization notes: no secrets present

### Defects Or Blocks

- Bug:
- Block:

### Retest Notes

Rerun after any changes to covered packages or test harness scripts.

## BASE-002 Regression Full

- Lane: `Coordinator`
- Owner agent: `Coordinator`
- Priority: `p0`
- Automation level: `automated`
- Evidence level achieved: `E0`
- Status: `pass`
- Attempts: `2`
- Start time: `2026-06-07 local`
- End time: `2026-06-07 local`

### Preconditions

- Workspace at commit `cd6743c7968ea6c809859a9a1f8a8f30ea930e88`
- Repo make target `regression-full` available

### Steps

1. Run `make regression-full` inside sandbox.
2. Observe sandbox listener/socket failures and Go module stat-cache permission warning.
3. Rerun `make regression-full` with escalated permissions.

### Expected Result

Repo-supported full regression gate passes in an environment that allows local listener and socket tests.

### Actual Result

First attempt failed due sandbox restrictions for `httptest` listeners and Unix domain sockets. Escalated rerun passed.

### Evidence

- Command output excerpt: `Full regression gate passed.`
- Sanitization notes: no secrets present

### Defects Or Blocks

- Bug:
- Block:

### Retest Notes

Use escalated permissions when reproducing in this environment.

## BASE-003 Load Suite

- Lane: `Coordinator`
- Owner agent: `Coordinator`
- Priority: `p1`
- Automation level: `automated`
- Evidence level achieved: `E0`
- Status: `pass`
- Attempts: `1`
- Start time: `2026-06-07 local`
- End time: `2026-06-07 local`

### Preconditions

- Workspace at commit `cd6743c7968ea6c809859a9a1f8a8f30ea930e88`
- Repo make target `load-suite` available

### Steps

1. Run `make load-suite`.

### Expected Result

Repo-supported load/perf suite passes and emits benchmark output.

### Actual Result

Passed. Benchmark output emitted for long-transcript render/view paths in `internal/tui`.

### Evidence

- Command output excerpt: `Load/perf suite passed.`
- Sanitization notes: no secrets present

### Defects Or Blocks

- Bug:
- Block:

### Retest Notes

Rerun after TUI rendering or analysis-path changes.

## Lane Merge Status

- Lane AB: `blocked` - see [E2E-AGENT-AB-REPORT-2026-06-07.md](./E2E-AGENT-AB-REPORT-2026-06-07.md)
- Lane CD: `blocked` - see [E2E-AGENT-CD-REPORT-2026-06-07.md](./E2E-AGENT-CD-REPORT-2026-06-07.md)
- Lane EF: `blocked` - see [E2E-AGENT-EF-REPORT-2026-06-07.md](./E2E-AGENT-EF-REPORT-2026-06-07.md)
- Lane GH: `blocked` - see [E2E-AGENT-GH-REPORT-2026-06-07.md](./E2E-AGENT-GH-REPORT-2026-06-07.md)
- Lane I: `in_progress_checkpoint_only` - see [E2E-AGENT-I-REPORT-2026-06-07.md](./E2E-AGENT-I-REPORT-2026-06-07.md)
