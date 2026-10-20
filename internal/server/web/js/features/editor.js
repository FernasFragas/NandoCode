// Editable code view (V7b). Extends tab kind 'file' through the seam
// ctx.codeEditor.claims(tab): codeview.js hides its read-only section for tabs
// this module claims. The draft lives in a per-tab <textarea>; tabs.js keeps the
// dirty flag, content and base sha256 (via tab:patch). No innerHTML anywhere.
import { h, mount as mountTree } from '../dom.js';
import { delegate } from '../delegate.js';
import { getPendingStore } from '../pendingstore.js';
import { renderSaveConfirmCard } from '../ui/permission.js';
import { renderDialog } from '../dialogview.js';

// Files over this size (same cap as highlighting) stay read-only in codeview.
// Files containing CR also stay read-only: a textarea would normalise CRLF to LF
// and silently rewrite the file's line endings.
export const MAX_EDIT = 256 * 1024;
const MAX_DIFF_LINES = 1500;

const claims = (tab) => !!tab && tab.kind === 'file' && typeof tab.content === 'string' && !tab.tooLarge &&
  !tab.error && tab.content.length <= MAX_EDIT && !tab.content.includes('\r');

// Line diff (LCS) of disk vs draft; null when too large.
export function diffLines(disk, mine) {
  const a = disk.split('\n'), b = mine.split('\n');
  if (a.length > MAX_DIFF_LINES || b.length > MAX_DIFF_LINES) return null;
  const dp = Array.from({ length: a.length + 1 }, () => new Array(b.length + 1).fill(0));
  for (let i = a.length - 1; i >= 0; i--) {
    for (let j = b.length - 1; j >= 0; j--) dp[i][j] = a[i] === b[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
  }
  const out = [];
  let i = 0, j = 0;
  while (i < a.length || j < b.length) {
    if (i < a.length && j < b.length && a[i] === b[j]) { i++; j++; } else if (j < b.length && (i >= a.length || dp[i][j + 1] >= dp[i + 1][j])) out.push({ sign: '+', text: b[j++] });
    else out.push({ sign: '-', text: a[i++] });
  }
  return out;
}

export function mount(ctx) {
  const host = ctx.el('tab-content');
  if (!host) return;
  ctx.codeEditor = { claims };
  const pending = getPendingStore(ctx);
  const section = ctx.doc.createElement('section');
  section.id = 'code-editor';
  section.className = 'bv-editor';
  section.hidden = true;
  const head = ctx.doc.createElement('div');
  const notes = ctx.doc.createElement('div');
  notes.className = 'bv-editor__notes';
  const body = ctx.doc.createElement('div');
  body.className = 'bv-editor__body';
  const status = ctx.doc.createElement('div');
  section.appendChild(head); section.appendChild(notes); section.appendChild(body); section.appendChild(status);
  host.appendChild(section);

  const states = new Map(); // tab id -> {ta, saved, seen, changed, conflict, diff, note, saving, confirm}
  let cur = null;
  let seq = 0;
  const url = (path) => '/v1/sessions/' + encodeURIComponent(ctx.state.sessionId) + '/files?path=' + encodeURIComponent(path);

  function stateOf(tab) {
    let st = states.get(tab.id);
    if (st) return st;
    const ta = ctx.doc.createElement('textarea');
    ta.className = 'bv-editor__input';
    ta.setAttribute('spellcheck', 'false');
    ta.setAttribute('aria-label', 'Editing ' + tab.path);
    ta.value = tab.content;
    st = { ta, saved: tab.content, seen: tab.content, base: tab.sha256, diffSha: null, changed: false, conflict: false, diff: null, note: null, saving: false, confirm: null };
    ta.addEventListener('input', () => { if (cur && states.get(cur.id) === st) syncDirty(cur, st); drawChrome(); });
    ta.addEventListener('keydown', (evt) => {
      if ((evt.ctrlKey || evt.metaKey) && String(evt.key).toLowerCase() === 's') {
        evt.preventDefault();
        if (cur) save(cur);
      }
    });
    states.set(tab.id, st);
    return st;
  }

  function syncDirty(tab, st) {
    const dirty = st.ta.value !== st.saved;
    if (tab.dirty !== dirty) ctx.emit('tab:patch', { id: tab.id, dirty });
  }

  function visibleCard(tab) {
    return pending.list('save').find((item) => !item.hidden && item.payload.tab === tab.id);
  }

  function drawChrome() {
    if (!cur) return;
    const tab = cur, st = states.get(tab.id);
    if (!st) return;
    const dirty = st.ta.value !== st.saved;
    const button = (action, label) => h('button', { type: 'button', 'data-action': action }, label);
    const items = [];
    if (st.conflict) {
      items.push(h('div', { class: 'bv-code__banner', role: 'alert' },
        'File changed on disk — your draft was kept. ',
        button('conflict-reload', 'Reload'), ' / ', button('conflict-diff', 'Show diff'), ' / ', button('conflict-overwrite', 'Overwrite')));
    } else if (st.changed) {
      items.push(h('div', { class: 'bv-code__banner', role: 'status' },
        'Changed on disk — ', button('editor-reload', 'Reload'), ' / ', button('editor-keep', 'Keep mine')));
    }
    if (st.diff) {
      items.push(h('pre', { class: 'bv-editor__diff', 'aria-label': 'Diff between disk and your draft' }, st.diff.lines
        ? st.diff.lines.map((l) => h('div', { class: 'bv-editor__diff-line bv-editor__diff-line--' + (l.sign === '+' ? 'add' : 'del') }, l.sign + ' ' + l.text))
        : 'Too large to diff.'));
    }
    if (st.note) {
      items.push(h('div', { class: 'bv-code__banner', role: 'status' }, st.note.message + ' ',
        st.note.retry ? button('retry-save', 'Retry') : null));
    }
    const card = visibleCard(tab);
    const hidden = pending.list('save').filter((item) => item.hidden && item.payload.tab === tab.id);
    if (card) items.push(renderSaveConfirmCard({ request_id: card.id, path: tab.path }, { inline: true }));
    else if (hidden.length) items.push(h('button', { type: 'button', class: 'bv-permission-review', 'data-action': 'review-save' }, 'Review pending save'));
    notes.replaceChildren(...items.map((tree) => mountTree(tree, ctx.doc)));
    head.className = 'bv-editor__head';
    head.replaceChildren(mountTree(h('div', { class: 'bv-editor__headrow' },
      h('span', { class: 'bv-code__breadcrumb' }, tab.path),
      h('button', { type: 'button', class: 'bv-editor__save', 'data-action': 'save', disabled: !dirty || st.saving || st.confirm ? true : null },
        st.saving ? 'Saving…' : 'Save')), ctx.doc));
    status.className = 'bv-code__status';
    status.replaceChildren(mountTree(h('div', {}, h('span', {}, dirty ? 'Modified' : 'Saved'), ' ',
      h('span', {}, tab.lang ? tab.lang.toUpperCase() : 'Plain text'), ' ', h('span', {}, 'UTF-8')), ctx.doc));
  }

  function render(tab) {
    if (!claims(tab)) { cur = null; section.hidden = true; return; }
    cur = tab;
    section.hidden = false;
    const st = stateOf(tab);
    if (tab.content !== st.seen) { // reloaded from disk (or replaced externally)
      st.seen = tab.content;
      if (st.ta.value !== st.saved) { // the user has typed: never replace the draft
        st.changed = true; st.stale = true;
      } else {
        st.saved = tab.content; st.ta.value = tab.content; st.changed = false; st.stale = false;
      }
    }
    if (!st.stale) st.base = tab.sha256; // else keep the sha the draft started from: a save then hits 409
    if (st.ta.parentNode !== body) body.replaceChildren(st.ta);
    syncDirty(tab, st);
    drawChrome();
  }

  function reason(err, fallback) { return (err?.body && err.body.reason) || fallback || err?.message || 'Save failed'; }

  async function save(tab, opts = {}) {
    const st = states.get(tab.id);
    if (!st || st.saving || !ctx.state.sessionId) return;
    if (st.confirm) { // one save at a time: the pending card must be answered first
      st.note = { message: 'A save is waiting for your approval.' };
      pending.show('save', st.confirm.id);
      drawChrome();
      return;
    }
    const content = opts.content ?? st.ta.value;
    const base = opts.base ?? st.base ?? tab.sha256;
    if (opts.content === undefined && content === st.saved && !opts.confirm) return;
    const session = ctx.state.sessionId;
    const body = { content, base_sha256: base };
    if (opts.confirm) body.confirm = opts.confirm;
    st.saving = true; st.note = null;
    drawChrome();
    try {
      const res = await ctx.api.json('PUT', url(tab.path), body);
      if (ctx.state.sessionId !== session || !states.has(tab.id)) return;
      if (res?.needs_confirm) {
        const id = 'save:' + tab.id + ':' + (++seq);
        st.confirm = { id, content, base };
        pending.add('save', id, { path: tab.path, tab: tab.id });
      } else if (res?.sha256) {
        st.saved = content; st.seen = content; st.base = res.sha256; st.stale = false; st.conflict = false; st.diff = null; st.diffSha = null; st.changed = false; st.confirm = null;
        ctx.emit('tab:patch', { id: tab.id, sha256: res.sha256, content, dirty: st.ta.value !== content });
      } else {
        throw Error('Unexpected save response');
      }
    } catch (err) {
      if (!states.has(tab.id)) return;
      if (err?.status === 409) { st.conflict = true; } else if (err?.status === 423) {
        st.note = { message: reason(err, 'The agent is writing this file.') + ' Try again shortly.', retry: true };
        ctx.toast(st.note.message);
      } else if (err?.status === 403) {
        st.note = { message: 'Save denied: ' + reason(err) };
        ctx.toast(st.note.message);
      } else {
        st.note = { message: 'Save failed: ' + reason(err), retry: true };
        ctx.toast(st.note.message);
      }
    } finally {
      st.saving = false;
      if (cur && cur.id === tab.id) drawChrome();
    }
  }

  async function fetchDisk(tab) {
    const file = await ctx.api.get(url(tab.path));
    if (file?.kind !== 'file' || typeof file.content !== 'string') throw Error('Not an editable file');
    return file;
  }

  async function reload(tab) {
    const st = states.get(tab.id);
    if (!st) return;
    try {
      const file = await fetchDisk(tab);
      st.saved = file.content; st.seen = file.content; st.ta.value = file.content; st.base = file.sha256 || null; st.stale = false;
      st.conflict = false; st.changed = false; st.diff = null; st.diffSha = null; st.note = null;
      ctx.emit('tab:patch', { id: tab.id, content: file.content, sha256: file.sha256 || null, dirty: false });
      drawChrome();
    } catch (err) {
      st.note = { message: 'Could not reload: ' + err.message };
      ctx.toast(st.note.message);
      drawChrome();
    }
  }

  async function showDiff(tab) {
    const st = states.get(tab.id);
    if (!st) return;
    try {
      const file = await fetchDisk(tab);
      st.diff = { lines: diffLines(file.content, st.ta.value) }; st.diffSha = file.sha256 || null;
    } catch (err) {
      st.note = { message: 'Could not load the disk version: ' + err.message };
    }
    drawChrome();
  }

  async function overwrite(tab) {
    const st = states.get(tab.id);
    if (!st) return;
    try {
      const file = await fetchDisk(tab);
      if (!st.diff || st.diffSha !== (file.sha256 || null)) { // the user has not seen this disk version yet
        const again = !!st.diff;
        st.diff = { lines: diffLines(file.content, st.ta.value) }; st.diffSha = file.sha256 || null;
        st.note = { message: again ? 'The file changed again on disk. Review the updated diff, then Overwrite.' : 'Review the diff, then Overwrite.' };
        drawChrome();
        return;
      }
      await save(tab, { base: file.sha256, content: st.ta.value });
    } catch (err) {
      st.note = { message: 'Could not overwrite: ' + err.message };
      ctx.toast(st.note.message);
      drawChrome();
    }
  }

  function decide(action, id) {
    if (!cur) return;
    const st = states.get(cur.id);
    if (!st || !st.confirm || st.confirm.id !== id) return;
    const { content, base } = st.confirm;
    pending.resolve('save', id);
    st.confirm = null;
    if (action === 'save-cancel') { st.note = { message: 'Save cancelled. Your draft is kept.' }; drawChrome(); return; }
    save(cur, { content, base, confirm: action === 'save-always' ? 'always' : 'once' });
  }

  delegate(section, 'click', (action, id) => {
    if (!cur) return;
    if (action === 'save') save(cur);
    else if (action === 'retry-save') { const st = states.get(cur.id); if (st) st.note = null; save(cur); }
    else if (action === 'conflict-reload' || action === 'editor-reload') reload(cur);
    else if (action === 'conflict-diff') showDiff(cur);
    else if (action === 'conflict-overwrite') overwrite(cur);
    else if (action === 'editor-keep') { const st = states.get(cur.id); if (st) { st.changed = false; drawChrome(); } }
    else if (action === 'save-once' || action === 'save-always' || action === 'save-cancel') decide(action, id);
    else if (action === 'dismiss') pending.dismiss('save', id);
    else if (action === 'review-save') { const item = pending.list('save').find((i) => i.payload.tab === cur.id); if (item) pending.show('save', item.id); }
  });

  ctx.on('tabs:active', ({ tab } = {}) => render(tab));
  ctx.on('tab:disk-change', ({ tab } = {}) => {
    const st = tab && states.get(tab.id);
    if (!st) return;
    if (st.ta.value !== st.saved) { st.changed = true; if (cur && cur.id === tab.id) drawChrome(); }
  });
  ctx.on('tab:close', ({ id } = {}) => {
    const st = states.get(id);
    if (!st) return;
    if (st.confirm) pending.resolve('save', st.confirm.id);
    states.delete(id);
  });
  ctx.on('pending:changed', ({ kind } = {}) => { if (kind === 'save' || kind === null) drawChrome(); });
  ctx.on('escape', (evt) => {
    if (evt.handled || !cur) return;
    const card = visibleCard(cur);
    if (!card) return;
    evt.handled = true;
    pending.dismiss('save', card.id);
  });
  ctx.on('sse', (msg) => {
    if (msg?.type !== 'hook_notice') return;
    const p = msg.data || msg.payload || {};
    if (p.message) ctx.toast(p.message, 'info');
  });
  // Closing a dirty tab asks first (tabs.js calls ctx.closeGuard from the strip's close button).
  let closing = null;
  ctx.closeGuard = (tab, proceed) => {
    const st = states.get(tab.id);
    if (!st || st.ta.value === st.saved) return false;
    closing = { owner: 'editor-close', proceed };
    ctx.emit('dialog:open', { owner: closing.owner, tree: renderDialog({
      title: 'Discard unsaved changes?',
      description: tab.path + ' has unsaved changes. Closing it discards your draft.',
      actions: [{ action: 'cancel', label: 'Cancel' }, { action: 'discard', label: 'Discard', kind: 'danger' }],
    }) });
    return true;
  };
  ctx.on('dialog:action', ({ owner, action } = {}) => {
    if (!closing || owner !== closing.owner) return;
    const { proceed } = closing;
    closing = null;
    ctx.emit('dialog:close', owner);
    if (action === 'discard') proceed();
  });
  ctx.on('session:reset', () => {
    // tabs.js clears every tab on reset (V5b's flow cannot be blocked from here), so name what is lost.
    const lost = [...states.entries()].filter(([, st]) => st.ta.value !== st.saved).map(([id]) => id.replace(/^file:/, ''));
    if (lost.length) ctx.toast('Unsaved drafts discarded: ' + lost.join(', '));
    pending.clear('save');
    states.clear(); cur = null; body.replaceChildren(); section.hidden = true;
  });
}
