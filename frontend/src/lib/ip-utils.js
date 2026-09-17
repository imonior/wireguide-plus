// IPv4/IPv6 helpers shared by the tunnel UI.
//
// The latency probe target is a plain text field, so the UI must decide
// whether the typed address is actually routable through the tunnel
// (i.e. covered by some peer's AllowedIPs). Doing that comparison needs
// real CIDR arithmetic — string matching ("starts with 10.2.") would be
// wrong for anything other than clean octet boundaries.
//
// Deliberately BigInt-free: the Wails bundle targets environments where
// BigInt literals may be unavailable, and esbuild flagged them as an
// unsupported feature for the configured target. IPv6 therefore compares
// per 16-bit group, IPv4 via unsigned 32-bit arithmetic.

function parseIPv4(value) {
  if (typeof value !== 'string' || value.indexOf('.') === -1) return null;
  const parts = value.split('.');
  if (parts.length !== 4) return null;
  const octets = [];
  for (const part of parts) {
    if (!/^\d{1,3}$/.test(part)) return null;
    const octet = Number(part);
    if (octet > 255) return null;
    octets.push(octet);
  }
  return { family: 4, octets };
}

function groupsToIPv6(groups) {
  if (groups.length !== 8) return null;
  const out = [];
  for (const group of groups) {
    if (!/^[0-9a-fA-F]{1,4}$/.test(group)) return null;
    out.push(parseInt(group, 16));
  }
  return { family: 6, groups: out };
}

// Turn a trailing IPv4 group ("::ffff:192.168.0.1") into its two hex
// groups so the rest of the parser only deals with 16-bit groups.
function expandIPv6Mixed(groups) {
  if (!groups.length || groups[groups.length - 1].indexOf('.') === -1) return groups;
  const v4 = parseIPv4(groups.pop());
  if (!v4) return null;
  return groups.concat([
    ((v4.octets[0] << 8) | v4.octets[1]).toString(16),
    ((v4.octets[2] << 8) | v4.octets[3]).toString(16)
  ]);
}

function parseIPv6(value) {
  if (typeof value !== 'string' || value.indexOf(':') === -1) return null;
  const double = value.indexOf('::');
  if (double !== -1) {
    if (double !== value.lastIndexOf('::')) return null; // at most one "::"
    const head = double === 0 ? [] : value.slice(0, double).split(':');
    const tailRaw = value.slice(double + 2);
    const tail = expandIPv6Mixed(tailRaw ? tailRaw.split(':') : []);
    if (!tail) return null;
    const fill = 8 - head.length - tail.length;
    if (fill < 0) return null;
    return groupsToIPv6(head.concat(Array(fill).fill('0'), tail));
  }
  let groups = value.split(':');
  if (groups.length !== 8) return null;
  groups = expandIPv6Mixed(groups);
  if (!groups) return null;
  return groupsToIPv6(groups);
}

export function parseIP(value) {
  return parseIPv4(value) || parseIPv6(value);
}

// parseAllowed parses one AllowedIPs entry ("10.2.0.0/16", "fd00::/8",
// or a bare address meaning "exactly this host").
function parseAllowed(entry) {
  if (typeof entry !== 'string') return null;
  const slash = entry.indexOf('/');
  const addr = parseIP((slash === -1 ? entry : entry.slice(0, slash)).trim());
  if (!addr) return null;
  if (slash === -1) return { ...addr, bits: addr.family === 4 ? 32 : 128 };
  const raw = entry.slice(slash + 1).trim();
  if (!/^\d{1,3}$/.test(raw)) return null;
  const bits = Number(raw);
  if (bits > (addr.family === 4 ? 32 : 128)) return null;
  return { ...addr, bits };
}

function v4ToU32(addr) {
  const [a, b, c, d] = addr.octets;
  return (((a << 24) | (b << 16) | (c << 8) | d) >>> 0);
}

function v4Mask(bits) {
  if (bits <= 0) return 0;
  // Shift counts wrap at 32, so bits === 32 must not reach `<< 32`.
  return ((Math.pow(2, bits) - 1) << (32 - bits)) >>> 0;
}

function v4Within(addr, allowed) {
  const mask = v4Mask(allowed.bits);
  return (((v4ToU32(addr) & mask) >>> 0) === ((v4ToU32(allowed) & mask) >>> 0));
}

function v6Within(addr, allowed) {
  for (let i = 0; i < 8; i++) {
    const bits = Math.min(Math.max(allowed.bits - i * 16, 0), 16);
    if (bits === 0) return true; // remaining groups are entirely outside the prefix
    if (bits === 16) {
      if (addr.groups[i] !== allowed.groups[i]) return false;
      continue;
    }
    const mask = ((0xffff << (16 - bits)) & 0xffff);
    if ((addr.groups[i] & mask) !== (allowed.groups[i] & mask)) return false;
  }
  return true;
}

/**
 * Decide whether `target` is routable through the tunnel described by
 * `allowedIPs`.
 *
 * Returns one of:
 *   'not-an-ip'  — target is a hostname (or malformed); coverage cannot be
 *                  decided statically, so callers must not warn on this.
 *   'covered'    — inside at least one AllowedIPs entry.
 *   'uncovered'  — a valid IP that no peer claims.
 */
export function classifyIPCoverage(target, allowedIPs) {
  const addr = parseIP((target || '').trim());
  if (!addr) return 'not-an-ip';
  for (const entry of allowedIPs || []) {
    const allowed = parseAllowed(entry);
    if (!allowed || allowed.family !== addr.family) continue;
    const within = allowed.family === 4 ? v4Within(addr, allowed) : v6Within(addr, allowed);
    if (within) return 'covered';
  }
  return 'uncovered';
}

/**
 * Strip the ":port" suffix from an endpoint. Ports are part of the WireGuard
 * endpoint value but meaningless for a ping target, and IPv6 keeps them
 * bracketed ("[fd00::1]:51820") which naive string portsplitting mangles.
 */
export function stripPort(value) {
  const text = (value || '').trim();
  if (!text) return '';
  if (text.startsWith('[')) {
    const close = text.indexOf(']');
    return close === -1 ? text : text.slice(1, close);
  }
  const colons = text.split(':').length - 1;
  // Exactly one colon means host:port; more than one is bare IPv6.
  if (colons === 1) {
    const port = text.slice(text.lastIndexOf(':') + 1);
    if (/^\d+$/.test(port)) return text.slice(0, text.lastIndexOf(':'));
  }
  return text;
}
