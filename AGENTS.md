# Agent Instructions

Instructions for coding agents (Claude Code, Codex, and others) working in this repository.

**Start:** [`docs/roadmap/NEXT-PHASES-IMPLEMENTATION-PLAN.md`](docs/roadmap/NEXT-PHASES-IMPLEMENTATION-PLAN.md). Take the next step and slice, and open the plan its Read-Next row names.

**Authority:** roadmap (what and when) > plan or phase doc (how) > reports (evidence only). When code and docs disagree, the code is right; fix the doc.

## Rules

- **Test first.** Write the named failing test (RED), then fix (GREEN). Refactors need characterization tests first and go in a separate commit.
- **Browser UI is the primary surface; the TUI is maintenance-only** ([ADR-002](docs/adr/ADR-002-BROWSER-UI-PRIMARY-SURFACE.md)). The server is localhost only: no JWT, no remote access, no `nandocodego connect`.
- **Browser JS:** plain ES modules under `internal/server/web/`, no build tools, no npm. Test with `node --test` only.
- **Dependencies and network:** no new Go dependency without an entry in `tools/allowed-deps.txt` and a justification; no new network endpoints (`tools/check-network-policy.sh`).
- **Ollama only.** Local models are the default; a cloud model needs a credential before any project context is sent.
- **No generic slash-command HTTP bridge.** Each browser capability gets one small, tested endpoint that reuses the owning package.
- **Latency:** don't lower `num_ctx` globally; use the context-budget machinery.
- **Tests:** never touch the real home directory (`t.Setenv("NANDOCODEGO_DATA_HOME", t.TempDir())` and similar); replace `time.Sleep` synchronization in any test file you modify.

## Done

A step or slice is done when the **Step Exit Check** in the roadmap passes, including a [`docs/phases/PHASE-LOG.md`](docs/phases/PHASE-LOG.md) entry.
