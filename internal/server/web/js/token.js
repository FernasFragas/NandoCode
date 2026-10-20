// Bearer-token handoff (BF-7). `nandocodego server` prints a launch URL ending
// in #token=...; the fragment never reaches the server. The token moves into
// sessionStorage and is removed from the address bar. Storage access is always
// wrapped: it can throw in private windows or with blocked site data.

const KEY = 'nandocodego.token';

function params(hash) {
  const raw = typeof hash === 'string' ? hash.replace(/^#/, '') : '';
  return raw === '' ? [] : raw.split('&');
}

// extractToken('#token=abc&x=1') -> 'abc'. Returns null when there is none.
export function extractToken(hash) {
  for (const part of params(hash)) {
    if (!part.startsWith('token=')) continue;
    let value = part.slice('token='.length);
    try { value = decodeURIComponent(value); } catch { /* keep the raw value */ }
    return value === '' ? null : value;
  }
  return null;
}

// stripToken('#token=abc&x=1') -> '#x=1'.
export function stripToken(hash) {
  const rest = params(hash).filter((p) => !p.startsWith('token='));
  return rest.length ? '#' + rest.join('&') : '';
}

export function loadToken(storage) {
  try {
    return (storage && storage.getItem(KEY)) || '';
  } catch {
    return '';
  }
}

export function saveToken(storage, token) {
  try {
    if (storage) storage.setItem(KEY, token);
  } catch { /* storage unavailable: the token stays in memory only */ }
}

// adoptToken reads #token= from win.location, stores it, and rewrites the URL
// without it. With no fragment token it returns whatever storage holds.
export function adoptToken(win, storage) {
  const fromURL = extractToken(win.location.hash);
  if (fromURL === null) return loadToken(storage);
  saveToken(storage, fromURL);
  win.history.replaceState(null, '', win.location.pathname + win.location.search + stripToken(win.location.hash));
  return fromURL;
}
