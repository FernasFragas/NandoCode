# Gate G0 - Phase 8-14 Validation Plan

**Date:** 2026-05-16 (rewritten browser-first 2026-10-06)  
**Status:** Pending evidence; runs after browser steps B1-B4 (see `docs/roadmap/NEXT-PHASES-IMPLEMENTATION-PLAN.md`)  
**Purpose:** Convert the already-implemented Phase 8-14 work from "code landed" to "accepted" by running live/manual exit gates, recording evidence, and fixing release-blocking defects.

## Why This Gate Exists

Phases 8-14 have substantial implementation and automated coverage, but their phase docs still mark manual/live validation as pending. This gate is validation and reconciliation, not new features.

Gate G0 prevents release packaging from resting on unverified assumptions about memory, hooks, MCP, sub-agents, skills, config/commands, and tasks. Failures found here must be fixed (or explicitly accepted as non-blocking) before Phase 17 if they affect the normal ask/response flow, tool safety, permissions, or session lifecycle.

## Surfaces (ADR-002)

Since 2026-10-05 the browser UI is the primary surface ([ADR-002](../adr/ADR-002-BROWSER-UI-PRIMARY-SURFACE.md)). Run each flow in the **browser** wherever the browser exposes the feature, and record the surface used for every result.

Each phase below has a **Surface** line:

- **Browser:** the browser exposes everything the flow needs today.
- **Browser + TUI:** run the chat/tool/permission part in the browser; run the listed slash commands in the TUI (or `--print`) until their browser endpoint or panel lands (B2 command endpoints, B3 panels). When a browser equivalent exists at validation time, use it instead and note that.
- **CLI:** the flow is about a CLI entry point (`init`, `--print`).

Browser session: start `./bin/nandocodego server` and open the printed `#token=` URL. Use a new browser session (reload or new session) where a flow says "new session".
TUI session: `./bin/nandocodego --model <model> --no-alt-screen` (`--no-alt-screen` keeps transcript evidence in scrollback).

## Source Documents

- `docs/roadmap/NEXT-PHASES-IMPLEMENTATION-PLAN.md`
- `docs/phases/PHASE-8-DETAILED-PLAN.md`
- `docs/phases/PHASE-9-DETAILED-PLAN.md`
- `docs/phases/PHASE-10-DETAILED-PLAN.md`
- `docs/phases/PHASE-11-DETAILED-PLAN.md`
- `docs/phases/PHASE-12-DETAILED-PLAN.md`
- `docs/phases/PHASE-13-DETAILED-PLAN.md`
- `docs/phases/PHASE-14-DETAILED-PLAN.md`
- `docs/phases/PHASE-LOG.md`

The former standalone Phase 14 exit-gate guide is folded into the Phase 14 section below (original archived at `docs/archive/phases/PHASE-14-EXIT-GATE.md`).

## Ground Rules

- Do not add new product scope during Gate G0.
- Do not mark a phase complete based only on automated tests when its detailed plan requires a live/manual flow.
- Use a real local Ollama model for model-dependent flows.
- Record the surface (browser, TUI, CLI) for every flow.
- Keep validation evidence concise: command or URL, surface, model, date, pass/fail, relevant transcript excerpt (browser transcript copy or TUI scrollback), and files touched.
- If a manual flow cannot be run because a prerequisite is missing, record it as `blocked`, not `passed`.
- If a failure is found, classify it before moving on:
  - **Release blocker:** safety, data loss, permission bypass, tool execution wrongness, session lifecycle leak, or core ask/response failure.
  - **Phase blocker:** the documented exit gate cannot pass.
  - **Follow-up:** polish or docs drift that does not invalidate the phase.
- A failure that only reproduces in one surface is still a failure; record which surface.

## Prerequisites

1. Build the current binary:

   ```bash
   go build -o bin/nandocodego ./cmd/nandocodego
   ```

2. Run the standard automated checks:

   ```bash
   go test ./...
   tools/check-allowed-deps.sh
   tools/check-network-policy.sh
   ```

3. Confirm Ollama is running and at least one capable model is available:

   ```bash
   ollama list
   ```

