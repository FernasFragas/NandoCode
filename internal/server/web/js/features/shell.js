// Grid shell: mirrors the viewport width and the nav / panel state onto
// data-* attributes of #app, which css/app.css styles. No inline styles.
import { layoutFor } from '../layout.js';
import { delegate } from '../delegate.js';

export function visibility(state) {
  const l = state.layout;
  return {
    tree: l.tree === 'full' ? state.nav === 'project' : !!state.treeOpen,
    rail: l.rail === 'full' ? true : !!state.railOpen,
  };
}

export function mount(ctx) {
  const app = ctx.el('app');
  if (!app) return;

  function apply() {
    const l = ctx.state.layout;
    const v = visibility(ctx.state);
    app.setAttribute('data-nav', l.nav);
    app.setAttribute('data-tree', l.tree);
    app.setAttribute('data-rail', l.rail);
    app.setAttribute('data-nav-active', ctx.state.nav);
    app.setAttribute('data-tree-visible', String(v.tree));
    app.setAttribute('data-rail-visible', String(v.rail));
  }

  function measure() {
    const layout = layoutFor(ctx.win.innerWidth);
    const prev = ctx.state.layout;
    if (prev.nav === layout.nav && prev.tree === layout.tree && prev.rail === layout.rail && prev.permission === layout.permission) return;
    // Leaving the narrow modes closes their overlays.
    ctx.set({ layout, treeOpen: layout.tree === 'full' ? false : ctx.state.treeOpen, railOpen: layout.rail === 'full' ? false : ctx.state.railOpen });
  }

  ctx.state.layout = layoutFor(ctx.win.innerWidth);
  apply();
  ctx.on('state', apply);
  if (typeof ctx.win.addEventListener === 'function') ctx.win.addEventListener('resize', measure);

  const toggles = ctx.el('panel-toggles');
  if (toggles) {
    delegate(toggles, 'click', (action) => {
      if (action === 'toggle-tree') ctx.set({ treeOpen: !ctx.state.treeOpen });
      else if (action === 'toggle-rail') ctx.set({ railOpen: !ctx.state.railOpen });
    });
  }
}
