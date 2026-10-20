import { test } from 'node:test';
import assert from 'node:assert/strict';
import { makeCtx, flush } from '../fakedom.js';
import { mount } from './session.js';

const enc = new TextEncoder();
const body = (chunks) => {
  let i = 0;
  return { getReader: () => ({ read: async () => (i < chunks.length ? { done: false, value: enc.encode(chunks[i++]) } : await new Promise(() => {})), cancel: async () => {} }) };
};
const frame = (id, type, data = {}) => `id: ${id}\nevent: ${type}\ndata: ${JSON.stringify({ id, type, time: '2026-01-01T00:00:00Z', data })}\n\n`;

function setup(extra = {}) {
  const routes = {
    'POST /v1/sessions': () => ({ json: { session_id: 'sess_1' } }),
    'GET /v1/sessions/sess_1/events': () => ({ stream: body([frame('1', 'run_started'), frame('2', 'assistant_text_delta', { content: 'hi' }), 'id: hb-1\nevent: heartbeat\ndata: {"type":"heartbeat"}\n\n', frame('3', 'terminal', {})]) }),
    ...extra,
  };
  return makeCtx({ routes, token: 'tok' });
}

test('creates a session, announces it and streams events with run state', async () => {
  const t = setup();
  const seen = [];
  const ready = [];
  const states = [];
  t.ctx.on('sse', (m) => seen.push(m.type));
  t.ctx.on('session:ready', (id) => ready.push(id));
  t.ctx.on('state', (p) => { if ('running' in p) states.push(p.running); });
  let beats = 0;
  t.ctx.on('sse:heartbeat', () => { beats += 1; });
  mount(t.ctx);
  await flush(); await flush(); await flush();
  assert.deepEqual(ready, ['sess_1']);
  assert.equal(t.ctx.state.sessionId, 'sess_1');
  assert.deepEqual(seen, ['run_started', 'assistant_text_delta', 'terminal']);
  assert.deepEqual(states.filter((v, i, a) => i === 0 || v !== a[i - 1]), [false, true, false]);
  assert.equal(t.ctx.state.running, false);
  assert.equal(beats, 1);
  assert.equal(t.ctx.state.connection, 'connected');
  assert.equal(t.calls[0].headers.Authorization, 'Bearer tok');
});

test('a failed create reports a notice and 401 asks for the token', async () => {
  const t = setup({ 'POST /v1/sessions': () => ({ status: 401, text: 'unauthorized' }) });
  const notices = [];
  let auth = 0;
  t.ctx.on('notice', (n) => notices.push(n.message));
  t.ctx.on('auth:required', () => { auth += 1; });
  mount(t.ctx);
  await flush(); await flush();
  assert.match(notices[0], /Failed to create session/);
  assert.equal(auth, 1);
  assert.equal(t.ctx.state.connection, 'disconnected');
});

test('a pasted token retries session creation', async () => {
  let n = 0;
  const t = setup({ 'POST /v1/sessions': () => (++n === 1 ? { status: 401, text: 'unauthorized' } : { json: { id: 'sess_1' } }) });
  mount(t.ctx);
  await flush(); await flush();
  t.ctx.setToken('good');
  await flush(); await flush();
  assert.equal(t.ctx.state.sessionId, 'sess_1');
});

test('session:new resets state, tells features, and creates another session', async () => {
  let n = 0;
  const t = setup({ 'POST /v1/sessions': () => ({ json: { session_id: 'sess_' + (++n) } }), 'GET /v1/sessions/sess_1/events': () => ({ stream: body([]) }), 'GET /v1/sessions/sess_2/events': () => ({ stream: body([]) }) });
  const events = [];
  t.ctx.on('session:reset', () => events.push('reset'));
  t.ctx.on('session:ready', (id) => events.push(id));
  mount(t.ctx);
  await flush(); await flush();
  t.ctx.emit('session:new');
  await flush(); await flush();
  assert.deepEqual(events, ['sess_1', 'reset', 'sess_2']);
  assert.equal(t.ctx.state.sessionId, 'sess_2');
});

test('session creation asks the server to reuse a blank orphan (?reuse=blank)', async () => {
  const t = setup({ 'POST /v1/sessions': () => ({ status: 200, json: { session_id: 'sess_1' } }) });
  mount(t.ctx);
  await flush(); await flush();
  const create = t.calls.find((c) => c.method === 'POST' && c.path.startsWith('/v1/sessions'));
  assert.equal(create.path, '/v1/sessions?reuse=blank');
  assert.equal(t.ctx.state.sessionId, 'sess_1', '200 (reused) is success');
  const t2 = setup({ 'POST /v1/sessions': () => ({ status: 201, json: { session_id: 'sess_1' } }) });
  mount(t2.ctx);
  await flush(); await flush();
  assert.equal(t2.ctx.state.sessionId, 'sess_1', '201 (created) is success');
});

test('create and select publish the session workspace root as ctx.state.workspaceRoot', async () => {
  const t = setup({
    'POST /v1/sessions': () => ({ json: { session_id: 'sess_1', workspace_root: '/work/a' } }),
    'GET /v1/sessions/sess_2': () => ({ json: { session_id: 'sess_2', workspace_root: '/work/b' } }),
    'GET /v1/sessions/sess_2/events': () => ({ stream: body([]) }),
  });
  mount(t.ctx);
  await flush(); await flush();
  assert.equal(t.ctx.state.workspaceRoot, '/work/a');
  t.ctx.emit('session:select', 'sess_2');
  await flush(); await flush();
  assert.equal(t.ctx.state.workspaceRoot, '/work/b');
  t.ctx.emit('session:new');
  assert.equal(t.ctx.state.workspaceRoot, '', 'reset clears the root');
});
