// Permission card (DESIGN-SPEC: tool, target, reason, Allow / Deny / Always allow, X).
// data is the permission_request payload: {request_id, tool_name, target, reason}.
import { h, icon } from '../dom.js';

const str = (v) => (v === null || v === undefined ? '' : String(v));

// opts.inline renders the < 1200px fallback instead of the popover.
export function renderPermissionCard(data = {}, opts = {}) {
  const id = str(data.request_id);
  const btn = (action, label, kind) => h('button', {
    type: 'button', class: 'bv-permission__btn bv-permission__btn--' + kind, 'data-action': action, 'data-id': id,
  }, label);
  return h('div', { class: 'bv-permission bv-permission--' + (opts.inline ? 'inline' : 'popover'), role: 'alertdialog' },
    h('div', { class: 'bv-permission__head' },
      icon('shield'),
      h('span', { class: 'bv-permission__tool' }, str(data.tool_name) || 'Tool'),
      h('button', { type: 'button', class: 'bv-permission__close', 'data-action': 'dismiss', 'data-id': id, title: 'Dismiss', 'aria-label': 'Dismiss' }, icon('x'))),
    data.target ? h('code', { class: 'bv-permission__target' }, str(data.target)) : null,
    data.reason ? h('p', { class: 'bv-permission__reason' }, str(data.reason)) : null,
    h('div', { class: 'bv-permission__actions' },
      btn('allow', 'Allow', 'allow'),
      btn('deny', 'Deny', 'deny'),
      btn('always-allow', 'Always allow', 'always')));
}

// Save confirmation card (V7b): shown when PUT /files answers needs_confirm.
// data: {request_id, path}. Actions: save-once, save-always, save-cancel, dismiss.
export function renderSaveConfirmCard(data = {}, opts = {}) {
  const id = str(data.request_id);
  const btn = (action, label, kind) => h('button', {
    type: 'button', class: 'bv-permission__btn bv-permission__btn--' + kind, 'data-action': action, 'data-id': id,
  }, label);
  return h('div', { class: 'bv-permission bv-permission--' + (opts.inline ? 'inline' : 'popover') + ' bv-permission--save', role: 'alertdialog' },
    h('div', { class: 'bv-permission__head' },
      icon('shield'),
      h('span', { class: 'bv-permission__tool' }, 'Save file'),
      h('button', { type: 'button', class: 'bv-permission__close', 'data-action': 'dismiss', 'data-id': id, title: 'Dismiss', 'aria-label': 'Dismiss' }, icon('x'))),
    data.path ? h('code', { class: 'bv-permission__target' }, str(data.path)) : null,
    h('p', { class: 'bv-permission__reason' }, 'Saving this file needs your approval. Always allow also lets the agent write this file.'),
    h('div', { class: 'bv-permission__actions' },
      btn('save-once', 'Allow once', 'allow'),
      btn('save-always', 'Always allow', 'always'),
      btn('save-cancel', 'Cancel', 'deny')));
}
