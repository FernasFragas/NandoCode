// Session-scoped sub-agent facts and activity. Rows are built on V3b's eventRow
// (one row shape, the SSE envelope id is kept) plus the local text/agent fields.
import { eventRow } from './eventstore.js';

const TYPES = new Set(['subagent_started', 'subagent_progress', 'subagent_usage', 'subagent_message', 'subagent_finished']);
const TERMINAL = new Set(['completed', 'failed', 'cancelled', 'killed']);
const MAX_RUNES = 200;
const MAX_WORKERS = 200;
const MAX_SEEN = 2000;
const payload = (msg) => msg && msg.data && typeof msg.data === 'object' ? msg.data : {};
const terminal = (status) => TERMINAL.has(status);
// Defence in depth: the server strips control characters and caps at 200 runes.
const clean = (v) => {
  if (v === null || v === undefined) return '';
  const s = String(v).replace(/[\u0000-\u001f\u007f-\u009f]/g, '');
  const runes = [...s];
  return runes.length > MAX_RUNES ? runes.slice(0, MAX_RUNES).join('') : s;
};

function describe(type, p) {
  switch (type) {
    case 'subagent_started': return `Started ${clean(p.task || p.name) || 'worker'}`;
    case 'subagent_progress': return [p.tool, p.target, p.status].filter(Boolean).map(clean).join(' · ');
    case 'subagent_usage': return p.files_touched > 0 ? `${p.files_touched} files touched` : 'Usage updated';
    case 'subagent_message': return clean(p.summary) || 'Message sent';
    case 'subagent_finished': return clean(p.error || p.status) || 'Finished';
    default: return '';
  }
}

export function createAgentsLogStore({ limit = 200 } = {}) {
  const capacity = Number.isInteger(limit) && limit > 0 ? limit : 200;
  let byId = new Map();
  let rows = [];
  let seen = new Set();

  const eventKey = (msg, p, id) => msg.id ? 'id:' + msg.id : [msg.type, id, p.status || p.at || p.started_at || p.finished_at || ''].join('|');
  function bound() {
    while (byId.size > MAX_WORKERS) {
      let victim = null;
      for (const [k, w] of byId) if (terminal(w.status)) { victim = k; break; }
      byId.delete(victim ?? byId.keys().next().value);
    }
  }
  function put(id, value) { byId.set(id, value); bound(); }

  return {
    add(msg) {
      if (!msg || !TYPES.has(msg.type)) return false;
      const p = payload(msg);
      const id = p.task_id || p.from || '';
      const key = eventKey(msg, p, id);
      if (seen.has(key)) return false;
      const previous = byId.get(id) || { task_id: id };
      // At most one finished per task; nothing is accepted after it.
      if (msg.type === 'subagent_finished' && terminal(previous.status)) return false;
      if ((msg.type === 'subagent_progress' || msg.type === 'subagent_usage') && terminal(previous.status)) return false;
      seen.add(key);
      if (seen.size > MAX_SEEN) seen.delete(seen.values().next().value);
      const safe = msg.type === 'subagent_message'
        ? { ...p, from: clean(p.from), to: clean(p.to), summary: clean(p.summary) }
        : p;
      if (msg.type === 'subagent_started' && id) put(id, { ...previous, ...p, status: terminal(previous.status) ? previous.status : 'running' });
      if (msg.type === 'subagent_progress' && id) put(id, { ...previous, step: [p.tool, p.target].filter(Boolean).join(' · ') });
      if (msg.type === 'subagent_usage' && id) put(id, { ...previous, usage: p.usage, files_touched: p.files_touched });
      if (msg.type === 'subagent_finished' && id) put(id, { ...previous, status: p.status || 'completed', finished_at: p.finished_at, error: p.error });
      const recipients = msg.type === 'subagent_message' ? [safe.from, safe.to].filter(Boolean) : [id];
      const base = eventRow({ ...msg, data: safe });
      rows.push({
        ...base,
        time: p.at || msg.time,
        task_id: id,
        recipients,
        agent: byId.get(id)?.name || id,
        text: describe(msg.type, p),
        kind: msg.type === 'subagent_finished' ? (p.status === 'completed' ? 'success' : 'failed') : (msg.type === 'subagent_started' ? 'running' : 'info'),
      });
      if (rows.length > capacity) rows.shift();
      return true;
    },
    setWorkers(workers) {
      if (!Array.isArray(workers)) return;
      for (const worker of workers) if (worker && worker.task_id) {
        const live = byId.get(worker.task_id) || {};
        const status = terminal(live.status) ? live.status : (terminal(worker.status) ? worker.status : (live.status || worker.status));
        put(worker.task_id, { ...worker, ...live, status });
      }
    },
    workers() { return [...byId.values()].filter((w) => w.name || w.task).sort((a, b) => String(a.started_at || '').localeCompare(String(b.started_at || ''))); },
    // agent is a task_id; messages may address a worker by task_id or by name.
    list(agent = '') {
      if (!agent) return rows.slice();
      const names = [agent];
      const w = byId.get(agent);
      if (w && w.name) names.push(w.name);
      return rows.filter((row) => row.recipients.some((r) => names.includes(r)));
    },
    reset() { byId = new Map(); rows = []; seen = new Set(); },
  };
}
