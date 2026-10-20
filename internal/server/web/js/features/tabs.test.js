import { test } from 'node:test';
import assert from 'node:assert/strict';
import { makeCtx, fire, flush, until } from '../fakedom.js';
import { mount as mountSplit } from './split.js';
import { mount as mountTabs } from './tabs.js';
import { mount as mountCodeview, TOKEN_TYPES, LINE_PAGE, MAX_MARKDOWN } from './codeview.js';
import { samePath } from './tabs.js';
import { readFileSync } from 'node:fs';
import { createHash } from 'node:crypto';

const FILE_ROUTE = (content = 'current', extra = {}) => ({ 'GET /v1/sessions/s1/files': () => ({ json: { kind: 'file', content, sha256: 'hash', size: content.length, lang: 'go', ...extra } }) });
const open = async (t, path = 'a.go') => { t.ctx.state.sessionId = 's1'; mountTabs(t.ctx); mountCodeview(t.ctx); t.ctx.emit('file:open', path); await flush(); };
const emitWrite = (t, path, id = 'w1', time = '2026-01-01T00:00:00Z') => {
  t.ctx.emit('sse', { type: 'tool_use_start', time, data: { id, name: 'FileWrite', input: { path } } });
  t.ctx.emit('sse', { type: 'tool_use_result', time, data: { id } });
};

function setup(routes = {}) {
  const t = makeCtx({ routes });
  mountSplit(t.ctx);
  return t;
}

test('tabs restore and operate when sessionStorage throws', () => {
  const t = setup();
  Object.defineProperty(t.win, 'sessionStorage', { get() { throw Error('blocked'); } });
  const active = [], counts = [];
  t.ctx.on('tabs:active', ({ tab }) => active.push(tab));
  t.ctx.on('tabs:count', ({ count }) => counts.push(count));
  mountTabs(t.ctx);
  t.ctx.emit('tab:open', { id: 'file:a.go', kind: 'file', path: 'a.go', title: 'a.go' });
  assert.equal(active.at(-1).path, 'a.go');
  assert.equal(counts.at(-1), 1);
  assert.equal(t.ctx.el('tab-pane').hidden, false);
  t.ctx.emit('tab:close', { id: 'file:a.go' });
  assert.equal(active.at(-1), null);
  assert.equal(counts.at(-1), 0);
});

test('restores only paths and active selection from sessionStorage', () => {
  const data = new Map();
  const first = setup();
  first.win.sessionStorage = { getItem: (key) => data.get(key), setItem: (key, value) => data.set(key, value) };
  first.ctx.state.sessionId = 's1';
  mountTabs(first.ctx);
  first.ctx.emit('tab:open', { id: 'file:a.go', kind: 'file', path: 'a.go' });
  first.ctx.emit('tab:patch', { id: 'file:a.go', dirty: true, content: 'private draft', sha256: 'hash' });
  const saved = JSON.stringify([...data.values()]);
  assert.doesNotMatch(saved, /private draft|hash|dirty/);
  const second = setup();
  second.win.sessionStorage = first.win.sessionStorage;
  second.ctx.state.sessionId = 's1';
  const active = [];
  second.ctx.on('tabs:active', ({ tab }) => active.push(tab));
  mountTabs(second.ctx);
  second.ctx.emit('session:ready', 's1');
  assert.equal(active.at(-1).path, 'a.go');
  assert.equal(active.at(-1).dirty, false);
  assert.equal(active.at(-1).content, null);
});

