// errText normalises error values coming out of Wails RPC.
// Wails errors can be Error objects, plain strings, or structured
// `{code, message}` payloads depending on which layer failed. A naive
// `e.toString()` returns "[object Object]" on the last case — use this
// helper everywhere instead.
export function errText(e) {
  if (!e) return 'unknown error';
  if (typeof e === 'string') return e;
  if (e.message) return e.message;
  if (e.error) return e.error;
  try {
    return JSON.stringify(e);
  } catch {
    return String(e);
  }
}

// localizeValidation maps the backend's encoded validation errors
// ("code|field|message", from config.ValidationResult.EncodedMessages) onto
// validator.<code> i18n keys, so a rejected save reads in the user's
// language instead of Go's English. Behaviour:
//   - entry without '|' (older payload / parse detail): shown as-is;
//   - known code: the translated validator.<code> message (numbers such as
//     the offending MTU/range bound are pulled out of the raw message where
//     the translation needs them);
//   - unknown code (app older than the code list): "field: message" fallback.
// `t` is the translated store function ($t) — passed in rather than imported
// so this helper stays store-agnostic and testable.
export function localizeValidation(list, t) {
  if (!Array.isArray(list)) return [];
  return list.map((entry) => {
    if (typeof entry !== 'string' || !entry.includes('|')) return entry;
    const i1 = entry.indexOf('|');
    const i2 = entry.indexOf('|', i1 + 1);
    if (i2 < 0) return entry;
    const code = entry.slice(0, i1);
    const field = entry.slice(i1 + 1, i2);
    const raw = entry.slice(i2 + 1);
    // Parse errors carry no stable code — the parser's message is the detail.
    // Localise the prefix, keep the raw detail alongside it.
    if (code === 'parse_error') {
      const base = t('validator.parse_error');
      const prefix = base && base !== 'validator.parse_error' ? base : 'config parse error';
      return raw ? prefix + ': ' + raw : prefix;
    }
    const key = 'validator.' + code;
    const params = { field };
    // awg_range / keepalive_range raw text: "must be between 0 and <max>, got <got>"
    const m = raw.match(/between 0 and (\d+), got (-?\d+)/);
    if (m) { params.max = m[1]; params.got = m[2]; }
    const msg = t(key, params);
    if (!msg || msg === key) {
      return field ? `${field}: ${raw}` : raw;
    }
    return msg;
  });
}
