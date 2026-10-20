import { h, mount as mountTree } from '../dom.js';
import { delegate } from '../delegate.js';
import { createAgentsLogStore } from '../agentslog.js';
import { renderAgentsView, renderLogRow } from '../ui/agents.js';

const TAB_KIND = 'agents';
const LOG_KIND = 'agents-log';

export function mount(ctx) {
  // Views are registered with V6c's tab registry (ctx.registerTabKind); tabs.js
  // owns visibility of each host section. Without it there is nothing to show.
  if (typeof ctx.registerTabKind !== 'function') return;
  const store = createAgentsLogStore();
  let selected = '';
  let epoch = 0;
  const views = new Map(); // kind -> { host, visible }
  let scheduled = false;

  function drawOne(kind, v) {
    if (!v.visible) return;
    const workers = store.workers();
    const log = store.list(selected);
    const logsOnly = kind === LOG_KIND;
    const tree = h('div', { class: 'bv-agents-tab__body' },
      h('header', { class: 'bv-agents-tab__header' },
        h('h2', null, logsOnly ? 'Sub-agent events' : 'Agents in this session'),
        !logsOnly && ctx.state.running ? h('span', { class: 'bv-agents-tab__running' }, 'Running') : null,
        !logsOnly ? h('button', { type: 'button', 'data-action': 'view-logs' }, 'View logs') : null,
        ctx.state.running ? h('button', { type: 'button', 'data-action': 'stop-all' }, 'Stop all') : null),
      h('div', { class: 'bv-agents-tab__filters', role: 'group', 'aria-label': 'Filter activity log' },
        h('button', { type: 'button', 'data-action': 'filter-all', 'aria-pressed': selected === '' ? 'true' : 'false' }, 'All agents'),
        workers.map((w) => h('button', { type: 'button', 'data-action': 'filter-agent', 'data-id': w.task_id, 'aria-pressed': selected === w.task_id ? 'true' : 'false' }, w.name || w.task_id))),
      logsOnly ? h('div', { class: 'bv-agents-tab__logs' }, log.length ? log.map(renderLogRow) : h('p', null, 'No sub-agent events yet')) : renderAgentsView({ workers, log }));
    v.host.replaceChildren(mountTree(tree, ctx.doc));
  }
  function draw() { scheduled = false; for (const [kind, v] of views) drawOne(kind, v); }
  function schedule() {
    if (scheduled) return;
    scheduled = true;
    ctx.frame(draw);
  }

  async function load(id) {
    const mine = ++epoch;
    if (!id) return;
    try {
      const view = await ctx.api.get('/v1/sessions/' + encodeURIComponent(id));
      if (mine === epoch && ctx.state.sessionId === id) {
        store.setWorkers(view && view.workers);
        schedule();
      }
    } catch { /* SSE still supplies live workers. */ }
  }

  for (const kind of [TAB_KIND, LOG_KIND]) {
    ctx.registerTabKind(kind, ({ host }) => {
      const v = { host, visible: false };
      views.set(kind, v);
      delegate(host, 'click', (action, id) => {
        if (action === 'select-worker' || action === 'filter-agent') { selected = id || ''; schedule(); }
        else if (action === 'filter-all') { selected = ''; schedule(); }
        else if (action === 'view-logs') ctx.emit('tab:open', { id: LOG_KIND, kind: LOG_KIND, title: 'Agent logs' });
        else if (action === 'stop-all') ctx.emit('runcontrol:stop-run');
      });
      return { show() { v.visible = true; schedule(); }, hide() { v.visible = false; } };
    });
  }
  ctx.on('session:ready', load);
  ctx.on('session:reset', () => { epoch++; store.reset(); selected = ''; schedule(); });
  ctx.on('state', (patch) => { if ('running' in patch) schedule(); });
  ctx.on('sse', (msg) => { if (store.add(msg)) schedule(); });
}
