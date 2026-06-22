# Go Response Time Performance Refactor Report

Date: 2026-05-28

## Scope

This report reviews the current codebase for response-time refactors, with
specific attention to the Go concurrency and performance guidance in:

- `book/ch07-concurrency.md`
- `book/ch17-performance.md`

The codebase scan covered the main request path, semantic retrieval, context
packing, TUI rendering, tool execution, hooks, memory recall, startup, server
SSE sessions, and CLI print/index paths.

Primary files inspected:

- `internal/agent/agent.go`
- `internal/agent/stream.go`
- `internal/agent/tools.go`
- `internal/agent/speculative.go`
- `internal/agent/partition.go`
- `internal/agent/context_policy.go`
- `internal/tui/app.go`
- `internal/server/session.go`
- `internal/semantic/service.go`
- `internal/semantic/store.go`
- `internal/semantic/search.go`
- `internal/semantic/scanner.go`
- `internal/semantic/vectors.go`
- `internal/semantic/embedder.go`
- `internal/llm/ollama/ollama.go`
- `internal/llm/router.go`
- `internal/observability/llm.go`
- `internal/contextpack/current_turn.go`
- `internal/contextpack/range_pipeline.go`
- `internal/mentions/expand.go`
- `internal/tui/fileindex/index.go`
- `internal/tui/picker/file_provider.go`
- `internal/hooks/runner.go`
- `internal/hooks/dispatch.go`
- `internal/memory/runner.go`
- `internal/cli/repl.go`
- `internal/cli/print.go`
- `internal/bootstrap/state.go`

## Executive Summary

The project already has several strong Go-first performance foundations:

- concurrent safe tool execution exists through `Partition` and
  `SpeculativeExecutor`;
- tool concurrency is bounded by `MaxConcurrentTools`;
- TUI transcript rendering has virtualization, height caching, markdown caching,
  and streaming refresh throttling;
- context mode now has adaptive `num_ctx` policy support;
- semantic routing exists and can bypass embeddings for simple/general prompts;
- semantic build progress is observable;
- server sessions emit semantic route/retrieval SSE events.

The largest remaining response-time opportunities are not raw Go CPU
micro-optimizations. They are reducing unnecessary work before the model starts
and using Go concurrency/caching where the current code repeats serial I/O.

Highest-impact candidates:

1. Keep simple prompts on a true fast path: no embeddings, reduced/no tool
   schemas, small output reservation, and small `num_ctx`.
2. Fix embedding option propagation through runtime/observability wrappers so
   dimensions and keep-alive are preserved everywhere.
3. Cache loaded semantic records/vectors in memory and avoid re-reading JSONL
   and `.f32` files on every retrieval.
4. Split semantic retrieval timing into manifest, records load, vectors load,
   query embed, scoring, and rendering.
5. Make semantic light mode actually narrow the candidate set instead of
   scoring the whole index and merely adding current-path weight.
6. Apply worker-pool concurrency to index scanning/context packing where many
   independent files are read.
7. Use token-budget defaults from `book/ch17-performance.md`: reserve about
   8K output by default and escalate on truncation instead of sizing every turn
   for very large output.
8. Add startup parallelism around independent I/O and model/runtime checks.

## Book Guidance Applied To This Codebase

### Chapter 7: Concurrency

Relevant takeaways:

- Tool calls that are independent and read-like should run concurrently.
- Safety is per-call, not only per tool type.
- Preserve model-visible result order even when execution is parallel.
- Bound concurrency. The book uses a default of 10.
- Start speculative work as soon as stream parsing allows it.
- Fail closed when a tool call cannot be parsed or classified.
- Progress events can stream immediately, while final results remain ordered.

Current implementation status:

- `internal/agent/partition.go` implements a left-to-right partitioner.
- `internal/agent/speculative.go` executes safe batches with `errgroup` and a
  concurrency limit.
- `internal/agent/agent.go` uses `executeToolCallsConcurrent`.
- `internal/tools/fileread/fileread.go` marks `FileRead` as concurrent-safe.
- `internal/tools/builtin` tools are wrapped with observability.

Remaining gaps:

- The Ollama client only receives tool calls when the done event arrives, so
  true mid-stream speculative tool execution is not active for Ollama.
- Concurrent batch execution does not forward per-tool progress channels.
- Hook dispatch is serial even when multiple matching hooks are independent.
- Context packing and semantic indexing read independent files serially.

### Chapter 17: Performance

Relevant takeaways:

- Startup should overlap independent I/O.
- Preconnect/warm network clients when a model request is likely.
- Fast-path commands should not load the full interactive stack.
- Token efficiency matters as much as CPU speed.
- Use an 8K-ish default output slot and retry/escalate on truncation.
- Do not blindly request very large context windows.
- Prompt cache friendliness improves when stable prompt sections come first.
- Rendering should throttle to frame budget and avoid re-rendering unchanged
  content.
- Search should use prefilters, score-bound rejection, and async indexability.
- Measure before final tuning.

Current implementation status:

- `internal/tui/app.go` throttles streaming refreshes to roughly 50ms.
- Transcript rendering is virtualized when the viewport is at the bottom.
- Semantic benchmarks exist in `internal/semantic/retrieve_bench_test.go`.
- Render benchmarks exist in `internal/tui/render_benchmark_test.go`.
- The context policy can auto-size `num_ctx`.

Remaining gaps:

- `MaxOutputTokens` is still commonly initialized from large live model limits,
  which can make auto `num_ctx` choose a much larger window than simple prompts
  need.
- Every agent turn still builds full enabled tool definitions.
- Semantic retrieval reloads records/vectors from disk on each call.
- Semantic scoring scans every record linearly even for light mode.
- Startup initialization in `internal/cli/repl.go` is largely serial.
- Observability does not yet expose semantic substage timings or prompt/tool
  schema byte budgets.

## Findings And Refactor Opportunities

### P0: Preserve Embedding Options Through LLM Wrappers

Current path:

- `internal/semantic/embedder.go` uses `llm.EmbedderWithOptions` when the
  client exposes it.
- `internal/llm/ollama/ollama.go` supports `EmbedWithOptions`, including
  `dimensions`, `truncate`, and `keep_alive`.
- `internal/llm/router.go` only exposes `Embed`.
- `internal/observability/llm.go` only exposes `Embed`.
- REPL/server semantic services are constructed with the wrapped runtime client:
  `semantic.LLMEmbedder{Client: client}`.

Impact:

- The direct index CLI path can pass embedding dimensions.
- The TUI/server retrieval path can lose those options through wrappers.
- This can recreate or worsen the earlier dimensions mismatch pattern:
  `got 4096 want 1024`.
- It can also drop `query_keep_alive`, increasing query-embedding cold starts.

Proposed refactor:

