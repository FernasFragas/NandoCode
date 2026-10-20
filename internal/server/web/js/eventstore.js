// Bounded, DOM-free recent-event store. V8b may reuse createEventStore for
// agent activity: add(msg) accepts the Session.Emit envelope, list() returns
// oldest-to-newest rows, lastEvent() returns the last accepted type, reset()
// clears a session. Deltas and heartbeats never enter the store.
const NOISE = new Set(['assistant_text_delta', 'assistant_thinking_delta', 'heartbeat']);
const KIND = {
  error: 'failed', terminal: 'success', run_started: 'running', tool_use_start: 'running',
  permission_request: 'warning', hook_notice: 'warning', retry_notice: 'warning',
};

export function eventRow(msg) {
  const p = msg && msg.data && typeof msg.data === 'object' ? msg.data : (msg || {});
  let label = msg.type;
  if (msg.type === 'tool_use_start' || msg.type === 'tool_use_result') label += ' ' + (p.name || p.tool_name || '');
  else if (msg.type === 'permission_request') label += ' ' + (p.tool_name || '');
  else if (msg.type === 'terminal' && p.reason) label += ' ' + p.reason;
  return {
    id: msg.id, type: msg.type, data: p, time: msg.time,
    label: label.trim(),
    kind: msg.type === 'tool_use_result' && p.error ? 'failed' : (KIND[msg.type] || 'info'),
  };
}

export function createEventStore({ limit = 5 } = {}) {
  const capacity = Number.isInteger(limit) && limit > 0 ? limit : 5;
  let rows = [];
  return {
    add(msg) {
      if (!msg || typeof msg.type !== 'string' || !msg.type || NOISE.has(msg.type)) return false;
      rows.push(eventRow(msg));
      if (rows.length > capacity) rows.shift();
      return true;
    },
    list() { return rows.slice(); },
    lastEvent() { return rows.at(-1)?.type || ''; },
    reset() { rows = []; },
  };
}
