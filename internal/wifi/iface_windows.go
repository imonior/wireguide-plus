//go:build windows

package wifi

import "strings"

// ifaceIsWiFi reports whether a Windows interface is a Wi-Fi adapter.
// Go's net.Interfaces() exposes FriendlyName here, and the common Windows
// Wi-Fi FriendlyNames contain "Wi-Fi", "Wireless", "WLAN" or "802.11".
// This is heuristic (a wired adapter named "Ethernet Wireless Dock" would
// be misclassified), but covers the vast majority of machines; the
// Windows default for the built-in Wi-Fi adapter is literally "Wi-Fi".
func ifaceIsWiFi(name string) bool {
	l := strings.ToLower(name)
	return strings.Contains(l, "wi-fi") || strings.Contains(l, "wireless") ||
		strings.Contains(l, "wlan") || strings.Contains(l, "802.11")
}
