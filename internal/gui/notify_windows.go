//go:build windows

package gui

import (
	"log/slog"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Connection-status tray bubble (Windows).
//
// After the app is running and permission prompts are settled, and whenever
// the connection state changes (auto-connect, Wi-Fi switch, Ethernet
// unplug/replug, network loss, manual connect/disconnect), a small
// self-drawn bubble pops up near the notification area reporting the
// current connection situation. It carries a mini menu (Open Window /
// Disconnect), can be dismissed with the ✕ button, and closes itself after
// the configured duration (default 10s).

const (
	connectPopupClass = "WireGuideConnectPopup"

	// Window styles
	wsExTopmost    = 0x00000008
	wsExToolWindow = 0x00000080
	wsExNoActivate = 0x08000000
	wsPopup        = 0x80000000

	swShowNoActivate = 4

	// Messages
	wmEraseBkgnd  = 0x0014
	wmPaint       = 0x000F
	wmClose       = 0x0010
	wmDestroy     = 0x0002
	wmTimer       = 0x0113
	wmMouseMove   = 0x0200
	wmLButtonDown = 0x0201
	wmLButtonUp   = 0x0202
	wmMouseLeave  = 0x02A3

	// GDI
	transparentBkMode = 1
	nullBrush         = 5
	nullPen           = 8

	// SystemParametersInfo
	spiGetWorkArea = 0x0030

	// AppBar
	abmGetTaskbarPos = 0x00000005
	abeLeft          = 0
	abeTop           = 1
	abeRight         = 2
	abeBottom        = 3

	idcArrow = 32512

	// DrawText flags
	dtTop         = 0x00000008
	dtSingleLine  = 0x00000020
	dtEndEllipsis = 0x00008000
)

var (
	user32dll   = windows.NewLazySystemDLL("user32.dll")
	gdi32dll    = windows.NewLazySystemDLL("gdi32.dll")
	shell32dll  = windows.NewLazySystemDLL("shell32.dll")
	kernel32dll = windows.NewLazySystemDLL("kernel32.dll")

	procCreateWindowExW       = user32dll.NewProc("CreateWindowExW")
	procDefWindowProcW        = user32dll.NewProc("DefWindowProcW")
	procDestroyWindow         = user32dll.NewProc("DestroyWindow")
	procRegisterClassExW      = user32dll.NewProc("RegisterClassExW")
	procSetTimer              = user32dll.NewProc("SetTimer")
	procKillTimer             = user32dll.NewProc("KillTimer")
	procPostQuitMessage       = user32dll.NewProc("PostQuitMessage")
	procGetMessageW           = user32dll.NewProc("GetMessageW")
	procTranslateMessage      = user32dll.NewProc("TranslateMessage")
	procDispatchMessageW      = user32dll.NewProc("DispatchMessageW")
	procSystemParametersInfoW = user32dll.NewProc("SystemParametersInfoW")
	procShowWindow            = user32dll.NewProc("ShowWindow")
	procGetDpiForSystem       = user32dll.NewProc("GetDpiForSystem")
	procGetModuleHandleW      = kernel32dll.NewProc("GetModuleHandleW")
	procLoadCursorW           = user32dll.NewProc("LoadCursorW")
	procGetClientRect         = user32dll.NewProc("GetClientRect")
	procGetDC                 = user32dll.NewProc("GetDC")
	procReleaseDC             = user32dll.NewProc("ReleaseDC")
	procInvalidateRect        = user32dll.NewProc("InvalidateRect")
	procBeginPaint            = user32dll.NewProc("BeginPaint")
	procEndPaint              = user32dll.NewProc("EndPaint")
	procSetCapture            = user32dll.NewProc("SetCapture")
	procReleaseCapture        = user32dll.NewProc("ReleaseCapture")
	procTrackMouseEvent       = user32dll.NewProc("TrackMouseEvent")
	procPostMessageW          = user32dll.NewProc("PostMessageW")
	procSHAppBarMessage       = shell32dll.NewProc("SHAppBarMessage")
	procSetWindowRgn          = user32dll.NewProc("SetWindowRgn")

	procCreateSolidBrush      = gdi32dll.NewProc("CreateSolidBrush")
	procCreateFontIndirectW   = gdi32dll.NewProc("CreateFontIndirectW")
	procSelectObject          = gdi32dll.NewProc("SelectObject")
	procDeleteObject          = gdi32dll.NewProc("DeleteObject")
	procSetTextColor          = gdi32dll.NewProc("SetTextColor")
	procSetBkMode             = gdi32dll.NewProc("SetBkMode")
	procGetStockObject        = gdi32dll.NewProc("GetStockObject")
	procTextOutW              = gdi32dll.NewProc("TextOutW")
	procFillRect              = user32dll.NewProc("FillRect") // FillRect is a user32 export, not GDI
	procCreateRoundRectRgn    = gdi32dll.NewProc("CreateRoundRectRgn")
	procGetTextExtentPoint32W = gdi32dll.NewProc("GetTextExtentPoint32W")
	procCreatePen             = gdi32dll.NewProc("CreatePen")
	procMoveToEx              = gdi32dll.NewProc("MoveToEx")
	procLineTo                = gdi32dll.NewProc("LineTo")
	procRectangle             = gdi32dll.NewProc("Rectangle")
	procRoundRect             = gdi32dll.NewProc("RoundRect")
	procEllipse               = gdi32dll.NewProc("Ellipse")
	procDrawTextW             = user32dll.NewProc("DrawTextW") // DrawTextW is a user32 export, not GDI
)

// ---- Win32 structs (not provided by x/sys/windows) ----

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

type logFont struct {
	lfHeight         int32
	lfWidth          int32
	lfEscapement     int32
	lfOrientation    int32
	lfWeight         int32
	lfItalic         byte
	lfUnderline      byte
	lfStrikeOut      byte
	lfCharSet        byte
	lfOutPrecision   byte
	lfClipPrecision  byte
	lfQuality        byte
	lfPitchAndFamily byte
	lfFaceName       [32]uint16
}

type paintStruct struct {
	hdc         uintptr
	fErase      int32
	rcPaint     windows.Rect
	fRestore    int32
	fIncUpdate  int32
	rgbReserved [32]byte
}

type trackMouseEvent struct {
	cbSize      uint32
	dwFlags     uint32
	hWnd        uintptr
	dwHoverTime uint32
}

type popupPoint struct{ X, Y int32 }

type popupMsg struct {
	HWND     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       popupPoint
	LPrivate uint32
}

type appBarData struct {
	cbSize           uint32
	hWnd             uintptr
	uCallbackMessage uint32
	uEdge            uint32
	rc               windows.Rect
	lParam           uintptr
}

// ---- Texts / palette / fonts ----

type popupTexts struct {
	connected    string
	connecting   string
	notConnected string
	// outOfRange labels the warning line naming probe targets that are no
	// longer routed through the tunnel (see popupData.outOfRange).
	outOfRange string
	openLabel  string
	discLabel  string
	// autoLabel/manualLabel tag each row of the launch overview bubble
	// (see popupData.overview): automation may act on the tunnel or not.
	autoLabel   string
	manualLabel string
}

// popupState (and its three values) lives in popup_state.go — it is part of
// the tray's contract on every platform, not just this one.

func popupTextsFor(lang string) popupTexts {
	switch lang {
	case "zh", "zh-CN", "zh-Hans", "zh-TW", "zh-Hant":
		return popupTexts{connected: "已连接", connecting: "连接中", notConnected: "未连接", outOfRange: "探测目标不在隧道范围内", openLabel: "打开主界面", discLabel: "断开连接", autoLabel: "自动", manualLabel: "手动"}
	case "ko":
		return popupTexts{connected: "연결됨", connecting: "연결 중", notConnected: "연결 안 됨", outOfRange: "터널 범위 밖의 프로브 대상", openLabel: "창 열기", discLabel: "연결 끊기", autoLabel: "자동", manualLabel: "수동"}
	case "ja":
		return popupTexts{connected: "接続済み", connecting: "接続中", notConnected: "未接続", outOfRange: "トンネル範囲外のプローブ先", openLabel: "ウィンドウを開く", discLabel: "切断", autoLabel: "自動", manualLabel: "手動"}
	default:
		return popupTexts{connected: "Connected", connecting: "Connecting", notConnected: "Disconnected", outOfRange: "Probe target outside tunnel routes", openLabel: "Open Window", discLabel: "Disconnect", autoLabel: "auto", manualLabel: "manual"}
	}
}

func popupFontFamily(lang string) string {
	switch lang {
	case "zh", "zh-CN", "zh-Hans", "zh-TW", "zh-Hant":
		return "Microsoft YaHei UI"
	case "ja":
		return "Yu Gothic UI"
	case "ko":
		return "Malgun Gothic"
	default:
		return "Segoe UI"
	}
}

func namesSeparator(lang string) string {
	if strings.HasPrefix(lang, "zh") {
		return "、"
	}
	return ", "
}

func rgb(r, g, b uint32) uint32 { return r | g<<8 | b<<16 }

// popupPalette returns (bg, fg, border, btnHover, dot) as COLORREF values.
func popupPalette(light bool) (bg, fg, border, btnHover, dot, warn uint32) {
	if light {
		return rgb(0xF7, 0xF7, 0xF7), rgb(0x11, 0x11, 0x11), rgb(0xD9, 0xD9, 0xD9),
			rgb(0xE7, 0xE7, 0xE7), rgb(0x2F, 0xBF, 0x71), rgb(0xB5, 0x6A, 0x0F)
	}
	return rgb(0x20, 0x20, 0x20), rgb(0xFF, 0xFF, 0xFF), rgb(0x3C, 0x3C, 0x3C),
		rgb(0x2E, 0x2E, 0x2E), rgb(0x34, 0xC7, 0x59), rgb(0xF0, 0xB4, 0x5E)
}

func systemLightTheme() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("AppsUseLightTheme")
	if err != nil {
		return false
	}
	return v != 0
}

