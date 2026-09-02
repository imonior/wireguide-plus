package wifi

import (
	"net"
	"sort"
	"strings"
)

// tunnelIfacePrefixes are interface-name prefixes that belong to VPN /
// tunnel adapters, not physical uplinks. Subnet conditions must ignore
// these — a rule like "disconnect on 10.0.0.0/24" should test the real
// network the machine is on, not an address the tunnel itself assigned.
var tunnelIfacePrefixes = []string{"utun", "wg", "tun", "tap", "ipsec", "ppp"}

// PhysicalInterfaceIPs returns the unicast IP addresses currently
// assigned to physical (non-tunnel, up, non-loopback) interfaces. Used
// to evaluate subnet-based Automation conditions. Uses only the standard
// net package — no per-OS code needed.
func PhysicalInterfaceIPs() []net.IP {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []net.IP
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		// isVirtualIface (Linux sysfs) catches what the name denylist
		// can't: docker0, virbr0, tailscale0, veth*, CNI bridges — all
		// up, non-loopback, carrying routable IPs that would otherwise
		// satisfy subnet Automation rules for networks the machine isn't
		// physically on (issue #22).
		if isTunnelIface(ifi.Name) || isVirtualIface(ifi.Name) {
			continue
		}
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
				continue
			}
			out = append(out, ip)
		}
	}
	return out
}

// PhysicalInterfaces returns the physical (up, non-tunnel, non-loopback)
// interfaces that currently carry a routable address, with a best-effort
// Wi-Fi flag per platform. Used by the interface and ethernet Automation
// conditions at evaluation time, so it only reflects currently-active uplinks.
// The Wi-Fi flag is heuristic on Windows (FriendlyName match) and exact on
// Linux (sysfs) / macOS (CoreWLAN-discovered interface).
func PhysicalInterfaces() []InterfaceInfo { return listPhysicalInterfaces(true) }

// AllPhysicalInterfaces returns every physical (non-tunnel, non-loopback)
// adapter found on the machine, including ones that are currently down or
// have no address. Used by the Automation editor to populate the interface
// suggestion list so users can target a fixed wired/Wi-Fi egress even when
// it is not connected right now.
func AllPhysicalInterfaces() []InterfaceInfo { return listPhysicalInterfaces(false) }

func listPhysicalInterfaces(requireUp bool) []InterfaceInfo {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []InterfaceInfo
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		if requireUp && ifi.Flags&net.FlagUp == 0 {
			continue
		}
		if isTunnelIface(ifi.Name) || isVirtualIface(ifi.Name) {
			continue
		}
		if requireUp {
			addrs, err := ifi.Addrs()
			if err != nil {
				continue
			}
			hasRoutable := false
			for _, a := range addrs {
				var ip net.IP
				switch v := a.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}
				if ip != nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() {
					hasRoutable = true
					break
				}
			}
			if !hasRoutable {
				continue
			}
		}
		out = append(out, InterfaceInfo{Name: ifi.Name, IsWiFi: ifaceIsWiFi(ifi.Name)})
	}
	return out
}

// PhysicalSubnets returns the network CIDRs of the physical
// (non-tunnel, up) interfaces the machine is currently on — e.g.
// "192.168.0.0/24" for an interface holding 192.168.0.127/24. Used to
// suggest subnet values in the Automation editor so a user can target
// the network they're on (Wi-Fi or Ethernet) without knowing its CIDR.
// IPv4 and IPv6 (excluding link-local) are included; duplicates removed,
// output sorted for stable UI ordering.
func PhysicalSubnets() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 ||
			isTunnelIface(ifi.Name) || isVirtualIface(ifi.Name) {
			continue
		}
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok || ipnet.IP == nil {
				continue
			}
			if ipnet.IP.IsLoopback() || ipnet.IP.IsLinkLocalUnicast() || ipnet.IP.IsLinkLocalMulticast() {
				continue
			}
			// Mask the host IP down to the network address so the CIDR is
			// the subnet (192.168.0.127/24 → 192.168.0.0/24).
			network := &net.IPNet{IP: ipnet.IP.Mask(ipnet.Mask), Mask: ipnet.Mask}
			cidr := network.String()
			if !seen[cidr] {
				seen[cidr] = true
				out = append(out, cidr)
			}
		}
	}
	sort.Strings(out)
	return out
}

// isTunnelIface reports whether name looks like a VPN/tunnel adapter.
// The Windows WireGuard adapter is named "WireGuidePlus-<hash>" (legacy
// installs: "WireGuide-<hash>") which the wg/tun prefixes below don't
// catch, so match it explicitly.
func isTunnelIface(name string) bool {
	lower := strings.ToLower(name)
	// Substring match: covers our "WireGuidePlus-" adapter, the legacy
	// "WireGuide-" name, and third-party "wireguard"-named adapters.
	if strings.Contains(lower, "wireguide") || strings.Contains(lower, "wireguard") {
		return true
	}
	for _, p := range tunnelIfacePrefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}