- Add `EmbedWithOptions` to `RuntimeClient` when the current client supports it.
- Add `EmbedWithOptions` to `observedLLMClient` and record it as an embedding
  call, not a generic chat call.
- Add tests proving dimensions/keep-alive survive:
  Ollama client -> runtime router -> observability wrapper -> semantic embedder.

Expected benefit:

- Removes a correctness risk in semantic retrieval.
- Avoids unnecessary full-dimension embeddings when the index was built with a
  reduced dimension.
- Reduces fallback/error churn before chat generation starts.

### P0: Make Simple Prompts A True No-Extra-Work Fast Path

Current path:

- `internal/retrievalroute/route.go` now skips embeddings for general prompts.
- `internal/agent/stream.go` still builds and sends all enabled tool definitions
  for every turn.
- `internal/agent/context_policy.go` uses `outputTokenBudget` in auto
  `num_ctx` sizing.
- `internal/bootstrap/state.go` defaults `MaxOutputTokens` to `65000`.
- `internal/llm/limits.go` can derive large output limits from live model
  context length.

Impact:

- A prompt like "how is the weather" can skip semantic retrieval but still pay
  for a large model request shape.
- Large `num_predict` plus large `num_ctx` increases prompt setup/prefill cost.
- Full tool schemas make the prompt larger and can slow first token.

Proposed refactor:

- Introduce a `RequestProfile` or `ToolMode` on `agent.Input`:
  `chat_only`, `read_only`, `default_agent`.
- For `chat_only`:
  - omit tool definitions;
  - cap output reserve to 1024-2048 unless the prompt asks for long output;
  - force context mode to `small` or compute a small tier;
  - skip memory recall unless explicitly requested.
- For `read_only`:
  - include only file/list/search/read tools.
- For `default_agent`:
  - keep current behavior.

Expected benefit:

- Lower first-token latency for general/simple prompts.
- Fewer prompt tokens.
- Less chance that Ollama allocates a huge context window for a short answer.

Book mapping:

- `book/ch17-performance.md`: fast-path dispatch, token efficiency, context
  window sizing, and output-slot escalation.

### P0: Change Output Budget Policy To 8K Default Plus Retry

Current path:

- `agent.DefaultConfig()` defaults `MaxOutputTokens` to `8192`.
- `bootstrap.DefaultInitial()` defaults `MaxOutputTokens` to `65000`.
- REPL startup overwrites agent config with `startupMaxOutputTokens`.
- `ComputeLimits` can choose half the model context as max output when
  `num_predict` is absent.
- Auto context sizing uses `outputTokenBudget`.

Impact:

- Large output budgets can inflate `num_ctx` even for normal prompts.
- This directly affects "Waiting for model..." because Ollama may allocate and
  prefill a larger context.

Proposed refactor:

- Make interactive default `MaxOutputTokens = 8192`.
- Keep `LengthRetryTokens = 65536`.
- Use live model limits as a hard ceiling, not the default request size.
- Add a `LongOutputRequested` detector only for prompts asking for long plans,
  full files, reports, or docs.
- On `done_reason=length`, retry with the larger budget exactly as the agent
  already does.

Expected benefit:

- Large responses still work through retry/escalation.
- Common prompts avoid paying the maximum-output tax.

Book mapping:

- `book/ch17-performance.md`: default output slot should be modest and increase
  only on truncation.

### P1: Cache Semantic Records And Vectors In Memory

Current path:

- `LocalService.Retrieve` loads manifest, records, and vectors on every call.
- `LoadRecords` parses `records.jsonl` into `[]Record`.
- `LoadVectors` reads `.f32` into `[][]float32`.
- Existing benchmark evidence in `docs/WAITING-FOR-MODEL-LATENCY-REPORT.md`
  shows load cost and allocations grow with record count.

Impact:

- Retrieval after indexing repeats disk I/O and allocations every prompt.
- A 4,800-record synthetic index already reaches about 32ms end-to-end with
  256 dimensions; production dimensions and record counts can be much higher.
- Repeated load cost also creates garbage collector pressure.

Proposed refactor:

- Add a `semantic.IndexCache` owned by `LocalService`.
- Cache key:
  - canonical root;
  - workspace ID;
  - manifest updated timestamp;
  - schema version;
  - model;
  - dimensions.
- Cache value:
  - manifest;
  - records;
  - vectors;
  - optional path metadata/indexes for candidate filtering.
- Invalidate on:
  - `Build`;
  - `Refresh`;
  - `Clear`;
  - manifest timestamp/workspace ID change.
- Add a memory cap and an opt-out config flag.
- Use `sync.RWMutex` or `atomic.Value` for read-heavy access.

Expected benefit:

- Turns repeated retrieval from "load + embed + score + render" into
  "embed + score + render".
- Less GC churn.
- Better responsiveness after the first retrieval.

Book mapping:

- `book/ch17-performance.md`: search speed and measurement-first caching.

### P1: Make Semantic Light Mode Actually Narrow The Search

Current path:

- `scoreRecords` computes dot product for every record.
- `UseCurrentPathWeight` adds score for current-turn paths.
- It does not reduce the candidate set.
- Historical findings in `docs/WAITING-FOR-MODEL-LATENCY-REPORT.md` note that
  current-turn paths did not materially affect output in tests.

Impact:

- `semantic_light` is cheaper in returned context, not necessarily cheaper in
  search work.
- Explicit-file related-code prompts still score the whole index.

Proposed refactor:

- Build a candidate selector before dot-product scoring.
- For `semantic_light`, include:
  - exact current file records;
  - folder records for current directories;
  - sibling files in the same directory;
  - files with lexical matches to import/package/function terms;
  - explicitly mentioned paths.
- Only fall back to full-index scoring if the candidate set is too small.
- Keep deterministic ordering and stable tie-breaks.

Expected benefit:

- Much less CPU for explicit-file related prompts.
- Lower latency and fewer allocations.
- Better semantic precision because "nearby" really stays nearby.

Book mapping:

- `book/ch17-performance.md`: bitmap/prefilter/search narrowing before scoring.

### P1: Parallelize Semantic Index Scanning And Extraction

Current path:

- `ScanWorkspace` walks files, then processes each file serially.
- Each file may perform:
  - `os.ReadFile`;
  - content filtering;
  - hashing;
  - Go symbol extraction;
  - markdown section extraction;
  - chunk fallback.

Impact:

- Index build/refresh is slower than it needs to be on multi-core machines.
- The work is highly parallel by file and naturally fits a worker pool.

Proposed refactor:

- Keep `dirwalk.Walk` as the deterministic discovery step.
- Feed file entries into a bounded worker pool:
  - default workers: `min(runtime.NumCPU(), 8)` or configurable;
  - preserve result determinism by storing each file's result with original
    index and sorting/merging at the end.
- Make progress counters atomic or channel-owned.
- Keep secret detection and max-file caps exactly equivalent.
- Preserve cancellation via context.

