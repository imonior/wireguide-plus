package diag

import (
	"fmt"
	"net"
	"strings"
	"testing"
)

// Live-system verification for the TS451D_f3322 case (AllowedIPs
// 10.30.30.0/24 + 10.30.35.0/24). The whole point is to prove that a
// tunnel's OWN running interface is skipped — via its Address — so its
// own routes are never reported back as "conflicting" against itself.
//
// This is a live repro: it only runs where such a tunnel is actually up.
// Previously the selfAddr was hard-coded to 10.30.35.2/32, which only
// matched the author's original host; on any other machine the address
// mismatch let the tunnel's own interface through and the test failed
// even though the conflict code was correct. We now discover the real
// tunnel address from the live interface, so the self-skip is exercised
// end-to-end, and skip (not fail) when the system isn't in that shape.
func TestLiveReproTS451D(t *testing.T) {
	allowed := []string{"10.30.30.0/24", "10.30.35.0/24"}

	tunnelAddr, ok := findLiveTunnelAddress(allowed)
	if !ok {
		t.Skipf("system not in TS451D shape: no WireGuide interface carrying %v", allowed)
	}
	selfAddrs := []string{tunnelAddr}

	conflicts, err := CheckConflicts(allowed, selfAddrs)
	if err != nil {
		t.Fatalf("CheckConflicts: %v", err)
	}
	fmt.Printf("=== selfAddr=%v: %d conflicts ===\n", selfAddrs, len(conflicts))
	for _, c := range conflicts {
		fmt.Printf("  iface=%s owner=%s overlaps=%v\n", c.InterfaceName, c.Owner, c.OverlappingIPs)
	}

	// Expected: zero conflicts. The tunnel's own interface is skipped by
	// its Address, so its routes can never match themselves.
	if len(conflicts) != 0 {
		t.Errorf("expected 0 conflicts after fix, got %d: %+v", len(conflicts), conflicts)
	}
}

// findLiveTunnelAddress locates a running WireGuide interface whose routes
// cover all of the given AllowedIPs and returns its first IPv4 address —
// the address the tunnel would pass as its own selfAddr for the self-skip.
// Returns ok=false when no such interface exists (test should skip).
func findLiveTunnelAddress(allowed []string) (string, bool) {
	ifaces, err := scanWireGuardInterfaces(nil)
	if err != nil {
		return "", false
	}
	for _, iface := range ifaces {
		if iface.Owner != "WireGuide" {
			continue
		}
		if !routesCover(iface.Routes, allowed) {
			continue
		}
		if ip := firstIPv4(iface.Name); ip != "" {
			return ip, true
		}
	}
	return "", false
}

// routesCover reports whether have (an interface's routes) covers every
// CIDR in want (the tunnel's AllowedIPs). Extra routes on the interface
// are allowed; we only require the AllowedIPs to be present.
func routesCover(have, want []string) bool {
	set := make(map[string]bool, len(have))
	for _, r := range have {
		set[normalizeCIDR(r)] = true
	}
	for _, w := range want {
		if !set[normalizeCIDR(w)] {
			return false
		}
	}
	return true
}

// firstIPv4 returns the first IPv4 unicast address of the named interface,
// mask stripped. Empty when the interface has no IPv4 address.
func firstIPv4(name string) string {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return ""
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		ipStr := a.String()
		if i := strings.Index(ipStr, "/"); i >= 0 {
			ipStr = ipStr[:i]
		}
		if net.ParseIP(ipStr).To4() != nil {
			return ipStr
		}
	}
	return ""
}
