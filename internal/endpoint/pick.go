// Package endpoint selects among several peer endpoints for one tunnel.
//
// A WireGuard peer has exactly one endpoint in its .conf. This package adds
// an OPTIONAL candidate list: several host:port addresses the same peer may
// be reached at, from which the app picks the best one at connect time and
// rotates away from one that has gone dead. With no candidate list every
// function here is a no-op, so tunnels that never opted in behave exactly as
// before.
//
// package endpoint 为一个隧道在多个 peer endpoint 之间做选择。
//
// WireGuard peer 在 .conf 里只有一个 endpoint。本包提供一份**可选**的候选列表：
// 同一个 peer 可能位于多个 host:port，应用据此在连接时挑最优、并在某个地址死掉后
// 轮换到下一个。没有候选列表时这里的所有函数都是空操作，未启用的隧道行为与之前
// 完全一致。
package endpoint

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/imonior/wireguide-plus/internal/diag"
)

// Measurement is the outcome of probing one endpoint candidate.
// Measurement 是对一个 endpoint 候选探测的结果。
type Measurement struct {
	Address string `json:"address"`
	// ResolvedIP is what the endpoint's HOST resolves to right now: the
	// public address behind a DDNS name. Literal-IP endpoints resolve to
	// themselves, so the column is populated for every row and an empty
	// value means one thing only — the lookup failed.
	// ResolvedIP 是 endpoint 的 HOST 当前解析到的地址，即 DDNS 名字背后的公网
	// 地址。字面 IP 的 endpoint 解析为自身，因此每一行都有值，空值只表示一件事：
	// 解析失败。
	ResolvedIP string  `json:"resolved_ip,omitempty"`
	Reachable  bool    `json:"reachable"`
	LatencyMs  float64 `json:"latency_ms"`
	Error      string  `json:"error,omitempty"`
}

// Switch tuning knobs. A switch is disruptive enough (it rewrites the
// endpoint and reconnects) that it must be worth it: the current address has
// to be clearly bad, and the alternative clearly better, or nothing moves.
// 切换阈值：切换本身有代价（改写 endpoint 并重连），所以必须「值得」——
// 当前地址要明显更差、备选要明显更好，否则不动。
const (
	// DefaultThresholdMs is the latency above which the current candidate is
	// even considered for replacement, used when a caller does not supply
	// its own Opts.
	// DefaultThresholdMs 是当前候选延迟超过该值才被考虑替换的默认门槛，
	// 用于调用方未提供 Opts 时。
	DefaultThresholdMs = 150
	// DefaultMarginPct is how much faster an alternative must be (relative
	// to the current latency) before it replaces it, by default.
	// DefaultMarginPct 是备选相对当前延迟必须「快出多少个百分点」才替换的默认值。
	DefaultMarginPct = 30
)

// Opts carries the switching thresholds. The zero value is NOT valid: callers
// must fill it (conveniently from storage.Settings' Effective* accessors), so
// a missing threshold can never silently mean "switch on every probe".
// Opts 携带切换阈值。零值不可用：调用方必须填写（通常直接取自 storage.Settings 的
// Effective* 访问器），这样「漏填阈值」绝不会静默变成「每次探测都切」。
type Opts struct {
	// ThresholdMs: a current candidate slower than this is considered.
	// ThresholdMs：慢于该值的当前候选才会被考虑替换。
	ThresholdMs int
	// MarginPct: an alternative must be at least this much faster (relative
	// to the current latency) to replace it.
	// MarginPct：备选必须相对当前延迟快出至少这么多百分点才替换。
	MarginPct int
}

// DefaultOpts returns the package defaults.
// DefaultOpts 返回包级默认值。
func DefaultOpts() Opts {
	return Opts{ThresholdMs: DefaultThresholdMs, MarginPct: DefaultMarginPct}
}

// Host extracts the host part of a host:port endpoint. An address without a
// port (or an unparsable one) is returned unchanged — callers only need it
// for an ICMP probe, which takes a host anyway.
// Host 取出 host:port 里的 host 部分。没有端口（或无法解析）时原样返回——
// 调用方只是拿它去做 ICMP 探测，而 ICMP 本来就只需要主机部分。
func Host(addr string) string {
	if host, _, err := net.SplitHostPort(strings.TrimSpace(addr)); err == nil {
		return host
	}
	return strings.TrimSpace(addr)
}

// resolveTimeout bounds a name lookup inside Measure. The resolver has no
// timeout of its own and Measure is called straight from the editor, so an
// unbound lookup on a dead DNS server would freeze the whole candidate panel
// on the system resolver's schedule (tens of seconds) instead of failing
// fast like the ICMP probe beside it.
// resolveTimeout 为 Measure 内的名字解析设上限。解析器自身没有超时，而 Measure
// 是由编辑器直接调用的，因此在 DNS 服务器不可达时，无上限的解析会把整个候选面板
// 冻结在系统解析器的节奏上（几十秒），而不是像旁边的 ICMP 探测那样快速失败。
const resolveTimeout = 3 * time.Second

