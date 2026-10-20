import { test } from 'node:test';
import assert from 'node:assert/strict';
import { makeCtx, fire } from '../fakedom.js';
import { mount, visibility } from './shell.js';
import { mount as mountNav } from './nav.js';

const attr = (t, k) => t.doc.getElementById('app').getAttribute(k);

test('1448 wide: full nav, tree visible on Project, rail visible', () => {
  const t = makeCtx({ width: 1448 });
  mount(t.ctx);
  assert.equal(attr(t, 'data-nav'), 'full');
  assert.equal(attr(t, 'data-tree-visible'), 'true');
  assert.equal(attr(t, 'data-rail-visible'), 'true');
});

test('1280 wide: icon rail nav, no overlays', () => {
  const t = makeCtx({ width: 1280 });
  mount(t.ctx);
  assert.equal(attr(t, 'data-nav'), 'rail');
  assert.equal(attr(t, 'data-tree'), 'full');
  assert.equal(attr(t, 'data-rail'), 'full');
});

test('900 wide: tree and rail are toggles that open from the buttons', () => {
  const t = makeCtx({ width: 900 });
  mount(t.ctx);
  assert.equal(attr(t, 'data-tree-visible'), 'false');
  assert.equal(attr(t, 'data-rail-visible'), 'false');
  const toggles = t.doc.getElementById('panel-toggles');
  const tree = t.doc.createElement('button'); tree.setAttribute('data-action', 'toggle-tree'); toggles.appendChild(tree);
  const rail = t.doc.createElement('button'); rail.setAttribute('data-action', 'toggle-rail'); toggles.appendChild(rail);
  fire(tree, 'click');
  fire(rail, 'click');
  assert.equal(attr(t, 'data-tree-visible'), 'true');
  assert.equal(attr(t, 'data-rail-visible'), 'true');
  fire(tree, 'click');
  assert.equal(attr(t, 'data-tree-visible'), 'false');
});

test('resize re-measures and closes overlays when widening', () => {
  const t = makeCtx({ width: 900 });
  mount(t.ctx);
  t.ctx.set({ treeOpen: true, railOpen: true });
  t.win.innerWidth = 1500;
  t.win.listeners.resize.forEach((fn) => fn());
  assert.equal(attr(t, 'data-nav'), 'full');
  assert.equal(t.ctx.state.treeOpen, false);
  assert.equal(t.ctx.state.railOpen, false);
  assert.equal(t.ctx.state.layout.permission, 'popover');
});

test('Chat hides the tree, Project shows it (wide layout)', () => {
  const t = makeCtx({ width: 1448 });
  mount(t.ctx);
  mountNav(t.ctx);
  const item = (id) => t.doc.getElementById('nav').querySelectorAll('button').find((b) => b.getAttribute('data-id') === id);
  fire(item('chat'), 'click');
  assert.equal(attr(t, 'data-tree-visible'), 'false');
  assert.equal(attr(t, 'data-nav-active'), 'chat');
  fire(item('project'), 'click');
  assert.equal(attr(t, 'data-tree-visible'), 'true');
});

test('visibility is pure', () => {
  assert.deepEqual(visibility({ nav: 'chat', treeOpen: false, railOpen: false, layout: { tree: 'full', rail: 'full' } }), { tree: false, rail: true });
  assert.deepEqual(visibility({ nav: 'chat', treeOpen: true, railOpen: true, layout: { tree: 'toggle', rail: 'drawer' } }), { tree: true, rail: true });
});
