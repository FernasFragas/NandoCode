// Project tree. The `@` hover action inserts a mention into the composer; the
// files API (V6a) supplies the data. V6c listens for file:open(path).
import { renderTree } from '../ui/tree.js';
import { delegate } from '../delegate.js';
import { buildTree, loadTree } from '../treedata.js';

export function mount(ctx) {
  const region = ctx.el('tree');
  if (!region) return;
  let nested = [];
  const expanded = new Set();
  const dirs = new Set();

  const collect = (nodes, parent) => nodes.forEach((n) => {
    const path = parent ? parent + '/' + n.name : n.name;
    if (n.kind === 'dir') { dirs.add(path); collect(n.children || [], path); }
  });

  const draw = () => ctx.render('tree', renderTree({ entries: nested, expanded: [...expanded] }));
  async function refresh(force) {
    const entries = await loadTree(ctx, { force });
    nested = buildTree(entries);
    dirs.clear();
    collect(nested, '');
    draw();
  }
  draw();

  ctx.on('session:ready', () => refresh(true));
  ctx.on('session:reset', () => { nested = []; expanded.clear(); dirs.clear(); draw(); });
  ctx.on('tree:refresh', () => { if (ctx.state.sessionId) refresh(true); });
  ctx.on('tree:error', (err) => ctx.toast('Could not load the file tree: ' + (err && err.message ? err.message : 'error')));

  delegate(region, 'click', (action, id) => {
    if (action === 'tree-toggle' || action === 'tree-select') {
      if (action === 'tree-toggle' || dirs.has(id)) {
        if (expanded.has(id)) expanded.delete(id); else expanded.add(id);
        draw();
      } else {
        ctx.emit('file:open', id);
      }
    } else if (action === 'mention') {
      ctx.emit('composer:insert-mention', id);
    }
  });
}
