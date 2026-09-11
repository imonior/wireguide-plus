package policy

import (
	"fmt"
	"sort"
	"strings"
)

// DNSConflictKind enumerates DNS policy conflicts.
//
// DNS routing and IP routing are separate concerns (principle 12): a DNS
// server address does not decide which tunnel carries the query's traffic —
// the route table does. So these conflicts live in their own analyzer and
// are never merged into the route report (principle 28).
type DNSConflictKind string

const (
	// DNSConflictDomainOverlap: two tunnels claim the exact same domain.
	// Nothing decides which resolver answers — a true tie.
	DNSConflictDomainOverlap DNSConflictKind = "domain_overlap"
	// DNSConflictDomainContained: one claimed domain is a subdomain of
	// another (example.com vs internal.example.com). The more specific
	// entry wins, mirroring longest-prefix match for routes, so this is
	// informational.
	DNSConflictDomainContained DNSConflictKind = "domain_contained"
	// DNSConflictDefaultDNS: more than one tunnel is set as the default
	// DNS resolver (principle 29). Only one resolver can be "the"
	// default, so this needs explicit detection.
	DNSConflictDefaultDNS DNSConflictKind = "default_dns_multiple"
	// DNSConflictResolvePath: more than one tunnel claims the system's DNS
	// resolve path (principle 33) — "every DNS query must egress through
	// me". Only one CONNECTED tunnel can carry it, so two claims are
	// informational while at most one of them is up (the user may run them
	// at different times) and blocking once both are up.
	DNSConflictResolvePath DNSConflictKind = "dns_resolve_path_multiple"
	// DNSConflictResolvePathVsDefault: one tunnel claims the DNS resolve
	// PATH while another claims the system RESOLVER address, so the two
	// disagree about where DNS goes. Blocking as soon as either is up.
	DNSConflictResolvePathVsDefault DNSConflictKind = "dns_resolve_path_vs_default_dns"
)

// DNSConflict describes one DNS policy conflict between two tunnels.
type DNSConflict struct {
	A        string          `json:"a"`
	B        string          `json:"b"`
	Domain   string          `json:"domain,omitempty"`
	Other    string          `json:"other,omitempty"`
	Kind     DNSConflictKind `json:"kind"`
	Severity Severity        `json:"severity"`
	Message  string          `json:"message"`
}

