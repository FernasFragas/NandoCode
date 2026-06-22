# Performance Follow-Up Multi-Agent Plan

Date: 2026-06-06
Status: In progress
Scope: finish the remaining performance follow-up after the response-time refactor landed on `performance_branch`

## Objective

Close the remaining performance work without reworking already-landed optimizations.

Focus areas:

- collect real latency evidence for small, medium, and large prompts;
- extend semantic benchmarks to more production-like dimensions;
- improve trace/debug output for slow runs;
- strengthen TUI render benchmarks and caching-related tests;
- strengthen fast-path, startup, and hook validation coverage.

## Current Ground Truth

Already implemented in source:

- adaptive context policy and 8K output-budget default;
- simple-prompt routing with semantic skip and `ToolModeNone`;
- semantic records/vector cache;
- semantic light candidate narrowing;
- parallel semantic scan and bounded parallel context file reads;
- startup parallel preparation;
- TUI transcript caching and throttled streaming refresh;
- semantic retrieval timing/events and focused benchmark coverage.

Still worth doing:

- real-world latency evidence capture from the current app behavior;
- production-like semantic benchmark dimensions and larger index shapes;
- higher-signal `/trace last` diagnostics;
- stronger TUI render benchmark coverage for long transcripts and active tails;
- stronger tests for fast-path, startup, and hook behavior;
- final focused verification and documentation of remaining risks.

## Workstreams

Coordinator ownership:

- this tracking document;
- baseline verification;
- integration of worker changes;
- focused test and benchmark runs;
- final summary and remaining-risk documentation.

Worker 1 ownership:

- `internal/semantic/retrieve_bench_test.go`
- optional additional `internal/semantic/*_test.go` benchmark helpers only

Worker 2 ownership:

- `internal/commands/registry.go`
- `internal/commands/registry_test.go`
- optional narrow changes in `internal/observability/metrics.go`

Worker 3 ownership:

- `internal/tui/render_benchmark_test.go`
- `internal/tui/app_test.go`

Worker 4 ownership:

- `internal/retrievalroute/route_test.go`
- `internal/agent/agent_test.go`
- `internal/hooks/dispatch_test.go`
- `internal/cli/repl_test.go`

## Baseline

Focused verification completed before parallel changes:

```bash
go test ./internal/contextpack ./internal/tui ./internal/hooks ./internal/cli ./internal/bootstrap ./internal/agent ./internal/semantic ./internal/retrievalroute ./internal/server
```

Result: pass

Notes:

- the worktree is already dirty; integration must avoid reverting unrelated user/doc changes;
- this follow-up should stay scoped to performance evidence, diagnostics, and validation first;
- production-code changes should remain narrow unless tests expose a concrete gap.

Semantic benchmark baseline captured locally:

- `BenchmarkLocalStoreLoadRecordsScaling/large_4800`: about `9.51 ms/op`
- `BenchmarkLocalStoreLoadVectorsScaling/large_4800`: about `4.31 ms/op`
- `BenchmarkScoreRecordsScaling/large_4800`: about `2.53 ms/op`
- `BenchmarkScoreRecordsLightModeScaling/large_4800`: about `0.92 ms/op`
- `BenchmarkLocalServiceRetrieveScaling/large_4800`: about `12.69 ms/op`

Interpretation:

- cold record/vector loading still dominates retrieval more than scoring does;
- light-mode candidate narrowing is materially cheaper than full scoring;
- retrieval remains a good target for larger-dimension benchmark expansion, but
  the biggest wins now come from routing and cache behavior rather than raw dot
  product tuning alone.

TUI baseline captured locally:

- `BenchmarkRenderTranscript_1000AssistantDeltas`: about `5.96 us/op`
- `BenchmarkRenderTranscript_10000AssistantDeltas`: about `6.02 us/op`
- `BenchmarkRenderTranscript_InterleavedToolEvents`: about `8.97 us/op`
- `BenchmarkRenderTranscript_ThinkingAndSystemMix`: about `9.45 us/op`
- `BenchmarkView1000Items`: about `108.06 us/op`

## Acceptance Criteria

- simple prompts remain on the cheap path with semantic skip and no tool schemas;
- explicit-file and listing prompts do not regress into broad semantic retrieval;
- broad discovery prompts still retain semantic retrieval behavior;
- semantic benchmarks cover more realistic dimensions than the current `256` baseline;
- `/trace last` better explains slow runs without leaking prompt or file contents;
- TUI performance coverage better reflects long transcripts and active streaming tails;
- focused tests pass after integration;
- remaining risks are written down explicitly.

## Execution Log

- 2026-06-06: coordinator spawned four workers on disjoint ownership lanes.
- 2026-06-06: focused baseline test suite passed before integration work.
- 2026-06-06: worker lane integrated dimension-aware semantic benchmarks with
  `256`, `1024`, and constrained `4096` profiles.
- 2026-06-06: worker lane integrated stronger `/trace last` diagnosis output.
- 2026-06-06: coordinator extended run traces to carry `tool_mode`,
  `route_profile`, `route_action`, and `route_reason`.
- 2026-06-06: worker lane integrated stronger TUI render benchmarks and cache
  safety tests for long transcripts and active tails.
- 2026-06-06: worker lane integrated extra fast-path, startup fallback, and
  hook concurrency validation.
- 2026-06-06: focused integration suite passed:

```bash
go test ./internal/retrievalroute ./internal/agent ./internal/hooks ./internal/cli ./internal/tui ./internal/semantic ./internal/observability ./internal/commands
```

Additional benchmark evidence from the worker TUI lane:

- `BenchmarkRenderTranscript_LongTranscript_WarmCache`: `41750 ns/op`
- `BenchmarkRenderTranscript_LongTranscript_Cold`: `798209 ns/op`
- `BenchmarkRenderTranscript_LongTranscript_StreamingTail`: `79792 ns/op`
- `BenchmarkView_LongTranscript_AtBottom`: `195583 ns/op`
