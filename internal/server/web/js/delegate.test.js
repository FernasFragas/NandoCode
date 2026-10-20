import { test } from 'node:test';
import assert from 'node:assert/strict';
import { delegate } from './delegate.js';
import { pageWith, fire } from './fakedom.js';
import { mount, h } from './dom.js';

test('resolves the closest [data-action] for rows wrapping inner buttons', () => {
  const doc = pageWith(['region']);
  const region = doc.getElementById('region');
  region.appendChild(mount(h('li', { 'data-action': 'select-session', 'data-id': 's1' },
    h('span', {}, 'title'),
    h('button', { 'data-action': 'delete-session', 'data-id': 's1' }, 'x')), doc));
  const seen = [];
  delegate(region, 'click', (a, id) => seen.push([a, id]));
  fire(region.querySelector('span'), 'click');
  fire(region.querySelector('button'), 'click');
  assert.deepEqual(seen, [['select-session', 's1'], ['delete-session', 's1']]);
});

test('clicks without an action are ignored', () => {
  const doc = pageWith(['region']);
  const region = doc.getElementById('region');
  region.appendChild(mount(h('p', {}, 'plain'), doc));
  const seen = [];
  delegate(region, 'click', (a) => seen.push(a));
  fire(region.querySelector('p'), 'click');
  assert.deepEqual(seen, []);
});
