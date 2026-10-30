package gui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"math"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	wgapp "github.com/imonior/wireguide-plus/internal/app"
	"github.com/imonior/wireguide-plus/internal/version"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/icons"
	"golang.org/x/image/draw"
)

// Tray icon variants. macOS uses plain (non-template) coloured icons so
// the connected state can keep its GREEN dot badge — template images are
// alpha-only masks and cannot carry colour, which is why this design was
// chosen originally. Legibility across menu-bar appearances (issue #18:
// a fixed white W is invisible on a light menu bar) is handled the way
// native colour-icon apps do it: a KVO observation of the status-item
// button's effectiveAppearance (menubar_appearance_darwin.go), which
// tracks the menu bar's ACTUAL rendered appearance — wallpaper-driven,
// not the system theme — and swaps between the white-W and black-W
// sets. The default is white (historical behaviour); black is used only
// when the observer verifiably reports a light menu bar. We never call
// SetTemplateIcon, so the Wails v3 sticky-isTemplateIcon bug (once set,
// later SetIcon calls render monochrome) is never triggered.
// appQuitting latches once the user picks Quit from the tray. Package-level
// (unlike trayManager.quitting, which is per-instance) so the window-show
// retry goroutines in the dock_* files can observe teardown without holding
// a trayManager reference.
var appQuitting atomic.Bool

// Menu connection glyphs. Menu item text itself cannot be coloured on any
// of the three platforms (AppKit/Win32/GTK menus draw labels with the theme
// ink), so colour has to ride on the glyph — which means emoji there, since
// plain text characters are monochrome. Colored emoji render natively in the
// macOS menu (AppKit), but the Windows tray popup is drawn with GDI, which
// cannot render colored emoji — a 🟢 degrades to a grey outline circle
// there. Windows therefore uses plain vector glyphs (✔ / ✗) that stay
// legible in both light and dark themes, and accepts their monochrome ink.
// connectingGlyph marks a tunnel that is up but has not completed a
// WireGuard handshake yet (or is still dialling): present and moving, not
// yet carrying traffic. It exists so "established" never borrows the
// connected glyph while the peer is silent.
// connectingGlyph 标记「已建立但尚未握手」（或仍在拨号）的隧道：已经在动，
// 但还没真正通流量。有它之后，"已建立"就不会在对端沉默时借用"已连接"的符号。
var (
	connectedGlyph    = "🟢"
	connectingGlyph   = "🟡"
	disconnectedGlyph = "🔴"
)

func init() {
	if runtime.GOOS == "windows" {
		// GDI renders these as plain text glyphs (no colored emoji). A
		// filled ✔ vs hollow ✗ reads far better than ●/○, which both look
		// like grey circles in the small tray font. "…" is the monochrome
		// stand-in for the yellow disc: legible in both themes and in the
		// small menu font, unlike the coloured emoji.
		connectedGlyph = "✔"
		connectingGlyph = "…"
		disconnectedGlyph = "✗"
	}
}

// trayClickWindow is how close two left clicks must land for the second to
// count as a double-click. A fixed value rather than the OS setting because
// the router is deliberately cross-platform: Wails only hands us "a click
// happened" (macOS fires one callback per mouse-down and offers no
// double-click event at all; Windows fires one per button-UP, so a double
// click delivers UP, UP with a DBLCLK in between that we ignore). 450ms
// covers the default double-click interval on every platform while staying
// snappy for the menu.
const trayClickWindow = 450 * time.Millisecond

// trayClickRouter turns raw tray clicks into the two user intents the app
// recognises, identically on all platforms: a SINGLE left click opens the
// context menu, a DOUBLE click shows the main window. The menu is delayed
// by trayClickWindow so a second click can cancel it — the unavoidable
// price of separating the two when only click events arrive. Right clicks
// never reach the router: Wails' smart default (and, on Linux, the panel
// itself) already shows the menu there.
type trayClickRouter struct {
	mu      sync.Mutex
	last    time.Time
	pending *time.Timer
	open    func()
	show    func()
}

func newTrayClickRouter(openMenu, showWindow func()) *trayClickRouter {
	return &trayClickRouter{open: openMenu, show: showWindow}
}

// onClick is the tray OnClick callback. Both actions are safe from any
// goroutine: openMenu round-trips the UI thread via InvokeSync, and
// showDock (showWindow) is InvokeAsync-based precisely so tray callbacks
// can call it from any thread.
func (r *trayClickRouter) onClick() {
	now := time.Now()
	r.mu.Lock()
	if !r.last.IsZero() && now.Sub(r.last) <= trayClickWindow {
		// Second click of a double-click: stand the pending menu down.
		// Clearing last stops a third rapid click from chaining into a
		// second "double".
		r.last = time.Time{}
		if r.pending != nil {
			r.pending.Stop()
			r.pending = nil
		}
		r.mu.Unlock()
		r.show()
		return
	}
	r.last = now
	if r.pending != nil {
		r.pending.Stop()
	}
	r.pending = time.AfterFunc(trayClickWindow, func() {
		r.mu.Lock()
		r.pending = nil
		if !r.last.IsZero() {
			r.last = time.Time{}
			open := r.open
			r.mu.Unlock()
			open()
		} else {
			r.mu.Unlock()
		}
	})
	r.mu.Unlock()
}

var (
	trayOnIconDark   []byte // white W + green dot (dark menu bar)
	trayOffIconDark  []byte // white W, no badge
	trayOnIconLight  []byte // black W + green dot (light menu bar)
	trayOffIconLight []byte // black W, no badge

	// macOS variants rendered from the full app icon (red disc + white
	// dragon glyph) so the menu-bar icon matches the app icon. Built at
	// runtime after SetTrayIconPNG supplies the source PNG (init() can't
	// see customTrayIconPNG); nil leaves macIcons on the monochrome W
	// template pair above.
	trayOnIconMac  []byte // app icon + green dot badge
	trayOffIconMac []byte // app icon, no badge

	// Windows/Linux variants rendered from the full app icon (rounded
	// red tile + white W) so the system tray actually shows something
	// visible against light tray backgrounds.
	trayOnIconWindows  []byte // app icon + green dot badge
	trayOffIconWindows []byte // app icon, no badge

	// customTrayIconPNG is the source the macOS/Windows builders read
	// from. Set by SetTrayIconPNG before gui.Run; init can't read it
	// because Go runs package init() at load time, well before main
	// wires this up.
	customTrayIconPNG []byte
)

// SetTrayIconPNG hands the GUI package the raw bytes of the source app
// icon (a 1024×1024 RGBA PNG today). main.go embeds the file and calls
// this before invoking Run. The package keeps a copy so the Windows
// tray-icon builders can read it later. Calling with an empty slice
// disables the custom Windows tray icon (we fall back to the template).
func SetTrayIconPNG(b []byte) { customTrayIconPNG = b }

