package gui

// Cross-platform status-popup window for macOS and Linux.
//
// Windows draws the connection-situation bubble as a hand-rolled Win32/GDI
// window (notify_windows.go). On macOS/Linux there is no equivalent native
// popup, so this file implements the same feature as a small Wails secondary
// window: a frameless, always-on-top webview that loads a dedicated Svelte
// view (frontend/src/lib/StatusPopup.svelte, mounted by App.svelte when the
// URL carries ?popup=1). The view mirrors the Windows bubble: title, status
// dot + caption, the tunnel list, an amber "out of tunnel routes" warning,
// and Open Window / Disconnect buttons. Status transitions arrive as
// persistent bubbles (popupPersistent — dismissed by the user, never by a
// timer); only the informational launch overview carries a duration.
//
// This file is intentionally build-tag-free so the Wails-window logic (which
// is valid on every platform) is compiled and type-checked on Windows too —
// only notify_other.go (the thin showStatusPopup wrapper) is platform-tagged.
// On Windows the secondary window is never opened (showStatusPopup is the
// Win32 one), but the event handlers below are registered regardless and stay
// dormant until a popup:open/close event actually arrives.

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	// popupWidth matches the Windows bubble width (348 DIPs).
	popupWidth = 348
	// popupBaseHeight is the bubble height with no warning line.
	popupBaseHeight = 120
	// popupWarnLine is the extra height the amber out-of-range warning needs.
	popupWarnLine = 18
	// popupRowLine is the extra height one launch-overview tunnel row
	// needs beyond the first (the single-line status bubble is the base).
	popupRowLine = 16
	popupMargin  = 8
)

// popupPayload is the data the Svelte popup renders. State is one of
// "connected" | "connecting" | "disconnected" (the popupState values, sent as
// strings so the frontend can localise them via i18n rather than receiving
// already-translated text), or "overview" for the launch status report, in
// which case Rows carries one entry per tunnel.
type popupPayload struct {
	Names      []string      `json:"names"`
	State      string        `json:"state"`
	OutOfRange []string      `json:"out_of_range"`
	DurationMs int           `json:"duration_ms"`
	Rows       []overviewRow `json:"rows,omitempty"`
}

var (
	popupApp *application.App
	popupWin *application.WebviewWindow
	popupMu  sync.Mutex
	popupCb  struct {
		onOpen       func()
		onDisconnect func()
	}
	popupLatest popupPayload
)

// registerPopupEvents wires the Svelte popup's button/close events back to the
// stored Go callbacks and window teardown. Called once from Run() on every
// platform.
func registerPopupEvents(app *application.App) {
	app.Event.On("popup:ready", func(_ *application.CustomEvent) {
		// The freshly-loaded popup view subscribed to popup:data; replay the
		// most recent payload so it doesn't miss the emit that raced ahead of
		// its JS subscription.
		popupMu.Lock()
		p := popupLatest
		popupMu.Unlock()
		app.Event.Emit("popup:data", p)
	})
	app.Event.On("popup:open", func(_ *application.CustomEvent) {
		popupMu.Lock()
		cb := popupCb.onOpen
		popupMu.Unlock()
		// Log line anchoring the "clicked Open Window but the main window
		// flashed" reports: if this appears but show-dock attempts do not,
		// the callback wiring is the broken link, not showDock.
		slog.Info("popup: open-window requested", "haveCallback", cb != nil)
		if cb != nil {
			go cb()
		}
		destroyPopupWindow()
	})
	app.Event.On("popup:disconnect", func(_ *application.CustomEvent) {
		popupMu.Lock()
		cb := popupCb.onDisconnect
		popupMu.Unlock()
		if cb != nil {
			go cb()
		}
		destroyPopupWindow()
	})
	app.Event.On("popup:close", func(_ *application.CustomEvent) {
		destroyPopupWindow()
	})
}

// showStatusPopupWails opens (or refreshes) the status bubble. It is the
// mac/Linux implementation behind notify_other.go's showStatusPopup; it is
// never called on Windows (there notify_windows.go owns the popup).
func showStatusPopupWails(names []string, state popupState, outOfRange []string, lang string, duration time.Duration, onOpen, onDisconnect func()) {
	// The bubble is line-per-tunnel ("Connected: wg0", red/green whole line)
	// on every platform: transitionRows builds the same rows the Windows GDI
	// popup draws, and the Svelte view renders them one per line when state is
	// not "overview". Names still travel for renderers that prefer the joined
	// form and as a fallback if rows are ever dropped.
	showPopupWails(transitionRows(names, state), names, state.String(), outOfRange, lang, duration, onOpen, onDisconnect)
}

// showOverviewPopupWails is the mac/Linux launch status-overview bubble:
// one row per tunnel (name, connection state, automation mode). Like its
// Windows twin it is informational — Open Window is the only action.
func showOverviewPopupWails(rows []overviewRow, lang string, duration time.Duration, onOpen func()) {
	showPopupWails(rows, nil, "overview", nil, lang, duration, onOpen, nil)
}

