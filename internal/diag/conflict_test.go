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

func TestNormalizeCIDR(t *testing.T) {
	if normalizeCIDR("10.0.0.1") != "10.0.0.1/32" {
		t.Error("should add /32 to bare IP")
	}
	if normalizeCIDR("10.0.0.0/24") != "10.0.0.0/24" {
		t.Error("should keep existing CIDR")
	}
}
