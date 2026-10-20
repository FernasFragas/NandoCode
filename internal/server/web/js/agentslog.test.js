import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createAgentsLogStore } from './agentslog.js';
import { createEventStore } from './eventstore.js';

const event = (id, type, data) => ({ id, type, time: '2026-01-02T03:04:05Z', data });

test('selected agent filters activity and keeps the real SSE event ids', () => {
  const agents = createAgentsLogStore();
  const recent = createEventStore({ limit: 20 });
  const stream = [
    event('1', 'subagent_started', { task_id: 'a', name: 'Explorer', task: 'inspect' }),
    event('2', 'subagent_started', { task_id: 'b', name: 'Builder', task: 'build' }),
    event('3', 'subagent_progress', { task_id: 'a', tool: 'grep', target: 'src', status: 'completed' }),
    event('4', 'subagent_message', { from: 'a', to: 'b', summary: 'Found it', at: '2026-01-02T03:04:05Z' }),
    event('5', 'subagent_finished', { task_id: 'b', status: 'completed' }),
  ];
  for (const msg of stream) { agents.add(msg); recent.add(msg); }
  assert.deepEqual(agents.list().map((r) => r.id), recent.list().filter((r) => r.type.startsWith('subagent_')).map((r) => r.id));
  assert.deepEqual(agents.list('a').map((r) => r.id), ['1', '3', '4']);
  assert.deepEqual(agents.list('b').map((r) => r.id), ['2', '4', '5']);
  assert.equal(agents.workers().length, 2);
  agents.reset();
  assert.deepEqual(agents.list(), []);
  assert.deepEqual(agents.workers(), []);
});

test('session snapshot restores workers and later live events update them', () => {
  const store = createAgentsLogStore();
  store.setWorkers([{ task_id: 'a', name: 'Explorer', status: 'running', started_at: '2026-01-02T03:04:05Z' }]);
  store.add(event('6', 'subagent_usage', { task_id: 'a', usage: { EvalCount: 10 }, files_touched: 2 }));
  store.add(event('7', 'subagent_finished', { task_id: 'a', status: 'completed', finished_at: '2026-01-02T03:05:05Z' }));
  assert.deepEqual(store.workers()[0].usage, { EvalCount: 10 });
  assert.equal(store.workers()[0].status, 'completed');
  assert.equal(store.workers()[0].files_touched, 2);
});

test('a late snapshot cannot regress a worker already finished by SSE replay', () => {
  const store = createAgentsLogStore();
  store.add(event('1', 'subagent_started', { task_id: 'a', name: 'Explorer' }));
  store.add(event('2', 'subagent_finished', { task_id: 'a', status: 'completed' }));
  store.setWorkers([{ task_id: 'a', name: 'Explorer', status: 'running' }]);
  assert.equal(store.workers()[0].status, 'completed');
  const second = createAgentsLogStore();
  second.add(event('1', 'subagent_started', { task_id: 'a', name: 'Explorer' }));
  second.setWorkers([{ task_id: 'a', name: 'Explorer', status: 'completed' }]);
  assert.equal(second.workers()[0].status, 'completed');
});

// ---- review fixes (V8b.review-claude.md) ----
test('killed is terminal: started, finished(killed), snapshot in both orders', () => {
  const a = createAgentsLogStore();
  a.add(event('1', 'subagent_started', { task_id: 'a', name: 'Explorer', parent: 'coordinator' }));
  a.add(event('2', 'subagent_finished', { task_id: 'a', status: 'killed' }));
  a.setWorkers([{ task_id: 'a', name: 'Explorer', status: 'running' }]);
  assert.equal(a.workers()[0].status, 'killed');
  const b = createAgentsLogStore();
  b.add(event('1', 'subagent_started', { task_id: 'a', name: 'Explorer', parent: 'coordinator' }));
  b.setWorkers([{ task_id: 'a', name: 'Explorer', status: 'killed' }]);
  assert.equal(b.workers()[0].status, 'killed');
  const c = createAgentsLogStore();
  c.setWorkers([{ task_id: 'a', name: 'Explorer', status: 'killed' }]);
  c.add(event('1', 'subagent_started', { task_id: 'a', name: 'Explorer' }));
  assert.equal(c.workers()[0].status, 'killed');
});

