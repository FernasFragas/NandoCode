// Settings gear: the bearer-token paste fallback (the normal path is the
// #token= launch URL). Also opens itself when a request comes back 401.
import { renderDialog, renderField } from '../dialogview.js';

const OWNER = 'settings';

export function mount(ctx) {
  let open = false;

  function show(error) {
    ctx.emit('dialog:open', {
      owner: OWNER,
      tree: renderDialog({
        title: 'Settings',
        description: 'Paste the bearer token printed by `nandocodego server` if the page was opened without #token=.',
        fields: renderField({ id: 'settings-token', label: 'Token', type: 'password', placeholder: 'Token', hint: ctx.getToken() ? 'A token is set for this tab.' : '' }),
        error,
        actions: [{ action: 'save', label: 'Save', kind: 'primary' }, { action: 'close', label: 'Close' }],
      }),
    });
    open = true; // after the emit: replacing an open dialog fires dialog:closed first
  }

  ctx.on('settings:open', () => show());
  ctx.on('auth:required', () => {
    if (open) return;
    ctx.toast('Unauthorized: paste the token in Settings.');
    show();
  });
  ctx.on('dialog:closed', ({ owner }) => { if (owner === OWNER) open = false; });
  ctx.on('dialog:action', ({ owner, action }) => {
    if (owner !== OWNER) return;
    if (action === 'close') {
      ctx.emit('dialog:close', OWNER);
      return;
    }
    if (action === 'save' || action === 'submit') {
      const input = ctx.el('settings-token');
      const value = input ? input.value.trim() : '';
      if (!value) { show('Enter a token first.'); return; }
      input.value = '';
      ctx.emit('dialog:close', OWNER);
      ctx.setToken(value);
      ctx.toast('Token saved for this tab.', 'info');
    }
  });
}