Expected benefit:

- Faster `/index build` and `/index refresh`.
- Better use of Go's scheduler on large repositories.

Book mapping:

- `book/ch07-concurrency.md`: independent safe work should run concurrently.

### P1: Add Parallel Context Packing For Multi-File Mentions

Current path:

- `contextpack.BuildCurrentTurnPrompt` expands and packs mentions before model
  execution.
- `buildEvidenceParts` reads mentioned files serially.
- `selectDirectoryEvidence` walks directories and reads selected files serially.
- Large-file metadata and match scanning are serial per file.

Impact:

- A prompt mentioning several files or a directory can stall before any model
  event is emitted.
- The user experiences this as "Waiting for model..." unless status
  instrumentation is detailed.

Proposed refactor:

- Resolve mentions serially to preserve permission/path validation.
- Read independent files with a bounded worker pool.
- For directories, separate selection from content reading:
  - walk/select candidates deterministically;
  - read selected candidates in parallel;
  - render in deterministic path order.
- Preserve budget allocation deterministically by computing sizes first, then
  applying budget in stable order.

Expected benefit:

- Faster prompt preparation for multi-file prompts.
- More predictable latency for `@folder` and broad context tasks.

Book mapping:

- `book/ch07-concurrency.md`: run adjacent safe read operations concurrently
  while preserving result order.

### P1: Split Semantic Retrieval Observability Into Substages

Current path:

- TUI records only `semantic_retrieve`.
- Server emits `semantic_query_embed_started/finished` and
  `semantic_search_finished`, but `semantic_query_embed_finished` currently
  covers the whole retrieval duration in the success path.
- `LocalService.Retrieve` internally has clear substages but does not report
  them.

Impact:

- It is hard to tell whether retrieval latency is:
  - manifest scan;
  - records load;
  - vectors load;
  - query embedding;
  - vector scoring;
  - context rendering/stale checks.

Proposed refactor:

- Add optional retrieval observer/events to `RetrieveRequest`.
- Emit:
  - `semantic_manifest_load_ms`;
  - `semantic_records_load_ms`;
  - `semantic_vectors_load_ms`;
  - `semantic_query_embed_ms`;
  - `semantic_score_ms`;
  - `semantic_render_ms`;
  - `semantic_total_ms`;
  - record/vector counts and cache hit/miss.
- Keep public TUI copy compact:
  `[Semantic: embed=420ms search=18ms render=7ms records=12 files=4 cache=hit]`.

Expected benefit:

- Makes future tuning evidence-based.
- Separates embedding-model cost from Go CPU/I/O cost.

Book mapping:

- `book/ch17-performance.md`: measure first.

### P1: Reduce Tool Schema Payload Per Turn

Current path:

- `executeOneTurn` always calls `buildToolDefs`.
- `buildToolDefs` iterates every enabled tool and serializes every schema.
- The chat request always includes the full tool set when registry is present.

Impact:

- Even simple answers pay prompt-token and JSON serialization cost.
- Tool schemas can reduce prompt-cache effectiveness if generated or ordered
  differently across environments.

Proposed refactor:

- Cache converted `[]llm.ToolDef` per registry/toolset/profile.
- Add `ToolMode`:
  - `none`;
  - `read_only`;
  - `code_edit`;
  - `full_agent`;
  - `coordinator`.
- Select mode from retrieval route/prompt intent:
  - general prompt -> none;
  - explicit read/list/status -> read-only;
  - fix/implement/refactor -> full_agent;
  - coordinator mode -> coordinator.
- Make tool definition ordering stable.

Expected benefit:

- Smaller prompt.
- Faster request marshaling.
- Faster model first token on non-agent prompts.

Book mapping:

- `book/ch17-performance.md`: fast-path dispatch and prompt-cache structure.

### P2: Parallelize Safe Hook Dispatch With Deterministic Aggregation

Current path:

- `Dispatcher.Dispatch` runs matching hooks serially.
- Hook kinds can call command, prompt, agent, or HTTP.

Impact:

- Multiple independent hooks stack latency.
- Pre-tool and user-prompt hooks can delay model start.

Proposed refactor:

- Add a hook config flag: `parallel_safe = true`.
- Default existing hooks to serial for compatibility.
- For hooks explicitly marked safe:
  - run with `errgroup.SetLimit`;
  - preserve result aggregation order;
  - retain fail-closed behavior for decisive deny/ask results;
  - use per-hook timeouts.

Expected benefit:

- Users with multiple independent HTTP/command hooks avoid additive latency.

Book mapping:

- `book/ch07-concurrency.md`: concurrency must preserve semantic order and
  safety.

### P2: Improve TUI View Caching Beyond Transcript Virtualization

Current path:

- `View()` calls `renderTranscript()` and `viewport.SetContent()` every render.
- `refreshViewportContent()` also re-renders transcript.
- Active stream refresh is throttled to 50ms.
- Completed assistant markdown is cached per transcript item.

Impact:

- Existing benchmarks show rendering is already acceptable for many cases, but
  redundant render work still happens during frequent ticks/input changes.

Proposed refactor:

- Add transcript dirty version:
  - increment on transcript mutation, width change, collapse toggle, or style
    changes;
  - cache rendered transcript string by version and viewport width.
- Avoid `viewport.SetContent` in `View()` if content version did not change.
- Separate input/picker changes from transcript changes.
- Add benchmark:
  - repeated `View()` with no transcript mutation;
  - streaming delta while picker open;
  - large transcript scrolled away from bottom.

Expected benefit:

- Lower CPU during idle ticks and typing.
- More stable rendering on slower terminals.

Book mapping:

- `book/ch17-performance.md`: damage/diff rendering and object reuse.

### P2: File Picker Prefilter And Ranking Cache

Current path:

- `FileProvider.Suggest` copies the full index snapshot and fuzzy-scores every
  entry on each query change.
- `fuzzyScore` lowercases and rune-converts query/candidate repeatedly.

Impact:

- For 50k paths, typing in the picker can become O(entries * query length) per
  keystroke.

Proposed refactor:

- Store lowercase path/base and a character bitmask in `fileindex.Entry`.
- Quickly reject candidates that cannot contain all query characters.
- Cache last query results and refine when the new query extends the previous
  query.
- Use a bounded top-K heap instead of sorting all candidates.

Expected benefit:

- Faster mention autocomplete in large repos.
- Lower allocations while typing.

Book mapping:

- `book/ch17-performance.md`: bitmap prefilter and score-bound rejection.

### P2: Startup Parallelism

Current path:

- `runREPL` performs config, model detail fetch, registry construction, skills
  loader, MCP load/start/register, hooks load, memory directory setup, semantic
  service setup, model runtime setup, and TUI construction mostly serially.

Impact:

- REPL startup pays additive I/O/network costs.

Proposed refactor:

