package app

import (
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"

	"github.com/imonior/wireguide-plus/internal/config"
	"github.com/imonior/wireguide-plus/internal/domain"
	"github.com/imonior/wireguide-plus/internal/endpoint"
	"github.com/imonior/wireguide-plus/internal/ipc"
	"github.com/imonior/wireguide-plus/internal/storage"
)

// EndpointSettings is the tunnel editor's view of endpoint failover: the
// address currently written into the .conf, the candidate list, and whether
// automatic selection is armed.
// EndpointSettings 是隧道编辑器看到的 endpoint  failover 视图：.conf 里当前写入的
// 地址、候选列表，以及是否启用了自动选择。
type EndpointSettings struct {
	Active     string   `json:"active"`
	Candidates []string `json:"candidates"`
	AutoSwitch bool     `json:"auto_switch"`
	// SwitchMode is the normalized switch timing: "off", "reconnect" or
	// "dynamic" (see storage.TunnelMeta.EndpointSwitchTiming).
	// SwitchMode 是归一化的切换时机："off" / "reconnect" / "dynamic"
	// （见 storage.TunnelMeta.EndpointSwitchTiming）。
	SwitchMode string `json:"switch_mode"`
}

// GetEndpointSettings returns the candidate list for the tunnel editor. The
// active endpoint always reads back from the .conf (never from the meta), so
// what the user sees is what WireGuard will actually dial.
// GetEndpointSettings 返回供隧道编辑器使用的候选列表。生效地址始终从 .conf 读取
// （绝不取自 meta），因此用户看到的就是 WireGuard 真正会去连的那个地址。
func (s *TunnelService) GetEndpointSettings(name string) (EndpointSettings, error) {
	if !s.tunnelStore.Exists(name) {
		return EndpointSettings{}, fmt.Errorf("tunnel %q does not exist", name)
	}
	cfg, meta, err := s.tunnelStore.LoadWithMeta(name)
	if err != nil {
		return EndpointSettings{}, err
	}
	out := EndpointSettings{}
	if cfg != nil {
		out.Active = firstPeerEndpoint(cfg)
	}
	if meta != nil {
		out.Candidates = meta.EndpointCandidateList()
		out.AutoSwitch = meta.EndpointAutoSwitchEnabled()
		out.SwitchMode = meta.EndpointSwitchTiming()
	}
	return out, nil
}

// SetEndpointCandidates stores the candidate list. The tunnel's current
// endpoint is kept as the first candidate even if the caller omits it —
// dropping it would let a failover strand the tunnel on an address the user
// can no longer get back to.
//
// Values are validated as host:port, the same shape the .conf's Endpoint
// accepts, so a typo is rejected here instead of at connect time.
//
// SetEndpointCandidates 保存候选列表。即便调用方没有带上，隧道当前的 endpoint
// 也会被保留为第一个候选——丢掉它会让一次故障切换把隧道丢在一个用户再也回不去的地址上。
//
// 每个值都按 host:port 校验（与 .conf 的 Endpoint 同形），所以拼写错误在这里就被拒绝，
// 而不是等到连接时才炸。
func (s *TunnelService) SetEndpointCandidates(name string, candidates []string) error {
	if !s.tunnelStore.Exists(name) {
		return fmt.Errorf("tunnel %q does not exist", name)
	}
	cfg, _, err := s.tunnelStore.LoadWithMeta(name)
	if err != nil {
		return err
	}
	active := ""
	if cfg != nil {
		active = firstPeerEndpoint(cfg)
	}

	list := make([]string, 0, len(candidates)+1)
	if active != "" {
		list = append(list, active)
	}
	for _, raw := range candidates {
		v := strings.TrimSpace(raw)
		if v == "" || v == active {
			continue
		}
		if err := validateEndpointCandidate(v); err != nil {
			return err
		}
		list = append(list, v)
	}

	return s.tunnelStore.UpdateMeta(name, func(meta *storage.TunnelMeta) {
		meta.SetEndpointCandidates(list)
	})
}

// SetEndpointAutoSwitch arms or disarms automatic candidate selection.
// Switching it off never moves the active endpoint: the tunnel keeps using
// whatever address is in the .conf until the user changes it by hand.
// SetEndpointAutoSwitch 打开或关闭候选自动选择。关掉它绝不会挪动生效地址：
// 隧道会继续用 .conf 里那个地址，直到用户手动修改。
func (s *TunnelService) SetEndpointAutoSwitch(name string, enabled bool) error {
	if !s.tunnelStore.Exists(name) {
		return fmt.Errorf("tunnel %q does not exist", name)
	}
	return s.tunnelStore.UpdateMeta(name, func(meta *storage.TunnelMeta) {
		meta.EndpointAutoSwitch = enabled
	})
}

