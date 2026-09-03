package wifi

import (
	"encoding/json"
	"net"
	"testing"
	"time"
)

func ips(ss ...string) []net.IP {
	out := make([]net.IP, 0, len(ss))
	for _, s := range ss {
		out = append(out, net.ParseIP(s))
	}
	return out
}

// singleCond is a helper mirroring the legacy single-condition rule shape.
func singleCond(c Condition, do Action) Rule {
	return Rule{When: []Condition{c}, Do: do}
}

func TestEvaluate_SSIDDisconnectElseConnect(t *testing.T) {
	// lucidnx's canonical workflow: off on the office network, on elsewhere.
	rules := []Rule{
		singleCond(Condition{Type: CondSSID, SSID: "corp-wifi"}, ActionDisconnect),
		singleCond(Condition{Type: CondNoneMatch}, ActionConnect),
	}
	if got := Evaluate(rules, NetworkContext{SSID: "corp-wifi"}); got != StateDisconnect {
		t.Errorf("on corp-wifi: got %v, want disconnect", got)
	}
	if got := Evaluate(rules, NetworkContext{SSID: "home"}); got != StateConnect {
		t.Errorf("on home: got %v, want connect", got)
	}
	// SSID matching is EXACT (full name, case-sensitive): "CORP-WIFI" is a
	// different network from "corp-wifi", so it must NOT match.
	if got := Evaluate(rules, NetworkContext{SSID: "CORP-WIFI"}); got != StateConnect {
		t.Errorf("case-sensitive SSID: got %v, want connect (no match)", got)
	}
	// Middle spaces and special characters are part of the name and must
	// be compared byte-for-byte.
	spaced := []Rule{
		singleCond(Condition{Type: CondSSID, SSID: "My Home 5G!"}, ActionDisconnect),
	}
	if got := Evaluate(spaced, NetworkContext{SSID: "My Home 5G!"}); got != StateDisconnect {
		t.Errorf("SSID with middle space/special chars: got %v, want disconnect", got)
	}
	if got := Evaluate(spaced, NetworkContext{SSID: "My Home 5G"}); got != StateUnmanaged {
		t.Errorf("SSID missing trailing char: got %v, want unmanaged (no match)", got)
	}
}

func TestEvaluate_SubnetDisconnect(t *testing.T) {
	rules := []Rule{
		singleCond(Condition{Type: CondSubnet, Subnet: "10.1.1.0/24"}, ActionDisconnect),
		singleCond(Condition{Type: CondNoneMatch}, ActionConnect),
	}
	if got := Evaluate(rules, NetworkContext{PhysicalIPs: ips("10.1.1.42")}); got != StateDisconnect {
		t.Errorf("inside subnet: got %v, want disconnect", got)
	}
	if got := Evaluate(rules, NetworkContext{PhysicalIPs: ips("192.168.0.5")}); got != StateConnect {
		t.Errorf("outside subnet: got %v, want connect", got)
	}
}

func TestEvaluate_NetworkGatewayMAC(t *testing.T) {
	// Two homes both on 192.168.0.0/24 but with different routers — the
	// gateway-MAC fingerprint disambiguates them where subnet can't.
	rules := []Rule{
		singleCond(Condition{Type: CondNetwork, GatewayMAC: "b0:38:6c:54:8b:ab"}, ActionDisconnect),
		singleCond(Condition{Type: CondNoneMatch}, ActionConnect),
	}
	// On the fingerprinted network → disconnect (case-insensitive match).
	if got := Evaluate(rules, NetworkContext{GatewayMAC: "B0:38:6C:54:8B:AB", PhysicalIPs: ips("192.168.0.5")}); got != StateDisconnect {
		t.Errorf("matching gateway MAC: got %v, want disconnect", got)
	}
	// A different router on the SAME subnet → not matched → connect.
	if got := Evaluate(rules, NetworkContext{GatewayMAC: "aa:bb:cc:dd:ee:ff", PhysicalIPs: ips("192.168.0.5")}); got != StateConnect {
		t.Errorf("different gateway MAC, same subnet: got %v, want connect", got)
	}
	// Unknown gateway MAC → does not match.
	if got := Evaluate(rules, NetworkContext{GatewayMAC: ""}); got != StateConnect {
		t.Errorf("empty gateway MAC: got %v, want connect", got)
	}
	// Separator/case variants of the SAME MAC must all match — users
	// paste dashes, no separators, upper-case, etc.
	for _, variant := range []string{"B0-38-6C-54-8B-AB", "b0386c548bab", "B0:38:6C:54:8B:AB", "b0-38-6c-54-8b-ab"} {
		vr := []Rule{
			singleCond(Condition{Type: CondNetwork, GatewayMAC: variant}, ActionDisconnect),
			singleCond(Condition{Type: CondNoneMatch}, ActionConnect),
		}
		if got := Evaluate(vr, NetworkContext{GatewayMAC: "b0:38:6c:54:8b:ab"}); got != StateDisconnect {
			t.Errorf("MAC variant %q should match canonical form: got %v", variant, got)
		}
	}
}

