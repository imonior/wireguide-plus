package policy

import (
	"net/netip"
	"testing"
)

func mustPrefix(t *testing.T, s string) netip.Prefix {
	t.Helper()
	p, err := netip.ParsePrefix(s)
	if err != nil {
		t.Fatalf("bad test prefix %q: %v", s, err)
	}
	return p.Masked()
}

func TestParsePrefixesCanonical(t *testing.T) {
	got := ParsePrefixes([]string{"10.30.30.0/24", "10.30.30.0/24", "10.30.30.5", " 192.168.1.0/24 ", "fd00::/8", "not-a-cidr"})
	want := []string{"10.30.30.0/24", "10.30.30.5/32", "192.168.1.0/24", "fd00::/8"}
	if len(got) != len(want) {
		t.Fatalf("got %d prefixes %v, want %d", len(got), got, len(want))
	}
	for i, w := range want {
		if got[i].String() != w {
			t.Errorf("prefix[%d] = %s, want %s", i, got[i], w)
		}
	}
}

// The user-reported false positive: sibling /24s are disjoint and must never
// be reported as conflicting.
func TestClassifySiblingPrefixesAreDisjoint(t *testing.T) {
	a := mustPrefix(t, "10.30.30.0/24")
	b := mustPrefix(t, "10.30.35.0/24")
	if got := Classify(a, b); got != ConflictNone {
		t.Fatalf("sibling prefixes classified as %s, want none", got)
	}
}

func TestClassifyFamiliesAreIndependent(t *testing.T) {
	if got := Classify(mustPrefix(t, "0.0.0.0/0"), mustPrefix(t, "::/0")); got != ConflictNone {
		t.Fatalf("cross-family classify = %s, want none (IPv4/IPv6 are separate domains)", got)
	}
}

func TestClassifyKinds(t *testing.T) {
	cases := []struct {
		a, b string
		want RouteConflictKind
	}{
		{"10.10.0.0/16", "10.10.0.0/16", ConflictIdentical},
		{"10.0.0.0/8", "10.10.0.0/16", ConflictContained},
		{"10.10.0.0/16", "10.0.0.0/8", ConflictContained},
		{"0.0.0.0/0", "10.0.0.0/8", ConflictFullTunnel},
		{"::/0", "fd00::/8", ConflictFullTunnel},
		{"10.30.30.0/24", "10.30.35.0/24", ConflictNone},
		{"192.168.1.0/24", "10.0.0.0/8", ConflictNone},
	}
	for _, c := range cases {
		got := Classify(mustPrefix(t, c.a), mustPrefix(t, c.b))
		if got != c.want {
			t.Errorf("Classify(%s, %s) = %s, want %s", c.a, c.b, got, c.want)
		}
	}
}

// Only a tie blocks. Containment is deterministic under longest-prefix
// match and full tunnel is the maximum (not a conflict) scope.
func TestRouteSeverityIsDeterminismBased(t *testing.T) {
	tunnels := []TunnelView{
		{Name: "identicalA", AllowedIPs: []string{"10.10.0.0/16"}},
		{Name: "identicalB", AllowedIPs: []string{"10.10.0.0/16"}},
		{Name: "outer", AllowedIPs: []string{"10.0.0.0/8"}},
		{Name: "inner", AllowedIPs: []string{"10.10.0.0/16"}},
		{Name: "full", AllowedIPs: []string{"0.0.0.0/0"}},
	}
	report := Analyze(tunnels)
	for _, c := range report.Routes {
		switch c.Kind {
		case ConflictIdentical:
			if c.Severity != SeverityBlocking {
				t.Errorf("identical %s/%s severity = %s, want blocking", c.A, c.B, c.Severity)
			}
		case ConflictContained, ConflictFullTunnel:
			if c.Severity != SeverityInfo {
				t.Errorf("%s for %s/%s severity = %s, want info", c.Kind, c.A, c.B, c.Severity)
			}
		}
	}
	if !report.Blocking() {
		t.Error("report with an identical-prefix tie should be blocking")
	}
}

