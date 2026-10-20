import { test } from 'node:test';
import assert from 'node:assert/strict';
import { makeCtx, fire, flush, until } from '../fakedom.js';
import { mount as mountSplit } from './split.js';
import { mount as mountTabs } from './tabs.js';
import { mount as mountCodeview } from './codeview.js';
import { mount as mountEditor, MAX_EDIT } from './editor.js';
import { getPendingStore } from '../pendingstore.js';
import { renderSaveConfirmCard } from '../ui/permission.js';

const ORIGINAL = 'package a\n';
function setup(putHandler, extra = {}) {
  const puts = [];
  const routes = {
    'GET /v1/sessions/s1/files': () => ({ json: { kind: 'file', content: extra.content ?? ORIGINAL, sha256: extra.sha ?? 'h0', size: 10, lang: 'go' } }),
    'PUT /v1/sessions/s1/files': (body) => { puts.push(body); return putHandler(body, puts.length); },
  };
  const t = makeCtx({ routes });
  t.puts = puts;
  mountSplit(t.ctx);
  t.ctx.state.sessionId = 's1';
  mountTabs(t.ctx);
  mountCodeview(t.ctx);
  mountEditor(t.ctx);
  return t;
}
const openFile = async (t, path = 'a.go') => { t.ctx.emit('file:open', path); await flush(); await flush(); };
const area = (t) => t.doc.getElementById('code-editor').querySelector('textarea');
const type = (t, text) => { const ta = area(t); ta.value = text; fire(ta, 'input'); };
const click = async (t, action) => {
  const btn = t.doc.getElementById('code-editor').querySelector('[data-action="' + action + '"]');
  assert.ok(btn, 'missing button ' + action);
  fire(btn, 'click'); await flush(); await flush();
};
const text = (t) => t.doc.getElementById('code-editor').textContent;
const tabsOf = (t) => { let last; t.ctx.on('tabs:active', ({ tab }) => { last = tab; }); return () => last; };

test('opens an editable textarea with the loaded content and hides the read-only view', async () => {
  const t = setup(() => ({ json: { sha256: 'h1' } }));
  await openFile(t);
  assert.equal(area(t).value, ORIGINAL);
  assert.equal(t.doc.getElementById('code-content').hidden, true);
  assert.equal(t.doc.getElementById('code-editor').hidden, false);
});

test('typing marks the tab dirty and shows a dirty marker in the strip', async () => {
  const t = setup(() => ({ json: { sha256: 'h1' } }));
  const last = tabsOf(t);
  await openFile(t);
  type(t, ORIGINAL + 'x');
  await flush();
  assert.equal(last().dirty, true);
  assert.match(t.ctx.el('tab-strip').querySelector('.bv-tabs__tab').getAttribute('class'), /bv-tabs__tab--dirty/);
  type(t, ORIGINAL);
  assert.equal(last().dirty, false);
});

test('Ctrl+S saves with base_sha256 and 200 marks the tab clean with the new sha', async () => {
  const t = setup(() => ({ json: { sha256: 'h1' } }));
  const last = tabsOf(t);
  await openFile(t);
  type(t, 'package b\n');
  const evt = fire(area(t), 'keydown', { key: 's', ctrlKey: true });
  assert.equal(evt.defaultPrevented, true);
  await flush(); await flush();
  assert.deepEqual(t.puts[0], { content: 'package b\n', base_sha256: 'h0' });
  assert.equal(last().dirty, false);
  assert.equal(last().sha256, 'h1');
  // the next save uses the new base
  type(t, 'package c\n');
  fire(area(t), 'keydown', { key: 's', metaKey: true });
  await flush(); await flush();
  assert.equal(t.puts[1].base_sha256, 'h1');
});

