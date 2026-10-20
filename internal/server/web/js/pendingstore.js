// Shared per-context pending items for permission cards and save confirmations.
// getPendingStore(ctx) returns {add, dismiss, show, resolve, clear, reset, list, count}.
// Resolved/cleared ids are remembered (add returns false) so SSE replays after a
// reconnect cannot reopen them; reset() forgets them on a session change.
// Each change emits ctx 'pending:changed' with {kind, id, action, counts, items};
// items is the current snapshot across kinds, including hidden items. Consumers
// such as the rail can count or reopen permissions without owning their queue.
const stores = new WeakMap();

export function getPendingStore(ctx) {
  if (stores.has(ctx)) return stores.get(ctx);
  const items = new Map();
  const DONE_CAP = 500;
  const done = new Set(); // resolved or cleared ids: SSE replays must not reopen them
  const keyOf = (kind, id) => kind + '\u0000' + id;
  const list = (kind) => [...items.values()]
    .filter((item) => !kind || item.kind === kind)
    .map((item) => ({ ...item, payload: { ...item.payload } }));
  const count = (kind) => list(kind).length;
  function remember(key) {
    done.delete(key);
    done.add(key);
    if (done.size > DONE_CAP) done.delete(done.values().next().value);
  }
  function publish(kind, id, action) {
    const counts = {};
    counts.permission = 0;
    counts.save = 0;
    for (const item of items.values()) counts[item.kind] = (counts[item.kind] || 0) + 1;
    ctx.emit('pending:changed', { kind, id, action, counts, items: list() });
  }
  const store = {
    list,
    count,
    add(kind, id, payload = {}) {
      if (!kind || !id || items.has(keyOf(kind, id)) || done.has(keyOf(kind, id))) return false;
      items.set(keyOf(kind, id), { kind, id, payload: { ...payload }, hidden: false });
      publish(kind, id, 'added');
      return true;
    },
    dismiss(kind, id) {
      const item = items.get(keyOf(kind, id));
      if (!item || item.hidden) return false;
      item.hidden = true;
      publish(kind, id, 'dismissed');
      return true;
    },
    show(kind, id) {
      const item = items.get(keyOf(kind, id));
      if (!item || !item.hidden) return false;
      item.hidden = false;
      publish(kind, id, 'shown');
      return true;
    },
    resolve(kind, id) {
      if (!items.delete(keyOf(kind, id))) { remember(keyOf(kind, id)); return false; }
      remember(keyOf(kind, id));
      publish(kind, id, 'resolved');
      return true;
    },
    clear(kind) {
      let changed = false;
      for (const item of items.values()) {
        if (!kind || item.kind === kind) {
          items.delete(keyOf(item.kind, item.id));
          remember(keyOf(item.kind, item.id));
          changed = true;
        }
      }
      if (changed) publish(kind || null, null, 'cleared');
      return changed;
    },
    // New session: forget everything, including remembered ids.
    reset() { items.clear(); done.clear(); publish(null, null, 'reset'); },
  };
  stores.set(ctx, store);
  return store;
}