test('clean file reloads after a matching FileWrite result', async () => {
  let reads = 0;
  const t = setup({ 'GET /v1/sessions/s1/files': () => ({ json: { kind: 'file', content: `package main // ${++reads}`, sha256: `hash${reads}`, size: 24, lang: 'go' } }) });
  t.ctx.state.sessionId = 's1';
  mountTabs(t.ctx); mountCodeview(t.ctx);
  t.ctx.emit('file:open', 'handler.go');
  await flush();
  assert.equal(reads, 1);
  t.ctx.emit('sse', { type: 'tool_use_start', data: { id: 'w1', name: 'FileWrite', input: { path: 'handler.go' } } });
  t.ctx.emit('sse', { type: 'tool_use_result', data: { id: 'w1', result: 'ok' } });
  await flush();
  assert.equal(reads, 2);
  assert.match(t.ctx.el('tab-content').textContent, /package main \/\/ 2/);
});

test('dirty file shows disk-change banner and Keep mine preserves its content', async () => {
  let reads = 0;
  const t = setup({ 'GET /v1/sessions/s1/files': () => ({ json: { kind: 'file', content: `disk ${++reads}`, sha256: `hash${reads}`, size: 6, lang: 'go' } }) });
  t.ctx.state.sessionId = 's1';
  mountTabs(t.ctx); mountCodeview(t.ctx);
  t.ctx.emit('file:open', 'handler.go');
  await flush();
  t.ctx.emit('tab:patch', { id: 'file:handler.go', dirty: true, content: 'mine' });
  t.ctx.emit('sse', { type: 'tool_use_start', data: { id: 'e1', name: 'FileEdit', input: { path: 'handler.go' } } });
  t.ctx.emit('sse', { type: 'tool_use_result', data: { id: 'e1' } });
  await flush();
  assert.equal(reads, 1);
  assert.match(t.ctx.el('tab-content').textContent, /Changed on disk/);
  fire(t.ctx.el('tab-content').querySelector('[data-action="keep-mine"]'), 'click');
  assert.match(t.ctx.el('tab-content').textContent, /mine/);
  assert.equal(reads, 1);
});

test('an inactive open file reloads after a matching edit', async () => {
  const t = setup({ 'GET /v1/sessions/s1/files': () => ({ json: { kind: 'file', content: 'current', sha256: 'hash', size: 7, lang: 'go' } }) });
  // Count actual requests by path; the fake fetch records each call.
  t.ctx.state.sessionId = 's1';
  mountTabs(t.ctx); mountCodeview(t.ctx);
  t.ctx.emit('file:open', 'a.go');
  await flush();
  t.ctx.emit('file:open', 'b.go');
  await flush();
  t.ctx.emit('sse', { type: 'tool_use_start', data: { id: 'e1', name: 'FileEdit', input: { path: 'a.go' } } });
  t.ctx.emit('sse', { type: 'tool_use_result', data: { id: 'e1' } });
  await flush();
  assert.equal(t.calls.filter((call) => call.path.includes('a.go')).length, 2);
});

test('an edit in another directory does not reload a same-name file', async () => {
  const t = setup({ 'GET /v1/sessions/s1/files': () => ({ json: { kind: 'file', content: 'current', sha256: 'hash', size: 7, lang: 'go' } }) });
  t.ctx.state.sessionId = 's1';
  mountTabs(t.ctx); mountCodeview(t.ctx);
  t.ctx.emit('file:open', 'a.go');
  await flush();
  t.ctx.emit('sse', { type: 'tool_use_start', data: { id: 'e2', name: 'FileEdit', input: { path: 'other/a.go' } } });
  t.ctx.emit('sse', { type: 'tool_use_result', data: { id: 'e2' } });
  await flush();
  assert.equal(t.calls.length, 1);
});

test('tabs leave other renderers children in shared content container', () => {
  const t = setup();
  const child = t.doc.createElement('section');
  child.id = 'agents-content';
  t.ctx.el('tab-content').appendChild(child);
  mountTabs(t.ctx); mountCodeview(t.ctx);
  t.ctx.emit('tab:open', { id: 'agents', kind: 'agents', title: 'Agents' });
  assert.equal(t.ctx.el('agents-content'), child);
  assert.equal(t.ctx.el('code-content').hidden, true);
});

