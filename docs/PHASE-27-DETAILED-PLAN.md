# Phase 27 — Directory Mention Expansion (`@dir/`, multi-directory prompts)

## Goal

Let users analyze whole directories — and several at once — in a single prompt
by mentioning them with the existing `@` syntax. Today the prompt
`Summarize @docs/ @.claude/ @.codex` fails because `mentions.ExpandPrompt`
explicitly rejects directories at `internal/mentions/expand.go:53`. After this
phase, the same prompt walks each directory, inlines the contained text files
into the prompt as `<file>` blocks (subject to caps), and emits a single
`<directory>` summary block with the tree.

Files keep working exactly as today. Directories become first-class.

## Non-goals

- New tool. Expansion stays in `internal/mentions`; the agent loop is unchanged.
- Streaming/lazy expansion. Expansion is synchronous at submit time, same as
  files.
- Symbol or AST-aware extraction. We inline raw bytes.
- Watch mode / re-expansion across turns. Each prompt expands independently.
- Glob syntax (`@docs/**/*.md`). Deferred — see Phase 12.

## Integration points (current code)

- Mention expansion entrypoint: `internal/mentions/expand.go:32` (`ExpandPrompt`)
- Directory rejection (to remove): `internal/mentions/expand.go:53`
- Path safety: `internal/tools/pathsafe.go` via `tools.ResolvePath`
- Read-cap accounting: `tools.Context.EffectiveMaxReadChars`
  (`internal/tools/context.go`)
- File index excludes (reuse): `internal/tui/fileindex/index.go`
- TUI submission path: `internal/tui/app.go:386`
- Picker file provider (already surfaces directories): `internal/tui/picker/file_provider.go`

## Architecture

```
ExpandPrompt(input, ctx)
   │
   ├── extractMentionPaths       (unchanged)
   │
   ├── for each path:
   │     ResolvePath(PathRead)   (unchanged — denies escapes)
   │     Stat
   │       ├── file → existing inline path (unchanged)
   │       └── dir  → expandDirectory(...)
   │                    ├── walk (excludes, caps)
   │                    ├── per-file: read, utf-8 check, truncate
   │                    └── emit <directory><tree>…</tree><file>…</file>…</directory>
   │
   └── share a single budget across all mentions (files + dirs)
```

A single per-prompt budget is shared across **all** mentions. Three large
directories must not silently exceed limits because each one independently
fits.

## Data shape

Extend `ResolvedFile` (`internal/mentions/expand.go:14`) into a discriminated
union, or add a sibling `ResolvedDirectory`. Prefer a sibling — `ResolvedFile`
is referenced elsewhere (snapshot recording, transcript) and changing its
shape ripples.

```go
type ResolvedDirectory struct {
    Path        string   // raw mention as the user typed it
    AbsPath     string
    FileCount   int      // files actually inlined
    SkippedCount int     // files walked but not inlined (binary, oversize, budget)
    TotalBytes  int      // bytes of inlined content (post-truncation)
    Truncated   bool     // any files truncated OR walk hit a cap
    Reason      string   // when Truncated=true: "file-cap" | "byte-cap" | "depth-cap"
}

func ExpandPrompt(input string, ctx tools.Context) (
    string, []ResolvedFile, []ResolvedDirectory, error,
)
```

Update the two callers (TUI submit, REPL) for the new return value. Both
currently only log/record the file list; pass the directory list through the
same plumbing for transcript display.

## Caps and defaults

Hard limits chosen so a typical prompt stays well under context. All come from
`tools.Context` so they remain configurable.

| Knob | Default | Field |
|---|---|---|
| Max files inlined per directory | 200 | `MaxDirFiles` |
| Max files inlined per prompt (all `@` mentions combined) | 400 | `MaxPromptFiles` |
| Max bytes per file (already exists) | `EffectiveMaxReadChars()` | — |
| Max bytes inlined per directory | 512 KiB | `MaxDirBytes` |
| Max bytes inlined per prompt | 2 MiB | `MaxPromptBytes` |
| Max walk depth | 8 | `MaxDirDepth` |

Add fields to `tools.Context` with zero-value sentinels meaning "use default."
Wire defaults in `internal/tools/context.go` next to `EffectiveMaxReadChars`.

When a cap fires, **stop walking** but emit what we have plus a clear note in
the `<directory>` block (`truncated="true" reason="byte-cap"`). Never error
the whole prompt because of caps — error only on path-safety violation, IO
error, or genuinely malformed input.

