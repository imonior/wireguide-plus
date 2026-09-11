package policy

import (
	"fmt"
	"net/netip"
	"sort"
)

// ProtectionConflictKind enumerates traffic-protection policy conflicts.
//
// Protection is deliberately analysed separately from routing (principle 27).
// Two tunnels whose routes are perfectly resolvable by longest-prefix match
// can still make contradictory protection claims:
//
//	A: AllowedIPs 10.0.0.0/8,  Traffic Protect = ON
//	B: AllowedIPs 10.10.0.0/16, Traffic Protect = ON
//
// Routing is deterministic here (B wins inside 10.10/16), but the policy
// layer still has two tunnels claiming that 10.10/16 must not leak through
// any other egress — an unsatisfiable pair of promises if both are up.
type ProtectionConflictKind string

const (
	// ProtectionConflictIdentical: two protected tunnels claim the exact
	// same range — the protection promise cannot be kept for both.
	ProtectionConflictIdentical ProtectionConflictKind = "protection_identical"
	// ProtectionConflictNested: one protected range sits inside another
	// protected range. Routable, but the outer tunnel's promise to carry
	// that traffic conflicts with the inner one's.
	ProtectionConflictNested ProtectionConflictKind = "protection_nested"
)

// ProtectionConflict describes one traffic-protection policy conflict.
type ProtectionConflict struct {
	A        string                 `json:"a"`
	B        string                 `json:"b"`
	APrefix  string                 `json:"a_prefix"`
	BPrefix  string                 `json:"b_prefix"`
	Family   Family                 `json:"family"`
	Kind     ProtectionConflictKind `json:"kind"`
	Severity Severity               `json:"severity"`
	Message  string                 `json:"message"`
}

// AnalyzeProtection finds traffic-protection conflicts between tunnels whose
// Traffic Protect is enabled (principles 8/9/27). Tunnels without Traffic
// Protect make no protection promise and are excluded entirely — their
// overlapping routes are a routing matter handled by AnalyzeRoutes.
func AnalyzeProtection(tunnels []TunnelView) []ProtectionConflict {
	type claim struct {
		tunnel string
		prefix netip.Prefix
	}
	var claims []claim
	for _, t := range tunnels {
		if !t.TrafficProtect {
			continue
		}
		for _, p := range ParsePrefixes(t.AllowedIPs) {
			claims = append(claims, claim{tunnel: t.Name, prefix: p})
		}
	}

	var out []ProtectionConflict
	for i := 0; i < len(claims); i++ {
		for j := i + 1; j < len(claims); j++ {
			a, b := claims[i], claims[j]
			if a.tunnel == b.tunnel {
				continue
			}
			kind := Classify(a.prefix, b.prefix)
			var pc ProtectionConflict
			switch kind {
			case ConflictIdentical:
				pc = ProtectionConflict{
					Kind: ProtectionConflictIdentical, Severity: SeverityBlocking,
					Message: fmt.Sprintf("%s and %s both promise to protect %s — only one can carry it",
						a.tunnel, b.tunnel, a.prefix),
				}
			case ConflictContained, ConflictFullTunnel:
				pc = ProtectionConflict{
					Kind: ProtectionConflictNested, Severity: SeverityWarning,
					Message: fmt.Sprintf("%s and %s both protect overlapping ranges (%s / %s); routing is unambiguous but protection is claimed twice",
						a.tunnel, b.tunnel, a.prefix, b.prefix),
				}
			default:
				// Disjoint (or partial) — no protection overlap.
				continue
			}
			pc.A, pc.B = a.tunnel, b.tunnel
			pc.APrefix, pc.BPrefix = a.prefix.String(), b.prefix.String()
			pc.Family = FamilyOf(a.prefix)
			out = append(out, pc)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].A != out[j].A {
			return out[i].A < out[j].A
		}
		if out[i].B != out[j].B {
			return out[i].B < out[j].B
		}
		return out[i].APrefix < out[j].APrefix
	})
	return out
}
