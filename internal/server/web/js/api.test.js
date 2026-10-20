import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createApi, ApiError } from './api.js';

const res = (status, body) => ({ ok: status >= 200 && status < 300, status, text: async () => body });

test('sends bearer token and JSON body', async () => {
  const seen = [];
  const api = createApi({ getToken: () => 't', fetchImpl: async (p, i) => { seen.push([p, i]); return res(200, '{"a":1}'); } });
  const out = await api.post('/v1/x', { k: 'v' });
  assert.deepEqual(out, { a: 1 });
  assert.equal(seen[0][1].method, 'POST');
  assert.equal(seen[0][1].headers.Authorization, 'Bearer t');
  assert.equal(seen[0][1].headers['Content-Type'], 'application/json');
  assert.equal(seen[0][1].body, '{"k":"v"}');
});

test('no token means no Authorization header; GET has no body', async () => {
  const seen = [];
  const api = createApi({ fetchImpl: async (p, i) => { seen.push(i); return res(200, ''); } });
  assert.equal(await api.get('/v1/x'), null);
  assert.equal(seen[0].headers.Authorization, undefined);
  assert.equal(seen[0].body, undefined);
});

test('non-2xx throws ApiError with status and parsed body', async () => {
  const api = createApi({ fetchImpl: async () => res(428, '{"error":"requires_credential"}') });
  await assert.rejects(api.post('/v1/x'), (e) => e instanceof ApiError && e.status === 428 && e.body.error === 'requires_credential');
});

test('plain-text error bodies are kept as text', async () => {
  const api = createApi({ fetchImpl: async () => res(400, 'model not found\n') });
  await assert.rejects(api.post('/v1/x'), (e) => e.status === 400 && e.text === 'model not found' && e.body === null);
});

test('204 empty response resolves null', async () => {
  const api = createApi({ fetchImpl: async () => res(204, '') });
  assert.equal(await api.post('/v1/x'), null);
});