// AnalyzeDNS finds DNS policy conflicts across tunnels (principles 10/11/28/29).
// Pure and deterministic: output is sorted by kind, then tunnel names.
func AnalyzeDNS(tunnels []TunnelView) []DNSConflict {
	var out []DNSConflict

	// ①/② Domain overlap and containment.
	type claim struct {
		tunnel string
		domain string
	}
	var claims []claim
	for _, t := range tunnels {
		for _, d := range t.Domains {
			if norm := normalizeDomain(d); norm != "" {
				claims = append(claims, claim{tunnel: t.Name, domain: norm})
			}
		}
	}
	for i := 0; i < len(claims); i++ {
		for j := i + 1; j < len(claims); j++ {
			a, b := claims[i], claims[j]
			if a.tunnel == b.tunnel {
				continue
			}
			switch {
			case a.domain == b.domain:
				out = append(out, DNSConflict{
					A: a.tunnel, B: b.tunnel, Domain: a.domain,
					Kind: DNSConflictDomainOverlap, Severity: SeverityBlocking,
					Message: fmt.Sprintf("both %s and %s route %s — no rule decides which resolver answers",
						a.tunnel, b.tunnel, a.domain),
				})
			case domainContains(a.domain, b.domain):
				out = append(out, DNSConflict{
					A: a.tunnel, B: b.tunnel, Domain: b.domain, Other: a.domain,
					Kind: DNSConflictDomainContained, Severity: SeverityInfo,
					Message: fmt.Sprintf("%s (%s) is more specific than %s (%s) — the specific entry wins",
						b.tunnel, b.domain, a.tunnel, a.domain),
				})
			case domainContains(b.domain, a.domain):
				out = append(out, DNSConflict{
					A: a.tunnel, B: b.tunnel, Domain: a.domain, Other: b.domain,
					Kind: DNSConflictDomainContained, Severity: SeverityInfo,
					Message: fmt.Sprintf("%s (%s) is more specific than %s (%s) — the specific entry wins",
						a.tunnel, a.domain, b.tunnel, b.domain),
				})
			}
		}
	}

	// ③ Multiple Default DNS selections.
	var defaults []string
	for _, t := range tunnels {
		if t.UseAsDefaultDNS {
			defaults = append(defaults, t.Name)
		}
	}
	if len(defaults) > 1 {
		sort.Strings(defaults)
		for i := 0; i < len(defaults); i++ {
			for j := i + 1; j < len(defaults); j++ {
				out = append(out, DNSConflict{
					A: defaults[i], B: defaults[j],
					Kind: DNSConflictDefaultDNS, Severity: SeverityBlocking,
					Message: fmt.Sprintf("%s and %s are both set as the default DNS resolver — only one can be",
						defaults[i], defaults[j]),
				})
			}
		}
	}

	// ④ Multiple DNS resolve path claims (principle 33) and ⑤ resolve path
	// vs Default DNS: the path claim and the resolver claim disagree.
	//
	// Severity here is state-dependent on purpose. Two tunnels that both
	// have the switch ON in their configuration is a LEGAL state — the
	// user may well run them at different times — so saving only warns
	// (principle 22). It becomes a real conflict only once both are up,
	// because then two connected tunnels are each claiming to be the one
	// path the system's DNS must take. The runtime half — a tunnel asking
	// for the path while another connected tunnel already owns it — is
	// enforced by the helper at connect time; this analyzer only ever sees
	// configuration, never a connect request.
	active := make(map[string]bool, len(tunnels))
	var pathClaims []string
	for _, t := range tunnels {
		active[t.Name] = t.Active
		if t.DNSResolvePath {
			pathClaims = append(pathClaims, t.Name)
		}
	}
	sort.Strings(pathClaims)
	for i := 0; i < len(pathClaims); i++ {
		for j := i + 1; j < len(pathClaims); j++ {
			severity := SeverityInfo
			if active[pathClaims[i]] && active[pathClaims[j]] {
				severity = SeverityBlocking
			}
			out = append(out, DNSConflict{
				A: pathClaims[i], B: pathClaims[j],
				Kind: DNSConflictResolvePath, Severity: severity,
				Message: fmt.Sprintf("%s and %s both claim the system's DNS resolve path — only one connected tunnel can carry it",
					pathClaims[i], pathClaims[j]),
			})
		}
	}
	for _, s := range pathClaims {
		for _, d := range defaults {
			if s == d {
				continue
			}
			severity := SeverityInfo
			if active[s] || active[d] {
				severity = SeverityBlocking
			}
			out = append(out, DNSConflict{
				A: s, B: d,
				Kind: DNSConflictResolvePathVsDefault, Severity: severity,
				Message: fmt.Sprintf("%s claims the DNS resolve path while %s claims the system resolver — the two roles disagree about where DNS goes",
					s, d),
			})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].A != out[j].A {
			return out[i].A < out[j].A
		}
		return out[i].B < out[j].B
	})
	return out
}

// normalizeDomain lowercases and strips the trailing root dot and any
// leading wildcard label, so "Example.com." and "*.example.com" compare
// consistently. Empty result means "not a usable claim".
func normalizeDomain(d string) string {
	d = strings.ToLower(strings.TrimSpace(d))
	d = strings.TrimSuffix(d, ".")
	d = strings.TrimPrefix(d, "*.")
	return d
}

// domainContains reports whether outer is a parent domain of inner
// (example.com contains internal.example.com).
func domainContains(outer, inner string) bool {
	if outer == inner {
		return false
	}
	return strings.HasSuffix(inner, "."+outer)
}
