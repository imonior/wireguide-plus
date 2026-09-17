package storage

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestProbeTargets_LegacyMigration pins the upgrade path: a sidecar written
// before the four-row editor stored one optional target, and that value must
// land in slot 2 — the first free-form slot, which is the role it had
// ("an extra host pinned alongside the built-ins"). Slot 0/1 are public-probe
// overrides, so mapping it there would silently reinterpret the user's
// setting as something else.
func TestProbeTargets_LegacyMigration(t *testing.T) {
	m := &TunnelMeta{LatencyProbeTarget: "nas.example.com"}

	got := m.ProbeTargets()
	want := []string{"", "", "nas.example.com", ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy migration: got %q, want %q", got, want)
	}

	// A stored four-slot list wins: the legacy field is only a fallback and
	// must not overwrite a value the user has since edited.
	m2 := &TunnelMeta{
		LatencyProbeTarget:  "old.example.com",
		LatencyProbeTargets: []string{"1.1.1.1", "", "new.example.com", ""},
	}
	got2 := m2.ProbeTargets()
	want2 := []string{"1.1.1.1", "", "new.example.com", ""}
	if !reflect.DeepEqual(got2, want2) {
		t.Fatalf("stored slots must win: got %q, want %q", got2, want2)
	}
}

// TestProbeTargets_NormalizesToFour pins the shape contract the GUI handler
// and the frontend both rely on: indexable without bounds checks, whitespace
// stripped, and a nil receiver safe to call (ListTunnels calls it on a nil
// meta for a tunnel without a sidecar).
func TestProbeTargets_NormalizesToFour(t *testing.T) {
	if got := (*TunnelMeta)(nil).ProbeTargets(); !reflect.DeepEqual(got, []string{"", "", "", ""}) {
		t.Fatalf("nil meta: got %q, want four empty slots", got)
	}

	m := &TunnelMeta{LatencyProbeTargets: []string{"  8.8.8.8  ", "223.5.5.5"}}
	got := m.ProbeTargets()
	want := []string{"8.8.8.8", "223.5.5.5", "", ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	if len(got) != ProbeTargetSlotCount {
		t.Fatalf("len = %d, want %d", len(got), ProbeTargetSlotCount)
	}

	// More entries than slots are truncated rather than panicking.
	m2 := &TunnelMeta{LatencyProbeTargets: []string{"a", "b", "c", "d", "e"}}
	if got := m2.ProbeTargets(); !reflect.DeepEqual(got, []string{"a", "b", "c", "d"}) {
		t.Fatalf("overflow: got %q", got)
	}
}

// TestSetProbeTargets_MirrorsSlotTwo pins the downgrade contract: an older
// reader only knows the single-target field, so slot 2 is mirrored into it.
// Without this, editing the rows and then running a previous build would show
// an empty box — the setting would look lost.
func TestSetProbeTargets_MirrorsSlotTwo(t *testing.T) {
	m := &TunnelMeta{}
	m.SetProbeTargets([]string{"8.8.8.8", "", "nas.local", ""})

	if m.LatencyProbeTarget != "nas.local" {
		t.Fatalf("legacy mirror = %q, want nas.local", m.LatencyProbeTarget)
	}
	// Round-trips: what was written is what is read back.
	if got, want := m.ProbeTargets(), []string{"8.8.8.8", "", "nas.local", ""}; !reflect.DeepEqual(got, want) {
		t.Fatalf("round-trip: got %q, want %q", got, want)
	}

	// Slots are always written out in full, even when all are empty: the
	// JSON tag is omitempty, so a nil slice would vanish and the frontend's
	// positional read would fall back to the legacy field instead.
	m2 := &TunnelMeta{}
	m2.SetProbeTargets(nil)
	if m2.LatencyProbeTargets == nil || len(m2.LatencyProbeTargets) != ProbeTargetSlotCount {
		t.Fatalf("empty list must still serialise %d slots, got %#v", ProbeTargetSlotCount, m2.LatencyProbeTargets)
	}
	blob, err := json.Marshal(m2)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(blob, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := raw["latency_probe_targets"]; !ok {
		t.Fatalf("latency_probe_targets missing from %s", blob)
	}
}
