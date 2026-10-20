// Component gallery (V1f). Renders every js/ui component from recorded
// fixtures (dev/fixtures/*.json). Pure helpers are exported for node --test;
// the DOM wiring at the bottom only runs in a browser.
import { h, mount } from '../js/dom.js';
import { renderHeader } from '../js/ui/header.js';
import { renderNav, renderNavItem, NAV_ITEMS } from '../js/ui/nav.js';
import { renderSessionRow, renderSessionList } from '../js/ui/sessions.js';
import { renderTreeRow, renderTree } from '../js/ui/tree.js';
import {
  renderMessage, renderThinkingCard, renderStageChip, renderRetrievalChip,
  renderFilesChip, renderPackChip, renderToolCard, renderTranscript,
} from '../js/ui/transcript.js';
import { renderPermissionCard, renderSaveConfirmCard } from '../js/ui/permission.js';
import {
  renderStatusRow, renderUsage, renderTodoRow, renderTaskRow, renderTodoList, renderEventRow,
} from '../js/ui/rail.js';
import { renderTab, renderTabStrip } from '../js/ui/tabs.js';
import {
  renderWorkerRow, renderAgentNode, renderAggregationNode, renderLogRow, renderAgentsView,
} from '../js/ui/agents.js';

export const WIDTHS = [1280, 1440, 1920];

const FIXTURES = ['run-basic', 'run-permission', 'run-todo'];

// Permission card verbs (data-action) to K2 permission_resolved decisions.
const DECISIONS = { allow: 'allow', deny: 'deny', 'always-allow': 'always_allow' };
export function decisionFor(action) {
  return Object.hasOwn(DECISIONS, action) ? DECISIONS[action] : null;
}

// Delegated listeners must resolve the nearest [data-action]: rows wrap inner
// buttons (select-session > delete-session, tree-select > tree-toggle).
export function actionOf(target) {
  const el = target && typeof target.closest === 'function' ? target.closest('[data-action]') : null;
  if (!el) return null;
  return { action: el.dataset.action, id: el.dataset.id ?? null };
}

const ms = (a, b) => {
  const d = Date.parse(b) - Date.parse(a);
  return Number.isFinite(d) && d >= 0 ? d : undefined;
};

// Folds recorded SSE events (the Session.Emit envelope) into render data.
export function eventsToView(events) {
  const items = [];
  const tools = new Map();
  const recent = [];
  let thinking = null;
  let text = null;
  let permission = null;
  let usage = null;
  for (const ev of Array.isArray(events) ? events : []) {
    const d = ev.data || {};
    switch (ev.type) {
      case 'run_started':
        recent.push({ time: ev.time, label: 'Run started', kind: 'info' });
        break;
      case 'assistant_thinking_delta':
        if (!thinking) { thinking = { kind: 'thinking', text: '', open: true, _from: ev.time }; items.push(thinking); }
        thinking.text += d.thinking || '';
        thinking.durationMs = ms(thinking._from, ev.time);
        break;
      case 'assistant_text_delta':
        if (!text) { text = { kind: 'message', role: 'assistant', content: '', time: ev.time }; items.push(text); }
        text.content += d.content || '';
        break;
      case 'tool_use_start': {
        const t = { kind: 'tool', id: d.id, name: d.name, input: d.input, status: 'running', _from: ev.time };
        tools.set(d.id, t);
        items.push(t);
        recent.push({ time: ev.time, label: 'Tool ' + d.name, kind: 'running' });
        break;
      }
      case 'tool_use_result': {
        const t = tools.get(d.id);
        if (t) {
          t.status = d.error ? 'failed' : 'completed';
          t.result = d.result;
          t.error = d.error || undefined;
          t.durationMs = ms(t._from, ev.time);
        }
        recent.push({ time: ev.time, label: 'Tool result' + (d.error ? ' (error)' : ''), kind: d.error ? 'failed' : 'success' });
        break;
      }
      case 'permission_request':
        permission = { request_id: d.request_id, tool_name: d.tool_name, target: d.target, reason: d.reason };
        recent.push({ time: ev.time, label: 'Permission requested', kind: 'warning' });
        break;
      case 'permission_resolved':
        permission = null;
        break;
      case 'terminal':
        usage = d.usage || null;
        recent.push({ time: ev.time, label: 'Run ' + (d.reason || 'finished'), kind: d.reason === 'completed' ? 'success' : 'muted' });
        break;
      default:
    }
  }
  for (const it of items) delete it._from;
  return { items, permission, usage, events: recent };
}

