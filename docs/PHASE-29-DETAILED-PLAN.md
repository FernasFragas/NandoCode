# Phase 29 - TUI Semantic Index Progress Observability

Date: 2026-05-27
Status: Implemented (MVP)
Priority: Completed prerequisite for Phase 25
Workstream: TUI observability, semantic indexing UX

## Roadmap Directive

Phase 29 is implemented as an MVP. Phase 25 Remote / Bridge Mode can proceed
after this phase subject to the remaining roadmap gates.

Phase 28 made semantic indexing functional. Phase 29 makes long-running
indexing understandable while it is happening. The user must be able to see
which stage the indexer is in and how much work has completed without waiting
for only a final success or error line.

## Implementation Snapshot (2026-05-27)

Delivered in code:

- Semantic event contract now includes additional progress counters:
  - `FilesTotal`, `FilesIndexed`, `FilesSkipped`, `RecordsTotal`.
- Scanner progress plumbing added with throttled callbacks and monotonic
  counters for scan visibility.
- Semantic build/refresh now emit staged progress events:
  - `scan_start`, `scan_progress`, `extract_progress`, `embed_progress`,
    `write_start`, `write_done`.
- Refresh fallback-to-build now emits an explicit message for observability.
- TUI now receives semantic progress events via `indexProgressMsg`.
- TUI has an `indexProgressState` runtime model with stage/counter tracking.
- `/index build` and `/index refresh` now pass `EventSink` into semantic
  service calls.
- TUI status bar shows live index progress while operations are active.
- Transcript remains concise (start/completion/error), without repeated
  progress spam.
- Concurrent index operation attempts are rejected with `[Index already running]`.
- Added dedicated TUI tests for sink wiring, status rendering, progress
  lifecycle, and error handling.
- Added semantic tests for progress event staging, counters, and fallback
  messaging.

Primary files changed:

- `internal/semantic/contracts.go`
- `internal/semantic/scanner.go`
- `internal/semantic/service.go`
- `internal/semantic/service_test.go`
- `internal/tui/messages.go`
- `internal/tui/app.go`
- `internal/tui/index_events.go`
- `internal/tui/index_progress.go`
- `internal/tui/index_progress_test.go`

Validation completed:

- `go test ./internal/semantic ./internal/tui ./internal/cli ./internal/server`
- `go test ./...`
- `./tools/check-allowed-deps.sh`
- `./tools/check-network-policy.sh`

## Review Outcome

The first progress plan has the right shape: use the existing semantic
`EventSink`, bridge events into Bubble Tea messages, keep live counters in
model state, and avoid transcript spam.

Implementation details that must be tightened:

- Progress belongs primarily in the status area, not as repeated transcript
  lines.
- The transcript should record start, final result, cancellation, and errors.
- The semantic event contract needs total and skipped counters; without totals,
  the UI cannot show `184/920`.
- Event emission must be throttled or stage-based. Do not send one Bubble Tea
  message per file on large repositories.
- The TUI event sink must only send immutable messages into the Bubble Tea
  update loop. Do not mutate `Model` from indexing goroutines.
- The implementation should support both `/index build` and `/index refresh`.
- Cancellation should be designed into the model state even if the first slice
  only exposes progress.

## Goal

Show live semantic index progress in the TUI while `/index build` or
`/index refresh` is running.

Target user-facing examples:

```text
Index build: scanning files 184/920
Index build: extracting files 920/920 | indexed 701 | skipped 219 | records 2,944
Index build: embedding batch 8/92 | records 256/2,944
Index build: writing index
```

If a total is not available yet:

```text
Index build: scanning files 184
```

The user should know:

- active operation: build or refresh
- active stage: scanning, extracting, embedding, writing
- total candidate files when known
- files seen
- files indexed
- files skipped
- records extracted
- embedding batches completed and total batches
- elapsed time
- final success, error, or cancellation

## Non-Goals

- No semantic ranking changes.
- No embedding model changes.
- No vector store format changes unless required to report progress.
- No transcript line per file.
- No raw file content in progress messages, logs, telemetry, or events.
- No broad TUI redesign.
- No web UI progress work in this phase unless server event parity is needed
  for tests.

## Current State

The semantic service already has progress primitives:

