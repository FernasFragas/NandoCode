import { test } from 'node:test';
import assert from 'node:assert/strict';
import { makeCtx, fire, flush, sse } from '../fakedom.js';
import { mount, payloadOf, formatPayload } from './transcript.js';

function setup() {
  const t = makeCtx();
  mount(t.ctx);
  return t;
}
const region = (t) => t.doc.getElementById('transcript');
const emit = async (t, ...events) => { events.forEach((e) => t.ctx.emit('sse', e)); await flush(); };

test('payloadOf unwraps the envelope and formatPayload prints values', () => {
  assert.deepEqual(payloadOf({ data: { a: 1 } }), { a: 1 });
  assert.deepEqual(payloadOf({ a: 1 }), { a: 1 });
  assert.deepEqual(payloadOf(null), {});
  assert.equal(formatPayload(''), '');
  assert.equal(formatPayload('x'), 'x');
  assert.equal(formatPayload({ a: 1 }), '{\n  "a": 1\n}');
});

test('starts with an empty state', () => {
  assert.match(region(setup()).textContent, /No messages yet/);
});

test('a user message renders as plain text', async () => {
  const t = setup();
  t.ctx.emit('transcript:user-message', { text: '<img onerror=x> hi' });
  await flush();
  const bubble = region(t).querySelector('.bv-message--user .bv-message__bubble');
  assert.equal(bubble.textContent, '<img onerror=x> hi');
  assert.equal(bubble.innerHTML, '', 'no HTML injected');
});

test('streamed assistant text accumulates in one bubble and goes through markdown', async () => {
  const t = setup();
  await emit(t, sse('run_started'), sse('assistant_text_delta', { content: 'Hello **wor' }), sse('assistant_text_delta', { content: 'ld**' }));
  assert.equal(region(t).querySelectorAll('.bv-message--assistant').length, 1);
  const bubble = region(t).querySelector('.bv-message--assistant .bv-message__bubble');
  assert.equal(bubble.innerHTML, '<p>Hello <strong>world</strong></p>');
  assert.ok(bubble.classList.contains('bv-message__bubble--md'));
});

test('hostile assistant text is escaped by markdown.js', async () => {
  const t = setup();
  await emit(t, sse('assistant_text_delta', { content: '<script>alert(1)</script> [x](javascript:alert(1))' }));
  const html = region(t).querySelector('.bv-message--assistant .bv-message__bubble').innerHTML;
  assert.ok(!html.includes('<script'));
  assert.ok(!html.includes('href="javascript'));
});

test('thinking renders before the assistant bubble and toggles open', async () => {
  const t = setup();
  await emit(t, sse('run_started'),
    sse('assistant_text_delta', { content: 'answer' }),
    sse('assistant_thinking_delta', { thinking: 'hmm ' }, 'a', '2026-01-01T00:00:01Z'),
    sse('assistant_thinking_delta', { thinking: 'ok' }, 'b', '2026-01-01T00:00:03Z'));
  const kids = region(t).querySelector('.bv-transcript').children;
  assert.match(kids[0].className, /bv-thinking/);
  assert.match(kids[1].className, /bv-message/);
  assert.equal(kids[0].querySelector('.bv-thinking__text').textContent, 'hmm ok');
  assert.equal(kids[0].querySelector('.bv-thinking__duration').textContent, '2s');
  assert.ok(!kids[0].classList.contains('bv-thinking--open'));
  fire(kids[0].querySelector('.bv-thinking__toggle'), 'click');
  await flush();
  assert.ok(region(t).querySelector('.bv-thinking').classList.contains('bv-thinking--open'));
});

test('thinking is open while streaming and collapses after the first text delta', async () => {
  const t = setup();
  await emit(t, sse('run_started'), sse('assistant_thinking_delta', { thinking: 'working' }));
  assert.equal(region(t).querySelector('.bv-thinking__toggle').getAttribute('aria-expanded'), 'true');
  await emit(t, sse('assistant_text_delta', { content: 'answer' }));
  assert.equal(region(t).querySelector('.bv-thinking__toggle').getAttribute('aria-expanded'), 'false');
});

