# Waiting for Model Latency Report

Date: 2026-05-27

## Implementation Source Of Truth

For the next implementation phase, the authoritative sections are:

1. `Final Decisions For Next Phase`
2. `Parallel Implementation Task Plan`

All earlier/later hypothesis, validation, brainstorm, and latency-only sections
are retained as historical evidence. They explain why the decisions were made,
but they must not override the owner-confirmed implementation lock.

## Summary

The TUI status `Waiting for model...` is currently too broad. It does not only
mean "Ollama is generating." In the current implementation it means the
application has marked a run active, but the TUI has not yet received an
assistant text delta, assistant thinking delta, or tool event.

That makes several different bottlenecks look identical to the user:

- prompt context preparation that completed just before the run was marked active;
- hook execution before the inner agent starts;
- memory recall before the inner agent starts;
- Ollama model cold start;
- Ollama request setup and response-header wait;
- prompt prefill over a large context;
- first-token latency from the model.

The most likely immediate culprit in the current codebase is the combination of:

1. a very large default/requested chat context window;
2. no chat `keep_alive` being sent to Ollama;
3. stream watchdog coverage starting only after the Ollama `Chat` call returns.

## Observed Real Case

This report is based on a real user-observed interaction, not only code review.
The application entered `Waiting for model...` and stayed there for a long time
before answering the question.

That means the issue should be treated as a confirmed latency/observability
problem in the main interaction path. The current UI did not give enough
information to tell whether the delay was caused by context preparation, memory,
hooks, Ollama model load, request setup, prompt prefill, or first-token latency.

The next implementation stage should therefore prioritize instrumentation before
or alongside performance tuning. Without stage-level timings, changes such as
reducing context, adding keep-alive, or changing retrieval limits can improve
the symptom but still leave future stalls hard to diagnose.

### Captured Interaction

Prompt:

```text
@docs/WEB-UI-UX-PRODUCT-PLAN.md can you acess this document
```

System notices and timing evidence:

```text
[Context packed: files=1 raw=14130 chars excerpted=1 omitted=1 budget=113792 tokens]
[Turn 1]
[stage summary] slowest: first_visible_render 5m17.79s, semantic_retrieve 1.822s
[slow stage] semantic_retrieve took 1.799s
[Semantic retrieval: records=40 files=12 stale_dropped=0 context=32021 bytes]
```

Observed assistant behavior:

- the answer eventually succeeded;
- the model confirmed it could access the document;
- the application spent a long time in `Waiting for model...` before visible
  output appeared;
- semantic retrieval completed in about 1.8 seconds;
- the slowest measured stage was `first_visible_render` at 5 minutes and 17.79
  seconds.

### Interpretation Of The Captured Case

This case strongly suggests the primary delay was not semantic retrieval.
Semantic retrieval took approximately 1.8 seconds, while the visible-response
delay was more than 5 minutes.

The stage `first_visible_render` is recorded from `runStartedAt` until the first
visible render after a stream event. The implementation records it in
`recordFirstVisibleRenderIfNeeded`.

Relevant code:

- `internal/tui/app.go`

That means the long delay likely happened after `ActiveRun` was set and before
the first useful visible model output. The current instrumentation is not
granular enough to determine whether the 5-minute delay was caused by:

- memory recall before the inner agent emitted events;
- hooks before the inner agent emitted events;
- the call to `client.Chat` waiting for Ollama response headers;
- Ollama loading the model;
- huge `num_ctx` allocation;
- prompt prefill over the packed prompt plus semantic context;
- first-token latency after the stream opened.

However, this observed case reduces the likelihood that semantic retrieval is
the main culprit for the stall.

### Concrete Concern From This Case

The prompt directly mentioned one file, but semantic retrieval still appended:

```text
records=40 files=12 context=32021 bytes
```

That may be valid for semantic augmentation, but for a simple "can you access
this document" prompt it may be unnecessary extra context. The implementation
should consider intent-aware semantic retrieval limits or bypass rules for
simple explicit-file access/status prompts.

Possible rule:

- if the prompt is a simple explicit-file availability/status question, use only
  the explicitly mentioned file context and skip semantic retrieval;
- if semantic retrieval still runs, cap it more aggressively for explicit-file
  status/access prompts.

This would not explain the full 5-minute delay by itself, but it can reduce
prompt size and prompt prefill time.

## Embedding/Semantic Index Regression Hypotheses

The user observed that responses became slower after the embedding logic was
added and the project was indexed with `/index build`.

That observation is consistent with the current code path. Before an index
exists, semantic retrieval usually falls back quickly with `ErrIndexMissing`.
After `/index build`, the index exists and the application performs semantic
retrieval on most prompts.

Relevant code:

- `internal/tui/app.go`
- `internal/server/session.go`
- `internal/semantic/service.go`

### 1. `/index build` effectively activates per-prompt retrieval work

Semantic retrieval is enabled by default in config.

Relevant code:

- `internal/config/defaults.go`
- `internal/semantic/config.go`

The default does not auto-build the index, but once the user manually runs
`/index build`, retrieval has real records/vectors to load and search.

Current TUI condition:

```go
if m.semanticService != nil &&
    m.semanticConfig.Enabled &&
    expansionReport.Intent.AttachmentPolicy != mentions.AttachListingTreeOnly {
    res, err := m.semanticService.Retrieve(...)
}
```

That means after indexing, most non-listing prompts pay the semantic retrieval
cost before the chat model starts.

Expected impact:

- every normal prompt may now perform an embedding request;
- every normal prompt may load and scan the vector index;
- every normal prompt may append additional context to the LLM prompt.

### 2. Each prompt performs an embedding request for the user query

`Retrieve` embeds the latest user query using the embedding model stored in the
index manifest:

```go
embedRes, err := s.embedder.Embed(ctx, EmbedRequest{
    Model:      manifest.Model,
    Input:      []string{strings.TrimSpace(req.Query)},
    Dimensions: manifest.Dimensions,
    KeepAlive:  cfg.KeepAlive,
})
```

Relevant code:

- `internal/semantic/service.go`

This adds a second model call before the chat model generation call.

Possible effects:

- if the embedding model is not already loaded, Ollama must load it;
- if the embedding model and chat model compete for RAM/VRAM, this can evict or
  slow the chat model;
- the TUI records this under `semantic_retrieve`, but any resulting chat model
  cold start or prompt prefill shows later as `Waiting for model...`.

### 3. Embedding keep-alive is set, chat keep-alive is not

Semantic embedding requests pass `KeepAlive` and default to `10m`.

Relevant code:

- `internal/semantic/config.go`
- `internal/semantic/service.go`
- `internal/semantic/embedder.go`

The chat request path supports `keep_alive` at the Ollama client layer, but the
agent does not currently set it for normal chat turns.

Relevant code:

- `internal/llm/ollama/ollama.go`
- `internal/agent/stream.go`

This creates an asymmetry:

- the embedding model may remain loaded for 10 minutes;
- the chat model may still cold-start or be evicted;
- on memory-constrained machines, the embedding model can occupy resources that
  the chat model needs.

This is a plausible reason the slowdown appeared only after the embedding/index
feature was introduced.

### 4. Retrieval loads records and vectors from disk on every prompt

`Retrieve` loads the manifest, records, and vectors each time:

```go
records, err := s.store.LoadRecords(ctx, manifest)
vectors, err := s.store.LoadVectors(ctx, manifest)
```

Relevant code:

- `internal/semantic/service.go`
- `internal/semantic/store.go`
- `internal/semantic/vectors.go`

The vector file is binary float32 data. With the default `1024` dimensions, the
raw vector data size is approximately:

```text
record_count * 1024 * 4 bytes
```

Examples:

```text
10,000 records  -> ~39 MB vectors
50,000 records  -> ~195 MB vectors
200,000 records -> ~781 MB vectors
```

This does not include JSONL record loading, allocations, Go slice overhead, or
CPU work for scoring.

Because there is no visible in-memory retrieval cache in the current path, a
large index can add disk I/O, allocation pressure, garbage collection pressure,
and CPU work to every prompt.

### 5. Scoring is linear over all records

`scoreRecords` computes a dot product for every record/vector pair, then sorts
all hits.

Relevant code:

- `internal/semantic/search.go`

For small indexes this is fine. For large codebases, this can become noticeable:

- `O(record_count * dimensions)` dot-product work;
- sorting all hits before diversification;
- extra lexical scoring for every record;
- more memory churn.

This cost is currently part of semantic retrieval, not model generation, but the
additional retrieved context can also slow the later chat request.

### 6. Semantic retrieval can add substantial prompt context

Default retrieval limits are:

```text
top_k_records = 40
top_k_files = 12
max_context_bytes = 262144
```

Relevant code:

- `internal/semantic/config.go`

In the captured case, the simple document-access prompt appended:

```text
records=40 files=12 context=32021 bytes
```

That is about 32KB of extra prompt text, even though the user asked whether a
single explicitly mentioned document was accessible.

Possible impact:

- larger prompt sent to the chat model;
- longer prompt prefill;
- more time before first token;
- more chance of hitting large `num_ctx` behavior;
- more irrelevant context for simple prompts.

### 7. Semantic retrieval currently runs for too many prompt types

The only explicit bypass in the TUI/server path is listing-tree-only intent.

Relevant code:

- `internal/tui/app.go`
- `internal/server/session.go`

That means semantic retrieval can run for:

- simple explicit-file access questions;
- "can you read this file?";
- file status questions;
- small follow-up questions;
- prompts where explicit mention context is already sufficient.

This can make the application feel slower after indexing even when retrieval is
not needed for the task.

### 8. `CurrentTurnPaths` is passed but not used by scoring

`RetrieveRequest` includes both `ExplicitPaths` and `CurrentTurnPaths`.

Relevant code:

- `internal/semantic/contracts.go`
- `internal/tui/app.go`
- `internal/server/session.go`

However, `Retrieve` currently passes only `req.ExplicitPaths` into
`scoreRecords`.

Relevant code:

- `internal/semantic/service.go`
- `internal/semantic/search.go`

This suggests the retrieval implementation may not fully use the context pack's
knowledge of what files were already attached for the current turn.

Possible impact:

- retrieval may add broad extra files even when current-turn context already
  contains the important file;
- explicit/current-turn intent is underweighted;
- prompt size can grow unnecessarily.

### 9. Runtime semantic config is only partially applied during retrieval

The TUI and server pass request limits into `Retrieve`:

- `MaxRecords`;
- `MaxFiles`;
- `MaxContextBytes`.

But inside `Retrieve`, the scoring config starts from `DefaultConfig()`.

Relevant code:

- `internal/semantic/service.go`

That means some configured retrieval behavior may not be honored in the search
path, such as:

- `HybridLexicalWeight`;
- `FrecencyWeight`;
- `KeepAlive`.

This can make tuning less effective than expected and can hide why retrieval is
still adding broad or costly context after config changes.

## Embedding-Related Solutions To Explore

### Option E1: Add semantic retrieval stage breakdown

Current TUI only records total `semantic_retrieve` duration.

Add detailed timings for:

- manifest lookup;
- records load;
- vectors load;
- query embedding;
- vector scoring;
- diversification;
- context rendering;
- stale/read filtering.

This should use existing semantic stages:

- `retrieve_start`;
- `retrieve_query_embed`;
- `retrieve_search`;
- `retrieve_render`;
- `retrieve_done`.

Expected impact:

- confirms whether retrieval cost is query embedding, disk load, vector search,
  rendering, or prompt prefill after retrieval.

### Option E2: Cache loaded index records/vectors in memory

Avoid loading `records.jsonl` and `vectors.f32` from disk for every prompt.

Possible implementation:

- add an index cache keyed by workspace ID and manifest updated timestamp;
- cache records and vectors after first successful load;
- invalidate on `/index build`, `/index refresh`, and `/index clear`;
- cap memory usage or make the cache optional.

Expected impact:

- reduces per-prompt disk I/O;
- reduces allocations;
- improves retrieval latency on large indexes.

Risk:

- large vector indexes can consume significant memory;
- cache must be invalidated correctly.

### Option E3: Add intent-aware semantic retrieval bypass

Skip or reduce semantic retrieval for prompts where explicit context is enough.

Candidate bypasses:

- simple `@file can you access/read this?`;
- explicit file status prompts;
- prompts with one explicit file and no broad semantic intent;
- listing/tree prompts;
- very short follow-up prompts unless retrieval is explicitly requested.

Possible behavior:

- bypass semantic retrieval entirely;
- or reduce to `top_k_records=4`, `top_k_files=1`, `max_context_bytes=8192`.

Expected impact:

- less prompt bloat;
- faster first token;
- fewer irrelevant files added to simple tasks.

### Option E4: Make semantic retrieval opt-in or adaptive

Current default enables semantic retrieval once an index exists.

Possible modes:

```text
semantic_index.mode = "off" | "explicit" | "auto"
```

Suggested behavior:

- `off`: never retrieve;
- `explicit`: retrieve only when user asks for semantic search or uses a command;
- `auto`: current behavior, but with intent-aware bypasses.

Expected impact:

- users can index without paying retrieval cost on every prompt;
- safer default for slower machines.

### Option E5: Reduce default retrieval limits

Current defaults allow up to 40 records, 12 files, and 256KB context.

Potential new defaults:

```text
top_k_records = 12
top_k_files = 4
max_context_bytes = 65536
```

For explicit-file prompts:

```text
top_k_records = 4
top_k_files = 1
max_context_bytes = 8192
```

Expected impact:

- less prompt prefill;
- less context noise;
- faster output after indexing.

