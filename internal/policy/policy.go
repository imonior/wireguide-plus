// Package policy implements the WireGuide Plus policy layer: the pure,
// deterministic analyzers that decide whether tunnels' routing, DNS and
// traffic-protection policies can coexist.
//
// The package is deliberately side-effect free (principle 30): every entry
// point takes a snapshot of tunnel state and returns a Report. It never
// connects or disconnects a tunnel, never rewrites configuration, never
// touches system routes, never emits notifications and never shows UI.
// Those are the caller's job — see internal/app and internal/helper for the
// Conflict Policy layer that turns a Report into an allow/warn/block
// decision.
//
// Same input always yields the same Report: prefix sets are canonicalised
// and every list in the Report is deterministically ordered.
package policy

import "sort"

// TunnelView is a read-only snapshot of one tunnel's policy-relevant state.
// It is the ONLY input the analyzers accept — they never load files or query
// the helper, which is what keeps them pure and testable.
type TunnelView struct {
	// Name identifies the tunnel in reports.
	Name string

	// AllowedIPs are the raw [Peer] AllowedIPs values, exactly as stored
	// in the .conf. They are parsed into a canonical prefix set by the
	// analyzer (principle 13) — never compared as strings.
	AllowedIPs []string

	// TrafficProtect marks the tunnel's AllowedIPs as must-not-leak: the
	// traffic may only egress through this tunnel, and if the tunnel is
	// down it must not fall through to another egress (principle 8/9).
	// This is tunnel-scoped, NOT a global kill switch.
	TrafficProtect bool

	// Domains are the "domains through tunnel" DNS routing entries
	// (principle 10) and UseAsDefaultDNS the "use as default DNS" policy
	// (principle 11). Both are WireGuide Plus private policies — they
	// never enter the .conf (principle 2). DNSResolvePath is the "while
	// this tunnel is up, the system's DNS resolution may only egress
	// through it" policy (principle 33) — again a path constraint, not a
	// resolver-address choice.
	Domains         []string
	UseAsDefaultDNS bool
	DNSResolvePath  bool

	// Active reports whether the tunnel is currently up. Used to rank
	// conflicts (a conflict against a running tunnel matters more) and to
	// skip self-comparison noise.
	Active bool
}

// Severity expresses how strongly a conflict should influence the Conflict
// Policy layer (principle 22-25). It describes the conflict's determinism,
// NOT what to do — the caller maps severity to an action.
type Severity string

const (
	// SeverityInfo: the situation is deterministic and safe — the kernel
	// or resolver picks a winner unambiguously. Surface it for
	// awareness, never block anything.
	SeverityInfo Severity = "info"
	// SeverityWarning: real ambiguity that the user should know about,
	// but the resulting behaviour is still predictable enough that
	// blocking would be overbearing. Saving is always allowed; manual
	// connect proceeds after a confirmation.
	SeverityWarning Severity = "warning"
	// SeverityBlocking: a true tie or an unsatisfiable policy — two
	// tunnels claim the same routing/DNS space with no deterministic
	// winner. Manual connect is blocked pending "Connect Anyway";
	// automation refuses to connect and notifies instead of prompting
	// (principle 25).
	SeverityBlocking Severity = "blocking"
)

// Report is the complete output of the analyzers for a set of tunnels. Every
// slice is sorted deterministically so equal input yields byte-identical
// reports (principle 30).
type Report struct {
	Routes     []RouteConflict      `json:"routes"`
	DNS        []DNSConflict        `json:"dns"`
	Protection []ProtectionConflict `json:"protection"`
}

// Empty reports whether no conflict of any domain was found.
func (r Report) Empty() bool {
	return len(r.Routes) == 0 && len(r.DNS) == 0 && len(r.Protection) == 0
}

// ForTunnel filters the report down to conflicts involving one tunnel —
// what the UI shows in a tunnel's own conflict list and what the connect
// path evaluates.
func (r Report) ForTunnel(name string) Report {
	out := Report{}
	for _, c := range r.Routes {
		if c.A == name || c.B == name {
			out.Routes = append(out.Routes, c)
		}
	}
	for _, c := range r.DNS {
		if c.A == name || c.B == name {
			out.DNS = append(out.DNS, c)
		}
	}
	for _, c := range r.Protection {
		if c.A == name || c.B == name {
			out.Protection = append(out.Protection, c)
		}
	}
	return out
}

// Severity returns the highest severity present in the report.
func (r Report) Severity() Severity {
	worst := SeverityInfo
	for _, c := range r.Routes {
		if rank(c.Severity) > rank(worst) {
			worst = c.Severity
		}
	}
	for _, c := range r.DNS {
		if rank(c.Severity) > rank(worst) {
			worst = c.Severity
		}
	}
	for _, c := range r.Protection {
		if rank(c.Severity) > rank(worst) {
			worst = c.Severity
		}
	}
	return worst
}

// Blocking reports whether the report contains anything that should block a
// connect by default (principle 23/25).
func (r Report) Blocking() bool {
	return r.Severity() == SeverityBlocking
}

func rank(s Severity) int {
	switch s {
	case SeverityBlocking:
		return 3
	case SeverityWarning:
		return 2
	default:
		return 1
	}
}

// Analyze runs every analyzer over a tunnel set. It is the single entry
// point of the package: pure, deterministic, side-effect free.
//
// Tunnel order does not influence the result — it is never used as a
// priority tie-break (principle 18/19); conflicts where order would matter
// are reported as ties instead.
func Analyze(tunnels []TunnelView) Report {
	ordered := make([]TunnelView, len(tunnels))
	copy(ordered, tunnels)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Name < ordered[j].Name })

	return Report{
		Routes:     AnalyzeRoutes(ordered),
		DNS:        AnalyzeDNS(ordered),
		Protection: AnalyzeProtection(ordered),
	}
}