func TestEvaluate_FirstConcreteMatchWins(t *testing.T) {
	rules := []Rule{
		singleCond(Condition{Type: CondSSID, SSID: "a"}, ActionConnect),
		singleCond(Condition{Type: CondSSID, SSID: "a"}, ActionDisconnect), // shadowed
	}
	if got := Evaluate(rules, NetworkContext{SSID: "a"}); got != StateConnect {
		t.Errorf("first match should win: got %v, want connect", got)
	}
}

func TestEvaluate_ConflictTopWins(t *testing.T) {
	// Two DIFFERENT condition types that both match the same context but
	// disagree on the action — the topmost rule must win, and reordering
	// must flip the result (drag-to-reorder = priority).
	ctx := NetworkContext{
		SSID:        "office-wifi",
		GatewayMAC:  "b0:38:6c:54:8b:ab",
		PhysicalIPs: ips("192.168.0.5"),
	}
	netRule := singleCond(Condition{Type: CondNetwork, GatewayMAC: "b0:38:6c:54:8b:ab"}, ActionDisconnect)
	wifiRule := singleCond(Condition{Type: CondSSID, SSID: "office-wifi"}, ActionConnect)

	if got := Evaluate([]Rule{netRule, wifiRule}, ctx); got != StateDisconnect {
		t.Errorf("network rule on top: got %v, want disconnect", got)
	}
	if got := Evaluate([]Rule{wifiRule, netRule}, ctx); got != StateConnect {
		t.Errorf("wifi rule on top (reordered): got %v, want connect", got)
	}
}

func TestEvaluate_NoRulesOrNoMatch(t *testing.T) {
	if got := Evaluate(nil, NetworkContext{SSID: "x"}); got != StateUnmanaged {
		t.Errorf("no rules: got %v, want unmanaged", got)
	}
	// Concrete conditions present but none match, and no none_match rule.
	rules := []Rule{singleCond(Condition{Type: CondSSID, SSID: "a"}, ActionConnect)}
	if got := Evaluate(rules, NetworkContext{SSID: "b"}); got != StateUnmanaged {
		t.Errorf("no match, no fallback: got %v, want unmanaged", got)
	}
}

func TestEvaluate_NoneMatchOnlyWhenNoConcreteMatch(t *testing.T) {
	rules := []Rule{
		singleCond(Condition{Type: CondSSID, SSID: "corp"}, ActionDisconnect),
		singleCond(Condition{Type: CondNoneMatch}, ActionConnect),
	}
	// Concrete matches → none_match must NOT fire.
	if got := Evaluate(rules, NetworkContext{SSID: "corp"}); got != StateDisconnect {
		t.Errorf("concrete match present: got %v, want disconnect", got)
	}
}

func TestEvaluate_InvalidSubnetIgnored(t *testing.T) {
	rules := []Rule{
		singleCond(Condition{Type: CondSubnet, Subnet: "not-a-cidr"}, ActionDisconnect),
		singleCond(Condition{Type: CondNoneMatch}, ActionConnect),
	}
	if got := Evaluate(rules, NetworkContext{PhysicalIPs: ips("10.1.1.1")}); got != StateConnect {
		t.Errorf("invalid subnet should not match: got %v, want connect", got)
	}
}

// none_match is unconditional AT ITS POSITION: dragged to the top it
// overrides everything below (issue #12, was previously always held to
// the end regardless of order).
func TestEvaluate_NoneMatchHonoursPosition(t *testing.T) {
	rules := []Rule{
		singleCond(Condition{Type: CondNoneMatch}, ActionDisconnect), // top → unconditional
		singleCond(Condition{Type: CondSSID, SSID: "home"}, ActionConnect),
	}
	// Even on "home", the top else wins.
	if got := Evaluate(rules, NetworkContext{SSID: "home"}); got != StateDisconnect {
		t.Errorf("else at top should override: want Disconnect, got %v", got)
	}
	// Moving the concrete rule above the else restores first-match-wins.
	rules[0], rules[1] = rules[1], rules[0]
	if got := Evaluate(rules, NetworkContext{SSID: "home"}); got != StateConnect {
		t.Errorf("concrete above else: want Connect, got %v", got)
	}
}

