//go:build darwin

package hotkey

/*
#cgo darwin CFLAGS: -framework Carbon -framework CoreFoundation
#cgo darwin LDFLAGS: -framework Carbon -framework CoreFoundation
#include <Carbon/Carbon.h>
#include <CoreFoundation/CoreFoundation.h>

// wgHotkeyHandler is implemented in Go (see the //export below) and installed
// as the application-wide keyboard hot-key event handler. Carbon needs a real C
// function symbol, which cgo provides for exported Go functions.
extern OSStatus wgHotkeyHandler(EventHandlerCallRef ref, EventRef event, void* userData);

// wgInstallHandler installs the handler once. InstallApplicationEventHandler
// takes an EventHandlerUPP (a function pointer); casting the Go-exported
// symbol's address to that type is the standard cgo pattern.
static void wgInstallHandler() {
    EventTypeSpec spec;
    spec.eventClass = kEventClassKeyboard;
    spec.eventKind = kEventHotKeyPressed;
    InstallApplicationEventHandler((EventHandlerUPP)&wgHotkeyHandler, 1, &spec, NULL, NULL);
}

// wgRegisterHotKey registers a hotkey and returns the ref as a uintptr so the
// Go side can hold it without importing Carbon headers. The signature 'WGht'
// namespaces our hotkeys away from any other app using the same id space.
static uintptr_t wgRegisterHotKey(UInt32 keyCode, UInt32 modifiers, UInt32 id) {
    EventHotKeyRef ref = NULL;
    EventHotKeyID hotKeyID;
    hotKeyID.signature = 'WGht';
    hotKeyID.id = id;
    OSStatus err = RegisterEventHotKey(keyCode, modifiers, hotKeyID,
                                       GetApplicationEventTarget(), 0, &ref);
    if (err != noErr) return 0;
    return (uintptr_t)ref;
}

// wgUnregisterHotKey releases a previously registered hotkey.
static void wgUnregisterHotKey(uintptr_t refInt) {
    if (refInt == 0) return;
    EventHotKeyRef ref = (EventHotKeyRef)(uintptr_t)refInt;
    UnregisterEventHotKey(ref);
}
*/
import "C"

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"
)

// currentOnFired is the callback the C handler invokes when the hotkey fires.
// It is package-global because Carbon delivers the event on the main run loop
// and only one hotkey is registered at a time.
var currentOnFired func()

var darwinOnce sync.Once
var darwinID atomic.Uint32

// darwinKeyCodes maps a normalized KeyboardEvent.code token to the macOS
// virtual keycode (kVK_*, read from Carbon/HIToolbox Events.h). Values are the
// exact constants from the SDK, not guesses.
var darwinKeyCodes = map[string]uint32{
	// Letters (ANSI layout, physical-key virtual codes).
	"A": 0x00, "S": 0x01, "D": 0x02, "F": 0x03, "H": 0x04, "G": 0x05,
	"Z": 0x06, "X": 0x07, "C": 0x08, "V": 0x09, "B": 0x0B, "Q": 0x0C,
	"W": 0x0D, "E": 0x0E, "R": 0x0F, "Y": 0x10, "T": 0x11, "O": 0x1F,
	"U": 0x20, "I": 0x22, "P": 0x23, "L": 0x25, "J": 0x26, "K": 0x28,
	"N": 0x2D, "M": 0x2E,
	// Digits.
	"1": 0x12, "2": 0x13, "3": 0x14, "4": 0x15, "5": 0x17, "6": 0x16,
	"7": 0x1A, "8": 0x1C, "9": 0x19, "0": 0x1D,
	// Punctuation / named keys.
	"Minus": 0x1B, "Equal": 0x18, "BracketLeft": 0x21, "BracketRight": 0x1E,
	"Backslash": 0x2A, "Semicolon": 0x29, "Quote": 0x27, "Comma": 0x2B,
	"Period": 0x2F, "Slash": 0x2C, "Backquote": 0x32,
	"Space": 0x31, "Return": 0x24, "Tab": 0x30, "Escape": 0x35, "Delete": 0x33,
	// Function keys.
	"F1": 0x7A, "F2": 0x78, "F3": 0x63, "F4": 0x76, "F5": 0x60, "F6": 0x61,
	"F7": 0x62, "F8": 0x64, "F9": 0x65, "F10": 0x6D, "F11": 0x67, "F12": 0x6F,
	// Arrow / navigation keys.
	"ArrowUp": 0x7E, "ArrowDown": 0x7D, "ArrowLeft": 0x7B, "ArrowRight": 0x7C,
	"Home": 0x73, "End": 0x77, "PageUp": 0x74, "PageDown": 0x79,
}

//export wgHotkeyHandler
func wgHotkeyHandler(ref C.EventHandlerCallRef, event C.EventRef, userData unsafe.Pointer) C.OSStatus {
	if currentOnFired != nil {
		currentOnFired()
	}
	return 0
}

// resolveDarwinCombo maps a Combo to the macOS virtual keycode + modifier mask
// (controlKey=0x1000, optionKey=0x0800, shiftKey=0x0200, cmdKey=0x0100, read
// from Events.h). A combo with no modifier is rejected.
func resolveDarwinCombo(c Combo) (keyCode uint32, modifiers uint32, err error) {
	kc, ok := darwinKeyCodes[c.Key]
	if !ok {
		return 0, 0, fmt.Errorf("unsupported hotkey key on macOS: %s", c.Key)
	}
	var m uint32
	if c.Modifiers&ModCtrl != 0 {
		m |= 0x1000 // controlKey
	}
	if c.Modifiers&ModAlt != 0 {
		m |= 0x0800 // optionKey
	}
	if c.Modifiers&ModShift != 0 {
		m |= 0x0200 // shiftKey
	}
	if c.Modifiers&ModWin != 0 {
		m |= 0x0100 // cmdKey
	}
	if m == 0 {
		return 0, 0, errors.New("hotkey requires at least one modifier")
	}
	return kc, m, nil
}

func platformRegister(combo Combo, onFired func()) (uintptr, func(), error) {
	keyCode, modifiers, err := resolveDarwinCombo(combo)
	if err != nil {
		return 0, nil, err
	}
	darwinOnce.Do(func() {
		C.wgInstallHandler()
	})
	id := darwinID.Add(1)
	ref := uintptr(C.wgRegisterHotKey(C.UInt32(keyCode), C.UInt32(modifiers), C.UInt32(id)))
	if ref == 0 {
		return 0, nil, fmt.Errorf("RegisterEventHotKey failed (key=%d, mod=%d)", keyCode, modifiers)
	}
	currentOnFired = onFired
	stop := func() {
		C.wgUnregisterHotKey(C.uintptr_t(ref))
		currentOnFired = nil
	}
	return ref, stop, nil
}