4. Choose one validation model and use it consistently unless a phase requires another:

   ```text
   the code default (qwen3.6:35b) or another locally installed tool-capable model
   ```

5. Create a temporary validation workspace outside normal project state if possible:

   ```bash
   mkdir -p /private/tmp/nandocodego-g0
   ```

6. Capture paths used during validation:

   - repository path
   - config dir
   - data dir
   - cache dir
   - state dir
   - memory dir
   - selected model
   - server URL and port (browser flows; never record the token)

## Evidence Template

Append evidence to `docs/phases/PHASE-LOG.md` under a new Gate G0 entry, or to a short dated validation note if the run is partial.

```markdown
## Gate G0 - Phase 8-14 Validation (YYYY-MM-DD)

Model: <model>
Ollama endpoint: <url>
Binary: <bin/nandocodego version or commit>

| Phase | Surface | Status | Evidence | Follow-up |
|---|---|---|---|---|
| 8 Memory | browser | pass/fail/blocked | <short transcript/file evidence> | <issue/task or none> |
```

Use `pass`, `fail`, or `blocked`. Do not use vague statuses like "seems ok".

## Phase 8 - Memory

**Goal:** prove memory persists across sessions and naturally affects later responses.

**Surface:** Browser.

**Manual flow:**

1. Start a browser session in the project.
2. Ask:

   ```text
   Remember that I prefer table-driven tests in Go.
   ```

3. Confirm one of these happens:

   - a memory file is written under the project/user memory directory;
   - a pending memory draft is written and surfaced clearly;
   - the assistant instructs how to review/promote the draft.

4. Start a new session in the same project (restart the server, then open a new browser session).
5. Ask:

   ```text
   Write or describe a Go unit test for a simple function.
   ```

6. Confirm the response naturally uses or recommends table-driven style without being told again.

**Evidence to record:**

- memory file or pending draft path;
- excerpt showing stored preference;
- excerpt from second session using table-driven tests.

**Fail if:**

- no memory/draft is created;
- second session ignores the remembered preference;
- memory path is unclear or outside expected state/data directories;
- validation requires any network destination other than Ollama.

## Phase 9 - Hooks

**Goal:** prove command hooks can block dangerous tools before execution and that hook snapshots are frozen for the session.

**Surface:** Browser. (Phase 18 also requires this hook-blocking gate to run in the browser.)

**Manual flow:**

1. Configure a user-level command hook matching `Bash(rm -rf*)`.
2. Hook command exits with code `2` and stderr:

   ```text
   denied by policy
   ```

3. Start the server in `dontAsk` permission mode and open a browser session.
4. Prompt the model to attempt a matching command.
5. Confirm:

   - tool execution is blocked before the command runs;
   - the model-visible tool result includes `denied by policy`;
   - a user-visible hook notice appears in the browser transcript;
   - editing the hook file during the session has no effect until restart.

**Evidence to record:**

- hook config path and relevant snippet;
- transcript excerpt showing denied tool result;
- note confirming same-session hook edit had no effect.

**Fail if:**

- matching bash command executes;
- denial is only visible to user but not model;
- live hook file edits mutate the active snapshot.

## Phase 10 - MCP

**Goal:** prove real MCP server integration and HTTP hook safety behavior.

**Surface:** Browser (flow A tool use and permission prompt; flow B hook decision). Startup diagnostics: server stderr.

**Manual flow A - stdio MCP tool:**

1. Configure a local stdio MCP server in `config.toml`.
2. Start the server and open a browser session.
3. Ask the model to use one server-provided tool.
4. Confirm:

   - tool appears as `mcp__<server>__<tool>`;
   - first use raises a permission request in the browser modal;
   - result is rendered in the transcript;
   - stopping the server does not leave an orphan MCP process.

**Manual flow B - HTTP hook safety:**

1. Configure a user-level HTTP hook targeting a local test server.
2. Start the server and open a browser session.
3. Attempt a bash tool call.
4. Confirm the hook fires and its decision is honored.
5. Configure a hook targeting a private IP outside the explicitly allowed list.
6. Confirm startup rejects it with a clear diagnostic.

**Evidence to record:**

