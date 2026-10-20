import { test } from 'node:test';
import assert from 'node:assert/strict';
import { makeCtx } from './fakedom.js';
import { getPendingStore } from './pendingstore.js';

test('pending store shares requests per context and publishes changes for rail and saves', () => {
  const { ctx } = makeCtx();
  const events = [];
  ctx.on('pending:changed', (event) => events.push(event));
  const store = getPendingStore(ctx);
  assert.equal(getPendingStore(ctx), store);
  store.add('permission', 'r1', { tool_name: 'run_command' });
  store.add('save', 'path/a', { path: 'path/a' });
  assert.equal(store.list('permission').length, 1);
  assert.equal(store.list('save').length, 1);
  store.dismiss('permission', 'r1');
  assert.equal(store.list('permission')[0].hidden, true);
  store.resolve('permission', 'r1');
  assert.deepEqual(store.list('permission'), []);
  assert.equal(events.at(-1).counts.permission, 0);
  assert.equal(events.at(-1).counts.save, 1);
});

test('resolved and cleared ids are remembered so replays are ignored until reset', () => {
  const { ctx } = makeCtx();
  const store = getPendingStore(ctx);
  store.add('permission', 'r1');
  store.resolve('permission', 'r1');
  assert.equal(store.add('permission', 'r1'), false);
  store.add('permission', 'r2');
  store.clear('permission');
  assert.equal(store.add('permission', 'r2'), false);
  assert.equal(store.add('save', 'r1'), true, 'kinds are independent');
  store.reset();
  assert.equal(store.add('permission', 'r1'), true);
});

test('the remembered-id set is capped and reset publishes pending:changed', () => {
  const { ctx } = makeCtx();
  const store = getPendingStore(ctx);
  for (let i = 0; i < 600; i++) { store.add('permission', 'p' + i); store.resolve('permission', 'p' + i); }
  assert.equal(store.add('permission', 'p0'), true, 'oldest id evicted');
  assert.equal(store.add('permission', 'p599'), false, 'recent id still remembered');
  store.add('save', 's1');
  const events = [];
  ctx.on('pending:changed', (e) => events.push(e));
  store.reset();
  assert.equal(events.length, 1);
  assert.equal(events[0].action, 'reset');
  assert.equal(events[0].counts.save, 0);
  assert.equal(store.count(), 0);
});
