import { test } from 'node:test';
import assert from 'node:assert/strict';
import { makeCtx, fire } from '../fakedom.js';
import { mount, clampSplitHeight } from './split.js';

function setup(opts = {}) {
  const t = makeCtx();
  const center = t.ctx.el('composer').parentNode;
  center.clientHeight = opts.height || 600;
  const values = [];
  center.style = { setProperty: (_n, v) => values.push(v) };
  if (opts.store) t.ctx.win.localStorage = opts.store;
  mount(t.ctx);
  return { t, center, values };
}

test('pane is collapsed at 0 tabs and shown by tabs:count; split owns no tab state', () => {
  const { t } = setup();
  const pane = t.ctx.el('tab-pane');
  assert.equal(pane.hidden, true);
  assert.ok(t.ctx.el('tab-content'));
  t.ctx.emit('tabs:count', { count: 2 });
  assert.equal(pane.hidden, false);
  t.ctx.emit('tabs:count', { count: 0 });
  assert.equal(pane.hidden, true);
  t.ctx.emit('tabs:count', undefined);
  t.ctx.emit('tabs:count', null);
  t.ctx.emit('tabs:count', { count: 'x' });
  assert.equal(pane.hidden, true);
  // tab:open and session:reset are V6c's, not ours.
  t.ctx.emit('tab:open', { id: 'a', title: 'a' });
  assert.equal(pane.hidden, true);
  t.ctx.emit('tabs:count', { count: 1 });
  t.ctx.emit('session:reset', {});
  assert.equal(pane.hidden, false);
});

test('split handle resizes with pointer and keyboard within useful limits', () => {
  const { t, values } = setup();
  t.ctx.emit('tabs:count', { count: 1 });
  const handle = t.ctx.el('split-handle');
  fire(handle, 'pointerdown', { clientY: 400 });
  fire(handle, 'pointermove', { clientY: 350 });
  fire(handle, 'pointerup');
  assert.equal(values.at(-1), '350px');
  fire(handle, 'keydown', { key: 'ArrowDown' });
  assert.equal(values.at(-1), '330px');
  assert.equal(clampSplitHeight(900, 600), 420);
});

test('handle exposes aria-valuemin/max and re-clamps on window resize', () => {
  const { t, center, values } = setup();
  t.ctx.emit('tabs:count', { count: 1 });
  const handle = t.ctx.el('split-handle');
  assert.equal(handle.getAttribute('aria-valuemin'), '100');
  assert.equal(handle.getAttribute('aria-valuemax'), '420');
  fire(handle, 'pointerdown', { clientY: 400 });
  fire(handle, 'pointermove', { clientY: 0 });
  fire(handle, 'pointerup');
  assert.equal(values.at(-1), '420px');
  center.clientHeight = 400;
  t.win.listeners.resize.forEach((fn) => fn());
  assert.equal(values.at(-1), '220px');
  assert.equal(handle.getAttribute('aria-valuemax'), '220');
});

test('split height persists in localStorage and storage failures are tolerated', () => {
  const store = {};
  const { t } = setup({ store: { getItem: (k) => store[k] ?? null, setItem: (k, v) => { store[k] = v; } } });
  t.ctx.emit('tabs:count', { count: 1 });
  const handle = t.ctx.el('split-handle');
  fire(handle, 'pointerdown', { clientY: 400 });
  fire(handle, 'pointermove', { clientY: 350 });
  fire(handle, 'pointerup');
  assert.equal(store['nandocode.split.height'], '350');
  const bad = setup({ store: { getItem: () => { throw new Error('blocked'); }, setItem: () => { throw new Error('blocked'); } } });
  bad.t.ctx.emit('tabs:count', { count: 1 });
  fire(bad.t.ctx.el('split-handle'), 'keydown', { key: 'ArrowUp' });
});