## Walking strategy

Reuse the same exclusion list and git-aware path the file index already uses
(`internal/tui/fileindex/index.go`). Refactor the walker into a small shared
helper instead of copy-pasting:

```
internal/tools/dirwalk/walk.go
   Walk(root, opts) ([]Entry, Stats, error)
      opts: Excludes, MaxFiles, MaxBytes, MaxDepth, FollowSymlinks=false
```

Then both `fileindex.Index.Refresh` and the new directory expander call into
`dirwalk.Walk`. The shared helper:

1. Try `git ls-files --cached --others --exclude-standard -z` rooted at the
   target dir. If `git` returns non-zero or the dir is outside any repo, fall
   back to `filepath.WalkDir`.
2. Apply excludes: `.git`, `node_modules`, `.svn`, `vendor`, `dist`,
   `.gocache`, `.tmp-config`, `.next`, `target`, `build`, `out`, `coverage`.
3. Skip symlinks by default (path-safety; avoids loops).
4. Sort the final list for deterministic output (lexicographic by relative
   path).

Symlink handling deserves an explicit decision: **do not follow**. Skip with
no error. Following symlinks gives us loops, denial-of-service via giant
`/proc` mounts, and path-escape surprises. The picker already takes this
stance.

## Per-file inlining rules (within a directory)

For each walked entry that is a regular file:

1. Re-resolve through `tools.ResolvePath(ctx, abs, tools.PathRead)`. The walk
   started from a resolved root, but child paths must still pass the policy
   gate (denylist + roots). On deny, skip silently — counted in
   `SkippedCount`.
2. Stat → if size > `EffectiveMaxReadChars()` × 4, skip as oversize. (The ×4
   factor avoids reading huge files just to truncate; cheap files are still
   fully read and may be truncated.)
3. Read; if `!utf8.Valid`, skip as binary. Increment `SkippedCount`.
4. Truncate to `EffectiveMaxReadChars()` (per-file cap, same as today).
5. Charge bytes to per-dir and per-prompt budgets. If the next file would
   overshoot the per-dir or per-prompt byte budget, stop walking, mark
   truncated, record reason.
6. Charge file count to per-dir and per-prompt counters. Same stop-on-overshoot
   rule.
7. Call `ctx.RecordFileSnapshot` (existing hook) so file edits during the turn
   stay snapshot-aware.

## Rendering

Each directory emits one block. Tree summary first, then file blocks inline
with the same `<file>` shape used today so downstream parsing doesn't change.

```
<directory path="docs" files="34" bytes="287144" truncated="false">
<tree>
docs/
├── PHASE-1-DETAILED-PLAN.md
├── PHASE-2-DETAILED-PLAN.md
├── …
└── PHASE-LOG.md
</tree>
<file path="docs/PHASE-1-DETAILED-PLAN.md">
…contents…
</file>
<file path="docs/PHASE-2-DETAILED-PLAN.md" truncated="true">
…contents…
</file>
…
</directory>
```

Rules:

- `path` is the user's mention path normalized to forward slashes, relative to
  `ctx.WorkingDir` when possible (matches existing file-block behavior).
- The tree lists **walked** entries, including ones we skipped (mark them
  `[skipped: binary]`, `[skipped: too-large]`). This gives the model a
  complete picture without inflating the byte budget.
- Truncation is surfaced both at the directory level (attribute) and at the
  file level (per-file `truncated="true"` already exists).
- A single appended section header `Referenced files and directories:` (was
  `Referenced files:`) — keep backwards-compatible by detecting whether any
  directories were resolved before deciding which header to emit.

## Multi-mention semantics

Three rules cover the messy cases:

1. **Order = mention order.** `@a @b @a` deduplicates `@a` and processes
   `[a, b]`. Existing behavior; keep it.
2. **Nesting overlap.** If the user mentions both `@docs/` and
   `@docs/PHASE-1-DETAILED-PLAN.md`, expand `@docs/` (which already inlines
   that file) and **drop** the redundant file mention. Detection: after
   resolving all roots, drop any file mention that lives under any directory
   mention's `AbsPath`. Surface a single line in the directory block:
   `note="dropped 1 redundant file mention"`.
3. **Sibling directories overlap (`@a/b/` and `@a/`).** Expand the parent;
   drop the child. Same rule as #2.

These rules avoid the obvious double-billing of bytes against the prompt
budget and stop the model from seeing the same file twice with different
preambles.

