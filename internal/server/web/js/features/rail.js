// Right rail: live session state, usage, tasks, diagnostics and recent events.
import { h } from '../dom.js';
import { delegate } from '../delegate.js';
import { createEventStore, eventRow } from '../eventstore.js';
import { payloadOf } from './transcript.js';
import { renderStatusRow, renderUsage, renderTodoList, renderEventRow, _clock } from '../ui/rail.js';

export { eventRow };

function section(id, title, ...body) {
  return h('section', { id: 'rail-' + id, class: 'bv-rail__section', tabindex: '-1' },
    h('h2', { class: 'bv-rail__title' }, title), ...body);
}

const CONN = { connected: ['Connected', 'success'], connecting: ['Connecting', 'warning'], disconnected: ['Disconnected', 'danger'], unauthorized: ['Unauthorized', 'danger'] };

export function indexLabel(status) {
  if (!status) return 'Unavailable';
  if (status.running) return 'Indexing…';
  if (!status.exists) return 'Not built';
  if (!status.compatible) return 'Incompatible';
  return `${status.files || 0} files · ${status.records || 0} records`;
}

export function mount(ctx) {
  const region = ctx.el('rail');
  if (!region) return;
  let usage = null;
  let todos = [];
  let tasks = new Map();
  const events = createEventStore();
  let pending = new Set();
  let heartbeat = null;
  let health = null;
  let index = null;
  let scheduled = false;
  let indexEpoch = 0;

  function draw() {
    scheduled = false;
    const s = ctx.state;
    const conn = CONN[s.connection] || CONN.disconnected;
    const pendingCount = Number.isInteger(s.permissionPending) ? s.permissionPending : pending.size;
    const hb = heartbeat === null ? null : _clock(new Date(heartbeat).toISOString());
    const hbAge = heartbeat === null ? null : `${Math.max(0, Math.floor((ctx.now() - heartbeat) / 1000))}s ago`;
    ctx.render('rail', h('div', { class: 'bv-rail' },
      section('status', 'Session status',
        renderStatusRow({ label: 'Session', value: s.sessionId }),
        renderStatusRow({ label: 'Model', value: s.model }),
        renderStatusRow({ label: 'Run', value: s.running ? 'Running' : 'Idle', dot: s.running ? 'info' : 'muted' }),
        renderStatusRow({ label: 'Last event', value: events.lastEvent() }),
        h('button', { type: 'button', class: 'bv-rail__permission-link', 'data-action': 'focus-permission', disabled: pendingCount === 0 },
          renderStatusRow({ label: 'Permissions pending', value: String(pendingCount), dot: pendingCount ? 'warning' : null }))),
      section('usage', 'Usage', renderUsage(usage),
        usage && usage.Turns > 0 ? renderStatusRow({ label: 'Done reason', value: s.running ? 'running' : usage.DoneReason }) : null),
      section('tasks', 'Tasks', renderTodoList({ todos, tasks: [...tasks.values()] })),
      section('diagnostics', 'Diagnostics',
        renderStatusRow({ label: 'Server stream', value: conn[0], dot: conn[1] }),
        renderStatusRow({ label: 'Ollama', value: health && health.ollama ? health.ollama : (health && health.status === 'unreachable' ? 'unreachable' : null), dot: health && health.ollama === 'reachable' ? 'success' : (health ? 'danger' : null) }),
        renderStatusRow({ label: 'Last heartbeat', value: hb }),
        renderStatusRow({ label: 'SSE age', value: hbAge }),
        renderStatusRow({ label: 'Index', value: indexLabel(index), dot: index && index.exists && index.compatible ? 'success' : 'muted' })),
      section('events', 'Recent events',
        events.list().length ? events.list().map(renderEventRow) : h('p', { class: 'bv-rail__empty' }, 'No events yet'))));
  }

  function schedule() {
    if (scheduled) return;
    scheduled = true;
    ctx.frame(draw);
  }

  async function loadIndex() {
    const id = ctx.state.sessionId;
    const epoch = ++indexEpoch;
    if (!id) { index = null; schedule(); return; }
    try {
      const value = await ctx.api.get('/v1/sessions/' + encodeURIComponent(id) + '/index/status');
      if (epoch !== indexEpoch || ctx.state.sessionId !== id) return;
      index = value && typeof value === 'object' ? value : null;
    } catch {
      if (epoch !== indexEpoch || ctx.state.sessionId !== id) return;
      index = null;
    }
    schedule();
  }

  ctx.on('sse', (msg) => {
    const p = payloadOf(msg);
    if ((msg.type === 'terminal' || msg.type === 'usage_update') && p.usage) usage = p.usage;
    else if (msg.type === 'session_cleared') todos = [];
    else if (msg.type === 'todo_updated') todos = Array.isArray(p.todos) ? p.todos : [];
    else if (msg.type === 'task_lifecycle' && p.task_id) tasks.set(p.task_id, { ...tasks.get(p.task_id), ...p });
    if (msg.type === 'permission_request' && p.request_id) pending.add(p.request_id);
    else if (msg.type === 'permission_resolved' && p.request_id) pending.delete(p.request_id);
    else if (msg.type === 'terminal' || msg.type === 'error') pending.clear();
    if (msg.type === 'index_started' || msg.type === 'index_done' || msg.type === 'index_error') loadIndex();
    events.add(msg);
    schedule();
  });
  ctx.on('sse:heartbeat', ({ time }) => { heartbeat = time; schedule(); });
  ctx.on('health', (hl) => { health = hl; schedule(); });
  ctx.on('state', (patch) => {
    if ('sessionId' in patch) loadIndex();
    if ('connection' in patch && patch.connection === 'connected') loadIndex();
    if (['sessionId', 'model', 'running', 'connection', 'permissionPending'].some((k) => k in patch)) schedule();
  });
  ctx.on('session:reset', () => {
    usage = null; todos = []; tasks = new Map(); events.reset(); pending = new Set(); heartbeat = null; index = null; indexEpoch++;
    schedule();
  });
  ctx.on('rail:focus', (name) => {
    const el = ctx.el('rail-' + name);
    if (!el) return;
    if (typeof el.scrollIntoView === 'function') el.scrollIntoView();
    el.focus();
  });
  delegate(region, 'click', (action) => {
    if (action === 'focus-permission') ctx.emit('permission:open');
  });

  draw();
  if (ctx.state.sessionId) loadIndex();
}
