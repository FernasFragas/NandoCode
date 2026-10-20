// One document-level Escape listener. Features that react to Escape register on
// the 'escape' bus event in mount order (dialog, permission, composer) and the
// first one that sets evt.handled wins.
export function mount(ctx) {
  ctx.doc.addEventListener('keydown', (e) => {
    if (e.key !== 'Escape') return;
    const evt = { handled: false };
    ctx.emit('escape', evt);
    if (evt.handled && e.preventDefault) e.preventDefault();
  });
}
