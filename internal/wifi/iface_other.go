//go:build !darwin && !linux && !windows

package wifi

// ifaceIsWiFi reports whether an interface is a Wi-Fi adapter. Unknown on
// unsupported platforms — the ethernet condition then treats every physical
// interface as wired (still correct for the "disconnect on ethernet" case
// when the machine has no Wi-Fi).
func ifaceIsWiFi(name string) bool {
	return false
}
