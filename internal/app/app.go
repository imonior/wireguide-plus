// Package app provides Wails bindings bridging the Svelte frontend to the
// IPC helper client and local storage.
//
// The package is split across four files so that each has a single reason
// to change:
//   - app.go          (this file)    — TunnelService facade, constructor, shared types
//   - tunnel_ops.go                  — tunnel lifecycle: connect, disconnect, list, status, rename, delete
//   - file_ops.go                    — file/dialog operations: import, export, read, parse, edit
//   - settings_ops.go                — settings + firewall toggles
package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/imonior/wireguide-plus/internal/domain"
	"github.com/imonior/wireguide-plus/internal/hotkey"
	"github.com/imonior/wireguide-plus/internal/ipc"
	"github.com/imonior/wireguide-plus/internal/storage"
	"github.com/imonior/wireguide-plus/internal/update"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// TunnelService is the Wails-bound service.
// Storage (tunnel files, settings) stays in the GUI process.
// Tunnel operations go through the helper via an ipc.ClientHolder (so the
// helper can be re-spawned and the connection swapped without rebuilding
// the whole service graph).
type TunnelService struct {
	tunnelStore   *storage.TunnelStore
	settingsStore *storage.SettingsStore
	historyStore  *storage.HistoryStore
	clients       *ipc.ClientHolder
	app           *application.App

	// updateScheduler + updateStore are wired in by the GUI Run() entry
	// after the Wails service registers (we can't inject them at
	// construction time because the scheduler needs the app reference to
	// emit events). Both may stay nil in non-GUI test contexts; the
	// CheckForUpdate / DismissUpdate methods fall back to one-shot
	// behaviour in that case.
	updateScheduler *update.Scheduler
	updateStore     *update.StateStore

	// activeSessions maps tunnel name → open history session ID. Used by
	// ReconcileHistoryFromStatus to identify which row to close when a
	// tunnel disappears from the active set; sync.Map keeps the per-event
	// lookup lock-free against concurrent Connect / Disconnect.
	activeSessions sync.Map

	// pendingSessions holds tunnels that are active but have not yet
	// completed a WireGuard handshake. ActiveTunnels also reports
	// StateConnecting, so without this gate a failed attempt (DNS failure,
	// adapter error, rejected handshake) would leave a seconds-long
	// "connected then disconnected" row in the connection history. Entries
	// here are promoted to activeSessions on handshake, or dropped silently
	// if the attempt disappears first.
	pendingSessions sync.Map

	// lastKnownStats caches the most recent rx/tx + reason hint (see
	// lastKnownTunnelStats) for each currently-active tunnel. The reconcile
	// path looks this up when a tunnel disappears so the closed history
	// session reflects real bytes transferred — without it, helper-driven
	// disconnects (Wi-Fi rule, auto-reconnect cycle, etc.) would record
	// sessions with rx=0/tx=0 because the disappearance status event itself
	// doesn't carry the gone tunnel's counters.
	lastKnownStats sync.Map

	// reconcileMu guards lastReconcileSig — used by ReconcileHistoryFromStatus
	// to fast-skip the active-set diff when nothing changed since the prior
	// status event. The status stream is 1 Hz; without the skip every event
	// would walk the activeSessions sync.Map even in the steady state.
	reconcileMu      sync.Mutex
	lastReconcileSig string

	// mainWin is the primary WebviewWindow, injected after creation so the
	// hotkey handler can toggle it. May stay nil in non-GUI test contexts.
	mainWin *application.WebviewWindow

	// hotkeyToggle is the window show/hide/focus closure injected by the GUI
	// (it owns the platform-specific dock-window glue). The hotkey manager
	// fires it when the global shortcut is pressed.
	hotkeyToggle func()

	// hotkeyMgr owns the global toggle-window hotkey (nil until initHotkey).
	hotkeyMgr *hotkey.Manager
}

// lastKnownTunnelStats is the value type for TunnelService.lastKnownStats.
//
// reason is a hint set by user-initiated Disconnect / DisconnectTunnel
// before the IPC tear-down call. Reconcile reads it on close so the
// resulting history row is labelled "user" instead of the default
// "reconnect" — without this, every user disconnect would look
// indistinguishable from a helper-driven one in the timeline.
type lastKnownTunnelStats struct {
	rx     int64
	tx     int64
	reason string
}