Risk:

- broad tasks may need higher recall; expose `/semantic deep` or adaptive
  escalation for those cases.

### Option E6: Separate embedding model keep-alive from chat model keep-alive

Because embedding and chat models can compete for RAM/VRAM, keep-alive should be
intentional for both.

Possible behavior:

- keep embedding model alive only during index build/refresh;
- use shorter embedding keep-alive for query retrieval, such as `30s`;
- set chat keep-alive for interactive sessions, such as `5m`;
- expose both settings clearly.

Expected impact:

- avoids embedding model occupying resources while chat model cold-starts;
- improves repeated chat prompt latency.

### Option E7: Reuse or preload embedding model carefully

If query embedding is consistently slow but memory is available:

- preload embedding model when semantic retrieval is enabled;
- keep it warm during active sessions;
- show memory tradeoff in `/semantic status`.

This should be secondary to chat keep-alive because the observed pain is waiting
for final answer generation, not only the 1.8-second retrieval step.

### Option E8: Use current-turn paths in retrieval scoring

Apply `CurrentTurnPaths` during scoring and diversification.

Possible behavior:

- boost current-turn files;
- suppress extra files when explicit/current-turn files already score highly;
- limit retrieval to siblings/import-related files unless the prompt asks for
  broad search.

Expected impact:

- fewer unrelated files;
- smaller semantic context;
- more predictable retrieval.

### Option E9: Make retrieval cost visible in the status bar

While semantic retrieval runs, display:

```text
Semantic: embedding query...
Semantic: loading vectors...
Semantic: scoring 42k records...
Semantic: adding 12 files / 32KB...
```

Expected impact:

- user can tell the index is being used;
- slow retrieval no longer looks like model waiting;
- easier to compare runs with `/semantic on` and `/semantic off`.

## Model Routing Lessons From `embedding-and-model-routing.md`

The `embedding-and-model-routing.md` document describes a stricter reference
architecture than the current post-Phase-28 behavior. Its central lesson is that
embeddings should not become a general prompt-answering path. The main answer
path, side queries, memory recall, session search, prompt suggestions, token
counting, UI fuzzy search, and permission/security decisions all have separate
routes. In that architecture, embeddings are absent today and would only be
added later as retrieval-only infrastructure.

Important findings from that document:

- Normal prompt answering always goes through the main query loop and streaming
  Messages API path. Embeddings never answer the user directly.
- Local slash commands can return `shouldQuery=false`; when that happens no
  answer model should be called.
- Side queries are normal lightweight model calls used for classification,
  ranking, naming, summarization, validation, and small helper tasks.
- Some "semantic" features are implemented without vectors: they prefilter
  candidate text locally and ask a small model to rank or select candidates.
- Memory recall is gated, starts as a background prefetch, and does not block the
  first model call. If the prefetch is not ready by the injection point, the main
  query continues without it.
- Session search uses direct substring matching first, caps the model-rerank
  candidate set, and only then asks a small fast model to rank results.
- Prompt suggestions are heavily gated and suppressed during expensive or
  blocked states. They use a forked agent/model path, not embeddings.
- File and folder context is loaded from explicit `@` mentions, attachments,
  read results, grep output, and memory attachments, not vector lookup.
- UI fuzzy search is local fuzzy matching, not vector search.
- Permission checks, sandbox checks, and security decisions are not based on
  vector similarity.
- If embeddings are added later, they should be used only when the input is a
  large mostly static corpus, the task is candidate retrieval, exact search is
  too brittle, a model side-query over all candidates is too expensive, and the
  final answer still goes through the main model with explicit retrieved text.

The current application should adapt those lessons rather than treating a built
semantic index as permission to run embedding retrieval for every non-listing
prompt.

### Proposed Solution R1: Add A Shared Retrieval Router

Add a single retrieval-routing layer before any semantic embedding call. The
router should be used by TUI, REPL, server sessions, and any future web/remote
entry point so retrieval behavior does not drift by surface.

The router should decide:

```text
skip_all_retrieval
use_explicit_context_only
use_local_search_only
use_small_model_rerank
use_semantic_light
use_semantic_full
```

Suggested input fields:

```text
raw_user_prompt
normalized_user_prompt
slash_or_local_command_result / should_query
mention_expansion_report
attachment_policy
current_turn_paths
current_turn_dirs
attached_file_count
attached_context_bytes
latest_tool_results
index_exists
index_freshness_summary
semantic_config_mode
prompt_intent
workspace_size_hint
model_memory_pressure_hint, if available
```

Suggested output fields:

```text
action
reason
semantic_limits
deadline
allow_side_query
allow_embedding
allow_prompt_growth_bytes
current_path_weighting_mode
telemetry_fields
user_visible_status_label
```

Example decisions:

| Input | Router action | Reason |
| --- | --- | --- |
| Slash command that returns locally | `skip_all_retrieval` | `shouldQuery=false`; no model path needed. |
| `@file can you access this?` | `use_explicit_context_only` | Exact file context is already attached. |
| `summarize @file` | `use_explicit_context_only` | The task is scoped to the explicit file. |
| `list files in @docs/` | `use_local_search_only` | Deterministic filesystem listing is the correct tool. |
| `search sessions for deploy bug` | `use_small_model_rerank` | Local prefilter plus small-model ranking is enough for bounded candidates. |
| `fix the authentication bug` | `use_semantic_full` | Broad codebase discovery is needed. |
| `@auth.go find related callers` | `use_semantic_light` | Expand from exact context, but bound the search around current paths. |

Why this helps:

- prevents accidental embedding calls for simple prompts;
- gives tests one place to assert policy;
- makes route decisions explainable in logs and UI;
- prevents TUI/server/web behavior from diverging;
- lets future routing add side-query rerank without touching every prompt path.

Implementation notes:

- Put the router in a shared package, for example `internal/semantic/router` or
  `internal/context/retrievalroute`.
- Keep the router pure and deterministic. It should not perform IO, read the
  index, or call a model.
- Model/index clients should receive a router decision, not re-derive intent.
- Emit a trace event for every prompt:

  ```text
  retrieval_route_decided action=use_explicit_context_only reason=skip_explicit_context
  ```

- The semantic service should refuse to run if the route says
  `allow_embedding=false`, except in explicit test hooks.

### Proposed Solution R2: Use A Hybrid Retrieval Ladder

The routing document shows that many semantic-looking features can be solved
without vectors by using deterministic prefilters and small model selectors.
Adopt a ladder where expensive semantic vector retrieval is not the first step.

Recommended ladder:

1. Explicit context:
   - `@file`, `@dir`, attached files, already-read tool results.
   - No embedding call.
   - No side query unless the user asks for expansion.

2. Deterministic local search:
   - path lookup;
   - filename/folder matching;
   - symbol names;
   - import/dependency neighbors;
   - ripgrep-style lexical search;
   - recent/frecency signals.

3. Small candidate side-query rerank:
   - use when the candidate set is bounded, for example fewer than 100 files or
     summaries;
   - ask a small/fast model to select or rank candidate files;
   - useful for negation and contextual instructions that embeddings handle
     poorly;
   - do not include raw full files, only compact candidate metadata/snippets.

4. Semantic embedding retrieval:
   - use when the corpus is large enough that a side-query over all candidates is
     too expensive;
   - use for broad workspace discovery, repeated docs/catalog retrieval, and
     vague bug/fix/refactor prompts with no exact files.

5. Main LLM generation:
   - always receive explicit retrieved text, paths, line ranges, and freshness
     metadata;
   - the embedding model never answers directly and never decides edits.

This ladder avoids the current failure mode where `/index build` turns one
available retrieval mechanism into the default path for almost every prompt.

### Proposed Solution R3: Define Strict No-Embedding Zones

The routing document is explicit that embeddings should not participate in many
application decisions. Add hard no-embedding zones and enforce them with tests.

Embeddings must not be used for:

- final answer generation;
- tool-use decisions;
- permission allow/deny decisions;
- sandbox/security policy;
- token counting;
- local slash-command output;
- UI fuzzy search;
- exact `@file` access, read, status, or summarization prompts;
- deterministic file listings and path lookups;
- memory recall/user preference recall;
- small candidate sets where a side query is cheaper and easier to audit;
- fresh working-tree state unless freshness and invalidation are proven.

This does not mean the workspace index is useless. It means the index is one
retrieval candidate provider, not the central model router.

Suggested tests:

- `@file can you access this?` does not call the embedder.
- `summarize @file` does not call the embedder.
- `list files in @dir` does not call the embedder.
- local slash commands do not call the embedder or chat model.
- permission prompts do not call the embedder.
- memory recall paths do not call the workspace semantic service.
- broad discovery prompts still call the embedder exactly once.

### Proposed Solution R4: Add Side-Query Rerank For Bounded Candidates

The routing document uses normal model side queries for ranking memory manifests
and session search candidates. This is a useful middle ground between lexical
search and embedding retrieval.

Add an optional bounded rerank path:

1. collect candidates with deterministic methods:
   - current-turn paths;
   - explicitly mentioned directories;
   - filenames and path terms;
   - symbol names;
   - imports;
   - recent files;
   - lexical grep hits;
2. cap to a configured limit, such as `100` candidates;
3. send compact metadata to a small/fast model:

   ```text
   path
   kind
   symbols
   short excerpt
   match reason
   freshness
   ```

4. ask for JSON indices or paths only;
5. read the selected current snippets from disk;
6. inject those snippets as explicit context for the main model.

Use this path when:

- the candidate set is small enough that a side query is cheaper than embedding;
- the prompt includes negation or nuanced constraints;
- the current index is stale;
- exact context exists but the user asks for nearby related files.

Avoid this path when:

- the task is simple and exact context is already enough;
- the candidate set is huge and repeated semantic lookup is expected;
- the user has disabled auxiliary model calls.

Expected impact:

- fewer embedding calls;
- better handling of negation than vector similarity;
- more auditable selected context;
- lower prompt bloat than broad semantic top-k retrieval.

### Proposed Solution R5: Make Retrieval Prefetch Non-Blocking Where Possible

The routing document's memory recall path starts a prefetch once per user turn
and avoids blocking the first model call. The current semantic retrieval path is
more blocking: it runs before the final chat request, so any retrieval delay and
all added context affect time to first visible answer.

Adopt deadline-based retrieval behavior:

| Route | Deadline behavior |
| --- | --- |
| explicit context only | no retrieval |
| local search only | short synchronous deadline, for example `100ms` |
| small-model rerank | bounded deadline, for example `750ms-1500ms` |
| semantic light | bounded deadline, for example `750ms-1500ms` |
| semantic full | longer deadline, for example `3000ms`, or explicit `/semantic deep` |

If retrieval misses its deadline:

- continue with explicit/local context;
- emit `semantic_skipped reason=deadline`;
- optionally attach late results after a tool round or on the next turn;
- never leave the UI stuck on generic `Waiting for model...`.

This gives simple prompts fast responses while still allowing deeper retrieval
for tasks that benefit from it.

Important caveat:

- For broad code fixes, retrieval is often needed before the main model can act.
  In that case use `semantic_full` or explicit `/semantic deep`, but keep the UI
  honest with visible retrieval stages and a hard timeout.

### Proposed Solution R6: Separate Model Routing From Embedding Routing

The routing document separates the main answer model, small fast helper model,
caller-selected side-query model, and forked-agent model path. The current Go
application should similarly avoid treating `/model` and embedding model choice
as the same thing.

Recommended model routes:

```text
main_chat_model        -> answers, tool use, edits, reasoning
small_fast_model       -> labels, bounded rerank, small classifiers
side_query_model       -> memory/file/session selection when needed
embedding_model        -> vectorization only
```

Rules:

- `/model` should affect `main_chat_model`, not silently change the embedding
  model.
- `/semantic model ...` or config should change `embedding_model`.
- side-query rerank should use the small/fast route, not the embedding model.
- embedding dimension and model name in the index manifest must match query-time
  embedding settings.
- if they do not match, retrieval should skip with a clear diagnostic instead of
  entering a slow or broken path.

Suggested config shape:

```toml
[model_routing]
main_chat_model = ""
small_fast_model = ""
side_query_model = ""

[semantic_index]
mode = "auto" # off | explicit | auto
model = "qwen3-embedding:8b"
dimensions = 1024
query_deadline_ms = 1200
full_query_deadline_ms = 3000
small_candidate_rerank_limit = 100
explicit_context_max_extra_files = 1
```

This also helps diagnose the earlier dimensions mismatch:

```text
semantic embed dimensions mismatch got 4096 want 1024
```

The route layer should distinguish:

- index build model/dimensions;
- query embedding model/dimensions;
- main chat model;
- side-query model.

Those are different routes and should have different status/diagnostic output.

### Proposed Solution R7: Treat Workspace Code As Fresh State, Not Static Memory

The routing document says future embeddings should target large, mostly static
corpora and avoid frequently changing data unless indexing and invalidation are
robust. Source code in an active workspace is fresh working-tree state, so
semantic retrieval needs stronger freshness checks than a documentation catalog.

Required metadata per indexed record:

```text
workspace_id
path
record_kind
line_start
line_end
content_hash
mtime
size
source_scope
index_model
index_dimensions
index_config_hash
permissions_scope
deleted_or_stale_state
```

At retrieval render time:

- re-stat each selected file;
- drop records whose file is deleted;
- drop or refresh records whose `mtime`, size, or hash changed;
- never inject stale snippets without marking them stale;
- fall back to exact read/search for current-turn explicit paths;
- include stale-drop counts in telemetry.

