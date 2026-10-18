# BV: Browser Visual Revamp

**Date:** 2026-10-18 · **Status:** Promoted to the roadmap as step BV · **Plan:** [`BROWSER-VISUAL-REVAMP-PLAN.md`](../plans/BROWSER-VISUAL-REVAMP-PLAN.md)

## Problem statement

How might we make the browser UI look like the two mockups in `docs/plans/UX/`
and work like an IDE-grade control room for a local agent, showing only real
data, without duplicating or jumping ahead of the security and parity work
already on the roadmap?

## Recommended direction

A dedicated **BV track**: one roadmap step, one plan, and slices that wait for
the roadmap step that delivers their backend. The mockups are the visual spec,
not the functional spec. The look (tokens, layout, components) is designed and
tested early against recorded event fixtures in a dev-only component gallery.
Each region of the live app lands when its data exists, and a region without
data shows an honest empty state.

Build order favours the right rail, because you mostly glance at it from
another window while the agent works. The code tab ships read-only first; Save
follows as its own slice and goes through the same permission path, hooks and
audit event as an agent `FileWrite`. The Agents view shows real
coordinator/worker activity. The mockup's registry of agent *types* does not
exist, so that panel lists this session's workers instead.

The alternative, spreading the work across B2 and B3, was rejected. The editor,
the files API and the worker events are new scope no step owns, and spreading
them out would hide them.

## Key assumptions to validate

- [ ] Four columns fit at 1280–1440 px with the nav as an icon rail. Check in
      the BV-1 gallery at 1280 / 1440 / 1920.
- [x] The worker data needed by the graph partly exists. Spike 2026-10-18:
      name, task, model, status, timing and mailbox exist; current step, usage
      and files touched need forwarding (BV-8a); agent types and skill links do
      not exist.
- [x] An agent todo list exists (`TodoWrite` / `TodoRead`), but it is wired
      only in the REPL. BV-3 wires it into server sessions.
- [ ] Today's event shapes are stable enough to build on. Golden fixtures plus
      a drift test; line up with Phase 25 slice 0 typed events.
- [ ] Save can reuse the agent's permission decision outside a run. Check with
      the BV-7 RED tests.
- [ ] Exact sampled colours read well on real monitors (`--border` especially).
      Check in the BV-1 gallery.

## MVP scope

BV-0 spec and tokens (done) → BV-1 gallery → BV-2 shell (with the B2 split) →
BV-3 right rail and BV-4 transcript → BV-5 sessions → BV-6 read-only viewer →
BV-7 Save → BV-8a sub-agent events → BV-8b Agents tab.

## Not doing (and why)

- **"Add agent" / agent-type presets.** This is a new backend concept and
  nothing asks for it yet.
- **A user "Add task".** The agent owns its todo list. Revisit after BV-3.
- **Monaco / CodeMirror / highlight libraries.** They need bundling or npm.
- **Editor extras (LSP, find/replace, multiple cursors).** The agent writes most
  code; the editor is for review and small fixes.
- **Light theme, mobile layout.** The mockups are dark-only and desktop-only.
- **Copying mockup noise.** Footer typos, invented numbers, and the duplicate
  New session button.
- **Pixel-diffing against the mockups.** Their content is invented; diff
  against approved baselines instead.

## Open questions

None open. All were decided interactively on 2026-10-18 (see the plan's
Decisions table).