// NewTunnelService creates a service. Set the app reference via SetApp()
// after application.New() for dialog support.
func NewTunnelService(ts *storage.TunnelStore, ss *storage.SettingsStore, hs *storage.HistoryStore, clients *ipc.ClientHolder) *TunnelService {
	return &TunnelService{
		tunnelStore:   ts,
		settingsStore: ss,
		historyStore:  hs,
		clients:       clients,
	}
}

// SetApp injects the Wails app for dialog access.
func (s *TunnelService) SetApp(app *application.App) {
	s.app = app
}

// SetUpdateScheduler injects the periodic update-check scheduler and its
// persistent state store. Called once from gui.Run() after the Wails app
// is constructed. The frontend's "Check now" / dismiss / last-checked
// queries route through these.
func (s *TunnelService) SetUpdateScheduler(sched *update.Scheduler, store *update.StateStore) {
	s.updateScheduler = sched
	s.updateStore = store
}

// SetMainWindow injects the primary window after it is created in gui.Run, so
// the hotkey toggle closure can show/hide/focus it.
// SetMainWindow 在 gui.Run 中窗口创建后注入主窗口，供热键切换闭包使用。
func (s *TunnelService) SetMainWindow(win *application.WebviewWindow) {
	s.mainWin = win
}

// SetHotkeyToggle injects the window toggle closure (provided by the GUI
// package, which owns the platform-specific dock-window glue). Called once
// before initHotkey.
// SetHotkeyToggle 注入窗口切换闭包（由 GUI 包提供，因其持有平台相关的
// dock 窗口胶水代码）。在 initHotkey 之前调用一次。
func (s *TunnelService) SetHotkeyToggle(fn func()) {
	s.hotkeyToggle = fn
}

// InitHotkey builds the hotkey manager and registers it from the persisted
// settings. Called once during GUI startup; safe to skip (hotkeyMgr stays nil)
// in non-GUI contexts.
// InitHotkey 构建热键管理器并按已保存的设置注册。在 GUI 启动期调用一次；
// 非 GUI 上下文可跳过（hotkeyMgr 保持 nil）。
func (s *TunnelService) InitHotkey() {
	if s.hotkeyToggle == nil {
		return
	}
	s.hotkeyMgr = hotkey.NewManager(s.onHotkeyFired)
	settings, err := s.settingsStore.Load()
	if err == nil && settings != nil {
		if err := applyHotkeyConfig(s.hotkeyMgr, settings.Hotkey); err != nil {
			slog.Warn("hotkey: initial registration failed", "error", err)
		}
	}
}

// onHotkeyFired is the callback the hotkey manager invokes on a press. It
// marshals to the UI thread (the closure does that itself) and toggles the
// window.
// onHotkeyFired 是热键管理器在按键时调用的回调。它（由闭包自身）编组到 UI
// 线程并切换窗口。
func (s *TunnelService) onHotkeyFired() {
	if s.hotkeyToggle != nil {
		s.hotkeyToggle()
	}
}

// reregisterHotkey applies a new hotkey config at runtime (e.g. on save), so
// a changed combo or enable/disable takes effect without restarting the app.
// reregisterHotkey 在运行时套用新的热键配置（如保存时），使修改后的组合键或
// 开关立即生效，无需重启应用。
func (s *TunnelService) reregisterHotkey(cfg storage.HotkeyConfig) {
	if s.hotkeyMgr == nil {
		return
	}
	if err := applyHotkeyConfig(s.hotkeyMgr, cfg); err != nil {
		slog.Warn("hotkey: re-registration failed", "error", err)
	}
}

// CloseHotkey tears the hotkey manager down (used on app shutdown).
// CloseHotkey 拆除热键管理器（用于应用退出）。
func (s *TunnelService) CloseHotkey() {
	if s.hotkeyMgr != nil {
		_ = s.hotkeyMgr.Close()
	}
}