// An unknown/empty action fails closed (rule skipped), it does not
// default to connect (issue #12).
func TestEvaluate_UnknownActionSkipped(t *testing.T) {
	rules := []Rule{
		{When: []Condition{{Type: CondSSID, SSID: "home"}}, Do: Action("bogus")},
		singleCond(Condition{Type: CondNoneMatch}, ActionDisconnect),
	}
	if got := Evaluate(rules, NetworkContext{SSID: "home"}); got != StateDisconnect {
		t.Errorf("unknown action must be skipped, not treated as connect: got %v", got)
	}
	// A lone invalid rule leaves the tunnel unmanaged.
	if got := Evaluate(rules[:1], NetworkContext{SSID: "home"}); got != StateUnmanaged {
		t.Errorf("lone invalid rule: want Unmanaged, got %v", got)
	}
}

// New (issue: Any Wi-Fi) — a "wifi" condition matches whenever the
// current network is Wi-Fi, regardless of which SSID. Combined with a
// specific-SSID disconnect this is the WireTunnels-style "connect on any
// Wi-Fi, disconnect on these specific networks" workflow.
func TestEvaluate_WiFiAny(t *testing.T) {
	rules := []Rule{
		singleCond(Condition{Type: CondSSID, SSID: "ASUS_AC68U_QS_5G"}, ActionDisconnect),
		singleCond(Condition{Type: CondSSID, SSID: "ASUS_QS_JYH"}, ActionDisconnect),
		singleCond(Condition{Type: CondWiFi}, ActionConnect),
	}
	if got := Evaluate(rules, NetworkContext{SSID: "ASUS_QS_JYH"}); got != StateDisconnect {
		t.Errorf("on known home SSID: got %v, want disconnect", got)
	}
	if got := Evaluate(rules, NetworkContext{SSID: "Starbucks-WiFi"}); got != StateConnect {
		t.Errorf("on random Wi-Fi: got %v, want connect", got)
	}
	// Ethernet / no Wi-Fi (SSID unknown) → the wifi condition must NOT fire.
	if got := Evaluate(rules, NetworkContext{SSID: "", PhysicalIPs: ips("192.168.0.5")}); got != StateUnmanaged {
		t.Errorf("on ethernet: got %v, want unmanaged", got)
	}
}

// New (issue: AND/OR) — a rule with multiple conditions combines them per
// its Match field: "all" requires every condition (AND), ""/any fires on
// the first match (OR).
func TestEvaluate_MultiConditionAlwaysAND(t *testing.T) {
	rule := Rule{
		When: []Condition{
			{Type: CondSSID, SSID: "office-wifi"},
			{Type: CondSubnet, Subnet: "10.2.0.0/16"},
		},
		Do: ActionConnect,
	}
	if got := Evaluate([]Rule{rule}, NetworkContext{SSID: "office-wifi"}); got != StateUnmanaged {
		t.Errorf("missing subnet must not match: got %v, want unmanaged", got)
	}
	if got := Evaluate([]Rule{rule}, NetworkContext{PhysicalIPs: ips("10.2.33.7")}); got != StateUnmanaged {
		t.Errorf("missing SSID must not match: got %v, want unmanaged", got)
	}
	if got := Evaluate([]Rule{rule}, NetworkContext{SSID: "office-wifi", PhysicalIPs: ips("10.2.33.7")}); got != StateConnect {
		t.Errorf("both conditions must match: got %v, want connect", got)
	}
}

func TestEvaluate_MultiConditionAND(t *testing.T) {
	// AND: connect only on the office subnet AND while on the office SSID.
	and := Rule{
		When: []Condition{
			{Type: CondSSID, SSID: "office-wifi"},
			{Type: CondSubnet, Subnet: "10.2.0.0/16"},
		},
		Match: "all",
		Do:    ActionConnect,
	}
	// Both hold → connect.
	if got := Evaluate([]Rule{and}, NetworkContext{SSID: "office-wifi", PhysicalIPs: ips("10.2.33.7")}); got != StateConnect {
		t.Errorf("AND both hold: got %v, want connect", got)
	}
	// SSID matches but subnet doesn't → NOT connect.
	if got := Evaluate([]Rule{and}, NetworkContext{SSID: "office-wifi", PhysicalIPs: ips("192.168.1.5")}); got != StateUnmanaged {
		t.Errorf("AND subnet misses: got %v, want unmanaged", got)
	}
	// Subnet matches but SSID doesn't → NOT connect.
	if got := Evaluate([]Rule{and}, NetworkContext{SSID: "cafe", PhysicalIPs: ips("10.2.33.7")}); got != StateUnmanaged {
		t.Errorf("AND ssid misses: got %v, want unmanaged", got)
	}
}

