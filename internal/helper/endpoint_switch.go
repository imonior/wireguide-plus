package helper

import (
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/imonior/wireguide-plus/internal/domain"
	"github.com/imonior/wireguide-plus/internal/endpoint"
	"github.com/imonior/wireguide-plus/internal/storage"
)

// minEndpointCooldown mirrors storage.MinEndpointCooldownSec as a
// time.Duration, so the helper-side re-check shares the single floor constant
// defined in the storage package. It must stay above the 180s dead-link
// action window (internal/reconnect/monitor.go): a freshly switched endpoint
// has to get a full dead-link evaluation cycle before it may switch again,
// otherwise a slow-but-alive link would churn on the same cadence as
// dead-link detection.
// minEndpointCooldown 把 storage.MinEndpointCooldownSec 以 time.Duration 形式镜像，
// 使 helper 端再校验共享 storage 包定义的单一下限常量。它必须高于 180s 的死链动作窗口
// （internal/reconnect/monitor.go）：刚切到的地址必须拿到一个完整的死链评估周期
// 才有资格再切，否则慢但活着的链会和死链检测同频抖动。
const minEndpointCooldown = time.Duration(storage.MinEndpointCooldownSec) * time.Second

// switchTracker keeps the last switch time per tunnel. Package-scoped rather
// than a Helper field because the decision is per tunnel name, not per
// Helper instance, and a restart legitimately resets it.
// switchTracker 记录每个隧道上次切换的时间。放在包级而非 Helper 字段里，
// 因为这个判断是按隧道名而非 Helper 实例的，且进程重启后重置也是合理的。
type switchTracker struct {
	mu sync.Mutex
	at map[string]time.Time
}

var endpointSwitches = &switchTracker{at: make(map[string]time.Time)}

// allow reports whether a switch is permitted right now and, when it is,
// stamps the clock.
// allow 判断此刻是否允许切换，允许时顺手打上时间戳。
func (t *switchTracker) allow(name string, cooldown time.Duration) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if prev, ok := t.at[name]; ok && time.Since(prev) < cooldown {
		return false
	}
	t.at[name] = time.Now()
	return true
}

// pendingImprovement records a proposed IMPROVEMENT switch (not a failover)
// that has not yet cleared the sustained-confirmation gate. The switch only
// commits when the same candidate is proposed again at least sustainedSec
// later, so a single transient probe cannot flip the address. Keyed by tunnel
// name; a new proposal for a different candidate replaces the old one.
// pendingImprovement 记录一条尚未通过「持续确认」门的**择优**切换提案（不含故障切换）。
// 只有当同一候选在至少 sustainedSec 之后再次被提案时才提交，从而单次瞬冲探测无法翻转地址。
// 按隧道名索引；对同一隧道提出不同候选的提案会替换旧的。
type pendingImprovement struct {
	candidate string
	at        time.Time
}

var pendingImprovements = struct {
	mu sync.Mutex
	m  map[string]pendingImprovement
}{m: make(map[string]pendingImprovement)}

