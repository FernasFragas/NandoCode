import { test } from 'node:test';
import assert from 'node:assert/strict';
import { makeCtx, flush } from '../fakedom.js';
import { mount, renderFooter } from './footer.js';

test('shows connection, URL and the build', () => {
  const t = makeCtx();
  mount(t.ctx);
  const text = () => t.doc.getElementById('footer').textContent;
  assert.match(text(), /Server: Connecting/);
  assert.match(text(), /test\.invalid:1/);
  t.ctx.set({ connection: 'disconnected' });
  assert.match(text(), /Server: Disconnected/);
});

test('connected polls /v1/health; a version in the reply is shown', async () => {
  const t = makeCtx({ routes: { 'GET /v1/health': () => ({ json: { status: 'ok', ollama: 'reachable', version: 'v9.9' } }) } });
  mount(t.ctx);
  const health = [];
  t.ctx.on('health', (h) => health.push(h));
  t.ctx.set({ connection: 'connected' });
  await flush();
  assert.equal(health[0].ollama, 'reachable');
  assert.match(t.doc.getElementById('footer').textContent, /nandocodego v9\.9/);
});

test('unreachable health still emits', async () => {
  const t = makeCtx();
  mount(t.ctx);
  const health = [];
  t.ctx.on('health', (h) => health.push(h));
  t.ctx.set({ connection: 'connected' });
  await flush();
  assert.equal(health[0].status, 'unreachable');
});

test('renderFooter is pure and hostile-safe', () => {
  const tree = renderFooter({ connection: '"><script>', url: '<b>', version: '<i>' });
  assert.match(tree.children[0].attrs.class, /--disconnected/);
});
