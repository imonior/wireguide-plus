package wifi

import (
	"sort"
)

// Rules defines WiFi auto-connect behavior. The model is per-tunnel:
// each tunnel owns the list of SSIDs that should auto-activate it.
// The "trusted" list is a global override that disconnects auto-managed
// tunnels when joining those networks.
type Rules struct {
	TrustedSSIDs []string               `json:"trusted_ssids"` // override: VPN off on these networks
	PerTunnel    map[string]TunnelSSIDs `json:"per_tunnel"`    // keyed by tunnel name
}

// TunnelSSIDs holds the per-tunnel auto-connect list. Wrapped in a
// struct (rather than just []string) so future per-tunnel fields can
// be added without changing the JSON shape.
type TunnelSSIDs struct {
	AutoConnectSSIDs []string `json:"auto_connect_ssids"`
}

// Action determines what to do when the system joins the given SSID.
// Returns:
//
//	"disconnect", ""            — SSID is trusted, drop auto-managed tunnels
//	"connect",    "tunnel-name" — SSID matches a tunnel's auto-connect list
//	"none",       ""            — no rule applies
//
// When multiple tunnels would match the same SSID, the lexicographically
// first tunnel wins. Sorting yields deterministic behavior across runs
// and makes the choice predictable for the user.
func (r *Rules) Action(ssid string) (action string, tunnelName string) {
	if ssid == "" {
		return "none", ""
	}
	// SSID matching is EXACT: full name, case-sensitive (issue: "ssid比较
	// 应该做全名匹配"). The 802.11 standard defines an SSID as a byte
	// string, so "MyWifi" and "mywifi" are genuinely different networks,
	// and a middle space or special character must count. The editor's
	// live match indicators make any mismatch visible immediately, so
	// strict matching no longer surprises users who mistype.
	for _, trusted := range r.TrustedSSIDs {
		if ssidEqual(trusted, ssid) {
			return "disconnect", ""
		}
	}
	names := make([]string, 0, len(r.PerTunnel))
	for n := range r.PerTunnel {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, s := range r.PerTunnel[name].AutoConnectSSIDs {
			if ssidEqual(s, ssid) {
				return "connect", name
			}
		}
	}
	return "none", ""
}

// ssidEqual compares two SSIDs EXACTLY (full name, case-sensitive).
// An SSID is a byte string per 802.11: case, middle spaces and special
// characters are all significant. The editor's live preview shows the
// mismatch immediately when the typed name differs from the broadcast
// one, so strict comparison is safe.
func ssidEqual(a, b string) bool {
	return a == b
}