- After config/bootstrap is available, start independent tasks in an errgroup:
  - `ShowModel`;
  - hook snapshot load;
  - skills loader construction;
  - MCP config load/start;
  - memory scope directory check;
  - semantic status warm read, if enabled.
- Join before constructing the final agent/TUI model.
- Add startup stage timings.

Expected benefit:

- Faster initial TUI availability.
- Better diagnosis of slow MCP/model/config startup.

Book mapping:

- `book/ch17-performance.md`: module-level I/O parallelism and startup
  checkpointing.

### P2: API Preconnect And Keep-Alive Warming

Current path:

- Ollama HTTP client uses default transport.
- Chat keep-alive is supported and now passed when configured.
- Embedding keep-alive is configured separately but can be lost through wrappers.

Impact:

- First request can pay connection setup and model load.
- Embedding and chat models may compete for memory.

Proposed refactor:

- Add a small Ollama preconnect/warmup task:
  - `HEAD`/`GET /api/tags` or cheap model metadata request during startup;
  - bounded timeout;
  - no blocking of TUI if it exceeds a small budget.
- Keep chat model warm longer than query embedding model.
- Keep query embedding `keep_alive` short by default.
- Show model-load/request-open latency separately.

Expected benefit:

- Lower first request latency when Ollama is reachable.
- Better explanation when model cold start dominates.

Book mapping:

- `book/ch17-performance.md`: API preconnect and cache/warmup strategy.

### P3: Semantic Vector Search Data Layout

Current path:

- Vectors are loaded as `[][]float32`.
- `Dot` checks dimensions and loops over one slice at a time.
- `scoreRecords` allocates one `SearchHit` for every record before sorting.

Impact:

- Pointer chasing and allocations grow with index size.
- Full sort costs more than needed when only top-K is returned.

Proposed refactor:

- Add an internal flat vector representation:
  - `[]float32` with `dimensions` stride;
  - optional compatibility conversion for existing APIs.
- Validate dimensions once at load/cache time.
- Use a bounded min-heap for top `N * diversityFactor`, then diversify.
- Parallelize dot scoring above a threshold:
  - partition record ranges;
  - each worker keeps local top-K;
  - merge local heaps deterministically.

Expected benefit:

- Lower allocations and better CPU cache locality.
- Better scaling at 1024/4096 dimensions and large record counts.

Book mapping:

- `book/ch17-performance.md`: fused scans and score-bound rejection.

### P3: Prompt Cache Friendly Message Layout

Current path:

- System prompt can be augmented by memory.
- Tool schemas are included each turn.
- Semantic context is appended to current user prompt.

Impact:

- Volatile sections can reduce prompt cache reuse.

Proposed refactor:

- Keep stable sections first:
  - base system prompt;
  - stable tool definitions;
  - stable memory policy text.
- Place volatile sections late:
  - latest user prompt;
  - semantic context;
  - transient memory recall;
  - hook notices.
- Memoize stable generated sections for a session:
  - tool schema JSON;
  - base system prompt;
  - memory instruction boilerplate.

Expected benefit:

- Better provider-side prompt caching where supported.
- Less repeated string generation.

Book mapping:

- `book/ch17-performance.md`: stable prefix, volatile suffix.

## What Is Already Good And Should Be Preserved

- `Partition` is deterministic and matches the concurrency algorithm from the
  book.
- `SpeculativeExecutor` preserves result order while running safe batches.
- `MaxConcurrentTools` bounds parallel tool work.
- TUI transcript virtualization and markdown caching are already in place.
- Semantic route decisions now avoid embeddings for general prompts.
- `contextpack` has range-based packing for large files and explicit ranges.
- `FileRead` avoids returning unchanged context when a range snapshot already
  exists.
- Semantic build emits scan/extract/embed/write progress.

## Suggested Parallel Implementation Lanes

### Lane A: Request Shape Fast Path

Files:

- `internal/retrievalroute/route.go`
- `internal/agent/input.go`
- `internal/agent/stream.go`
- `internal/tui/app.go`
- `internal/server/session.go`
- `internal/cli/print.go`
- `internal/bootstrap/state.go`
- `internal/llm/limits.go`

Tasks:

- Add request/tool profile fields.
- Map route decisions to `chat_only`, `read_only`, or `default_agent`.
- Omit tools for chat-only.
- Cap simple-prompt output reserve.
- Treat live model limits as ceilings.
- Add tests for general prompts proving no embeddings and no tools.

### Lane B: Embedding Wrapper Correctness

Files:

- `internal/llm/router.go`
- `internal/observability/llm.go`
- `internal/semantic/embedder.go`
- `internal/semantic/service.go`

Tasks:

- Forward `EmbedWithOptions` through runtime router.
- Forward `EmbedWithOptions` through observed LLM wrapper.
- Record embedding metrics separately from chat metrics.
- Add tests for dimensions and keep-alive propagation.
- Add regression test for `got 4096 want 1024`.

### Lane C: Semantic Retrieval Cache And Substage Metrics

Files:

- `internal/semantic/service.go`
- `internal/semantic/store.go`
- `internal/semantic/contracts.go`
- `internal/tui/app.go`
- `internal/server/session.go`
- `internal/observability/metrics.go`

Tasks:

- Add cache struct and invalidation.
- Add observer callbacks for retrieval substages.
- Emit cache hit/miss and per-stage durations.
- Add benchmarks for cold vs warm retrieval.
- Add memory cap/disable config.

### Lane D: Semantic Search Narrowing And Vector Layout

Files:

- `internal/semantic/search.go`
- `internal/semantic/vectors.go`
- `internal/semantic/retrieve_bench_test.go`

Tasks:

- Add candidate selector for light mode.
- Add top-K heap instead of full sort.
- Add flat vector representation behind cache.
- Add parallel scoring threshold for large indexes.
- Benchmark 10k, 50k, and 100k records at 1024 dimensions.

### Lane E: Parallel Index Build And Context Packing

Files:

- `internal/semantic/scanner.go`
- `internal/contextpack/current_turn.go`
- `internal/contextpack/range_pipeline.go`
- `internal/mentions/expand.go`

Tasks:

- Add bounded worker pool for semantic file extraction.
- Add bounded worker pool for multi-file context packing.
- Preserve deterministic output ordering.
- Preserve budget semantics.
- Add cancellation tests and race tests.

### Lane F: UI And Picker Runtime Cost

Files:

- `internal/tui/app.go`
- `internal/tui/fileindex/index.go`
- `internal/tui/picker/file_provider.go`
- `internal/tui/picker/score.go`
- `internal/tui/render_benchmark_test.go`

Tasks:

- Add transcript render version/cache.
- Avoid redundant viewport `SetContent`.
- Add lowercase/bitmask path metadata.
- Add top-K heap for picker candidates.
- Add no-change `View()` benchmark and picker large-index benchmark.

### Lane G: Startup And Hooks

