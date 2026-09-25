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
		ssidBlind    bool
		manualOn     bool
		paused       bool
		exempted     bool
		want         bool
	}{
		{"no policy keeps legacy restore", wifi.StateUnmanaged, false, true, false, false, false, false, true},
		{"policy disconnect blocks", wifi.StateDisconnect, true, true, false, false, false, false, false},
		{"policy connect allows", wifi.StateConnect, true, true, false, false, false, false, true},
		{"unmanaged with policy allows", wifi.StateUnmanaged, true, true, false, false, false, false, true},
		{"manual-on latch outranks disconnect policy", wifi.StateDisconnect, true, true, false, true, false, false, true},
		{"unidentified network blocks policy tunnels", wifi.StateConnect, true, false, false, false, false, false, false},
		{"unidentified network, no policy, still restores", wifi.StateUnmanaged, false, false, false, false, false, false, true},
		{"paused blocks even with connect policy", wifi.StateConnect, true, true, false, false, true, false, false},
		{"exempted blocks even with no policy", wifi.StateUnmanaged, false, true, false, false, false, true, false},
		{"manual-on cannot override pause", wifi.StateConnect, true, true, false, true, true, false, false},
		// The SSID-blind cases: the engine refuses to decide a tunnel whose
		// rules depend on an unknown SSID, so the monitor must not act on
		// the half picture either (the same-SSID static→DHCP misconnect).
		{"ssid-blind blocks policy tunnels", wifi.StateConnect, true, true, true, false, false, false, false},
		{"ssid-blind, no policy, still restores", wifi.StateUnmanaged, false, true, true, false, false, false, true},
		{"ssid-blind cannot override manual-on", wifi.StateConnect, true, true, true, true, false, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := reconnectAllowed(tc.state, tc.hasPolicy, tc.networkKnown, tc.ssidBlind, tc.manualOn, tc.paused, tc.exempted); got != tc.want {
				t.Errorf("reconnectAllowed(%v, hasPolicy=%v, known=%v, blind=%v, manualOn=%v, paused=%v, exempted=%v) = %v, want %v",
					tc.state, tc.hasPolicy, tc.networkKnown, tc.ssidBlind, tc.manualOn, tc.paused, tc.exempted, got, tc.want)
			}
		})
	}
}

// TestSsidBlind pins the fail-closed gate: only a rule set that conditions
// on the SSID is skipped when the SSID is unknown, and only when the
// context actually has a live Wi-Fi uplink (a wired machine's permanent
// empty SSID is a fact, not missing data).
func TestSsidBlind(t *testing.T) {
	ssidRules := []wifi.Rule{{
		When: []wifi.Condition{{Type: wifi.CondSSID, SSID: "Office"}},
		Do:   wifi.ActionDisconnect,
	}}
	wifiRules := []wifi.Rule{{
		When: []wifi.Condition{{Type: wifi.CondWiFi}, {Type: wifi.CondSSID, SSID: "Home"}},
		Do:   wifi.ActionConnect,
	}}
	subnetRules := []wifi.Rule{{
		When: []wifi.Condition{{Type: wifi.CondSubnet, Subnet: "10.0.0.0/24"}},
		Do:   wifi.ActionDisconnect,
	}}
	wifiUp := wifi.InterfaceInfo{Name: "en0", IsWiFi: true, Active: true}
	wired := wifi.InterfaceInfo{Name: "Ethernet", IsWiFi: false, Active: true}

	cases := []struct {
		name string
		rule []wifi.Rule
		ctx  wifi.NetworkContext
		want bool
	}{
		{"blind: ssid rules, no ssid, wifi uplink", ssidRules,
			wifi.NetworkContext{Interfaces: []wifi.InterfaceInfo{wifiUp}}, true},
		{"not blind: ssid known", ssidRules,
			wifi.NetworkContext{SSID: "Café", Interfaces: []wifi.InterfaceInfo{wifiUp}}, false},
		{"not blind: wired only, empty SSID is a fact", ssidRules,
			wifi.NetworkContext{Interfaces: []wifi.InterfaceInfo{wired}}, false},
		{"not blind: no interfaces at all", ssidRules, wifi.NetworkContext{}, false},
		{"not blind: subnet rules survive an unknown SSID", subnetRules,
			wifi.NetworkContext{Interfaces: []wifi.InterfaceInfo{wifiUp}}, false},
		{"blind: wifi condition counts", wifiRules,
			wifi.NetworkContext{Interfaces: []wifi.InterfaceInfo{wifiUp, wired}}, true},
		{"not blind: no rules", nil,
			wifi.NetworkContext{Interfaces: []wifi.InterfaceInfo{wifiUp}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ssidBlind(tc.rule, tc.ctx); got != tc.want {
				t.Errorf("ssidBlind = %v, want %v", got, tc.want)
			}
		})
	}
}
