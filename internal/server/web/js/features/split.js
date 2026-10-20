// Center split: the transcript above, a resizable code/tab pane below.
// This module owns LAYOUT only (pane, handle, persisted size, collapse at 0 tabs).
// It owns no tab state. Seam for V6c (features/tabs.js):
//   - V6c renders its tab strip into #tab-strip and its content into #tab-content.
//   - V6c emits `tabs:count` {count:number} whenever the open tab count changes;
//     the pane is hidden while count is 0 and shown otherwise.

const STORE_KEY = 'nandocode.split.height';
const MIN = 100;
const RESERVE = 180;

export function clampSplitHeight(height, available) {
  const max = Math.max(0, available - RESERVE);
  return Math.max(MIN, Math.min(max, height));
}

function loadHeight(win) {
  try {
    const n = Number(win && win.localStorage && win.localStorage.getItem(STORE_KEY));
    return Number.isFinite(n) && n > 0 ? n : null;
  } catch { return null; }
}

function saveHeight(win, height) {
  try { win.localStorage.setItem(STORE_KEY, String(height)); } catch { /* storage unavailable */ }
}

export function mount(ctx) {
  const composer = ctx.el('composer');
  const center = composer && composer.parentNode;
  if (!center) return;
  const handle = ctx.doc.createElement('div');
  handle.id = 'split-handle';
  handle.className = 'bv-split__handle';
  handle.setAttribute('role', 'separator');
  handle.setAttribute('aria-label', 'Resize tab area');
  handle.setAttribute('aria-orientation', 'horizontal');
  handle.setAttribute('tabindex', '0');
  const pane = ctx.doc.createElement('section');
  pane.id = 'tab-pane';
  pane.className = 'bv-split__tabs';
  const strip = ctx.doc.createElement('div');
  strip.id = 'tab-strip';
  const content = ctx.doc.createElement('div');
  content.id = 'tab-content';
  content.className = 'bv-split__content';
  pane.appendChild(strip);
  pane.appendChild(content);
  center.appendChild(handle);
  center.appendChild(pane);

  let count = 0;
  let stored = loadHeight(ctx.win) || 300;
  let height = stored;
  let dragging = null;
  const available = () => center.clientHeight || 800;

  function draw() {
    const visible = count > 0;
    pane.hidden = !visible;
    handle.hidden = !visible;
    height = clampSplitHeight(stored, available());
    if (center.style) center.style.setProperty('--split-height', height + 'px');
    handle.setAttribute('aria-valuemin', String(MIN));
    handle.setAttribute('aria-valuemax', String(Math.max(MIN, available() - RESERVE)));
    handle.setAttribute('aria-valuenow', String(height));
  }
  function resize(next) {
    stored = clampSplitHeight(next, available());
    draw();
  }

  ctx.on('tabs:count', (payload) => {
    const n = payload && Number(payload.count);
    count = Number.isFinite(n) && n > 0 ? n : 0;
    draw();
  });
  handle.addEventListener('pointerdown', (event) => {
    dragging = { y: event.clientY, height };
    if (handle.setPointerCapture && event.pointerId !== undefined) handle.setPointerCapture(event.pointerId);
    event.preventDefault();
  });
  handle.addEventListener('pointermove', (event) => {
    if (dragging) resize(dragging.height + dragging.y - event.clientY);
  });
  handle.addEventListener('pointerup', () => { if (dragging) saveHeight(ctx.win, stored); dragging = null; });
  handle.addEventListener('pointercancel', () => { dragging = null; });
  handle.addEventListener('keydown', (event) => {
    if (event.key === 'ArrowUp' || event.key === 'ArrowDown') {
      resize(height + (event.key === 'ArrowUp' ? 20 : -20));
      saveHeight(ctx.win, stored);
      event.preventDefault();
    }
  });
  if (typeof ctx.win.addEventListener === 'function') ctx.win.addEventListener('resize', draw);
  draw();
}