This prevents a stale vector index from competing with exact file context.

### Proposed Solution R8: Make Permission And Visibility Checks Pre-Injection

The routing document requires retrieved candidates to be checked against
permissions and freshness metadata before injection. The workspace semantic index
must obey the same principle.

Minimum checks before adding semantic context:

- path is inside the allowed workspace roots;
- path is not ignored by project/user exclusion rules;
- path is still readable;
- path has not become a secret, binary, generated artifact, or denied file;
- current session/user is allowed to access it;
- retrieved text is read fresh from disk, not from old index text;
- telemetry never logs raw source text, raw prompt text, or vectors.

Permission/security decisions should be deterministic. Vector similarity should
never decide whether something is allowed.

### Proposed Solution R9: Add Fallbacks For Missing, Stale, Or Expensive Indexes

The routing document recommends exact-search fallback when an index is stale or
unavailable. This should become a user-visible behavior.

Fallback matrix:

| Condition | Behavior |
| --- | --- |
| index missing | skip semantic retrieval; use exact context/local search |
| model missing | skip semantic retrieval; show actionable diagnostic |
| dimensions mismatch | disable semantic retrieval for the turn; suggest rebuild or config fix |
| index stale | use exact read/search for explicit paths; optionally refresh in background |
| query embedding timeout | continue without semantic context |
| semantic context too large | apply route caps or ask user for `/semantic deep` |
| memory pressure | shorten embedding keep-alive or skip retrieval for simple prompts |

The critical behavior is that a broken semantic route should not block ordinary
chat, exact-file prompts, or slash commands.

### Proposed Solution R10: Add Routing Observability

The routing document distinguishes model paths clearly. The report should require
the TUI/server observability to show the same distinction.

Add one route event per user turn:

```text
retrieval_route_decided action=use_semantic_light reason=related_code_request
```

Add stage events only when the route uses them:

```text
local_search_start
local_search_done candidates=37
side_rerank_start model=small_fast candidates=37
side_rerank_done selected=6
semantic_query_embed_start model=qwen3-embedding:8b dimensions=1024
semantic_query_embed_done duration_ms=...
semantic_search_done records=12 files=4 context_bytes=...
semantic_skipped reason=skip_explicit_context
```

Status labels should map to actual work:

```text
Reading mentioned file...
Searching workspace...
Ranking candidate files...
Embedding query...
Scoring semantic index...
Preparing model request...
Waiting for model response...
```

This avoids using `Waiting for model...` as a catch-all for retrieval, prompt
packing, prefill, model load, and streaming startup.

### Proposed Solution R11: Implementation Workstreams For Multiple Agents

If this becomes the next implementation phase, split it into independent lanes:

| Lane | Ownership | Output |
| --- | --- | --- |
| Router contract | shared route package and tests | pure retrieval decision API |
| Prompt-path integration | TUI, REPL, server | all entry points consume route decisions |
| Local candidate search | path/symbol/grep/current-turn scoring | deterministic candidate provider |
| Side-query rerank | small-model selector and JSON parsing | bounded rerank path |
| Semantic caps/deadlines | semantic service and config | light/full retrieval modes |
| Freshness/security | store metadata and render-time checks | stale/unauthorized records dropped |
| Observability | TUI/server events and traces | route and stage visibility |
| Eval/tests | fixtures and regression tests | no-embedding zones and broad-search positives |

Suggested sequencing:

1. Add router types, decisions, and table-driven tests.
2. Wire the router into all prompt entry points without changing retrieval logic.
3. Enforce no-embedding zones and verify existing explicit-file regression cases.
4. Add semantic light/full caps and deadlines.
5. Add deterministic candidate provider and optional side-query rerank.
6. Add freshness/security checks before semantic context injection.
7. Add route/stage observability and TUI labels.
8. Run live before/after latency tests with `/semantic off`, `explicit`, and
   `auto`.

## Pre-Decision Recommendation After Embedding Regression Observation

This section is retained as the recommendation that existed before the
owner-confirmed lock. The active implementation order is now defined by
`Final Decisions For Next Phase` and `Parallel Implementation Task Plan`.

Because the slowdown appeared after `/index build`, treat semantic retrieval as a
first-class suspect.

Recommended next order:

1. add detailed semantic retrieval stage timings;
2. add a shared retrieval router with explicit no-embedding zones;
3. add LLM pre-stream request timings;
4. log `num_ctx`, prompt size, semantic context bytes, and retrieval record/file
   counts per run;
5. add semantic light/full modes with deadlines and route-specific caps;
6. add local-search and small-model rerank as alternatives to embeddings for
   bounded candidate sets;
7. reduce or normalize chat `num_ctx`;
8. wire chat `keep_alive`;
9. evaluate index caching after measuring records/vectors load cost.

The captured case already shows semantic retrieval itself was only about 1.8s,
but it also added 32KB of context and likely contributed to the later
first-visible-output delay. The report should therefore not blame retrieval
alone; the likely issue is retrieval plus larger prompt plus model/context
startup behavior.

## Final Decisions For Next Phase

This section turns the report into an implementation-ready decision checklist.
The key owner decisions were confirmed on 2026-05-28. Implementation agents
should treat the locked answers below as the next-phase baseline unless the owner
explicitly changes them later.

### Locked Baseline

The next phase should implement:

1. shared retrieval router across TUI, REPL, server, and future web/remote entry
   points;
2. strict no-embedding zones for exact context, listings, local commands, memory
   recall, security, and permission decisions;
3. `semantic_index.mode = "auto"` with intent-aware bypasses;
4. route actions:

   ```text
   skip_all_retrieval
   use_explicit_context_only
   use_local_search_only
   use_semantic_light
   use_semantic_full
   ```

5. route reasons:

   ```text
   skip_local_command
   skip_explicit_context
   skip_listing_intent
   skip_memory_recall
   run_related_context
   run_workspace_discovery
   skip_index_missing
   skip_index_stale
   skip_dimensions_mismatch
   skip_deadline
   ```

6. `semantic_light` for explicit-file related-code expansion;
7. `semantic_full` for broad bug/fix/refactor/discovery prompts with no exact
   files;
8. detailed route and stage observability before broader tuning;
9. no side-query rerank in the first router MVP;
10. `/semantic deep` as the explicit broad-search escape hatch;
11. dimension mismatch fallback that skips semantic retrieval for the turn and
    continues with non-semantic context;
12. retrieval caching deferred until instrumentation proves records/vectors load
    cost is a real bottleneck;
13. separate chat and embedding keep-alive settings.

### Owner-Confirmed Answers

Confirmed answers from 2026-05-28:

| Question | Locked answer |
| --- | --- |
| Should semantic retrieval default to `auto` or `explicit`? | `auto` with strict router bypasses. |
| Should the retrieval router be implemented first? | Yes. |
| Should side-query rerank be included now? | No. Defer it. |
| Are the first-pass caps acceptable? | Yes: `semantic_light = 4 records / 1 file / 8KB / 1.2s`; `semantic_full = 12 records / 4 files / 64KB / 3s`. |
| Should `/semantic deep` exist? | Yes. |
| Should dimension mismatch skip semantic retrieval and continue instead of blocking? | Yes. |
| Should retrieval caching wait until after instrumentation proves the bottleneck? | Yes. |
| Should chat and embedding keep-alive be separate? | Yes. |

### Decision Matrix With Simple Explanations

The table below keeps the questions and plain-language explanations used to lock
the defaults. These are retained so future implementation agents understand the
tradeoffs behind the decisions.

| ID | Question | Locked answer | 12-year-old explanation | Why it matters |
| --- | --- | --- | --- | --- |
| Q1 | Should semantic retrieval default to `auto` or `explicit`? | `auto`, but only through the router and no-embedding zones. | Should the app use its code map by itself when the question is big, or only when you say "use the map"? `auto` is helpful, but it needs rules so it does not open the map for easy questions. | Sets the default config and user experience after `/index build`. |
| Q2 | Should the next phase implement the router first, before more retrieval tuning? | Yes. Router first. | Before making the search engine faster, we need a traffic light that says when search is allowed at all. | Prevents agents from optimizing a path that should often be skipped. |
| Q3 | Which route actions are in the MVP? | `skip_all_retrieval`, `use_explicit_context_only`, `use_local_search_only`, `use_semantic_light`, `use_semantic_full`. Defer `use_small_model_rerank`. | Start with five doors. The sixth door, "ask a smaller model to rank choices," is useful but can wait. | Keeps the first implementation smaller while preserving room for rerank later. |
| Q4 | Should side-query rerank be included now or deferred? | Defer. Add router extension points and tests, but do not build rerank in the first pass. | Imagine sorting a small pile of papers by asking a helper. Useful, but first we need to stop grabbing the whole filing cabinet for simple questions. | Avoids adding another model path while debugging the embedding regression. |
| Q5 | What should happen for `@file can you access this?`, `summarize @file`, and `what is the status of @file`? | Use explicit context only. Do not call the embedder. | If you hand the app the exact page, it should read that page, not search the whole library. | Fixes the observed slow prompt class directly. |
| Q6 | What should happen for `list files in @dir` and path/fuzzy search? | Use local filesystem/index search only. Do not call the embedder. | If you ask "what books are on this shelf?", the app should look at the shelf labels, not guess by meaning. | Keeps listing tasks fast and deterministic. |
| Q7 | What should happen for broad prompts like `fix the authentication bug`? | Use `semantic_full`. | If you say "find the broken part" without naming a file, the app should use the big code map to look around. | Preserves the core value of embeddings. |
| Q8 | What should happen for `@auth.go find related callers/utilities`? | Use `semantic_light`, bounded and weighted toward `CurrentTurnPaths`. | If you point to one page and ask for nearby related pages, the app can look around, but it should stay close. | Prevents related-code expansion from becoming whole-repo context bloat. |
| Q9 | What are the first-pass route caps? | `semantic_light`: `top_k_records=4`, `top_k_files=1`, `max_context_bytes=8192`, `deadline_ms=1200`. `semantic_full`: `top_k_records=12`, `top_k_files=4`, `max_context_bytes=65536`, `deadline_ms=3000`. | Light mode brings a small backpack. Full mode brings a bigger backpack, but still not the whole closet. | Controls prompt growth and first-token latency. |
| Q10 | Should `/semantic deep` exist in this phase? | Yes, as an escape hatch for users who knowingly want broader retrieval. | Sometimes the user really does want the giant search. Give them a button for that instead of making every question giant. | Lets defaults stay conservative without removing power-user behavior. |
| Q11 | What should happen on dimension mismatch, such as `got 4096 want 1024`? | Skip semantic retrieval for the turn, show a clear rebuild/config diagnostic, and continue with non-semantic context. | If the map and the compass use different scales, do not get stuck. Tell the user the map needs rebuilding and keep answering what you can. | Prevents broken indexes from blocking normal prompts. |
| Q12 | What should happen when the index is missing or stale? | Missing: skip retrieval quietly with a trace/status note. Stale: drop stale records, use exact current file reads, and optionally suggest `/index refresh`. | If there is no map, keep walking with street signs. If the map is old, do not trust the old parts. | Keeps answers based on current files, not stale vectors. |
| Q13 | Should retrieval be allowed to block indefinitely? | No. Every route gets a deadline; on timeout, continue with available context and emit `skip_deadline`. | If search takes too long, stop searching and answer with what you already have instead of making the user wait forever. | Directly addresses long `Waiting for model...` stalls. |
| Q14 | Should chat and embedding keep-alive be separate settings? | Yes. Add chat keep-alive and shorten query embedding keep-alive by default. | The chat model and map model can both take memory. Keep the talking model warm; do not let the map model hog the chair. | Reduces possible model eviction/cold-start regressions. |
| Q15 | Should retrieval caching be in the first implementation wave? | Not first. Add instrumentation first, then cache if records/vectors load cost is proven meaningful. | Do not build a storage shortcut until we know storage is the slow part. | Avoids memory tradeoffs before measurement. |
| Q16 | What route/stage events are required? | Add `RetrievalRouteDecided`, `SemanticQueryEmbedStarted/Finished`, `SemanticSearchFinished`, `SemanticSkipped`, `LLMRequestStarted`, `LLMStreamOpened`, and first-visible-render timing. | The app should say what it is doing: choosing a route, searching, opening the model, or showing the first words. | Makes `Waiting for model...` debuggable. |
| Q17 | Should permission/security decisions ever use embedding similarity? | No. Never. | The map can help find pages, but it cannot decide which doors are locked. | Keeps safety decisions deterministic and auditable. |
| Q18 | What is the multi-agent split? | Router contract, prompt-path integration, semantic caps/deadlines, observability, freshness/security, tests/evals. Side-query rerank is a later lane. | Give each builder a different room so they do not all try to paint the same wall. | Reduces merge conflicts and lets work proceed in parallel. |

### Implementation Lock

Agents should treat the following as locked for the next phase:

- router first;
- `auto` default with strict bypasses;
- no embedding for exact file, listing, memory, security, permission, local
  command, or token-counting paths;
- semantic light/full modes with the caps above;
- `/semantic deep` included for deliberate broad search;
- clear skip diagnostics for missing, stale, mismatched, or timed-out indexes;
- route and stage observability required before final validation;
- side-query rerank deferred from the first implementation wave;
- retrieval caching deferred unless instrumentation proves it is needed;
- chat and embedding keep-alive configured separately.

## Parallel Implementation Task Plan

This section breaks the locked decisions into agent-ready implementation lanes.
The work is designed for multiple agents, but the router contract must land first
or be agreed as a shared patch before other lanes wire behavior into entry
points.