test('duplicate events (same id, or same finished) do not add rows', () => {
  const s = createAgentsLogStore();
  s.add(event('1', 'subagent_started', { task_id: 'a', name: 'Explorer' }));
  s.add(event('2', 'subagent_finished', { task_id: 'a', status: 'completed' }));
  assert.equal(s.add(event('2', 'subagent_finished', { task_id: 'a', status: 'completed' })), false);
  s.add(event('9', 'subagent_finished', { task_id: 'a', status: 'completed' }));
  assert.equal(s.list().length, 2);
});

test('no progress, usage or message events after finished', () => {
  const s = createAgentsLogStore();
  s.add(event('1', 'subagent_started', { task_id: 'a', name: 'Explorer' }));
  s.add(event('2', 'subagent_finished', { task_id: 'a', status: 'failed', error: 'x' }));
  s.add(event('3', 'subagent_progress', { task_id: 'a', tool: 'grep' }));
  s.add(event('4', 'subagent_usage', { task_id: 'a', files_touched: 5 }));
  assert.equal(s.list().length, 2);
  assert.equal(s.workers()[0].status, 'failed');
  assert.equal(s.workers()[0].files_touched, undefined);
});

test('per-agent filter matches messages by task_id and by worker name', () => {
  const s = createAgentsLogStore();
  s.add(event('1', 'subagent_started', { task_id: 't-1', name: 'explorer', parent: 'coordinator' }));
  s.add(event('2', 'subagent_started', { task_id: 't-2', name: 'builder', parent: 'coordinator' }));
  s.add(event('3', 'subagent_message', { from: 'coordinator', to: 'explorer', summary: 'go', at: '2026-01-02T03:04:05Z' }));
  s.add(event('4', 'subagent_message', { from: 't-2', to: 'coordinator', summary: 'done' }));
  s.add(event('5', 'subagent_message', { from: 'main', to: 't-1', summary: 'hi' }));
  assert.deepEqual(s.list('t-1').map((r) => r.id), ['1', '3', '5']);
  assert.deepEqual(s.list('t-2').map((r) => r.id), ['2', '4']);
});

test('summaries are stripped of control characters and capped at 200 runes', () => {
  const s = createAgentsLogStore();
  s.add(event('1', 'subagent_message', { from: 'coordinator', to: 'x', summary: 'a\u0007b' + '界'.repeat(100000) }));
  const row = s.list()[0];
  assert.ok(!/[\u0000-\u001f]/.test(row.text));
  assert.ok([...row.text].length <= 200);
  assert.ok([...String(row.data.summary)].length <= 200);
});

test('the worker map is bounded and evicts the oldest finished worker', () => {
  const s = createAgentsLogStore();
  for (let i = 0; i < 205; i++) {
    s.add(event('s' + i, 'subagent_started', { task_id: 'w' + i, name: 'n' + i, started_at: String(1000 + i) }));
    if (i < 5) s.add(event('f' + i, 'subagent_finished', { task_id: 'w' + i, status: 'completed' }));
  }
  const ids = s.workers().map((w) => w.task_id);
  assert.equal(ids.length, 200);
  assert.ok(!ids.includes('w0'));
  assert.ok(ids.includes('w204'));
});

test('rows share the V3b eventRow shape', () => {
  const s = createAgentsLogStore();
  const msg = event('1', 'subagent_started', { task_id: 'a', name: 'Explorer' });
  s.add(msg);
  const row = s.list()[0];
  for (const k of ['id', 'type', 'data', 'time', 'label', 'kind']) assert.ok(k in row, k);
  assert.equal(row.label, 'subagent_started');
});
