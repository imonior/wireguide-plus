//go:build !windows

package wifi

// startWindowsWlanWatcher is a no-op on non-Windows platforms. The Windows
// build uses wlanapi's WlanRegisterNotification to react instantly to SSID
// changes; the per-OS poll handles the same role elsewhere.
func startWindowsWlanWatcher(onChange func()) (stop func(), attached bool) {
	return func() {}, false
}

// currentSSIDFromWlanapi is a no-op on non-Windows platforms. detect.go's
// detectWindows() switches on runtime.GOOS, so every case compiles on every
// platform and needs this symbol defined. The Windows build implements it
// via wlanapi in detect_windows.go.
func currentSSIDFromWlanapi() string { return "" }
