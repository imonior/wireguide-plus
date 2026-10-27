package storage

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/imonior/wireguide-plus/internal/logging"
	"github.com/imonior/wireguide-plus/internal/wifi"
)

// ModuleVisibility is the per-module show/hide map used only when
// Settings.UIMode == "custom". Every flag defaults to true (see
// DefaultModuleVisibility) so "custom" starts from the full layout and the
// user hides what they don't want. The flag names are the keys the frontend
// reads through its moduleFlags store.
//
// ModuleVisibility 是仅当 UIMode == "custom" 时生效的逐项可见性开关，每个开关
// 默认 true（见 DefaultModuleVisibility），因此「自定义」从完整布局出发，由用户
// 隐藏不需要的项。开关名即前端 moduleFlags store 读取的键。
type ModuleVisibility struct {
	// NavHistory / NavDnsLeak / NavRoute / NavLog toggle the four optional
	// sidebar quick-menu entries. The tunnels entry and the settings entry
	// are always shown.
	NavHistory bool `json:"nav_history"`
	NavDnsLeak bool `json:"nav_dns_leak"`
	NavRoute   bool `json:"nav_route"`
	NavLog     bool `json:"nav_log"`
	// AutomationExtra toggles the explanatory prose inside the automation
	// editor — the help details and the automation on/off hint. The rule
	// list and the default connect/disconnect state (the "condition
	// settings") are always shown. The live-matching panel has its own
	// switch (AutomationLive below).
	//
	// AutomationExtra 控制自动化编辑器内的说明性文字——帮助折叠块与自动化开关
	// 提示。规则列表与默认连接/断开状态（即「条件设置」）始终显示。实时匹配
	// 面板有独立开关（见下方 AutomationLive）。
	AutomationExtra bool `json:"automation_extra"`
	// TunnelLatency / TunnelTraffic / TunnelNotes / TunnelEndpoint /
	// TunnelHeroMeta toggle the verbose detail blocks on the tunnel detail
	// surface (latency row, rx/tx + graph, notes, endpoint candidate table,
	// handshake/duration/interface meta). The config area, connect/disconnect
	// button, tunnel state, and endpoint quick-switch stay always on; the
	// bottom management button grid is governed by TunnelActions below.
	//
	// TunnelLatency / TunnelTraffic / TunnelNotes / TunnelEndpoint /
	// TunnelHeroMeta 控制隧道详情界面上的冗详信息块（延迟行、rx/tx + 图表、
	// 备注、endpoint 候选表、握手/时长/接口元数据）。配置区、连接/断开按钮、
	// 隧道状态与 endpoint 快速切换始终显示；底部管理按钮栅格由下方的
	// TunnelActions 控制。
	TunnelLatency  bool `json:"tunnel_latency"`
	TunnelTraffic  bool `json:"tunnel_traffic"`
	TunnelNotes    bool `json:"tunnel_notes"`
	TunnelEndpoint bool `json:"tunnel_endpoint"`
	TunnelHeroMeta bool `json:"tunnel_hero_meta"`
	// TunnelActions toggles the bottom management button grid (edit / export
	// / automation editor / delete) on the tunnel detail surface. Unlike the
	// other tunnel_* flags it stays VISIBLE in the mini preset — those
	// buttons are essential management actions, not verbose detail — so only
	// the "custom" preset can turn them off.
	//
	// TunnelActions 控制隧道详情界面底部的管理按钮栅格（编辑 / 导出 / 自动化
	// 编辑器 / 删除）。与其他 tunnel_* 开关不同，它在 mini 预设下保持可见——
	// 这些是必备的管理操作而非冗详信息——因此只有「自定义」预设能将其关闭。
	TunnelActions bool `json:"tunnel_actions"`
	// CreateHints toggles the explanatory hint text inside the tunnel editor
	// dialog (egress binding, policies, scripts). Hidden in the mini preset,
	// where only the config itself and the action buttons of features enabled
	// in Settings remain.
	//
	// CreateHints 控制隧道编辑器对话框内的说明性提示文字（出口绑定、策略、
	// 脚本）。在 mini 预设下隐藏，只保留配置本身与设置中已开启功能的操作按钮。
	CreateHints bool `json:"create_hints"`
	// AutomationLive toggles the live-matching panel inside the automation
	// editor — the data view (current network facts, per-rule match counts,
	// the live decision strip). Deliberately separate from AutomationExtra:
	// the extra switch covers explanatory prose while this one covers a
	// functional panel, so the custom preset can keep the data and drop the
	// prose (or the reverse).
	//
	// AutomationLive 控制自动化编辑器内的实时匹配面板——数据视图（当前网络
	// 事实、每条规则的匹配计数、实时决策条）。刻意与 AutomationExtra 分开：
	// 后者管的是说明性文字，这一条管的是功能面板，因此自定义预设可以保留
	// 数据、去掉文字（反之亦然）。
	AutomationLive bool `json:"automation_live"`
	// TunnelHeroNote toggles the explanatory note under the tunnel hero (the
	// handshake-interval explanation). It is deliberately separate from
	// TunnelHeroMeta above: the meta line is data (handshake / duration /
	// interface) while this one is prose, so the custom preset can keep one
	// without the other.
	//
	// TunnelHeroNote 控制隧道 hero 下方的说明文字（握手间隔说明）。它与上面的
	// TunnelHeroMeta 刻意分开：meta 行是数据（握手 / 时长 / 接口），这一条是
	// 说明文字，因此自定义预设可以只保留其中之一。
	TunnelHeroNote bool `json:"tunnel_hero_note"`
	// TunnelEndpointHints toggles the explanatory text around the endpoint
	// candidate table (switching-mode hint, candidate-list hint, ICMP note
	// and the inline help toggle). Separate from TunnelEndpoint, which gates
	// the table itself.
	//
	// TunnelEndpointHints 控制 endpoint 候选表周围的说明文字（切换模式提示、
	// 候选列表提示、ICMP 说明与内联帮助开关）。与控制表格本身的
	// TunnelEndpoint 相互独立。
	TunnelEndpointHints bool `json:"tunnel_endpoint_hints"`
	// SettingsHints toggles the hint / description text inside Settings. When
	// it is off each hint collapses into a compact "+" toggle that reveals
	// just that one line, so the compact layout stays compact until a single
	// explanation is needed. Error and warning text is never collapsed.
	//
	// SettingsHints 控制设置界面内的提示与说明文字。关闭时每条说明会收成一个
	// 紧凑的「+」开关，点击只展开该条，从而保持紧凑布局；错误与警告文字永不
	// 折叠。
	SettingsHints bool `json:"settings_hints"`
	// SettingsProxy toggles the proxy section (accelerator mirrors + public
	// IP) inside Settings.
	SettingsProxy bool `json:"settings_proxy"`
}

