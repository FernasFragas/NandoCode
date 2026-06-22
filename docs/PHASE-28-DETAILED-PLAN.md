# Phase 28 - Semantic Workspace Index And Embedding Retrieval

Date: 2026-05-26
Status: Implemented (MVP)
Priority: Completed prerequisite for Phase 29 and Phase 25
Workstream: Context retrieval, project-scale coding assistance

## Roadmap Directive

Phase 28 is implemented as an MVP. Phase 29 TUI Semantic Index Progress
Observability is also implemented as an MVP. The next feature implementation
stage is Phase 25 Remote / Bridge Mode, which should build on the completed
Phase 28/29 semantic retrieval and index-progress behavior.

Phase 28 changed prompt assembly, model configuration, local cache/state layout,
indexing behavior, run observability, CLI/TUI control surfaces, and release
validation. It must remain complete before packaging or final hardening.

The MVP is not "an Embed method exists." The MVP is an end-to-end loop:

1. Build a local semantic index for a workspace.
2. Retrieve semantically relevant records for a normal user prompt.
3. Read current snippets from disk, pack cited evidence into the prompt, and
   preserve the original user request.
4. Let the existing generation LLM/tool loop use that evidence to act.
5. Degrade cleanly when the model, index, or fresh evidence is unavailable.

## Implementation Snapshot (2026-05-26)

Phase 28 MVP is now implemented in this repository.

Delivered in code:

- Ollama embedding path migrated to `POST /api/embed` with batch embedding
  support and optional dimensions/truncate/keep_alive/options.
- New `internal/semantic` package with:
  - contracts, config, scanner, filters, record extraction, vector math,
    persistence store, retrieval/ranking, and rendered semantic evidence.
- CLI surface for index lifecycle:
  - `nandocodego index build|refresh|status|clear`
- TUI surface for semantic/index control:
  - `/semantic on|off|status`
  - `/index build|refresh|status|clear`
- Prompt-time retrieval integration in:
  - REPL/TUI prompt submit flow
  - server session prompt flow (including retrieval SSE events)
- Config integration through `[semantic_index]` defaults and loader support.

Validation completed:

- `go test ./...`
- `./tools/check-allowed-deps.sh`
- `./tools/check-network-policy.sh`

Notes:

- This file remains the full design/coordination spec; many checklist items
  below are historical planning tasks and not a live TODO list anymore.

## Review Outcome

The first proposal had the right architecture, but it was not yet detailed
enough for several agents to implement safely at the same time. This revision
adds:

- contract-first sequencing before broad implementation
- explicit file ownership by workstream
- PR order and merge gates
- shared interfaces and data structures that agents must not casually change
- resolved product decisions for the first implementation
- a test/eval matrix tied to each agent lane
- integration points for TUI, server, CLI, agent prompt assembly, and analysis
  workflow

The most important coordination rule is: stabilize interfaces and fixtures
first, then parallelize. Without that, every agent will need to edit the same
types, fakes, and prompt assembly code.

## Goal

Turn the dormant embedding capability into an active workspace retrieval system.

The application should be able to scan a codebase or selected folders, convert
files, folders, symbols, and documentation sections into vector embeddings with
an Ollama embedding model, and use those vectors during normal prompts to find
semantically relevant context before the main LLM starts work.

Primary target model:

- `qwen3-embedding:8b`
- Ollama pull command: `ollama pull qwen3-embedding:8b`
- Ollama Embed API endpoint: `POST /api/embed`

Expected user-facing behavior:

```text
User: Fix the authentication bug

1. The app embeds the prompt with the configured embedding model.
2. The local semantic index returns relevant files, functions, docs, and line
   ranges even when the exact words "authentication bug" are absent.
3. The app reads the current file snippets from disk, packs them into the
   prompt as cited context, and sends the original user request plus retrieved
   evidence to the generation LLM.
4. The generation LLM reasons over the retrieved context and uses the existing
   tool/edit/terminal flow to inspect, modify, and verify the workspace.
```

This phase upgrades context selection. It does not make the embedding model the
agent. The embedding model retrieves; the configured chat/code LLM still plans,
writes code, asks for permissions, runs commands, and edits files.

## Post-MVP Retrieval Activation Policy

The initial MVP made semantic retrieval available during normal prompt handling.
Post-MVP hardening must make retrieval intent-aware instead of simply running
because an index exists.

Book-aligned rule:

Use embeddings for broad semantic workspace discovery. Do not use embeddings
when the application already has exact context or when a deterministic local
index is the better tool.

Policy matrix:

| Prompt or feature path | Semantic retrieval policy |
| --- | --- |
| `@file can you access this?` | Skip |
| `summarize @file` | Skip by default |
| `what is the status of @file` | Skip or cap hard |
| `list files in @dir` or path search | Skip |
| memory recall/user preferences | Skip |
| broad bug/fix/refactor prompt with no exact files | Run |
| `find code related to auth/session/token bug` | Run |
| `@file find related utilities/usages/dependencies` | Run, but bounded and current-path weighted |

A built semantic index means retrieval is available; it must not mean every
non-listing prompt pays the retrieval and prompt-growth cost. The detailed
latency/regression rationale and acceptance criteria live in
`docs/WAITING-FOR-MODEL-LATENCY-REPORT.md`.

## Official API And Model Facts

Facts verified from official Ollama pages on 2026-05-26:

- The current Ollama embedding endpoint is `POST /api/embed`.
- `/api/embed` accepts a string or array of strings as `input`.
- `/api/embed` returns `embeddings` as `number[][]`.
- Request fields include `model`, `input`, `truncate`, `dimensions`,
  `keep_alive`, and `options`.
- `qwen3-embedding:8b` is an embedding model in the Qwen3 Embedding family.
- The Ollama model page lists the artifact at about 4.7 GB with Q4_K_M
  quantization and 7.57B parameters.
- The model card describes 32k context and output dimensions up to 4096, with
  user-defined output dimensions supported.

References:

- `https://ollama.com/library/qwen3-embedding:8b`
- `https://docs.ollama.com/api/embed`

Implementation implication:

The existing Ollama client currently implements `Embed` against
`/api/embeddings` one input at a time. Phase 28 should add or migrate to the
current `/api/embed` batch shape before building the indexer.

## Non-Goals

- No external vector database in this phase. Store the index locally under the
  normal nandocodego cache/state directories.
- No cloud embedding service requirement.
- No replacement for the Phase 8 memory system. Memory remains transparent,
  file-based, and LLM-recalled. Semantic indexing is for workspace files.
- No hidden whole-repo prompt dump. Retrieved context must be budgeted,
  cited, and visibly packed.
- No permission bypass. Indexing and retrieval must obey the same path safety
  and exclusion rules as normal file reads.
- No automatic edit based only on vector search. The generation LLM must still
  reason over concrete retrieved snippets and use normal tools.
- No large dependency on a vector database, server, or native ANN library unless
  a later performance phase proves linear scan is insufficient.

