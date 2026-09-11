package policy

import (
	"fmt"
	"net/netip"
	"sort"
)

// RouteConflict describes one overlapping prefix pair between two tunnels.
//
// Routing and DNS are separate concerns (principle 12) — this struct is
// about where IP traffic egresses, never about which resolver answers a
// name. Nothing here rewrites AllowedIPs (principle 26): the analyzer
// reports, the user decides.
type RouteConflict struct {
	// A and B are the two tunnel names, always in deterministic order.
	A string `json:"a"`
	B string `json:"b"`
	// APrefix / BPrefix are the canonical (masked) prefixes that overlap.
	APrefix string `json:"a_prefix"`
	BPrefix string `json:"b_prefix"`
	// Family is ipv4 or ipv6 — the two are analysed independently
	// (principle 15).
	Family Family `json:"family"`
	// Kind is the geometric relationship of the two prefixes.
	Kind RouteConflictKind `json:"kind"`
	// Severity is what the Conflict Policy layer acts on.
	Severity Severity `json:"severity"`
	// Message is a short English explanation for logs; the UI localises
	// from Kind instead of showing this verbatim.
	Message string `json:"message"`
}

// AnalyzeRoutes compares every prefix of every tunnel against every prefix
// of every other tunnel (principle 14). Two tunnels are compared once; IPv4
// and IPv6 are compared independently (principle 15).
//
// Deterministic by construction: tunnels and prefixes are iterated in sorted
// order and the result is sorted before returning, so tunnel list order and
// connection order never influence the output (principle 18/19).
func AnalyzeRoutes(tunnels []TunnelView) []RouteConflict {
	type prefixSet struct {
		tunnel string
		prefix netip.Prefix
	}
	sets := make([]prefixSet, 0, len(tunnels)*2)
	for _, t := range tunnels {
		for _, p := range ParsePrefixes(t.AllowedIPs) {
			sets = append(sets, prefixSet{tunnel: t.Name, prefix: p})
		}
	}

	var out []RouteConflict
	for i := 0; i < len(sets); i++ {
		for j := i + 1; j < len(sets); j++ {
			a, b := sets[i], sets[j]
			if a.tunnel == b.tunnel {
				// Prefixes inside one tunnel share an egress and can
				// never conflict with each other — they are routes of
				// the same interface.
				continue
			}
			kind := Classify(a.prefix, b.prefix)
			if kind == ConflictNone {
				continue
			}
			c := RouteConflict{
				A:       a.tunnel,
				B:       b.tunnel,
				APrefix: a.prefix.String(),
				BPrefix: b.prefix.String(),
				Family:  FamilyOf(a.prefix),
				Kind:    kind,
			}
			c.Severity, c.Message = routeSeverity(kind, c.A, c.B, a.prefix, b.prefix)
			out = append(out, c)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].A != out[j].A {
			return out[i].A < out[j].A
		}
		if out[i].B != out[j].B {
			return out[i].B < out[j].B
		}
		if out[i].APrefix != out[j].APrefix {
			return out[i].APrefix < out[j].APrefix
		}
		return out[i].BPrefix < out[j].BPrefix
	})
	return out
}

// routeSeverity maps a geometric relationship to severity.
//
// The guiding rule is determinism (principle 17): longest-prefix match gives
// a single unambiguous winner for containment, so those conflicts are
// informational. Only a tie — identical prefixes, or a partial overlap with
// no containing relationship — has no deterministic winner and therefore
// blocks by default (principle 20).
func routeSeverity(kind RouteConflictKind, a, b string, pa, pb netip.Prefix) (Severity, string) {
	switch kind {
	case ConflictIdentical:
		return SeverityBlocking, fmt.Sprintf(
			"identical prefix %s claimed by %s and %s — longest-prefix match cannot break this tie",
			pa, a, b)
	case ConflictPartial:
		return SeverityBlocking, fmt.Sprintf(
			"%s (%s) partially overlaps %s (%s) with no containing prefix",
			a, pa, b, pb)
	case ConflictFullTunnel:
		full, other := pa, pb
		fullName, otherName := a, b
		if IsFullTunnel(pb) {
			full, other, fullName, otherName = pb, pa, b, a
		}
		return SeverityInfo, fmt.Sprintf(
			"full-tunnel %s (%s) covers %s (%s); the more specific route still wins inside the tunnel",
			fullName, full, otherName, other)
	case ConflictContained:
		// The more specific (higher /bits) prefix wins under LPM.
		narrowP, wideP := pa, pb
		narrowN, wideN := a, b
		if pb.Bits() > pa.Bits() {
			narrowP, wideP, narrowN, wideN = pb, pa, b, a
		}
		return SeverityInfo, fmt.Sprintf(
			"%s (%s) is inside %s (%s) — longest-prefix match prefers the more specific route",
			narrowN, narrowP, wideN, wideP)
	default:
		return SeverityInfo, ""
	}
}
