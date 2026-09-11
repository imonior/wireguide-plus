package diag

import (
	"context"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/imonior/wireguide-plus/internal/sysexec"
)

// routeCmdTimeout bounds the route-table-listing commands. These are called
// from the diagnostics UI; a hung command would freeze the helper.
const routeCmdTimeout = 10 * time.Second

func runRouteCmd(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), routeCmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	sysexec.Hide(cmd)
	return cmd.CombinedOutput()
}

// RouteEntry represents a single routing table entry.
type RouteEntry struct {
	Destination string `json:"destination"`
	Gateway     string `json:"gateway"`
	Interface   string `json:"interface"`
	Flags       string `json:"flags"`
	// InterfaceType is a stable kind label for the Interface column —
	// one of wifi / ethernet / vpn / loopback / bridge / virtual /
	// cellular, empty when the platform cannot tell. The UI translates
	// the kind, so locale-specific strings never cross the IPC boundary.
	InterfaceType string `json:"interface_type,omitempty"`
	// InterfaceDetail is the specific label behind the interface: the
	// macOS hardware port name when known ("Wi-Fi", "USB 10/100/1000
	// LAN" — identical to the tunnel editor's egress dropdown), else
	// the creator software ("Tailscale", "Docker", "AWDL", …) when the
	// name identifies it. Product/port names are proper nouns and
	// intentionally NOT localized. Shown as the badge text, with the
	// generic kind kept in the tooltip.
	InterfaceDetail string `json:"interface_detail,omitempty"`
	// IsVPN is set by the app layer when this route's interface matches
	// a currently active tunnel interface. The diagnostics UI uses it to
	// distinguish "through the tunnel" routes from direct ones.
	IsVPN bool `json:"is_vpn,omitempty"`
}

// GetRoutingTable returns the current OS routing table.
func GetRoutingTable() ([]RouteEntry, error) {
	switch runtime.GOOS {
	case "darwin":
		return getRoutesDarwinFull()
	case "linux":
		return getRoutesLinuxFull()
	case "windows":
		return getRoutesWindowsFull()
	default:
		return nil, nil
	}
}

func getRoutesDarwinFull() ([]RouteEntry, error) {
	// Run both `inet` and `inet6` so IPv6 routes (Tailscale, full
	// IPv6 tunnels, ULA prefixes) show up in diagnostics. Without
	// `-f inet6` an IPv6-only tunnel was completely invisible.
	addrs := buildIfaceAddrs()
	types := darwinHardwarePorts()
	v4, err := runRouteCmd("netstat", "-rn", "-f", "inet")
	if err != nil {
		return nil, err
	}
	v6, err := runRouteCmd("netstat", "-rn", "-f", "inet6")
	if err != nil {
		// Non-fatal: IPv6 may be disabled on this system. Return
		// just the v4 routes rather than the whole call failing.
		return parseDarwinRouteOutput(string(v4), 4, addrs, types), nil
	}
	routes := parseDarwinRouteOutput(string(v4), 4, addrs, types)
	routes = append(routes, parseDarwinRouteOutput(string(v6), 6, addrs, types)...)
	return routes, nil
}

func parseDarwinRouteOutput(out string, family int, addrs map[string]ifaceAddr, types map[string]string) []RouteEntry {
	var routes []RouteEntry
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		// Skip header / banner lines (`Internet:`, `Internet6:`, etc.)
		if fields[0] == "Destination" || fields[0] == "Routing" ||
			strings.HasPrefix(fields[0], "Internet") {
			continue
		}
		entry := RouteEntry{
			// "default" stays "default"; macOS netstat's elided octets
			// ("127", "192.168.1") expand back to full CIDR. The zone id
			// is stripped so the Destination column is a pure address
			// ("fe80::%lo0/64" → "fe80::/64") — the Interface column
			// already names the interface.
			Destination: stripIPv6Zone(darwinDestination(fields[0])),
		}
		if len(fields) > 2 {
			entry.Flags = fields[2]
		}
		if len(fields) > 3 {
			entry.Interface = fields[3]
			entry.InterfaceType, entry.InterfaceDetail = classifyIface(fields[3], types[fields[3]])
		}
		// The Gateway column shows a real L3 next hop, or "" (→ "—"). A
		// directly-attached LAN route prints its next hop as a MAC address
		// (or "link#N") on some systems; that MAC is filtered out of the
		// Gateway column and the route itself is kept (on-link → "—"). The
		// Interface column already names the egress interface, so the
		// Gateway reads "—" rather than a non-forwarding MAC value.
		entry.Gateway = resolveOnLinkGateway(fields[1])
		// Pure ARP / neighbor entries carry the resolved MAC as the gateway
		// and are flagged 'H' (host); they are not forwarding routes, so
		// drop the whole row. A subnet route can also show a MAC gateway,
		// but it lacks the 'H' flag and so is kept above with an empty
		// Gateway.
		if isMAC(fields[1]) && len(fields) > 2 && strings.Contains(fields[2], "H") {
			continue
		}
		// Keep only real unicast forwarding routes: loopback, multicast,
		// broadcast and the box's own local host route are filtered out.
		if !shouldKeepRoute(entry.Destination, entry.Interface, family, addrs) {
			continue
		}
		routes = append(routes, entry)
	}
	return routes
}

