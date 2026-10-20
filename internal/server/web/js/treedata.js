// Project tree data shared by the tree view and the composer's @ autocomplete.
// The Files API returns one directory at a time. Flatten it for the tree and
// @ autocomplete; each entry keeps its workspace-relative path.

// buildTree(entries) -> [{name, kind: 'dir'|'file', children?}] with dirs first.
export function buildTree(entries) {
  const root = { children: [] };
  const byPath = new Map([['', root]]);
  for (const e of Array.isArray(entries) ? entries : []) {
    const parts = String(e.path || '').replace(/^\.\//, '').split('/').filter(Boolean);
    let parent = root;
    let current = '';
    parts.forEach((part, i) => {
      current = current ? current + '/' + part : part;
      let node = byPath.get(current);
      if (!node) {
        const dir = i < parts.length - 1 ? true : e.kind === 'dir';
        node = dir ? { name: part, kind: 'dir', children: [] } : { name: part, kind: 'file' };
        byPath.set(current, node);
        parent.children.push(node);
      }
      parent = node;
    });
  }
  const sort = (nodes) => {
    nodes.sort((a, b) => (a.kind === b.kind ? a.name.localeCompare(b.name) : a.kind === 'dir' ? -1 : 1));
    nodes.forEach((n) => n.children && sort(n.children));
    return nodes;
  };
  return sort(root.children);
}

let inflight = null;

// loadTree walks directory responses and caches the flattened entries once
// per session. A bounded walk protects the browser from enormous projects.
export function loadTree(ctx, { force = false } = {}) {
  const id = ctx.state.sessionId;
  if (!id) return Promise.resolve([]);
  if (!force && ctx.state.treeEntries && ctx.state.treeFor === id) return Promise.resolve(ctx.state.treeEntries);
  if (inflight && inflight.id === id) return inflight.promise;
  const promise = (async () => {
    const entries = [];
    const pending = ['.'];
    while (pending.length && entries.length < 5000) {
      const dir = pending.shift();
      let data;
      try {
        data = await ctx.api.get('/v1/sessions/' + encodeURIComponent(id) + '/files?path=' + encodeURIComponent(dir));
      } catch (err) {
        if (dir === '.') throw err;
        // Keep what loaded so far; flag the directory that failed.
        const failed = entries.find((e) => e.path === dir);
        if (failed) failed.error = true;
        ctx.emit('tree:error', err);
        continue;
      }
      if (ctx.state.sessionId !== id) return [];
      for (const e of Array.isArray(data?.entries) ? data.entries : []) {
        if (e.kind !== 'dir' && e.kind !== 'file') continue;
        const path = dir === '.' ? e.name : dir + '/' + e.name;
        entries.push({ path, name: e.name, kind: e.kind, size: e.size });
        if (e.kind === 'dir') pending.push(path);
        if (entries.length >= 5000) break;
      }
    }
    if (ctx.state.sessionId !== id) return [];
    ctx.state.treeEntries = entries;
    ctx.state.treeFor = id;
    return entries;
  })()
    .catch((err) => {
      ctx.emit('tree:error', err);
      return [];
    })
    .finally(() => { if (inflight && inflight.promise === promise) inflight = null; });
  inflight = { id, promise };
  return promise;
}

export function resetTree(ctx) {
  ctx.state.treeEntries = null;
  ctx.state.treeFor = null;
  inflight = null;
}
