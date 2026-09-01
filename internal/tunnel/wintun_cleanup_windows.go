//go:build windows

package tunnel

import (
	"log/slog"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// cleanupStaleWintunAdapter removes leftover wintun adapters from a
// previous run before we attempt CreateTUN. The adapter was renamed to
// "WireGuidePlus-" when the app rebranded, so a helper crash before the
// upgrade can leave a "WireGuide-<hash>" adapter behind that the new name
// won't match — both names are swept.
//
// Best-effort. If wintun.dll isn't loadable (e.g. wintun-go embedded a
// different path) we simply do nothing and let the regular CreateTUN
// path either succeed or fail with its own error message.
// wintunDLLName mirrors third_party/wintun's architecture selection so the
// pre-cleanup path loads the same DLL variant the driver itself uses.
func wintunDLLName() string {
	switch runtime.GOARCH {
	case "386":
		return "wintun-x86.dll"
	case "arm64":
		return "wintun-arm64.dll"
	default:
		return "wintun-amd64.dll"
	}
}

func cleanupStaleWintunAdapter(name string) {
	// Sweep the current name plus the pre-plus "WireGuide-<hash>" name a
	// helper running before the rebrand may have left in the kernel.
	closeStaleWintunAdapter(name)
	closeStaleWintunAdapter("WireGuide-" + strings.TrimPrefix(name, "WireGuidePlus-"))
}

// closeStaleWintunAdapter opens a wintun adapter by name and closes it,
// tearing down a leftover adapter from a previous run. Without this, a
// helper crash that didn't tear down the adapter leaves it pinned to the
// dead instance and the next CreateTUN may fail with ERROR_ALREADY_EXISTS
// (1073, also reported as "object already exists").
//
// Best-effort. If wintun.dll isn't loadable we simply do nothing and let
// the regular CreateTUN path either succeed or fail with its own error
// message.
func closeStaleWintunAdapter(name string) {
	dll, err := windows.LoadDLL(wintunDLLName())
	if err != nil {
		// wintun-go may extract the DLL to a temp location only after
		// the first CreateAdapter call. In that case we can't
		// pre-clean, but the subsequent CreateAdapter itself returns
		// the existing adapter, which is OK.
		slog.Debug("wintun.dll not loadable for pre-cleanup", "error", err)
		return
	}
	defer dll.Release()

	openProc, err := dll.FindProc("WintunOpenAdapter")
	if err != nil {
		return
	}
	closeProc, err := dll.FindProc("WintunCloseAdapter")
	if err != nil {
		return
	}

	// Names cross the FFI boundary as null-terminated UTF-16.
	utf16Name, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return
	}
	handle, _, _ := openProc.Call(uintptr(unsafe.Pointer(utf16Name)))
	if handle == 0 {
		// No leftover adapter — common case, no-op.
		return
	}
	slog.Warn("found stale wintun adapter from previous run, closing", "name", name)
	closeProc.Call(handle)
}