// darwinDestination canonicalizes the Destination column. "default" stays the
// descriptive word "default" (the egress Interface column already names the
// interface, so "default" reads better than a synthetic 0.0.0.0/0 / ::/0).
// Everything else goes through the elided-octet expander so macOS BSD
// shorthand ("10.20.20/24") becomes standard CIDR ("10.20.20.0/24").
func darwinDestination(s string) string {
	if s == "default" {
		return "default"
	}
	return expandDarwinNetAddr(s)
}

// ifaceAddr pairs an interface name with one IPv4 and one IPv6 address.
// It is used purely to recognize and suppress the box's own local host
// route (a /32 whose destination equals this interface's address is not a
// forwarding route and must not appear in the table).
type ifaceAddr struct {
	v4, v6 string
}

// buildIfaceAddrs snapshots every interface's addresses by name. Best
// effort: an interface that fails to enumerate simply resolves nothing.
func buildIfaceAddrs() map[string]ifaceAddr {
	out := make(map[string]ifaceAddr)
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, ifc := range ifaces {
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		a := ifaceAddr{}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP
			if v4 := ip.To4(); v4 != nil {
				if a.v4 == "" {
					a.v4 = v4.String()
				}
				continue
			}
			if ip.To16() == nil {
				continue
			}
			// Prefer a global v6 over a link-local; fall back to
			// whichever came first.
			if a.v6 == "" || (ip.IsGlobalUnicast() && net.ParseIP(a.v6).IsLinkLocalUnicast()) {
				a.v6 = ip.String()
			}
		}
		out[ifc.Name] = a
	}
	return out
}

// resolveOnLinkGateway returns the value the Gateway column should display.
// The column is a pure L3 next hop. A genuine next-hop IP is shown as-is
// (zone id stripped); everything that is NOT an explicit next hop maps to an
// empty string, which the UI renders as "—":
//
//   - link-layer tokens ("link#4", "link", "On-link");
//   - the bare interface NAME macOS prints for direct routes
//     ("10.30.30.0/24" via "utun4") — the interface itself is the egress,
//     so there is no separate next hop;
//   - the link-layer MAC address macOS prints on directly-attached Ethernet
//     (a neighbor/ARP entry, never a gateway);
//   - the unspecified sentinel Windows uses for on-link routes ("0.0.0.0").
//
// Per the routing-data contract: the Gateway column never carries a MAC, an
// interface name, or the host's own address — the Interface column already
// names the egress interface.
func resolveOnLinkGateway(gw string) string {
	if gw == "" {
		return ""
	}
	// macOS prints IPv6 addresses with an embedded zone id ("fe80::1%en0").
	// Strip it up front so the IP check below sees a clean address and the
	// Gateway column never carries an interface name.
	gwClean := stripIPv6Zone(gw)
	ip := net.ParseIP(gwClean)
	if ip == nil {
		return "" // not an address → link token / iface name / MAC → no next hop
	}
	// The unspecified address is Windows' "no next hop" sentinel — never a
	// real gateway.
	if ip.IsUnspecified() {
		return ""
	}
	return gwClean
}

// isMAC reports whether s is a colon-separated link-layer (MAC) address,
// the form macOS netstat prints for neighbor / ARP entries in the Gateway
// column. Each octet is 1–2 hex digits — macOS (and ifconfig) elide the
// leading zero, so "52:54:0:25:1a:7d" is a valid MAC, not a malformed one.
// A non-hex octet, a wrong field count, or an out-of-range length returns
// false so IPv6 addresses ("fe80::1%en0") are never mistaken for MACs.
func isMAC(s string) bool {
	parts := strings.Split(s, ":")
	if len(parts) != 6 {
		return false
	}
	const hex = "0123456789abcdefABCDEF"
	for _, p := range parts {
		if len(p) < 1 || len(p) > 2 {
			return false
		}
		for _, r := range p {
			if !strings.ContainsRune(hex, r) {
				return false
			}
		}
	}
	return true
}

