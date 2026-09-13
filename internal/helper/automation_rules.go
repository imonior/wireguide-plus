package helper

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/imonior/wireguide-plus/internal/ipc"
	"github.com/imonior/wireguide-plus/internal/storage"
	"github.com/imonior/wireguide-plus/internal/wifi"
)

// loadUserSettings reads the user's settings.json directly. Reading
// fresh on every SSID transition (instead of caching + IPC sync from
// the GUI) means rule edits made in Settings take effect on the next
// network change without any explicit push, and there's no "in-memory
// state diverged from disk" failure mode.
func (h *Helper) loadUserSettings() (*storage.Settings, error) {
	if h.userAppSupport == "" {
		return nil, fmt.Errorf("user app-support dir not derived")
	}
	path := filepath.Join(h.userAppSupport, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return storage.DefaultSettings(), nil
		}
		return nil, err
	}
	s := storage.DefaultSettings()
	if err := json.Unmarshal(data, s); err != nil {
		return nil, err
	}
	return s, nil
}

// loadTunnelBinding reads the per-tunnel physical-egress binding from the
// meta sidecar in the user dir (the helper keeps no TunnelStore). Returns
// (0, "") when absent or unreadable — every failure mode means "auto", the
// conservative default. Mirrors the authoritative-check pattern of
// loadUserSettings so CLI/automation connects get the same binding as
// GUI-initiated ones.
func (h *Helper) loadTunnelBinding(name string) (int, string) {
	if h.userAppSupport == "" {
		return 0, ""
	}
	if err := storage.ValidateTunnelName(name); err != nil {
		return 0, ""
	}
	path := filepath.Join(h.userAppSupport, "tunnels", name+".meta.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, ""
	}
	var meta struct {
		BindIfIndex int    `json:"bind_if_index"`
		BindIfName  string `json:"bind_if_name"`
	}
	if json.Unmarshal(data, &meta) != nil {
		return 0, ""
	}
	return meta.BindIfIndex, meta.BindIfName
}

// currentNetworkContext builds the NetworkContext automation rules are
// evaluated against — the single source for both the live engine
// (reevaluateAutomation) and the read-only preview, so `wireguideplus ctl
// automation` always shows exactly what the engine would act on.
//
// SSID staleness: on macOS the SSID only arrives via GUI reports (the
// root helper can't read it). If the GUI has exited and the machine then
// moved networks, that value is stale. A gateway MAC different from the
// one stamped at report time proves the network changed since the report,
// so the SSID is treated as unknown — SSID rules stop matching rather
// than misfiring on the old network's name, while subnet/MAC rules keep
// working off fresh data. An empty stamp (no report yet, or gateway
// unknown at report time) never invalidates.
func (h *Helper) currentNetworkContext() wifi.NetworkContext {
	ssid := ""
	if h.wifiMon != nil {
		ssid = h.wifiMon.LastSSID()
	}
	gw := wifi.GatewayMAC()
	if ssid != "" && gw != "" {
		h.wifiMu.Lock()
		stamp := h.ssidStampGW
		h.wifiMu.Unlock()
		if stamp != "" && stamp != gw {
			slog.Debug("SSID considered stale: gateway changed since GUI report",
				"ssid", ssid, "stamped_gw", stamp, "current_gw", gw)
			ssid = ""
		}
	}
	return wifi.NetworkContext{
		SSID:        ssid,
		PhysicalIPs: wifi.PhysicalInterfaceIPs(),
		GatewayMAC:  gw,
		GatewayIP:   wifi.GatewayIP(),
		Interfaces:  wifi.PhysicalInterfaces(),
	}
}

// handleSSIDChange is one trigger for Automation re-evaluation: the
// Wi-Fi monitor fires it on every SSID transition. It only posts a
// coalesced request — the decision logic (and the network-context probe)
// lives in reevaluateAutomation, run by automationEvalLoop, which every
// trigger (SSID change, network change, poll, post-connect) shares.
func (h *Helper) handleSSIDChange(oldSSID, newSSID string) {
	h.requestAutomationEval("ssid-change")
}

