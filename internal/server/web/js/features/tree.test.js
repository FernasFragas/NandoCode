import { test } from 'node:test';
import assert from 'node:assert/strict';
import { makeCtx, fire, flush } from '../fakedom.js';
import { mount } from './tree.js';

const listings = () => [
  { entries: [{ name: 'cmd', kind: 'dir', size: 0 }, { name: 'README.md', kind: 'file', size: 4 }] },
  { entries: [{ name: 'main.go', kind: 'file', size: 9 }] },
];
// One response per directory request, in breadth-first order.
const filesRoute = () => { const q = listings(); return () => ({ json: q.shift() || { entries: [] } }); };

function setup() {
  const t = makeCtx({ routes: { 'GET /v1/sessions/s1/files': filesRoute() } });
  mount(t.ctx);
  return t;
}
const rows = (t) => t.doc.getElementById('tree').querySelectorAll('li');

test('loads on session:ready and shows the tree, folders first and collapsed', async () => {
  const t = setup();
  t.ctx.state.sessionId = 's1';
  t.ctx.emit('session:ready', 's1');
  await flush();
  const text = t.doc.getElementById('tree').textContent;
  assert.match(text, /cmd/);
  assert.match(text, /README\.md/);
  assert.ok(!text.includes('main.go'), 'collapsed');
  assert.match(t.calls[0].path, /\/files\?path=\./);
  assert.match(t.calls[1].path, /\/files\?path=cmd/);
});

test('clicking a folder row or its chevron expands it', async () => {
  const t = setup();
  t.ctx.state.sessionId = 's1';
  t.ctx.emit('session:ready', 's1');
  await flush();
  fire(t.doc.getElementById('tree').querySelector('.bv-tree__name'), 'click'); // folder name -> row
  assert.match(t.doc.getElementById('tree').textContent, /main\.go/);
  fire(t.doc.getElementById('tree').querySelector('.bv-tree__chevron'), 'click');
  assert.ok(!t.doc.getElementById('tree').textContent.includes('main.go'));
});

test('the @ action inserts a mention instead of toggling', async () => {
  const t = setup();
  t.ctx.state.sessionId = 's1';
  t.ctx.emit('session:ready', 's1');
  await flush();
  const mentions = [];
  t.ctx.on('composer:insert-mention', (p) => mentions.push(p));
  const readme = rows(t).find((r) => r.getAttribute('data-id') === 'README.md');
  fire(readme.querySelector('.bv-tree__mention'), 'click');
  assert.deepEqual(mentions, ['README.md']);
});

test('clicking a file row emits file:open with its path; reset clears the tree', async () => {
  const t = setup();
  t.ctx.state.sessionId = 's1';
  t.ctx.emit('session:ready', 's1');
  await flush();
  const opened = [];
  t.ctx.on('file:open', (p) => opened.push(p));
  const before = t.doc.getElementById('tree').textContent;
  fire(rows(t).find((r) => r.getAttribute('data-id') === 'README.md').querySelector('.bv-tree__name'), 'click');
  assert.deepEqual(opened, ['README.md']);
  assert.equal(t.doc.getElementById('tree').textContent, before);
  t.ctx.emit('session:reset');
  assert.match(t.doc.getElementById('tree').textContent, /No files/);
});

test('a failed load toasts', async () => {
  const t = makeCtx({ routes: { 'GET /v1/sessions/s1/files': () => ({ status: 500, text: 'x' }) } });
  mount(t.ctx);
  t.ctx.state.sessionId = 's1';
  t.ctx.emit('session:ready', 's1');
  await flush();
  assert.match(t.doc.getElementById('toasts').textContent, /Could not load the file tree/);
});
