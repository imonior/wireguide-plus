package helper

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/imonior/wireguide-plus/internal/diag"
	"github.com/imonior/wireguide-plus/internal/domain"
	"github.com/imonior/wireguide-plus/internal/ipc"
)

// defaultFullTunnelProbes are the addresses tried, in order, when a tunnel
// holds the default route (AllowedIPs contains 0.0.0.0/0 or ::/0). A full
// tunnel is supposed to carry *everything*, so "can I reach the internet
// through it" is the meaningful question — and it is answered by public
// anycast addresses, not by the peer endpoint (which is exempt from the
// tunnel route, so pinging it measures the underlay, not the tunnel).
//
// Two addresses, one global and one mainland-China: either can be filtered
// by a local network, so both must fail before the tunnel is declared
// unprobed and we fall back to the endpoint.
var defaultFullTunnelProbes = []string{"8.8.8.8", "223.5.5.5"}

// isFullTunnel reports whether any peer claims the default route.
func isFullTunnel(cfg *domain.WireGuardConfig) bool {
	if cfg == nil {
		return false
	}
	for _, peer := range cfg.Peers {
		for _, allowed := range peer.AllowedIPs {
			if allowed == "0.0.0.0/0" || allowed == "::/0" {
				return true
			}
		}
	}
	return false
}

// probeCandidate is one address to ping, tagged with what it represents.
type probeCandidate struct {
	target string
	kind   string // "public" | "endpoint" | "custom"
	// slot is the editor row this candidate came from, or -1 for the
	// endpoint. The UI uses it to put the live resolved address beside the
	// box the user typed it in; matching by target name would collide as
	// soon as two slots hold the same value.
	slot int
}

// probeCandidatesForTunnel returns every address worth pinging for this
// tunnel. All of them are probed on every cycle — the caller no longer stops
// at the first reply — because each row answers a different question:
//
//   - public probes (full tunnels only): does the tunnel actually carry
//     traffic? These are the addresses routed *through* the tunnel.
//   - the peer endpoint: is the peer itself reachable? It is the one address
//     guaranteed to be up whenever the tunnel is up.
//   - user-pinned targets (slots 2/3): is that specific host answering?
//
// `targets` is the four-slot list from the tunnel's sidecar (see
// storage.ProbeTargets). Slots 0/1 are positional overrides for the public
// probes and are read only on a full tunnel: on a split tunnel those
// addresses are not routed through the tunnel at all, so probing them would
// measure the wrong path — the editor hides them there for the same reason.
// An empty slot 0/1 keeps the compiled-in default; the defaults are not
// stored per tunnel because they are a policy of this build, not a choice
// the user made.
//
// The old "first /32 host from AllowedIPs" branch is gone: that host is
// frequently offline (a laptop, a spun-down NAS), which made healthy tunnels
// look dead.
//
// Order is display order, not priority: public probes first, then the
// endpoint, then the user-pinned slots in order.
func probeCandidatesForTunnel(fullTunnel bool, endpoint string, targets []string) []probeCandidate {
	candidates := make([]probeCandidate, 0, len(targets)+2)
	if fullTunnel {
		for i, def := range defaultFullTunnelProbes {
			target := def
			if i < len(targets) && strings.TrimSpace(targets[i]) != "" {
				target = strings.TrimSpace(targets[i])
			}
			candidates = append(candidates, probeCandidate{target: target, kind: "public", slot: i})
		}
	}
	endpoint = strings.TrimSpace(endpoint)
	if endpoint != "" {
		candidates = append(candidates, probeCandidate{target: endpoint, kind: "endpoint", slot: -1})
	}
	// Slots 2/3 are free-form and available on every tunnel type. Slot 0/1
	// are not repeated here: on a full tunnel they are already the public
	// rows, and on a split tunnel they are not routable.
	for i := 2; i < len(targets); i++ {
		target := strings.TrimSpace(targets[i])
		if target == "" {
			continue
		}
		if isDuplicateProbe(candidates, target) {
			continue
		}
		candidates = append(candidates, probeCandidate{target: target, kind: "custom", slot: i})
	}
	if len(candidates) == 0 {
		return nil
	}
	return candidates
}

