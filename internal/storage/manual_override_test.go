package storage

import "testing"

// TestManualLatches_MutuallyExclusive pins the core invariant of the manual
// override model: the off and on latches can never both hold the same tunnel.
// Setting one must clear the other, so the helper's reconcile rule
// (manual-off suppresses connect, manual-on suppresses disconnect) always has
// exactly one winner.
func TestManualLatches_MutuallyExclusive(t *testing.T) {
	s := &Settings{}

	if s.IsManualOff("a") || s.IsManualOn("a") {
		t.Fatal("fresh settings should hold no latch")
	}

	s.SetManualOff("a")
	if !s.IsManualOff("a") || s.IsManualOn("a") {
		t.Fatalf("after SetManualOff: off=%v on=%v, want off=true on=false", s.IsManualOff("a"), s.IsManualOn("a"))
	}

	// Switching to manual-on must release the off latch.
	s.SetManualOn("a")
	if s.IsManualOff("a") || !s.IsManualOn("a") {
		t.Fatalf("after SetManualOn: off=%v on=%v, want off=false on=true", s.IsManualOff("a"), s.IsManualOn("a"))
	}

	// And back again.
	s.SetManualOff("a")
	if !s.IsManualOff("a") || s.IsManualOn("a") {
		t.Fatalf("after SetManualOff again: off=%v on=%v, want off=true on=false", s.IsManualOff("a"), s.IsManualOn("a"))
	}

	// Idempotent: setting the same latch twice must not duplicate entries.
	s.SetManualOff("a")
	if n := len(s.ManualOffTunnels); n != 1 {
		t.Fatalf("duplicate manual-off entries: got %d, want 1", n)
	}
}

// TestManualLatches_ClearAndClearAll covers the release paths.
func TestManualLatches_ClearAndClearAll(t *testing.T) {
	s := &Settings{}
	s.SetManualOff("a")
	s.SetManualOn("b")

	s.ClearManualOff("a")
	if s.IsManualOff("a") {
		t.Error("ClearManualOff did not release a")
	}
	if !s.IsManualOn("b") {
		t.Error("ClearManualOff must not touch the on latch")
	}

	s.ClearManualOn("b")
	if s.IsManualOn("b") {
		t.Error("ClearManualOn did not release b")
	}

	// ClearAllManualOverrides releases both lists at once (startup path).
	s.SetManualOff("a")
	s.SetManualOn("b")
	s.ClearAllManualOverrides()
	if s.IsManualOff("a") || s.IsManualOn("b") {
		t.Fatalf("ClearAllManualOverrides left latches: off=%v on=%v", s.IsManualOff("a"), s.IsManualOn("b"))
	}
}

// TestManualLatches_RenameAndDelete checks the lifecycle hooks carry / drop
// the on latch just like the off latch, so a renamed or deleted tunnel never
// leaves a stale override behind (nor silently reattaches to a same-named
// tunnel created later).
func TestManualLatches_RenameAndDelete(t *testing.T) {
	s := &Settings{}
	s.SetManualOn("old")

	s.RenameTunnelRules("old", "new")
	if s.IsManualOn("old") {
		t.Error("manual-on latch should not survive a rename under the old name")
	}
	if !s.IsManualOn("new") {
		t.Error("manual-on latch should move to the new name")
	}

	s.DeleteTunnelRules("new")
	if s.IsManualOn("new") {
		t.Error("manual-on latch should be dropped on delete")
	}
}
