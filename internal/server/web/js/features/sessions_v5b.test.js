import { test } from 'node:test';
import assert from 'node:assert/strict';
import { makeCtx, fire, flush } from '../fakedom.js';
import { mount as mountSessions } from './sessions.js';
import { mount as mountSession } from './session.js';

test('loads the server list, updates a title, and selects another session with a new SSE subscription', async () => {
  const t = makeCtx({ routes: {
    'POST /v1/sessions': () => ({ json: { id: 's1' } }),
    'GET /v1/sessions': () => ({ json: { sessions: [
      { id: 's2', title: 'Other', model: 'm2', running: false },
      { id: 's1', title: 'First', model: 'm1', running: false },
    ] } }),
    'GET /v1/sessions/s1/events': () => ({ stream: { getReader: () => ({ read: () => new Promise(() => {}), cancel: async () => {} }) } }),
    'GET /v1/sessions/s2/events': () => ({ stream: { getReader: () => ({ read: () => new Promise(() => {}), cancel: async () => {} }) } }),
    'GET /v1/sessions/s2': () => ({ json: { id: 's2', running: false, model: 'm2' } }),
  } });
  mountSessions(t.ctx); mountSession(t.ctx);
  await flush(); await flush(); await flush();
  const sidebar = t.doc.getElementById('sessions');
  assert.equal(sidebar.querySelectorAll('li').length, 2);
  t.ctx.emit('sse', { type: 'session_titled', data: { title: 'Renamed' } });
  assert.match(sidebar.textContent, /Renamed/);
  fire(sidebar.querySelector('[data-id="s2"] .bv-sessions__title') || sidebar.querySelectorAll('.bv-sessions__row')[0], 'click');
  await flush(); await flush();
  assert.equal(t.ctx.state.sessionId, 's2');
  assert.equal(t.ctx.state.model, 'm2');
  assert.ok(t.calls.some((c) => c.path === '/v1/sessions/s2/events'));
});

test('New session at ten evicts the oldest idle session and preserves a running one', async () => {
  const rows = Array.from({ length: 10 }, (_, i) => ({ id: 's' + i, title: 'S' + i, running: i === 9, created_at: new Date(1000 + i * 1000).toISOString() })).reverse();
  const t = makeCtx({ routes: {
    'GET /v1/sessions': () => ({ json: { sessions: rows } }),
    'DELETE /v1/sessions/s0': () => ({ status: 204 }),
  } });
  mountSessions(t.ctx);
  t.ctx.set({ sessionId: 's9' }); t.ctx.emit('session:ready', 's9');
  await flush();
  fire(t.doc.getElementById('sessions').querySelector('[data-action="new-session"]'), 'click');
  await flush();
  assert.ok(t.calls.some((c) => c.method === 'DELETE' && c.path === '/v1/sessions/s0'));
  assert.ok(!t.calls.some((c) => c.path === '/v1/sessions/s9' && c.method === 'DELETE'));
  assert.match(t.doc.getElementById('toasts').textContent, /evict/i);
});

function deleteCtx(extra = {}) {
  const rows = [
    { id: 's3', title: 'Three', running: false },
    { id: 's2', title: 'Two', running: false },
    { id: 's1', title: 'One', running: false },
  ];
  const t = makeCtx({ routes: {
    'GET /v1/sessions': () => ({ json: { sessions: rows } }),
    'DELETE /v1/sessions/s3': () => ({ status: 204 }),
    'DELETE /v1/sessions/s2': () => ({ status: 204 }),
    'DELETE /v1/sessions/s1': () => ({ status: 204 }),
    ...extra,
  } });
  mountSessions(t.ctx);
  return t;
}
const del = (t, id) => t.doc.getElementById('sessions').querySelector('[data-action="delete-session"][data-id="' + id + '"]');
const ids = (t) => t.doc.getElementById('sessions').querySelectorAll('li').map((li) => li.getAttribute('data-id'));

test('Deleting the current session switches to the newest remaining session', async () => {
  const t = deleteCtx();
  t.ctx.set({ sessionId: 's1' }); t.ctx.emit('session:ready', 's1'); await flush();
  const picked = [];
  t.ctx.on('session:select', (id) => picked.push(id));
  fire(del(t, 's1'), 'click'); await flush();
  assert.deepEqual(picked, ['s3']);
  assert.ok(t.calls.some((c) => c.method === 'DELETE' && c.path === '/v1/sessions/s1'));
});

test('Deleting the last session creates a new one', async () => {
  const t = makeCtx({ routes: {
    'GET /v1/sessions': () => ({ json: { sessions: [{ id: 's1', title: 'One' }] } }),
    'DELETE /v1/sessions/s1': () => ({ status: 204 }),
  } });
  mountSessions(t.ctx);
  t.ctx.set({ sessionId: 's1' }); t.ctx.emit('session:ready', 's1'); await flush();
  let fresh = 0; const picked = [];
  t.ctx.on('session:new', () => { fresh += 1; });
  t.ctx.on('session:select', (id) => picked.push(id));
  fire(del(t, 's1'), 'click'); await flush();
  assert.equal(fresh, 1);
  assert.deepEqual(picked, []);
});

test('Deleting a non-current session updates the list and keeps the current one', async () => {
  const t = deleteCtx();
  t.ctx.set({ sessionId: 's3' }); t.ctx.emit('session:ready', 's3'); await flush();
  let events = 0;
  t.ctx.on('session:select', () => { events += 1; });
  t.ctx.on('session:new', () => { events += 1; });
  fire(del(t, 's1'), 'click'); await flush();
  assert.deepEqual(ids(t), ['s3', 's2']);
  assert.equal(t.ctx.state.sessionId, 's3');
  assert.equal(events, 0);
});

test('Clicking the nested delete button deletes and does not select the row', async () => {
  const t = deleteCtx();
  t.ctx.set({ sessionId: 's3' }); t.ctx.emit('session:ready', 's3'); await flush();
  let selected = 0;
  t.ctx.on('session:select', () => { selected += 1; });
  fire(del(t, 's2'), 'click'); await flush();
  assert.ok(t.calls.some((c) => c.method === 'DELETE' && c.path === '/v1/sessions/s2'));
  assert.equal(selected, 0);
});

test('session.js emits session:reset when selecting or starting a session (transcript clears on it)', async () => {
  const t = makeCtx({ routes: {
    'POST /v1/sessions': () => ({ json: { id: 's1' } }),
    'GET /v1/sessions/s1/events': () => ({ stream: { getReader: () => ({ read: () => new Promise(() => {}), cancel: async () => {} }) } }),
    'GET /v1/sessions/s2/events': () => ({ stream: { getReader: () => ({ read: () => new Promise(() => {}), cancel: async () => {} }) } }),
    'GET /v1/sessions/s2': () => ({ json: { id: 's2', running: false } }),
  } });
  mountSession(t.ctx);
  await flush(); await flush();
  let resets = 0;
  t.ctx.on('session:reset', () => { resets += 1; });
  t.ctx.emit('session:select', 's2'); await flush(); await flush();
  assert.equal(resets, 1);
  t.ctx.emit('session:new'); await flush(); await flush();
  assert.equal(resets, 2);
});