// ---- Per-window state ----

type popupData struct {
	names []string
	state popupState
	// outOfRange lists probe targets that are currently NOT routed through
	// the tunnel (an AllowedIPs edit or a moved DDNS name can invalidate a
	// target that was accepted when it was saved). Rendered as an amber
	// warning line under the tunnel list — the companion to the editor's
	// colour marking, for the case where the app window is closed and the
	// only thing on screen is this bubble.
	outOfRange []string
	// overview, when non-empty, switches the bubble to the launch status
	// report: one row per tunnel (dot + name + state + automation tag)
	// replacing the single status line, and no Disconnect button — the
	// overview describes, it does not act.
	overview     []overviewRow
	lang         string
	onOpen       func()
	onDisconnect func()

	themeLight bool
	dpi        uint32
	scale      float32
	hwnd       uintptr

	titleFont uintptr
	bodyFont  uintptr

	titleRect windows.Rect
	closeRect windows.Rect
	dotRect   windows.Rect
	bodyRect  windows.Rect
	warnRect  windows.Rect
	openRect  windows.Rect
	discRect  windows.Rect

	hoverClose bool
	hoverOpen  bool
	hoverDisc  bool
}

var (
	popupClassOnce    sync.Once
	popupWndProc      uintptr
	popupClassNamePtr *uint16
	popupHwnds        sync.Map // uintptr -> *popupData
	activePopupHWND   atomic.Uintptr
)