// isDuplicateProbe reports whether target is already probed, so a user who
// pinned the endpoint (or a public probe) sees one row instead of two
// identical ones.
func isDuplicateProbe(candidates []probeCandidate, target string) bool {
	for _, c := range candidates {
		if c.target == target {
			return true
		}
	}
	return false
}

// probeCoverage classifies whether addr is actually routed through this
// tunnel: "inside", "outside", or "" when the question cannot be answered
// (no config, or addr is not a literal address).
//
// A full tunnel routes everything, so every address is "inside" by
// definition. On a split tunnel only AllowedIPs are routed; an address
// outside that set leaves through the physical interface, so its RTT
// describes the plain internet path and says nothing about this tunnel.
func probeCoverage(cfg *domain.WireGuardConfig, addr string) string {
	if cfg == nil {
		return ""
	}
	if isFullTunnel(cfg) {
		return "inside"
	}
	ip, err := netip.ParseAddr(addr)
	if err != nil {
		return ""
	}
	for _, peer := range cfg.Peers {
		for _, allowed := range peer.AllowedIPs {
			prefix, perr := netip.ParsePrefix(allowed)
			if perr != nil {
				continue
			}
			if prefix.Contains(ip) {
				return "inside"
			}
		}
	}
	return "outside"
}

// resolvedIPForProbe returns target as-is when it is already a literal IP,
// otherwise its first resolved address (or "" when DNS has no answer).
//
// Shown next to the target so the user can see what a hostname actually
// points at — without it, "nas.example.com: 180 ms" hides which address was
// measured, and a DDNS peer can silently move.
func resolvedIPForProbe(target string) string {
	if target == "" {
		return ""
	}
	if _, err := netip.ParseAddr(target); err == nil {
		return ""
	}
	host := target
	if h, _, err := net.SplitHostPort(target); err == nil && h != "" {
		host = h
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return ""
	}
	addrs, err := net.LookupHost(host)
	if err != nil || len(addrs) == 0 {
		return ""
	}
	return addrs[0]
}

// probeStateString renders the overall probe outcome for the UI.
func probeStateString(p latencyProbe) string {
	if p.at.IsZero() {
		return "" // not measured yet
	}
	if p.reachable {
		return "ok"
	}
	return "unreachable"
}

