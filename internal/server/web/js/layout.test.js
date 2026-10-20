import { test } from 'node:test';
import assert from 'node:assert/strict';
import { navMode, treeMode, railMode, permissionMode, layoutFor } from './layout.js';

test('navMode: full from 1440, icon rail below', () => {
  assert.equal(navMode(1920), 'full');
  assert.equal(navMode(1448), 'full');
  assert.equal(navMode(1440), 'full');
  assert.equal(navMode(1439), 'rail');
  assert.equal(navMode(1280), 'rail');
});

test('treeMode: full from 1200, toggle below', () => {
  assert.equal(treeMode(1200), 'full');
  assert.equal(treeMode(1199), 'toggle');
  assert.equal(treeMode(1448), 'full');
});

test('railMode: full from 960, drawer below', () => {
  assert.equal(railMode(960), 'full');
  assert.equal(railMode(959), 'drawer');
  assert.equal(railMode(1280), 'full');
});

test('permissionMode follows the tree breakpoint', () => {
  assert.equal(permissionMode(1200), 'popover');
  assert.equal(permissionMode(1199), 'inline');
});

test('layoutFor bundles every mode', () => {
  assert.deepEqual(layoutFor(1280), { nav: 'rail', tree: 'full', rail: 'full', permission: 'popover' });
  assert.deepEqual(layoutFor(900), { nav: 'rail', tree: 'toggle', rail: 'drawer', permission: 'inline' });
});

test('bad widths fall back to the narrowest layout', () => {
  assert.equal(navMode(NaN), 'rail');
  assert.equal(railMode(undefined), 'drawer');
});
