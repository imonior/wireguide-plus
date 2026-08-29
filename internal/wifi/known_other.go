//go:build !windows

package wifi

// knownSSIDsWindows is a no-op on non-Windows platforms. KnownSSIDs()
// switches on runtime.GOOS — a runtime value, so every case compiles on
// every platform — and needs this symbol defined everywhere. The real
// Windows implementation lives in known_windows.go.
func knownSSIDsWindows() []string { return nil }
