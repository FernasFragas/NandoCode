import { test } from 'node:test';
import assert from 'node:assert/strict';

import { readFileSync } from 'node:fs';
import { renderStatusRow, renderUsage, renderTodoRow, renderTodoList, renderTaskRow, renderEventRow } from './rail.js';

const walk = (t, f) => { if (t && typeof t === 'object') { f(t); t.children.forEach((c) => walk(c, f)); } };
const find = (t, p) => { const r = []; walk(t, (n) => { if (p(n)) r.push(n); }); return r; };
const texts = (t) => { const r = []; const go = (n) => { if (typeof n === 'string') r.push(n); else if (n) n.children.forEach(go); }; go(t); return r.join(' '); };
const HOSTILE = '"><script>';
const noScript = (t) => assert.equal(find(t, (n) => n.tag === 'script').length, 0);

const fixture = (name) => JSON.parse(readFileSync(new URL('../../dev/fixtures/' + name, import.meta.url), 'utf8'));

test('status row renders label, value and dot; hostile strings stay text', () => {
  const t = renderStatusRow({ label: 'Model', value: 'qwen', dot: 'success' });
  assert.match(texts(t), /Model/);
  assert.match(texts(t), /qwen/);
  assert.equal(find(t, (n) => /bv-status-row__dot--success/.test(n.attrs.class || '')).length, 1);
  const x = renderStatusRow({ label: HOSTILE, value: HOSTILE, dot: HOSTILE });
  noScript(x);
  assert.ok(texts(x).includes(HOSTILE));
  assert.ok(!/<script/.test(JSON.stringify(x.attrs)));
});

test('status row empty value renders an em dash', () => {
  assert.match(texts(renderStatusRow({ label: 'Index' })), /—/);
});

test('usage before the first turn shows em dash per row, never 0', () => {
  for (const u of [null, undefined, {}, { Turns: 0, PromptEvalCount: 0, EvalCount: 0 }]) {
    const t = renderUsage(u);
    const vals = find(t, (n) => /bv-usage__value/.test(n.attrs.class || ''));
    assert.equal(vals.length, 5);
    assert.ok(vals.every((v) => texts(v) === '—'));
  }
});

test('usage renders from the terminal fixture shape', () => {
  const term = fixture('run-todo.json').find((e) => e.type === 'terminal');
  const t = renderUsage(term.data.usage);
  assert.match(texts(t), /Turns/);
  assert.match(texts(t), /3/);
  assert.match(texts(t), /Tool calls/);
});

test('usage formats tokens and duration (K2 usage_update literal)', () => {
  const t = renderUsage({ PromptEvalCount: 1234, EvalCount: 56, TotalDuration: 2500000000, Turns: 2, ToolCalls: 4, DoneReason: 'stop' });
  const s = texts(t);
  assert.match(s, /1234/);
  assert.match(s, /56/);
  assert.match(s, /2\.5s/);
});

test('todo row: status drives class; hostile content stays text', () => {
  const t = renderTodoRow({ id: 'todo-1', content: HOSTILE, status: 'in_progress', priority: 'high' });
  noScript(t);
  assert.ok(texts(t).includes(HOSTILE));
  assert.match(t.attrs.class, /bv-todo-row--in_progress/);
  assert.equal(t.attrs['data-id'], 'todo-1');
  const bad = renderTodoRow({ id: 'x', content: 'a', status: HOSTILE });
  assert.ok(!/script/.test(bad.attrs.class));
});

test('todo row renders from fixture TodoWrite input', () => {
  const start = fixture('run-todo.json').find((e) => e.data && e.data.name === 'TodoWrite');
  const rows = start.data.input.todos.map((x) => renderTodoRow(x));
  assert.match(texts(rows[0]), /Review src\/main.go/);
});

test('tasks section: empty reads No tasks yet; todos first, tasks below', () => {
  assert.match(texts(renderTodoList({ todos: [], tasks: [] })), /No tasks yet/);
  assert.match(texts(renderTodoList({})), /No tasks yet/);
  const t = renderTodoList({
    todos: [{ id: 't1', content: 'todo one', status: 'pending' }],
    tasks: [{ task_id: 'k1', kind: 'bash', status: 'running', description: 'sleep' }],
  });
  const s = texts(t);
  assert.ok(s.indexOf('todo one') < s.indexOf('sleep'));
  assert.ok(!/No tasks yet/.test(s));
});

test('task row renders task_lifecycle payload; hostile text stays text', () => {
  const t = renderTaskRow({ task_id: 'k1', kind: HOSTILE, status: 'completed', description: HOSTILE, output_file: '' });
  noScript(t);
  assert.ok(texts(t).includes(HOSTILE));
  assert.match(t.attrs.class, /bv-task-row--completed/);
  assert.equal(t.attrs['data-id'], 'k1');
  assert.match(texts(renderTaskRow({ task_id: 'k2' })), /k2/);
});

test('event row: dot, time, label; hostile label stays text; missing time omitted', () => {
  const t = renderEventRow({ time: '2026-01-02T03:04:05Z', label: HOSTILE, kind: 'failed' });
  noScript(t);
  assert.ok(texts(t).includes(HOSTILE));
  assert.match(texts(t), /03:04:05/);
  assert.equal(find(t, (n) => /bv-event-row__dot--failed/.test(n.attrs.class || '')).length, 1);
  assert.equal(find(renderEventRow({ label: 'x' }), (n) => /bv-event-row__time/.test(n.attrs.class || '')).length, 0);
});
