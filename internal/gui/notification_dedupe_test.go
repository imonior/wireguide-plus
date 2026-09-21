package gui

import "testing"

// TestDecideNotification_EdgeTriggered covers the reporting contract that
// regressed into a notification loop: a situation is announced once, and
// churn that settles back to the same situation must NOT re-announce it.
//
// The real-world case this locks down: a tunnel whose automation connect
// fails every poll cycle cycles in and out of the active set, which flipped
// trayManager.setIconState's activeChanged gate every cycle and re-popped the
// same "TS453Dmini is connected" bubble roughly every 30s.
func TestDecideNotification_EdgeTriggered(t *testing.T) {
	const connected = "2|TS453Dmini|"

	// First report of a situation: show it.
	d := decideNotification(connected, "", "")
	if !d.Show {
		t.Fatal("first report of a situation must be shown")
	}
	if d.NotifiedSig != connected {
		t.Fatalf("NotifiedSig = %q, want %q", d.NotifiedSig, connected)
	}
	if d.FirstSuppression {
		t.Fatal("a shown notification is not a suppression")
	}

	// Same situation again, unchanged world: suppress, and flag it so the
	// caller logs the reason once.
	d = decideNotification(connected, d.NotifiedSig, d.SuppressedSig)
	if d.Show {
		t.Fatal("an identical situation must not be re-announced")
	}
	if !d.FirstSuppression {
		t.Fatal("first suppression of a situation must be flagged for logging")
	}
	if d.SuppressedSig != connected {
		t.Fatalf("SuppressedSig = %q, want %q", d.SuppressedSig, connected)
	}

	// And again, and again: still suppressed, but no longer "first" — so a
	// 30s loop produces one log line, not one per cycle.
	d = decideNotification(connected, d.NotifiedSig, d.SuppressedSig)
	if d.Show || d.FirstSuppression {
		t.Fatal("repeats beyond the first must stay silent and stop logging at Info")
	}
}

// TestDecideNotification_ChangeIsAlwaysReported proves the gate is
// edge-triggered, not a permanent muzzle: any change (connected set,
// disconnection, probe targets) reports again — including a return to a
// situation that was announced earlier in the session.
func TestDecideNotification_ChangeIsAlwaysReported(t *testing.T) {
	const (
		connected    = "2|TS453Dmini|"
		bothTunnels  = "2|TS451D,TS453Dmini|"
		disconnected = "0||"
	)

	// connected → both → connected: the third state equals the FIRST one and
	// must still be shown, because it differs from the last report.
	last, suppressed := "", ""
	for _, tc := range []struct {
		sig  string
		want bool
	}{
		{connected, true},
		{connected, false},
		{bothTunnels, true},
		{connected, true},
		{disconnected, true},
		{disconnected, false},
	} {
		d := decideNotification(tc.sig, last, suppressed)
		if d.Show != tc.want {
			t.Fatalf("decideNotification(%q) Show = %v, want %v", tc.sig, d.Show, tc.want)
		}
		last, suppressed = d.NotifiedSig, d.SuppressedSig
	}
}

// TestNotificationSignature ensures the signature distinguishes everything a
// user can read off the bubble — the state, the tunnel names and the
// out-of-range probe targets — and that it is stable under reordering, so a
// reshuffled list is not mistaken for a change (which would re-open the spam
// loop through the back door).
func TestNotificationSignature(t *testing.T) {
	base := notificationSignature(popupStateConnected, []string{"A", "B"}, []string{"8.8.8.8", "1.1.1.1"})

	// Order-insensitive on out-of-range (names arrive pre-sorted by contract).
	if got := notificationSignature(popupStateConnected, []string{"A", "B"}, []string{"1.1.1.1", "8.8.8.8"}); got != base {
		t.Errorf("signature changed when out-of-range order changed:\n got %q\nwant %q", got, base)
	}
	// Sensitive to state.
	if got := notificationSignature(popupStateConnecting, []string{"A", "B"}, []string{"8.8.8.8", "1.1.1.1"}); got == base {
		t.Error("connecting and connected must not share a signature")
	}
	// Sensitive to the tunnel set.
	if got := notificationSignature(popupStateConnected, []string{"A"}, []string{"8.8.8.8", "1.1.1.1"}); got == base {
		t.Error("a different tunnel set must not share a signature")
	}
	// Sensitive to the probe-target warning.
	if got := notificationSignature(popupStateConnected, []string{"A", "B"}, nil); got == base {
		t.Error("dropping an out-of-range warning must change the signature")
	}
}
