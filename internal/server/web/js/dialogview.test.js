import { test } from 'node:test';
import assert from 'node:assert/strict';
import { renderDialog, renderField } from './dialogview.js';

const find = (tree, pred, out = []) => {
  if (tree && typeof tree === 'object') {
    if (pred(tree)) out.push(tree);
    (tree.children || []).forEach((c) => find(c, pred, out));
  }
  return out;
};

test('renderDialog renders title, text, error and action buttons with data-action', () => {
  const tree = renderDialog({
    title: 'Settings', description: 'desc', error: 'bad',
    actions: [{ action: 'save', label: 'Save', kind: 'primary' }, { action: 'cancel', label: 'Cancel' }],
  });
  assert.equal(tree.attrs.class, 'bv-modal-backdrop');
  const buttons = find(tree, (n) => n.tag === 'button');
  assert.deepEqual(buttons.map((b) => b.attrs['data-action']), ['save', 'cancel']);
  assert.ok(find(tree, (n) => n.attrs && n.attrs.role === 'alert').length === 1);
  assert.ok(find(tree, (n) => n.tag === 'h2' && n.children[0] === 'Settings').length === 1);
});

test('hostile strings stay text children and no on* attributes appear', () => {
  const tree = renderDialog({ title: '"><script>x', actions: [{ action: 'a', label: '<img onerror=1>' }] });
  const all = find(tree, () => true);
  all.forEach((n) => Object.keys(n.attrs || {}).forEach((k) => assert.ok(!/^on/i.test(k))));
  assert.ok(find(tree, (n) => n.tag === 'script').length === 0);
});

test('renderField links label and input', () => {
  const tree = renderField({ id: 'k', label: 'Key', type: 'password', hint: 'h' });
  const input = find(tree, (n) => n.tag === 'input')[0];
  assert.equal(input.attrs.type, 'password');
  assert.equal(find(tree, (n) => n.tag === 'label')[0].attrs.for, 'k');
});