## Current State

Embedding exists but is not active application behavior:

- `internal/llm/types.go` defines `Client.Embed(...)`.
- `internal/llm/ollama/ollama.go` implements embeddings for Ollama.
- `internal/llm/router.go` and `internal/observability/llm.go` forward
  embedding calls.
- Tests and fakes implement `Embed` to satisfy the interface.

Runtime retrieval does not use embeddings today:

- `internal/memory/recall.go` uses a chat side-query over memory metadata.
- `internal/analysis/retrieval.go` ranks files with lexical path terms plus
  frecency.
- `internal/analysis/chunker.go` provides text chunking that can be reused.
- `internal/tools/dirwalk/walk.go` and `internal/tui/fileindex/index.go`
  provide a workspace walking foundation.

Phase 28 should build on those pieces instead of creating a separate indexing
world.

## Existing Code Touchpoints

Agents should inspect these files before implementation:

| Area | Files | Why it matters |
|---|---|---|
| LLM interface | `internal/llm/types.go`, `internal/llm/router.go`, `internal/llm/ollama/ollama.go` | Existing `Embed` surface and Ollama HTTP behavior |
| Model/runtime plumbing | `internal/llm/modelruntime/service.go`, `internal/llm/modelresolver/resolver.go`, `internal/llm/capabilities.go` | Model availability, defaults, capability diagnostics |
| Config | `internal/config/config.go`, `internal/config/defaults.go`, `internal/config/loader.go` | Semantic config must load consistently |
| Paths/cache | `internal/paths/paths.go` | Index location must use existing path policy |
| Directory walking | `internal/tools/dirwalk/walk.go`, `internal/tui/fileindex/index.go` | Reuse excludes and workspace traversal |
| Frecency | `internal/tui/fileindex/frecency.go` | Hybrid score input |
| Analysis retrieval | `internal/analysis/retrieval.go`, `internal/analysis/workflow.go`, `internal/analysis/chunker.go` | Existing lexical/project-analysis path to augment |
| Prompt packing | `internal/agent/prompt_packer.go`, `internal/agent/stream.go`, `internal/agent/input.go` | Semantic context must be budgeted before generation |
| Mention expansion | `internal/mentions/expand.go`, `internal/mentions/line_range.go` | Explicit context priority and line-range handling |
| TUI submit path | `internal/tui/app.go`, `internal/tui/bridge.go`, `internal/tui/slash.go` | Prompt-time retrieval and slash commands |
| Server API | `internal/server/handler.go`, `internal/server/session.go`, `internal/server/types.go` | Server prompt path and event payloads |
| CLI | `internal/cli/root.go`, `internal/cli/doctor.go` | `index` command and doctor checks |
| Observability | `internal/observability/*`, `internal/logging/*` | Metrics, redaction, no raw content logs |

Touchpoint rule:

Semantic indexing should be a new package plus narrow integrations. If an agent
finds itself rewriting agent loops, TUI state, server sessions, or analysis
workflow wholesale, stop and split the change.

## Product UX

### Default Behavior

Semantic retrieval should be available by default once an index exists, but it
must not make a fresh checkout noisy or slow.

Recommended default for initial landing:

- Semantic retrieval is enabled in "use-if-index-exists" mode.
- The app never auto-builds a full index during a normal prompt.
- If the index exists and matches config, prompts automatically use semantic
  retrieval.
- If the index is missing, the app falls back to existing lexical/frecency
  retrieval and may show a one-time hint to run `/index build`.
- If the index is stale, the app can refresh a small changed-file set within a
  short prompt-time budget, otherwise it falls back and asks the user to run
  `/index refresh`.
- Explicit `@file` and `@dir` mentions always outrank semantic retrieval.
- `/semantic off` disables prompt-time retrieval without deleting the index.
- `index clear` deletes the local index for the workspace.

### Commands

Add CLI commands:

```text
nandocodego index build [path]
nandocodego index refresh [path]
nandocodego index status [path]
nandocodego index clear [path]
```

Add slash commands:

```text
/index build
/index refresh
/index status
/semantic on
/semantic off
```

The first implementation can make slash commands thin wrappers over shared
index service methods. The CLI and TUI must report:

- model name
- dimensions
- indexed root
- file count
- record count
- vector count
- skipped files
- last build time
- stale file count
- whether semantic retrieval is enabled for prompts

### Prompt Status

When semantic retrieval runs, the TUI/server event stream should expose a stage
similar to:

```text
semantic retrieval: 18 records from 7 files
```

The user should not need to understand embeddings to trust the result. Show
file paths and line ranges in prompt/debug views so retrieved context is
auditable.

## Configuration

Add a semantic index config block:

```toml
[semantic_index]
enabled = true
auto_build = false
model = "qwen3-embedding:8b"
dimensions = 1024
max_chunk_tokens = 700
chunk_overlap_tokens = 120
max_file_bytes = 1048576
max_records = 200000
batch_size = 32
top_k_records = 40
top_k_files = 12
max_context_bytes = 262144
hybrid_lexical_weight = 0.20
frecency_weight = 0.10
prompt_refresh_max_files = 8
prompt_refresh_timeout_ms = 1500
keep_alive = "10m"
store_previews = true
```

Default dimensions should be lower than the model maximum. `4096` dimensions
is highest quality but creates large indexes. `1024` is the recommended first
default because it keeps local disk and memory costs reasonable while still
using a capable embedding model. Users can raise this for smaller repositories
or better retrieval quality.

Config rules:

- Changing model, dimensions, chunk size, overlap, or record schema invalidates
  the index.
- `enabled=false` must keep current app behavior unchanged and skip prompt-time
  index checks.
- `enabled=true` does not build an index by itself. It means "use the semantic
  index if a compatible one exists."
- `auto_build=false` means normal prompts never trigger full indexing.
- Missing model must produce a clear diagnostic with the exact pull command.
- `doctor` should report whether semantic retrieval is configured, model is
  installed, and index metadata is readable.

## Architecture

Add a new package:

```text
internal/semantic/
  contracts.go       - stable public types and small interfaces
  config.go          - defaults and validation
  service.go         - index build, refresh, retrieve orchestration
  embedder.go        - llm.Client adapter and batching
  scanner.go         - workspace scan and file filtering
  filters.go         - generated/vendor/binary/secret filtering
  records.go         - file/folder/symbol/doc/chunk record types
  symbols_go.go      - Go symbol extraction via go/parser
  symbols_generic.go - generic symbol/heading extraction
  store.go           - local vector index persistence
  vectors.go         - float32 read/write, normalization, dot product
  search.go          - cosine similarity, hybrid scoring, diversification
  render.go          - prompt evidence rendering
  stale.go           - content hash and incremental invalidation
  testutil/          - fake embedders, fixture helpers, deterministic vectors
```

The package should be independent from TUI code. TUI, server, CLI, and analysis
workflow code should call it through a small service interface.

### Contract-First Rule

