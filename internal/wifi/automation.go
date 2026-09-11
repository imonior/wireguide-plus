package wifi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Automation is the per-tunnel condition→action rule model that
// generalises the older TrustedSSIDs + AutoConnectSSIDs pair (issue #12).
// Each tunnel owns an ordered list of rules; evaluation decides, from the
// current network context, whether that tunnel should be connected or
// disconnected — independently of how it was brought up.
//
// This type is additive: the legacy Rules (TrustedSSIDs / PerTunnel
// AutoConnectSSIDs) still exist for migration. MigrateFromLegacy builds
// an equivalent Automation from a legacy Rules value.
type Automation struct {
	// Defaults maps a tunnel name to the state its policy converges to
	// when NO rule matches ("connect" / "disconnect"). Written by the
	// none_match migration (see Normalize) and by the editor's Default
	// State control. A missing entry (or a tunnel with no rules) means
	// the engine never touches the tunnel on a no-match evaluation.
	Defaults map[string]Action `json:"default_state,omitempty"`
	// PerTunnel maps a tunnel name to its ordered rule list.
	PerTunnel map[string][]Rule `json:"per_tunnel_rules"`
}

// Rule is one condition→action pair. A rule carries one or more
// conditions, which are always combined with AND.
type Rule struct {
	// When lists the conditions that must hold for the rule to fire.
	When []Condition `json:"when"`
	// Match is retained for config compatibility. Conditions are always ANDed;
	// new configs use "all" and legacy configs with no value are equivalent.
	Match string `json:"match,omitempty"`
	Do    Action `json:"do"`
}

// UnmarshalJSON accepts both the legacy single-condition form
// ({"when": {"type": "ssid", ...}}) and the multi-condition array form
// ({"when": [{"type": "ssid", ...}, ...]}), so config.json files written
// before the multi-condition upgrade keep loading unchanged. The "match"
// field is retained for compatibility, but all conditions are ANDed.
func (r *Rule) UnmarshalJSON(data []byte) error {
	type plain struct {
		When  json.RawMessage `json:"when"`
		Match string          `json:"match,omitempty"`
		Do    Action          `json:"do"`
	}
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	r.Do = p.Do
	switch strings.ToLower(p.Match) {
	case "all", "and":
		r.Match = "all"
	default:
		r.Match = ""
	}
	t := bytes.TrimSpace(p.When)
	if len(t) == 0 || bytes.Equal(t, []byte("null")) {
		r.When = nil
		return nil
	}
	if t[0] == '[' {
		return json.Unmarshal(t, &r.When)
	}
	var one Condition
	if err := json.Unmarshal(t, &one); err != nil {
		return err
	}
	r.When = []Condition{one}
	return nil
}

// Action is what a matched rule does to its tunnel.
type Action string

const (
	ActionConnect    Action = "connect"
	ActionDisconnect Action = "disconnect"
)

// Condition types.
const (
	CondSSID      = "ssid"       // current Wi-Fi SSID equals SSID
	CondWiFi      = "wifi"       // current network is Wi-Fi (any SSID)
	CondSubnet    = "subnet"     // a physical-interface address is inside Subnet (CIDR)
	CondNetwork   = "network"    // the default gateway's MAC equals GatewayMAC
	CondGatewayIP = "gateway_ip" // the default gateway's IPv4 equals GatewayIP
	CondInterface = "interface"  // a physical (non-tunnel) interface name matches InterfaceName
	CondEthernet  = "ethernet"   // the machine is on a wired network (a non-Wi-Fi physical interface is up)
	CondTime      = "time"       // local time is inside [Start, End) on one of Days (empty = every day)
	CondNoneMatch = "none_match" // none of this tunnel's concrete conditions matched
)

