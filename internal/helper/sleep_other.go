//go:build !windows && !darwin && !linux

package helper

import "log/slog"

// applySystemSleepPrevention is a no-op on platforms without a first-class sleep
// inhibitor (Windows uses SetThreadExecutionState, macOS uses caffeinate, Linux
// uses systemd-inhibit — see the per-OS files). The "keep running in background"
// setting is still persisted in config; here it simply does nothing to the OS.
// The broadcast still fires so other clients stay in sync.
func applySystemSleepPrevention(enabled bool) error {
	slog.Info("prevent system sleep is not implemented on this platform; setting ignored",
		"enabled", enabled)
	return nil
}