// shouldKeepRoute decides whether a canonicalized route belongs in the
// user-facing unicast routing table. Per the routing-data contract, the
// following are NOT forwarding routes and are dropped:
//
//   - loopback (127.0.0.0/8, ::1/128);
//   - multicast (224.0.0.0/4, ff00::/8);
//   - the broadcast address (255.255.255.255/32);
//   - the box's own local host route — a /32 (or /128) whose destination
//     equals this interface's own address, or a bare host IP equal to it.
//
// Legitimate /32 routes (e.g. a WireGuard peer) whose destination is NOT the
// local address are kept. "default" and any unrecognized token parse as keep.
func shouldKeepRoute(dest, iface string, family int, addrs map[string]ifaceAddr) bool {
	// A bare host address (no prefix) — macOS prints host routes this way.
	// Exclude loopback and the box's own address; keep everything else.
	if ip := net.ParseIP(dest); ip != nil {
		if ip.IsLoopback() {
			return false
		}
		if own := addrs[iface]; family == 6 {
			if own.v6 != "" && ip.Equal(net.ParseIP(own.v6)) {
				return false
			}
		} else if own.v4 != "" && ip.Equal(net.ParseIP(own.v4)) {
			return false
		}
		return true
	}
	ip, ipNet, err := net.ParseCIDR(dest)
	if err != nil {
		return true // "default" or an unrecognized token — keep
	}
	if ip.IsLoopback() || ip.IsMulticast() || ip.Equal(net.IPv4bcast) {
		return false
	}
	if ones, _ := ipNet.Mask.Size(); ones == 32 || ones == 128 {
		if own := addrs[iface]; family == 6 {
			if own.v6 != "" && ip.Equal(net.ParseIP(own.v6)) {
				return false
			}
		} else if own.v4 != "" && ip.Equal(net.ParseIP(own.v4)) {
			return false
		}
	}
	return true
}

// stripIPv6Zone removes the zone id ("%utun0", "%lo0") that macOS (and
// occasionally Linux) print inside IPv6 addresses. The Interface column
// already names the interface, and both the Destination and Gateway
// columns must be pure addresses — a zone-suffixed value is exactly the
// "IP mixed with utun" noise this view existed to fix. A trailing prefix
// length is preserved: "fe80::%lo0/64" → "fe80::/64". Non-IPv6 values
// pass through untouched.
func stripIPv6Zone(s string) string {
	if !strings.Contains(s, ":") || !strings.Contains(s, "%") {
		return s
	}
	addr, prefix := s, ""
	if i := strings.Index(s, "/"); i >= 0 {
		addr, prefix = s[:i], s[i:]
	}
	if i := strings.Index(addr, "%"); i > 0 {
		addr = addr[:i]
	}
	return addr + prefix
}