// HotkeySupported reports whether the platform can register a global hotkey,
// so the frontend can show/hide the setting.
// HotkeySupported 报告当前平台是否支持注册全局热键，供前端显隐该设置项。
func (s *TunnelService) HotkeySupported() bool {
	return hotkey.Supported()
}

// applyHotkeyConfig registers (or clears) the hotkey from a HotkeyConfig.
// applyHotkeyConfig 按 HotkeyConfig 注册（或清除）热键。
func applyHotkeyConfig(mgr *hotkey.Manager, cfg storage.HotkeyConfig) error {
	if !cfg.Enabled || cfg.Combo == "" {
		return mgr.Register(hotkey.Combo{})
	}
	c, err := hotkey.ParseCombo(cfg.Combo)
	if err != nil {
		return err
	}
	return mgr.Register(c)
}

// errHelperUnavailable is the error returned when the IPC client has been
// torn down (e.g. during app shutdown). Using a sentinel keeps every RPC
// wrapper method uniform.
var errHelperUnavailable = fmt.Errorf("helper connection not available")

// call performs an RPC against the current helper client. Fetches the client
// fresh each call so a helper restart (which swaps the holder's client)
// takes effect immediately. Returns `errHelperUnavailable` if the holder has
// been closed — this prevents nil-pointer panics in the narrow window
// between doShutdown() and Wails app termination.
func (s *TunnelService) call(method string, params interface{}, result interface{}) error {
	c := s.clients.Get()
	if c == nil {
		return errHelperUnavailable
	}
	return c.Call(method, params, result)
}

// LogFrontend records a diagnostic breadcrumb sent from the webview. The
// edit-save chain was able to stall between its JS and Go halves with no
// trace at all (a 06:06 repro showed the disconnect landing but the
// UpdateConfig RPC never arriving, and nothing on either side). This gives
// every frontend step a line in the same log file the backend writes, so
// "the button did nothing" can always be reconstructed afterwards. It is
// deliberately fire-and-forget on the JS side and must never fail loudly.
func (s *TunnelService) LogFrontend(message string) {
	slog.Info("frontend", "category", "app", "detail", message)
}

// callLong performs an RPC with a generous timeout for operations that may
// take many seconds (Connect, Disconnect). The default 10-second timeout
// is too short for Connect, which involves DNS resolution + route setup +
// networksetup DNS configuration across all services (can take 15+ seconds
// on a Mac with many network services). If the client times out before the
// server finishes, the tunnel gets connected server-side but the GUI sees
// a false error, and the health monitor may trigger unnecessary recovery.
func (s *TunnelService) callLong(method string, params interface{}, result interface{}) error {
	c := s.clients.Get()
	if c == nil {
		return errHelperUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return c.CallWithContext(ctx, method, params, result)
}

// TunnelInfo is the summary shown in the tunnel list.
type TunnelInfo struct {
	Name        string `json:"name"`
	IsConnected bool   `json:"is_connected"`
	Endpoint    string `json:"endpoint"`
	Notes       string `json:"notes,omitempty"`
	// LatencyProbeTargets is the four-slot probe-target list (see
	// storage.TunnelMeta.ProbeTargets). Always four entries so the editor can
	// render its rows positionally; an empty entry means "not configured".
	LatencyProbeTargets []string `json:"latency_probe_targets,omitempty"`
	// Protocol is "" or "wireguard" for standard WireGuard tunnels and
	// "amneziawg" for AmneziaWG tunnels. The frontend shows an AWG badge
	// when it is set, so users can tell at a glance why an AWG tunnel's
	// status path behaves differently (e.g. no `wg` CLI access).
	Protocol string `json:"protocol,omitempty"`
	// CreatedAtUnix is the .conf file's mtime; LastUsedUnix is the most
	// recent connection start from history (0 if never connected). Both
	// feed the tunnel-list "date added" / "last used" sort (issue #17).
	CreatedAtUnix int64 `json:"created_at_unix,omitempty"`
	LastUsedUnix  int64 `json:"last_used_unix,omitempty"`
}

// ConnectionStatus is re-exported from the domain package so Wails bindings
// expose the same type that the helper broadcasts — preventing field drift
// between wire format and frontend expectations.
type ConnectionStatus = domain.ConnectionStatus