Files:

- `internal/cli/repl.go`
- `internal/server/server.go`
- `internal/hooks/dispatch.go`
- `internal/hooks/runner.go`

Tasks:

- Add startup errgroup for independent initialization.
- Add startup stage timings.
- Add optional parallel-safe hook execution.
- Add hook timing summary in TUI/server events.

## Multi-Agent Implementation Task Breakdown

### Readiness Review

The plan is ready to implement, but the work should not start as one large
uncoordinated edit. Several improvements touch central files such as
`internal/tui/app.go`, `internal/server/session.go`, and
`internal/semantic/service.go`. Multiple agents can work in parallel if file
ownership and merge order are explicit.

Implementation should be split into three waves:

1. Contract and isolated-package work.
2. Integration work through TUI/server/agent request paths.
3. Cross-package validation and benchmark evidence.

Do not let multiple agents edit these files at the same time:

- `internal/tui/app.go`
- `internal/server/session.go`
- `internal/semantic/service.go`
- `internal/semantic/contracts.go`
- `internal/agent/input.go`
- `internal/agent/stream.go`
- `internal/cli/repl.go`

If two agents need one of those files, the earlier agent must land first and
the later agent must re-read the file before editing.

### Shared Implementation Rules

All agents must follow these rules:

- Start with `git status --short` and do not revert unrelated work.
- Read the exact files assigned before editing.
- Keep changes narrow to the assigned lane.
- Add tests in the package being changed.
- Preserve deterministic ordering when adding concurrency.
- Bound all new goroutine pools.
- Respect `context.Context` cancellation in new concurrent code.
- Prefer feature flags or config defaults that preserve current behavior unless
  the task explicitly changes default latency behavior.
- Record benchmark output in the report or in `docs/PHASE-LOG.md` only after
  the benchmark actually runs.
- Before marking a lane complete, run that lane's package tests.

### Shared Contracts To Land First

These contracts reduce merge conflicts and give all agents stable targets.

Owner: Agent 0, or the first implementation agent if no coordinator agent is
available.

Files:

- `internal/agent/input.go`
- `internal/retrievalroute/route.go`
- `internal/semantic/contracts.go`
- `internal/observability/metrics.go`

Tasks:

- Add request/tool profile fields without changing behavior yet:
  - `agent.Input.RequestProfile`
  - `agent.Input.ToolMode`
  - `retrievalroute.Decision.RequestProfile`
  - `retrievalroute.Decision.ToolMode`
- Define initial profile constants:
  - `chat_only`
  - `read_only`
  - `default_agent`
  - `coordinator`
- Add semantic retrieval observer types:
  - `semantic.RetrieveStage`
  - `semantic.RetrieveStageEvent`
  - `semantic.RetrieveObserver`
  - optional `RetrieveRequest.Observer`
- Add metric field placeholders for:
  - embedding calls;
  - embedding errors;
  - embedding total duration;
  - semantic cache hits;
  - semantic cache misses.
- Add unit tests that prove the zero-value/default behavior is unchanged.

Acceptance criteria:

- `go test ./internal/agent ./internal/retrievalroute ./internal/semantic ./internal/observability`
  passes.
- No production behavior changes except new exported fields/types.
- All new fields have safe zero-value behavior.

### Agent A: Simple Prompt Fast Path And Request Shape

Goal:

Make simple prompts cheap after semantic bypass by avoiding unnecessary tool
schemas, huge output reserves, and large context windows.

Primary files:

- `internal/retrievalroute/route.go`
- `internal/retrievalroute/route_test.go`
- `internal/agent/input.go`
- `internal/agent/stream.go`
- `internal/agent/context_policy.go`
- `internal/agent/agent_test.go`
- `internal/tui/app.go`
- `internal/server/session.go`
- `internal/cli/print.go`

Do not edit:

- semantic cache/search files;
- hook dispatch files;
- picker files.

Tasks:

- Extend `retrievalroute.Decide` so each route returns request shape metadata:
  - general prompt -> `RequestProfile=chat_only`, `ToolMode=none`;
  - local command/listing -> no agent model request change;
  - explicit file status/read -> `RequestProfile=read_only`, `ToolMode=read_only`;
  - broad fix/debug/refactor -> `RequestProfile=default_agent`, `ToolMode=default`;
  - coordinator mode keeps coordinator-specific tools.
- Pass the route decision into `agent.Input` from TUI and server runs.
- For CLI `--print`, derive the same request profile from prompt intent where
  possible.
- Update `executeOneTurn` so `ToolMode=none` sends no `Tools` field.
- Add a read-only tool definition path:
  - reuse existing read-only registry if possible;
  - keep tool names stable;
  - do not include write/bash tools in read-only mode.
- Cache converted tool definitions by registry/tool mode where safe.
- Add a simple-output budget cap:
  - `chat_only` defaults to 1024-2048 `num_predict`;
  - `read_only` defaults to 4096 unless explicit long output is detected;
  - `default_agent` keeps normal agent budget.
- Keep length retry behavior:
  - if the model returns `done_reason=length`, retry with `LengthRetryTokens`;
  - do not disable existing continuation recovery.
- Update `effectiveNumCtx` tests so small prompts with `chat_only` request a
  small context tier.

Tests:

- General prompt route:
  - prompt `how is the weather` returns semantic skip and `ToolMode=none`.
- Agent request:
  - a fake LLM client captures request and verifies `Tools` is empty/nil for
    `chat_only`.
- Read-only prompt:
  - explicit file status prompt includes read-only tools only.
- Broad agent prompt:
  - `fix the authentication bug` keeps default tools.
- Context policy:
  - `chat_only` does not reserve 65K output tokens.

Acceptance criteria:

- `go test ./internal/retrievalroute ./internal/agent ./internal/tui ./internal/server ./internal/cli`
  passes.
- Captured simple prompt requests show:
  - no semantic retrieval;
  - no tool schemas;
  - small `num_predict`;
  - small/adaptive `num_ctx`.

### Agent B: Embedding Option Propagation And Embedding Metrics

Goal:

Make sure semantic retrieval always preserves embedding dimensions and
keep-alive through runtime and observability wrappers.

Primary files:

- `internal/llm/router.go`
- `internal/llm/router_test.go`
- `internal/observability/llm.go`
- `internal/observability/llm_test.go`
- `internal/semantic/embedder.go`
- `internal/semantic/hypotheses_test.go`

Do not edit:

- `internal/semantic/service.go` unless only adding tests that prove the
  embedder receives options;
- TUI/server request path files.

Tasks:

- Implement `EmbedWithOptions` on `llm.RuntimeClient`.
- If the current client implements `llm.EmbedderWithOptions`, forward options.
- If the current client does not implement options, fall back to `Embed`.
- Implement `EmbedWithOptions` on `observedLLMClient`.
- Record embedding metrics separately from chat metrics:
  - `EmbeddingCalls`;
  - `EmbeddingErrors`;
  - average or total embedding duration.
