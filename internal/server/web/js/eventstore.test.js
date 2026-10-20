import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createEventStore } from './eventstore.js';
import { makeCtx, flush } from './fakedom.js';
import { mount as mountTranscript } from './features/transcript.js';

const fixture = JSON.parse(readFileSync(new URL('../dev/fixtures/run-todo.json', import.meta.url), 'utf8'));

test('recent event store excludes deltas and heartbeats and caps at five', () => {
  const store = createEventStore();
  for (const type of ['heartbeat', 'assistant_text_delta', 'assistant_thinking_delta']) {
    assert.equal(store.add({ id: type, type }), false);
  }
  for (let n = 0; n < 7; n++) store.add({ id: String(n), type: 'stage_timing', time: '2026-01-01T00:00:00Z' });
  assert.deepEqual(store.list().map((row) => row.id), ['2', '3', '4', '5', '6']);
  assert.equal(store.lastEvent(), 'stage_timing');
  store.reset();
  assert.deepEqual(store.list(), []);
  assert.equal(store.lastEvent(), '');
});

test('fixture tool event ids agree with transcript tool cards in arrival order', async () => {
  const t = makeCtx();
  mountTranscript(t.ctx);
  const store = createEventStore({ limit: fixture.length });
  for (const msg of fixture) { store.add(msg); t.ctx.emit('sse', msg); }
  await flush();
  assert.deepEqual(store.list().map((row) => row.id), fixture.filter((msg) => !['heartbeat', 'assistant_text_delta', 'assistant_thinking_delta'].includes(msg.type)).map((msg) => msg.id));
  const fromStore = store.list().filter((row) => row.type === 'tool_use_start').map((row) => row.data.id || row.data.tool_call_id);
  const fromTranscript = t.doc.getElementById('transcript').querySelectorAll('.bv-tool').map((el) => el.getAttribute('data-id'));
  assert.deepEqual(fromStore, fromTranscript);
});

test('mutating list() output leaves the store unchanged', () => {
  const store = createEventStore();
  store.add({ id: '1', type: 'stage_timing' });
  const out = store.list();
  out.push({ id: 'x' });
  out.length = 0;
  assert.deepEqual(store.list().map((r) => r.id), ['1']);
});

test('an invalid limit falls back to 5', () => {
  for (const limit of [0, -1, 2.5, 'x', NaN]) {
    const store = createEventStore({ limit });
    for (let n = 0; n < 8; n++) store.add({ id: String(n), type: 'stage_timing' });
    assert.equal(store.list().length, 5, String(limit));
  }
});
