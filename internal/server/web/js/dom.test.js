import { test } from 'node:test';
import assert from 'node:assert/strict';
import { h, icon, mount } from './dom.js';

// Minimal DOM double so mount() is testable under plain node.
function fakeDoc() {
  const mk = (ns, tag) => ({
    ns, tag, attrs: {}, children: [],
    setAttribute(k, v) { this.attrs[k] = String(v); },
    appendChild(c) { this.children.push(c); return c; },
  });
  return {
    createElement: (tag) => mk(null, tag),
    createElementNS: (ns, tag) => mk(ns, tag),
    createTextNode: (text) => ({ text: String(text) }),
  };
}

test('h returns {tag, attrs, children} and flattens arrays, skipping null/false/undefined', () => {
  const t = h('ul', { class: 'x' }, [h('li', null, 'a'), null, false, [undefined, 'b']], null);
  assert.equal(t.tag, 'ul');
  assert.deepEqual(t.attrs, { class: 'x' });
  assert.equal(t.children.length, 2);
  assert.equal(t.children[0].tag, 'li');
  assert.equal(t.children[1], 'b');
  assert.deepEqual(h('p').attrs, {});
});

test('icon builds an svg use tree', () => {
  assert.deepEqual(icon('send'), h('svg', { class: 'icon' }, h('use', { href: 'icons.svg#i-send' })));
});

test('mount drops javascript: href', () => {
  const el = mount(h('a', { href: 'javascript:alert(1)' }, 'x'), fakeDoc());
  assert.equal(el.attrs.href, undefined);
  const el2 = mount(h('a', { href: ' JavaScript:alert(1)' }), fakeDoc());
  assert.equal(el2.attrs.href, undefined);
});

test('mount keeps safe href and drops data: except on img', () => {
  assert.equal(mount(h('a', { href: '/docs/page' }), fakeDoc()).attrs.href, '/docs/page');
  assert.equal(mount(h('a', { href: 'data:text/html,x' }), fakeDoc()).attrs.href, undefined);
  assert.equal(mount(h('img', { src: 'data:image/png;base64,AA' }), fakeDoc()).attrs.src, 'data:image/png;base64,AA');
  assert.equal(mount(h('img', { src: 'javascript:x' }), fakeDoc()).attrs.src, undefined);
});

test('mount drops on* attributes', () => {
  const el = mount(h('div', { onclick: 'x', onerror: 'y', class: 'c' }), fakeDoc());
  assert.equal(el.attrs.onclick, undefined);
  assert.equal(el.attrs.onerror, undefined);
  assert.equal(el.attrs.class, 'c');
});

test('mount drops attributes outside the allowlist but keeps data-* and aria-*', () => {
  const el = mount(h('div', { style: 'x', srcdoc: 'x', 'data-action': 'go', 'data-id': '3', 'aria-label': 'l' }), fakeDoc());
  assert.equal(el.attrs.style, undefined);
  assert.equal(el.attrs.srcdoc, undefined);
  assert.equal(el.attrs['data-action'], 'go');
  assert.equal(el.attrs['data-id'], '3');
  assert.equal(el.attrs['aria-label'], 'l');
});

test('text containing <img onerror> stays a text node', () => {
  const s = '<img src=x onerror=alert(1)>';
  const el = mount(h('p', null, s), fakeDoc());
  assert.equal(el.children.length, 1);
  assert.deepEqual(el.children[0], { text: s });
});

test('svg tags use the SVG namespace', () => {
  const el = mount(icon('send'), fakeDoc());
  assert.equal(el.ns, 'http://www.w3.org/2000/svg');
  assert.equal(el.children[0].ns, 'http://www.w3.org/2000/svg');
  assert.equal(el.children[0].attrs.href, 'icons.svg#i-send');
  assert.equal(mount(h('div'), fakeDoc()).ns, null);
});