// statusDTO returns the current connection status for broadcast.
// Pulled from manager.AllStatuses() in a single call — the previous
// version queried the primary tunnel via Status() AND again via
// AllStatuses(), doubling UAPI round-trips on every 1Hz tick for the
// common single-tunnel case.
func (h *Helper) statusDTO() ipc.ConnectionStatus {
	allStats := h.manager.AllStatuses()
	if len(allStats) == 0 {
		// Mirror the previous Status()==nil behavior with an empty
		// disconnected struct so subscribers don't spuriously see
		// "tunnel disappeared" events on idle helpers.
		return ipc.ConnectionStatus{State: domain.StateDisconnected}
	}

	// Pick the primary the same way manager.Status() did: prefer
	// the first connected tunnel; otherwise the first non-nil entry.
	var primary *domain.ConnectionStatus
	for _, ts := range allStats {
		if ts == nil {
			continue
		}
		if ts.State == domain.StateConnected {
			primary = ts
			break
		}
		if primary == nil {
			primary = ts
		}
	}
	if primary == nil {
		return ipc.ConnectionStatus{State: domain.StateDisconnected}
	}
	result := *primary
	result.ActiveTunnels = h.manager.ActiveTunnels()
	// Established = setup actually finished. UI entities that assert "connected"
	// to the user read this one, so a tunnel still negotiating (DNS failing,
	// handshake pending) cannot be advertised as up and then silently vanish.
	result.EstablishedTunnels = h.manager.EstablishedTunnels()

	// Snapshot the latency cache once per call so we don't take the
	// lock per-tunnel inside the loop. The probe outcome (which address
	// answered) rides along — the UI needs it to tell "no measurement
	// yet" apart from "nothing behind this tunnel answers".
	h.latencyMu.Lock()
	latencies := make(map[string]float64, len(h.latencyByTunnel))
	for k, v := range h.latencyByTunnel {
		latencies[k] = v
	}
	probes := make(map[string]latencyProbe, len(h.latencyProbeByTunnel))
	for k, v := range h.latencyProbeByTunnel {
		probes[k] = v
	}
	h.latencyMu.Unlock()

	if lat, ok := latencies[result.TunnelName]; ok {
		result.LatencyMs = lat
	}
	if p, ok := probes[result.TunnelName]; ok {
		result.LatencyProbeTarget = p.target
		result.LatencyProbeState = probeStateString(p)
		result.LatencyProbeResults = p.results
	}

	// Include complete per-tunnel status. The same DTO backs both the
	// frontend's selected-tunnel statistics and `ctl status --json`; copying
	// only name/state/handshake silently zeroed interface, duration, traffic,
	// and endpoint whenever more than one tunnel was active.
	// Pre-allocate to avoid the latent-bug of `append` aliasing a
	// slice on the manager-returned struct.
	if len(allStats) > 1 {
		result.Tunnels = make([]domain.ConnectionStatus, 0, len(allStats))
		for _, ts := range allStats {
			if ts == nil {
				continue
			}
			sub := *ts
			sub.ActiveTunnels = nil
			sub.Tunnels = nil
			if lat, ok := latencies[ts.TunnelName]; ok {
				sub.LatencyMs = lat
			}
			if p, ok := probes[ts.TunnelName]; ok {
				sub.LatencyProbeTarget = p.target
				sub.LatencyProbeState = probeStateString(p)
				sub.LatencyProbeResults = p.results
			}
			result.Tunnels = append(result.Tunnels, sub)
		}
	}
	return result
}

// latencyLoop probes each connected tunnel's endpoint with an ICMP
// ping every 30 seconds and stores the result in latencyByTunnel.
// Runs in a goroutine supervised by goSafe — panics are logged and
// restarted up to maxRestarts times.
//
// Each measurement uses diag.PingEndpoint which has its own 15s ctx
// timeout. Tunnel pings dispatch in parallel (one goroutine per
// connected tunnel) so total wall time is max(per-tunnel) instead of
// sum — a row of N hung tunnels no longer multiplies the loop by N
// and risks chewing through the 30s tick.
func (h *Helper) latencyLoop() {
	const tickInterval = 30 * time.Second
	// With no GUI subscribed, nobody consumes 30s-fresh values — only an
	// occasional `ctl status` reads the cache. The helper deliberately
	// outlives the GUI while a tunnel is up (wg-quick semantics), so
	// without this the headless state would spawn ping subprocesses every
	// 30s forever. Probes continue at a slow cadence rather than stopping
	// so ctl status latency stays approximately fresh.
	const idleTickInterval = 5 * time.Minute
	// Sleep briefly on startup so we don't ping immediately during
	// helper boot, when the tunnel state is still settling.
	select {
	case <-h.done:
		return
	case <-time.After(5 * time.Second):
	}

	for {
		h.measureLatencies()

		interval := tickInterval
		if !h.server.HasSubscribers() {
			interval = idleTickInterval
		}
		select {
		case <-h.done:
			return
		case <-time.After(interval):
		}
	}
}

// latencyPoolSize bounds parallel ICMP probes per tick. With N connected
// tunnels we used to spawn N goroutines every 30s; on a flaky network with
// 15s per-probe timeouts this could keep dozens of goroutines alive at peak
// for very little wall-clock benefit. A fixed pool of 3 covers the typical
// "two tunnels, both responsive" case and degrades gracefully when many
// tunnels hang on ICMP.
const latencyPoolSize = 3