// DefaultModuleVisibility returns a ModuleVisibility with every flag on.
// DefaultModuleVisibility 返回所有开关均为开启的 ModuleVisibility。
func DefaultModuleVisibility() ModuleVisibility {
	return ModuleVisibility{
		NavHistory:          true,
		NavDnsLeak:          true,
		NavRoute:            true,
		NavLog:              true,
		AutomationExtra:     true,
		AutomationLive:      true,
		TunnelLatency:       true,
		TunnelTraffic:       true,
		TunnelNotes:         true,
		TunnelEndpoint:      true,
		TunnelHeroMeta:      true,
		TunnelHeroNote:      true,
		TunnelEndpointHints: true,
		TunnelActions:       true,
		CreateHints:         true,
		SettingsHints:       true,
		SettingsProxy:       true,
	}
}

// HotkeyConfig is the global hotkey that toggles (show / hide / focus) the
// main window. Enabled is opt-in (default false) so a freshly installed app
// never grabs a system-wide shortcut the user didn't ask for. Combo is the
// canonical string "Ctrl+Alt+Space" — modifiers are Ctrl/Alt/Shift/Win and
// the final segment is a KeyboardEvent.code token (e.g. "Space", "KeyT",
// "F12", "Digit1"). An empty Combo means "no key assigned".
//
// HotkeyConfig 是切换（显示/隐藏/聚焦）主窗口的全局热键配置。Enabled 为
// 选择性开启（默认 false），保证全新安装不会擅自占用系统级快捷键。Combo 是
// 形如 "Ctrl+Alt+Space" 的规范字符串——修饰键为 Ctrl/Alt/Shift/Win，末段是
// KeyboardEvent.code 令牌（如 "Space"、"KeyT"、"F12"、"Digit1"）。空 Combo
// 表示「未指定按键」。
type HotkeyConfig struct {
	Enabled bool   `json:"enabled"`
	Combo   string `json:"combo"`
}

// DefaultHotkeyConfig returns the hotkey default: disabled, with a sensible
// cross-platform combo pre-filled so a one-click enable just works.
// DefaultHotkeyConfig 返回热键默认值：关闭，并预填一个跨平台通用的组合键，
// 这样一键开启即可生效。
func DefaultHotkeyConfig() HotkeyConfig {
	return HotkeyConfig{Enabled: false, Combo: "Ctrl+Alt+Space"}
}

