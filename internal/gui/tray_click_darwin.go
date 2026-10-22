package gui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// wgTrayDoubleClick is implemented in Go (tray_click_export_darwin.go) and
// only DECLARED here: per cgo rules a preamble that defines C code may not
// also host //export.
extern void wgTrayDoubleClick(void);

// How close two left clicks must land for the second to count as a
// double-click. Same value as trayClickWindow on Windows/Linux so the
// gesture feels identical on every platform.
static const double wgDoubleClickWindowMs = 450.0;

// The monitor plus the click it is currently holding back. A held click is
// re-posted once the window closes with no second click behind it, so the
// menu still opens through Wails' own native path.
static id wgClickMonitor = nil;
static NSEvent *wgHeldClick = nil;
static NSEvent *wgReposted = nil;

// Finds this process's menu-bar window. Wails creates the status item
// asynchronously, so nil is a legitimate early answer and callers retry.
static NSWindow *wgFindStatusbarWindow(void) {
	for (NSWindow *w in [NSApp windows]) {
		if ([NSStringFromClass([w class]) containsString:@"NSStatusBarWindow"]) {
			return w;
		}
	}
	return nil;
}

// Installs the double-click detector. Returns true once installed.
//
// A single left click must open the tray menu while a double click opens
// the main window — but AppKit only tells us "a mouse-down happened", and
// the first click of a double is indistinguishable from a single click until
// the second one lands. So the first click is swallowed and re-posted (via
// -[NSApplication postEvent:atStart:]) when no second click arrives inside
// the window: a re-posted event travels the ordinary dispatch path, which is
// exactly where Wails' own monitor attaches the menu and lets AppKit track
// it natively. Re-posting rather than calling Wails' OpenMenu() is the whole
// point — OpenMenu synthesizes a fake mouse-down and hands it to the button
// directly, and that trick needs two clicks on some systems and stopped
// opening the menu altogether on macOS 26.
static bool wgInstallTrayDoubleClick(void) {
	if (wgClickMonitor != nil) {
		return true;
	}
	NSWindow *win = wgFindStatusbarWindow();
	if (win == nil) {
		return false;
	}
	wgClickMonitor = [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskLeftMouseDown
		handler:^NSEvent *(NSEvent *event) {
			// Our own re-post: let it through untouched.
			if (wgReposted != nil && event == wgReposted) {
				wgReposted = nil;
				return event;
			}
			if (event.window != win) {
				return event;
			}
			if (event.clickCount >= 2) {
				// Second click of a double: consume it and open the window.
				wgHeldClick = nil;
				dispatch_async(dispatch_get_main_queue(), ^{ wgTrayDoubleClick(); });
				return nil;
			}
			// First click: hold it until we know whether a second lands.
			NSEvent *held = event;
			wgHeldClick = held;
			dispatch_after(dispatch_time(DISPATCH_TIME_NOW,
				(int64_t)(wgDoubleClickWindowMs * (double)NSEC_PER_MSEC)),
				dispatch_get_main_queue(), ^{
					if (wgHeldClick != held) {
						return; // a second click already consumed this one
					}
					wgHeldClick = nil;
					wgReposted = held;
					[NSApp postEvent:held atStart:NO];
				});
			return nil;
		}];
	return true;
}
*/
import "C"

import (
	"log/slog"
	"sync/atomic"
	"time"
)

var trayDoubleClickCB atomic.Pointer[func()]

// installTrayDoubleClickMonitor makes a double left click on the menu-bar
// icon show the main window, WITHOUT giving up the native single-click menu.
//
// macOS is the one platform where the two gestures genuinely collide: Wails
// hands a status item either native menu tracking or a click callback, never
// both (see systrayPreClickCallback in the vendored Wails source), and
// SystemTray.OnDoubleClick is only ever read by the Windows backend —
// registering it here does nothing. Registering a plain OnClick would cost
// us the menu anyway, and the only way to get the menu back from inside a
// click handler is OpenMenu(), which synthesizes a fake mouse-down and is
// broken on macOS 26. So we keep Wails' handler unset and detect the double
// from an NSEvent monitor instead, re-posting a lone click so the menu still
// opens along the path that works.
//
// The status item is created asynchronously, so installation is retried
// briefly; if the menu-bar window never appears, cb is simply never called
// and single-click behaviour is untouched.
func installTrayDoubleClickMonitor(cb func()) {
	trayDoubleClickCB.Store(&cb)
	go func() {
		for i := 0; i < 40; i++ { // ~20s budget for the status item to exist
			if bool(C.wgInstallTrayDoubleClick()) {
				slog.Info("tray: double-click monitor installed")
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
		slog.Warn("tray: double-click monitor not installed (menu-bar window not found); double click stays a no-op")
	}()
}
