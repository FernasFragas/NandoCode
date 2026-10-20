// Sessions list (DESIGN-SPEC "Session row"). Pure render functions.
// Data is the K3 GET /v1/sessions shape: {id, title, created_at, last_active, running, model}.
import { h, icon } from '../dom.js';

// "5m ago" style label. now is epoch ms, injected so the function stays pure.
export function formatAgo(iso, now) {
  const t = Date.parse(iso);
  if (!iso || Number.isNaN(t) || typeof now !== 'number') return '';
  const s = Math.max(0, Math.floor((now - t) / 1000));
  if (s < 60) return 'just now';
  if (s < 3600) return Math.floor(s / 60) + 'm ago';
  if (s < 86400) return Math.floor(s / 3600) + 'h ago';
  return Math.floor(s / 86400) + 'd ago';
}

export function renderSessionRow(session, opts = {}) {
  const active = !!opts.active;
  const id = String(session.id ?? '');
  const ago = formatAgo(session.last_active || session.created_at, opts.now);
  return h('li', {
    class: 'bv-sessions__row' + (active ? ' bv-sessions__row--active' : '') + (session.running ? ' bv-sessions__row--running' : ''),
    'data-action': 'select-session',
    'data-id': id,
    'aria-current': active ? 'true' : null,
  },
  icon(active ? 'circle-dot' : 'circle'),
  h('div', { class: 'bv-sessions__text' },
    h('span', { class: 'bv-sessions__title' }, session.title || 'Untitled'),
    h('span', { class: 'bv-sessions__meta' },
      h('span', { class: 'bv-sessions__id' }, id),
      ago ? h('span', { class: 'bv-sessions__time' }, ago) : null)),
  h('button', { type: 'button', class: 'bv-sessions__delete', 'data-action': 'delete-session', 'data-id': id, title: 'Delete session', 'aria-label': 'Delete session' }, icon('trash')));
}

// data: {sessions: [], activeId}
export function renderSessionList(data = {}, opts = {}) {
  const sessions = Array.isArray(data.sessions) ? data.sessions : [];
  return h('section', { class: 'bv-sessions' },
    h('div', { class: 'bv-sessions__head' },
      h('span', { class: 'bv-sessions__heading' }, 'Sessions'),
      h('button', { type: 'button', class: 'bv-sessions__new', 'data-action': 'new-session' }, icon('plus'), 'New session')),
    sessions.length
      ? h('ul', { class: 'bv-sessions__list' },
        sessions.map((s) => renderSessionRow(s, { active: s.id === data.activeId, now: opts.now })))
      : h('p', { class: 'bv-sessions__empty' }, 'No sessions yet'));
}
