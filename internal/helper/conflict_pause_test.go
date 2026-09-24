package helper

import (
	"testing"
)

func TestPauseForAddressConflictTransitions(t *testing.T) {
	h := &Helper{}

	if !h.pauseForAddressConflict("TS453Dmini", "10.20.25.5/32", "TS453Dmini_hpg168.f3322.net", "down") {
		t.Fatalf("first detection should produce a new pause")
	}
	if h.pauseForAddressConflict("TS453Dmini", "10.20.25.5/32", "TS453Dmini_hpg168.f3322.net", "down") {
		t.Fatalf("re-detecting the same pending conflict must not re-pause/re-broadcast")
	}
	if !h.isAutoConnectPaused("TS453Dmini") {
		t.Fatalf("tunnel should read as paused")
	}
	if h.isAutoConnectPaused("TS451D") {
		t.Fatalf("unrelated tunnel must not be affected")
	}
	if !h.automationBlockedForReconnect("TS453Dmini") {
		t.Fatalf("paused tunnels must be blocked for reconnect")
	}

	// The pull must expose what the dialog never answered…
	pending := h.pendingAddressConflicts()
	if len(pending) != 1 || pending[0].Tunnel != "TS453Dmini" {
		t.Fatalf("pending conflicts = %+v", pending)
	}
	if pending[0].Address != "10.20.25.5" {
		t.Errorf("address should be stripped to the bare IP, got %q", pending[0].Address)
	}

	// …and the resume answer must clear both the pause and its block.
	if !h.resumeAutoConnect("TS453Dmini") {
		t.Fatalf("resume should report clearing a real pause")
	}
	if h.resumeAutoConnect("TS453Dmini") {
		t.Fatalf("second resume has nothing to clear")
	}
	if h.isAutoConnectPaused("TS453Dmini") {
		t.Fatalf("resume must lift the pause")
	}
	if len(h.pendingAddressConflicts()) != 0 {
		t.Fatalf("resolved conflicts must not be pulled")
	}

	// After resuming, a fresh detection is a fresh pause (dialog re-raised).
	if !h.pauseForAddressConflict("TS453Dmini", "10.20.25.5/32", "TS453Dmini_hpg168.f3322.net", "down") {
		t.Fatalf("re-detection after resume must pause again")
	}
}

func TestStripCIDRFallback(t *testing.T) {
	if got := stripCIDR("10.20.25.5/32"); got != "10.20.25.5" {
		t.Errorf("stripCIDR(v4) = %q", got)
	}
	if got := stripCIDR("not-a-cidr"); got != "not-a-cidr" {
		t.Errorf("stripCIDR(garbage) should fall back to the raw string, got %q", got)
	}
}