const (
	// startupRuleCheckWindow is how long after helper start a manual
	// connect gets an immediate rule re-check.
	startupRuleCheckWindow = 60 * time.Second
	// postConnectRuleCheckDelay is how long after a connect we wait before
	// re-evaluating the rules.
	postConnectRuleCheckDelay = 3 * time.Second

	// evalReasonStartup tags the one-shot evaluation the helper runs
	// shortly after start (login / boot / LaunchDaemon restart). It is a
	// log label only — startup evaluations obey exactly the same rules as
	// every other trigger, including the unidentified-network skip below.
	evalReasonStartup = "startup"
)

// scheduleRuleCheck re-runs the automation rules shortly after a manual
// (RPC) connect made during the helper's startup window. The startup
// evaluation already decides each tunnel's intended state from the rules;
// a connect that arrives afterwards (e.g. a GUI restore of the last
// session) would otherwise stay up until the 30s poll notices and tears
// it down — looking like the app "connects first, then obeys the rules".
// A quick post-connect eval makes the rule decision authoritative within
// a few seconds. Outside the startup window a deliberate manual connect
// is left alone and the regular poll remains the fallback.
func (h *Helper) scheduleRuleCheck() {
	if time.Since(h.startedAt) > startupRuleCheckWindow {
		return
	}
	go func() {
		select {
		case <-h.done:
			return
		case <-time.After(postConnectRuleCheckDelay):
		}
		h.requestAutomationEval("post-connect")
	}()
}

// reconcileAction maps an evaluated desired state plus the tunnel's
// actual reality to the ONE action the engine takes:
//
//	"connect"         → bring the tunnel up (rule/default says connect, it
//	                    is down, and the manual-off latch is not set)
//	"disconnect"      → tear the tunnel down (rule/default says disconnect
//	                    and it is up — regardless of who brought it up)
//	"skip-manual-off" → the policy wants connect but the user switched the
//	                    tunnel off by hand; the latch wins until they
//	                    reconnect manually or the app restarts
//	"skip-manual-on"  → the policy wants disconnect but the user switched
//	                    the tunnel on by hand; the latch wins until they
//	                    disconnect manually or the app restarts
//	""                → leave the tunnel alone (policy agrees with reality,
//	                    the tunnel is unmanaged, or a wanted disconnect has
//	                    nothing to tear down)
//
// The two manual latches are mutually exclusive (a tunnel is at most one of
// off/on): manual-off suppresses connect, manual-on suppresses disconnect.
// Pure and side-effect free so the reconcile invariants (idempotency,
// manual-latch supremacy) are testable without a live tunnel manager.
func reconcileAction(state wifi.DesiredState, active, manualOff, manualOn bool) string {
	switch state {
	case wifi.StateConnect:
		if manualOff {
			return "skip-manual-off"
		}
		if !active {
			return "connect"
		}
	case wifi.StateDisconnect:
		if manualOn {
			return "skip-manual-on"
		}
		if active {
			return "disconnect"
		}
	}
	return ""
}