- `semantic.Event`
- `semantic.EventSink`
- stages such as `scan_start`, `extract_progress`, `embed_progress`,
  `write_start`, and `write_done`
- `BuildRequest.EventSink`
- `RefreshRequest.EventSink`

Current TUI behavior in `/index build` and `/index refresh` only reports:

```text
[Index build started...]
[Index build complete: files_seen=... files_indexed=... records=... skipped=... duration=...]
```

The TUI does not pass an event sink into `Build` or `Refresh`, so live progress
is not visible.

## Existing Code Touchpoints

| Area | Files | Ownership |
| --- | --- | --- |
| Semantic event contract | `internal/semantic/contracts.go` | Phase 29 |
| Scanner/build progress | `internal/semantic/scanner.go`, `internal/semantic/service.go` | Phase 29 |
| TUI messages | `internal/tui/messages.go` | Phase 29 |
| TUI model/update/render | `internal/tui/app.go`, `internal/tui/runstate.go` if needed | Phase 29 |
| TUI transcript helpers | `internal/tui/transcript.go` | Only if needed |
| CLI output | `internal/cli/index.go` | Optional, only if event contract requires shared formatting |
| Tests | `internal/semantic/*_test.go`, `internal/tui/*_test.go` | Phase 29 |

## Plan Review Addendum

The initial Phase 29 plan was directionally correct but not yet precise enough
for multiple agents to implement in parallel. The risky areas are:

- `internal/tui/app.go` is a high-conflict file. Multiple agents editing
  command handling, update handling, and rendering in the same patch will
  collide.
- `semantic.Event` is a shared contract. It must be extended first and then
  treated as stable.
- Scanner progress and TUI rendering can proceed in parallel only if the
  event fields and stage meanings are fixed.
- The status-area UX should be implemented in a helper file where possible,
  not by expanding unrelated rendering code inline.
- Tests need clear ownership so one lane does not rely on another lane's fake
  service shape.

This addendum makes Phase 29 a contract-first, multi-agent implementation.

## Coordination Rules

1. **Freeze shared contracts first.** The first merged patch must add the final
   Phase 29 `semantic.Event` fields and `indexProgressMsg` shape. After that,
   agents must not rename fields without coordinating all lanes.
2. **Avoid broad `app.go` edits.** Prefer new focused files:
   - `internal/tui/index_progress.go`
   - `internal/tui/index_progress_test.go`
   - `internal/tui/index_events.go` if the sink needs its own file
3. **One lane owns each `app.go` region.**
   - Slash command wiring owns `handleIndexCommand`.
   - Update handling owns the `case indexProgressMsg` and `case indexOpDoneMsg`
     behavior.
   - Rendering owns only the status rendering call site and helper output.
4. **No goroutine mutates `Model`.** Indexing goroutines send immutable
   Bubble Tea messages only.
5. **No transcript spam.** Any lane adding repeated transcript progress lines
   is out of scope.
6. **No raw content in events.** Paths and counts are allowed; file snippets,
   extracted text, embeddings, and previews are not allowed in progress events.
7. **Keep old indexes safe.** Cancellation/error behavior must not delete or
   corrupt an existing valid index.

## Parallelization Map

| Sequence | Lane | Can Start | Blocks | Primary Files |
| --- | --- | --- | --- | --- |
| 29-0 | Contract freeze | Immediately | All compile-dependent lanes | `internal/semantic/contracts.go`, `internal/tui/messages.go` |
| 29-A | Semantic progress emission | After 29-0 fields exist | Final integration tests | `internal/semantic/scanner.go`, `internal/semantic/service.go` |
| 29-B | TUI state/update bridge | After 29-0 message exists | Rendering and slash final wiring | `internal/tui/index_progress.go`, `internal/tui/app.go` update cases |
| 29-C | TUI status rendering | After 29-B helper shape exists | Manual UX acceptance | `internal/tui/index_progress.go`, status render call site |
| 29-D | Slash command wiring | After 29-B sink exists | End-to-end TUI behavior | `internal/tui/app.go` `handleIndexCommand` |
| 29-E | Validation/docs | After A-D compile | Acceptance | tests, `docs/PHASE-LOG.md` |

