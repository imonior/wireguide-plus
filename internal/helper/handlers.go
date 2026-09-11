package helper

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/imonior/wireguide-plus/internal/config"
	"github.com/imonior/wireguide-plus/internal/diag"
	"github.com/imonior/wireguide-plus/internal/domain"
	"github.com/imonior/wireguide-plus/internal/ipc"
	"github.com/imonior/wireguide-plus/internal/update"
	"github.com/imonior/wireguide-plus/internal/wifi"
)

// registerHandlers binds every RPC method to a Helper method. Splitting the
// handlers into named methods (vs inline closures) makes them directly unit
// testable — `handler := &Helper{manager: mockMgr}; handler.handleConnect(...)`.
func (h *Helper) registerHandlers() {
	h.server.Handle(ipc.MethodPing, h.handlePing)
	h.server.Handle(ipc.MethodShutdown, h.handleShutdown)
	h.server.Handle(ipc.MethodForceShutdown, h.handleForceShutdown)
	h.server.Handle(ipc.MethodRequestQuit, h.handleRequestQuit)
	h.server.Handle(ipc.MethodSetLogLevel, h.handleSetLogLevel)
	h.server.Handle(ipc.MethodConnect, h.handleConnect)
	h.server.Handle(ipc.MethodDisconnect, h.handleDisconnect)
	h.server.Handle(ipc.MethodStatus, h.handleStatus)
	h.server.Handle(ipc.MethodIsConnected, h.handleIsConnected)
	h.server.Handle(ipc.MethodActiveName, h.handleActiveName)
	h.server.Handle(ipc.MethodActiveTunnels, h.handleActiveTunnels)
	h.server.Handle(ipc.MethodRename, h.handleRename)
	h.server.Handle(ipc.MethodResolveDNSPathConflict, h.handleResolveDNSPathConflict)
	h.server.Handle(ipc.MethodClearDNSPathEnforcement, h.handleClearDNSPathEnforcement)
	h.server.Handle(ipc.MethodSetHealthCheck, h.handleSetHealthCheck)
	h.server.Handle(ipc.MethodSetPinInterface, h.handleSetPinInterface)
	h.server.Handle(ipc.MethodReportSSID, h.handleReportSSID)
	h.server.Handle(ipc.MethodAutomationPreview, h.handleAutomationPreview)
	h.server.Handle(ipc.MethodAutomationReevaluate, h.handleAutomationReevaluate)
}

// handleAutomationReevaluate runs one automation evaluation immediately.
// It returns as soon as the request is queued — the evaluation itself is
// coalesced and executed by automationEvalLoop, so a save never blocks on
// connect/disconnect work.
func (h *Helper) handleAutomationReevaluate(_ json.RawMessage) (interface{}, error) {
	h.requestAutomationEval("settings-save")
	return ipc.Empty{}, nil
}

func (h *Helper) handleSetLogLevel(params json.RawMessage) (interface{}, error) {
	var req ipc.SetLogLevelRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	lvl := parseLevel(req.Level)
	// No-op when nothing changed. The GUI calls SetLogLevel on startup
	// and on every Settings save; logging + broadcasting each time
	// produced repeated "log level changed" lines and spammed
	// settings_changed subscribers even when the value was identical.
	if h.logLevel.Level() == lvl {
		return ipc.Empty{}, nil
	}
	h.logLevel.Set(lvl)
	slog.Info("log level changed", "level", req.Level)
	h.server.Broadcast(ipc.EventSettingsChanged, ipc.SettingsChangedPayload{LogLevel: &req.Level})
	return ipc.Empty{}, nil
}

func (h *Helper) handlePing(params json.RawMessage) (interface{}, error) {
	return ipc.PingResponse{Version: ipc.ProtocolVersion, AppVersion: update.CurrentVersion(), PID: os.Getpid()}, nil
}

func (h *Helper) handleShutdown(params json.RawMessage) (interface{}, error) {
	go func() {
		time.Sleep(100 * time.Millisecond) // let the response go out first
		h.shutdown()
	}()
	return ipc.Empty{}, nil
}