// classifyIface maps an interface name (plus, on macOS, the hardware port
// string networksetup reports for it) to a stable kind label plus a detail
// label. Name heuristics are the fallback everywhere; the hardware port is
// what makes macOS correct (en0 is Wi-Fi on laptops and Ethernet on
// desktops — only the port string knows).
//
// The detail is shown as the Interface column's badge text. When the
// hardware port is known it IS the detail (original case, e.g. "Wi-Fi"),
// so the Routes view and the tunnel editor's egress dropdown — which uses
// the same networksetup port name as its first label part — display the
// exact same string. Without a port (Linux/Windows, or anonymous utun),
// the detail falls back to creator-software detection ("Tailscale",
// "Docker", …); it stays empty when nothing is known.
func classifyIface(name, hwPort string) (kind, detail string) {
	lower := strings.ToLower(name)
	port := strings.ToLower(hwPort)
	switch {
	case strings.HasPrefix(lower, "lo"):
		return "loopback", ""
	case strings.HasPrefix(lower, "utun"), strings.HasPrefix(lower, "wg"),
		strings.HasPrefix(lower, "tun"), strings.HasPrefix(lower, "tap"):
		kind = "vpn"
	}
	switch {
	case strings.Contains(port, "wi-fi"), strings.Contains(port, "wlan"):
		return "wifi", hwPort
	case strings.Contains(port, "ethernet"), strings.Contains(port, "thunderbolt"):
		return "ethernet", hwPort
	case strings.Contains(port, "bridge"):
		return "bridge", hwPort
	case strings.Contains(port, "bluetooth"):
		return "virtual", hwPort
	case strings.Contains(port, "iphone"), strings.Contains(port, "cellular"),
		strings.Contains(port, "wwan"), strings.Contains(port, "modem"):
		return "cellular", hwPort
	}
	if kind == "" {
		switch {
		case strings.HasPrefix(lower, "wl"), strings.Contains(lower, "wlan"):
			kind = "wifi"
		case strings.HasPrefix(lower, "en"), strings.HasPrefix(lower, "eth"),
			strings.HasPrefix(lower, "enp"):
			kind = "ethernet"
		case strings.HasPrefix(lower, "bridge"), strings.HasPrefix(lower, "br-"):
			kind = "bridge"
		case strings.HasPrefix(lower, "awdl"), strings.HasPrefix(lower, "llw"),
			strings.HasPrefix(lower, "p2p"), strings.HasPrefix(lower, "veth"),
			strings.HasPrefix(lower, "docker"), strings.HasPrefix(lower, "vethernet"):
			kind = "virtual"
		}
	}
	detail = ifaceDetail(lower)
	// A hardware port name (macOS) is the authoritative detail — it is
	// the exact string the tunnel editor's egress dropdown shows — and
	// wins over any creator-software guess, including when the port
	// string matched no keyword above ("USB 10/100/1000 LAN").
	if hwPort != "" {
		detail = hwPort
	}
	// A recognized software name implies the kind even when the name
	// itself carried no usable prefix ("tailscale0", "zt…"): VPN/overlay
	// products are tunnels, container/VM products are virtual.
	if kind == "" && detail != "" {
		switch detail {
		case "Tailscale", "ZeroTier", "ProtonVPN", "NordVPN", "ExpressVPN",
			"Cloudflare WARP", "WireGuard", "Clash", "Mihomo", "Surge":
			kind = "vpn"
		default:
			kind = "virtual"
		}
	}
	return kind, detail
}

// ifaceDetail recognizes well-known interface naming from VPN / container /
// VM / tunnel software so the UI can show WHO created the interface
// ("VPN · Tailscale") instead of a bare "VPN". macOS utun interfaces are
// anonymous in user space — the detail stays empty when the name carries
// no hint (matched case-insensitively against the whole name).
func ifaceDetail(lower string) string {
	switch {
	// "vEthernet" must be checked before the "veth" Docker prefix —
	// Hyper-V's virtual switches start with the same letters.
	case strings.Contains(lower, "wsl"):
		return "WSL"
	case strings.Contains(lower, "vethernet"):
		return "Hyper-V"
	case strings.Contains(lower, "tailscale"):
		return "Tailscale"
	case strings.HasPrefix(lower, "zt") && len(lower) >= 8:
		return "ZeroTier"
	case strings.Contains(lower, "proton"):
		return "ProtonVPN"
	case strings.Contains(lower, "nordlynx"), strings.Contains(lower, "nordvpn"):
		return "NordVPN"
	case strings.Contains(lower, "expressvpn"):
		return "ExpressVPN"
	case strings.Contains(lower, "warp"):
		return "Cloudflare WARP"
	case strings.Contains(lower, "wireguard"), strings.Contains(lower, "wireguide"):
		return "WireGuard"
	case strings.Contains(lower, "awdl"), strings.HasPrefix(lower, "llw"):
		return "AWDL"
	case strings.HasPrefix(lower, "veth"), strings.HasPrefix(lower, "docker"),
		strings.HasPrefix(lower, "br-"):
		return "Docker"
	case strings.Contains(lower, "orbstack"):
		return "OrbStack"
	case strings.Contains(lower, "podman"):
		return "Podman"
	case strings.HasPrefix(lower, "virbr"), strings.Contains(lower, "libvirt"):
		return "libvirt"
	case strings.Contains(lower, "vbox"), strings.Contains(lower, "virtualbox"):
		return "VirtualBox"
	case strings.Contains(lower, "vmware"), strings.HasPrefix(lower, "vmnet"):
		return "VMware"
	case strings.Contains(lower, "clash"):
		return "Clash"
	case strings.Contains(lower, "mihomo"):
		return "Mihomo"
	case strings.Contains(lower, "surge"):
		return "Surge"
	}
	return ""
}

