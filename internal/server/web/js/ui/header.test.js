import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { renderHeader } from './header.js';

const walk = (t, f) => { if (t && typeof t === 'object') { f(t); t.children.forEach((c) => walk(c, f)); } };
const find = (t, p) => { const r = []; walk(t, (n) => { if (p(n)) r.push(n); }); return r; };
const texts = (t) => { const r = []; const go = (n) => { if (typeof n === 'string') r.push(n); else if (n) n.children.forEach(go); }; go(t); return r.join(' '); };
const action = (t, a) => find(t, (n) => n.attrs['data-action'] === a);

const fixture = JSON.parse(readFileSync(new URL('../../dev/fixtures/run-basic.json', import.meta.url), 'utf8'));

test('header renders from the session_ready fixture', () => {
  const ready = fixture.find((e) => e.type === 'session_ready');
  const t = renderHeader({ sessionId: ready.session_id, model: 'llama3', models: ['llama3', 'qwen'], connection: 'connected', running: true });
  assert.equal(t.tag, 'header');
  assert.match(t.attrs.class, /bv-header/);
  assert.match(texts(t), /gallery-basic/);
  assert.match(texts(t), /Connected/);
  assert.equal(action(t, 'copy-session')[0].attrs['data-id'], 'gallery-basic');
  assert.equal(find(t, (n) => n.tag === 'option').length, 2);
  assert.equal(action(t, 'stop-run')[0].attrs.disabled, false);
});

test('stop run is disabled when idle and connection states map to modifiers', () => {
  const t = renderHeader({ sessionId: 's', connection: 'disconnected', running: false });
  assert.equal(action(t, 'stop-run')[0].attrs.disabled, true);
  assert.match(texts(t), /Disconnected/);
  assert.ok(find(t, (n) => /bv-header__conn--disconnected/.test(n.attrs.class || '')).length);
});

test('empty state: no session shows a placeholder and no copy button', () => {
  const t = renderHeader({});
  assert.match(texts(t), /No session/);
  assert.equal(action(t, 'copy-session').length, 0);
  assert.equal(find(t, (n) => n.tag === 'select').length, 0);
});

test('hostile strings stay text children or attribute values', () => {
  const bad = '"><script>';
  const t = renderHeader({ sessionId: bad, model: bad, models: [bad], connection: bad });
  assert.equal(find(t, (n) => n.tag === 'script').length, 0);
  assert.ok(texts(t).includes(bad));
  assert.equal(action(t, 'copy-session')[0].attrs['data-id'], bad);
});