### Coordination Rules

1. Contract first:
   - Agent A owns the retrieval route package, route enums, decision payload, and
     table-driven policy tests.
   - Other agents may prepare code against the documented contract, but should
     avoid inventing duplicate route logic.

2. Keep route decisions pure:
   - the router must not call the LLM;
   - the router must not read the semantic index from disk;
   - the router must not call the embedder;
   - the router decides intent and limits from already-known prompt/session
     metadata.

3. One owner per conflict-heavy file:
   - `internal/tui/app.go`: Agent B only;
   - `internal/server/session.go`: Agent C only;
   - `internal/semantic/service.go`: Agent D only;
   - `internal/config/*`: Agent F only;
   - REPL/print entry points: Agent I only.

4. Agent E owns shared observability helpers and event contracts. Agent E must
   not directly edit `internal/tui/app.go` or `internal/server/session.go`
   without coordinating with Agent B or Agent C. Agent B and Agent C wire the
   shared helpers into their owned entry points.

5. Shared event contract is fixed. Internal names are used in Go code and tests;
   wire types are used for SSE/transcript/debug payloads.

   | Internal name | Wire type |
   | --- | --- |
   | `RetrievalRouteDecided` | `retrieval_route_decided` |
   | `SemanticQueryEmbedStarted` | `semantic_query_embed_started` |
   | `SemanticQueryEmbedFinished` | `semantic_query_embed_finished` |
   | `SemanticSearchFinished` | `semantic_search_finished` |
   | `SemanticSkipped` | `semantic_skipped` |
   | `LLMRequestStarted` | `llm_request_started` |
   | `LLMStreamOpened` | `llm_stream_opened` |
   | `FirstTokenReceived` | `first_token_received` |
   | `FirstVisibleRender` | `first_visible_render` |

6. Router index status inputs are supplied by the entry point, not the router:
   - TUI/server may pass cached status updated by `/index build`, `/index refresh`,
     `/index status`, startup initialization, or a cheap non-blocking status check;
   - the router must not perform status IO;
   - if index status is unknown, pass `IndexKnown=false` and let the semantic
     service return a non-blocking skip/fallback if retrieval is attempted.

7. `/semantic deep` is a one-shot command for the next prompt. It is not a
   persistent semantic mode. Persistent modes are `off`, `explicit`, and `auto`.
   The one-shot state must be passed to the router as `ForceDeep=true`, consumed
   by exactly one submitted prompt, and then cleared even when retrieval skips.

8. Side-query rerank is explicitly out of scope for this implementation wave.
   Add extension points only; do not implement the rerank model call.

9. Retrieval caching is explicitly out of scope for this implementation wave
   unless instrumentation proves records/vectors loading is the bottleneck.

### Agent A - Retrieval Router Contract And Policy

Goal:

- Create the shared router that decides whether semantic retrieval is skipped,
  light, or full.

Primary files:

- `internal/semantic/contracts.go`
- new package, recommended: `internal/retrievalroute`
- new tests, recommended: `internal/retrievalroute/route_test.go`

Tasks:

1. Define route actions:

   ```go
   type Action string

   const (
       ActionSkipAllRetrieval      Action = "skip_all_retrieval"
       ActionExplicitContextOnly   Action = "use_explicit_context_only"
       ActionLocalSearchOnly       Action = "use_local_search_only"
       ActionSemanticLight         Action = "use_semantic_light"
       ActionSemanticFull          Action = "use_semantic_full"
   )
   ```

2. Define route reasons:

   ```go
   type Reason string

   const (
       ReasonSkipLocalCommand        Reason = "skip_local_command"
       ReasonSkipExplicitContext     Reason = "skip_explicit_context"
       ReasonSkipListingIntent       Reason = "skip_listing_intent"
       ReasonSkipMemoryRecall        Reason = "skip_memory_recall"
       ReasonRunRelatedContext       Reason = "run_related_context"
       ReasonRunWorkspaceDiscovery   Reason = "run_workspace_discovery"
       ReasonSkipIndexMissing        Reason = "skip_index_missing"
       ReasonSkipIndexStale          Reason = "skip_index_stale"
       ReasonSkipDimensionsMismatch  Reason = "skip_dimensions_mismatch"
       ReasonSkipDeadline            Reason = "skip_deadline"
   )
   ```

3. Define router input:

   ```go
   type Input struct {
       RawPrompt             string
       NormalizedPrompt      string
       ShouldQuery           bool
       AttachmentPolicy      string
       CurrentTurnPaths      []string
       CurrentTurnDirs       []string
       AttachedFileCount     int
       AttachedContextBytes  int
       IndexKnown            bool
       HasIndex              bool
       IndexCompatible       bool
       SemanticEnabled       bool
       SemanticMode          string
       ForceDeep             bool
       PromptIntent          string
   }
   ```

4. Define router decision:

   ```go
   type Decision struct {
       Action               Action
       Reason               Reason
       AllowEmbedding       bool
       MaxRecords           int
       MaxFiles             int
       MaxContextBytes      int
       Deadline             time.Duration
       UseCurrentPathWeight bool
       Profile              string // "", "light", "full", or "deep"
       StatusLabel          string
   }
   ```

5. Define the router config contract used by Agent F and implement
   `Decide(input Input, cfg Config) Decision`.

   ```go
   type Config struct {
       Mode  string // "off", "explicit", or "auto"
       Light Limits
       Full  Limits
       Deep  Limits
   }

   type Limits struct {
       MaxRecords      int
       MaxFiles        int
       MaxContextBytes int
       Deadline        time.Duration
   }
   ```

   The router package should treat `AttachmentPolicy` and `PromptIntent` as
   strings to avoid import cycles with `internal/mentions`. TUI/server/context
   packing code may convert `mentions.AttachmentPolicy` and `mentions.IntentKind`
   to strings before calling the router.

6. Implement locked defaults:

   ```text
   semantic_light:
     top_k_records = 4
     top_k_files = 1
     max_context_bytes = 8192
     deadline_ms = 1200

   semantic_full:
     top_k_records = 12
     top_k_files = 4
     max_context_bytes = 65536
     deadline_ms = 3000

   semantic_deep:
     top_k_records = 40
     top_k_files = 12
     max_context_bytes = 262144
     deadline_ms = 3000
   ```

   `/semantic deep` should return `ActionSemanticFull`, `AllowEmbedding=true`,
   `Profile="deep"`, and the deep caps. It is a broader version of full semantic
   retrieval, not a new persistent action or mode.

7. Implement no-embedding zones:
   - `ShouldQuery=false`;
   - explicit file access/read/status/summarization;
   - listing/tree/path search;
   - memory recall paths;
   - permission/security paths;
   - token-counting/local command paths.

8. Add tests for:
   - `@file can you access this?` -> `use_explicit_context_only`;
   - `summarize @file` -> `use_explicit_context_only`;
   - `what is the status of @file` -> `use_explicit_context_only`;
   - `list files in @docs/` -> `use_local_search_only`;
   - local slash command with `ShouldQuery=false` -> `skip_all_retrieval`;
   - `fix the authentication bug` -> `use_semantic_full`;
   - `@auth.go find related callers` -> `use_semantic_light`;
   - `/semantic deep` followed by `fix the authentication bug` ->
     `use_semantic_full` with `Profile="deep"` and deep caps;
   - disabled semantic mode -> no embedding;
   - missing index -> skip semantic retrieval;
   - incompatible index -> skip semantic retrieval.

Acceptance:

- every route decision has action, reason, caps, deadline, and status label;
- tests cover all locked no-embedding zones;
- no semantic service, embedder, or LLM dependency appears in the router package.

### Agent B - TUI Prompt Path And Status Integration

Goal:

- Make TUI prompt submission consume router decisions and stop calling semantic
  retrieval for exact-context prompts.

Primary files:

- `internal/tui/app.go`
- `internal/tui/runstate.go`
- `internal/tui/app_test.go`
- `internal/tui/snapshot_status_test.go`
- `internal/tui/testdata/status_*.txt`

Tasks:

1. Replace the current broad TUI condition:

   ```go
   m.semanticService != nil &&
   m.semanticConfig.Enabled &&
   expansionReport.Intent.AttachmentPolicy != mentions.AttachListingTreeOnly
   ```

   with a router decision from Agent A.

2. Pass the router:
   - raw prompt;
   - attachment policy;
   - current-turn paths;
   - current-turn dirs when available;
   - attached context counts;
   - semantic enabled/mode;
   - one-shot `ForceDeep` state from `/semantic deep`;
   - cached index status only if already known or available through a cheap
     non-blocking check.

   Do not block prompt submission just to compute index status. If status is not
   already known, pass `IndexKnown=false`; the semantic service handles missing or
   incompatible indexes as skip/fallback results.

3. For `use_explicit_context_only`, do not call `Retrieve`.

4. For `use_local_search_only`, do not call `Retrieve`; rely on existing mention
   expansion, file index, or filesystem behavior.

5. For `use_semantic_light`, call `Retrieve` with:

   ```text
   MaxRecords = 4
   MaxFiles = 1
   MaxContextBytes = 8192
   Deadline = 1200ms
   CurrentTurnPaths = currentTurnPaths
   ```

6. For `use_semantic_full`, call `Retrieve` with:

   ```text
   MaxRecords = 12
   MaxFiles = 4
   MaxContextBytes = 65536
   Deadline = 3000ms
   CurrentTurnPaths = currentTurnPaths
   ```

   If the decision has `Profile="deep"`, use the router decision's deep caps
   instead of the normal full caps.

7. Add `/semantic deep`:
   - forces broad semantic retrieval for the next prompt only;
   - displays a clear system message that deep semantic retrieval is enabled;
   - automatically clears after that prompt is submitted, cancelled, or skipped;
   - does not change the persistent `off`, `explicit`, or `auto` semantic mode.

8. Add status labels:

   ```text
   Reading mentioned file...
   Searching workspace...
   Embedding query...
   Scoring semantic index...
   Preparing model request...
   Waiting for model response...
   ```

9. Add TUI transcript/system messages for semantic skips:

   ```text
   [Semantic skipped: skip_explicit_context]
   [Semantic skipped: skip_dimensions_mismatch]
   [Semantic skipped: skip_deadline]
   ```

10. Update tests:
    - explicit-file prompts do not invoke the semantic stub;
    - listing prompts do not invoke the semantic stub;
    - broad prompts invoke the semantic stub once;
    - related-code explicit-file prompts invoke the semantic stub once with light
      caps;
    - `/semantic deep` forces `Profile="deep"` and deep caps;
    - waiting status is no longer used for semantic retrieval stages.

Acceptance:

- the real observed prompt `@docs/WEB-UI-UX-PRODUCT-PLAN.md can you access this
  document` skips semantic retrieval in TUI;
- TUI shows route/stage-specific status before generic `Waiting for model...`;
- no TUI test expects semantic retrieval for simple explicit-file prompts.

### Agent C - Server Session And API Event Integration

Goal:

- Apply the same router decisions to server sessions and SSE events.

Primary files:

- `internal/server/session.go`
- `internal/server/session_test.go`
- `internal/server/types.go`
- `internal/server/sse.go`
- `internal/server/web/index.html` only if event names are rendered there

Tasks:

1. Replace server-side broad semantic retrieval with router decisions.

2. Emit one route event per turn:

   ```json
   {
     "type": "retrieval_route_decided",
     "action": "use_explicit_context_only",
     "reason": "skip_explicit_context",
     "allow_embedding": false
   }
   ```

3. Emit semantic stage events when retrieval runs:

   ```text
   semantic_query_embed_started
   semantic_query_embed_finished
   semantic_search_finished
   semantic_skipped
   ```

4. On dimension mismatch or incompatible index:
   - emit `semantic_skipped`;
   - continue the prompt with non-semantic context;
   - do not return a prompt-blocking error.

5. Add server tests:
   - explicit-file access prompt skips semantic retrieval;
   - explicit-file status prompt skips semantic retrieval;
   - listing prompt skips semantic retrieval;
   - broad workspace prompt invokes semantic retrieval once;
   - related explicit-file prompt invokes semantic retrieval once with light caps;
   - dimension mismatch emits skip and continues;
   - route event is emitted before semantic retrieval or model generation.

6. Preserve existing SSE compatibility:
   - do not remove existing `semantic_retrieval` event until clients are migrated;
   - add new events in an additive way.

7. Index status for the router:
   - use cached status if the session already has it;
   - refresh cached status after `/index build`, `/index refresh`, `/index clear`,
     and `/index status`;
   - do not add blocking status IO to every prompt just to fill router inputs;
   - pass `IndexKnown=false` when status is unknown.

Acceptance:

- server behavior matches TUI route behavior for the same prompts;
- the old regression tests that expected explicit-file prompts to invoke semantic
  retrieval are updated to the locked behavior;
- web/server clients can distinguish route, semantic, and model wait stages.

### Agent D - Semantic Service Caps, Deadlines, And Freshness

Goal:

- Make semantic retrieval obey router caps/deadlines and degrade cleanly when the
  index is missing, stale, mismatched, or too slow.

Primary files:

- `internal/semantic/contracts.go`
- `internal/semantic/service.go`
- `internal/semantic/search.go`
- `internal/semantic/render.go`
- `internal/semantic/stale.go`
- `internal/semantic/service_test.go`
- `internal/semantic/hypotheses_test.go`
- `internal/semantic/retrieve_bench_test.go`

Tasks:

1. Extend `RetrieveRequest` with router metadata:

   ```go
   Deadline             time.Duration
   RouteAction          string
   RouteReason          string
   RouteProfile         string
   UseCurrentPathWeight bool
   ```

2. Enforce request caps:
   - `MaxRecords`;
   - `MaxFiles`;
   - `MaxContextBytes`;
   - deadline.

3. Add context deadline handling:
   - create `context.WithTimeout` from the route deadline;
   - if exceeded, return a fallback result with reason `skip_deadline`;
   - never return a fatal error for deadline expiration in normal prompt flow.

4. Handle dimensions mismatch:
   - detect manifest/query dimension mismatch before expensive work when possible;
   - return fallback reason `skip_dimensions_mismatch`;
   - include expected and actual dimensions in diagnostics;
   - do not call the chat model with stale semantic context.

5. Implement `CurrentTurnPaths` weighting:
   - boost records whose path is in current-turn paths for related-code prompts;
   - suppress unrelated records when explicit context is enough;
   - constrain `semantic_light` to current file, siblings, imports, or directly
     related symbols when possible.

6. Strengthen freshness checks:
   - re-stat selected files before rendering;
   - drop deleted files;
   - drop changed files when size/mtime/hash no longer match;
   - read final snippets from disk, not stored preview text;
   - include `stale_dropped` in result metadata.

7. Add tests:
   - light caps are enforced;
   - full caps are enforced;
   - deadline returns fallback instead of fatal error;
   - dimensions mismatch returns fallback;
   - `CurrentTurnPaths` changes ranking for related-code prompts;
   - stale files are dropped before render;
   - rendered context never exceeds route byte cap.

Acceptance:

- semantic service can be safely called by both TUI and server with route caps;
- broken/mismatched/stale indexes degrade without blocking ordinary prompts;
- `CurrentTurnPaths` is no longer ineffective.

### Agent E - Observability, Trace, And Stage Timing

Goal:

- Make the latency visible enough to distinguish retrieval, prompt packing,
  Ollama request opening, first token, and first visible render.

Primary files:

- `internal/observability/llm.go`
- `internal/observability/llm_test.go`
- `internal/agent/stream.go`
- shared event/helper files needed by Agent B and Agent C
- `internal/commands/registry_test.go` if trace output is exposed there

Agent E should not directly own TUI/server integration edits. Instead, Agent E
defines shared helper APIs and test fixtures; Agent B wires TUI-specific calls
and Agent C wires server/SSE-specific calls.

Tasks:

1. Add trace/stage fields:

   ```text
   retrieval_route_decided.action
   retrieval_route_decided.reason
   semantic_query_embed_ms
   semantic_search_ms
   semantic_context_bytes
   semantic_records
   semantic_files
   semantic_stale_dropped
   llm_request_started_at
   llm_stream_opened_at
   first_token_received_at
   first_visible_render_at
   prompt_bytes
   effective_num_ctx
   model
   provider
   ```

2. Emit `LLMRequestStarted` immediately before `client.Chat`.

3. Emit `LLMStreamOpened` immediately after `client.Chat` returns a stream and
   before tokens are consumed.

4. Emit first-token timing on the first thinking/text/tool event from the model.

5. Preserve existing slow-stage summary and extend it with:

   ```text
   route_decision
   semantic_total
   llm_request_open
   first_token
   first_visible_render
   ```

6. Add tests:
   - route event is present for every prompt turn;
   - LLM request open timing is recorded;
   - first visible render timing still works;
   - semantic skip does not show as semantic retrieve duration;
   - slow-stage summary identifies the correct slowest stage.

Acceptance:

- a run like the observed 5-minute case can show whether time was spent in
  retrieval, request opening, model prefill, token generation, or rendering;
- `Waiting for model...` is only a fallback label, not the only visible state.

### Agent F - Config, Slash Commands, And Keep-Alive Policy

Goal:

- Add configuration for the locked defaults and separate chat/embedding
  keep-alive behavior.

Primary files:

- `internal/semantic/config.go`
- `internal/config/defaults.go`
- `internal/config/loader.go`
- `internal/config/config.go`
- `internal/config/loader_test.go`
- `internal/commands/registry.go`
- command help/registry files for semantic commands
- `internal/bootstrap/state.go`
- `internal/llm/types.go`
- `internal/llm/ollama/ollama.go`
- `internal/llm/ollama/ollama_test.go`

Tasks:

1. Add config fields:

   ```toml
   [semantic_index]
   mode = "auto"
   light_top_k_records = 4
   light_top_k_files = 1
   light_max_context_bytes = 8192
   light_deadline_ms = 1200
   full_top_k_records = 12
   full_top_k_files = 4
   full_max_context_bytes = 65536
   full_deadline_ms = 3000
   deep_top_k_records = 40
   deep_top_k_files = 12
   deep_max_context_bytes = 262144
   query_keep_alive = "30s"
   build_keep_alive = "10m"
   ```

2. Keep existing config names backward compatible:
   - map existing `top_k_records`, `top_k_files`, and `max_context_bytes` to full
     mode if no new full values are set;
   - do not break existing config files.

3. Add chat keep-alive config if missing:

   ```toml
   chat_keep_alive = "5m"
   ```

   This is a top-level config key. It maps to the existing session/bootstrap
   chat keep-alive concept and must be kept separate from
   `semantic_index.query_keep_alive` and `semantic_index.build_keep_alive`.

4. Wire chat `KeepAlive` into Ollama chat requests if not already active.

5. Split embedding keep-alive by use case:
   - index build/refresh can keep the embedding model warm longer;
   - prompt query embedding should default shorter, such as `30s`.

6. Add slash command behavior:
   - `/semantic status` shows mode, light/full caps, index compatibility, and
     current keep-alive settings;
   - `/semantic deep` is documented in command help/registry;
   - Agent B owns the actual TUI state transition in `internal/tui/app.go`.

7. Add tests:
   - config loader parses new fields;
   - defaults match locked decisions;
   - old config remains valid;
   - chat keep-alive is sent to Ollama chat request;
   - query embedding keep-alive can differ from build keep-alive.

Acceptance:

- config defaults match the owner-confirmed decisions;
- users can inspect semantic mode and caps;
- chat and embedding keep-alive are no longer accidentally coupled.

### Agent G - Regression Tests And Evaluation Fixtures

Goal:

- Build the safety net that proves the regression is fixed and the intended
  semantic feature still works.

Primary files:

- `internal/server/session_test.go`
- `internal/tui/app_test.go`
- `internal/semantic/hypotheses_test.go`
- `internal/semantic/service_test.go`
- `internal/semantic/retrieve_bench_test.go`
- `internal/contextpack/current_turn_test.go`
- new fixtures under `internal/semantic/testdata` if needed

Tasks:

1. Convert current regression tests:
   - tests named like `InvokesSemanticRetrievalForExplicitFileStatusPrompt`
     should be replaced by tests proving explicit-file status/access prompts skip
     semantic retrieval.

2. Add no-embedding-zone tests for both TUI and server:
   - exact file access;
   - exact file summarization;
   - exact file status;
   - directory listing;
   - local slash command;
   - memory recall path if testable without large setup.

3. Add positive semantic tests:
   - broad workspace bug prompt uses `semantic_full`;
   - explicit related-code prompt uses `semantic_light`;
   - `/semantic deep` uses deep/full broad caps.

4. Add observability tests:
   - route decision event exists;
   - skip event exists with reason;
   - semantic stage events exist only when retrieval runs;
   - LLM request/open/first-token timings are recorded.

5. Add mismatch/stale tests:
   - dimensions mismatch skips retrieval and continues;
   - stale selected file is dropped;
   - missing index skips retrieval without embedder call.

6. Add benchmark updates:
   - compare skip route vs light route vs full route;
   - include 1024 and 4096 dimension vector sizes;
   - report allocations and context bytes.

Acceptance:

- test names describe the locked behavior;
- failing tests clearly identify route, caps, or event regressions;
- no test relies on live Ollama or a downloaded embedding model.

### Agent H - Documentation, Manual Smoke, And Release Notes

Goal:

- Keep user-facing docs and manual validation aligned with the new retrieval
  policy.

Primary files:

- `docs/WAITING-FOR-MODEL-LATENCY-REPORT.md`
- `docs/PHASE-28-DETAILED-PLAN.md`
- `docs/PHASE-29-DETAILED-PLAN.md`
- `docs/PHASE-LOG.md`
- `USER_MANUAL.md` or nearest command/user guide if present

Tasks:

1. Document final semantic modes and commands:

   ```text
   off
   explicit
   auto
   ```

   `/semantic deep` is a one-shot command for the next prompt, not a persistent
   mode.

2. Document examples:
   - `@file can you access this?` skips semantic retrieval;
   - `list files in @docs/` skips semantic retrieval;
   - `fix the authentication bug` uses semantic retrieval;
   - `@auth.go find related callers` uses light semantic retrieval;
   - `/semantic deep` opts into broader retrieval.

3. Add manual smoke steps:
   - run without index;
   - build index;
   - run explicit-file access prompt;
   - run broad bug prompt;
   - run dimension mismatch scenario if feasible;
   - compare `/semantic off`, `auto`, and `/semantic deep`.

4. Document expected observability:

   ```text
   retrieval_route_decided ...
   semantic_skipped ...
   semantic_query_embed_started ...
   llm_request_started ...
   llm_stream_opened ...
   ```

5. Update release notes with:
   - exact-context prompts are faster after indexing;
   - semantic retrieval remains available for broad discovery;
   - broken/mismatched indexes no longer block ordinary prompts.

Acceptance:

- docs match the locked decisions;
- manual validation can be followed by someone who did not implement the feature;
- no docs claim embeddings answer prompts or make permission decisions.

### Agent I - REPL And Print Entry-Point Audit

Goal:

- Make the locked routing behavior true for non-TUI entry points named in the
  baseline, or prove those entry points do not currently perform prompt-time
  semantic retrieval.

Primary files:

- `internal/cli/*`
- `cmd/nandocodego/*`
- `internal/agent/*`
- REPL/print tests nearest to those files

Tasks:

1. Trace REPL and one-shot/print prompt execution paths from command entry to
   model request.

2. Determine whether either path currently invokes semantic retrieval before
   model generation.

3. If an entry point already invokes semantic retrieval:
   - consume Agent A's router contract;
   - use the same no-embedding zones as TUI/server;
   - use the same light/full/deep caps from Agent F;
   - emit route/semantic skip diagnostics through the entry point's existing
     logging or transcript mechanism.

4. If an entry point does not invoke semantic retrieval:
   - add a small regression test or implementation note proving no prompt-time
     embedder call exists there;
   - do not add semantic retrieval just to make the entry point match TUI/server.

5. Keep ownership boundaries:
   - do not edit `internal/tui/app.go`;
   - do not edit `internal/server/session.go`;
   - reuse Agent A router and Agent D semantic request fields when needed.

Acceptance:

- REPL and print paths either follow the router or are documented/tested as
  semantic-free;
- no entry point keeps the old broad rule that embeds exact file access/status
  prompts;
- non-TUI behavior does not regress while TUI/server integration lands.

### Merge And Validation Order

1. Merge Agent A first or create a shared base branch with Agent A's router
   contract.
2. Merge Agent F config defaults after Agent A so route decisions can use config.
3. Merge Agent D semantic caps/deadlines after Agent A and F.
4. Merge Agent C server integration and Agent B TUI integration after Agent A and
   D.
5. Merge Agent E shared observability helpers once the event contract is stable;
   Agent B and Agent C perform the TUI/server wiring in their owned files.
6. Merge Agent G tests throughout, but final regression/eval gate runs after B,
   C, D, E, and F are integrated.
7. Merge Agent I after Agent A and before the final full validation gate.
8. Merge Agent H docs last, then update with exact test outputs.

Required validation commands:

```text
go test ./internal/retrievalroute ./internal/semantic ./internal/server ./internal/tui ./internal/config ./internal/llm/ollama
go test ./internal/cli ./cmd/nandocodego
go test ./...
```

Manual validation prompts:

```text
@docs/WEB-UI-UX-PRODUCT-PLAN.md can you access this document
summarize @docs/WEB-UI-UX-PRODUCT-PLAN.md
list files in @docs/
fix the authentication bug
@internal/server/session.go find related callers and utilities
/semantic deep
fix the authentication bug
```

Exit criteria:

- explicit-file access/status/summarization prompts do not call the embedder;
- listing/path prompts do not call the embedder;
- broad discovery prompts call the embedder once and use `semantic_full`;
- related-code prompts call the embedder once and use `semantic_light`;
- dimension mismatch skips semantic retrieval and continues;
- `Waiting for model...` is no longer the only status visible during long
  pre-answer work;
- route and stage timing appear in trace/SSE/TUI output;
- no permission or security decision uses embeddings.

## Historical Validation Tests For Embedding/Semantic Hypotheses

This section is historical validation background. It is useful for understanding
the regression, but the active implementation contract is the `Final Decisions
For Next Phase` and `Parallel Implementation Task Plan` sections above.

The tests below are designed to prove or disprove the semantic-index regression
hypotheses with measurable evidence. They should be run before choosing a large
implementation fix.

### Test T1: Baseline Before/After Index Existence

Purpose:

- verify whether the same prompt is slower only after an index exists.

Setup:

1. choose a fixed model and keep it constant;
2. choose a fixed prompt, for example:

   ```text
   @docs/WEB-UI-UX-PRODUCT-PLAN.md can you access this document
   ```

3. clear the semantic index;
4. run the prompt three times;
5. build the index with `/index build`;
6. run the same prompt three more times.

