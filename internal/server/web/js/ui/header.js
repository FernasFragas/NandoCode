// Header bar (DESIGN-SPEC "Header"). Pure render function: no DOM, window or storage.
import { h, icon } from '../dom.js';

const CONNECTION = {
  connected: 'Connected',
  connecting: 'Connecting',
  disconnected: 'Disconnected',
};

// data: {sessionId, model, models: string[], connection, running}
export function renderHeader(data = {}) {
  const { sessionId, model, models, connection, running } = data;
  const state = Object.hasOwn(CONNECTION, connection) ? connection : 'disconnected';
  const list = Array.isArray(models) ? models : [];
  return h('header', { class: 'bv-header' },
    h('div', { class: 'bv-header__brand' },
      h('span', { class: 'bv-header__name' }, 'NandoCode'),
      h('span', { class: 'bv-header__mode' }, 'Local server mode')),
    h('div', { class: 'bv-header__session' },
      h('span', { class: 'bv-header__label' }, 'Session:'),
      sessionId
        ? [
          h('span', { class: 'bv-header__session-id' }, String(sessionId)),
          h('button', { type: 'button', class: 'bv-header__copy', 'data-action': 'copy-session', 'data-id': String(sessionId), title: 'Copy session id', 'aria-label': 'Copy session id' }, icon('copy')),
        ]
        : h('span', { class: 'bv-header__session-id bv-header__session-id--empty' }, 'No session')),
    list.length
      ? h('select', { class: 'bv-header__model', 'data-action': 'set-model', 'aria-label': 'Model' },
        list.map((m) => h('option', { value: String(m), selected: m === model }, String(m))))
      : null,
    h('span', { class: 'bv-header__conn bv-header__conn--' + state },
      h('span', { class: 'bv-header__dot' }),
      CONNECTION[state]),
    h('button', { type: 'button', class: 'bv-header__stop', 'data-action': 'stop-run', disabled: !running }, icon('stop'), 'Stop run'),
    h('button', { type: 'button', class: 'bv-header__settings', 'data-action': 'open-settings', title: 'Settings', 'aria-label': 'Settings' }, icon('settings')));
}
