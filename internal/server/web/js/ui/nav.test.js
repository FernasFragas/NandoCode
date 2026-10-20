import { test } from 'node:test';
import assert from 'node:assert/strict';
import { renderNav, NAV_ITEMS } from './nav.js';

const walk = (t, f) => { if (t && typeof t === 'object') { f(t); t.children.forEach((c) => walk(c, f)); } };
const find = (t, p) => { const r = []; walk(t, (n) => { if (p(n)) r.push(n); }); return r; };
const texts = (t) => { const r = []; const go = (n) => { if (typeof n === 'string') r.push(n); else if (n) n.children.forEach(go); }; go(t); return r.join(' '); };

test('nav lists the six spec items in order, including Agents', () => {
  assert.deepEqual(NAV_ITEMS.map((i) => i.id), ['chat', 'project', 'agents', 'transcript', 'tasks', 'diagnostics']);
  const t = renderNav({ active: 'project' });
  const items = find(t, (n) => n.attrs['data-action'] === 'nav');
  assert.deepEqual(items.map((n) => n.attrs['data-id']), NAV_ITEMS.map((i) => i.id));
  assert.equal(items.filter((n) => n.attrs['aria-current'] === 'page').length, 1);
  assert.equal(items.find((n) => n.attrs['data-id'] === 'project').attrs['aria-current'], 'page');
  assert.match(texts(t), /Diagnostics/);
});

test('empty state: no active item marks none current; collapsed is an icon rail', () => {
  const t = renderNav({ collapsed: true });
  assert.match(t.attrs.class, /bv-nav--collapsed/);
  assert.equal(find(t, (n) => n.attrs['aria-current']).length, 0);
  // labels stay available to assistive tech as title attributes
  assert.ok(find(t, (n) => n.attrs['data-action'] === 'nav').every((n) => n.attrs.title));
});

test('unknown active id from hostile input is inert text-free', () => {
  const t = renderNav({ active: '"><script>' });
  assert.equal(find(t, (n) => n.tag === 'script').length, 0);
  assert.equal(find(t, (n) => n.attrs['aria-current']).length, 0);
});
