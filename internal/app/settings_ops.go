package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/imonior/wireguide-plus/internal/autostart"
	"github.com/imonior/wireguide-plus/internal/ipc"
	"github.com/imonior/wireguide-plus/internal/logging"
	"github.com/imonior/wireguide-plus/internal/storage"
	"github.com/imonior/wireguide-plus/internal/update"
	"github.com/imonior/wireguide-plus/internal/wifi"
)

// validNotifyDurations is the closed set of notification-duration choices
// offered by the Settings <select>. A value outside this set (hand-edited
// config.json, a value written by a future version) leaves the select
// blank in the UI, so SaveSettings normalizes back to the default.
var validNotifyDurations = []int{5000, 10000, 15000, 30000, 60000}

func validNotifyDuration(ms int) bool {
	for _, v := range validNotifyDurations {
		if ms == v {
			return true
		}
	}
	return false
}

// redactURL hides any userinfo credentials in a proxy URL for audit logs
// ("http://user:secret@host:7890" → "http://***@host:7890").
func redactURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	u.User = url.User("***")
	return u.String()
}

// KnownSSIDs is the response shape for GetKnownSSIDs. The frontend uses
// it to render a picker so users can tap saved networks instead of
// retyping SSIDs they've already joined.
type KnownSSIDs struct {
	Current string   `json:"current"` // currently-connected SSID (empty if not on Wi-Fi)
	Known   []string `json:"known"`   // saved/preferred networks reported by the OS
}

// GetKnownSSIDs returns the currently-connected SSID (if any) plus the
// system's saved wireless networks. Both are best-effort — empty values
// are normal on a Mac that's only ever been on Ethernet.
func (s *TunnelService) GetKnownSSIDs() KnownSSIDs {
	return KnownSSIDs{
		Current: wifi.CurrentSSID(),
		Known:   wifi.KnownSSIDs(),
	}
}

// GetCurrentSubnets returns the network CIDRs of the physical interfaces
// the machine is currently on (Wi-Fi or Ethernet). The Automation editor
// offers these as suggestions for subnet conditions so the user can
// target the network they're on without knowing its CIDR.
func (s *TunnelService) GetCurrentSubnets() []string {
	return wifi.PhysicalSubnets()
}

// CurrentNetwork is the fingerprint of the network the machine is on now,
// for the Automation editor's "use current network" button.
type CurrentNetwork struct {
	GatewayMAC string `json:"gateway_mac"` // "" when unavailable (e.g. Windows, no gateway)
	Label      string `json:"label"`       // human hint, e.g. "192.168.0.0/24"
}

// GetCurrentNetwork returns the current default-gateway MAC fingerprint
// plus a readable label (the current subnet) so the editor can capture
// "this network" precisely without the user typing a MAC. GatewayMAC is
// empty when it can't be determined.
func (s *TunnelService) GetCurrentNetwork() CurrentNetwork {
	label := ""
	if subs := wifi.PhysicalSubnets(); len(subs) > 0 {
		label = subs[0]
	}
	return CurrentNetwork{
		GatewayMAC: wifi.GatewayMAC(),
		Label:      label,
	}
}

// AutomationPreviewResponse is the GUI-computed read-only evaluation of
// every tunnel's Automation rules against the current network context. It
// carries per-rule and per-condition match detail so the editor can render
// live "this condition matches now" indicators without re-implementing the
// engine's matching in JS. Computed locally (no helper round-trip) and
// identical in spirit to the helper's ipc.AutomationPreviewResponse.
type AutomationPreviewResponse struct {
	OnWiFi      bool                       `json:"on_wifi"`
	SSID        string                     `json:"ssid"`
	PhysicalIPs []string                   `json:"physical_ips"`
	GatewayMAC  string                     `json:"gateway_mac"`
	GatewayIP   string                     `json:"gateway_ip"`
	Interfaces  []wifi.InterfaceInfo       `json:"interfaces"`
	Tunnels     []AutomationTunnelPreview  `json:"tunnels"`
}

// AutomationTunnelPreview is one tunnel's evaluated rules plus the overall
// decision Evaluate would reach (accounting for the manual-off latch).
type AutomationTunnelPreview struct {
	Name     string             `json:"name"`
	Rules    []wifi.RuleDetail  `json:"rules"`
	Decision string             `json:"decision"` // "connect" | "disconnect" | "unmanaged" | "manual-off"
}

