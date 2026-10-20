// Center-bottom tab strip (DESIGN-SPEC "Tab strip"). Pure render functions.
import { h, icon } from '../dom.js';

// tab: {id, title, icon, closable}.
export function renderTab(tab = {}, opts = {}) {
  const id = tab.id === undefined || tab.id === null ? null : String(tab.id);
  return h('div', { class: 'bv-tabs__tab' + (opts.active ? ' bv-tabs__tab--active' : '') + (tab.dirty ? ' bv-tabs__tab--dirty' : ''), role: 'presentation' },
    h('button', {
      type: 'button',
      class: 'bv-tabs__select',
      role: 'tab',
      'data-action': 'tab',
      'data-id': id,
      'aria-selected': opts.active ? 'true' : 'false',
      title: tab.title === undefined ? null : String(tab.title),
    },
    tab.icon ? icon(String(tab.icon)) : null,
    h('span', { class: 'bv-tabs__title' }, tab.title === undefined || tab.title === null ? '' : String(tab.title)),
    tab.dirty ? h('span', { class: 'bv-tabs__dirty', role: 'img', 'aria-label': 'Unsaved changes', title: 'Unsaved changes' }, '●') : null),
    tab.closable ? h('button', {
      type: 'button',
      class: 'bv-tabs__close',
      'data-action': 'tab-close',
      'data-id': id,
      title: 'Close',
      'aria-label': 'Close tab',
    }, icon('x')) : null);
}

// data: {tabs, active}. No tabs renders only the + button.
export function renderTabStrip(data = {}) {
  const tabs = Array.isArray(data.tabs) ? data.tabs : [];
  return h('div', { class: 'bv-tabs', role: 'tablist' },
    tabs.map((t) => renderTab(t, { active: t.id === data.active })),
    h('button', { type: 'button', class: 'bv-tabs__new', 'data-action': 'tab-new', title: 'New tab', 'aria-label': 'New tab' }, icon('plus')));
}