// handleRequestQuit implements `wireguideplus ctl stop` — bring the whole app
// down, GUI included.
//
// Two cases, because "stop" has to mean the same thing either way:
//
//   - A GUI is attached: broadcast EventQuit and let the GUI terminate
//     itself. Its normal quit path disconnects tunnels and then stops us.
//     We must NOT shut down directly here — the GUI's health monitor would
//     see the helper vanish and respawn it, which on macOS means an admin
//     password prompt seconds after the user asked everything to stop.
//
//   - No GUI attached (helper running solo): nothing will relay the quit,
//     so shut ourselves down on the same grace-free path as MethodShutdown.
//
// Reports which branch ran so `ctl stop` can tell the user whether it
// stopped an app or just a stray helper.
func (h *Helper) handleRequestQuit(params json.RawMessage) (interface{}, error) {
	if h.server.HasControlConn() {
		slog.Info("quit requested via CLI — asking the GUI to terminate")
		h.server.Broadcast(ipc.EventQuit, ipc.Empty{})
		return ipc.RequestQuitResponse{NotifiedGUI: true}, nil
	}
	slog.Info("quit requested via CLI — no GUI attached, shutting the helper down")
	go func() {
		time.Sleep(100 * time.Millisecond) // let the response go out first
		h.shutdown()
	}()
	return ipc.RequestQuitResponse{NotifiedGUI: false}, nil
}

// handleForceShutdown bypasses graceful teardown and exits as fast as
// possible. Used by the GUI's upgrade path when MethodShutdown failed
// (wedged handler, stale state).
//
// We still do a minimum-effort firewall cleanup before exit:
//   - macOS: pf anchors persist past process death, so leaving the kill
//     switch up would lock the user out of the internet.
//   - Linux: nftables wireguide table persists in the kernel, same risk.
//   - Windows: WFP dynamic-session filters are auto-deleted by BFE when
//     the process dies (FWPM_SESSION_FLAG_DYNAMIC contract), so the
//     Cleanup call here is a no-op on the kernel side — still cheap.
//
// Tunnels themselves (TUN device + wireguard-go) are NOT torn down — the
// utun/wg interface disappears when the process dies on Unix, and Wintun
// adapters get cleaned up by our cleanupStaleWintunAdapter on next launch.
// The whole sequence is bounded by a 1-second deadline so a wedged
// firewall.Cleanup can't keep ForceShutdown hostage.
func (h *Helper) handleForceShutdown(params json.RawMessage) (interface{}, error) {
	go func() {
		// First, give the response 50ms to actually reach the client.
		// Then run firewall cleanup with a 1s hard cap, then exit.
		time.Sleep(50 * time.Millisecond)
		slog.Warn("helper ForceShutdown requested, cleaning firewall then exiting")
		done := make(chan struct{})
		go func() {
			defer close(done)
			if err := h.firewall.Cleanup(); err != nil {
				slog.Warn("ForceShutdown: firewall.Cleanup failed", "error", err)
			}
			// The utun devices die with this process, but networksetup DNS
			// overrides persist in SystemConfiguration — restore them or a
			// helper upgrade while connected leaves tunnel DNS behind
			// (issue #34). Best-effort under the same deadline.
			if h.manager != nil {
				h.manager.RestoreDNSBestEffort()
			}
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			slog.Warn("ForceShutdown: cleanup timed out; exiting anyway")
		}
		os.Exit(0)
	}()
	return ipc.Empty{}, nil
}

