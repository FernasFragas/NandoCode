// Shared context handed to every feature's mount(ctx): a tiny event bus, the
// shared UI state, the API client, and DOM helpers. Features talk to each other
// only through bus events and ctx.state, never by importing one another.
import { h, mount } from './dom.js';
import { createApi } from './api.js';
import { saveToken } from './token.js';

export function createContext({ doc, win, fetchImpl, storage, token = '', setTimer, now } = {}) {
  const listeners = new Map();
  let bearer = token;
  const timer = setTimer || ((fn, ms) => globalThis.setTimeout(fn, ms));

  const state = {
    sessionId: null,
    running: false,
    connection: 'connecting', // connecting | connected | disconnected | unauthorized
    model: '',
    models: [],
    nav: 'project',
    treeOpen: false,
    railOpen: false,
    layout: { nav: 'full', tree: 'full', rail: 'full', permission: 'popover' },
  };

  const ctx = {
    doc,
    win,
    state,
    now: now || (() => Date.now()),
    fetch: (...a) => (fetchImpl ? fetchImpl(...a) : globalThis.fetch(...a)),
    setTimer: timer,
    // Coalesces renders: one per animation frame in the browser, one per microtask in tests.
    frame: (fn) => (win && typeof win.requestAnimationFrame === 'function' ? win.requestAnimationFrame(fn) : Promise.resolve().then(fn)),
    api: createApi({ fetchImpl, getToken: () => bearer }),
    getToken: () => bearer,
    setToken(value) {
      bearer = String(value || '').trim();
      saveToken(storage, bearer);
      ctx.emit('token', bearer);
    },
    on(name, fn) {
      if (!listeners.has(name)) listeners.set(name, new Set());
      listeners.get(name).add(fn);
      return () => listeners.get(name).delete(fn);
    },
    emit(name, payload) {
      for (const fn of [...(listeners.get(name) || [])]) fn(payload);
    },
    // set(patch) updates ctx.state and notifies 'state' listeners with the patch.
    set(patch) {
      Object.assign(state, patch);
      ctx.emit('state', patch);
    },
    el(id) { return doc.getElementById(id); },
    // render replaces a region's content with a mounted K1 tree.
    render(id, tree) {
      const region = doc.getElementById(id);
      if (region) region.replaceChildren(mount(tree, doc));
      return region;
    },
    toast(message, kind = 'error') {
      const host = doc.getElementById('toasts');
      if (!host) return;
      const node = mount(h('div', { class: 'bv-toast bv-toast--' + kind, role: 'status' }, String(message)), doc);
      host.appendChild(node);
      timer(() => node.remove(), 4000);
    },
  };
  return ctx;
}