The first implementation PR must create compiling, tested contracts before
other agents start broad work. These contracts are allowed to be incomplete
internally, but the public types must be stable enough that scanner, store,
search, prompt integration, and UX agents can work in parallel.

Minimum first-PR contracts:

```go
package semantic

type Config struct {
    Enabled                 bool
    AutoBuild               bool
    Model                   string
    Dimensions              int
    MaxChunkTokens          int
    ChunkOverlapTokens      int
    MaxFileBytes            int64
    MaxRecords              int
    BatchSize               int
    TopKRecords             int
    TopKFiles               int
    MaxContextBytes         int
    HybridLexicalWeight     float64
    FrecencyWeight          float64
    PromptRefreshMaxFiles   int
    PromptRefreshTimeout    time.Duration
    KeepAlive               string
    StorePreviews           bool
}

type Service interface {
    Status(ctx context.Context, root string) (Status, error)
    Build(ctx context.Context, req BuildRequest) (BuildReport, error)
    Refresh(ctx context.Context, req RefreshRequest) (BuildReport, error)
    Clear(ctx context.Context, root string) error
    Retrieve(ctx context.Context, req RetrieveRequest) (RetrieveResult, error)
}

type Embedder interface {
    Embed(ctx context.Context, req EmbedRequest) (EmbedResult, error)
}

type Store interface {
    LoadManifest(ctx context.Context, root string) (Manifest, error)
    LoadRecords(ctx context.Context, manifest Manifest) ([]Record, error)
    LoadVectors(ctx context.Context, manifest Manifest) (VectorSet, error)
    Replace(ctx context.Context, manifest Manifest, records []Record, vectors VectorSet) error
    Clear(ctx context.Context, root string) error
}
```

The exact method names can change in the first PR, but after that PR merges,
agents should treat the contracts as frozen unless a blocking issue is found.
Any contract change after the first PR must update all fakes, tests, CLI/TUI
callers, and this phase plan in the same PR.

### Shared Error Contract

Semantic retrieval must return typed errors or warnings that callers can render
without string matching:

```go
var (
    ErrDisabled       = errors.New("semantic index disabled")
    ErrIndexMissing   = errors.New("semantic index missing")
    ErrIndexStale     = errors.New("semantic index stale")
    ErrModelMissing   = errors.New("semantic embedding model missing")
    ErrSchemaMismatch = errors.New("semantic index schema mismatch")
)
```

Prompt-time callers should treat `ErrDisabled`, `ErrIndexMissing`,
`ErrIndexStale`, and `ErrModelMissing` as non-fatal fallback conditions.
Index build commands should treat `ErrModelMissing` as actionable failure and
print:

```text
ollama pull qwen3-embedding:8b
```

### Progress Event Contract

Indexing and retrieval should emit structured progress events so the TUI,
server, CLI, and tests do not invent separate status text.

```go
type Event struct {
    Stage       Stage
    Message     string
    Root        string
    Path        string
    FilesSeen   int
    FilesDone   int
    RecordsDone int
    BatchesDone int
    TotalBatches int
    Duration     time.Duration
}
```

Minimum stages:

- `scan_start`
- `scan_progress`
- `extract_progress`
- `embed_progress`
- `write_start`
- `write_done`
- `retrieve_start`
- `retrieve_query_embed`
- `retrieve_search`
- `retrieve_render`
- `retrieve_done`

Callers may display shorter messages, but the event fields are the testable
contract.

### Data Flow

```text
Index build/refresh
   dirwalk.Walk(root)
      -> filter files
      -> read text
      -> content hash
      -> extract records
         - file record
         - folder record
         - function/symbol records
         - doc heading records
         - fallback chunk records
      -> batch Embed(model, inputs, dimensions)
      -> normalize vectors
      -> persist metadata + vectors

Prompt retrieval
   user prompt
      -> Embed(query)
      -> vector top-k search
      -> lexical/frecency hybrid boost
      -> diversify by file/path/kind
      -> stale check
      -> read exact current snippets from disk
      -> pack cited evidence under budget
      -> generation LLM receives original prompt + semantic context
```

### Record Types

Use one common record struct:

```go
type Record struct {
    ID          string
    Kind        RecordKind // file, folder, symbol, doc_section, chunk
    Path        string
    Language    string
    Name        string
    Parent      string
    StartLine   int
    EndLine     int
    ContentHash string
    TextHash    string
    TextPreview string
    EmbedText   string
    EstTokens   int
    Generated   bool
    Skipped     bool
    SkipReason  string
}
```

Embedding text should be short, structured, and purpose-built for retrieval.
Do not embed raw giant files as one input.

`TextPreview` is for status/debug output and should be bounded. `EmbedText` is
the text sent to the embedding model during indexing and does not need to be
persisted when `store_previews=false`. Prompt evidence must be read from the
current file on disk by `Path`, `StartLine`, and `EndLine`.

Build, status, and retrieval types should be explicit enough for agents to test
without reaching into implementation internals:

```go
type Status struct {
    Root             string
    Exists           bool
    Compatible       bool
    Enabled          bool
    Model            string
    Dimensions       int
    SchemaVersion    int
    FileCount        int
    RecordCount      int
    VectorCount      int
    StaleFileCount   int
    SkippedFileCount int
    UpdatedAt        time.Time
    IndexPath        string
    Warnings         []string
}

type BuildReport struct {
    Root             string
    Model            string
    Dimensions       int
    FilesSeen        int
    FilesIndexed     int
    FilesSkipped     int
    RecordsIndexed   int
    EmbedBatches     int
    Duration         time.Duration
    IndexPath        string
    Skipped          []SkippedFile
    Warnings         []string
}

type RetrieveRequest struct {
    Root              string
    Query             string
    ExplicitPaths     []string
    CurrentTurnPaths  []string
    MaxRecords        int
    MaxFiles          int
    MaxContextBytes   int
}

type RetrieveResult struct {
    Used              bool
    FallbackReason    string
    Records           []SearchHit
    Files             []RetrievedFile
    RenderedContext   string
    ContextBytes      int
    StaleDropped      int
    Warnings          []string
}
```

Examples:

```text
kind: symbol
path: internal/server/auth.go
language: go
symbol: validateBearerToken
signature: func validateBearerToken(...)
comments: ...
body excerpt: ...
```

```text
kind: folder
path: internal/server
children: auth.go, handler.go, permission.go, session.go
readme/summary: HTTP API, SSE session handling, auth, permissions
```

```text
kind: doc_section
path: docs/PHASE-21-DETAILED-PLAN.md
heading: Authentication and server security
text: ...
```

### Symbol Extraction

Phase 28 should support Go well because this repository is Go:

- Use `go/parser` and `go/ast` for Go files.
- Index functions, methods, types, interfaces, constants with comments,
  signatures, and bounded body excerpts.
- Store exact line ranges from the token file set.

For non-Go files, avoid a new parser dependency in this phase:

- Markdown: heading-based sections.
- JSON/YAML/TOML: top-level keys and bounded chunks.
- Shell/JS/TS/Python/Ruby: simple function/class regexes plus fallback chunks.
- Unknown text: chunk by line/token budget.

Generated, vendored, dependency, binary, and huge files should be skipped by
default.

## Local Index Store

Use a local store keyed by workspace root, model, dimensions, and schema
version.

Suggested layout:

```text
<cache>/semantic/<workspace-id>/
  manifest.json
  records.jsonl
  vectors.f32
  deleted.jsonl
  lock
```

`manifest.json`:

```json
{
  "schema_version": 1,
  "workspace_root": "/abs/path",
  "workspace_id": "...",
  "model": "qwen3-embedding:8b",
  "dimensions": 1024,
  "record_count": 18432,
  "file_count": 1735,
  "created_at": "2026-05-26T00:00:00Z",
  "updated_at": "2026-05-26T00:00:00Z"
}
```

`vectors.f32` stores little-endian float32 vectors in record order. Vectors are
L2-normalized at write time so cosine similarity is a dot product at search
time.

Use JSONL records for inspectability and debugging. If this becomes too slow
for large repositories, a later phase can add a compact binary metadata store or
ANN index. Do not add that complexity first.

### Index Size Budget

At 1024 dimensions:

- 1 vector is 4096 bytes.
- 10,000 records is about 39 MiB of vector data.
- 50,000 records is about 195 MiB of vector data.

At 4096 dimensions:

- 1 vector is 16,384 bytes.
- 50,000 records is about 781 MiB of vector data.

This is why `dimensions=1024` is the recommended default.

## Embedding Client Changes

Update `llm.Client.Embed` support so the Ollama implementation can batch:

Current shape:

```go
Embed(ctx context.Context, model string, input []string) ([][]float32, error)
```

This method can remain, but the Ollama implementation should use the current
`/api/embed` endpoint and send all inputs as a single batch when possible.

Add an option-bearing method only if needed:

```go
type EmbedOptions struct {
    Dimensions int
    Truncate   *bool
    KeepAlive  string
}

type Embedder interface {
    Embed(ctx context.Context, model string, input []string, opts EmbedOptions) ([][]float32, error)
}
```

Compatibility approach:

1. Keep `llm.Client.Embed(...)` for existing interface compatibility.
2. Add a semantic package adapter that uses an optional extended interface when
   available.
3. Implement the extended interface in the Ollama client.
4. Make the old `Embed(...)` call the extended method with zero-value options.

Embedding requests for indexing should use:

- `input: []string`
- `dimensions: config.dimensions`
- `truncate: false`
- `keep_alive: config.keep_alive`

If `truncate:false` errors because a record is too long, that is a chunking bug;
the indexer should split the record and retry rather than silently accepting
truncated embeddings.

## Retrieval And Prompt Packing

Semantic retrieval should run after the user submits a prompt and before the
generation LLM call is built.

Priority order for context:

1. Explicit current-turn `@file`, `@dir`, and line-range mentions.
2. Files already opened or edited in the current turn.
3. Semantic retrieval from the workspace index.
4. Existing lexical/frecency retrieval as a fallback or hybrid boost.
5. Memory recall.
6. Older conversation history.

Memory remains separate because it answers "what durable preferences/facts
should I remember?" Semantic retrieval answers "which workspace code/docs are
relevant right now?"

### Hybrid Ranking

Use vector similarity as the base score, then add small boosts:

```text
score =
  vector_cosine
  + lexical_weight * lexical_score
  + frecency_weight * frecency_score
  + explicit_scope_boost
```

Use Maximal Marginal Relevance or a simpler file/path diversification rule so
the top results do not all come from the same large file.

Recommended first-pass limits:

- Search top 200 records by vector score.
- Keep top 40 records after hybrid score.
- Keep at most 12 files.
- Keep at most 4 snippets per file.
- Pack at most 256 KiB of semantic context into the generation prompt.

### Evidence Rendering

Render retrieved context as a clearly labeled block:

```xml
<semantic_context model="qwen3-embedding:8b" dimensions="1024" records="18" files="7">
<evidence path="internal/server/auth.go" kind="symbol" name="validateToken" lines="42-88" score="0.82">
...
</evidence>
<evidence path="docs/WEB-UI-UX-PRODUCT-PLAN.md" kind="doc_section" name="Auth" lines="120-168" score="0.74">
...
</evidence>
</semantic_context>
```

Read evidence from the current filesystem immediately before rendering, not
from stale embedded text in the index. If a file hash changed, re-index that
file synchronously within a small budget or drop it and report it as stale.

The original user prompt must remain the authoritative task. Semantic context
is evidence, not a rewritten instruction.

## Indexing Workflow

### Build

`index build` creates a fresh index:

1. Resolve and validate root.
2. Acquire workspace index lock.
3. Walk files with `dirwalk.Walk`.
4. Apply excludes and binary/size filters.
5. Extract records.
6. Batch embeddings.
7. Normalize and write vectors.
8. Atomically replace manifest/records/vectors.
9. Emit summary and skipped-file diagnostics.

### Refresh

`index refresh` updates incrementally:

1. Load manifest and record metadata.
2. Walk current files.
3. Hash candidate files.
4. Delete records for removed files.
5. Re-embed records for changed files.
6. Add records for new files.
7. Rewrite compacted vectors and records.

The first implementation can rewrite the full vector file after incremental
metadata decisions. Optimizing append/compaction can wait.

### Prompt-Time Staleness

At prompt time:

- If no index exists: skip semantic retrieval and optionally show a hint.
- If manifest schema/model/dimensions mismatch config: skip and show stale
  status.
- If fewer than a small threshold of files are stale: refresh those files within
  a short deadline.
- If many files are stale: skip semantic retrieval and ask user to run
  `/index refresh`.

Do not block every normal prompt on a full repository rebuild.

## Security And Privacy

Local embedding does not remove security concerns. The index stores derived
representations of source files and text previews.

Rules:

- Store indexes only under nandocodego cache/state roots.
- Never index files denied by path policy.
- Never index `.env`, secret files, key material, credentials, private SSH/GPG
  directories, or files matching existing sensitive path filters.
- Reuse or extend `tools/check-network-policy.sh` so semantic indexing cannot
  introduce unexpected network calls.
- Do not log raw file contents, raw prompts, or embedding vectors at INFO.
- Show index location in `doctor` so users can delete it.
- `index clear` must remove the local semantic index for the workspace.

Default skip patterns:

- directories: `.git`, `.hg`, `.svn`, `node_modules`, `vendor`, `dist`,
  `build`, `out`, `target`, `coverage`, `.next`, `.nuxt`, `.cache`,
  `.gocache`, `.tmp-config`, `.terraform`
- files: `.env`, `.env.*`, `*.pem`, `*.key`, `*.p12`, `*.pfx`, `id_rsa`,
  `id_ed25519`, `known_hosts`, `credentials`, `credentials.json`,
  `secrets.json`, `*.sqlite`, `*.db`, `*.db-wal`