// Condition is a single match predicate. Only the field relevant to Type
// is used.
type Condition struct {
	Type   string `json:"type"`
	SSID   string `json:"ssid,omitempty"`
	Subnet string `json:"subnet,omitempty"` // CIDR, e.g. "10.0.0.0/24"
	// GatewayMAC fingerprints a SPECIFIC network by its default-gateway
	// (router) MAC address — precise and medium-agnostic, so it
	// disambiguates two different networks that share a common subnet
	// like 192.168.0.0/24. Lower-cased colon form, e.g. "b0:38:6c:...".
	GatewayMAC string `json:"gateway_mac,omitempty"`
	// GatewayIP is the current default gateway's IPv4 address (e.g.
	// "192.168.0.1"). Medium-agnostic like GatewayMAC, but stable across
	// router hardware swaps on the same network.
	GatewayIP string `json:"gateway_ip,omitempty"`
	// InterfaceName is a physical interface name (e.g. "en0", "Ethernet").
	// Matched case-insensitively against the up, non-tunnel interfaces
	// that currently carry a routable address.
	InterfaceName string `json:"interface_name,omitempty"`
	// Start and End bound a local-time window in 24-hour "HH:MM" form.
	// End exclusive; an End before Start wraps past midnight (22:00–06:00
	// is "overnight"). Either may be empty (empty Start = 00:00, empty
	// End = 24:00), so a rule can be day-of-week only.
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
	// Days filters the window by weekday, 0=Sunday … 6=Saturday. Empty
	// means every day.
	Days []int `json:"days,omitempty"`
	// Label is a human-readable hint shown in the editor for a network
	// condition (e.g. "Office · 192.168.0.0/24"). Not used for matching.
	Label string `json:"label,omitempty"`
}

// InterfaceInfo describes one active physical interface. Used by the
// interface and ethernet conditions.
type InterfaceInfo struct {
	Name   string `json:"name"`
	IsWiFi bool   `json:"is_wifi"`
	Active bool   `json:"active"`
	// Type is a stable kind label (wifi / ethernet / bridge / virtual /
	// loopback / cellular / vpn) derived from the interface name and the
	// Wi-Fi flag. The automation editor's network-status board shows it as a
	// generic short label next to the raw name (e.g. "en0 · Wi-Fi"), matching
	// the Routes view's interface column. Empty when the kind can't be told.
	Type string `json:"type,omitempty"`
}

// NetworkContext is the current network state a rule set is evaluated
// against.
type NetworkContext struct {
	SSID string // current Wi-Fi SSID ("" when not on Wi-Fi / unknown)
	// PhysicalIPs are the IP addresses currently assigned to physical
	// (non-tunnel) interfaces. Used for subnet conditions.
	PhysicalIPs []net.IP
	// GatewayMAC is the current default gateway's MAC ("" if unknown).
	// Used for network conditions.
	GatewayMAC string
	// GatewayIP is the current default gateway's IPv4 address ("" if
	// unknown). Used for gateway_ip conditions.
	GatewayIP string
	// Interfaces are the physical (up, non-tunnel) interfaces currently
	// carrying a routable address. Used for interface and ethernet
	// conditions.
	Interfaces []InterfaceInfo
	// Now is the local time used by time conditions. Zero means "use
	// time.Now()", which keeps the matching code injectable in tests.
	Now time.Time
}

// DefaultAutomation returns an empty Automation with the maps initialised
// so JSON marshals to {} rather than null.
func DefaultAutomation() *Automation {
	return &Automation{
		Defaults:  make(map[string]Action),
		PerTunnel: make(map[string][]Rule),
	}
}

// DesiredState is the outcome of evaluating one tunnel's rules.
type DesiredState int

const (
	// StateUnmanaged means no rule applied — leave the tunnel exactly as
	// it is (never auto-touch it).
	StateUnmanaged DesiredState = iota
	StateConnect
	StateDisconnect
)

