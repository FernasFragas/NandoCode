// Ollama Cloud credential prompt (model picker requires_credential flow, K3
// POST /v1/credentials/ollama-cloud). The key is read from the field once, the
// field is cleared immediately, and the key is never rendered, logged or kept.
import { renderDialog, renderField } from '../dialogview.js';
import { h } from '../dom.js';

const OWNER = 'credential';

// True for the structured 428 body, or any error that names the credential.
export function needsCredential(err) {
  if (!err) return false;
  if (err.status === 428) return true;
  if (err.body && err.body.error === 'requires_credential') return true;
  return /requires_credential/.test(String(err.text || ''));
}

export function mount(ctx) {
  let retry = null;
  let busy = false;
  let reopening = false;

  function show(error) {
    reopening = true; // replacing the open dialog fires dialog:closed; keep the retry
    ctx.emit('dialog:open', {
      owner: OWNER,
      tree: renderDialog({
        title: 'Ollama Cloud API key',
        description: 'This model runs on Ollama Cloud. The key goes to this local server only; it is never shown again or sent with project context before it is set.',
        fields: [
          renderField({ id: 'credential-key', label: 'API key', type: 'password', placeholder: 'Ollama API key' }),
          h('div', { class: 'bv-field' },
            h('label', { class: 'bv-field__label', for: 'credential-persist' }, 'Remember'),
            h('select', { id: 'credential-persist', class: 'bv-field__input', name: 'credential-persist' },
              h('option', { value: 'session', selected: true }, 'For this server session only'),
              h('option', { value: 'keychain' }, 'Save to the system keychain'))),
        ],
        error,
        actions: [{ action: 'save', label: 'Save key', kind: 'primary' }, { action: 'cancel', label: 'Cancel' }],
      }),
    });
    reopening = false;
  }

  ctx.on('credential:required', (req) => {
    retry = req && typeof req.retry === 'function' ? req.retry : null;
    show();
  });

  ctx.on('dialog:closed', ({ owner }) => { if (owner === OWNER && !busy && !reopening) retry = null; });

  ctx.on('dialog:action', async ({ owner, action }) => {
    if (owner !== OWNER) return;
    if (action === 'cancel') { ctx.emit('dialog:close', OWNER); return; }
    if (action !== 'save' && action !== 'submit') return;
    if (busy) return;
    const input = ctx.el('credential-key');
    const persist = ctx.el('credential-persist');
    const key = input ? input.value.trim() : '';
    if (!key) { show('Enter the API key first.'); return; }
    input.value = '';
    busy = true;
    try {
      await ctx.api.post('/v1/credentials/ollama-cloud', { key, save: !!persist && persist.value === 'keychain' });
    } catch (err) {
      busy = false;
      // Never include request details: the message is the status line only.
      show('Could not save the key (HTTP ' + (err && err.status ? err.status : 'error') + ').');
      return;
    }
    busy = false;
    const again = retry;
    retry = null;
    ctx.emit('dialog:close', OWNER);
    ctx.toast('API key saved.', 'info');
    if (again) again();
  });
}