// handleRename atomically renames a tunnel's .conf file. Holds the same
// connectMu that handleConnect/handleDisconnect take, so a Connect arriving
// during the rename blocks until we finish — closing the GUI-side TOCTOU
// where the user could rename a tunnel just as it was being auto-connected.
//
// Active-tunnel rename is rejected: the WireGuard interface name is derived
// from the tunnel name on macOS, so renaming would orphan the running utun.
func (h *Helper) handleRename(params json.RawMessage) (interface{}, error) {
	h.connectMu.Lock()
	defer h.connectMu.Unlock()

	var req ipc.RenameRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	if req.OldName == "" || req.NewName == "" {
		return nil, fmt.Errorf("rename: old and new names are required")
	}
	if req.OldName == req.NewName {
		return ipc.Empty{}, nil
	}

	// Reject if the tunnel is currently active. h.connectMu is held so
	// no Connect/Disconnect can race past this check.
	for _, name := range h.manager.ActiveTunnels() {
		if name == req.OldName {
			return nil, fmt.Errorf("cannot rename connected tunnel %q — disconnect first", req.OldName)
		}
	}

	if h.userTunnelStore == nil {
		return nil, fmt.Errorf("rename: helper has no user tunnel store (running as root without --uid?)")
	}
	if err := h.userTunnelStore.Rename(req.OldName, req.NewName); err != nil {
		return nil, err
	}

	// Sync activeCfgs under h.mu.
	h.mu.Lock()
	if cfg, ok := h.activeCfgs[req.OldName]; ok {
		delete(h.activeCfgs, req.OldName)
		if cfg != nil {
			cfg.Name = req.NewName
		}
		h.activeCfgs[req.NewName] = cfg
	}
	h.mu.Unlock()

	// Sync autoConnectedBy under h.wifiMu — separate from h.mu to preserve
	// lock ordering: wifiMu is acquired before connectMu inside handleSSIDChange,
	// so we must NOT hold both simultaneously here.
	h.wifiMu.Lock()
	if owner, ok := h.autoConnectedBy[req.OldName]; ok {
		delete(h.autoConnectedBy, req.OldName)
		h.autoConnectedBy[req.NewName] = owner
	}
	h.wifiMu.Unlock()

	// Move the cached latency under the new key so the old name doesn't
	// linger as an orphaned map entry (the sibling maps above are already
	// re-keyed; this one was missed).
	h.latencyMu.Lock()
	if lat, ok := h.latencyByTunnel[req.OldName]; ok {
		delete(h.latencyByTunnel, req.OldName)
		h.latencyByTunnel[req.NewName] = lat
	}
	h.latencyMu.Unlock()
	return ipc.Empty{}, nil
}

// doConnectHeld caches cfg BEFORE calling manager.Connect (so the reconnect
// monitor sees the config during Connect), then rolls back on failure.
// Caller MUST hold h.connectMu.
func (h *Helper) doConnectHeld(cfg *domain.WireGuardConfig) error {
	// Authoritative AmneziaWG gate: refuse AWG tunnels while the user has
	// disabled AWG support (Settings → Advanced). Done here — before any
	// state is mutated — so every connect path (RPC from the GUI or CLI,
	// and the automation engine's automationConnect) hits it. AWG configs
	// cannot be downgraded to plain WireGuard (the server requires the
	// obfuscated handshake), so "disabled" means "refuse to connect", not
	// "connect without obfuscation".
	if cfg.Protocol == domain.ProtocolAmneziaWG {
		settings, err := h.loadUserSettings()
		// A settings read failure (e.g. no user dir derived) is NOT a
		// reason to block a connect the user explicitly requested — the
		// gate only fires on a definitive "disabled" from disk.
		if err == nil && settings != nil && !settings.EnableAWG {
			return fmt.Errorf("%s — enable it in Settings → Advanced to connect this tunnel", domain.ErrAWGDisabled)
		}
	}

	// Authoritative per-tunnel physical-egress binding: read from the meta
	// sidecar so EVERY connect path (RPC, CLI, automation, reconnect) pins
	// the same egress. Values are runtime-injected on the config — they are
	// never part of the .conf file itself. Gated on the Settings toggle so
	// turning the feature off neutralizes all saved bindings.
	if idx, ifName := h.loadTunnelBinding(cfg.Name); idx > 0 {
		if settings, err := h.loadUserSettings(); err == nil && settings != nil && settings.PinInterface {
			cfg.BindIfIndex = idx
			cfg.BindIfName = ifName
		}
	}

	h.mu.Lock()
	prevCfgs := h.copyActiveCfgs()
	h.activeCfgs[cfg.Name] = cfg
	h.mu.Unlock()

	if err := h.manager.Connect(cfg); err != nil {
		h.mu.Lock()
		delete(h.activeCfgs, cfg.Name)
		if prev, ok := prevCfgs[cfg.Name]; ok {
			h.activeCfgs[cfg.Name] = prev
		}
		h.mu.Unlock()
		return err
	}
	return nil
}

// enableKillSwitchForActiveTunnels and the global kill-switch toggle were
// removed with the kill switch itself (policy principle 8/9): a machine-wide
// blockade cannot express which of several tunnels is authoritative. Tunnel-
// scoped protection lives in TunnelMeta.TrafficProtect, and DNS enforcement
// lives in TunnelMeta.DNSResolvePath (applyPostConnectFirewall).

