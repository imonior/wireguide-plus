//go:build windows

package hotkey

import (
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"sync/atomic"
	"syscall"
	"unsafe"
)

// Windows modifier flags for RegisterHotKey (from winuser.h).
const (
	modAlt     = 0x0001
	modControl = 0x0002
	modShift   = 0x0004
	modWin     = 0x0008
	wmHotkey   = 0x0312
	wmQuit     = 0x0012
)

// msgStruct is the 64-bit Windows MSG layout (8-byte aligned). HWND(8) +
// UINT(4) + pad(4) + WPARAM(8) + LPARAM(8) + DWORD time(4) + POINT(8) +
// DWORD lPrivate(4) = 48 bytes. Go lays the fields out identically.
type msgStruct struct {
	hwnd     uintptr
	message  uint32
	wParam   uintptr
	lParam   uintptr
	time     uint32
	ptX      int32
	ptY      int32
	lPrivate uint32
}

var (
	user32             = syscall.NewLazyDLL("user32.dll")
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	registerHotKey     = user32.NewProc("RegisterHotKey")
	unregisterHotKey   = user32.NewProc("UnregisterHotKey")
	getMessage         = user32.NewProc("GetMessageW")
	translateMessage   = user32.NewProc("TranslateMessage")
	dispatchMessage    = user32.NewProc("DispatchMessageW")
	postThreadMessage  = user32.NewProc("PostThreadMessageW")
	getCurrentThreadId = kernel32.NewProc("GetCurrentThreadId")
)

// currentOnFired is the callback invoked when WM_HOTKEY arrives.
var currentOnFired func()

var winID atomic.Uint32

// winKeyCodes maps a normalized KeyboardEvent.code token to a Windows virtual
// key code (VK_*, from winuser.h). Letters/digits follow ASCII; the rest are
// the documented OEM/control codes.
var winKeyCodes = map[string]uint32{
	// Letters.
	"A": 0x41, "B": 0x42, "C": 0x43, "D": 0x44, "E": 0x45, "F": 0x46,
	"G": 0x47, "H": 0x48, "I": 0x49, "J": 0x4A, "K": 0x4B, "L": 0x4C,
	"M": 0x4D, "N": 0x4E, "O": 0x4F, "P": 0x50, "Q": 0x51, "R": 0x52,
	"S": 0x53, "T": 0x54, "U": 0x55, "V": 0x56, "W": 0x57, "X": 0x58,
	"Y": 0x59, "Z": 0x5A,
	// Digits.
	"0": 0x30, "1": 0x31, "2": 0x32, "3": 0x33, "4": 0x34, "5": 0x35,
	"6": 0x36, "7": 0x37, "8": 0x38, "9": 0x39,
	// Punctuation / named keys.
	"Space": 0x20, "Return": 0x0D, "Tab": 0x09, "Escape": 0x1B, "Delete": 0x2E,
	"Backquote": 0xC0, "Minus": 0xBD, "Equal": 0xBB, "BracketLeft": 0xDB,
	"BracketRight": 0xDD, "Backslash": 0xDC, "Semicolon": 0xBA, "Quote": 0xDE,
	"Comma": 0xBC, "Period": 0xBE, "Slash": 0xBF,
	// Function keys.
	"F1": 0x70, "F2": 0x71, "F3": 0x72, "F4": 0x73, "F5": 0x74, "F6": 0x75,
	"F7": 0x76, "F8": 0x77, "F9": 0x78, "F10": 0x79, "F11": 0x7A, "F12": 0x7B,
	// Navigation / arrow keys.
	"Home": 0x24, "End": 0x23, "PageUp": 0x21, "PageDown": 0x22,
	"ArrowLeft": 0x25, "ArrowUp": 0x26, "ArrowRight": 0x27, "ArrowDown": 0x28,
}

// resolveWindowsCombo maps a Combo to the Windows modifier mask + virtual key.
func resolveWindowsCombo(c Combo) (mod uint32, vk uint32, err error) {
	vkVal, ok := winKeyCodes[c.Key]
	if !ok {
		return 0, 0, fmt.Errorf("unsupported hotkey key on Windows: %s", c.Key)
	}
	var m uint32
	if c.Modifiers&ModCtrl != 0 {
		m |= modControl
	}
	if c.Modifiers&ModAlt != 0 {
		m |= modAlt
	}
	if c.Modifiers&ModShift != 0 {
		m |= modShift
	}
	if c.Modifiers&ModWin != 0 {
		m |= modWin
	}
	if m == 0 {
		return 0, 0, errors.New("hotkey requires at least one modifier")
	}
	return m, vkVal, nil
}

// platformRegister spawns a dedicated, OS-thread-locked goroutine that
// registers the hotkey and pumps its message queue. RegisterHotKey with a NULL
// window associates the hotkey with the calling thread, so the pump MUST run on
// that same thread (LockOSThread) and the thread's message queue must exist
// before we signal readiness (RegisterHotKey itself creates it).
func platformRegister(combo Combo, onFired func()) (uintptr, func(), error) {
	mod, vk, err := resolveWindowsCombo(combo)
	if err != nil {
		return 0, nil, err
	}
	id := winID.Add(1)
	started := make(chan struct{})
	var threadID uint32
	stop := func() {
		<-started
		postThreadMessage.Call(uintptr(threadID), wmQuit, 0, 0)
	}
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		tid, _, _ := getCurrentThreadId.Call()
		threadID = uint32(tid)
		ret, _, _ := registerHotKey.Call(0, uintptr(id), uintptr(mod), uintptr(vk))
		if ret == 0 {
			slog.Error("hotkey: RegisterHotKey failed", "id", id, "mod", mod, "vk", vk)
			close(started)
			return
		}
		defer unregisterHotKey.Call(0, uintptr(id))
		currentOnFired = onFired
		// The queue now exists (RegisterHotKey is a USER call), so a later
		// PostThreadMessage(WM_QUIT) will be delivered.
		close(started)
		var msg msgStruct
		for {
			r, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if int32(r) == -1 {
				break
			}
			if msg.message == wmQuit {
				break
			}
			if msg.message == wmHotkey && currentOnFired != nil {
				currentOnFired()
			}
			translateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			dispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
		}
		currentOnFired = nil
	}()
	return uintptr(id), stop, nil
}
