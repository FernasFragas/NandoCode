# Backlog

Last reviewed: 2026-10-20 (after the browser revamp batch B1, B1.5, B2, BV-1 to BV-8b; see [ADR-002](../adr/ADR-002-BROWSER-UI-PRIMARY-SURFACE.md)).

This is the single list of work that is **not** on the committed v0.1 path.
The committed path (what is being built next, in order) lives in
[NEXT-PHASES-IMPLEMENTATION-PLAN.md](NEXT-PHASES-IMPLEMENTATION-PLAN.md).

How to use this file:

- An item here is an idea or a known gap, not a commitment.
- To promote an item, add it to the roadmap with an owning plan, then remove it here or mark it `Promoted`.
- When something ships, record it in [PHASE-LOG.md](../phases/PHASE-LOG.md) and delete it here.
- Priority: **P0** = should block v0.1 release, **P1** = strong v0.1 or immediately-after candidate, **P2** = post-launch, **P3** = moonshot.

B1, B1.5, B2 and BV-1 to BV-8b are implemented in the working tree (2026-10-20, pending commit); validation is recorded in [PHASE-LOG.md](../phases/PHASE-LOG.md) and the shipped items were removed from this file.

## 1. Open Bugs

None open. The two bugs from the 2026-06-07 E2E run (cloud-model switch, malformed config in `--print`) were fixed in B1 (2026-10-20); see [PHASE-LOG.md](../phases/PHASE-LOG.md). Add a row (bug, severity, status, next step) when a new one is found.

Blocked E2E scenarios B007/B008 (cloud credential gate observability) are in [reports/blocks/](../reports/blocks/). B2-K2 now provides the credential endpoint they were waiting for; re-run them after the commit.

## 2. Decisions Needed

None open as of 2026-10-06. Add a row here (decision, why it matters, owner) when one comes up.

Resolved 2026-10-06: **launch positioning** "the local agentic engineer" with Proof Mode (roadmap B5) as the differentiator. Also resolved 2026-10-06 (recorded in [ADR-002](../adr/ADR-002-BROWSER-UI-PRIMARY-SURFACE.md) "Follow-Up Decisions"): **TUI entry point** `nandocodego tui`; **default model** chosen by the user in the browser first run (Phase 17), `qwen3.6:35b` stays the config default; **browser JS tests** via `node --test` in CI, no npm; **permission-rule `*`** matches `/`. **Malformed config** - `--print` fails fast (bug above, B1). **Private vulnerability reporting** - `SECURITY.md` now points to GitHub private vulnerability reporting; the maintainer must enable it in the repository settings (Security → Private vulnerability reporting) before public release.

Resolved 2026-10-05 ([ADR-002](../adr/ADR-002-BROWSER-UI-PRIMARY-SURFACE.md)): **Browser UI at launch** - the browser is the primary surface, the TUI stays in maintenance mode, plain `nandocodego` opens the browser, v0.1 is localhost only, and the frontend stays plain JS split into embedded files.

## 3. Product Features (Ideas)

Source: [PRODUCT-BRAINSTORM-DISRUPTIVE-LAUNCH.md](../product/PRODUCT-BRAINSTORM-DISRUPTIVE-LAUNCH.md), which has the scoring rationale. Scheduled on 2026-10-06: Proof Mode (roadmap B5), Local Trust Center and Agent Mission Control (B3 Trust and Activity panels), Prompt Inspector (B3), and the "ask the codebase" demo (Phase 18 docs); 60-second install is Phase 17. **Everything still listed below is post-v0.1.** Remote/Bridge Mode was removed from v0.1 by ADR-002 (localhost only; see §6).

| Idea | Priority | Builds on |
| --- | --- | --- |
| **Verification Ledger:** per-run evidence of commands, tests, files, model, permissions, open risks | P1 | Analysis ledger, observability |
| **Agentic Code Review Board:** role agents (security, perf, tests, docs) review a change and produce one prioritized review | P1 | Coordinator mode |
| **Issue-to-PR workflow:** branch, plan, edit, test, summarize, PR body | P1 | Needs strong safeguards |
| **Repo Understanding Map:** browsable structure, hotspots, dependencies, stale docs from the semantic index | P1 | Phase 28 index |
| **Skills gallery:** curated local skills (Go maintainer, security review, release notes, triage) | P1 | Skills |
| **One-Engineer Startup Mode:** idea → roadmap → scaffold → tasks → launch checklist | P1 | Multiple |
| **Offline builder kit:** bundled model guidance, sample tasks, offline docs | P1/P2 | Phase 17 |
| **Local Eval Arena:** compare local models on your own repo tasks | P2 | Eval framework (`nandocodego eval`) |
| **Agentic education mode:** agent explains reasoning and trade-offs as it works | P2 | Prompting |
| **Public-good maintainer mode:** OSS issue triage, stale docs, release notes | P2 | Multiple |
| **IDE bridges** (VS Code, Zed, Neovim) to the same session | P2 | Phase 25 |
| **Team memory vault:** encrypted, opt-in, bring-your-own-storage sync | P2 | Memory |
| **Agent marketplace with signed manifests** | P2/P3 | High supply-chain risk |
| **Voice-to-agent** | P3 | None |

