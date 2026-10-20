import { test } from 'node:test';
import assert from 'node:assert/strict';
import { makeCtx, fire, flush, sse } from '../fakedom.js';
import { mount as mountSplit } from './split.js';
import { mount as mountAgents } from './agents.js';

// Minimal stand-in for V6c's tabs.js registration seam (ctx.registerTabKind).
function withTabs(t) {
  const kinds = new Map();
  t.ctx.registerTabKind = (kind, mountFn) => {
    const host = t.doc.createElement('section');
    host.id = 'tab-view-' + kind;
    host.hidden = true;
    t.ctx.el('tab-content').appendChild(host);
    kinds.set(kind, { host, view: mountFn({ ctx: t.ctx, host, kind }) || {} });
    return true;
  };
  t.show = (tab) => {
    for (const [kind, { host, view }] of kinds) {
      host.hidden = tab.kind !== kind;
      if (!host.hidden) view.show?.(tab); else view.hide?.();
    }
    t.ctx.emit('tabs:active', { tab });
  };
  t.kinds = kinds;
}

test('Agents and log views register as kinds agents and agents-log, filter and stop', async () => {
  const t = makeCtx({ routes: { 'GET /v1/sessions/s1': () => ({ json: { workers: [
    { task_id: 'a', name: 'Explorer', status: 'running', task: 'scan', parent: 'coordinator' },
    { task_id: 'b', name: 'Builder', status: 'running', task: 'build', parent: 'coordinator' },
  ] } }) } });
  withTabs(t);
  mountSplit(t.ctx);
  mountAgents(t.ctx);
  assert.deepEqual([...t.kinds.keys()].sort(), ['agents', 'agents-log']);
  t.ctx.set({ sessionId: 's1', running: true });
  t.ctx.emit('session:ready', 's1');
  await flush();
  t.show({ id: 'agents', kind: 'agents' });
  await flush();
  const host = t.kinds.get('agents').host;
  assert.match(host.textContent, /Explorer/);
  assert.match(host.textContent, /Builder/);
  t.ctx.emit('sse', sse('subagent_progress', { task_id: 'a', tool: 'grep', target: 'src', status: 'completed' }, 'evt-a'));
  t.ctx.emit('sse', sse('subagent_progress', { task_id: 'b', tool: 'read', target: 'README', status: 'completed' }, 'evt-b'));
  await flush();
  fire(host.querySelector('[data-action="filter-agent"]'), 'click');
  assert.match(host.textContent, /grep/);
  assert.doesNotMatch(host.textContent, /read README/);
  let stops = 0;
  t.ctx.on('runcontrol:stop-run', () => { stops++; });
  fire(host.querySelector('[data-action="stop-all"]'), 'click');
  assert.equal(stops, 1);
  const opened = [];
  t.ctx.on('tab:open', (tab) => opened.push(tab));
  fire(host.querySelector('[data-action="view-logs"]'), 'click');
  assert.deepEqual(opened, [{ id: 'agents-log', kind: 'agents-log', title: 'Agent logs' }]);
  t.show(opened[0]);
  await flush();
  const log = t.kinds.get('agents-log').host;
  assert.match(log.textContent, /grep/);
  assert.equal(log.querySelector('.bv-agents__graph'), null);
  assert.equal(t.kinds.get('agents').host.hidden, true);
});

test('empty state covers coordinator workers and normal sub-agents', async () => {
  const t = makeCtx();
  withTabs(t);
  mountSplit(t.ctx);
  mountAgents(t.ctx);
  t.show({ id: 'agents', kind: 'agents' });
  await flush();
  const text = t.kinds.get('agents').host.textContent;
  assert.match(text, /No sub-agents in this session yet/);
  assert.match(text, /coordinator/i);
  assert.match(text, /main agent|Agent tool|sub-agent/i);
});