func init() {
	white := color.NRGBA{255, 255, 255, 255}
	black := color.NRGBA{0, 0, 0, 255}
	trayOnIconDark = buildTrayOnIcon(white)
	trayOffIconDark = buildTrayOffIcon(white)
	trayOnIconLight = buildTrayOnIcon(black)
	trayOffIconLight = buildTrayOffIcon(black)
}

// buildWindowsTrayIcons renders the rounded-red app icon at tray size
// for Windows, optionally with a green dot badge for the "connected"
// state. Called once from gui.Run after SetTrayIconPNG has populated
// the source bytes. Safe to call with an unset source — leaves the
// Windows variants nil and setIconState falls back to its existing
// macOS template path.
//
// We render at 32×32: Windows tray DPI scales icons to fit, and 32 is
// the next sane size up from 16 with enough room for anti-aliasing on
// the rounded corners and the W glyph. The source is downsampled with
// CatmullRom (high-quality bicubic) so the W stays crisp.
func buildWindowsTrayIcons() {
	if len(customTrayIconPNG) == 0 {
		return
	}
	src, err := png.Decode(bytes.NewReader(customTrayIconPNG))
	if err != nil {
		slog.Warn("tray: decode appicon for Windows tray failed", "error", err)
		return
	}

	const trayPx = 32
	// Corner radius chosen to mirror the inner red tile's curve so the
	// rounded outer silhouette and the inner red rounded square sit
	// concentric — that's the "일체감" (unified feel) the user asked for
	// instead of a hard white square framing the red tile.
	const trayCornerRadius = 7
	render := func(withBadge bool) []byte {
		dst := image.NewNRGBA(image.Rect(0, 0, trayPx, trayPx))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
		// Force-round the outer silhouette. Wails on Windows passes the
		// PNG straight to CreateIconFromResourceEx, which honours alpha,
		// so zeroing the alpha outside the rounded rect guarantees the
		// tray draws a rounded shape — independent of whether the
		// resized appicon happened to leave a few white-ish pixels in
		// the corners from CatmullRom's bleed at the tile edge.
		applyRoundedCorners(dst, trayCornerRadius)
		if withBadge {
			drawGreenBadge(dst)
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, dst); err != nil {
			slog.Warn("tray: encode Windows tray icon failed", "error", err)
			return nil
		}
		return buf.Bytes()
	}

	trayOffIconWindows = render(false)
	trayOnIconWindows = render(true)
}

// buildMacTrayIcons renders the app icon (red disc + white dragon glyph)
// at menu-bar size for macOS, optionally with a green dot badge for the
// "connected" state. Called once from gui.Run after SetTrayIconPNG has
// populated the source bytes. Safe to call with an unset source — leaves
// the macOS variants nil and macIcons falls back to the monochrome W
// template set.
//
// macOS renders at 64×64, matching the previous template's native size
// (a crisp 22pt @3x in the menu bar). The app icon is a red disc with a
// white dragon, so it stays legible on both dark and light menu bars —
// the colour no longer has to follow the menu-bar appearance, which is
// why macIcons prefers these over the white/black W pair. The disc's own
// rounded silhouette carries through the CatmullRom downscale, so no
// extra corner masking is needed (unlike Windows' square tile).
func buildMacTrayIcons() {
	if len(customTrayIconPNG) == 0 {
		return
	}
	src, err := png.Decode(bytes.NewReader(customTrayIconPNG))
	if err != nil {
		slog.Warn("tray: decode appicon for macOS tray failed", "error", err)
		return
	}

	const trayPx = 64
	render := func(withBadge bool) []byte {
		dst := image.NewNRGBA(image.Rect(0, 0, trayPx, trayPx))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
		if withBadge {
			drawGreenBadge(dst)
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, dst); err != nil {
			slog.Warn("tray: encode macOS tray icon failed", "error", err)
			return nil
		}
		return buf.Bytes()
	}

	trayOffIconMac = render(false)
	trayOnIconMac = render(true)
}

// applyRoundedCorners zeros the alpha of pixels that lie outside a
// centered rounded rectangle of the given corner radius. A 1-pixel ring
// at the boundary is set to a fractional alpha proportional to its
// distance from the corner centre so the rounded edge is anti-aliased
// instead of stair-stepped.
//
// This runs AFTER the appicon is composited and is the last step before
// PNG encode — anything we want to keep visible must already be drawn.
func applyRoundedCorners(img *image.NRGBA, radius int) {
	if radius <= 0 {
		return
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Identify which corner's circle this pixel belongs to.
			// The "straight edge" interior of the rounded rect needs no
			// mutation — bail early so the inner pixels are untouched.
			var cx, cy int
			switch {
			case x < radius && y < radius:
				cx, cy = radius, radius
			case x >= w-radius && y < radius:
				cx, cy = w-1-radius, radius
			case x < radius && y >= h-radius:
				cx, cy = radius, h-1-radius
			case x >= w-radius && y >= h-radius:
				cx, cy = w-1-radius, h-1-radius
			default:
				continue
			}
			dx, dy := float64(x-cx), float64(y-cy)
			d := math.Sqrt(dx*dx + dy*dy)
			r := float64(radius)
			switch {
			case d > r:
				img.SetNRGBA(x, y, color.NRGBA{})
			case d > r-1:
				// Anti-aliased edge: fade alpha from full at d=r-1 to
				// zero at d=r. Multiply by existing alpha so the
				// underlying icon's own translucency is preserved.
				c := img.NRGBAAt(x, y)
				c.A = uint8(float64(c.A) * (r - d))
				img.SetNRGBA(x, y, c)
			}
		}
	}
}

// drawGreenBadge paints a small green disc in the bottom-left corner of
// img, matching the macOS "connected" badge position. Anti-aliasing is
// a 1-pixel ring at the disc boundary so the badge doesn't look jagged
// at 32×32.
func drawGreenBadge(img *image.NRGBA) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	// Badge geometry: 7px radius disc, centered ~6px from each edge.
	r := h / 5
	if r < 4 {
		r = 4
	}
	cx, cy := r+1, h-r-1
	green := color.NRGBA{52, 199, 89, 255} // macOS systemGreen
	for y := cy - r - 1; y <= cy+r+1; y++ {
		for x := cx - r - 1; x <= cx+r+1; x++ {
			if x < 0 || y < 0 || x >= w || y >= h {
				continue
			}
			dx, dy := x-cx, y-cy
			d2 := dx*dx + dy*dy
			r2 := r * r
			switch {
			case d2 <= r2:
				img.SetNRGBA(x, y, green)
			case d2 <= (r+1)*(r+1):
				// 1-pixel AA ring: blend by alpha proportional to
				// how far past the disc edge the pixel sits.
				img.SetNRGBA(x, y, color.NRGBA{green.R, green.G, green.B, 128})
			}
		}
	}
}

