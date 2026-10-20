// Project tree (DESIGN-SPEC "Tree row"). Pure render functions.
// Entries follow the K3 files API ({name, kind: 'dir'|'file', size}); a dir gets
// `children` once loaded (undefined = not loaded yet). Indentation is the
// data-depth attribute, styled by CSS; no inline styles.
import { h, icon } from '../dom.js';

export function renderTreeRow(entry, opts = {}) {
  const dir = entry.kind === 'dir';
  const path = opts.path ?? entry.name;
  const depth = opts.depth ?? 0;
  const expanded = !!opts.expanded;
  return h('li', {
    class: 'bv-tree__row bv-tree__row--' + (dir ? 'dir' : 'file'),
    role: 'treeitem',
    'data-action': 'tree-select',
    'data-id': String(path),
    'data-depth': depth,
    'aria-expanded': dir ? String(expanded) : null,
  },
  dir
    ? h('button', { type: 'button', class: 'bv-tree__chevron', 'data-action': 'tree-toggle', 'data-id': String(path), 'aria-label': expanded ? 'Collapse' : 'Expand' }, icon(expanded ? 'chevron-down' : 'chevron-right'))
    : h('span', { class: 'bv-tree__chevron bv-tree__chevron--none' }),
  icon(dir ? 'folder' : 'file'),
  h('span', { class: 'bv-tree__name' }, String(entry.name)),
  h('button', { type: 'button', class: 'bv-tree__mention', 'data-action': 'mention', 'data-id': String(path), title: 'Mention in message', 'aria-label': 'Mention in message' }, icon('at')));
}

function rows(entries, parent, depth, expandedSet) {
  const out = [];
  for (const entry of entries) {
    const path = parent ? parent + '/' + entry.name : String(entry.name);
    const dir = entry.kind === 'dir';
    const open = dir && expandedSet.has(path);
    out.push(renderTreeRow(entry, { path, depth, expanded: open }));
    if (open) {
      if (!Array.isArray(entry.children)) {
        out.push(h('li', { class: 'bv-tree__note', 'data-depth': depth + 1 }, 'Loading…'));
      } else if (entry.children.length === 0) {
        out.push(h('li', { class: 'bv-tree__note', 'data-depth': depth + 1 }, 'Empty folder'));
      } else {
        out.push(...rows(entry.children, path, depth + 1, expandedSet));
      }
    }
  }
  return out;
}

// data: {entries: [], expanded: [paths]}
export function renderTree(data = {}) {
  const entries = Array.isArray(data.entries) ? data.entries : [];
  const expanded = new Set(Array.isArray(data.expanded) ? data.expanded : []);
  return h('section', { class: 'bv-tree' },
    h('div', { class: 'bv-tree__head' }, 'Project'),
    entries.length
      ? h('ul', { class: 'bv-tree__list', role: 'tree' }, rows(entries, '', 0, expanded))
      : h('p', { class: 'bv-tree__empty' }, 'No files'));
}
