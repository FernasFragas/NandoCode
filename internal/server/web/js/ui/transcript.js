// Transcript components (DESIGN-SPEC: message row, thinking card, chips, tool card).
// Pure render functions returning K1 trees; no DOM, window or storage.
import { h, icon } from '../dom.js';

export function formatDuration(ms) {
  if (typeof ms !== 'number' || !Number.isFinite(ms) || ms < 0) return '';
  if (ms < 1000) return Math.round(ms) + 'ms';
  const s = ms / 1000;
  if (s < 60) return (Math.round(s * 10) / 10) + 's';
  const whole = Math.floor(s);
  return Math.floor(whole / 60) + 'm ' + (whole % 60) + 's';
}

const str = (v) => (v === null || v === undefined ? '' : String(v));

function timeLabel(iso) {
  const t = Date.parse(iso);
  if (!iso || Number.isNaN(t)) return '';
  return new Date(t).toISOString().slice(11, 16);
}

// data: {role: 'user'|'assistant', content, time}
export function renderMessage(data = {}) {
  const role = data.role === 'user' ? 'user' : 'assistant';
  const time = timeLabel(data.time);
  return h('div', { class: 'bv-message bv-message--' + role },
    h('span', { class: 'bv-message__avatar' }, role === 'user' ? 'U' : 'AI'),
    h('div', { class: 'bv-message__body' },
      h('div', { class: 'bv-message__bubble' }, str(data.content)),
      time ? h('span', { class: 'bv-message__time' }, time) : null));
}

// data: {text, durationMs, open}
export function renderThinkingCard(data = {}) {
  const dur = formatDuration(data.durationMs);
  return h('div', { class: 'bv-thinking' + (data.open ? ' bv-thinking--open' : '') },
    h('button', { type: 'button', class: 'bv-thinking__toggle', 'data-action': 'toggle-thinking', 'aria-expanded': data.open ? 'true' : 'false' },
      icon(data.open ? 'chevron-down' : 'chevron-right'),
      h('span', { class: 'bv-thinking__label' }, 'Thinking'),
      dur ? h('span', { class: 'bv-thinking__duration' }, dur) : null),
    h('div', { class: 'bv-thinking__text' }, str(data.text)));
}

function chip(kind, ...children) {
  return h('span', { class: 'bv-chip bv-chip--' + kind }, ...children);
}

// stage_timing payload: {stage, duration_ms}
export function renderStageChip(data = {}) {
  const dur = formatDuration(data.duration_ms);
  return chip('stage', h('span', { class: 'bv-chip__label' }, str(data.stage) || 'stage'),
    dur ? h('span', { class: 'bv-chip__value' }, dur) : null);
}

// semantic_retrieval payload: {records, files, context_bytes, stale_dropped} | {fallback, reason} | {error}
export function renderRetrievalChip(data = {}) {
  if (data.error) {
    return chip('retrieval', h('span', { class: 'bv-chip__label' }, 'Retrieval error'),
      h('span', { class: 'bv-chip__value' }, str(data.error)));
  }
  if (data.fallback) {
    return chip('retrieval', h('span', { class: 'bv-chip__label' }, 'Retrieval fallback'),
      data.reason ? h('span', { class: 'bv-chip__value' }, str(data.reason)) : null);
  }
  const parts = [];
  if (typeof data.records === 'number') parts.push(data.records + ' records');
  if (typeof data.files === 'number') parts.push(data.files + ' files');
  return chip('retrieval', h('span', { class: 'bv-chip__label' }, 'Retrieval'),
    parts.length ? h('span', { class: 'bv-chip__value' }, parts.join(', ')) : null);
}

// data: {files: [path]}
export function renderFilesChip(data = {}) {
  const files = Array.isArray(data.files) ? data.files : [];
  return chip('files', icon('file'), h('span', { class: 'bv-chip__label' }, files.length + ' files'),
    files.length ? h('span', { class: 'bv-chip__value' }, files.map(str).join(', ')) : null);
}

// prompt_pack_report payload: {estimated_included, estimated_skipped}
export function renderPackChip(data = {}) {
  const n = (v) => (typeof v === 'number' ? String(v) : '—');
  return chip('pack', h('span', { class: 'bv-chip__label' }, 'Pack'),
    h('span', { class: 'bv-chip__value' }, n(data.estimated_included) + ' included, ' + n(data.estimated_skipped) + ' skipped'));
}

const CHIPS = { stage: renderStageChip, retrieval: renderRetrievalChip, files: renderFilesChip, pack: renderPackChip };
const STATUSES = new Set(['running', 'completed', 'failed']);

// data: {id, name, input, status: running|completed|failed, durationMs, result, error}
export function renderToolCard(data = {}) {
  const status = STATUSES.has(data.status) ? data.status : 'running';
  const dur = formatDuration(data.durationMs);
  const input = data.input && typeof data.input === 'object' ? Object.entries(data.input) : [];
  let result = data.result;
  if (result !== null && result !== undefined && typeof result !== 'string') result = JSON.stringify(result);
  const longResult = typeof result === 'string' && result.length > 1200;
  return h('div', { class: 'bv-tool bv-tool--' + status, 'data-id': str(data.id) },
    h('div', { class: 'bv-tool__head' },
      icon('terminal'),
      h('span', { class: 'bv-tool__name' }, str(data.name)),
      h('span', { class: 'bv-tool__status' }, status[0].toUpperCase() + status.slice(1)),
      dur ? h('span', { class: 'bv-tool__duration' }, dur) : null),
    input.length
      ? h('dl', { class: 'bv-tool__params' },
        input.map(([k, v]) => [
          h('dt', { class: 'bv-tool__param-key' }, k),
          h('dd', { class: 'bv-tool__param-value' }, typeof v === 'string' ? v : JSON.stringify(v)),
        ]))
      : null,
    data.error ? h('pre', { class: 'bv-tool__error' }, str(data.error)) : null,
    result ? h('pre', { class: 'bv-tool__result' }, longResult && !data.showAll ? result.slice(0, 1200) + '…' : result) : null,
    longResult ? h('button', { type: 'button', class: 'bv-tool__show-all', 'data-action': 'toggle-result', 'aria-expanded': data.showAll ? 'true' : 'false' }, data.showAll ? 'Show less' : 'Show all') : null);
}

// data: {items: [{kind: message|thinking|tool|chip, ...}]}
export function renderTranscript(data = {}) {
  const items = Array.isArray(data.items) ? data.items : [];
  const rendered = items.map((it) => {
    if (!it) return null;
    switch (it.kind) {
      case 'message': return renderMessage(it);
      case 'thinking': return renderThinkingCard(it);
      case 'tool': return renderToolCard(it);
      case 'chip': return CHIPS[it.chip] ? CHIPS[it.chip](it.data || {}) : null;
      default: return null;
    }
  }).filter(Boolean);
  return h('section', { class: 'bv-transcript' },
    rendered.length ? rendered : h('p', { class: 'bv-transcript__empty' }, 'No messages yet'));
}