// ---- Public API (called from tray.go) ----

// showStatusPopup displays the connection-situation bubble near the tray
// icon. duration controls how long it stays before auto-closing (the ✕
// button or a click also closes it immediately).
// state must be derived from the *established* tunnel set (see
// Manager.EstablishedTunnels): popupStateConnecting exists so the bubble can
// say "still trying" instead of "connected" for a tunnel whose connect
// attempt has not finished — and may yet fail.
func showStatusPopup(names []string, state popupState, outOfRange []string, lang string, duration time.Duration, onOpen, onDisconnect func()) {
	spawnPopup(nil, names, state, outOfRange, lang, duration, onOpen, onDisconnect)
}

// showOverviewPopup displays the launch status-overview bubble: every
// tunnel with its connection state and automation mode. Informational —
// the only action is Open Window.
func showOverviewPopup(rows []overviewRow, lang string, duration time.Duration, onOpen func()) {
	spawnPopup(rows, nil, popupStateConnected, nil, lang, duration, onOpen, nil)
}

func spawnPopup(overview []overviewRow, names []string, state popupState, outOfRange []string, lang string, duration time.Duration, onOpen, onDisconnect func()) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("popup: recovered from panic in showStatusPopup", "err", r, "stack", string(debug.Stack()))
		}
	}()
	if duration <= 0 {
		duration = 10 * time.Second
	}
	slog.Info("popup: showStatusPopup called", "names", names, "state", state,
		"out_of_range", outOfRange, "duration", duration, "overview_rows", len(overview))
	ensurePopupClass()
	closeConnectPopup() // replace any stale popup
	go runPopupLoop(overview, names, state, outOfRange, lang, duration, onOpen, onDisconnect)
}

func closeConnectPopup() {
	if hwnd := activePopupHWND.Load(); hwnd != 0 {
		procPostMessageW.Call(hwnd, wmClose, 0, 0)
	}
}

// ---- Setup ----

func ensurePopupClass() {
	popupClassOnce.Do(func() {
		popupWndProc = windows.NewCallback(popupWindowProc)
		popupClassNamePtr = windows.StringToUTF16Ptr(connectPopupClass)
		hinst, _, _ := procGetModuleHandleW.Call(0)
		wc := wndClassEx{
			cbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
			style:         3, // CS_HREDRAW | CS_VREDRAW
			lpfnWndProc:   popupWndProc,
			hInstance:     hinst,
			hCursor:       loadCursor(0, idcArrow),
			lpszClassName: popupClassNamePtr,
		}
		if atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); atom == 0 {
			if err != windows.ERROR_CLASS_ALREADY_EXISTS {
				slog.Warn("popup: RegisterClassEx failed", "err", err)
			}
		}
	})
}

func loadCursor(hinst uintptr, id int32) uintptr {
	c, _, _ := procLoadCursorW.Call(hinst, uintptr(id))
	return c
}