// Evaluate decides the desired state for a single tunnel given the
// current network context. Semantics:
//
//   - Rules are examined in order and the FIRST matching, well-formed
//     rule wins — uniformly, priority == position (issue #12). This is
//     what the editor's "top rule wins, drag to reorder" promises.
//   - Every condition within one rule must match (AND). Rules themselves are
//     alternatives (OR), with only the first matching rule selecting action.
//   - none_match ("else") is an unconditional match at its own position,
//     so it acts as a fallback when placed last and as an unconditional
//     override if dragged to the top — no special end-of-list handling.
//   - A rule with a malformed condition (bad CIDR/MAC, empty SSID) or an
//     unknown action never fires; it is skipped rather than defaulting to
//     connect. So an invalid rule fails closed (leaves the tunnel alone),
//     it doesn't silently connect.
//   - If nothing matches, the tunnel is Unmanaged (untouched).
//
// This lets the canonical workflow — "disconnect on the office network,
// connect everywhere else" — be expressed as
//
//	{when: [ssid=corp],  do: disconnect}
//	{when: [subnet=10/8], do: disconnect}
//	{when: [none_match],  do: connect}
//
// CONTROL path only: picks the ONE action the helper should enforce.
// The first matching rule (in list order) decides the outcome, but this walk
// continues through every rule. Rule MARKING uses EvaluateDetail to retain
// the per-rule results for shadowed and deprioritized rules.
func Evaluate(rules []Rule, ctx NetworkContext) DesiredState {
	state := StateUnmanaged
	selected := false
	for i := range rules {
		r := rules[i]
		if r.Validate() != nil {
			continue // malformed rule → can't fire
		}
		if !selected && r.matches(ctx) {
			state, _ = actionState(r.Do)
			selected = true
		}
	}
	return state
}

// EvaluatePolicy is the CONTROL entry point for the Default State +
// Ordered Rules policy model. It evaluates the ordered rule list (first
// match wins, see Evaluate) and, when NO rule matched but the tunnel HAS
// rules, converges to the tunnel's Default State instead of leaving it
// unmanaged:
//
//   - no rules at all → Unmanaged (no policy = automation never touches
//     the tunnel, matching pre-default-state behaviour);
//   - rules present, one matched → that rule's action;
//   - rules present, nothing matched → def (ActionConnect /
//     ActionDisconnect); an empty/unknown def fails closed as Unmanaged.
//
// `def` is the tunnel's entry in Automation.Defaults.
func EvaluatePolicy(rules []Rule, def Action, ctx NetworkContext) DesiredState {
	state := Evaluate(rules, ctx)
	if state == StateUnmanaged {
		// Default State applies with zero rules too — see
		// EvaluatePolicyDetail for why.
		switch def {
		case ActionConnect:
			return StateConnect
		case ActionDisconnect:
			return StateDisconnect
		}
	}
	return state
}

// EvaluatePolicyDetail is the MARKING counterpart of EvaluatePolicy for
// the editor's live preview: same rule-by-rule reporting as
// EvaluateDetail, with the no-match outcome resolved through the tunnel's
// Default State.
func EvaluatePolicyDetail(rules []Rule, def Action, ctx NetworkContext) (DesiredState, []RuleDetail) {
	state, details := EvaluateDetail(rules, ctx)
	if state == StateUnmanaged {
		// The Default State applies whenever no rule matched — including
		// the zero-rule case. A tunnel with an explicit Default State and
		// no rules has a well-defined policy ("always converge to the
		// default"), which is what makes a plain "default: connect"
		// tunnel come up at boot without any SSID rules. Only a tunnel
		// with NEITHER rules NOR a default stays Unmanaged (never
		// touched).
		switch def {
		case ActionConnect:
			return StateConnect, details
		case ActionDisconnect:
			return StateDisconnect, details
		}
	}
	return state, details
}