test('code and markdown file contents do not create executable elements', async () => {
  const t = setup({ 'GET /v1/sessions/s1/files': () => ({ json: {
    kind: 'file', content: '<img src=x onerror=alert(1)>', sha256: 'hash', size: 28, lang: 'markdown',
  } }) });
  t.ctx.state.sessionId = 's1';
  mountTabs(t.ctx); mountCodeview(t.ctx);
  t.ctx.emit('file:open', 'README.md');
  await flush();
  const content = t.ctx.el('code-content');
  assert.equal(content.querySelector('img'), null);
  assert.match(content.querySelector('.bv-code__markdown').innerHTML, /&lt;img/);
});

test('the plus tab displays a bounded event log without clearing file content', () => {
  const t = setup();
  mountTabs(t.ctx); mountCodeview(t.ctx);
  t.ctx.emit('file:open', 'a.go');
  const code = t.ctx.el('code-content');
  fire(t.ctx.el('tab-strip').querySelector('[data-action="tab-new"]'), 'click');
  t.ctx.emit('sse', { type: 'terminal', data: { reason: '<script>alert(1)</script>' } });
  assert.equal(t.ctx.el('code-content'), code);
  assert.equal(code.hidden, true);
  assert.match(t.ctx.el('tab-content').textContent, /terminal/);
  assert.equal(t.ctx.el('tab-content').querySelector('script'), null);
});

test('binary or invalid UTF-8 (415) shows a per-tab notice, no toast, no retry on reselect', async () => {
  const t = setup({ 'GET /v1/sessions/s1/files': () => ({ status: 415, json: { error: 'binary' } }) });
  await open(t, 'a.png');
  assert.match(t.ctx.el('tab-content').textContent, /Binary or non-UTF-8 file cannot be shown/);
  assert.doesNotMatch(t.ctx.el('tab-content').textContent, /HTTP 415|Loading/);
  assert.equal(t.ctx.el('toasts').children.length, 0);
  t.ctx.emit('file:open', 'b.png'); await flush();
  const before = t.calls.length;
  t.ctx.emit('tab:select', { id: 'file:a.png' }); await flush();
  t.ctx.emit('tab:select', { id: 'file:b.png' }); await flush();
  t.ctx.emit('tab:select', { id: 'file:a.png' }); await flush();
  assert.equal(t.calls.length, before);
});

test('tool paths are normalized against the workspace root and dot segments', async () => {
  assert.equal(samePath('/work/internal/a.go', 'internal/a.go', '/work'), true);
  assert.equal(samePath('/work//internal/./x/../a.go', 'internal/a.go', '/work/'), true);
  assert.equal(samePath('./internal//a.go', 'internal/a.go'), true);
  assert.equal(samePath('/elsewhere/internal/a.go', 'internal/a.go', '/work'), false);
  assert.equal(samePath('/work/other/a.go', 'a.go', '/work'), false);
  assert.equal(samePath('other/a.go', 'a.go'), false);
  assert.equal(samePath('../a.go', 'a.go'), false);
  const t = setup(FILE_ROUTE());
  t.ctx.state.workspaceRoot = '/work';
  await open(t, 'internal/a.go');
  emitWrite(t, '/work/internal/a.go');
  await flush();
  assert.equal(t.calls.length, 2);
  const u = setup(FILE_ROUTE()); // root unknown: whole-segment suffix match
  await open(u, 'internal/a.go');
  emitWrite(u, '/work/internal/a.go');
  await flush();
  assert.equal(u.calls.length, 2);
});

