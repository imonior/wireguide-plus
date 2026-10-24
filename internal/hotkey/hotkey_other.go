//go:build !darwin && !windows

package hotkey

// platformRegister on unsupported platforms always returns ErrUnsupported so
// the caller (and the UI) can gracefully disable global hotkeys. Linux with
// X11/Wayland lacks a portable global-hotkey API without pulling in extra
// desktop-specific dependencies, so we intentionally do not register one here.
func platformRegister(_ Combo, _ func()) (uintptr, func(), error) {
	return 0, nil, ErrUnsupported
}
