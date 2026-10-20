// Transcript: turns SSE events into items and renders them with the V1e render
// modules. Streaming text is accumulated per turn (one assistant bubble, one
// thinking card, tool cards in arrival order), exactly like the old page.
// Assistant text goes through markdown.js, whose output is HTML-escaped first.
import { h, mount as mountTree } from '../dom.js';
import { renderMarkdown } from '../markdown.js';
import {
  formatDuration, renderMessage, renderThinkingCard, renderToolCard,
  renderStageChip, renderPackChip, renderRetrievalChip,
} from '../ui/transcript.js';
import { delegate } from '../delegate.js';

export function payloadOf(msg) {
  if (!msg) return {};
  if (msg.data && typeof msg.data === 'object') return msg.data;
  return msg;
}

export function formatPayload(value) {
  if (value === null || value === undefined || value === '') return '';
  if (typeof value === 'string') return value;
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

function noticeText(type, p) {
  switch (type) {
    case 'error': return 'Error: ' + (p.message || p.error || p.code || formatPayload(p));
    case 'hook': return 'Hook: ' + (p.message || p.hook || formatPayload(p));
    case 'retry': return 'Retry notice' + (p.cause ? ': ' + p.cause : '');
    case 'idle': return 'LLM idle warning' + (p.timeout_str ? ': ' + p.timeout_str : '');
    case 'info': return p.message || '';
    default: return formatPayload(p);
  }
}

const at = (msg) => {
  const t = Date.parse(msg && msg.time);
  return Number.isNaN(t) ? null : t;
};

export function renderNotice(item) {
  return h('p', { class: 'bv-notice bv-notice--' + (item.level || 'info') }, String(item.text));
}

export function mount(ctx) {
  const region = ctx.el('transcript');
  if (!region) return;

  let items = [];
  let turn = null; // {assistant, thinking} item references for the open turn
  let scheduled = false;
  let stick = true;
  let reorder = false; // an item was inserted mid-list or a node could not be swapped in place

  function startTurn() { turn = { assistant: null, thinking: null }; }
  function endTurn() { turn = null; }

  function push(item) { items.push(item); dirty.add(item); return item; }

  function appendText(delta, msg) {
    if (!turn) startTurn();
    if (delta && turn.thinking && !turn.thinking.userToggled && turn.thinking.open) { turn.thinking.open = false; dirty.add(turn.thinking); }
    if (!turn.assistant) {
      turn.assistant = { kind: 'message', role: 'assistant', content: '', time: msg && msg.time };
      push(turn.assistant);
    }
    turn.assistant.content += delta;
    dirty.add(turn.assistant);
  }

  function appendThinking(delta, msg) {
    if (!turn) startTurn();
    if (!turn.thinking) {
      turn.thinking = { kind: 'thinking', text: '', open: !turn.assistant, firstAt: at(msg), lastAt: at(msg), durationMs: undefined };
      if (turn.assistant) { items.splice(items.indexOf(turn.assistant), 0, turn.thinking); reorder = true; }
      else push(turn.thinking);
    }
    turn.thinking.text += delta;
    dirty.add(turn.thinking);
    const now = at(msg);
    if (now !== null) {
      turn.thinking.lastAt = now;
      if (turn.thinking.firstAt !== null && now > turn.thinking.firstAt) turn.thinking.durationMs = now - turn.thinking.firstAt;
    }
  }

  const toolsById = new Map();
  const subagents = new Map();
  const findTool = (id) => toolsById.get(id);
  const toolId = (p) => String(p.id || p.tool_call_id || '');

  function toolStart(p, msg) {
    if (!turn) startTurn();
    const tool = push({ kind: 'tool', id: toolId(p), name: p.name || p.tool_name || '', input: p.input, status: 'running', startedAt: at(msg) });
    toolsById.set(tool.id, tool);
  }
  function toolProgress(p) {
    const tool = findTool(toolId(p));
    if (!tool) return;
    dirty.add(tool);
    tool.result = p.status || formatPayload(p.data) || 'Running…';
  }
  function toolResult(p, msg) {
    const tool = findTool(toolId(p));
    if (!tool) return;
    const error = p.error ? String(p.error) : '';
    tool.status = error ? 'failed' : 'completed';
    tool.error = error || undefined;
    tool.result = error ? undefined : formatPayload(p.result);
    tool.showAll = false;
    dirty.add(tool);
    const end = at(msg);
    if (end !== null && tool.startedAt !== null && end >= tool.startedAt) tool.durationMs = end - tool.startedAt;
  }

  function notice(type, p) {
    return push({ kind: 'notice', level: type === 'error' ? 'error' : 'info', text: noticeText(type, p) });
  }

  let compaction = null;
  function compactionDone(p) {
    let text = 'compacted: ' + p.before + ' → ' + p.after;
    if (p.skipped) text = 'Compaction skipped';
    else if (p.denied) text = 'Compaction denied';
    if (compaction && items.includes(compaction)) { compaction.text = text; dirty.add(compaction); }
    else notice('info', { message: text });
    compaction = null;
  }

  function resetAll() {
    items = []; nodes.clear(); nodes2.clear(); dirty.clear(); toolsById.clear(); subagents.clear();
    turn = null; compaction = null; stick = true; reorder = true;
    schedule();
  }

  function handle(msg) {
    const p = payloadOf(msg);
    switch (msg.type) {
      case 'subagent_started': {
        if (!p.task_id) break;
        const existing = subagents.get(p.task_id);
        if (existing) {
          // Repeated start: update in place. Parents are "main", "coordinator" or a task_id.
          existing.name = p.name || existing.name; existing.task = p.task || existing.task;
          dirty.add(rootAgent(existing));
          break;
        }
        const parent = subagents.get(p.parent);
        const item = { kind: 'subagent', id: p.task_id, name: p.name || p.task_id, task: p.task || '', status: 'running', children: [], parent: parent || null };
        subagents.set(p.task_id, item);
        if (parent) { parent.children.push(item); dirty.add(rootAgent(parent)); }
        else push(item);
        break;
      }
      case 'subagent_finished': {
        const item = subagents.get(p.task_id);
        if (item) { item.status = p.status || 'finished'; dirty.add(rootAgent(item)); }
        break;
      }
      case 'run_started': startTurn(); break;
      case 'terminal': endTurn(); break;
      case 'assistant_text_delta': appendText(p.content || p.delta || p.text || '', msg); break;
      case 'assistant_thinking_delta': appendThinking(p.thinking || p.delta || p.text || '', msg); break;
      case 'tool_use_start': toolStart(p, msg); break;
      case 'tool_use_progress': toolProgress(p); break;
      case 'tool_use_result': toolResult(p, msg); break;
      case 'hook_notice': notice('hook', p); break;
      case 'retry_notice': notice('retry', p); break;
      case 'llm_idle_warning': notice('idle', p); break;
      case 'stage_timing':
      case 'semantic_stage_timing': push({ kind: 'chip', chip: 'stage', data: p }); break;
      case 'prompt_pack_report': push({ kind: 'chip', chip: 'pack', data: p }); break;
      case 'semantic_retrieval': push({ kind: 'chip', chip: 'retrieval', data: p }); break;
      case 'error': notice('error', p); break;
      case 'session_cleared': resetAll(); break;
      case 'compaction_started': compaction = notice('info', { message: 'Compacting conversation…' }); break;
      case 'compaction_completed': compactionDone(p); break;
      default: return false;
    }
    return true;
  }

  function renderItem(item) {
    switch (item.kind) {
      case 'subagent': return renderSubagent(item);
      case 'message': return renderMessage(item);
      case 'thinking': return renderThinkingCard(item);
      case 'tool': return renderToolCard(item);
      case 'notice': return renderNotice(item);
      case 'chip':
        if (item.chip === 'stage') return renderStageChip(item.data);
        if (item.chip === 'pack') return renderPackChip(item.data);
        return renderRetrievalChip(item.data);
      default: return null;
    }
  }

  function rootAgent(item) { while (item.parent) item = item.parent; return item; }
  function renderSubagent(item) {
    return h('section', { class: 'bv-subagent-card', 'data-id': item.id },
      h('div', { class: 'bv-subagent-card__head' },
        h('strong', null, item.name),
        h('span', { class: 'bv-subagent-card__status' }, item.status)),
      item.task ? h('p', { class: 'bv-subagent-card__task' }, item.task) : null,
      item.children.length ? h('div', { class: 'bv-subagent-card__children' }, item.children.map(renderSubagent)) : null);
  }

  // Incremental drawing: every item owns one DOM node. Only dirty items are
  // re-rendered; a streaming assistant bubble is patched in place so the rest of
  // the transcript (and any text selection in it) is left untouched.
  const nodes = new Map(); // item -> DOM node
  const dirty = new Set();
  let section = null;
  let empty = null;

  function buildNode(item) {
    const node = mountTree(renderItem(item), ctx.doc);
    if (item.kind === 'message' && item.role === 'assistant') patchBubble(node, item);
    return node;
  }
  // Assistant text is markdown; markdown.js escapes before adding tags.
  function patchBubble(node, item) {
    const bubble = node.querySelector('.bv-message__bubble');
    if (!bubble) return;
    bubble.innerHTML = renderMarkdown(item.content);
    bubble.classList.add('bv-message__bubble--md');
  }

  const nodes2 = new Map(); // node -> item
  let showingEmpty = false;

  function patchThinking(node, item) {
    const text = node.querySelector('.bv-thinking__text');
    const open = node.classList.contains('bv-thinking--open');
    const dur = node.querySelector('.bv-thinking__duration');
    const wantDur = formatDuration(item.durationMs);
    if (!text || open !== !!item.open || (wantDur && !dur)) return false;
    if (dur) dur.textContent = wantDur;
    text.textContent = String(item.text);
    return true;
  }

  function draw() {
    scheduled = false;
    if (!section) {
      section = ctx.doc.createElement('section');
      section.className = 'bv-transcript';
      region.replaceChildren(section);
    }
    const near = region.scrollHeight - region.scrollTop - region.clientHeight < 80;
    const appended = [];
    for (const item of dirty) {
      const old = nodes.get(item);
      if (old && item.kind === 'message' && item.role === 'assistant') { patchBubble(old, item); continue; }
      if (old && item.kind === 'thinking' && patchThinking(old, item)) continue;
      const node = buildNode(item);
      nodes.set(item, node);
      nodes2.set(node, item);
      if (!old) { appended.push(node); continue; }
      nodes2.delete(old);
      if (typeof old.replaceWith === 'function') old.replaceWith(node); else reorder = true;
    }
    dirty.clear();
    if (items.length === 0) {
      if (!empty) empty = mountTree(h('p', { class: 'bv-transcript__empty' }, 'No messages yet'), ctx.doc);
      section.replaceChildren(empty);
      showingEmpty = true;
      reorder = false;
    } else if (reorder || showingEmpty) {
      section.replaceChildren(...items.map((item) => nodes.get(item)));
      showingEmpty = false;
      reorder = false;
    } else {
      for (const node of appended) section.appendChild(node);
    }
    if (stick || near) region.scrollTop = region.scrollHeight;
    stick = false;
  }

  function schedule() {
    if (scheduled) return;
    scheduled = true;
    ctx.frame(draw);
  }

  ctx.on('sse', (msg) => { if (handle(msg)) schedule(); });
  ctx.on('notice', ({ type, message }) => { notice(type || 'info', { message }); schedule(); });
  ctx.on('transcript:user-message', ({ text }) => {
    push({ kind: 'message', role: 'user', content: text, time: new Date(ctx.now()).toISOString() });
    stick = true;
    schedule();
  });
  ctx.on('session:reset', resetAll);

  // Thinking cards toggle open; the node's position equals the item's index.
  delegate(region, 'click', (action, _id, el) => {
    if (action === 'toggle-result') {
      const card = el.closest('.bv-tool');
      const item = card && findTool(card.getAttribute('data-id'));
      if (item) { item.showAll = !item.showAll; dirty.add(item); schedule(); }
      return;
    }
    if (action !== 'toggle-thinking') return;
    const item = nodes2.get(el.closest('.bv-thinking'));
    if (item && item.kind === 'thinking') {
      item.open = !item.open;
      item.userToggled = true;
      dirty.add(item);
      schedule();
    }
  });

  draw();
}
