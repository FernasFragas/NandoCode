// Thin fetch wrapper: bearer token, JSON bodies, and an error that carries the
// HTTP status and a parsed body. Endpoint paths stay in the features.

export class ApiError extends Error {
  constructor(status, text, body) {
    super('HTTP ' + status + (text ? ': ' + text : ''));
    this.status = status;
    this.text = text;
    this.body = body;
  }
}

export function createApi({ fetchImpl = (...a) => globalThis.fetch(...a), getToken = () => '' } = {}) {
  async function request(method, path, body) {
    const headers = {};
    const token = getToken();
    if (token) headers.Authorization = 'Bearer ' + token;
    const init = { method, headers };
    if (body !== undefined) {
      headers['Content-Type'] = 'application/json';
      init.body = JSON.stringify(body);
    }
    return fetchImpl(path, init);
  }

  async function json(method, path, body) {
    const res = await request(method, path, body);
    const raw = typeof res.text === 'function' ? await res.text() : '';
    let parsed = null;
    try { parsed = raw ? JSON.parse(raw) : null; } catch { parsed = null; }
    if (!res.ok) throw new ApiError(res.status, raw.trim().slice(0, 200), parsed);
    return parsed;
  }

  return {
    request,
    json,
    get: (path) => json('GET', path),
    post: (path, body) => json('POST', path, body ?? {}),
  };
}