## Path safety and policy

- Every walked path goes through `tools.ResolvePath`. No exceptions.
- Roots that resolve outside the policy's allowed roots return the existing
  error and the **whole prompt fails** — do not silently drop. This matches
  current behavior for files. Users get the same loud failure mode they
  already understand.
- Hidden directories (`.claude`, `.codex`) are walked normally if explicitly
  mentioned. They are excluded from default top-level walks only when not
  explicitly named — but since the user typed `@.codex`, the user is opting
  in. Implement this by applying the exclude list only to **descendants**, not
  the root itself.

## TUI surface

Two small changes:

1. Picker already lists directories. Today, accepting a directory keeps the
   picker open and drills in (Phase 26 behavior). Add a second key — `Shift+Tab`
   — that **accepts the directory as-is** with a trailing `/`, closes the
   picker, and treats it as a directory mention. Documented in the picker
   hint footer.
2. Transcript: when a prompt resolves directories, render a single-line note
   under the user's bubble: `expanded 3 directories, 87 files, 1.2 MiB`.
   Reuses the existing `ResolvedFile`-style summary path; just sums across
   both slices.

Both changes are nice-to-have; the feature works without them.

## Edge cases (explicit decisions)

| Case | Behavior |
|---|---|
| Empty directory | Emit a `<directory>` block with `files="0"` and an empty `<tree>` body. No error. |
| Directory contains only binaries | Same as above, with `SkippedCount>0`. |
| Symlinked directory | Skipped silently. Counted in tree as `[skipped: symlink]`. |
| Symlinked regular file pointing inside roots | Skipped (consistent with dir rule). |
| Directory mentioned twice | Deduplicated in `extractMentionPaths`. |
| Very deep tree | Walk stops at `MaxDirDepth`; truncation reason `depth-cap`. |
| Permission-denied subdir | Skipped; recorded in tree as `[skipped: permission]`. Walk continues. |
| `@./` or `@.` | Normalize to working-dir root; expand it. Caps are the only protection. |
| Prompt budget exhausted by directory N of M | Remaining directories emit a `<directory>` block with `files="0" truncated="true" reason="prompt-byte-cap"`. They do **not** silently disappear — the model sees the user wanted them. |
| Trailing slash vs. no trailing slash | Identical semantics. `@docs` and `@docs/` both work. |
| Mention path is a file (existing behavior) | Unchanged. |

## Phase 1 — Refactor walker

Extract `dirwalk` from `internal/tui/fileindex/index.go`:

- New `internal/tools/dirwalk/walk.go`
- `internal/tui/fileindex/index.go:Refresh` calls `dirwalk.Walk`
- All existing fileindex tests stay green (regression target)

This unblocks the directory expander without copy-paste.

## Phase 2 — Mention expander

Edits to `internal/mentions/expand.go`:

1. Replace the IsDir error at line 53 with a branch into `expandDirectory`.
2. Add `expandDirectory(ctx, abs, raw, budget)` that calls `dirwalk.Walk` and
   emits the `<directory>…</directory>` block.
3. Track a per-prompt `budget` struct (files, bytes) shared across all
   mentions. File branch charges the same budget.
4. Add the overlap-resolution pass after `extractMentionPaths`: split into
   files vs. dirs after stat, then prune redundant entries.
5. Return the new `[]ResolvedDirectory` slice.

Update callers:

- `internal/tui/app.go:386` (submit) — accept the new slice; surface the
  one-line transcript note.
- REPL submit path — same.
- Any test that calls `ExpandPrompt` — extend signature.

## Phase 3 — Context knobs

`internal/tools/context.go`:

```go
MaxDirFiles    int
MaxPromptFiles int
MaxDirBytes    int64
MaxPromptBytes int64
MaxDirDepth    int
```

Plus `EffectiveMaxDirFiles()` etc. with the defaults listed above. Wire
through `internal/bootstrap/state.go` so config files / env can override.

## Phase 4 — Tests

`internal/tools/dirwalk/walk_test.go`:

1. Walk respects exclude list (`.git`, `node_modules` etc.).
2. Walk stops at MaxFiles cap and reports `Stats.Truncated=true`.
3. Walk stops at MaxBytes cap with reason `byte-cap`.
4. Walk stops at MaxDepth cap with reason `depth-cap`.
5. Symlinked dir skipped, walk continues.
6. Permission-denied subdir skipped, walk continues.
7. Empty directory returns zero entries, no error.