Record:

- total time to final answer;
- time to first visible output;
- `semantic_retrieve` duration;
- semantic records/files/context bytes;
- requested `num_ctx`;
- prompt size after packing;
- model/provider.

Expected confirmation:

- if the indexed runs are consistently slower and include semantic retrieval
  records/context, indexing changed the prompt path.

Expected rejection:

- if indexed and non-indexed runs are similar, the regression may be unrelated to
  semantic retrieval and more likely due to model cold start, `num_ctx`, or
  machine load.

### Test T2: `/semantic off` Versus `/semantic on`

Purpose:

- isolate semantic retrieval while keeping the built index present.

Setup:

1. ensure the index exists;
2. run:

   ```text
   /semantic off
   ```

3. run the fixed prompt three times;
4. run:

   ```text
   /semantic on
   ```

5. run the fixed prompt three times.

Record:

- total time;
- first visible output latency;
- `semantic_retrieve` duration;
- semantic context bytes;
- answer quality.

Expected confirmation:

- if `/semantic off` is materially faster, retrieval or the extra retrieved
  context is contributing to the slowdown.

Expected refinement:

- if `semantic_retrieve` is small but first visible output grows only when
  semantic context is attached, the problem is probably prompt-size/prefill, not
  vector search itself.

### Test T3: Query Embedding Cold/Warm Latency

Purpose:

- measure whether the query embedding model adds startup latency or competes
  with the chat model.

Setup:

1. ensure semantic retrieval is enabled and index exists;
2. run one prompt after Ollama has been idle long enough for models to unload;
3. run the same prompt again immediately;
4. repeat with different `semantic_index.keep_alive` values if configurable.

Record:

- semantic query embedding duration;
- total `semantic_retrieve` duration;
- first visible output latency;
- Ollama model load/active model state if available through `ollama ps`;
- system RAM/VRAM pressure.

Expected confirmation:

- first run has slow query embedding and later runs are faster;
- or embedding model stays loaded while chat model cold-starts.

Expected rejection:

- query embedding is consistently fast and does not correlate with the long
  waiting period.

### Test T4: Chat Model Eviction After Embedding Retrieval

Purpose:

- test whether loading/keeping the embedding model causes the chat model to be
  unloaded or slowed.

Setup:

1. warm the chat model with a tiny prompt and record first-token latency;
2. run a semantic retrieval prompt that uses the embedding model;
3. immediately run the same tiny chat prompt again;
4. compare with a control run where semantic retrieval is disabled.

Record:

- first-token latency before embedding;
- first-token latency after embedding;
- active Ollama models before/after each step;
- memory pressure.

Expected confirmation:

- chat first-token latency worsens after embedding retrieval;
- Ollama shows the embedding model resident and the chat model missing or
  reloading.

Possible fix if confirmed:

- wire chat `keep_alive`;
- shorten embedding keep-alive for query retrieval;
- avoid semantic retrieval for simple prompts.

### Test T5: Semantic Context Size Impact

Purpose:

- determine whether retrieval slows generation mostly by adding prompt context.

Setup:

Run the same prompt with different retrieval limits:

```text
top_k_records = 0 or semantic off
top_k_records = 4,  top_k_files = 1, max_context_bytes = 8192
top_k_records = 12, top_k_files = 4, max_context_bytes = 65536
top_k_records = 40, top_k_files = 12, max_context_bytes = 262144
```

Record:

- semantic context bytes;
- final prompt estimated tokens;
- first visible output latency;
- answer quality.

Expected confirmation:

- first visible output latency grows with semantic context bytes even when
  semantic retrieval itself remains fast.

Expected rejection:

- context size changes do not affect first visible output latency.

### Test T6: Vector Load And Search Benchmark

Purpose:

- measure local retrieval cost independent of Ollama chat generation.

Implementation approach:

- add Go benchmarks around `LocalService.Retrieve`;
- use a fake embedder that returns a deterministic vector;
- create synthetic indexes at several sizes:
  - 1,000 records;
  - 10,000 records;
  - 50,000 records;
  - 200,000 records if feasible.

Record:

- records load time;
- vectors load time;
- scoring time;
- render time;
- allocations;
- total benchmark time.

Useful commands:

```text
go test ./internal/semantic -bench Retrieve -benchmem
```

Expected confirmation:

- retrieval time or allocations scale sharply with record count;
- loading vectors dominates repeated prompts.

Possible fix if confirmed:

- in-memory index cache;
- approximate nearest-neighbor index later;
- lower `max_records`;
- persist precomputed search-friendly structures.

### Test T7: Retrieval Cache Experiment

Purpose:

- test whether repeated disk loads are a major part of semantic latency.

Setup:

1. implement a temporary or test-only cached store wrapper;
2. run `Retrieve` repeatedly on the same index;
3. compare uncached versus cached records/vectors.

Record:

- first retrieval duration;
- second retrieval duration;
- allocations;
- memory retained by cache.

Expected confirmation:

- cached retrieval is much faster after the first load.

Expected rejection:

- cache provides little improvement, meaning query embedding or scoring is the
  dominant cost.

### Test T8: Explicit-File Prompt Bypass Test

Purpose:

- prove whether semantic retrieval is unnecessary for simple explicit-file
  prompts.

Prompt classes:

```text
@docs/WEB-UI-UX-PRODUCT-PLAN.md can you access this document
what is the status of @docs/WEB-UI-UX-PRODUCT-PLAN.md
summarize @docs/WEB-UI-UX-PRODUCT-PLAN.md
find related code for authentication failures
```

Expected behavior:

- access/status prompts should skip or heavily limit semantic retrieval;
- summarize explicit file should usually rely on explicit file context;
- broad related-code prompts should use semantic retrieval.

Automated test approach:

- use a semantic service stub in TUI/server prompt submission tests;
- assert `Retrieve` is not called for simple explicit-file access/status prompts;
- assert `Retrieve` is called for broad semantic prompts.

Expected confirmation:

- current code calls retrieval too broadly;
- bypassing it improves latency without hurting answer quality for simple
  explicit-file prompts.

### Test T9: Current-Turn Path Scoring Test

Purpose:

- verify whether `CurrentTurnPaths` should change retrieval output.

Setup:

- build a synthetic semantic index with:
  - one explicitly mentioned file;
  - several semantically similar unrelated files;
  - one current-turn packed file that should dominate.

Current behavior to inspect:

- `RetrieveRequest.CurrentTurnPaths` is populated by TUI/server;
- scoring currently only passes `ExplicitPaths` into `scoreRecords`.

Automated test:

- call `Retrieve` with and without `CurrentTurnPaths`;
- compare returned files.

Expected confirmation:

- results do not change when `CurrentTurnPaths` changes, proving the field is
  currently ineffective.

Possible fix:

- use `CurrentTurnPaths` to boost, suppress, or constrain retrieval.

### Test T10: Runtime Config Application Test

Purpose:

- verify that semantic retrieval uses configured retrieval weights and limits.

Current concern:

- `Retrieve` starts from `DefaultConfig()` internally;
- request limits are passed separately, but scoring weights and keep-alive may
  not reflect runtime config.

Automated test:

- configure different `HybridLexicalWeight` values;
- run retrieval against an index where lexical and vector scores disagree;
- assert result ordering changes when config changes.

Expected confirmation:

- if result ordering does not change, runtime config is not being applied
  deeply enough.

Possible fix:

- pass normalized runtime semantic config into `Retrieve`;
- avoid calling `DefaultConfig()` inside retrieval except as fallback.

### Test T11: Prompt Prefill Versus Retrieval Time

Purpose:

- separate "retrieval took time" from "retrieval made the chat prompt slower."

Setup:

Create three final prompt variants:

1. explicit file context only;
2. explicit file context plus 32KB semantic context;
3. explicit file context plus 256KB semantic context.

Send each to the same chat model with identical `num_ctx` and `num_predict`.

Record:

- time from LLM request start to stream open;
- time from stream open to first token;
- total duration;
- prompt eval token count if Ollama reports it.

Expected confirmation:

- larger semantic context increases prompt eval / first-token latency.

Possible fix:

- reduce default semantic context limits;
- add intent-aware semantic caps;
- report prompt eval latency explicitly.

### Test T12: `num_ctx` Interaction With Semantic Context

Purpose:

- determine whether semantic context becomes much slower because the app asks
  Ollama for an extremely large context window.

Setup:

Run the same semantic-enabled prompt with:

```text
num_ctx = 8192
num_ctx = 32768
num_ctx = model reported context length
num_ctx = current default/requested value
```

Record:

- LLM request open latency;
- first-token latency;
- total duration;
- memory usage;
- whether the answer quality changes.

Expected confirmation:

- large `num_ctx` values create disproportionate waiting time, especially when
  semantic context is attached.

Possible fix:

- normalize default `num_ctx`;
- use adaptive `num_ctx` based on prompt size rather than huge defaults.

### Test T13: Semantic Retrieval Status UX Test

Purpose:

- verify whether users can understand what the app is doing during retrieval.

Setup:

- run a prompt that triggers semantic retrieval on a large index;
- observe the TUI status line and transcript notices.

Expected current result:

- the user only sees `Waiting for model...` for the broad active-run period;
- semantic details appear late or as coarse transcript notices.

Expected improved result:

```text
Semantic: embedding query...
Semantic: loading vectors...
Semantic: scoring 42k records...
Semantic: adding 12 files / 32KB...
Waiting for first token...
```

Acceptance criteria:

- users can distinguish semantic retrieval from chat model waiting;
- a slow stage can be identified without reading code.

### Test T14: Answer Quality Regression Test

Purpose:

- make sure performance fixes do not remove useful retrieval context.

Prompt set:

- simple explicit-file access prompt;
- explicit file summary prompt;
- broad "find relevant files" prompt;
- bug-fix prompt with indirect terminology;
- architecture question involving multiple packages.

Run each with:

- semantic off;
- semantic current defaults;
- reduced semantic limits;
- intent-aware semantic bypass.

Score:

- correct files referenced;
- hallucinated files;
- answer completeness;
- time to first output;
- total answer time.

Expected result:

- simple explicit-file prompts should not need semantic retrieval;
- broad discovery prompts should benefit from semantic retrieval;
- reduced/adaptive limits should preserve most quality with lower latency.

## Minimal Metrics Needed For The Tests

To make these tests reliable, add or expose these per-run metrics:

- `mention_expand_ms`;
- `semantic_manifest_load_ms`;
- `semantic_records_load_ms`;
- `semantic_vectors_load_ms`;
- `semantic_query_embed_ms`;
- `semantic_score_ms`;
- `semantic_render_ms`;
- `semantic_records_returned`;
- `semantic_files_returned`;
- `semantic_context_bytes`;
- `llm_request_open_ms`;
- `llm_first_token_ms`;
- `first_visible_render_ms`;
- `num_ctx`;
- `num_predict`;
- final prompt estimated tokens/bytes;
- active model/provider;
- embedding model.

Without these metrics, it is easy to misattribute a slow answer to semantic
retrieval when the real cost is prompt prefill, model load, or oversized
`num_ctx`.

## Historical Validation Results (2026-05-27)

This section records pre-decision validation results. Some confirmed behaviors
below are intentionally the old behavior that the next phase must change.

Multiple agents validated the semantic-index hypotheses in parallel. The work
added characterization tests and benchmarks; no production behavior was changed.

Changed validation files:

- `internal/server/session_test.go`
- `internal/semantic/hypotheses_test.go`
- `internal/semantic/retrieve_bench_test.go`

Environment used for parent verification:

- `goos=darwin`
- `goarch=arm64`
- CPU: `Apple M5 Pro`

### Agent Findings

Agent A validated server invocation behavior in
`internal/server/session_test.go`.

- Listing prompts such as `list all files in @docs/` bypass semantic retrieval.
- Explicit-file status prompts such as `what is the current status of @plan.md`
  invoke semantic retrieval once.
- Simple explicit-file access prompts such as
  `@plan.md can you access this document` also invoke semantic retrieval once.

This explains why the real observed prompt entered the semantic retrieval path
after `/index build`: a simple explicit-file prompt is enough to retrieve
semantic context even when explicit file context is already available.

Agent B validated semantic retrieval behavior in
`internal/semantic/hypotheses_test.go`.

- Missing-index retrieval returns `ErrIndexMissing` and does not call the query
  embedder.
- Existing-index retrieval calls the query embedder once and returns semantic
  hits.
- `CurrentTurnPaths` does not affect returned records, returned files, or
  rendered semantic context.
- Retrieval uses default semantic behavior internally for keep-alive and lexical
  weighting in the current path.

This confirms that `/index build` changes the request path from cheap fallback
to active query embedding, vector loading/scoring, and semantic context
rendering. It also confirms that current-turn file knowledge is collected but
not used to suppress unnecessary extra context.

Agent C validated retrieval scaling in
`internal/semantic/retrieve_bench_test.go`.

- Record loading, vector loading, and scoring scale upward with index size.
- Record/vector loading allocations grow materially with record count.
- Scoring is comparatively fast at the tested sizes.
- End-to-end synthetic retrieval is measurable but does not independently
  explain a 5-minute wait.

This means semantic retrieval is a real additive cost after indexing, but the
multi-minute stall is more likely retrieval plus larger prompt/prefill/model-load
behavior than retrieval CPU alone.

### Verification Commands

