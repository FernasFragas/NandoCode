# Phase 26 — Inline Completion in the TUI Input

## Goal

Add an inline completion picker to the TUI input so the user can quickly insert
file references (`@path`) and slash commands (`/cmd`) without leaving insert
mode. Layered on top of the existing `textarea.Model`. The agent loop and
prompt expansion are untouched — completion is purely TUI assistance.

## Scope expansion vs. the original draft

The first draft only covered `@file` completion. This plan widens scope to
deliver a single picker that handles **two trigger contexts** with shared
infrastructure:

| Trigger | Source | Acceptance |
|---|---|---|
| `@<query>` | Workspace files (path-safe) | Inserts `@relative/path` into the buffer |
| `/<query>` at start of line | Registered slash commands | Inserts `/command ` into the buffer |

Both use the same picker UI, key bindings, ranking pipeline, and cache. This
costs little extra code and removes a class of UX papercuts (users typing
`/he<Tab>` already expect completion).

A third trigger (`@@symbol`) for symbol search is explicitly out of scope —
deferred until we have a symbol index.

## Key UX improvements over draft v1

1. **Fuzzy matching with subsequence scoring**, not strict prefix. Typing
   `@itap` should match `internal/tui/app.go`. Strict prefix is too strict in
   any non-trivial repo and is the single biggest reason inline pickers feel
   bad.
2. **Frecency ranking**: weight recently-mentioned and recently-edited files
   higher. Cheap to implement (in-memory map keyed by absolute path), big UX
   win — the file you just inserted is one keystroke away the next time.
3. **Empty-query suggestions**: when the user types just `@` with nothing
   after, show recent files and top-level directories instead of nothing.
   Removes the cold-start problem.
4. **Cached, debounced index**, not synchronous `filepath.WalkDir` on every
   keypress. The draft's "synchronous … is acceptable for v1" is a trap on
   any repo with >10k files — keystroke latency becomes user-visible.
   Build the index once at startup (and refresh on a coarse interval or on
   explicit refresh), filter in memory.
5. **Git-aware listing when available**: prefer `git ls-files --cached --others
   --exclude-standard` over manual walking. It's faster and respects
   `.gitignore` for free. Fall back to walking with the existing exclude list.
6. **Highlight matched characters** in suggestions (lipgloss bold on matched
   runes). Makes ranking feel intentional and helps users refine the query.
7. **Accept on Tab and Enter-when-picker-visible**: the draft argued against
   Enter to avoid send/complete ambiguity. The cleaner rule is *picker
   visibility wins*: if the picker is open, Enter accepts. The user explicitly
   triggered the picker by typing `@` or `/`; Esc to dismiss is the standard
   escape hatch. This matches every IDE/shell completion the user has muscle
   memory for. Document it once and ship it.
8. **Right-arrow at end-of-buffer accepts** (shell-style "ghost text" feel,
   optional v1.1).
9. **Shared mention parser from day one**, not a Phase 6 refactor. Diverging
   parsers between picker and `mentions.ExpandPrompt` is a guaranteed source
   of "the picker suggested it but submit rejected it" bugs.

## Integration points (current code)

- Input model and key handling: `internal/tui/app.go:24` and
  `internal/tui/app.go:262`
- Prompt submission with mention expansion: `internal/tui/app.go:386`
- Input rendering: `internal/tui/app.go:713`
- Mention parser: `internal/mentions/expand.go:90`
- Path safety: `internal/tools/pathsafe.go` (already enforced inside
  `tools.ResolvePath`)
- Slash command registry: `internal/commands` (used at `internal/tui/app.go:359`)

## Architecture

```
┌───────────────────────────────────────────────────────────────┐
│  textarea.Model  (unchanged behavior)                         │
└─────────────────┬─────────────────────────────────────────────┘
                  │ value + cursor
                  ▼
┌───────────────────────────────────────────────────────────────┐
│  TriggerDetector                                              │
│    detects active @-token or leading /-token at cursor        │
│    → TriggerContext { Kind, Start, End, Query }               │
└─────────────────┬─────────────────────────────────────────────┘
                  │
                  ▼
┌───────────────────────────────────────────────────────────────┐
│  SuggestionProvider (interface)                               │
│   ├── FileProvider     (reads from FileIndex + frecency)      │
│   └── CommandProvider  (reads from commands.Registry)         │
└─────────────────┬─────────────────────────────────────────────┘
                  │ ranked []Suggestion
                  ▼
┌───────────────────────────────────────────────────────────────┐
│  PickerState  (ephemeral on Model)                            │
│    visible, items, index, trigger                             │
└─────────────────┬─────────────────────────────────────────────┘
                  │
                  ▼
┌───────────────────────────────────────────────────────────────┐
│  renderPicker()  (called from renderInput)                    │
└───────────────────────────────────────────────────────────────┘
```

