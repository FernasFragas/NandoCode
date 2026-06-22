# Phase 12 Detailed Plan - Skills (File-Driven Tools)

Date: 2026-05-07
Status: ✅ Implemented in code and automated checks (2026-05-08); manual live REPL exit-gate still pending
Source plans:

- `.codex/go-ollama-plan-AGENTS.md`
- `.codex/go-ollama-plan-HUMANS.md`
- `docs/PHASE-LOG.md`
- `docs/PROJECT-STATUS-AND-ONBOARDING.md`
- `docs/PHASE-8-DETAILED-PLAN.md`
- `docs/PHASE-9-DETAILED-PLAN.md`
- `book/ch12-extensibility.md`
- `.codex/agent-context/learnings-memory.md`

## Goal

Phase 12 implements file-driven skill prompts that users and projects can define to extend and customize agent behavior across sessions. Skills are Markdown files with YAML frontmatter. They are loaded from a priority-ordered source chain and invoked by the agent through a `SkillTool`. They are NOT executable tools — they inject behavior descriptions as system context for the model to adopt on demand.

Deliverables:

- `internal/skills` package with loading, source priority, scanning, hot-reload, and embed support.
- A `SkillTool` in `internal/tools/skilltool/` that surfaces skills to the agent loop as a first-class tool.
- Three bundled starter skills via `embed.FS`: `code-review`, `debug-session`, and `write-tests`.
- Hot reload via `github.com/fsnotify/fsnotify` for user and project skill directories.
- `/skills list` and `/skills show <name>` slash commands in the TUI.
- A clear source-priority model so project skills override user skills, and MCP-provided skills override project skills.
- Tests for loading, hot reload, source priority, MCP skills, and the SkillTool invocation contract.
- Phase log update after implementation.

## Definition Of Success

The Phase 12 exit gate is a three-step flow:

1. Create a project skill file at `.nandocodego/skills/my-review.md` with a code-review checklist in the frontmatter and body.
2. Start the REPL. Ask the agent to review a file. Invoke the skill via the SkillTool. Verify the assistant uses the checklist from the file without being prompted to do so.
3. Write a new skill file to `.nandocodego/skills/` while the REPL is running. Within 1 second, verify that `/skills list` includes the new skill without restarting the session.

This exit gate must work with a local Ollama endpoint and without any network destination other than Ollama.

## Baseline Analysis From Implemented Phases

### Phase 0 - Security and Supply Chain

Implemented:

- `SECURITY.md`
- dependency allowlist
- network policy checker
- CI/security baseline
- no-secrets policy

Phase 12 implications:

- Skill files are user-editable Markdown content. They must never be treated as executable code. Skills influence model behavior through prompt injection only.
- Skill content must never be logged at INFO; it may contain sensitive system-prompt logic.
- Hot reload via fsnotify must not introduce a TOCTOU security issue. Since skills are non-executable prompt text (not process-launched like hooks), hot reload is safe here.
- `github.com/fsnotify/fsnotify` is listed as a Phase 10 added dependency; it must be verified in `tools/allowed-deps.txt` before Phase 12 use.
- MCP-provided skills enter through the Phase 10 trust boundary and must be treated as external data, not trusted system configuration.
- Skills must not have an auto-execute path. They are context, not policy.

### Phase 1 - CLI, Paths, Logging, Scaffold

Implemented:

- `internal/paths` with `SkillsDir()` and `ProjectSkillsDir()` helpers already present.
- `internal/logging` with structured slog.
- Empty `internal/skills/` scaffold directory.

Phase 12 implications:

- Reuse `internal/paths.SkillsDir()` for user-level skills (`~/.nandocodego/skills/`).
- Reuse `internal/paths.ProjectSkillsDir()` for project-level skills (`.nandocodego/skills/`).
- Do not introduce new XDG path logic; path helpers already exist.
- Log skill scan counts, load errors, and hot-reload events at DEBUG only. Never log skill body content at INFO.

### Phase 2 - LLM Client

Implemented:

- Provider-neutral `llm.Client`.
- `ChatRequest.Format` for structured JSON output.
- Streaming and non-stream chat support.

Phase 12 implications:

- SkillTool does not call `llm.Client` directly. It returns skill content as a string result, and the agent loop injects it into the conversation context.
- If a future "skill auto-select" feature is desired (similar to memory recall), it would use `llm.Client` in a separate select step. That is explicitly out of scope for Phase 12.
- No structured output is needed for Phase 12's core path.

### Phase 3 - Tool Interface and Starter Tools

Implemented:

- `tools.Tool` interface.
- `tools.Context` with permission mode, working dirs, env.
- `tools.Registry` for deterministic lookup.
- `Bash`, `FileRead`, `FileWrite`.
- Path safety helpers.

Phase 12 implications:

- `SkillTool` must implement the `tools.Tool` interface exactly like the starter tools.
- `SkillTool` must produce a `tools.Result` that contains the skill content as its display string.
- The SkillTool is read-only and concurrency-safe; it accesses only in-memory loaded skill data and the on-disk Markdown file at invocation time.
- SkillTool must be registered in `internal/tools/builtin` or a new registry extension, not hardcoded in the agent loop.
- Permission classification for SkillTool: always `PermAllow`, no user confirmation needed. Skills are not executable.

### Phase 4 - Agent Loop

Implemented:

- `agent.Run(ctx, input) <-chan agent.Event`.
- `agent.Input.SystemPrompt`.
- Tool execution loop.
- `agent.Terminal` event with conversation payload.

Phase 12 implications:

- SkillTool result is injected into the conversation as a tool result message, not as a mutation of `Input.SystemPrompt`.
- The agent loop already handles tool results as `llm.RoleTool` messages appended to history. SkillTool follows the same path.
- SkillTool result text should instruct the model to adopt the behavior described for the remainder of the session.
- Framing the skill content as a tool result (rather than a system prompt amendment) means it stays visible in conversation history, and the model can reference it across turns.

