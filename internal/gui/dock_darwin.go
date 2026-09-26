package gui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

void dockHide() {
	dispatch_async(dispatch_get_main_queue(), ^{
		[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
	});
}

void dockShowAndActivate() {
	dispatch_async(dispatch_get_main_queue(), ^{
		[NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
		[NSApp activateIgnoringOtherApps:YES];
	});
}

// popupFloatAsync queues one configure attempt on the main thread and reports
// (via the atomic flag) whether the previous attempt matched. Async rather
// than dispatch_sync: a sync round-trip can stall indefinitely while a tray
// menu is in modal tracking, and the retry loop needs no verdict sooner
// than the next tick anyway. total/wails/floated counters are packed into
// one int so the Go side can log what a failed match actually saw.
//
// The pass owns the whole macOS presentation of the bubble: level/space
// behaviour, rounded clipping, the top-right anchor and the drag gesture.
// Wails' own screen service is deliberately NOT involved — its macOS backend
// never marks a screen primary, so Screen.GetPrimary() returns nil and
// SetPosition/WorkArea disagree with Cocoa about units (points vs scaled).
// Reading NSScreen.visibleFrame here is the one coordinate source that is
// simply true on this platform.
#import <stdatomic.h>
static _Atomic int gPopupFloatSeen = 0; // packed total<<16|wails<<8|floated

static BOOL isPopupWindow(NSWindow *win) {
	// Identification: the popup is the app's only borderless WebviewWindow
	// (the main window is titled; menus/status items are not Wails
	// windows). Title matching is NOT an option — Wails alpha.74 skips
	// setTitle for frameless windows. The Wails NSWindow subclass is
	// private to its own translation unit, so it is matched by runtime
	// class name.
	return [[win className] hasPrefix:@"WebviewWindow"] &&
		!([win styleMask] & NSWindowStyleMaskTitled);
}

void popupFloatAsync(int width, int height) {
	dispatch_async(dispatch_get_main_queue(), ^{
		int total = 0, wails = 0, floated = 0;
		for (NSWindow *win in [NSApp windows]) {
			total++;
			if (![[win className] hasPrefix:@"WebviewWindow"]) continue;
			wails++;
			if (!isPopupWindow(win)) continue;
			// Status level (same as the menu bar) rather than merely
			// "floating": the bubble must clear other apps' windows while
			// we deliberately do NOT activate the app. isVisible is NOT a
			// criterion — ordering in is part of what this call does.
			//
			// The collection behavior is what makes the bubble visible
			// over a FULLSCREEN frontmost app (a different Space): without
			// joining all spaces, a non-activating popup is stranded on
			// our Space — invisible until the user focuses our main
			// window, exactly the original complaint.
			[win setLevel:NSStatusWindowLevel];
			[win setCollectionBehavior:(NSWindowCollectionBehaviorCanJoinAllSpaces |
				NSWindowCollectionBehaviorFullScreenAuxiliary)];
			[win setHidesOnDeactivate:NO];
			// The bubble's rounded border lives in CSS (StatusPopup.svelte,
			// 12px radius) — but a frameless NSWindow is a square opaque
			// surface, so the window background poked out at the four
			// corners and the border read as broken/chopped. Clear the
			// window background and clip the content to the same radius:
			// the card now ends where its border ends, on any theme.
			[win setOpaque:NO];
			[win setBackgroundColor:[NSColor clearColor]];
			NSView *content = [win contentView];
			content.wantsLayer = YES;
			content.layer.cornerRadius = 12.0;
			content.layer.masksToBounds = YES;
			// Size and anchor in Cocoa points (frameless: content == frame
			// for our purposes). Park at the top-right of the screen the
			// window is on, just below the menu bar — the pocket macOS
			// notifications own. visibleFrame already excludes menu bar
			// and Dock, and its origin is bottom-left, so the window's
			// origin.y is its TOP minus height.
			[win setContentSize:NSMakeSize(width, height)];
			NSScreen *screen = [win screen];
			if (screen == nil) screen = [NSScreen mainScreen];
			if (screen != nil) {
				NSRect vf = [screen visibleFrame];
				NSSize sz = [win frame].size;
				CGFloat x = NSMaxX(vf) - sz.width - 8;
				CGFloat y = NSMaxY(vf) - sz.height - 8;
				[win setFrameOrigin:NSMakePoint(x, y)];
			}
			[win orderFrontRegardless];
			floated++;
		}
		atomic_store(&gPopupFloatSeen, (total << 16) | (wails << 8) | floated);
	});
}

// popupDragWatch installs the bubble's drag gesture once. Wails' JS drag
// path (drag.js -> performWindowDragWithEvent:delegate.leftMouseEvent) is
// unreliable here: the bubble is shown without activating the app, and the
// first click on it is swallowed by activation (WKWebView rejects
// first-mouse), so the delegate's cached mousedown is often nil or stale by
// the time the JS invoke round-trips. A local monitor sees every mousedown
// our app dispatches; we arm a candidate and hand the gesture to AppKit
// once the pointer has actually moved. The threshold matters for the
// buttons: a click that never moves stays a click (no drag session is ever
// started, so the mouse-up reaches the web view), while a drag eats the
// mouse-up and cannot fire the button underneath.
static NSWindow *gDragWin = nil;
static NSEvent *gDragDown = nil;

// popupUnderPoint finds the bubble when the event carries no window
// reference (global monitors never do): its screen-space frame is matched
// against the pointer location.
static NSWindow *popupUnderPoint(NSPoint p) {
	for (NSWindow *win in [NSApp windows]) {
		if (!isPopupWindow(win)) continue;
		if (NSPointInRect(p, [win frame])) return win;
	}
	return nil;
}

static void dragArm(NSWindow *w, NSEvent *down) {
	if (gDragDown != down) {
		[down retain];
		[gDragDown release];
		gDragDown = down;
	}
	gDragWin = w;
}

static void dragDisarm(void) {
	[gDragDown release];
	gDragDown = nil;
	gDragWin = nil;
}

void popupDragWatch(void) {
	// Called from a random goroutine's OS thread; AppKit wants event
	// monitors installed from the main thread, so hop there. The static
	// flag is set INSIDE the block: a second call while the first is still
	// queued must not slip through and queue a duplicate install.
	dispatch_async(dispatch_get_main_queue(), ^{
		static BOOL installed = NO;
		if (installed) return;
		installed = YES;
	const unsigned long long mask =
		NSEventMaskLeftMouseDown | NSEventMaskLeftMouseDragged | NSEventMaskLeftMouseUp;
	void (^handle)(NSEvent *, NSWindow *) = ^(NSEvent *event, NSWindow *fallbackHit) {
		switch ([event type]) {
		case NSEventTypeLeftMouseDown: {
			NSWindow *w = [event window] ?: fallbackHit;
			if (w != nil && isPopupWindow(w)) {
				dragArm(w, event);
			} else {
				dragDisarm();
			}
			break;
		}
		case NSEventTypeLeftMouseDragged: {
			if (gDragWin == nil || gDragDown == nil) break;
			NSPoint a = [gDragDown locationInWindow];
			NSPoint b = [event locationInWindow];
			if (fabs(b.x - a.x) < 4 && fabs(b.y - a.y) < 4) break;
			// Hand AppKit the ORIGINAL button-down: that is the event the
			// drag session is supposed to start from. Below the threshold
			// nothing is consumed, so a plain click still reaches the web
			// view and its buttons.
			[gDragWin performWindowDragWithEvent:gDragDown];
			dragDisarm();
			break;
		}
		case NSEventTypeLeftMouseUp:
			dragDisarm();
			break;
		default:
			break;
		}
	};
	[NSEvent addLocalMonitorForEventsMatchingMask:mask handler:^NSEvent *(NSEvent *event) {
		handle(event, nil);
		return event;
	}];
	[NSEvent addGlobalMonitorForEventsMatchingMask:mask handler:^void(NSEvent *event) {
		// Events our app is going to receive anyway surface here too once
		// the click activates us; the hit-test stands in for the missing
		// window reference.
		handle(event, popupUnderPoint([event locationInWindow]));
	}];
	});
}

int popupFloatSeen(void) { return atomic_load(&gPopupFloatSeen); }

void popupFloatReset(void) { atomic_store(&gPopupFloatSeen, 0); }
*/
import "C"
import (
	"log/slog"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var dockWindow *application.WebviewWindow

// floatPopupWindow runs the native presentation pass (see popupFloatAsync):
// level, spaces, clipping, size and the top-right anchor — ordering the
// bubble front WITHOUT activating the app. Activation
// (activateIgnoringOtherApps) would steal focus and yank the user out of
// whatever they were doing — a notification must surface *above* the current
// app, not *instead of* it. Raising the NSWindow level and ordering front
// regardless achieves exactly that. The window may still be mid-creation /
// mid-order-in when Show returns to us, so retry briefly until an attempt
// reports a match.
func floatPopupWindow(width, height int) {
	C.popupDragWatch()

	go func() {
		for attempt := 1; attempt <= 20; attempt++ {
			// Reset before queuing: the flag still carries the PREVIOUS
			// popup's verdict, and reading it without clearing would let a
			// fresh show stop retrying on a stale success.
			C.popupFloatReset()
			C.popupFloatAsync(C.int(width), C.int(height))
			time.Sleep(50 * time.Millisecond)
			r := int(C.popupFloatSeen())
			total, wails, floated := (r>>16)&0xff, (r>>8)&0xff, r&0xff
			if floated > 0 {
				slog.Info("popup: floated status window",
					"attempt", attempt, "nsapp_windows", total, "wails_windows", wails)
				return
			}
			if attempt == 20 {
				slog.Warn("popup: status window never matched for float",
					"nsapp_windows", total, "wails_windows", wails)
			}
		}
	}()
}

func showDock() {
	C.dockShowAndActivate()

	go func() {
		for i := 0; i < 10; i++ {
			time.Sleep(200 * time.Millisecond)
			// dockWindow is assigned once and never cleared, so the nil
			// check alone can't stop this loop during teardown. Show() on
			// a destroyed Wails window RE-RUNS window creation, so a retry
			// landing mid-quit would resurrect the window or deadlock in
			// InvokeSync — bail out once quit has been initiated.
			if dockWindow == nil || appQuitting.Load() {
				return
			}
			dockWindow.Show()
			dockWindow.Focus()
			C.dockShowAndActivate()

			// Check if the window actually became visible. Info level:
			// this loop is the story behind "main window flashes and is
			// gone" reports, and it fires only on user-initiated shows.
			time.Sleep(50 * time.Millisecond)
			if dockWindow.IsVisible() {
				slog.Info("show window succeeded", "attempt", i+1)
				return
			}
			slog.Info("show window retry", "attempt", i+1)
		}
		slog.Warn("show window failed after 10 attempts")
	}()
}

func hideDock() { C.dockHide() }

// popupAnchorsNatively reports that on macOS the bubble's size and position
// are owned by the native pass above (popupFloatAsync), so the generic Wails
// SetSize/SetPosition/computePopupPos path is skipped. Wails' macOS screen
// service never marks a screen primary (GetPrimary() is nil) and its
// position maths disagrees with Cocoa about units; visibleFrame read here is
// the one coordinate source that is simply true on this platform.
func popupAnchorsNatively() bool { return true }