- generated/minified artifacts: `*.min.js`, `*.map`, `go.sum` can be indexed
  only as a bounded file record, generated files with standard generated-code
  headers should be skipped unless explicitly included later
- binary/media/archive: images, video, audio, fonts, archives, object files,
  executables, PDFs unless a later text-extraction phase is added

Secret filtering rule:

If a text file contains obvious private-key delimiters or high-confidence
credential markers in the first scan window, skip the whole file and record
`SkipReason="secret-like"`. Do not store a preview for skipped secret-like
files.

Potential future hardening:

- Optional encrypted index storage.
- Optional no-preview mode that stores vectors plus hashes but no text preview.
- Redaction pass before embedding for known secret patterns.

## Observability

Add metrics/events:

- `semantic.index.files_seen`
- `semantic.index.files_indexed`
- `semantic.index.records_indexed`
- `semantic.index.files_skipped`
- `semantic.index.embed_batches`
- `semantic.index.duration_ms`
- `semantic.retrieve.duration_ms`
- `semantic.retrieve.records_returned`
- `semantic.retrieve.files_returned`
- `semantic.retrieve.stale_records_dropped`

Add trace stages to the existing run trace work:

- semantic index status check
- query embedding
- vector search
- stale evidence read
- semantic prompt packing

## Multi-Agent Implementation Model

This phase is large enough to split across agents, but only if the first PR
stabilizes contracts and tests. The work should be run as coordinated lanes,
not as isolated rewrites.

### Global Coordination Rules

- One agent owns contracts at a time. Other agents must not independently
  reshape shared semantic types.
- Each agent should keep changes inside its owning files unless the lane
  explicitly lists an integration file.
- Every lane must compile with fake embedders and local fixtures. Normal CI must
  not require a live Ollama daemon or downloaded embedding model.
- Live Ollama validation is opt-in and must be guarded by environment
  variables.
- No agent should add a vector database, native ANN dependency, or external
  service in Phase 28.
- Prompt integration must be feature-gated so `enabled=false` returns exactly
  the existing behavior.
- Agents should avoid broad formatting churn in files outside their lane.
- If a lane finds a missing shared primitive, it should add the primitive in a
  small coordination PR before continuing with feature work.

### Agent Lanes

| Lane | Owner | Primary files | Can start after | Must not own |
|---|---|---|---|---|
| A0 | Contracts and skeleton | `internal/semantic/contracts.go`, `config.go`, `testutil/`, this plan | immediately | deep TUI/server/CLI integration |
| A1 | Ollama embed modernization | `internal/llm/types.go`, `internal/llm/ollama/ollama.go`, Ollama tests | A0 contracts drafted | semantic store/search |
| A2 | Config and store | `internal/config/*`, `internal/semantic/store.go`, `vectors.go`, `stale.go` | A0 | prompt integration |
| A3 | Scanner and records | `internal/semantic/scanner.go`, `filters.go`, `records.go`, `symbols_*` | A0 | vector persistence |
| A4 | Build/refresh service | `internal/semantic/service.go`, `embedder.go`, build tests | A1, A2, A3 interfaces | TUI slash commands |
| A5 | Search and rendering | `internal/semantic/search.go`, `render.go`, retrieval tests | A2 record/vector shapes | LLM provider code |
| A6 | Prompt integration | `internal/agent/*`, `internal/analysis/*`, TUI bridge/server prompt paths | A4, A5 usable fake service | index extraction internals |
| A7 | CLI, slash, doctor, UX events | `internal/cli/*`, `internal/commands/*`, `internal/tui/*`, `internal/server/*` event types | A4 status/build APIs | vector scoring |
| A8 | Evals, security, performance | fixtures, tests, `tools/*`, docs/manual tests | A2-A7 partial APIs | core contract churn |

### Lane A0 - Contracts And Skeleton

Objective: create the stable compilation surface everyone else builds against.

Tasks:

- [ ] Add `internal/semantic` package with config defaults and validation.
- [ ] Add public request/report/status/result types.
- [ ] Add `Service`, `Embedder`, `Store`, and progress callback interfaces.
- [ ] Add typed fallback errors.
- [ ] Add deterministic fake embedder in `internal/semantic/testutil`.
- [ ] Add tiny fixture workspace under `internal/semantic/testdata/workspace`.
- [ ] Add no-op service implementation only if needed for integration
  compilation.
- [ ] Add tests for config defaulting and validation.

Exit criteria:

- `go test ./internal/semantic ./internal/config` passes.
- Other agents can import semantic contracts without circular dependencies.
- The package has no dependency on TUI, server, or CLI packages.

### Lane A1 - Ollama Embed Modernization

Objective: make embeddings efficient and compatible with current Ollama.

Tasks:

- [ ] Add `llm.EmbedOptions` or a package-local extended interface without
  breaking `llm.Client`.
- [ ] Move Ollama embedding calls from `/api/embeddings` to `/api/embed`.
- [ ] Send batch `input` arrays in one request.
- [ ] Support `dimensions`, `truncate`, `keep_alive`, and model options.
- [ ] Validate response vector count equals input count.
- [ ] Convert `number[][]` to `[][]float32`.
- [ ] Preserve old `Client.Embed(ctx, model, input)` behavior through a wrapper.
- [ ] Ensure `Authorization` headers are still applied for Ollama Cloud paths.
- [ ] Add fake HTTP tests for batch request body, options, errors, and malformed
  response shapes.

Exit criteria:

- Existing LLM tests still pass.
- New tests prove the new endpoint and batch behavior.
- No semantic package imports are required from `internal/llm`.

### Lane A2 - Config And Local Store

Objective: persist an inspectable local index safely.

Tasks:

- [ ] Add semantic config to `internal/config` with TOML load/save coverage.
- [ ] Derive workspace ID from canonical absolute root plus config model/schema.
- [ ] Implement manifest load/save with compatibility checks.
- [ ] Implement JSONL record read/write.
- [ ] Implement little-endian float32 vector file read/write.
- [ ] Implement L2 normalization and dimension validation.
- [ ] Implement atomic replace by writing to a temp directory/file then renaming.
- [ ] Implement workspace lock file with timeout and stale-lock handling.
- [ ] Implement `Clear` for one workspace index.
- [ ] Reject partial/corrupt index files with typed warnings.

Exit criteria:

- Tests cover new index, compatible load, schema mismatch, dimensions mismatch,
  corrupt vectors, clear, and atomic replace preserving old index on failure.
- Store APIs work without an embedder or scanner.

### Lane A3 - Scanner And Record Extraction

Objective: turn workspace files into stable semantic records.

Tasks:

- [ ] Reuse `dirwalk.Walk` and existing exclude behavior.
- [ ] Add semantic filters for binary, generated, vendored, dependency, large,
  and secret-like files.
