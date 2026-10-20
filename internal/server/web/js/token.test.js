import { test } from 'node:test';
import assert from 'node:assert/strict';
import { extractToken, stripToken, adoptToken, loadToken, saveToken } from './token.js';

test('extractToken reads the token from the fragment', () => {
  assert.equal(extractToken('#token=abc&x=1'), 'abc');
  assert.equal(extractToken('#x=1&token=abc'), 'abc');
  assert.equal(extractToken('#token=a%20b'), 'a b');
});

test('extractToken returns null when absent or empty', () => {
  assert.equal(extractToken(''), null);
  assert.equal(extractToken('#x=1'), null);
  assert.equal(extractToken('#token='), null);
  assert.equal(extractToken(undefined), null);
});

test('stripToken removes the token and keeps other params', () => {
  assert.equal(stripToken('#token=abc&x=1'), '#x=1');
  assert.equal(stripToken('#x=1&token=abc'), '#x=1');
  assert.equal(stripToken('#token=abc'), '');
  assert.ok(!stripToken('#token=abc&x=1').includes('abc'));
});

function fakeStorage(seed = {}, throws = false) {
  const data = { ...seed };
  return {
    data,
    getItem(k) { if (throws) throw new Error('blocked'); return Object.hasOwn(data, k) ? data[k] : null; },
    setItem(k, v) { if (throws) throw new Error('blocked'); data[k] = String(v); },
  };
}

test('adoptToken stores in sessionStorage and cleans the URL', () => {
  const storage = fakeStorage();
  const calls = [];
  const win = {
    location: { hash: '#token=abc&x=1', pathname: '/', search: '?q=1' },
    history: { replaceState: (...a) => calls.push(a) },
  };
  const token = adoptToken(win, storage);
  assert.equal(token, 'abc');
  assert.equal(loadToken(storage), 'abc');
  assert.equal(calls.length, 1);
  assert.equal(calls[0][2], '/?q=1#x=1');
  assert.ok(!calls[0][2].includes('abc'));
});

test('adoptToken without a fragment token falls back to storage and leaves the URL', () => {
  const storage = fakeStorage();
  saveToken(storage, 'kept');
  const calls = [];
  const win = { location: { hash: '', pathname: '/', search: '' }, history: { replaceState: (...a) => calls.push(a) } };
  assert.equal(adoptToken(win, storage), 'kept');
  assert.equal(calls.length, 0);
});

test('storage failures never throw', () => {
  const storage = fakeStorage({}, true);
  const win = { location: { hash: '#token=abc', pathname: '/', search: '' }, history: { replaceState() {} } };
  assert.equal(adoptToken(win, storage), 'abc');
  assert.equal(loadToken(storage), '');
  assert.doesNotThrow(() => saveToken(storage, 'x'));
  assert.equal(loadToken(null), '');
});