Agents may work in parallel after 29-0. If 29-0 is not merged yet, other
agents should limit work to local drafts and avoid renaming shared fields.

## Product Behavior

### Transcript

Append one line when work starts:

```text
[Index build started: scanning workspace...]
```

Append one line when work completes:

```text
[Index build complete: files_seen=920 files_indexed=701 records=2944 skipped=219 batches=92 duration=1m14s]
```

Append one line on error:

```text
[Index error during embedding: semantic embedding model missing: ...]
```

Do not append repeated progress lines.

### Status Area

Render live progress while active. The status line should be compact and
stable:

```text
Index build: scanning files 184/920
Index build: embedding batch 8/92 | records 256/2944
```

When an agent run is also active, index progress can be a secondary segment.
Do not hide permission prompts, model credential prompts, or active tool state.

### Commands

Existing commands remain:

```text
/index build
/index refresh
/index status
/index clear
```

Optional follow-up if cancellation is implemented in this phase:

```text
/index cancel
```

If `/index cancel` lands, it must cancel the indexing context and leave the
previous valid index intact.

## Event Contract Changes

Extend `semantic.Event` with enough counters for live progress:

```go
type Event struct {
    Stage        Stage
    Message      string
    Root         string
    Path         string

    FilesTotal   int
    FilesSeen    int
    FilesDone    int
    FilesIndexed int
    FilesSkipped int

    RecordsDone  int
    RecordsTotal int

    BatchesDone  int
    TotalBatches int

    Duration     time.Duration
}
```

Rules:

- `FilesTotal == 0` means unknown.
- Counters must be monotonic within a single operation.
- `Path` may be set for debug/test events, but the TUI should not render every
  path by default.
- No event may include file contents.
- Event additions must be backward-compatible for existing callers.

## Semantic Progress Implementation

### Candidate Count

Preferred implementation:

1. Add `CountWorkspaceCandidates(ctx, options)` to `internal/semantic`.
2. Reuse the same path, binary, generated, vendor, secret, size, and depth
   filters as `ScanWorkspace`.
3. Return candidate total and skipped total before full extraction starts.
4. Emit `StageScanStart` with `FilesTotal`.

Fallback implementation if a separate count duplicates too much traversal:

1. Keep `FilesTotal == 0` during scan.
2. Emit total once `ScanWorkspace` completes.
3. The TUI renders `scanning files 184` until the denominator is known.

Do not create a second set of filtering rules just for counting.

### Event Emission Cadence

Emit progress:

- on stage transitions
- every completed embedding batch
- every 250ms during scanning/extraction, or every N files, whichever is
  easier in the scanner structure

Do not emit one event per file in the TUI path for large repositories.

### Build/Refresh Behavior

`Build` should report:

- scan start
- scan progress
- extraction completion
- embedding progress
- write start
- write done

`Refresh` should report:

- scan start/progress
- changed-file detection summary
- embedding progress for changed/new records
- write start/done

If refresh falls back to full build because dimensions changed or the index is
missing, emit an event message that makes that visible:

```text
refresh falling back to full build
```

## TUI Architecture

### Messages

Add to `internal/tui/messages.go`:

```go
type indexProgressMsg struct {
    Event semantic.Event
}
```

### Event Sink

Add a small adapter in `internal/tui`:

```go
type tuiIndexEventSink struct {
    send func(tea.Msg)
}

func (s tuiIndexEventSink) Publish(evt semantic.Event) {
    if s.send != nil {
        s.send(indexProgressMsg{Event: evt})
    }
}
```

Use the existing `ProgramSender` abstraction when available. The sink must not
hold or mutate `*Model`.

### Model State

Add state to `Model`:

```go
type indexProgressState struct {
    Active       bool
    Operation    string
    Stage        semantic.Stage
    StartedAt    time.Time
    UpdatedAt    time.Time

    FilesTotal   int
    FilesSeen    int
    FilesDone    int
    FilesIndexed int
    FilesSkipped int

    RecordsDone  int
    RecordsTotal int
    BatchesDone  int
    TotalBatches int

    LastMessage   string
}
```

Optional if cancellation lands:

```go
indexCancel context.CancelFunc
```

### Update Handling

Handle progress messages in the Bubble Tea update loop:

```go
case indexProgressMsg:
    m.updateIndexProgress(msg.Event)
    m.refreshViewportContent(true)
    return m, nil
```

`indexOpDoneMsg` must set `m.indexProgress.Active = false`.

### Rendering

Add:

```go
func (m *Model) renderIndexProgressStatus() string
```

Integrate it into the existing status rendering path. The output should be
short enough for narrow terminals and should degrade gracefully:

- with total: `Index build: scanning files 184/920`
- without total: `Index build: scanning files 184`
- embedding: `Index build: embedding 8/92 batches`
- writing: `Index build: writing index`

Avoid wide, multi-line progress panels in the first implementation.

## Slash Command Wiring

Change `/index build` from:

```go
BuildRequest{Root: root, Config: cfg}
```

to:

```go
BuildRequest{
    Root: root,
    Config: cfg,
    EventSink: tuiIndexEventSink{send: ...},
}
```

Do the same for `RefreshRequest`.

Set `m.indexProgress` active before returning the command.

## Testing Plan

### Semantic Tests

Add tests for:

- build emits stage events in order
- build emits file counters
- embed progress reports batches done and total batches
- refresh emits progress when it reuses unchanged records
- events do not include raw file contents
- counters do not go backwards

### TUI Tests

Add tests for:

- `/index build` passes an event sink to the semantic service
- `indexProgressMsg` updates `Model.indexProgress`
- rendered status includes file progress
- rendered status handles unknown totals
- `indexOpDoneMsg` clears active progress
- index error clears active progress and appends one transcript line

### Manual Checks

With `qwen3-embedding:8b` installed:

```text
/index build
```

Confirm:

- transcript gets one start line
- status updates during scan/extract/embed/write
- transcript gets one completion line
- no repeated per-file transcript spam

Then:

```text
/index refresh
```

Confirm changed-file progress and completion counters.

## Multi-Agent Implementation Lanes

### Lane 29-0 - Contract Freeze

Purpose: create the stable compile target for all other agents.

Owns:

- `internal/semantic/contracts.go`
- `internal/tui/messages.go`

Tasks:

- [ ] Extend `semantic.Event` with:
  - `FilesTotal`
  - `FilesIndexed`
  - `FilesSkipped`
  - `RecordsTotal`
- [ ] Keep existing fields and stages backward-compatible.
- [ ] Add `indexProgressMsg` to TUI messages.
- [ ] Add comments to new event fields explaining zero-value behavior.
- [ ] Run `go test ./internal/semantic ./internal/tui` and fix compile
  fallout only inside owned files.

Exit gate:

- Other lanes can import/use the new fields without inventing local copies.

Do not:

- Implement scanner progress.
- Render UI.
- Wire slash commands.

### Lane 29-A - Semantic Progress Emission

Purpose: make the index service publish complete, throttled, structured
progress.

Owns:

- `internal/semantic/scanner.go`
- `internal/semantic/service.go`
- `internal/semantic/*progress*_test.go` or focused additions to existing
  semantic tests

Tasks:

- [ ] Decide implementation path for `FilesTotal`:
  - preferred: shared candidate count helper using the same filters as scan
  - fallback: unknown total until scan completes
- [ ] If adding a count helper, keep filter logic shared with `ScanWorkspace`.
- [ ] Emit `StageScanStart` with root, operation message, and `FilesTotal`
  when known.
- [ ] Emit `StageScanProgress` with monotonic `FilesSeen` and `FilesTotal`.
- [ ] Emit `StageExtractProgress` with `FilesIndexed`, `FilesSkipped`,
  `RecordsDone`, and `RecordsTotal` when extraction is complete.
- [ ] Emit `StageEmbedProgress` on every embedding batch with:
  - `BatchesDone`
  - `TotalBatches`
  - `RecordsDone`
  - `RecordsTotal`
- [ ] Emit `StageWriteStart` and `StageWriteDone`.
- [ ] For refresh fallback to build, emit a message such as
  `refresh falling back to full build`.
- [ ] Add throttling so scan progress does not publish once per file on large
  repositories. Use elapsed time or file-count intervals.
- [ ] Preserve existing build/refresh reports.

Tests:

- [ ] Build emits scan, extract, embed, write stages in order.
- [ ] Build counters are monotonic.
- [ ] Embed progress reports batch count and total batch count.
- [ ] Refresh emits progress when it reuses unchanged vectors.
- [ ] Refresh fallback to build emits a visible message.
- [ ] Event messages do not contain fixture file contents.

Exit gate:

- `go test ./internal/semantic` passes.

Do not:

- Edit `internal/tui/app.go`.
- Decide final status-line wording.
- Add CLI-only progress unless required for compile or shared tests.

### Lane 29-B - TUI Progress State And Event Bridge

Purpose: move semantic progress events safely into Bubble Tea state.

Owns:

- `internal/tui/index_progress.go`
- `internal/tui/index_events.go` if split out
- `internal/tui/app.go` update-loop cases only
- `internal/tui/*index_progress*_test.go`

Tasks:

- [ ] Add `indexProgressState`.
- [ ] Add `indexProgress indexProgressState` to `Model`.
- [ ] Add `tuiIndexEventSink` that sends `indexProgressMsg`.
- [ ] Add `updateIndexProgress(evt semantic.Event)` helper.
- [ ] Initialize active state on first progress event.
- [ ] Preserve `StartedAt` across progress events.
- [ ] Update `UpdatedAt`, stage, message, and counters on each event.
- [ ] Ensure counter updates never move backwards unless a new operation starts.
- [ ] Clear active state on `indexOpDoneMsg`.
- [ ] Clear active state on index error.
- [ ] Keep all state mutation inside Bubble Tea `Update`.

Tests:

- [ ] `indexProgressMsg` activates progress state.
- [ ] Progress state keeps operation and stage.
- [ ] Unknown totals render/state as zero without panic.
- [ ] Later lower counters do not regress visible state.
- [ ] `indexOpDoneMsg` clears active state.
- [ ] Error completion clears active state and leaves transcript error behavior
  intact.

Exit gate:

- `go test ./internal/tui -run IndexProgress` passes.
- Full `go test ./internal/tui` passes after merge with Lane C/D.

Do not:

- Mutate `Model` from the event sink.
- Add scan/extract events in semantic service.
- Add repeated transcript progress entries.

### Lane 29-C - TUI Status Rendering And UX

Purpose: make progress visible in a compact, stable status area.

Owns:

- `internal/tui/index_progress.go` rendering helpers
- one narrow status-render integration point in `internal/tui/app.go`
- rendering tests

Tasks:

- [ ] Add `renderIndexProgressStatus()` or equivalent helper.
- [ ] Format stages:
  - scan: `Index build: scanning files 184/920`
  - extract: `Index build: extracting files 920/920 | indexed 701 | skipped 219 | records 2944`
  - embed: `Index build: embedding batch 8/92 | records 256/2944`
  - write: `Index build: writing index`
- [ ] Use `refresh` instead of `build` for refresh operations.
- [ ] Include elapsed time when width allows, but keep it optional.
- [ ] Keep narrow widths readable. Prefer dropping optional segments over
  wrapping into multiple lines.
- [ ] Ensure permission/model/tool status remains higher priority than index
  progress.
- [ ] Do not render raw paths unless a future debug mode is added.

Tests:

- [ ] Render scan with total.
- [ ] Render scan without total.
- [ ] Render extract with indexed/skipped/records.
- [ ] Render embed with batch counts.
- [ ] Render write stage.
- [ ] Render output stays compact for narrow width.

Exit gate:

- TUI status tests pass.
- Manual visual check is documented by Lane 29-E.

Do not:

- Change slash command behavior.
- Change semantic scanner behavior.
- Add new cards, panels, or broad layout changes.

### Lane 29-D - Slash Wiring And Operation Lifecycle

Purpose: connect `/index build` and `/index refresh` to live progress and
handle operation lifecycle cleanly.

Owns:

- `internal/tui/app.go` `handleIndexCommand`
- any small helper needed to start index operations
- TUI slash/index command tests

Tasks:

- [ ] Pass `EventSink` into `semantic.BuildRequest`.
- [ ] Pass `EventSink` into `semantic.RefreshRequest`.
- [ ] Set initial `indexProgressState` before returning the async command.
- [ ] Record operation as `build` or `refresh`.
- [ ] Preserve existing start transcript messages, but update wording to match
  this plan.
