// Modal dialog frame shared by Settings and the cloud-credential prompt.
// Pure render function: data -> K1 tree. Buttons carry data-action; the dialog
// feature delegates one click listener on the host region.
import { h } from './dom.js';

// data: {title, description, fields: [tree], error, actions: [{action, label, kind}]}
export function renderDialog(data = {}) {
  const actions = Array.isArray(data.actions) ? data.actions : [];
  return h('div', { class: 'bv-modal-backdrop' },
    h('div', { class: 'bv-modal', role: 'dialog', 'aria-modal': 'true', 'aria-labelledby': 'dialog-title' },
      h('h2', { id: 'dialog-title', class: 'bv-modal__title' }, String(data.title ?? '')),
      data.description ? h('p', { class: 'bv-modal__text' }, String(data.description)) : null,
      data.fields || null,
      data.error ? h('p', { class: 'bv-modal__error', role: 'alert' }, String(data.error)) : null,
      h('div', { class: 'bv-modal__actions' },
        actions.map((a) => h('button', {
          type: 'button',
          class: 'bv-btn bv-btn--' + (a.kind || 'ghost'),
          'data-action': a.action,
        }, String(a.label))))));
}

// A labelled input row. kind: 'password' | 'text'.
export function renderField({ id, label, type = 'text', placeholder = '', hint = '' }) {
  return h('div', { class: 'bv-field' },
    h('label', { class: 'bv-field__label', for: id }, label),
    h('input', { id, class: 'bv-field__input', type, placeholder, name: id }),
    hint ? h('span', { class: 'bv-field__hint' }, hint) : null);
}