func runPopupLoop(overview []overviewRow, names []string, state popupState, outOfRange []string, lang string, duration time.Duration, onOpen, onDisconnect func()) {
	// The window's message queue is bound to the OS thread that created it.
	// A plain goroutine can migrate between OS threads between syscalls, which
	// would make GetMessage/DispatchMessage run on a different thread than the
	// window — the popup would never receive clicks, WM_TIMER or WM_DESTROY and
	// appear frozen (dismissing with ✕ or auto-close would hang). Pin this
	// goroutine to one OS thread for the whole popup lifetime.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	d := &popupData{
		overview:     overview,
		names:        names,
		state:        state,
		outOfRange:   outOfRange,
		lang:         lang,
		onOpen:       onOpen,
		onDisconnect: onDisconnect,
		themeLight:   systemLightTheme(),
		dpi:          96,
	}
	defer func() {
		if r := recover(); r != nil {
			slog.Error("popup: recovered from panic in runPopupLoop", "err", r, "stack", string(debug.Stack()))
		}
		closeConnectPopup()
		d.hwnd = 0
		activePopupHWND.Store(0)
	}()
	if dpi, _, _ := procGetDpiForSystem.Call(); dpi != 0 {
		d.dpi = uint32(dpi)
	}
	d.scale = float32(d.dpi) / 96.0

	d.titleFont = createPopupFont(d, 14, true)
	d.bodyFont = createPopupFont(d, 12, false)
	cleanupFonts := func() {
		if d.titleFont != 0 {
			procDeleteObject.Call(d.titleFont)
		}
		if d.bodyFont != 0 {
			procDeleteObject.Call(d.bodyFont)
		}
	}

	w := int32(348 * d.scale)
	// The warning line is the only optional row in a status bubble.
	// Growing the window (rather than squeezing the warning in) keeps the
	// buttons at their fixed offsets from the bottom edge — they are
	// positioned from h, so raising h moves them down with the extra row
	// instead of overlapping the tunnel names.
	warnLine := int32(0)
	if len(d.outOfRange) > 0 {
		warnLine = int32(18 * d.scale)
	}
	h := int32(120*d.scale) + warnLine
	rowH := int32(16 * d.scale)
	if len(d.overview) > 0 {
		// Overview mode: rows start at the status line's y and replace the
		// single status caption + names line, so the window grows with the
		// tunnel count. 92 = 42 (rows start) + 8 (gap) + 28 (buttons) +
		// 14 (bottom pad), scaled; every row adds rowH.
		rowsH := int32(92*d.scale) + rowH*int32(len(d.overview))
		if base := int32(120 * d.scale); rowsH > base {
			h = rowsH + warnLine
		} else {
			h = base + warnLine
		}
	}
	pad := int32(14 * d.scale)
	d.titleRect = windows.Rect{Left: pad, Top: int32(12 * d.scale), Right: w - pad, Bottom: int32(32 * d.scale)}
	d.closeRect = windows.Rect{Left: w - pad - int32(22*d.scale), Top: int32(10 * d.scale), Right: w - pad, Bottom: int32(32 * d.scale)}
	statusY := int32(42 * d.scale)
	dotSize := int32(11 * d.scale)
	d.dotRect = windows.Rect{Left: pad + int32(1*d.scale), Top: statusY + int32(3*d.scale),
		Right: pad + dotSize + int32(1*d.scale), Bottom: statusY + dotSize + int32(3*d.scale)}
	if len(d.overview) > 0 {
		d.bodyRect = windows.Rect{Left: pad, Top: statusY, Right: w - pad,
			Bottom: statusY + rowH*int32(len(d.overview))}
	} else {
		d.bodyRect = windows.Rect{Left: pad, Top: statusY + int32(20*d.scale), Right: w - pad, Bottom: statusY + int32(38*d.scale)}
	}
	if warnLine > 0 {
		d.warnRect = windows.Rect{Left: pad, Top: d.bodyRect.Bottom, Right: w - pad, Bottom: d.bodyRect.Bottom + warnLine}
	}

	// Buttons, right-aligned: [Disconnect] [Open Window]. The overview
	// bubble only describes — Disconnect has no meaningful "all of the
	// above" target there — so it keeps just Open Window, flush right.
	texts := popupTextsFor(lang)
	measDC, _, _ := procGetDC.Call(0)
	btnH := int32(28 * d.scale)
	btnY := h - pad - btnH
	discW := int32(0)
	if len(d.overview) == 0 {
		discW = measureText(measDC, d.bodyFont, texts.discLabel) + int32(24*d.scale)
	}
	openW := measureText(measDC, d.bodyFont, texts.openLabel) + int32(24*d.scale)
	if measDC != 0 {
		procReleaseDC.Call(0, measDC)
	}
	d.discRect = windows.Rect{Left: w - pad - discW, Top: btnY, Right: w - pad, Bottom: btnY + btnH}
	if len(d.overview) > 0 {
		d.discRect = windows.Rect{} // zero rect: never hit
		d.openRect = windows.Rect{Left: w - pad - openW, Top: btnY, Right: w - pad, Bottom: btnY + btnH}
	} else {
		d.openRect = windows.Rect{Left: w - pad - discW - int32(8*d.scale) - openW, Top: btnY,
			Right: w - pad - discW - int32(8*d.scale), Bottom: btnY + btnH}
	}

	x, y := popupPosition(w, h, d.scale)
	hinst, _, _ := procGetModuleHandleW.Call(0)
	hwnd, _, _ := procCreateWindowExW.Call(
		wsExTopmost|wsExToolWindow|wsExNoActivate,
		uintptr(unsafe.Pointer(popupClassNamePtr)),
		0,
		wsPopup,
		x, y, uintptr(w), uintptr(h),
		0, 0, hinst, 0,
	)
	if hwnd == 0 {
		slog.Warn("popup: CreateWindowEx failed")
		cleanupFonts()
		return
	}
	d.hwnd = hwnd
	popupHwnds.Store(hwnd, d)
	activePopupHWND.Store(hwnd)
	slog.Info("popup: window created", "hwnd", hwnd, "x", x, "y", y, "w", w, "h", h, "dpi", d.dpi)

	// Rounded corners (the region is owned by the window after SetWindowRgn).
	if hrgn, _, _ := procCreateRoundRectRgn.Call(0, 0, uintptr(w), uintptr(h),
		uintptr(int32(12*d.scale)), uintptr(int32(12*d.scale))); hrgn != 0 {
		procSetWindowRgn.Call(hwnd, hrgn, 1)
	}

	procSetTimer.Call(hwnd, 1, uintptr(duration.Milliseconds()), 0)
	procShowWindow.Call(hwnd, swShowNoActivate)

	var m popupMsg
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if ret == 0 || ret == ^uintptr(0) { // WM_QUIT or error
			slog.Info("popup: message loop exited", "getMessageRet", ret)
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}

	popupHwnds.Delete(hwnd)
	if activePopupHWND.Load() == hwnd {
		activePopupHWND.Store(0)
	}
	cleanupFonts()
}