type latencyTask struct {
	tunnelName string
	endpoint   string
	fullTunnel bool
	// targets is the tunnel's four probe-target slots (see
	// storage.TunnelMeta.ProbeTargets), read once per cycle so an edit made
	// in the GUI takes effect on the next tick without an IPC push.
	targets []string
	// cfg is the tunnel's config at collection time. Kept so the coverage
	// check (is the probed address actually routed through this tunnel?)
	// does not need a second locked lookup per candidate.
	cfg *domain.WireGuardConfig
}

// latencyProbe is the outcome of one probe cycle for one tunnel.
type latencyProbe struct {
	target    string // candidate that produced the headline LatencyMs
	reachable bool   // did ANY candidate answer
	at        time.Time
	results   []domain.ProbeResult // per-candidate breakdown, display order
}

// measureLatencies pings each connected tunnel's preferred latency target
// and updates the cache. Failures store 0, which the frontend renders as "—".
func (h *Helper) measureLatencies() {
	statuses := h.manager.AllStatuses()

	// Collect tasks first so we know how many workers are needed.
	var tasks []latencyTask
	for _, s := range statuses {
		if s == nil || s.State != domain.StateConnected || s.Endpoint == "" {
			continue
		}
		latencyProbeTargets := []string(nil)
		if h.userTunnelStore != nil {
			if meta, err := h.userTunnelStore.LoadMeta(s.TunnelName); err == nil && meta != nil {
				latencyProbeTargets = meta.ProbeTargets()
			}
		}
		// Full-tunnel detection decides which default addresses to probe,
		// so it needs the config. Read it under the same lock as before;
		// a nil config (not yet cached) falls back to split-tunnel rules.
		h.mu.Lock()
		cfg := h.activeCfgs[s.TunnelName]
		h.mu.Unlock()
		tasks = append(tasks, latencyTask{
			tunnelName: s.TunnelName,
			endpoint:   s.Endpoint,
			fullTunnel: isFullTunnel(cfg),
			targets:    latencyProbeTargets,
			cfg:        cfg,
		})
	}
	if len(tasks) == 0 {
		return
	}

	taskCh := make(chan latencyTask, len(tasks))
	for _, t := range tasks {
		taskCh <- t
	}
	close(taskCh)

	workers := latencyPoolSize
	if len(tasks) < workers {
		workers = len(tasks)
	}

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			// defer wg.Done() FIRST so a panic in runOneLatencyProbe
			// doesn't leak the WaitGroup counter and deadlock the
			// 30-second latency loop. The inner func() + recover()
			// catches per-task panics without killing the worker —
			// next probe in the same worker still runs.
			defer wg.Done()
			for t := range taskCh {
				func(t latencyTask) {
					defer func() {
						if r := recover(); r != nil {
							slog.Warn("latency probe panic recovered",
								"tunnel", t.tunnelName, "panic", r)
						}
					}()
					h.runOneLatencyProbe(t)
				}(t)
			}
		}()
	}
	wg.Wait()
}

