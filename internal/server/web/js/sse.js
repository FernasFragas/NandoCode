// SSE client over fetch (EventSource cannot send the bearer header). Parsing is
// DOM-free. On reconnect it sends Last-Event-ID and drops event ids it has
// already delivered, which fixes the duplicated transcript after a reconnect.

// parseSSE(buf) -> {events: [{id, event, data}], remaining}
export function parseSSE(buf) {
  const events = [];
  let remaining = String(buf).replace(/\r\n/g, '\n');
  let idx;
  while ((idx = remaining.indexOf('\n\n')) !== -1) {
    const block = remaining.slice(0, idx);
    remaining = remaining.slice(idx + 2);
    let id = '';
    let event = '';
    const data = [];
    for (const line of block.split('\n')) {
      if (line.startsWith('id:')) id = line.slice(3).trim();
      else if (line.startsWith('event:')) event = line.slice(6).trim();
      else if (line.startsWith('data:')) data.push(line.slice(5).replace(/^ /, ''));
    }
    if (data.length) events.push({ id, event, data: data.join('\n') });
  }
  return { events, remaining };
}

// Heartbeats carry a throwaway id the server cannot replay from.
const isHeartbeat = (e) => e.event === 'heartbeat' || e.id.startsWith('hb-');

// Statuses passed to onStatus: connecting, connected, disconnected, unauthorized.
export function createEventStream(opts) {
  const {
    url, getToken, onEvent, onStatus,
    fetchImpl = (...a) => globalThis.fetch(...a),
    setTimer = (fn, ms) => globalThis.setTimeout(fn, ms),
    clearTimer = (t) => globalThis.clearTimeout(t),
    baseDelay = 1000, maxDelay = 5000, seenLimit = 5000,
  } = opts;
  const seen = new Set();
  const order = [];
  let lastId = '';
  let attempt = 0;
  let timer = null;
  let reader = null;
  let stopped = true;
  let generation = 0;

  const status = (s) => { if (onStatus) onStatus(s); };

  function remember(id) {
    if (!id || seen.has(id)) return;
    seen.add(id);
    order.push(id);
    if (order.length > seenLimit) seen.delete(order.shift());
  }

  function scheduleReconnect(gen) {
    if (stopped || gen !== generation) return;
    status('connecting');
    const delay = Math.min(baseDelay * 2 ** attempt, maxDelay);
    attempt += 1;
    timer = setTimer(() => { timer = null; connect(); }, delay);
  }

  async function connect() {
    if (stopped) return;
    const gen = ++generation;
    status('connecting');
    const headers = {};
    const token = getToken ? getToken() : '';
    if (token) headers.Authorization = 'Bearer ' + token;
    if (lastId) headers['Last-Event-ID'] = lastId;
    try {
      const res = await fetchImpl(url, { headers });
      if (stopped || gen !== generation) return;
      if (res.status === 401) { stopped = true; status('unauthorized'); return; }
      if (res.status === 404) { stopped = true; status('disconnected'); return; }
      if (!res.ok || !res.body) throw new Error('HTTP ' + res.status);
      attempt = 0;
      status('connected');
      reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buf = '';
      for (;;) {
        const { done, value } = await reader.read();
        if (stopped || gen !== generation) return;
        if (done) break;
        buf += decoder.decode(value, { stream: true });
        const parsed = parseSSE(buf);
        buf = parsed.remaining;
        for (const evt of parsed.events) {
          if (isHeartbeat(evt)) { onEvent(evt); continue; }
          if (evt.id && seen.has(evt.id)) continue;
          remember(evt.id);
          if (evt.id) lastId = evt.id;
          onEvent(evt);
        }
      }
    } catch {
      /* fall through to the reconnect below */
    }
    reader = null;
    scheduleReconnect(gen);
  }

  return {
    start() {
      if (!stopped) return;
      stopped = false;
      attempt = 0;
      connect();
    },
    stop() {
      stopped = true;
      generation += 1;
      if (timer !== null) { clearTimer(timer); timer = null; }
      if (reader) { Promise.resolve(reader.cancel()).catch(() => {}); reader = null; }
      status('disconnected');
    },
    lastEventId() { return lastId; },
  };
}