test('a failed tool result has a Failed status pill', async () => {
  const t = setup();
  await emit(t, sse('tool_use_start', { id: 'failed', name: 'bash' }), sse('tool_use_result', { id: 'failed', error: 'exit 1' }));
  assert.equal(region(t).querySelector('.bv-tool__status').textContent, 'Failed');
});

test('sub-agent spawns appear as nested transcript cards with lifecycle status', async () => {
  const t = setup();
  await emit(t, sse('subagent_started', { task_id: 'a', name: 'Explorer', task: 'scan', parent: '' }),
    sse('subagent_started', { task_id: 'b', name: 'Builder', task: 'build', parent: 'a' }));
  const parent = region(t).querySelector('[data-id="a"]');
  assert.match(parent.textContent, /Explorer/);
  assert.match(parent.textContent, /Builder/);
  await emit(t, sse('subagent_finished', { task_id: 'b', status: 'completed' }));
  assert.match(region(t).querySelector('[data-id="a"]').textContent, /completed/);
});

test('long tool results are capped until Show all is pressed', async () => {
  const t = setup();
  const long = 'x'.repeat(1500);
  await emit(t, sse('tool_use_start', { id: 'long', name: 'bash' }), sse('tool_use_result', { id: 'long', result: long }));
  assert.ok(region(t).querySelector('.bv-tool__result').textContent.length < long.length);
  fire(region(t).querySelector('.bv-tool__show-all'), 'click');
  await flush();
  assert.equal(region(t).querySelector('.bv-tool__result').textContent, long);
});

test('tool card lifecycle: running, progress, completed with duration', async () => {
  const t = setup();
  await emit(t, sse('run_started'), sse('tool_use_start', { id: 'c1', name: 'search_files', input: { q: 'x' } }, 'e1', '2026-01-01T00:00:00Z'));
  assert.match(region(t).querySelector('.bv-tool').className, /--running/);
  assert.equal(region(t).querySelector('.bv-tool__name').textContent, 'search_files');
  await emit(t, sse('tool_use_progress', { id: 'c1', status: 'scanning' }));
  assert.equal(region(t).querySelector('.bv-tool__result').textContent, 'scanning');
  await emit(t, sse('tool_use_result', { id: 'c1', result: { files: 2 } }, 'e2', '2026-01-01T00:00:02Z'));
  assert.match(region(t).querySelector('.bv-tool').className, /--completed/);
  assert.match(region(t).querySelector('.bv-tool__result').textContent, /"files": 2/);
  assert.equal(region(t).querySelector('.bv-tool__duration').textContent, '2s');
});

test('tool_call_id / tool_name aliases and failures', async () => {
  const t = setup();
  await emit(t, sse('tool_use_start', { tool_call_id: 'c2', tool_name: 'run_command', input: {} }), sse('tool_use_result', { tool_call_id: 'c2', error: 'exit 1' }));
  assert.match(region(t).querySelector('.bv-tool').className, /--failed/);
  assert.equal(region(t).querySelector('.bv-tool__error').textContent, 'exit 1');
  await emit(t, sse('tool_use_result', { id: 'nope', result: 'x' }));
  assert.equal(region(t).querySelectorAll('.bv-tool').length, 1);
});

