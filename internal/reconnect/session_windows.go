//go:build windows

package reconnect

import (
	"log/slog"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// windowsSessionMonitor watches the Windows session for resume events using a
// message-only window:
//
//   - RegisterPowerSettingNotification(GUID_MONITOR_POWER_UP) delivers
//     WM_POWERBROADCAST / PBT_POWERSETTINGCHANGE with the display state
//     (Data 0 = off, 1 = on). This covers screen-saver / display sleep.
//   - WTSRegisterSessionNotification delivers WM_WTSSESSION_CHANGE with
//     WTS_SESSION_LOCK / WTS_SESSION_UNLOCK — the same state the user trips
//     by locking (Win+L) or the idle lock policy.
//
// Both ON / UNLOCK paths feed the same ResumeChan, because they mean the same
// thing to a VPN: the user is back, the NIC is awake (or waking), so now is
// the moment to probe the tunnel and self-heal it if it died while idle.
//
// NOTE: x/sys/windows does not export the WNDCLASSEX / CreateWindowEx /
// GetMessage / DefWindowProc family, so — exactly like internal/gui/
// notify_windows.go — we bind them through windows.NewLazySystemDLL(...)
// + .NewProc(...) and define the structs ourselves.
type windowsSessionMonitor struct {
	mu           sync.Mutex
	running      bool
	resumeCh     chan struct{}
	wnd          uintptr
	classAtom    uint16
	hInstance    uintptr
	classNamePtr *uint16
	wndProc      uintptr // kept alive so the GC never collects the callback
	powerReg     uintptr // RegisterPowerSettingNotification handle
	wtsReg       bool
}

// Windows constants for the session monitor.
const (
	wmPowerBroadcast         = 0x0218
	pbtPowerSettingChange    = 0x8013
	wmWtsSessionChange       = 0x02B1
	wtsSessionLock           = 0x7
	wtsSessionUnlock         = 0x8
	notifyForThisSession     = 0x0
	deviceNotifyWindowHandle = 0x00000000
	wmQuit                   = 0x0012
	classNameSessionNotify   = "WGPlusSessionNotify"

	// HWND_MESSAGE = (HWND)-3 — the parent for a message-only window that
	// never appears on screen and never receives broadcast messages meant
	// for top-level windows.
	hwndMessage = 0xFFFFFFFD
)

// GUID_MONITOR_POWER_UP identifies the display power-state setting.
var guidMonitorPowerUp = windows.GUID{
	Data1: 0x02731027,
	Data2: 0x451e,
	Data3: 0x446f,
	Data4: [8]byte{0xa1, 0x75, 0x9c, 0x47, 0xb2, 0xe6, 0x03, 0x85},
}

// wndClassEx mirrors the Win32 WNDCLASSEXW struct (not exported by x/sys).
type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

// msg mirrors the Win32 MSG struct used by GetMessageW.
type msg struct {
	HWND     uintptr
	Message  uint32
	_        uint32 // padding after Message
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       struct{ X, Y int32 }
	LPrivate uint32
}

var (
	modUser32   = windows.NewLazySystemDLL("user32.dll")
	modKernel32 = windows.NewLazySystemDLL("kernel32.dll")
	modWtsapi32 = windows.NewLazySystemDLL("wtsapi32.dll")

	procRegisterClassExW                   = modUser32.NewProc("RegisterClassExW")
	procCreateWindowExW                    = modUser32.NewProc("CreateWindowExW")
	procGetMessageW                        = modUser32.NewProc("GetMessageW")
	procTranslateMessage                   = modUser32.NewProc("TranslateMessage")
	procDispatchMessageW                   = modUser32.NewProc("DispatchMessageW")
	procDefWindowProcW                     = modUser32.NewProc("DefWindowProcW")
	procPostMessageW                       = modUser32.NewProc("PostMessageW")
	procDestroyWindow                      = modUser32.NewProc("DestroyWindow")
	procUnregisterClassW                   = modUser32.NewProc("UnregisterClassW")
	procRegisterPowerSettingNotification   = modUser32.NewProc("RegisterPowerSettingNotification")
	procUnregisterPowerSettingNotification = modUser32.NewProc("UnregisterPowerSettingNotification")
	procWTSRegisterSessionNotification     = modWtsapi32.NewProc("WTSRegisterSessionNotification")
	procWTSUnRegisterSessionNotification   = modWtsapi32.NewProc("WTSUnRegisterSessionNotification")
	procGetModuleHandleW                   = modKernel32.NewProc("GetModuleHandleW")
)

// NewSessionDetector returns the Windows implementation.
func NewSessionDetector() SessionDetector {
	return &windowsSessionMonitor{}
}

func (m *windowsSessionMonitor) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.resumeCh = make(chan struct{}, 1)
	m.mu.Unlock()

	go m.run()
}

