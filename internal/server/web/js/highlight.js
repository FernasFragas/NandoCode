// Pure, lossless syntax tokenizers. Consumers must render token.text as text,
// never as HTML. Unrecognized and malformed syntax remains visible verbatim.

const KEYWORDS = {
  go: new Set('break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var'.split(' ')),
  js: new Set('async await break case catch class const continue debugger default delete do else export extends finally for from function if import in instanceof let new of return static super switch throw try typeof var void while yield'.split(' ')),
  shell: new Set('case do done elif else esac fi for function if in select then until while echo'.split(' ')),
};

const NUMBER = /^(?:0[xX][\da-fA-F]+|0[bB][01]+|0[oO][0-7]+|(?:\d+(?:_\d+)*(?:\.\d+(?:_\d+)*)?|\.\d+)(?:[eE][+-]?\d+)?)/;
const WORD = /^[A-Za-z_$][\w$]*/;

function push(tokens, type, text) {
  if (!text) return;
  const last = tokens.at(-1);
  if (type === 'plain' && last?.type === 'plain') last.text += text;
  else tokens.push({ type, text });
}

function quotedEnd(source, start, quote, raw = false) {
  let i = start + 1;
  while (i < source.length) {
    if (!raw && source[i] === '\\') {
      i += Math.min(2, source.length - i);
    } else if (source[i] === quote) {
      return i + 1;
    } else {
      i++;
    }
  }
  return source.length;
}

function previousSignificant(source, index) {
  let i = index - 1;
  while (i >= 0 && /\s/.test(source[i])) i--;
  return i >= 0 ? source[i] : '';
}

// Linear scan for a CSS property name followed by optional whitespace and ':'.
function matchProperty(source, start) {
  let i = start;
  if (source.startsWith('--', i)) i += 2;
  if (!/[A-Za-z]/.test(source[i] ?? '')) return null;
  while (i < source.length && /[\w-]/.test(source[i])) i++;
  const end = i;
  while (i < source.length && /\s/.test(source[i])) i++;
  return source[i] === ':' ? source.slice(start, end) : null;
}

