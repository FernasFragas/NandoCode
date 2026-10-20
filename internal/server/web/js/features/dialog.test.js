import { test } from 'node:test';
import assert from 'node:assert/strict';
import { makeCtx, fire } from '../fakedom.js';
import { mount as mountKeys } from './keys.js';
import { mount as mountDialog } from './dialog.js';
import { mount as mountSettings } from './settings.js';
import { renderDialog, renderField } from '../dialogview.js';

function setup() {
  const t = makeCtx();
  mountKeys(t.ctx);
  mountDialog(t.ctx);
  return t;
}
const tree = () => renderDialog({ title: 'T', fields: renderField({ id: 'f', label: 'F' }), actions: [{ action: 'go', label: 'Go' }] });

test('open renders the tree, focuses the first input and flags state', () => {
  const t = setup();
  t.ctx.emit('dialog:open', { owner: 'a', tree: tree() });
  assert.ok(t.doc.getElementById('dialog').querySelector('.bv-modal'));
  assert.equal(t.doc.activeElement.getAttribute('id'), 'f');
  assert.equal(t.ctx.state.dialogOpen, true);
});

test('clicks route to the owner as dialog:action and Enter in an input submits', () => {
  const t = setup();
  const actions = [];
  t.ctx.on('dialog:action', (a) => actions.push(a));
  t.ctx.emit('dialog:open', { owner: 'a', tree: tree() });
  fire(t.doc.getElementById('dialog').querySelector('button'), 'click');
  fire(t.doc.getElementById('f'), 'keydown', { key: 'Enter' });
  assert.deepEqual(actions, [{ owner: 'a', action: 'go' }, { owner: 'a', action: 'submit' }]);
});

test('Escape closes the dialog and is marked handled', () => {
  const t = setup();
  const closed = [];
  t.ctx.on('dialog:closed', (c) => closed.push(c.owner));
  t.ctx.emit('dialog:open', { owner: 'a', tree: tree() });
  const evt = fire(t.doc, 'keydown', { key: 'Escape' });
  assert.deepEqual(closed, ['a']);
  assert.equal(t.doc.getElementById('dialog').children.length, 0);
  assert.equal(t.ctx.state.dialogOpen, false);
  assert.equal(evt.defaultPrevented, true);
});

test('settings: gear opens it, save stores the token, empty save complains', () => {
  const t = setup();
  mountSettings(t.ctx);
  t.ctx.emit('settings:open');
  const dialog = t.doc.getElementById('dialog');
  assert.match(dialog.textContent, /Settings/);
  const save = () => fire(dialog.querySelectorAll('button').find((b) => b.getAttribute('data-action') === 'save'), 'click');
  save();
  assert.match(dialog.textContent, /Enter a token first/);
  t.doc.getElementById('settings-token').value = ' secret ';
  save();
  assert.equal(t.ctx.getToken(), 'secret');
  assert.equal(dialog.children.length, 0);
});

test('settings opens by itself on auth:required, once', () => {
  const t = setup();
  mountSettings(t.ctx);
  t.ctx.emit('auth:required');
  t.ctx.emit('auth:required');
  assert.equal(t.doc.getElementById('toasts').children.length, 1);
  assert.match(t.doc.getElementById('dialog').textContent, /Settings/);
});
