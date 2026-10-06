# ADR-002: Browser UI Is The Primary Surface

Date: 2026-10-05

Status: Accepted for v0.1 roadmap

Supersedes: the "TUI is the primary surface" assumption in
[ADR-001](ADR-001-TUI-USER-EXPERIENCE-IMPROVEMENTS.md). ADR-001 remains the
record for the terminal UI work that already landed.

## Context

Until now the roadmap treated the Bubble Tea TUI as the main product surface:
Phase 22 polished it, Phase 25 planned a remote TUI client
(`nandocodego connect` + `internal/tui/remote_bridge.go`), and Phase 18 had a
TUI frame-time release gate. The browser UI served by `nandocodego server`
(Phase 21, `internal/server/web/index.html`) reached its P0 chat scope on
2026-06-23 but still lacks basic parity with the TUI.

Browser gaps found in a source review on 2026-10-05:

- Server routes cover only session create/get/delete, SSE events, message
  POST, permission resolution, model switch, file tree, model list, and health.
- No stop/cancel for an active run (only deleting the whole session).
- No way to provide an Ollama Cloud API key; the server never prompts, so
  cloud models require `OLLAMA_API_KEY` before startup.
- No equivalents for `/clear`, `/compact`, `/index`, `/cost`, memory,
  permissions, tasks, skills, or hooks.
- Sessions are in memory only; there is no session list and nothing survives
  a server restart.
- The served page is one ~1,400-line HTML file with inline JS and no ARIA roles.

## Decision

1. **The browser UI is the primary v0.1 surface.** Product, docs, validation,
   and release gates are browser-first.
2. **The TUI stays shipped in maintenance mode.** Bug fixes and regressions
   only; no new TUI features. `--print` stays as the scripting interface.
3. **Plain `nandocodego` starts the server and opens the browser.** The TUI
   moves behind an explicit entry point (proposed: `nandocodego tui`).
   `nandocodego --print ...` and other subcommands keep working unchanged.
4. **Localhost only for v0.1.** The server binds to loopback and uses the
   existing generated opaque bearer token. No JWT, no remote access, no
   `nandocodego connect`.
5. **Frontend stays plain JS with no build tools**, but is split from the single
   inline `index.html` into separate embedded static files (HTML, CSS, JS
   modules) under `internal/server/web/` so it is easier to maintain and test.
   No npm toolchain; the dependency allowlist policy is unchanged.
6. **The cloud-model switch bug is P0.** See
   [BUG-20260607-server-model-endpoint-rejects-listed-cloud-model](../reports/bugs/BUG-20260607-server-model-endpoint-rejects-listed-cloud-model.md);
   retest on 2026-10-05 showed it is still reproducible for stale `:cloud` tags.

## Consequences

Removed from v0.1 scope:

- Phase 25 `nandocodego connect`, `internal/tui/remote_bridge.go`, JWT auth,
  the bridge UDS listener, and `prctl` hardening for remote hosts.
- Deferred TUI features: hierarchical activity tree, click-to-expand tool
  panels, mouse lost-release recovery, full textarea Vim, concurrent `/btw`.
- The Phase 18 TUI frame-time gate as a release blocker, and TUI render
  benchmarks in the performance follow-up plan.

Added to or promoted in v0.1 scope:

- Browser stop/cancel, cloud API key entry, specific command endpoints
  (clear, compact, index, cost), and a session list.
- Server session persistence and detach/reattach (rescoped Phase 25).
- The P1 browser management panels from `WEB-UI-UX-PRODUCT-PLAN.md`.
- Browser accessibility, security-header tests, Origin checks, and file-tree
  traversal hardening.
- Default-command change and browser-first install/first-run flow (Phase 17).
- Browser-first manual validation for Gate G0 and CL/PA.

Rules that stay in force:

- The agent event loop remains the integration spine; the browser consumes the
  same typed events as the TUI.
- No generic slash-command HTTP bridge. Each browser capability gets a small,
  tested endpoint that reuses the existing package (`permissions.Resolve`,
  `internal/memory`, `internal/skills`, `tasks.Supervisor`, ...).
- Cloud credentials are requested before any project context is sent.
- Ollama-only provider scope; Phase 23 stays parked.

## Follow-Up Decisions (2026-10-06)

- **TUI entry point:** `nandocodego tui`. `nandocodego --model X` with no subcommand starts the browser with that model; no warning.
- **Default model:** the browser first-run flow asks the user to pick an installed Ollama model (or suggests a small one to pull). `qwen3.6:35b` stays the config default. Phase 17 work.
- **Browser JS tests:** CI may run Node's built-in test runner (`node --test`) on DOM-free ES modules under `internal/server/web/`. No npm, no `package.json`, no dependencies; the plain-JS, no-build-tools rule is unchanged.
- **Permission-rule `*`:** matches any characters, including `/` (roadmap step B1).

## Owning Documents

- Roadmap order: [NEXT-PHASES-IMPLEMENTATION-PLAN.md](../roadmap/NEXT-PHASES-IMPLEMENTATION-PLAN.md)
- Browser product plan: [WEB-UI-UX-PRODUCT-PLAN.md](../plans/WEB-UI-UX-PRODUCT-PLAN.md)
- Session durability: [PHASE-25-DETAILED-PLAN.md](../phases/PHASE-25-DETAILED-PLAN.md) (rescoped)
- Backlog: [BACKLOG.md](../roadmap/BACKLOG.md)