- [ ] Preserve existing completion/error transcript behavior.
- [ ] On completion, include `batches` when available.
- [ ] Prevent concurrent index operations if the current service/store cannot
  safely handle them. Render a concise error:
  `[Index already running]`
- [ ] If cancellation is included, add `/index cancel`; otherwise leave it as a
  documented follow-up and keep an internal state shape that can support it.

Tests:

- [ ] `/index build` passes a non-nil event sink.
- [ ] `/index refresh` passes a non-nil event sink.
- [ ] Starting build marks progress active.
- [ ] Starting refresh marks progress active with operation `refresh`.
- [ ] Completion clears progress active.
- [ ] Error clears progress active and appends one transcript error.
- [ ] Concurrent operation is rejected or safely serialized.

Exit gate:

- `go test ./internal/tui ./internal/semantic` passes.

Do not:

- Rewrite unrelated slash command handling.
- Change semantic retrieval behavior.
- Trigger automatic index build during normal prompts.

### Lane 29-E - Final Validation, Docs, And Acceptance Evidence

Purpose: merge the lanes, verify the full behavior, and record what landed.

Owns:

- `docs/PHASE-LOG.md`
- minor updates to `docs/PHASE-29-DETAILED-PLAN.md` status when complete
- final test execution

Tasks:

- [ ] Run focused tests:
  - `go test ./internal/semantic`
  - `go test ./internal/tui`
  - `go test ./internal/cli`
- [ ] Run full suite:
  - `go test ./...`
- [ ] Run policy checks if dependencies or endpoints changed:
  - `./tools/check-allowed-deps.sh`
  - `./tools/check-network-policy.sh`
- [ ] Manually run TUI `/index build` with `qwen3-embedding:8b`.
- [ ] Manually run TUI `/index refresh`.
- [ ] Confirm transcript start/end/error behavior.
- [ ] Confirm no progress event contains raw file contents.
- [ ] Update `docs/PHASE-LOG.md` with:
  - files changed
  - tests run
  - manual checks
  - known constraints
  - acceptance status

Exit gate:

- Phase 29 can be marked implemented only after tests and manual TUI evidence
  are recorded.

Do not:

- Expand scope into Phase 25 remote APIs.
- Add unrelated TUI redesign work.

## Merge Strategy

1. Merge 29-0 first.
2. Merge 29-A and 29-B after they both compile against 29-0.
3. Merge 29-C after 29-B exposes progress state helpers.
4. Merge 29-D after 29-B exposes the event sink.
5. Merge 29-E last.

If two lanes need `internal/tui/app.go`, keep patches small and anchored to
different functions. Prefer helper functions in new files so rebases stay
mechanical.

## Conflict Hotspots

| File | Risk | Rule |
| --- | --- | --- |
| `internal/tui/app.go` | High | Only edit assigned function/case; move helper logic to new files |
| `internal/tui/messages.go` | Medium | 29-0 owns message type additions |
| `internal/semantic/contracts.go` | High | 29-0 owns field additions; later lanes may read fields only |
| `internal/semantic/service.go` | Medium | 29-A owns event emission; no TUI imports |
| `internal/semantic/scanner.go` | Medium | 29-A owns counting/progress; reuse filters |
| `docs/PHASE-LOG.md` | Low | 29-E owns final status update |

## Acceptance Criteria

Phase 29 is accepted when:

1. `/index build` shows live status progress in the TUI.
2. `/index refresh` shows live status progress in the TUI.
3. The user can see files processed versus total when total is known.
4. The user can see indexed/skipped file counts and record counts.
5. The user can see embedding batch progress.
6. Transcript contains start/end/error summaries, not progress spam.
7. Progress events are structured and tested.
8. No progress event or transcript line includes raw file contents.
9. `go test ./internal/semantic ./internal/tui ./internal/cli` passes.
10. `go test ./...` passes.

## Follow-Ups After MVP

- `/index cancel`
- server/SSE index build endpoint progress if remote clients need to trigger
  index builds
- persisted last index operation summary
- estimated time remaining after enough batches complete
- richer status panel for wide terminals
