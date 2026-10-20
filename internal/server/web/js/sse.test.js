import { test } from 'node:test';
import assert from 'node:assert/strict';
import { parseSSE, createEventStream } from './sse.js';

test('parseSSE splits blocks and keeps id, event and data', () => {
  const { events, remaining } = parseSSE('id: 1\nevent: a\ndata: {"x":1}\n\nid: 2\nevent: b\ndata: {"y"');
  assert.equal(events.length, 1);
  assert.deepEqual(events[0], { id: '1', event: 'a', data: '{"x":1}' });
  assert.equal(remaining, 'id: 2\nevent: b\ndata: {"y"');
});

test('parseSSE handles CRLF and multi-line data', () => {
  const { events } = parseSSE('id: 9\r\ndata: one\r\ndata: two\r\n\r\n');
  assert.deepEqual(events, [{ id: '9', event: '', data: 'one\ntwo' }]);
});

test('parseSSE skips blocks without data', () => {
  assert.equal(parseSSE(': comment\n\n').events.length, 0);
});

const enc = new TextEncoder();
function streamBody(chunks, { failAfter = false } = {}) {
  let i = 0;
  return {
    getReader() {
      return {
        async read() {
          if (i < chunks.length) return { done: false, value: enc.encode(chunks[i++]) };
          if (failAfter) throw new Error('reset');
          return { done: true };
        },
        async cancel() {},
      };
    },
  };
}

function harness(responses) {
  const calls = [];
  const timers = [];
  let n = 0;
  const fetchImpl = async (url, init) => {
    calls.push({ url, headers: init.headers });
    const r = responses[Math.min(n++, responses.length - 1)];
    if (r instanceof Error) throw r;
    return r;
  };
  const events = [];
  const statuses = [];
  const stream = createEventStream({
    url: '/v1/sessions/s1/events',
    getToken: () => 'tok',
    fetchImpl,
    onEvent: (e) => events.push(e),
    onStatus: (s) => statuses.push(s),
    setTimer: (fn, ms) => { timers.push({ fn, ms }); return timers.length; },
    clearTimer: () => {},
  });
  return { calls, timers, events, statuses, stream };
}
const ok = (chunks, opts) => ({ ok: true, status: 200, body: streamBody(chunks, opts) });
const flush = () => new Promise((r) => setImmediate(r));

test('first connect sends the bearer token and no Last-Event-ID', async () => {
  const t = harness([ok(['id: 1\nevent: run_started\ndata: {"type":"run_started"}\n\n'])]);
  t.stream.start();
  await flush();
  assert.equal(t.calls[0].headers.Authorization, 'Bearer tok');
  assert.equal(t.calls[0].headers['Last-Event-ID'], undefined);
  assert.equal(t.events.length, 1);
  assert.equal(t.events[0].id, '1');
  assert.equal(t.statuses[0], 'connecting');
  assert.ok(t.statuses.includes('connected'));
});

test('reconnect sends Last-Event-ID and drops already-seen ids', async () => {
  const t = harness([
    ok(['id: 1\ndata: {"n":1}\n\nid: 2\ndata: {"n":2}\n\n']),
    ok(['id: 2\ndata: {"n":2}\n\nid: 3\ndata: {"n":3}\n\n']),
  ]);
  t.stream.start();
  await flush();
  assert.equal(t.timers.length, 1, 'a reconnect is scheduled when the stream ends');
  assert.equal(t.statuses.at(-1), 'connecting');
  t.timers[0].fn();
  await flush();
  assert.equal(t.calls.length, 2);
  assert.equal(t.calls[1].headers['Last-Event-ID'], '2');
  assert.deepEqual(t.events.map((e) => e.id), ['1', '2', '3']);
});

test('heartbeat ids are never used as Last-Event-ID', async () => {
  const t = harness([
    ok(['id: 5\ndata: {"n":5}\n\nid: hb-99\nevent: heartbeat\ndata: {"type":"heartbeat"}\n\n']),
    ok([]),
  ]);
  t.stream.start();
  await flush();
  t.timers[0].fn();
  await flush();
  assert.equal(t.calls[1].headers['Last-Event-ID'], '5');
  assert.equal(t.events.filter((e) => e.event === 'heartbeat').length, 1);
});

test('backoff doubles up to the cap and resets after a successful connect', async () => {
  const t = harness([new Error('down'), new Error('down'), new Error('down'), ok(['id: 1\ndata: {}\n\n']), new Error('down')]);
  t.stream.start();
  await flush();
  t.timers[0].fn(); await flush();
  t.timers[1].fn(); await flush();
  t.timers[2].fn(); await flush();
  assert.deepEqual(t.timers.slice(0, 3).map((x) => x.ms), [1000, 2000, 4000]);
  assert.equal(t.timers[3].ms, 1000, 'reset after success');
});

test('a 404 stops reconnecting and a 401 reports unauthorized', async () => {
  const gone = harness([{ ok: false, status: 404, body: null }]);
  gone.stream.start();
  await flush();
  assert.equal(gone.timers.length, 0);
  assert.equal(gone.statuses.at(-1), 'disconnected');

  const denied = harness([{ ok: false, status: 401, body: null }]);
  denied.stream.start();
  await flush();
  assert.equal(denied.timers.length, 0);
  assert.equal(denied.statuses.at(-1), 'unauthorized');
});

test('stop cancels, prevents reconnects and reports disconnected', async () => {
  const t = harness([ok(['id: 1\ndata: {}\n\n'])]);
  t.stream.start();
  await flush();
  t.stream.stop();
  assert.equal(t.statuses.at(-1), 'disconnected');
  t.timers.forEach((x) => x.fn());
  await flush();
  assert.equal(t.calls.length, 1);
});