func (h *Helper) handleConnect(params json.RawMessage) (interface{}, error) {
	var req ipc.ConnectRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	if req.Config == nil {
		return nil, fmt.Errorf("config is required")
	}

	// DNS resolve path: park the connect BEFORE taking connectMu if
	// another connected tunnel already owns the machine-wide DNS path.
	// Waiting under connectMu would deadlock the very actions the dialog
	// offers (disconnecting the owning tunnel, connecting something else).
	h.clearDNSPathWaiver(req.Config.Name)
	if err := h.awaitDNSResolvePathClear(req.Config.Name); err != nil {
		return nil, err
	}

	// Serialize Connect calls so two GUIs can't race on activeCfg.
	h.connectMu.Lock()
	defer h.connectMu.Unlock()

	// Re-validate config server-side (don't trust client).
	if result := config.Validate(req.Config); !result.IsValid() {
		return nil, fmt.Errorf("invalid config: %s", strings.Join(result.ErrorMessages(), "; "))
	}

	// Log if the config contains scripts — they are parsed but ignored.
	if req.Config.HasScripts() {
		slog.Info("config contains Pre/PostUp/Down scripts; ignoring (not supported in GUI client)",
			"tunnel", req.Config.Name)
	}

	// Check for routing conflicts with existing interfaces (Tailscale etc).
	// Log warnings but don't block — users can override via UI. Skip the
	// tunnel's OWN interface when it is already up (reconnect): its routes
	// come from its own AllowedIPs, so counting them reports every CIDR as
	// conflicting with itself.
	var allowedIPs []string
	for _, peer := range req.Config.Peers {
		allowedIPs = append(allowedIPs, peer.AllowedIPs...)
	}
	var exclude []string
	if st := h.manager.StatusFor(req.Config.Name); st != nil && st.InterfaceName != "" {
		exclude = append(exclude, st.InterfaceName)
	}
	if conflicts, err := diag.CheckConflicts(allowedIPs, req.Config.Interface.Address, exclude...); err == nil && len(conflicts) > 0 {
		for _, c := range conflicts {
			slog.Warn("routing conflict detected",
				"interface", c.InterfaceName,
				"owner", c.Owner,
				"overlaps", c.OverlappingIPs)
		}
	}

	// Log the source so the log viewer can distinguish rule-driven,
	// restore-driven and user-driven connects.
	slog.Info("connect requested", "tunnel", req.Config.Name, "source", "rpc")

	if err := h.doConnectHeld(req.Config); err != nil {
		slog.Warn("tunnel connect failed", "category", "tunnel", "tunnel", req.Config.Name, "error", err)
		return nil, err
	}

	h.applyPostConnectFirewall(req.Config)
	// A GUI restore / crash-recovery path can bring up a tunnel the
	// automation rules would leave down; re-check shortly after connect
	// (only within the startup window) so the rules stay authoritative.
	h.scheduleRuleCheck()

	// Success record for the log viewer: surface the egress interface, peer
	// count and the DNS servers this tunnel now owns so a connect is
	// traceable without re-deriving it from the config.
	status := h.manager.StatusFor(req.Config.Name)
	iface := ""
	if status != nil {
		iface = status.InterfaceName
	}
	slog.Info("tunnel connected",
		"category", "tunnel",
		"tunnel", req.Config.Name,
		"interface", iface,
		"peers", len(req.Config.Peers),
		"dns", req.Config.Interface.DNS,
	)
	return ipc.Empty{}, nil
}

// applyPostConnectFirewall runs the firewall follow-up that must happen
// after a tunnel comes up, shared by manual (handleConnect) and
// automation (automationConnect) connects. Best-effort: logs and continues
// on error, exactly like manual connect.
//
// The only firewall follow-up left is the per-tunnel DNS resolve path policy
// (TunnelMeta.DNSResolvePath, gated by the master switch): from the moment
// this tunnel is up, port 53 is dropped on every other interface, so no
// process on the machine can resolve a name by a route that bypasses this
// tunnel — whatever resolver address it happens to ask. It is the
// per-tunnel replacement of the removed global "DNS protection" toggle
// (policy principle 11/33).
//
// If the tunnel does not claim the path nothing is installed and the system
// keeps using DNS exactly as the OS configured it.
func (h *Helper) applyPostConnectFirewall(cfg *domain.WireGuardConfig) {
	if !h.tunnelClaimsDNSPath(cfg.Name) {
		return
	}
	dnsServers := cfg.Interface.DNS
	if len(dnsServers) == 0 {
		slog.Warn("DNS resolve path is on but the tunnel has no DNS servers — nothing to allow through it",
			"category", "policy", "tunnel", cfg.Name)
		return
	}
	status := h.manager.StatusFor(cfg.Name)
	if status == nil || status.InterfaceName == "" {
		slog.Warn("DNS resolve path: no interface for tunnel", "category", "policy", "tunnel", cfg.Name)
		return
	}
	if err := h.firewall.EnableDNSProtection(status.InterfaceName, dnsServers); err != nil {
		slog.Warn("DNS resolve path enforcement failed", "category", "policy",
			"tunnel", cfg.Name, "interface", status.InterfaceName, "error", err)
		return
	}
	h.mu.Lock()
	h.dnsPathOwner = cfg.Name
	h.mu.Unlock()
	slog.Info("system DNS resolution now egresses through this tunnel only", "category", "policy",
		"tunnel", cfg.Name, "interface", status.InterfaceName, "dns", dnsServers)
}