// popupPosition places the bubble near the notification area based on the
// taskbar edge, inside the primary display work area.
func popupPosition(w, h int32, scale float32) (uintptr, uintptr) {
	var work windows.Rect
	procSystemParametersInfoW.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&work)), 0)
	margin := int32(12 * scale)
	edge := uint32(abeBottom)
	var abd appBarData
	abd.cbSize = uint32(unsafe.Sizeof(abd))
	if r, _, _ := procSHAppBarMessage.Call(abmGetTaskbarPos, uintptr(unsafe.Pointer(&abd))); r != 0 {
		edge = abd.uEdge
	}
	switch edge {
	case abeTop:
		return uintptr(work.Right - w - margin), uintptr(work.Top + margin)
	case abeLeft:
		return uintptr(work.Left + margin), uintptr(work.Bottom - h - margin)
	case abeRight:
		return uintptr(work.Right - w - margin), uintptr(work.Top + margin)
	default: // abeBottom
		return uintptr(work.Right - w - margin), uintptr(work.Bottom - h - margin)
	}
}

func createPopupFont(d *popupData, px int, bold bool) uintptr {
	lf := logFont{
		lfHeight:        -int32(float32(px) * d.scale),
		lfWeight:        400,
		lfCharSet:       1, // DEFAULT_CHARSET
		lfOutPrecision:  0,
		lfClipPrecision: 2, // CLIP_DEFAULT_PRECIS
		lfQuality:       5, // CLEARTYPE_QUALITY
	}
	if bold {
		lf.lfWeight = 600
	}
	family, _ := windows.UTF16FromString(popupFontFamily(d.lang))
	copy(lf.lfFaceName[:], family)
	font, _, _ := procCreateFontIndirectW.Call(uintptr(unsafe.Pointer(&lf)))
	return font
}

// ---- Window procedure ----

func popupWindowProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	dAny, ok := popupHwnds.Load(hwnd)
	if !ok {
		return defWindowProc(hwnd, msg, wParam, lParam)
	}
	d := dAny.(*popupData)

	switch msg {
	case wmClose:
		slog.Info("popup: WM_CLOSE received")
		procDestroyWindow.Call(hwnd)
		return 0
	case wmTimer:
		slog.Info("popup: WM_TIMER fired", "wParam", wParam)
		procKillTimer.Call(hwnd, wParam)
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		slog.Info("popup: WM_DESTROY")
		procPostQuitMessage.Call(0)
		return 0
	case wmEraseBkgnd:
		return 1 // skip background erase to avoid flicker
	case wmPaint:
		drawPopup(d)
		return 0
	case wmLButtonDown:
		slog.Info("popup: WM_LBUTTONDOWN")
		procSetCapture.Call(hwnd)
		return 0
	case wmLButtonUp:
		procReleaseCapture.Call(hwnd)
		p := pointFromLPARAM(lParam)
		switch {
		case inRect(p, d.closeRect):
			procDestroyWindow.Call(hwnd)
		case inRect(p, d.discRect):
			procDestroyWindow.Call(hwnd)
			if d.onDisconnect != nil {
				// Run async: the callbacks (tunnel service RPCs,
				// showDock's InvokeSync) may block; holding this
				// bubble's message loop would stall WM_TIMER/WM_QUIT.
				go d.onDisconnect()
			}
		case inRect(p, d.openRect):
			procDestroyWindow.Call(hwnd)
			if d.onOpen != nil {
				go d.onOpen()
			}
		case inRect(p, d.bodyRect) || inRect(p, d.titleRect):
			procDestroyWindow.Call(hwnd)
			if d.onOpen != nil {
				go d.onOpen()
			}
		}
		return 0
	case wmMouseMove:
		p := pointFromLPARAM(lParam)
		hClose := inRect(p, d.closeRect)
		hOpen := inRect(p, d.openRect)
		hDisc := inRect(p, d.discRect)
		if hClose != d.hoverClose || hOpen != d.hoverOpen || hDisc != d.hoverDisc {
			d.hoverClose, d.hoverOpen, d.hoverDisc = hClose, hOpen, hDisc
			procInvalidateRect.Call(hwnd, 0, 1)
		}
		trackMouseLeave(hwnd)
		return 0
	case wmMouseLeave:
		if d.hoverClose || d.hoverOpen || d.hoverDisc {
			d.hoverClose, d.hoverOpen, d.hoverDisc = false, false, false
			procInvalidateRect.Call(hwnd, 0, 1)
		}
		return 0
	}
	return defWindowProc(hwnd, msg, wParam, lParam)
}

func defWindowProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

// ---- Drawing ----

func drawPopup(d *popupData) {
	hwnd := d.hwnd
	var ps paintStruct
	hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return
	}
	defer procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))

	var client windows.Rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
	bg, fg, border, _, _, warnColor := popupPalette(d.themeLight)
	texts := popupTextsFor(d.lang)

	// Background
	brush := createSolidBrush(bg)
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&client)), brush)
	procDeleteObject.Call(brush)

	// Border
	pen := createPen(0, 1, border)
	oldPen := selectObject(hdc, pen)
	oldBrush := selectObject(hdc, getStockObject(nullBrush))
	procRectangle.Call(hdc, 0, 0, uintptr(client.Right-client.Left), uintptr(client.Bottom-client.Top))
	selectObject(hdc, oldPen)
	selectObject(hdc, oldBrush)
	procDeleteObject.Call(pen)

	// Title
	selectFont(hdc, d.titleFont)
	procSetBkMode.Call(hdc, transparentBkMode)
	procSetTextColor.Call(hdc, uintptr(fg))
	drawTextOut(hdc, "WireGuide Plus", d.titleRect.Left, d.titleRect.Top)

	// Close ✕
	drawCloseGlyph(hdc, d, fg)

	if len(d.overview) > 0 {
		drawOverviewRows(hdc, d, texts, fg)
	} else {
		// Status mark + caption: ✅-style green check for connected, ❌-style
		// red cross for disconnected, 🟡-style yellow disc while connecting.
		// A tunnel that is still connecting gets neither the check nor the
		// cross — it has not connected yet and may still fail.
		statusText := texts.connected
		state := "connected"
		switch d.state {
		case popupStateConnecting:
			state = "connecting"
			statusText = texts.connecting
		case popupStateConnected:
			// keep check + "Connected"
		default:
			state = "disconnected"
			statusText = texts.notConnected
		}
		drawStatusMark(hdc, d.dotRect, state, d.scale, d.themeLight)

		// Status line ("已连接" / "连接中" / "未连接") next to the mark,
		// in the mark's colour: green for connected, red for lost, yellow
		// while still trying.
		selectFont(hdc, d.bodyFont)
		_, ink := statusMarkInk(state, d.themeLight)
		procSetTextColor.Call(hdc, uintptr(ink))
		drawTextOut(hdc, statusText, d.dotRect.Right+int32(8*d.scale), statusLineTop(d))

		// Tunnel names (single line with ellipsis)
		bodyText := strings.Join(d.names, namesSeparator(d.lang))
		drawTextClipped(hdc, d.bodyFont, fg, bodyText, d.bodyRect)
	}

	// Warning line: probe targets that are not routed through this tunnel.
	// Amber, not red — the tunnel itself is up and carrying traffic; it is
	// the *reading* for those targets that is meaningless, which is a
	// setting to revisit rather than a failure.
	if len(d.outOfRange) > 0 {
		warn := texts.outOfRange + ": " + strings.Join(d.outOfRange, namesSeparator(d.lang))
		drawTextClipped(hdc, d.bodyFont, warnColor, warn, d.warnRect)
	}

	// Buttons
	drawPopupButton(hdc, d, d.openRect, texts.openLabel, d.hoverOpen)
	if len(d.overview) == 0 {
		drawPopupButton(hdc, d, d.discRect, texts.discLabel, d.hoverDisc)
	}
}

