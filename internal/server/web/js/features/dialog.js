// Dialog host: renders a dialog tree into #dialog and routes clicks, Enter and
// Escape back to the feature that opened it through bus events.
//   ctx.emit('dialog:open', {owner, tree})   ctx.emit('dialog:close', owner)
//   listens: 'dialog:action' {owner, action}  'dialog:closed' {owner}
export function mount(ctx) {
  const host = ctx.el('dialog');
  if (!host) return;
  let owner = null;

  function close(by) {
    if (owner === null || (by && by !== owner)) return;
    const was = owner;
    owner = null;
    host.replaceChildren();
    ctx.set({ dialogOpen: false });
    ctx.emit('dialog:closed', { owner: was });
  }

  ctx.on('dialog:open', ({ owner: who, tree }) => {
    if (owner !== null) close();
    owner = who;
    ctx.render('dialog', tree);
    ctx.set({ dialogOpen: true });
    const first = host.querySelector('input') || host.querySelector('button');
    if (first) first.focus();
  });
  ctx.on('dialog:close', close);

  host.addEventListener('click', (e) => {
    const target = e.target;
    const el = target && target.closest ? target.closest('[data-action]') : null;
    if (el && host.contains(el) && owner !== null) {
      ctx.emit('dialog:action', { owner, action: el.getAttribute('data-action') });
    }
  });
  host.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && owner !== null && e.target && e.target.tagName === 'INPUT') {
      e.preventDefault();
      ctx.emit('dialog:action', { owner, action: 'submit' });
    }
  });

  ctx.on('escape', (evt) => {
    if (evt.handled || owner === null) return;
    evt.handled = true;
    close();
  });
}