// AND + none_match: none_match is unconditional, so in an AND rule it
// contributes nothing and the concrete conditions decide.
func TestEvaluate_ANDWithNoneMatch(t *testing.T) {
	and := Rule{
		When: []Condition{
			{Type: CondSubnet, Subnet: "10.2.0.0/16"},
			{Type: CondNoneMatch},
		},
		Match: "all",
		Do:    ActionDisconnect,
	}
	if got := Evaluate([]Rule{and}, NetworkContext{PhysicalIPs: ips("10.2.1.1")}); got != StateDisconnect {
		t.Errorf("AND + none_match on subnet: got %v, want disconnect", got)
	}
	if got := Evaluate([]Rule{and}, NetworkContext{PhysicalIPs: ips("192.168.1.1")}); got != StateUnmanaged {
		t.Errorf("AND + none_match outside subnet: got %v, want unmanaged", got)
	}
}

func TestValidateRule(t *testing.T) {
	bad := []Rule{
		singleCond(Condition{Type: CondSSID, SSID: ""}, ActionConnect),
		singleCond(Condition{Type: CondSubnet, Subnet: "not-a-cidr"}, ActionConnect),
		singleCond(Condition{Type: CondNetwork, GatewayMAC: "zz:zz"}, ActionConnect),
		singleCond(Condition{Type: CondSSID, SSID: "ok"}, Action("nope")),
		singleCond(Condition{Type: "weird"}, ActionConnect),
		{When: nil, Do: ActionConnect},                                   // no conditions
		{When: []Condition{{Type: CondSSID, SSID: "ok"}}, Match: "xor", Do: ActionConnect}, // bad combiner
	}
	for i, r := range bad {
		if err := ValidateRule(r); err == nil {
			t.Errorf("bad rule %d should fail validation: %+v", i, r)
		}
	}
	good := []Rule{
		singleCond(Condition{Type: CondSSID, SSID: "home"}, ActionConnect),
		singleCond(Condition{Type: CondSubnet, Subnet: "10.0.0.0/8"}, ActionDisconnect),
		singleCond(Condition{Type: CondNetwork, GatewayMAC: "B0-38-6C-54-8B-AB"}, ActionConnect),
		singleCond(Condition{Type: CondNoneMatch}, ActionConnect),
		singleCond(Condition{Type: CondWiFi}, ActionConnect),
		{When: []Condition{{Type: CondSSID, SSID: "a"}, {Type: CondSubnet, Subnet: "10.0.0.0/8"}}, Match: "all", Do: ActionConnect},
	}
	for i, r := range good {
		if err := ValidateRule(r); err != nil {
			t.Errorf("good rule %d should pass: %+v: %v", i, r, err)
		}
	}
}

// New — JSON round-trip must accept both the legacy single-object "when"
// form and the new array form, and preserve "match" (AND).
func TestRuleJSONCompat(t *testing.T) {
	// Legacy shape: "when" is a single object.
	legacy := `{"when":{"type":"ssid","ssid":"corp-wifi"},"do":"disconnect"}`
	var r1 Rule
	if err := json.Unmarshal([]byte(legacy), &r1); err != nil {
		t.Fatalf("unmarshal legacy: %v", err)
	}
	if r1.Do != ActionDisconnect || len(r1.When) != 1 || r1.When[0].Type != CondSSID || r1.When[0].SSID != "corp-wifi" {
		t.Fatalf("legacy parse wrong: %+v", r1)
	}
	if r1.Match != "" { // absence of match = OR
		t.Errorf("legacy rule should default to OR, got match=%q", r1.Match)
	}

	// New shape: array + match all.
	newFmt := `{"when":[{"type":"ssid","ssid":"office"},{"type":"subnet","subnet":"10.0.0.0/8"}],"match":"all","do":"connect"}`
	var r2 Rule
	if err := json.Unmarshal([]byte(newFmt), &r2); err != nil {
		t.Fatalf("unmarshal new: %v", err)
	}
	if r2.Match != "all" || len(r2.When) != 2 || r2.When[1].Subnet != "10.0.0.0/8" {
		t.Fatalf("new format parse wrong: %+v", r2)
	}
	if err := ValidateRule(r2); err != nil {
		t.Errorf("new-format rule should validate: %v", err)
	}

	// Marshalling a new-shape rule must round-trip (when stays an array).
	out, err := json.Marshal(r2)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var r3 Rule
	if err := json.Unmarshal(out, &r3); err != nil {
		t.Fatalf("unmarshal round-trip: %v", err)
	}
	if r3.Match != "all" || len(r3.When) != 2 {
		t.Errorf("round-trip lost data: %+v (json %s)", r3, out)
	}
}

