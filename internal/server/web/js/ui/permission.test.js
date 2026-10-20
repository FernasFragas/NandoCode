import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { renderPermissionCard } from './permission.js';

const walk = (t, f) => { if (t && typeof t === 'object') { f(t); t.children.forEach((c) => walk(c, f)); } };
const find = (t, p) => { const r = []; walk(t, (n) => { if (p(n)) r.push(n); }); return r; };
const texts = (t) => { const r = []; const go = (n) => { if (typeof n === 'string') r.push(n); else if (n) n.children.forEach(go); }; go(t); return r.join(' '); };
const action = (t, a) => find(t, (n) => n.attrs['data-action'] === a);
const bad = '"><script>';

const fixture = JSON.parse(readFileSync(new URL('../../dev/fixtures/run-permission.json', import.meta.url), 'utf8'));
const req = fixture.find((e) => e.type === 'permission_request').data;

test('permission card renders from fixture with all three decisions and close', () => {
  const t = renderPermissionCard(req);
  assert.match(texts(t), /Bash/);
  assert.match(texts(t), /printf ok/);
  assert.match(texts(t), /Run project checks/);
  for (const a of ['allow', 'deny', 'always-allow', 'dismiss']) {
    const els = action(t, a);
    assert.equal(els.length, 1, a);
    assert.equal(els[0].attrs['data-id'], 'permission-1');
  }
  assert.match(texts(action(t, 'allow')[0]), /Allow/);
  assert.match(texts(action(t, 'always-allow')[0]), /Always allow/);
});

test('popover vs inline variants', () => {
  assert.match(renderPermissionCard(req).attrs.class, /bv-permission--popover/);
  assert.match(renderPermissionCard(req, { inline: true }).attrs.class, /bv-permission--inline/);
});

test('empty states omit missing target and reason', () => {
  const t = renderPermissionCard({ request_id: 'r', tool_name: 'Bash' });
  assert.equal(find(t, (n) => String(n.attrs.class || '').includes('bv-permission__target')).length, 0);
  assert.equal(find(t, (n) => String(n.attrs.class || '').includes('bv-permission__reason')).length, 0);
  assert.doesNotMatch(texts(renderPermissionCard({})), /undefined/);
});

test('hostile strings stay text children', () => {
  const t = renderPermissionCard({ request_id: bad, tool_name: bad, target: bad, reason: bad });
  assert.equal(find(t, (n) => n.tag === 'script').length, 0);
  assert.ok(texts(t).includes(bad));
  assert.equal(action(t, 'allow')[0].attrs['data-id'], bad);
});
