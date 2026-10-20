// One delegated listener per region (contract K1): rows that wrap inner buttons
// resolve to the closest [data-action] element, and elements outside the region
// are ignored.
export function delegate(region, type, handler, selector = '[data-action]') {
  region.addEventListener(type, (evt) => {
    const target = evt.target;
    if (!target || typeof target.closest !== 'function') return;
    const el = target.closest(selector);
    if (!el || !region.contains(el)) return;
    handler(el.getAttribute('data-action'), el.getAttribute('data-id'), el, evt);
  });
}
