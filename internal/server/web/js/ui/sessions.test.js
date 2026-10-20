import { test } from 'node:test';
import assert from 'node:assert/strict';
import { renderSessionList, renderSessionRow } from './sessions.js';

const walk = (t, f) => { if (t && typeof t === 'object') { f(t); t.children.forEach((c) => walk(c, f)); } };
const find = (t, p) => { const r = []; walk(t, (n) => { if (p(n)) r.push(n); }); return r; };
const texts = (t) => { const r = []; const go = (n) => { if (typeof n === 'string') r.push(n); else if (n) n.children.forEach(go); }; go(t); return r.join(' '); };
const action = (t, a) => find(t, (n) => n.attrs['data-action'] === a);

const now = Date.parse('2026-01-02T03:10:00Z');
const s1 = { id: 'abc123', title: 'Fix login', created_at: '2026-01-02T01:00:00Z', last_active: '2026-01-02T03:05:00Z', running: true, model: 'llama3' };
const s2 = { id: 'def456', title: '', created_at: '2026-01-01T01:00:00Z', last_active: '2026-01-01T03:00:00Z', running: false, model: 'llama3' };

test('session list renders rows from K3-shaped data with the only New session button', () => {
  const t = renderSessionList({ sessions: [s1, s2], activeId: 'abc123' }, { now });
  assert.equal(action(t, 'new-session').length, 1);
  const rows = action(t, 'select-session');
  assert.deepEqual(rows.map((r) => r.attrs['data-id']), ['abc123', 'def456']);
  assert.match(rows[0].attrs.class, /bv-sessions__row--active/);
  assert.equal(rows[0].attrs['aria-current'], 'true');
  assert.doesNotMatch(rows[1].attrs.class, /--active/);
  assert.match(texts(t), /Fix login/);
  assert.match(texts(t), /5m ago/);
  assert.match(texts(t), /Untitled/);
  assert.equal(action(t, 'delete-session').length, 2);
});

test('empty state', () => {
  const t = renderSessionList({ sessions: [] });
  assert.match(texts(t), /No sessions yet/);
  assert.equal(action(t, 'new-session').length, 1);
  assert.match(texts(renderSessionList({})), /No sessions yet/);
});

test('row tolerates a missing time', () => {
  const t = renderSessionRow({ id: 'x' }, {});
  assert.match(texts(t), /Untitled/);
});

test('hostile strings stay text children', () => {
  const bad = '"><script>';
  const t = renderSessionList({ sessions: [{ id: bad, title: bad, model: bad, last_active: bad }], activeId: bad }, { now });
  assert.equal(find(t, (n) => n.tag === 'script').length, 0);
  assert.ok(texts(t).includes(bad));
  assert.equal(action(t, 'select-session')[0].attrs['data-id'], bad);
});