```bash
go test ./internal/server -run 'TestSessionRun(BypassesSemanticRetrievalForListingIntent|InvokesSemanticRetrievalForExplicitFileStatusPrompt|InvokesSemanticRetrievalForSimpleExplicitFilePrompt|InjectsSemanticContextAndEmitsEvent|EmitsSemanticFallbackEvent)$' -count=1
go test ./internal/semantic -run 'TestRetrieveUsesDefaultConfigForKeepAliveAndScoring|TestRetrieveCurrentTurnPathsDoesNotAffectOutput|TestRetrieveReturnsErrIndexMissingWhenManifestMissing|TestRetrievePathWhenIndexExistsReturnsSemanticHits' -count=1
go test ./internal/semantic -run '^$' -bench 'Benchmark(LocalStoreLoadRecordsScaling|LocalStoreLoadVectorsScaling|ScoreRecordsScaling|LocalServiceRetrieveScaling)$' -benchmem -benchtime=1s
```

All commands passed.

### Benchmark Evidence

The benchmark dataset uses synthetic indexes with `256` dimensions. Production
embedding dimensions are currently `1024`, so these numbers are directional
rather than final production timing.

`LoadRecords`:

- `300` records: `~0.71ms`, `621244 B/op`, `4807 allocs/op`
- `1200` records: `~2.33ms`, `2278482 B/op`, `19207 allocs/op`
- `4800` records: `~8.62ms`, `8899140 B/op`, `76807 allocs/op`

`LoadVectors`:

- `300` records: `~0.40ms`, `624393 B/op`, `608 allocs/op`
- `1200` records: `~1.06ms`, `2492185 B/op`, `2408 allocs/op`
- `4800` records: `~3.86ms`, `9955081 B/op`, `9608 allocs/op`

`scoreRecords`:

- `300` records: `~0.14ms`, `81624 B/op`, `314 allocs/op`
- `1200` records: `~0.57ms`, `308024 B/op`, `1214 allocs/op`
- `4800` records: `~2.34ms`, `1230008 B/op`, `4814 allocs/op`

`LocalService.Retrieve` end-to-end:

- `300` records: `~8.69ms`, `5627119 B/op`, `7233 allocs/op`
- `1200` records: `~12.84ms`, `9381487 B/op`, `24336 allocs/op`
- `4800` records: `~32.20ms`, `24386840 B/op`, `92737 allocs/op`

### Historical Validated Hypothesis Status

Confirmed:

- Semantic retrieval is invoked for explicit-file status/access prompts.
- Semantic retrieval is bypassed for listing-only prompts.
- `CurrentTurnPaths` is currently ineffective in retrieval scoring/output.
- Missing-index retrieval is cheap because it does not call the embedder.
- Existing-index retrieval calls the embedder and performs real work.
- Retrieval cost and allocations scale upward with record count.

Partially confirmed:

- Semantic retrieval contributes to the post-index slowdown, but synthetic
  retrieval timing does not explain the full 5-minute `Waiting for model...`
  stall by itself.
- The stronger explanation is combined cost: retrieval adds context, the final
  chat prompt grows, Ollama does more prefill work, and the current UI hides the
  pre-first-token breakdown.

Still open from the historical validation pass:

- Whether the embedding model's `keep_alive` causes chat model eviction or cold
  starts on the user's machine.
- How much of the 5-minute wait is Ollama request-open latency versus prompt
  prefill versus first-token latency.
- How the benchmark changes at `1024` or `4096` dimensions and production-size
  record counts.

### Book-Derived Embedding Activation Rule

The project book does not define one explicit code-level rule for when semantic
retrieval must run. It does provide a clear design heuristic:

Use embeddings for broad semantic discovery. Do not use embeddings when the
application already has exact context.

Relevant book guidance:

- `book/ch11-memory.md`: standard RAG embeddings fit documentation, FAQs, and
  reference material, but are mismatched for agent memory because memory is
  small, frequently changing, and human-editable.
- `book/ch11-memory.md`: memory should avoid storing facts derivable from the
  current project state, such as code patterns, architecture, file structure,
  and git history.
- `book/ch11-memory.md`: recall for memory should use LLM selection over a
  lightweight index rather than embeddings.
- `book/ch17-performance.md`: fuzzy file/path discovery should use fast local
  indexing/filtering techniques, not semantic vector retrieval.
- `book/ch18-epilogue.md`: file-based memory plus LLM recall is preferred over
  a database or vector database for transparent agent memory.

That maps to this implementation rule:

| Prompt or feature path | Semantic retrieval policy | Reason |
| --- | --- | --- |
| `@file can you access this?` | Skip | The file is already explicit context. |
| `summarize @file` | Skip by default | The requested evidence is already attached. |
| `what is the status of @file` | Skip or cap hard | Status questions should stay file-scoped unless they ask for related code. |
| `list files in @dir` or path search | Skip | Filesystem listing/fuzzy path search should use deterministic indexes. |
| memory recall/user preferences | Skip | Book guidance favors file-based memory plus LLM recall. |
| broad bug/fix/refactor prompt with no exact files | Run | The app needs semantic discovery across the workspace. |
| `find code related to auth/session/token bug` | Run | The relevant code may not contain the exact words in the prompt. |
| `@file find related utilities/usages/dependencies` | Run, but bounded and current-path weighted | The user asks to expand beyond explicit context. |

Operationally, semantic retrieval should be intent-gated rather than merely
index-gated. A built index means retrieval is available; it should not mean every
non-listing prompt pays the retrieval and prompt-growth cost.

Acceptance criteria for the next fix:

- explicit-file access/status/summarization prompts do not call the embedder;
- listing/tree/path-search prompts do not call the embedder;
- memory recall does not call the semantic workspace index;
- broad workspace discovery prompts still call the embedder;
- explicit-file related-code prompts call the embedder with lower caps and
  `CurrentTurnPaths` used for scoring, suppression, or bounds;
- semantic retrieval decisions are logged in trace output with a reason such as
  `skip_explicit_context`, `skip_listing_intent`, `run_workspace_discovery`, or
  `run_related_context`.

### Superseded Implementation Notes

This section is superseded by `Final Decisions For Next Phase` and
`Parallel Implementation Task Plan`. It remains as background for why the locked
plan prioritizes router-first behavior, observability, keep-alive, and context
normalization.

Priority 1: add live timing instrumentation.

Add stage timings for semantic manifest load, records load, vectors load, query
embedding, scoring, render/stale filtering, LLM request start, LLM stream open,
first token, and first visible render.

Reason: current tests prove retrieval is invoked and additive, but live
telemetry is needed to locate the multi-minute stall precisely.

Priority 2: add intent-aware semantic retrieval activation policy.

Implement the book-derived activation rule above. Bypass semantic retrieval for
simple explicit-file access prompts, explicit-file status prompts,
summarization-only prompts, listing/path-search prompts, and prompts where
current-turn explicit file context is already sufficient. Run retrieval for broad
workspace discovery and for explicit related-code requests.

Suggested cap when not fully bypassed:

```text
top_k_records = 4
top_k_files = 1
max_context_bytes = 8192
```

Reason: validated server tests show these simple prompts currently invoke
retrieval even though the explicit file is already attached. The book guidance
also argues against using embeddings when exact context or deterministic local
indexes are the better tool.

Priority 3: use `CurrentTurnPaths` in retrieval.

Use current-turn paths to boost current files, suppress unrelated files when
current context is enough, or constrain retrieval for explicit-file prompts.

Reason: tests prove `CurrentTurnPaths` is passed but currently ineffective.

Priority 4: normalize chat context and keep-alive behavior.

Implement safer default `num_ctx` behavior, chat `keep_alive`, and separate
embedding/chat keep-alive tuning.

Reason: retrieval alone does not explain the real 5-minute delay. The remaining
likely bottlenecks are model load, model eviction, prompt prefill, and oversized
context windows.

Priority 5: add larger-dimension benchmarks.

Extend benchmarks to model production dimensions:

```text
256 dimensions
1024 dimensions
4096 dimensions
```

Reason: current benchmarks prove scaling direction, but production embeddings
use larger vectors than the synthetic benchmark.

Priority 6: evaluate retrieval caching.

Test an in-memory cache for records/vectors keyed by workspace ID and manifest
updated time.

Reason: benchmarks show repeated records/vectors loading allocates noticeably
and scales with index size. Caching may reduce additive retrieval cost, but it
should follow instrumentation so memory tradeoffs are visible.

### Superseded Brainstormed Solution Directions

This section is superseded by the locked owner decisions. It remains as
brainstorm history only.

The following are proposed next-step solutions based on the validated findings.
At the time this section was written, these were not implementation decisions.
They are now superseded by the owner-confirmed lock above.

1. Add live timing instrumentation first.

Track exact time spent in:

```text
semantic manifest load
semantic records load
semantic vectors load
semantic query embed
semantic score
semantic render
LLM request start
LLM stream open
first token
first visible render
```

Reason:

The 5-minute delay is not fully explained by semantic retrieval alone. The app
needs stage-level evidence before tuning blindly.

2. Add a book-derived semantic retrieval activation policy.

For prompts like:

```text
@plan.md can you access this document
what is the status of @plan.md
summarize @plan.md
list files in @docs/
```

the app already has explicit file or deterministic filesystem context. Semantic
retrieval should be skipped.

For prompts like:

```text
fix the authentication bug
find code related to session expiry
@auth.go find related utilities and callers
```

semantic retrieval should run because the user is asking for broad semantic
workspace discovery or related-code expansion.

Possible capped behavior:

```text
top_k_records = 4
top_k_files = 1
max_context_bytes = 8192
```

The capped behavior applies only when the user starts from an explicit file but
asks for related code. It should not apply to plain access/status/summarization
prompts, which should fully bypass semantic retrieval.

3. Add a shared retrieval router from the model-routing lessons.

The `embedding-and-model-routing.md` document separates main answering, local
commands, side queries, memory recall, session search, prompt suggestions,
token counting, UI fuzzy search, and permission decisions into different model
or non-model routes. Apply the same pattern here by adding one shared router
before semantic retrieval.

The router should return decisions such as:

```text
skip_all_retrieval
use_explicit_context_only
use_local_search_only
use_small_model_rerank
use_semantic_light
use_semantic_full
```

The router should also emit a decision reason:

```text
skip_local_command
skip_explicit_context
skip_listing_intent
skip_memory_recall
run_bounded_rerank
run_related_context
run_workspace_discovery
```

This makes the embedding model one retrieval route instead of the default model
path after `/index build`.

4. Add a hybrid retrieval ladder.

Use cheaper and more auditable mechanisms before vector search:

```text
explicit @ context
deterministic path/symbol/grep/current-turn search
small-model rerank over bounded candidates
semantic embedding retrieval for large/broad discovery
main chat model with explicit retrieved text
```

This follows the routing document's pattern: "semantic" behavior can often be
implemented by local prefiltering plus a small model selector without a vector
lookup.

5. Use `CurrentTurnPaths` in retrieval.

The app passes current-turn paths into retrieval, but validation shows they do
not currently affect output.

Use them to:

- boost already-mentioned/current files;
- avoid adding unrelated files;
- suppress retrieval when explicit context is enough.

6. Reduce semantic retrieval defaults.

Current defaults can add broad context:

```text
top_k_records = 40
top_k_files = 12
max_context_bytes = 262144
```

Potential safer defaults:

```text
top_k_records = 12
top_k_files = 4
max_context_bytes = 65536
```

Potential explicit-file prompt caps:

```text
top_k_records = 4
top_k_files = 1
max_context_bytes = 8192
```

7. Normalize `num_ctx`.

The app can request a huge context window, which can make Ollama slow before the
first token.

Possible behavior:

- use model-reported context by default;
- avoid silent `num_ctx=1000000`;
- make huge context explicit rather than default;
- adapt `num_ctx` to actual prompt size.

8. Wire chat `keep_alive`.

Embedding requests use keep-alive, but chat requests currently do not. That can
leave the embedding model warm while the chat model cold-starts.

Possible behavior:

- add chat `keep_alive`;
- keep the chat model warm during interactive sessions;
- tune embedding keep-alive separately from chat keep-alive.

9. Add retrieval caching.

Benchmarks show repeated records/vectors loading allocates and scales with index
size.

Possible cache key:

```text
workspace_id + manifest_updated_at
```

Invalidate on:

```text
/index build
/index refresh
/index clear
```

10. Make semantic retrieval mode configurable.

Possible config:

```text
semantic_index.mode = "off" | "explicit" | "auto"
```

Possible behavior:

- `off`: never retrieve;
- `explicit`: retrieve only when requested;
- `auto`: current behavior, but with intent-aware bypasses.

Recommended brainstorm order:

1. Instrument live timing.
2. Add the shared retrieval router and no-embedding zones.
3. Implement the book-derived semantic retrieval activation policy through that
   router.
4. Add semantic light/full modes, route caps, and route deadlines.
5. Add deterministic local candidate search and optional small-model rerank.
6. Normalize `num_ctx`.
7. Add chat `keep_alive`.
8. Use `CurrentTurnPaths`.
9. Reduce default semantic limits.
10. Evaluate retrieval caching.

## User-Visible Symptom

The status bar remains on:

```text
Waiting for model...
```

for a long time before changing to:

```text
Thinking...
```

or:

```text
Streaming...
```

In some cases it may never show `Thinking...` because not every selected model
is configured as a thinking-capable model, and some models may stream content or
tool calls without separate thinking deltas.

## What The Status Means Today

The status is computed in `internal/tui/runstate.go`.

Current transition logic:

- permission prompt wins first;
- running tool wins next;
- compaction and retry states win next;
- if `thinkingActive` is true, show `Thinking`;
- if `firstStreamAt` is set, show `Streaming`;
- if `ActiveRun` is true and none of the above happened, show `Waiting for model`.