// ResolveHost returns the address an endpoint host currently points at.
// A literal IP resolves to itself; a name is looked up and its first IPv4
// address returned (falling back to the first answer when there is no A
// record). "" means the lookup failed — the editor renders that as an em
// dash rather than a stale or guessed value.
// ResolveHost 返回 endpoint 主机当前指向的地址。字面 IP 解析为自身；域名则查
// 询并返回其第一个 IPv4 地址（没有 A 记录时退回第一个应答）。"" 表示解析失败——
// 编辑器把它渲染成破折号，而不是一个陈旧或猜测的值。
func ResolveHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	ctx, cancel := context.WithTimeout(context.Background(), resolveTimeout)
	defer cancel()
	ips, err := (&net.Resolver{}).LookupIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return ""
	}
	// IPv4 first: a WireGuard endpoint may have both, and the v4 address is
	// the one users recognise as "the public IP" of a host.
	// 优先 IPv4：WireGuard endpoint 可能同时有两类地址，而 v4 才是用户认作
	// 「这台主机的公网 IP」的那个。
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil {
			return v4.String()
		}
	}
	return ips[0].String()
}

// Normalize trims, drops empties and removes duplicates while preserving
// order. Order is the user's preference order and is what breaks ties, so it
// must survive normalization.
// Normalize 去空白、丢空项、去重，同时保持原有顺序。顺序即用户的偏好顺序，
// 也是平局时的裁决依据，因此必须在归一化后保留。
func Normalize(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, raw := range in {
		v := strings.TrimSpace(raw)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// Measure probes every candidate in parallel and returns one measurement per
// candidate, in the input order.
//
// ICMP has no concept of a port, so only the HOST is pinged: two candidates
// that differ only by port are indistinguishable here. That is a real
// limitation, not an oversight — the failover path therefore falls back to
// the user's ordering when the probe cannot tell them apart.
//
// Measure 并行探测每个候选，按输入顺序返回每个候选一条结果。
//
// ICMP 没有端口概念，因此只 ping HOST：仅端口不同的两个候选在这里无法区分。
// 这是真实的局限而非疏忽——故障切换路径因此在探测无法分辨时回退到用户的排序。
func Measure(candidates []string) []Measurement {
	list := Normalize(candidates)
	out := make([]Measurement, len(list))
	var wg sync.WaitGroup
	for i, addr := range list {
		wg.Add(1)
		go func(idx int, addr string) {
			defer wg.Done()
			m := Measurement{Address: addr, ResolvedIP: ResolveHost(Host(addr))}
			res := diag.PingEndpoint(Host(addr))
			if res == nil {
				m.Error = "probe failed"
				out[idx] = m
				return
			}
			m.Reachable = res.Reachable
			m.LatencyMs = res.LatencyMs
			m.Error = res.Error
			out[idx] = m
		}(i, addr)
	}
	wg.Wait()
	return out
}

// Best returns the reachable candidate with the lowest latency, or "" when
// none answered. Ties break on input order (the user's preference).
// Best 返回延迟最低的可达候选；若全都无应答则返回 ""。平局按输入顺序（用户偏好）裁决。
func Best(ms []Measurement) string {
	best := ""
	var bestLatency float64
	for _, m := range ms {
		if !m.Reachable || m.LatencyMs <= 0 {
			continue
		}
		if best == "" || m.LatencyMs < bestLatency {
			best, bestLatency = m.Address, m.LatencyMs
		}
	}
	return best
}

// Better reports a candidate that should replace `current`:
//   - current is unreachable and something else answers (failover), or
//   - current is slower than Opts.ThresholdMs and the alternative is faster
//     by at least Opts.MarginPct (improvement).
//
// It returns the chosen address and whether the choice is a failover (current
// down). The caller uses the failover flag to decide whether the switch needs
// the sustained-confirmation gate (improvement does; failover never does —
// a dead address must move now). It returns ("", false) when nothing is
// clearly better, which is the common case — flapping between two roughly
// equal addresses costs more than it saves.
//
// Better 给出应当替换 current 的候选：
//   - current 不可达而有别的候选有应答（故障切换），或
//   - current 慢于 Opts.ThresholdMs 且备选快至少 Opts.MarginPct（择优）。
//
// 返回被选中的地址以及该选择是否为故障切换（当前已死）。调用方用 failover
// 标志决定本次切换是否要过「持续确认」门控（择优要过；故障切换不过——死地址必须立刻挪）。
// 没有明显更优者时返回 ("", false)——这是常态：在两个差不多的地址之间来回切换，
// 代价大于收益。
func Better(ms []Measurement, current string, opts Opts) (string, bool) {
	threshold := opts.ThresholdMs
	if threshold < 1 {
		threshold = DefaultThresholdMs
	}
	margin := opts.MarginPct
	if margin < 1 {
		margin = DefaultMarginPct
	}
	current = strings.TrimSpace(current)
	var currentMs *Measurement
	var fallback *Measurement
	for i := range ms {
		if ms[i].Address == current {
			currentMs = &ms[i]
		} else if ms[i].Reachable && ms[i].LatencyMs > 0 && (fallback == nil || ms[i].LatencyMs < fallback.LatencyMs) {
			fallback = &ms[i]
		}
	}
	if fallback == nil {
		return "", false // nothing else answers — stay put / 没有别的候选应答，保持不动
	}
	if currentMs == nil || !currentMs.Reachable || currentMs.LatencyMs <= 0 {
		return fallback.Address, true // current is down → failover / 当前已死 → 故障切换
	}
	if currentMs.LatencyMs < float64(threshold) {
		return "", false // current is good enough → never churn / 当前够好 → 不折腾
	}
	if fallback.LatencyMs >= currentMs.LatencyMs*(1-float64(margin)/100.0) {
		return "", false // not better by the margin → keep / 优势未达阈值 → 保持
	}
	return fallback.Address, false // improvement, not failover / 择优而非故障
}