- [ ] Keep a structured skip reason for status and tests.
- [ ] Extract Go functions, methods, types, interfaces, constants, variables,
  comments, signatures, and bounded body excerpts with `go/parser`.
- [ ] Extract Markdown heading sections with line ranges.
- [ ] Extract JSON/YAML/TOML top-level key records when cheap.
- [ ] Add generic line/token chunks for other text files.
- [ ] Add folder records from direct child names, package names, and README-like
  files.
- [ ] Generate deterministic record IDs from path, kind, name, line range, and
  text hash.
- [ ] Keep chunk overlap bounded and avoid duplicate records.

Exit criteria:

- Fixture tests prove exact line ranges for Go and Markdown.
- Secret/binary/vendor/generated filters are covered.
- Record extraction does not call the embedder and does not write the store.

### Lane A4 - Build And Refresh Service

Objective: orchestrate scanning, embedding, and store replacement.

Tasks:

- [ ] Implement `Build` with scan, extract, batch, normalize, persist.
- [ ] Implement `Refresh` with content-hash comparison.
- [ ] Re-embed only changed/new files during refresh.
- [ ] Remove records for deleted files.
- [ ] Keep old index valid if build/refresh is canceled or fails.
- [ ] Enforce `MaxRecords`, `MaxFileBytes`, and batch size.
- [ ] Emit progress events at scan, extract, embed, and write stages.
- [ ] Detect missing model and return `ErrModelMissing` with actionable detail.
- [ ] Add context cancellation checks between expensive steps.

Exit criteria:

- Fake embedder builds a fixture index.
- Refresh tests prove unchanged files are not re-embedded.
- Cancellation tests prove old index remains valid.
- Build reports include counts, skipped files, warnings, and index path.

### Lane A5 - Search, Ranking, And Rendering

Objective: retrieve useful evidence and render it safely for prompt assembly.

Tasks:

- [ ] Embed the query through the semantic embedder.
- [ ] Search normalized vectors with dot product.
- [ ] Add lexical score using path, basename, symbol name, and preview terms.
- [ ] Add frecency boost through the existing file index/frecency interface when
  available.
- [ ] Add explicit path/scope boost without letting it outrank direct mentions.
- [ ] Diversify results by file and record kind.
- [ ] Drop stale hits or request bounded refresh through the service.
- [ ] Read current evidence snippets from disk by line range.
- [ ] Enforce max files, max records, per-file snippets, and context bytes.
- [ ] Render `<semantic_context>` with metadata and `<evidence>` children.
- [ ] Escape XML-like attribute values and body delimiters safely.

Exit criteria:

- Deterministic fake-vector tests prove semantic retrieval without lexical term
  overlap.
- Rendering tests prove line ranges, byte caps, stale drops, and escaping.
- Search works with only store fixtures and fake query vectors.

### Lane A6 - Prompt Integration

Objective: make normal prompts use semantic retrieval without breaking current
context rules.

Tasks:

- [ ] Identify the single prompt assembly boundary for normal TUI/REPL/server
  requests.
- [ ] Insert semantic retrieval after mention expansion and before final prompt
  packing.
- [ ] Keep explicit `@file`, `@dir`, and line-range evidence ahead of semantic
  evidence.
- [ ] Keep the original user prompt as the final task anchor.
- [ ] Ensure `enabled=false`, missing index, missing model, and stale index
  fall back to current behavior.
- [ ] Add semantic evidence to `/prompt last` or equivalent prompt debug output.
- [ ] Integrate semantic retrieval into `/analyze-project` without replacing
  its explicit workflow constraints.
- [ ] Make semantic retrieval timeout/cancellation obey the current run context.
- [ ] Ensure tool permission and path-safety behavior is unchanged.

Exit criteria:

- Tests compare disabled semantic mode with current prompt output.
- Tests prove semantic evidence appears when the fake service returns hits.
- Tests prove explicit mentions come before semantic context.
- Server and TUI prompt paths use the same retrieval service behavior.

### Lane A7 - CLI, Slash Commands, Doctor, And Events

Objective: expose the feature in ways users and API clients can control.

Tasks:

- [ ] Add `nandocodego index build [path]`.
- [ ] Add `nandocodego index refresh [path]`.
- [ ] Add `nandocodego index status [path]`.
- [ ] Add `nandocodego index clear [path]`.
- [ ] Add optional `--model`, `--dimensions`, `--json`, and `--pull` flags only
  if they are implemented cleanly.
- [ ] Add `/index build`, `/index refresh`, `/index status`, `/semantic on`,
  and `/semantic off`.
- [ ] Add TUI progress messages from semantic events.
- [ ] Add server event payloads for retrieval/index stages.
- [ ] Add `doctor` checks for model availability, index compatibility, disk
  path, and disabled state.
- [ ] Ensure command output never prints raw indexed file content.

Exit criteria:

- CLI command tests cover success, missing model, missing index, and JSON status.
- Slash command tests cover state changes and status rendering.
- Server event tests cover semantic retrieval stage emission.

### Lane A8 - Evals, Security, Performance, And Docs

Objective: prove the feature is useful and safe enough to be the next core
stage.

Tasks:

- [ ] Add fixture repositories with renamed concepts to test semantic matching
  without exact words.
- [ ] Add eval queries for auth, permissions, config, server sessions, TUI
  state, hooks, memory, and docs.
- [ ] Add benchmark datasets or synthetic records for 10k, 50k, and 100k
  vector searches.
- [ ] Add tests that `.env`, key files, credentials, and configured secret paths
  are skipped.
- [ ] Extend network policy checks if any new command could trigger unexpected
  external access.
- [ ] Add live manual test doc for `qwen3-embedding:8b`.
- [ ] Add docs for index storage location and cleanup.
- [ ] Add regression tests that no embedding vector or raw source content is
  logged at INFO.

Exit criteria:

- Normal test suite uses fake embeddings.
- Live tests are opt-in and documented.
- Performance numbers are recorded before any ANN/native dependency is
  considered.

### PR And Merge Order

1. **PR 28-0: contracts and skeleton.** Owns `internal/semantic` contracts,
   config defaults, fake embedder, and fixture workspace.
2. **PR 28-1: Ollama embed API.** Owns `/api/embed`, batching, options, and
   tests.
3. **PR 28-2: store and vectors.** Owns manifest, records, vector persistence,
   normalization, locks, and clear.
4. **PR 28-3: scanner and record extraction.** Owns file filters and record
   extraction.
5. **PR 28-4: build/refresh service.** Owns orchestration and progress events.
6. **PR 28-5: search and rendering.** Owns query retrieval, hybrid ranking, and
   evidence rendering.
7. **PR 28-6: prompt integration.** Owns TUI/REPL/server/analysis prompt
   assembly integration behind config gates.
8. **PR 28-7: CLI/slash/doctor UX.** Owns command surfaces and status events.
9. **PR 28-8: evals, security, performance, docs.** Owns release-grade
   evidence, live test docs, and final gate closure.