func TestSameTunnelPrefixesNeverConflict(t *testing.T) {
	report := Analyze([]TunnelView{
		{Name: "ts", AllowedIPs: []string{"10.30.30.0/24", "10.30.35.0/24"}},
	})
	if !report.Empty() {
		t.Fatalf("a single tunnel cannot conflict with itself, got %+v", report)
	}
}

func TestDNSConflicts(t *testing.T) {
	report := Analyze([]TunnelView{
		{Name: "a", Domains: []string{"corp.example.com"}},
		{Name: "b", Domains: []string{"CORP.example.com."}}, // normalises to the same claim
		{Name: "c", Domains: []string{"example.com"}},
		{Name: "d", Domains: []string{"internal.example.com"}},
		{Name: "e", UseAsDefaultDNS: true},
		{Name: "f", UseAsDefaultDNS: true},
	})
	if len(report.DNS) == 0 {
		t.Fatal("expected DNS conflicts")
	}
	kinds := map[DNSConflictKind]Severity{}
	for _, c := range report.DNS {
		kinds[c.Kind] = c.Severity
	}
	if kinds[DNSConflictDomainOverlap] != SeverityBlocking {
		t.Errorf("identical domain severity = %q, want blocking", kinds[DNSConflictDomainOverlap])
	}
	if kinds[DNSConflictDefaultDNS] != SeverityBlocking {
		t.Errorf("multiple default DNS severity = %q, want blocking", kinds[DNSConflictDefaultDNS])
	}
	if kinds[DNSConflictDomainContained] != SeverityInfo {
		t.Errorf("contained domain severity = %q, want info", kinds[DNSConflictDomainContained])
	}
}

func TestProtectionConflictsAreIndependentOfRouting(t *testing.T) {
	// Same nested ranges as the routing case, but protection claims are
	// what make this a policy conflict.
	withProtect := []TunnelView{
		{Name: "a", AllowedIPs: []string{"10.0.0.0/8"}, TrafficProtect: true},
		{Name: "b", AllowedIPs: []string{"10.10.0.0/16"}, TrafficProtect: true},
	}
	if got := Analyze(withProtect).Protection; len(got) != 1 || got[0].Kind != ProtectionConflictNested {
		t.Fatalf("expected one nested protection conflict, got %+v", got)
	}

	withoutProtect := []TunnelView{
		{Name: "a", AllowedIPs: []string{"10.0.0.0/8"}},
		{Name: "b", AllowedIPs: []string{"10.10.0.0/16"}},
	}
	if got := Analyze(withoutProtect).Protection; len(got) != 0 {
		t.Fatalf("no Traffic Protect means no protection claim, got %+v", got)
	}
}

// Principle 30: the analyzers are pure. Shuffling the input must not change
// the output, otherwise tunnel list order would silently become a priority.
func TestAnalyzeIsOrderIndependent(t *testing.T) {
	set := []TunnelView{
		{Name: "zeta", AllowedIPs: []string{"10.0.0.0/8"}, Domains: []string{"example.com"}, TrafficProtect: true},
		{Name: "alpha", AllowedIPs: []string{"10.10.0.0/16"}, Domains: []string{"internal.example.com"}, UseAsDefaultDNS: true},
		{Name: "mid", AllowedIPs: []string{"10.10.0.0/16"}, Domains: []string{"corp.example.com"}, UseAsDefaultDNS: true},
	}
	first := Analyze(set)
	reversed := Analyze([]TunnelView{set[2], set[1], set[0]})
	if len(first.Routes) != len(reversed.Routes) ||
		len(first.DNS) != len(reversed.DNS) ||
		len(first.Protection) != len(reversed.Protection) {
		t.Fatalf("order changed the result sizes: %+v vs %+v", first, reversed)
	}
	for i := range first.Routes {
		if first.Routes[i] != reversed.Routes[i] {
			t.Fatalf("route[%d] differs: %+v vs %+v", i, first.Routes[i], reversed.Routes[i])
		}
	}
	for i := range first.DNS {
		if first.DNS[i] != reversed.DNS[i] {
			t.Fatalf("dns[%d] differs: %+v vs %+v", i, first.DNS[i], reversed.DNS[i])
		}
	}
}