`internal/mentions/expand_test.go` (extend):

8. `@dir/` inlines all utf-8 files in the directory in lexicographic order.
9. Binary file in directory is skipped and counted; tree marks it `[skipped: binary]`.
10. Per-directory byte cap stops walk early; `truncated="true"` and
    `reason="byte-cap"` set.
11. Multi-directory prompt shares prompt budget; later directories degrade
    cleanly when prompt cap hits.
12. Mixed file + directory mention with overlap drops the redundant file
    mention and adds a note.
13. Sibling overlap (`@a/` and `@a/b/`) keeps parent only.
14. Directory outside allowed roots → whole prompt errors (parity with files).
15. Hidden dir mentioned explicitly (`@.codex`) is walked.
16. Recorded snapshots fire for every inlined file (asserts `RecordFileSnapshot`
    called per path).
17. Round-trip: result string contains exactly one `<directory>` block per
    directory mention.

`internal/tui/app_test.go`:

18. Submitting a prompt with one directory mention surfaces the
    "expanded N directories" transcript line.
19. Picker `Shift+Tab` on a directory accepts it as a directory mention
    (closes picker, no drill-in).

## Phase 5 — Documentation

Update:

- `README.md` "Prompt syntax" section — document `@dir/` and the caps.
- `docs/PHASE-LOG.md` — add the Phase 27 implementation addendum after
  shipping.

No CLAUDE.md change needed.

## Phase 6 — Out of scope (call out, don't build)

- Glob mentions (`@docs/**/*.md`). Doable later by detecting `*` or `?` in
  the normalized path and routing through the existing `glob` tool.
- Selective inclusion filters (`@docs/?ext=md`). Premature.
- Auto-summarization of large directories instead of inlining. Belongs in
  the compaction pipeline, not in mention expansion.
- Persistent per-workspace overrides for caps. Live in config, not here.
- Live re-expansion when files change mid-turn.

## Implementation order

1. Phase 1 — extract `dirwalk`. Lands cleanly; no behavior change.
2. Phase 3 — add context knobs (used by Phase 2). Defaults only; no caller
   need pass anything yet.
3. Phase 2 — directory expander + overlap resolution + caller signature
   update.
4. Phase 4 — tests, alongside Phase 2 where natural.
5. Phase 5 — docs.
6. Picker `Shift+Tab` — small follow-up, can land in the same PR or a
   trailing one.

Each phase compiles and ships independently. The directory branch is gated
by the IsDir check, so until Phase 2 lands the existing file-only behavior
is untouched.

## Minimal file change set

- `internal/tools/dirwalk/walk.go` — new (extracted from fileindex)
- `internal/tools/dirwalk/walk_test.go` — new
- `internal/tui/fileindex/index.go` — switch to `dirwalk.Walk`
- `internal/tools/context.go` — new knobs + `Effective*` helpers
- `internal/bootstrap/state.go` — wire defaults / overrides
- `internal/mentions/expand.go` — directory branch, overlap resolution,
  new return value
- `internal/mentions/expand_test.go` — extend
- `internal/tui/app.go` — new signature, transcript note, optional
  `Shift+Tab` accept
- `internal/tui/app_test.go` — extend
- `internal/cli/repl.go` — new signature
- `README.md` — prompt syntax docs

## Design decisions to keep

1. **Caps enforce graceful degradation, not errors.** A directory that hits
   a cap inlines what it can and tells the model what it skipped. Errors are
   reserved for path-safety violations and genuine IO failures.
2. **One walker, two callers.** Picker index and prompt expansion share
   `dirwalk`. Diverging walkers will drift in exclusion lists and bite us.
3. **Don't follow symlinks.** Cheap rule, large class of bugs avoided.
4. **Overlap pruning is silent-but-noted.** Drop redundant mentions
   automatically; surface a one-line note so the user can verify.
5. **Single shared prompt budget.** Per-directory caps are not enough — three
   large directories must collectively fit.
6. **Directory blocks are additive XML.** The model already understands
   `<file>`; `<directory><tree>…</tree><file>…</file>…</directory>` is the
   smallest extension that preserves the existing parser.

## Recommendation

Land Phases 1+3 as a small refactor PR (no behavior change). Then ship
Phases 2+4+5 as the feature PR. The picker `Shift+Tab` follow-up is optional
and can wait — typing `@docs/` manually already works once Phase 2 ships.
