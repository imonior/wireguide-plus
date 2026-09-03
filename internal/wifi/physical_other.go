//go:build !linux

package wifi

import "strings"

// isVirtualIface reports whether a non-Linux adapter is a well-known virtual
// / container / hypervisor switch rather than a physical uplink. Linux gets
// this from sysfs; macOS and Windows rely on name conventions here. The
// match set is deliberately narrow (lowercased substrings of adapter names
// that never belong to a physical Wi-Fi / Ethernet adapter on those OSes)
// so a real NIC is never hidden.
//
// Observed in the wild (Windows): FlClash / Clash / mihomo / sing-box TUN
// adapters carry a default gateway and routable IPs (198.18.x fake-ip range,
// 172.x hyper-v switch range), so without this filter the subnet / ethernet
// / interface Automation conditions judge the machine as "on a wired LAN"
// while it is actually on Wi-Fi behind a proxy TUN.
func isVirtualIface(name string) bool {
	l := strings.ToLower(name)
	for _, frag := range []string{
		// Hyper-V / WSL / virtual switches.
		"vethernet", "hyper-v", "hyperv", "wsl",
		// VM vendors.
		"vmware", "virtualbox", "vbox", "parallels", "vethernet",
		// Containers.
		"docker", "com.docker", "veth",
		// Bluetooth / dial-up / loopback / transition pseudo-adapters.
		"bluetooth", "loopback", "teredo", "isatap", "6to4", "ras wan",
	} {
		if strings.Contains(l, frag) {
			return true
		}
	}
	return false
}
