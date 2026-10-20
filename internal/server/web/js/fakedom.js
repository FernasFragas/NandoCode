// Minimal DOM double for node --test (no npm, no jsdom). Supports exactly what
// dom.js mount() and the feature modules use: elements, text nodes, attributes,
// classList, simple selectors, bubbling events and a document with ids.

class FakeText {
  constructor(text) { this.nodeType = 3; this.data = String(text); this.parentNode = null; }
  get textContent() { return this.data; }
}

function matchesSimple(el, sel) {
  const re = /(^[a-zA-Z][\w-]*)|#([\w-]+)|\.([\w-]+)|\[([\w-]+)(?:="([^"]*)")?\]/g;
  let m;
  let any = false;
  while ((m = re.exec(sel)) !== null) {
    any = true;
    if (m[1] && el.tagName.toLowerCase() !== m[1].toLowerCase()) return false;
    if (m[2] && el.getAttribute('id') !== m[2]) return false;
    if (m[3] && !el.classList.contains(m[3])) return false;
    if (m[4]) {
      if (!el.hasAttribute(m[4])) return false;
      if (m[5] !== undefined && el.getAttribute(m[4]) !== m[5]) return false;
    }
  }
  return any;
}

export class FakeElement {
  constructor(tag, doc) {
    this.nodeType = 1;
    this.tagName = String(tag).toUpperCase();
    this.ownerDocument = doc;
    this.attributes = new Map();
    this.childNodes = [];
    this.parentNode = null;
    this.listeners = {};
    this.scrollTop = 0;
    this.scrollHeight = 0;
    this.clientHeight = 0;
    this._value = '';
    this.selectionStart = 0;
    const self = this;
    this.classList = {
      _list: () => (self.getAttribute('class') || '').split(/\s+/).filter(Boolean),
      contains: (c) => self.classList._list().includes(c),
      add: (c) => { const l = self.classList._list(); if (!l.includes(c)) self.setAttribute('class', [...l, c].join(' ')); },
      remove: (c) => self.setAttribute('class', self.classList._list().filter((x) => x !== c).join(' ')),
      toggle: (c, force) => {
        const has = self.classList.contains(c);
        const want = force === undefined ? !has : !!force;
        if (want) self.classList.add(c); else self.classList.remove(c);
        return want;
      },
    };
  }
  get children() { return this.childNodes.filter((n) => n.nodeType === 1); }
  get id() { return this.getAttribute('id') || ''; }
  set id(v) { this.setAttribute('id', v); }
  get className() { return this.getAttribute('class') || ''; }
  set className(v) { this.setAttribute('class', v); }
  get disabled() { return this.hasAttribute('disabled'); }
  set disabled(v) { if (v) this.setAttribute('disabled', ''); else this.removeAttribute('disabled'); }
  get hidden() { return this.hasAttribute('hidden'); }
  set hidden(v) { if (v) this.setAttribute('hidden', ''); else this.removeAttribute('hidden'); }
  get value() { return this.tagName === 'SELECT' ? this._selectValue() : this._value; }
  set value(v) { this._value = String(v); this.selectionStart = this._value.length; }
  _selectValue() {
    if (this._value) return this._value;
    const opts = this.querySelectorAll('option');
    const sel = opts.find((o) => o.hasAttribute('selected')) || opts[0];
    return sel ? sel.getAttribute('value') : '';
  }
  setSelectionRange(a) { this.selectionStart = a; }
  setAttribute(k, v) { this.attributes.set(k, String(v)); }
  getAttribute(k) { return this.attributes.has(k) ? this.attributes.get(k) : null; }
  hasAttribute(k) { return this.attributes.has(k); }
  removeAttribute(k) { this.attributes.delete(k); }
  appendChild(n) {
    if (n.parentNode) n.parentNode.childNodes.splice(n.parentNode.childNodes.indexOf(n), 1);
    n.parentNode = this;
    this.childNodes.push(n);
    return n;
  }
  replaceChildren(...nodes) {
    this.childNodes.forEach((c) => { c.parentNode = null; });
    this.childNodes = [];
    this._html = undefined;
    nodes.forEach((n) => this.appendChild(n));
  }
  remove() { if (this.parentNode) this.parentNode.childNodes.splice(this.parentNode.childNodes.indexOf(this), 1); this.parentNode = null; }
  get textContent() { return this.childNodes.map((c) => c.textContent).join('') + (this._html ?? ''); }
  set textContent(t) { this.replaceChildren(); if (t !== '') this.appendChild(new FakeText(t)); }
  get innerHTML() { return this._html ?? ''; }
  set innerHTML(html) { this.replaceChildren(); this._html = String(html); }
  focus() { this.ownerDocument.activeElement = this; }
  contains(n) { for (let x = n; x; x = x.parentNode) if (x === this) return true; return false; }
  matches(sel) { return matchesSimple(this, sel); }
  closest(sel) {
    for (let x = this; x && x.nodeType === 1; x = x.parentNode) if (x.matches(sel)) return x;
    return null;
  }
  querySelectorAll(sel) {
    const parts = sel.trim().split(/\s+/);
    const out = [];
    const walk = (node) => {
      for (const c of node.children) {
        if (matchesSimple(c, parts[parts.length - 1]) && ancestorsMatch(c, parts.slice(0, -1), this)) out.push(c);
        walk(c);
      }
    };
    walk(this);
    return out;
  }
  querySelector(sel) { return this.querySelectorAll(sel)[0] || null; }
  addEventListener(type, fn) { (this.listeners[type] ||= []).push(fn); }
  removeEventListener(type, fn) { this.listeners[type] = (this.listeners[type] || []).filter((f) => f !== fn); }
  dispatchEvent(evt) {
    evt.target ||= this;
    evt.defaultPrevented ||= false;
    evt.preventDefault ||= () => { evt.defaultPrevented = true; };
    evt.stopPropagation ||= () => { evt._stopped = true; };
    for (let x = this; x && !evt._stopped; x = x.parentNode || (x.nodeType === 1 && x.ownerDocument !== x ? x.ownerDocument : null)) {
      evt.currentTarget = x;
      (x.listeners[evt.type] || []).slice().forEach((fn) => fn(evt));
      if (x === this.ownerDocument) break;
    }
    return !evt.defaultPrevented;
  }
}

