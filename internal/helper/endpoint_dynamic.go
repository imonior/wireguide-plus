package helper

import (
	"log/slog"
	"time"

	"github.com/imonior/wireguide-plus/internal/endpoint"
)

// dynamicSwitchTick is how often the dynamic loop re-evaluates tunnels that
// opted into proactive switching. ICMP on a handful of candidates every 30s
// per tunnel is negligible traffic, and 30s plays well with the
// sustained-confirmation window: with the default 30s window two consecutive
// ticks are enough to confirm, and larger windows simply accumulate ticks
// until the gate opens.
// dynamicSwitchTick 是动态循环对选择了主动切换的隧道的重评估周期。每隧道每 30s
// 对几个候选做一次 ICMP，流量可忽略；30s 与持续确认窗口也配合良好：默认 30s
// 窗口下连续两拍即可确认，更大的窗口则累积到门开为止。
const dynamicSwitchTick = 30 * time.Second

// endpointDynamicLoop periodically re-evaluates dynamic-mode tunnels and
// triggers a controlled reconnect when the improvement rule has held across
// the sustained window. It runs for the helper's lifetime and exits when
// h.done is closed during cleanup.
//
// Scope note: the loop deliberately handles IMPROVEMENT only. A connected
// tunnel whose endpoint stopped answering ICMP is the health loop's
// jurisdiction — handshake age is the authoritative dead-link signal (ICMP
// can be deprioritized or blocked on a perfectly working tunnel), and the
// health loop's reconnect path already performs the failover switch. Two
// aggressive detectors on the same signal would only race each other.
//
// endpointDynamicLoop 周期重评估动态模式的隧道，在改进规则于持续窗口内成立时
// 触发一次受控重连。它随 helper 生命周期运行，cleanup 关闭 h.done 时退出。
//
// 范围说明：本循环刻意只处理「择优」。已连接隧道的 endpoint 不回 ICMP 属于
// 健康循环的管辖——握手年龄才是死链的权威信号（完全正常的隧道也可能被 ICMP
// 降级或屏蔽），且健康循环的重连路径本身就会做故障切换。两个激进探测器盯着
// 同一个信号只会互相竞争。
func (h *Helper) endpointDynamicLoop() {
	ticker := time.NewTicker(dynamicSwitchTick)
	defer ticker.Stop()
	for {
		select {
		case <-h.done:
			return
		case <-ticker.C:
			h.evaluateDynamicSwitches()
		}
	}
}

// evaluateDynamicSwitches runs one evaluation pass over every connected
// tunnel. Each tunnel is independently gated: global master switch, its own
// dynamic mode, candidate count, cooldown, and the sustained-confirmation
// gate. A tunnel that passes all of them gets ONE reconnect trigger; the
// reconnect path re-measures and only then moves the address, so the final
// decision is always made against a fresh measurement.
// evaluateDynamicSwitches 对所有已连接隧道执行一轮评估。每条隧道独立过闸：
// 全局总开关、自身的动态模式、候选数量、冷却、持续确认门。全过的隧道只触发
// 一次重连；重连路径会现场复测，之后才真正挪动地址——最终决定永远基于
// 新鲜的测量。
func (h *Helper) evaluateDynamicSwitches() {
	settings, err := h.settingsStore.Load()
	if err != nil || settings == nil || !settings.EndpointFailoverEnabled() {
		return
	}
	h.mu.Lock()
	cfgs := h.copyActiveCfgs()
	h.mu.Unlock()
	cooldown := effectiveCooldown(settings)
	sustained := settings.EffectiveEndpointSustainedSec()
	opts := endpoint.Opts{
		ThresholdMs: settings.EffectiveEndpointThresholdMs(),
		MarginPct:   settings.EffectiveEndpointMarginPct(),
	}
	for name, cfg := range cfgs {
		meta, err := h.userTunnelStore.LoadMeta(name)
		if err != nil || meta == nil || !meta.EndpointSwitchDynamic() {
			continue
		}
		candidates := meta.EndpointCandidateList()
		if len(candidates) < 2 {
			continue
		}
		if endpointSwitches.inCooldown(name, cooldown) {
			continue
		}
		active := firstPeerEndpoint(cfg)
		if active == "" {
			continue
		}
		measurements := endpoint.Measure(candidates)
		chosen, failover := endpoint.Better(measurements, active, opts)
		if chosen == "" || failover {
			// Nothing clearly better, or the current endpoint looks down:
			// both leave the tunnel untouched here (see the scope note
			// above for the failover case).
			// 没有明确更优的候选，或当前 endpoint 看似不通：两种情况都
			// 不在这里动隧道（failover 情形见上面的范围说明）。
			continue
		}
		if !confirmImprovement(name, chosen, sustained) {
			continue
		}
		// Hand the confirmed decision to the reconnect path and let it run
		// the standard gated sequence. The blessing is what lets
		// maybeSwitchEndpoint commit immediately on its re-measurement.
		// 把已确认的决定交接给重连路径，走标准的受控流程。祝福让
		// maybeSwitchEndpoint 在复测后立即提交。
		blessEndpointSwitch(name, chosen)
		slog.Info("dynamic endpoint evaluation: improvement confirmed, reconnecting",
			"category", "tunnel",
			"tunnel", name,
			"from", active,
			"to", chosen)
		h.monitor.TriggerTunnelReconnect(name)
	}
}