const SAMPLE_SESSIONS = [
  { id: 'gallery-permission', title: 'Fix failing build', created_at: '2026-01-02T03:00:00Z', last_active: '2026-01-02T03:04:11Z', running: true, model: 'qwen3-coder' },
  { id: 'gallery-basic', title: 'Explain main.go', created_at: '2026-01-01T09:00:00Z', last_active: '2026-01-01T09:20:00Z', running: false, model: 'qwen3-coder' },
  { id: 'gallery-todo', title: '', created_at: '2025-12-30T09:00:00Z', last_active: '2025-12-30T09:20:00Z', running: false, model: 'qwen3-coder' },
];
const SAMPLE_NOW = Date.parse('2026-01-02T03:10:00Z');
const SAMPLE_TREE = [
  { name: 'cmd', kind: 'dir', children: [{ name: 'main.go', kind: 'file', size: 812 }] },
  { name: 'internal', kind: 'dir' },
  { name: 'docs', kind: 'dir', children: [] },
  { name: 'go.mod', kind: 'file', size: 240 },
  { name: 'README.md', kind: 'file', size: 1900 },
];
const SAMPLE_TABS = [
  { id: 'agents', title: 'Agents', icon: 'agents' },
  { id: 'main.go', title: 'main.go', icon: 'file-code', closable: true },
  { id: 'notes.md', title: 'notes.md', icon: 'file', closable: true },
];
const SAMPLE_WORKERS = [
  { task_id: 't1', name: 'Code Analysis', task: 'Scan handlers', model: 'qwen3-coder', status: 'completed', step: 'Done', files_touched: 4, usage: { PromptEvalCount: 900, EvalCount: 240 }, started_at: '2026-01-02T03:04:05Z', finished_at: '2026-01-02T03:04:50Z' },
  { task_id: 't2', name: 'Test Runner', task: 'Run go test', model: 'qwen3-coder', status: 'running', step: 'Running package server', started_at: '2026-01-02T03:04:10Z' },
  { task_id: 't3', name: 'Reviewer', task: 'Review diff', model: 'qwen3-coder', status: 'failed', files_touched: 1, started_at: '2026-01-02T03:04:12Z', finished_at: '2026-01-02T03:04:20Z' },
];
const SAMPLE_LOG = [
  { time: '2026-01-02T03:04:05Z', agent: 'Code Analysis', text: 'Started', kind: 'info' },
  { time: '2026-01-02T03:04:50Z', agent: 'Code Analysis', text: 'Finished 4 files', kind: 'success' },
  { time: '2026-01-02T03:04:20Z', agent: 'Reviewer', text: 'Failed: model timeout', kind: 'failed' },
];
const SAMPLE_TASKS = [
  { task_id: 'bg-1', kind: 'shell', status: 'running', description: 'go test ./...' },
  { task_id: 'bg-2', kind: 'shell', status: 'completed', description: 'npm-free lint' },
];

const view = (fixtures, name) => eventsToView(fixtures[name] || []);