### Phase 5 - Permission System

Implemented:

- Central `permissions.Resolve`.
- Source-tagged rules.
- `PermissionMode` modes.
- Hook decision and prompt function extension points.

Phase 12 implications:

- SkillTool always returns `PermAllow` from its classifier because skill loading is a read-only, safe operation.
- Skills themselves cannot trigger any permission path; they describe behavior, they do not execute commands.
- No skill should ever bypass the permission system by encoding shell commands in its body for direct execution.
- The loader must not provide an auto-execute path for skill body content.

### Phase 6 - State Layer

Implemented:

- `bootstrap.State` with `Snapshot()` and `Update()`.
- `state.Store[state.App]`.
- `state.App.ToolSettings`.
- `state.OnChange`.

Phase 12 implications:

- The loaded skill registry should be held in the REPL startup context or the TUI model, not in `bootstrap.State`.
- Skills are session-scoped (loaded at startup, hot-reloaded as files change). They are not infrastructure state.
- `state.App` should not hold the full loaded skills list; the SkillTool holds a reference to the loader.
- Hot-reload notifications can be surfaced through a TUI system item or log message; they do not need to trigger a `state.Store.Set` call.

### Phase 7 - Bubble Tea TUI and REPL

Implemented:

- Interactive REPL with prompt submission.
- `internal/tui/slash.go` with `/help`, `/clear`, `/exit`, `/model`.
- Agent bridge and transcript rendering.
- Permission modal.

Phase 12 implications:

- Phase 12 adds `/skills list` and `/skills show <name>` to the slash command handler in `internal/tui/slash.go`.
- `/skills list` should show each skill's name, source, and description from loaded frontmatter only. Full body is not displayed in the list.
- `/skills show <name>` renders the full skill Markdown body using the existing Glamour renderer.
- No new Bubble Tea message types are needed for slash commands that only produce transcript items.
- Hot-reload events should surface as a small system transcript item (e.g., "Skill `my-review` reloaded").

### Phase 8 - Memory Core

Implemented:

- `internal/memory` with frontmatter scanning, index loading, recall, and runner decorator.
- `memory.NewRunner` wraps the agent.

Phase 12 implications:

- Skills follow the same two-phase pattern as memory: frontmatter scanned at startup for listing/search; full body loaded on invocation.
- Skills use `gopkg.in/yaml.v3` for frontmatter parsing, the same dependency already added in Phase 8.
- The `internal/skills` loader must not share state with `internal/memory`; they are independent subsystems.
- Skills do not use an LLM recall query. Selection is explicit (tool call with skill name), not semantic matching. This is the primary design difference from memory recall.

### Phase 9 - Hooks

Implemented:

- `internal/hooks` with snapshot, matcher, command/prompt runners, and runner decorator.
- Hook config loaded once at session start.
- `hooks.NewRunner` wraps the memory runner.

Phase 12 implications:

- The composition root in `internal/cli/repl.go` currently has the chain: `agent -> memory.Runner -> hooks.Runner -> TUI`.
- Phase 12 does not need to add another runner decorator. The SkillTool is registered in the tool registry and invoked by the existing agent loop.
- Hot reload is not needed for hook snapshots (frozen by design); for skills, hot reload is acceptable because skills are not policy-enforcement code.
- Skills and hooks are both described as "extensibility" in `book/ch12-extensibility.md`, but they occupy different layers: hooks are control-plane interceptors; skills are model behavior context.

### Phase 10 - MCP (Treat As Complete)

Implemented:

- `github.com/modelcontextprotocol/go-sdk` for MCP tool integration.
- `github.com/zalando/go-keyring` for credential storage.
- `golang.org/x/oauth2` for OAuth flows.
- `github.com/fsnotify/fsnotify` for config file watching.

Phase 12 implications:

- MCP servers can provide skills through a resource type. The Phase 12 skill loader must define a `SourceMCP` source constant and an `AddMCPSkill(s Skill)` method.
- MCP-provided skills override project skills of the same name (they have the highest source priority).
- `github.com/fsnotify/fsnotify` is already in the dependency graph from Phase 10; Phase 12 can reuse it for skill hot reload without a new allowlist entry.
- MCP skill integration in Phase 12 is limited to the type system and source priority. Full MCP skill discovery over a live server belongs to Phase 15 or later.

### Phase 11 - Sub-agents and AgentTool (Treat As Complete)

Implemented:

- Sub-agent spawning.
- `AgentTool` for invoking child agents.
- Agent fork and isolation.

Phase 12 implications:

- Sub-agents inherit the parent tool registry, including `SkillTool`.
- When a sub-agent calls `SkillTool`, it reads from the same loaded skill registry (shared via the loader reference).
- Skills invoked by sub-agents should be treated as context for that sub-agent turn, not as parent system-prompt amendments.
- No new wiring is needed in Phase 12 to support sub-agent skill access; it is automatic through registry inheritance.

## Documentation and Log Findings

`docs/PHASE-LOG.md` records the current authoritative phase order as:

- Phase 12: Skills
- Phase 13: Slash commands and config UX
- Phase 14: Tasks

`book/ch12-extensibility.md` describes skills as "the horizontal extensibility axis" and hooks as "the vertical control-plane axis." Skills extend what the model knows and how it behaves. Hooks control when behaviors are allowed to proceed. The book's skills chapter also warns explicitly that skills should have no auto-execute path and should not be confused with hook-like policy enforcement.

## Deep Analysis Of `book/ch12-extensibility.md` (Skills Layer)

The book's chapter 12 is structured around two complementary extension dimensions. Phase 9 covered hooks. Phase 12 covers skills.

### Principles to Preserve