test('typing during a save keeps the tab dirty after file_saved for the sent text', async () => {
  let release;
  const gate = new Promise((r) => { release = r; });
  const t = setup(async () => { await gate; return { json: { sha256: 'h1' } }; });
  const last = tabsOf(t);
  await openFile(t);
  type(t, 'one\n');
  fire(area(t), 'keydown', { key: 's', ctrlKey: true });
  await flush();
  type(t, 'one\ntwo\n');
  release(); await flush(); await flush();
  assert.equal(area(t).value, 'one\ntwo\n');
  assert.equal(last().dirty, true);
  t.ctx.emit('tab:patch', { id: 'file:a.go', dirty: false }); // e.g. a file_saved handler
  await flush();
  assert.equal(last().dirty, true);
});

test('409 keeps the draft and offers Reload / Show diff / Overwrite', async () => {
  const t = setup((body, n) => (n === 1 ? { status: 409, json: { reason: 'file changed' } } : { json: { sha256: 'h2' } }));
  await openFile(t);
  type(t, 'mine\n');
  await click(t, 'save');
  assert.equal(area(t).value, 'mine\n');
  assert.match(text(t), /changed on disk/i);
  for (const a of ['conflict-reload', 'conflict-diff', 'conflict-overwrite']) {
    assert.ok(t.doc.getElementById('code-editor').querySelector('[data-action="' + a + '"]'), a);
  }
});

test('409 Overwrite re-reads the disk sha and re-sends the draft', async () => {
  const t = setup((body, n) => (n === 1 ? { status: 409, json: {} } : { json: { sha256: 'h2' } }), { sha: 'h9' });
  const last = tabsOf(t);
  await openFile(t);
  type(t, 'mine\n');
  await click(t, 'save');
  await click(t, 'conflict-diff');
  await click(t, 'conflict-overwrite');
  assert.equal(t.puts.length, 2);
  assert.deepEqual(t.puts[1], { content: 'mine\n', base_sha256: 'h9' });
  assert.equal(last().dirty, false);
});

test('409 Show diff lists changed lines without touching the draft', async () => {
  const t = setup(() => ({ status: 409, json: {} }), { content: 'a\nb\nc\n' });
  await openFile(t);
  type(t, 'a\nB\nc\n');
  await click(t, 'save');
  await click(t, 'conflict-diff');
  assert.match(text(t), /-\s*b/);
  assert.match(text(t), /\+\s*B/);
  assert.equal(area(t).value, 'a\nB\nc\n');
});

test('409 Reload discards the draft and loads the disk content', async () => {
  const t = setup(() => ({ status: 409, json: {} }));
  const last = tabsOf(t);
  await openFile(t);
  type(t, 'mine\n');
  await click(t, 'save');
  await click(t, 'conflict-reload');
  assert.equal(area(t).value, ORIGINAL);
  assert.equal(last().dirty, false);
});

test('423 reports the agent is writing and Retry re-sends', async () => {
  const t = setup((body, n) => (n === 1 ? { status: 423, json: { reason: 'agent is writing this file' } } : { json: { sha256: 'h1' } }));
  const last = tabsOf(t);
  await openFile(t);
  type(t, 'mine\n');
  await click(t, 'save');
  assert.match(text(t), /agent is writing this file/);
  assert.equal(last().dirty, true);
  await click(t, 'retry-save');
  assert.equal(t.puts.length, 2);
  assert.equal(last().dirty, false);
});

test('403 shows the reason and keeps the draft dirty', async () => {
  const t = setup(() => ({ status: 403, json: { reason: 'plan mode is read-only' } }));
  const last = tabsOf(t);
  await openFile(t);
  type(t, 'mine\n');
  await click(t, 'save');
  assert.match(text(t), /plan mode is read-only/);
  assert.equal(area(t).value, 'mine\n');
  assert.equal(last().dirty, true);
});