// Normalize migrates stored rule sets into the Default State model, in
// place and idempotently:
//
//   - The first none_match ("Otherwise") rule in a tunnel's list marks the
//     point where every later rule was unreachable under the old
//     first-match engine (none_match matched unconditionally). Its action
//     becomes the tunnel's Default State, it and everything after it are
//     removed.
//   - A tunnel with rules but no none_match and no explicit default gets
//     Default State = disconnect (the conservative "off unless told
//     otherwise" reading; old no-match behaviour was Unmanaged).
//   - Tunnels with no rules are left alone: no rules = no policy = the
//     engine never touches them.
//
// Existing explicit Defaults entries are never overwritten.
func (a *Automation) Normalize() {
	if a == nil {
		return
	}
	if a.Defaults == nil {
		a.Defaults = make(map[string]Action)
	}
	for name, rules := range a.PerTunnel {
		if len(rules) == 0 {
			continue
		}
		cut, def := len(rules), Action("")
		for i, r := range rules {
			if r.Validate() != nil {
				continue
			}
			if len(r.When) == 1 && r.When[0].Type == CondNoneMatch {
				def = r.Do
				cut = i
				break
			}
		}
		if cut < len(rules) {
			rules = rules[:cut]
			a.PerTunnel[name] = rules
		}
		if _, ok := a.Defaults[name]; !ok {
			if def != ActionConnect && def != ActionDisconnect {
				def = ActionDisconnect
			}
			a.Defaults[name] = def
		}
	}
}

// ConditionDetail reports one condition's match outcome, for the editor's
// live match indicators. Value is the condition's human-readable target
// (SSID / CIDR / MAC), empty for wifi and none_match.
type ConditionDetail struct {
	Type    string `json:"type"`
	Value   string `json:"value"`
	Matched bool   `json:"matched"`
}

// RuleDetail reports how one rule (and each of its conditions) fared
// against the current network context. Matched is the overall rule
// outcome after applying Match (AND/OR).
type RuleDetail struct {
	Do         Action            `json:"do"`
	MatchAll   bool              `json:"match_all"`
	Matched    bool              `json:"matched"`
	Conditions []ConditionDetail `json:"conditions"`
}

// EvaluateDetail is the MARKING path for the editor's live indicators,
// reporting how each rule and each condition fared against the context so
// the GUI does not re-implement the engine's matching in JS. Unlike
// Evaluate (control) it NEVER stops at the first match: every rule is
// judged — the first matching rule decides the executed action (later
// rules behind it stay matched-but-shadowed / deprioritized), while every
// remaining rule still gets its full per-condition report. The returned
// state is exactly what Evaluate would decide; the walk only ends after
// ALL rules are processed.
func EvaluateDetail(rules []Rule, ctx NetworkContext) (DesiredState, []RuleDetail) {
	details := make([]RuleDetail, 0, len(rules))
	state := StateUnmanaged
	for i := range rules {
		r := rules[i]
		detail := RuleDetail{Do: r.Do, MatchAll: true}
		if r.Validate() == nil {
			for _, c := range r.When {
				detail.Conditions = append(detail.Conditions, ConditionDetail{
					Type:    c.Type,
					Value:   conditionValue(c),
					Matched: ruleMatchesCond(c, ctx),
				})
			}
			detail.Matched = r.matches(ctx)
		}
		details = append(details, detail)
		if detail.Matched && state == StateUnmanaged {
			if st, ok := actionState(r.Do); ok {
				state = st
			}
		}
	}
	return state, details
}

// conditionValue returns the human-readable target of a condition for UI
// display ("" for wifi / ethernet / none_match, which have no value).
func conditionValue(c Condition) string {
	switch c.Type {
	case CondSSID:
		return c.SSID
	case CondSubnet:
		return c.Subnet
	case CondNetwork:
		return c.GatewayMAC
	case CondGatewayIP:
		return c.GatewayIP
	case CondInterface:
		return c.InterfaceName
	case CondTime:
		if c.Start == "" && c.End == "" {
			return "day-of-week"
		}
		if c.Start == "" {
			return "≤ " + c.End
		}
		if c.End == "" {
			return c.Start + " ≤"
		}
		return c.Start + "–" + c.End
	}
	return ""
}

// matches reports whether the rule's conditions (combined per Match)
// match the context. Callers validate first, so matches on malformed
// rules is undefined but never panics.
func (r Rule) matches(ctx NetworkContext) bool {
	if len(r.When) == 0 {
		return false
	}
	for i := range r.When {
		if !ruleMatchesCond(r.When[i], ctx) {
			return false
		}
	}
	return true
}