// SetEndpointSwitchMode selects WHEN automatic switching may happen, as one
// actionable choice: "off" disarms switching entirely, "reconnect" arms it
// for reconnect-time evaluation only (the historical behaviour), and
// "dynamic" additionally lets the helper switch proactively while the
// tunnel is connected. Switching mode off never moves the active endpoint.
// SetEndpointSwitchMode 选择自动切换的发生时机，作为一个可操作选项："off" 完全
// 关闭切换；"reconnect" 仅在重连时评估（历史行为）；"dynamic" 额外允许 helper
// 在隧道连接期间主动切换。切换为 off 绝不会挪动生效地址。
func (s *TunnelService) SetEndpointSwitchMode(name, mode string) error {
	if !s.tunnelStore.Exists(name) {
		return fmt.Errorf("tunnel %q does not exist", name)
	}
	return s.tunnelStore.UpdateMeta(name, func(meta *storage.TunnelMeta) {
		switch mode {
		case storage.EndpointSwitchDynamic:
			meta.EndpointAutoSwitch = true
			meta.EndpointSwitchMode = storage.EndpointSwitchDynamic
		case storage.EndpointSwitchReconnect:
			meta.EndpointAutoSwitch = true
			meta.EndpointSwitchMode = storage.EndpointSwitchReconnect
		default:
			// "off" and anything unexpected both mean: stop switching.
			// The stored mode is cleared so a later re-arm starts from
			// the reconnect default, not from a stale choice.
			// "off" 与任何意外值都表示：停止切换。同时清空已存模式，
			// 之后重新启用时从 reconnect 默认值开始，而不是残留旧选择。
			meta.EndpointAutoSwitch = false
			meta.EndpointSwitchMode = ""
		}
	})
}

// MeasureEndpointCandidates pings every candidate (plus the active endpoint)
// once and reports each latency, so the editor can show WHY a candidate is
// preferred instead of asking the user to trust it.
// MeasureEndpointCandidates 对每个候选（含生效地址）各 ping 一次并回报延迟，
// 编辑器因此能展示「为什么选它」，而不是让用户盲信。
func (s *TunnelService) MeasureEndpointCandidates(name string) ([]endpoint.Measurement, error) {
	if !s.tunnelStore.Exists(name) {
		return nil, fmt.Errorf("tunnel %q does not exist", name)
	}
	cfg, meta, err := s.tunnelStore.LoadWithMeta(name)
	if err != nil {
		return nil, err
	}
	list := []string{}
	if cfg != nil {
		if active := firstPeerEndpoint(cfg); active != "" {
			list = append(list, active)
		}
	}
	if meta != nil {
		for _, c := range meta.EndpointCandidateList() {
			if len(list) > 0 && c == list[0] {
				continue // already there as the active endpoint / 已作为生效地址存在
			}
			list = append(list, c)
		}
	}
	if len(list) == 0 {
		return []endpoint.Measurement{}, nil
	}
	return endpoint.Measure(list), nil
}

