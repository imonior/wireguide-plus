//go:build !windows

package gui

import "time"

// showStatusPopup mirrors the Windows connection-situation bubble using a
// Wails secondary window (see popup_wails.go). macOS/Linux have no native
// tray popup of their own, so this delegates to the shared, cross-platform
// implementation rather than a no-op.
func showStatusPopup(names []string, state popupState, outOfRange []string, lang string, duration time.Duration, onOpen, onDisconnect func()) {
	showStatusPopupWails(names, state, outOfRange, lang, duration, onOpen, onDisconnect)
}

// showOverviewPopup mirrors the Windows launch-overview bubble using the
// same Wails secondary window (see popup_wails.go).
func showOverviewPopup(rows []overviewRow, lang string, duration time.Duration, onOpen func()) {
	showOverviewPopupWails(rows, lang, duration, onOpen)
}

// closeConnectPopup tears down any visible status bubble so a fresh one can
// replace it (the trailing-edge dedup in scheduleStatusNotification, or a
// manual dismiss, both funnel through here).
func closeConnectPopup() {
	destroyPopupWindow()
}