// actionState maps an action to its desired state, reporting ok=false for
// an unknown/empty action so callers can skip the rule instead of
// treating anything-but-disconnect as connect.
func actionState(a Action) (DesiredState, bool) {
	switch a {
	case ActionConnect:
		return StateConnect, true
	case ActionDisconnect:
		return StateDisconnect, true
	}
	return StateUnmanaged, false
}

// ruleMatchesCond reports whether a (pre-validated) condition matches the
// context. none_match is unconditional at its position.
func ruleMatchesCond(c Condition, ctx NetworkContext) bool {
	if c.Type == CondNoneMatch {
		return true
	}
	return conditionMatches(c, ctx)
}

// Validate reports whether the condition is well-formed. A malformed
// condition can never match, so save paths reject it (issue #12) and
// Evaluate skips it.
func (c Condition) Validate() error {
	switch c.Type {
	case CondSSID:
		if strings.TrimSpace(c.SSID) == "" {
			return fmt.Errorf("ssid condition requires a non-empty SSID")
		}
	case CondWiFi:
		// "any Wi-Fi" — no value needed; matches whenever the current
		// uplink is Wi-Fi (SSID is known).
	case CondSubnet:
		if _, _, err := net.ParseCIDR(strings.TrimSpace(c.Subnet)); err != nil {
			return fmt.Errorf("invalid subnet %q (want CIDR like 192.168.0.0/24)", c.Subnet)
		}
	case CondNetwork:
		if canonicalMAC(c.GatewayMAC) == "" {
			return fmt.Errorf("invalid gateway MAC %q (want 12 hex digits)", c.GatewayMAC)
		}
	case CondGatewayIP:
		if net.ParseIP(strings.TrimSpace(c.GatewayIP)) == nil {
			return fmt.Errorf("invalid gateway IP %q", c.GatewayIP)
		}
	case CondInterface:
		if strings.TrimSpace(c.InterfaceName) == "" {
			return fmt.Errorf("interface condition requires a non-empty interface name")
		}
	case CondEthernet:
		// no value needed; matches whenever a wired physical interface is up
	case CondTime:
		if err := validateTimeWindow(c); err != nil {
			return err
		}
	case CondNoneMatch:
		// always valid
	default:
		return fmt.Errorf("unknown condition type %q", c.Type)
	}
	return nil
}

// Validate checks a rule's action and every condition, plus the Match
// combiner. Used by the CLI and helper to reject bad rules on save rather
// than silently no-op'ing them.
func (r Rule) Validate() error {
	if _, ok := actionState(r.Do); !ok {
		return fmt.Errorf("unknown action %q (want connect or disconnect)", r.Do)
	}
	if r.Match != "" && r.Match != "all" {
		return fmt.Errorf("unknown match %q (want all or any)", r.Match)
	}
	if len(r.When) == 0 {
		return fmt.Errorf("rule requires at least one condition")
	}
	for i := range r.When {
		if err := r.When[i].Validate(); err != nil {
			return err
		}
	}
	return nil
}

// ValidateRule checks a rule's action and conditions. Kept as a package
// function for callers that pass rules around by value.
func ValidateRule(r Rule) error {
	return r.Validate()
}

// conditionMatches reports whether a concrete (ssid/wifi/subnet/network)
// condition matches the context. none_match is handled by ruleMatchesCond.
func conditionMatches(c Condition, ctx NetworkContext) bool {
	switch c.Type {
	case CondSSID:
		return ctx.SSID != "" && ssidEqual(c.SSID, ctx.SSID)
	case CondWiFi:
		return ctx.SSID != ""
	case CondSubnet:
		_, network, err := net.ParseCIDR(strings.TrimSpace(c.Subnet))
		if err != nil {
			return false
		}
		for _, ip := range ctx.PhysicalIPs {
			if network.Contains(ip) {
				return true
			}
		}
	case CondNetwork:
		want := canonicalMAC(c.GatewayMAC)
		got := canonicalMAC(ctx.GatewayMAC)
		return want != "" && want == got
	case CondGatewayIP:
		want := net.ParseIP(strings.TrimSpace(c.GatewayIP))
		got := net.ParseIP(ctx.GatewayIP)
		return want != nil && got != nil && want.Equal(got)
	case CondInterface:
		want := strings.TrimSpace(c.InterfaceName)
		if want == "" {
			return false
		}
		for _, inf := range ctx.Interfaces {
			if strings.EqualFold(inf.Name, want) {
				return true
			}
		}
		return false
	case CondEthernet:
		// A wired uplink exists when some physical interface that is NOT
		// Wi-Fi currently carries a routable address. On single-homed
		// machines this is exactly "not on Wi-Fi"; on multi-homed boxes a
		// wired connection (even alongside Wi-Fi) counts — the user IS on
		// Ethernet, so "disconnect on ethernet" should fire.
		for _, inf := range ctx.Interfaces {
			if !inf.IsWiFi {
				return true
			}
		}
		return false
	case CondTime:
		return timeMatches(c, ctx.Now)
	}
	return false
}

