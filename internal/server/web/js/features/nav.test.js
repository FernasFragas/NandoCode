import { test } from 'node:test';
import assert from 'node:assert/strict';
import { makeCtx, fire } from '../fakedom.js';
import { mount } from './nav.js';

const btn = (t, id) => t.doc.getElementById('nav').querySelectorAll('button').find((b) => b.getAttribute('data-id') === id);

test('renders six items, collapsed to icons below 1440', () => {
  const t = makeCtx({ width: 1280 });
  t.ctx.state.layout = { nav: 'rail', tree: 'full', rail: 'full', permission: 'popover' };
  mount(t.ctx);
  assert.equal(t.doc.getElementById('nav').querySelectorAll('button').length, 6);
  assert.ok(t.doc.getElementById('nav').querySelector('.bv-nav--collapsed'));
  assert.equal(btn(t, 'chat').getAttribute('title'), 'Chat', 'tooltip');
});

test('Project refreshes the tree and opens it in toggle mode', () => {
  const t = makeCtx();
  t.ctx.state.layout = { nav: 'rail', tree: 'toggle', rail: 'full', permission: 'inline' };
  mount(t.ctx);
  let refreshed = 0;
  t.ctx.on('tree:refresh', () => { refreshed += 1; });
  fire(btn(t, 'project'), 'click');
  assert.equal(t.ctx.state.treeOpen, true);
  assert.equal(refreshed, 1);
});

test('Tasks and Diagnostics focus the rail, opening the drawer when needed', () => {
  const t = makeCtx();
  t.ctx.state.layout = { nav: 'rail', tree: 'toggle', rail: 'drawer', permission: 'inline' };
  mount(t.ctx);
  const focus = [];
  t.ctx.on('rail:focus', (n) => focus.push(n));
  fire(btn(t, 'tasks'), 'click');
  fire(btn(t, 'diagnostics'), 'click');
  assert.deepEqual(focus, ['tasks', 'diagnostics']);
  assert.equal(t.ctx.state.railOpen, true);
});

test('Agents nav opens and selects the shared agents tab', () => {
  const t = makeCtx();
  mount(t.ctx);
  const calls = [];
  t.ctx.on('tab:open', (p) => calls.push(p));
  fire(btn(t, 'agents'), 'click');
  assert.deepEqual(calls, [{ id: 'agents', kind: 'agents', title: 'Agents' }]);
  assert.equal(t.ctx.state.nav, 'agents');
});
