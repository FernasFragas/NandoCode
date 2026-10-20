// Shared tab registry. The split owns only layout; renderers own their child
// sections under #tab-content. Session storage contains paths and selection only.
import { renderTabStrip } from '../ui/tabs.js';
import { delegate } from '../delegate.js';
import { createEventStore } from '../eventstore.js';

// Tab kinds. Each feature opens tabs with its own kind and registers its view
// with ctx.registerTabKind(kind, mountFn) (see mount()). Built-ins: 'file'
// (features/codeview.js owns its own section) and 'events' (raw event log below).
export const EVENTS_KIND = 'events';

const key = (id) => 'nandocode.tabs.' + id;
const valid = (tab) => tab && typeof tab.id === 'string' && tab.id &&
  typeof tab.kind === 'string' && /^[a-z][a-z0-9-]*$/.test(tab.kind) &&
  (tab.kind !== 'file' || (typeof tab.path === 'string' && tab.path));
const titleOf = (tab) => tab.title || (tab.kind === 'file' ? tab.path.split('/').at(-1)
  : tab.kind === 'agents' ? 'Agents' : tab.kind === EVENTS_KIND ? 'Event log' : tab.kind);

// Collapses '', '.', duplicate slashes and '..' segments. Returns {abs, parts}.
export function normalizePath(p) {
  if (typeof p !== 'string' || !p) return null;
  const abs = p.startsWith('/');
  const parts = [];
  for (const seg of p.split('/')) {
    if (!seg || seg === '.') continue;
    if (seg === '..') { if (parts.length && parts.at(-1) !== '..') parts.pop(); else if (!abs) parts.push('..'); } else parts.push(seg);
  }
  return { abs, parts };
}
// True when a tool path names the tab's workspace-relative file. `root` is the
// workspace root when the UI knows it; otherwise an absolute tool path matches
// by whole-segment suffix (the session view carries no workspace root).
export function samePath(toolPath, tabPath, root) {
  const a = normalizePath(toolPath), b = normalizePath(tabPath);
  if (!a || !b || !b.parts.length || b.parts[0] === '..') return false;
  let rel = a.parts;
  if (a.abs) {
    const r = normalizePath(root);
    if (r && r.abs) {
      if (a.parts.length <= r.parts.length || !r.parts.every((seg, i) => seg === a.parts[i])) return false;
      rel = a.parts.slice(r.parts.length);
    } else {
      const n = b.parts.length;
      return a.parts.length >= n && a.parts.slice(-n).join('/') === b.parts.join('/');
    }
  }
  return rel.join('/') === b.parts.join('/');
}
const workspaceRoot = (state) => state.workspaceRoot || state.workingDir || state.cwd || state.root || '';
const sha256Hex = async (text) => {
  const subtle = globalThis.crypto?.subtle;
  if (!subtle) return null;
  const buf = await subtle.digest('SHA-256', new TextEncoder().encode(text));
  return [...new Uint8Array(buf)].map((b) => b.toString(16).padStart(2, '0')).join('');
};