test('notices and chips: error, hook, retry, idle, stage, pack, retrieval', async () => {
  const t = setup();
  await emit(t,
    sse('error', { message: 'boom' }), sse('hook_notice', { message: 'blocked' }), sse('retry_notice', { cause: 'timeout' }),
    sse('llm_idle_warning', { timeout_str: '30s' }),
    sse('stage_timing', { stage: 'pack', duration_ms: 1200 }), sse('semantic_stage_timing', { stage: 'embed', duration_ms: 5 }),
    sse('prompt_pack_report', { estimated_included: 3, estimated_skipped: 1 }),
    sse('semantic_retrieval', { records: 4, files: 2 }));
  const text = region(t).textContent;
  for (const want of ['Error: boom', 'Hook: blocked', 'Retry notice: timeout', 'LLM idle warning: 30s', '1.2s', 'embed', '3 included, 1 skipped', '4 records']) {
    assert.ok(text.includes(want), 'missing ' + want);
  }
  assert.match(region(t).querySelector('.bv-notice--error').textContent, /boom/);
});

test('local notices (send failed, permission failed) appear in the transcript', async () => {
  const t = setup();
  t.ctx.emit('notice', { type: 'error', message: 'Send failed: x' });
  t.ctx.emit('notice', { type: 'info', message: 'Permission request auto-closed.' });
  await flush();
  assert.match(region(t).textContent, /Error: Send failed: x/);
  assert.match(region(t).textContent, /Permission request auto-closed\./);
});

test('unknown events and heartbeats change nothing; reset clears', async () => {
  const t = setup();
  await emit(t, sse('heartbeat'), sse('something_new', { a: 1 }), sse('usage_update', {}));
  assert.match(region(t).textContent, /No messages yet/);
  await emit(t, sse('error', { message: 'x' }));
  t.ctx.emit('session:reset');
  await flush();
  assert.match(region(t).textContent, /No messages yet/);
});

test('a new turn after terminal starts a new assistant bubble', async () => {
  const t = setup();
  await emit(t, sse('run_started'), sse('assistant_text_delta', { content: 'one' }), sse('terminal'),
    sse('run_started'), sse('assistant_text_delta', { content: 'two' }), sse('terminal'));
  assert.equal(region(t).querySelectorAll('.bv-message--assistant').length, 2);
});

test('stays pinned to the bottom when new content arrives', async () => {
  const t = setup();
  const r = region(t);
  r.scrollHeight = 1000; r.clientHeight = 400; r.scrollTop = 600;
  await emit(t, sse('assistant_text_delta', { content: 'x' }));
  assert.equal(r.scrollTop, 1000);
  r.scrollTop = 0; // user scrolled up
  await emit(t, sse('assistant_text_delta', { content: 'y' }));
  assert.equal(r.scrollTop, 0);
});

test('streaming deltas patch only the streaming bubble; earlier nodes keep identity', async () => {
  const t = setup();
  t.ctx.emit('transcript:user-message', { text: 'hi' });
  await emit(t, sse('run_started'), sse('assistant_text_delta', { content: 'a' }));
  const section = region(t).querySelector('.bv-transcript');
  const user = section.children[0];
  const bubble = section.children[1];
  await emit(t, sse('assistant_text_delta', { content: 'b' }), sse('assistant_text_delta', { content: 'c' }));
  assert.equal(section.children[0], user, 'user node untouched');
  assert.equal(section.children[1], bubble, 'assistant node patched in place');
  assert.equal(bubble.querySelector('.bv-message__bubble').innerHTML, '<p>abc</p>');
  await emit(t, sse('tool_use_start', { id: 't1', name: 'bash' }));
  assert.equal(section.children[0], user);
  assert.equal(section.children.length, 3);
});

test('session_cleared empties the transcript', async () => {
  const t = setup();
  await emit(t, sse('assistant_text_delta', { content: 'old' }));
  await emit(t, sse('session_cleared', {}));
  assert.ok(!/old/.test(region(t).textContent));
  assert.match(region(t).textContent, /No messages yet/);
  await emit(t, sse('assistant_text_delta', { content: 'new' }));
  assert.equal(region(t).querySelectorAll('.bv-message--assistant').length, 1);
});