// Settings holds application-wide settings.
type Settings struct {
	Language       string `json:"language"`        // "auto", "en", "ko", "ja", "zh"
	Theme          string `json:"theme"`           // "dark", "light", "system"
	TrayIconStyle  string `json:"tray_icon_style"` // "color" (MVP: color only)
	AutoStart      bool   `json:"auto_start"`      // launch GUI on OS login
	StartMinimized bool   `json:"start_minimized"` // start hidden in the tray
	// DisconnectOnQuit decides what "Quit" means for tunnels that are up.
	//
	// true (the historical behaviour, and the default when the key is
	// absent) tears the tunnels down and stops the helper, so quitting the
	// GUI ends the VPN session. false leaves both alone: the GUI process
	// exits, the helper keeps running with its tunnels, and the next GUI
	// launch reattaches to the same helper. That second mode is the one
	// users want when the tunnel is the machine's only route out — quitting
	// the window manager must not drop the network.
	//
	// Pointer so "never touched this" is distinguishable from an explicit
	// false, which is what a plain bool cannot express for a key whose
	// default is true. Use DisconnectOnQuitEnabled rather than
	// dereferencing.
	DisconnectOnQuit *bool `json:"disconnect_on_quit,omitempty"`
	// NotifyDurationMs is how long the connection-status notification
	// bubble stays on screen (ms). 0 means the default (10s).
	NotifyDurationMs int `json:"notify_duration_ms,omitempty"`
	// Legacy keys "kill_switch" and "dns_protection" were global toggles
	// and are GONE: a machine-wide kill switch cannot express intent when
	// several tunnels run at once, and a single "force DNS through the
	// VPN" switch cannot say WHICH of several connected tunnels the
	// system resolver should use. Both are now per-tunnel policies in the
	// .meta.json sidecar (TrafficProtect and DNSResolvePath). Unknown keys in
	// an existing config.json are simply ignored on load, so a user who
	// had them enabled keeps working — the rules they installed are torn
	// down with the helper that installed them.
	// DNSResolvePath is the MASTER switch for the per-tunnel "DNS resolve
	// path" policy (principle 33). Off means the feature does not exist:
	// no per-tunnel toggle is shown, nothing is enforced, and no conflict
	// is reported. Turning it on only reveals the per-tunnel switches —
	// each tunnel still has to opt in on its own.
	DNSResolvePath bool `json:"dns_resolve_path"`
	// HealthCheck is the single "auto-repair" switch: when on, the helper
	// periodically monitors handshake age, reconnects a stale tunnel, AND
	// self-heals dead tunnels on session resume (display on / unlock).
	// When off, no automatic repair happens anywhere — the user reconnects
	// by hand. (Formerly "keep connection on idle" gated only the resume
	// heal; the two overlapping switches were merged into this one, and
	// the old settings key is silently ignored on load.)
	HealthCheck bool `json:"health_check"` // periodic handshake age monitoring
	// PreventSystemSleep is the master switch for "keep running in the
	// background": while on, WireGuide Plus asks the OS not to sleep, suspend,
	// or turn the display off (ignore screensaver / screen-off / hibernation).
	// OFF by default — this is an invasive power behaviour the user must opt
	// into (it overrides the system's own sleep policy and can keep a laptop
	// awake on battery). The helper applies it via SetThreadExecutionState on
	// Windows; other platforms persist the flag but have no equivalent call.
	PreventSystemSleep bool   `json:"prevent_system_sleep"`
	PinInterface       bool   `json:"pin_interface"` // pin bypass routes to upstream interface (-ifscope)
	LogLevel           string `json:"log_level"`     // "debug", "info", "warn", "error"
	CompactList        bool   `json:"compact_list"`  // dense tunnel list: hide endpoint line, shorter rows
	// UIMode selects the GUI module-visibility preset:
	//   "all"   (default) — every module shows, exactly as before.
	//   "mini"  — every OPTIONAL module is hidden; only the always-on
	//             essentials stay (config area, condition settings, connect/
	//             disconnect, tunnel state, scripts when enabled).
	//   "custom" — each module's visibility follows the per-flag map in
	//             ModuleVisibility below.
	// The json key is kept as "ui_mode" for backward compatibility; an older
	// stored value "normal" is treated as "all" on the frontend.
	//
	// UIMode 选择 GUI 模块可见性预设：
	//   "all"（默认）—— 所有模块均显示，与改动前一致。
	//   "mini" —— 隐藏所有可选项，仅保留始终显示的要素（配置区、条件设置区、
	//             连接/断开、隧道状态、脚本（开启时））。
	//   "custom" —— 各模块可见性遵循下方 ModuleVisibility 的逐项开关。
	// json 键保留为 "ui_mode" 以兼容旧配置；旧值 "normal" 在前端按 "all" 处理。
	UIMode string `json:"ui_mode"` // "all", "mini", "custom"
	// ModuleVisibility is the per-module show/hide map used only when
	// UIMode == "custom". Every flag defaults to true so "custom" starts
	// from the full layout and the user hides what they don't want.
	//
	// ModuleVisibility 是仅当 UIMode == "custom" 时生效的逐项可见性开关。
	// 每个开关默认 true，因此「自定义」从完整布局出发，由用户隐藏不需要的项。
	ModuleVisibility ModuleVisibility `json:"ui_visibility"`
	// Hotkey toggles (show / hide / focus) the main window with a
	// system-wide shortcut. Disabled by default — see HotkeyConfig.
	// Hotkey 用系统级快捷键切换（显示/隐藏/聚焦）主窗口。默认关闭，详见 HotkeyConfig。
	Hotkey HotkeyConfig `json:"hotkey"`
	// LogRetentionDays is how many days of daily log files to keep.
	// 0 means the default (7). Files older than this are removed at
	// startup and whenever settings are saved.
	LogRetentionDays int `json:"log_retention_days,omitempty"`
	// HistoryRetentionDays is how many days of connection history to keep.
	// 0 means the default (7). Records older than this are pruned whenever
	// the history is loaded for display.
	HistoryRetentionDays int `json:"history_retention_days,omitempty"`
	// EnableWgScripts controls whether PreUp/PostUp/PreDown/PostDown
	// scripts embedded in tunnel configs are executed. OFF by default:
	// scripts run inside the privileged helper (root/admin) and arbitrary
	// config-driven commands are a security risk. Enabling shows a
	// prominent warning in Settings.
	EnableWgScripts bool `json:"enable_wg_scripts"`

	// EnableAWG controls whether AmneziaWG (AWG) tunnel configs are allowed
	// to connect. ON by default. It is an escape hatch: AWG runs on a fork
	// of WireGuard (amneziawg-go) with obfuscation, and if that backend
	// misbehaves on a given machine the user can disable it and get a clear
	// "AmneziaWG is disabled" error instead of a broken connect. Disabling
	// does NOT downgrade AWG configs to plain WireGuard — the server side
	// requires the obfuscated handshake, so AWG tunnels simply refuse to
	// connect until this is re-enabled.
	EnableAWG bool `json:"enable_awg"`

	// ListSort controls tunnel-list ordering: "name_asc" (default),
	// "name_desc". ListActiveOnTop floats connected tunnels above the
	// sort. Both are pure view state managed from the list header.
	ListSort        string `json:"list_sort"`
	ListActiveOnTop bool   `json:"list_active_on_top"`
	// ListPaneWidth is the draggable width (px) of the tunnel-list
	// column. 0 falls back to the default.
	ListPaneWidth int `json:"list_pane_width,omitempty"`

	// AutoUpdateCheck controls the periodic update scheduler. *bool so we
	// can distinguish "user never touched this" from "user explicitly
	// turned it off" — defaults to true on first load. A user who installs
	// via brew might prefer to disable in-app checks and let brew handle
	// it instead; an offline / corporate-network user might disable to
	// silence the failed-check log noise.
	AutoUpdateCheck *bool `json:"auto_update_check,omitempty"`

	// Automation is the per-tunnel condition→action rule model (issue
	// #12) — the sole automation model. A nil pointer simply means "no
	// policy yet"; EnsureAutomation() initialises it to an empty model,
	// so every caller (engine, preview, editor) can rely on a non-nil
	// value. The legacy SSID-only WifiRules model it replaced was
	// retired: any `wifi_rules` key in an existing settings file is now
	// ignored on load, and upgrading from a pre-2.0.0 release is a
	// manual, documented step (see CHANGELOG).
	Automation *wifi.Automation `json:"automation,omitempty"`

	// ManualOffTunnels lists tunnels the user has manually switched off
	// from the UI or tray menu. While a tunnel is listed, the automation
	// engine refuses to auto-connect it even when rules match — the user's
	// manual off wins until they manually reconnect that tunnel once or
	// the app restarts (the GUI clears the list on startup).
	ManualOffTunnels []string `json:"manual_off_tunnels,omitempty"`

	// ManualOnTunnels is the mirror of ManualOffTunnels: tunnels the user
	// has manually switched ON. While a tunnel is listed, the automation
	// engine refuses to auto-DISCONNECT it even when rules match — the
	// user's manual on wins until they manually disconnect that tunnel
	// once or the app restarts (the GUI clears the list on startup).
	// The two latches are mutually exclusive: a tunnel appears in at most
	// one of the lists.
	ManualOnTunnels []string `json:"manual_on_tunnels,omitempty"`

	// ProxyMode controls outbound proxying for update checks: "direct"
	// (default, no proxy), "mirror" (rewrite the API URL through a GitHub
	// accelerator prefix stored in ProxyURL) or "manual" (use ProxyURL as
	// an HTTP/SOCKS proxy). Intended for users who can't reach GitHub
	// directly (e.g. mainland China) — they can route update checks through
	// an accelerator mirror or a local HTTP/SOCKS proxy.
	ProxyMode string `json:"proxy_mode,omitempty"`
	// ProxyURL is the proxy address used when ProxyMode is "manual" (e.g.
	// "http://127.0.0.1:7890" or "socks5://127.0.0.1:1080"), or the
	// accelerator prefix used when ProxyMode is "mirror" (e.g.
	// "https://ghfast.top").
	ProxyURL string `json:"proxy_url,omitempty"`

	// ProxyMirrors is the user-managed list of GitHub accelerator mirror
	// prefixes offered by Settings → Proxy. nil means "never customized",
	// in which case EffectiveProxyMirrors falls back to DefaultProxyMirrors;
	// an empty (but non-nil) slice means the user deleted every mirror and
	// wants none offered. The tag deliberately omits `omitempty`: with it,
	// both nil and [] serialize to nothing and a deleted list would
	// silently resurrect the defaults on the next load.
	// ProxyMirrors 是用户在「设置 → 代理」中自行管理的 GitHub 加速镜像前缀列表。
	// nil 表示「从未自定义」，此时 EffectiveProxyMirrors 回落到 DefaultProxyMirrors；
	// 空但非 nil 的切片表示用户删光了所有镜像、一个都不想要。这里刻意不加
	// `omitempty`：否则 nil 与 [] 都会序列化成空，删掉的列表会在下次读取时悄悄复活。
	ProxyMirrors []string `json:"proxy_mirrors"`

	// PublicIPLookup gates the "current public IP + geolocation" readout in
	// Settings → Proxy. The lookup necessarily discloses the machine's
	// public IP to a third-party service, so the provider name is always
	// shown next to the result; nil (never set) means enabled, which keeps
	// the feature discoverable while leaving a permanent opt-out.
	// PublicIPLookup 控制「设置 → 代理」里的「当前公网 IP + 归属地」读数。
	// 该查询必然要把本机公网 IP 暴露给第三方服务，因此结果旁边永远显示服务商名；
	// nil（从未设置）视为启用，既保证功能可见，也留下永久的关闭开关。
	PublicIPLookup *bool `json:"public_ip_lookup,omitempty"`

	// EndpointFailover is the GLOBAL master switch for multi-endpoint
	// candidate selection. nil (never set) counts as enabled, the default —
	// every other install already behaves as if it were on, and an upgrade
	// must not silently disable a feature the user never toggled. When off,
	// NO tunnel switches endpoints, even one with auto_switch armed: the
	// per-tunnel toggle becomes inert and is greyed out in the UI. The
	// per-tunnel switch is therefore a refinement of this master switch,
	// never its replacement.
	// EndpointFailover 是多 endpoint 候选选择的**全局总开关**。nil（从未设置）
	// 视为启用（默认）——其它安装本就表现得像开着，升级不能静默关掉用户从没动过的功能。
	// 关闭时，任何隧道都不会切换 endpoint，即便它已开启 auto_switch：每隧道开关失效、
	// 并在界面灰掉。因此每隧道开关只是本总开关的细化，绝不是它的替代品。
	EndpointFailover *bool `json:"endpoint_failover_enabled,omitempty"`

	// EndpointThresholdMs is the latency (ms) above which the current
	// candidate is even considered for replacement. Below it the current
	// address is "good enough" and the app never churns. 0 is clamped to the
	// default (150) so a hand-edited 0 cannot disable the floor entirely.
	// EndpointThresholdMs 是当前候选延迟超过该值（毫秒）才被考虑替换的门槛。
	// 低于它当前地址即「够好」，应用绝不折腾。0 会被钳到默认值（150），
	// 以免手改的 0 把门槛整个取消。
	EndpointThresholdMs int `json:"endpoint_threshold_ms,omitempty"`

	// EndpointMarginPct is how much faster (relative to the current latency)
	// an alternative must be to replace it. A smaller margin causes more
	// switching; 0 is clamped to the default (30) so two near-equal
	// addresses never trade places on noise.
	// EndpointMarginPct 是备选相对当前延迟必须「快出多少个百分点」才会替换。
	// 幅度越小切换越频繁；0 钳到默认值（30），避免两个差不多快的地址在噪声上互切。
	EndpointMarginPct int `json:"endpoint_margin_pct,omitempty"`

	// EndpointSustainedSec is the confirmation window for an IMPROVEMENT
	// switch: the better candidate must be observed across this many seconds
	// (two switch opportunities at least that far apart) before the app
	// moves. It guards against a single transient probe flipping the
	// address. A failover (current unreachable) is always immediate. 0 means
	// "commit on the first observation" — no sustained gate.
	// EndpointSustainedSec 是「择优」切换的确认窗口：备选必须在这么长时间内被持续观察到更优
	// （两次切换机会至少相隔这么久）才会移动。它防止单次瞬冲探测就翻转地址。
	// 故障切换（当前不可达）永远立即执行。0 表示「首次观察到即提交」——不加持续门控。
	EndpointSustainedSec int `json:"endpoint_sustained_sec,omitempty"`

	// EndpointCooldownSec is the minimum gap between two automatic switches
	// on the same tunnel. It is clamped to >= MinEndpointCooldownSec (240) so
	// it always exceeds the 180s dead-link action window
	// (internal/reconnect/monitor.go): a freshly switched endpoint must get a
	// full dead-link evaluation cycle before it is eligible to switch again,
	// otherwise a slow-but-alive link could churn on the same cadence as
	// dead-link detection. The default (DefaultEndpointCooldownSec, 300) keeps
	// a comfortable margin.
	// EndpointCooldownSec 是同一隧道两次自动切换之间的最小间隔。它被钳到 ≥ MinEndpointCooldownSec（240），
	// 从而恒大于 180s 的死链动作窗口（internal/reconnect/monitor.go）：刚切到的地址必须
	// 拿到一个完整的死链评估周期才有资格再切，否则一个慢但活着的链会和死链检测同频抖动。
	// 默认 DefaultEndpointCooldownSec（300）留了舒服的余量。
	EndpointCooldownSec int `json:"endpoint_cooldown_sec,omitempty"`

	// DNSTestPublicServers is the user's customized list of public DNS
	// resolvers probed as a cross-check by the DNS leak test. nil or empty
	// means "no customization" — the effective list falls back to the
	// network-fetched list (DNSTestPublicFetched) when available, else the
	// built-in defaults. A non-empty list replaces the defaults verbatim.
	// Clearing the list NEVER disables public probing: public cross-check is
	// core to the leak test, so an empty custom list simply restores the
	// default cross-check set. The DNS leak panel edits this list.
	DNSTestPublicServers []string `json:"dns_test_public_servers"`
	// DNSTestPublicFetched caches the last network-fetched public DNS list
	// (see diag.FetchPublicResolvers) so the effective default cross-check
	// set stays current even offline / when a later refresh fails. It is
	// overridden by DNSTestPublicServers when the user customizes.
	DNSTestPublicFetched []string `json:"dns_test_public_fetched,omitempty"`
	// DNSTestPublicFetchedAt is the unix timestamp of the last successful
	// network fetch of DNSTestPublicFetched (0 = never fetched).
	DNSTestPublicFetchedAt int64 `json:"dns_test_public_fetched_at,omitempty"`
}