// trimAndSquare finds the bounding box of non-transparent pixels, crops,
// then centers in a square canvas (max of width/height). Wails forces
// the tray icon to a thickness×thickness square, so providing a square
// image avoids distortion and controls the padding ourselves.
func trimAndSquare(src image.Image) *image.NRGBA {
	b := src.Bounds()
	minX, minY, maxX, maxY := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := src.At(x, y).RGBA()
			if a > 0 {
				if x < minX {
					minX = x
				}
				if y < minY {
					minY = y
				}
				if x > maxX {
					maxX = x
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if maxX < minX {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	cropW := maxX - minX + 1
	cropH := maxY - minY + 1
	// Square canvas: use the larger dimension
	side := cropW
	if cropH > side {
		side = cropH
	}
	dst := image.NewNRGBA(image.Rect(0, 0, side, side))
	offX := (side - cropW) / 2
	offY := (side - cropH) / 2
	for y := 0; y < cropH; y++ {
		for x := 0; x < cropW; x++ {
			dst.Set(x+offX, y+offY, src.At(x+minX, y+minY))
		}
	}
	return dst
}

// buildTrayOnIcon composites a W glyph (in wColor) with a green dot badge
// at the bottom-left — the "connected" state. wColor should be white for
// dark menu bars and black for light menu bars; the dot stays green in
// both variants (the whole point of using non-template icons).
func buildTrayOnIcon(wColor color.NRGBA) []byte {
	base, err := png.Decode(bytes.NewReader(icons.SystrayMacTemplate))
	if err != nil {
		slog.Warn("failed to decode base tray icon, using unmodified", "error", err)
		return icons.SystrayMacTemplate
	}

	trimmed := trimAndSquare(base)
	bounds := trimmed.Bounds()

	// Re-tint: replace black pixels with wColor, preserving alpha.
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := trimmed.At(x, y).RGBA()
			if a > 0 {
				trimmed.SetNRGBA(x, y, color.NRGBA{
					R: wColor.R,
					G: wColor.G,
					B: wColor.B,
					A: uint8(a >> 8),
				})
			}
		}
	}

	// Green badge: bottom-left corner.
	w, h := bounds.Dx(), bounds.Dy()
	cx, cy, r := w/5, h-h/5, h/8
	if r < 3 {
		r = 3
	}
	green := color.NRGBA{52, 199, 89, 255} // macOS systemGreen
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= r*r && x >= 0 && y >= 0 && x < w && y < h {
				trimmed.SetNRGBA(x, y, green)
			}
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, trimmed); err != nil {
		slog.Warn("failed to encode tray-on icon, using unmodified", "error", err)
		return icons.SystrayMacTemplate
	}
	return buf.Bytes()
}

// buildTrayOffIcon renders the W glyph in wColor with no badge — the
// disconnected state.
func buildTrayOffIcon(wColor color.NRGBA) []byte {
	base, err := png.Decode(bytes.NewReader(icons.SystrayMacTemplate))
	if err != nil {
		slog.Warn("failed to decode base tray icon, using unmodified", "error", err)
		return icons.SystrayMacTemplate
	}

	trimmed := trimAndSquare(base)
	bounds := trimmed.Bounds()

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := trimmed.At(x, y).RGBA()
			if a > 0 {
				trimmed.SetNRGBA(x, y, color.NRGBA{
					R: wColor.R,
					G: wColor.G,
					B: wColor.B,
					A: uint8(a >> 8),
				})
			}
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, trimmed); err != nil {
		slog.Warn("failed to encode tray-off icon, using unmodified", "error", err)
		return icons.SystrayMacTemplate
	}
	return buf.Bytes()
}

// trayManager owns the system tray menu and its visual state.
//
// There are TWO update paths, intentionally separate:
//
//  1. setIconState(activeName) — cheap, called from the status event stream
//     every second. Only touches label + tooltip. NO IPC, NO disk I/O, so it
//     never blocks the event loop goroutine.
//
//  2. rebuildMenu() — expensive, rebuilds the full tunnel list in the menu.
//     Called only on user actions that change the list (add, delete, rename)
//     or on explicit refresh after connect/disconnect finishes.
//
// The previous design called the full rebuildMenu on every status event and
// did an IPC round-trip to the helper inside ListTunnels — that blocked the
// event stream, making the UI feel sluggish under a 1 Hz status broadcast.

// tunnelConnState is the per-tunnel connection state the status bubble
// tracks, so it can announce only *transitions* (see lastNotifiedConnected
// in showStatusNotification). tcsUnknown means the tunnel is not currently
// active (it was dropped or never started).
type tunnelConnState int

const (
	tcsUnknown tunnelConnState = iota
	tcsConnecting
	tcsConnected
)

