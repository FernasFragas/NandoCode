// Right status rail components (DESIGN-SPEC "Component inventory", BV-3).
// Pure render functions: no DOM, window or storage. Class names only.
import { h } from '../dom.js';

const DASH = '—';
const KNOWN = {
  dot: ['success', 'info', 'warning', 'danger', 'muted'],
  todo: ['pending', 'in_progress', 'completed', 'cancelled'],
  task: ['pending', 'running', 'completed', 'failed', 'killed', 'cancelled'],
  event: ['info', 'success', 'warning', 'failed', 'running', 'muted'],
};

// Only a known token may reach a class name.
function mod(kind, value) {
  return KNOWN[kind].includes(value) ? value : 'unknown';
}

function present(v) {
  return v !== null && v !== undefined && v !== '';
}

// row: {label, value, dot}. A missing value renders an em dash.
export function renderStatusRow(row = {}) {
  return h('div', { class: 'bv-status-row' },
    h('span', { class: 'bv-status-row__label' }, present(row.label) ? String(row.label) : ''),
    h('span', { class: 'bv-status-row__value' },
      row.dot ? h('span', { class: 'bv-status-row__dot bv-status-row__dot--' + mod('dot', row.dot) }) : null,
      present(row.value) ? String(row.value) : DASH));
}

function formatDuration(ns) {
  const s = ns / 1e9;
  if (s < 60) return (Math.round(s * 10) / 10) + 's';
  const m = Math.floor(s / 60);
  return m + 'm ' + Math.round(s - m * 60) + 's';
}

// usage: agent.Usage (K2 usage_update / terminal.usage). Before the first
// turn every row shows an em dash, never 0.
export function renderUsage(usage) {
  const u = usage && usage.Turns > 0 ? usage : null;
  const num = (k) => (u && present(u[k]) ? String(u[k]) : DASH);
  const rows = [
    ['Prompt tokens', num('PromptEvalCount')],
    ['Output tokens', num('EvalCount')],
    ['Duration', u && u.TotalDuration > 0 ? formatDuration(u.TotalDuration) : DASH],
    ['Turns', num('Turns')],
    ['Tool calls', num('ToolCalls')],
  ];
  return h('div', { class: 'bv-usage' }, rows.map(([label, value]) =>
    h('div', { class: 'bv-usage__row' },
      h('span', { class: 'bv-usage__label' }, label),
      h('span', { class: 'bv-usage__value' }, value))));
}

// todo: {id, content, status, priority} (todo.TodoItem).
export function renderTodoRow(todo = {}) {
  return h('div', {
    class: 'bv-todo-row bv-todo-row--' + mod('todo', todo.status),
    'data-id': present(todo.id) ? String(todo.id) : null,
  },
  h('span', { class: 'bv-todo-row__radio' }),
  h('span', { class: 'bv-todo-row__content' }, present(todo.content) ? String(todo.content) : ''),
  present(todo.status) ? h('span', { class: 'bv-todo-row__status' }, String(todo.status)) : null);
}

// task: task_lifecycle payload {task_id, kind, status, description, output_file}.
export function renderTaskRow(task = {}) {
  const title = present(task.description) ? task.description : task.task_id;
  return h('div', {
    class: 'bv-task-row bv-task-row--' + mod('task', task.status),
    'data-id': present(task.task_id) ? String(task.task_id) : null,
  },
  h('span', { class: 'bv-task-row__dot' }),
  h('span', { class: 'bv-task-row__title' }, present(title) ? String(title) : ''),
  present(task.kind) ? h('span', { class: 'bv-task-row__kind' }, String(task.kind)) : null,
  present(task.status) ? h('span', { class: 'bv-task-row__status' }, String(task.status)) : null);
}

// data: {todos, tasks}. Todos first, background tasks below.
export function renderTodoList(data = {}) {
  const todos = Array.isArray(data.todos) ? data.todos : [];
  const tasks = Array.isArray(data.tasks) ? data.tasks : [];
  if (todos.length === 0 && tasks.length === 0) {
    return h('div', { class: 'bv-tasks bv-tasks--empty' }, h('p', { class: 'bv-tasks__empty' }, 'No tasks yet'));
  }
  return h('div', { class: 'bv-tasks' },
    todos.length ? h('div', { class: 'bv-tasks__todos' }, todos.map((t) => renderTodoRow(t))) : null,
    tasks.length ? h('div', { class: 'bv-tasks__background' }, tasks.map((t) => renderTaskRow(t))) : null);
}

function clock(iso) {
  const m = typeof iso === 'string' && /T(\d\d:\d\d:\d\d)/.exec(iso);
  return m ? m[1] : null;
}

// ev: {time (RFC 3339), label, kind}.
export function renderEventRow(ev = {}) {
  const time = clock(ev.time);
  return h('div', { class: 'bv-event-row' },
    h('span', { class: 'bv-event-row__dot bv-event-row__dot--' + mod('event', ev.kind) }),
    time ? h('span', { class: 'bv-event-row__time' }, time) : null,
    h('span', { class: 'bv-event-row__label' }, present(ev.label) ? String(ev.label) : ''));
}

export { clock as _clock };
