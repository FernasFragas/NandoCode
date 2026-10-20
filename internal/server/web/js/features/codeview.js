// Read-only file viewer. This section is the only #tab-content child it owns.
import { h, mount as mountTree } from '../dom.js';
import { tokenize } from '../highlight.js';
import { renderMarkdown } from '../markdown.js';
import { delegate } from '../delegate.js';

// Limits (documented in the V6c handoff): highlight up to 256 KB; render the
// first 5000 lines, +5000 per "Show more"; render Markdown as HTML up to 256 KB,
// larger Markdown falls back to the capped plain-source view.
export const MAX_HIGHLIGHT = 256 * 1024;
export const LINE_PAGE = 5000;
export const MAX_MARKDOWN = 256 * 1024;

// Every token type highlight.js can emit maps to a class; css/code.css styles
// each one (func/type use --syn-func/--syn-type). Unknown types render plain.
export const TOKEN_TYPES = ['keyword', 'string', 'comment', 'number', 'func', 'type', 'key', 'property',
  'tag', 'attribute', 'variable', 'boolean', 'null', 'punctuation', 'plain'];
const tokenClass = (type) => TOKEN_TYPES.includes(type) ? type : 'plain';

function codeLines(content, lang, limit) {
  const lines = [[]];
  const tokens = content.length > MAX_HIGHLIGHT ? [{ type: 'plain', text: content }] : tokenize(content, lang);
  for (const token of tokens) {
    const parts = token.text.split('\n');
    parts.forEach((part, index) => {
      if (index) lines.push([]);
      if (part) lines.at(-1).push({ type: tokenClass(token.type), part });
    });
  }
  const shown = lines.slice(0, limit).map((parts, index) => h('div', { class: 'bv-code__line' },
    h('span', { class: 'bv-code__number', 'aria-hidden': 'true' }, String(index + 1)),
    h('span', { class: 'bv-code__text' }, parts.length
      ? parts.map(({ type, part }) => h('span', { class: 'bv-code__token bv-code__token--' + type }, part)) : ' ')));
  return { shown, total: lines.length };
}

