package gui

import "testing"

// keySet builds a bool map from a string slice (order irrelevant).
func keySet(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// connSet builds a tunnelConnState map from name→state pairs.
func connSet(pairs ...interface{}) map[string]tunnelConnState {
	m := make(map[string]tunnelConnState)
	for i := 0; i+1 < len(pairs); i += 2 {
		n := pairs[i].(string)
		s := pairs[i+1].(tunnelConnState)
		m[n] = s
	}
	return m
}

func assertTransition(t *testing.T, tr statusTransition, ups, downs, trying []string) {
	t.Helper()
	if len(tr.ups) != len(ups) {
		t.Errorf("ups = %v, want %v", tr.ups, ups)
	}
	if len(tr.downs) != len(downs) {
		t.Errorf("downs = %v, want %v", tr.downs, downs)
	}
	if len(tr.trying) != len(trying) {
		t.Errorf("trying = %v, want %v", tr.trying, trying)
	}
}

// TestComputeStatusTransitionsStableTunnelNotReAnnounced is the core
// regression for #111: a tunnel that is STABLY connected must not be
// re-announced on every 30s status poll. Only the tunnel that actually
// changed state is named.
func TestComputeStatusTransitionsStableTunnelNotReAnnounced(t *testing.T) {
	// 1) Startup: TS453Dmini is up, nothing announced before.
	tr, cur := computeStatusTransitions(nil, keySet("TS453Dmini"), keySet("TS453Dmini"))
	assertTransition(t, tr, []string{"TS453Dmini"}, nil, nil)
	_ = cur

	// 2) Next 30s poll: TS453Dmini is STILL up and nothing else changed.
	//    This is the exact spam case — it must produce NO transition.
	tr, _ = computeStatusTransitions(connSet("TS453Dmini", tcsConnected), keySet("TS453Dmini"), keySet("TS453Dmini"))
	assertTransition(t, tr, nil, nil, nil)

	// 3) The churner TS451D drops out of the established set while
	//    TS453Dmini stays up. The bubble must name TS451D (down), NOT
	//    re-announce TS453Dmini as "connected".
	tr, _ = computeStatusTransitions(
		connSet("TS451D", tcsConnected, "TS453Dmini", tcsConnected),
		keySet("TS453Dmini"),  // only TS453Dmini established now
		keySet("TS453Dmini"),  // only TS453Dmini active now (TS451D gone)
	)
	assertTransition(t, tr, nil, []string{"TS451D"}, nil)

	// 4) TS451D comes back: only TS451D is named (up). TS453Dmini is
	//    untouched and must not appear again.
	tr, _ = computeStatusTransitions(
		connSet("TS453Dmini", tcsConnected),
		keySet("TS451D", "TS453Dmini"),
		keySet("TS451D", "TS453Dmini"),
	)
	assertTransition(t, tr, []string{"TS451D"}, nil, nil)
}

// TestComputeStatusTransitionsConnecting captures the dialing→connected and
// connected→retrying transitions so a fresh attempt and a lost connection are
// both surfaced (the user is told about failures, never suppressed).
func TestComputeStatusTransitionsConnecting(t *testing.T) {
	// Fresh attempt: tunnel is active but not established yet.
	tr, _ := computeStatusTransitions(nil, keySet(), keySet("X"))
	assertTransition(t, tr, nil, nil, []string{"X"})

	// Attempt succeeds: dialing → connected.
	tr, _ = computeStatusTransitions(connSet("X", tcsConnecting), keySet("X"), keySet("X"))
	assertTransition(t, tr, []string{"X"}, nil, nil)

	// Connection lost but still retrying: connected → connecting.
	tr, _ = computeStatusTransitions(connSet("X", tcsConnected), keySet(), keySet("X"))
	assertTransition(t, tr, nil, []string{"X"}, nil)
}
