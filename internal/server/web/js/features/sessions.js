// Session list backed by GET /v1/sessions.
import { h, icon } from '../dom.js';
import { renderSessionList } from '../ui/sessions.js';
import { delegate } from '../delegate.js';

export function mount(ctx) {
  const region = ctx.el('sessions');
  if (!region) return;
  let sessions = [];
  let revision = 0;
  let busy = false;

  function draw() {
    ctx.render('sessions', h('div', { class: 'bv-sessions-panel' },
      renderSessionList({ sessions, activeId: ctx.state.sessionId }, { now: ctx.now() }),
      h('div', { class: 'bv-sessions__actions' },
        h('button', { type: 'button', 'data-action': 'clear-session', disabled: !ctx.state.sessionId || ctx.state.running }, icon('trash'), 'Clear'),
        h('button', { type: 'button', 'data-action': 'compact-session', disabled: !ctx.state.sessionId || ctx.state.running }, icon('compact'), 'Compact'))));
  }

  async function refresh() {
    const mine = ++revision;
    try {
      const data = await ctx.api.get('/v1/sessions');
      if (mine !== revision) return;
      sessions = Array.isArray(data?.sessions) ? data.sessions : [];
      draw();
    } catch (err) {
      if (mine === revision) ctx.toast('Failed to load sessions: ' + err.message);
    }
  }

  async function create() {
    if (busy) return;
    busy = true;
    try {
      await refresh();
      if (sessions.length >= 10) {
        const idle = sessions.filter((s) => !s.running).sort((a, b) => Date.parse(a.created_at) - Date.parse(b.created_at))[0];
        if (!idle) { ctx.toast('All sessions are running. Stop one before creating another.'); return; }
        const res = await ctx.api.request('DELETE', '/v1/sessions/' + encodeURIComponent(idle.id));
        if (!res.ok) throw new Error('HTTP ' + res.status);
        sessions = sessions.filter((s) => s.id !== idle.id);
        ctx.toast('Evicted oldest idle session.', 'info');
      }
      ctx.emit('session:new');
    } catch (err) { ctx.toast('Could not create session: ' + err.message); }
    finally { busy = false; }
  }

  async function remove(id) {
    if (busy) return;
    busy = true;
    try {
      const res = await ctx.api.request('DELETE', '/v1/sessions/' + encodeURIComponent(id));
      if (!res.ok) throw new Error('HTTP ' + res.status);
      sessions = sessions.filter((s) => s.id !== id);
      draw();
      if (id === ctx.state.sessionId) {
        const next = sessions[0];
        ctx.emit(next ? 'session:select' : 'session:new', next?.id);
      }
    } catch (err) { ctx.toast('Could not delete session: ' + err.message); }
    finally { busy = false; }
  }

  draw();
  ctx.on('session:ready', refresh);
  ctx.on('state', (patch) => {
    if ('sessionId' in patch && patch.sessionId && !sessions.some((s) => s.id === patch.sessionId)) {
      sessions.unshift({ id: patch.sessionId, title: 'Current session', created_at: new Date(ctx.now()).toISOString(), last_active: new Date(ctx.now()).toISOString(), running: ctx.state.running });
    }
    if ('sessionId' in patch || 'running' in patch) draw();
  });
  ctx.on('sse', (msg) => {
    if (msg.type === 'session_titled') {
      const active = sessions.find((s) => s.id === ctx.state.sessionId);
      if (active) { active.title = msg.data?.title || active.title; draw(); }
    } else if (msg.type === 'run_started' || msg.type === 'terminal') {
      const active = sessions.find((s) => s.id === ctx.state.sessionId);
      if (active) { active.running = msg.type === 'run_started'; draw(); }
    }
  });
  ctx.on('sessions:new', create);
  ctx.on('sessions:refresh', refresh);
  delegate(region, 'click', (action, id) => {
    if (action === 'new-session') create();
    else if (action === 'select-session' && id && id !== ctx.state.sessionId) ctx.emit('session:select', id);
    else if (action === 'delete-session' && id) remove(id);
    else if (action === 'clear-session' || action === 'compact-session') ctx.emit('runcontrol:' + action);
  });
}