// New — EvaluateDetail reports per-condition outcomes that the editor's
// live match indicators consume, and its state agrees with Evaluate.
func TestEvaluateDetail(t *testing.T) {
	ctx := NetworkContext{SSID: "office-wifi", PhysicalIPs: ips("10.2.33.7")}
	rules := []Rule{
		{When: []Condition{{Type: CondSSID, SSID: "office-wifi"}, {Type: CondSubnet, Subnet: "10.2.0.0/16"}}, Match: "all", Do: ActionConnect},
		singleCond(Condition{Type: CondSSID, SSID: "cafe"}, ActionDisconnect),
	}
	state, details := EvaluateDetail(rules, ctx)
	if state != StateConnect {
		t.Errorf("state: got %v, want connect", state)
	}
	if len(details) != 2 {
		t.Fatalf("details: got %d rules, want 2", len(details))
	}
	if !details[0].Matched || details[0].MatchAll != true {
		t.Errorf("rule 0 should match (AND both true): %+v", details[0])
	}
	if len(details[0].Conditions) != 2 {
		t.Fatalf("rule 0 conditions: got %d, want 2", len(details[0].Conditions))
	}
	if !details[0].Conditions[0].Matched || !details[0].Conditions[1].Matched {
		t.Errorf("both AND conditions should match: %+v", details[0].Conditions)
	}
	if details[1].Matched {
		t.Errorf("rule 1 (cafe) should not match on office-wifi")
	}
}

// Marker audit: after the first match the engine must KEEP judging every
// remaining rule, so the editor can mark later rules as matched-but-
// shadowed ("match" without "in use") — connect rules behind a matched
// disconnect rule are deprioritized and never executed.
func TestEvaluateDetail_ShadowedAndDeprioritized(t *testing.T) {
	ctx := NetworkContext{SSID: "Corp", PhysicalIPs: ips("10.1.1.5")}
	rules := []Rule{
		singleCond(Condition{Type: CondSSID, SSID: "Corp"}, ActionDisconnect),
		{When: []Condition{{Type: CondSSID, SSID: "Corp"}, {Type: CondSubnet, Subnet: "10.0.0.0/8"}}, Match: "all", Do: ActionDisconnect},
		singleCond(Condition{Type: CondSSID, SSID: "Corp"}, ActionConnect),
		singleCond(Condition{Type: CondNoneMatch}, ActionConnect),
	}
	state, details := EvaluateDetail(rules, ctx)
	if state != StateDisconnect {
		t.Errorf("state: got %v, want disconnect (first rule wins)", state)
	}
	if len(details) != 4 {
		t.Fatalf("details: got %d rules, want 4", len(details))
	}
	// Rule 0: matched → the winner (gets match + in-use in the editor).
	if !details[0].Matched {
		t.Errorf("rule 0 should match: %+v", details[0])
	}
	// Rule 1: AND with both conditions holding → matched too, but shadowed.
	if !details[1].Matched {
		t.Errorf("rule 1 should match (AND both hold): %+v", details[1])
	}
	if len(details[1].Conditions) != 2 || !details[1].Conditions[0].Matched || !details[1].Conditions[1].Matched {
		t.Errorf("rule 1 per-condition results wrong: %+v", details[1].Conditions)
	}
	// Rule 2: a matching CONNECT rule behind a matched disconnect rule —
	// deprioritized: reported as matched, but the state stays disconnect.
	if !details[2].Matched {
		t.Errorf("rule 2 (connect) should match: %+v", details[2])
	}
	// Rule 3: none_match is unconditional at its position → matched too,
	// yet still never executed because an earlier rule won.
	if !details[3].Matched {
		t.Errorf("rule 3 (none_match) should match: %+v", details[3])
	}
}