// validateTimeWindow reports whether a time condition carries a usable
// window. At least one of Start / End / Days must be set, and any clock
// value present must parse as 24-hour HH:MM.
func validateTimeWindow(c Condition) error {
	if strings.TrimSpace(c.Start) == "" && strings.TrimSpace(c.End) == "" && len(c.Days) == 0 {
		return fmt.Errorf("time condition requires a start, end, or day-of-week")
	}
	if _, err := parseClock(c.Start); err != nil {
		return fmt.Errorf("invalid start time %q (want HH:MM, 24h): %w", c.Start, err)
	}
	if _, err := parseClock(c.End); err != nil {
		return fmt.Errorf("invalid end time %q (want HH:MM, 24h): %w", c.End, err)
	}
	for _, d := range c.Days {
		if d < 0 || d > 6 {
			return fmt.Errorf("invalid day-of-week %d (0=Sunday … 6=Saturday)", d)
		}
	}
	return nil
}

// parseClock parses "HH:MM" (24h) into minutes since midnight. An empty
// string is allowed and yields -1. Anything else must be a valid clock.
func parseClock(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return -1, nil
	}
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return -1, fmt.Errorf("want HH:MM")
	}
	h, errH := strconv.Atoi(parts[0])
	m, errM := strconv.Atoi(parts[1])
	if errH != nil || errM != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return -1, fmt.Errorf("want HH:MM")
	}
	return h*60 + m, nil
}

// timeMatches evaluates a time condition against the (possibly injected)
// local clock. Semantics:
//
//   - Day filter first: if Days is non-empty the weekday must be listed.
//   - Then the window: empty Start = 00:00, empty End = 24:00. End is
//     exclusive. Start > End wraps past midnight.
//   - With no window at all (day-of-week-only rule) any time matches.
func timeMatches(c Condition, now time.Time) bool {
	if now.IsZero() {
		now = time.Now()
	}
	if len(c.Days) > 0 {
		wd := int(now.Weekday()) // 0=Sunday … 6=Saturday
		dayOK := false
		for _, d := range c.Days {
			if d == wd {
				dayOK = true
				break
			}
		}
		if !dayOK {
			return false
		}
	}
	start, errS := parseClock(c.Start)
	end, errE := parseClock(c.End)
	if errS != nil || errE != nil {
		return false // malformed clocks were validated on save; be safe
	}
	if start < 0 && end < 0 {
		return true // day-of-week only
	}
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = 24 * 60
	}
	mins := now.Hour()*60 + now.Minute()
	if start <= end {
		return mins >= start && mins < end
	}
	return mins >= start || mins < end // overnight wrap
}

// canonicalMAC reduces a MAC to its bare lower-case hex digits so that
// values differing only in separator (":" vs "-" vs none) or case
// compare equal — users paste MACs in every style. Returns "" when there
// aren't exactly 12 hex digits (malformed / empty), which never matches.
func canonicalMAC(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f':
			b.WriteRune(r)
		case r >= 'A' && r <= 'F':
			b.WriteRune(r + ('a' - 'A'))
		}
	}
	hex := b.String()
	if len(hex) != 12 {
		return ""
	}
	return hex
}

