// Tunnel-name rules shared by every entry point: file/QR/zip import, the
// config editor's name field, and the detail panel's inline rename.
//
// IMPORTANT: this module mirrors the Go validator in
// internal/storage/name.go (ValidateTunnelName). The backend stays the
// authority — it rejects anything that slips past this module — so any
// drift shows up as a raw English error inside a localised UI. Keep
// MAX_NAME_LENGTH, the allowed character set and RESERVED_DEVICE_NAMES
// in sync with that file.

export const MAX_NAME_LENGTH = 64;

// Windows reserved device names, rejected on every platform so a config
// synced from macOS/Linux to Windows stays usable. Mirrors
// reservedDeviceNames in internal/storage/name.go.
//
// CONIN$ / CONOUT$ can never actually reach the reserved-name branch
// ('$' is outside the allowed character set), but they are listed for
// parity with the backend so the two lists stay identical.
const RESERVED_DEVICE_NAMES = new Set([
  'CON', 'PRN', 'AUX', 'NUL',
  'COM1', 'COM2', 'COM3', 'COM4', 'COM5', 'COM6', 'COM7', 'COM8', 'COM9',
  'LPT1', 'LPT2', 'LPT3', 'LPT4', 'LPT5', 'LPT6', 'LPT7', 'LPT8', 'LPT9',
  'CONIN$', 'CONOUT$',
]);

const ALLOWED_CHARS = /^[A-Za-z0-9\-_ ]+$/;

/**
 * Collapse arbitrary text into a name ValidateTunnelName will accept.
 *
 * This is the algorithm the import path has always used, now shared, so
 * a name typed by hand and a name derived from a dropped file get
 * identical treatment instead of one erroring out and the other being
 * silently rewritten.
 *
 * @param {string} raw
 * @returns {{ name: string, original: string, changed: boolean }}
 *   `name` is '' when nothing usable is left — callers decide whether to
 *   fall back to a default (import) or ask the user again (rename).
 */
export function sanitizeTunnelName(raw) {
  const original = (raw ?? '').trim();
  let s = original
    .replace(/[^A-Za-z0-9\-_ ]+/g, '-')  // unsupported characters → '-'
    .replace(/-{2,}/g, '-')              // collapse the resulting runs
    .replace(/^[-\s]+|[-\s]+$/g, '');    // no leading/trailing '-' or space
  if (s.length > MAX_NAME_LENGTH) {
    s = s.slice(0, MAX_NAME_LENGTH).replace(/[-\s]+$/g, '');
  }
  return { name: s, original, changed: s !== original };
}

/**
 * Validate an already-sanitised name.
 *
 * Returns null when the name is acceptable, otherwise `{ key, params }`
 * naming the i18n message to show. Callers render it with
 * `$t(key, params)` so this module stays free of store imports.
 *
 * Mirrors ValidateTunnelName() rule for rule; after sanitizeTunnelName()
 * only the `empty` and `reserved` branches are reachable, the rest are
 * kept so this can also vet raw input.
 */
export function validateTunnelName(name) {
  if (!name) return { key: 'name.empty', params: {} };
  if (name.length > MAX_NAME_LENGTH) {
    return { key: 'name.too_long', params: { max: MAX_NAME_LENGTH } };
  }
  if (name[0] === ' ' || name[name.length - 1] === ' ') {
    return { key: 'name.edge_space', params: {} };
  }
  if (!ALLOWED_CHARS.test(name)) {
    return { key: 'name.invalid_chars', params: { name } };
  }
  if (RESERVED_DEVICE_NAMES.has(name.toUpperCase())) {
    return { key: 'name.reserved', params: { name } };
  }
  return null;
}
