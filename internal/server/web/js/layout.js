// Breakpoints from DESIGN-SPEC "Layout". CSS custom properties cannot be used
// in media queries, so the widths live here and the shell sets data attributes
// that app.css styles.

export const BREAKPOINTS = { nav: 1440, tree: 1200, rail: 960 };

const wide = (w, min) => typeof w === 'number' && Number.isFinite(w) && w >= min;

// < 1440: the nav collapses to a 56 px icon rail.
export function navMode(width) { return wide(width, BREAKPOINTS.nav) ? 'full' : 'rail'; }

// < 1200: the project tree becomes a toggle.
export function treeMode(width) { return wide(width, BREAKPOINTS.tree) ? 'full' : 'toggle'; }

// < 960: the right rail becomes a drawer.
export function railMode(width) { return wide(width, BREAKPOINTS.rail) ? 'full' : 'drawer'; }

// < 1200: the permission popover falls back to an inline card.
export function permissionMode(width) { return wide(width, BREAKPOINTS.tree) ? 'popover' : 'inline'; }

export function layoutFor(width) {
  return { nav: navMode(width), tree: treeMode(width), rail: railMode(width), permission: permissionMode(width) };
}
