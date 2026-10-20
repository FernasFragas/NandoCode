// Header: brand, session id + copy, model picker, connection pill, Stop-run
// slot (wired in V5b) and the settings gear. A model that needs the Ollama
// Cloud key opens the credential prompt and retries the switch afterwards.
import { renderHeader } from '../ui/header.js';
import { delegate } from '../delegate.js';
import { needsCredential } from './credential.js';

const modelName = (m) => (m && (m.name || m.model)) || 'Unknown';

export function mount(ctx) {
  const region = ctx.el('header');
  if (!region) return;

  function draw() {
    const s = ctx.state;
    ctx.render('header', renderHeader({
      sessionId: s.sessionId,
      model: s.model,
      models: s.models,
      connection: s.connection === 'unauthorized' ? 'disconnected' : s.connection,
      running: s.running,
    }));
    const select = region.querySelector('select');
    if (select) select.disabled = !!s.running;
  }
  draw();
  ctx.on('state', (patch) => {
    if (['sessionId', 'model', 'models', 'connection', 'running'].some((k) => k in patch)) draw();
  });

  async function loadModels() {
    try {
      const data = await ctx.api.get('/v1/models');
      const models = ((data && data.models) || []).map(modelName);
      ctx.set({ models, model: ctx.state.model || models[0] || '' });
    } catch (err) {
      ctx.toast('Failed to load models: ' + err.message);
    }
  }
  ctx.on('session:ready', loadModels);

  async function selectModel(name) {
    const id = ctx.state.sessionId;
    if (!id || ctx.state.running) { draw(); return; }
    try {
      const data = await ctx.api.post('/v1/sessions/' + encodeURIComponent(id) + '/model', { model: name });
      ctx.set({ model: (data && data.model) || name });
    } catch (err) {
      draw(); // put the select back on the active model
      if (needsCredential(err)) {
        ctx.emit('credential:required', { provider: 'ollama_cloud_api', retry: () => selectModel(name) });
        return;
      }
      ctx.toast('Model switch failed: ' + (err.text || err.message));
    }
  }

  delegate(region, 'change', (action, _id, el) => {
    if (action === 'set-model') selectModel(el.value);
  });
  delegate(region, 'click', async (action, id) => {
    if (action === 'new-session') {
      ctx.emit('sessions:new');
    } else if (action === 'stop-run') {
      ctx.emit('runcontrol:stop-run');
    } else if (action === 'open-settings') {
      ctx.emit('settings:open');
    } else if (action === 'copy-session') {
      try {
        await ctx.win.navigator.clipboard.writeText(id);
        ctx.toast('Session id copied.', 'info');
      } catch {
        ctx.toast('Could not copy the session id.');
      }
    }
  });
}