Relevant code:

- `internal/tui/runstate.go`
- `internal/tui/app.go`

Important detail: `Waiting for model...` is a fallback active-run state. It is
not a precise measurement of where the application is blocked.

## Event Flow

### TUI run startup

Prompt submission eventually calls `submitPrompt` in `internal/tui/app.go`.

Before the run starts, the TUI does:

- mention expansion;
- current-turn prompt packing;
- semantic retrieval, if enabled;
- transcript updates.

Then it sets:

```go
app.ActiveRun = true
```

and starts the agent command.

From this moment until the first assistant/tool event, the status bar can show
`Waiting for model...`.

### Agent startup

The TUI calls `startAgentCmd`, which drains events from the agent runner and
sends them to Bubble Tea.

Relevant code:

- `internal/tui/bridge.go`

### Runner wrappers

The REPL wraps the core agent runner in additional layers:

- memory runner;
- hooks runner;
- observability runner.

Relevant code:

- `internal/cli/repl.go`
- `internal/memory/runner.go`
- `internal/hooks/runner.go`
- `internal/observability/agent.go`

These wrappers can run before the core model turn emits text/thinking/tool
events. When they do, the TUI can still appear to be "waiting for model" even
though it is actually running memory or hook work.

### Core agent model turn

The core agent builds an Ollama chat request in `internal/agent/stream.go`.

It then calls:

```go
stream, err := a.client.Chat(ctx, req)
```

Only after this call returns does the agent wrap the stream with the watchdog.

Relevant code:

- `internal/agent/stream.go`
- `internal/llm/watchdog.go`
- `internal/llm/ollama/ollama.go`

This means the current watchdog does not measure or warn about time spent before
Ollama returns the HTTP response stream.

## Findings

### 1. `Waiting for model...` hides multiple stages

The current UI state does not distinguish between:

- running memory recall;
- running prompt hooks;
- sending request to Ollama;
- waiting for Ollama response headers;
- waiting for first model token;
- prompt prefill.

This makes the status misleading during slow startup.

Impact:

- users cannot tell whether the app is stuck;
- users cannot tell whether Ollama, context building, hooks, or memory caused
  the delay;
- debugging first-token latency is harder than necessary.

### 2. Chat `num_ctx` can be extremely large

The bootstrap default sets `NumCtx` to `1000000`.

Relevant code:

- `internal/bootstrap/state.go`

The REPL initializes `startupNumCtx` from that snapshot and only replaces it
with the live model limit when `startupNumCtx == 0`.

Relevant code:

- `internal/cli/repl.go`

The agent later passes the effective context window to Ollama as:

```go
Options: map[string]any{
    "num_predict": outputTokenBudget,
    "num_ctx":     effectiveNumCtx,
}
```

Relevant code:

- `internal/agent/stream.go`

Why this matters:

- a very large context window can cause expensive KV-cache allocation;
- model prefill can be much slower;
- local machines may appear stuck while Ollama prepares the request;
- the first streamed token can be delayed substantially.

This is the highest-likelihood performance issue found in the current code.

### 3. Chat requests do not set `keep_alive`

The Ollama request type supports `keep_alive`.

Relevant code:

- `internal/llm/ollama/ollama.go`

Semantic embedding calls already pass keep-alive through embedding options.

Relevant code:

- `internal/semantic/service.go`
- `internal/semantic/embedder.go`

But the normal chat request built by the agent does not set `KeepAlive`.

Relevant code:

- `internal/agent/stream.go`

Why this matters:

- Ollama may unload the chat model between runs;
- each prompt can pay cold-start/model-load cost again;
- the user sees this as a long `Waiting for model...` period.

### 4. The stream watchdog starts too late

The watchdog is attached only after `a.client.Chat` returns a stream.

Relevant code:

- `internal/agent/stream.go`
- `internal/llm/watchdog.go`

But the Ollama client performs the HTTP request before returning:

```go
resp, err := c.httpClient.Do(httpReq)
```

Relevant code:

- `internal/llm/ollama/ollama.go`

If Ollama takes a long time before returning response headers, the watchdog has
not started yet. That means no idle warning is emitted during that phase.

### 5. Large prompt packing can increase first-token latency

Prompt packing and semantic retrieval happen before `ActiveRun` is set in the
TUI path, so their measured durations can be shown as slow-stage notices.

However, once the packed prompt is submitted, Ollama still needs to evaluate the
entire prompt before the first useful token. With large directory/file context
and semantic retrieval context, that prefill phase can be long.

Relevant code:

- `internal/tui/app.go`
- `internal/contextpack`
- `internal/semantic`

### 6. Memory recall can run before the core agent emits anything

Memory recall is enabled by default and wraps the agent runner.

Relevant code:

- `internal/config/defaults.go`
- `internal/memory/runner.go`
- `internal/cli/repl.go`

The default memory recall mode is `fast`, which is lexical and should usually be
cheap. If set to `llm`, memory recall performs a separate LLM side query with a
3-second timeout.

Relevant code:

- `internal/memory/types.go`
- `internal/memory/recall.go`

Even when it is not the main bottleneck, it is another stage hidden behind
`Waiting for model...`.

### 7. Hooks can run before the core agent emits anything

The hooks runner dispatches `SessionStart` and `UserPromptSubmit` before
calling the next runner.

Relevant code:

- `internal/hooks/runner.go`
- `internal/hooks/dispatch.go`

If project/user hooks are slow, users may still see `Waiting for model...`.

## Historical Possible Solutions To Explore

This section predates the locked router-first plan. Treat it as latency analysis
background only.

### Option 1: Reduce or normalize default `num_ctx`

Replace the hardcoded `NumCtx: 1000000` default with a safer behavior.

Possible approaches:

- set default `NumCtx` to `0` and let live model metadata decide;
- set default `NumCtx` to a conservative value such as `32768`;
- clamp requested `num_ctx` to model-reported `context_length`;
- only allow very large `num_ctx` when explicitly configured by the user.

Recommended direction:

- use `0` as "auto";
- after `ShowModel`, use `ComputeLimits(details).NumCtx`;
- if metadata is unavailable, fall back to static `ModelCapabilities`;
- never silently request `1000000`.

Expected impact:

- faster model request startup;
- less memory pressure;
- fewer long waits before first token.

Risk:

- users who intentionally want huge context may need an explicit config knob.

### Option 2: Send chat `keep_alive`

Add `KeepAlive` to the agent chat request.

Possible implementation:

- add `KeepAlive` to `agent.Config`;
- initialize it from bootstrap/config;
- set `req.KeepAlive` in `executeOneTurn`;
- ensure local and cloud behavior are both acceptable.

Expected impact:

- fewer model cold starts;
- shorter repeated-run latency;
- better perceived responsiveness.

Risk:

- keeping large models loaded uses RAM/VRAM longer;
- should remain configurable.

### Option 3: Split `Waiting for model...` into more precise phases

Introduce explicit pre-first-token UI stages.

Candidate stages:

- `Preparing context...`
- `Running hooks...`
- `Loading memory...`
- `Sending prompt to model...`
- `Waiting for first token...`
- `Evaluating prompt...`

Possible implementation:

- add new agent events such as:
  - `RunStageStarted`
  - `RunStageCompleted`
  - `LLMRequestStarted`
  - `LLMStreamOpened`
  - `LLMFirstToken`
- update TUI run state to display the latest active stage while `ActiveRun` is
  true and no stream delta has arrived;
- keep current `Waiting for model...` as fallback only.

Expected impact:

- users know what the app is doing;
- support/debugging becomes much easier;
- slow hooks/memory/model-load/prefill are distinguishable.

Risk:

- requires event model changes and tests across TUI/server if both surfaces need
  parity.

### Option 4: Start request-level watchdog before `client.Chat`

Add timing and warning coverage around the entire LLM request, not only the
stream channel.

Possible implementation:

- emit `LLMRequestStarted` immediately before `a.client.Chat`;
- start a timer before `a.client.Chat`;
- emit a warning if `client.Chat` has not returned after threshold;
- distinguish:
  - request open latency;
  - stream first-token latency;
  - total generation duration.

Expected impact:

- long model-load/header waits become visible;
- users see warnings before the stream exists;
- watchdog telemetry matches what the user experiences.

Risk:

- cancellation behavior must be handled carefully to avoid aborting legitimate
  cloud requests too aggressively.

### Option 5: Improve first-token observability

The observability wrapper already records first-token latency after seeing a
first-token signal.

Relevant code:

- `internal/observability/llm.go`

Possible improvements:

- expose current-run first-token latency in the TUI status line;
- include prompt token count and `num_ctx` in debug/status output;
- emit stage summary when first-token latency exceeds threshold;
- include model name, provider, `num_ctx`, `num_predict`, and prompt size in
  slow-stage diagnostics.

Expected impact:

- easier to identify prompt/context/model causes;
- better regression data for future latency work.

### Option 6: Add prompt size and context settings to status/debug output

When a run starts, show or log:

- estimated prompt tokens;
- packed context bytes;
- semantic context bytes;
- `num_ctx`;
- `num_predict`;
- model/provider;
- whether thinking is enabled;
- whether keep-alive is set.

Possible places:

- transcript system notice when debug mode is enabled;
- `/debug status` or `/status`;
- prompt dump metadata.

Expected impact:

- users and agents can diagnose slow starts without guessing.

Risk:

- too much transcript noise if always displayed.

### Option 7: Avoid thinking-state confusion

Clarify that `Thinking...` only appears for models that actually stream thinking
deltas.

Possible implementation:

- if a model is not thinking-capable, show `Streaming...` after first content
  delta without implying thinking is expected;
- expose a model capability badge or status field;
- review `ModelCapabilities` for currently used models.

Expected impact:

- fewer false expectations that the app should always enter `Thinking...`.

Risk:

- hardcoded capability matrix can drift from Ollama model behavior.

## Historical Latency-Only Implementation Order

This section predates the locked router-first next phase. Keep it as latency
background only; agents should follow `Parallel Implementation Task Plan` for
the next implementation.

### Phase A: Diagnostics First

Implement low-risk instrumentation before changing behavior:

1. emit `LLMRequestStarted` before `client.Chat`;
2. emit `LLMStreamOpened` after `client.Chat` returns;
3. emit first-token/first-event timings;
4. display request-stage status in TUI;
5. include `num_ctx`, `num_predict`, model, provider, prompt estimate, and
   context bytes in debug metadata.

Acceptance criteria:

- a long delay before response headers shows as `Sending prompt to model...` or
  `Waiting for model response...`;
- a long delay after response headers but before first token shows as
  `Waiting for first token...`;
- slow stage summary identifies which stage exceeded the threshold.

### Phase B: Fix The Most Likely Latency Source

Normalize `num_ctx`.

Recommended behavior:

- `NumCtx == 0` means auto;
- live model metadata wins when available;
- static capability table is fallback;
- explicit user config can request larger context;
- requested context is clamped to known model maximum unless explicitly allowed.

Acceptance criteria:

- default chat requests no longer send `num_ctx=1000000`;
- tests cover default, model metadata, static fallback, and explicit override;
- prompt packing still receives a coherent effective context budget.

### Phase C: Add Chat Keep-Alive

Wire `keep_alive` into chat requests.

Acceptance criteria:

- chat request JSON includes configured `keep_alive`;
- default remains configurable;
- docs explain memory/RAM tradeoff;
- repeated prompts avoid unnecessary cold starts where Ollama honors keep-alive.

### Phase D: UI Phase Refinement

Replace broad pre-token waiting state with precise stage labels.

Acceptance criteria:

- status line differentiates context prep, hooks, memory, model request, stream
  open, first-token wait, thinking, streaming, tools, permissions, retry, and
  compaction;
- tests cover phase priority;
- server/web UI receives equivalent status events if applicable.

## How To Confirm The Current Bottleneck

Run with prompt dumps or observability enabled and compare:

- time from prompt submit to `LLMRequestStarted`;
- time from `LLMRequestStarted` to `LLMStreamOpened`;
- time from `LLMStreamOpened` to first token/thinking/tool event;
- requested `num_ctx`;
- packed prompt size.

If the delay is mostly before stream open, suspect:

- model cold start;
- huge `num_ctx`;
- Ollama loading/allocation;
- network/cloud response-header wait.

If the delay is after stream open but before first token, suspect:

- prompt prefill;
- large packed context;
- semantic context size;
- model reasoning latency.

If the delay is before `LLMRequestStarted`, suspect:

- hooks;
- memory recall;
- context packing;
- semantic retrieval.

## Immediate Workarounds

Until code changes land:

- use a smaller model or a smaller configured context window;
- avoid `context_mode=max` for normal prompts;
- reduce large explicit `@folder` mentions when not needed;
- use `@folder?tree` for listing-only requests;
- keep memory recall in `fast` mode rather than `llm`;
- check whether hooks are configured and slow;
- keep Ollama model loaded manually if repeated cold starts are observed.

## Historical Latency-Only Recommendation

This recommendation predates the owner-confirmed router-first plan. It is
retained to explain the original latency analysis, not to override the locked
implementation order.

Prioritize these fixes:

1. add pre-stream LLM request instrumentation;
2. stop sending huge default `num_ctx`;
3. wire chat `keep_alive`;
4. split the TUI status into precise pre-token stages.

This sequence gives immediate diagnostic value, likely improves latency, and
prevents future reports from collapsing unrelated bottlenecks into the same
`Waiting for model...` label.
