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

// inCooldown is the read-only counterpart of allow: it reports whether a
// switch is currently blocked WITHOUT stamping the clock. The dynamic
// evaluation loop uses it so a periodic probe that finds nothing to do
// never starts a cooldown it cannot honour.
// inCooldown 是 allow 的只读版：只报告切换当前是否被挡，不打时间戳。
// 动态评估循环用它，保证一次什么都没做的周期探测不会启动一个自己无法兑现的冷却。
func (t *switchTracker) inCooldown(name string, cooldown time.Duration) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if prev, ok := t.at[name]; ok && time.Since(prev) < cooldown {
		return true
	}
	return false
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

// blessedSwitch hands a confirmed improvement decision from the dynamic
// evaluation loop to the reconnect path. The loop measures and waits out the
// sustained window; when the gate passes it blesses (candidate, tunnel) and
// triggers a reconnect. maybeSwitchEndpoint — running inside that reconnect
// — takes the blessing to skip the sustained gate it would otherwise fail
// (its own confirmImprovement state was just consumed by the loop), then
// re-measures: if the improvement no longer reproduces, the blessing simply
// expires unused and nothing switches.
// blessedSwitch 把动态评估循环确认过的择优决定交接给重连路径。循环负责测量并
// 等满持续窗口；门通过后对（隧道，候选）打上祝福并触发重连。运行在重连内的
// maybeSwitchEndpoint 凭祝福跳过持续门（它自己的 confirmImprovement 状态刚被
// 循环消费掉），随后仍会现场复测——若改进不再复现，祝福自然过期，什么都不会切。
type blessing struct {
	candidate string
	at        time.Time
}

// blessingTTL bounds how long a confirmed decision stays redeemable. It only
// needs to cover the gap between the loop's confirmation and the reconnect
// path's own measurement — seconds, not minutes.
// blessingTTL 限定一条已确认决定的有效期。它只需覆盖从循环确认到重连路径复测
// 之间的间隙——以秒计，而非分钟。
const blessingTTL = 90 * time.Second

var blessedSwitches = struct {
	mu sync.Mutex
	m  map[string]blessing
}{m: make(map[string]blessing)}

// blessEndpointSwitch records a confirmed improvement for name/candidate.
// blessEndpointSwitch 为 name/candidate 记录一条已确认的择优切换。
func blessEndpointSwitch(name, candidate string) {
	blessedSwitches.mu.Lock()
	defer blessedSwitches.mu.Unlock()
	blessedSwitches.m[name] = blessing{candidate: candidate, at: time.Now()}
}

// takeBlessing consumes a fresh blessing matching name/candidate. A stale or
// mismatched blessing is dropped either way.
// takeBlessing 消费一条与 name/candidate 匹配且未过期的祝福。过期或不匹配的
// 祝福无论哪种情况都会被丢弃。
func takeBlessing(name, candidate string) bool {
	blessedSwitches.mu.Lock()
	defer blessedSwitches.mu.Unlock()
	b, ok := blessedSwitches.m[name]
	if !ok {
		return false
	}
	delete(blessedSwitches.m, name)
	return b.candidate == candidate && time.Since(b.at) < blessingTTL
}

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
	// The cooldown stamp lives here — after a candidate was actually chosen
	// — so an evaluation that finds nothing to switch (the common case on a
	// healthy link) does not burn the cooldown for nothing.
	// 冷却打点放在这里——真的选出了候选之后——使一次没东西可切的评估
	// （健康链路上的常态）不会白白烧掉冷却期。
	if !endpointSwitches.allow(name, effectiveCooldown(settings)) {
		return
	}
	if !failover {
		// Improvement only. A blessing from the dynamic loop's sustained
		// confirmation redeems the gate directly; otherwise require the
		// better candidate to have been observed across the sustained
		// window before committing, so one noisy probe does not move a
		// live link.
		// 仅择优。动态循环持续确认后的祝福可直接兑换本门；否则要求更优
		// 候选在持续窗口内被持续观察到才提交，避免一次噪声探测就挪动
		// 活着的链。
		if !takeBlessing(name, chosen) &&
			!confirmImprovement(name, chosen, settings.EffectiveEndpointSustainedSec()) {
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
