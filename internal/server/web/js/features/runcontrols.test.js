import { test } from 'node:test';
import assert from 'node:assert/strict';
import { makeCtx, fire, flush } from '../fakedom.js';
import { mount as mountDialog } from './dialog.js';
import { mount as mountRunControls } from './runcontrols.js';
import { mount as mountHeader } from './header.js';
import { mount as mountSessions } from './sessions.js';

test('Stop sends cancel immediately for the running session and ignores repeated clicks', async () => {
  const t = makeCtx({ routes: { 'POST /v1/sessions/s1/cancel': () => ({ status: 202, json: {} }) } });
  mountHeader(t.ctx); mountRunControls(t.ctx);
  t.ctx.set({ sessionId: 's1', running: true });
  const stop = t.doc.getElementById('header').querySelector('[data-action="stop-run"]');
  assert.equal(stop.disabled, false);
  fire(stop, 'click'); fire(stop, 'click');
  assert.equal(t.calls.filter((c) => c.path === '/v1/sessions/s1/cancel').length, 1);
  await flush();
  assert.equal(t.ctx.state.running, true, 'terminal owns the final running state');
});

test('Clear requires confirmation and clears the visible transcript; Compact does not prompt', async () => {
  const t = makeCtx({ routes: {
    'GET /v1/sessions': () => ({ json: { sessions: [{ id: 's1', title: 'One' }] } }),
    'POST /v1/sessions/s1/clear': () => ({ json: { ok: true } }),
    'POST /v1/sessions/s1/compact': () => ({ json: { ok: true } }),
  } });
  mountDialog(t.ctx); mountSessions(t.ctx); mountRunControls(t.ctx);
  t.ctx.set({ sessionId: 's1' }); t.ctx.emit('session:ready', 's1');
  await flush();
  let resets = 0;
  t.ctx.on('session:reset', () => { resets += 1; });
  const sidebar = t.doc.getElementById('sessions');
  fire(sidebar.querySelector('[data-action="clear-session"]'), 'click');
  assert.equal(t.calls.filter((c) => c.path.endsWith('/clear')).length, 0);
  assert.match(t.doc.getElementById('dialog').textContent, /Clear session/);
  fire(t.doc.getElementById('dialog').querySelector('[data-action="cancel"]'), 'click');
  assert.equal(t.calls.filter((c) => c.path.endsWith('/clear')).length, 0);
  fire(sidebar.querySelector('[data-action="clear-session"]'), 'click');
  fire(t.doc.getElementById('dialog').querySelector('[data-action="confirm"]'), 'click');
  await flush();
  assert.equal(t.calls.filter((c) => c.path.endsWith('/clear')).length, 1);
  assert.equal(resets, 1);
  fire(sidebar.querySelector('[data-action="compact-session"]'), 'click');
  await flush();
  assert.equal(t.calls.filter((c) => c.path.endsWith('/compact')).length, 1);
  assert.equal(t.doc.getElementById('dialog').textContent, '');
  assert.match(t.doc.getElementById('toasts').textContent, /compact/i);
});

function compactCtx(result) {
  const t = makeCtx({ routes: {
    'GET /v1/sessions': () => ({ json: { sessions: [{ id: 's1', title: 'One' }] } }),
    'POST /v1/sessions/s1/compact': () => result,
  } });
  mountDialog(t.ctx); mountSessions(t.ctx); mountRunControls(t.ctx);
  t.ctx.set({ sessionId: 's1' });
  return t;
}
async function clickCompact(t) {
  fire(t.doc.getElementById('sessions').querySelector('[data-action="compact-session"]'), 'click');
  await flush();
  return t.doc.getElementById('toasts').textContent;
}

test('Compact reports a PreCompact denial instead of success', async () => {
  const text = await clickCompact(compactCtx({ json: { ok: true, before: 10, after: 10, skipped: false, denied: true } }));
  assert.match(text, /denied/i);
  assert.doesNotMatch(text, /compacted\./i);
});

test('Compact reports skipped and before/after sizes', async () => {
  assert.match(await clickCompact(compactCtx({ json: { ok: true, before: 5, after: 5, skipped: true, denied: false } })), /nothing to compact|skipped/i);
  assert.match(await clickCompact(compactCtx({ json: { ok: true, before: 40, after: 12, skipped: false, denied: false } })), /40.*12/);
});

test('Compact and Clear conflicts (409) say a run is active', async () => {
  assert.match(await clickCompact(compactCtx({ status: 409, text: 'busy' })), /run is active|running/i);
});

function stopCtx(responses) {
  const t = makeCtx({ routes: { 'POST /v1/sessions/s1/cancel': () => responses.shift() } });
  mountHeader(t.ctx); mountRunControls(t.ctx);
  t.ctx.set({ sessionId: 's1', running: true });
  const stop = t.doc.getElementById('header').querySelector('[data-action="stop-run"]');
  const cancels = () => t.calls.filter((c) => c.path === '/v1/sessions/s1/cancel').length;
  return { t, stop, cancels };
}

test('Stop is usable again after {"cancelled":false}', async () => {
  const { stop, cancels } = stopCtx([{ status: 202, json: { cancelled: false } }, { status: 202, json: { cancelled: true } }]);
  fire(stop, 'click'); await flush();
  fire(stop, 'click'); await flush();
  assert.equal(cancels(), 2);
});

test('Stop is usable again after a non-2xx response', async () => {
  const { stop, cancels } = stopCtx([{ status: 500, text: 'boom' }, { status: 202, json: { cancelled: true } }]);
  fire(stop, 'click'); await flush();
  fire(stop, 'click'); await flush();
  assert.equal(cancels(), 2);
});

test('Stop is usable again when a new run starts', async () => {
  const { t, stop, cancels } = stopCtx([{ status: 202, json: { cancelled: true } }, { status: 202, json: { cancelled: true } }]);
  fire(stop, 'click'); await flush();
  t.ctx.emit('sse', { type: 'run_started', data: {} });
  fire(stop, 'click'); await flush();
  assert.equal(cancels(), 2);
});

test('Stop is usable again after a session switch', async () => {
  const { t, stop, cancels } = stopCtx([{ status: 202, json: { cancelled: true } }, { status: 202, json: { cancelled: true } }]);
  fire(stop, 'click'); await flush();
  t.ctx.emit('session:reset');
  fire(stop, 'click'); await flush();
  assert.equal(cancels(), 2);
});