// reevaluateAutomation drives every tunnel that has Automation rules
// toward its desired state for the current network context (SSID +
// physical-interface subnets). This runs entirely inside the helper, so
// rules keep firing whether or not a GUI is alive.
//
// Semantics (issue #12): a rule can connect OR disconnect its tunnel
// regardless of how it was brought up. A tunnel with neither rules nor
// an explicit Default State is never touched; a default-only policy
// always converges on its Default State. On an UNIDENTIFIED network the
// whole evaluation is skipped (see below) — no condition, no action.
//
// Concurrency: triggers must NOT call this directly — they post through
// requestAutomationEval, and automationEvalLoop is the only caller. A
// mid-burst trigger therefore never queues a stale duplicate evaluation;
// the NetworkContext is sampled fresh at the top of each run. reevalMu
// remains as defence-in-depth serialisation.
func (h *Helper) reevaluateAutomation(reason string) {
	h.reevalMu.Lock()
	defer h.reevalMu.Unlock()

	settings, err := h.loadUserSettings()
	if err != nil {
		slog.Debug("automation: cannot load settings", "error", err)
		return
	}
	settings.EnsureAutomation()
	auto := settings.Automation
	// Both maps count: a tunnel with rules has a policy, and so does a
	// tunnel with only a Default State ("always converge to it"). Only a
	// tunnel in NEITHER map is policy-free.
	if auto == nil || (len(auto.PerTunnel) == 0 && len(auto.Defaults) == 0) {
		return
	}

	ctx := h.currentNetworkContext()

	// An UNIDENTIFIED network (no SSID reported AND no physical-interface
	// address yet) means no condition can be judged: SSID rules have
	// nothing to match against, subnet/MAC rules have no interfaces to
	// inspect. Acting anyway would blind-connect tunnels onto a network
	// we haven't seen — then tear them down seconds later when the SSID
	// report arrives — or wrongly tear down a crash-recovered tunnel.
	// So: skip the entire evaluation, in BOTH directions, and wait. The
	// SSID report, route-change events and the poll each re-post an eval
	// request, so the real decision runs the moment the network is
	// identified — a Default State of "connect" therefore still takes
	// effect at autostart, only a few seconds later.
	if ctx.SSID == "" && len(ctx.PhysicalIPs) == 0 {
		slog.Debug("automation: network unidentified — skipping evaluation",
			"reason", reason)
		return
	}

	// Context line: every evaluation states WHAT it judged against. This
	// is the first thing anyone diagnosing "the tunnel didn't come up"
	// needs, and it used to be missing entirely.
	ipStrs := make([]string, 0, len(ctx.PhysicalIPs))
	for _, ip := range ctx.PhysicalIPs {
		ipStrs = append(ipStrs, ip.String())
	}
	ifaces := make([]string, 0, len(ctx.Interfaces))
	for _, inf := range ctx.Interfaces {
		ifaces = append(ifaces, inf.Name)
	}
	slog.Info("automation: evaluating",
		"category", "network",
		"reason", reason,
		"tunnels", len(auto.PolicyTunnelNames()),
		"ssid", ctx.SSID,
		"gateway_mac", ctx.GatewayMAC,
		"gateway_ip", ctx.GatewayIP,
		"physical_ips", strings.Join(ipStrs, ","),
		"interfaces", strings.Join(ifaces, ","))

	// A tunnel the user has manually switched off (UI or tray menu) must
	// not be silently reconnected by its rules until they reconnect it by
	// hand once or the app restarts — the manual off wins over automation.
	manualOff := make(map[string]bool, len(settings.ManualOffTunnels))
	for _, n := range settings.ManualOffTunnels {
		manualOff[n] = true
	}
	// The mirror latch: a tunnel the user has manually switched ON must not
	// be silently torn down by its rules until they disconnect it by hand
	// once or the app restarts — the manual on wins over automation.
	manualOn := make(map[string]bool, len(settings.ManualOnTunnels))
	for _, n := range settings.ManualOnTunnels {
		manualOn[n] = true
	}

	active := make(map[string]bool)
	for _, n := range h.manager.ActiveTunnels() {
		active[n] = true
	}

	for _, name := range auto.PolicyTunnelNames() {
		rules := auto.PerTunnel[name]
		state := wifi.EvaluatePolicy(rules, auto.Defaults[name], ctx)
		switch reconcileAction(state, active[name], manualOff[name], manualOn[name]) {
		case "connect":
			h.automationConnect(name, reason, ctx.SSID)
		case "disconnect":
			slog.Info("automation: rule disconnect",
				"category", "network",
				"tunnel", name, "reason", reason, "ssid", ctx.SSID)
			h.disconnectAutoManaged(name)
		case "skip-manual-off":
			slog.Info("automation: skip connect (manually switched off)",
				"category", "network",
				"tunnel", name, "reason", reason, "ssid", ctx.SSID)
		case "skip-manual-on":
			slog.Info("automation: skip disconnect (manually switched on)",
				"category", "network",
				"tunnel", name, "reason", reason, "ssid", ctx.SSID)
		default:
			// No action — log it anyway. Without this line an evaluation
			// that leaves everything alone is invisible in the log viewer,
			// and "why didn't my tunnel connect?" has no answer.
			slog.Info("automation: no action",
				"category", "network",
				"tunnel", name,
				"reason", reason,
				"decision", decisionLabel(state, manualOff[name], manualOn[name]),
				"rules", len(rules),
				"default", auto.Defaults[name],
				"active", active[name],
				"ssid", ctx.SSID)
		}
	}
}

