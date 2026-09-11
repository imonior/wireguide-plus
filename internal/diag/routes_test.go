package diag

import "testing"

func TestExpandDarwinNetAddr(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		// macOS netstat compresses the trailing ".0" octets of network
		// routes; the parser must expand them back to canonical form.
		{"127", "127.0.0.0/8"},
		{"169.254", "169.254.0.0/16"},
		{"192.168.1", "192.168.1.0/24"},
		{"10", "10.0.0.0/8"},
		// Classless routes (the ones a split-tunnel VPN installs) print
		// their prefix but still elide the address: "2/7" is 2.0.0.0/7.
		// These used to reach the diagnostics UI looking truncated.
		{"2/7", "2.0.0.0/7"},
		{"4/6", "4.0.0.0/6"},
		{"128.0/1", "128.0.0.0/1"},
		{"10.20.20/24", "10.20.20.0/24"},
		// Already-canonical or non-network entries pass through.
		{"127.0.0.1", "127.0.0.1"},
		{"255.255.255.255", "255.255.255.255"},
		{"default", "default"},
		{"fe80::1%en0", "fe80::1%en0"},
		{"link#4", "link#4"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := expandDarwinNetAddr(tt.in); got != tt.want {
			t.Errorf("expandDarwinNetAddr(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestStripIPv6Zone(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		// macOS netstat embeds the interface name in IPv6 addresses;
		// neither Destination nor Gateway may ever show it.
		{"fe80::%utun0", "fe80::"},
		{"fe80::1%lo0", "fe80::1"},
		{"fe80::%lo0/64", "fe80::/64"},
		{"2001:db8::1%en0/128", "2001:db8::1/128"},
		// Already clean values pass through.
		{"fe80::1", "fe80::1"},
		{"fe80::/64", "fe80::/64"},
		{"10.0.0.1", "10.0.0.1"},
		{"link#4", "link#4"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := stripIPv6Zone(tt.in); got != tt.want {
			t.Errorf("stripIPv6Zone(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseDarwinRouteOutput(t *testing.T) {
	// Mirrors the scheme's expected macOS output: a real next-hop gateway
	// is shown, on-link routes get an empty Gateway (UI → "—"), and
	// neighbor / loopback / multicast / local-host routes are filtered out.
	in := `Routing tables

Internet:
Destination        Gateway            Flags        Netif Expire
default            10.20.20.20        UGScg           en0
10.20.20/24        link#11            UCS             en0
10.20.20.168       link#11            UH              en0
10.20.20.20        52:54:0:25:1a:7d UHLWIir         en0   1190
10.20.20.25        3c:5a:4f:2b:1c:9d UHLWI           en0
10.30.30/24        link#23            UCS             utun4
10.30.35/24        link#23            UCS             utun4
10.40.40/24        3c:5a:4f:2b:1c:9d UCS             en1
127.0.0.0/8        link#4             UCS             lo0
127.0.0.1          lo0                UH              lo0
224.0.0.0/4        lo0                UCS             lo0
`
	addrs := map[string]ifaceAddr{
		"en0":   {v4: "10.20.20.168", v6: "fe80::1"}, // host's own address
		"lo0":   {v4: "127.0.0.1", v6: "::1"},
		"utun4": {v4: "10.30.30.2", v6: "fd00::2"},
	}
	types := map[string]string{"en0": "Wi-Fi"}
	routes := parseDarwinRouteOutput(in, 4, addrs, types)

	want := []RouteEntry{
		// Real next hop shown; "default" stays "default".
		{Destination: "default", Gateway: "10.20.20.20", Flags: "UGScg", Interface: "en0", InterfaceType: "wifi", InterfaceDetail: "Wi-Fi"},
		// On-link route → empty Gateway (UI renders "—"); BSD shorthand
		// "10.20.20/24" canonicalized to "10.20.20.0/24".
		{Destination: "10.20.20.0/24", Gateway: "", Flags: "UCS", Interface: "en0", InterfaceType: "wifi", InterfaceDetail: "Wi-Fi"},
		// VPN tunnel routes: on-link via utun4 → empty Gateway.
		{Destination: "10.30.30.0/24", Gateway: "", Flags: "UCS", Interface: "utun4", InterfaceType: "vpn"},
		{Destination: "10.30.35.0/24", Gateway: "", Flags: "UCS", Interface: "utun4", InterfaceType: "vpn"},
		// On some systems a directly-connected LAN subnet prints its next
		// hop as a MAC (no 'H' host flag): the route is kept, but the MAC
		// is filtered from the Gateway column → empty (UI renders "—").
		{Destination: "10.40.40.0/24", Gateway: "", Flags: "UCS", Interface: "en1", InterfaceType: "ethernet"},
	}
	if len(routes) != len(want) {
		t.Fatalf("parsed %d routes, want %d:\n%+v", len(routes), len(want), routes)
	}
	for i := range want {
		if routes[i] != want[i] {
			t.Errorf("route[%d] = %+v, want %+v", i, routes[i], want[i])
		}
	}
}

// The inet6 table keeps "default" as "default" and shows a real v6 next hop;
// on-link v6 routes get an empty Gateway.
func TestParseDarwinRouteOutputV6(t *testing.T) {
	in := `Internet6:
Destination                             Gateway                         Flags         Netif Expire
default                                 fe80::1%en0                     UGcIg           en0
2001:db8:1::/64                         link#4                         UCS             en0
`
	addrs := map[string]ifaceAddr{
		"en0": {v4: "192.168.1.10", v6: "2001:db8::5"},
	}
	// types=nil mirrors networksetup failing: the name heuristic still
	// classifies en0 as ethernet.
	routes := parseDarwinRouteOutput(in, 6, addrs, nil)

	want := []RouteEntry{
		// The v6 default's gateway "fe80::1%en0" loses its zone id —
		// the Gateway column must never carry an interface name.
		{Destination: "default", Gateway: "fe80::1", Flags: "UGcIg", Interface: "en0", InterfaceType: "ethernet"},
		// v6 family link#N is on-link: empty Gateway.
		{Destination: "2001:db8:1::/64", Gateway: "", Flags: "UCS", Interface: "en0", InterfaceType: "ethernet"},
	}
	if len(routes) != len(want) {
		t.Fatalf("parsed %d routes, want %d:\n%+v", len(routes), len(want), routes)
	}
	for i := range want {
		if routes[i] != want[i] {
			t.Errorf("route[%d] = %+v, want %+v", i, routes[i], want[i])
		}
	}
}

// The Gateway column shows a genuine L3 next-hop IP and nothing else: a real
// IP passes through (zone stripped); link tokens / interface names / MAC
// addresses / the Windows unspecified sentinel all map to empty (UI → "—").
func TestResolveOnLinkGatewayFallback(t *testing.T) {
	// Genuine next-hop IP → shown as-is.
	if got := resolveOnLinkGateway("192.168.1.1"); got != "192.168.1.1" {
		t.Errorf("real next-hop gw = %q, want passthrough", got)
	}
	// A genuine v6 next hop (router) → shown, zone stripped.
	if got := resolveOnLinkGateway("fe80::9%en0"); got != "fe80::9" {
		t.Errorf("real v6 next-hop gw = %q, want stripped IP", got)
	}
	// link#N → no next hop → empty.
	if got := resolveOnLinkGateway("link#4"); got != "" {
		t.Errorf("link# gw = %q, want empty (no next hop)", got)
	}
	// Windows "On-Link" → empty.
	if got := resolveOnLinkGateway("On-Link"); got != "" {
		t.Errorf("On-Link gw = %q, want empty", got)
	}
	// Bare interface NAME macOS prints for direct routes → empty.
	if got := resolveOnLinkGateway("utun4"); got != "" {
		t.Errorf("named-iface gw = %q, want empty (no next hop)", got)
	}
	// MAC address on directly-attached Ethernet → empty.
	if got := resolveOnLinkGateway("aa:bb:cc:dd:ee:ff"); got != "" {
		t.Errorf("MAC gw = %q, want empty (neighbor, not a route)", got)
	}
	// Windows unspecified sentinel → empty.
	if got := resolveOnLinkGateway("0.0.0.0"); got != "" {
		t.Errorf("0.0.0.0 gw = %q, want empty (unspecified)", got)
	}
	// Empty → empty.
	if got := resolveOnLinkGateway(""); got != "" {
		t.Errorf("empty gw = %q, want empty", got)
	}
}

func TestShouldKeepRoute(t *testing.T) {
	addrs := map[string]ifaceAddr{
		"en0": {v4: "10.20.20.168", v6: "fe80::1"},
		"lo0": {v4: "127.0.0.1", v6: "::1"},
	}
	tests := []struct {
		dest, iface string
		family      int
		want        bool
	}{
		// Real forwarding routes are kept.
		{"default", "en0", 4, true},
		{"10.20.20.0/24", "en0", 4, true},
		{"10.30.30.0/24", "utun4", 4, true},
		{"0.0.0.0/0", "en0", 4, true},
		{"2001:db8:1::/64", "en0", 6, true},
		// Legitimate /32 (WireGuard peer) whose dest is NOT the local
		// address is kept — the scheme forbids filtering by prefix length.
		{"10.30.30.5/32", "utun4", 4, true},
		// Loopback / multicast / broadcast are dropped.
		{"127.0.0.0/8", "lo0", 4, false},
		{"127.0.0.1", "lo0", 4, false},
		{"224.0.0.0/4", "lo0", 4, false},
		{"255.255.255.255/32", "en0", 4, false},
		{"ff02::/16", "en0", 6, false},
		// The box's own local host route (dest == interface address) is
		// dropped, whether written as a bare host IP or a /32.
		{"10.20.20.168", "en0", 4, false},
		{"10.20.20.168/32", "en0", 4, false},
	}
	for _, tt := range tests {
		if got := shouldKeepRoute(tt.dest, tt.iface, tt.family, addrs); got != tt.want {
			t.Errorf("shouldKeepRoute(%q, %q, %d) = %v, want %v", tt.dest, tt.iface, tt.family, got, tt.want)
		}
	}
}

func TestIsMAC(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"3c:5a:4f:2b:1c:9d", true},
		{"AA:BB:CC:DD:EE:FF", true},
		{"00:00:00:00:00:00", true},
		// macOS elides the leading zero of a single-digit octet — these
		// are valid MACs and MUST be detected (the single-digit-octet bug
		// that let neighbor entries leak into the route table).
		{"52:54:0:25:1a:7d", true},
		{"a4:fc:77:1:50:2a", true},
		{"link#4", false},
		{"utun4", false},
		{"fe80::1%en0", false},
		{"192.168.1.1", false},
		{"ab:cd:ef", false},
		{"ab:cd:ef:gh:ij:kl", false}, // non-hex octet
		{"", false},
	}
	for _, tt := range tests {
		if got := isMAC(tt.in); got != tt.want {
			t.Errorf("isMAC(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestClassifyIface(t *testing.T) {
	tests := []struct {
		name, port   string
		want, detail string
	}{
		// macOS: the hardware port string is authoritative — en0 is
		// Wi-Fi on laptops but Ethernet on desktops. The port name is
		// ALSO the detail, original case: the Routes badge and the
		// tunnel editor's egress dropdown (same networksetup source)
		// then show the identical string.
		{"en0", "Wi-Fi", "wifi", "Wi-Fi"},
		{"en0", "Ethernet", "ethernet", "Ethernet"},
		{"en5", "USB 10/100/1000 LAN", "ethernet", "USB 10/100/1000 LAN"},
		{"bridge0", "Thunderbolt Bridge", "ethernet", "Thunderbolt Bridge"}, // "thunderbolt" matches before "bridge"
		{"utun4", "", "vpn", ""},
		{"wg0", "", "vpn", ""},
		{"lo0", "", "loopback", ""},
		{"awdl0", "", "virtual", "AWDL"},
		{"bridge100", "", "bridge", ""},
		// Name heuristics when no port string exists (Linux/Windows).
		{"wlp3s0", "", "wifi", ""},
		{"wlan0", "", "wifi", ""},
		{"eth0", "", "ethernet", ""},
		{"enp0s31f6", "", "ethernet", ""},
		{"enigma0", "", "ethernet", ""}, // plain "en" prefix heuristic — best effort, documented
		// Well-known software attribution ("VPN · Tailscale" etc).
		{"tailscale0", "", "vpn", "Tailscale"},
		{"Tailscale Tunnel", "", "vpn", "Tailscale"},
		{"ztalyh6vqk", "", "vpn", "ZeroTier"},
		{"proton0", "", "vpn", "ProtonVPN"},
		{"nordlynx", "", "vpn", "NordVPN"},
		{"CloudflareWARP", "", "vpn", "Cloudflare WARP"},
		{"WireGuard Tunnel", "", "vpn", "WireGuard"},
		{"WireGuide", "", "vpn", "WireGuard"},
		{"docker0", "", "virtual", "Docker"},
		{"veth8a2c1f", "", "virtual", "Docker"},
		{"br-a1b2c3d4", "", "bridge", "Docker"},
		{"orbstack0", "", "virtual", "OrbStack"},
		{"virbr0", "", "virtual", "libvirt"},
		{"vEthernet (WSL)", "", "virtual", "WSL"},
		{"vEthernet (Default Switch)", "", "virtual", "Hyper-V"},
		{"Clash", "", "vpn", "Clash"},
		{"utun6", "", "vpn", ""}, // anonymous macOS utun — no invented detail
	}
	for _, tt := range tests {
		kind, detail := classifyIface(tt.name, tt.port)
		if kind != tt.want || detail != tt.detail {
			t.Errorf("classifyIface(%q, %q) = (%q, %q), want (%q, %q)",
				tt.name, tt.port, kind, detail, tt.want, tt.detail)
		}
	}
}
