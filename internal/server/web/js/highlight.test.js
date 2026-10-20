import { test } from 'node:test';
import assert from 'node:assert/strict';

import { tokenize, tokenizeGo, tokenizeJS, tokenizeJSON, tokenizeHTML, tokenizeCSS, tokenizeYAML, tokenizeTOML, tokenizeShell } from './highlight.js';

const join = (tokens) => tokens.map((token) => token.text).join('');
const stringToken = (tokens, value) => tokens.find((token) => token.type === 'string' && token.text === value);

test('Go raw strings contain comment markers and quotes', () => {
  const source = 's := `first // line\n/* block */ "quoted"` // real comment';
  const tokens = tokenizeGo(source);
  assert.ok(stringToken(tokens, '`first // line\n/* block */ "quoted"`'));
  assert.deepEqual(tokens.filter((token) => token.type === 'comment').map((token) => token.text), ['// real comment']);
  assert.equal(join(tokens), source);
});

test('comment markers inside strings stay in their strings', () => {
  for (const tokenizeLanguage of [tokenizeGo, tokenizeJS, tokenizeJSON, tokenizeCSS, tokenizeYAML, tokenizeTOML, tokenizeShell]) {
    const source = '"// text /* text */ # text"';
    const tokens = tokenizeLanguage(source);
    assert.ok(stringToken(tokens, source), tokenizeLanguage.name);
    assert.equal(tokens.some((token) => token.type === 'comment'), false, tokenizeLanguage.name);
  }
});

test('unterminated strings tokenize through EOF without throwing', () => {
  for (const [tokenizeLanguage, source] of [
    [tokenizeGo, '"unfinished // comment'],
    [tokenizeJS, "'unfinished /* comment"],
    [tokenizeJSON, '"unfinished'],
    [tokenizeHTML, '<div title="unfinished >'],
    [tokenizeCSS, 'a { content: "unfinished /* text'],
    [tokenizeYAML, 'key: "unfinished # text'],
    [tokenizeTOML, 'key = "unfinished # text'],
    [tokenizeShell, "echo 'unfinished # text"],
  ]) {
    const tokens = tokenizeLanguage(source);
    assert.equal(join(tokens), source, tokenizeLanguage.name);
    assert.equal(tokens.at(-1).type, 'string', tokenizeLanguage.name);
  }
});

test('script closing text inside a JavaScript string stays one token', () => {
  const value = '"before </script> after"';
  const tokens = tokenizeJS(`const x = ${value};`);
  assert.ok(stringToken(tokens, value));
  assert.equal(join(tokens), `const x = ${value};`);
});

test('each language classifies representative syntax', () => {
  const cases = [
    ['go', 'package main\nfunc main() { n := 42 }', 'keyword', 'func'],
    ['js', 'const count = 42; // note', 'keyword', 'const'],
    ['json', '{"ok": true, "count": 42}', 'boolean', 'true'],
    ['html', '<div class="x">Hi</div><!-- note -->', 'tag', 'div'],
    ['css', 'a { color: red; } /* note */', 'property', 'color'],
    ['yaml', 'enabled: true # note', 'key', 'enabled'],
    ['toml', 'enabled = true # note', 'key', 'enabled'],
    ['shell', 'echo "$HOME" # note', 'keyword', 'echo'],
  ];
  for (const [language, source, type, value] of cases) {
    assert.ok(tokenize(source, language).some((token) => token.type === type && token.text === value), language);
  }
});

test('token streams round-trip arbitrary source for every language', () => {
  const languages = ['go', 'js', 'json', 'html', 'css', 'yaml', 'toml', 'shell', 'unknown'];
  let seed = 0x12345678;
  const chars = 'abcDEF09 \t\n\r\u0000😀"\'`#<>/*\\{}[]:=;$';
  for (let sample = 0; sample < 100; sample++) {
    let source = '';
    const length = sample % 47;
    for (let i = 0; i < length; i++) {
      seed = (Math.imul(seed, 1664525) + 1013904223) >>> 0;
      source += chars[seed % chars.length];
    }
    for (const language of languages) {
      const tokens = tokenize(source, language);
      assert.equal(join(tokens), source, `${language}: ${JSON.stringify(source)}`);
      for (const token of tokens) {
        assert.deepEqual(Object.keys(token).sort(), ['text', 'type']);
        assert.ok(token.text.length > 0);
      }
    }
  }
});

test('shell variables are single variable tokens', () => {
  for (const variable of ['$HOME', '${USER}', '$1', '$?', '${A:-b}']) {
    const tokens = tokenizeShell(`echo ${variable} x`);
    assert.ok(tokens.some((token) => token.type === 'variable' && token.text === variable), variable);
    assert.equal(join(tokens), `echo ${variable} x`);
  }
});

test('shell # starts a comment only at a word boundary', () => {
  const mid = tokenizeShell('echo foo#bar # real');
  assert.deepEqual(mid.filter((token) => token.type === 'comment').map((token) => token.text), ['# real']);
  assert.equal(join(mid), 'echo foo#bar # real');
  const lead = tokenizeShell('# top\nls #x');
  assert.deepEqual(lead.filter((token) => token.type === 'comment').map((token) => token.text), ['# top', '#x']);
  const param = tokenizeShell('echo ${#arr[@]} $#');
  assert.equal(param.filter((token) => token.type === 'comment').length, 0);
  assert.equal(join(param), 'echo ${#arr[@]} $#');
});

test('pathological long inputs tokenize in linear time', { timeout: 5000 }, () => {
  const n = 200000;
  const inputs = [
    ' '.repeat(n) + 'a',
    'a'.repeat(n),
    '1'.repeat(n),
    'a { b: c; '.repeat(n / 10),
    '"\\'.repeat(n / 2),
    '<a '.repeat(n / 3),
    '/*'.repeat(n / 2),
    '\n'.repeat(n),
    ';'.repeat(n) + ' '.repeat(n),
    '${'.repeat(n / 2),
    'a' + ' '.repeat(n) + ':',
  ];
  for (const input of inputs) {
    for (const language of ['go', 'js', 'json', 'html', 'css', 'yaml', 'toml', 'shell']) {
      const started = performance.now();
      assert.equal(join(tokenize(input, language)), input);
      const elapsed = performance.now() - started;
      // Synchronous code is not interrupted by the test timeout, so bound each run explicitly.
      assert.ok(elapsed < 1500, `${language} took ${Math.round(elapsed)}ms on ${JSON.stringify(input.slice(0, 12))}`);
    }
  }
});

test('CSS hyphen-joined identifier runs tokenize in linear time', { timeout: 10000 }, () => {
  const n = 60000;
  const inputs = ['a-'.repeat(n / 2), 'a{' + 'a-b'.repeat(n / 3), '--a'.repeat(n / 3)];
  for (const input of inputs) {
    const started = performance.now();
    assert.equal(join(tokenizeCSS(input)), input);
    const elapsed = performance.now() - started;
    assert.ok(elapsed < 500, `css took ${Math.round(elapsed)}ms on ${JSON.stringify(input.slice(0, 12))}`);
  }
});