The provider interface keeps the file and command paths from leaking into
each other while sharing the picker shell.

## Phase 1 — Shared mention parsing (do this first)

Move token extraction into reusable helpers. This is **before** the picker so
both code paths share semantics from day one.

In `internal/mentions/expand.go`:

```go
// TokenAtCursor returns the @-token currently under the cursor, if any.
// pos is a rune offset (not byte). Token.Active=false when not inside one.
func TokenAtCursor(line string, pos int) Token

// NormalizeMentionPath cleans a raw query into the form ExpandPrompt expects.
func NormalizeMentionPath(raw string) string
```

`extractMentionPaths` becomes a thin caller of these. Existing tests stay
green; add cursor-position tests.

## Phase 2 — File index

New `internal/tui/fileindex/index.go`:

```go
type Index struct {
    root    string
    entries []Entry        // immutable snapshot
    byPath  map[string]int // entries lookup
    mu      sync.RWMutex
}

type Entry struct {
    Rel      string  // forward-slash relative path
    Base     string
    IsDir    bool
}

func New(root string) *Index
func (i *Index) Refresh(ctx context.Context) error
func (i *Index) Snapshot() []Entry
```

Population strategy in `Refresh`:

1. Try `git ls-files --cached --others --exclude-standard -z` (fast, respects
   `.gitignore`). Add directory entries by deduping path prefixes.
2. Fallback: `filepath.WalkDir` with excludes
   `{.git, node_modules, .svn, vendor, dist, .gocache, .tmp-config}`.
3. Cap at e.g. 50k entries; if exceeded, set a degraded-mode flag and stop
   walking. Renderer surfaces a one-line "index truncated" hint.

Refresh policy:

- Once at TUI startup (async, via `tea.Cmd`).
- After agent run completes (cheap; new files may exist).
- On explicit `/refresh-index` slash command (escape hatch).
- **Not** on every keystroke.

## Phase 3 — Frecency tracker

New `internal/tui/fileindex/frecency.go`:

```go
type Frecency struct {
    scores map[string]float64 // rel path → score
    mu     sync.Mutex
}

func (f *Frecency) Touch(rel string)              // bump on accept
func (f *Frecency) Score(rel string) float64      // for ranking
func (f *Frecency) Decay()                        // halve all scores; called periodically
```

Persistence is out of scope for v1. Session-only. If we want persistence
later, dump to `~/.config/nandocodego/frecency.json` on shutdown.

## Phase 4 — Suggestion provider interface

New `internal/tui/picker/provider.go`:

```go
type Trigger int
const (
    TriggerFile Trigger = iota
    TriggerCommand
)

type Suggestion struct {
    Display    string  // "internal/tui/app.go"
    Insert     string  // "internal/tui/app.go" (no leading @ — picker adds it)
    Detail     string  // optional right-aligned hint, e.g. "dir" or "core"
    IsDir      bool
    MatchRunes []int   // indices into Display for highlight
    Score      float64
}

type Provider interface {
    Suggest(query string, limit int) []Suggestion
}
```

`FileProvider` ranks via `score = fuzzyScore(query, candidate) + α·frecency`.
Use a small subsequence scorer (junegunn/fzf-style is overkill — see
implementation note below). `CommandProvider` lists commands from
`commands.Registry`; queries are short and prefix-sensible there.

**Fuzzy scoring** (lightweight): for a query of length q and candidate c,
walk c left-to-right, tracking matched indices for each query rune. Score:

- +10 per matched rune
- +5 if matched rune is at a path separator boundary (camelCase/`/`/`_`)
- +20 if query is a prefix of basename
- −1 per gap rune between matches

Limit to ~150 LOC, no dependencies.

## Phase 5 — Trigger detection

New `internal/tui/picker/trigger.go`:

```go
type Context struct {
    Kind    Trigger
    Start   int   // rune offset of trigger char (@ or /)
    End     int   // rune offset (exclusive) — end of current token
    Query   string
    Active  bool
}

func Detect(line string, cursor int) Context
```

Rules:

- `@`: token starts with `@`, ends at whitespace or end-of-line; cursor must be
  inside `[start, end]`. Empty query (`@` alone) is active.
- `/`: only active when the trigger char is the **first non-whitespace rune of
  the line** and cursor is in the same token. Avoids `@scope/pkg` and `path/x`
  triggering command completion.
- Both: ignore inside an in-progress backtick code span on the line (rare in
  prompts but cheap to handle: count backticks before cursor; odd count = skip).