- **Skills are behavioral context, not policy.** A skill tells the model "when reviewing code, apply this checklist" — it does not block or allow any operation.
- **Markdown with YAML frontmatter is the natural format.** It is human-readable, version-controllable, and composable with existing project tooling.
- **Source priority is deterministic and explicit.** Bundled → user → project → MCP. Later sources override earlier ones for the same skill name. Users understand where their skills come from.
- **Two-phase loading.** Frontmatter is loaded at startup (cheap). Body content is loaded on invocation (potentially larger). This pattern mirrors memory's scan/recall split and keeps startup fast.
- **No auto-invocation.** Skills must be explicitly requested via the SkillTool or the model must be instructed to suggest them. They are never injected automatically.
- **Hot reload is safe for skills.** Unlike hooks (which must freeze at startup because they execute processes), skills are read-only prompt text. If a file changes, a fresh load is safe.
- **MCP as a source tier.** MCP servers can expose skills as resources. The type system must account for this source from the beginning even if full MCP skill discovery is deferred.

### Corrections Applied to the Original Phase 12 Sketch

The original Phase 12 sketch in `.codex/go-ollama-plan-AGENTS.md` was correct at the product level but lacked implementation detail for this repo:

- It did not specify how `embed.FS` integrates with the priority chain (bundled skills should always be the lowest priority; user and project skills can shadow them).
- It assumed `github.com/fsnotify/fsnotify` would be a new dependency; it is already present from Phase 10.
- It did not define the exact `SkillTool` result contract for how skill content is injected into conversation history.
- It did not specify that MCP skills enter through `AddMCPSkill` rather than a file path.
- It did not define the `Source` enum with its four values.
- It did not address how skills with parse errors should be handled (skip with warning, not fatal).

## Evaluation of the Original Phase 12 Plan

The original plan is correct in intent. It needs more precision:

- No `Source` enum was defined.
- No spec for the embed.FS layout of bundled skills.
- No contract for what `SkillTool.Call` returns as a `tools.Result`.
- No spec for `/skills list` output format.
- No spec for hot-reload notification channel or callback.
- No test targets specified.
- No boundary stated between Phase 12 and Phase 13 for `/skills` command routing.

This plan addresses all of these gaps.

## Final Phase 12 Scope

In scope:

- `internal/skills` package: `Skill` type, `Source` enum, loader with priority chain, frontmatter scanning, hot-reload watcher, bundled embed.FS skills.
- `internal/tools/skilltool/` package: `SkillTool` implementing `tools.Tool`.
- Three bundled starter skills as embed.FS assets: `code-review`, `debug-session`, `write-tests`.
- `/skills list` and `/skills show <name>` slash commands in `internal/tui/slash.go`.
- `AddMCPSkill` method on the loader for MCP source integration.
- Comprehensive tests for loading, priority, hot reload, and SkillTool invocation.
- Phase log update.

Out of scope:

- Auto-invocation of skills without an explicit tool call or user instruction.
- LLM-based skill auto-selection (similar to memory recall).
- Skills as policy enforcement or permission bypass.
- Full MCP skill discovery over a live server.
- `/skills create` or `/skills delete` commands.
- Skill versioning or conflict resolution beyond source priority.
- Skill marketplace or remote skill registries.
- Team or shared skill management.
- Telemetry or metrics for skill usage rates.
- Skill scheduling or auto-application to specific tool events.

## Target User Experience

### Session Start

When the REPL starts:

1. `skills.NewLoader()` is constructed with references to:
   - bundled embed.FS (always available),
   - user skills dir from `paths.SkillsDir()`,
   - project skills dir from `paths.ProjectSkillsDir(workingDir)`.
2. Loader performs an initial scan of all three directories.
3. Frontmatter is parsed for each `.md` file found.
4. Files with malformed frontmatter are skipped with a DEBUG warning.
5. The watcher is started on user and project skill directories.
6. `SkillTool` is instantiated with a reference to the loader.
7. `SkillTool` is registered in the tool registry alongside `Bash`, `FileRead`, `FileWrite`, and MCP tools.

### Agent Invokes SkillTool

When the model calls `Skill(name: "code-review")`:

1. SkillTool looks up `"code-review"` in the loader.
2. Loader returns the highest-priority version of that skill (MCP > project > user > bundled).
3. SkillTool reads the full Markdown body from the source.
4. SkillTool returns a `tools.Result` with `Display` set to the full skill content, prefixed with a behavioral adoption framing.
5. The agent loop appends the tool result as `llm.RoleTool` in conversation history.
6. The model sees the skill content and adopts the behavior for the session.

### Hot Reload

When a new or changed `.md` file appears in `.nandocodego/skills/`:

1. fsnotify event fires in the watcher goroutine.
2. Loader re-parses the affected file's frontmatter.
3. Updated skill replaces the old entry in the in-memory index.
4. Loader notifies registered callbacks with the skill name and source.
5. If a callback is registered in the TUI, a system transcript item is appended.

### Slash Commands

`/skills list`:

1. Loader returns all frontmatter entries sorted by name.
2. Each entry is shown as: `[name] (source) - description`
3. Source labels: `bundled`, `user`, `project`, `mcp`.

`/skills show <name>`:

1. Loader looks up the skill by name.
2. Full Markdown body is loaded and rendered with the Glamour renderer.
3. Frontmatter fields (name, description, version, author, tags) are shown as a header.

## Architecture

### Package Layout

```text
internal/skills/
  skill.go          — Skill type, Source enum, SkillFile type
  loader.go         — Loader struct, scan, priority chain, AddMCPSkill
  watcher.go        — fsnotify-based hot reload, callbacks
  embed.go          — embed.FS bundled skills and embed loader
  loader_test.go    — priority, scan, MCP override, error skip
  watcher_test.go   — hot reload detection within 1s
  embed_test.go     — bundled skill presence and content

internal/tools/skilltool/
  skilltool.go      — SkillTool implementing tools.Tool
  skilltool_test.go — invocation, unknown skill, permission always-allow

assets/skills/
  code-review.md    — bundled starter skill
  debug-session.md  — bundled starter skill
  write-tests.md    — bundled starter skill
```

