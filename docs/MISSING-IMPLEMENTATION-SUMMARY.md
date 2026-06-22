# Missing Implementation Summary

**Date:** 2026-06-22  
**Project:** `nandocodego`  
**Source Material:** `docs/NEXT-PHASES-IMPLEMENTATION-PLAN.md`, `docs/GATE-G0-PHASE-8-14-VALIDATION-PLAN.md`, `docs/REMAINING-PHASES-TASK-REVIEW.md`, `docs/PHASE-LOG.md`, `docs/PROJECT-STATUS-AND-ONBOARDING.md`, `docs/PHASE-*.md`, `docs/CONTEXT-LATENCY-OPTIMIZATION-PLAN.md`, `docs/INCOMPLETE-RESPONSE-RECOVERY-REPORT.md`, `docs/ADR-001-TUI-USER-EXPERIENCE-IMPROVEMENTS.md`, `docs/TASKS-TUI.md`, and repository scan.

## Executive Summary

`nandocodego` has implemented the core local-first agent stack and several later runtime improvements: CLI, TUI, Ollama LLM client, optional direct Ollama Cloud API routing, agent loop, tools, permissions, state, memory, hooks, MCP, sub-agents, skills, command/config UX, background tasks, concurrency, observability, complete tool ecosystem, content compaction, inline completion, directory mention expansion, incomplete-response recovery foundations, HTTP/SSE server mode, multi-agent coordination, semantic workspace indexing/retrieval, TUI index progress, and the Go response-time refactor.

The remaining work should not follow raw numeric phase order. Phase 17 and Phase 18 are intentionally last:

- **Phase 17** is the penultimate release-packaging phase.
- **Phase 18** is the final hardening, eval, docs, and v0.1.0 release-approval phase.
- Phase 25 remote/bridge mode is required for v0.1 and must be implemented before Phase 17 begins.

The immediate feature goal is Phase 25 Remote / Bridge Mode. Carry-forward validation remains required for Gate G0, context/latency/project-analysis evidence, and Phase 22 live/manual UX evidence before release packaging.

## Roadmap Order To Follow

Use `docs/NEXT-PHASES-IMPLEMENTATION-PLAN.md` for the detailed agent-ready implementation plan. Use `docs/REMAINING-PHASES-TASK-REVIEW.md` for reviewed task details, blockers, and evidence requirements after Gate G0. The order below is the compact summary.

1. **Carry forward validation evidence.**
   - Validate Phases 8-14 against their phase docs and live/manual acceptance criteria.
   - Use `docs/GATE-G0-PHASE-8-14-VALIDATION-PLAN.md` for the detailed procedure and evidence template.
   - Treat the code as substantially implemented, but do not mark these phases closed until the live checks are recorded.
   - Record CL/PA evidence for `/trace last`, `/prompt last`, context mode, semantic retrieval, checkpoint/resume behavior, and final-answer completeness.
   - Record Phase 22 live REPL evidence and explicitly close or defer deep-interaction follow-ups.

2. **Implement Phase 25 - Remote / Bridge Mode.**
   - Add connect mode, detached sessions, replay/reconnect, remote TUI bridge, JWT auth, and server-side session persistence.
   - Phase 25 is required v0.1 work and must be complete before Phase 17.

3. **Implement Phase 17 - Distribution and Install.**
   - Add GoReleaser, release artifacts, checksums, direct installer, release workflow, changelog, and release-facing `doctor` checks.
   - Do not use Phase 17 to absorb unfinished feature work.

4. **Implement Phase 18 - Hardening, Eval Suite, and Docs.**
   - Final release gate: evals, fuzz/property tests, security review, docs site, release notes, performance gates, vulnerability checks, and v0.1.0 approval.
   - No later v0.1.0 implementation phase should follow Phase 18.

## Pending Exit-Gate Validations

The following phases have core logic implemented and automated coverage, but still need live/manual validation before they are fully closed:

- **Phase 8 - Memory:** two-session behavioral validation, scan benchmark confirmation, and pending-draft review flow documentation.
- **Phase 9 - Hooks:** live REPL blocking demo and confirmation that project-controlled hooks remain disabled with a clear diagnostic.
- **Phase 10 - MCP:** live validation against a real MCP server, including tool wrapping, timeout handling, and untrusted-content handling.
- **Phase 11 - Sub-agents and Fork:** live validation of child permissions, context sharing, cancellation, result return, and recursion prevention.
- **Phase 12 - Skills:** live REPL validation of discovery, frontmatter parsing, and prompt-injection boundaries.
- **Phase 13 - Slash Commands and Config UX:** manual checks for source-tagged config, model/config commands, and command registry UX.
- **Phase 14 - Tasks:** manual validation of task lifecycle, output streaming, stop/cleanup, and supervisor status.

## Completed Later Plans

These plans are already implemented or substantially delivered and should not be reimplemented unless a regression is found:

- **Phase 15 - Concurrency and Speculative Execution**
- **Phase 16 - Observability and Metrics**
- **Phase 19 - Complete Tool Ecosystem**
- **Phase 20 - Content Compaction**, with hook-dispatch caveats recorded in its plan
- **Phase 21 - Web Interface and HTTP API**
- **Phase 24 - Multi-Agent Coordination**
- **Ollama Cloud API key support**
- **Phase 26 - Inline Completion in TUI Input**
- **Phase 27 - Directory Mention Expansion**
- **Phase 28 - Semantic Workspace Index And Embedding Retrieval**
- **Phase 29 - TUI Semantic Index Progress Observability**
- **Go response-time refactor**

## Required Pre-Release Phases

- **Phase 23 - OpenAI-Compatible LLM Adapter:** removed from the active v0.1 plan.
- **Phase 25 - Remote / Bridge Mode:** required for v0.1. Implement after Phase 21, Phase 24, Ollama Cloud API key support, and Phase 28/29.

## Documentation Drift To Clean Up

- `docs/PROJECT-STATUS-AND-ONBOARDING.md` contains older status tables from earlier project history. Use its current roadmap section and `docs/PHASE-LOG.md` as the ordering source of truth.
- Docker/web docs should stay aligned with the implemented `nandocodego server` command, port 8080 defaults, and `--bind` / `--port` flags.
- `SECURITY.md` still has `security@example.invalid` as a placeholder contact.
- Some docs and examples prefer `qwen3`, while code defaults may still point at older model defaults. Engineers should pass `--model` explicitly until config defaults are reconciled.
- `.codex/agent-context/ARCHITECTURE.md` and parts of `.codex/agent-context/testing-standards.md` reference unrelated stack details and should not be treated as authoritative for this repo.