type trayManager struct {
	app        *application.App
	win        *application.WebviewWindow
	tray       *application.SystemTray
	svc        *wgapp.TunnelService
	doShutdown func()

	mu            sync.Mutex
	activeTunnels map[string]bool // cached from status events (connecting too)
	// establishedTunnels is the subset that finished setup (StateConnected).
	// Lifecycle truth: it says the interface is up, nothing about whether
	// the peer answers.
	establishedTunnels map[string]bool
	// upTunnels is the subset of establishedTunnels whose peer has completed
	// at least one WireGuard handshake — the tunnels the UI may CLAIM as
	// "connected" (green icon, tooltip, tray bubble, overview rows).
	//
	// Interface-up and tunnel-working are two different questions: a tunnel
	// whose peer is unreachable (wrong key, dead endpoint, upstream down)
	// finishes setup and sits in StateConnected forever while never
	// handshaking. Announcing that as "connected" is the bug this field
	// exists to prevent.
	//
	// "At least once" rather than "recently": staleness (no handshake inside
	// the threshold) toggles with peer silence, and feeding a toggling bit
	// into the transition engine produced phantom "X disconnected" bubbles
	// for tunnels that had simply gone idle. Freshness stays a colour-only
	// axis (see TunnelDetail's amber dot); announcements latch.
	upTunnels map[string]bool
	// lastNotifiedConnected records, per tunnel, the connection state the
	// last status bubble announced. The bubble is gated on a *transition*
	// (a tunnel going up/down/connecting) rather than on the raw established
	// set, so a tunnel that is stably up is announced exactly once and never
	// re-spammed on every 30s status poll (see #111: TS453Dmini was being
	// re-announced "connected" every cycle because the churner TS451D kept
	// dropping out of the set — the bubble must name the tunnel that
	// actually changed, with the correct direction).
	lastNotifiedConnected map[string]tunnelConnState
	// outOfRange caches the probe targets that are no longer routed through
	// their tunnel, as of the last status event. Surfaced in the status
	// bubble: when the window is closed the bubble is the only place the
	// user can be told, and the editor's colour marking is invisible then.
	outOfRange   []string
	hasHandshake map[string]bool // per-tunnel handshake status
	rebuildTimer *time.Timer     // debounce timer for rebuildMenu
	notifyTimer  *time.Timer     // debounce timer for the status notification
	// menu is the ONE Menu object backing the tray for the app's whole
	// lifetime. rebuildMenu clears and refills it in place instead of
	// creating a fresh Menu: Wails reuses the same NSMenu instance on
	// Update(), and AppKit live-updates a menu that is currently open —
	// swapping in a new NSMenu (the old design) left an open menu
	// showing stale connection glyphs until dismissed, and leaked the
	// replaced NSMenu on every rebuild. Guarded by rebuildMu.
	menu *application.Menu
	// rebuildMu serialises rebuildMenu calls so two concurrent rebuilds
	// can't interleave their ListTunnelsLocal snapshot and SetMenu —
	// rebuilds are sub-ms so queued callers never wait meaningfully.
	rebuildMu sync.Mutex
	// quitting flips to true the moment Quit is clicked. Both the
	// debounce AfterFunc and the rebuildMenu body short-circuit when
	// set, so a late-firing timer can't call SetMenu on a tray that
	// has already been Destroy()'d.
	quitting atomic.Bool
	// darkMenuBar tracks the menu bar's current rendered appearance
	// (macOS only) so setIconState can pick the icon variant whose W
	// glyph contrasts with the menu bar. Initialised to true (white W —
	// the app's historical behaviour, correct on the common dark menu
	// bar) and updated by the KVO observer in startAppearanceWatch; it
	// only ever becomes false when the menu bar is verifiably light.
	darkMenuBar atomic.Bool
	// iconMu serialises SetIcon calls to the underlying Wails tray. Wails'
	// windowsSystemTray destroys the previous HICON and creates the new
	// one inside SetIcon without its own lock, so concurrent callers
	// (the status stream and the self-heal heartbeat) can race on the
	// handle and destroy the wrong icon — exactly one of the ways the
	// tray icon can go blank while the app stays alive.
	iconMu sync.Mutex
	// iconHeartbeatDone terminates the self-heal ticker on quit. nil until
	// startIconHeartbeat has run (it is a no-op on macOS).
	iconHeartbeatDone chan struct{}
}

// macIcons returns the on/off icon pair for the macOS menu bar. When the
// app-icon variants were built (SetTrayIconPNG supplied the source PNG)
// those win — the red disc stays legible on any menu-bar appearance.
// Otherwise fall back to the monochrome W pair keyed off the observed
// appearance.
func (t *trayManager) macIcons() (on, off []byte) {
	if len(trayOnIconMac) > 0 && len(trayOffIconMac) > 0 {
		return trayOnIconMac, trayOffIconMac
	}
	if t.darkMenuBar.Load() {
		return trayOnIconDark, trayOffIconDark
	}
	return trayOnIconLight, trayOffIconLight
}

// startAppearanceWatch installs the KVO-based menu-bar appearance
// observer (menubar_appearance_darwin.go). The callback delivers the
// exact appearance the menu bar is rendering us with — initial value
// included — so the icon flips to the black-W set only when the menu
// bar is genuinely light, and stays white everywhere else.
func (t *trayManager) startAppearanceWatch() {
	if runtime.GOOS != "darwin" {
		return
	}
	watchMenuBarAppearance(func(dark bool) {
		// KVO delivers on the AppKit main thread; hop to a goroutine so
		// SetIcon's own main-thread dispatch can't deadlock against it.
		go func() {
			if t.quitting.Load() {
				return
			}
			if t.darkMenuBar.Swap(dark) == dark {
				return
			}
			slog.Debug("menubar appearance changed", "dark", dark)
			t.mu.Lock()
			anyConnected := len(t.activeTunnels) > 0
			t.mu.Unlock()
			on, off := t.macIcons()
			if anyConnected {
				t.tray.SetIcon(on)
			} else {
				t.tray.SetIcon(off)
			}
		}()
	})
}

// quitApp is the single, platform-independent app teardown: stop the tray
// rebuild machinery, run doShutdown (which disconnects tunnels and stops the
// helper), then terminate. It backs both the tray's Quit item and
// `wireguideplus ctl stop`, which reaches it via the helper's EventQuit
// broadcast — so both routes leave exactly the same state behind.
//
// Safe to call more than once: doShutdown is guarded by a sync.Once and the
// quit flags are idempotent.
func (t *trayManager) quitApp() {
	// Close any visible connect popup before tearing down the app.
	closeConnectPopup()
	// Latch the quit flag BEFORE Destroy so any in-flight debounce
	// timer that fires between here and the AfterFunc cancel will
	// see it and bail. Then stop the timer explicitly to prevent
	// the goroutine from running at all in the common case.
	t.quitting.Store(true)
	appQuitting.Store(true)
	t.mu.Lock()
	if t.rebuildTimer != nil {
		t.rebuildTimer.Stop()
		t.rebuildTimer = nil
	}
	if t.notifyTimer != nil {
		t.notifyTimer.Stop()
		t.notifyTimer = nil
	}
	if t.iconHeartbeatDone != nil {
		close(t.iconHeartbeatDone)
		t.iconHeartbeatDone = nil
	}
	t.mu.Unlock()
	t.doShutdown()
	t.tray.Destroy()
	t.app.Quit()
}

func newTrayManager(app *application.App, win *application.WebviewWindow, tray *application.SystemTray, svc *wgapp.TunnelService, doShutdown func()) *trayManager {
	t := &trayManager{
		app:        app,
		win:        win,
		tray:       tray,
		svc:        svc,
		doShutdown: doShutdown,
	}
	// White W until the appearance observer proves the menu bar is light.
	t.darkMenuBar.Store(true)
	return t
}

// initialBuild draws the menu once at startup. No OnClick handler is
// registered — with no custom click handler, Wails lets AppKit run its
// native status-item menu tracking (the pre-click event monitor attaches
// the cached menu before the mouse-down is processed). The previous
// design intercepted the click and called OpenMenu, which synthesizes a
// fake NSEvent mouseDown — an unsupported trick that needed two clicks
// on some systems and stopped opening the menu at all on macOS 26
// (Tahoe reworked NSStatusItem internals). Menu freshness is covered by
// the status-driven scheduleRebuild path: each tunnel is a checkbox whose
// checkmark and emoji are refreshed on every rebuild, and clicking the
// checkmark toggles the tunnel via ctx.IsChecked(), so a ≤1s-stale
// switch is the worst case.
func (t *trayManager) initialBuild() {
	t.rebuildMenu()
	t.startAppearanceWatch()
	t.startIconHeartbeat()
}

