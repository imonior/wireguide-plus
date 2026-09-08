package diag

import "testing"

func TestFindOverlapsFullTunnel(t *testing.T) {
	overlaps := findOverlaps(
		[]string{"0.0.0.0/0"},
		[]string{"0.0.0.0/0"},
	)
	if len(overlaps) == 0 {
		t.Error("expected overlap for two full tunnels")
	}
}

func TestFindOverlapsSubnetContained(t *testing.T) {
	overlaps := findOverlaps(
		[]string{"10.0.0.0/16"},
		[]string{"10.0.5.0/24"},
	)
	if len(overlaps) == 0 {
		t.Error("expected overlap: /24 is inside /16")
	}
}

func TestFindOverlapsNoConflict(t *testing.T) {
	overlaps := findOverlaps(
		[]string{"10.0.0.0/24"},
		[]string{"192.168.0.0/24"},
	)
	if len(overlaps) != 0 {
		t.Errorf("expected no overlap, got %v", overlaps)
	}
}

// Regression: sibling prefixes of the same size were once reported as
// overlapping (each was judged to "contain" the other's address). Equal-
// width neighbours never intersect — 10.30.30.0/24 ends at 10.30.30.255,
// while 10.30.35.0/24 starts at 10.30.35.0.
func TestFindOverlapsSiblingPrefixes(t *testing.T) {
	overlaps := findOverlaps(
		[]string{"10.30.30.0/24"},
		[]string{"10.30.35.0/24"},
	)
	if len(overlaps) != 0 {
		t.Errorf("sibling /24s must not overlap, got %v", overlaps)
	}
}

// Adjacent (not overlapping) ranges: one ends where the next begins.
func TestFindOverlapsAdjacentPrefixes(t *testing.T) {
	overlaps := findOverlaps(
		[]string{"10.0.0.0/24"},
		[]string{"10.0.1.0/24"},
	)
	if len(overlaps) != 0 {
		t.Errorf("adjacent /24s must not overlap, got %v", overlaps)
	}
}

// Different families can never conflict.
func TestFindOverlapsCrossFamily(t *testing.T) {
	overlaps := findOverlaps(
		[]string{"0.0.0.0/0"},
		[]string{"::/0"},
	)
	if len(overlaps) != 0 {
		t.Errorf("IPv4 vs IPv6 must not overlap, got %v", overlaps)
	}
}

// A host route read back with host bits set must be masked first, or it
// masquerades as a wider range than it is.
func TestFindOverlapsHostBitsMasked(t *testing.T) {
	overlaps := findOverlaps(
		[]string{"10.30.35.7/24"},
		[]string{"10.30.30.0/24"},
	)
	if len(overlaps) != 0 {
		t.Errorf("10.30.35.7/24 is 10.30.35.0/24 after masking — must not overlap, got %v", overlaps)
	}
}

func TestFindOverlapsFullVsSubnet(t *testing.T) {
	overlaps := findOverlaps(
		[]string{"0.0.0.0/0"},
		[]string{"10.0.0.0/24"},
	)
	if len(overlaps) == 0 {
		t.Error("full tunnel should overlap with any subnet")
	}
}

// Regression (v1.7.9 user report): the tunnel's AllowedIPs sat strictly
// inside a Clash/Mihomo TUN near-default route (10.30.30.0/24 ⊂ 8.0.0.0/5),
// and the pre-connect dialog reported both of the tunnel's CIDRs as
// "conflicting". Longest-prefix-match means the tunnel's more-specific
// route always wins regardless of install order, so a strict subset of
// an existing route is NOT a conflict.
func TestFindOverlapsNewInsideExistingSuppressed(t *testing.T) {
	overlaps := findOverlaps(
		[]string{"10.30.30.0/24", "10.30.35.0/24"},
		[]string{"8.0.0.0/5"}, // covers 8.0.0.0–15.255.255.255 incl. all of 10/8
	)
	if len(overlaps) != 0 {
		t.Errorf("new CIDR strictly inside an existing route must not warn, got %v", overlaps)
	}
}

// Equal prefixes are last-writer-wins — genuine conflict, still reported.
func TestFindOverlapsEqualPrefixes(t *testing.T) {
	overlaps := findOverlaps(
		[]string{"10.30.30.0/24"},
		[]string{"10.30.30.0/24"},
	)
	if len(overlaps) == 0 {
		t.Error("equal prefixes must still be reported as a conflict")
	}
}

// New superset over an existing more-specific route is still reported —
// the classic "full tunnel vs Tailscale" warning.
func TestFindOverlapsNewSupersetStillReported(t *testing.T) {
	overlaps := findOverlaps(
		[]string{"0.0.0.0/0"},
		[]string{"100.64.0.0/10"},
	)
	if len(overlaps) == 0 {
		t.Error("new superset over an existing route must still warn")
	}
}

func TestNormalizeCIDR(t *testing.T) {
	if normalizeCIDR("10.0.0.1") != "10.0.0.1/32" {
		t.Error("should add /32 to bare IP")
	}
	if normalizeCIDR("10.0.0.0/24") != "10.0.0.0/24" {
		t.Error("should keep existing CIDR")
	}
}
