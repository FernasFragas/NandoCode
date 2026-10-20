import { test } from 'node:test';
import assert from 'node:assert/strict';
import { makeCtx, fire, flush, sse } from '../fakedom.js';
import { mount } from './permission.js';
import { getPendingStore } from '../pendingstore.js';

function setup(width = 1448) {
  const t = makeCtx({ width, routes: { 'POST /v1/sessions/s1/permissions/r1': () => ({ status: 204 }), 'POST /v1/sessions/s1/permissions/r2': () => ({ status: 204 }) } });
  t.ctx.state.sessionId = 's1';
  t.ctx.state.layout = { nav: 'full', tree: 'full', rail: 'full', permission: width < 1200 ? 'inline' : 'popover' };
  mount(t.ctx);
  return t;
}
const region = (t) => t.doc.getElementById('permission');
const request = (id = 'r1') => sse('permission_request', { request_id: id, tool_name: 'run_command', target: 'rm -rf x', reason: 'writes files', classification: 'destructive' });
const button = (t, action) => region(t).querySelectorAll('button').find((b) => b.getAttribute('data-action') === action);

test('a request shows the card with tool, target, reason and classification, focusing Allow', () => {
  const t = setup();
  t.ctx.emit('sse', request());
  assert.equal(region(t).hidden, false);
  const text = region(t).textContent;
  for (const want of ['run_command', 'rm -rf x', 'writes files', 'destructive']) assert.ok(text.includes(want), want);
  assert.ok(region(t).querySelector('.bv-permission--popover'));
  assert.equal(t.doc.activeElement, button(t, 'allow'));
  assert.equal(t.ctx.state.permissionPending, 1);
});

test('narrow layouts use the inline card', () => {
  const t = setup(1000);
  t.ctx.emit('sse', request());
  assert.ok(region(t).querySelector('.bv-permission--inline'));
});

test('Allow, Deny and Always allow post the K2 decisions and close the card', async () => {
  const cases = [['allow', 'allow'], ['deny', 'deny'], ['always-allow', 'always_allow']];
  for (const [action, decision] of cases) {
    const t = setup();
    t.ctx.emit('sse', request());
    fire(button(t, action), 'click');
    await flush();
    const post = t.calls.find((c) => c.path.includes('/permissions/r1'));
    assert.deepEqual(post.body, { decision });
    assert.equal(region(t).hidden, true);
    assert.equal(t.ctx.state.permissionPending, 0);
  }
});

test('dismiss (X) hides the card but leaves the broker request pending and reviewable', async () => {
  const t = setup();
  t.ctx.emit('sse', request());
  fire(button(t, 'dismiss'), 'click');
  await flush();
  assert.equal(t.calls.length, 0);
  assert.equal(region(t).querySelector('.bv-permission'), null);
  assert.equal(t.ctx.state.permissionPending, 1);
  assert.equal(getPendingStore(t.ctx).list('permission').length, 1);
  fire(button(t, 'review'), 'click');
  assert.ok(region(t).querySelector('.bv-permission'));
});

test('Escape hides the current card without deciding and leaves the request pending', async () => {
  const t = setup();
  t.ctx.emit('sse', request());
  const evt = { handled: false };
  t.ctx.emit('escape', evt);
  assert.equal(evt.handled, true);
  await flush();
  assert.equal(t.calls.length, 0);
  assert.equal(region(t).querySelector('.bv-permission'), null);
  assert.equal(t.ctx.state.permissionPending, 1);
  assert.equal(getPendingStore(t.ctx).list('permission')[0].hidden, true);
  const again = { handled: false };
  t.ctx.emit('escape', again);
  assert.equal(again.handled, false, 'the card is already hidden');
  assert.equal(t.calls.length, 0);
  fire(button(t, 'review'), 'click');
  assert.equal(t.doc.activeElement, button(t, 'allow'));
});

test('permission:open focuses Allow for an already visible card and reopens a hidden card', () => {
  const t = setup();
  t.ctx.emit('sse', request());
  button(t, 'deny').focus();
  t.ctx.emit('permission:open', 'r1');
  assert.equal(t.doc.activeElement, button(t, 'allow'));
  fire(button(t, 'dismiss'), 'click');
  assert.equal(getPendingStore(t.ctx).list('permission')[0].hidden, true);
  t.ctx.emit('permission:open', 'r1');
  assert.ok(region(t).querySelector('.bv-permission'));
  assert.equal(t.doc.activeElement, button(t, 'allow'));
  assert.equal(t.ctx.state.permissionPending, 1);
});

test('double clicks send one request', async () => {
  const t = setup();
  t.ctx.emit('sse', request());
  const allow = button(t, 'allow');
  fire(allow, 'click'); fire(allow, 'click');
  await flush();
  assert.equal(t.calls.length, 1);
});

test('permission_resolved (including a timeout) closes the card', () => {
  const t = setup();
  t.ctx.emit('sse', request());
  t.ctx.emit('sse', sse('permission_resolved', { request_id: 'r1', decision: 'timeout' }));
  assert.equal(region(t).hidden, true);
  assert.equal(t.ctx.state.permissionPending, 0);
  assert.deepEqual(getPendingStore(t.ctx).list('permission'), []);
});

