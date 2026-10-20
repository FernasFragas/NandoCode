// Left navigation (DESIGN-SPEC "Nav"). Chat hides the tree, Project shows it,
// Tasks and Diagnostics focus a section of the right rail. Agents and
// Transcript arrive with later packets.
import { renderNav } from '../ui/nav.js';
import { delegate } from '../delegate.js';

const LATER = { transcript: 'Transcript' };

export function mount(ctx) {
  const region = ctx.el('nav');
  if (!region) return;

  const draw = () => ctx.render('nav', renderNav({ active: ctx.state.nav, collapsed: ctx.state.layout.nav === 'rail' }));
  draw();
  ctx.on('state', (patch) => { if ('nav' in patch || 'layout' in patch) draw(); });

  delegate(region, 'click', (action, id) => {
    if (action !== 'nav') return;
    if (id === 'chat') {
      ctx.set({ nav: 'chat', treeOpen: false });
    } else if (id === 'project') {
      ctx.set({ nav: 'project', treeOpen: ctx.state.layout.tree === 'toggle' ? true : ctx.state.treeOpen });
      ctx.emit('tree:refresh');
    } else if (id === 'tasks' || id === 'diagnostics') {
      if (ctx.state.layout.rail === 'drawer') ctx.set({ railOpen: true });
      ctx.emit('rail:focus', id);
    } else if (id === 'agents') {
      ctx.set({ nav: 'agents' });
      ctx.emit('tab:open', { id: 'agents', kind: 'agents', title: 'Agents' });
    } else if (LATER[id]) {
      ctx.toast(LATER[id] + ' is not available yet.', 'info');
    }
  });
}
