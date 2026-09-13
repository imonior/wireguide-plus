package diag

import "testing"

func TestApplyOwners(t *testing.T) {
	owners := map[string]string{
		"utun3": "FlClash",
		"utun4": "WireGuard",
	}
	entries := []RouteEntry{
		{Interface: "utun3", InterfaceDetail: ""},         // filled
		{Interface: "utun4", InterfaceDetail: ""},         // filled
		{Interface: "utun5", InterfaceDetail: ""},         // no owner -> untouched
		{Interface: "en0", InterfaceDetail: "Wi-Fi"},     // existing detail preserved
		{Interface: "lo0", InterfaceDetail: "Tailscale"},  // existing detail preserved even if owner known
	}
	got := applyOwners(entries, owners)
	want := []struct {
		iface, detail string
	}{
		{"utun3", "FlClash"},
		{"utun4", "WireGuard"},
		{"utun5", ""},
		{"en0", "Wi-Fi"},
		{"lo0", "Tailscale"},
	}
	if len(got) != len(want) {
		t.Fatalf("entry count changed: got %d want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Interface != w.iface || got[i].InterfaceDetail != w.detail {
			t.Errorf("entry %d: got %+v want iface=%s detail=%s", i, got[i], w.iface, w.detail)
		}
	}
}

func TestApplyOwnersEmptyMap(t *testing.T) {
	entries := []RouteEntry{{Interface: "utun3", InterfaceDetail: ""}}
	got := applyOwners(entries, nil)
	if got[0].InterfaceDetail != "" {
		t.Errorf("nil owner map must not modify entries, got detail=%q", got[0].InterfaceDetail)
	}
}

// TestDetectInterfaceOwnersSmoke ensures the platform detection runs without
// panicking and returns a usable map (may legitimately be empty when no
// third-party tunnel is active).
func TestDetectInterfaceOwnersSmoke(t *testing.T) {
	owners := detectInterfaceOwners()
	_ = owners // best-effort; success is "did not panic"
}
