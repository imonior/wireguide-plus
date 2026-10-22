//go:build !darwin

package gui

// installTrayDoubleClickMonitor is macOS-only. Windows and Linux never reach
// it: Wails delivers a real double-click there (systemtray_windows.go reads
// doubleClickHandler), and both already go through trayClickRouter, which
// separates single from double by click spacing.
func installTrayDoubleClickMonitor(func()) {}
