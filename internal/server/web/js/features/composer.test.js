import { test } from 'node:test';
import assert from 'node:assert/strict';
import { makeCtx, fire, flush } from '../fakedom.js';
import { mount, renderMentionList } from './composer.js';

const listings = () => [
  { entries: [{ name: 'cmd', kind: 'dir', size: 0 }, { name: 'README.md', kind: 'file', size: 4 }] },
  { entries: [{ name: 'main.go', kind: 'file', size: 9 }] },
];
// One response per directory request, in breadth-first order.
const filesRoute = () => { const q = listings(); return () => ({ json: q.shift() || { entries: [] } }); };

function setup(routes = {}) {
  const t = makeCtx({ routes: { 'GET /v1/sessions/s1/files': filesRoute(), 'POST /v1/sessions/s1/messages': () => ({ status: 202 }), ...routes } });
  t.ctx.state.sessionId = 's1';
  mount(t.ctx);
  return t;
}
const input = (t) => t.doc.getElementById('message-input');
const type = (t, text) => { input(t).value = text; input(t).selectionStart = text.length; fire(input(t), 'input'); };
const key = (t, k, props = {}) => fire(input(t), 'keydown', { key: k, ...props });
const dropdown = (t) => t.doc.getElementById('mention-dropdown');

test('Enter sends: user bubble event, cleared input, POST with message_id', async () => {
  const t = setup();
  const sent = [];
  t.ctx.on('transcript:user-message', (m) => sent.push(m.text));
  input(t).value = '  hello  ';
  const evt = key(t, 'Enter');
  await flush();
  assert.equal(evt.defaultPrevented, true);
  assert.deepEqual(sent, ['hello']);
  assert.equal(input(t).value, '');
  const post = t.calls.find((c) => c.path === '/v1/sessions/s1/messages');
  assert.equal(post.body.prompt, 'hello');
  assert.match(post.body.message_id, /^msg_\d+_/);
});

test('Shift+Enter does not send; empty text and no session do not send', async () => {
  const t = setup();
  input(t).value = 'a';
  key(t, 'Enter', { shiftKey: true });
  input(t).value = '   ';
  key(t, 'Enter');
  t.ctx.state.sessionId = null;
  input(t).value = 'a';
  fire(t.doc.getElementById('send-btn'), 'click');
  await flush();
  assert.equal(t.calls.length, 0);
});

test('Send button works and input is disabled while running or a permission is pending', () => {
  const t = setup();
  assert.equal(input(t).disabled, false);
  t.ctx.set({ running: true });
  assert.equal(input(t).disabled, true);
  assert.equal(t.doc.getElementById('send-btn').disabled, true);
  t.ctx.set({ running: false });
  assert.equal(input(t).disabled, false);
  assert.equal(t.doc.activeElement, input(t), 'focused after the run ends');
  t.ctx.set({ permissionPending: 1 });
  assert.equal(input(t).disabled, true);
  t.ctx.set({ running: true });
  t.ctx.set({ running: false });
  assert.equal(input(t).disabled, true, 'still waiting on the permission');
  t.ctx.set({ permissionPending: 0 });
  assert.equal(input(t).disabled, false);
});

test('a failed send reports a notice with the status', async () => {
  const t = setup({ 'POST /v1/sessions/s1/messages': () => ({ status: 500, text: 'busy' }) });
  const notices = [];
  t.ctx.on('notice', (n) => notices.push(n.message));
  input(t).value = 'x';
  key(t, 'Enter');
  await flush();
  assert.deepEqual(notices, ['Send failed: busy']);
});

test('requires_credential on send opens the prompt and resends without a second bubble', async () => {
  let n = 0;
  const t = setup({ 'POST /v1/sessions/s1/messages': () => (++n === 1 ? { status: 428, json: { error: 'requires_credential' } } : { status: 202 }) });
  const bubbles = [];
  const asks = [];
  t.ctx.on('transcript:user-message', (m) => bubbles.push(m));
  t.ctx.on('credential:required', (r) => asks.push(r));
  input(t).value = 'hi';
  key(t, 'Enter');
  await flush();
  assert.equal(asks.length, 1);
  asks[0].retry();
  await flush();
  assert.equal(n, 2);
  assert.equal(bubbles.length, 1);
});

test('typing @ lists every entry, then narrows as the query grows', async () => {
  const t = setup();
  type(t, 'look at @');
  await flush();
  assert.equal(dropdown(t).hidden, false);
  assert.equal(dropdown(t).querySelectorAll('.bv-mention__item').length, 3);
  type(t, 'look at @ma');
  assert.equal(dropdown(t).querySelectorAll('.bv-mention__item').length, 1); // only cmd/main.go contains 'ma'
});

test('filtering is case-insensitive; Enter on a match inserts the path', async () => {
  const t = setup();
  type(t, '@read');
  await flush();
  const items = dropdown(t).querySelectorAll('.bv-mention__item');
  assert.equal(items.length, 1);
  assert.match(items[0].textContent, /README\.md/);
  const evt = key(t, 'Enter');
  assert.equal(evt.defaultPrevented, true);
  assert.equal(input(t).value, '@README.md ');
  assert.equal(dropdown(t).hidden, true);
  assert.equal(t.calls.filter((c) => c.path.endsWith('/messages')).length, 0, 'Enter picked, did not send');
});

test('arrow keys cycle the active item; Escape closes', async () => {
  const t = setup();
  type(t, '@');
  await flush();
  const active = () => dropdown(t).querySelectorAll('.bv-mention__item').findIndex((i) => i.classList.contains('bv-mention__item--active'));
  assert.equal(active(), 0);
  key(t, 'ArrowDown');
  assert.equal(active(), 1);
  key(t, 'ArrowUp'); key(t, 'ArrowUp');
  assert.equal(active(), 2);
  key(t, 'Escape');
  assert.equal(dropdown(t).hidden, true);
});

test('a click on an item inserts it; clicking elsewhere closes', async () => {
  const t = setup();
  type(t, 'see @cmd/m');
  await flush();
  fire(dropdown(t).querySelector('.bv-mention__item'), 'click');
  assert.equal(input(t).value, 'see @cmd/main.go ');
  type(t, '@');
  await flush();
  fire(t.doc.getElementById('transcript'), 'click');
  assert.equal(dropdown(t).hidden, true);
});

test('@ inside a word or after a space-containing query does not open it', async () => {
  const t = setup();
  type(t, 'mail@x');
  type(t, '@abc def');
  await flush();
  assert.equal(dropdown(t).hidden, true);
});

test('the tree @ action inserts at the caret with spacing', () => {
  const t = setup();
  input(t).value = 'check';
  input(t).selectionStart = 5;
  t.ctx.emit('composer:insert-mention', 'src/a.go');
  assert.equal(input(t).value, 'check @src/a.go ');
  assert.equal(t.doc.activeElement, input(t));
});

test('the document Escape chain closes an open dropdown', async () => {
  const t = setup();
  type(t, '@');
  await flush();
  const evt = { handled: false };
  t.ctx.emit('escape', evt);
  assert.equal(evt.handled, true);
  assert.equal(dropdown(t).hidden, true);
});

test('session:reset clears the input', () => {
  const t = setup();
  input(t).value = 'draft';
  t.ctx.emit('session:reset');
  assert.equal(input(t).value, '');
});

test('renderMentionList keeps paths as text', () => {
  const tree = renderMentionList([{ path: '"><script>', kind: 'file' }], 0);
  assert.equal(tree.children[0].children[1].children[0], '"><script>');
});