test('resolution also removes a dismissed request from the pending store', () => {
  const t = setup();
  t.ctx.emit('sse', request());
  fire(button(t, 'dismiss'), 'click');
  t.ctx.emit('sse', sse('permission_resolved', { request_id: 'r1', decision: 'timeout' }));
  assert.equal(t.ctx.state.permissionPending, 0);
  assert.deepEqual(getPendingStore(t.ctx).list('permission'), []);
  assert.equal(region(t).hidden, true);
});

test('requests queue; the next one shows after the first is answered', async () => {
  const t = setup();
  t.ctx.emit('sse', request('r1'));
  t.ctx.emit('sse', request('r2'));
  t.ctx.emit('sse', request('r2'));
  assert.equal(t.ctx.state.permissionPending, 2);
  fire(button(t, 'allow'), 'click');
  await flush();
  assert.equal(t.ctx.state.permissionPending, 1);
  fire(button(t, 'deny'), 'click');
  await flush();
  assert.equal(t.calls[1].path.includes('/r2'), true);
});

test('terminal or error auto-closes with a notice; a failed decision keeps the card', async () => {
  const t = setup();
  const notices = [];
  t.ctx.on('notice', (n) => notices.push(n.message));
  t.ctx.emit('sse', request());
  t.ctx.emit('sse', sse('terminal'));
  assert.equal(region(t).hidden, true);
  assert.deepEqual(notices, ['Permission request auto-closed.']);

  const f = makeCtx({ routes: { 'POST /v1/sessions/s1/permissions/r1': () => ({ status: 500, text: 'x' }) } });
  f.ctx.state.sessionId = 's1';
  f.ctx.state.layout = { permission: 'popover' };
  mount(f.ctx);
  const fails = [];
  f.ctx.on('notice', (n) => fails.push(n.message));
  f.ctx.emit('sse', request());
  fire(f.doc.getElementById('permission').querySelectorAll('button').find((b) => b.getAttribute('data-action') === 'allow'), 'click');
  await flush();
  assert.match(fails[0], /Permission decision failed/);
  assert.equal(f.doc.getElementById('permission').hidden, false);
});

test('session reset drops pending requests', () => {
  const t = setup();
  t.ctx.emit('sse', request());
  t.ctx.emit('session:reset');
  assert.equal(region(t).hidden, true);
});

test('hostile fields stay text', () => {
  const t = setup();
  t.ctx.emit('sse', sse('permission_request', { request_id: 'r1', tool_name: '<img onerror=1>', target: '"><script>', reason: '' }));
  assert.equal(region(t).querySelector('.bv-permission__tool').textContent, '<img onerror=1>');
  assert.equal(region(t).querySelector('script'), null);
});

test('a replayed permission_request after resolution does not reopen the card', async () => {
  const t = setup();
  t.ctx.emit('sse', request('r1'));
  t.ctx.emit('sse', sse('permission_resolved', { request_id: 'r1', decision: 'timeout' }));
  t.ctx.emit('sse', request('r1'));
  assert.equal(region(t).hidden, true);
  assert.equal(t.ctx.state.permissionPending, 0);
  // decided locally, then replayed after an SSE reconnect
  t.ctx.emit('sse', request('r2'));
  fire(button(t, 'allow'), 'click');
  await flush();
  t.ctx.emit('sse', request('r2'));
  assert.equal(region(t).hidden, true);
  // terminal-cleared requests are not resurrected either
  t.ctx.emit('sse', request('r3'));
  t.ctx.emit('sse', sse('terminal'));
  t.ctx.emit('sse', request('r3'));
  assert.equal(region(t).hidden, true);
  // a new session starts clean
  t.ctx.emit('session:reset');
  t.ctx.emit('sse', request('r1'));
  assert.equal(region(t).hidden, false);
});

test('a second request or layout redraw keeps focus on the same button', () => {
  const t = setup();
  t.ctx.emit('sse', request('r1'));
  button(t, 'deny').focus();
  t.ctx.emit('sse', request('r2'));
  assert.equal(t.doc.activeElement, button(t, 'deny'));
  t.ctx.set({ layout: { nav: 'full', tree: 'full', rail: 'full', permission: 'inline' } });
  assert.equal(t.doc.activeElement, button(t, 'deny'));
});

test('dismiss moves focus to the review button; resolving the last request returns it to the composer', () => {
  const t = setup();
  t.ctx.emit('sse', request('r1'));
  button(t, 'dismiss').focus();
  fire(button(t, 'dismiss'), 'click');
  assert.equal(t.doc.activeElement, button(t, 'review'));
  t.ctx.emit('sse', sse('permission_resolved', { request_id: 'r1', decision: 'timeout' }));
  assert.equal(t.doc.activeElement, t.doc.getElementById('message-input'));
});

test('a 404 from the decision POST resolves the request locally', async () => {
  const t = makeCtx({ routes: { 'POST /v1/sessions/s1/permissions/r1': () => ({ status: 404, text: 'gone' }) } });
  t.ctx.state.sessionId = 's1';
  t.ctx.state.layout = { permission: 'popover' };
  mount(t.ctx);
  t.ctx.emit('sse', request('r1'));
  fire(t.doc.getElementById('permission').querySelectorAll('button').find((b) => b.getAttribute('data-action') === 'allow'), 'click');
  await flush();
  assert.equal(t.doc.getElementById('permission').hidden, true);
  assert.equal(t.ctx.state.permissionPending, 0);
});