// clearDNSPathIfOwner tears down the DNS resolve path enforcement when its
// owning tunnel goes down, but only if that tunnel still owns it — another
// tunnel may have taken over the role while this one was up (which the
// parked-connect flow makes possible).
func (h *Helper) clearDNSPathIfOwner(name string) {
	h.clearDNSPathWaiver(name)
	h.mu.Lock()
	owner := h.dnsPathOwner
	if owner == name {
		h.dnsPathOwner = ""
	}
	h.mu.Unlock()
	if owner != name {
		return
	}
	if err := h.firewall.DisableDNSProtection(); err != nil {
		slog.Warn("disabling DNS resolve path enforcement failed", "category", "policy",
			"tunnel", name, "error", err)
	} else {
		slog.Info("DNS resolve path enforcement removed with its tunnel — DNS is back to the OS default",
			"category", "policy", "tunnel", name)
	}
}

// handleClearDNSPathEnforcement tears down any live DNS resolve path
// enforcement right now. The GUI calls it when the master switch turns OFF:
// with the feature off nothing may keep enforcing it, so the firewall rules
// a still-connected tunnel installed must not outlive the toggle. A no-op
// when nothing currently owns the path (tunnel down, nothing installed).
func (h *Helper) handleClearDNSPathEnforcement(_ json.RawMessage) (interface{}, error) {
	h.mu.Lock()
	owner := h.dnsPathOwner
	h.dnsPathOwner = ""
	h.mu.Unlock()
	if owner == "" {
		return ipc.Empty{}, nil
	}
	if err := h.firewall.DisableDNSProtection(); err != nil {
		slog.Warn("disabling DNS resolve path enforcement after master switch off failed",
			"category", "policy", "tunnel", owner, "error", err)
	} else {
		slog.Info("DNS resolve path enforcement removed — master switch is off, DNS is back to the OS default",
			"category", "policy", "tunnel", owner)
	}
	return ipc.Empty{}, nil
}

func (h *Helper) handleDisconnect(params json.RawMessage) (interface{}, error) {
	h.connectMu.Lock()
	defer h.connectMu.Unlock()

	// Parse optional tunnel name from request. len() on a nil slice
	// returns 0, so the previous explicit `params != nil` check was
	// redundant (S1009).
	var tunnelName string
	if len(params) > 0 {
		var req ipc.DisconnectRequest
		if err := json.Unmarshal(params, &req); err == nil {
			tunnelName = req.TunnelName
		}
		// If unmarshal fails (e.g. empty params), disconnect first tunnel (backward compat).
	}

	// Cancel only the in-flight reconnect for the tunnel(s) being
	// torn down — a per-tunnel disconnect of A must not abort a
	// healthy retry for B.
	if h.monitor != nil {
		if tunnelName != "" {
			h.monitor.CancelRetryFor(tunnelName)
		} else {
			h.monitor.CancelRetry()
		}
	}

	// Tunnels that were torn down by this call — filled by both paths and
	// used afterwards to drop the DNS resolve path enforcement if one of them
	// carried that policy.
	var tornDown []string

	if tunnelName != "" {
		tornDown = append(tornDown, tunnelName)
		if err := h.manager.DisconnectTunnel(tunnelName); err != nil {
			return nil, err
		}
		slog.Info("tunnel disconnected", "category", "tunnel", "tunnel", tunnelName, "source", "user")
		h.mu.Lock()
		delete(h.activeCfgs, tunnelName)
		h.mu.Unlock()
		h.wifiMu.Lock()
		delete(h.autoConnectedBy, tunnelName)
		h.wifiMu.Unlock()
		h.latencyMu.Lock()
		delete(h.latencyByTunnel, tunnelName)
		h.latencyMu.Unlock()
	} else {
		// Legacy "no name" path: tear down EVERY active tunnel via
		// per-tunnel calls so manager.Disconnect()'s "pick the first"
		// semantic doesn't leave half the snapshot still up while we
		// blanket-evict their cached configs. Each successful per-
		// tunnel disconnect drops its cache entry; partial failures
		// leave the still-up tunnels intact in activeCfgs so the
		// reconnect monitor can still recover them.
		toDisconnect := h.manager.ActiveTunnels()
		var firstErr error
		for _, name := range toDisconnect {
			if err := h.manager.DisconnectTunnel(name); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				slog.Warn("legacy disconnect: tunnel teardown failed",
					"tunnel", name, "error", err)
				continue
			}
			h.mu.Lock()
			delete(h.activeCfgs, name)
			h.mu.Unlock()
			h.wifiMu.Lock()
			delete(h.autoConnectedBy, name)
			h.wifiMu.Unlock()
			h.latencyMu.Lock()
			delete(h.latencyByTunnel, name)
			h.latencyMu.Unlock()
			slog.Info("tunnel disconnected", "category", "tunnel", "tunnel", name, "source", "user")
			tornDown = append(tornDown, name)
		}
		if firstErr != nil {
			return nil, firstErr
		}
	}

	// Drop the DNS resolve path enforcement of any tunnel that went down.
	for _, name := range tornDown {
		h.clearDNSPathIfOwner(name)
	}
	h.maybeArmShutdownAfterTeardown("tunnel disconnected, no GUI attached")
	return ipc.Empty{}, nil
}

