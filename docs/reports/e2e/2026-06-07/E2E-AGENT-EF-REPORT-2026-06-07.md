# E2E Agent EF Report - 2026-06-07

## Scope

- Lane: `E` and `F`
- Owner agent: `Lane EF worker`
- Functional areas:
  - Lane E: memory, hooks, skills, MCP, prompt fidelity
  - Lane F: tasks, sub-agents, fork, coordinator mode, mailbox/`SendMessage`, dream lifecycle
- Source commit: `cd6743c7968ea6c809859a9a1f8a8f30ea930e88`
- Start/end time:
  - Start: `2026-06-07T11:24:00+01:00`
  - Last updated: `2026-06-07T11:46:00+01:00`

## Environment

- OS: `Darwin 25.5.0 arm64`
- Go version: `go version go1.26.2 darwin/arm64`
- Ollama status: `ollama binary present at /usr/local/bin/ollama; localhost:11434 not listening at checkpoint`
- Model/provider: `pending live execution; planning local stub Ollama-compatible endpoint for REPL coverage`
- Isolated config/data/cache/state paths: `pending creation`
- Browser/terminal details where relevant: `TTY-based REPL execution pending`

## Scenario Results

Checkpoint status only. Live execution had not started when this artifact was created.

| Scenario | Priority | Automation | Evidence | Status | Bug/Block | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| E-001 memory list/show/edit/promote flow | p1 | tty_repl | pending | pending |  | Source and plan reviewed; live run pending |
| E-002 memory recall mode switching | p1 | tty_repl | pending | pending |  | Source and plan reviewed; live run pending |
| E-003 fast recall versus llm recall behavior | p1 | tty_repl + stub_llm | pending | pending |  | Needs controlled LLM endpoint |
| E-004 hooks list and reload flow | p1 | tty_repl | pending | pending |  | Source and plan reviewed; live run pending |
| E-005 command hook execution | p1 | tty_repl | pending | pending |  | Requires controlled hook fixture |
| E-006 prompt hook execution | p1 | tty_repl + stub_llm | pending | pending |  | Requires controlled LLM endpoint |
| E-007 disabled project/HTTP/agent hook diagnostics | p1 | tty_repl | pending | pending |  | Source review found possible docs/code drift to verify |
| E-008 skills list/show | p1 | tty_repl | pending | pending |  | Source and plan reviewed; live run pending |
| E-009 skill hot reload or watcher behavior if enabled | p2 | tty_repl | pending | pending |  | Needs persistent REPL session and file mutation |
| E-010 MCP server config load and tool exposure | p1 | doctor + tty_repl | pending | pending |  | Needs local MCP fixture |
| E-011 MCP live tool call where environment permits | p1 | tty_repl + stub_llm + local_mcp | pending | pending |  | Needs local MCP fixture and controlled LLM endpoint |
| E-012 prompt fidelity for listing prompts | p1 | tty_repl + stub_llm | pending | pending |  | Needs prompt dump capture |
| F-001 task create/list/status path | p1 | tty_repl + stub_llm | pending | pending |  | Needs controlled LLM endpoint |
| F-002 task stop/cleanup | p1 | tty_repl + stub_llm | pending | pending |  | Needs controlled LLM endpoint |
| F-003 sub-agent spawn and result return | p1 | tty_repl + stub_llm | pending | pending |  | Needs controlled LLM endpoint |
| F-004 fork behavior and isolation | p2 | analysis + live_if_exposed | pending | pending |  | User-facing exposure still under review |
| F-005 coordinator mode spawn of multiple workers | p1 | tty_repl + stub_llm | pending | pending |  | Requires `NANDOCODEGO_COORDINATOR=1` session |
| F-006 worker restriction enforcement | p1 | tty_repl + stub_llm | pending | pending |  | Will verify coordinator-only tool exclusion |
| F-007 `SendMessage` mailbox path | p1 | tty_repl + stub_llm | pending | pending |  | Requires coordinator worker lifecycle |
| F-008 auto-resume behavior on completed worker | p2 | tty_repl + stub_llm | pending | pending |  | Requires completed worker plus `SendMessage` |
| F-009 dream lifecycle start and kill on new prompt | p2 | tty_repl + stub_llm | pending | pending |  | Requires `NANDOCODEGO_DREAM=1` session and observable proof |
| F-010 task output replay and transcript visibility | p2 | tty_repl + stub_llm | pending | pending |  | Requires task output capture and follow-up prompt |

## Coverage Notes

- Functional paths covered:
  - Plan, source-of-truth docs, and relevant code paths for Lane E/F
  - Environment baseline (`go`, `git`, OS, `ollama` presence, localhost listener check)
- Positive paths covered:
  - None yet
- Negative/error paths covered:
  - `curl -sS http://localhost:11434/api/tags` failed at checkpoint because no local Ollama daemon was listening
- Performance or reliability evidence captured:
  - None yet
- Known coverage gaps:
  - No live REPL evidence yet
  - No isolated fixture directories created yet
  - No MCP or LLM stub runtime started yet

## Findings

| Finding | Severity | Disposition | Impacted Scenarios | Evidence |
| --- | --- | --- | --- | --- |
| No confirmed Lane E/F defects at checkpoint |  |  |  |  |

## Bugs

- None at checkpoint.

## Blocks

- None at checkpoint. Environment limitation noted: local Ollama daemon absent, but alternate local stub path is being prepared before any scenario is marked blocked.

## Risk Assessment

- Top user-facing risks:
  - Untested REPL-only slash-command behavior across memory/hooks/skills flows
  - Untested multi-agent coordination behavior under coordinator mode
- Top release risks:
  - Potential docs/code drift around hook disablement behavior
  - Lane F flows may depend on subtle tool-registry differences between coordinator and worker runtimes
- Top test-confidence risks:
  - No live evidence yet
  - Some scenarios may require custom local stubs to replace absent Ollama runtime

## Rerun Recommendation

- Scenarios to rerun immediately:
  - All once the isolated runtime and local stubs are up
- Scenarios to rerun after fixes:
  - None yet
- Scenarios that need a different environment:
  - None declared yet; will use block reports only if local stubs cannot cover required interfaces

## Lane Recommendation

- `blocked`

Checkpoint rationale: live execution was still in setup at the time of this first report write, so no scenario can yet be recommended as pass or pass_with_known_non_blockers.