- Keep existing `Embed` behavior by calling `EmbedWithOptions(ctx, model, input, nil)`
  where appropriate.
- Add tests with fake clients proving:
  - dimensions pointer arrives unchanged;
  - truncate pointer arrives unchanged;
  - keep-alive string arrives unchanged;
  - fallback client still works without options;
  - observability wrapper still records errors.

Tests:

- `go test ./internal/llm ./internal/observability ./internal/semantic`
- Add focused regression test for qwen3 embedding dimensions:
  - requested dimensions `1024`;
  - fake underlying client would return `4096` if options are absent;
  - test must fail before the fix and pass after the fix.

Acceptance criteria:

- No route through REPL/server semantic services drops `EmbedOptions`.
- Embedding calls no longer inflate generic chat metrics.

### Agent C: Semantic Retrieval Cache And Substage Observability

Goal:

Remove repeated per-prompt index loads and expose enough timing data to know
where semantic latency is coming from.

Primary files:

- `internal/semantic/contracts.go`
- `internal/semantic/service.go`
- `internal/semantic/store.go`
- `internal/semantic/config.go`
- `internal/semantic/service_test.go`
- `internal/semantic/retrieve_bench_test.go`
- `internal/observability/metrics.go`

Integration files after core package is complete:

- `internal/tui/app.go`
- `internal/server/session.go`

Do not edit:

- search ranking logic in `internal/semantic/search.go`, except to accept
  cached vector data if absolutely required.

Tasks:

- Add an internal semantic index cache:
  - key by canonical root, workspace ID, schema version, model, dimensions,
    and `UpdatedAt`;
  - value contains manifest, records, vectors, and load timestamp.
- Cache API:
  - `getLoadedIndex(ctx, root)`;
  - `invalidateRoot(root)`;
  - `invalidateWorkspaceID(workspaceID)`.
- Invalidate cache on:
  - `Build`;
  - `Refresh`;
  - `Clear`;
  - manifest mismatch.
- Add config fields:
  - `semantic_index.cache_enabled` default true;
  - `semantic_index.cache_max_workspaces` default 2;
  - optional `semantic_index.cache_max_bytes` if simple to estimate.
- Add substage timing inside `Retrieve`:
  - manifest load;
  - records load;
  - vectors load;
  - cache lookup;
  - query embed;
  - scoring;
  - render/stale filtering;
  - total.
- Emit observer events through `RetrieveRequest.Observer`.
- Add TUI/server integration only after package tests pass:
  - TUI shows compact semantic timing status;
  - server SSE sends separate semantic timing events;
  - preserve legacy `semantic_retrieval` event.

Tests:

- Cache hit/miss unit test:
  - first retrieval loads records/vectors;
  - second retrieval reuses cached data;
  - refresh invalidates.
- Deadline test:
  - context deadline still stops retrieval.
- Observer test:
  - all expected stages emit non-negative durations.
- Benchmark:
  - cold vs warm `LocalService.Retrieve`.

Acceptance criteria:

- Warm retrieval avoids `LoadRecords` and `LoadVectors`.
- Existing fallback behavior stays unchanged.
- TUI/server can display semantic substage timings without breaking old events.

### Agent D: Semantic Light Candidate Narrowing And Search Scaling

Goal:

Make semantic light mode actually cheaper and improve search scaling for large
indexes.

Primary files:

- `internal/semantic/search.go`
- `internal/semantic/search_test.go`
- `internal/semantic/vectors.go`
- `internal/semantic/vectors_test.go`
- `internal/semantic/retrieve_bench_test.go`

Do not edit:

- `internal/semantic/service.go` while Agent C owns cache work, unless Agent C
  has landed and the file has been re-read.

Tasks:

- Add a search options struct used by `scoreRecords` or a replacement function:
  - route profile;
  - explicit paths;
  - current-turn paths;
  - current-turn dirs;
  - max candidate count;
  - allow full fallback.
- Implement candidate selection for light mode:
  - exact current paths;
  - records under current dirs;
  - sibling files;
  - lexical path/name matches;
  - explicit paths always included.
- If candidate count is below a minimum, fall back to full scoring.
- Replace full sort with a bounded top-K heap where practical.
- Keep deterministic tie-breaks:
  - score desc;
  - path asc;
  - start line asc;
  - record ID asc.
- Add optional flat vector representation behind a small internal type:
  - validate dimensions once;
  - expose `VectorAt(i)` or `DotAt(i, query)`;
  - keep old `VectorSet` API stable until service/cache integration lands.
- Add parallel scoring only above a threshold:
  - example threshold: more than 10,000 records or more than 10 million float
    operations;
  - worker count bounded by `runtime.NumCPU()`;
  - local top-K heaps merged deterministically.

Tests:

- Light mode only scores candidate records when enough candidates exist.
- Light mode falls back to full scoring when candidates are insufficient.
- Explicit paths are never lost.
- Results are deterministic across repeated runs.
- Parallel and serial scoring return identical ordered results.

Benchmarks:

- 10k records at 1024 dimensions.
- 50k records at 1024 dimensions.
- 100k records at 1024 dimensions, if local runtime is reasonable.
- Compare:
  - existing full sort;
  - top-K heap;
  - serial dot;
  - parallel dot threshold.

Acceptance criteria:

- `semantic_light` does less work on explicit related-code prompts.
- Full semantic search behavior remains compatible.
- Benchmark evidence shows either improvement or a clear reason to keep a
  simpler implementation.

### Agent E: Parallel Semantic Index Build And Context Packing

Goal:

Use bounded Go concurrency for independent file work while preserving stable
output and existing safety behavior.

Primary files:

- `internal/semantic/scanner.go`
- `internal/semantic/scanner_test.go`
- `internal/contextpack/current_turn.go`
- `internal/contextpack/range_pipeline.go`
- `internal/contextpack/current_turn_test.go`
- `internal/mentions/expand.go`
- `internal/mentions/expand_test.go`

Do not edit:

- `internal/semantic/service.go`;
- retrieval ranking files.

Tasks for semantic indexing:

- Keep `dirwalk.Walk` as the discovery source.
- Convert the per-file processing loop in `ScanWorkspace` into a bounded worker
  pool.
- Worker input includes original discovery index and entry metadata.
- Worker output includes:
  - records;
  - skipped file info;
  - files seen/indexed/skipped deltas;
  - any read/filter error classification.
- Merge outputs in original discovery order.
- Sort final records exactly as current code does.
- Use context cancellation in workers and dispatcher.
- Keep progress events stable and monotonic.

Tasks for context packing:

- Split mention packing into phases:
  - resolve/stat mentions;
  - estimate or read metadata;
  - allocate budget in stable order;
  - read selected bodies/ranges in parallel;
  - render in stable order.
