import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { boot } from './app.js';
import { pageWith, REGION_IDS, ORIGIN, flush, fire } from './fakedom.js';

const enc = new TextEncoder();
const frame = (id, type, data = {}) => `id: ${id}\nevent: ${type}\ndata: ${JSON.stringify({ id, type, time: '2026-01-01T00:00:00Z', data })}\n\n`;

function env({ hash = '', width = 1448 } = {}) {
  const doc = pageWith(REGION_IDS);
  const calls = [];
  const replaced = [];
  const store = {};
  const win = {
    innerWidth: width,
    location: { origin: ORIGIN, hash, pathname: '/', search: '' },
    history: { replaceState: (...a) => replaced.push(a) },
    navigator: { clipboard: { writeText: async () => {} } },
    addEventListener() {},
  };
  const storage = { getItem: (k) => store[k] ?? null, setItem: (k, v) => { store[k] = v; } };
  const fetchImpl = async (path, init = {}) => {
    const method = init.method || 'GET';
    calls.push({ method, path, headers: init.headers || {}, body: init.body ? JSON.parse(init.body) : undefined });
    const json = (o, status = 200) => ({ ok: status < 300, status, text: async () => JSON.stringify(o) });
    if (method === 'POST' && path === '/v1/sessions?reuse=blank') return json({ session_id: 'sess_1' });
    if (path === '/v1/sessions/sess_1/events') {
      let sent = false;
      return { ok: true, status: 200, body: { getReader: () => ({ read: async () => (sent ? new Promise(() => {}) : (sent = true, { done: false, value: enc.encode(frame('1', 'run_started') + frame('2', 'assistant_text_delta', { content: 'hi there' }) + frame('3', 'terminal', { usage: { Turns: 1, PromptEvalCount: 5, EvalCount: 2 } })) })), cancel: async () => {} }) } };
    }
    if (path === '/v1/models') return json({ models: [{ name: 'qwen3' }] });
    if (path.startsWith('/v1/sessions/sess_1/files')) return json({ kind: 'dir', entries: [{ name: 'a.go', kind: 'file', size: 1 }] });
    if (path === '/v1/health') return json({ status: 'ok', ollama: 'reachable' });
    return json({}, 404);
  };
  return { doc, win, storage, store, fetchImpl, calls, replaced };
}

test('boot adopts the URL token, removes it from the address bar, and sends it as a bearer', async () => {
  const e = env({ hash: '#token=sekrit' });
  boot(e);
  await flush(); await flush(); await flush();
  assert.equal(e.replaced.length, 1);
  assert.ok(!e.replaced[0][2].includes('sekrit'));
  assert.ok(Object.values(e.store).includes('sekrit'));
  assert.ok(e.calls.length > 0 && e.calls.every((c) => c.headers.Authorization === 'Bearer sekrit'));
});

test('boot wires a full session: header, tree, transcript, rail, footer', async () => {
  const e = env();
  const ctx = boot(e);
  for (let i = 0; i < 6; i++) await flush();
  assert.equal(ctx.state.sessionId, 'sess_1');
  assert.match(e.doc.getElementById('header').textContent, /sess_1/);
  assert.match(e.doc.getElementById('header').textContent, /Connected/);
  assert.match(e.doc.getElementById('tree').textContent, /a\.go/);
  assert.match(e.doc.getElementById('transcript').textContent, /hi there/);
  assert.match(e.doc.getElementById('rail').textContent, /Session status/);
  assert.match(e.doc.getElementById('footer').textContent, /Server: Connected/);
  assert.equal(ctx.state.running, false);
  assert.equal(e.doc.getElementById('message-input').disabled, false);
  assert.equal(e.doc.getElementById('app').getAttribute('data-nav'), 'full');
});

test('sending a message from the composer reaches the server', async () => {
  const e = env();
  boot(e);
  for (let i = 0; i < 6; i++) await flush();
  const input = e.doc.getElementById('message-input');
  input.value = 'hello';
  fire(input, 'keydown', { key: 'Enter' });
  await flush();
  const post = e.calls.find((c) => c.path === '/v1/sessions/sess_1/messages');
  assert.equal(post.body.prompt, 'hello');
});

test('app.js is only imports, a feature list and boot', () => {
  const src = readFileSync(new URL('./app.js', import.meta.url), 'utf8');
  assert.ok(!/innerHTML|createElement|addEventListener|\.textContent/.test(src), 'no feature logic in app.js');
  const imports = src.match(/^import .*features\/.*$/gm) || [];
  const mounts = src.match(/mount[A-Z]\w+/g) || [];
  assert.equal(imports.length, 21);
  assert.ok(mounts.length >= 40, 'each feature is imported once and listed once');
});

test('every module under features/ is mounted by app.js', async () => {
  const { readdirSync } = await import('node:fs');
  const features = readdirSync(new URL('./features/', import.meta.url)).filter((f) => f.endsWith('.js') && !f.endsWith('.test.js'));
  const src = readFileSync(new URL('./app.js', import.meta.url), 'utf8');
  for (const f of features) assert.ok(src.includes("./features/" + f), f + ' is not wired in app.js');
});

test('full module graph: Agents registers its tab kinds with tabs.js and the nav opens the view', async () => {
  const e = env();
  const ctx = boot(e);
  for (let i = 0; i < 6; i++) await flush();
  assert.equal(typeof ctx.registerTabKind, 'function', 'tabs.js mounted before agents.js');
  ctx.emit('tab:open', { id: 'agents', kind: 'agents', title: 'Agents' });
  for (let i = 0; i < 3; i++) await flush();
  const view = e.doc.getElementById('tab-view-agents');
  assert.ok(view, 'agents view host exists under #tab-content');
  assert.equal(view.parentNode, e.doc.getElementById('tab-content'));
  assert.equal(ctx.registerTabKind('agents', () => ({})), false, 'a kind registers once');
});
