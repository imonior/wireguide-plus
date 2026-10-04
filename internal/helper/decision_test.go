package helper

import (
	"net"
	"testing"

	"github.com/imonior/wireguide-plus/internal/wifi"
)

// TestEvaluateTunnelDecision pins the convergent decision core so the engine,
// reconnect monitor and preview cannot drift apart. Every gate (network
// known, automation-disabled, address-conflict pause, ssid-blind, policy/DNS
// block, back-off, manual latch, default state) is exercised here.
func TestEvaluateTunnelDecision(t *testing.T) {
	known := wifi.NetworkContext{SSID: "Home", PhysicalIPs: []net.IP{net.ParseIP("10.0.0.1")}}
	unknown := wifi.NetworkContext{}

	cases := []struct {
		name string
		env  DecisionEnv
		want TunnelDecision
	}{
		{
			name: "unidentified network is unresolved",
			env:  DecisionEnv{HasPolicy: true, NetworkKnown: false, Ctx: unknown},
			want: TunnelDecision{Action: "unresolved", Reason: "network-unidentified", NetworkKnown: false, HasPolicy: true},
		},
		{
			name: "automation-disabled is exempt",
			env:  DecisionEnv{HasPolicy: true, NetworkKnown: true, Exempted: true, Ctx: known},
			want: TunnelDecision{Action: "", Reason: "automation-disabled", NetworkKnown: true, Exempted: true, HasPolicy: true},
		},
		{
			name: "address-conflict pause parks",
			env:  DecisionEnv{HasPolicy: true, NetworkKnown: true, Paused: true, Ctx: known},
			want: TunnelDecision{Action: "", Reason: "address-conflict-paused", NetworkKnown: true, Paused: true, HasPolicy: true},
		},
		{
			name: "ssid-blind is unresolved (fail-closed)",
			env:  DecisionEnv{HasPolicy: true, NetworkKnown: true, SSIDBlind: true, Ctx: known},
			want: TunnelDecision{Action: "unresolved", Reason: "ssid-blind", NetworkKnown: true, SSIDBlind: true, HasPolicy: true},
		},
		{
			name: "default connect on a known network connects",
			env:  DecisionEnv{HasPolicy: true, Default: wifi.ActionConnect, NetworkKnown: true, Ctx: known},
			want: TunnelDecision{Action: "connect", NetworkKnown: true, HasPolicy: true, Desired: wifi.StateConnect},
		},
		{
			name: "default connect suppressed by manual-off latch",
			env:  DecisionEnv{HasPolicy: true, Default: wifi.ActionConnect, NetworkKnown: true, ManualOff: true, Ctx: known},
			want: TunnelDecision{Action: "skip-manual-off", Reason: "manual-off", NetworkKnown: true, HasPolicy: true, ManualOff: true, Desired: wifi.StateConnect},
		},
		{
			name: "connect blocked by policy",
			env:  DecisionEnv{HasPolicy: true, Default: wifi.ActionConnect, NetworkKnown: true, PolicyBlock: "traffic", Ctx: known},
			want: TunnelDecision{Action: "blocked", Blocked: true, BlockReason: "traffic", NetworkKnown: true, HasPolicy: true, Desired: wifi.StateConnect},
		},
		{
			name: "connect blocked by DNS resolve path",
			env:  DecisionEnv{HasPolicy: true, Default: wifi.ActionConnect, NetworkKnown: true, DNSBlock: true, Ctx: known},
			want: TunnelDecision{Action: "blocked", Blocked: true, BlockReason: "dns", NetworkKnown: true, HasPolicy: true, Desired: wifi.StateConnect},
		},
		{
			name: "connect deferred by back-off",
			env:  DecisionEnv{HasPolicy: true, Default: wifi.ActionConnect, NetworkKnown: true, BackingOff: true, Ctx: known},
			want: TunnelDecision{Action: "deferred", Reason: "back-off", NetworkKnown: true, HasPolicy: true, BackingOff: true, Desired: wifi.StateConnect},
		},
		{
			name: "default disconnect tears down an active tunnel",
			env:  DecisionEnv{HasPolicy: true, Default: wifi.ActionDisconnect, NetworkKnown: true, Active: true, Ctx: known},
			want: TunnelDecision{Action: "disconnect", NetworkKnown: true, HasPolicy: true, Desired: wifi.StateDisconnect},
		},
		{
			name: "no policy and no default is untouched",
			env:  DecisionEnv{HasPolicy: false, NetworkKnown: true, Ctx: known},
			want: TunnelDecision{Action: "", NetworkKnown: true, HasPolicy: false, Desired: wifi.StateUnmanaged},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := evaluateTunnelDecision(tc.env)
			if got.Action != tc.want.Action || got.Reason != tc.want.Reason ||
				got.Blocked != tc.want.Blocked || got.BlockReason != tc.want.BlockReason ||
				got.NetworkKnown != tc.want.NetworkKnown || got.SSIDBlind != tc.want.SSIDBlind ||
				got.Exempted != tc.want.Exempted || got.Paused != tc.want.Paused ||
				got.ManualOff != tc.want.ManualOff || got.HasPolicy != tc.want.HasPolicy ||
				got.BackingOff != tc.want.BackingOff || got.Desired != tc.want.Desired {
				t.Errorf("evaluateTunnelDecision = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestEvaluateTunnelDecisionMatchedRule confirms a matching rule is surfaced
// for preview, so the editor and CLI can show which rule won.
func TestEvaluateTunnelDecisionMatchedRule(t *testing.T) {
	rule := wifi.Rule{When: []wifi.Condition{{Type: wifi.CondWiFi}}, Do: wifi.ActionConnect}
	env := DecisionEnv{
		HasPolicy:    true,
		Rules:        []wifi.Rule{rule},
		NetworkKnown: true,
		Ctx:          wifi.NetworkContext{SSID: "Home"},
	}
	got := evaluateTunnelDecision(env)
	if got.Action != "connect" {
		t.Fatalf("Action = %q, want connect", got.Action)
	}
	if got.MatchedRule == nil || got.MatchedRule.Do != wifi.ActionConnect {
		t.Errorf("MatchedRule = %+v, want the connect rule", got.MatchedRule)
	}
}
