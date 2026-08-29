//go:build !windows

package gui

import "time"

// showStatusPopup is a no-op on platforms without a tray popup
// implementation yet (macOS/Linux). The connection-status notification is a
// Windows-focused feature.
func showStatusPopup(_ []string, _ bool, _ string, _ time.Duration, _, _ func()) {}

func closeConnectPopup() {}