test('a disk change during a load triggers exactly one trailing refresh', async () => {
  let release;
  let reads = 0;
  const t = setup({ 'GET /v1/sessions/s1/files': async () => { reads++; if (reads === 1) await new Promise((r) => { release = r; }); return { json: { kind: 'file', content: 'c' + reads, sha256: 'h', size: 2, lang: 'go' } }; } });
  t.ctx.state.sessionId = 's1';
  mountTabs(t.ctx); mountCodeview(t.ctx);
  t.ctx.emit('file:open', 'a.go');
  await flush();
  for (let i = 0; i < 20; i++) emitWrite(t, 'a.go', 'w' + i);
  assert.equal(reads, 1);
  release(); await flush(); await flush(); await flush();
  assert.equal(reads, 2);
  assert.match(t.ctx.el('tab-content').textContent, /c2/);
});

test('every token type highlight.js can emit has a class and a CSS rule', () => {
  const src = readFileSync(new URL('../highlight.js', import.meta.url), 'utf8');
  const emitted = new Set([...src.matchAll(/push\(tokens, '(\w+)'/g)].map((m) => m[1]));
  for (const m of src.matchAll(/(?:type = |\? |: )'(\w+)'/g)) emitted.add(m[1]);
  emitted.delete('');
  assert.ok(emitted.size >= 12);
  const css = readFileSync(new URL('../../css/code.css', import.meta.url), 'utf8');
  for (const type of [...emitted, 'func', 'type']) {
    assert.ok(TOKEN_TYPES.includes(type), 'class for ' + type);
    assert.match(css, new RegExp('bv-code__token--' + type + '\\b'), 'css rule for ' + type);
  }
  assert.match(css, /--syn-func/); assert.match(css, /--syn-type/);
});

test('unknown token types render plain', async () => {
  const t = setup(FILE_ROUTE('x'));
  await open(t);
  assert.equal(t.ctx.el('code-content').querySelectorAll('.bv-code__token--mystery').length, 0);
});

test('failed reload keeps the draft, the dirty flag and the banner; success clears them', async () => {
  let fail = false;
  const t = setup({ 'GET /v1/sessions/s1/files': () => (fail ? { status: 500, text: 'boom' } : { json: { kind: 'file', content: 'disk', sha256: 'h', size: 4, lang: 'go' } }) });
  await open(t);
  t.ctx.emit('tab:patch', { id: 'file:a.go', dirty: true, content: 'my draft' });
  emitWrite(t, 'a.go'); await flush();
  assert.match(t.ctx.el('tab-content').textContent, /Changed on disk/);
  fail = true;
  fire(t.ctx.el('tab-content').querySelector('[data-action="reload-file"]'), 'click');
  await flush();
  assert.match(t.ctx.el('tab-content').textContent, /Changed on disk/);
  let tab; t.ctx.on('tabs:active', (e) => { tab = e.tab; });
  t.ctx.emit('tab:select', { id: 'file:a.go' });
  assert.equal(tab.dirty, true); assert.equal(tab.content, 'my draft');
  fail = false;
  fire(t.ctx.el('tab-content').querySelector('[data-action="reload-file"]'), 'click');
  await flush();
  t.ctx.emit('tab:select', { id: 'file:a.go' });
  assert.equal(tab.dirty, false); assert.equal(tab.content, 'disk');
  assert.doesNotMatch(t.ctx.el('tab-content').textContent, /Changed on disk/);
});

test('history replay does not trigger disk-change reloads', async () => {
  const t = setup(FILE_ROUTE());
  await open(t);
  emitWrite(t, 'a.go', 'old', '2020-01-01T00:00:00Z');
  await flush();
  assert.equal(t.calls.length, 1);
  emitWrite(t, 'a.go', 'new');
  await flush();
  assert.equal(t.calls.length, 2);
});

test('file_saved marks a tab clean when it holds the saved content, else flags a disk change', async () => {
  const t = setup(FILE_ROUTE());
  await open(t);
  const sha = createHash('sha256').update('my edit').digest('hex');
  let cleaned = false;
  t.ctx.on('tab:patch', (e) => { if (e.sha256 === sha && e.dirty === false) cleaned = true; });
  t.ctx.emit('tab:patch', { id: 'file:a.go', dirty: true, content: 'my edit' });
  t.ctx.emit('sse', { type: 'file_saved', time: '2026-01-01T00:00:00Z', data: { path: 'a.go', size: 7, sha256: sha, decision_source: 'user' } });
  await until(() => cleaned); // tabs.js hashes with crypto.subtle, which settles off the event loop
  let tab; t.ctx.on('tabs:active', (e) => { tab = e.tab; });
  t.ctx.emit('tab:select', { id: 'file:a.go' });
  assert.equal(tab.dirty, false); assert.equal(tab.sha256, sha);
  assert.doesNotMatch(t.ctx.el('tab-content').textContent, /Changed on disk/);
  t.ctx.emit('tab:patch', { id: 'file:a.go', dirty: true, content: 'other' });
  t.ctx.emit('sse', { type: 'file_saved', time: '2026-01-01T00:00:00Z', data: { path: 'a.go', size: 7, sha256: 'f'.repeat(64) } });
  await until(() => /Changed on disk/.test(t.ctx.el('tab-content').textContent));
  assert.match(t.ctx.el('tab-content').textContent, /Changed on disk/);
});

test('the event log uses eventstore (no deltas) and renders only for kind events', () => {
  const t = setup();
  mountTabs(t.ctx);
  t.ctx.emit('sse', { type: 'assistant_text_delta', data: { text: 'noise' } });
  t.ctx.emit('sse', { type: 'terminal', data: { reason: 'done' } });
  const log = t.ctx.el('event-log-content');
  for (const kind of ['transcript', 'mystery', 'agents']) {
    t.ctx.emit('tab:open', { id: kind, kind, title: kind });
    assert.equal(log.hidden, true, kind);
  }
  t.ctx.emit('tab:open', { id: 'events', kind: 'events' });
  assert.equal(log.hidden, false);
  assert.match(log.textContent, /terminal done/);
  assert.doesNotMatch(log.textContent, /noise/);
});

test('registerTabKind plugs a view in without editing tabs.js', () => {
  const t = setup();
  mountTabs(t.ctx);
  const seen = [];
  assert.equal(t.ctx.registerTabKind('agents-log', ({ host }) => ({ show: (tab) => seen.push(tab.id), hide: () => seen.push('hide') })), true);
  assert.equal(t.ctx.registerTabKind('events', () => ({})), false);
  const host = t.ctx.el('tab-view-agents-log');
  assert.equal(host.hidden, true);
  t.ctx.emit('tab:open', { id: 'agents-log', kind: 'agents-log' });
  assert.equal(host.hidden, false);
  assert.equal(t.ctx.el('event-log-content').hidden, true);
  t.ctx.emit('tab:open', { id: 'x', kind: 'x' });
  assert.equal(host.hidden, true);
  assert.ok(seen.includes('agents-log'));
  assert.equal(seen.at(-1), 'hide');
});

test('large files render a capped number of lines with a show-more control', async () => {
  const big = Array.from({ length: LINE_PAGE * 2 + 10 }, (_, i) => 'x' + i).join('\n');
  const t = setup(FILE_ROUTE(big));
  await open(t);
  const code = t.ctx.el('code-content');
  assert.equal(code.querySelectorAll('.bv-code__line').length, LINE_PAGE);
  fire(code.querySelector('[data-action="show-more"]'), 'click');
  assert.equal(code.querySelectorAll('.bv-code__line').length, LINE_PAGE * 2);
});

test('oversized markdown is not rendered as HTML', async () => {
  const md = '# title\n' + 'a'.repeat(MAX_MARKDOWN);
  const t = setup(FILE_ROUTE(md, { lang: 'markdown' }));
  await open(t, 'R.md');
  assert.equal(t.ctx.el('code-content').querySelector('.bv-code__markdown'), null);
  assert.ok(t.ctx.el('code-content').querySelector('.bv-code__source'));
});
