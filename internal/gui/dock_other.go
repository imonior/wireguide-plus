//go:build !darwin

package gui

import (
	"runtime"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var dockWindow *application.WebviewWindow

// showDock is reachable concurrently on Linux — StatusNotifier Activate and
// dbusmenu events each arrive on their own godbus goroutine, and Wails runs
// menu callbacks on fresh goroutines. The frameless toggle below must not
// interleave between two callers or the window can map undecorated with no
// recovery (GTK caches the decorated hint), so the toggle is serialized.
var showDockMu sync.Mutex

// showDock brings back the main window after close-to-tray / start-minimised.
// Unlike macOS there is no dock icon / activation policy to juggle (and no
// async retry dance) — un-hide + un-minimise + Show + Focus is the whole job.
// Restore comes first: Show() alone does not un-minimise on Windows, so a
// window hidden while minimised would otherwise reappear only in the
// taskbar, still collapsed.
//
// Deadlock note (fixes "tray-bubble Open Window sometimes hangs"): showDock
// is reachable concurrently — the Windows tray left-click handler runs on the
// Wails UI thread (systemtray_windows.go wndProc), while the tray-bubble's
// Open Window callback runs on the bubble's own message-loop goroutine. Every
// Wails window call below (IsMinimised/Restore/SetFrameless/Show/Focus) is an
// InvokeSync that the UI thread's message loop must service. Holding
// showDockMu across them is fatal: if the UI thread enters showDock while the
// bubble thread holds the lock, the UI thread blocks in Lock() and can never
// pump the wmInvokeCallback message the bubble thread is waiting on — a
// classic lock-order inversion that froze the whole app. The mutex therefore
// guards ONLY the Linux frameless toggle (whose interleaving corrupts the GTK
// decoration hint); the Linux UI thread never enters showDock (Activate /
// dbusmenu callbacks run on dbus goroutines), so no lock-order inversion is
// possible there.
func showDock() {
	if dockWindow == nil {
		return
	}
	if dockWindow.IsMinimised() {
		dockWindow.Restore()
	}
	if runtime.GOOS == "linux" {
		showDockMu.Lock()
		defer showDockMu.Unlock()
		// labwc/XWayland may forget server-side decorations when a hidden GTK
		// window is mapped again. Merely setting decorated=true is ineffective
		// because GTK already caches that value and sends no new WM hint. Toggle
		// it while the window is hidden so the next map is unambiguously
		// decorated, without exposing an intermediate frameless frame.
		dockWindow.SetFrameless(true)
		dockWindow.SetFrameless(false)
	}
	dockWindow.Show()
	dockWindow.Focus()
}

// hideDock only exists to hide the macOS dock icon — the window itself is
// already hidden by the WindowClosing hook, so there is nothing to do here.
func hideDock() {}