Parallelization after PR 28-0:

- PR 28-1, PR 28-2, and PR 28-3 can run in parallel.
- PR 28-4 waits for enough of PR 28-1/2/3 to wire fake and real paths.
- PR 28-5 can start with store fixtures before PR 28-4 is complete.
- PR 28-6 should wait until PR 28-5 can produce rendered context.
- PR 28-7 can start command skeletons after PR 28-4 exposes build/status APIs.
- PR 28-8 starts early for fixtures/security tests and finishes last.

### Integration Freeze Points

Freeze these before multiple agents depend on them:

- semantic config field names
- record ID format
- manifest schema version
- vector file ordering
- progress event stage names
- prompt-rendered XML-like tag names
- fallback error names
- slash/CLI command names

If any freeze point changes after dependent PRs begin, the changing PR owns all
repo-wide updates in the same branch.

### Conflict-Avoidance File Ownership

High-conflict files need explicit care:

- `internal/llm/types.go`: owned by A1 for embed options. Others should not
  edit it during PR 28-1.
- `internal/config/config.go`, `loader.go`, `defaults.go`: owned by A2 for
  config. A7 can consume after A2 lands.
- `internal/tui/app.go`, `bridge.go`, slash files: owned by A6/A7 only after
  semantic service contracts are stable.
- `internal/server/types.go`, `handler.go`: A6/A7 only, with one PR owning event
  payload changes at a time.
- `internal/analysis/workflow.go`: A6 owns semantic injection; A5 should keep
  rendering package-local.
- `docs/PHASE-LOG.md`: each PR appends its own entry; do not rewrite prior
  history.

## Implementation Plan

This section maps the agent lanes above into implementation slices. The slice
numbers are the order in which the product should become usable. They are not a
license to create one massive PR per slice; use the PR order above.

### Phase 28.0 - Embed API Modernization

Objective: make embedding calls efficient and current.

Owner lane: A1.
Depends on: A0 contract draft.

Tasks:

- [ ] Update Ollama embedding implementation to use `POST /api/embed`.
- [ ] Preserve `llm.Client.Embed(...)` compatibility.
- [ ] Add optional `EmbedOptions` support for `dimensions`, `truncate`, and
  `keep_alive`.
- [ ] Batch multiple inputs in one HTTP request.
- [ ] Parse `embeddings: number[][]`.
- [ ] Keep backward-compatible tests for `Embed(...)`.
- [ ] Add tests for dimensions, truncate false, batch inputs, and error bodies.

Acceptance:

- Unit tests prove one `Embed` call with multiple inputs sends one
  `/api/embed` request and returns aligned vectors.
- Existing tests that use `llm.Client` still compile.

### Phase 28.1 - Semantic Config And Store

Objective: add stable configuration and local persistence.

Owner lane: A2.
Depends on: A0 contracts.

Tasks:

- [ ] Add `semantic_index` config fields and defaults.
- [ ] Add validation for dimensions, chunk sizes, top-k values, and byte caps.
- [ ] Add workspace ID derivation from absolute root.
- [ ] Implement manifest, records JSONL, vector binary read/write.
- [ ] Add atomic replace and lock handling.
- [ ] Add `index status` data model.

Acceptance:

- Tests can create, load, validate, and clear a local index store.
- Model/dimensions/schema mismatches are detected.

### Phase 28.2 - Workspace Scanner And Record Extraction

Objective: convert workspace files into retrievable semantic records.

Owner lane: A3.
Depends on: A0 contracts and fixture workspace.

Tasks:

- [ ] Reuse `dirwalk.Walk` for workspace scanning.
- [ ] Add semantic file filters and secret/path excludes.
- [ ] Add Go AST symbol extraction.
- [ ] Add Markdown heading extraction.
- [ ] Add generic chunk fallback using `analysis.ChunkText` or shared logic.
- [ ] Add folder records from child names and README/package files.
- [ ] Track line ranges and content hashes.

Acceptance:

- Tests show Go functions/types are indexed with correct path and line ranges.
- Tests show Markdown sections become doc records.
- Binary/generated/vendor/secret files are skipped.

### Phase 28.3 - Index Build And Refresh

Objective: build and incrementally refresh semantic vectors.

Owner lane: A4.
Depends on: A1 embedder, A2 store, A3 scanner.

Tasks:

- [ ] Implement build service with batching and progress callbacks.
- [ ] Implement refresh service with content-hash invalidation.
- [ ] Normalize vectors before writing.
- [ ] Add cancellation support.
- [ ] Add CLI `nandocodego index build|refresh|status|clear`.
- [ ] Add clear diagnostics when the embedding model is missing.

Acceptance:

- A fake embedder can build an index for a fixture repo.
- Refresh only re-embeds changed files in tests.
- Cancellation leaves the old valid index in place.

### Phase 28.4 - Semantic Search And Prompt Integration

Objective: implement semantic search, evidence rendering, and prompt use.

Owner lanes: A5 for search/rendering, then A6 for prompt integration.
Depends on: A2 store shape, A4 service, fake retrieval fixtures.

Tasks:

- [ ] Implement cosine/dot-product top-k search over normalized vectors.
- [ ] Add lexical and frecency hybrid boosts.
- [ ] Add file/path diversification.
- [ ] Add current-file staleness checks before evidence rendering.
- [ ] Render `<semantic_context>` evidence blocks under a strict byte/token
  budget.
- [ ] Wire retrieval into normal TUI, REPL, server, and project-analysis prompt
  assembly paths.
- [ ] Ensure explicit mentions always outrank semantic retrieval.

Acceptance:

- A prompt without exact lexical terms retrieves semantically related fixture
  code through deterministic fake embeddings.
- Packed prompts preserve the original user request and include cited evidence.
- Disabling semantic indexing returns the current behavior.

### Phase 28.5 - TUI, Slash Commands, Server Events

Objective: make indexing and retrieval visible and controllable.

Owner lane: A7.
Depends on: A4 status/build APIs and A6 retrieval event integration.

Tasks:

- [ ] Add `/index build`, `/index refresh`, `/index status`.
- [ ] Add `/semantic on` and `/semantic off`.
- [ ] Add TUI status events for indexing progress and retrieval results.
- [ ] Add server event payloads for semantic retrieval stages.
- [ ] Add `doctor` checks for model availability and index status.

Acceptance:

- Users can build, inspect, refresh, disable, and clear semantic indexing
  without leaving the app.
- Prompt debug views show retrieved evidence paths and line ranges.

### Phase 28.6 - Evals, Performance, And Hardening

Objective: prove quality and keep local performance bounded.

Owner lane: A8.
Depends on: starts after A0 fixtures; completes after A7 UX surfaces.

Tasks:

- [ ] Add retrieval fixture repos with non-exact semantic queries.
- [ ] Add evals for authentication, permissions, config, server, TUI, and docs
  queries.