// SetEndpointActive makes addr the endpoint WireGuard dials for this tunnel.
// The address is written straight into the .conf's first peer and persisted,
// so what the editor shows is always what comes up next — the .conf is the
// single source of truth for "which address is in use" (see applyEndpointChoice).
//
// If the tunnel is currently up, it is briefly brought down and back up so the
// new address actually takes effect: WireGuard has no UAPI hot-switch for a
// live endpoint, so a re-create is unavoidable. Crucially this does NOT touch
// the manual-override latch — switching the address is not "taking over" the
// tunnel from automation. The helper simply re-reads the .conf on connect, so
// the persisted address is what dials.
//
// SetEndpointActive 把 addr 设为该隧道 WireGuard 实际会拨的地址。地址直接写入
// .conf 首个 peer 并落盘——编辑器看到的永远是下次会拨的那个，因为「当前在用哪个
// 地址」的唯一真相源就是 .conf（见 applyEndpointChoice）。
//
// 若隧道当前是通的，会先短暂断开再连上，让新地址真正生效：WireGuard 没有针对
// 活动 endpoint 的 UAPI 热切换，重建连接不可避免。关键是这不会碰「手动接管」闩锁——
// 切换地址不等于把隧道从自动化手里「接管」走。helper 连接时重读 .conf，因此落盘的
// 地址就是会去拨的那个。
func (s *TunnelService) SetEndpointActive(name, addr string) error {
	if !s.tunnelStore.Exists(name) {
		return fmt.Errorf("tunnel %q does not exist", name)
	}
	cfg, err := s.tunnelStore.Load(name)
	if err != nil {
		return fmt.Errorf("loading tunnel %s: %w", name, err)
	}
	// Reject anything that isn't a valid "host:port" before writing it into
	// the .conf — a typo here would only fail later, at connect time.
	// 先拒绝任何不是合法 host:port 的值，再写进 .conf——这里的拼写错误
	// 若不及时拦下，只能等到连接时才炸。
	if err := validateEndpointCandidate(addr); err != nil {
		return err
	}
	if err := setFirstPeerEndpoint(cfg, addr); err != nil {
		return err
	}
	if err := s.tunnelStore.Save(cfg); err != nil {
		return fmt.Errorf("saving tunnel %s: %w", name, err)
	}
	slog.Info("endpoint: active set by user",
		"category", "tunnel", "tunnel", name, "to", addr, "mode", "manual_switch")

	// Apply now only if the tunnel is up. We deliberately skip the Connect/
	// Disconnect wrappers — they set the manual latch, which is exactly the
	// "takeover" this action must NOT do (per product decision: switch the
	// address, don't take the tunnel over). The bare IPC pair re-creates the
	// link with the freshly-saved .conf and leaves automation in control.
	// 仅在隧道当前连通时立即生效。刻意绕过 Connect/Disconnect 包装——它们会设置
	// 手动闩锁，而那正是本动作绝不可以做的「接管」（产品决策：只换地址，不接管）。
	// 裸 IPC 用刚落盘的 .conf 重建链路，自动化仍掌权。
	active, err := s.isActiveTunnel(name)
	if err != nil {
		// Helper unreachable — the persisted address still applies on the
		// next connect, so this is not an error.
		// helper 不可达——落盘的地址下次连接仍会生效，所以不算错误。
		slog.Warn("endpoint: cannot query active state; change applies on next connect",
			"category", "tunnel", "tunnel", name, "error", err)
		return nil
	}
	if !active {
		return nil
	}

	s.clients.MarkInflight()
	defer s.clients.UnmarkInflight()
	if err := s.callLong(ipc.MethodDisconnect, ipc.DisconnectRequest{TunnelName: name}, nil); err != nil {
		if isClientClosed(err) {
			err = s.callLong(ipc.MethodDisconnect, ipc.DisconnectRequest{TunnelName: name}, nil)
		}
		if err != nil {
			slog.Warn("endpoint: disconnect failed; address still saved",
				"category", "tunnel", "tunnel", name, "error", err)
			return err
		}
	}

	// Reload the freshly-saved config so the connect carries the new endpoint.
	// 重新加载落盘后的配置，确保 connect 带上新地址。
	cfg2, err := s.tunnelStore.Load(name)
	if err != nil {
		return fmt.Errorf("loading tunnel %s after endpoint switch: %w", name, err)
	}
	// Mirror Connect's scripting/AWG gating, but NOT its manual-on latch and
	// NOT its auto-switch re-evaluation — the user explicitly chose this
	// address, so we must not let auto-switch silently override it.
	// 照搬 Connect 的脚本/AWG 门控，但绝不照搬它的 manual-on 闩锁，也不跑它的
	// 自动切换重评估——用户是显式选了这个地址，绝不能被自动切换悄悄覆盖。
	if st, err := s.settingsStore.Load(); err == nil && st != nil {
		cfg2.EnableScripts = st.EnableWgScripts
		if cfg2.Protocol == domain.ProtocolAmneziaWG && !st.EnableAWG {
			return fmt.Errorf("%s — enable it in Settings → Advanced to connect this tunnel", domain.ErrAWGDisabled)
		}
	}
	if err := s.callLong(ipc.MethodConnect, ipc.ConnectRequest{Config: cfg2}, nil); err != nil {
		slog.Warn("endpoint: reconnect failed; tunnel left down",
			"category", "tunnel", "tunnel", name, "error", err)
		return err
	}
	return nil
}

// firstPeerEndpoint returns the endpoint of the first peer that has one.
// Multi-peer configs are left alone on purpose: candidate switching is a
// single-peer feature, and silently rewriting a second peer's endpoint would
// change a tunnel the user never asked us to touch.
// firstPeerEndpoint 返回第一个带 endpoint 的 peer 的地址。多 peer 配置刻意不处理：
// 候选切换是单 peer 特性，偷偷改写第二个 peer 的 endpoint 会改到用户没让我们动的东西。
func firstPeerEndpoint(cfg *config.WireGuardConfig) string {
	if cfg == nil {
		return ""
	}
	for _, p := range cfg.Peers {
		if strings.TrimSpace(p.Endpoint) != "" {
			return strings.TrimSpace(p.Endpoint)
		}
	}
	return ""
}