## 4. UX Follow-Ups

### Terminal UI

The TUI is in maintenance mode (ADR-002): bug and regression fixes only. The deferred Phase 22 features are now in §6 Parked.

### Browser UI

Promoted to the committed roadmap on 2026-10-05 (steps B2, B3, B4 in [NEXT-PHASES](NEXT-PHASES-IMPLEMENTATION-PLAN.md); detail in [WEB-UI-UX-PRODUCT-PLAN.md](../plans/WEB-UI-UX-PRODUCT-PLAN.md)). Still not committed:

- Mobile-first responsive layout (post-launch).
- Theme engine beyond a basic light/dark toggle (post-launch).
- Editing MCP servers from the browser (post-launch; v0.1 shows them read-only in the B3 Trust panel).
- Visual revamp promoted 2026-10-18 as roadmap step BV ([plan](../plans/BROWSER-VISUAL-REVAMP-PLAN.md)). Left out of BV:
  - User "Add task": the user appends a todo that the agent sees on its next `TodoRead` (P2; BV-3 shows the agent's todos read-only).
  - "Add agent": named worker presets (prompt, tools, model) shown as an agent-type registry, as in the mockup (P2).
  - Worker ↔ skill link ("Skill: retrieval" on agent nodes) (P3).
  - Light theme from the BV tokens (post-launch; folds into the theme item above).

Scheduled 2026-10-06: model pull progress in the browser is part of the Phase 17 first run.

## 5. Engineering And Tech Debt

### Code review 2026-10-06

From [CODE-COMPLEXITY-AND-ARCHITECTURE-REVIEW-2026-10-06.md](../reports/investigations/CODE-COMPLEXITY-AND-ARCHITECTURE-REVIEW-2026-10-06.md); IDs refer to that report. Fix test-first (RED reproduction, then fix). On 2026-10-06 the P0 items moved to roadmap step B1, the extraction work and agent event invariants to B1.5, and the server event-log fix to Phase 25 slice 0 (see `NEXT-PHASES-IMPLEMENTATION-PLAN.md`). What remains here is not yet scheduled. Everything else from the review was scheduled on 2026-10-06 (B1, B1.5, C1, B2, B3, Phase 25 slice 0, CL/PA gate, Phase 18).

| Item | Priority | Notes |
| --- | --- | --- |
| Fix-when-touched cleanups: `dirwalk` walkers share one admit policy and honor ctx; `grep` uses `fs.SkipAll` and honors ctx; `webfetch.stripHTML` simplification; `permissions.applyMode` unreachable deny branches; `selfinfo` nondeterministic output order; `hooks.Runner` named wrappers; duplicated semantic/route profile defaults; memory's duplicated filename sanitizer | P2/P3 | Fix the listed issue when you are already changing that file; write the test first. Details in the review report |

### Browser revamp follow-ups (2026-10-20)

Open items from the B1, B1.5, B2 and BV packets (details in [PHASE-LOG.md](../phases/PHASE-LOG.md)). Fix test-first.

| Item | Priority | Notes |
| --- | --- | --- |
| `internal/cli` `TestDoctorFailsWhenSecurityBaselineMissing` touches the real home | P1 | Creates and removes a transient entry under `~/.local/{state,share}/nandocodego`. Give it `NANDOCODEGO_DATA_HOME`/`STATE_HOME` temp dirs (AGENTS test rule). Also check `~/.local/state/nandocodego/analysis/latest-checkpoint.json` for a stray test checkpoint |
| Chrome visual approval of the gallery and mockup parity | P1 | Blocks the BV final gate (VZ). Record `baselines/gallery.png`, run `make visual-check`, go through the per-slice checks in PHASE-LOG |
| Re-run E2E blocks B007/B008 on the credential endpoint | P1 | Needs B2-K2 committed |
| Rule matching is case-sensitive | P2 | On case-insensitive macOS `SECRETS/k.txt` is not matched by a `secrets/*` rule, in the browser save path and the agent loop alike |
| Bash-written files are not detected by the save conflict check | P2 | Only `FileWrite`/`FileEdit` calls and path locks produce the 423; a Bash write can be overwritten by a browser save with a stale-looking hash only caught by `base_sha256` |
| Browser saves only existing UTF-8 files | P2 | No new-file creation; editor capped at 256 KB; CR files read-only; no highlight overlay while editing |
| Browser save permission is separate from the agent loop | P2 | Plan mode blocks even with an allow rule; a hook allow reports `decision_source: "rule"` (K2 enum has no hook value) |
| Permission rules cannot be loaded from a config file | P2 | Only session rules and hooks exist, so a live deny-rule 403 is covered by in-process tests only |
| Cost view is last run only | P2 | Needs a per-session accumulator (the Meter is server-level) |
| `workers` field serialises as `null` instead of `[]` | P3 | The UI tolerates it; fix in `SessionView` |
| Files API limits | P3 | Small language map; 500-entry directory cap with no truncation flag; no read deny-rule check beyond `tools.ResolvePath` |
| `files_touched` ignores Bash edits; message summary capped at 200 runes | P3 | V8a |
| Coordinator workers inherit the parent's mode but not its rules and hooks | P2 | Pass `agent.ParentPolicy` into a policy-aware worker RunFunc; the speculative executor also builds requests without rules or hooks |
| Bash read-only classification is a static allowlist | P2 | `git log` with config-defined external tools, `file -C`, and similar remain |
| TUI Always Allow rule still unescaped | P2 | `internal/tui/app.go` should call `permissions.SessionAllowRule`; Always Allow applies from the next run only |
| `internal/hooks/snapshot.go` still imports `mcp` for URL validation | P3 | Switch to `netguard.ValidateURL`; MCP/hooks deliberately allow loopback |
| Model picker should mark stale `:cloud` tags unavailable | P2 | `/v1/models` still lists them |
| Credential resolver is process-wide | P3 | A key submitted in one browser session unlocks cloud for all sessions (localhost, single user) |
| `time.Sleep` synchronization in untouched tests | P3 | `internal/tasks` (`agent_mailbox_test.go`, `dream_test.go`, `replay_test.go`), `internal/server` (`session_test.go`, `permission_test.go`); replace when touched |
| Blocking sends in the Ollama stream error path and `llm/watchdog.go` | P2 | Add `select` with `ctx.Done()` (review P1-3 second half) |
| Open B1.5 leftovers | P2 | Local-search fallback for uncertain code questions and visible evidence state; grounding on `/btw`, re-run and sub-agents; PreCompact/PostCompact in mid-run compaction; TUI `Activate` adoption |
| Server `TodoList` is in memory | P3 | Lost on restart; Phase 25 persistence could cover it |
| Test and tooling gaps | P3 | `gosec`/`govulncheck` not installed locally so `make check` skips them; `fakedom` lacks `insertBefore`/`replaceChild`/`replaceWith`; one unidentified node test failed once under load (pass 12a) |
| Agent tool schema still advertises `permission_mode` | P3 | It is now clamped; consider removing it |

## 6. Parked / Out Of Scope For v0.1

- **Phase 23: OpenAI-compatible / generic multi-provider adapter.** Explicitly removed from v0.1 (its plan was deleted 2026-09-28; recover it from git history if the decision is reversed). Ollama Cloud support must stay Ollama-only.
- **Homebrew/Scoop publishing.** Optional in Phase 17, and only if publishing credentials exist.
- **Remote access, JWT auth, and `nandocodego connect` (former Phase 25 bridge).** Removed by ADR-002; v0.1 is localhost only. The bridge UDS listener for `SendMessage` is parked with it.
- **Multi-user collaboration and cloud hosting.** v0.1 is localhost only (ADR-002).
- **Workspace trust flow** (lets project, HTTP, and agent hooks execute). Post-v0.1 feature; until then project-controlled hooks stay parsed but disabled (decided 2026-10-06).
- **Deferred TUI features (from Phase 22):** collapsible hierarchical activity tree, click-to-expand tool panels, mouse lost-release recovery, full textarea-integrated Vim (mutations, dot-repeat, find-repeat, registers, paste/yank), true concurrent `/btw`, `/btw` read-only tool manifest restriction and context-stack modal priority fix.
