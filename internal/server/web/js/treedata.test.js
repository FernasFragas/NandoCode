import { test } from 'node:test';
import assert from 'node:assert/strict';
import { buildTree, loadTree, resetTree } from './treedata.js';

test('buildTree nests by path, dirs first, sorted', () => {
  const tree = buildTree([
    { path: 'b.go', kind: 'file' },
    { path: 'src', kind: 'dir' },
    { path: 'src/a.js', kind: 'file' },
    { path: 'src/lib', kind: 'dir' },
    { path: './README.md', kind: 'file' },
  ]);
  assert.deepEqual(tree.map((n) => n.name), ['src', 'b.go', 'README.md']);
  const src = tree[0];
  assert.equal(src.kind, 'dir');
  assert.deepEqual(src.children.map((n) => n.name), ['lib', 'a.js']);
  assert.equal(src.children[0].kind, 'dir');
  assert.deepEqual(src.children[0].children, []);
});

test('buildTree makes parents of orphan paths and ignores junk', () => {
  const tree = buildTree([{ path: 'a/b/c.txt' }, { path: '' }]);
  assert.equal(tree[0].name, 'a');
  assert.equal(tree[0].children[0].children[0].name, 'c.txt');
  assert.deepEqual(buildTree(undefined), []);
});

function ctxFor(get) {
  return { state: { sessionId: 's1' }, api: { get }, emit() {} };
}

test('loadTree caches per session and shares an in-flight request', async () => {
  let calls = 0;
  const ctx = ctxFor(async (path) => { calls += 1; assert.match(path, /^\/v1\/sessions\/s1\/files\?path=/); return { entries: [{ name: 'x', kind: 'file', size: 1 }] }; });
  resetTree(ctx);
  const [a, b] = await Promise.all([loadTree(ctx), loadTree(ctx)]);
  assert.equal(calls, 1);
  assert.deepEqual(a, b);
  await loadTree(ctx);
  assert.equal(calls, 1);
  await loadTree(ctx, { force: true });
  assert.equal(calls, 2);
});

test('loadTree reports errors and returns []', async () => {
  const errs = [];
  const ctx = { state: { sessionId: 's1' }, api: { get: async () => { throw new Error('x'); } }, emit: (n, e) => errs.push([n, e.message]) };
  resetTree(ctx);
  assert.deepEqual(await loadTree(ctx), []);
  assert.deepEqual(errs, [['tree:error', 'x']]);
});

test('loadTree without a session is empty', async () => {
  assert.deepEqual(await loadTree({ state: { sessionId: null } }), []);
});

test('loadTree walks subdirectories and builds workspace-relative paths', async () => {
  const seen = [];
  const answers = {
    '.': [{ name: 'src', kind: 'dir', size: 0 }, { name: 'a.go', kind: 'file', size: 1 }],
    'src': [{ name: 'b.js', kind: 'file', size: 2 }],
  };
  const ctx = ctxFor(async (path) => {
    const dir = decodeURIComponent(path.split('path=')[1]);
    seen.push(dir);
    return { kind: 'dir', entries: answers[dir] || [] };
  });
  resetTree(ctx);
  const entries = await loadTree(ctx);
  assert.deepEqual(seen, ['.', 'src']);
  assert.deepEqual(entries.map((e) => e.path), ['src', 'a.go', 'src/b.js']);
});

test('a failed subdirectory keeps loaded entries and flags that directory', async () => {
  const errs = [];
  const ctx = {
    state: { sessionId: 's1' },
    emit: (n, e) => errs.push([n, e.message]),
    api: {
      get: async (path) => {
        const dir = decodeURIComponent(path.split('path=')[1]);
        if (dir === 'bad') throw new Error('boom');
        if (dir === '.') return { entries: [{ name: 'bad', kind: 'dir', size: 0 }, { name: 'good', kind: 'dir', size: 0 }, { name: 'a.go', kind: 'file', size: 1 }] };
        return { entries: [{ name: 'x.js', kind: 'file', size: 1 }] };
      },
    },
  };
  resetTree(ctx);
  const entries = await loadTree(ctx);
  assert.deepEqual(entries.map((e) => e.path), ['bad', 'good', 'a.go', 'good/x.js']);
  assert.equal(entries.find((e) => e.path === 'bad').error, true);
  assert.equal(entries.find((e) => e.path === 'good').error, undefined);
  assert.deepEqual(errs, [['tree:error', 'boom']]);
});