- For small numbers of files, keep serial path or use a low overhead threshold.
- For directory evidence:
  - walk/select candidates deterministically;
  - read selected candidate files in parallel;
  - preserve existing binary/UTF-8 filtering.
- Keep exact error behavior for invalid mentions.

Tests:

- Existing scan output remains deterministic.
- Parallel scan output equals serial scan fixture.
- Context cancellation stops workers.
- Race test passes for semantic and contextpack packages.
- Multi-file mention prompt renders files in stable order.
- Budget behavior remains stable.

Benchmarks:

- index scan over synthetic 1k and 10k file fixtures;
- multi-file context packing for 10, 50, and 200 files.

Acceptance criteria:

- No nondeterministic prompt output.
- No data races under `go test -race`.
- Index build/refresh progress counts remain correct.

### Agent F: TUI Rendering And Picker Runtime Cost

Goal:

Reduce avoidable UI CPU work during streaming, typing, and picker suggestions.

Primary files:

- `internal/tui/app.go`
- `internal/tui/render_benchmark_test.go`
- `internal/tui/fileindex/index.go`
- `internal/tui/fileindex/index_test.go`
- `internal/tui/picker/file_provider.go`
- `internal/tui/picker/score.go`
- `internal/tui/picker/provider_test.go`

Conflict rule:

- Wait until Agent A and Agent C have finished their edits to
  `internal/tui/app.go`, or restrict early work to picker/fileindex files only.

Tasks for rendering:

- Add transcript render versioning:
  - increment on transcript append/update;
  - increment on width/style/collapse changes;
  - do not increment for input-only edits.
- Cache rendered transcript string by version and width.
- Avoid calling `viewport.SetContent` in `View()` when transcript content is
  unchanged.
- Keep active streaming item cheap as plain text.
- Add tests for:
  - cache invalidation on transcript mutation;
  - no invalidation on input-only updates;
  - width change invalidates cache.

Tasks for picker:

- Extend `fileindex.Entry` with precomputed metadata:
  - lowercase relative path;
  - lowercase basename;
  - ASCII/rune character bitmask for quick rejection.
- Update `FileProvider.Suggest` to:
  - reject impossible candidates before fuzzy scoring;
  - use precomputed lowercase strings;
  - keep result order deterministic;
  - use bounded top-K instead of sorting every candidate if benchmark supports it.
- Add query refinement cache only if it stays simple and deterministic.

Benchmarks:

- repeated `View()` with no transcript changes;
- streaming `AssistantTextDelta` over 10k chunks;
- picker suggestions over 50k entries;
- picker suggestions over 50k entries with query extension.

Acceptance criteria:

- No visual behavior regression in existing TUI tests.
- Render benchmarks do not regress.
- Picker benchmark shows fewer allocations and lower time per suggestion.

### Agent G: Startup Parallelism And Hook Dispatch

Goal:

Reduce startup latency and prevent serial hook stacks from hiding behind
`Waiting for model...`.

Primary files:

- `internal/cli/repl.go`
- `internal/cli/repl_test.go`
- `internal/server/server.go`
- `internal/server/server_test.go`
- `internal/hooks/dispatch.go`
- `internal/hooks/runner.go`
- `internal/hooks/dispatch_test.go`
- `internal/hooks/runner_test.go`

Tasks for startup:

- Identify independent startup operations after config load:
  - live `ShowModel`;
  - hook snapshot load;
  - skill loader creation;
  - MCP config load/start;
  - memory scope setup;
  - optional semantic status warm read.
- Use `errgroup.WithContext` or explicit goroutines with bounded timeouts.
- Preserve startup warning order by collecting results and rendering warnings in
  a stable sequence.
- Add stage timings for:
  - config load;
  - show model;
  - registry build;
  - skills load;
  - MCP start;
  - hooks load;
  - semantic warm status.
- Do not block TUI startup on optional semantic warm status.

Tasks for hooks:

- Add `parallel_safe` or equivalent opt-in config field to hook definition.
- Serial behavior remains default.
- For matching hooks marked parallel safe:
  - run in bounded `errgroup`;
  - preserve aggregate result order by original hook order;
  - retain deny/ask precedence;
  - keep timeout behavior.
- Add stage timings for total hook dispatch and slow individual hooks.

Tests:

- Startup warnings remain deterministic.
- Failed optional warmup does not fail REPL startup.
- Serial hooks preserve old behavior.
- Parallel-safe hooks run concurrently and aggregate deterministically.
- Deny/ask still wins over allow.

Acceptance criteria:

- Startup path uses concurrency only where outputs are independent.
- Hooks are never made parallel unless explicitly safe.

### Agent H: Integration, Benchmarks, And Documentation Evidence

Goal:

Validate that all lanes improve response time without breaking agent behavior.

Primary files:

- `docs/GO-RESPONSE-TIME-PERFORMANCE-REFACTOR-REPORT.md`
- `docs/WAITING-FOR-MODEL-LATENCY-REPORT.md`
- `docs/PHASE-LOG.md`
- optional scripts under `tools/` if the repo already uses them for benchmarks.

Tasks:

- After each lane lands, run that lane's tests and record results.
- After all lanes land, run:
  - focused package tests;
  - race tests for concurrent packages;
  - semantic benchmarks;
  - TUI render/picker benchmarks;
  - agent concurrency benchmarks.
- Add a before/after latency table with:
  - prompt;
  - retrieval route;
  - semantic cache hit/miss;
  - semantic total;
  - `num_ctx`;
  - `num_predict`;
  - tool count;
  - `llm_request_open`;
  - first token;
  - total response time.
- Verify the simple prompt path manually:
  - `how is the weather`;
  - `hello`;
  - no semantic retrieval;
  - no tool schemas;
  - small `num_ctx`;
  - small `num_predict`.
- Verify the broad code prompt path manually:
  - `fix the authentication bug`;
  - semantic full still runs;
  - tool mode remains default agent;
  - context is bounded.
- Verify explicit-file related prompt:
  - `@internal/tui/app.go find related callers`;
  - semantic light runs;
  - candidate narrowing is visible in metrics.

Acceptance criteria:

- Report has final benchmark evidence.
- No lane is marked complete without test commands and outcomes.
- Any regression or deferred item is documented with owner and reason.

### Parallel Execution Matrix

Wave 0:

- Agent 0 lands shared contracts.

Wave 1, can run in parallel after Wave 0:

- Agent B: embedding option propagation.
- Agent D: search narrowing in package-local code.
- Agent E: parallel scanner/contextpack work.
- Agent F: picker/fileindex work only.
- Agent G: hook dispatch tests and startup analysis.

Wave 2, starts after conflicting files are clear:

- Agent A: TUI/server/agent request-shape integration.
- Agent C: semantic cache and TUI/server semantic timing integration.
- Agent F: TUI render cache work in `app.go`.
- Agent G: REPL startup parallelism in `repl.go`.

