import { test } from 'node:test';
import assert from 'node:assert/strict';
import { makeCtx, fire, flush } from '../fakedom.js';
import { mount as mountKeys } from './keys.js';
import { mount as mountDialog } from './dialog.js';
import { mount as mountCredential, needsCredential } from './credential.js';

test('needsCredential recognises 428, the structured body and the error text', () => {
  assert.equal(needsCredential({ status: 428 }), true);
  assert.equal(needsCredential({ status: 400, body: { error: 'requires_credential' } }), true);
  assert.equal(needsCredential({ status: 400, text: 'x requires_credential y' }), true);
  assert.equal(needsCredential({ status: 400, text: 'model not found' }), false);
  assert.equal(needsCredential(null), false);
});

function setup(post) {
  const t = makeCtx({ routes: { 'POST /v1/credentials/ollama-cloud': post } });
  mountKeys(t.ctx); mountDialog(t.ctx); mountCredential(t.ctx);
  return t;
}
const click = (t, action) => fire(t.doc.getElementById('dialog').querySelectorAll('button').find((b) => b.getAttribute('data-action') === action), 'click');

test('saving posts the key once, clears the field, closes and retries', async () => {
  const t = setup(() => ({ json: { ok: true, provider: 'ollama_cloud_api', stored: 'session' } }));
  let retried = 0;
  t.ctx.emit('credential:required', { retry: () => { retried += 1; } });
  t.doc.getElementById('credential-key').value = 'sk-secret';
  click(t, 'save');
  await flush();
  const post = t.calls.find((c) => c.path === '/v1/credentials/ollama-cloud');
  assert.deepEqual(post.body, { key: 'sk-secret', save: false });
  assert.equal(t.doc.getElementById('credential-key'), null, 'dialog closed');
  assert.equal(retried, 1);
});

test('the key is never rendered or shown in toasts, even on failure', async () => {
  const t = setup(() => ({ status: 500, text: 'boom sk-secret' }));
  let retried = 0;
  t.ctx.emit('credential:required', { retry: () => { retried += 1; } });
  t.doc.getElementById('credential-key').value = 'sk-secret';
  click(t, 'save');
  await flush();
  const dialog = t.doc.getElementById('dialog');
  assert.ok(!dialog.textContent.includes('sk-secret'));
  assert.ok(!t.doc.getElementById('toasts').textContent.includes('sk-secret'));
  assert.match(dialog.textContent, /Could not save the key \(HTTP 500\)/);
  assert.equal(t.doc.getElementById('credential-key').value, '', 'field cleared');
  // The retry survives the failed attempt.
  t.doc.getElementById('credential-key').value = 'sk-2';
  t.ctx.api.post = async () => null;
  click(t, 'save');
  await flush();
  assert.equal(retried, 1);
});

test('cancel and Escape drop the retry; empty key complains', async () => {
  const t = setup(() => ({ json: {} }));
  let retried = 0;
  t.ctx.emit('credential:required', { retry: () => { retried += 1; } });
  click(t, 'save');
  assert.match(t.doc.getElementById('dialog').textContent, /Enter the API key first/);
  click(t, 'cancel');
  t.ctx.emit('credential:required', { retry: () => { retried += 1; } });
  t.doc.getElementById('credential-key').value = 'k';
  fire(t.doc, 'keydown', { key: 'Escape' });
  await flush();
  assert.equal(retried, 0);
  assert.equal(t.calls.length, 0);
});

test('keychain persistence is sent when chosen', async () => {
  const t = setup(() => ({ json: {} }));
  t.ctx.emit('credential:required', {});
  t.doc.getElementById('credential-key').value = 'k';
  t.doc.getElementById('credential-persist').value = 'keychain';
  click(t, 'save');
  await flush();
  assert.equal(t.calls[0].body.save, true);
});
