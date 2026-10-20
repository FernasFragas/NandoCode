import { test } from 'node:test';
import assert from 'node:assert/strict';
import { makeCtx, fire, flush, sse } from '../fakedom.js';
import { mount, eventRow } from './rail.js';

function setup() {
  const t = makeCtx();
  mount(t.ctx);
  return t;
}
const rail = (t) => t.doc.getElementById('rail');
const emit = async (t, ...events) => { events.forEach((e) => t.ctx.emit('sse', e)); await flush(); };

test('renders the five sections with honest empty states', () => {
  const t = setup();
  for (const id of ['status', 'usage', 'tasks', 'diagnostics', 'events']) assert.ok(rail(t).querySelector('#rail-' + id), id);
  const text = rail(t).textContent;
  assert.match(text, /No tasks yet/);
  assert.match(text, /No events yet/);
  assert.match(text, /Unavailable/);
});

test('usage shows an em dash until the first turn, then real numbers', async () => {
  const t = setup();
  assert.ok(rail(t).querySelector('.bv-usage__value').textContent === '—');
  await emit(t, sse('terminal', { usage: { PromptEvalCount: 120, EvalCount: 30, TotalDuration: 2e9, Turns: 2, ToolCalls: 1, DoneReason: 'stop' } }));
  const values = rail(t).querySelectorAll('.bv-usage__value').map((v) => v.textContent);
  assert.deepEqual(values.slice(0, 2), ['120', '30']);
  assert.ok(values.includes('2s'));
});

test('usage_update (K2) feeds the same table', async () => {
  const t = setup();
  await emit(t, sse('usage_update', { usage: { PromptEvalCount: 7, EvalCount: 1, TotalDuration: 1e9, Turns: 1, ToolCalls: 0 } }));
  assert.equal(rail(t).querySelector('.bv-usage__value').textContent, '7');
});

test('todos render first, then background tasks (task_lifecycle)', async () => {
  const t = setup();
  await emit(t,
    sse('task_lifecycle', { task_id: 't1', status: 'running', description: 'build index', kind: 'bash' }),
    sse('todo_updated', { todos: [{ id: '1', content: 'write tests', status: 'in_progress', priority: 'high' }] }),
    sse('task_lifecycle', { task_id: 't1', status: 'completed', description: 'build index' }));
  assert.ok(rail(t).querySelector('.bv-tasks__todos'));
  assert.equal(rail(t).querySelectorAll('.bv-task-row').length, 1);
  assert.match(rail(t).querySelector('.bv-task-row').className, /--completed/);
  assert.match(rail(t).textContent, /write tests/);
});

test('recent events keep the last five and skip deltas and heartbeats', async () => {
  const t = setup();
  await emit(t, sse('assistant_text_delta', { content: 'x' }), sse('assistant_thinking_delta', {}), sse('heartbeat'));
  assert.match(rail(t).textContent, /No events yet/);
  const names = ['run_started', 'tool_use_start', 'tool_use_result', 'stage_timing', 'prompt_pack_report', 'terminal'];
  await emit(t, ...names.map((n) => sse(n, { name: 'search' })));
  const rows = rail(t).querySelectorAll('.bv-event-row');
  assert.equal(rows.length, 5);
  assert.match(rows[0].textContent, /tool_use_start search/);
  assert.match(rows[4].textContent, /terminal/);
});

test('eventRow marks failures red and permission requests as warnings', () => {
  assert.equal(eventRow(sse('tool_use_result', { error: 'x' })).kind, 'failed');
  assert.equal(eventRow(sse('error', {})).kind, 'failed');
  assert.equal(eventRow(sse('permission_request', { tool_name: 'a' })).kind, 'warning');
  assert.equal(eventRow(sse('stage_timing', {})).kind, 'info');
});

test('status rows follow session state and pending permissions', async () => {
  const t = setup();
  t.ctx.set({ sessionId: 'sess_1', model: 'qwen3', running: true, connection: 'connected', permissionPending: 2 });
  await flush();
  const text = rail(t).textContent;
  for (const want of ['sess_1', 'qwen3', 'Running', 'Connected', '2']) assert.ok(text.includes(want), want);
});

test('diagnostics use /v1/health and the heartbeat clock', async () => {
  const t = setup();
  t.ctx.emit('health', { status: 'ok', ollama: 'reachable' });
  t.ctx.emit('sse:heartbeat', { time: Date.UTC(2026, 0, 1, 12, 30, 45) });
  await flush();
  assert.match(rail(t).textContent, /reachable/);
  assert.match(rail(t).textContent, /12:30:45/);
  t.ctx.emit('health', { status: 'unreachable' });
  await flush();
  assert.match(rail(t).textContent, /unreachable/);
});

test('rail:focus focuses a section; reset clears the data', async () => {
  const t = setup();
  t.ctx.emit('rail:focus', 'tasks');
  assert.equal(t.doc.activeElement.getAttribute('id'), 'rail-tasks');
  await emit(t, sse('error', { message: 'x' }));
  t.ctx.emit('session:reset');
  await flush();
  assert.match(rail(t).textContent, /No events yet/);
});

