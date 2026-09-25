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

// popupFloatAsync queues one raise attempt on the main thread and reports
// (via the atomic flag) whether the previous attempt matched. Async rather
// than dispatch_sync: a sync round-trip can stall indefinitely while a tray
// menu is in modal tracking, and the retry loop needs no verdict sooner
// than the next tick anyway. total/wails/floated counters are packed into
// one int so the Go side can log what a failed match actually saw.
#import <stdatomic.h>
static _Atomic int gPopupFloatSeen = 0; // packed total<<16|wails<<8|floated

void popupFloatAsync(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		int total = 0, wails = 0, floated = 0;
		for (NSWindow *win in [NSApp windows]) {
			total++;
			// Identification: the popup is the app's only borderless
			// WebviewWindow (the main window is titled; menus/status items
			// are not Wails windows). Title matching is NOT an option —
			// Wails alpha.74 skips setTitle for frameless windows. The
			// Wails NSWindow subclass is private to its own translation
			// unit, so it is matched by runtime class name.
			if (![[win className] hasPrefix:@"WebviewWindow"]) continue;
			wails++;
			if ([win styleMask] & NSWindowStyleMaskTitled) continue;
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
			[win orderFrontRegardless];
			floated++;
		}
		atomic_store(&gPopupFloatSeen, (total << 16) | (wails << 8) | floated);
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

// floatPopupWindow orders the status bubble front WITHOUT activating the app.
// Activation (activateIgnoringOtherApps) would steal focus and yank the user
// out of whatever they were doing — a notification must surface *above* the
// current app, not *instead of* it. Raising the NSWindow level and ordering
// front regardless achieves exactly that. The window may still be mid-
// creation/mid-order-in when Show returns to us, so retry briefly until an
// attempt reports a match (see popupFloatAsync).
func floatPopupWindow() {
	go func() {
		for attempt := 1; attempt <= 20; attempt++ {
			// Reset before queuing: the flag still carries the PREVIOUS
			// popup's verdict, and reading it without clearing would let a
			// fresh show stop retrying on a stale success.
			C.popupFloatReset()
			C.popupFloatAsync()
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