- [ ] Add benchmarks for 10k, 50k, and 100k record searches.
- [ ] Add disk-size assertions for configured dimensions.
- [ ] Add large-repo cancellation tests.
- [ ] Add secret-file exclusion tests.
- [ ] Add optional live Ollama test gated by
  `NANDOCODEGO_LIVE_EMBED_MODEL=qwen3-embedding:8b`.

Acceptance:

- Fake-vector tests are deterministic and run in normal CI.
- Live Ollama tests are opt-in and skipped by default.
- Retrieval latency for a 50k-record fixture is acceptable on a normal laptop
  or the phase records a measured need for a later ANN index.

## Test Plan

Every agent lane owns tests for its contract. The test suite must be useful
before the real embedding model is installed.

Unit tests:

- embed request/response parsing
- embed batching and options
- vector normalization and cosine search
- record extraction for Go and Markdown
- secret and binary file skipping
- manifest mismatch detection
- stale file invalidation
- prompt evidence rendering
- explicit mention priority

Integration tests:

- fixture repo index build with fake embedder
- refresh after file edit/delete/add
- retrieval into project-analysis prompt builder
- retrieval into normal agent prompt builder
- server/TUI status event emission with fake services

Optional live tests:

```text
NANDOCODEGO_LIVE_EMBED_MODEL=qwen3-embedding:8b go test ./internal/semantic ./internal/llm/ollama -run Live
```

Manual validation:

1. Pull `qwen3-embedding:8b`.
2. Run `nandocodego index build .`.
3. Ask for a bug fix using semantically related wording that does not appear in
   filenames.
4. Confirm the prompt debug output includes relevant files and line ranges.
5. Confirm the generation LLM can use the retrieved context to inspect and edit
   files through existing tools.
6. Edit one retrieved file, run `/index refresh`, and confirm stale records are
   replaced.
7. Disable semantic retrieval and confirm behavior falls back to lexical/current
   retrieval.

### Test Ownership Matrix

| Area | Owner lane | Required tests |
|---|---|---|
| Config defaults and validation | A0/A2 | default values, invalid dimensions, invalid caps, disabled behavior |
| Ollama embed API | A1 | request body, batch response, options, errors, auth headers |
| Store and vectors | A2 | manifest compatibility, corrupt files, atomic replace, clear, normalization |
| Scanner and records | A3 | Go AST lines, Markdown sections, generic chunks, skip reasons |
| Build/refresh | A4 | fake build, incremental refresh, cancellation, missing model |
| Search/ranking | A5 | semantic no-exact-match retrieval, hybrid boosts, diversification |
| Rendering | A5 | evidence tags, escaping, byte caps, stale drops |
| Prompt integration | A6 | disabled equals old behavior, explicit mention priority, server/TUI parity |
| CLI/slash/doctor | A7 | status output, command errors, JSON output, toggles |
| Security/perf/evals | A8 | secret skips, no content logs, search benchmarks, live-gated tests |

### MVP Acceptance Gate

Phase 28 MVP is accepted only when all of these pass:

1. `go test ./...`
2. `tools/check-allowed-deps.sh`
3. `tools/check-network-policy.sh`
4. `nandocodego index build .` works against a live local Ollama install with
   `qwen3-embedding:8b`.
5. A normal prompt uses semantic context when a compatible index exists.
6. The same prompt falls back cleanly with semantic disabled, no index, stale
   index, or missing model.
7. Prompt debug output shows retrieved paths and line ranges.
8. Explicit `@file`, `@dir`, and line ranges still outrank semantic retrieval.
9. Secret-like files are skipped and never appear in index status previews.
10. Phase log records tests, manual checks, known constraints, and follow-ups.

## Acceptance Criteria

- `Embed` is no longer just dormant provider plumbing; semantic retrieval calls
  it when enabled.
- The app can build a local semantic index for a workspace using
  `qwen3-embedding:8b`.
- The index contains file, folder, Go symbol, documentation section, and fallback
  chunk records.
- Prompt-time retrieval can find semantically relevant code without exact word
  overlap.
- Retrieved context is cited by path and line range and packed under a strict
  budget.
- Explicit mentions remain authoritative.
- Memory recall remains file-based and separate.
- Missing model, stale index, and disabled config degrade cleanly.
- Normal CI uses fake embeddings and does not require Ollama.
- Optional live tests verify the real Ollama embed endpoint.

## Risks And Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Index builds are slow on large repos | Bad first-run UX | Explicit build command, progress, cancellation, incremental refresh |
| Vector files become too large | Disk and memory pressure | Default 1024 dimensions, record caps, skip generated/vendor files |
| Retrieved snippets are stale | Wrong context | Content hashes, prompt-time stale check, refresh or drop stale evidence |
| Semantic search misses exact identifiers | Lower coding accuracy | Hybrid lexical/frecency boost and explicit mention priority |
| Sensitive files get indexed | Privacy/security issue | Strict excludes, path policy, secret-pattern filters, `index clear` |
| Missing embedding model blocks prompts | UX failure | Clear diagnostics, fallback retrieval, opt-in enablement |
| Linear search is too slow at scale | Latency | Benchmarks first; defer ANN/native index until measured |

## Roadmap Placement

This is now the next feature phase. It should be placed before Phase 25 Remote
/ Bridge Mode, Phase 17 Distribution, and Phase 18 Hardening.

Implementation order:

- Phase 28 - Semantic Workspace Index And Embedding Retrieval
- Phase 25 - Remote / Bridge Mode
- Phase 17 - Distribution and Install
- Phase 18 - Hardening, Eval Suite, and Docs

Rationale:

- Remote/bridge mode should expose the same retrieval behavior as local TUI and
  server mode.
- Distribution should package the semantic config, cache layout, doctor checks,
  and docs after they are stable.
- Hardening should evaluate the final prompt-retrieval path, not the older
  lexical-only path.

## Resolved Implementation Decisions

- Semantic retrieval is enabled by default in use-if-index-exists mode.
- Normal prompts do not auto-build a full index.
- `qwen3-embedding:8b` is the default embedding model.
- `1024` is the default embedding dimension for disk/memory balance.
- `4096` remains user-configurable for smaller repos or quality experiments.
- `index build` prints the pull command when the model is missing.
- `index build --pull` is allowed only if the implementation can do it through
  existing Ollama pull plumbing and clear progress events.
- The local store keeps bounded previews by default for status/debug output.
- Prompt evidence is always read from current files, not from stored previews.
- Project memory files are excluded from semantic indexing in the first
  implementation and remain handled by the memory subsystem.
- Linear vector scan is the first implementation; ANN/native indexes are
  deferred until benchmark evidence proves they are necessary.

## Remaining Open Decisions

- Exact TUI copy for one-time "index missing" hints.
- Whether `index build --pull` lands in the MVP or waits for a follow-up.
- Whether prompt-time small refresh should be synchronous by default or delayed
  until after the first semantic retrieval result.
- Whether server clients need an explicit API endpoint to trigger index build,
  or whether CLI/TUI control is sufficient for MVP.