func (h *Helper) runOneLatencyProbe(t latencyTask) {
	candidates := probeCandidatesForTunnel(t.fullTunnel, t.endpoint, t.targets)
	if len(candidates) == 0 {
		return
	}

	// Probe every candidate concurrently. Sequentially, a full tunnel with
	// two silent public probes plus a filtered endpoint would burn three
	// timeouts (tens of seconds) inside a 30 s cycle; in parallel the cycle
	// costs about as much as the slowest single probe.
	results := make([]domain.ProbeResult, len(candidates))
	var wg sync.WaitGroup
	for i, c := range candidates {
		wg.Add(1)
		go func(i int, c probeCandidate) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("latency probe panic recovered",
						"tunnel", t.tunnelName, "target", c.target, "panic", r)
				}
			}()
			pinged := c.target
			if host, _, err := net.SplitHostPort(c.target); err == nil && host != "" {
				pinged = host // an endpoint carries ":port"; ICMP wants the host
			}
			res := diag.PingEndpoint(pinged)
			resolved := resolvedIPForProbe(pinged)
			// The endpoint is exempt from the tunnel route by definition
			// (that is how WireGuard reaches it at all), so judging it
			// against AllowedIPs would flag every healthy tunnel. Coverage
			// is only meaningful for the targets the user actually routed
			// through the tunnel.
			coverage := ""
			if c.kind != "endpoint" {
				judged := pinged
				if resolved != "" {
					judged = resolved
				}
				coverage = probeCoverage(t.cfg, judged)
			}
			results[i] = domain.ProbeResult{
				Target:     c.target,
				ResolvedIP: resolved,
				Kind:       c.kind,
				Slot:       c.slot,
				Coverage:   coverage,
				Reachable:  res.Reachable,
				LatencyMs:  res.LatencyMs,
			}
		}(i, c)
	}
	wg.Wait()

	// Headline number: prefer what the user pinned, then the public probes
	// (they measure the through-tunnel path), then the endpoint. Keeping a
	// single headline value preserves every existing consumer — tray, CLI,
	// history — while the per-row breakdown carries the detail.
	var latency float64
	var measuredTarget string
	reachable := false
	pick := func(r domain.ProbeResult) {
		if !r.Reachable || reachable {
			return
		}
		latency = r.LatencyMs
		measuredTarget = r.Target
		reachable = true
	}
	for _, r := range results {
		if r.Kind == "custom" {
			pick(r)
		}
	}
	for _, r := range results {
		pick(r)
	}
	if !reachable {
		measuredTarget = candidates[0].target
	}

	h.latencyMu.Lock()
	h.latencyByTunnel[t.tunnelName] = latency
	if h.latencyProbeByTunnel == nil {
		h.latencyProbeByTunnel = make(map[string]latencyProbe)
	}
	h.latencyProbeByTunnel[t.tunnelName] = latencyProbe{
		target:    measuredTarget,
		reachable: reachable,
		at:        time.Now(),
		results:   results,
	}
	h.latencyMu.Unlock()
	// Debug, not Info: this fires per connected tunnel every 30s, and
	// launchd appends StandardOutPath forever with no rotation — at Info
	// it was 95.7% of a 7.7 MB helper log (33,720 of 35,240 lines over
	// four months). Nothing is lost by demoting it: the same value is
	// already broadcast in the status event, rendered in the UI and
	// readable via `ctl status`. The log level is runtime-mutable, so
	// anyone debugging a latency problem can turn it back on live with
	// `wireguideplus ctl set loglevel debug` (or the Settings UI).
	slog.Debug("endpoint latency measured",
		"tunnel", t.tunnelName, "target", measuredTarget,
		"configured_targets", t.targets,
		"candidates", len(candidates),
		"reachable", reachable, "latency_ms", latency)
}

// eventLoop broadcasts status updates to subscribed GUIs on change. Change
// detection is done by JSON round-trip compare (robust against field swaps).
//
// The loop short-circuits when no GUI is subscribed — building the status
// DTO involves a per-tunnel UAPI round trip to wireguard-go, so doing it
// once per second with no listener is pure waste. The next Subscribe call
// triggers a fresh refreshStatus on the GUI side, so the missed ticks
// don't leave the UI stale.
func (h *Helper) eventLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var lastJSON []byte
	for {
		select {
		case <-h.done:
			return
		case <-ticker.C:
			// Skip if nobody's listening — saves the wgctrl syscalls + JSON marshal.
			if !h.server.HasSubscribers() {
				lastJSON = nil // force next broadcast (post-resubscribe) to fire
				continue
			}
			status := h.statusDTO()
			currentJSON, err := json.Marshal(status)
			if err != nil {
				continue
			}
			if !bytes.Equal(lastJSON, currentJSON) {
				lastJSON = currentJSON
				// Pass the bytes the diff already produced — RawMessage
				// embeds as-is, avoiding a second marshal of the same
				// struct inside Broadcast at 1 Hz.
				h.server.Broadcast(ipc.EventStatus, json.RawMessage(currentJSON))
			}
		}
	}
}
