//go:build darwin

package wifi

// ifaceIsWiFi reports whether a macOS interface is the Wi-Fi hardware
// port, discovered via CoreWLAN (falling back to networksetup parsing).
func ifaceIsWiFi(name string) bool {
	if name == "" {
		return false
	}
	return name == discoverWiFiInterface()
}