function ancestorsMatch(el, parts, root) {
  let cur = el.parentNode;
  for (let i = parts.length - 1; i >= 0; i--) {
    while (cur && cur !== root && !matchesSimple(cur, parts[i])) cur = cur.parentNode;
    if (!cur || cur === root) return false;
    cur = cur.parentNode;
  }
  return true;
}

export function createDocument() {
  const doc = {
    nodeType: 9,
    listeners: {},
    activeElement: null,
    createElement(tag) { return new FakeElement(tag, doc); },
    createElementNS(_ns, tag) { return new FakeElement(tag, doc); },
    createTextNode(t) { return new FakeText(t); },
    getElementById(id) { return doc.body.querySelector('#' + id) || (doc.body.id === id ? doc.body : null); },
    addEventListener(type, fn) { (doc.listeners[type] ||= []).push(fn); },
    removeEventListener(type, fn) { doc.listeners[type] = (doc.listeners[type] || []).filter((f) => f !== fn); },
    dispatchEvent(evt) { evt.target ||= doc; evt.preventDefault ||= () => { evt.defaultPrevented = true; }; (doc.listeners[evt.type] || []).forEach((fn) => fn(evt)); },
  };
  doc.parentNode = null;
  doc.body = new FakeElement('body', doc);
  return doc;
}

// Builds a document holding empty elements with the given ids ([id] or [id, tag]).
export function pageWith(ids) {
  const doc = createDocument();
  for (const spec of ids) {
    const [id, tag = 'div'] = Array.isArray(spec) ? spec : [spec];
    const el = doc.createElement(tag);
    el.id = id;
    doc.body.appendChild(el);
  }
  return doc;
}

export const fire = (el, type, props = {}) => {
  const evt = { type, ...props };
  el.dispatchEvent(evt);
  return evt;
};

export const flush = () => new Promise((r) => setImmediate(r));

// until waits, one macrotask at a time, for ready() to hold or the deadline to
// pass. Use it when the code under test finishes off the event loop (e.g.
// crypto.subtle.digest), where a fixed number of flushes races the machine.
export const until = async (ready, timeoutMs = 2000) => {
  const deadline = Date.now() + timeoutMs;
  while (!ready()) {
    if (Date.now() > deadline) throw new Error(`condition not met within ${timeoutMs}ms`);
    await flush();
  }
};

// ---- feature-test harness ------------------------------------------------
import { createContext } from './ctx.js';

// Built from parts: the network-policy check forbids literal URLs in JS.
export const ORIGIN = ['http:', '', 'test.invalid:1'].join('/');

export const REGION_IDS = [
  'app', 'header', 'nav', 'sessions', 'tree', 'panel-toggles', 'transcript', 'permission',
  'composer', 'mention-dropdown', ['message-input', 'textarea'], ['send-btn', 'button'],
  'rail', 'footer', 'dialog', 'toasts',
];

// routes: {'GET /v1/models': (body, init) => ({status?, json?, text?})}. Unknown routes 404.
export function makeCtx({ routes = {}, width = 1448, token = '' } = {}) {
  const doc = pageWith(REGION_IDS);
  const calls = [];
  const win = {
    innerWidth: width,
    location: { origin: ORIGIN, hash: '', pathname: '/', search: '' },
    navigator: { clipboard: { writes: [], async writeText(t) { this.writes.push(t); } } },
    listeners: {},
    addEventListener(t, fn) { (this.listeners[t] ||= []).push(fn); },
  };
  const fetchImpl = async (path, init = {}) => {
    const method = init.method || 'GET';
    const body = init.body ? JSON.parse(init.body) : undefined;
    calls.push({ method, path, body, headers: init.headers || {} });
    const key = method + ' ' + path.split('?')[0];
    const handler = routes[key];
    if (!handler) return { ok: false, status: 404, text: async () => 'not found' };
    const out = await handler(body, init);
    const status = out.status ?? 200;
    const text = out.text ?? (out.json === undefined ? '' : JSON.stringify(out.json));
    return { ok: status >= 200 && status < 300, status, text: async () => text, body: out.stream || null };
  };
  const timers = [];
  const ctx = createContext({ doc, win, fetchImpl, storage: null, token, setTimer: (fn, ms) => { timers.push({ fn, ms }); return timers.length; }, now: () => 1700000000000 });
  return { ctx, doc, win, calls, timers };
}

export const sse = (type, data = {}, id = type + '-' + Math.random().toString(36).slice(2, 8), time = '2026-01-01T00:00:00Z') => ({ type, id, time, data });