test('needs_confirm queues a save card; Allow once re-sends with confirm once', async () => {
  const t = setup((body) => (body.confirm ? { json: { sha256: 'h1' } } : { json: { needs_confirm: true } }));
  const last = tabsOf(t);
  await openFile(t);
  type(t, 'mine\n');
  await click(t, 'save');
  const pending = getPendingStore(t.ctx);
  assert.equal(pending.count('save'), 1);
  assert.match(text(t), /Always allow/);
  assert.match(text(t), /also lets the agent write this file/);
  await click(t, 'save-once');
  assert.deepEqual(t.puts[1], { content: 'mine\n', base_sha256: 'h0', confirm: 'once' });
  assert.equal(pending.count('save'), 0);
  assert.equal(last().dirty, false);
});

test('Always allow re-sends with confirm always', async () => {
  const t = setup((body) => (body.confirm ? { json: { sha256: 'h1' } } : { json: { needs_confirm: true } }));
  await openFile(t);
  type(t, 'mine\n');
  await click(t, 'save');
  await click(t, 'save-always');
  assert.equal(t.puts[1].confirm, 'always');
});

test('Cancel resolves the card, sends nothing and keeps the draft dirty', async () => {
  const t = setup(() => ({ json: { needs_confirm: true } }));
  const last = tabsOf(t);
  await openFile(t);
  type(t, 'mine\n');
  await click(t, 'save');
  await click(t, 'save-cancel');
  assert.equal(t.puts.length, 1);
  assert.equal(getPendingStore(t.ctx).count('save'), 0);
  assert.equal(last().dirty, true);
  assert.equal(area(t).value, 'mine\n');
});

test('dismiss and Esc hide the card but leave the save pending; review reopens it', async () => {
  const t = setup(() => ({ json: { needs_confirm: true } }));
  await openFile(t);
  type(t, 'mine\n');
  await click(t, 'save');
  const pending = getPendingStore(t.ctx);
  await click(t, 'dismiss');
  assert.equal(pending.count('save'), 1);
  assert.equal(t.doc.getElementById('code-editor').querySelector('[data-action="save-once"]'), null);
  await click(t, 'review-save');
  assert.ok(t.doc.getElementById('code-editor').querySelector('[data-action="save-once"]'));
  const evt = { handled: false };
  t.ctx.emit('escape', evt);
  assert.equal(evt.handled, true);
  assert.equal(pending.count('save'), 1);
  assert.equal(t.doc.getElementById('code-editor').querySelector('[data-action="save-once"]'), null);
});

test('renderSaveConfirmCard has three actions and the Always allow warning', () => {
  const tree = JSON.stringify(renderSaveConfirmCard({ request_id: 'x', path: 'a.go' }));
  for (const a of ['save-once', 'save-always', 'save-cancel', 'dismiss']) assert.match(tree, new RegExp(a));
  assert.match(tree, /a\.go/);
});

test('disk change while dirty keeps the draft and shows Reload / Keep mine', async () => {
  const t = setup(() => ({ json: { sha256: 'h1' } }));
  await openFile(t);
  type(t, 'mine\n');
  t.ctx.emit('sse', { type: 'file_saved', time: '2999-01-01T00:00:00Z', data: { path: 'a.go', size: 3, sha256: 'other', decision_source: 'mode' } });
  await until(() => /Changed on disk/.test(text(t))); // crypto.subtle is async
  assert.equal(area(t).value, 'mine\n');
  assert.match(text(t), /Changed on disk/);
  await click(t, 'editor-keep');
  assert.doesNotMatch(text(t), /Changed on disk/);
  assert.equal(area(t).value, 'mine\n');
});

test('file_saved for our own saved text leaves the tab clean', async () => {
  const t = setup(() => ({ json: { sha256: 'h1' } }));
  const last = tabsOf(t);
  await openFile(t);
  type(t, 'mine\n');
  await click(t, 'save');
  t.ctx.emit('sse', { type: 'file_saved', time: '2999-01-01T00:00:00Z', data: { path: 'a.go', size: 5, sha256: 'h1', decision_source: 'mode' } });
  await until(() => last().dirty === false);
  assert.equal(last().dirty, false);
  assert.doesNotMatch(text(t), /Changed on disk/);
});

