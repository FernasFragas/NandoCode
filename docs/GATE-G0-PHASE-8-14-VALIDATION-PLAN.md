# Gate G0 - Phase 8-14 Validation Plan

**Date:** 2026-05-16  
**Status:** Next implementation task  
**Purpose:** Convert the already-implemented Phase 8-14 work from "code landed" to "accepted" by running live/manual exit gates, recording evidence, and fixing release-blocking defects.

## Why This Gate Exists

Phases 8-14 have substantial implementation and automated coverage, but their phase docs still mark manual/live validation as pending. The next roadmap step is not a new feature. It is a validation and reconciliation gate.

Gate G0 prevents later work from being built on unverified assumptions about memory, hooks, MCP, sub-agents, skills, config/commands, and tasks. Failures found here should be fixed before Workstream CL, Phase 22, Phase 21, Phase 24, or Phase 25 if they affect normal ask/response flow, tool safety, permissions, or session lifecycle.

## Source Documents

- `docs/NEXT-PHASES-IMPLEMENTATION-PLAN.md`
- `docs/PHASE-8-DETAILED-PLAN.md`
- `docs/PHASE-9-DETAILED-PLAN.md`
- `docs/PHASE-10-DETAILED-PLAN.md`
- `docs/PHASE-11-DETAILED-PLAN.md`
- `docs/PHASE-12-DETAILED-PLAN.md`
- `docs/PHASE-13-DETAILED-PLAN.md`
- `docs/PHASE-14-DETAILED-PLAN.md`
- `docs/PHASE-14-EXIT-GATE.md`
- `docs/PHASE-LOG.md`

## Ground Rules

- Do not add new product scope during Gate G0.
- Do not mark a phase complete based only on automated tests when its detailed plan requires a live/manual flow.
- Use a real local Ollama model for model-dependent flows.
- Prefer `--no-alt-screen` during manual validation so transcript evidence remains visible.
- Keep validation evidence concise: command, model, date, pass/fail, relevant transcript excerpt, and files touched.
- If a manual flow cannot be run because a prerequisite is missing, record it as `blocked`, not `passed`.
- If a failure is found, classify it before moving on:
  - **Release blocker:** safety, data loss, permission bypass, tool execution wrongness, session lifecycle leak, or core ask/response failure.
  - **Phase blocker:** the documented exit gate cannot pass.
  - **Follow-up:** polish or docs drift that does not invalidate the phase.

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
   qwen3 or another locally installed tool-capable model
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

## Evidence Template

Append evidence to `docs/PHASE-LOG.md` under a new Gate G0 entry, or to a short dated validation note if the run is partial.

```markdown
## Gate G0 - Phase 8-14 Validation (YYYY-MM-DD)

Model: <model>
Ollama endpoint: <url>
Binary: <bin/nandocodego version or commit>

| Phase | Status | Evidence | Follow-up |
|---|---|---|---|
| 8 Memory | pass/fail/blocked | <short transcript/file evidence> | <issue/task or none> |
```

Use `pass`, `fail`, or `blocked`. Do not use vague statuses like "seems ok".

## Phase 8 - Memory

**Goal:** prove memory persists across sessions and naturally affects later responses.

**Manual flow:**

1. Start a clean REPL in the same project:

   ```bash
   ./bin/nandocodego --model <model> --no-alt-screen
   ```

2. Ask:

   ```text
   Remember that I prefer table-driven tests in Go.
   ```

3. Confirm one of these happens:

   - a memory file is written under the project/user memory directory;
   - a pending memory draft is written and surfaced clearly;
   - the assistant instructs how to review/promote the draft.

4. Exit and start a fresh REPL in the same project.

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

**Manual flow:**

1. Configure a user-level command hook matching `Bash(rm -rf*)`.
2. Hook command exits with code `2` and stderr:

   ```text
   denied by policy
   ```

3. Start the REPL in `dontAsk` permission mode.
4. Prompt the model to attempt a matching command.
5. Confirm:

   - tool execution is blocked before the command runs;
   - the transcript/model-visible tool result includes `denied by policy`;
   - user-visible hook notice appears;
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

**Manual flow A - stdio MCP tool:**

1. Configure a local stdio MCP server in `config.toml`.
2. Start the REPL.
3. Ask the model to use one server-provided tool.
4. Confirm:

   - tool appears as `mcp__<server>__<tool>`;
   - first use prompts for permission;
   - result is rendered in transcript;
   - stopping the REPL does not leave an orphan MCP process.