// validateEndpointCandidate checks one "host:port" candidate.
// validateEndpointCandidate 校验一个 "host:port" 候选。
func validateEndpointCandidate(v string) error {
	host, port, err := net.SplitHostPort(v)
	if err != nil {
		return fmt.Errorf("endpoint %q must be host:port (e.g. example.com:51820)", v)
	}
	if strings.TrimSpace(host) == "" {
		return fmt.Errorf("endpoint %q has an empty host", v)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("endpoint %q has an invalid port", v)
	}
	if !config.IsValidHostOrIP(host) {
		return fmt.Errorf("endpoint %q has an invalid host %q", v, host)
	}
	return nil
}

// applyEndpointChoice picks the best candidate before a connect and rewrites
// the .conf only when the winner differs from what is stored. It is a no-op
// unless the tunnel armed auto-switch AND has more than one candidate, so
// every other tunnel takes exactly the path it always did.
//
// The rewrite is persisted on purpose: the .conf is the single source of
// truth for "which address is in use", and leaving the choice only in memory
// would make a helper restart silently dial the old address again.
//
// applyEndpointChoice 在连接前挑出最佳候选，且仅当胜出者与已保存的不同时才改写
// .conf。除非隧道启用了自动切换且候选多于一个，否则本函数是空操作——其它隧道
// 走的仍是与过去完全相同的路径。
//
// 这里刻意落盘：.conf 是「当前在用哪个地址」的唯一真相源，若只把选择留在内存里，
// helper 重启后会静默地再去拨旧地址。
func (s *TunnelService) applyEndpointChoice(name string, cfg *config.WireGuardConfig) {
	meta, err := s.tunnelStore.LoadMeta(name)
	if err != nil || meta == nil || cfg == nil {
		return
	}
	// The global master switch must be on for any tunnel to switch — the
	// per-tunnel toggle is a refinement of it, never a bypass.
	// 全局总开关不开，任何隧道都不切——每隧道开关只是它的细化，绝非绕过。
	if settings, _ := s.settingsStore.Load(); settings != nil && !settings.EndpointFailoverEnabled() {
		return
	}
	candidates := meta.EndpointCandidateList()
	if !meta.EndpointAutoSwitchEnabled() || len(candidates) < 2 {
		return
	}
	active := firstPeerEndpoint(cfg)
	if active == "" {
		return
	}
	measurements := endpoint.Measure(candidates)
	opts := endpoint.DefaultOpts()
	if settings, _ := s.settingsStore.Load(); settings != nil {
		opts = endpoint.Opts{
			ThresholdMs: settings.EffectiveEndpointThresholdMs(),
			MarginPct:   settings.EffectiveEndpointMarginPct(),
		}
	}
	// Connect time is a fresh start, so the best candidate wins immediately
	// — no sustained-confirmation gate here (that only guards an in-place
	// improvement while a link is already alive, which never happens at
	// connect).
	// 连接时是全新起点，因此最佳候选立即胜出——此处不加持续确认门控
	// （那只在链路已活时「就地择优」才需要，连接时不会发生）。
	chosen, _ := endpoint.Better(measurements, active, opts)
	if chosen == "" {
		// Nothing clearly better — keep the user's address.
		return
	}
	if err := setFirstPeerEndpoint(cfg, chosen); err != nil {
		slog.Warn("endpoint switch: cannot apply", "category", "tunnel", "tunnel", name, "error", err)
		return
	}
	if err := s.tunnelStore.Save(cfg); err != nil {
		slog.Warn("endpoint switch: cannot persist", "category", "tunnel", "tunnel", name, "error", err)
		return
	}
	slog.Info("endpoint switched",
		"category", "tunnel",
		"tunnel", name,
		"from", active,
		"to", chosen,
		"reason", "connect_best")
}

// setFirstPeerEndpoint rewrites the endpoint of the first peer that has one.
// setFirstPeerEndpoint 改写第一个带 endpoint 的 peer 的地址。
func setFirstPeerEndpoint(cfg *config.WireGuardConfig, addr string) error {
	for i := range cfg.Peers {
		if strings.TrimSpace(cfg.Peers[i].Endpoint) != "" {
			cfg.Peers[i].Endpoint = addr
			return nil
		}
	}
	return fmt.Errorf("config has no peer endpoint to rewrite")
}
