//go:build !windows

package tunnel

// TunnelLUID returns 0 on non-Windows platforms. The Windows build
// returns the wintun adapter LUID consumed by the socket-bind monitor
// (internal/tunnel/socketbind_windows.go); on Linux/macOS that monitor
// is a no-op and no LUID exists. manager.go calls it unconditionally
// because its call site is platform-agnostic.
func (e *Engine) TunnelLUID() uint64 { return 0 }
