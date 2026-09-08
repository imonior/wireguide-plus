package diag

import (
	"fmt"
	"testing"
)

// Live-system verification for the TS451D_f3322 case (AllowedIPs
// 10.30.30.0/24 + 10.30.35.0/24; on this machine the tunnel runs on
// utun4 while a Clash TUN on utun5 holds the near-default split
// …/5,/4,/3…). Skips itself when the system is not in that shape.
func TestLiveReproTS451D(t *testing.T) {
	allowed := []string{"10.30.30.0/24", "10.30.35.0/24"}
	selfAddrs := []string{"10.30.35.2/32"}

	conflicts, err := CheckConflicts(allowed, selfAddrs)
	if err != nil {
		t.Fatalf("CheckConflicts: %v", err)
	}
	fmt.Printf("=== no-name-exclusion, selfAddr=%v: %d conflicts ===\n", selfAddrs, len(conflicts))
	for _, c := range conflicts {
		fmt.Printf("  iface=%s owner=%s overlaps=%v\n", c.InterfaceName, c.Owner, c.OverlappingIPs)
	}

	// Expected after the fix: zero conflicts. The tunnel's own utun is
	// skipped via its Address (10.30.35.2), and utun5's 8/5 route is a
	// strict superset of both AllowedIPs (more-specific wins under LPM).
	if len(conflicts) != 0 {
		t.Errorf("expected 0 conflicts after fix, got %d: %+v", len(conflicts), conflicts)
	}
}