// IsManualOff reports whether the user has manually switched the tunnel
// off, suspending its automation rules until it is reconnected by hand.
func (s *Settings) IsManualOff(name string) bool {
	if s == nil {
		return false
	}
	for _, n := range s.ManualOffTunnels {
		if n == name {
			return true
		}
	}
	return false
}

// SetManualOff marks a tunnel as manually switched off. The two latches
// are mutually exclusive, so setting "off" clears any "on". Call inside a
// SettingsStore.Update (or on a freshly loaded object before Save).
func (s *Settings) SetManualOff(name string) {
	s.ClearManualOn(name)
	if s.IsManualOff(name) {
		return
	}
	s.ManualOffTunnels = append(s.ManualOffTunnels, name)
}

// ClearManualOff removes a tunnel from the manual-off list — a manual
// reconnect releases it back to automation. Call inside a
// SettingsStore.Update (or on a freshly loaded object before Save).
func (s *Settings) ClearManualOff(name string) {
	for i, n := range s.ManualOffTunnels {
		if n == name {
			s.ManualOffTunnels = append(s.ManualOffTunnels[:i], s.ManualOffTunnels[i+1:]...)
			return
		}
	}
}

// IsManualOn reports whether the user has manually switched the tunnel on,
// suspending its automation rules (auto-disconnect) until it is disconnected
// by hand or the app restarts.
func (s *Settings) IsManualOn(name string) bool {
	if s == nil {
		return false
	}
	for _, n := range s.ManualOnTunnels {
		if n == name {
			return true
		}
	}
	return false
}