**Manual flow B - HTTP hook safety:**

1. Configure a user-level HTTP hook targeting a local test server.
2. Start the REPL.
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

- MCP process leaks after REPL exit;
- MCP tool bypasses permission;
- unsafe HTTP hook is silently skipped or allowed.

## Phase 11 - Sub-Agents And Fork

**Goal:** prove bounded sub-agent execution, result return, recursion prevention, and cancellation.

**Manual flow:**

1. Start REPL with the selected model.
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

6. Start a long child task and press Ctrl-C.
7. Confirm child cancels within two seconds.

**Evidence to record:**

- transcript excerpt with child lifecycle;
- recursion error excerpt;
- cancellation timing.

**Fail if:**

- parent never receives child result;
- recursion succeeds;
- Ctrl-C leaves child running.

## Phase 12 - Skills

**Goal:** prove project skills load, influence behavior, and hot-reload.

**Manual flow:**

1. Create `.nandocodego/skills/my-review.md` with valid frontmatter and a code-review checklist.
2. Start REPL.
3. Ask the agent to review a file and invoke/use the skill.
4. Confirm assistant uses the checklist from the skill.
5. While REPL is running, add another valid skill file.
6. Within one second, run `/skills list`.
7. Confirm the new skill appears without restart.

**Evidence to record:**

- skill file path and frontmatter excerpt;
- transcript excerpt showing checklist use;
- `/skills list` excerpt after hot-reload.

**Fail if:**

- skill is ignored;
- invalid frontmatter is silently accepted;
- hot-reload does not update list.

## Phase 13 - Slash Commands And Config UX

**Goal:** prove config defaults, one-shot print mode, and core slash commands work live.

**Manual flow:**

1. Run:

   ```bash
   ./bin/nandocodego init
   ```

2. Edit config to set the default model to the selected local model.
3. Run `./bin/nandocodego` without `--model` and confirm the REPL starts with the configured model.
4. Run:

   ```bash
   ./bin/nandocodego --print "What is 2+2?"
   ```

5. Confirm stdout contains the response and process exits `0`.
6. In the REPL, run:

   ```text
   /models
   /memory list
   /permissions show
   /hooks list
   ```

7. Confirm each command returns useful source-tagged or state-aware output.

**Evidence to record:**

- config path and redacted model setting;
- `--print` command result and exit code;
- slash command transcript excerpts.

**Fail if:**

- config model is ignored;
- `--print` enters the TUI or hangs;
- `/model` or `/models` does not validate against live Ollama;
- source-tagged config/rules are missing where promised.

## Phase 14 - Tasks

**Goal:** prove background task lifecycle is non-blocking, inspectable, stoppable, and session-scoped.

Use `docs/PHASE-14-EXIT-GATE.md` as the detailed guide. The shorter required flow is:

1. Start REPL.
2. Ask agent to run:

   ```text
   sleep 30 && echo done
   ```

   as a background task.

3. Confirm TaskCreate returns a task ID immediately.
4. Confirm `/agents list` or TaskList shows it as running.
5. Stop the task.
6. Confirm it transitions to `killed` within 200 ms.
7. Confirm JSONL output file exists and is readable during execution.
8. Start a second REPL session and confirm it does not inherit or resume the previous session task.

**Evidence to record:**

- task ID;
- task output path;
- TaskList/TaskGet excerpts;
- stop timing;
- second-session isolation note.

**Fail if:**

- REPL blocks until task completion;
- stop does not cancel promptly;
- JSONL output is missing;
- task leaks into another session.

## Required Updates After Validation

After running Gate G0:

1. Update `docs/PHASE-LOG.md` with a Gate G0 validation entry.
2. For each phase that passes, update its detailed plan status line or exit-gate section with the pass date.
3. For each failure, either:
   - fix it immediately if it is a release/phase blocker;
   - add a focused follow-up task before Workstream CL;
   - document it as non-blocking with rationale.
4. Re-run relevant automated tests after any code fix.
5. Only move to Workstream CL after all release/phase blockers from Gate G0 are closed.

## Gate G0 Completion Criteria

Gate G0 is complete only when:

- Phases 8-14 each have `pass` or accepted `non-blocking follow-up` status.
- No permission, tool-execution, memory persistence, child-agent lifecycle, MCP lifecycle, skill trust, config, or task lifecycle blocker remains open.
- `docs/PHASE-LOG.md` records the validation evidence.
- The next step in `docs/NEXT-PHASES-IMPLEMENTATION-PLAN.md` is Workstream CL.