// decisionLabel renders an evaluated desired state (plus the manual latches)
// as the word the log viewer and the CLI preview both use, so the same
// vocabulary appears everywhere.
func decisionLabel(state wifi.DesiredState, manualOff, manualOn bool) string {
	switch state {
	case wifi.StateConnect:
		if manualOff {
			return "manual-off"
		}
		return "connect"
	case wifi.StateDisconnect:
		if manualOn {
			return "manual-on"
		}
		return "disconnect"
	}
	return "unmanaged"
}

// handleAutomationPreview is a read-only dry-run of the Automation
// engine: it reports the current network context and each rule-bearing
// tunnel's evaluated decision, without connecting or disconnecting
// anything. Backs `wireguideplus ctl automation` and answers "why did this
// tunnel (dis)connect?".
func (h *Helper) handleAutomationPreview(_ json.RawMessage) (interface{}, error) {
	settings, err := h.loadUserSettings()
	if err != nil {
		return nil, err
	}
	settings.EnsureAutomation()
	auto := settings.Automation

	ctx := h.currentNetworkContext()

	ipStrs := make([]string, 0, len(ctx.PhysicalIPs))
	for _, ip := range ctx.PhysicalIPs {
		ipStrs = append(ipStrs, ip.String())
	}

	active := make(map[string]bool)
	for _, n := range h.manager.ActiveTunnels() {
		active[n] = true
	}

	manualOff := make(map[string]bool, len(settings.ManualOffTunnels))
	for _, n := range settings.ManualOffTunnels {
		manualOff[n] = true
	}
	manualOn := make(map[string]bool, len(settings.ManualOnTunnels))
	for _, n := range settings.ManualOnTunnels {
		manualOn[n] = true
	}

	resp := ipc.AutomationPreviewResponse{
		SSID:        ctx.SSID,
		PhysicalIPs: ipStrs,
		GatewayMAC:  ctx.GatewayMAC,
		GatewayIP:   ctx.GatewayIP,
		Interfaces:  ctx.Interfaces,
	}
	if auto != nil {
		for _, name := range auto.PolicyTunnelNames() {
			rules := auto.PerTunnel[name]
			decision := "unmanaged"
			switch wifi.EvaluatePolicy(rules, auto.Defaults[name], ctx) {
			case wifi.StateConnect:
				if manualOff[name] {
					decision = "manual-off" // suppressed by the manual-off latch
				} else {
					decision = "connect"
				}
			case wifi.StateDisconnect:
				if manualOn[name] {
					decision = "manual-on" // suppressed by the manual-on latch
				} else {
					decision = "disconnect"
				}
			}
			resp.Tunnels = append(resp.Tunnels, ipc.AutomationTunnelDecision{
				Name:      name,
				RuleCount: len(rules),
				Decision:  decision,
				Active:    active[name],
				ManualOff: manualOff[name],
				ManualOn:  manualOn[name],
			})
		}
	}
	return resp, nil
}