### Core Types

```go
// Source indicates where a skill was loaded from. Lower priority sources are
// overridden by higher priority sources of the same name.
type Source int

const (
    SourceBundled Source = iota // embed.FS, lowest priority
    SourceUser                  // ~/.nandocodego/skills/
    SourceProject               // .nandocodego/skills/
    SourceMCP                   // MCP server resource, highest priority
)

func (s Source) String() string

// SkillFile holds parsed frontmatter and the resolved file path or embed path.
type SkillFile struct {
    Name        string
    Description string
    Version     string
    Author      string
    Tags        []string
    Source      Source
    Path        string // absolute filesystem path or empty for embed
    EmbedPath   string // path within embed.FS, empty for filesystem skills
    ModTime     time.Time
}

// Loader holds the in-memory skill index and manages hot reload.
type Loader struct {
    mu       sync.RWMutex
    skills   map[string]SkillFile // keyed by Name, highest-priority entry wins
    userDir  string
    projDir  string
    embedFS  fs.FS
    watcher  *fsnotify.Watcher
    onChange []func(name string, src Source)
}

// NewLoader creates a Loader, scans all sources, and starts the file watcher.
func NewLoader(userDir, projDir string, embedFS fs.FS) (*Loader, error)

// List returns all loaded SkillFiles sorted by Name.
func (l *Loader) List() []SkillFile

// Lookup returns the highest-priority SkillFile for the given name.
func (l *Loader) Lookup(name string) (SkillFile, bool)

// ReadBody returns the full Markdown body (without frontmatter) for a skill.
func (l *Loader) ReadBody(sf SkillFile) (string, error)

// AddMCPSkill adds or replaces a skill from an MCP server source.
// MCP skills always override filesystem skills of the same name.
func (l *Loader) AddMCPSkill(sf SkillFile)

// OnChange registers a callback invoked when a skill is added, updated,
// or removed due to hot reload.
func (l *Loader) OnChange(fn func(name string, src Source))

// Close stops the file watcher.
func (l *Loader) Close() error
```

### SkillTool Type

```go
// SkillTool implements tools.Tool and allows the agent to load and adopt
// a named skill as behavioral context for the current session.
type SkillTool struct {
    loader *skills.Loader
}

// Name returns "Skill".
func (t *SkillTool) Name() string

// Description returns a model-visible description of what the tool does.
func (t *SkillTool) Description() string

// Schema returns the JSON schema for the skill name parameter.
func (t *SkillTool) Schema() json.RawMessage

// UnmarshalInput parses the name field from tool call arguments.
func (t *SkillTool) UnmarshalInput(raw json.RawMessage) (any, error)

// CheckPermissions always returns PermAllow; skill loading is safe and read-only.
func (t *SkillTool) CheckPermissions(ctx tools.Context, input any) tools.PermissionResult

// Call loads the skill body and returns it as a tools.Result for the agent.
func (t *SkillTool) Call(ctx context.Context, tc tools.Context, input any) (tools.Result, error)

// RenderHints returns hints for the TUI transcript renderer.
func (t *SkillTool) RenderHints() tools.RenderHints
```

### Bundled Skill Frontmatter Format

Every bundled skill file uses YAML frontmatter followed by the behavior body:

```markdown
---
name: code-review
description: Apply a structured checklist when reviewing Go code for correctness, style, and safety
version: 1.0.0
author: nandocodego
tags: [review, go, checklist]
---

When reviewing Go code, apply the following checklist...
```

Required frontmatter fields: `name`, `description`.
Optional: `version`, `author`, `tags`.

Files with missing `name` or `description` are skipped with a warning. All other parse errors are also non-fatal skips.

### Source Priority Logic

The loader scans all sources and builds the index by applying priority rules:

1. Load all bundled skills from `embed.FS` with `SourceBundled`.
2. For each user skill file in `~/.nandocodego/skills/`, parse frontmatter. If the name already exists from bundled, replace it with the user version.
3. For each project skill file in `.nandocodego/skills/`, parse frontmatter. Replace any existing entry of the same name (bundled or user) with the project version.
4. MCP skills added via `AddMCPSkill` replace any existing entry of the same name regardless of prior source.

If two files in the same directory tier have the same `name` field (not filename), the file with the lexicographically later filename wins. This is deterministic but should be warned at load time.

### Hot Reload Contract

- The file watcher monitors user and project skill directories only.
- Bundled (embed.FS) skills are never reloaded.
- MCP skills are updated only via `AddMCPSkill`.
- When a `Write`, `Create`, or `Chmod` event fires for a `.md` file:
  - Re-parse frontmatter of the affected file.
  - If valid, update the in-memory index under the write lock.
  - Call all registered `onChange` callbacks.
- When a `Remove` or `Rename` event fires:
  - Remove the skill from the index if it was the only representative.
  - If a lower-priority source had a skill of that name, reinstate it.
  - Call all registered `onChange` callbacks.
- Debounce rapid events: if multiple events fire within 50 ms for the same file, collapse to one reload.
- Hot reload goroutine must not block the Bubble Tea `Update` loop.

## Implementation Plan

### Step 1 - Skill Type and Source Enum

Files:

- `internal/skills/skill.go`

Implement:

- `Source` type with four constants (`SourceBundled`, `SourceUser`, `SourceProject`, `SourceMCP`).
- `Source.String()` for display labels.
- `SkillFile` struct with all frontmatter fields plus `Source`, `Path`, `EmbedPath`, `ModTime`.
- `SkillFile.IsFilesystem() bool` — true when `Path` is non-empty.
- `SkillFile.IsEmbedded() bool` — true when `EmbedPath` is non-empty.

