//go:build !windows

package helper

import "log/slog"

// applySystemSleepPrevention is a no-op on non-Windows platforms. The
// "keep running in background" setting is still persisted in config (so it is
// remembered when the user runs on Windows), but these platforms have no
// single-call equivalent to SetThreadExecutionState and are not the GUI's
// primary target. The toggle therefore moves and saves, but does nothing to
// the OS here — the broadcast still fires so other clients stay in sync.
func applySystemSleepPrevention(enabled bool) error {
	slog.Info("prevent system sleep is not implemented on this platform; setting ignored",
		"enabled", enabled)
	return nil
}