## Phase 6 — Picker state on Model

Extend `Model` in `internal/tui/app.go:24` with:

```go
fileIndex   *fileindex.Index
frecency    *fileindex.Frecency
picker      picker.State    // value type, zero = closed
providers   map[picker.Trigger]picker.Provider
```

`picker.State`:

```go
type State struct {
    Visible bool
    Trigger Trigger
    Token   trigger.Context
    Items   []Suggestion
    Index   int
}
```

Update triggers — call `m.refreshPicker()` at the **end** of `Update` after
the textarea has consumed the key, so we read post-update value/cursor:

- After any `KeyMsg` while in insert mode and no permission modal.
- After `WindowSizeMsg` (only to re-clamp index/visible based on space).
- Force-close on: leaving insert mode, prompt submit, Esc when visible,
  `agentDoneMsg`/`permissionPromptMsg`.

`refreshPicker` flow:

1. `picker.Detect(line, cursor)` → if not active, close picker, return.
2. Pick provider by trigger kind.
3. `provider.Suggest(query, limit=8)`.
4. Update `state.Items`, clamp `state.Index`, set `Visible=len(items)>0 || query==""`.

## Phase 7 — Key bindings

Insert mode, picker **visible**:

| Key | Action |
|---|---|
| `Tab` | Accept current item |
| `Enter` | Accept current item (does **not** submit) |
| `Up` / `Ctrl+P` | Move selection up |
| `Down` / `Ctrl+N` | Move selection down |
| `Esc` | Close picker; **do not** leave insert mode (one-shot) |
| any other key | Forward to textarea, then refresh picker |

Insert mode, picker **closed**:

| Key | Action |
|---|---|
| `Enter` | Submit prompt (existing behavior) |
| `Esc` | Leave insert mode (existing behavior) |
| any other | Existing behavior |

The "Esc closes picker without leaving insert mode" rule is the one place we
deviate from a strict layered model. Worth it: matches every IDE.

## Phase 8 — Insertion

Replace the active token range `[Token.Start, Token.End)` with the selected
insert text. Always preserve the leading trigger char.

- File, regular: insert `@internal/tui/app.go`, append a single space, close
  picker, `Frecency.Touch(rel)`.
- File, directory: insert `@internal/tui/`, **no trailing space**, keep picker
  open, refresh suggestions for the new query (drilling).
- Command: insert `/help`, append a single space, close picker.

Cursor lands immediately after the inserted text (and the appended space).

## Phase 9 — Rendering

Extend `renderInput` in `internal/tui/app.go:713`:

```
┌────────────────────────────────────────────┐
│ Tell me about @internal/tu█                │   ← textarea
└────────────────────────────────────────────┘
┌────────────────────────────────────────────┐
│ › internal/tui/app.go              260 ln  │   ← picker panel
│   internal/tui/messages.go          50 ln  │      (only when Visible)
│   internal/tui/                     dir    │
│   internal/tools/                   dir    │
│   internal/tui/transcript.go        80 ln  │
└────────────────────────────────────────────┘
  Tab accept · ↑↓ navigate · Esc close
```

Guidelines:

- Cap visible rows to `min(len(items), 8, viewportRoom-2)`. Reduce viewport
  height by the panel height so the transcript doesn't get clipped.
- Bold the matched runes (`MatchRunes`) per row.
- Right-align a `Detail` column (line count for files, `dir` for directories,
  command summary for commands).
- One-line footer with the active key hints, dim style.
- Empty results with non-empty query: render a single muted row "No matches".
- Don't render in normal mode.

New styles in `internal/tui/styles.go`:

```go
PickerPanel    lipgloss.Style
PickerItem     lipgloss.Style
PickerSelected lipgloss.Style
PickerMatch    lipgloss.Style  // bold for highlighted runes
PickerDetail   lipgloss.Style  // dim, right-aligned
PickerHint     lipgloss.Style  // footer line
```

## Phase 10 — Edge cases (explicit decisions)

| Case | Behavior |
|---|---|
| `@` at start of buffer | Active, empty query → recent files + top-level dirs |
| Multiple mentions in line | Only the one under cursor is active |
| Mid-sentence mention | Same as above; whitespace delimits |
| `@internal//t` (double slash) | Treat as malformed; show "No matches" rather than crashing — the normalizer collapses `//` for matching but keeps the user's text until they accept |
| Hidden directories (`.foo`) | Indexed unless excluded; ranked normally |
| Directory with many children | Drilling shows top 8 by frecency + name |
| Query with `./` | Strip prefix when matching |
| Query points outside allowed roots | Provider returns empty |
| Punctuation suffix (`@file.go.`) | Token end is whitespace; the trailing `.` is part of the query, so suggestions filter accordingly. Acceptance rewrites the full token, so the `.` is gone after Tab |
| Spaces in filenames | Out of scope; documented limitation |
| File too large to expand later | Picker still suggests it. Expansion enforces read limits at submit time, same as today |
| `/` mid-line | Not a trigger; user is typing a path, not a command |
| `/<unknown>` | Picker shows "No matches"; Enter submits as today (registry handles unknown command) |

