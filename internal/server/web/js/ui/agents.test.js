import { test } from 'node:test';
import assert from 'node:assert/strict';

import { renderWorkerRow, renderAgentNode, renderAggregationNode, renderLogRow, renderAgentsView } from './agents.js';

const walk = (t, f) => { if (t && typeof t === 'object') { f(t); t.children.forEach((c) => walk(c, f)); } };
const find = (t, p) => { const r = []; walk(t, (n) => { if (p(n)) r.push(n); }); return r; };
const texts = (t) => { const r = []; const go = (n) => { if (typeof n === 'string') r.push(n); else if (n) n.children.forEach(go); }; go(t); return r.join(' '); };
const HOSTILE = '"><script>';
const noScript = (t) => assert.equal(find(t, (n) => n.tag === 'script').length, 0);

// K2-shaped literals: subagent_* events do not exist yet.
const started = { task_id: 'w1', name: 'Explorer', task: 'scan repo', model: 'qwen', parent: '', started_at: '2026-01-02T03:04:05Z' };

test('empty state: no workers reads the honest message and hides the graph', () => {
  for (const d of [undefined, {}, { workers: [] }]) {
    const t = renderAgentsView(d);
    assert.match(texts(t), /No sub-agents in this session yet/);
    assert.equal(find(t, (n) => /bv-agents__graph/.test(n.attrs.class || '')).length, 0);
  }
});

test('worker row renders K2 subagent_started shape with status', () => {
  const t = renderWorkerRow({ ...started, status: 'running' });
  assert.match(texts(t), /Explorer/);
  assert.equal(t.attrs['data-id'], 'w1');
  assert.match(t.attrs.class, /bv-worker-row--running/);
});

test('node omits unknown lines instead of placeholders', () => {
  const bare = renderAgentNode({ task_id: 'w1', name: 'Explorer', status: 'running' });
  assert.equal(find(bare, (n) => /bv-agent-node__(step|files|tokens)/.test(n.attrs.class || '')).length, 0);
  assert.ok(!/—|\b0\b|undefined|null/.test(texts(bare)));
  const full = renderAgentNode({ task_id: 'w1', name: 'Explorer', status: 'completed', step: 'Grep src', files_touched: 3, usage: { PromptEvalCount: 10, EvalCount: 5 }, started_at: started.started_at, finished_at: '2026-01-02T03:04:35Z' });
  const s = texts(full);
  assert.match(s, /Grep src/);
  assert.match(s, /3 files/);
  assert.match(s, /15 tokens/);
  assert.match(s, /30s/);
});

test('node: hostile fields stay text and status never becomes a class injection', () => {
  const t = renderAgentNode({ task_id: HOSTILE, name: HOSTILE, status: HOSTILE, step: HOSTILE });
  noScript(t);
  assert.ok(texts(t).includes(HOSTILE));
  assert.ok(!/script/.test(t.attrs.class));
  assert.ok(find(t, (n) => /script/.test(n.attrs.class || '')).length === 0);
});

test('aggregation node counts completed workers', () => {
  const t = renderAggregationNode({ workers: [{ status: 'completed' }, { status: 'running' }, { status: 'failed' }] });
  assert.match(texts(t), /1 of 3/);
  assert.match(texts(renderAggregationNode({ workers: [] })), /0 of 0/);
  assert.match(texts(renderAggregationNode()), /0 of 0/);
});

test('log row: dot, time, agent, text; hostile stays text', () => {
  const t = renderLogRow({ time: '2026-01-02T03:04:05Z', agent: HOSTILE, text: HOSTILE, kind: 'failed' });
  noScript(t);
  assert.ok(texts(t).includes(HOSTILE));
  assert.match(texts(t), /03:04:05/);
  assert.equal(find(t, (n) => /bv-log-row__dot--failed/.test(n.attrs.class || '')).length, 1);
});

test('agents view with workers shows list, graph, aggregation and log', () => {
  const t = renderAgentsView({ workers: [{ ...started, status: 'completed' }, { ...started, task_id: 'w2', status: 'completed' }], log: [{ time: started.started_at, agent: 'Explorer', text: 'done' }] });
  assert.equal(find(t, (n) => /bv-agents__graph/.test(n.attrs.class || '')).length, 1);
  assert.equal(find(t, (n) => /bv-worker-row\b/.test(n.attrs.class || '')).length, 2);
  assert.equal(find(t, (n) => /bv-log-row\b/.test(n.attrs.class || '')).length, 1);
  assert.match(texts(t), /2 of 2/);
});

test('aggregation appears only after two workers have finished', () => {
  const one = renderAgentsView({ workers: [{ ...started, status: 'completed' }, { ...started, task_id: 'w2', status: 'running' }] });
  assert.equal(find(one, (n) => n.attrs.class === 'bv-agg-node').length, 0);
  const two = renderAgentsView({ workers: [{ ...started, status: 'completed' }, { ...started, task_id: 'w2', status: 'completed' }] });
  assert.equal(find(two, (n) => n.attrs.class === 'bv-agg-node').length, 1);
});

test('killed is a known status, not the unknown class', () => {
  assert.match(renderAgentNode({ task_id: 'a', name: 'x', status: 'killed' }).attrs.class, /--killed/);
  assert.match(renderWorkerRow({ task_id: 'a', name: 'x', status: 'killed' }).attrs.class, /--killed/);
});

test('root labels follow the real parent: main, coordinator, nested task_id', () => {
  const main = renderAgentsView({ workers: [{ task_id: 'a', name: 'A', status: 'running', parent: 'main' }] });
  assert.match(texts(main), /Main agent/);
  assert.doesNotMatch(texts(main), /Coordinator/);
  const coord = renderAgentsView({ workers: [{ task_id: 'a', name: 'A', status: 'running', parent: 'coordinator' }] });
  assert.match(texts(coord), /Coordinator/);
  assert.doesNotMatch(texts(coord), /Main agent/);
  const nested = renderAgentsView({ workers: [
    { task_id: 'a', name: 'A', status: 'running', parent: 'main' },
    { task_id: 'b', name: 'B', status: 'running', parent: 'a' },
  ] });
  assert.equal(find(nested, (n) => /bv-agents__root/.test(n.attrs.class || '')).length, 1);
  const bNode = find(nested, (n) => /bv-agent-node/.test(n.attrs.class || '') && n.attrs['data-id'] === 'b')[0];
  assert.equal(bNode.attrs['data-parent'], 'a');
});

test('empty state mentions both coordinator workers and normal sub-agents', () => {
  const s = texts(renderAgentsView({}));
  assert.match(s, /coordinator/i);
  assert.match(s, /Agent tool|main agent/i);
});
