import { test } from 'node:test';
import assert from 'node:assert/strict';

import { renderTabStrip, renderTab } from './tabs.js';

const walk = (t, f) => { if (t && typeof t === 'object') { f(t); t.children.forEach((c) => walk(c, f)); } };
const find = (t, p) => { const r = []; walk(t, (n) => { if (p(n)) r.push(n); }); return r; };
const texts = (t) => { const r = []; const go = (n) => { if (typeof n === 'string') r.push(n); else if (n) n.children.forEach(go); }; go(t); return r.join(' '); };
const HOSTILE = '"><script>';
const noScript = (t) => assert.equal(find(t, (n) => n.tag === 'script').length, 0);

test('tab strip renders tabs, one active, close and add actions', () => {
  const t = renderTabStrip({ tabs: [{ id: 'a', title: 'handler.go', icon: 'file-code', closable: true }, { id: 'agents', title: 'Agents', icon: 'agents' }], active: 'a' });
  const tabs = find(t, (n) => n.attrs['data-action'] === 'tab');
  assert.deepEqual(tabs.map((n) => n.attrs['data-id']), ['a', 'agents']);
  assert.equal(tabs.filter((n) => n.attrs['aria-selected'] === 'true').length, 1);
  assert.equal(find(t, (n) => n.attrs['data-action'] === 'tab-close').length, 1);
  assert.equal(find(t, (n) => n.attrs['data-action'] === 'tab-new').length, 1);
});

test('empty state: no tabs renders only the add button, none selected', () => {
  for (const d of [undefined, {}, { tabs: [] }]) {
    const t = renderTabStrip(d);
    assert.equal(find(t, (n) => n.attrs['data-action'] === 'tab').length, 0);
    assert.equal(find(t, (n) => n.attrs['data-action'] === 'tab-new').length, 1);
  }
});

test('hostile title and id stay text / inert', () => {
  const t = renderTab({ id: HOSTILE, title: HOSTILE }, { active: false });
  noScript(t);
  assert.ok(texts(t).includes(HOSTILE));
  assert.ok(!/script/.test(t.attrs.class));
});