func (h *Helper) handleStatus(params json.RawMessage) (interface{}, error) {
	return h.statusDTO(), nil
}

func (h *Helper) handleIsConnected(params json.RawMessage) (interface{}, error) {
	return ipc.BoolResponse{Value: h.manager.IsConnected()}, nil
}

func (h *Helper) handleActiveName(params json.RawMessage) (interface{}, error) {
	return ipc.StringResponse{Value: h.manager.ActiveTunnel()}, nil
}

func (h *Helper) handleActiveTunnels(params json.RawMessage) (interface{}, error) {
	return ipc.ActiveTunnelsResponse{Names: h.manager.ActiveTunnels()}, nil
}

func (h *Helper) handleSetHealthCheck(params json.RawMessage) (interface{}, error) {
	var req ipc.SetHealthCheckRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	if h.monitor == nil {
		// Should be impossible — monitor is created unconditionally in Run().
		// If it's nil, helper init was broken; surface that instead of
		// pretending the setting was applied.
		return nil, fmt.Errorf("reconnect monitor not initialised")
	}
	h.monitor.SetHealthCheck(req.Enabled)
	h.server.Broadcast(ipc.EventSettingsChanged, ipc.SettingsChangedPayload{HealthCheck: &req.Enabled})
	return ipc.Empty{}, nil
}

func (h *Helper) handleSetPinInterface(params json.RawMessage) (interface{}, error) {
	var req ipc.SetPinInterfaceRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	if err := h.manager.SetPinInterface(req.Enabled); err != nil {
		return nil, err
	}
	h.server.Broadcast(ipc.EventSettingsChanged, ipc.SettingsChangedPayload{PinInterface: &req.Enabled})
	return ipc.Empty{}, nil
}

// handleReportSSID receives the current SSID from the GUI process.
// On macOS 14+ the helper (root LaunchDaemon) cannot read SSID via
// CoreWLAN because Location Services permission is bundle-scoped; the
// GUI holds the permission and forwards changes here.
func (h *Helper) handleReportSSID(params json.RawMessage) (interface{}, error) {
	var req ipc.ReportSSIDRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	if h.wifiMon == nil {
		// Should be impossible — wifiMon is created unconditionally in Run().
		// Surfacing the error lets the GUI know Wi-Fi rules won't fire,
		// instead of silently swallowing every SSID update.
		return nil, fmt.Errorf("wifi monitor not initialised")
	}
	// Stamp the gateway BEFORE ReportExternalSSID: its onChanged callback
	// re-evaluates automation synchronously, and the staleness check must
	// see the fresh stamp or it would invalidate the SSID we're delivering.
	gw := wifi.GatewayMAC()
	h.wifiMu.Lock()
	h.ssidStampGW = gw
	h.wifiMu.Unlock()
	h.wifiMon.ReportExternalSSID(req.SSID)
	return ipc.Empty{}, nil
}
