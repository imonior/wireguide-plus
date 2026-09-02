//go:build linux

package wifi

import (
	"os"
	"strings"
)

// ifaceIsWiFi reports whether a Linux interface is wireless: the kernel
// exposes a "wireless" subdirectory under the interface's sysfs node for
// every 802.11 device (absent for Ethernet, bridges, veth, tun, …).
func ifaceIsWiFi(name string) bool {
	if name == "" || strings.ContainsAny(name, "/\\") {
		return false
	}
	_, err := os.Stat("/sys/class/net/" + name + "/wireless")
	return err == nil
}