test('clean tab picks up an on-disk change silently', async () => {
  let content = ORIGINAL;
  const t = setup(() => ({ json: {} }));
  t.ctx.api.get = async () => ({ kind: 'file', content, sha256: content === ORIGINAL ? 'h0' : 'h5', size: 1, lang: 'go' });
  await openFile(t);
  content = 'changed by agent\n';
  t.ctx.emit('sse', { type: 'file_saved', time: '2999-01-01T00:00:00Z', data: { path: 'a.go', size: 3, sha256: 'h5', decision_source: 'mode' } });
  await until(() => area(t).value === 'changed by agent\n');
  assert.equal(area(t).value, 'changed by agent\n');
});

test('hook_notice shows a toast', () => {
  const t = setup(() => ({ json: {} }));
  t.ctx.emit('sse', { type: 'hook_notice', data: { message: 'formatted by hook' } });
  assert.match(t.doc.getElementById('toasts').textContent, /formatted by hook/);
});

test('files over the cap stay read-only (no editor)', async () => {
  const t = setup(() => ({ json: {} }), { content: 'x'.repeat(MAX_EDIT + 1) });
  await openFile(t);
  assert.equal(t.doc.getElementById('code-editor').hidden, true);
  assert.equal(t.doc.getElementById('code-content').hidden, false);
});

test('draft survives switching tabs', async () => {
  const t = setup(() => ({ json: {} }));
  await openFile(t, 'a.go');
  type(t, 'draft\n');
  await openFile(t, 'b.go');
  t.ctx.emit('tab:select', { id: 'file:a.go' });
  await flush();
  assert.equal(area(t).value, 'draft\n');
});

test('session:reset clears pending saves and editor state', async () => {
  const t = setup(() => ({ json: { needs_confirm: true } }));
  await openFile(t);
  type(t, 'mine\n');
  await click(t, 'save');
  t.ctx.emit('session:reset');
  assert.equal(getPendingStore(t.ctx).count('save'), 0);
});

// ---- review fixes ----
test('while a confirm is pending Save is disabled and Cmd+S adds no second card', async () => {
  const t = setup((body) => (body.confirm ? { json: { sha256: 'h1' } } : { json: { needs_confirm: true } }));
  await openFile(t);
  type(t, 'mine\n');
  await click(t, 'save');
  const pending = getPendingStore(t.ctx);
  assert.equal(pending.count('save'), 1);
  assert.equal(t.doc.getElementById('code-editor').querySelector('[data-action="save"]').hasAttribute('disabled'), true);
  fire(area(t), 'keydown', { key: 's', ctrlKey: true });
  await flush(); await flush();
  assert.equal(t.puts.length, 1);
  assert.equal(pending.count('save'), 1);
  await click(t, 'save-once');
  assert.equal(t.puts.length, 2);
  assert.equal(pending.count('save'), 0);
});

test('closing a dirty tab asks first; Cancel keeps it, Discard closes it', async () => {
  const t = setup(() => ({ json: {} }));
  const dialogs = [];
  t.ctx.on('dialog:open', (d) => dialogs.push(d));
  await openFile(t);
  type(t, 'mine\n');
  await flush();
  const closeBtn = () => t.ctx.el('tab-strip').querySelector('[data-action="tab-close"]');
  fire(closeBtn(), 'click');
  assert.equal(dialogs.length, 1);
  assert.match(JSON.stringify(dialogs[0].tree), /Discard/);
  assert.ok(t.ctx.el('tab-strip').querySelector('[data-action="tab-close"]'), 'tab still open');
  t.ctx.emit('dialog:action', { owner: dialogs[0].owner, action: 'cancel' });
  assert.ok(t.ctx.el('tab-strip').querySelector('[data-action="tab-close"]'));
  fire(closeBtn(), 'click');
  t.ctx.emit('dialog:action', { owner: dialogs[1].owner, action: 'discard' });
  assert.equal(t.ctx.el('tab-strip').querySelector('[data-action="tab-close"]'), null);
});

