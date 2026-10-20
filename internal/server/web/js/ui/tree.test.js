import { test } from 'node:test';
import assert from 'node:assert/strict';
import { renderTree, renderTreeRow } from './tree.js';

const walk = (t, f) => { if (t && typeof t === 'object') { f(t); t.children.forEach((c) => walk(c, f)); } };
const find = (t, p) => { const r = []; walk(t, (n) => { if (p(n)) r.push(n); }); return r; };
const texts = (t) => { const r = []; const go = (n) => { if (typeof n === 'string') r.push(n); else if (n) n.children.forEach(go); }; go(t); return r.join(' '); };
const action = (t, a) => find(t, (n) => n.attrs['data-action'] === a);

// K3 GET .../files?path= entries, with children filled in once loaded.
const entries = [
  { name: 'internal', kind: 'dir', children: [{ name: 'main.go', kind: 'file', size: 10 }] },
  { name: 'README.md', kind: 'file', size: 5 },
  { name: 'empty', kind: 'dir', children: [] },
  { name: 'lazy', kind: 'dir' },
];

test('tree renders rows with paths, chevrons and @ actions', () => {
  const t = renderTree({ entries, expanded: ['internal'] });
  const rows = action(t, 'tree-select');
  assert.deepEqual(rows.map((r) => r.attrs['data-id']), ['internal', 'internal/main.go', 'README.md', 'empty', 'lazy']);
  assert.equal(action(t, 'tree-toggle').length, 3);
  assert.equal(action(t, 'mention').length, 5);
  assert.equal(action(t, 'mention')[1].attrs['data-id'], 'internal/main.go');
  assert.equal(rows[0].attrs['aria-expanded'], 'true');
  assert.equal(rows[3].attrs['aria-expanded'], 'false');
});

test('collapsed dirs hide children; expanded unloaded dir shows loading, empty dir says so', () => {
  const t = renderTree({ entries, expanded: [] });
  assert.equal(action(t, 'tree-select').length, 4);
  const t2 = renderTree({ entries, expanded: ['empty', 'lazy'] });
  assert.match(texts(t2), /Empty folder/);
  assert.match(texts(t2), /Loading/);
});

test('empty state: no entries', () => {
  assert.match(texts(renderTree({ entries: [] })), /No files/);
  assert.match(texts(renderTree({})), /No files/);
});

test('depth sets indentation via a data attribute, not inline style', () => {
  const row = renderTreeRow({ name: 'a', kind: 'file' }, { depth: 2, path: 'x/a' });
  assert.equal(row.attrs['data-depth'], 2);
  assert.equal(row.attrs.style, undefined);
});

test('hostile names stay text children', () => {
  const bad = '"><script>';
  const t = renderTree({ entries: [{ name: bad, kind: 'dir', children: [{ name: bad, kind: 'file' }] }], expanded: [bad] });
  assert.equal(find(t, (n) => n.tag === 'script').length, 0);
  assert.ok(texts(t).includes(bad));
  assert.equal(action(t, 'tree-select')[0].attrs['data-id'], bad);
});