func (m *windowsSessionMonitor) run() {
	// The window's message queue is bound to the OS thread that created it.
	// Pin this goroutine to one OS thread for the whole lifetime, otherwise
	// GetMessage/DispatchMessage would run on a different thread than the
	// window and the monitor would never receive the power/WTS callbacks.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Module handle for the window class. GetModuleHandleW(NULL) returns
	// the EXE's HINSTANCE, which is what RegisterClassEx expects.
	hInst, _, _ := procGetModuleHandleW.Call(0)
	m.hInstance = hInst

	classNamePtr := windows.StringToUTF16Ptr(classNameSessionNotify)
	m.classNamePtr = classNamePtr

	m.wndProc = windows.NewCallback(m.wndProcDispatch)
	wndClass := wndClassEx{
		cbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		style:         0,
		lpfnWndProc:   m.wndProc,
		hInstance:     hInst,
		lpszClassName: classNamePtr,
	}
	atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wndClass)))
	if atom == 0 {
		slog.Warn("session monitor: RegisterClassEx failed", "error", err)
		return
	}
	m.classAtom = uint16(atom)

	hwnd, _, err := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(classNamePtr)),
		0,
		0,
		0, 0, 0, 0,
		uintptr(hwndMessage),
		0,
		hInst,
		0,
	)
	if hwnd == 0 {
		slog.Warn("session monitor: CreateWindowEx failed", "error", err)
		procUnregisterClassW.Call(uintptr(unsafe.Pointer(classNamePtr)), hInst)
		return
	}
	m.wnd = hwnd

	// Display power state (screen-saver / display sleep).
	ph, _, err := procRegisterPowerSettingNotification.Call(
		hwnd,
		uintptr(unsafe.Pointer(&guidMonitorPowerUp)),
		uintptr(deviceNotifyWindowHandle),
	)
	if ph == 0 {
		slog.Warn("session monitor: RegisterPowerSettingNotification failed", "error", err)
	} else {
		m.powerReg = ph
	}

	// Session lock / unlock.
	rc, _, err := procWTSRegisterSessionNotification.Call(
		hwnd,
		uintptr(notifyForThisSession),
	)
	if rc == 0 {
		slog.Warn("session monitor: WTSRegisterSessionNotification failed", "error", err)
	} else {
		m.wtsReg = true
	}

	slog.Info("Windows session monitor registered (display + lock/unlock)")

	// Message pump must run on the thread that created the window.
	var mmsg msg
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&mmsg)), 0, 0, 0)
		if ret == 0 || ret == ^uintptr(0) { // WM_QUIT or error
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&mmsg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&mmsg)))
	}

	// GetMessage returned 0 (WM_QUIT) — tear everything down.
	m.cleanup()
}

// wndProcDispatch is the window procedure. It is stored in m.wndProc (a
// windows callback) so the GC cannot collect it while the window lives.
func (m *windowsSessionMonitor) wndProcDispatch(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	switch msg {
	case wmPowerBroadcast:
		if wparam == pbtPowerSettingChange {
			// This window is registered ONLY for GUID_MONITOR_POWER_UP, so
			// every PBT_POWERSETTINGCHANGE delivered here means the display
			// power state changed. We react to both transitions: healIfNeeded
			// is a no-op when the tunnel is already healthy, so the OFF
			// transition costs nothing, and the ON transition self-heals a
			// tunnel that died while the screen was dark (screen-saver /
			// display sleep → NIC idle → WireGuard handshake stale).
			//
			// We deliberately do NOT dereference lparam: reading the
			// POWERBROADCAST_SETTING struct would require
			// unsafe.Pointer(lparam), which trips go vet's unsafeptr check.
			slog.Info("session monitor: display power change (resume check)")
			m.resume()
		}
	case wmWtsSessionChange:
		switch wparam {
		case wtsSessionUnlock:
			slog.Info("session monitor: session UNLOCKED (resume)")
			m.resume()
		case wtsSessionLock:
			slog.Info("session monitor: session LOCKED (idle)")
		}
	}
	return defWindowProc(hwnd, msg, wparam, lparam)
}

func defWindowProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}

func (m *windowsSessionMonitor) resume() {
	m.mu.Lock()
	ch := m.resumeCh
	m.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (m *windowsSessionMonitor) ResumeChan() <-chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.resumeCh
}

func (m *windowsSessionMonitor) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	wnd := m.wnd
	m.mu.Unlock()

	// Post WM_QUIT so the message pump's GetMessage returns and run()
	// proceeds to cleanup. Best-effort: if the window is gone the pump
	// already exited.
	if wnd != 0 {
		procPostMessageW.Call(wnd, wmQuit, 0, 0)
	}
}

// cleanup unregisters notifications, destroys the window and unregisters the
// class. Called from the message-pump goroutine on WM_QUIT.
func (m *windowsSessionMonitor) cleanup() {
	m.mu.Lock()
	powerReg := m.powerReg
	wtsReg := m.wtsReg
	wnd := m.wnd
	atom := m.classAtom
	hInst := m.hInstance
	classNamePtr := m.classNamePtr
	m.powerReg = 0
	m.wtsReg = false
	m.wnd = 0
	m.classAtom = 0
	m.mu.Unlock()

	if powerReg != 0 {
		procUnregisterPowerSettingNotification.Call(powerReg)
	}
	if wtsReg {
		procWTSUnRegisterSessionNotification.Call(wnd)
	}
	if wnd != 0 {
		procDestroyWindow.Call(wnd)
	}
	if atom != 0 && classNamePtr != nil {
		procUnregisterClassW.Call(uintptr(unsafe.Pointer(classNamePtr)), hInst)
	}
	slog.Info("Windows session monitor stopped")
}