Tests:

- Source string labels match expected values.
- `SkillFile.IsFilesystem()` and `IsEmbedded()` return correct values for each case.

### Step 2 - YAML Frontmatter Parser

Files:

- `internal/skills/skill.go` (extend) or a small `parseFrontmatter` function

Implement:

- `parseFrontmatter(r io.Reader) (SkillFile, string, error)` — parses YAML frontmatter and returns the file metadata and remaining Markdown body.
- Uses `gopkg.in/yaml.v3` (already a direct dependency from Phase 8).
- Reads at most the first 50 lines or 4096 bytes to find the `---` delimiters; the rest of the file is the body.
- Returns an error if the opening `---` is not present within the cap.
- Returns an error if `name` or `description` is empty after parsing.
- Returns an error if YAML parsing itself fails.

Tests:

- Valid frontmatter with all fields.
- Frontmatter with only required fields.
- Missing `name` field returns error.
- Missing `description` field returns error.
- No frontmatter delimiter returns error.
- YAML parse error returns error.
- Body text is returned separately from frontmatter.

### Step 3 - Bundled Skill Embed

Files:

- `internal/skills/embed.go`
- `assets/skills/code-review.md`
- `assets/skills/debug-session.md`
- `assets/skills/write-tests.md`

Implement:

- `//go:embed assets/skills/*.md` directive in `embed.go`.
- `BundledFS` exported `embed.FS` variable.
- `loadBundledSkills(fsys fs.FS) ([]SkillFile, error)` that walks the embed FS, parses each file, and assigns `SourceBundled` and `EmbedPath`.
- Errors in bundled skills are logged at WARN but do not panic; they should not occur in a tested binary.

Bundled skill content requirements:

- `code-review.md`: A review checklist for Go code covering correctness, error handling, test coverage, and style.
- `debug-session.md`: A structured debugging protocol: reproduce, isolate, hypothesize, confirm, fix.
- `write-tests.md`: A test-writing guide emphasizing table-driven tests, subtests, fake dependencies, and no live I/O in unit tests.

Tests:

- All three bundled skills are loadable via `loadBundledSkills(BundledFS)`.
- Each skill has a non-empty name and description.
- Skill names are `code-review`, `debug-session`, `write-tests`.
- Bodies are non-empty.

### Step 4 - Loader: Scan and Priority Chain

Files:

- `internal/skills/loader.go`
- `internal/skills/loader_test.go`

Implement:

- `NewLoader(userDir, projDir string, embedFS fs.FS) (*Loader, error)`.
- Internal `scan()` method that:
  1. Loads bundled skills from `embedFS`.
  2. Walks `userDir` for `*.md` files, parses frontmatter, adds with `SourceUser` priority.
  3. Walks `projDir` for `*.md` files, parses frontmatter, adds with `SourceProject` priority.
  4. Applies priority rules: higher source replaces lower source of the same name.
  5. Logs a warning when two files in the same directory tier share the same `name` field.
- `List() []SkillFile` — returns all entries sorted by Name, thread-safe.
- `Lookup(name string) (SkillFile, bool)` — thread-safe lookup.
- `ReadBody(sf SkillFile) (string, error)` — reads and returns the body:
  - For embed skills: read from the embedded FS and strip frontmatter.
  - For filesystem skills: open the file and strip frontmatter.
- `AddMCPSkill(sf SkillFile)` — adds or replaces a skill at `SourceMCP` priority, under write lock.
- `OnChange(fn func(name string, src Source))` — registers a hot-reload callback.
- `Close() error` — stops the watcher.

Tests:

- Bundled skills load without a filesystem.
- User skill of the same name overrides bundled skill.
- Project skill of the same name overrides user skill.
- MCP skill added via `AddMCPSkill` overrides project skill.
- File with missing `name` field is skipped; other skills still load.
- File with malformed YAML is skipped; other skills still load.
- Non-existent user or project directory is treated as empty (not an error).
- `List()` returns entries sorted by name.
- `Lookup` returns false for unknown name.
- `ReadBody` returns non-empty body for a known skill.

### Step 5 - File Watcher and Hot Reload

Files:

- `internal/skills/watcher.go`
- `internal/skills/watcher_test.go`

Implement:

- `startWatcher(l *Loader) error` — called from `NewLoader` after the initial scan. Starts an `fsnotify.NewWatcher` watching `userDir` and `projDir`.
- Watcher goroutine:
  - Filters to `*.md` files only.
  - On `Write` or `Create`: re-parses the file, updates the index under write lock, calls callbacks.
  - On `Remove` or `Rename`: removes from index (or reinstates lower-priority skill), calls callbacks.
  - On `Chmod`: ignored (file content unchanged).
  - Debounce: track last-event time per file; if a second event fires within 50 ms, reset the timer instead of processing immediately.
- Non-existent directories are silently skipped; no watcher error is fatal.

Tests (use real temporary directories):

- Create a new `.md` file in the watched directory; verify `List()` includes it within 1 second.
- Modify an existing `.md` file; verify `Lookup` returns updated metadata.
- Delete a `.md` file; verify it is no longer returned by `List()`.
- Verify `onChange` callback is called with the correct name and source.
- Stop watcher with `Close()`; no goroutine leak (use `goleak` or a manual check with a short sleep).

### Step 6 - SkillTool Implementation

Files:

- `internal/tools/skilltool/skilltool.go`
- `internal/tools/skilltool/skilltool_test.go`

Implement `SkillTool`:

- `Name() string` returns `"Skill"`.
- `Description() string` returns a model-visible description explaining that calling this tool loads the named skill as behavioral context for the current session.
- `Schema() json.RawMessage` returns a JSON schema with one required string property: `name` (description: "The name of the skill to load").
- `UnmarshalInput` parses `{"name": "..."}`.
- `CheckPermissions` always returns `tools.PermissionResult{Decision: tools.PermAllow}`.
- `Call`:
  1. Looks up the skill by name in the loader.
  2. If not found: returns an error result with a helpful message listing available skills.
  3. If found: loads the full body via `loader.ReadBody`.
  4. Returns a `tools.Result` with `Display` set to the framed skill content.