// Linear scan for a YAML "key:" or TOML "key =" at the start of a line.
function matchKey(source, start, language) {
  let i = start;
  while (source[i] === ' ' || source[i] === '\t') i++;
  const nameStart = i;
  if (language === 'toml') {
    if (!/[A-Za-z_]/.test(source[i] ?? '')) return null;
    while (i < source.length && /[\w-]/.test(source[i])) i++;
    const nameEnd = i;
    while (source[i] === ' ' || source[i] === '\t') i++;
    return source[i] === '=' ? { indent: source.slice(start, nameStart), name: source.slice(nameStart, nameEnd), end: nameEnd } : null;
  }
  if (i >= source.length || /[\s:#]/.test(source[i])) return null;
  while (i < source.length && source[i] !== ':' && source[i] !== '#' && source[i] !== '\n') i++;
  if (source[i] !== ':') return null;
  let nameEnd = i;
  while (nameEnd > nameStart && /\s/.test(source[nameEnd - 1])) nameEnd--;
  return { indent: source.slice(start, nameStart), name: source.slice(nameStart, nameEnd), end: nameEnd };
}

function scanCode(source, language) {
  const tokens = [];
  const lineComment = language === 'go' || language === 'js' ? '//' : language === 'yaml' || language === 'toml' || language === 'shell' ? '#' : null;
  const blockComment = language === 'go' || language === 'js' || language === 'css';
  let i = 0;
  while (i < source.length) {
    const char = source[i];
    const rest = source.slice(i);
    if (lineComment && source.startsWith(lineComment, i) && (language !== 'shell' || i === 0 || /[\s;&|()]/.test(source[i - 1]))) {
      const end = source.indexOf('\n', i);
      const next = end < 0 ? source.length : end;
      push(tokens, 'comment', source.slice(i, next));
      i = next;
      continue;
    }
    if (blockComment && rest.startsWith('/*')) {
      const end = source.indexOf('*/', i + 2);
      const next = end < 0 ? source.length : end + 2;
      push(tokens, 'comment', source.slice(i, next));
      i = next;
      continue;
    }
    if (char === '"' || (language !== 'json' && char === '\'') || ((language === 'go' || language === 'js' || language === 'toml') && char === '`')) {
      const raw = char === '`' || (language === 'toml' && source.slice(i, i + 3) === "'''");
      let next;
      if (language === 'toml' && (rest.startsWith('"""') || rest.startsWith("'''"))) {
        const delimiter = rest.slice(0, 3);
        const end = source.indexOf(delimiter, i + 3);
        next = end < 0 ? source.length : end + 3;
      } else {
        next = quotedEnd(source, i, char, raw || (language === 'shell' && char === '\''));
      }
      push(tokens, 'string', source.slice(i, next));
      i = next;
      continue;
    }
    if ((language === 'yaml' || language === 'toml') && (i === 0 || source[i - 1] === '\n')) {
      const key = matchKey(source, i, language);
      if (key) {
        push(tokens, 'plain', key.indent);
        push(tokens, 'key', key.name);
        i = key.end;
        continue;
      }
    }
    if (language === 'css') {
      const property = /[A-Za-z-]/.test(char) && (i === 0 || !/[\w-]/.test(source[i - 1])) && /[;{]/.test(previousSignificant(source, i)) ? matchProperty(source, i) : null;
      if (property) {
        push(tokens, 'property', property);
        i += property.length;
        continue;
      }
    }
    if (language === 'shell' && char === '$') {
      const variable = rest.match(/^\$(?:[A-Za-z_][\w]*|\{[^}\n]{0,256}\}|[?#!@$*0-9])/);
      if (variable) {
        push(tokens, 'variable', variable[0]);
        i += variable[0].length;
        continue;
      }
    }
    const number = rest.match(NUMBER);
    if (number && (i === 0 || !/[\w$]/.test(source[i - 1]))) {
      push(tokens, 'number', number[0]);
      i += number[0].length;
      continue;
    }
    const word = rest.match(WORD);
    if (word) {
      const value = word[0];
      const type = KEYWORDS[language]?.has(value) ? 'keyword'
        : /^(?:true|false)$/.test(value) && ['json', 'yaml', 'toml', 'js'].includes(language) ? 'boolean'
          : value === 'null' && ['json', 'js'].includes(language) ? 'null' : 'plain';
      push(tokens, type, value);
      i += value.length;
      continue;
    }
    push(tokens, /[{}()[\],;:.=+*/<>!?&|%-]/.test(char) ? 'punctuation' : 'plain', char);
    i++;
  }
  return tokens;
}

export const tokenizeGo = (source) => scanCode(String(source), 'go');
export const tokenizeJS = (source) => scanCode(String(source), 'js');
export const tokenizeJSON = (source) => scanCode(String(source), 'json');
export const tokenizeCSS = (source) => scanCode(String(source), 'css');
export const tokenizeYAML = (source) => scanCode(String(source), 'yaml');
export const tokenizeTOML = (source) => scanCode(String(source), 'toml');
export const tokenizeShell = (source) => scanCode(String(source), 'shell');

export function tokenizeHTML(source) {
  source = String(source);
  const tokens = [];
  let i = 0;
  let inTag = false;
  while (i < source.length) {
    const rest = source.slice(i);
    if (!inTag && rest.startsWith('<!--')) {
      const end = source.indexOf('-->', i + 4);
      const next = end < 0 ? source.length : end + 3;
      push(tokens, 'comment', source.slice(i, next));
      i = next;
      continue;
    }
    if (!inTag && source[i] === '<') {
      const match = rest.match(/^<(\/?)([A-Za-z][\w:-]*)/);
      if (match) {
        push(tokens, 'punctuation', '<' + match[1]);
        push(tokens, 'tag', match[2]);
        i += match[0].length;
        inTag = true;
        continue;
      }
    }
    if (inTag) {
      if (source[i] === '"' || source[i] === '\'') {
        const next = quotedEnd(source, i, source[i]);
        push(tokens, 'string', source.slice(i, next));
        i = next;
        continue;
      }
      if (rest.startsWith('/>') || source[i] === '>') {
        const value = rest.startsWith('/>') ? '/>' : '>';
        push(tokens, 'punctuation', value);
        i += value.length;
        inTag = false;
        continue;
      }
      const attribute = rest.match(/^[A-Za-z_:][\w:.-]*/);
      if (attribute) {
        push(tokens, 'attribute', attribute[0]);
        i += attribute[0].length;
        continue;
      }
    }
    push(tokens, 'plain', source[i]);
    i++;
  }
  return tokens;
}

const TOKENIZERS = {
  go: tokenizeGo, js: tokenizeJS, javascript: tokenizeJS, jsx: tokenizeJS,
  json: tokenizeJSON, html: tokenizeHTML, htm: tokenizeHTML,
  css: tokenizeCSS, yaml: tokenizeYAML, yml: tokenizeYAML,
  toml: tokenizeTOML, shell: tokenizeShell, sh: tokenizeShell, bash: tokenizeShell, zsh: tokenizeShell,
};

export function tokenize(source, language) {
  const text = String(source ?? '');
  const tokenizer = TOKENIZERS[String(language ?? '').toLowerCase()];
  return tokenizer ? tokenizer(text) : text ? [{ type: 'plain', text }] : [];
}