// Each section declares the render functions it exercises (COVERED, asserted
// against the real exports by gallery.test.js).
const SECTIONS = [
  { id: 'shell', title: 'Layout (header, nav, tree, transcript, tabs, rail)', uses: [], build: (f) => shell(f) },
  { id: 'header', title: 'Header', uses: ['renderHeader'], build: () => h('div', { class: 'gallery-sample wide' }, [
    renderHeader({ sessionId: 'gallery-permission', model: 'qwen3-coder', models: ['qwen3-coder', 'llama3.1'], connection: 'connected', running: true }),
    renderHeader({ connection: 'disconnected', running: false }),
  ]) },
  { id: 'nav', title: 'Nav and icon rail', uses: ['renderNav', 'renderNavItem'], build: () => h('div', { class: 'gallery-sample' }, [
    renderNav({ active: 'chat' }),
    renderNav({ active: 'agents', collapsed: true }),
    renderNavItem(NAV_ITEMS[2], { active: false }),
  ]) },
  { id: 'sessions', title: 'Sessions', uses: ['renderSessionRow', 'renderSessionList'], build: () => h('div', { class: 'gallery-sample' }, [
    renderSessionList({ sessions: SAMPLE_SESSIONS, activeId: 'gallery-permission' }, { now: SAMPLE_NOW }),
    h('ul', { class: 'bv-sessions__list' }, renderSessionRow(SAMPLE_SESSIONS[1], { now: SAMPLE_NOW })),
    renderSessionList({ sessions: [] }),
  ]) },
  { id: 'tree', title: 'Project tree', uses: ['renderTree', 'renderTreeRow'], build: () => h('div', { class: 'gallery-sample' }, [
    renderTree({ entries: SAMPLE_TREE, expanded: ['cmd', 'docs'] }),
    h('ul', { class: 'bv-tree__list' }, renderTreeRow(SAMPLE_TREE[3], { path: 'go.mod' })),
    renderTree({ entries: [] }),
  ]) },
  { id: 'messages', title: 'Messages and thinking', uses: ['renderMessage', 'renderThinkingCard', 'renderTranscript'], build: (f) => {
    const v = view(f, 'run-permission');
    return h('div', { class: 'gallery-sample wide' }, [
      renderMessage({ role: 'user', content: 'Why does the build fail?', time: '2026-01-02T03:04:00Z' }),
      renderTranscript({ items: v.items }),
      renderThinkingCard({ text: 'Collapsed example.', durationMs: 2400, open: false }),
      renderTranscript({ items: [] }),
    ]);
  } },
  { id: 'chips', title: 'Chips', uses: ['renderStageChip', 'renderRetrievalChip', 'renderFilesChip', 'renderPackChip'], build: () => h('div', { class: 'gallery-sample' }, [
    renderStageChip({ stage: 'retrieval', duration_ms: 420 }),
    renderRetrievalChip({ records: 12, files: 4 }),
    renderRetrievalChip({ fallback: true, reason: 'index stale' }),
    renderRetrievalChip({ error: 'index unavailable' }),
    renderFilesChip({ files: ['main.go', 'handler.go'] }),
    renderPackChip({ estimated_included: 8, estimated_skipped: 2 }),
  ]) },
  { id: 'tools', title: 'Tool cards', uses: ['renderToolCard'], build: (f) => {
    const tools = view(f, 'run-permission').items.filter((i) => i.kind === 'tool');
    return h('div', { class: 'gallery-sample wide' }, [
      ...tools.map((t) => renderToolCard(t)),
      renderToolCard({ id: 'x', name: 'Bash', input: { command: 'go build' }, status: 'running' }),
      renderToolCard({ id: 'y', name: 'Edit', input: { path: 'a.go' }, status: 'failed', error: 'file changed on disk', durationMs: 130 }),
    ]);
  } },
  { id: 'permission', title: 'Permission card', uses: ['renderPermissionCard', 'renderSaveConfirmCard'], build: (f) => {
    const p = view(f, 'run-permission').permission || { request_id: 'p', tool_name: 'Bash', target: 'go test ./...', reason: 'Run checks' };
    return h('div', { class: 'gallery-sample' }, [renderPermissionCard(p), renderPermissionCard(p, { inline: true })]);
  } },
  { id: 'tabs', title: 'Tab strip', uses: ['renderTab', 'renderTabStrip'], build: () => h('div', { class: 'gallery-sample wide' }, [
    renderTabStrip({ tabs: SAMPLE_TABS, active: 'main.go' }),
    renderTab(SAMPLE_TABS[1], { active: false }),
    renderTabStrip({ tabs: [] }),
  ]) },
  { id: 'status', title: 'Status rows', uses: ['renderStatusRow'], build: () => h('div', { class: 'gallery-sample' }, h('div', { class: 'gallery-rail' }, [
    renderStatusRow({ label: 'Server', value: 'Connected', dot: 'success' }),
    renderStatusRow({ label: 'Run', value: 'Running', dot: 'info' }),
    renderStatusRow({ label: 'Permission', value: 'Pending', dot: 'warning' }),
    renderStatusRow({ label: 'Index', value: 'Unavailable', dot: 'muted' }),
    renderStatusRow({ label: 'Last error', dot: 'danger' }),
  ])) },
  { id: 'usage', title: 'Usage', uses: ['renderUsage'], build: (f) => h('div', { class: 'gallery-sample' }, [
    h('div', { class: 'gallery-rail' }, renderUsage(view(f, 'run-permission').usage)),
    h('div', { class: 'gallery-rail' }, renderUsage(view(f, 'run-basic').usage)),
  ]) },
  { id: 'tasks', title: 'Tasks', uses: ['renderTodoRow', 'renderTaskRow', 'renderTodoList'], build: () => h('div', { class: 'gallery-sample' }, [
    h('div', { class: 'gallery-rail' }, renderTodoList({ todos: [
      { id: '1', content: 'Read handler.go', status: 'completed', priority: 'high' },
      { id: '2', content: 'Fix the failing test', status: 'in_progress', priority: 'high' },
      { id: '3', content: 'Update docs', status: 'pending', priority: 'low' },
    ], tasks: SAMPLE_TASKS })),
    h('div', { class: 'gallery-rail' }, [renderTodoRow({ id: '4', content: 'Dropped item', status: 'cancelled' }), renderTaskRow(SAMPLE_TASKS[0])]),
    h('div', { class: 'gallery-rail' }, renderTodoList({})),
  ]) },
  { id: 'events', title: 'Recent events', uses: ['renderEventRow'], build: (f) => h('div', { class: 'gallery-sample' },
    h('div', { class: 'gallery-rail' }, view(f, 'run-permission').events.map((e) => renderEventRow(e)))) },
  { id: 'agents', title: 'Agents', uses: ['renderWorkerRow', 'renderAgentNode', 'renderAggregationNode', 'renderLogRow', 'renderAgentsView'], build: () => h('div', { class: 'gallery-sample wide' }, [
    renderAgentsView({ workers: SAMPLE_WORKERS, log: SAMPLE_LOG }),
    renderAgentsView({}),
    h('div', {}, [renderWorkerRow(SAMPLE_WORKERS[1]), renderAgentNode(SAMPLE_WORKERS[1]), renderAggregationNode({ workers: SAMPLE_WORKERS }), renderLogRow(SAMPLE_LOG[0])]),
  ]) },
];

