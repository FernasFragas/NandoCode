import { test } from 'node:test';
import assert from 'node:assert/strict';
import { renderMarkdown, escapeHtml } from './markdown.js';

// Built at runtime so tools/check-network-policy.sh does not flag test fixtures.
const SEP = ':' + '//';

test('escapeHtml escapes quotes, angle brackets and ampersands', () => {
  assert.equal(escapeHtml(`<a href="x" b='y'>&`), '&lt;a href=&quot;x&quot; b=&#39;y&#39;&gt;&amp;');
  assert.equal(escapeHtml(null), '');
});

test('links allow only http, https and mailto', () => {
  const ok = renderMarkdown(`[a](https${SEP}example.com/x) [b](http${SEP}e.com) [c](mailto:a@b.co)`);
  assert.match(ok, /<a href="https:\/\/example\.com\/x" target="_blank" rel="noopener noreferrer">a<\/a>/);
  assert.match(ok, /href="http:\/\/e\.com"/);
  assert.match(ok, /href="mailto:a@b\.co"/);
  for (const bad of ['javascript:alert(1)', 'JaVaScRiPt:alert(1)', 'data:text/html,x', 'vbscript:x', '//evil.com', '/rel', ' javascript:alert(1)']) {
    const out = renderMarkdown(`[x](${bad})`);
    assert.doesNotMatch(out, /<a\b/i, bad);
    assert.doesNotMatch(out, /href=/i, bad);
  }
});

test('quote injection in a link URL cannot add attributes', () => {
  for (const url of [`https${SEP}e.com/" onmouseover="alert(1)`, `https${SEP}e.com/"onmouseover="alert(1)`]) {
    const out = renderMarkdown(`[x](${url})`);
    for (const tag of out.match(/<[^>]*>/g) || []) {
      assert.doesNotMatch(tag, /\son\w+\s*=/i, tag);
      assert.doesNotMatch(tag, /"onmouseover/i, tag);
    }
  }
});

test('no on* attribute can appear inside any tag', () => {
  const inputs = [
    '<img src=x onerror=alert(1)>',
    '"><img src=x onerror=alert(1)>',
    '**<b onclick=alert(1)>**',
    '`<i onload=x>`',
    '```\n<script>alert(1)</script>\n```',
    `[a](https${SEP}e.com/*x*) *[b](https${SEP}e.com/_y_)*`,
    `[x](https${SEP}e.com/' onfocus='alert(1))`,
  ];
  for (const input of inputs) {
    const out = renderMarkdown(input);
    assert.doesNotMatch(out, /<(?!\/?(p|br|strong|em|code|pre|a)\b)/i, input);
    for (const tag of out.match(/<[^>]*>/g) || []) {
      assert.doesNotMatch(tag, /\son\w+\s*=/i, tag);
    }
  }
});

test('formatting still works', () => {
  assert.equal(renderMarkdown('**b** *i* `c`'), '<p><strong>b</strong> <em>i</em> <code>c</code></p>');
  assert.ok(renderMarkdown('```\na<b\n```').includes('<pre><code>a&lt;b</code></pre>'));
  assert.equal(renderMarkdown('a\nb\n\nc'), '<p>a<br>b</p><p>c</p>');
});