// automationConnect brings up a tunnel a rule matched and records it in
// the auto-managed map. Caller holds reevalMu.
func (h *Helper) automationConnect(name, reason, ssid string) {
	if h.userTunnelStore == nil {
		slog.Warn("automation: tunnel store unavailable, cannot connect", "tunnel", name)
		return
	}
	cfg, err := h.userTunnelStore.Load(name)
	if err != nil {
		slog.Warn("automation: cannot load tunnel config", "tunnel", name, "error", err)
		return
	}

	// Policy validation sits BETWEEN the desired action and the tunnel
	// operation (principle 32): the automation engine decides WHAT it
	// wants; the policy layer decides whether that is allowed. Automation
	// is blocked by default on a true tie and never prompts — it logs and
	// notifies the tray instead (principle 25).
	if block := h.policyBlockFor(name); block != nil {
		slog.Warn("automation: connect blocked by policy",
			"category", "policy",
			"tunnel", name, "reason", block.Reason, "summary", block.Summary)
		h.server.Broadcast(ipc.EventPolicyBlocked, *block)
		return
	}

	// The DNS resolve path is exclusive among CONNECTED tunnels, which is a
	// runtime fact the configuration-only analyzer cannot see. A manual
	// connect gets parked and asks the user; automation has nobody to
	// answer, so it never waits — it skips and notifies, same as any other
	// policy block (principle 25).
	if h.tunnelClaimsDNSPath(name) {
		if blockers := h.dnsPathBlockers(name); len(blockers) > 0 {
			block := ipc.PolicyBlockedPayload{
				Tunnel: name,
				Reason: "dns",
				Summary: fmt.Sprintf("the system's DNS resolve path is already carried by %s — only one connected tunnel can carry it",
					strings.Join(blockers, ", ")),
			}
			slog.Warn("automation: connect blocked by DNS resolve path",
				"category", "policy", "tunnel", name, "blockers", blockers)
			h.server.Broadcast(ipc.EventPolicyBlocked, block)
			return
		}
	}

	slog.Info("automation: rule connect",
		"category", "network",
		"tunnel", name, "reason", reason, "ssid", ssid)
	h.connectMu.Lock()
	err = h.doConnectHeld(cfg)
	if err == nil {
		// Same firewall follow-up a manual connect does — otherwise a
		// headless automation connect never enforces the tunnel's DNS resolve path
		// DNS policy (issue #12).
		h.applyPostConnectFirewall(cfg)
	}
	h.connectMu.Unlock()
	if err != nil {
		slog.Warn("automation connect failed", "tunnel", name, "error", err)
		return
	}
	h.wifiMu.Lock()
	h.autoConnectedBy[name] = ssid
	h.wifiMu.Unlock()
	// Notify GUI so it runs the same post-connect refresh as a manual connect.
	h.server.Broadcast(ipc.EventAutoConnect, ipc.AutoConnectPayload{TunnelName: name})
}

// disconnectAutoManaged tears down a tunnel that the wifi-rule
// engine auto-connected, then clears every cache that referenced it
// (activeCfgs, autoConnectedBy, in-flight retry). Without each of
// these cleanups the helper's various recovery paths would
// resurrect the tunnel: the reconnect monitor would fire its
// pending retry; manager.Disconnect()'s legacy "all tunnels" path
// would re-Connect from a stale activeCfgs entry; and the next
// SSID change handler would try to disconnect a tunnel already
// gone.
func (h *Helper) disconnectAutoManaged(name string) {
	if h.monitor != nil {
		h.monitor.CancelRetryFor(name)
	}
	if err := h.manager.DisconnectTunnel(name); err != nil {
		slog.Warn("automation disconnect failed", "tunnel", name, "error", err)
	}
	// If this tunnel carried the DNS resolve path policy, drop its enforcement.
	h.clearDNSPathIfOwner(name)
	h.mu.Lock()
	delete(h.activeCfgs, name)
	h.mu.Unlock()
	h.wifiMu.Lock()
	delete(h.autoConnectedBy, name)
	h.wifiMu.Unlock()
	// Prune the latency cache exactly as handleDisconnect does — otherwise
	// the status broadcast keeps reporting the dead tunnel's last RTT.
	h.latencyMu.Lock()
	delete(h.latencyByTunnel, name)
	h.latencyMu.Unlock()
	h.maybeArmShutdownAfterTeardown("rule-driven disconnect, no GUI attached")
}