// AutomationPreview evaluates every tunnel's Automation rules against the
// CURRENT network context, read-only. The GUI process runs this directly so
// the editor's live indicators stay in lockstep with what the helper's
// engine would do (same rules, same wifi package evaluator) without an IPC
// round-trip. ManualOffTunnels is honoured the same way the helper honours
// it: a matching connect on a manually-off tunnel reports "manual-off".
func (s *TunnelService) AutomationPreview() AutomationPreviewResponse {
	ssid := wifi.CurrentSSID()
	gw := wifi.GatewayMAC()
	gwIP := wifi.GatewayIP()
	phys := wifi.PhysicalInterfaceIPs()
	ifaces := wifi.PhysicalInterfaces()
	ipStrs := make([]string, 0, len(phys))
	for _, ip := range phys {
		ipStrs = append(ipStrs, ip.String())
	}
	ctx := wifi.NetworkContext{
		SSID:        ssid,
		PhysicalIPs: phys,
		GatewayMAC:  gw,
		GatewayIP:   gwIP,
		Interfaces:  ifaces,
	}

	resp := AutomationPreviewResponse{
		OnWiFi:      ssid != "",
		SSID:        ssid,
		PhysicalIPs: ipStrs,
		GatewayMAC:  gw,
		GatewayIP:   gwIP,
		Interfaces:  wifi.AllPhysicalInterfaces(),
	}

	st, err := s.settingsStore.Load()
	if err != nil {
		return resp
	}
	st.EnsureAutomation()
	if st.Automation == nil || len(st.Automation.PerTunnel) == 0 {
		return resp
	}
	manualOff := make(map[string]bool, len(st.ManualOffTunnels))
	for _, n := range st.ManualOffTunnels {
		manualOff[n] = true
	}

	for _, name := range st.Automation.TunnelNames() {
		rules := st.Automation.PerTunnel[name]
		state, details := wifi.EvaluateDetail(rules, ctx)
		decision := "unmanaged"
		switch state {
		case wifi.StateConnect:
			if manualOff[name] {
				decision = "manual-off" // suppressed by the manual-off latch
			} else {
				decision = "connect"
			}
		case wifi.StateDisconnect:
			decision = "disconnect"
		}
		resp.Tunnels = append(resp.Tunnels, AutomationTunnelPreview{
			Name:     name,
			Rules:    details,
			Decision: decision,
		})
	}
	return resp
}

// CheckSSIDPermission reports whether the process can read the current SSID.
// Used by the frontend to prompt the user for Location Services access before
// Wi-Fi auto-connect rules can fire.
func (s *TunnelService) CheckSSIDPermission() wifi.SSIDPermissionStatus {
	return wifi.CheckSSIDPermission()
}

// OpenLocationSettings opens System Settings to the Location Services page so
// the user can grant SSID access without navigating there manually.
func (s *TunnelService) OpenLocationSettings() {
	if runtime.GOOS == "darwin" {
		exec.Command("open", "x-apple.systempreferences:com.apple.preference.security?Privacy_LocationServices").Start() //nolint:errcheck
	}
}

// guiLogLevelSetter is set by internal/gui at startup so the app package
// (which is Wails-bound) can update the GUI process's own log level at
// runtime without importing internal/gui (which would create an import
// cycle). SetLogLevel below calls this in addition to forwarding the
// level to the helper. Uses atomic.Value for safe concurrent access.
var guiLogLevelSetter atomic.Value // stores func(string)

// SetGUILogLevelSetter is called once from internal/gui.Run to register
// the GUI-side log level mutator. Safe to call before NewTunnelService.
func SetGUILogLevelSetter(f func(string)) {
	guiLogLevelSetter.Store(f)
}

func getGUILogLevelSetter() func(string) {
	if v := guiLogLevelSetter.Load(); v != nil {
		return v.(func(string))
	}
	return nil
}

// --- Settings (all local, no IPC) ---

func (s *TunnelService) GetSettings() (*storage.Settings, error) {
	settings, err := s.settingsStore.Load()
	if err != nil {
		return nil, err
	}
	// Always hand the frontend a populated Automation model so the rule
	// editor can read/edit it directly — EnsureAutomation lazily migrates
	// the legacy WifiRules the first time (in memory; persisted only when
	// the user saves). Without this the UI would see a null automation for
	// legacy users and couldn't show their migrated rules.
	if settings != nil {
		settings.EnsureAutomation()
	}
	return settings, nil
}