// darwinHardwarePorts parses `networksetup -listallhardwareports` into a
// device→hardware-port map ("en0"→"Wi-Fi"). Best effort: nil on failure
// (callers fall back to name heuristics).
func darwinHardwarePorts() map[string]string {
	out, err := runRouteCmd("networksetup", "-listallhardwareports")
	if err != nil {
		return nil
	}
	m := make(map[string]string)
	port := ""
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "Hardware Port: "); ok {
			port = v
			continue
		}
		if v, ok := strings.CutPrefix(line, "Device: "); ok && port != "" {
			m[v] = port
			port = ""
		}
	}
	return m
}

// expandDarwinNetAddr normalizes macOS netstat's compressed IPv4 network
// notation back into canonical dotted-quad + prefix form. `netstat -rn
// -f inet` on macOS/FreeBSD prints network routes with the trailing zero
// octets elided:
//
//	127.0.0.0/8    → "127"
//	169.254.0.0/16 → "169.254"
//	192.168.1.0/24 → "192.168.1"
//
// It does the same for classless networks, whose prefix is printed but
// whose address is still elided — those used to reach the UI as "2/7" or
// "128.0/1" and read like truncated addresses:
//
//	2.0.0.0/7      → "2/7"
//	128.0.0.0/1    → "128.0/1"
//
// Host addresses (10.20.20.5), IPv6 addresses and link-layer names (link#4)
// pass through untouched; "default" is handled by darwinDestination.
func expandDarwinNetAddr(s string) string {
	// IPv6 is left completely alone (the zone id and the prefix are both
	// meaningful and never elided).
	if strings.Contains(s, ":") {
		return s
	}
	// Split an explicit prefix off ("2/7", "10.20.20/24", "128.0/1") so
	// the address part can be expanded on its own. macOS omits the prefix
	// only for classful networks ("127", "169.254", "192.168.1") and for
	// host routes ("10.20.20.5").
	addr, prefix := s, ""
	if i := strings.Index(s, "/"); i >= 0 {
		addr, prefix = s[:i], s[i+1:]
	}
	parts := strings.Split(addr, ".")
	if len(parts) > 4 {
		// Full dotted-quad — already canonical (a host route when there
		// is no prefix).
		return s
	}
	// 1..4 dotted-decimal octets: verify every octet is pure digits so
	// "default", "link#4", "fe80" etc. never get mangled.
	octets := len(parts)
	for _, p := range parts {
		if p == "" {
			return s
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return s
			}
		}
	}
	for len(parts) < 4 {
		parts = append(parts, "0")
	}
	if prefix == "" {
		if octets == 4 {
			// A host address was already complete — don't invent a /32.
			return strings.Join(parts, ".")
		}
		// Classful elision: "127" is 127.0.0.0/8, "169.254" is /16.
		prefix = strconv.Itoa(octets * 8)
	}
	return strings.Join(parts, ".") + "/" + prefix
}

func getRoutesLinuxFull() ([]RouteEntry, error) {
	out, err := runRouteCmd("ip", "route", "show")
	if err != nil {
		return nil, err
	}
	addrs := buildIfaceAddrs()
	var routes []RouteEntry
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		entry := RouteEntry{}
		fields := strings.Fields(line)
		if len(fields) > 0 {
			entry.Destination = stripIPv6Zone(fields[0])
		}
		for i, f := range fields {
			if f == "via" && i+1 < len(fields) {
				entry.Gateway = fields[i+1]
			}
			if f == "dev" && i+1 < len(fields) {
				entry.Interface = fields[i+1]
			}
		}
		entry.Gateway = resolveOnLinkGateway(entry.Gateway)
		entry.InterfaceType, entry.InterfaceDetail = classifyIface(entry.Interface, "")
		// Keep only real unicast forwarding routes (loopback / multicast /
		// broadcast / local host route are filtered out).
		if !shouldKeepRoute(entry.Destination, entry.Interface, 4, addrs) {
			continue
		}
		routes = append(routes, entry)
	}
	return routes, nil
}

// getRoutesWindowsFull is defined in routes_windows.go so the iphlpapi
// dependency stays platform-scoped. The previous implementation here
// parsed `route print -4` output and produced an empty list on at least
// one user's machine; the iphlpapi path uses the same kernel API that
// PowerShell's Get-NetRoute calls and is immune to console-process
// quirks and locale differences in the route.exe output.