test('pending count follows permission events and pending store state', async () => {
  const t = setup();
  await emit(t, sse('permission_request', { request_id: 'r1' }), sse('permission_request', { request_id: 'r2' }));
  assert.match(rail(t).textContent, /Permissions pending\s*2/);
  await emit(t, sse('permission_resolved', { request_id: 'r1', decision: 'allow' }));
  assert.match(rail(t).textContent, /Permissions pending\s*1/);
  t.ctx.set({ permissionPending: 3 });
  await flush();
  assert.match(rail(t).textContent, /Permissions pending\s*3/);
});

test('index diagnostics read the session status endpoint', async () => {
  const t = makeCtx({ routes: { 'GET /v1/sessions/s1/index/status': () => ({ json: { exists: true, compatible: true, enabled: true, files: 4, records: 9, running: false } }) } });
  mount(t.ctx);
  t.ctx.set({ sessionId: 's1' });
  await flush();
  assert.ok(t.calls.some((c) => c.path === '/v1/sessions/s1/index/status'));
  assert.match(rail(t).textContent, /4 files/);
});

test('pending permission link emits permission:open on the bus', async () => {
  const t = setup();
  let opened = 0;
  t.ctx.on('permission:open', () => { opened++; });
  await emit(t, sse('permission_request', { request_id: 'r1' }));
  fire(rail(t).querySelector('[data-action="focus-permission"]'), 'click');
  assert.equal(opened, 1);
});

const statusRoute = (fn) => ({ routes: { 'GET /v1/sessions/s1/index/status': fn } });

test('a burst of index_progress events sends at most two status requests', async () => {
  const t = makeCtx(statusRoute(() => ({ json: { exists: true, compatible: true, files: 1, records: 1 } })));
  mount(t.ctx);
  t.ctx.set({ sessionId: 's1' });
  await flush();
  const before = t.calls.length;
  await emit(t, ...Array.from({ length: 50 }, () => sse('index_progress', { done: 1 })));
  assert.ok(t.calls.length - before <= 2, 'requests: ' + (t.calls.length - before));
  await emit(t, sse('index_done', {}));
  assert.ok(t.calls.length - before <= 3);
});

test('a stale index status response is ignored after the session changes', async () => {
  let release;
  const gate = new Promise((r) => { release = r; });
  const t = makeCtx(statusRoute(async () => { await gate; return { json: { exists: true, compatible: true, files: 99, records: 1 } }; }));
  mount(t.ctx);
  t.ctx.set({ sessionId: 's1' });
  await flush();
  t.ctx.set({ sessionId: 's2' });
  release();
  await flush();
  assert.ok(!rail(t).textContent.includes('99 files'));
});

test('duplicate or out-of-order permission events never make the count negative or stuck', async () => {
  const t = setup();
  await emit(t, sse('permission_resolved', { request_id: 'r1', decision: 'allow' }));
  assert.match(rail(t).textContent, /Permissions pending\s*0/);
  await emit(t, sse('permission_request', { request_id: 'r2' }), sse('permission_request', { request_id: 'r2' }));
  assert.match(rail(t).textContent, /Permissions pending\s*1/);
  await emit(t, sse('permission_resolved', { request_id: 'r2', decision: 'deny' }), sse('permission_resolved', { request_id: 'r2', decision: 'deny' }));
  assert.match(rail(t).textContent, /Permissions pending\s*0/);
});

test('usage_update keys are case-sensitive', async () => {
  const t = setup();
  await emit(t, sse('usage_update', { usage: { promptevalcount: 5, evalcount: 6, turns: 3 } }));
  assert.equal(rail(t).querySelector('.bv-usage__value').textContent, '—');
  await emit(t, sse('usage_update', { usage: { PromptEvalCount: 11, EvalCount: 2, TotalDuration: 1e9, Turns: 1, ToolCalls: 0 } }));
  assert.equal(rail(t).querySelector('.bv-usage__value').textContent, '11');
});

test('todo_updated renders the todos and replaces the previous list', async () => {
  const t = setup();
  await emit(t, sse('todo_updated', { todos: [{ id: '1', content: 'first item', status: 'pending', priority: 'low' }] }));
  assert.match(rail(t).textContent, /first item/);
  await emit(t, sse('todo_updated', { todos: [{ id: '2', content: 'second item', status: 'completed', priority: 'high' }] }));
  assert.ok(!rail(t).textContent.includes('first item'));
  assert.match(rail(t).textContent, /second item/);
});

test('session_cleared empties the todo list', async () => {
  const t = setup();
  await emit(t, sse('todo_updated', { todos: [{ id: '1', content: 'stale item', status: 'pending', priority: 'low' }] }));
  assert.match(rail(t).textContent, /stale item/);
  await emit(t, sse('session_cleared', {}));
  assert.ok(!rail(t).textContent.includes('stale item'));
});

test('task_lifecycle merges updates per task_id', async () => {
  const t = setup();
  await emit(t,
    sse('task_lifecycle', { task_id: 'a', status: 'running', description: 'job A', kind: 'bash' }),
    sse('task_lifecycle', { task_id: 'b', status: 'running', description: 'job B', kind: 'bash' }),
    sse('task_lifecycle', { task_id: 'a', status: 'completed' }));
  const rows = rail(t).querySelectorAll('.bv-task-row');
  assert.equal(rows.length, 2);
  assert.match(rail(t).textContent, /job A/);
  assert.match(rows[0].className, /--completed/);
});

test('Last event appears once', async () => {
  const t = setup();
  assert.equal((rail(t).textContent.match(/Last event/g) || []).length, 1);
});