// SaveSettings persists the settings file AND applies any side effects:
// currently, pushing the new log level to both the GUI's slog handler and
// the helper's slog handler. Without those side effects a user lowering the
// level to Debug wouldn't see any new records — the saved file would match
// the UI but the running process would still be at Info.
func (s *TunnelService) SaveSettings(settings *storage.Settings) error {
	// Read the previous state first so we only (un)install the autostart
	// entry when the user actually toggles it. This avoids rewriting the
	// LaunchAgent plist / desktop file on every unrelated setting change.
	prev, _ := s.settingsStore.Load()

	// Sanitize: notification duration must be one of the UI's offered
	// values, otherwise the Settings <select> renders blank (the frontend
	// cannot display a value with no matching <option>).
	if !validNotifyDuration(settings.NotifyDurationMs) {
		settings.NotifyDurationMs = 10000
	}
	// Clamp log retention to a sane range (0 = default 7).
	if settings.LogRetentionDays < 0 {
		settings.LogRetentionDays = 0
	}
	if settings.LogRetentionDays > 90 {
		settings.LogRetentionDays = 90
	}

	// Preserve the manual-off latch: the frontend's settings object never
	// edits that list, and saving a stale in-memory copy must not silently
	// drop tunnels the user switched off by hand (manual off wins over
	// automation until they reconnect or the app restarts).
	if prev != nil && len(prev.ManualOffTunnels) > 0 {
		settings.ManualOffTunnels = append([]string(nil), prev.ManualOffTunnels...)
	}

	if err := s.settingsStore.Save(settings); err != nil {
		return err
	}

	// Audit log: record what changed so the log file answers "who turned
	// on the proxy / kill switch / …" without diffing config.json by hand.
	// The proxy URL is redacted so credentials never hit the log.
	changed := changedSettingsFields(prev, settings)
	if len(changed) > 0 {
		slog.Info("settings changed",
			"category", "settings",
			"changed", strings.Join(changed, ","),
			"proxy_mode", settings.ProxyMode,
			"proxy_url", redactURL(settings.ProxyURL),
			"auto_start", settings.AutoStart,
			"start_minimized", settings.StartMinimized,
			"log_level", settings.LogLevel,
			"log_retention_days", settings.LogRetentionDays,
			"auto_update_check", settings.AutoUpdateCheckEnabled(),
			"notify_duration_ms", settings.NotifyDurationMs,
			"enable_wg_scripts", settings.EnableWgScripts,
			"kill_switch", settings.KillSwitch,
			"dns_protection", settings.DNSProtection,
			"health_check", settings.HealthCheck,
			"pin_interface", settings.PinInterface,
			"enable_awg", settings.EnableAWG,
			)
	}

	// Retention changed → sweep old log files immediately instead of
	// waiting for the next startup.
	if prev != nil && prev.LogRetentionDays != settings.LogRetentionDays {
		s.cleanupLogs(settings.LogRetentionDays)
	}

	// Proxy setting applies to update checks immediately — the next
	// scheduled check (and any manual "Check now") uses the new mode/URL
	// without a restart.
	update.SetProxy(settings.ProxyMode, settings.ProxyURL)

	if prev == nil || prev.AutoStart != settings.AutoStart {
		if settings.AutoStart {
			exe, err := os.Executable()
			if err != nil {
				return fmt.Errorf("autostart: cannot resolve exe path: %w", err)
			}
			if err := autostart.InstallAutostart(exe); err != nil {
				return fmt.Errorf("autostart: install failed: %w", err)
			}
		} else {
			if err := autostart.RemoveAutostart(); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("autostart: remove failed: %w", err)
			}
		}
	}
	if settings.LogLevel != "" {
		if fn := getGUILogLevelSetter(); fn != nil {
			fn(settings.LogLevel)
		}
		// Best-effort: the helper may be unreachable during shutdown, and
		// the level change is not critical to Save succeeding.
		_ = s.call(ipc.MethodSetLogLevel, ipc.SetLogLevelRequest{Level: settings.LogLevel}, nil)
	}

	// Auto-update toggle OFF→ON: nudge the scheduler so the user doesn't
	// have to wait up to 24 h for the next regular tick. The force flag
	// on Kick bypasses the focusRecheckThreshold gate (we *want* to
	// re-check immediately after the user opts back in, regardless of
	// when the last check ran). OFF→OFF, ON→ON, and ON→OFF all skip.
	prevEnabled := prev == nil || prev.AutoUpdateCheckEnabled()
	if !prevEnabled && settings.AutoUpdateCheckEnabled() && s.updateScheduler != nil {
		s.updateScheduler.Kick(true)
	}

	return nil
}