Framing of skill content in `Call` result:

```
Skill loaded: <name>
Source: <source>

The following behavioral context has been adopted for this session:

<body content>
```

Tests:

- `Call` with a valid skill name returns the skill body in `Display`.
- `Call` with an unknown skill name returns an error result listing known skills.
- `CheckPermissions` always returns `PermAllow` regardless of context.
- Schema is valid JSON.
- Tool name is `"Skill"`.

### Step 7 - Register SkillTool in Builtin Registry

Files:

- `internal/tools/builtin/builtin.go` (extend)
- `internal/skills/embed.go` (no changes needed)
- `internal/cli/repl.go` (extend)

Changes in `internal/cli/repl.go`:

1. Construct `skills.NewLoader(userSkillsDir, projSkillsDir, skills.BundledFS)`.
2. Defer `loader.Close()`.
3. Construct `skilltool.New(loader)`.
4. Register `SkillTool` in the tool registry.
5. Register an `onChange` callback that appends a system transcript item to the TUI if a skill is updated while the session is running.

SkillTool should be registered in the existing `builtin.NewRegistry()` function only if the loader reference can be passed cleanly. If not, a `builtin.NewRegistryWithSkills(loader)` factory is acceptable.

Tests:

- `builtin.NewRegistry()` without skills does not panic.
- `builtin.NewRegistryWithSkills(loader)` includes `"Skill"` in the registry's `All()` output.

### Step 8 - Slash Commands

Files:

- `internal/tui/slash.go` (extend)
- `internal/tui/slash_test.go` (extend)

Implement two new slash commands:

`/skills list`:

- Calls `loader.List()`.
- Formats each skill as: `  <name>  (<source>)  <description>`.
- If no skills are loaded, returns a message saying so.
- Returns a `TranscriptItem` of kind `KindSystem`.

`/skills show <name>`:

- Calls `loader.Lookup(name)`.
- If not found, returns an error transcript item.
- If found, calls `loader.ReadBody` and renders the full Markdown body via the Glamour `MarkdownRenderer`.
- Returns a `TranscriptItem` of kind `KindAssistant` or `KindSystem` with the rendered content.

For both commands, the loader must be accessible from the slash handler. Options:

- Pass the loader to the `HandleSlashCommand` function as an optional argument.
- Or hold the loader as a field on the TUI `Model` and dispatch there.

Prefer holding the loader on the TUI `Model` since that avoids a function signature change that would also require changes to existing slash tests.

Tests:

- `/skills list` with zero skills returns a "no skills loaded" message.
- `/skills list` with loaded skills returns a formatted list including name, source, and description.
- `/skills show code-review` returns the body of the bundled `code-review` skill.
- `/skills show unknown` returns an error message.
- Existing `/help`, `/clear`, `/exit`, `/model` tests remain unaffected.

### Step 9 - Tests, Checks, and Manual Smoke

Required test commands:

```sh
go test ./internal/skills/...
go test ./internal/tools/skilltool/...
go test ./internal/tui/...
go test ./internal/cli/...
go test ./...
go test -race ./internal/skills/...
tools/check-allowed-deps.sh
tools/check-network-policy.sh
```

Manual smoke:

```sh
go run ./cmd/nandocodego --model qwen3 --no-alt-screen
```

Flow:

1. Run `/skills list`. Verify the three bundled skills appear with `(bundled)` label.
2. Ask the agent: "Please review the following Go function" and paste a function. Then ask it to run the `code-review` skill first.
3. Verify the agent calls `Skill(name: "code-review")`, the skill body appears in the transcript tool result, and the agent applies the checklist.
4. Create a new file at `.nandocodego/skills/my-custom.md` with valid frontmatter while the REPL is running.
5. Within 1 second, run `/skills list` again and verify `my-custom` appears with `(project)` label.
6. Run `/skills show my-custom` and verify the body renders.

## Acceptance Criteria

- [ ] `internal/skills` package exists and compiles.
- [ ] Three bundled skills (`code-review`, `debug-session`, `write-tests`) are loadable via `embed.FS` without a filesystem.
- [ ] Project skills (`.nandocodego/skills/`) override user skills of the same `name` field.
- [ ] User skills (`~/.nandocodego/skills/`) override bundled skills of the same `name` field.
- [ ] MCP skills added via `AddMCPSkill` override all filesystem and bundled sources.
- [ ] `/skills list` shows all loaded skills with source label and description.
- [ ] `/skills show <name>` renders the full body of the named skill.
- [ ] `SkillTool` implements `tools.Tool` and is registered in the agent's tool registry.
- [ ] `SkillTool.CheckPermissions` always returns `PermAllow`.
- [ ] `SkillTool.Call` with an unknown name returns a helpful error listing available skills.
- [ ] `SkillTool.Call` with a valid name returns the framed skill body in `tools.Result.Display`.
- [ ] Hot reload picks up a new project skill file within 1 second of file creation.
- [ ] Hot reload picks up a modified project skill file within 1 second of modification.
- [ ] Hot reload fires the `onChange` callback with the correct skill name and source.
- [ ] Skills with missing `name` or `description` in frontmatter are skipped with a warning, not fatal.
- [ ] Skills with malformed YAML frontmatter are skipped with a warning, not fatal.
- [ ] Two files in the same tier sharing the same `name` field produce a warning; the lexicographically later filename wins.
- [ ] Non-existent user or project skill directories are treated as empty (no error).
- [ ] `Close()` stops the watcher without goroutine leaks.
- [ ] Skills cannot execute commands; no auto-execute path exists.
- [ ] Skill body content is not logged at INFO.
- [ ] `go test -race ./internal/skills/...` passes.
- [ ] All existing tests in `./...` continue to pass.
- [ ] `tools/check-allowed-deps.sh` passes (no new allowlist entries needed; fsnotify already present from Phase 10).
- [ ] `tools/check-network-policy.sh` passes.
- [ ] `docs/PHASE-LOG.md` has a Phase 12 entry.

