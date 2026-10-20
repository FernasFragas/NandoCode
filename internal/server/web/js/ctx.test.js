import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createContext } from './ctx.js';
import { pageWith } from './fakedom.js';
import { h } from './dom.js';

function make(extra = {}) {
  const doc = pageWith(['toasts', 'region']);
  const timers = [];
  const ctx = createContext({ doc, win: {}, fetchImpl: async () => ({}), storage: null, setTimer: (fn) => timers.push(fn), ...extra });
  return { ctx, doc, timers };
}

test('bus: on/emit/off', () => {
  const { ctx } = make();
  const seen = [];
  const off = ctx.on('x', (v) => seen.push(v));
  ctx.emit('x', 1);
  off();
  ctx.emit('x', 2);
  assert.deepEqual(seen, [1]);
});

test('set merges into state and emits the patch', () => {
  const { ctx } = make();
  const patches = [];
  ctx.on('state', (p) => patches.push(p));
  ctx.set({ running: true });
  assert.equal(ctx.state.running, true);
  assert.deepEqual(patches, [{ running: true }]);
});

test('render mounts a tree into a region by id', () => {
  const { ctx, doc } = make();
  ctx.render('region', h('p', { class: 'a' }, 'hi'));
  assert.equal(doc.getElementById('region').textContent, 'hi');
  ctx.render('region', h('p', {}, 'again'));
  assert.equal(doc.getElementById('region').children.length, 1);
});

test('toast appends a status node and removes it later', () => {
  const { ctx, doc, timers } = make();
  ctx.toast('boom');
  const host = doc.getElementById('toasts');
  assert.equal(host.children.length, 1);
  assert.equal(host.children[0].getAttribute('role'), 'status');
  timers[0]();
  assert.equal(host.children.length, 0);
});

test('setToken stores the token and notifies; api uses it', async () => {
  const stored = {};
  const calls = [];
  const { ctx } = make({
    storage: { setItem: (k, v) => { stored[k] = v; }, getItem: () => null },
    fetchImpl: async (path, init) => { calls.push(init.headers); return { ok: true, status: 200, text: async () => '{}' }; },
  });
  const seen = [];
  ctx.on('token', (t) => seen.push(t));
  ctx.setToken('  abc ');
  assert.deepEqual(seen, ['abc']);
  assert.ok(Object.values(stored).includes('abc'));
  await ctx.api.get('/v1/x');
  assert.equal(calls[0].Authorization, 'Bearer abc');
});