// setIconState swaps the tray ICON (not a text label) based on connection
// state, and updates the tooltip. Called from the status event stream, so
// it must stay O(1) — no IPC, no disk I/O.
//
//	disconnected → W glyph (white on dark menu bars, black on light)
//	connected    → same W with a green dot badge
//
// safeSetIcon updates the tray icon without letting a Win32 API failure
// kill the app or hang the status stream. Wails' SystemTray.SetIcon
// dispatches to the main thread via InvokeSync; a failing ShellNotifyIcon
// becomes a panic there that Wails' handler swallows but which leaves
// InvokeSync's WaitGroup un-Done (deadlocking the caller). We therefore
// run it fire-and-forget with a recover guard. The icon bytes themselves
// are validated on startup (see ztraytest), so this is purely defensive.
//
// iconMu serialises concurrent callers so the underlying Wails tray's
// non-atomic handle swap can't be raced by the status stream and the
// self-heal heartbeat at the same instant.
func (t *trayManager) safeSetIcon(icon []byte) {
	if len(icon) == 0 {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("tray SetIcon panicked", "err", r, "stack", string(debug.Stack()))
			}
		}()
		t.iconMu.Lock()
		defer t.iconMu.Unlock()
		t.tray.SetIcon(icon)
	}()
}

// upTunnelSet returns the established tunnels that may be *announced* as
// connected: setup finished AND the peer has completed at least one
// WireGuard handshake.
//
// Two independent facts, deliberately kept apart. StateConnected only says
// the interface was configured and brought up; it stays true forever for a
// tunnel whose peer never answers (wrong public key, dead endpoint, upstream
// down, NAT hole closed). The handshake is the first evidence that packets
// actually reached the peer, so it — not the state machine — decides what
// the UI is allowed to claim.
//
// upTunnelSet 返回可以被**宣告**为「已连接」的已建立隧道：setup 完成，且对端
// 至少完成过一次 WireGuard 握手。
//
// 这是两件独立的事，刻意分开。StateConnected 只说明接口已配置并拉起；对端永不
// 应答的隧道（公钥错、endpoint 死、上游断、NAT 洞关闭）会永远停在这个状态。
// 握手是「包真的到了对端」的第一个证据，所以由它——而不是状态机——来决定 UI
// 可以宣称什么。
func upTunnelSet(established, handshake map[string]bool) map[string]bool {
	up := make(map[string]bool, len(established))
	for name := range established {
		// No handshake entry means "no handshake seen yet", which is
		// exactly the case this filter exists to catch — not "unknown,
		// assume the best".
		if handshake[name] {
			up[name] = true
		}
	}
	return up
}

// setIconState receives BOTH sets on purpose:
//
//	activeNames      — connecting/connected/disconnecting (lifecycle truth,
//	                   used to decide *whether the menu needs a rebuild*)
//	establishedNames — tunnels whose setup actually finished
//
// Anything the user can read as "it's connected" (the green "on" icon, the
// tooltip listing names, the status bubble) keys off upTunnelSet(established,
// handshake). Using activeNames there is what made a failing connect look
// like "connected, then dropped": during the attempt the tunnel is neither up
// nor gone. Using establishedNames alone is the quieter half of the same lie:
// an interface that came up against a dead peer stays "connected" forever.
func (t *trayManager) setIconState(activeNames []string, establishedNames []string, handshakeMap map[string]bool, outOfRange []string) {
	newSet := make(map[string]bool, len(activeNames))
	for _, n := range activeNames {
		newSet[n] = true
	}
	newEstablished := make(map[string]bool, len(establishedNames))
	for _, n := range establishedNames {
		// An established tunnel is by definition part of the active set;
		// trust only the intersection so a helper that sends one field and
		// not the other can never advertise a phantom connection.
		if newSet[n] {
			newEstablished[n] = true
		}
	}
	// The claim set: established AND handshook at least once.
	newUp := upTunnelSet(newEstablished, handshakeMap)

	t.mu.Lock()
	prev := t.activeTunnels
	prevEstablished := t.establishedTunnels
	prevUp := t.upTunnels
	prevHandshake := t.hasHandshake
	prevAnyConnected := len(prev) > 0
	t.activeTunnels = newSet
	t.establishedTunnels = newEstablished
	t.upTunnels = newUp
	t.hasHandshake = handshakeMap
	t.outOfRange = outOfRange
	t.mu.Unlock()

	// Everything user-visible reads these, not the raw active list.
	connectedNames := make([]string, 0, len(newUp))
	for n := range newUp {
		connectedNames = append(connectedNames, n)
	}
	sort.Strings(connectedNames)
	anyConnected := len(newUp) > 0

	// Gate on BOTH sets: the icon must change when a tunnel finishes its
	// transition (connecting → connected) even though the active set
	// (which already contained it) did not change.
	activeChanged := prevAnyConnected != anyConnected || len(prev) != len(newSet) || len(prevEstablished) != len(newEstablished)
	if !activeChanged {
		for k := range prev {
			if !newSet[k] {
				activeChanged = true
				break
			}
		}
	}
	if !activeChanged {
		for k := range prevEstablished {
			if !newEstablished[k] {
				activeChanged = true
				break
			}
		}
	}
	// A tunnel finishing its first handshake is a real change to what we may
	// claim, even though neither the active nor the established set moved.
	if !activeChanged {
		for k := range newUp {
			if !prevUp[k] {
				activeChanged = true
				break
			}
		}
	}
	if !activeChanged {
		for k := range prevUp {
			if !newUp[k] {
				activeChanged = true
				break
			}
		}
	}

	if activeChanged {
		onIcon, offIcon := t.macIcons()
		if (runtime.GOOS == "windows" || runtime.GOOS == "linux") && len(trayOnIconWindows) > 0 {
			// Use the rounded-red app-icon variants on Windows and Linux so
			// the tray icon stands out against light system-tray backgrounds.
			onIcon, offIcon = trayOnIconWindows, trayOffIconWindows
		}
		if anyConnected {
			t.safeSetIcon(onIcon)
			tooltip := "WireGuide Plus — " + strings.Join(connectedNames, ", ")
			t.tray.SetTooltip(tooltip)
		} else {
			if runtime.GOOS == "darwin" || ((runtime.GOOS == "windows" || runtime.GOOS == "linux") && len(offIcon) > 0) {
				t.safeSetIcon(offIcon)
			}
			t.tray.SetTooltip("WireGuide Plus")
		}
		if runtime.GOOS != "darwin" {
			// Windows/Linux: SetLabel has no menu-bar meaning (that's a
			// macOS concept) and GDI renders the ● as '?', so keep it a
			// plain text tooltip-style label.
			t.tray.SetLabel("WireGuide Plus")
		}
	} else if anyConnected {
		// Established names may have reordered without count changing.
		// Refresh tooltip only when names differ from last broadcast.
		newTooltip := "WireGuide Plus — " + strings.Join(connectedNames, ", ")
		t.tray.SetTooltip(newTooltip)
	}

	if activeChanged {
		// Any connection-state change — auto-connect, Wi-Fi switch,
		// Ethernet unplug/replug, network loss, manual connect/disconnect —
		// schedules the status notification. The 10s debounce means the
		// bubble shows the settled state once churn has quieted down.
		t.scheduleStatusNotification()
	}

	// Rebuild menu if active set changed OR if handshake state changed for
	// any active tunnel (◐ → ● flip without a connect/disconnect event).
	// Reuse activeChanged from the SetIcon gate above; only check
	// handshake transitions if the active-set itself didn't change.
	changed := activeChanged
	if !changed {
		for name := range newSet {
			if handshakeMap[name] != prevHandshake[name] {
				changed = true
				break
			}
		}
	}
	if changed {
		t.scheduleRebuild()
	}
}