- MCP config snippet with secrets redacted;
- transcript excerpt with `mcp__...` tool;
- process check confirming no orphan;
- HTTP hook diagnostic excerpt.

**Fail if:**

- MCP process leaks after server exit;
- MCP tool bypasses permission;
- unsafe HTTP hook is silently skipped or allowed.

## Phase 11 - Sub-Agents And Fork

**Goal:** prove bounded sub-agent execution, result return, recursion prevention, and cancellation.

**Surface:** Browser for steps 1-5. Cancellation (steps 6-7): browser Stop button once B2 run cancel lands; until then TUI `Ctrl-C`.

**Manual flow:**

1. Start a browser session with the selected model.
2. Ask the main agent to delegate a bounded research task to a sub-agent.
3. Confirm:

   - sub-agent start notice appears;
   - child tool activity is visible;
   - sub-agent completion appears;
   - main agent receives the child result and continues.
4. Ask for or induce nested sub-agent spawning from inside the child.
5. Confirm error:

   ```text
   sub-agent recursion not allowed
   ```

6. Start a long child task and cancel the run.
7. Confirm the child cancels within two seconds.

**Evidence to record:**

- transcript excerpt with child lifecycle;
- recursion error excerpt;
- cancellation surface and timing.

**Fail if:**

- parent never receives child result;
- recursion succeeds;
- cancellation leaves the child running.

## Phase 12 - Skills

**Goal:** prove project skills load, influence behavior, and hot-reload.

**Surface:** Browser + TUI (`/skills list` until the B3 skills panel lands).

**Manual flow:**

1. Create `.nandocodego/skills/my-review.md` with valid frontmatter and a code-review checklist.
2. Start a browser session.
3. Ask the agent to review a file and invoke/use the skill.
4. Confirm the assistant uses the checklist from the skill.
5. While the session is running, add another valid skill file.
6. Within one second, list skills (`/skills list` in a TUI session on the same project, or the skills panel if it exists).
7. Confirm the new skill appears without restart.

**Evidence to record:**

- skill file path and frontmatter excerpt;
- transcript excerpt showing checklist use;
- skills list excerpt after hot-reload, with surface.

**Fail if:**

- skill is ignored;
- invalid frontmatter is silently accepted;
- hot-reload does not update the list.

## Phase 13 - Slash Commands And Config UX

**Goal:** prove config defaults, one-shot print mode, and core commands work live.

**Surface:** CLI (steps 1-5); Browser for the configured-model check; Browser + TUI for commands (B2/B3 browser equivalents when present).

**Manual flow:**

1. Run:

   ```bash
   ./bin/nandocodego init
   ```

2. Edit config to set the default model to the selected local model.
3. Start `./bin/nandocodego server` without `--model`, open a browser session, and confirm it uses the configured model. Also confirm `./bin/nandocodego` (TUI, until Phase 17 changes the default command) starts with it.
4. Run:

   ```bash
   ./bin/nandocodego --print "What is 2+2?"
   ```

5. Confirm stdout contains the response and the process exits `0`.
6. Confirm `--print` with a malformed config exits non-zero with actionable text (decision 2026-10-06, `docs/reports/bugs/BUG-20260607-invalid-config-warning-does-not-fail-print.md`; `blocked` until B1 lands the fix).
7. Run these commands, each in the browser if an equivalent exists, otherwise in the TUI:

   ```text
   /models
   /memory list
   /permissions show
   /hooks list
   ```

8. Confirm each returns useful source-tagged or state-aware output.

**Evidence to record:**

- config path and redacted model setting;
- `--print` command results and exit codes;
- command output excerpts, with surface.

**Fail if:**

- config model is ignored by either surface;
- `--print` enters the TUI or hangs;
- `/model`, `/models`, or the browser model picker does not validate against live Ollama;
- source-tagged config/rules are missing where promised.

## Phase 14 - Tasks

**Goal:** prove background task lifecycle is non-blocking, inspectable, stoppable, and session-scoped.

**Surface:** Browser (tasks are driven through agent tools; the browser renders task lifecycle events). `/agents list` and the status-bar task count are TUI-only until the B3 tasks panel lands.

**Prerequisites:** Ollama reachable at the configured endpoint; the selected model pulled.