## Todos

### Skill Type and Parsing

- [ ] Create `internal/skills/skill.go` with `Source` type, four constants, and `String()`.
- [ ] Add `SkillFile` struct with `Name`, `Description`, `Version`, `Author`, `Tags`, `Source`, `Path`, `EmbedPath`, `ModTime`.
- [ ] Add `SkillFile.IsFilesystem() bool`.
- [ ] Add `SkillFile.IsEmbedded() bool`.
- [ ] Implement `parseFrontmatter(r io.Reader) (SkillFile, bodyString string, error)` using `gopkg.in/yaml.v3`.
- [ ] Read at most 50 lines or 4096 bytes to detect `---` delimiter.
- [ ] Return error if `name` is empty after parsing.
- [ ] Return error if `description` is empty after parsing.
- [ ] Return error if no opening `---` is found within the cap.
- [ ] Return body text (everything after closing `---`) separately.
- [ ] Write `parseFrontmatter` tests for all valid and error cases.
- [ ] Write `SkillFile.IsFilesystem` and `IsEmbedded` tests.

### Bundled Skills (embed.FS)

- [ ] Create `assets/skills/` directory.
- [ ] Write `assets/skills/code-review.md` with valid frontmatter and a Go code review checklist body.
- [ ] Write `assets/skills/debug-session.md` with valid frontmatter and a structured debugging protocol body.
- [ ] Write `assets/skills/write-tests.md` with valid frontmatter and a Go test writing guide body (emphasizing table-driven tests and no live I/O in unit tests).
- [ ] Create `internal/skills/embed.go` with `//go:embed assets/skills/*.md` and `BundledFS embed.FS`.
- [ ] Implement `loadBundledSkills(fsys fs.FS) ([]SkillFile, error)`.
- [ ] Handle walk errors in `loadBundledSkills` as WARN-level logs; do not return fatal error.
- [ ] Write `embed_test.go` verifying all three bundled skills load with correct names.

### Loader

- [ ] Create `internal/skills/loader.go` with `Loader` struct.
- [ ] Implement `NewLoader(userDir, projDir string, embedFS fs.FS) (*Loader, error)`.
- [ ] Implement internal `scan()` called from `NewLoader`.
- [ ] Apply source priority during `scan()`: embed < user < project.
- [ ] Log a WARN when two files in the same tier share the same `name` field.
- [ ] Implement `List() []SkillFile` with read lock, returns sorted slice.
- [ ] Implement `Lookup(name string) (SkillFile, bool)` with read lock.
- [ ] Implement `ReadBody(sf SkillFile) (string, error)` with filesystem and embed path branching.
- [ ] Implement `AddMCPSkill(sf SkillFile)` with write lock, always replaces same-name entries.
- [ ] Implement `OnChange(fn func(name string, src Source))` callback registration.
- [ ] Implement `Close() error` that stops the watcher.
- [ ] Write loader tests for all priority combinations.
- [ ] Write loader test for skip-on-bad-frontmatter.
- [ ] Write loader test for non-existent directories being treated as empty.

### File Watcher

- [ ] Create `internal/skills/watcher.go`.
- [ ] Implement `startWatcher(l *Loader) error` using `fsnotify.NewWatcher`.
- [ ] Watch `userDir` and `projDir`; skip directories that do not exist.
- [ ] Handle `Write` and `Create` events: re-parse affected `.md` file.
- [ ] Handle `Remove` and `Rename` events: remove skill from index; reinstate lower-priority source if applicable.
- [ ] Ignore `Chmod` events.
- [ ] Implement 50 ms debounce per file path.
- [ ] Invoke all registered `onChange` callbacks after each reload.
- [ ] Debounce goroutine must not hold the loader lock while sleeping.
- [ ] Write watcher tests using real temporary directories.
- [ ] Verify new skill file appears in `List()` within 1 second.
- [ ] Verify modified skill metadata is updated within 1 second.
- [ ] Verify deleted skill is absent from `List()` within 1 second.
- [ ] Verify `onChange` callback is called for each event.
- [ ] Verify `Close()` stops the goroutine without a leak.

### SkillTool

- [ ] Create `internal/tools/skilltool/skilltool.go`.
- [ ] Implement `SkillTool` struct with a `*skills.Loader` field.
- [ ] Implement `Name() string` returning `"Skill"`.
- [ ] Implement `Description() string` with a model-visible description.
- [ ] Implement `Schema() json.RawMessage` with a `name` string property (required).
- [ ] Implement `UnmarshalInput` parsing `{"name": "..."}`.
- [ ] Implement `CheckPermissions` always returning `PermAllow`.
- [ ] Implement `Call`:
  - [ ] Look up skill by name.
  - [ ] If not found, return error result listing all available skill names.
  - [ ] If found, call `loader.ReadBody(sf)`.
  - [ ] Return `tools.Result{Display: framedContent}` with source and name in the framing header.
- [ ] Implement `RenderHints` with a compact display hint.
- [ ] Write `skilltool_test.go` with a fake loader that returns known skills.
- [ ] Test `Call` with valid name returns expected body.
- [ ] Test `Call` with unknown name returns error message listing skills.
- [ ] Test `CheckPermissions` returns `PermAllow` in default and bypass modes.

### Builtin Registry Integration

- [ ] Decide whether `builtin.NewRegistry()` accepts a loader or a new `NewRegistryWithSkills(loader)` is added.
- [ ] Implement the chosen approach.
- [ ] Write a test verifying `"Skill"` appears in the registry's tool list.

