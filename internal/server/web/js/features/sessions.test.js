import { test } from 'node:test';
import assert from 'node:assert/strict';
import { makeCtx, fire, flush } from '../fakedom.js';
import { mount } from './sessions.js';

test('lists the current session and New session starts another', async () => {
  const t = makeCtx({ routes: { 'GET /v1/sessions': () => ({ json: { sessions: [] } }) } });
  mount(t.ctx);
  assert.match(t.doc.getElementById('sessions').textContent, /No sessions yet/);
  t.ctx.set({ sessionId: 'sess_9' });
  const root = t.doc.getElementById('sessions');
  assert.match(root.textContent, /sess_9/);
  assert.equal(root.querySelectorAll('li').length, 1);
  let fresh = 0;
  t.ctx.on('session:new', () => { fresh += 1; });
  fire(root.querySelector('.bv-sessions__new'), 'click');
  await flush();
  assert.equal(fresh, 1);
});

test('clicking inside the row (title) is harmless', () => {
  const t = makeCtx();
  mount(t.ctx);
  t.ctx.set({ sessionId: 's' });
  let fresh = 0;
  t.ctx.on('session:new', () => { fresh += 1; });
  fire(t.doc.getElementById('sessions').querySelector('.bv-sessions__title'), 'click');
  assert.equal(fresh, 0);
});