// drawOverviewRows renders the launch overview: one row per tunnel —
// status mark (✅/❌/🟡), name (ellipsized), and a right-aligned
// "state · auto/manual" tag with the state word in the mark's colour.
// The vocabulary is identical to the transition bubbles', so the overview
// can never claim more than they are allowed to say.
func drawOverviewRows(hdc uintptr, d *popupData, texts popupTexts, fg uint32) {
	rowH := int32(16 * d.scale)
	markSize := int32(11 * d.scale)
	textTop := int32(3 * d.scale)
	for i, r := range d.overview {
		top := d.bodyRect.Top + int32(i)*rowH
		word := texts.notConnected
		switch r.State {
		case "connected":
			word = texts.connected
		case "connecting":
			word = texts.connecting
		}
		auto := texts.manualLabel
		if r.Auto {
			auto = texts.autoLabel
		}
		sep := " · "
		rw := measureText(hdc, d.bodyFont, word+sep+auto)
		markRect := windows.Rect{Left: d.bodyRect.Left, Top: top + textTop,
			Right: d.bodyRect.Left + markSize, Bottom: top + textTop + markSize}
		drawStatusMark(hdc, markRect, r.State, d.scale, d.themeLight)

		nameRect := windows.Rect{
			Left:   d.bodyRect.Left + markSize + int32(6*d.scale),
			Top:    top,
			Right:  d.bodyRect.Right - rw - int32(10*d.scale),
			Bottom: top + rowH,
		}
		drawTextClipped(hdc, d.bodyFont, fg, r.Name, nameRect)

		_, ink := statusMarkInk(r.State, d.themeLight)
		wordW := measureText(hdc, d.bodyFont, word)
		x := d.bodyRect.Right - rw
		textY := top + textTop + int32(2*d.scale)
		procSetTextColor.Call(hdc, uintptr(ink))
		drawTextOut(hdc, word, x, textY)
		procSetTextColor.Call(hdc, uintptr(fg))
		drawTextOut(hdc, sep+auto, x+wordW, textY)
	}
}

// statusMarkInk returns the (mark, text) colours for a connection state:
// green for connected, red for lost, yellow while still trying. The text
// yellow darkens on a light background where the pure disc colour is hard
// to read.
func statusMarkInk(state string, light bool) (mark, text uint32) {
	switch state {
	case "connected":
		if light {
			return rgb(0x2F, 0xBF, 0x71), rgb(0x1F, 0x8A, 0x4C)
		}
		return rgb(0x34, 0xC7, 0x59), rgb(0x34, 0xC7, 0x59)
	case "connecting":
		if light {
			return rgb(0xF5, 0xC5, 0x18), rgb(0x9A, 0x6D, 0x00)
		}
		return rgb(0xFF, 0xD6, 0x0A), rgb(0xFF, 0xD6, 0x0A)
	default:
		if light {
			return rgb(0xE0, 0x3A, 0x3A), rgb(0xC0, 0x10, 0x10)
		}
		return rgb(0xFF, 0x45, 0x3A), rgb(0xFF, 0x45, 0x3A)
	}
}

// drawStatusMark renders the per-state glyph in the square r: a green
// check for connected, a red cross for disconnected, a filled yellow disc
// while connecting. GDI has no colour-emoji renderer, so these are the
// vector equivalents of ✅ / ❌ / 🟡 — same shapes, same semantics.
func drawStatusMark(hdc uintptr, r windows.Rect, state string, scale float32, light bool) {
	mark, _ := statusMarkInk(state, light)
	switch state {
	case "connecting":
		brush := createSolidBrush(mark)
		oldPen := selectObject(hdc, getStockObject(nullPen))
		oldBrush := selectObject(hdc, brush)
		procEllipse.Call(hdc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom))
		selectObject(hdc, oldPen)
		selectObject(hdc, oldBrush)
		procDeleteObject.Call(brush)
	case "connected":
		w := int32(2 * scale)
		if w < 1 {
			w = 1
		}
		pen := createPen(0, w, mark)
		oldPen := selectObject(hdc, pen)
		x := float32(r.Left)
		y := float32(r.Top)
		s := float32(r.Right - r.Left)
		procMoveToEx.Call(hdc, uintptr(int32(x+0.10*s)), uintptr(int32(y+0.52*s)), 0)
		procLineTo.Call(hdc, uintptr(int32(x+0.40*s)), uintptr(int32(y+0.82*s)), 0)
		procLineTo.Call(hdc, uintptr(int32(x+0.92*s)), uintptr(int32(y+0.18*s)), 0)
		selectObject(hdc, oldPen)
		procDeleteObject.Call(pen)
	default:
		w := int32(2 * scale)
		if w < 1 {
			w = 1
		}
		pen := createPen(0, w, mark)
		oldPen := selectObject(hdc, pen)
		procMoveToEx.Call(hdc, uintptr(r.Left), uintptr(r.Top), 0)
		procLineTo.Call(hdc, uintptr(r.Right), uintptr(r.Bottom), 0)
		procMoveToEx.Call(hdc, uintptr(r.Right), uintptr(r.Top), 0)
		procLineTo.Call(hdc, uintptr(r.Left), uintptr(r.Bottom), 0)
		selectObject(hdc, oldPen)
		procDeleteObject.Call(pen)
	}
}

