// Footer: server connection, the URL the page is served from, and the build.
// The version comes from /v1/health when the server reports it.
import { h } from '../dom.js';

const LABEL = { connected: 'Connected', connecting: 'Connecting', disconnected: 'Disconnected', unauthorized: 'Unauthorized' };

export function renderFooter({ connection, url, version }) {
  const state = Object.hasOwn(LABEL, connection) ? connection : 'disconnected';
  return h('div', { class: 'bv-footer' },
    h('span', { class: 'bv-footer__item bv-footer__conn bv-footer__conn--' + state },
      h('span', { class: 'bv-footer__dot' }), 'Server: ' + LABEL[state]),
    url ? h('span', { class: 'bv-footer__item bv-footer__url' }, url) : null,
    h('span', { class: 'bv-footer__item bv-footer__version' }, version ? 'nandocodego ' + version : 'nandocodego'));
}

export function mount(ctx) {
  if (!ctx.el('footer')) return;
  let version = '';
  const url = ctx.win && ctx.win.location ? ctx.win.location.origin : '';
  const draw = () => ctx.render('footer', renderFooter({ connection: ctx.state.connection, url, version }));
  draw();
  async function pollHealth() {
    try {
      ctx.emit('health', await ctx.api.get('/v1/health'));
    } catch {
      ctx.emit('health', { status: 'unreachable' });
    }
  }
  ctx.on('state', (patch) => {
    if (!('connection' in patch)) return;
    draw();
    if (patch.connection === 'connected') pollHealth();
  });
  ctx.on('health', (health) => {
    if (health && typeof health.version === 'string') { version = health.version; draw(); }
  });
}