// SetManualOn marks a tunnel as manually switched on (the mirror of
// SetManualOff). The two latches are mutually exclusive, so setting "on"
// clears any "off". Call inside a SettingsStore.Update (or on a freshly
// loaded object before Save).
func (s *Settings) SetManualOn(name string) {
	s.ClearManualOff(name)
	if s.IsManualOn(name) {
		return
	}
	s.ManualOnTunnels = append(s.ManualOnTunnels, name)
}

// ClearManualOn removes a tunnel from the manual-on list — a manual
// disconnect releases it back to automation. Call inside a
// SettingsStore.Update (or on a freshly loaded object before Save).
func (s *Settings) ClearManualOn(name string) {
	for i, n := range s.ManualOnTunnels {
		if n == name {
			s.ManualOnTunnels = append(s.ManualOnTunnels[:i], s.ManualOnTunnels[i+1:]...)
			return
		}
	}
}

// ClearAllManualOverrides releases every manual latch (both off and on),
// restoring full automation. The GUI calls this on startup so a fresh app
// session resumes automatic rule enforcement.
func (s *Settings) ClearAllManualOverrides() {
	s.ManualOffTunnels = nil
	s.ManualOnTunnels = nil
}

// EnsureAutomation guarantees a usable Automation model on the settings,
// so every consumer (rule engine, preview, editor) can read and write it
// without nil checks. It initialises an empty model when none is stored
// and then folds pre-Default-State rule shapes (none_match "Otherwise",
// missing defaults) into the current policy model; Normalize is
// idempotent and never overwrites an explicit Defaults entry, so this
// never clobbers rules the user edited. Callers that want the result
// persisted must Save afterwards; the rule engine can call it on each
// load without persisting (it's deterministic and cheap).
func (s *Settings) EnsureAutomation() {
	if s.Automation == nil {
		s.Automation = wifi.DefaultAutomation()
	}
	s.Automation.Normalize()
}

