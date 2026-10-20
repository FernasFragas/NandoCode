import { test } from 'node:test';
import assert from 'node:assert/strict';
import { makeCtx, fire, flush } from '../fakedom.js';
import { mount } from './header.js';
import { mount as mountDialog } from './dialog.js';
import { mount as mountKeys } from './keys.js';
import { mount as mountCredential } from './credential.js';
import { readFileSync } from 'node:fs';

function setup(routes = {}) {
  const t = makeCtx({ routes: { 'GET /v1/models': () => ({ json: { models: [{ name: 'qwen3' }, { name: 'gpt-oss:120b-cloud' }] } }), ...routes } });
  mountKeys(t.ctx); mountDialog(t.ctx); mountCredential(t.ctx); mount(t.ctx);
  return t;
}
const header = (t) => t.doc.getElementById('header');

test('brand, mode pill and a disabled Stop slot render before a session exists', () => {
  const t = setup();
  const text = header(t).textContent;
  assert.match(text, /NandoCode/);
  assert.match(text, /Local server mode/);
  assert.match(text, /No session/);
  const stop = header(t).querySelector('.bv-header__stop');
  assert.equal(stop.disabled, true);
});

test('session ready loads models into the picker and shows the id', async () => {
  const t = setup();
  t.ctx.set({ sessionId: 'sess_1' });
  t.ctx.emit('session:ready', 'sess_1');
  await flush();
  assert.equal(t.ctx.state.model, 'qwen3');
  const options = header(t).querySelectorAll('option');
  assert.deepEqual(options.map((o) => o.getAttribute('value')), ['qwen3', 'gpt-oss:120b-cloud']);
  assert.match(header(t).textContent, /sess_1/);
});

test('the connection pill follows state; unauthorized reads as disconnected', () => {
  const t = setup();
  t.ctx.set({ connection: 'connected' });
  assert.match(header(t).querySelector('.bv-header__conn').className, /--connected/);
  t.ctx.set({ connection: 'unauthorized' });
  assert.match(header(t).querySelector('.bv-header__conn').className, /--disconnected/);
});

test('changing the model posts it and updates state', async () => {
  const t = setup({ 'POST /v1/sessions/sess_1/model': (b) => ({ json: { model: b.model } }) });
  t.ctx.set({ sessionId: 'sess_1' });
  t.ctx.emit('session:ready');
  await flush();
  const select = header(t).querySelector('select');
  select.value = 'gpt-oss:120b-cloud';
  fire(select, 'change');
  await flush();
  assert.equal(t.calls.at(-1).body.model, 'gpt-oss:120b-cloud');
  assert.equal(t.ctx.state.model, 'gpt-oss:120b-cloud');
});

test('requires_credential opens the key prompt and retries the switch after saving', async () => {
  let attempts = 0;
  const t = setup({
    'POST /v1/sessions/sess_1/model': (b) => (++attempts === 1 ? { status: 428, json: { error: 'requires_credential', provider: 'ollama_cloud_api' } } : { json: { model: b.model } }),
    'POST /v1/credentials/ollama-cloud': () => ({ json: { ok: true } }),
  });
  t.ctx.set({ sessionId: 'sess_1' });
  t.ctx.emit('session:ready');
  await flush();
  const select = header(t).querySelector('select');
  select.value = 'gpt-oss:120b-cloud';
  fire(select, 'change');
  await flush();
  assert.match(t.doc.getElementById('dialog').textContent, /Ollama Cloud API key/);
  assert.equal(t.ctx.state.model, 'qwen3', 'not switched yet');
  t.doc.getElementById('credential-key').value = 'sk-x';
  fire(t.doc.getElementById('dialog').querySelectorAll('button').find((b) => b.getAttribute('data-action') === 'save'), 'click');
  await flush(); await flush();
  assert.equal(attempts, 2);
  assert.equal(t.ctx.state.model, 'gpt-oss:120b-cloud');
});

test('a plain switch failure toasts and keeps the old model', async () => {
  const t = setup({ 'POST /v1/sessions/sess_1/model': () => ({ status: 400, text: 'model not found' }) });
  t.ctx.set({ sessionId: 'sess_1' });
  t.ctx.emit('session:ready');
  await flush();
  const select = header(t).querySelector('select');
  select.value = 'gpt-oss:120b-cloud';
  fire(select, 'change');
  await flush();
  assert.match(t.doc.getElementById('toasts').textContent, /Model switch failed: model not found/);
  assert.equal(t.ctx.state.model, 'qwen3');
});

test('the picker is disabled while a run is active', async () => {
  const t = setup();
  t.ctx.set({ sessionId: 's' });
  t.ctx.emit('session:ready');
  await flush();
  t.ctx.set({ running: true });
  assert.equal(header(t).querySelector('select').disabled, true);
});

test('copy writes the session id; gear opens settings', async () => {
  const t = setup();
  t.ctx.set({ sessionId: 'sess_7' });
  fire(header(t).querySelector('.bv-header__copy'), 'click');
  await flush();
  assert.deepEqual(t.win.navigator.clipboard.writes, ['sess_7']);
  let opened = 0;
  t.ctx.on('settings:open', () => { opened += 1; });
  fire(header(t).querySelector('.bv-header__settings'), 'click');
  assert.equal(opened, 1);
});

test('a models failure toasts', async () => {
  const t = setup({ 'GET /v1/models': () => ({ status: 502, text: 'down' }) });
  t.ctx.emit('session:ready');
  await flush();
  assert.match(t.doc.getElementById('toasts').textContent, /Failed to load models/);
});

test('New session lives in the sidebar only and stays reachable in narrow rail mode', () => {
  const t = makeCtx({ width: 1280 });
  mount(t.ctx);
  assert.equal(header(t).querySelector('[data-action="new-session"]'), null);
  const css = readFileSync(new URL('../../css/sessions.css', import.meta.url), 'utf8');
  assert.match(css, /\.bv-app\[data-nav="rail"\]\s+\.bv-app__sessions\s*\{[^}]*display:\s*block/);
  assert.match(css, /\.bv-app\[data-nav="rail"\]\s+\.bv-sessions__new\s*\{/);
});

test('cloud-only catalog model is selectable and triggers the existing credential prompt', async () => {
  let attempts = 0;
  const t = setup({
    'GET /v1/models': () => ({ json: { models: [{ name: 'qwen3' }, { name: 'kimi-k2.6' }] } }),
    'POST /v1/sessions/sess_1/model': (body) => (++attempts === 1
      ? { status: 428, json: { error: 'requires_credential', provider: 'ollama_cloud_api' } }
      : { json: { model: body.model } }),
    'POST /v1/credentials/ollama-cloud': () => ({ json: { ok: true } }),
  });
  t.ctx.set({ sessionId: 'sess_1' });
  t.ctx.emit('session:ready');
  await flush();
  const select = header(t).querySelector('select');
  assert.ok(select.querySelectorAll('option').some((option) => option.getAttribute('value') === 'kimi-k2.6'));
  select.value = 'kimi-k2.6';
  fire(select, 'change');
  await flush();
  assert.match(t.doc.getElementById('dialog').textContent, /Ollama Cloud API key/);
  t.doc.getElementById('credential-key').value = 'secret';
  fire(t.doc.getElementById('dialog').querySelectorAll('button').find((button) => button.getAttribute('data-action') === 'save'), 'click');
  await flush(); await flush();
  assert.equal(t.ctx.state.model, 'kimi-k2.6');
  assert.equal(attempts, 2);
});