// iconForState returns the tray icon bytes matching the given connection
// state, honouring the same platform-specific variant selection as
// setIconState (macOS appearance-aware pair; Windows/Linux app-icon red-tile
// pair). Returns nil if no usable icon is available, in which case callers
// must not call SetIcon.
func (t *trayManager) iconForState(anyConnected bool) []byte {
	onIcon, offIcon := t.macIcons()
	if (runtime.GOOS == "windows" || runtime.GOOS == "linux") && len(trayOnIconWindows) > 0 {
		onIcon, offIcon = trayOnIconWindows, trayOffIconWindows
	}
	if anyConnected {
		return onIcon
	}
	return offIcon
}

// startIconHeartbeat re-applies the current tray icon on a fixed interval so
// any transient loss — Explorer.exe restart, a failed Shell_NotifyIcon
// (NIM_MODIFY) during a display/theme/DPI transition, or a concurrent HICON
// race inside the underlying Wails tray — is healed automatically instead of
// leaving a blank icon behind while the app keeps running. The process never
// dies: safeSetIcon's recover guard makes a failing beat a no-op that the
// next beat simply retries. macOS is excluded — its NSStatusItem has no
// equivalent transient-loss path, and the appearance observer owns its icon
// there.
func (t *trayManager) startIconHeartbeat() {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		return
	}
	if t.iconHeartbeatDone != nil {
		return // already running (idempotent)
	}
	t.iconHeartbeatDone = make(chan struct{})
	const period = 60 * time.Second
	ticker := time.NewTicker(period)
	go func() {
		for {
			select {
			case <-t.iconHeartbeatDone:
				ticker.Stop()
				return
			case <-ticker.C:
				if t.quitting.Load() {
					return
				}
				t.mu.Lock()
				anyConnected := len(t.upTunnels) > 0
				t.mu.Unlock()
				t.safeSetIcon(t.iconForState(anyConnected))
				slog.Debug("tray: icon heartbeat reapplied", "connected", anyConnected)
			}
		}
	}()
}

// statusNotificationDelay is how long a connection-state change is left to
// settle before the status notification appears. 10s gives the automation
// engine time to finish auto-connecting after launch or after a network
// change (Wi-Fi switch, Ethernet unplug/replug, network loss) before the
// bubble reports the settled situation.
const statusNotificationDelay = 10 * time.Second

