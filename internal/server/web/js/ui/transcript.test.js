import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  formatDuration, renderMessage, renderThinkingCard, renderStageChip, renderRetrievalChip,
  renderFilesChip, renderPackChip, renderToolCard, renderTranscript,
} from './transcript.js';

const walk = (t, f) => { if (t && typeof t === 'object') { f(t); t.children.forEach((c) => walk(c, f)); } };
const find = (t, p) => { const r = []; walk(t, (n) => { if (p(n)) r.push(n); }); return r; };
const texts = (t) => { const r = []; const go = (n) => { if (typeof n === 'string') r.push(n); else if (n) n.children.forEach(go); }; go(t); return r.join(' '); };
const cls = (t, c) => find(t, (n) => String(n.attrs.class || '').split(' ').includes(c));
const bad = '"><script>';

const fixture = JSON.parse(readFileSync(new URL('../../dev/fixtures/run-permission.json', import.meta.url), 'utf8'));
const ev = (type) => fixture.filter((e) => e.type === type);

test('formatDuration', () => {
  assert.equal(formatDuration(undefined), '');
  assert.equal(formatDuration(-1), '');
  assert.equal(formatDuration(850), '850ms');
  assert.equal(formatDuration(1500), '1.5s');
  assert.equal(formatDuration(65000), '1m 5s');
});

test('message renders avatar, bubble and time for each role', () => {
  const u = renderMessage({ role: 'user', content: 'hello', time: '2026-01-02T03:04:05Z' });
  assert.match(u.attrs.class, /bv-message--user/);
  assert.equal(texts(cls(u, 'bv-message__avatar')[0]), 'U');
  assert.match(texts(u), /hello/);
  assert.equal(cls(u, 'bv-message__time').length, 1);
  const a = renderMessage({ role: 'assistant', content: 'Done.' });
  assert.match(a.attrs.class, /bv-message--assistant/);
  assert.equal(texts(cls(a, 'bv-message__avatar')[0]), 'AI');
  assert.equal(cls(a, 'bv-message__time').length, 0);
});

test('message with empty content renders an empty bubble, not undefined', () => {
  const m = renderMessage({ role: 'assistant' });
  assert.equal(cls(m, 'bv-message__bubble').length, 1);
  assert.doesNotMatch(texts(m), /undefined|null/);
});

test('thinking card renders from fixture, collapsible with duration', () => {
  const d = ev('assistant_thinking_delta')[0].data;
  const t = renderThinkingCard({ text: d.thinking, durationMs: 2300 });
  assert.match(texts(t), /Checking the project\./);
  assert.match(texts(t), /2\.3s/);
  assert.equal(find(t, (n) => n.attrs['data-action'] === 'toggle-thinking').length, 1);
  assert.equal(t.attrs['aria-expanded'], undefined);
});

test('thinking card empty state shows no duration', () => {
  const t = renderThinkingCard({});
  assert.equal(cls(t, 'bv-thinking__duration').length, 0);
  assert.doesNotMatch(texts(t), /undefined/);
});

test('chips render from event payloads', () => {
  assert.match(texts(renderStageChip({ stage: 'memory_recall', duration_ms: 12 })), /memory_recall.*12ms/);
  assert.match(texts(renderRetrievalChip({ records: 4, files: 2, context_bytes: 900, stale_dropped: 1 })), /4 records.*2 files/);
  assert.match(texts(renderRetrievalChip({ fallback: true, reason: 'no index' })), /fallback.*no index/i);
  assert.match(texts(renderRetrievalChip({ error: 'boom' })), /boom/);
  assert.match(texts(renderFilesChip({ files: ['a.go', 'b.go'] })), /2 files/);
  assert.match(texts(renderPackChip({ estimated_included: 10, estimated_skipped: 3 })), /10.*3/);
});

test('chip empty states', () => {
  assert.match(texts(renderStageChip({})), /stage/i);
  assert.match(texts(renderFilesChip({})), /0 files/);
  assert.match(texts(renderPackChip({})), /—/);
  assert.doesNotMatch(texts(renderRetrievalChip({})), /undefined|NaN/);
});

test('tool card renders from fixture start + result', () => {
  const start = ev('tool_use_start')[0].data;
  const res = ev('tool_use_result')[0].data;
  const c = renderToolCard({ id: start.id, name: start.name, input: start.input, status: 'completed', result: res.result, error: res.error, durationMs: 1200 });
  assert.equal(c.attrs['data-id'], 'tool-1');
  assert.match(texts(c), /Grep/);
  assert.match(texts(c), /pattern/);
  assert.match(texts(c), /TODO/);
  assert.match(texts(c), /src\/main\.go:1: TODO/);
  assert.match(texts(cls(c, 'bv-tool__status')[0]), /completed/i);
  assert.match(texts(c), /1\.2s/);
});

test('tool card status variants and error', () => {
  const run = renderToolCard({ id: 't', name: 'Bash', input: { command: 'ls' }, status: 'running' });
  assert.match(run.attrs.class, /bv-tool--running/);
  assert.equal(cls(run, 'bv-tool__result').length, 0);
  const fail = renderToolCard({ id: 't', name: 'Bash', status: 'failed', error: 'exit 1' });
  assert.match(fail.attrs.class, /bv-tool--failed/);
  assert.match(texts(fail), /exit 1/);
});

test('tool card tolerates missing input and non-string result', () => {
  const c = renderToolCard({ name: 'X', status: 'completed', result: { a: 1 } });
  assert.equal(cls(c, 'bv-tool__params').length, 0);
  assert.match(texts(c), /"a":\s*1/);
  assert.doesNotMatch(texts(renderToolCard({})), /undefined/);
});

test('transcript dispatches items and has an empty state', () => {
  const t = renderTranscript({ items: [
    { kind: 'message', role: 'user', content: 'hi' },
    { kind: 'thinking', text: 'hm' },
    { kind: 'tool', id: '1', name: 'Grep', status: 'running' },
    { kind: 'chip', chip: 'stage', data: { stage: 's', duration_ms: 1 } },
    { kind: 'unknown' },
  ] });
  assert.equal(cls(t, 'bv-message').length, 1);
  assert.equal(cls(t, 'bv-thinking').length, 1);
  assert.equal(cls(t, 'bv-tool').length, 1);
  assert.equal(cls(t, 'bv-chip').length, 1);
  assert.match(texts(renderTranscript({ items: [] })), /No messages yet/);
  assert.match(texts(renderTranscript({})), /No messages yet/);
});

test('hostile strings stay text children', () => {
  const trees = [
    renderMessage({ role: bad, content: bad, time: bad }),
    renderThinkingCard({ text: bad }),
    renderStageChip({ stage: bad }),
    renderRetrievalChip({ fallback: true, reason: bad }),
    renderRetrievalChip({ error: bad }),
    renderFilesChip({ files: [bad] }),
    renderToolCard({ id: bad, name: bad, input: { [bad]: bad }, status: bad, result: bad, error: bad }),
  ];
  for (const t of trees) {
    assert.equal(find(t, (n) => n.tag === 'script').length, 0);
  }
  assert.ok(texts(trees[0]).includes(bad));
  assert.ok(texts(trees[6]).includes(bad));
  assert.equal(trees[6].attrs['data-id'], bad);
});