func showPopupWails(rows []overviewRow, names []string, state string, outOfRange []string, lang string, duration time.Duration, onOpen, onDisconnect func()) {
	if duration == 0 {
		duration = 10 * time.Second
	}
	slog.Info("popup: showStatusPopupWails called", "names", names, "state", state,
		"out_of_range", outOfRange, "lang", lang, "duration", duration, "rows", len(rows))

	popupMu.Lock()
	popupCb.onOpen = onOpen
	popupCb.onDisconnect = onDisconnect
	popupLatest = popupPayload{
		Names:      names,
		State:      state,
		OutOfRange: outOfRange,
		DurationMs: int(duration.Milliseconds()),
		Rows:       rows,
	}
	popupMu.Unlock()

	// Reuse the existing popup window if it is still on screen (just refresh
	// its content and re-show it); otherwise create one. We do NOT close-then-
	// recreate here: that would briefly leave two windows sharing the same
	// name, and window creation/closing must round-trip the main thread.
	ensurePopupWindow()
	popupMu.Lock()
	w := popupWin
	popupMu.Unlock()
	if w == nil {
		// No window object: the popup silently never appears — say so in
		// the log, because "why no notification?" otherwise has no trace.
		slog.Warn("popup: no window available (app not wired?), skipping show")
		return
	}
	// Grow the window for every overview row beyond the first and for the
	// warning line, so nothing overlaps.
	height := popupBaseHeight
	if len(rows) > 1 {
		height += popupRowLine * (len(rows) - 1)
	}
	if len(outOfRange) > 0 {
		height += popupWarnLine
	}
	w.SetSize(popupWidth, height)
	x, y := computePopupPos(popupWidth, height)
	w.SetPosition(x, y)
	w.Show()
	// A frameless always-on-top window is only frontmost *within* its own
	// app: while another application owns the screen, the whole app (popup
	// included) can stay behind it, so the "real-time" notification is
	// invisible until the user happens to focus the main window. On macOS
	// raise the popup's window level and order it front *without*
	// activating the app — a notification must surface above whatever the
	// user is doing, not yank focus to us. No-op on Linux (the WM decides)
	// and unused on Windows (the bubble is the separate Win32 popup).
	floatPopupWindow()
	// Emit after Show so the webview's subscription is in place; popup:ready
	// (fired by the view on mount) covers the brief race otherwise.
	popupMu.Lock()
	p := popupLatest
	popupMu.Unlock()
	popupApp.Event.Emit("popup:data", p)
}

// ensurePopupWindow creates the secondary popup window once. It is a no-op if
// the window already exists (callers may keep reusing it across notifications,
// or destroy it via destroyPopupWindow between shows).
func ensurePopupWindow() {
	popupMu.Lock()
	if popupWin != nil || popupApp == nil {
		popupMu.Unlock()
		return
	}
	popupMu.Unlock()

	opts := application.WebviewWindowOptions{
		// Unique name per window: the previous popup may still be mid-close
		// (Close is async) when a new one is created, and Wails keys windows
		// by name — reusing a fixed name would collide.
		Name:             fmt.Sprintf("status-popup-%d", time.Now().UnixNano()),
		Width:            popupWidth,
		Height:           popupBaseHeight,
		Frameless:        true,
		AlwaysOnTop:      true,
		URL:              "/?popup=1",
		BackgroundColour: application.NewRGB(30, 30, 30),
		// Start hidden so we can position it before it becomes visible —
		// avoids a flash at (0,0) on slower webviews.
		Hidden: true,
	}
	w := popupApp.Window.NewWithOptions(opts)

	popupMu.Lock()
	popupWin = w
	popupMu.Unlock()
}

// destroyPopupWindow closes the popup window (if any) and forgets the pending
// callbacks. Safe to call when no popup is open.
//
// w.Close() dispatches to the main/UI thread via InvokeSync, so it must not be
// called from that thread (it would deadlock waiting for itself). Running it in
// its own goroutine means: if the caller is already on the UI thread the
// goroutine simply blocks until the close is processed; if the caller is a
// normal goroutine (the usual case — the notification fires from a timer) there
// is no contention at all.
func destroyPopupWindow() {
	popupMu.Lock()
	w := popupWin
	popupWin = nil
	popupCb.onOpen = nil
	popupCb.onDisconnect = nil
	popupMu.Unlock()
	if w != nil {
		go w.Close()
	}
}

// computePopupPos anchors the bubble at the bottom-right corner of the
// primary display's WORK AREA (the screen minus menu bar / taskbar / Dock),
// mirroring where the Windows bubble parks by its tray. The work area —
// not the full bounds — is what keeps the bubble off the Dock, and the
// corner — not a main-window anchor — is what keeps it off the user's
// content: a bubble pinned to the window's corner used to sit on top of
// whatever the window was showing, which is exactly what a notification
// must not do. SetPosition and WorkArea are both in DIPs, so no scaling
// correction is needed here.
func computePopupPos(w, h int) (int, int) {
	if popupApp != nil {
		if sc := popupApp.Screen.GetPrimary(); sc != nil {
			wa := sc.WorkArea
			x := wa.X + wa.Width - w - popupMargin
			y := wa.Y + wa.Height - h - popupMargin
			if x < wa.X {
				x = wa.X + popupMargin
			}
			if y < wa.Y {
				y = wa.Y + popupMargin
			}
			return x, y
		}
	}
	// No screen info (headless / early startup): fall back to anchoring on
	// the main window, and if even that is unknown, let the WM place us.
	if dockWindow != nil {
		dx, dy := dockWindow.Position()
		dw, dh := dockWindow.Size()
		x := dx + dw - w - popupMargin
		y := dy + dh - h - popupMargin
		if x < 0 {
			x = popupMargin
		}
		if y < 0 {
			y = popupMargin
		}
		return x, y
	}
	return 0, 0
}