// scheduleStatusNotification debounces the connection-status notification:
// every state change resets a 10s timer, so the bubble shows the *settled*
// state once churn has quieted down. Also used at startup (gui.Run after
// initialBuild) so a fresh launch reports its connection situation once the
// auto-connect window closes.
func (t *trayManager) scheduleStatusNotification() {
	if t.quitting.Load() {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.notifyTimer != nil {
		t.notifyTimer.Stop()
	}
	t.notifyTimer = time.AfterFunc(statusNotificationDelay, func() {
		if t.quitting.Load() {
			return
		}
		t.showStatusNotification()
	})
}

// showStatusNotification renders the tray bubble with the current connection
// situation (connected tunnel list, or "not connected"). Its on-screen
// duration comes from the notify_duration_ms setting (default 10s).
//
// The bubble is gated on a *per-tunnel transition*, not on the raw
// established set. Each tunnel is announced only when its own connection
// state actually changes (down→up, up→down, or a fresh connect attempt), and
// the bubble names exactly the tunnel(s) that changed. This is what stops a
// stably-connected tunnel from being re-announced "connected" on every 30s
// status poll: when the churner TS451D keeps dropping out of the set, the
// bubble must say "TS451D disconnected/connected" — naming the tunnel that
// flapped — instead of re-spamming "TS453Dmini connected" for a neighbour
// that never moved (see #111). Genuine connect/disconnect transitions are
// still reported; only the spurious re-announcement of an unchanged tunnel
// is suppressed — which is correctness, not popup suppression.
func (t *trayManager) showStatusNotification() {
	t.mu.Lock()
	prev := t.lastNotifiedConnected
	// The *claim* set, not the established set: a tunnel that finished setup
	// against a silent peer is announced as still connecting until its first
	// handshake lands — then the handshake itself is the transition that
	// earns the "connected" bubble. Announcing it from establishedNames is
	// what showed "🟢 Connected" for a tunnel that never handshook.
	tr, cur := computeStatusTransitions(prev, t.upTunnels, t.activeTunnels)
	outOfRange := append([]string(nil), t.outOfRange...)
	t.lastNotifiedConnected = cur
	t.mu.Unlock()

	ups, downs, trying := tr.ups, tr.downs, tr.trying

	// Nothing actually changed since the last bubble → do not re-pop a
	// stable tunnel. This is the root fix for the every-30s "X connected"
	// spam: a tunnel that stays up is announced once, not on every poll.
	if len(ups) == 0 && len(downs) == 0 && len(trying) == 0 {
		return
	}

	// Announce the transition, naming the tunnel(s) that changed. A "down"
	// (connection lost) always wins over an "up" so the user is told about
	// the failure first; a fresh "connecting" attempt is reported only when
	// there is no more urgent change.
	var names []string
	state := popupStateConnected
	switch {
	case len(downs) > 0:
		state = popupStateDisconnected
		names = downs
	case len(ups) > 0:
		state = popupStateConnected
		names = ups
	default:
		state = popupStateConnecting
		names = trying
		// The warning is about what the *established* tunnels route, so it
		// makes no sense while nothing is established yet.
		outOfRange = nil
	}
	sort.Strings(names)

	lang := "en"
	if s, err := t.svc.GetSettings(); err == nil && s != nil && s.Language != "" {
		lang = s.Language
	}
	// Persistent on purpose: a status bubble that vanished on a timer could
	// be missed entirely while the user works in another app — the notify
	// duration setting now only governs the informational launch overview.
	//
	// The action button mirrors the state: while the tunnel is up it reads
	// Disconnect (tear down every active tunnel); after the drop it reads
	// Connect — the opposite of what just happened — and brings back the
	// tunnels this bubble named. A manual connect latches manual-on (see
	// TunnelService.Connect), so automation stays paused until the user
	// disconnects by hand or restarts, exactly as on the detail page.
	showStatusPopup(names, state, outOfRange, lang, popupPersistent, showDock, func() {
		t.mu.Lock()
		names := make([]string, 0, len(t.activeTunnels))
		for n := range t.activeTunnels {
			names = append(names, n)
		}
		t.mu.Unlock()
		for _, n := range names {
			if err := t.svc.DisconnectTunnel(n); err != nil {
				slog.Warn("tray: popup disconnect failed", "tunnel", n, "err", err)
			}
		}
	}, func() {
		// Only the tunnels that are still down: one the automation engine
		// already re-connected (or is mid-retry on) needs nothing, and
		// connecting it would just latch manual-on and pause the rules
		// for a tunnel that is coming back on its own anyway.
		t.mu.Lock()
		targets := make([]string, 0, len(downs))
		for _, n := range downs {
			if !t.upTunnels[n] && !t.activeTunnels[n] {
				targets = append(targets, n)
			}
		}
		t.mu.Unlock()
		for _, n := range targets {
			if err := t.svc.Connect(n); err != nil {
				slog.Warn("tray: popup connect failed", "tunnel", n, "err", err)
			}
		}
	})
}

// launchOverviewDelay is how long the GUI waits after startup before the
// status-overview bubble appears. 15s sits past the permission-prompt window
// and past the automation engine's usual auto-connect pass, so the overview
// reports the settled picture rather than the churn of the first seconds.
const launchOverviewDelay = 15 * time.Second

// scheduleLaunchOverview arms the one-shot post-startup overview bubble.
// Called from gui.Run after the tray's initial build.
func (t *trayManager) scheduleLaunchOverview() {
	time.AfterFunc(launchOverviewDelay, func() {
		if t.quitting.Load() {
			return
		}
		t.showLaunchOverview()
	})
}

// showLaunchOverview renders the bubble listing EVERY tunnel with its
// automation mode and connection state. It is the "what is this app about
// to do to my machine" answer at launch: which tunnels are entrusted to
// automation and which are the user's alone, and what is up right now.
// Data is fully local — the tunnel list and policies come from the store
// (no helper round-trip, unlike the status event stream), and the
// connection state comes from the tray's own caches, which the status
// events have been refreshing since launch.
func (t *trayManager) showLaunchOverview() {
	infos, err := t.svc.ListTunnelsLocal()
	if err != nil {
		slog.Warn("tray: launch overview: tunnel list failed", "err", err)
		return
	}
	if len(infos) == 0 {
		return // nothing to report
	}

	t.mu.Lock()
	// "connected" here is the claim set (established + handshook): the
	// overview is a status report, and reporting "connected" for a tunnel
	// whose peer never answered is the same false claim as the bubble's.
	up := make(map[string]bool, len(t.upTunnels))
	for n := range t.upTunnels {
		up[n] = true
	}
	active := make(map[string]bool, len(t.activeTunnels))
	for n := range t.activeTunnels {
		active[n] = true
	}
	t.mu.Unlock()

	rows := make([]overviewRow, 0, len(infos))
	for _, info := range infos {
		state := "disconnected"
		switch {
		case up[info.Name]:
			state = "connected"
		case active[info.Name]:
			state = "connecting"
		}
		auto := true
		if p, err := t.svc.GetTunnelPolicies(info.Name); err == nil && p != nil {
			auto = !p.AutomationDisabled
		}
		rows = append(rows, overviewRow{Name: info.Name, State: state, Auto: auto})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })

	lang := "en"
	duration := 10 * time.Second
	if s, err := t.svc.GetSettings(); err == nil && s != nil {
		if s.Language != "" {
			lang = s.Language
		}
		if s.NotifyDurationMs > 0 {
			duration = time.Duration(s.NotifyDurationMs) * time.Millisecond
		}
	}
	showOverviewPopup(rows, lang, duration, showDock)
}

// statusTransition is the per-tunnel change between two status snapshots,
// as computed by computeStatusTransitions. Only the tunnels named here are
// eligible for a bubble — a tunnel absent from all three slices stayed in
// the same state and must not be re-announced.
type statusTransition struct {
	ups    []string // became connected (down/connecting → connected)
	downs  []string // lost a real connection (connected → connecting/gone)
	trying []string // fresh connect attempt (down → connecting)
}

// computeStatusTransitions compares the previously-announced connection
// state (prev) with the current one — tunnels in the claim set (established
// AND handshook; see upTunnelSet) are "connected", every other active
// tunnel is "connecting" — and returns only the
// tunnels whose state actually changed. A tunnel present in both snapshots
// with the same state contributes nothing; that is exactly what stops a
// stably-connected tunnel from being re-announced "connected" on every 30s
// status poll (see #111). It returns the current state map too, so callers
// can store it as the baseline for the next comparison.
func computeStatusTransitions(prev map[string]tunnelConnState, established, active map[string]bool) (statusTransition, map[string]tunnelConnState) {
	cur := make(map[string]tunnelConnState, len(established)+len(active))
	for n := range established {
		cur[n] = tcsConnected
	}
	for n := range active {
		if _, ok := cur[n]; !ok {
			cur[n] = tcsConnecting
		}
	}
	var tr statusTransition
	for n, s := range cur {
		ps, ok := prev[n]
		if !ok {
			// First time we see this tunnel: announce the new state.
			switch s {
			case tcsConnected:
				tr.ups = append(tr.ups, n)
			case tcsConnecting:
				tr.trying = append(tr.trying, n)
			}
			continue
		}
		if ps == s {
			continue // no change — stay quiet (this is the spam fix)
		}
		// State changed.
		switch s {
		case tcsConnected:
			tr.ups = append(tr.ups, n) // was connecting/down → now up
		case tcsConnecting:
			if ps == tcsConnected {
				tr.downs = append(tr.downs, n) // lost connection, now retrying
			}
			// connecting→connecting: nothing new to report
		case tcsUnknown:
			// Only a tunnel that was genuinely connected can be announced
			// as lost. connecting→gone is a failed attempt that never
			// reached a connection — saying "disconnected" about it is a
			// lie, and in the address-conflict loop (connect attempt →
			// DNS-timeout failure, repeatedly) it was exactly the pair of
			// phantom "disconnected" bubbles users reported. The failure
			// is already in the log; the bubble stays honest.
			if ps == tcsConnected {
				tr.downs = append(tr.downs, n)
			}
		}
	}
	// Tunnels that vanished entirely (were active, now not) count as a
	// loss — but again only from a genuinely connected state, for the same
	// reason as above.
	for n, ps := range prev {
		if _, ok := cur[n]; !ok && ps == tcsConnected {
			tr.downs = append(tr.downs, n)
		}
	}
	sort.Strings(tr.ups)
	sort.Strings(tr.downs)
	sort.Strings(tr.trying)
	return tr, cur
}