## Phase 11 — Tests

`internal/mentions/expand_test.go` (extend):

1. `TokenAtCursor` returns inactive when cursor not in `@`-token
2. `TokenAtCursor` finds correct bounds with multiple mentions
3. `NormalizeMentionPath` strips trailing punctuation, leading `./`

`internal/tui/picker/trigger_test.go`:

4. `@` at start, mid, end of line
5. `/` only triggers at start of line
6. Cursor outside any token → inactive
7. Backtick span suppresses trigger

`internal/tui/picker/provider_test.go`:

8. File provider ranks basename-prefix above substring
9. Frecency boost moves recently-touched file up
10. Empty query returns recent + top-level dirs (capped)
11. Path-aware query (`internal/t`) filters by prefix on full path
12. Rejects suggestions outside allowed roots
13. Command provider lists registered commands, filters by prefix

`internal/tui/fileindex/index_test.go`:

14. Walk fallback excludes `.git`, `node_modules`, etc.
15. Refresh is idempotent
16. Truncation flag set when over cap

`internal/tui/app_test.go` (model-level):

17. Tab inserts selected file path and closes picker
18. Selecting directory inserts trailing `/` and keeps picker open
19. Esc closes picker, stays in insert mode
20. Enter while picker visible accepts (does not submit)
21. Enter while picker closed submits as before
22. Multiple mentions: only the active one is replaced
23. After acceptance, `mentions.ExpandPrompt` resolves the inserted path
24. Frecency.Touch is called on accept

## Phase 12 — Out of scope (call out, don't build)

- Symbol search (`@@`)
- Persistent frecency across sessions
- Asynchronous "ghost text" right-arrow accept (good v1.1 add)
- `#` for tags or PRs
- Spaces in filenames

## Implementation order

1. Phase 1 — extract `TokenAtCursor` + `NormalizeMentionPath`, port existing
   parser through them. Keep ExpandPrompt behavior identical.
2. Phase 5 — `picker.Detect` (depends on Phase 1).
3. Phase 2 — `fileindex.Index` with git+walk paths.
4. Phase 3 — `Frecency`.
5. Phase 4 — Provider interface + `FileProvider` + `CommandProvider` + scorer.
6. Phase 6 — Model fields, refresh wiring, lifecycle close points.
7. Phase 7 — Key bindings.
8. Phase 8 — Insertion (file / dir / command branches).
9. Phase 9 — Rendering + styles.
10. Phase 11 — Tests, in parallel with each phase where natural.

Each phase compiles and ships independently; the picker stays
`Visible=false` until Phase 6 wires it in.

## Minimal file change set

- `internal/mentions/expand.go` — extract shared helpers
- `internal/tui/app.go` — model fields, key handling, render hook, lifecycle
- `internal/tui/styles.go` — picker styles
- `internal/tui/fileindex/index.go` — new
- `internal/tui/fileindex/frecency.go` — new
- `internal/tui/picker/trigger.go` — new
- `internal/tui/picker/provider.go` — new
- `internal/tui/picker/file_provider.go` — new
- `internal/tui/picker/command_provider.go` — new
- `internal/tui/picker/score.go` — new (subsequence scorer)
- `internal/tui/picker/render.go` — new
- corresponding `_test.go` files

## Design decisions to keep

1. **Completion is TUI-only.** The agent and `mentions.ExpandPrompt` see only
   the final prompt text. Path safety remains enforced at expansion time, not
   at picker time — picker rejection is best-effort UX, expansion rejection is
   the source of truth.
2. **Picker is value-typed, not a Bubble Tea sub-model.** It has no
   independent `Update`/`View` lifecycle; the parent model owns everything.
   Keeps Bubble Tea message routing simple and avoids focus management.
3. **One picker, multiple providers.** Sharing UI across `@` and `/` pays for
   itself the day we add a third trigger.
4. **Index is a snapshot, not a live watcher.** No `fsnotify`. Refresh on
   coarse events (startup, post-run, manual). Live watching is a maintenance
   tax with poor return.

## Recommendation

Build phases 1–9 as a single PR sequence; ship behind no flag (it's purely
additive). Land Phase 1 alone first if the parser refactor is touchy — it has
value on its own.