func statusLineTop(d *popupData) int32 {
	return d.bodyRect.Top - int32(20*d.scale) + int32(2*d.scale)
}

func drawPopupButton(hdc uintptr, d *popupData, r windows.Rect, label string, hover bool) {
	_, fg, border, btnHover, _, _ := popupPalette(d.themeLight)
	if hover {
		brush := createSolidBrush(btnHover)
		procFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), brush)
		procDeleteObject.Call(brush)
	}
	pen := createPen(0, 1, border)
	oldPen := selectObject(hdc, pen)
	oldBrush := selectObject(hdc, getStockObject(nullBrush))
	procRoundRect.Call(hdc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom),
		uintptr(int32(6*d.scale)), uintptr(int32(6*d.scale)))
	selectObject(hdc, oldPen)
	selectObject(hdc, oldBrush)
	procDeleteObject.Call(pen)

	selectFont(hdc, d.bodyFont)
	procSetBkMode.Call(hdc, transparentBkMode)
	procSetTextColor.Call(hdc, uintptr(fg))
	tw := measureText(hdc, d.bodyFont, label)
	x := r.Left + (r.Right-r.Left-tw)/2
	y := r.Top + (r.Bottom-r.Top-int32(17*d.scale))/2
	drawTextOut(hdc, label, x, y)
}

func drawCloseGlyph(hdc uintptr, d *popupData, color uint32) {
	pen := createPen(0, 1, color)
	oldPen := selectObject(hdc, pen)
	cx := (d.closeRect.Left + d.closeRect.Right) / 2
	cy := (d.closeRect.Top + d.closeRect.Bottom) / 2
	r := int32(5 * d.scale)
	procMoveToEx.Call(hdc, uintptr(cx-r), uintptr(cy-r), 0)
	procLineTo.Call(hdc, uintptr(cx+r), uintptr(cy+r))
	procMoveToEx.Call(hdc, uintptr(cx-r), uintptr(cy+r), 0)
	procLineTo.Call(hdc, uintptr(cx+r), uintptr(cy-r))
	selectObject(hdc, oldPen)
	procDeleteObject.Call(pen)
}

func drawTextClipped(hdc uintptr, font uintptr, color uint32, s string, r windows.Rect) {
	if s == "" {
		return
	}
	selectFont(hdc, font)
	procSetBkMode.Call(hdc, transparentBkMode)
	procSetTextColor.Call(hdc, uintptr(color))
	text := windows.StringToUTF16Ptr(s)
	procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(text)), ^uintptr(0),
		uintptr(unsafe.Pointer(&r)), dtTop|dtSingleLine|dtEndEllipsis)
}

func drawTextOut(hdc uintptr, s string, x, y int32) {
	if s == "" {
		return
	}
	u16, err := windows.UTF16FromString(s)
	if err != nil {
		return
	}
	procTextOutW.Call(hdc, uintptr(x), uintptr(y), uintptr(unsafe.Pointer(&u16[0])), uintptr(len(u16)))
}

func measureText(hdc uintptr, font uintptr, s string) int32 {
	if s == "" {
		return 0
	}
	selectFont(hdc, font)
	var sz popupPoint
	u16, err := windows.UTF16FromString(s)
	if err != nil {
		return 0
	}
	procGetTextExtentPoint32W.Call(hdc, uintptr(unsafe.Pointer(&u16[0])), uintptr(len(u16)), uintptr(unsafe.Pointer(&sz)))
	return sz.X
}

// ---- GDI helpers ----

func createSolidBrush(color uint32) uintptr {
	b, _, _ := procCreateSolidBrush.Call(uintptr(color))
	return b
}

func createPen(style, width int32, color uint32) uintptr {
	p, _, _ := procCreatePen.Call(uintptr(style), uintptr(width), uintptr(color))
	return p
}

func selectObject(hdc, obj uintptr) uintptr {
	prev, _, _ := procSelectObject.Call(hdc, obj)
	return prev
}

func selectFont(hdc, font uintptr) {
	procSelectObject.Call(hdc, font)
}

func getStockObject(kind int32) uintptr {
	o, _, _ := procGetStockObject.Call(uintptr(kind))
	return o
}

func inRect(p popupPoint, r windows.Rect) bool {
	return p.X >= r.Left && p.X < r.Right && p.Y >= r.Top && p.Y < r.Bottom
}

func pointFromLPARAM(lParam uintptr) popupPoint {
	return popupPoint{
		X: int32(int16(lParam & 0xFFFF)),
		Y: int32(int16((lParam >> 16) & 0xFFFF)),
	}
}

func trackMouseLeave(hwnd uintptr) {
	tme := trackMouseEvent{cbSize: uint32(unsafe.Sizeof(trackMouseEvent{})), dwFlags: 2, hWnd: hwnd}
	procTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
}
