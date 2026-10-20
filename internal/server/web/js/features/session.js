// Session lifecycle: create a session, open its event stream, fan events out on
// the bus ('sse' for typed events), and start over on 'session:new'.
import { createEventStream } from '../sse.js';
import { resetTree } from '../treedata.js';

export function mount(ctx) {
  let stream = null;
  let epoch = 0;

  function parse(evt) {
    let msg;
    try {
      msg = JSON.parse(evt.data);
    } catch {
      return null;
    }
    if (!msg || typeof msg !== 'object') return null;
    if (!msg.type && evt.event) msg.type = evt.event;
    if (!msg.id && evt.id) msg.id = evt.id;
    return msg;
  }

  function onEvent(evt) {
    if (evt.event === 'heartbeat') {
      ctx.emit('sse:heartbeat', { time: ctx.now() });
      return;
    }
    const msg = parse(evt);
    if (!msg) return;
    if (msg.type === 'run_started') ctx.set({ running: true });
    else if (msg.type === 'terminal') ctx.set({ running: false });
    ctx.emit('sse', msg);
  }

  function open(id) {
    stream = createEventStream({
      url: '/v1/sessions/' + encodeURIComponent(id) + '/events',
      getToken: ctx.getToken,
      fetchImpl: ctx.fetch,
      setTimer: ctx.setTimer,
      onEvent,
      onStatus(status) {
        ctx.set({ connection: status });
        if (status === 'unauthorized') ctx.emit('auth:required');
      },
    });
    stream.start();
  }

  async function create() {
    const mine = ++epoch;
    ctx.set({ connection: 'connecting' });
    try {
      const data = await ctx.api.post('/v1/sessions?reuse=blank', {}) // 200 reuses a blank orphan, 201 creates; both are success;
      if (mine !== epoch) return;
      const id = data && (data.id || data.session_id);
      if (!id) throw new Error('No session ID returned');
      ctx.set({ sessionId: id, running: false, workspaceRoot: (data && data.workspace_root) || '' });
      ctx.emit('session:ready', id);
      open(id);
    } catch (err) {
      if (mine !== epoch) return;
      ctx.set({ connection: 'disconnected' });
      ctx.emit('notice', { type: 'error', message: 'Failed to create session: ' + err.message });
      if (err && err.status === 401) ctx.emit('auth:required');
    }
  }

  function reset() {
    epoch += 1;
    if (stream) { stream.stop(); stream = null; }
    resetTree(ctx);
    ctx.emit('session:reset');
    ctx.set({ sessionId: null, running: false, model: '', models: [], workspaceRoot: '', connection: 'connecting' });
  }

  ctx.on('session:new', () => { reset(); create(); });
  ctx.on('session:select', async (id) => {
    if (!id || id === ctx.state.sessionId) return;
    const mine = ++epoch;
    if (stream) { stream.stop(); stream = null; }
    resetTree(ctx);
    ctx.emit('session:reset');
    ctx.set({ sessionId: null, running: false, workspaceRoot: '', connection: 'connecting' });
    try {
      const view = await ctx.api.get('/v1/sessions/' + encodeURIComponent(id));
      if (mine !== epoch) return;
      ctx.set({ sessionId: id, running: !!view.running, model: view.model || '', workspaceRoot: view.workspace_root || '' });
      ctx.emit('session:ready', id);
      open(id);
    } catch (err) {
      if (mine !== epoch) return;
      ctx.set({ connection: 'disconnected' });
      ctx.toast('Could not open session: ' + err.message);
    }
  });

  // A pasted token retries whatever failed with 401.
  ctx.on('token', () => {
    if (!ctx.state.sessionId) create();
    else if (ctx.state.connection === 'unauthorized' && stream) stream.start();
  });

  create();
}