// PolicyTunnelNames returns every tunnel the engine has a POLICY for: the
// union of the rule lists (PerTunnel) and the Default State map
// (Defaults), sorted. A tunnel that appears only in Defaults still carries
// a complete policy — "always converge to my Default State" — so it must be
// evaluated (and listed by the preview) even though it has no rules.
// Iterating PerTunnel alone silently ignores those tunnels, which is how a
// plain "default: connect" tunnel failed to come up.
func (a *Automation) PolicyTunnelNames() []string {
	if a == nil {
		return nil
	}
	names := make([]string, 0, len(a.PerTunnel)+len(a.Defaults))
	seen := make(map[string]struct{}, len(a.PerTunnel)+len(a.Defaults))
	for n := range a.PerTunnel {
		if _, dup := seen[n]; dup {
			continue
		}
		seen[n] = struct{}{}
		names = append(names, n)
	}
	for n := range a.Defaults {
		if _, dup := seen[n]; dup {
			continue
		}
		seen[n] = struct{}{}
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// TunnelNames returns the rule set's tunnel names in deterministic
// (sorted) order.
func (a *Automation) TunnelNames() []string {
	names := make([]string, 0, len(a.PerTunnel))
	for n := range a.PerTunnel {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// MigrateFromLegacy builds an Automation equivalent to a legacy Rules
// value, so existing users keep working after the model change:
//
//   - each tunnel's AutoConnectSSIDs → {when: [ssid=X], do: connect}
//   - global TrustedSSIDs → for every tunnel that has any rule, a
//     {when: [ssid=Y], do: disconnect} placed BEFORE its connect rules so
//     "trusted" wins over "auto-connect" on an overlapping network
//     (matching the legacy precedence where trusted was checked first).
//
// Trusted SSIDs are only meaningful relative to a tunnel that could
// otherwise be connected, so they're attached to tunnels that have
// auto-connect rules; a tunnel with no legacy rules gets none.
func MigrateFromLegacy(legacy *Rules) *Automation {
	out := DefaultAutomation()
	if legacy == nil {
		return out
	}
	names := make([]string, 0, len(legacy.PerTunnel))
	for n := range legacy.PerTunnel {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		var connectRules []Rule
		for _, ssid := range legacy.PerTunnel[name].AutoConnectSSIDs {
			if ssid == "" {
				continue
			}
			connectRules = append(connectRules, Rule{
				When: []Condition{{Type: CondSSID, SSID: ssid}},
				Do:   ActionConnect,
			})
		}
		// Trusted SSIDs only ever affected auto-managed tunnels in the
		// legacy model, i.e. tunnels with an auto-connect list. Don't
		// attach trusted-disconnect rules to a tunnel that had no
		// connect rules — that would newly disconnect it on a trusted
		// network, which legacy never did.
		if len(connectRules) == 0 {
			continue
		}
		var rules []Rule
		// Trusted (disconnect) rules first, so they take precedence over
		// the connect rules on an overlapping network.
		for _, ssid := range legacy.TrustedSSIDs {
			if ssid == "" {
				continue
			}
			rules = append(rules, Rule{
				When: []Condition{{Type: CondSSID, SSID: ssid}},
				Do:   ActionDisconnect,
			})
		}
		rules = append(rules, connectRules...)
		// NOTE: we deliberately do NOT synthesize a none_match→disconnect
		// rule here. Legacy auto-connect implicitly disconnected the
		// tunnel when you left its Wi-Fi zone, but that behaviour was
		// coarse (Wi-Fi-transition only, auto-managed only) and, ported
		// literally into the new any-network-change engine, would
		// aggressively tear down tunnels on Ethernet or after a manual
		// connect. Migration therefore translates only what the user
		// EXPLICITLY configured (connect on SSID, disconnect on trusted
		// SSID); a user who wants "off when I leave" adds that rule
		// explicitly in the Automation editor, alongside separate
		// connect/disconnect conditions.
		out.PerTunnel[name] = rules
	}
	return out
}
