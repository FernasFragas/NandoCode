// Permission requests remain pending after the card is dismissed. The shared
// store is the source of truth; this feature owns only the card presentation.
import { h } from '../dom.js';
import { renderPermissionCard } from '../ui/permission.js';
import { delegate } from '../delegate.js';
import { getPendingStore } from '../pendingstore.js';
import { payloadOf } from './transcript.js';

const DECISION = { allow: 'allow', deny: 'deny', 'always-allow': 'always_allow' };

export function mount(ctx) {
  const region = ctx.el('permission');
  if (!region) return;
  const pending = getPendingStore(ctx);
  const sending = new Set();

  function visible() { return pending.list('permission').find((item) => !item.hidden); }

  let lastShownId = null;

  function draw() {
    const head = visible();
    const active = region.ownerDocument.activeElement;
    const hadFocus = !!active && region.contains(active);
    const focusedAction = hadFocus && active.getAttribute ? active.getAttribute('data-action') : null;
    const prevShown = lastShownId;
    lastShownId = head ? head.id : null;
    if (head) {
      const data = head.payload;
      const reason = [data.reason, data.classification ? '(' + data.classification + ')' : ''].filter(Boolean).join(' ');
      ctx.render('permission', renderPermissionCard({ ...data, reason }, { inline: ctx.state.layout.permission === 'inline' }));
      region.hidden = false;
      // Focus Allow only when a different request becomes visible; otherwise keep the user's place.
      const target = head.id !== prevShown || !focusedAction
        ? region.querySelector('[data-action="allow"]')
        : region.querySelector('[data-action="' + focusedAction + '"]');
      if (target && (head.id !== prevShown || hadFocus)) target.focus();
      return;
    }
    const count = pending.count('permission');
    if (count) {
      ctx.render('permission', h('button', {
        type: 'button', class: 'bv-permission-review', 'data-action': 'review',
        'aria-label': 'Review pending permissions',
      }, 'Review pending permissions (' + count + ')'));
      region.hidden = false;
      if (hadFocus) region.querySelector('[data-action="review"]')?.focus();
      return;
    }
    region.replaceChildren();
    region.hidden = true;
    if (hadFocus) ctx.el('message-input')?.focus();
  }

  async function decide(action, id) {
    const decision = DECISION[action];
    if (!decision || !ctx.state.sessionId || !id || sending.has(id) ||
        !pending.list('permission').some((item) => item.id === id)) return;
    sending.add(id);
    try {
      await ctx.api.post('/v1/sessions/' + encodeURIComponent(ctx.state.sessionId) + '/permissions/' + encodeURIComponent(id), { decision });
      pending.resolve('permission', id);
    } catch (err) {
      if (err && err.status === 404) { pending.resolve('permission', id); return; } // already expired server-side
      ctx.emit('notice', { type: 'error', message: 'Permission decision failed: ' + err.message });
    } finally {
      sending.delete(id);
    }
  }

  function clearAll(note) {
    if (!pending.clear('permission')) return;
    if (note) ctx.emit('notice', { type: 'info', message: 'Permission request auto-closed.' });
  }

  ctx.on('pending:changed', ({ kind }) => {
    if (kind !== 'permission') return;
    const count = pending.count('permission');
    if (ctx.state.permissionPending !== count) ctx.set({ permissionPending: count });
    draw();
  });
  ctx.on('sse', (msg) => {
    const p = payloadOf(msg);
    if (msg.type === 'permission_request') {
      if (p.request_id) pending.add('permission', p.request_id, p);
    } else if (msg.type === 'permission_resolved') {
      if (p.request_id) pending.resolve('permission', p.request_id);
    } else if (msg.type === 'terminal' || msg.type === 'error') {
      clearAll(true);
    }
  });

  delegate(region, 'click', (action, id) => {
    if (action === 'dismiss') pending.dismiss('permission', id);
    else if (action === 'review') {
      const item = pending.list('permission')[0];
      if (item) pending.show('permission', item.id);
    } else decide(action, id);
  });
  // Other regions, including the rail, may request the card by ID.
  ctx.on('permission:open', (id) => {
    const item = pending.list('permission').find((candidate) => !id || candidate.id === id);
    if (!item) return;
    pending.show('permission', item.id);
    if (visible()?.id === item.id) region.querySelector('[data-action="allow"]')?.focus();
  });
  ctx.on('escape', (evt) => {
    const head = visible();
    if (evt.handled || !head) return;
    evt.handled = true;
    pending.dismiss('permission', head.id);
  });
  ctx.on('state', (patch) => { if ('layout' in patch && pending.count('permission')) draw(); });
  ctx.on('session:reset', () => { clearAll(false); pending.reset(); });
  region.hidden = true;
}