// An AND rule whose conditions disagree reports per-condition results
// truthfully while the rule as a whole does not match or fire.
func TestEvaluateDetail_AndPartialMatch(t *testing.T) {
	ctx := NetworkContext{SSID: "Corp", PhysicalIPs: ips("192.168.1.5")}
	and := Rule{
		When:  []Condition{{Type: CondSSID, SSID: "Corp"}, {Type: CondSubnet, Subnet: "10.0.0.0/8"}},
		Match: "all",
		Do:    ActionDisconnect,
	}
	state, details := EvaluateDetail([]Rule{and}, ctx)
	if state != StateUnmanaged {
		t.Errorf("state: got %v, want unmanaged (AND not satisfied)", state)
	}
	if details[0].Matched {
		t.Errorf("rule must not match with only one condition holding: %+v", details[0])
	}
	if !details[0].Conditions[0].Matched || details[0].Conditions[1].Matched {
		t.Errorf("per-condition results wrong: %+v", details[0].Conditions)
	}
}

func TestMigrateFromLegacy(t *testing.T) {
	legacy := &Rules{
		TrustedSSIDs: []string{"corp-wifi"},
		PerTunnel: map[string]TunnelSSIDs{
			"company": {AutoConnectSSIDs: []string{"home", "cafe"}},
			"nolegacy": {},
		},
	}
	auto := MigrateFromLegacy(legacy)

	got := auto.PerTunnel["company"]
	// trusted disconnect + connect home + connect cafe (no synthesized
	// none_match — migration translates only explicit legacy settings).
	if len(got) != 3 {
		t.Fatalf("company rules: got %d, want 3 (%+v)", len(got), got)
	}
	// Trusted disconnect must come first (precedence).
	if got[0].Do != ActionDisconnect || got[0].When[0].SSID != "corp-wifi" {
		t.Errorf("first rule should be trusted disconnect, got %+v", got[0])
	}
	if got[1].Do != ActionConnect || got[1].When[0].SSID != "home" {
		t.Errorf("second rule should be connect home, got %+v", got[1])
	}
	// Migration must NOT synthesize a none_match rule.
	for _, r := range got {
		if r.When[0].Type == CondNoneMatch {
			t.Errorf("migration should not add a none_match rule, got %+v", r)
		}
	}
	// A tunnel with no legacy rules gets no rules.
	if _, ok := auto.PerTunnel["nolegacy"]; ok {
		t.Errorf("nolegacy should have no migrated rules")
	}

	// Behavioural check: on corp-wifi the migrated company tunnel
	// disconnects; on home it connects.
	if s := Evaluate(got, NetworkContext{SSID: "corp-wifi"}); s != StateDisconnect {
		t.Errorf("migrated: corp-wifi got %v, want disconnect", s)
	}
	if s := Evaluate(got, NetworkContext{SSID: "home"}); s != StateConnect {
		t.Errorf("migrated: home got %v, want connect", s)
	}
	// A network matching none of the tunnel's rules leaves it untouched —
	// migration no longer forces a disconnect on unlisted networks
	// (including Ethernet / no-SSID). This is the fix for the observed
	// "manually connected on Ethernet, got auto-killed" behaviour.
	if s := Evaluate(got, NetworkContext{SSID: "random-cafe"}); s != StateUnmanaged {
		t.Errorf("migrated: away network got %v, want unmanaged", s)
	}
	if s := Evaluate(got, NetworkContext{PhysicalIPs: ips("192.168.0.5")}); s != StateUnmanaged {
		t.Errorf("migrated: ethernet (no ssid) got %v, want unmanaged", s)
	}
}

func TestMigrateFromLegacy_Nil(t *testing.T) {
	auto := MigrateFromLegacy(nil)
	if auto == nil || auto.PerTunnel == nil {
		t.Fatal("nil legacy should yield an initialised empty Automation")
	}
}