test('compaction shows a notice, then before to after, and keeps history', async () => {
  const t = setup();
  await emit(t, sse('assistant_text_delta', { content: 'keep me' }), sse('compaction_started', { turn_count: 4 }));
  assert.match(region(t).textContent, /Compacting/);
  await emit(t, sse('compaction_completed', { before: 20, after: 6 }));
  const text = region(t).textContent;
  assert.match(text, /compacted: 20 → 6/);
  assert.ok(!/Compacting/.test(text));
  assert.match(text, /keep me/);
  assert.equal(region(t).querySelectorAll('.bv-notice').length, 1);
});

test('thinking deltas patch the text node and respect a manual collapse', async () => {
  const t = setup();
  await emit(t, sse('assistant_thinking_delta', { thinking: 'a' }));
  const card = region(t).querySelector('.bv-thinking');
  const textEl = card.querySelector('.bv-thinking__text');
  await emit(t, sse('assistant_thinking_delta', { thinking: 'b' }));
  assert.equal(region(t).querySelector('.bv-thinking'), card, 'card node kept');
  assert.equal(card.querySelector('.bv-thinking__text').textContent, 'ab');
  fire(card.querySelector('.bv-thinking__toggle'), 'click');
  await flush();
  assert.equal(region(t).querySelector('.bv-thinking__toggle').getAttribute('aria-expanded'), 'false');
  await emit(t, sse('assistant_thinking_delta', { thinking: 'c' }));
  assert.equal(region(t).querySelector('.bv-thinking__toggle').getAttribute('aria-expanded'), 'false');
  assert.equal(region(t).querySelector('.bv-thinking__text').textContent, 'abc');
  assert.ok(textEl);
});

test('scale: 200 deltas at 5000 items do no work proportional to the item count', async () => {
  const t = setup();
  const batch = [];
  for (let i = 0; i < 5000; i++) batch.push(sse('stage_timing', { stage: 's' + i, duration_ms: 1 }));
  await emit(t, ...batch, sse('tool_use_start', { id: 'tt', name: 'bash' }), sse('assistant_text_delta', { content: 'x' }));
  const section = region(t).querySelector('.bv-transcript');
  let childReads = 0;
  let structural = 0;
  let created = 0;
  const proto = Object.getOwnPropertyDescriptor(Object.getPrototypeOf(section), 'children');
  Object.defineProperty(section, 'children', { get() { childReads++; return proto.get.call(section); } });
  for (const m of ['appendChild', 'replaceChildren']) {
    const orig = section[m].bind(section);
    section[m] = (...a) => { structural++; return orig(...a); };
  }
  const origCreate = t.doc.createElement.bind(t.doc);
  t.doc.createElement = (...a) => { created++; return origCreate(...a); };
  for (let i = 0; i < 200; i++) await emit(t, sse('assistant_text_delta', { content: 'y' }));
  assert.equal(childReads, 0);
  assert.equal(structural, 0);
  assert.equal(created, 0);
  // tool result lookup also avoids scanning, and still lands on the right card
  await emit(t, sse('tool_use_result', { id: 'tt', result: 'done' }));
  assert.match(region(t).querySelector('.bv-tool__status').textContent, /Completed/);
});

test('real parents (main, coordinator) are top-level; a repeated started updates, not duplicates', async () => {
  const t = setup();
  await emit(t, sse('subagent_started', { task_id: 'a', name: 'Explorer', task: 'scan', parent: 'coordinator' }),
    sse('subagent_started', { task_id: 'm', name: 'Helper', task: 'x', parent: 'main' }),
    sse('subagent_started', { task_id: 'a', name: 'Explorer', task: 'scan v2', parent: 'coordinator' }));
  assert.equal(region(t).querySelectorAll('[data-id="a"]').length, 1);
  assert.match(region(t).querySelector('[data-id="a"]').textContent, /scan v2/);
  assert.equal(region(t).querySelectorAll('.bv-subagent-card').length, 2);
  await emit(t, sse('subagent_finished', { task_id: 'a', status: 'killed' }));
  assert.match(region(t).querySelector('[data-id="a"]').textContent, /killed/);
});
