// Agents tab components (DESIGN-SPEC "Agents graph", BV-8b). Pure render
// functions. Worker data is K2-shaped (subagent_started/progress/usage/finished).
import { h, icon } from '../dom.js';
import { _clock as clock } from './rail.js';

const STATUSES = ['running', 'completed', 'failed', 'killed', 'cancelled', 'pending'];
const KINDS = ['info', 'success', 'warning', 'failed', 'running', 'muted'];
const status = (s) => (STATUSES.includes(s) ? s : 'unknown');
const present = (v) => v !== null && v !== undefined && v !== '';
const str = (v) => (present(v) ? String(v) : '');

// worker: {task_id, name, task, model, status}.
export function renderWorkerRow(worker = {}) {
  return h('div', {
    class: 'bv-worker-row bv-worker-row--' + status(worker.status),
    'data-action': 'select-worker',
    'data-id': present(worker.task_id) ? String(worker.task_id) : null,
  },
  icon('agents'),
  h('span', { class: 'bv-worker-row__name' }, str(worker.name)),
  present(worker.task) ? h('span', { class: 'bv-worker-row__task' }, String(worker.task)) : null,
  present(worker.status) ? h('span', { class: 'bv-worker-row__status' }, String(worker.status)) : null);
}

function elapsed(a, b) {
  const t0 = Date.parse(a);
  const t1 = Date.parse(b);
  if (!Number.isFinite(t0) || !Number.isFinite(t1) || t1 < t0) return null;
  const s = Math.round((t1 - t0) / 1000);
  return s < 60 ? s + 's' : Math.floor(s / 60) + 'm ' + (s % 60) + 's';
}

// node: worker plus optional step, files_touched, usage, started_at, finished_at.
// Unknown fields omit their line; no placeholder numbers.
export function renderAgentNode(node = {}) {
  const u = node.usage;
  const tokens = u ? (Number(u.PromptEvalCount) || 0) + (Number(u.EvalCount) || 0) : 0;
  const time = elapsed(node.started_at, node.finished_at);
  const files = Number(node.files_touched);
  return h('div', {
    class: 'bv-agent-node bv-agent-node--' + status(node.status),
    'data-action': 'select-worker',
    'data-id': present(node.task_id) ? String(node.task_id) : null,
    'data-parent': present(node.parent) && node.parent !== 'main' && node.parent !== 'coordinator' ? String(node.parent) : null,
  },
  h('div', { class: 'bv-agent-node__head' },
    icon('agents'),
    h('span', { class: 'bv-agent-node__name' }, str(node.name)),
    present(node.status) ? h('span', { class: 'bv-agent-node__status' }, String(node.status)) : null,
    time ? h('span', { class: 'bv-agent-node__time' }, time) : null),
  present(node.step) ? h('div', { class: 'bv-agent-node__step' }, String(node.step)) : null,
  files > 0 ? h('div', { class: 'bv-agent-node__files' }, files + (files === 1 ? ' file' : ' files')) : null,
  tokens > 0 ? h('div', { class: 'bv-agent-node__tokens' }, tokens + ' tokens') : null);
}

// data: {workers}. Shows how many workers have completed.
export function renderAggregationNode(data = {}) {
  const workers = Array.isArray(data.workers) ? data.workers : [];
  const done = workers.filter((w) => w && w.status === 'completed').length;
  return h('div', { class: 'bv-agg-node' },
    icon('check'),
    h('span', { class: 'bv-agg-node__label' }, 'Aggregation'),
    h('span', { class: 'bv-agg-node__count' }, done + ' of ' + workers.length + ' completed'));
}

// entry: {time, agent, text, kind}.
export function renderLogRow(entry = {}) {
  const time = clock(entry.time);
  return h('div', { class: 'bv-log-row' },
    h('span', { class: 'bv-log-row__dot bv-log-row__dot--' + (KINDS.includes(entry.kind) ? entry.kind : 'unknown') }),
    time ? h('span', { class: 'bv-log-row__time' }, time) : null,
    present(entry.agent) ? h('span', { class: 'bv-log-row__agent' }, String(entry.agent)) : null,
    h('span', { class: 'bv-log-row__text' }, str(entry.text)));
}

// data: {workers, log}. No workers: honest empty state, graph hidden.
export function renderAgentsView(data = {}) {
  const workers = Array.isArray(data.workers) ? data.workers : [];
  const log = Array.isArray(data.log) ? data.log : [];
  if (workers.length === 0) {
    return h('div', { class: 'bv-agents bv-agents--empty' },
      h('p', { class: 'bv-agents__empty' }, 'No sub-agents in this session yet. They appear when the main agent starts one with the Agent tool, or when a coordinator starts workers.'));
  }
  // Root label follows the real parent: "main", "coordinator" (default), or a
  // task_id for a nested sub-agent, which joins its parent's root.
  const byId = new Map(workers.map((w) => [String(w.task_id), w]));
  const rootOf = (w) => {
    let cur = w;
    for (let i = 0; i < 20; i++) {
      const parent = cur && byId.get(String(cur.parent));
      if (!parent) return cur && cur.parent === 'main' ? 'main' : 'coordinator';
      cur = parent;
    }
    return 'coordinator';
  };
  const ROOT_LABEL = { main: 'Main agent', coordinator: 'Coordinator' };
  const groups = ['main', 'coordinator'].map((root) => ({ root, items: workers.filter((w) => rootOf(w) === root) })).filter((g) => g.items.length);
  return h('div', { class: 'bv-agents' },
    h('div', { class: 'bv-agents__list' }, workers.map((w) => renderWorkerRow(w))),
    h('div', { class: 'bv-agents__graph' },
      groups.map((g) => h('div', { class: 'bv-agents__group' },
        h('div', { class: 'bv-agents__root' }, icon('agents'), h('strong', null, ROOT_LABEL[g.root])),
        h('svg', { class: 'bv-agents__connectors', viewBox: '0 0 100 32', role: 'presentation' },
          g.items.map((_, i) => h('path', { d: `M50 0 V16 H${((i + 0.5) * 100 / g.items.length).toFixed(2)} V32`, fill: 'none', stroke: 'currentColor', 'stroke-width': '1' }))),
        h('div', { class: 'bv-agents__nodes' }, g.items.map((w) => renderAgentNode(w))))),
      workers.filter((w) => w.status === 'completed').length >= 2 ? renderAggregationNode({ workers }) : null),
    h('div', { class: 'bv-agents__log' }, log.map((e) => renderLogRow(e))));
}
