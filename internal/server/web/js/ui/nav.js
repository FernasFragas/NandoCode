// Left navigation (DESIGN-SPEC "Nav"). Pure render function.
import { h, icon } from '../dom.js';

export const NAV_ITEMS = [
  { id: 'chat', label: 'Chat', icon: 'chat' },
  { id: 'project', label: 'Project', icon: 'folder' },
  { id: 'agents', label: 'Agents', icon: 'agents' },
  { id: 'transcript', label: 'Transcript', icon: 'transcript' },
  { id: 'tasks', label: 'Tasks', icon: 'tasks' },
  { id: 'diagnostics', label: 'Diagnostics', icon: 'diagnostics' },
];

// data: {active: item id, collapsed: bool}. Collapsed renders the icon rail.
export function renderNav(data = {}) {
  const { active, collapsed } = data;
  return h('nav', { class: 'bv-nav' + (collapsed ? ' bv-nav--collapsed' : ''), 'aria-label': 'Main' },
    NAV_ITEMS.map((item) => renderNavItem(item, { active: item.id === active })));
}

export function renderNavItem(item, opts = {}) {
  return h('button', {
    type: 'button',
    class: 'bv-nav__item' + (opts.active ? ' bv-nav__item--active' : ''),
    'data-action': 'nav',
    'data-id': item.id,
    title: item.label,
    'aria-current': opts.active ? 'page' : null,
  }, icon(item.icon), h('span', { class: 'bv-nav__label' }, item.label));
}
