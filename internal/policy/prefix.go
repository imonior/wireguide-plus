package policy

import (
	"bytes"
	"net/netip"
	"sort"
	"strings"
)

// Policy layer principles that this file implements:
//
//   6.  AllowedIPs must be parsed into individual prefixes — never compared
//       as strings.
//   13. The result is a canonical prefix set: masked, deduplicated and
//       deterministically ordered, so the same input always yields the same
//       output (required for the analyzers to be pure).
//   15. IPv4 and IPv6 are analysed independently — a prefix is never
//       compared against one of the other address family.

// Family identifies an address family so the analyzers can keep IPv4 and
// IPv6 comparisons strictly separate.
type Family string

const (
	FamilyIPv4 Family = "ipv4"
	FamilyIPv6 Family = "ipv6"
)

// ParsePrefixes converts AllowedIPs-style strings into a canonical prefix
// set. Unparseable entries are dropped: the analyzer reports on what it can
// reason about and never fails the caller (a malformed CIDR is the
// validator's problem, not the policy layer's).
//
// Accepted forms:
//   - "10.0.0.0/8"      → 10.0.0.0/8
//   - "10.0.0.5"        → 10.0.0.5/32 (bare host address)
//   - "fd00::/8"        → fd00::/8
//   - "0.0.0.0/0", "::/0" (full tunnel) are kept as-is — they carry the
//     FULL_TUNNEL semantics the route analyzer needs.
func ParsePrefixes(values []string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(values))
	seen := make(map[netip.Prefix]struct{}, len(values))
	for _, raw := range values {
		for _, field := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' }) {
			p, ok := parsePrefix(field)
			if !ok {
				continue
			}
			if _, dup := seen[p]; dup {
				continue
			}
			seen[p] = struct{}{}
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Addr().BitLen() != out[j].Addr().BitLen() {
			return out[i].Addr().BitLen() < out[j].Addr().BitLen()
		}
		if c := out[i].Addr().Compare(out[j].Addr()); c != 0 {
			return c < 0
		}
		return out[i].Bits() < out[j].Bits()
	})
	return out
}

func parsePrefix(s string) (netip.Prefix, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return netip.Prefix{}, false
	}
	if p, err := netip.ParsePrefix(s); err == nil {
		return p.Masked(), true
	}
	// Bare address (no /len): treat as a single host.
	if a, err := netip.ParseAddr(s); err == nil {
		return netip.PrefixFrom(a, a.BitLen()).Masked(), true
	}
	return netip.Prefix{}, false
}

// FamilyOf reports the address family of a prefix.
func FamilyOf(p netip.Prefix) Family {
	if p.Addr().Is4() {
		return FamilyIPv4
	}
	return FamilyIPv6
}

// IsFullTunnel reports whether p covers the entire address space of its
// family — the maximum Traffic Protect scope (principle 9).
func IsFullTunnel(p netip.Prefix) bool {
	return p.Bits() == 0 && p.Addr().IsUnspecified()
}

// RouteConflictKind enumerates the relationship between two prefixes.
type RouteConflictKind string

const (
	// ConflictNone: disjoint ranges. Never reported — listed here so the
	// state machine is explicit (principle 16).
	ConflictNone RouteConflictKind = "none"
	// ConflictContained: one prefix strictly contains the other. Under
	// longest-prefix match the more-specific route always wins, so the
	// outcome is deterministic — informational, never blocking
	// (principle 17).
	ConflictContained RouteConflictKind = "contained"
	// ConflictIdentical: same prefix in two tunnels. No longest-prefix
	// winner exists — this is the true tie situation (principle 20) and
	// the only routing state that blocks by default.
	ConflictIdentical RouteConflictKind = "identical"
	// ConflictPartial: the ranges intersect without either containing the
	// other. Impossible for pure CIDR input but kept so non-CIDR ranges
	// (and future inputs) are classified instead of silently ignored.
	ConflictPartial RouteConflictKind = "partial"
	// ConflictFullTunnel: one prefix is 0.0.0.0/0 or ::/0 and covers the
	// other — reported specially as "full tunnel covers this route"
	// (principle 16).
	ConflictFullTunnel RouteConflictKind = "full_tunnel"
)

// Classify returns how two prefixes of the SAME family relate. Prefixes of
// different families are always ConflictNone: IPv4 and IPv6 are separate
// routing domains and can never contend for the same route (principle 15).
func Classify(a, b netip.Prefix) RouteConflictKind {
	if FamilyOf(a) != FamilyOf(b) {
		return ConflictNone
	}
	if a == b {
		return ConflictIdentical
	}
	aFull, bFull := IsFullTunnel(a), IsFullTunnel(b)
	switch {
	case aFull && bFull:
		return ConflictIdentical
	case aFull, bFull:
		// A full-tunnel prefix contains everything of its family.
		if aFull && a.Contains(b.Addr()) || bFull && b.Contains(a.Addr()) {
			return ConflictFullTunnel
		}
	case a.Contains(b.Addr()) && b.Bits() >= a.Bits():
		return ConflictContained
	case b.Contains(a.Addr()) && a.Bits() >= b.Bits():
		return ConflictContained
	}
	if rangesIntersect(a, b) {
		return ConflictPartial
	}
	return ConflictNone
}

// rangesIntersect reports whether two same-family prefixes share at least
// one address. Used only to detect the (CIDR-impossible) partial case.
func rangesIntersect(a, b netip.Prefix) bool {
	if FamilyOf(a) != FamilyOf(b) {
		return false
	}
	bits := a.Addr().BitLen()
	aStart, aEnd := rangeBounds(a, bits)
	bStart, bEnd := rangeBounds(b, bits)
	return bytes.Compare(aStart, bEnd) <= 0 && bytes.Compare(bStart, aEnd) <= 0
}

// rangeBounds returns the inclusive [start, end] address range of p as
// big-endian byte slices of width bits/8.
func rangeBounds(p netip.Prefix, bits int) ([]byte, []byte) {
	width := bits / 8
	start := p.Masked().Addr().AsSlice()
	end := make([]byte, width)
	copy(end, start)
	remaining := bits - p.Bits()
	for i := width - 1; i >= 0 && remaining > 0; i-- {
		take := remaining
		if take > 8 {
			take = 8
		}
		end[i] |= byte(0xff >> (8 - take))
		remaining -= take
	}
	return start, end
}

// Contains reports whether outer fully contains inner (same family).
func Contains(outer, inner netip.Prefix) bool {
	if FamilyOf(outer) != FamilyOf(inner) {
		return false
	}
	return outer.Bits() <= inner.Bits() && outer.Contains(inner.Addr())
}