func TestForTunnelFiltersOtherTunnels(t *testing.T) {
	report := Analyze([]TunnelView{
		{Name: "mine", AllowedIPs: []string{"10.10.0.0/16"}},
		{Name: "other", AllowedIPs: []string{"10.10.0.0/16"}},
		{Name: "unrelated", AllowedIPs: []string{"192.168.0.0/16"}},
	})
	mine := report.ForTunnel("mine")
	if len(mine.Routes) != 1 {
		t.Fatalf("expected exactly one conflict involving mine, got %+v", mine.Routes)
	}
	if len(report.ForTunnel("unrelated").Routes) != 0 {
		t.Error("unrelated tunnel should have no conflicts")
	}
}

func TestDNSResolvePathAloneIsClean(t *testing.T) {
	report := Analyze([]TunnelView{
		{Name: "a", AllowedIPs: []string{"10.30.30.0/24"}, DNSResolvePath: true},
		{Name: "b", AllowedIPs: []string{"10.30.35.0/24"}},
	})
	if !report.Empty() {
		t.Fatalf("single DNSResolvePath claim should not conflict: %+v", report.DNS)
	}
}

// Two tunnels that both have the switch ON in their CONFIGURATION is a
// legal state — the user may run them at different times — so saving only
// warns. It becomes blocking only once both are actually up, at which point
// two connected tunnels are each claiming to be the single DNS path.
func TestDNSResolvePathMultipleWarnsUntilBothUp(t *testing.T) {
	idle := Analyze([]TunnelView{
		{Name: "a", DNSResolvePath: true},
		{Name: "b", DNSResolvePath: true},
	})
	if len(idle.DNS) != 1 {
		t.Fatalf("expected one DNS conflict, got %+v", idle.DNS)
	}
	if idle.DNS[0].Kind != DNSConflictResolvePath || idle.DNS[0].Severity != SeverityInfo {
		t.Fatalf("expected informational dns_resolve_path_multiple while idle, got %s/%s",
			idle.DNS[0].Kind, idle.DNS[0].Severity)
	}

	both := Analyze([]TunnelView{
		{Name: "a", DNSResolvePath: true, Active: true},
		{Name: "b", DNSResolvePath: true, Active: true},
	})
	if len(both.DNS) != 1 {
		t.Fatalf("expected one DNS conflict, got %+v", both.DNS)
	}
	if both.DNS[0].Severity != SeverityBlocking {
		t.Fatalf("expected blocking once both are up, got %s", both.DNS[0].Severity)
	}

	// One up + one configured but down is still only a warning: the
	// runtime decision is made by the helper at connect time.
	oneUp := Analyze([]TunnelView{
		{Name: "a", DNSResolvePath: true, Active: true},
		{Name: "b", DNSResolvePath: true},
	})
	if len(oneUp.DNS) != 1 || oneUp.DNS[0].Severity != SeverityInfo {
		t.Fatalf("expected informational while only one is up, got %+v", oneUp.DNS)
	}
}

func TestDNSResolvePathVsDefaultDNSIsBlocking(t *testing.T) {
	report := Analyze([]TunnelView{
		{Name: "path", DNSResolvePath: true, Active: true},
		{Name: "resolver", UseAsDefaultDNS: true},
	})
	if len(report.DNS) != 1 {
		t.Fatalf("expected one DNS conflict, got %+v", report.DNS)
	}
	c := report.DNS[0]
	if c.Kind != DNSConflictResolvePathVsDefault || c.Severity != SeverityBlocking {
		t.Fatalf("expected blocking dns_resolve_path_vs_default_dns, got %s/%s", c.Kind, c.Severity)
	}
	// The same tunnel holding BOTH roles is consistent, not a conflict.
	consistent := Analyze([]TunnelView{
		{Name: "both", DNSResolvePath: true, UseAsDefaultDNS: true},
	})
	if !consistent.Empty() {
		t.Fatalf("one tunnel holding both roles should be consistent: %+v", consistent.DNS)
	}
}
