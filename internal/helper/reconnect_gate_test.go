package helper

import (
	"testing"

	"github.com/imonior/wireguide-plus/internal/wifi"
)

// DecisionConnectAllowed is the convergent policy gate the reconnect monitor
// consults (formerly reconnectAllowed). This table pins the same decision
// invariants — most importantly the issue-3 case: a network flap (IP
// reconfig briefly dropping en0) must not let the blind "it was up, put it
// back" restore connect a tunnel whose policy says disconnect.
func TestReconnectAllowed(t *testing.T) {
	cases := []struct {
		name     string
		decision TunnelDecision
		want     bool
	}{
		{"no policy keeps legacy restore", TunnelDecision{HasPolicy: false, NetworkKnown: true}, true},
		{"policy disconnect blocks", TunnelDecision{HasPolicy: true, Desired: wifi.StateDisconnect, NetworkKnown: true}, false},
		{"policy connect allows", TunnelDecision{HasPolicy: true, Desired: wifi.StateConnect, NetworkKnown: true}, true},
		{"unmanaged with policy allows", TunnelDecision{HasPolicy: true, Desired: wifi.StateUnmanaged, NetworkKnown: true}, true},
		{"manual-on latch outranks disconnect policy", TunnelDecision{HasPolicy: true, Desired: wifi.StateDisconnect, ManualOn: true, NetworkKnown: true}, true},
		{"unidentified network blocks policy tunnels", TunnelDecision{HasPolicy: true, Desired: wifi.StateConnect, NetworkKnown: false}, false},
		{"unidentified network, no policy, still restores", TunnelDecision{HasPolicy: false, NetworkKnown: false}, true},
		{"paused blocks even with connect policy", TunnelDecision{HasPolicy: true, Desired: wifi.StateConnect, Paused: true, NetworkKnown: true}, false},
		{"exempted blocks even with no policy", TunnelDecision{HasPolicy: false, Exempted: true, NetworkKnown: true}, false},
		{"manual-on cannot override pause", TunnelDecision{HasPolicy: true, Desired: wifi.StateConnect, ManualOn: true, Paused: true, NetworkKnown: true}, false},
		{"ssid-blind blocks policy tunnels", TunnelDecision{HasPolicy: true, Desired: wifi.StateConnect, SSIDBlind: true, NetworkKnown: true}, false},
		{"ssid-blind, no policy, still restores", TunnelDecision{HasPolicy: false, SSIDBlind: true, NetworkKnown: true}, true},
		{"ssid-blind cannot override manual-on", TunnelDecision{HasPolicy: true, Desired: wifi.StateConnect, SSIDBlind: true, ManualOn: true, NetworkKnown: true}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DecisionConnectAllowed(tc.decision); got != tc.want {
				t.Errorf("DecisionConnectAllowed(%+v) = %v, want %v", tc.decision, got, tc.want)
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
