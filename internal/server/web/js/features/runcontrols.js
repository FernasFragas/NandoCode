import { renderDialog } from '../dialogview.js';

const OWNER = 'clear-session';

function failure(verb, err) {
  if (err && err.status === 409) return 'Cannot ' + verb + ' while a run is active. Stop the run first.';
  return 'Could not ' + verb + ' session: ' + err.message;
}


export function mount(ctx) {
  let stopping = false;
  let operating = false;
  let clearId = null;

  ctx.on('state', (patch) => { if ('running' in patch && !patch.running) stopping = false; });
  ctx.on('sse', (msg) => { if (msg && msg.type === 'run_started') stopping = false; });
  ctx.on('session:reset', () => { stopping = false; });
  ctx.on('runcontrol:stop-run', async () => {
    const id = ctx.state.sessionId;
    if (!id || !ctx.state.running || stopping) return;
    stopping = true;
    try {
      const res = await ctx.api.post('/v1/sessions/' + encodeURIComponent(id) + '/cancel');
      if (res && res.cancelled === false) stopping = false;
    } catch (err) {
      stopping = false;
      ctx.toast('Could not stop run: ' + err.message);
    }
  });

  ctx.on('runcontrol:clear-session', () => {
    if (!ctx.state.sessionId || ctx.state.running || operating) return;
    clearId = ctx.state.sessionId;
    ctx.emit('dialog:open', { owner: OWNER, tree: renderDialog({
      title: 'Clear session',
      description: 'Clear this session’s conversation and start fresh?',
      actions: [{ action: 'cancel', label: 'Cancel' }, { action: 'confirm', label: 'Clear', kind: 'danger' }],
    }) });
  });
  ctx.on('dialog:closed', ({ owner }) => { if (owner === OWNER) clearId = null; });
  ctx.on('dialog:action', async ({ owner, action }) => {
    if (owner !== OWNER) return;
    if (action === 'cancel') { ctx.emit('dialog:close', OWNER); return; }
    if (action !== 'confirm' || !clearId || operating) return;
    const id = clearId;
    ctx.emit('dialog:close', OWNER);
    if (id !== ctx.state.sessionId || ctx.state.running) return;
    operating = true;
    try {
      await ctx.api.post('/v1/sessions/' + encodeURIComponent(id) + '/clear');
      if (id === ctx.state.sessionId) ctx.emit('session:reset');
      ctx.emit('sessions:refresh');
      ctx.toast('Session cleared.', 'info');
    } catch (err) { ctx.toast(failure('clear', err)); }
    finally { operating = false; }
  });

  ctx.on('runcontrol:compact-session', async () => {
    const id = ctx.state.sessionId;
    if (!id || ctx.state.running || operating) return;
    operating = true;
    try {
      const res = await ctx.api.post('/v1/sessions/' + encodeURIComponent(id) + '/compact');
      if (res && res.denied) ctx.toast('Compaction denied by a PreCompact hook.');
      else if (res && res.skipped) ctx.toast('Nothing to compact.', 'info');
      else if (res && Number.isFinite(res.before) && Number.isFinite(res.after)) ctx.toast('Session compacted: ' + res.before + ' to ' + res.after + ' messages.', 'info');
      else ctx.toast('Session compacted.', 'info');
    } catch (err) { ctx.toast(failure('compact', err)); }
    finally { operating = false; }
  });
}
