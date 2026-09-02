package helper

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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
// Wi-Fi monitor fires it on every SSID transition. The actual decision
// logic lives in reevaluateAutomation so the network-change and poll
// triggers share it.
func (h *Helper) handleSSIDChange(oldSSID, newSSID string) {
	h.reevaluateAutomation("ssid-change")
}

const (
	// startupRuleCheckWindow is how long after helper start a manual
	// connect gets an immediate rule re-check.
	startupRuleCheckWindow = 60 * time.Second
	// postConnectRuleCheckDelay is how long after a connect we wait before
	// re-evaluating the rules.
	postConnectRuleCheckDelay = 3 * time.Second
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
		h.reevaluateAutomation("post-connect")
	}()
}

// reevaluateAutomation drives every tunnel that has Automation rules
// toward its desired state for the current network context (SSID +
// physical-interface subnets). This runs entirely inside the helper, so
// rules keep firing whether or not a GUI is alive.
//
// Semantics (issue #12): a rule can connect OR disconnect its tunnel
// regardless of how the tunnel was brought up — unlike the legacy path
// which only touched helper-auto-connected tunnels. A tunnel with NO
// rules is never touched. reevalMu serialises evaluations so the slow
// connect/disconnect calls from two overlapping triggers can't race.
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
	if auto == nil || len(auto.PerTunnel) == 0 {
		return
	}

	ctx := h.currentNetworkContext()

	// A tunnel the user has manually switched off (UI or tray menu) must
	// not be silently reconnected by its rules until they reconnect it by
	// hand once or the app restarts — the manual off wins over automation.
	manualOff := make(map[string]bool, len(settings.ManualOffTunnels))
	for _, n := range settings.ManualOffTunnels {
		manualOff[n] = true
	}

	active := make(map[string]bool)
	for _, n := range h.manager.ActiveTunnels() {
		active[n] = true
	}

	for _, name := range auto.TunnelNames() {
		state := wifi.Evaluate(auto.PerTunnel[name], ctx)
		switch state {
		case wifi.StateConnect:
			if !active[name] {
				if manualOff[name] {
					slog.Info("automation: skip connect (manually switched off)",
						"tunnel", name, "reason", reason, "ssid", ctx.SSID)
					continue
				}
				h.automationConnect(name, reason, ctx.SSID)
			}
		case wifi.StateDisconnect:
			if active[name] {
				slog.Info("automation: rule disconnect", "tunnel", name, "reason", reason, "ssid", ctx.SSID)
				h.disconnectAutoManaged(name)
			}
		}
	}
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

	resp := ipc.AutomationPreviewResponse{
		SSID:        ctx.SSID,
		PhysicalIPs: ipStrs,
		GatewayMAC:  ctx.GatewayMAC,
		GatewayIP:   ctx.GatewayIP,
		Interfaces:  ctx.Interfaces,
	}
	if auto != nil {
		for _, name := range auto.TunnelNames() {
			rules := auto.PerTunnel[name]
			decision := "unmanaged"
			switch wifi.Evaluate(rules, ctx) {
			case wifi.StateConnect:
				if manualOff[name] {
					decision = "manual-off" // suppressed by the manual-off latch
				} else {
					decision = "connect"
				}
			case wifi.StateDisconnect:
				decision = "disconnect"
			}
			resp.Tunnels = append(resp.Tunnels, ipc.AutomationTunnelDecision{
				Name:      name,
				RuleCount: len(rules),
				Decision:  decision,
				Active:    active[name],
				ManualOff: manualOff[name],
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
	slog.Info("automation: rule connect", "tunnel", name, "reason", reason, "ssid", ssid)
	h.connectMu.Lock()
	err = h.doConnectHeld(cfg)
	if err == nil {
		// Same firewall follow-up a manual connect does — otherwise a
		// headless automation connect gets no DNS protection and, if the
		// kill switch is already on, its endpoints are never permitted so
		// the tunnel can't pass traffic (issue #12).
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
	// Snapshot the interface name before teardown so we can strip it from
	// the kill-switch filter set afterwards, exactly as handleDisconnect
	// does. Without this a rule-driven disconnect leaves a dead tunnel's
	// LUID permitted in the WFP filters (issue #12).
	iface := ""
	if h.firewall.IsKillSwitchEnabled() {
		for _, st := range h.manager.AllStatuses() {
			if st != nil && st.TunnelName == name && st.InterfaceName != "" {
				iface = st.InterfaceName
				break
			}
		}
	}
	if err := h.manager.DisconnectTunnel(name); err != nil {
		slog.Warn("automation disconnect failed", "tunnel", name, "error", err)
	}
	if iface != "" {
		if err := h.firewall.RemoveKillSwitchTunnel(iface); err != nil {
			slog.Warn("RemoveKillSwitchTunnel after automation disconnect failed",
				"interface", iface, "error", err)
		}
	}
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