// maybeSwitchEndpoint rotates a tunnel to a better endpoint candidate before
// an imminent reconnect. It is the "the link died, try the other door" half
// of failover: the other half (picking the fastest at connect time) lives in
// the GUI's connect path.
//
// It returns immediately — doing nothing at all — unless the global master
// switch is on, the tunnel has auto-switch armed, it has at least two
// candidates, and it is outside its cooldown, so tunnels that never opted in
// keep the exact behaviour they had before.
//
// maybeSwitchEndpoint 在即将重连之前把隧道轮换到更好的 endpoint 候选。它是
// failover 的「链路死了，换一扇门」那一半；另一半（连接时挑最快的）在 GUI 的连接
// 路径里。
//
// 除非全局总开关开启、隧道启用了自动切换、候选至少两个、且已过冷却期，否则本函数立刻
// 返回且什么都不做，因此未启用的隧道行为与此前完全一致。
func (h *Helper) maybeSwitchEndpoint(name string, cfg *domain.WireGuardConfig) {
	if h == nil || h.userTunnelStore == nil || cfg == nil {
		return
	}
	// The global master switch gates everything: when it is off no tunnel
	// switches, regardless of its own toggle.
	// 全局总开关管一切：关掉时任何隧道都不切，不论它自己的开关。
	settings, err := h.settingsStore.Load()
	if err != nil || settings == nil || !settings.EndpointFailoverEnabled() {
		return
	}
	meta, err := h.userTunnelStore.LoadMeta(name)
	if err != nil || meta == nil {
		return
	}
	candidates := meta.EndpointCandidateList()
	if !meta.EndpointAutoSwitchEnabled() || len(candidates) < 2 {
		return
	}
	if !endpointSwitches.allow(name, effectiveCooldown(settings)) {
		return
	}
	active := firstPeerEndpoint(cfg)
	if active == "" {
		return
	}
	measurements := endpoint.Measure(candidates)
	chosen, failover := endpoint.Better(measurements, active, endpoint.Opts{
		ThresholdMs: settings.EffectiveEndpointThresholdMs(),
		MarginPct:   settings.EffectiveEndpointMarginPct(),
	})
	if chosen == "" {
		return
	}
	if !failover {
		// Improvement only: require the better candidate to have been
		// observed across the sustained window before committing, so one
		// noisy probe does not move a live link. Record and wait.
		// 仅择优：要求备选在持续窗口内被持续观察到更优才提交，避免一次噪声探测
		// 就挪动活着的链。记录并等待。
		if !confirmImprovement(name, chosen, settings.EffectiveEndpointSustainedSec()) {
			return
		}
	}
	for i := range cfg.Peers {
		if strings.TrimSpace(cfg.Peers[i].Endpoint) == "" {
			continue
		}
		cfg.Peers[i].Endpoint = chosen
		break
	}
	// Persist: the .conf is the source of truth for "which address is in
	// use". Without this a helper restart would dial the dead address again
	// and the whole switch would have been theatre.
	// 落盘：.conf 是「当前用哪个地址」的真相源。不落盘的话，helper 重启后
	// 会再次去拨那个已经死掉的地址，整次切换就成了走过场。
	if err := h.userTunnelStore.Save(cfg); err != nil {
		slog.Warn("endpoint switch: cannot persist",
			"category", "tunnel", "tunnel", name, "error", err)
		return
	}
	slog.Info("endpoint switched",
		"category", "tunnel",
		"tunnel", name,
		"from", active,
		"to", chosen,
		"reason", map[bool]string{true: "reconnect_failover", false: "reconnect_improvement"}[failover])
}

// effectiveCooldown returns the user's cooldown, floored at minEndpointCooldown
// so it always exceeds the dead-link action window.
// effectiveCooldown 返回用户冷却，下限为 minEndpointCooldown，恒大于死链动作窗口。
func effectiveCooldown(settings *storage.Settings) time.Duration {
	if cd := settings.EffectiveEndpointCooldownSec(); cd >= int(minEndpointCooldown.Seconds()) {
		return time.Duration(cd) * time.Second
	}
	return minEndpointCooldown
}

// confirmImprovement implements the sustained-confirmation gate for an
// improvement switch. With a zero window it commits immediately; otherwise it
// commits only when the SAME candidate was proposed at least sustainedSec
// earlier. Each call also records/refreshes the current proposal.
// confirmImprovement 实现择优切换的「持续确认」门。窗口为 0 时立即提交；否则仅当
// 同一候选在至少 sustainedSec 之前被提案过才提交。每次调用也会记录/刷新当前提案。
func confirmImprovement(name, candidate string, sustainedSec int) bool {
	pendingImprovements.mu.Lock()
	defer pendingImprovements.mu.Unlock()
	prev, ok := pendingImprovements.m[name]
	now := time.Now()
	if sustainedSec <= 0 {
		delete(pendingImprovements.m, name)
		return true // no gate / 无门控
	}
	if ok && prev.candidate == candidate && now.Sub(prev.at) >= time.Duration(sustainedSec)*time.Second {
		delete(pendingImprovements.m, name)
		return true // confirmed across the window / 在窗口内持续确认
	}
	pendingImprovements.m[name] = pendingImprovement{candidate: candidate, at: now}
	return false // first sighting — wait / 首次见到，等待
}

// firstPeerEndpoint returns the endpoint of the first peer that has one.
// Multi-peer configs are left alone on purpose: candidate switching is a
// single-peer feature, and silently rewriting a second peer's endpoint would
// change a tunnel the user never asked us to touch.
// firstPeerEndpoint 返回第一个带 endpoint 的 peer 的地址。多 peer 配置刻意不处理：
// 候选切换是单 peer 特性，偷偷改写第二个 peer 的 endpoint 会改到用户没让我们动的东西。
func firstPeerEndpoint(cfg *domain.WireGuardConfig) string {
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