**Manual flow:**

1. Start a browser session.
2. **Non-blocking create.** Ask: `Please run this command in the background: sleep 30 && echo "Task complete"`. Confirm a task ID returns immediately (for example `b-1a2b3c4d`) with its output file path, the run finishes, and input is usable again while the task keeps running.
3. **List.** Ask: `List all running tasks and show their status`. Confirm the agent calls TaskList and shows ID, status, description, and output path.
4. **Get with tail.** Ask: `Check the status of task <ID> and show me the last few lines of output`. Confirm TaskGet returns the summary plus an output tail and the task is still running.
5. **Stop.** Ask: `Stop task <ID> and verify it's killed`. Confirm TaskStop moves the task from `running` to `killed` within 200 ms and a later TaskGet shows `killed`.
6. **Output file.** Ask the agent to read the task output file. Confirm it exists, is readable during execution, contains timestamped JSONL lines, and ends with the exit sentinel `{"kind":"exit","code":N}`.
7. **Session isolation.** Restart the server and open a new browser session. Ask: `List all running tasks`. Confirm the previous session's tasks are not listed (their output files may remain on disk; state does not reload).
8. **Agent task (optional).** Ask: `Create a background agent task to summarize this conversation`. Confirm a task with kind `agent` and an `a-` ID, status `running`, and JSONL-formatted agent events in its output.

**Acceptance checklist:**

- [ ] TaskCreate returns a task ID immediately (< 50 ms).
- [ ] The session stays responsive while a background task runs.
- [ ] TaskList shows all tasks sorted by creation time.
- [ ] TaskGet returns the full summary with output tail.
- [ ] TaskStop cancels within 200 ms and transitions to `killed`.
- [ ] JSONL output exists and is readable during execution, ending with the exit sentinel.
- [ ] Task lifecycle events render in the browser (and, TUI-only, the status bar shows `[N tasks running]` and `/agents list` shows only `a-` tasks).
- [ ] A new session does not inherit the previous session's tasks.
- [ ] `go test ./...`, `go test -race ./internal/tasks/...`, and `go build ./cmd/nandocodego` pass.

**Known limitations (by design for Phase 14):** task output files grow unbounded (no rotation in `internal/tasks` as of 2026-10-06), no automatic retry, no task dependency graphs, no cross-session task persistence; output files can take 0.6-2 s to appear on disk because of OS buffering.

**Troubleshooting:**

- `task supervisor unavailable`: the TaskCreate tool is not registered in the runtime being used.
- Task appears stuck: confirm Ollama responds (`curl http://localhost:11434/api/tags`) and the output file is being written.
- JSONL file not found: use the path returned by TaskCreate; check the session directory exists.
- Agent task missing from `/agents list`: confirm its kind is `agent` and it was created with TaskCreate, not the Agent tool.

**Evidence to record:**

- task ID and output path;
- TaskList/TaskGet excerpts;
- stop timing;
- second-session isolation note.

**Fail if:**

- the session blocks until task completion;
- stop does not cancel promptly;
- JSONL output is missing;
- a task leaks into another session.

## Required Updates After Validation

After running Gate G0:

1. Update `docs/phases/PHASE-LOG.md` with a Gate G0 validation entry.
2. For each phase that passes, update its detailed plan status line or exit-gate section with the pass date.
3. For each failure, either:
   - fix it immediately if it is a release/phase blocker (write the failing reproduction test first);
   - add a focused follow-up to `docs/roadmap/BACKLOG.md`;
   - document it as non-blocking with rationale.
4. Re-run relevant automated tests after any code fix.

## Gate G0 Completion Criteria

Gate G0 is complete only when:

- Phases 8-14 each have `pass` or accepted `non-blocking follow-up` status, with the surface recorded.
- No permission, tool-execution, memory persistence, child-agent lifecycle, MCP lifecycle, skill trust, config, or task lifecycle blocker remains open.
- `docs/phases/PHASE-LOG.md` records the validation evidence.
- Together with the Workstream CL/PA evidence gate, it is accepted before Phase 17 starts (see `docs/roadmap/NEXT-PHASES-IMPLEMENTATION-PLAN.md`).
