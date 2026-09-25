package helper

import (
	"testing"

	"github.com/imonior/wireguide-plus/internal/wifi"
)

// reconnectAllowed is the policy gate both reconnect paths consult before
// connecting; the table pins the decision invariants — most importantly the
// issue-3 case: a network flap (IP reconfig briefly dropping en0) must not
// let the blind "it was up, put it back" restore connect a tunnel whose
// policy says disconnect.
func TestReconnectAllowed(t *testing.T) {
	cases := []struct {
		name         string
		state        wifi.DesiredState
		hasPolicy    bool
		networkKnown bool
		manualOn     bool
		paused       bool
		exempted     bool
		want         bool
	}{
		{"no policy keeps legacy restore", wifi.StateUnmanaged, false, true, false, false, false, true},
		{"policy disconnect blocks", wifi.StateDisconnect, true, true, false, false, false, false},
		{"policy connect allows", wifi.StateConnect, true, true, false, false, false, true},
		{"unmanaged with policy allows", wifi.StateUnmanaged, true, true, false, false, false, true},
		{"manual-on latch outranks disconnect policy", wifi.StateDisconnect, true, true, true, false, false, true},
		{"unidentified network blocks policy tunnels", wifi.StateConnect, true, false, false, false, false, false},
		{"unidentified network, no policy, still restores", wifi.StateUnmanaged, false, false, false, false, false, true},
		{"paused blocks even with connect policy", wifi.StateConnect, true, true, false, true, false, false},
		{"exempted blocks even with no policy", wifi.StateUnmanaged, false, true, false, false, true, false},
		{"manual-on cannot override pause", wifi.StateConnect, true, true, true, true, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := reconnectAllowed(tc.state, tc.hasPolicy, tc.networkKnown, tc.manualOn, tc.paused, tc.exempted); got != tc.want {
				t.Errorf("reconnectAllowed(%v, hasPolicy=%v, known=%v, manualOn=%v, paused=%v, exempted=%v) = %v, want %v",
					tc.state, tc.hasPolicy, tc.networkKnown, tc.manualOn, tc.paused, tc.exempted, got, tc.want)
			}
		})
	}
}