// RenameTunnelRules moves a tunnel's Automation entries from oldName to
// newName. Automation rules are keyed by tunnel name, so a rename that
// doesn't carry them over silently orphans the rules — and, worse, they'd
// re-attach if a new tunnel later reused the old name (issue #12). Call
// inside a SettingsStore.Update.
func (s *Settings) RenameTunnelRules(oldName, newName string) {
	if oldName == newName {
		return
	}
	if s.Automation != nil && s.Automation.PerTunnel != nil {
		if r, ok := s.Automation.PerTunnel[oldName]; ok {
			s.Automation.PerTunnel[newName] = r
			delete(s.Automation.PerTunnel, oldName)
		}
	}
	if s.Automation != nil && s.Automation.Defaults != nil {
		if d, ok := s.Automation.Defaults[oldName]; ok {
			s.Automation.Defaults[newName] = d
			delete(s.Automation.Defaults, oldName)
		}
	}
	if s.IsManualOff(oldName) {
		s.ClearManualOff(oldName)
		s.SetManualOff(newName)
	}
	if s.IsManualOn(oldName) {
		s.ClearManualOn(oldName)
		s.SetManualOn(newName)
	}
}

// DeleteTunnelRules drops a tunnel's Automation entries (both its rules
// and its Default State) so a deleted tunnel leaves no stale policy
// behind that could unexpectedly attach to a same-named tunnel created
// later (issue #12).
func (s *Settings) DeleteTunnelRules(name string) {
	if s.Automation != nil {
		delete(s.Automation.PerTunnel, name)
		if s.Automation.Defaults != nil {
			delete(s.Automation.Defaults, name)
		}
	}
	s.ClearManualOff(name)
	s.ClearManualOn(name)
}

// DefaultSettings returns settings with sensible defaults.
func DefaultSettings() *Settings {
	on := true
	return &Settings{
		Language:             "auto",
		Theme:                "system", // follows OS dark/light mode
		TrayIconStyle:        "color",
		UIMode:               "all",                     // module-visibility preset: "all" | "mini" | "custom"
		ModuleVisibility:     DefaultModuleVisibility(), // every module on; used when UIMode == "custom"
		Hotkey:               DefaultHotkeyConfig(),     // global toggle-window hotkey, opt-in (disabled)
		NotifyDurationMs:     10000,                     // 10s default notification duration
		HealthCheck:          true,                      // periodic handshake monitoring + auto-reconnect (stale + on resume) ON by default / 周期性握手监测与自动重连（含解锁自愈）默认开启
		PreventSystemSleep:   false,                     // opt-in: an invasive power override, leave it to the user
		PinInterface:         false,                     // off by default — enable for dual-network setups
		EnableAWG:            true,                      // AWG support is on by default
		LogLevel:             "info",
		LogRetentionDays:     logging.DefaultRetentionDays,
		HistoryRetentionDays: DefaultHistoryRetentionDays,
		AutoUpdateCheck:      &on,
		DisconnectOnQuit:     &on,
		ListSort:             "name_asc",
		ListActiveOnTop:      true,
		ListPaneWidth:        240,

		// Endpoint failover: master on by default; the tuning knobs match
		// the package-level defaults in internal/endpoint so a fresh config
		// and an explicit one behave identically.
		// Endpoint 容灾：总开关默认开；调参默认值与 internal/endpoint 的包级默认一致，
		// 使全新配置与显式配置行为相同。
		EndpointThresholdMs:  150,
		EndpointMarginPct:    30,
		EndpointSustainedSec: 30,
		EndpointCooldownSec:  DefaultEndpointCooldownSec,
	}
}