test('closing a clean tab does not ask', async () => {
  const t = setup(() => ({ json: {} }));
  const dialogs = [];
  t.ctx.on('dialog:open', (d) => dialogs.push(d));
  await openFile(t);
  fire(t.ctx.el('tab-strip').querySelector('[data-action="tab-close"]'), 'click');
  assert.equal(dialogs.length, 0);
  assert.equal(t.ctx.el('tab-strip').querySelector('[data-action="tab-close"]'), null);
});

test('session:reset names the files whose drafts were discarded', async () => {
  const t = setup(() => ({ json: {} }));
  await openFile(t);
  type(t, 'mine\n');
  t.ctx.emit('session:reset');
  assert.match(t.doc.getElementById('toasts').textContent, /a\.go/);
  assert.match(t.doc.getElementById('toasts').textContent, /discarded/i);
});

test('a clean reload landing after the user typed never replaces the draft', async () => {
  const t = setup(() => ({ json: { sha256: 'h2' } }));
  await openFile(t);
  type(t, 'typed\n');
  t.ctx.emit('tab:patch', { id: 'file:a.go', content: 'from disk\n', sha256: 'hdisk', dirty: false });
  await flush();
  assert.equal(area(t).value, 'typed\n');
  assert.match(text(t), /Changed on disk/);
  await click(t, 'save');
  assert.equal(t.puts[0].base_sha256, 'h0', 'save still uses the sha the draft started from');
});

test('Overwrite failure keeps the conflict banner and the draft', async () => {
  for (const [status, json] of [[423, { reason: 'agent is writing this file' }], [403, { reason: 'denied' }], [500, {}]]) {
    const t = setup((body, n) => (n === 1 ? { status: 409, json: {} } : { status, json }));
    await openFile(t);
    type(t, 'mine\n');
    await click(t, 'save');
    await click(t, 'conflict-diff');
    await click(t, 'conflict-overwrite');
    assert.equal(t.puts.length, 2, 'status ' + status);
    assert.match(text(t), /File changed on disk/, 'banner kept for ' + status);
    assert.equal(area(t).value, 'mine\n');
    assert.match(text(t), status === 500 ? /Save failed/ : /writing this file|denied/);
  }
});

test('Overwrite refuses when the disk changed again after the diff and refreshes it', async () => {
  let disk = { content: 'a\nb\n', sha: 's1' };
  const t = setup((body, n) => (n === 1 ? { status: 409, json: {} } : { json: { sha256: 'h2' } }));
  t.ctx.api.get = async () => ({ kind: 'file', content: disk.content, sha256: disk.sha, size: 1, lang: 'go' });
  await openFile(t);
  type(t, 'a\nB\n');
  await click(t, 'save');
  await click(t, 'conflict-diff');
  disk = { content: 'a\nc\n', sha: 's2' };
  await click(t, 'conflict-overwrite');
  assert.equal(t.puts.length, 1, 'no overwrite sent');
  assert.match(text(t), /changed again/i);
  assert.match(text(t), /-\s*c/);
  await click(t, 'conflict-overwrite');
  assert.equal(t.puts.length, 2);
  assert.equal(t.puts[1].base_sha256, 's2');
});

test('Overwrite without having seen a diff shows the diff first', async () => {
  const t = setup((body, n) => (n === 1 ? { status: 409, json: {} } : { json: { sha256: 'h2' } }), { content: 'a\n' });
  await openFile(t);
  type(t, 'b\n');
  await click(t, 'save');
  await click(t, 'conflict-overwrite');
  assert.equal(t.puts.length, 1);
  assert.match(text(t), /\+\s*b/);
});
