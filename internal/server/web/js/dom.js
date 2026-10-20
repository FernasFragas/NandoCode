// Render-tree builder (contract K1). h() builds plain {tag, attrs, children}
// trees; mount() is the only place that touches the DOM, and it enforces the
// attribute allowlist. Interactive elements use data-action / data-id and a
// delegated listener, never on* attributes.

// XML namespace identifier, not a network endpoint.
const SVG_NS = 'http://www.w3.org/2000/svg';

const SVG_TAGS = new Set([
  'svg', 'use', 'path', 'g', 'circle', 'rect', 'line', 'polyline', 'polygon', 'ellipse', 'text', 'title',
]);

const ALLOWED_ATTRS = new Set([
  'class', 'id', 'href', 'src', 'alt', 'title', 'type', 'role', 'tabindex', 'target', 'rel',
  'name', 'value', 'placeholder', 'disabled', 'hidden', 'open', 'checked', 'selected', 'readonly',
  'for', 'colspan', 'rowspan', 'width', 'height', 'lang',
  // SVG presentation attributes
  'viewBox', 'd', 'fill', 'stroke', 'stroke-width', 'stroke-linecap', 'stroke-linejoin',
  'x', 'y', 'x1', 'y1', 'x2', 'y2', 'cx', 'cy', 'r', 'rx', 'ry', 'points', 'transform', 'opacity',
]);

const URL_ATTRS = new Set(['href', 'src']);

export function h(tag, attrs, ...children) {
  const out = [];
  const add = (c) => {
    if (c === null || c === undefined || c === false) return;
    if (Array.isArray(c)) { c.forEach(add); return; }
    out.push(c);
  };
  children.forEach(add);
  return { tag, attrs: attrs || {}, children: out };
}

export function icon(name) {
  return h('svg', { class: 'icon' }, h('use', { href: 'icons.svg#i-' + name }));
}

function attrAllowed(tag, name, value) {
  if (/^on/i.test(name)) return false;
  if (!(ALLOWED_ATTRS.has(name) || /^data-[a-z0-9_.-]+$/i.test(name) || /^aria-[a-z]+$/i.test(name))) {
    return false;
  }
  if (URL_ATTRS.has(name)) {
    // Browsers ignore whitespace and control characters inside the scheme.
    const v = String(value).replace(/[\u0000- \u007f-\u009f]/g, '').toLowerCase();
    if (v.startsWith('javascript:') || v.startsWith('vbscript:')) return false;
    if (v.startsWith('data:') && !(tag === 'img' && name === 'src')) return false;
  }
  return true;
}

// mount(tree, doc) builds a DOM node. doc defaults to the global document and
// exists so tests can pass a double.
export function mount(tree, doc = globalThis.document) {
  if (tree === null || tree === undefined || tree === false) return doc.createTextNode('');
  if (typeof tree !== 'object') return doc.createTextNode(String(tree));
  const svg = SVG_TAGS.has(tree.tag);
  const el = svg ? doc.createElementNS(SVG_NS, tree.tag) : doc.createElement(tree.tag);
  for (const [name, value] of Object.entries(tree.attrs || {})) {
    if (value === null || value === undefined || value === false) continue;
    if (!attrAllowed(tree.tag, name, value)) continue;
    el.setAttribute(name, value === true ? '' : String(value));
  }
  for (const child of tree.children || []) {
    el.appendChild(mount(child, doc));
  }
  return el;
}
