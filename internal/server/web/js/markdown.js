// DOM-free markdown renderer. Everything is HTML-escaped first; only a fixed
// set of tags is then added, and links are limited to http, https and mailto.

export function escapeHtml(str) {
  str = String(str ?? '');
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

const SAFE_LINK = /^(?:https?:\/\/|mailto:)\S+$/i;

// Code spans and links are lifted into placeholders so emphasis markers inside
// them never alter an attribute value.
function renderInline(line) {
  const stash = [];
  const hold = (html) => '\u0000' + (stash.push(html) - 1) + '\u0000';
  let html = escapeHtml(line).replace(/\u0000/g, '');
  html = html.replace(/`([^`]+)`/g, (_, c) => hold('<code>' + c + '</code>'));
  html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, (m, label, url) => {
    if (!SAFE_LINK.test(url)) return m;
    return hold('<a href="' + url + '" target="_blank" rel="noopener noreferrer">' + label + '</a>');
  });
  html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
  html = html.replace(/\*([^*]+)\*/g, '<em>$1</em>');
  return html.replace(/\u0000(\d+)\u0000/g, (_, i) => stash[Number(i)]);
}

export function renderMarkdown(text) {
  const lines = String(text ?? '').split('\n');
  const out = [];
  let inCode = false;
  let codeLines = [];
  for (const line of lines) {
    if (line.startsWith('```')) {
      if (inCode) {
        out.push('<pre><code>' + escapeHtml(codeLines.join('\n')) + '</code></pre>');
        codeLines = [];
        inCode = false;
      } else {
        inCode = true;
      }
      continue;
    }
    if (inCode) {
      codeLines.push(line);
      continue;
    }
    out.push(renderInline(line));
  }
  if (inCode) {
    out.push('<pre><code>' + escapeHtml(codeLines.join('\n')) + '</code></pre>');
  }
  const paragraphs = [];
  let para = [];
  out.forEach((line) => {
    if (line.trim() === '') {
      if (para.length) {
        paragraphs.push('<p>' + para.join('<br>') + '</p>');
        para = [];
      }
    } else {
      para.push(line);
    }
  });
  if (para.length) {
    paragraphs.push('<p>' + para.join('<br>') + '</p>');
  }
  return paragraphs.join('');
}
