import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const web = join(here, '..');
const read = (p) => readFileSync(join(web, p), 'utf8');
const fixture = (name) => JSON.parse(read('dev/fixtures/' + name + '.json'));
const gallery = await import('./gallery.js');

test('width buttons are 1280 / 1440 / 1920', () => {
  assert.deepEqual(gallery.WIDTHS, [1280, 1440, 1920]);
});

test('eventsToView folds a recorded run into render data', () => {
  const v = gallery.eventsToView(fixture('run-permission'));
  assert.ok(v.items.some((i) => i.kind === 'thinking'));
  const tools = v.items.filter((i) => i.kind === 'tool');
  assert.equal(tools.length, 2);
  assert.equal(tools[0].status, 'completed');
  assert.equal(v.permission.request_id, 'permission-1');
  assert.equal(v.usage.Turns > 0, true);
  assert.ok(v.events.length > 0);
});

test('eventsToView keeps a pending permission only when unresolved', () => {
  const v = gallery.eventsToView(fixture('run-basic'));
  assert.equal(v.permission, null);
});

test('usage shows an em dash before the first turn', () => {
  const s = gallery.buildSections({ 'run-basic': fixture('run-basic').filter((e) => e.type !== 'terminal') });
  const usage = JSON.stringify(s.find((x) => x.id === 'usage').tree);
  assert.ok(usage.includes('—'));
});

test('gallery renders every exported render function', () => {
  const dir = join(web, 'js/ui');
  const exported = [];
  for (const f of readdirSync(dir).filter((n) => !n.endsWith('.test.js') && n.endsWith('.js'))) {
    for (const m of read('js/ui/' + f).matchAll(/^export function (render\w+)/gm)) exported.push(m[1]);
  }
  assert.ok(exported.length >= 20);
  const covered = new Set(gallery.COVERED);
  assert.deepEqual(exported.filter((n) => !covered.has(n)), []);
  const sections = gallery.buildSections({ 'run-permission': fixture('run-permission'), 'run-todo': fixture('run-todo'), 'run-basic': fixture('run-basic') });
  assert.ok(sections.length >= 12);
  for (const s of sections) assert.ok(s.tree && s.tree.tag, s.id);
});

test('actionOf resolves nested buttons to the row action', () => {
  const row = { dataset: { action: 'select-session', id: 's1' } };
  const inner = { closest: (sel) => (sel === '[data-action]' ? { dataset: { action: 'delete-session', id: 's1' } } : null) };
  assert.equal(gallery.actionOf(inner).action, 'delete-session');
  const plain = { closest: () => row };
  assert.equal(gallery.actionOf(plain).id, 's1');
  assert.equal(gallery.actionOf({ closest: () => null }), null);
});

test('always-allow maps to the K2 decision always_allow', () => {
  assert.equal(gallery.decisionFor('always-allow'), 'always_allow');
  assert.equal(gallery.decisionFor('allow'), 'allow');
  assert.equal(gallery.decisionFor('deny'), 'deny');
  assert.equal(gallery.decisionFor('nope'), null);
});

test('gallery.html: widths, stylesheets, no inline code or URLs', () => {
  const html = read('dev/gallery.html');
  for (const w of [1280, 1440, 1920]) assert.match(html, new RegExp('data-width="' + w + '"'));
  assert.match(html, /css\/tokens\.css/);
  assert.match(html, /css\/components\.css/);
  assert.match(html, /<script type="module" src="dev\/gallery\.js"/);
  assert.doesNotMatch(html, /\sstyle=/);
  assert.doesNotMatch(html, /<style/i);
  assert.doesNotMatch(html, /<script(?![^>]*\ssrc=)/i);
  assert.doesNotMatch(html, /\son[a-z]+=/i);
  assert.doesNotMatch(html, /https?:\/\//);
});

test('components.css styles every bv- block the render modules emit', () => {
  const css = read('css/components.css') + read('css/agents.css');
  const blocks = new Set();
  const dir = join(web, 'js/ui');
  for (const f of readdirSync(dir).filter((n) => n.endsWith('.js') && !n.endsWith('.test.js'))) {
    for (const m of read('js/ui/' + f).matchAll(/'(bv-[a-z0-9-]+(?:__[a-z0-9-]+)?)/g)) blocks.add(m[1]);
  }
  assert.ok(blocks.size > 40);
  const missing = [...blocks].filter((b) => !css.includes('.' + b));
  assert.deepEqual(missing, []);
  assert.doesNotMatch(css, /https?:\/\//);
  assert.match(css, /var\(--border\)/);
});