// scheduleRebuild debounces rebuildMenu calls — multiple triggers within 100ms
// are coalesced into a single rebuild.
func (t *trayManager) scheduleRebuild() {
	if t.quitting.Load() {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.rebuildTimer != nil {
		t.rebuildTimer.Stop()
	}
	t.rebuildTimer = time.AfterFunc(100*time.Millisecond, func() {
		if t.quitting.Load() {
			return
		}
		t.rebuildMenu()
	})
}

// trayMenuTitle is the disabled first menu row: the app name plus the
// running version, so users can tell which build they're on without
// opening Settings (and so bug reports can quote it). The version is
// injected at link time; a dev build reports "0.0.0-dev". The version is
// rendered with a leading "v" (e.g. "WireGuide Plus v3.1.0") to match the
// in-app version pills and update banners — every user-facing version string
// in the app uses the "v" + number format.
//
// trayMenuTitle 是禁用的首行菜单项：应用名加当前运行版本，方便用户在不打开
// 设置的情况下确认所用构建（也便于 bug 报告引用）。版本在链接期注入；开发构建
// 显示 "0.0.0-dev"。版本号统一带前缀 "v"（如 "WireGuide Plus v3.1.0"），与应用内
// 的版本徽标和更新横幅保持一致——应用内所有面向用户的版本号均采用 "v" + 数字 格式。
func trayMenuTitle() string {
	v := strings.TrimSpace(version.Version)
	if v == "" {
		return "WireGuide Plus"
	}
	return "WireGuide Plus v" + v
}

// rebuildMenu reconstructs the whole tray menu: tunnel list, Show Window,
// Quit. Uses ListTunnelsLocal (disk only, no IPC) + the cached activeTunnel
// for connected-state glyphs. Safe to invoke from any goroutine.
func (t *trayManager) rebuildMenu() {
	if t.quitting.Load() {
		return
	}
	t.rebuildMu.Lock()
	defer t.rebuildMu.Unlock()
	// Re-check quitting after acquiring the lock — Quit may have run
	// while a queued rebuild was waiting.
	if t.quitting.Load() {
		return
	}

	tunnels, err := t.svc.ListTunnelsLocal()
	if err != nil {
		slog.Debug("tray: list tunnels failed", "error", err)
	}

	// Snapshot both maps under a single lock so the active set we
	// render and the handshake bits we render are consistent — a
	// concurrent setIconState was previously able to swap activeTunnels
	// between the two reads, leading to "connected but no handshake
	// glyph" flickers.
	t.mu.Lock()
	activeSet := t.activeTunnels
	upSet := t.upTunnels
	t.mu.Unlock()

	// Refill the single persistent Menu in place (see the menu field
	// doc): Wails reuses its NSMenu on Update(), and AppKit live-updates
	// an NSMenu that is currently open — creating a fresh Menu here left
	// an open tray menu showing stale glyphs until dismissed.
	created := t.menu == nil
	if created {
		t.menu = t.app.NewMenu()
	} else {
		t.menu.Clear()
	}
	m := t.menu
	m.Add(trayMenuTitle()).SetEnabled(false)
	m.AddSeparator()

	for _, tun := range tunnels {
		tun := tun // loop-var capture
		// The checkbox is the ON/OFF switch, so it tracks the ACTIVE set:
		// an up-but-silent tunnel must still read as "on" or clicking it
		// would try to connect again instead of disconnecting. The glyph
		// is the status claim and tracks the UP set instead — 🟢 only once
		// the peer has actually handshook, 🟡 while the tunnel is dialling
		// or waiting for its first handshake, 🔴 when it is off
		// (✔ / … / ✗ monochrome on the GDI-drawn Windows menu).
		//
		// 复选框是开关，所以跟随 ACTIVE 集合：已建立但对端沉默的隧道仍须显示为
		// 「开」，否则点它会被当成重新连接而不是断开。图标才是状态宣称，跟随 UP
		// 集合——对端真的握过手才🟢，拨号中或等首次握手时🟡，关闭时🔴
		// （GDI 绘制的 Windows 菜单上是单色的 ✔ / … / ✗）。
		active := activeSet[tun.Name]
		label := disconnectedGlyph + " " + tun.Name
		switch {
		case upSet[tun.Name]:
			label = connectedGlyph + " " + tun.Name
		case active:
			label = connectingGlyph + " " + tun.Name
		}
		tunName := tun.Name
		// Wails flips the checkmark itself on click and hands the NEW state
		// to the callback via ctx.IsChecked(), so the action follows the
		// switch; a failed connect/disconnect is corrected on the next
		// status-driven rebuild.
		m.AddCheckbox(label, active).OnClick(func(ctx *application.Context) {
			if ctx.IsChecked() {
				if err := t.svc.Connect(tunName); err != nil {
					slog.Warn("tray connect failed", "tunnel", tunName, "error", err)
				}
			} else {
				if err := t.svc.DisconnectTunnel(tunName); err != nil {
					slog.Warn("tray disconnect failed", "tunnel", tunName, "error", err)
				}
			}
		})
	}
	m.AddSeparator()
	m.Add("Show Window").OnClick(func(ctx *application.Context) {
		showDock()
	})
	m.AddSeparator()
	m.Add("Quit").OnClick(func(ctx *application.Context) {
		t.quitApp()
	})
	if created || runtime.GOOS == "windows" {
		// Windows must go through SetMenu on EVERY rebuild: the tray popup
		// is a separate PopupMenu that only SetMenu reconstructs, while
		// Menu.Update() refreshes a window-menu impl the popup never reads —
		// so refilled items (connection glyphs) stayed invisible forever.
		t.tray.SetMenu(m) // runs m.Update() and caches the NSMenu
	} else {
		// macOS: in-place refresh of the same NSMenu — live even while
		// open, and repeated SetMenu there leaked the replaced NSMenu.
		m.Update()
	}
}