// New — gateway_ip matches the current default gateway's IPv4 exactly
// (medium-agnostic like gateway MAC, but stable across router swaps).
func TestEvaluate_GatewayIP(t *testing.T) {
	rules := []Rule{
		singleCond(Condition{Type: CondGatewayIP, GatewayIP: "192.168.0.1"}, ActionDisconnect),
		singleCond(Condition{Type: CondNoneMatch}, ActionConnect),
	}
	if got := Evaluate(rules, NetworkContext{GatewayIP: "192.168.0.1"}); got != StateDisconnect {
		t.Errorf("matching gateway IP: got %v, want disconnect", got)
	}
	if got := Evaluate(rules, NetworkContext{GatewayIP: "192.168.1.1"}); got != StateConnect {
		t.Errorf("different gateway IP: got %v, want connect", got)
	}
	// Unknown gateway IP must not match.
	if got := Evaluate(rules, NetworkContext{GatewayIP: ""}); got != StateConnect {
		t.Errorf("empty gateway IP: got %v, want connect", got)
	}
	// Invalid condition value matches nothing (never crashes).
	if got := Evaluate([]Rule{singleCond(Condition{Type: CondGatewayIP, GatewayIP: "nope"}, ActionDisconnect)}, NetworkContext{GatewayIP: "nope"}); got != StateUnmanaged {
		t.Errorf("invalid gateway IP should never match: got %v, want unmanaged", got)
	}
}

// New — interface matches a physical interface name case-insensitively.
func TestEvaluate_Interface(t *testing.T) {
	ctx := NetworkContext{Interfaces: []InterfaceInfo{{Name: "en0"}, {Name: "Ethernet 2"}}}
	rules := []Rule{
		singleCond(Condition{Type: CondInterface, InterfaceName: "EN0"}, ActionDisconnect),
		singleCond(Condition{Type: CondNoneMatch}, ActionConnect),
	}
	if got := Evaluate(rules, ctx); got != StateDisconnect {
		t.Errorf("interface name (case-insensitive): got %v, want disconnect", got)
	}
	if got := Evaluate(rules, NetworkContext{Interfaces: []InterfaceInfo{{Name: "wlx00"}}}); got != StateConnect {
		t.Errorf("unlisted interface: got %v, want connect", got)
	}
	// No interfaces at all → never matches.
	if got := Evaluate(rules, NetworkContext{}); got != StateConnect {
		t.Errorf("empty interface list: got %v, want connect", got)
	}
}

// New — ethernet matches when a wired (non-Wi-Fi) physical interface is
// up, regardless of whether Wi-Fi is also connected.
func TestEvaluate_Ethernet(t *testing.T) {
	rules := []Rule{
		singleCond(Condition{Type: CondEthernet}, ActionDisconnect),
		singleCond(Condition{Type: CondNoneMatch}, ActionConnect),
	}
	wired := NetworkContext{Interfaces: []InterfaceInfo{{Name: "en0", IsWiFi: false}}}
	if got := Evaluate(rules, wired); got != StateDisconnect {
		t.Errorf("wired only: got %v, want disconnect", got)
	}
	// Wi-Fi + wired together → still on Ethernet, rule fires.
	dual := NetworkContext{Interfaces: []InterfaceInfo{{Name: "Wi-Fi", IsWiFi: true}, {Name: "Ethernet", IsWiFi: false}}}
	if got := Evaluate(rules, dual); got != StateDisconnect {
		t.Errorf("wired + wifi: got %v, want disconnect", got)
	}
	// Wi-Fi only → not Ethernet.
	wifiOnly := NetworkContext{Interfaces: []InterfaceInfo{{Name: "Wi-Fi", IsWiFi: true}}}
	if got := Evaluate(rules, wifiOnly); got != StateConnect {
		t.Errorf("wifi only: got %v, want connect", got)
	}
	if got := Evaluate(rules, NetworkContext{}); got != StateConnect {
		t.Errorf("no interfaces: got %v, want connect", got)
	}
}