// changedSettingsFields returns the names of the settings fields that
// differ between prev and next (nil prev = first save → all fields).
// Used for the audit log so a settings save logs what actually changed
// instead of dumping every value every time.
func changedSettingsFields(prev, next *storage.Settings) []string {
	if prev == nil {
		return []string{"all"}
	}
	var out []string
	if prev.Language != next.Language {
		out = append(out, "language")
	}
	if prev.Theme != next.Theme {
		out = append(out, "theme")
	}
	if prev.AutoStart != next.AutoStart {
		out = append(out, "auto_start")
	}
	if prev.StartMinimized != next.StartMinimized {
		out = append(out, "start_minimized")
	}
	if prev.NotifyDurationMs != next.NotifyDurationMs {
		out = append(out, "notify_duration_ms")
	}
	if prev.KillSwitch != next.KillSwitch {
		out = append(out, "kill_switch")
	}
	if prev.DNSProtection != next.DNSProtection {
		out = append(out, "dns_protection")
	}
	if prev.HealthCheck != next.HealthCheck {
		out = append(out, "health_check")
	}
	if prev.PinInterface != next.PinInterface {
		out = append(out, "pin_interface")
	}
	if prev.LogLevel != next.LogLevel {
		out = append(out, "log_level")
	}
	if prev.CompactList != next.CompactList {
		out = append(out, "compact_list")
	}
	if prev.ListSort != next.ListSort {
		out = append(out, "list_sort")
	}
	if prev.ListActiveOnTop != next.ListActiveOnTop {
		out = append(out, "list_active_on_top")
	}
	if prev.ListPaneWidth != next.ListPaneWidth {
		out = append(out, "list_pane_width")
	}
	if prev.AutoUpdateCheckEnabled() != next.AutoUpdateCheckEnabled() {
		out = append(out, "auto_update_check")
	}
	if prev.ProxyMode != next.ProxyMode {
		out = append(out, "proxy_mode")
	}
	if prev.ProxyURL != next.ProxyURL {
		out = append(out, "proxy_url")
	}
	if prev.LogRetentionDays != next.LogRetentionDays {
		out = append(out, "log_retention_days")
	}
	if prev.EnableWgScripts != next.EnableWgScripts {
		out = append(out, "enable_wg_scripts")
	}
	if prev.EnableAWG != next.EnableAWG {
		out = append(out, "enable_awg")
	}
	return out
}

// cleanupLogs sweeps daily log files older than retentionDays for both the
// GUI and helper log families. Best-effort: cleanup failure is not worth
// failing a settings save over.
func (s *TunnelService) cleanupLogs(retentionDays int) {
	paths, err := storage.GetPaths()
	if err != nil {
		return
	}
	for _, prefix := range []string{"wireguideplus", "helper"} {
		n, err := logging.CleanupOldLogs(paths.LogsDir, prefix, retentionDays)
		if err != nil {
			slog.Warn("log cleanup failed", "prefix", prefix, "error", err)
			continue
		}
		if n > 0 {
			slog.Info("removed old log files", "prefix", prefix, "removed", n, "retention_days", retentionDays)
		}
	}
}