### TUI Slash Command Integration

- [ ] Hold `*skills.Loader` as a field on `internal/tui.Model`.
- [ ] Pass loader to `New(...)` in `internal/tui` constructor.
- [ ] Update `HandleSlashCommand` (or the `Model`'s slash dispatch) to handle `/skills list`.
- [ ] Update slash dispatch to handle `/skills show <name>`.
- [ ] Register an `OnChange` callback in the TUI that appends a system transcript item on hot reload.
- [ ] Write tests for `/skills list` with zero skills.
- [ ] Write tests for `/skills list` with multiple skills.
- [ ] Write tests for `/skills show` with valid name.
- [ ] Write tests for `/skills show` with unknown name.

### REPL Wiring

- [ ] In `internal/cli/repl.go`, resolve `userSkillsDir` from `paths.SkillsDir()`.
- [ ] Resolve `projSkillsDir` from `paths.ProjectSkillsDir(workingDir)`.
- [ ] Construct `skills.NewLoader(userSkillsDir, projSkillsDir, skills.BundledFS)`.
- [ ] Defer `loader.Close()` for clean shutdown.
- [ ] Construct `skilltool.New(loader)`.
- [ ] Register SkillTool in the tool registry.
- [ ] Pass loader to the TUI model constructor.

### Phase Log

- [ ] Append Phase 12 entry to `docs/PHASE-LOG.md` with objective, files, dependencies, checks, manual smoke result, design decisions, known constraints, and exit gate status.

## Forbidden

- Skills with an auto-execute path of any kind. Skills are prompt context only.
- LLM-based skill auto-selection or injection without an explicit tool call.
- Skills as permission enforcement or policy bypass.
- Storing skill body content in `bootstrap.State` or `state.App`.
- Logging skill body content at INFO or higher.
- Executing code embedded in skill Markdown bodies.
- Modifying the hooks snapshot system or permission resolver in Phase 12.
- Adding a new network destination for skill discovery.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---:|---|
| Model adopts harmful behavior from a project skill | High | Skills are context, not policy. Hooks and permissions still govern all tool calls regardless of skill content. Document this explicitly. |
| Hot reload introduces a TOCTOU window | Low | Skills are non-executable. Even if a file is modified mid-read, the worst outcome is slightly stale content. No command execution is triggered. |
| embed.FS path changes break the build | Medium | Pin the embed glob to `assets/skills/*.md`; test in CI that all three skills load. |
| Two skills with the same name in the same tier cause confusion | Low | Log a warning; the lexicographically later filename wins. Document the determinism rule. |
| MCP skill overrides a user skill silently | Low | `/skills list` shows the source of every skill. Users can see which source won. |
| SkillTool leaks skill body content into logs | Medium | `tools.Result.Display` is not logged at INFO; review all log call sites after implementation. |
| fsnotify watcher goroutine leaks on REPL exit | Medium | `loader.Close()` is deferred in `runREPL`; write watcher test that verifies no goroutine leak after `Close()`. |
| Skill file with huge body slows the tool call | Low | `ReadBody` reads one file at invocation time; no size cap is needed for Phase 12. Document that skills should be concise (under ~4KB). |

## Phase Log Template

When implementation finishes, append a Phase 12 entry to `docs/PHASE-LOG.md` with:

- objective;
- files created and updated;
- dependencies added and allowlist status;
- tests run and results;
- manual smoke result (three-step exit gate);
- design decisions (source priority, hot reload safety, SkillTool framing);
- known constraints and deferred work;
- exit gate status.

## Implementation Reconciliation (2026-05-08)

This phase plan is now reconciled with the repository implementation and tests.

Implemented in this closure pass:

- Added warning diagnostics for invalid skill files and bundled-skill parse/read issues (`internal/skills/loader.go`, `internal/skills/embed.go`).
- Added deterministic duplicate-name warning within the same source tier; lexicographically later filename remains the winner (`internal/skills/loader.go`).
- Fixed watcher behavior for rename/remove lifecycle so renamed-away skills are removed from the active index and do not remain stale (`internal/skills/watcher.go`).
- Upgraded watcher debounce behavior to reset per-path processing windows (50 ms quiet period) instead of dropping near-coincident events (`internal/skills/watcher.go`).
- Expanded test coverage for:
  - frontmatter parser error modes and helper predicates (`internal/skills/skill_test.go`);
  - loader invalid-file skip, sorted list behavior, missing-dir behavior, duplicate-name resolution, and body reads (`internal/skills/loader_test.go`);
  - watcher modify/delete/rename handling and callback behavior (`internal/skills/watcher_test.go`);
  - `/skills list` and `/skills show` command flow through command registry (`internal/commands/registry_test.go`).

Automated checks executed successfully:

- `env GOCACHE=/private/tmp/go-build-cache go test ./internal/skills/... ./internal/tools/skilltool/... ./internal/commands/... ./internal/tui/...`
- `env GOCACHE=/private/tmp/go-build-cache go test ./...`
- `env GOCACHE=/private/tmp/go-build-cache go test -race ./internal/skills/...`
- `tools/check-allowed-deps.sh`
- `tools/check-network-policy.sh`

Remaining closure item:

- Manual live REPL exit-gate validation against a running Ollama model (skill invocation behavior + hot-reload observation in an interactive session).

## Exit Gate

Phase 12 is complete only when:

- all acceptance criteria above are checked;
- `go test -race ./...` passes;
- `tools/check-allowed-deps.sh` and `tools/check-network-policy.sh` pass;
- three bundled skills are loadable from `embed.FS` in a test without a filesystem;
- hot reload picks up a new project skill within 1 second in the watcher test;
- manual smoke confirms SkillTool invocation produces the correct behavior framing;
- the phase log records the implementation and any deviations from this plan.