export function mount(ctx) {
  const strip = ctx.el('tab-strip');
  if (!strip) return;
  const content = ctx.el('tab-content');
  const eventLog = ctx.doc.createElement('section');
  eventLog.id = 'event-log-content';
  eventLog.className = 'bv-code__events';
  eventLog.hidden = true;
  content?.appendChild(eventLog);
  const events = createEventStore({ limit: 200 });
  // Registration seam: ctx.registerTabKind(kind, mountFn). tabs.js creates one
  // <section id="tab-view-KIND"> under #tab-content, shown only while the active
  // tab has that kind, and calls mountFn({ ctx, host, kind }). mountFn may return
  // { show(tab), hide() }. Views never edit tabs.js and never touch other sections.
  const kinds = new Map();
  ctx.registerTabKind = (kind, mountFn) => {
    if (typeof kind !== 'string' || !kind || kind === 'file' || kind === EVENTS_KIND || kinds.has(kind)) return false;
    const host = ctx.doc.createElement('section');
    host.id = 'tab-view-' + kind;
    host.hidden = true;
    content?.appendChild(host);
    const view = (typeof mountFn === 'function' && mountFn({ ctx, host, kind })) || {};
    kinds.set(kind, { host, view });
    syncViews();
    return true;
  };
  function renderEvents() {
    eventLog.replaceChildren(...events.list().map((row) => {
      const el = ctx.doc.createElement('pre');
      el.className = 'bv-code__event';
      el.textContent = row.label + ' ' + JSON.stringify(row.data).slice(0, 2048);
      return el;
    }));
  }
  function syncViews() {
    const tab = current();
    eventLog.hidden = tab?.kind !== EVENTS_KIND;
    if (!eventLog.hidden) renderEvents();
    for (const [kind, { host, view }] of kinds) {
      const on = tab?.kind === kind;
      host.hidden = !on;
      if (on) view.show?.(tab); else view.hide?.();
    }
  }
  let tabs = [];
  let active = null;
  const tools = new Map();
  const storage = () => { try { return ctx.win.sessionStorage; } catch { return null; } };
  const current = () => tabs.find((tab) => tab.id === active) || null;
  function persist() {
    if (!ctx.state.sessionId) return;
    try { storage()?.setItem(key(ctx.state.sessionId), JSON.stringify({
      tabs: tabs.map(({ id, kind, path, title }) => ({ id, kind, path, title })), active,
    })); } catch { /* private mode or storage quota */ }
  }
  function publish(save = true) {
    ctx.render('tab-strip', renderTabStrip({ tabs: tabs.map((tab) => ({
      ...tab, title: titleOf(tab), icon: tab.kind === 'file' ? 'file-code' : tab.kind.startsWith('agents') ? 'agents' : 'list', closable: true,
    })), active }));
    ctx.emit('tabs:count', { count: tabs.length });
    ctx.emit('tabs:active', { tab: current() });
    syncViews();
    if (save) persist();
  }
  function open(input) {
    if (!valid(input)) return;
    let tab = tabs.find((item) => item.id === input.id);
    if (!tab) {
      tab = { id: input.id, kind: input.kind, path: input.path, title: titleOf(input), dirty: false, content: null, sha256: null, openedAt: ctx.now() };
      tabs.push(tab);
    }
    active = tab.id;
    publish();
  }
  ctx.on('tab:open', open);
  ctx.on('file:open', (path) => {
    if (typeof path === 'string' && path) open({ id: 'file:' + path, kind: 'file', path });
  });
  ctx.on('tab:select', ({ id } = {}) => { if (tabs.some((tab) => tab.id === id)) { active = id; publish(); } });
  ctx.on('tab:close', ({ id } = {}) => {
    const index = tabs.findIndex((tab) => tab.id === id);
    if (index < 0) return;
    tabs.splice(index, 1);
    if (active === id) active = tabs[Math.min(index, tabs.length - 1)]?.id || null;
    publish();
  });
  // V7b can update its in-memory draft without adding another tab owner.
  ctx.on('tab:patch', ({ id, ...patch } = {}) => {
    const tab = tabs.find((item) => item.id === id);
    if (!tab) return;
    for (const field of ['dirty', 'content', 'sha256', 'tooLarge', 'size', 'lang', 'error']) {
      if (Object.hasOwn(patch, field)) tab[field] = patch[field];
    }
    if (active === id) publish();
  });
  ctx.on('session:reset', () => { tabs = []; active = null; tools.clear(); events.reset(); eventLog.replaceChildren(); publish(false); });
  // History replay at open(id) re-sends old events. Only react to events stamped
  // at or after the tab was opened (events without a time are treated as live).
  const live = (msg, tab) => {
    const t = Date.parse(msg?.time);
    return !Number.isFinite(t) || t >= (tab.openedAt || 0);
  };
  const fileTabs = (path, msg) => tabs.filter((tab) => tab.kind === 'file' &&
    samePath(path, tab.path, workspaceRoot(ctx.state)) && live(msg, tab));
  ctx.on('sse', (msg) => {
    if (events.add(msg) && current()?.kind === EVENTS_KIND) renderEvents();
    const p = msg?.data || msg?.payload || {};
    const id = p.id || p.tool_call_id;
    if (msg?.type === 'tool_use_start' && ['FileWrite', 'FileEdit'].includes(p.name || p.tool_name)) {
      if (p.input?.path) tools.set(id, p.input.path);
    } else if (msg?.type === 'tool_use_result') {
      const path = tools.get(id);
      tools.delete(id);
      if (!path || p.error) return;
      fileTabs(path, msg).forEach((tab) => ctx.emit('tab:disk-change', { tab }));
    } else if (msg?.type === 'file_saved') {
      // K2 file_saved {path,size,sha256,decision_source}. Seam for V7b: its own
      // save needs no tab:patch dance; when the saved bytes are what the tab holds
      // the tab gets the new sha256 and goes clean, otherwise it is a real
      // disk change (banner if dirty, silent reload if clean).
      fileTabs(p.path, msg).forEach(async (tab) => {
        const held = typeof tab.content === 'string' ? await sha256Hex(tab.content) : null;
        if (p.sha256 && (held === p.sha256 || tab.sha256 === p.sha256)) {
          ctx.emit('tab:patch', { id: tab.id, sha256: p.sha256, dirty: false });
        } else ctx.emit('tab:disk-change', { tab });
      });
    }
  });
  ctx.on('session:ready', () => {
    let saved = null;
    try { saved = JSON.parse(storage()?.getItem(key(ctx.state.sessionId)) || 'null'); } catch { /* storage unavailable or malformed */ }
    tabs = Array.isArray(saved?.tabs) ? saved.tabs.filter(valid).map((tab) => ({
      id: tab.id, kind: tab.kind, path: tab.path, title: titleOf(tab), dirty: false, content: null, sha256: null, openedAt: ctx.now(),
    })) : [];
    active = tabs.some((tab) => tab.id === saved?.active) ? saved.active : tabs[0]?.id || null;
    publish();
  });
  delegate(strip, 'click', (action, id) => {
    if (action === 'tab') ctx.emit('tab:select', { id });
    else if (action === 'tab-close') {
      // Seam: features/editor.js sets ctx.closeGuard(tab, proceed) to confirm closing a dirty tab.
      const tab = tabs.find((item) => item.id === id);
      const proceed = () => ctx.emit('tab:close', { id });
      if (!(tab?.dirty && ctx.closeGuard?.(tab, proceed))) proceed();
    }
    else if (action === 'tab-new') ctx.emit('tab:open', { id: EVENTS_KIND, kind: EVENTS_KIND, title: 'Event log' });
  });
  publish(false);
}