// TestProxyResult is the outcome of a proxy connectivity test.
type TestProxyResult struct {
	OK        bool   `json:"ok"`
	LatencyMs int    `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

// TestProxy performs a round-trip request to the GitHub Releases API using
// the given proxy configuration and reports success plus latency. The
// Settings → Proxy "test connection" button calls this so the user can
// verify a proxy works before saving it.
//
// mode is one of "direct", "mirror" or "manual":
//   - "direct": rawURL ignored; a plain Transport (explicitly ignoring any
//     environment HTTP_PROXY/HTTPS_PROXY, which is what "direct" means in
//     the UI).
//   - "mirror": rawURL is a GitHub accelerator mirror prefix (e.g.
//     "https://ghfast.top"); the API endpoint is fetched directly through
//     "<mirror>/<official endpoint>".
//   - "manual": rawURL is an http/https/socks5 proxy URL used for a
//     CONNECT-style request to the official API endpoint.
func (s *TunnelService) TestProxy(mode, rawURL string) TestProxyResult {
	start := time.Now()
	client := &http.Client{Timeout: 8 * time.Second}
	target := update.APIEndpoint()
	switch mode {
	case "mirror":
		if !update.ValidMirrorPrefix(rawURL) {
			return TestProxyResult{OK: false, Error: "invalid mirror prefix (need e.g. https://ghfast.top)"}
		}
		client.Transport = &http.Transport{}
		target = update.MirroredEndpoint(rawURL)
	case "manual":
		u, err := url.Parse(rawURL)
		if err != nil || !update.ValidProxyURL(u) {
			return TestProxyResult{OK: false, Error: "invalid proxy URL (need e.g. http://127.0.0.1:7890)"}
		}
		client.Transport = &http.Transport{Proxy: http.ProxyURL(u)}
	default: // "direct"
		client.Transport = &http.Transport{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return TestProxyResult{OK: false, Error: err.Error()}
	}
	resp, err := client.Do(req)
	if err != nil {
		return TestProxyResult{OK: false, Error: err.Error()}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20)) // drain
	if resp.StatusCode >= 400 {
		return TestProxyResult{OK: false, Error: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	return TestProxyResult{OK: true, LatencyMs: int(time.Since(start).Milliseconds())}
}

// SaveAutomationRules atomically replaces one tunnel's Automation rules.
// It goes through SettingsStore.Update — the cross-process locked
// read-modify-write — instead of a whole-object SaveSettings, so a
// concurrent `wireguideplus ctl` edit to any other tunnel or field can never
// be clobbered by a stale GUI snapshot (issue #27 review follow-up).
// An empty rules slice removes the tunnel's entry entirely. The helper
// re-reads settings from disk on every evaluation, so no push is needed.
func (s *TunnelService) SaveAutomationRules(tunnel string, rules []wifi.Rule) error {
	if tunnel == "" {
		return fmt.Errorf("automation: empty tunnel name")
	}
	// Reject malformed rules up front — the helper's evaluator silently
	// no-ops on rules it can't interpret, so a bad save would otherwise
	// look accepted while doing nothing.
	for i, r := range rules {
		if err := wifi.ValidateRule(r); err != nil {
			return fmt.Errorf("automation: rule %d: %w", i+1, err)
		}
	}
	return s.settingsStore.Update(func(st *storage.Settings) error {
		st.EnsureAutomation()
		if len(rules) == 0 {
			delete(st.Automation.PerTunnel, tunnel)
			return nil
		}
		if st.Automation.PerTunnel == nil {
			st.Automation.PerTunnel = map[string][]wifi.Rule{}
		}
		st.Automation.PerTunnel[tunnel] = rules
		return nil
	})
}

// SetLogLevel updates both the GUI's and the helper's slog level
// immediately. Exposed as a Wails method so the Settings view can call
// it without waiting for a full SaveSettings round trip.
func (s *TunnelService) SetLogLevel(level string) error {
	if fn := getGUILogLevelSetter(); fn != nil {
		fn(level)
	}
	return s.call(ipc.MethodSetLogLevel, ipc.SetLogLevelRequest{Level: level}, nil)
}

// --- Firewall toggles (go through helper) ---

// SetKillSwitch asks the helper to enable or disable the firewall kill switch.
func (s *TunnelService) SetKillSwitch(enabled bool) error {
	return s.call(ipc.MethodSetKillSwitch, ipc.KillSwitchRequest{Enabled: enabled}, nil)
}

// SetDNSProtection asks the helper to lock DNS to the active tunnel's servers.
// When enabling, we look up the active tunnel's DNS list from local storage
// and pass it along (the helper never touches user-space storage).
func (s *TunnelService) SetDNSProtection(enabled bool) error {
	var dnsServers []string
	if enabled {
		var active ipc.StringResponse
		if err := s.call(ipc.MethodActiveName, nil, &active); err != nil {
			return fmt.Errorf("cannot verify tunnel state: %w", err)
		}
		if active.Value != "" {
			if cfg, err := s.tunnelStore.Load(active.Value); err == nil {
				dnsServers = cfg.Interface.DNS
			}
		}
	}
	return s.call(ipc.MethodSetDNSProtection, ipc.DNSProtectionRequest{
		Enabled:    enabled,
		DNSServers: dnsServers,
	}, nil)
}

// --- Auto-update ---

// SetPinInterface enables or disables -ifscope bypass route pinning.
func (s *TunnelService) SetPinInterface(enabled bool) error {
	return s.call(ipc.MethodSetPinInterface, ipc.SetPinInterfaceRequest{Enabled: enabled}, nil)
}

// SetHealthCheck enables or disables the tunnel health check monitor.
func (s *TunnelService) SetHealthCheck(enabled bool) error {
	return s.call(ipc.MethodSetHealthCheck, ipc.SetHealthCheckRequest{Enabled: enabled}, nil)
}

// OpenURL opens a URL in the default browser. Only HTTPS URLs on
// github.com are allowed to prevent misuse from a compromised frontend.
func (s *TunnelService) OpenURL(url string) error {
	if !strings.HasPrefix(url, "https://github.com/") {
		return fmt.Errorf("URL not allowed: %s", url)
	}
	if s.app != nil {
		return s.app.Browser.OpenURL(url)
	}
	return fmt.Errorf("app not initialized")
}

// GetVersion returns the current app version string.
func (s *TunnelService) GetVersion() string {
	return update.CurrentVersion()
}

// CheckForUpdate is the legacy synchronous check kept for backward
// compatibility with the (now-removed) onMount call. New code should call
// ManualCheckForUpdate, which routes through the scheduler so the result
// is also persisted (ETag, last-checked timestamp) and the in-app banner
// can update without a separate round-trip.
func (s *TunnelService) CheckForUpdate() (*update.UpdateInfo, error) {
	if s.updateScheduler != nil {
		res, err := s.updateScheduler.CheckNow()
		if err != nil {
			return nil, err
		}
		if res == nil || res.Info == nil {
			return &update.UpdateInfo{Available: false, CurrentVer: update.CurrentVersion()}, nil
		}
		return res.Info, nil
	}
	return update.CheckForUpdate()
}

// UpdateState is the frontend-facing snapshot of persisted check state.
// Exposed so Settings → About can render the "Last checked …" line and
// the first-check placeholder correctly.
//
// IsDevBuild + AutoEnabled together let the UI distinguish three
// "not-yet-checked" cases that look identical to a naive `last_check==0`
// gate:
//
//	dev build  → scheduler is intentionally inert, show "Never checked"
//	auto off   → user disabled the scheduler, show "Never checked"
//	auto on    → first tick is 30–120 s away, show "scheduled" hint
//
// CurrentVersion is duplicated here (also in GetVersion()) so the About
// tab doesn't need two round-trips to render.
type UpdateState struct {
	CurrentVersion    string   `json:"current_version"`
	LastCheckUnix     int64    `json:"last_check_unix"`
	LastSeenVersion   string   `json:"last_seen_version"`
	DismissedVersions []string `json:"dismissed_versions"`
	IsDevBuild        bool     `json:"is_dev_build"`
	AutoEnabled       bool     `json:"auto_enabled"`
}

// GetUpdateState returns persisted state for the About tab UI.
func (s *TunnelService) GetUpdateState() UpdateState {
	out := UpdateState{
		CurrentVersion: update.CurrentVersion(),
		IsDevBuild:     update.IsDevBuild(),
		AutoEnabled:    true,
	}
	if s.settingsStore != nil {
		if cfg, _ := s.settingsStore.Load(); cfg != nil {
			out.AutoEnabled = cfg.AutoUpdateCheckEnabled()
		}
	}
	if s.updateStore != nil {
		st := s.updateStore.Get()
		out.LastCheckUnix = st.LastCheckUnix
		out.LastSeenVersion = st.LastSeenVersion
		out.DismissedVersions = st.DismissedVersions
	}
	return out
}

// DismissUpdate persists a version dismissal so the in-app banner stays
// hidden across restarts until a newer version arrives.
func (s *TunnelService) DismissUpdate(version string) error {
	if s.updateStore == nil {
		return nil
	}
	return s.updateStore.Dismiss(version)
}

// RunUpdate performs the update end-to-end:
//
//   - Homebrew installs → `brew update && brew upgrade --cask wireguideplus`,
//     letting the cask's postflight handle the killall + relaunch. This
//     is the "one-click" expectation users have, not "copy this command
//     into your terminal".
//   - Windows/Linux/macOS (non-brew) → native in-process update: download
//     the release asset (.exe/.msi/.deb/.rpm/.dmg/.zip) through the user's
//     configured mirror/proxy, verify its SHA256 + Ed25519 signature, then
//     install it — the platform installer for Windows/Linux, and an
//     in-place app-bundle replacement (elevated when needed) for macOS.
//     No browser round-trip, so it works even where github.com is
//     unreachable (the reason the Settings mirror/proxy exists). If the
//     download or verification fails, this method falls back to opening
//     the GitHub Releases page — the safe manual path — so the user is
//     never stranded.
func (s *TunnelService) RunUpdate(info *update.UpdateInfo) error {
	if info == nil || !info.Available {
		return fmt.Errorf("no update available")
	}

	switch runtime.GOOS {
	case "darwin":
		if update.IsBrewInstall() {
			return s.runUpdateBrew(info)
		}
		return s.runUpdateNative(info)
	case "windows", "linux":
		return s.runUpdateNative(info)
	default:
		return s.openReleasePage()
	}
}

// runUpdateBrew updates a Homebrew cask install via `brew upgrade`. The
// cask postflight kills and relaunches this process; see the inline
// notes for the subtle failure modes (exit 0 with nothing installed,
// untrusted-tap gating).
func (s *TunnelService) runUpdateBrew(info *update.UpdateInfo) error {
	brewBin := update.BrewPath()

	// `brew update` is pure-network (git fetch on tap repos); 90 s is
	// generous even on a slow link. If it hangs past that, the GitHub
	// API or the user's DNS is wedged — we'd rather surface that to
	// the user via a clear timeout than spin forever with "Updating…".
	// The progress event keeps the UI honest during that window — the
	// tap refresh alone was observed taking 75 s with the button stuck
	// on a static "Updating…" label the whole time.
	s.emitUpdateProgress("refresh", 0)
	updCtx, updCancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer updCancel()
	slog.Info("update: running brew update", "brew", brewBin)
	if out, err := exec.CommandContext(updCtx, brewBin, "update").CombinedOutput(); err != nil {
		slog.Warn("brew update failed, continuing with upgrade", "error", err, "output", string(out))
	}

	// `brew upgrade --cask wireguideplus` runs the cask postflight which
	// killalls and relaunches us. The postflight typically completes
	// in 10–20 s; 5 min is a defensive ceiling for slow disks or
	// signature-check work — if we hit it, brew is genuinely stuck.
	//
	// Note: the cask postflight kills *this* process, which is the
	// parent of brew's exec. Go's exec.CommandContext attaches the
	// child's Wait, but a SIGKILL on the parent terminates the wait
	// before brew completes — the new wireguide binary that brew
	// installs will be launched fresh, so this RunUpdate's return
	// value never gets surfaced anywhere in practice.
	upCtx, upCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer upCancel()
	// --greedy: older Homebrew skips auto_updates casks even when named
	// explicitly, and the skip exits 0 — so this call reported success
	// while doing nothing, stranding installs on old versions (observed
	// live: 0.3.1 pinned for three months of "Update Now" clicks). The
	// flag forces the upgrade regardless of brew version or cask flags.
	s.emitUpdateProgress("install", 0)
	runUpgrade := func() ([]byte, error) {
		slog.Info("update: running brew upgrade --cask --greedy wireguideplus")
		cmd := exec.CommandContext(upCtx, brewBin, "upgrade", "--cask", "--greedy", "wireguideplus")
		// We already ran `brew update` above — suppress the implicit
		// re-update brew would otherwise bolt onto upgrade.
		cmd.Env = append(os.Environ(), "HOMEBREW_NO_AUTO_UPDATE=1")
		return cmd.CombinedOutput()
	}
	out, err := runUpgrade()
	if err != nil && strings.Contains(string(out), "untrusted tap") {
		// Homebrew 6 gates third-party taps behind an explicit trust
		// grant and refuses to LOAD the cask otherwise ("Refusing to
		// load cask … from untrusted tap"). Interactive brew asks the
		// user; our TTY-less subprocess just gets the error. Trusting
		// our own tap here is legitimate self-service: the user
		// installed this app from that tap and clicked "Update Now" —
		// that is the consent the prompt exists to collect. Scoped to
		// exactly our tap, never a blanket trust.
		slog.Info("update: tap untrusted on this machine — trusting imonior/tap and retrying")
		tCtx, tCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer tCancel()
		if tOut, tErr := exec.CommandContext(tCtx, brewBin, "trust", "imonior/tap").CombinedOutput(); tErr != nil {
			return fmt.Errorf("brew trust imonior/tap failed: %w (%s)", tErr, string(tOut))
		}
		out, err = runUpgrade()
	}
	if err != nil {
		return fmt.Errorf("brew upgrade failed: %w (%s)", err, string(out))
	}

	// Reaching this line means brew exited 0 WITHOUT the cask
	// postflight killing us — i.e. no install actually ran (a real
	// upgrade killalls this process before CombinedOutput returns).
	// brew exits 0 on "already installed"-style skips, and treating
	// that as success is exactly how "Update Now" no-op'd silently in
	// the past. Verify the bundle on disk actually became the target
	// version and fail loudly when it didn't.
	if installed := installedBundleVersion(); installed != "" && installed != info.Version {
		return fmt.Errorf(
			"brew exited 0 but /Applications/wireguideplus.app is still %s (expected %s) — brew output: %s",
			installed, info.Version, strings.TrimSpace(string(out)))
	}
	return nil
}

// runUpdateNative downloads the release asset in-process (honouring the
// user's mirror/proxy setting from Settings → Updates) and installs it —
// the platform installer on Windows/Linux, an in-place app-bundle
// replacement on macOS. Any failure — network, checksum mismatch,
// signature verification — falls back to opening the release page in the
// browser, so the user always has a working path to the new version.
func (s *TunnelService) runUpdateNative(info *update.UpdateInfo) error {
	s.emitUpdateProgress("download", 0)
	path, err := update.DownloadUpdateProgress(info, func(done, total int64) {
		pct := 0
		if total > 0 {
			pct = int(done * 100 / total)
		}
		s.emitUpdateProgress("download", pct)
	})
	if err != nil {
		slog.Warn("update: native download/verify failed; opening release page as fallback",
			"category", "update",
			"version", info.Version,
			"error", err)
		s.emitUpdateProgress("", 0)
		return s.fallbackOpenRelease(err)
	}
	s.emitUpdateProgress("install", 0)
	if err := update.Install(path, info); err != nil {
		slog.Warn("update: native install failed; opening release page as fallback",
			"category", "update",
			"version", info.Version,
			"error", err)
		s.emitUpdateProgress("", 0)
		return s.fallbackOpenRelease(err)
	}
	// Release the temp download only AFTER Install has launched the
	// installer — Install execs the file, and removing it first makes
	// the launch fail with `fork/exec: The system cannot find the file
	// specified` (seen live on 1.1.7's in-app updater). On Windows the
	// running installer usually keeps the exe locked, so removal may be
	// refused; that's harmless — the OS cleans up %TEMP% eventually.
	_ = os.Remove(path)
	return nil
}