// DefaultProxyMirrors are the built-in GitHub accelerator prefixes offered
// in Settings → Proxy until the user edits the list. They are only a
// starting point: every one of them can be deleted, and restoring the
// defaults brings them back. A copy is returned so callers cannot mutate
// the package-level list by accident.
// DefaultProxyMirrors 是「设置 → 代理」内置的 GitHub 加速镜像前缀，用户在编辑列表
// 之前看到的就这些。它们只是起点：每一个都可以被删除，恢复默认即可找回。
// 返回副本，避免调用方误改包级列表。
func DefaultProxyMirrors() []string {
	return []string{
		"https://ghfast.top",
		"https://gh-proxy.com",
		"https://ghproxy.net",
		"https://mirror.ghproxy.com",
	}
}

// EffectiveProxyMirrors returns the mirror prefixes the UI should offer. A
// nil ProxyMirrors (never customized) yields the defaults; any non-nil
// slice — including an empty one — is authoritative, so "delete them all"
// sticks across restarts.
// EffectiveProxyMirrors 返回界面应当展示的镜像前缀。nil（从未自定义）回落到默认；
// 任何非 nil 切片（含空切片）都以它为准，因此「全部删除」能在重启后保持。
func (s *Settings) EffectiveProxyMirrors() []string {
	if s == nil || s.ProxyMirrors == nil {
		return DefaultProxyMirrors()
	}
	return append([]string(nil), s.ProxyMirrors...)
}

// PublicIPLookupEnabled reports whether Settings may query the public IP /
// geolocation service. nil (never set) counts as enabled; an explicit false
// disables it permanently.
// PublicIPLookupEnabled 判断设置页是否允许查询公网 IP / 归属地服务。
// nil（从未设置）视为启用；显式 false 表示用户永久关闭。
func (s *Settings) PublicIPLookupEnabled() bool {
	if s == nil || s.PublicIPLookup == nil {
		return true
	}
	return *s.PublicIPLookup
}

// EndpointFailoverEnabled reports whether the global multi-endpoint master
// switch is on. nil (never set) counts as enabled — the default.
// EndpointFailoverEnabled 返回全局多 endpoint 总开关是否开启。nil（从未设置）视为开启——默认。
func (s *Settings) EndpointFailoverEnabled() bool {
	if s == nil || s.EndpointFailover == nil {
		return true
	}
	return *s.EndpointFailover
}

// EffectiveEndpointThresholdMs returns the latency floor, defaulting to 150
// when unset and clamped to at least 1 (0 would remove the floor).
// EffectiveEndpointThresholdMs 返回延迟门槛，未设置时为 150，且钳到至少 1（0 会取消门槛）。
func (s *Settings) EffectiveEndpointThresholdMs() int {
	if s == nil || s.EndpointThresholdMs < 1 {
		return 150
	}
	return s.EndpointThresholdMs
}

// EffectiveEndpointMarginPct returns the improvement margin, defaulting to 30
// when unset and clamped to at least 1 (0 would let equal addresses trade).
// EffectiveEndpointMarginPct 返回改善幅度，未设置时为 30，且钳到至少 1（0 会让相等的地址互切）。
func (s *Settings) EffectiveEndpointMarginPct() int {
	if s == nil || s.EndpointMarginPct < 1 {
		return 30
	}
	return s.EndpointMarginPct
}

// EffectiveEndpointSustainedSec returns the improvement confirmation window.
// 0 (the default-equivalent) means "commit on first observation" — there is
// no minimum, so 0 is NOT clamped to a default.
// EffectiveEndpointSustainedSec 返回择优确认窗口。0（等价默认）表示「首次观察到即提交」——
// 没有下限，因此 0 不被钳制成默认值。
func (s *Settings) EffectiveEndpointSustainedSec() int {
	if s == nil {
		return 0
	}
	if s.EndpointSustainedSec < 0 {
		return 0
	}
	return s.EndpointSustainedSec
}

// MinEndpointCooldownSec and DefaultEndpointCooldownSec are the single source
// of truth for the endpoint-switch cooldown. The floor must stay above the
// 180s dead-link action window (internal/reconnect/monitor.go) so a freshly
// switched endpoint gets a full dead-link evaluation cycle before it may
// switch again. SaveSettings, EffectiveEndpointCooldownSec and the helper-side
// re-check all reference these constants instead of hard-coding the numbers.
// MinEndpointCooldownSec 与 DefaultEndpointCooldownSec 是 endpoint 切换冷却的单一真相源。
// 下限必须高于 180s 死链动作窗口（internal/reconnect/monitor.go），使刚切到的地址拿到一个
// 完整死链评估周期才有资格再切。SaveSettings、EffectiveEndpointCooldownSec 与 helper 端
// 再校验都引用这两个常量，而非把数字写死。
const (
	MinEndpointCooldownSec     = 240
	DefaultEndpointCooldownSec = 300
)

// EffectiveEndpointCooldownSec returns the switch cooldown, defaulting to
// DefaultEndpointCooldownSec and clamped to at least MinEndpointCooldownSec so
// it always exceeds the 180s dead-link window.
// EffectiveEndpointCooldownSec 返回切换冷却，默认 DefaultEndpointCooldownSec，
// 且钳到至少 MinEndpointCooldownSec，恒大于 180s 死链窗口。
func (s *Settings) EffectiveEndpointCooldownSec() int {
	if s == nil || s.EndpointCooldownSec < MinEndpointCooldownSec {
		return DefaultEndpointCooldownSec
	}
	return s.EndpointCooldownSec
}

// AutoUpdateCheckEnabled returns the effective value, treating nil
// (unset in legacy settings.json) as true. Callers should use this
// rather than dereferencing the pointer directly.
func (s *Settings) AutoUpdateCheckEnabled() bool {
	if s == nil || s.AutoUpdateCheck == nil {
		return true
	}
	return *s.AutoUpdateCheck
}