// New — time matches a local-time window with optional day-of-week filter,
// including overnight windows and day-only rules.
func TestEvaluate_Time(t *testing.T) {
	at := func(h, m int) time.Time {
		return time.Date(2026, 9, 2, h, m, 0, 0, time.Local) // Wednesday
	}
	day := func(wd time.Weekday) time.Time { // 9:30 on a given weekday (2026-09-05 is a Saturday)
		return time.Date(2026, 9, 5, 9, 30, 0, 0, time.Local).AddDate(0, 0, int(wd)-int(time.Saturday))
	}

	// Window 09:00–17:00, any day.
	window := singleCond(Condition{Type: CondTime, Start: "09:00", End: "17:00"}, ActionDisconnect)
	if got := Evaluate([]Rule{window}, NetworkContext{Now: at(10, 0)}); got != StateDisconnect {
		t.Errorf("inside window: got %v, want disconnect", got)
	}
	if got := Evaluate([]Rule{window}, NetworkContext{Now: at(18, 0)}); got != StateUnmanaged {
		t.Errorf("outside window: got %v, want unmanaged", got)
	}
	// Boundary: end is exclusive.
	if got := Evaluate([]Rule{window}, NetworkContext{Now: at(17, 0)}); got != StateUnmanaged {
		t.Errorf("at end boundary: got %v, want unmanaged", got)
	}

	// Overnight window 22:00–06:00 wraps past midnight.
	night := singleCond(Condition{Type: CondTime, Start: "22:00", End: "06:00"}, ActionDisconnect)
	if got := Evaluate([]Rule{night}, NetworkContext{Now: at(23, 30)}); got != StateDisconnect {
		t.Errorf("overnight early: got %v, want disconnect", got)
	}
	if got := Evaluate([]Rule{night}, NetworkContext{Now: at(3, 0)}); got != StateDisconnect {
		t.Errorf("overnight after midnight: got %v, want disconnect", got)
	}
	if got := Evaluate([]Rule{night}, NetworkContext{Now: at(12, 0)}); got != StateUnmanaged {
		t.Errorf("overnight middle of day: got %v, want unmanaged", got)
	}

	// Day-of-week only: any time Saturday.
	sat := singleCond(Condition{Type: CondTime, Days: []int{6}}, ActionDisconnect)
	if got := Evaluate([]Rule{sat}, NetworkContext{Now: day(time.Saturday)}); got != StateDisconnect {
		t.Errorf("saturday any time: got %v, want disconnect", got)
	}
	if got := Evaluate([]Rule{sat}, NetworkContext{Now: day(time.Wednesday)}); got != StateUnmanaged {
		t.Errorf("wednesday: got %v, want unmanaged", got)
	}

	// Window + weekday: only Wednesdays inside the window.
	wed := singleCond(Condition{Type: CondTime, Start: "09:00", End: "17:00", Days: []int{int(time.Wednesday)}}, ActionDisconnect)
	if got := Evaluate([]Rule{wed}, NetworkContext{Now: at(10, 0)}); got != StateDisconnect {
		t.Errorf("wednesday in window: got %v, want disconnect", got)
	}
	if got := Evaluate([]Rule{wed}, NetworkContext{Now: day(time.Saturday)}); got != StateUnmanaged {
		t.Errorf("saturday in window should not match: got %v, want unmanaged", got)
	}

	// Unbounded window (start only): from 18:00 to midnight.
	open := singleCond(Condition{Type: CondTime, Start: "18:00"}, ActionDisconnect)
	if got := Evaluate([]Rule{open}, NetworkContext{Now: at(20, 0)}); got != StateDisconnect {
		t.Errorf("open end 20:00: got %v, want disconnect", got)
	}
	if got := Evaluate([]Rule{open}, NetworkContext{Now: at(10, 0)}); got != StateUnmanaged {
		t.Errorf("open end 10:00: got %v, want unmanaged", got)
	}
}

func TestValidateRule_NewConditions(t *testing.T) {
	bad := []Rule{
		singleCond(Condition{Type: CondGatewayIP, GatewayIP: "not-an-ip"}, ActionConnect),
		singleCond(Condition{Type: CondInterface, InterfaceName: ""}, ActionConnect),
		singleCond(Condition{Type: CondTime}, ActionConnect),                          // nothing set
		singleCond(Condition{Type: CondTime, Start: "25:99"}, ActionConnect),          // bad clock
		singleCond(Condition{Type: CondTime, Start: "09:00", Days: []int{9}}, ActionConnect), // bad weekday
	}
	for i, r := range bad {
		if err := ValidateRule(r); err == nil {
			t.Errorf("bad rule %d should fail validation: %+v", i, r)
		}
	}
	good := []Rule{
		singleCond(Condition{Type: CondGatewayIP, GatewayIP: "192.168.0.1"}, ActionConnect),
		singleCond(Condition{Type: CondInterface, InterfaceName: "en0"}, ActionConnect),
		singleCond(Condition{Type: CondEthernet}, ActionConnect),
		singleCond(Condition{Type: CondTime, Start: "09:00", End: "17:00"}, ActionConnect),
		singleCond(Condition{Type: CondTime, Start: "22:00", End: "06:00", Days: []int{0, 6}}, ActionConnect), // weekend overnight
		singleCond(Condition{Type: CondTime, Days: []int{3}}, ActionConnect), // wednesday only
		singleCond(Condition{Type: CondTime, End: "12:00"}, ActionConnect),   // until noon
	}
	for i, r := range good {
		if err := ValidateRule(r); err != nil {
			t.Errorf("good rule %d should pass: %+v: %v", i, r, err)
		}
	}
}
