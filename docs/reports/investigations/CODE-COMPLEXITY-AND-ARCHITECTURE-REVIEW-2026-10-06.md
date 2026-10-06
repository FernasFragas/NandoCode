# Code Complexity And Architecture Review - 2026-10-06

Status: Open. Findings are tracked in `docs/roadmap/BACKLOG.md` §5 ("Code review 2026-10-06"); nothing in this report has been fixed yet.

## Scope And Method

Review of all `internal/` packages and `cmd/`, using two lenses: clean code (simplify without changing behavior, Chesterton's Fence, follow project conventions) and TDD (no refactor without a characterization test; every suspected bug gets a failing reproduction test first). Findings are weighted by ADR-002: the browser/server path and the agent core matter most; the TUI is in maintenance mode, so TUI refactors are recommended only when they reduce bug risk or free logic the browser needs.

How it was done:

- Shared metric baseline: `golangci-lint` with gocyclo>15, gocognit>20, funlen, nestif>=5, dupl, non-test files. 127 findings: gocognit 89, nestif 22, gocyclo 11, funlen 3, dupl 2.
- Three parallel review agents, split as core runtime, context/retrieval/eval, and user surfaces. Each ran `go test -cover`, per-function coverage, `go test -race`, and `go vet`, and read every hotspot in full. Some bugs were confirmed with throwaway probe tests in a scratch copy; no repository file was changed by the review.
- The planned fourth agent (overall architecture) stopped on an API rate limit. The architecture section below was done directly: dependency graph from `go list`, composition roots, and cross-checks of the agents' system-level findings.
- The P0 claims marked *(spot-checked)* were re-read in source while writing this report.

Results: all tests pass; `-race` and `go vet` are clean on every package checked.

## Headline

The code is test-green, but the **primary surface (browser/server) is the least mature path**. It has confirmed correctness and security bugs, sits at 55% coverage, and duplicates logic that has already drifted from the TUI version.

The agent core works, but `(*Agent).run` and the tool-execution path are hard to change safely. The event-stream contract that every surface depends on is not enforced by tests.

Most of the complexity budget is in `internal/tui/app.go` (`handleKeyMsg` cognitive complexity 189). Under ADR-002 that should mostly be left alone, apart from extracting the logic the browser needs.

## P0 Findings

| # | Finding | Evidence | Reproduction test to write first |
| --- | --- | --- | --- |
| P0-1 | **Browser forgets the conversation every turn** *(spot-checked)* | `server/session.go` `StartRun` passes `s.conversation` as history. On `agent.Terminal` the session sets `s.conversation = e.Conversation`, which holds only the messages added during that run (`agent/agent.go` `addedConversation`), and the user prompt (`session.go:391`) is never stored. The TUI appends instead. | Two `StartRun` calls with a capturing fake runner. The second `Input.Messages` must be `[Q1, A1, Q2]`; today it is `[A1, Q2]`. |
| P0-2 | **XSS in browser markdown** *(spot-checked; agent confirmed in node)* | `index.html` `escapeHtml` does not escape quotes. The link regex inserts the raw URL into `href="…"`, so `javascript:` links and attribute injection work. Output goes to `innerHTML`; CSP has `script-src 'unsafe-inline'` (`server.go:333`). Model output steered by repo files or web content could run script with the session token. | JS test (DOM-free markdown module): no `on*=` attributes, only http(s)/mailto hrefs. Go test: the CSP has no `'unsafe-inline'` once the page is split. |
| P0-3 | **One model runtime shared by all server sessions** | Every message POST calls `Switch` on the single `llm.RuntimeClient` (`server/handler.go:106-126`), so one session's model choice reroutes the others. Embeddings follow it as well (P1-5). | Two sessions on different models; each run uses its own model. |
| P0-4 | **Directory evidence follows symlinks outside the workspace** (agent confirmed with probe) | `contextpack/current_turn.go` `selectDirectoryEvidence` walks, then `os.ReadFile`s each file with no per-file `tools.ResolvePath`; `mentions.expandDirectory` does check. Any non-listing `@dir/` mention on the browser path can pack an outside file's contents. | `TestPackCurrentTurnPromptDirectoryMentionDoesNotFollowSymlinkOutsideRoots`. |
| P0-5 | **Codebase questions lose their tools** (known; root cause analysed) | `retrievalroute`: one substring keyword list decides both "embed?" and "tools allowed?"; `ToolModeNone` is reachable only when semantic is on; 12 order-dependent early returns. False negatives ("explain the agent event loop", "where is the endpoint…") and false positives ("capital" ⊃ "api", "author" ⊃ "auth"). | 11-row `TestDecideToolModeAndAction` table, plus the invariant that endpoint, server, session, model, route, verify and code identifiers never give `ToolModeNone`. |
| P0-6 | **Bash read-only bypasses** (auto-allowed in default and plan modes) | `tools/bash/classify.go` `isReadOnlyCommand` mostly ignores arguments. These all count as read-only: `find -delete`, `find -exec rm`, `env rm -rf`, `sort -o`, `git branch -D`, `git remote add`, `go env -w`. | New RED rows in `TestBashPermissionMatrix`. Classify the inner command of wrapper commands (`env`, `xargs`, `nice`, `timeout`). |
| P0/P1-7 | **Permission escalation paths** *(partly spot-checked)* | (a) The `agenttool` schema exposes `permission_mode` to the model; a sub-agent can run looser than its parent and drops the parent's rules and hooks. (b) A project `.nandocodego/config.toml` can set `trusted = true` on an MCP server (`mcp/config.go:192`), and its stdio command then runs when the repo is opened. (c) In permission rules, `*` does not match `/`, so `Bash(cat *)` deny rules miss paths; the `**` matcher has 0% coverage. | (a) Parent in plan mode, sub-agent requests bypass, Bash write: expect deny or prompt. (b) Project config `trusted = true` yields `Trusted=false` plus a warning. (c) Decide the glob semantics, then add table rows. |

## P1 Findings (Selected)

**Agent core**

- **P1-1. `(*Agent).run`** (`agent.go:89`, gocognit 90) does six jobs: history prep, packing, the length/compaction state machine, proactive compaction, completion, and stop hooks.
  - Hard to read: 9 hand-built `Terminal` events, `turn--; continue` used as a hidden retry 4 times, and `executeOneTurn` takes 21 positional parameters.
  - Refactor: a `runState` struct, a `loopAction` enum (`nextTurn` / `retrySameTurn` / `stop`), and a `turnRequest` struct. About 350 lines, so do it in 3 PRs.
  - Characterization tests first for the branches with no coverage: `HistoryPolicyLatestOnly` (used by the server), mid-run Chat error, proactive compaction, stop hook.
- **P1-2. Event-stream contract.**
  - Denied tool calls emit a result with no start event, so the browser drops them (`agent.go:536`, `index.html` `finishToolCard`).
  - Tool results return out of call order (errors, then denials, then executed). Ollama has no tool-call IDs, so order is the only link.
  - The `Terminal` event can be lost on cancel, because `sendEvent` uses a random `select`.
  - `ToolUseProgress` is never emitted: the live executor passes a nil progress channel, and the forwarding code exists only in a dead serial path (`agent/tools.go`, 0% coverage, no callers).
  - `TestAgentRunProgressEvents` cannot fail.
  - Fix with a shared `assertEventInvariants` test helper: every result has an earlier start, exactly one `Terminal`, and tool messages in call order.
- **P1-3. Watchdog timeout leaks the upstream request.** `llm.WatchStream` cancels only its own ctx, while the Ollama request uses the run ctx (`agent/stream.go:73`), and `ollama.go:110` has an unguarded blocking send.

**Server and browser**

- **P1-4. Server event log.**
  - `Emit` assigns IDs, appends to the ring and fans out in three separate critical sections.
  - Events emitted between replay and subscribe are lost.
  - Slow subscribers silently drop events.
  - An unknown or evicted `Last-Event-ID` returns nothing instead of a gap event.
  - The browser never sends `Last-Event-ID`, so a reconnect duplicates the transcript.
  - Fix: an `eventLog` type with an atomic `SubscribeFrom(lastID)`, a `replay_gap` event, and typed event DTOs with JSON tags. Phase 25 builds on this.
- **P1-5. Embeddings follow the active model to Ollama Cloud.** `semantic.LLMEmbedder` wraps the `RuntimeClient` (`server.go:229`, `repl.go:526`, `cli/index.go:145`) *(spot-checked)*. This contradicts the local-embedding rule. Fix: bind the embedder to the local client.
- **P1-6. "Always Allow" in the browser works only once.** The session adds a `*(*)` rule that `permissions` never matches (`session.go:479` vs `permissions/match.go:114`).
- **P1-7. Other browser session bugs.**
  - "New Session" never deletes the old session, so the 10-session limit is hit within the idle window.
  - The permission modal sticks after the 30 s broker timeout.
  - The server ignores `config.Load` errors and maps only 13 of 26 config fields.
  - The server never closes the MCP manager.
- **P1-8. `semantic.LocalService.Status` reads all records and vectors on every browser prompt.** The 250 ms timeout is ineffective because the store ignores ctx.
- **P1-9. `semantic.Refresh` after an embedding-model change** relabels old vectors with the new model instead of re-embedding them.

**Other**

- **P1-10. Supervisor and sub-agents.**
  - `tasks.Supervisor.Start` rollback leaves a phantom pending task and leaks its cancel.
  - The two sub-agent runners write different JSONL schemas.
  - Coordinator workers get every tool call denied (Bubble mode with no prompt).
- **P1-11. memory has 40% coverage.** Recall, prompt-section, store and scan are at 0% even though they run on every browser prompt. A memory test also wrote into the real `~/.local/share/nandocodego` (31 leftover directories on the review machine). **Fixed 2026-10-06:** `TestRunnerDoesNotWaitForExtractionAfterTerminal` now sets `NANDOCODEGO_DATA_HOME` to a temp dir and asserts the memory dir lands there (RED before the fix, GREEN after); leftover directories removed.
- **P1-12. Smaller confirmed bugs.**
  - A trailing `?` in `@main.go?` fails the whole prompt.
  - A skill body line over 4 KB rejects the skill.
  - `deterministicExcerpt` can emit invalid UTF-8.
  - `grep` stops silently at lines over 64 KB and searches nested `node_modules`.
  - MCP `validateDialHost` lets `0.0.0.0` through; webfetch only checks literal IPs, so `localhost` names and redirects get past its SSRF guard.

## Architecture

### Dependency Graph (from `go list`)

No cycles: Go forbids them. Shape:

- **Foundations** (fan-out 0): `llm` (fan-in 23), `tools` (26), `types`, `paths`, `credentials`, `retrievalroute`, `skills`, `tools/dirwalk`.
- **Core:** `agent` (fan-in 13, depends only on llm, tools, permissions, bootstrap, paths, ids). `permissions` (fan-in 13).
- **Composition roots:** `cli` (fan-out **35**) and `server` (fan-out **27**). `tui` (17) and `eval` (13) are the other surface hubs.

Layering exceptions (now documented in `ARCHITECTURE.md`):

- `analysis` → `tui/fileindex`: a core package imports a surface subpackage. Move `fileindex` to a neutral package (e.g. `internal/fileindex`).
- `hooks` → `mcp`, only to reuse `ValidateHTTPDestination` and `NewSafeHTTPClient`. Extract a neutral `internal/netguard` and use it from hooks, mcp and webfetch. That also fixes the weak webfetch SSRF guard and the `0.0.0.0` gap in one place.
- `state` embeds `agent.TerminalReason` and `agent.Usage`; `config` embeds `semantic.Config`. Both are tolerable.

### Composition Roots And Duplication

This is the main architectural problem for the browser-first roadmap.

| Concern | Copies | Drift |
| --- | --- | --- |
| Runtime assembly | `cli/repl.go` `runREPL`, `cli/print.go` `runPrint`, `server.New`, `eval` `buildLiveClient` | REPL maps 26 config fields, print 24, server 13, eval 9. Server skips the skill, task and SelfInfo tools, the observability run wrapper, compaction config and MCP close. The sendmessage resume closure (~65 lines) is copied verbatim between REPL and server. |
| Turn preparation (pack → route → semantic → `agent.Input`) | TUI `submitPrompt` (gocognit 74), server `runAgent`, print `buildPrintInput` | Route-config helpers are byte-identical copies. `ForceDeep`, checkpoint resume, dream, cancel and compact channels exist only in the TUI. Eval bypasses the pipeline entirely, so evals cannot catch routing regressions. |
| Model switching | `commands.handleModel`, TUI, server (×2) | Only the TUI refreshes output budget, `num_ctx` and result limits after a switch. |
| Permission broker | `tui/permission.go`, `server/permission.go` | Near-identical. |
| Prompt intent / mention parsing | 5 intent classifiers, 4 `@`-mention tokenizers (`contextpack.parseMentionRefs` is a byte copy of `mentions.mentionTokens`) | Each `@dir/` is walked 3 times with 3 different exclusion rules. |

Logic the browser needs that lives only in `internal/tui` or `internal/commands`:

| Capability | Where today | Proposed home | Browser endpoint |
| --- | --- | --- | --- |
| Turn preparation | `tui/app.go:1501-1787` | `internal/turnprep` | messages; also fixes P0-1 |
| Model activation and limits | `tui/app.go:256-325`, `:1472-1499` | `modelruntime.Activate` + `state.App.ApplyActivation` | model picker, cloud key (B2) |
| Run control (cancel, abort, compact channel; copied 3× in the TUI) | `tui/app.go:1735, 2591, 2638` | `runctl` | stop/cancel (B2) |
| `/compact`, `/clear` | `tui/app.go:995-1023`, `:1099-1111` | `agent.CompactHistory`, `state.App.ResetConversation` | B2 command endpoints |
| End-of-run history, usage, checkpoint | `tui/app.go:1361-1386` | `ApplyTerminal` | fixes P0-1 |
| Prompt queue, `/btw` | `tui/app.go` Update | order-preserving `runqueue` (also fixes a TUI queued-prompt ordering bug) | queue panel (B3) |
| Semantic toggles, index ops with progress | `tui/app.go:2717-2907`, `cli/index.go` | `state.App.Semantic`, `semantic` ops with an event sink | B2 index endpoints |
| Always-allow rule | `tui/app.go:1823-1838` | `permissions.SessionAllowRule` | fixes P1-6 |

`internal/commands` handlers mix logic with text formatting, and 7 are stubs whose real behavior lives in the TUI. Since ADR-002 forbids a generic slash-command bridge, the path is:

1. A mechanical file split. Over 500 lines, so it gets its own pure-move commit.
2. Typed query functions such as `ListModels`, `ListMemory`, `PermissionsView`, `UsageSummary` and `TraceLast`. The TUI formats their results as text; server endpoints JSON-encode them.

### Event Spine

The design is right: one typed `agent.Event` stream, consumed by every surface. But the contract is implicit:

- Event invariants are broken (P1-2).
- The server converts events into about 25 hand-built `map[string]any` payloads, and `agent.Usage` has no JSON tags, so the browser reads usage fields under three casings.
- Per-session and process-wide state are mixed: the shared `RuntimeClient` (P0-3), process-wide `/cost` and current run trace, and the observability run wrapper, which is wired only into the REPL.

Make the contract explicit with:

- Exported invariants tested once in `agent`.
- Typed SSE DTOs with golden JSON per event type.
- Per-session runtime state in the server.

### Boundaries

Holding:

- `permissions.Resolve` is the single decision point.
- bash parsing fails closed on any redirect or command substitution.
- Credentials are requested before cloud context is sent.
- Server auth is always on for `/v1/*`.
- `NewRequestGuard` enforces loopback Host, same Origin and JSON bodies.
- Project hooks do not execute.

Not holding: P0-2, P0-4, P0-6, P0/P1-7, P1-5, and the SSRF gaps in P1-12.

### Test Architecture

Strengths to keep:

- The fake LLM client.
- Table-driven permission and bash matrices.
- The `Partition` property test.
- The deterministic eval framework.
- Integration tests gated behind a build tag and an env var.
- Fast packages (1–2 s each).

Gaps, in browser-first priority:

1. **No multi-turn server session test.** That gap is how P0-1 shipped.
2. **Untested server code.** No tests go through `routes()`. `server.New`, `sessionRoutes`, `handleDeleteSession`, `handleResolvePermission`, `runREPL` and `runPrint` are all at 0%; the `testServer` helper bypasses `New`.
3. **No JS tests.** Keep the markdown, SSE-parser and tree modules DOM-free so `node --test` can run them without a build step. Whether Node may run in CI is an open decision.
4. **Eval skips the real prompt pipeline** (contextpack, retrievalroute, memory).
5. **Tests that cannot fail:** `TestAgentRunProgressEvents`; `TestSupervisorStop` never calls `Stop`.
6. **Sleep-based synchronization:** about 30 `time.Sleep` calls across server, tui, tasks, skills and observability tests, plus wall-clock asserts (`< 50ms`, `>= 140ms`). Replace them with the existing `waitForStatus` / `blockingRunner` style helpers.

Low coverage:

| package | coverage |
| --- | --- |
| memory | 40% |
| cli | 44% |
| tasktool | 49% |
| modelruntime | 50% |
| mcp | 54% |
| dirwalk | 54% (git path 12.9%) |
| server | 55% |
| observability | 62% |

## Complexity Hotspots: Recommendation Per Area

| Area | Verdict |
| --- | --- |
| `agent.run` (90), `executeToolCallsConcurrent` (28) | Refactor (P1-1, P1-2) behind characterization and invariant tests. |
| `server` `handleGetTree` (45) | Replace the walk with `tools.ResolvePath` + `dirwalk.Walk` (roadmap B1). |
| `server` `buildCoordinatorSessionRunner` (43) | Shrinks once the resume closure is shared. |
| `server` `sessionRoutes` | Move to Go 1.22 pattern routing, after a route-table test. |
| `contextpack.buildEvidenceParts` (65), `selectDirectoryEvidence` (31) | Fix P0-4 first, then extract an `evidenceBudget.fit` helper; later consume `mentions`' walk instead of re-walking. |
| `semantic.Refresh` (44), `Retrieve` (37), `ScanWorkspace` (100) | Fix P1-9 and the dupl finding. `ScanWorkspace` is well pinned by tests; refactor only when touched. |
| `retrievalroute.Decide` | Split tool-mode from retrieval action; use word tokens; write it as a pipeline of named predicates (P0-5). |
| `tools/dirwalk` (43/30/23) | One shared `admit()` policy; consistent cap handling; honor ctx. |
| `tools/grep.call` (43) | Use `fs.SkipAll`, depth-aware excludes, ctx, and a larger scanner buffer. |
| `webfetch.stripHTML` (53) | Small cleanups; move the guard to the shared netguard. |
| `commands` `handlePrompt` / `handleTrace` / `handleMemory` (50/46/45) | Split logic from formatting (see above). |
| `tui` `handleKeyMsg` (189), `Update` (71), `handleAgentEvent` (76), vim (32) | **Deferred (maintenance mode).** Only pull slash dispatch out of `handleKeyMsg` as a side effect of the extractions. |
| `config.markSourcesFromKoanf` (63), `state.OnChange` (38), `eval.validateFixture` (71) | **Metric inflated**: flat field lists. Leave as they are. |

## Complex But Justified (Do Not Simplify)

- `sendEventForce` vs `sendEvent`: needed so the `Terminal` event is delivered on abort. Use the force variant for every `Terminal`.
- "Retries don't consume MaxTurns": pinned by tests. Change the mechanism, not the behavior.
- bash fail-closed parsing.
- `fileread`'s full scan: needed for TotalLines and invalid-UTF-8 detection.
- The `Partition` greedy algorithm.
- `LocalStore.Replace` rename dance: crash safety.
- `readFilesParallelBounded` cancellation back-fill: pinned by tests. Do not swap in errgroup, which would also hit the dependency allowlist.
- `ScanWorkspace` index reordering: needed for determinism.
- Vim state machine.
- `RecentIDs` dedupe.
- Token in the URL fragment: fragments never reach the server.
- SSE `WriteTimeout = 0`.
- Tree depth clamp and entry cap.
- The 4-way semantic-outcome switch.
- `ToolModeNone` itself: change how it is decided, but keep it.

## Recommended Order

Each step is TDD: RED reproduction test, then the minimal fix, then the refactor. Refactors land in separate commits from fixes.

1. **Security and data-loss fixes (fold into roadmap B1):**
   - P0-1: multi-turn server test, then append history.
   - P0-2: escape quotes, allow only http(s)/mailto hrefs.
   - P0-4: per-file `ResolvePath`.
   - P0-6: argument-aware bash classification.
   - P0/P1-7: clamp the sub-agent permission mode; ignore project `trusted`.
   - P1-5: local-only embedder.
   - P1-6: always-allow rule.
2. **P0-5 routing table test and tool-mode split** (already a BACKLOG P0).
3. **Agent event invariants (P1-2) and per-session runtime (P0-3).** Every browser feature depends on them.
4. **Extract surface-neutral packages before B2 endpoints:**
   - `bootstrap.ApplyConfig` (table test that every config field lands).
   - `turnprep`.
   - `modelruntime.Activate`.
   - `runctl`.
   - `ApplyTerminal`.

   B2 endpoints (cancel, cloud key, clear, compact, index, cost) then become thin.
5. **Server event log (P1-4)** as the base for Phase 25 durability. Then the page split (BF-1) and dropping `'unsafe-inline'`.
6. **`agent.run` refactor (P1-1)**, after the step 3 tests exist.
7. **Test hygiene throughout:**
   - memory test isolation.
   - Replace sleeps.
   - Make the two dead tests real.
   - Route-table test.
   - Eval fixture through the real prompt pipeline.

## Docs Updated From This Review

- `ARCHITECTURE.md`: surfaces, layering exceptions, composition roots, server security, known gaps.
- `EMBEDDING-AND-MODEL-ROUTING.md`: 12 drift items; known gaps.
- `APPLICATION-ARCHITECTURE-FLOWCHART.md`: server guards and auth, shared runtime, known-gap note.
- Roadmap docs: the Origin/Host guard already exists (`NewRequestGuard`), so B4, UI-8 and Phase 18 now ask for tests and a stricter CSP rather than a new check.