// fallbackOpenRelease opens the release page (the safe manual path) and
// returns an error explaining both halves of what happened, so the
// frontend can surface "auto-update failed; the release page was
// opened".
func (s *TunnelService) fallbackOpenRelease(cause error) error {
	if err := s.openReleasePage(); err != nil {
		slog.Warn("update: also failed to open the release page",
			"category", "update", "error", err)
	}
	return fmt.Errorf("auto-update failed (%v) — the release page has been opened in your browser", cause)
}

// OpenReleasePage opens the latest-release page in the default browser.
// This is the explicit "manual update" action the banner offers next to
// "Update now" — useful when the user prefers to download the installer
// by hand, or when auto-update isn't an option for their platform.
func (s *TunnelService) OpenReleasePage() error {
	return s.openReleasePage()
}

// openReleasePage is the implementation shared by the explicit
// OpenReleasePage binding and the auto-update fallback path. Uses the
// Wails browser (respects the OS default browser, no GitHub dependency
// beyond opening the page).
func (s *TunnelService) openReleasePage() error {
	slog.Info("update: opening GitHub Releases page", "url", update.GitHubReleasesURL)
	if s.app != nil {
		return s.app.Browser.OpenURL(update.GitHubReleasesURL)
	}
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", update.GitHubReleasesURL).Run()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", update.GitHubReleasesURL).Start()
	default:
		return exec.Command("xdg-open", update.GitHubReleasesURL).Start()
	}
}

// emitUpdateProgress tells the frontend which phase RunUpdate is in and
// (for "download") the percentage completed:
//   - "download"  + percent 0–100: native asset download progress
//   - "install"   + percent 0:   installer launched / brew upgrade running
//   - "refresh"   + percent 0:   brew tap refresh
//   - ""          + percent 0:   phase cleared (e.g. after a fallback)
//
// Best-effort — a nil app (tests) just skips the emit.
func (s *TunnelService) emitUpdateProgress(phase string, percent int) {
	if s.app != nil {
		s.app.Event.Emit("update_progress", map[string]any{"phase": phase, "percent": percent})
	}
}

// installedBundleVersion reads CFBundleShortVersionString from the
// installed app bundle. "" when unreadable (bundle missing, non-darwin);
// callers must treat "" as "cannot verify", not as a mismatch.
func installedBundleVersion() string {
	out, err := exec.Command("defaults", "read",
		"/Applications/wireguideplus.app/Contents/Info", "CFBundleShortVersionString").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