export function mount(ctx) {
  const host = ctx.el('tab-content');
  if (!host) return;
  const section = ctx.doc.createElement('section');
  section.id = 'code-content';
  section.className = 'bv-code';
  section.hidden = true;
  host.appendChild(section);
  let active = null;
  let epoch = 0;
  const loading = new Set();
  const changed = new Set();
  const flashed = new Set();
  const pending = new Map(); // tab id -> options of the one trailing refresh
  const shownLines = new Map();

  function draw(tab) {
    // Seam: features/editor.js sets ctx.codeEditor and owns editable file tabs.
    if (!tab || tab.kind !== 'file' || ctx.codeEditor?.claims(tab)) { section.hidden = true; return; }
    section.hidden = false;
    const parts = tab.path.split('/');
    const name = parts.pop();
    const banner = changed.has(tab.id) ? h('div', { class: 'bv-code__banner', role: 'status' },
      'Changed on disk — ',
      h('button', { type: 'button', 'data-action': 'reload-file' }, 'Reload'), ' / ',
      h('button', { type: 'button', 'data-action': 'keep-mine' }, 'Keep mine')) : null;
    const asMarkdown = (tab.lang === 'md' || tab.lang === 'markdown') && tab.content !== null &&
      !tab.tooLarge && tab.content.length <= MAX_MARKDOWN;
    let more = null;
    let body;
    if (tab.tooLarge) body = h('p', { class: 'bv-code__notice' }, 'File is too large to preview (' + tab.size + ' bytes).');
    else if (tab.error?.kind === 'binary') body = h('p', { class: 'bv-code__notice' }, 'Binary or non-UTF-8 file cannot be shown');
    else if (tab.error) body = h('p', { class: 'bv-code__notice' }, 'Could not load ' + name + ': ' + tab.error.message + ' ',
      h('button', { type: 'button', 'data-action': 'retry-load' }, 'Retry'));
    else if (tab.content === null) body = h('p', { class: 'bv-code__notice' }, 'Loading ' + name + '…');
    else if (asMarkdown) body = h('div', { class: 'bv-code__markdown' });
    else {
      const limit = shownLines.get(tab.id) || LINE_PAGE;
      const { shown, total } = codeLines(tab.content, tab.lang, limit);
      if (total > shown.length) more = h('div', { class: 'bv-code__more' },
        'Showing ' + shown.length + ' of ' + total + ' lines ',
        h('button', { type: 'button', 'data-action': 'show-more' }, 'Show more'));
      body = h('pre', { class: 'bv-code__source', 'aria-label': 'File contents' }, h('code', {}, shown));
    }
    section.replaceChildren(mountTree(h('div', { class: 'bv-code__view' + (flashed.has(tab.id) ? ' bv-code__view--reloaded' : '') },
      h('div', { class: 'bv-code__breadcrumb' }, [...parts, name].map((part, i) => h('span', { class: 'bv-code__crumb' },
        i ? ' / ' : '', part))),
      banner, body, more,
      h('div', { class: 'bv-code__status' },
        h('span', {}, tab.lang ? tab.lang.toUpperCase() : 'Plain text'),
        h('span', {}, 'Ln —, Col —'), h('span', {}, 'Spaces: —'), h('span', {}, 'UTF-8'))), ctx.doc));
    if (asMarkdown) {
      section.querySelector('.bv-code__markdown').innerHTML = renderMarkdown(tab.content);
    }
  }

  // opts.flash: highlight the reload; opts.clearDirty: mark the tab clean, but
  // only once the new content arrived (a failed reload keeps the draft + banner).
  async function load(tab, opts = {}) {
    if (!ctx.state.sessionId) return;
    if (loading.has(tab.id)) { pending.set(tab.id, { ...pending.get(tab.id), ...opts }); return; }
    const requestEpoch = epoch;
    const session = ctx.state.sessionId;
    loading.add(tab.id);
    try {
      const file = await ctx.api.get('/v1/sessions/' + encodeURIComponent(session) + '/files?path=' + encodeURIComponent(tab.path));
      if (requestEpoch !== epoch || ctx.state.sessionId !== session) return;
      if (file?.kind !== 'file') throw Error('Not a file');
      if (opts.flash) flashed.add(tab.id);
      changed.delete(tab.id);
      const patch = { id: tab.id, content: file.too_large ? null : file.content, error: null,
        sha256: file.sha256 || null, tooLarge: !!file.too_large, size: file.size, lang: file.lang || '' };
      if (opts.clearDirty) patch.dirty = false;
      ctx.emit('tab:patch', patch);
    } catch (err) {
      if (requestEpoch === epoch) {
        if (err?.status === 415) ctx.emit('tab:patch', { id: tab.id, error: { kind: 'binary' } });
        else {
          ctx.emit('tab:patch', { id: tab.id, error: { kind: 'failed', message: err.message } });
          ctx.toast('Could not load ' + tab.path + ': ' + err.message);
        }
      }
    } finally {
      loading.delete(tab.id);
      const again = pending.get(tab.id);
      pending.delete(tab.id);
      if (again && requestEpoch === epoch) load(tab, again);
    }
  }

  ctx.on('tabs:active', ({ tab } = {}) => {
    active = tab?.kind === 'file' ? tab : null;
    draw(active);
    if (active && active.content === null && !active.tooLarge && !active.error && !loading.has(active.id)) load(active);
  });
  ctx.on('session:reset', () => { epoch++; active = null; changed.clear(); flashed.clear(); pending.clear(); shownLines.clear(); section.hidden = true; });
  ctx.on('tab:disk-change', ({ tab }) => {
    if (tab.dirty) { changed.add(tab.id); if (active?.id === tab.id) draw(tab); }
    else load(tab, { flash: true });
  });
  delegate(section, 'click', (action) => {
    if (!active) return;
    if (action === 'reload-file') load(active, { flash: true, clearDirty: true });
    else if (action === 'retry-load') { ctx.emit('tab:patch', { id: active.id, error: null }); }
    else if (action === 'show-more') { shownLines.set(active.id, (shownLines.get(active.id) || LINE_PAGE) + LINE_PAGE); draw(active); }
    else if (action === 'keep-mine') { changed.delete(active.id); draw(active); }
  });
}
