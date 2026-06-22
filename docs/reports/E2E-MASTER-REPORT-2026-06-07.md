# E2E Master Report - 2026-06-07

## Executive Summary

- Final recommendation: `blocked`
- Commit tested: `cd6743c7968ea6c809859a9a1f8a8f30ea930e88`
- Overall status: baseline automation complete; lane validation partially completed with confirmed defects and incomplete critical coverage
- Highest severity found: `sev2_high`
- Release-impact summary: automated regression and load gates passed, but server-side cloud model selection is inconsistent with the advertised model catalog and interactive lanes remain incomplete

## Environment Summary

- OS and machine: `Darwin 25.5.0 arm64`
- Go version: `go1.26.2 darwin/arm64`
- Ollama version and models: client `0.30.5`; installed models observed with unsandboxed loopback access: `qwen3-embedding:8b`, `qwen3.6:35b`, `kimi-k2.6:cloud`
- Browser automation availability: `not verified`
- Isolated state paths: baseline used repo defaults; lane execution used isolated temp roots
- Known environment limitations:
  - sandboxed `go test` cannot bind local TCP listeners or Unix sockets
  - sandboxed Go module stat-cache writes under `/Users/fernando/go/pkg/mod` are not permitted
  - escalated rerun was required for `make regression-full`

## Coverage Summary

| Lane | Passed | Failed | Blocked | Future Phase | Evidence Quality | Confidence |
| --- | --- | --- | --- | --- | --- | --- |
| Coordinator baseline | 3 | 0 | 0 | 0 | `E0` | medium |
| Lane AB | 12 | 3 | 2 | 0 | mixed (`E0-E2`) | medium |
| Lane CD | 3 | 1 | partial | 0 | mixed (`E2`) | low |
| Lane EF | 0 | 0 | partial | 0 | none | low |
| Lane GH | 9 | 1 | partial | 0 | mixed (`E0-E1`) | medium |
| Lane I | 0 | 0 | partial | 0 | none | low |

## Scenario Matrix Summary

- Total scenarios: `partial coverage only`
- `p0` status: `not complete`
- `p1` status: `not complete`
- `p2` status: `partial`
- `p3` status: `not assessed`

## Release Blockers

None at `sev0_release_blocker` or `sev1_critical`.

## High And Medium Risks

| Finding | Severity | Area | User Impact | Workaround | Recommendation |
| --- | --- | --- | --- | --- | --- |
| Full regression gate requires unsandboxed local socket/listener access to reflect real results | `sev3_medium` | test environment | false failures under sandbox | rerun with escalated permissions | document as environment constraint in reports |
| Server session model endpoint rejects a cloud model that `/v1/models` advertises | `sev2_high` | server/runtime | server clients can be shown a selectable-looking model that the API refuses | stay on local models only | fix model-listing and model-switch consistency |
| Cheap `--print` prompts still include tool schemas in prompt dumps | `sev3_medium` | cli/agent routing | unnecessary prompt budget consumption on trivial requests | none in product behavior | align print-mode routing with `ToolModeNone` expectations |
| Interactive TUI, memory/hooks, and coordinator-task lanes remain only partially evidenced | `sev3_medium` | validation coverage | release confidence is incomplete on user-facing interactive surfaces | none | finish lane execution in a PTY/browser-capable environment |

## Blocked Coverage

Blocked or partial areas:

- `B-007` and `B-008` are blocked because current cloud credentials are already configured and the missing-credential path cannot be safely forced without mutating user auth state.
- Lane `EF` and `I` never progressed beyond setup checkpoints in this run.
- Browser-interactive server flows remain unproven.

## Documentation Drift

Confirmed documentation drift:

- Invalid config on `--print` warns and continues instead of failing, so `A-009` as currently written is too strong unless fail-fast behavior is made intentional.

## Performance And Reliability

- `make load-suite` passed.
- Benchmark signal captured from `internal/tui`:
  - `BenchmarkRenderTranscript_LongTranscript_WarmCache`: `30167 ns/op`
  - `BenchmarkRenderTranscript_LongTranscript_Cold`: `939519 ns/op`
  - `BenchmarkRenderTranscript_LongTranscript_StreamingTail`: `70698 ns/op`
  - `BenchmarkView_LongTranscript_AtBottom`: `97663 ns/op`

## Security And Permission Findings

No permission-boundary or auth-bypass defect was confirmed in this run.

- Non-loopback bind without `--token` correctly fails with `Error: refusing non-loopback bind without --token`.
- Replay via `Last-Event-ID` worked for the tested SSE session.

## Test Confidence Assessment

- Strongest evidence: repo-supported regression/load gates, direct loopback server checks, local/cloud `--print`, semantic index lifecycle
- Weakest evidence: TUI, memory/hooks, multi-agent tasks, and performance lane scenarios
- Scenarios requiring second-agent reproduction:
  - `BUG-20260607-server-model-endpoint-rejects-listed-cloud-model`
  - `BUG-20260607-print-cheap-prompt-includes-tool-schemas`
- Remaining blind spots:
  - Lane `EF` user-facing flows
  - most Lane `I` latency/reliability scenarios
  - browser-only UI flows under Lane `G`

## Final Recommendation

`blocked`

Rationale: the automated baseline is green, but the run is not releasable as executed because one confirmed `sev2_high` server defect remains open, one confirmed `sev3_medium` request-shaping defect remains open, and critical interactive coverage is incomplete.