Wave 3:

- Agent H runs integration tests, benchmarks, manual latency checks, and final
  documentation updates.

### Merge Order Recommendation

Use this order to reduce conflict and make regressions easier to isolate:

1. Agent B embedding option propagation.
2. Agent A request profile and no-tools fast path.
3. Agent C semantic substage observer without cache.
4. Agent C semantic cache.
5. Agent D semantic light narrowing and top-K improvements.
6. Agent E parallel semantic scanner.
7. Agent E parallel context packing.
8. Agent F picker prefilter.
9. Agent F TUI render cache.
10. Agent G startup parallelism.
11. Agent G parallel-safe hooks.
12. Agent H validation/docs.

### Definition Of Done For The Whole Phase

The phase is complete only when all of these are true:

- Simple/general prompts bypass embeddings and tool schemas.
- Embedding dimensions and keep-alive survive all wrappers.
- Semantic retrieval reports substage timings.
- Warm semantic retrieval uses cached records/vectors.
- Semantic light mode narrows candidates or reports why it fell back.
- Multi-file index/context work uses bounded concurrency without races.
- TUI/picker benchmarks are not worse than baseline.
- Startup and hook timing are visible.
- `go test` focused packages pass.
- Race tests pass for packages with new concurrency.
- Benchmark evidence is recorded.

## Validation Plan

### Required Tests

```bash
go test ./internal/retrievalroute ./internal/agent ./internal/tui ./internal/server ./internal/semantic ./internal/llm ./internal/observability
go test -race ./internal/agent ./internal/semantic ./internal/contextpack ./internal/tui ./internal/hooks
```

### Required Benchmarks

```bash
go test ./internal/semantic -run '^$' -bench 'Retrieve|ScoreRecords|LoadRecords|LoadVectors' -benchmem
go test ./internal/tui -run '^$' -bench 'View|RenderTranscript|Picker|Suggest' -benchmem
go test ./internal/analysis -run '^$' -bench 'RetrieveTopFiles' -benchmem
go test ./internal/agent -run '^$' -bench 'Concurrent|Serial|Partition' -benchmem
```

### Manual Latency Checks

Use the same model, same repo, and same terminal size for before/after runs.

Prompts:

- `how is the weather`
- `hello`
- `@docs/WEB-UI-UX-PRODUCT-PLAN.md can you access this document`
- `@internal/tui/app.go find related callers and utilities`
- `fix the authentication bug`
- `review the codebase for slow response time`

Capture:

- retrieval route;
- semantic cache hit/miss;
- semantic substage timings;
- prompt bytes;
- tool schema bytes;
- requested `num_ctx`;
- requested `num_predict`;
- `llm_request_open`;
- first token;
- first visible render;
- final prompt eval tokens.

## Implementation Completion Status (2026-05-28)

Status: implemented in code and validated with focused tests/race tests.

Completed lanes:

1. `P0` embedding option propagation through wrappers (`EmbedWithOptions` in
   runtime and observability wrappers) with regression tests.
2. `P0` simple/general prompt fast path with route-driven request profile and
   no-tool schema mode for chat-only prompts.
3. `P0` output budget policy updated to 8K default with existing length-retry
   continuation preserved (`65536` retry budget).
4. `P1` semantic retrieval substage observability added (manifest/records/
   vectors/embed/score/render/total).
5. `P1` semantic loaded-index cache added with invalidation on build/refresh/
   clear paths.
6. `P1` semantic light candidate narrowing implemented and benchmarked.
7. `P1` bounded parallelism for semantic scanner and context-pack file reads
   with deterministic merge/order.
8. `P2` picker prefilter optimization and TUI transcript render cache path to
   avoid redundant viewport content resets.
9. `P2` startup parallelism for independent initialization tasks.
10. `P2` optional hook-level `parallel_safe` dispatch with deterministic
    aggregation and default-serial behavior.

Focused validation run results:

- `go test ./internal/contextpack ./internal/tui ./internal/hooks ./internal/cli ./internal/bootstrap ./internal/agent ./internal/semantic ./internal/retrievalroute ./internal/server` -> pass
- `go test -race ./internal/contextpack ./internal/hooks ./internal/semantic ./internal/agent ./internal/tui` -> pass

Benchmark evidence captured:

- Semantic package (`go test ./internal/semantic -run '^$' -bench 'Retrieve|ScoreRecords|LoadRecords|LoadVectors' -benchmem`)
  - `BenchmarkScoreRecordsScaling/large_4800`: `~2.55ms/op`
  - `BenchmarkScoreRecordsLightModeScaling/large_4800`: `~0.92ms/op`
  - `BenchmarkLocalServiceRetrieveScaling/large_4800`: `~13.59ms/op`
- TUI package (`go test ./internal/tui -run '^$' -bench 'View|RenderTranscript|Picker|Suggest' -benchmem`)
  - `BenchmarkRenderTranscript_1000AssistantDeltas`: `~6.43us/op`
  - `BenchmarkRenderTranscript_InterleavedToolEvents`: `~9.09us/op`
  - `BenchmarkView1000Items`: `~109us/op`

Known environment caveat:

- Full-suite `go test ./...` in this sandbox can fail in
  `internal/llm/ollama` tests due restricted local listener bind
  (`listen tcp6 [::1]:0: bind: operation not permitted`). This is an execution
  environment restriction, not a logic failure in this phase.

## Risks

- Over-aggressive fast paths can remove needed tools from prompts that look
  simple but actually require action.
- In-memory vector cache can increase memory usage on large repositories.
- Parallel scanning can make ordering nondeterministic if results are merged
  carelessly.
- Parallel hooks can change behavior unless gated by explicit hook config.
- Smaller default output budgets can increase retry count for genuinely long
  reports unless long-output detection is good.
- Parallel scoring can be slower for small indexes due to goroutine overhead.

## Recommended Implementation Order

1. Preserve embedding options through wrappers.
2. Add no-tools/simple prompt request profile.
3. Change output budget policy to 8K default plus retry.
4. Add semantic substage observability.
5. Add semantic records/vectors cache.
6. Make semantic light mode narrow candidates.
7. Add worker-pool indexing/context packing.
8. Add picker/render cache refinements.
9. Add startup parallelism.
10. Add optional parallel-safe hooks.

## Bottom Line

The response-time issue is mostly a request-shape and repeated-work problem, not
a lack of raw Go speed. Go gives the project the right tools here: bounded
goroutines for independent file work, `sync`/`atomic` caches for read-heavy
indexes, deterministic worker-pool merges, and cheap instrumentation.

The first phase should focus on making simple prompts truly simple and making
semantic retrieval measurable. After that, cache and narrow semantic search.
Those changes should improve the latency users feel most often while preserving
the embedding feature for prompts that actually need workspace discovery.