// DisconnectOnQuitEnabled returns the effective value, treating nil (a
// settings.json written before the key existed) as true — the behaviour
// every existing install already has, so upgrading never silently starts
// leaving tunnels up behind the user's back.
func (s *Settings) DisconnectOnQuitEnabled() bool {
	if s == nil || s.DisconnectOnQuit == nil {
		return true
	}
	return *s.DisconnectOnQuit
}

// PreventSystemSleepEnabled reports whether the "keep running in background"
// master switch is on. When off the OS is free to sleep/suspend/turn the
// display off as usual; when on the helper inhibits those transitions.
func (s *Settings) PreventSystemSleepEnabled() bool {
	return s != nil && s.PreventSystemSleep
}

// SettingsStore manages the app settings JSON file.
type SettingsStore struct {
	mu   sync.Mutex
	path string
}

// NewSettingsStore creates a store for the given config directory.
func NewSettingsStore(configDir string) *SettingsStore {
	return &SettingsStore{
		path: filepath.Join(configDir, "config.json"),
	}
}

// Load reads settings from disk. Returns defaults if file doesn't exist.
func (s *SettingsStore) Load() (*Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

// loadLocked is Load without the mutex, for callers (Update) that already
// hold s.mu. Writes are atomic renames, so a concurrent writer never
// exposes a torn file to a reader — no file lock is needed just to read.
func (s *SettingsStore) loadLocked() (*Settings, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultSettings(), nil
		}
		return nil, err
	}

	settings := DefaultSettings()
	if err := json.Unmarshal(data, settings); err != nil {
		// Corrupt settings file (truncated write, manual edit, etc.) should
		// not prevent the application from starting. Log the error, back up
		// the corrupt file for debugging, and return default settings.
		slog.Warn("settings file is corrupt, falling back to defaults",
			"path", s.path, "error", err)
		// If the .corrupt rename fails (e.g. Windows file-in-use, ENOSPC),
		// the next Load would hit the same corrupt file and warn again
		// forever. Try to remove the corrupt source as a last resort so
		// the next Save can write a fresh file. Errors here are also
		// best-effort — the in-memory defaults still work for this
		// session.
		corruptPath := s.path + ".corrupt"
		if renameErr := os.Rename(s.path, corruptPath); renameErr != nil {
			slog.Warn("could not back up corrupt settings; attempting direct removal",
				"rename_error", renameErr)
			if rmErr := os.Remove(s.path); rmErr != nil {
				slog.Warn("could not remove corrupt settings either; will keep retrying on each Load",
					"remove_error", rmErr)
			}
		}
		return DefaultSettings(), nil
	}
	return settings, nil
}

// Update runs a read-modify-write of the settings file that is atomic
// ACROSS PROCESSES: it holds an exclusive file lock across the whole
// load → mutate → save, so a `wireguideplus ctl` edit and a GUI edit can't
// clobber each other (e.g. a CLI automation edit reverting a GUI health-check
// change). Field-level mutators (the CLI, tunnel rename/delete) should use this rather than Load-then-Save. mutate sees the
// freshest on-disk state.
func (s *SettingsStore) Update(mutate func(*Settings) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	lf, err := os.OpenFile(s.path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lf.Close()
	if err := flockExclusive(int(lf.Fd())); err != nil {
		return err
	}
	defer flockUnlock(int(lf.Fd())) //nolint:errcheck
	settings, err := s.loadLocked()
	if err != nil {
		return err
	}
	if err := mutate(settings); err != nil {
		return err
	}
	return s.saveLocked(settings)
}

// Save writes settings to disk atomically. It takes the same
// cross-process file lock as Update so a whole-object GUI write can't
// interleave with an in-progress CLI Update at the file level.
func (s *SettingsStore) Save(settings *Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	if lf, err := os.OpenFile(s.path+".lock", os.O_CREATE|os.O_RDWR, 0600); err == nil {
		defer lf.Close()
		if flockExclusive(int(lf.Fd())) == nil {
			defer flockUnlock(int(lf.Fd())) //nolint:errcheck
		}
	}
	return s.saveLocked(settings)
}

// ReplaceRaw installs a whole settings document supplied as ready-made bytes
// (a settings package restored from an export), holding the same mutex + file
// lock as Save so it can't interleave with a concurrent CLI Update. The bytes
// are parsed before anything touches the disk: a document that doesn't decode
// is refused instead of written, because a config.json the app can't parse
// gets quarantined to .corrupt on the next load and the user silently loses
// every setting they had. Writing through here rather than with os.WriteFile
// at the call site is the point — that would replace the file non-atomically
// and without the lock, so a crash mid-write leaves no settings at all and a
// concurrent reader can see a torn file.
func (s *SettingsStore) ReplaceRaw(data []byte) error {
	var probe Settings
	if err := json.Unmarshal(data, &probe); err != nil {
		return fmt.Errorf("settings document is not valid: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	if lf, err := os.OpenFile(s.path+".lock", os.O_CREATE|os.O_RDWR, 0600); err == nil {
		defer lf.Close()
		if flockExclusive(int(lf.Fd())) == nil {
			defer flockUnlock(int(lf.Fd())) //nolint:errcheck
		}
	}
	return writeBytesAtomic(data, s.path)
}

// saveLocked is Save's body without the mutex/flock, for callers (Update)
// that already hold both.
func (s *SettingsStore) saveLocked(settings *Settings) error {
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return writeBytesAtomic(data, s.path)
}
