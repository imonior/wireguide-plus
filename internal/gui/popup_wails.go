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
// and Open Window / Disconnect buttons, auto-closing after the configured
// duration.
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
	s := "disconnected"
	switch state {
	case popupStateConnecting:
		s = "connecting"
	case popupStateConnected:
		s = "connected"
	}
	showPopupWails(nil, names, s, outOfRange, lang, duration, onOpen, onDisconnect)
}

// showOverviewPopupWails is the mac/Linux launch status-overview bubble:
// one row per tunnel (name, connection state, automation mode). Like its
// Windows twin it is informational — Open Window is the only action.
func showOverviewPopupWails(rows []overviewRow, lang string, duration time.Duration, onOpen func()) {
	showPopupWails(rows, nil, "overview", nil, lang, duration, onOpen, nil)
}

func showPopupWails(rows []overviewRow, names []string, state string, outOfRange []string, lang string, duration time.Duration, onOpen, onDisconnect func()) {
	if duration <= 0 {
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

// computePopupPos anchors the bubble at the bottom-right corner of the main
// window (the closest cross-platform stand-in for "near the tray" — Wails v3
// alpha does not expose screen geometry publicly). Falls back to centred when
// the main window is not yet known.
func computePopupPos(w, h int) (int, int) {
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
