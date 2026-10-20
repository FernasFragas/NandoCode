// Composer: textarea + Send, and the @ file autocomplete. Input is disabled
// while a run is active or a permission request is pending (as before).
import { h, icon } from '../dom.js';
import { delegate } from '../delegate.js';
import { loadTree } from '../treedata.js';
import { needsCredential } from './credential.js';

const newMessageId = (ctx) => 'msg_' + ctx.now() + '_' + Math.random().toString(36).slice(2, 7);

export function renderMentionList(entries, activeIndex) {
  return h('div', { class: 'bv-mention__list', role: 'listbox' },
    entries.map((e, i) => h('div', {
      class: 'bv-mention__item' + (i === activeIndex ? ' bv-mention__item--active' : ''),
      role: 'option',
      'data-action': 'mention-pick',
      'data-id': String(e.path),
    }, icon(e.kind === 'dir' ? 'folder' : 'file'), h('span', { class: 'bv-mention__path' }, String(e.path)))));
}

export function mount(ctx) {
  const input = ctx.el('message-input');
  const sendBtn = ctx.el('send-btn');
  const dropdown = ctx.el('mention-dropdown');
  if (!input || !sendBtn || !dropdown) return;

  let mentionStart = -1;
  let mentionQuery = '';
  let filtered = [];
  let index = -1;
  let pending = 0;

  function syncDisabled() {
    const off = !!ctx.state.running || pending > 0;
    input.disabled = off;
    sendBtn.disabled = off;
    return off;
  }

  function closeDropdown() {
    dropdown.hidden = true;
    dropdown.replaceChildren();
    mentionStart = -1;
    mentionQuery = '';
    filtered = [];
    index = -1;
  }

  function drawDropdown() {
    if (filtered.length === 0) { closeDropdown(); return; }
    ctx.render('mention-dropdown', renderMentionList(filtered, index));
    dropdown.hidden = false;
  }

  function filter() {
    const q = mentionQuery.toLowerCase();
    const entries = ctx.state.treeEntries || [];
    filtered = entries.filter((e) => (e.path || e.name || '').toLowerCase().includes(q));
    index = filtered.length ? 0 : -1;
    drawDropdown();
  }

  function insertMention(path) {
    const val = input.value;
    const before = val.slice(0, mentionStart);
    const after = val.slice(input.selectionStart);
    input.value = before + '@' + path + ' ' + after;
    const pos = mentionStart + 1 + path.length + 1;
    input.setSelectionRange(pos, pos);
    input.focus();
    closeDropdown();
  }

  // The tree's `@` action: insert at the caret (or the end), spaced from the previous word.
  function insertMentionAtCaret(path) {
    const val = input.value;
    const pos = input.selectionStart || val.length;
    const before = val.slice(0, pos);
    const after = val.slice(pos);
    const prefix = before.length > 0 && !/\s$/.test(before) ? ' ' : '';
    input.value = before + prefix + '@' + path + ' ' + after;
    const next = before.length + prefix.length + 1 + path.length + 1;
    input.setSelectionRange(next, next);
    input.focus();
  }

  async function post(text, messageId) {
    try {
      await ctx.api.post('/v1/sessions/' + encodeURIComponent(ctx.state.sessionId) + '/messages', { prompt: text, message_id: messageId });
    } catch (err) {
      if (needsCredential(err)) {
        ctx.emit('credential:required', { provider: 'ollama_cloud_api', retry: () => post(text, messageId) });
        return;
      }
      ctx.emit('notice', { type: 'error', message: 'Send failed: ' + (err.text || err.message) });
    }
  }

  function send() {
    closeDropdown();
    const text = input.value.trim();
    if (!text || !ctx.state.sessionId || syncDisabled()) return;
    const messageId = newMessageId(ctx);
    ctx.emit('transcript:user-message', { text, id: messageId });
    input.value = '';
    post(text, messageId);
  }

  input.addEventListener('input', () => {
    const val = input.value;
    const caret = input.selectionStart;
    const before = val.slice(0, caret);
    const at = before.lastIndexOf('@');
    if (at === -1) { closeDropdown(); return; }
    const prev = at > 0 ? val[at - 1] : ' ';
    if (prev !== ' ' && prev !== '\n') { closeDropdown(); return; }
    const query = before.slice(at + 1);
    if (query.includes(' ') || query.includes('\n')) { closeDropdown(); return; }
    mentionQuery = query;
    mentionStart = at;
    if (!ctx.state.treeEntries) loadTree(ctx).then(filter);
    else filter();
  });

  input.addEventListener('keydown', (e) => {
    if (filtered.length > 0) {
      if (e.key === 'ArrowDown') { e.preventDefault(); index = (index + 1) % filtered.length; drawDropdown(); return; }
      if (e.key === 'ArrowUp') { e.preventDefault(); index = (index - 1 + filtered.length) % filtered.length; drawDropdown(); return; }
      if (e.key === 'Escape') { closeDropdown(); return; }
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        if (index >= 0) insertMention(filtered[index].path);
        return;
      }
    }
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      send();
    }
  });

  sendBtn.addEventListener('click', send);
  delegate(dropdown, 'click', (action, id) => { if (action === 'mention-pick') insertMention(id); });

  ctx.doc.addEventListener('click', (e) => {
    if (!dropdown.contains(e.target) && e.target !== input) closeDropdown();
  });
  ctx.on('escape', (evt) => {
    if (evt.handled || filtered.length === 0) return;
    evt.handled = true;
    closeDropdown();
  });

  ctx.on('composer:insert-mention', insertMentionAtCaret);
  ctx.on('state', (patch) => {
    if ('permissionPending' in patch) pending = patch.permissionPending;
    if ('running' in patch || 'permissionPending' in patch) {
      const wasOff = input.disabled;
      const off = syncDisabled();
      if (wasOff && !off) input.focus();
    }
  });
  ctx.on('session:reset', () => { closeDropdown(); input.value = ''; });
  syncDisabled();
}