// Composite page: the real layout widths, driven by the permission fixture.
function shell(f) {
  const v = view(f, 'run-permission');
  return h('div', { class: 'gallery-shell' }, [
    renderHeader({ sessionId: 'gallery-permission', model: 'qwen3-coder', models: ['qwen3-coder'], connection: 'connected', running: true }),
    h('div', {}, [renderNav({ active: 'chat' }), renderSessionList({ sessions: SAMPLE_SESSIONS, activeId: 'gallery-permission' }, { now: SAMPLE_NOW })]),
    renderTree({ entries: SAMPLE_TREE, expanded: ['cmd'] }),
    h('div', { class: 'gallery-shell__center' }, [
      renderTranscript({ items: [{ kind: 'message', role: 'user', content: 'Why does the build fail?', time: '2026-01-02T03:04:00Z' }, ...v.items] }),
      v.permission ? renderPermissionCard(v.permission, { inline: true }) : null,
      renderTabStrip({ tabs: SAMPLE_TABS, active: 'agents' }),
      renderAgentsView({ workers: SAMPLE_WORKERS, log: SAMPLE_LOG }),
    ]),
    h('div', { class: 'gallery-rail' }, [
      renderUsage(v.usage),
      renderTodoList({ todos: [{ id: '1', content: 'Fix the failing test', status: 'in_progress' }], tasks: SAMPLE_TASKS }),
      v.events.map((e) => renderEventRow(e)),
    ]),
  ]);
}

export const COVERED = SECTIONS.flatMap((s) => s.uses);

export function buildSections(fixtures = {}) {
  return SECTIONS.map((s) => ({ id: s.id, title: s.title, tree: s.build(fixtures) }));
}

// ---- Browser wiring (skipped under node --test) ----
async function start() {
  const fixtures = {};
  await Promise.all(FIXTURES.map(async (name) => {
    try {
      const res = await fetch('dev/fixtures/' + name + '.json');
      fixtures[name] = res.ok ? await res.json() : [];
    } catch {
      fixtures[name] = [];
    }
  }));
  const stage = document.getElementById('gallery-stage');
  for (const s of buildSections(fixtures)) {
    const sec = document.createElement('section');
    sec.className = 'gallery-section';
    sec.id = 'section-' + s.id;
    const title = document.createElement('h2');
    title.textContent = s.title;
    sec.append(title, mount(s.tree));
    stage.append(sec);
  }
  const last = document.getElementById('gallery-last');
  stage.addEventListener('click', (e) => {
    const hit = actionOf(e.target);
    if (!hit) return;
    const decision = decisionFor(hit.action);
    last.textContent = hit.action + (hit.id ? ' ' + hit.id : '') + (decision ? ' -> ' + decision : '');
  });
  const buttons = [...document.querySelectorAll('.gallery-width')];
  for (const b of buttons) {
    b.addEventListener('click', () => {
      stage.dataset.width = b.dataset.width;
      for (const o of buttons) o.setAttribute('aria-pressed', String(o === b));
    });
  }
}

if (typeof document !== 'undefined') start();
