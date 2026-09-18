package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"sort"
	"strings"
	"time"

	"github.com/imonior/wireguide-plus/internal/config"
	"github.com/imonior/wireguide-plus/internal/diag"
	"github.com/imonior/wireguide-plus/internal/domain"
	"github.com/imonior/wireguide-plus/internal/ipc"
	"github.com/imonior/wireguide-plus/internal/storage"
)

// ListTunnelsLocal returns stored tunnels WITHOUT asking the helper which one
// is active — callers that already know the active name (e.g. the system
// tray, which tracks it from the status event stream) should use this to
// avoid an IPC round-trip on every refresh. IsConnected is always false in
// the returned slice; the caller is responsible for applying its own
// active-name match.
func (s *TunnelService) ListTunnelsLocal() ([]TunnelInfo, error) {
	names, err := s.tunnelStore.List()
	if err != nil {
		return nil, err
	}
	lastUsed := s.lastUsedByTunnel()
	var result []TunnelInfo
	for _, name := range names {
		cfg, meta, err := s.tunnelStore.LoadWithMeta(name)
		if err != nil {
			slog.Warn("skipping broken tunnel config", "name", name, "error", err)
			continue
		}
		endpoint := ""
		if len(cfg.Peers) > 0 {
			endpoint = cfg.Peers[0].Endpoint
		}
		notes := ""
		var probeTargets []string
		if meta != nil {
			notes = meta.Notes
			probeTargets = meta.ProbeTargets()
		}
		// Date-added: the stamped creation time (survives edits, issue #17);
		// mtime fallback only for tunnels created before stamping existed.
		// meta is already loaded — no second sidecar read.
		created := int64(0)
		if meta != nil {
			created = meta.CreatedUnix
		}
		if created == 0 {
			created = s.tunnelStore.ModTimeUnix(name)
		}
		result = append(result, TunnelInfo{
			Name:                name,
			Endpoint:            endpoint,
			Notes:               notes,
			LatencyProbeTargets: probeTargets,
			Protocol:            cfg.Protocol,
			CreatedAtUnix:       created,
			LastUsedUnix:        lastUsed[name],
		})
	}
	return result, nil
}

// lastUsedByTunnel maps each tunnel name to the Unix time of its most
// recent connection start, from history. Missing tunnels get the zero
// value (0 = never connected). Computed once per list call.
func (s *TunnelService) lastUsedByTunnel() map[string]int64 {
	out := map[string]int64{}
	if s.historyStore == nil {
		return out
	}
	for _, sess := range s.historyStore.GetAll() {
		if t := sess.StartTime.Unix(); t > out[sess.TunnelName] {
			out[sess.TunnelName] = t
		}
	}
	return out
}

// SetTunnelNotes persists a freeform note for a tunnel. Empty notes still
// write an empty .meta.json — that matches the contract the frontend
// expects (write always succeeds, no special-case for "clear").
//
// Existence check first to avoid orphaning a .meta.json sidecar after the
// .conf was deleted: TunnelDetail's onDestroy can fire-and-forget a
// pending-edit flush after the user deletes a tunnel, and writing the
// sidecar in that window would leave a stale file the next tunnel-of-the-
// same-name would inherit.
func (s *TunnelService) SetTunnelNotes(name, notes string) error {
	if !s.tunnelStore.Exists(name) {
		return fmt.Errorf("tunnel %q does not exist", name)
	}
	return s.tunnelStore.UpdateMeta(name, func(meta *storage.TunnelMeta) {
		meta.Notes = notes
	})
}

// SetTunnelLatencyProbeTargets persists the four-slot probe-target list used
// only for latency display. The values are deliberately stored outside the
// WireGuard .conf so exports remain compatible with other clients.
//
// Returns the address each slot resolved to ("" for an empty slot or a
// literal IP), so the editor can show what a hostname points at the moment
// it is saved. Resolving on load instead would mean a DNS lookup per tunnel
// just to open the panel — the live value is refreshed by the helper's probe
// cycle anyway.
//
// Slots 0/1 are the positional public-probe overrides. On a split tunnel
// they are hidden by the editor and ignored by the probe planner, so their
// validation is skipped too: rejecting 8.8.8.8 there would make the stored
// default unsaveable the moment the tunnel stops being a full tunnel. The
// editor sends the stored values back unchanged for those rows, so hiding
// them never wipes them.
func (s *TunnelService) SetTunnelLatencyProbeTargets(name string, targets []string) ([]string, error) {
	if !s.tunnelStore.Exists(name) {
		return nil, fmt.Errorf("tunnel %q does not exist", name)
	}
	slots := make([]string, storage.ProbeTargetSlotCount)
	for i := range slots {
		if i < len(targets) {
			slots[i] = strings.TrimSpace(targets[i])
		}
	}

	// Full-tunnel status decides both which slots are meaningful and whether
	// AllowedIPs has to be honoured. A config that cannot be read is treated
	// as a full tunnel for validation purposes — the probe planner does the
	// same, and blocking a save on a read error would be worse than allowing
	// one that turns out to be unprobeable.
	fullTunnel := true
	if cfg, _, err := s.tunnelStore.LoadWithMeta(name); err == nil && cfg != nil {
		fullTunnel = tunnelIsFullRoute(cfg)
	}

	resolved := make([]string, storage.ProbeTargetSlotCount)
	for i, slot := range slots {
		if slot == "" {
			continue
		}
		if !config.IsValidHostOrIP(slot) {
			return nil, fmt.Errorf("latency target %q is not a valid IP address or hostname", slot)
		}
		// Resolve while the user is still looking at the field: the resolved
		// address is what a split tunnel can be checked against AllowedIPs
		// with, and what the probe rows display (a DDNS peer silently moving
		// otherwise looks like a latency spike).
		addr, err := resolveProbeTarget(slot)
		if err != nil {
			return nil, fmt.Errorf("target %d: %w", i+1, err)
		}
		resolved[i] = addr
		// Slots 0/1 only take effect on a full tunnel (see above).
		if i < 2 && !fullTunnel {
			continue
		}
		// Enforce before writing: on a split tunnel an address outside
		// AllowedIPs is not merely misleading, it is meaningless — the probe
		// never enters the tunnel, so the "tunnel latency" reading would
		// describe the plain internet path.
		if err := s.validateProbeTarget(name, slot, addr); err != nil {
			return nil, fmt.Errorf("target %d: %w", i+1, err)
		}
	}

	if err := s.tunnelStore.UpdateMeta(name, func(meta *storage.TunnelMeta) {
		meta.SetProbeTargets(slots)
		meta.LatencyProbeResolved = resolved[2]
	}); err != nil {
		return nil, err
	}

	// Re-probe now, not on the next 30s tick: the row the user just typed
	// has to show a reading while they are still looking at it. The helper
	// re-reads the targets from disk each cycle, so this picks up exactly
	// what was written above.
	//
	// Best-effort on purpose. The helper may not be running (nothing is
	// connected, nothing to probe), and an IPC failure here must not fail
	// a save that has already been committed to disk.
	if err := s.call(ipc.MethodProbeNow, nil, nil); err != nil {
		slog.Debug("probe-now after latency target save failed", "tunnel", name, "error", err)
	}
	return resolved, nil
}

// tunnelIsFullRoute reports whether any peer claims the default route.
func tunnelIsFullRoute(cfg *domain.WireGuardConfig) bool {
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

// probeResolutionTimeout bounds DNS on the write path. Saving a setting must
// not hang because a resolver is unreachable — and a probe target that
// cannot be resolved right now is not a usable probe target anyway.
const probeResolutionTimeout = 4 * time.Second

// resolveProbeTarget returns the address a probe target points at, or "" when
// the target is empty or already a literal IP (nothing to resolve).
//
// Hostnames are resolved eagerly, on save, so the value the user sees is the
// value that gets probed and checked. A name with no DNS answer is rejected:
// an unresolvable target would silently produce a permanent "—" row.
func resolveProbeTarget(target string) (string, error) {
	if target == "" {
		return "", nil
	}
	host := target
	if h, _, err := net.SplitHostPort(target); err == nil && h != "" {
		host = h
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return "", nil // literal IP — nothing to resolve
	}
	ctx, cancel := context.WithTimeout(context.Background(), probeResolutionTimeout)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil || len(addrs) == 0 {
		return "", fmt.Errorf("latency target %q could not be resolved: %w", target, err)
	}
	return addrs[0], nil
}

// validateProbeTarget rejects probe addresses that cannot be routed through
// the tunnel.
//
// Rules, matching the UI:
//   - Full tunnel (some peer claims 0.0.0.0/0 or ::/0): anything goes —
//     all traffic is routed through the tunnel anyway.
//   - Split tunnel: the address must fall inside AllowedIPs. Otherwise the
//     probe bypasses the tunnel entirely and its RTT says nothing about it,
//     so the value is refused rather than saved.
//
// Hostnames are resolved first (see resolveProbeTarget) and judged by the
// resolved address, which is the only point at which the check is meaningful.
func (s *TunnelService) validateProbeTarget(name, target, resolved string) error {
	if target == "" {
		return nil
	}
	addr, err := netip.ParseAddr(target)
	if err != nil {
		if resolved == "" {
			return nil // nothing to judge — resolveProbeTarget already errored
		}
		addr, err = netip.ParseAddr(resolved)
		if err != nil {
			return nil
		}
	}
	cfg, _, err := s.tunnelStore.LoadWithMeta(name)
	if err != nil || cfg == nil {
		return nil // config unreadable: don't block the user on a read error
	}
	for _, peer := range cfg.Peers {
		for _, allowed := range peer.AllowedIPs {
			if allowed == "0.0.0.0/0" || allowed == "::/0" {
				return nil
			}
			if prefix, perr := netip.ParsePrefix(allowed); perr == nil {
				if prefix.Contains(addr) {
					return nil
				}
				continue
			}
			// Bare address in AllowedIPs means "exactly this host".
			if host, aerr := netip.ParseAddr(allowed); aerr == nil && host == addr {
				return nil
			}
		}
	}
	slog.Warn("rejected latency probe target outside AllowedIPs",
		"tunnel", name, "target", target, "resolved", resolved)
	return fmt.Errorf("latency target %s is outside this tunnel's AllowedIPs — probes to it would bypass the tunnel, pick an address inside the tunnel's routed ranges", addr)
}

// ListTunnels returns every stored tunnel with its summary info.
//
// The active-tunnel marker used to come from an IPC round-trip on every call.
// That made the tray's rebuild-menu path slow when it was being invoked on
// the status event stream. The frontend now learns the active tunnel from
// the status event itself, and the tray caches it internally — so this
// function stays fully local (disk-only, no IPC) and returns IsConnected
// purely as a best-effort flag based on a single established-tunnels probe
// that is safe to skip entirely on slow paths.
func (s *TunnelService) ListTunnels() ([]TunnelInfo, error) {
	names, err := s.tunnelStore.List()
	if err != nil {
		return nil, err
	}

	// One cheap probe for the established tunnels — used by the frontend's
	// initial load before it has received its first status event. The tray no
	// longer relies on this (it tracks active tunnels via the status stream).
	//
	// Established (setup completed), not active: ActiveTunnels also contains
	// tunnels whose connect attempt is still running, and labelling those
	// connected paints a green badge on a tunnel that is about to fail —
	// exactly the "it said connected, then dropped" confusion from a
	// boot-time DNS miss.
	established := map[string]bool{}
	var est ipc.ActiveTunnelsResponse
	if err := s.call(ipc.MethodEstablishedTunnels, nil, &est); err == nil {
		for _, n := range est.Names {
			established[n] = true
		}
	} else {
		// Helper older than this method: fall back to the active list
		// rather than reporting everything as down.
		var activeTunnels ipc.ActiveTunnelsResponse
		if err := s.call(ipc.MethodActiveTunnels, nil, &activeTunnels); err == nil {
			for _, n := range activeTunnels.Names {
				established[n] = true
			}
		}
	}

	lastUsed := s.lastUsedByTunnel()
	var result []TunnelInfo
	for _, name := range names {
		cfg, meta, err := s.tunnelStore.LoadWithMeta(name)
		if err != nil {
			slog.Warn("skipping broken tunnel config", "name", name, "error", err)
			continue
		}
		endpoint := ""
		if len(cfg.Peers) > 0 {
			endpoint = cfg.Peers[0].Endpoint
		}
		notes := ""
		var probeTargets []string
		if meta != nil {
			notes = meta.Notes
			probeTargets = meta.ProbeTargets()
		}
		// Date-added: the stamped creation time (survives edits, issue #17);
		// mtime fallback only for tunnels created before stamping existed.
		// meta is already loaded — no second sidecar read.
		created := int64(0)
		if meta != nil {
			created = meta.CreatedUnix
		}
		if created == 0 {
			created = s.tunnelStore.ModTimeUnix(name)
		}
		result = append(result, TunnelInfo{
			Name:                name,
			IsConnected:         established[name],
			Endpoint:            endpoint,
			Notes:               notes,
			LatencyProbeTargets: probeTargets,
			Protocol:            cfg.Protocol,
			CreatedAtUnix:       created,
			LastUsedUnix:        lastUsed[name],
		})
	}
	return result, nil
}

// CheckConflicts loads a tunnel's config and scans local network interfaces
// for routing overlaps (e.g. Tailscale, another WireGuard instance). Runs
// entirely in the GUI process — no IPC needed. The frontend calls this before
// Connect so it can show a warning dialog if conflicts exist.
func (s *TunnelService) CheckConflicts(name string) ([]diag.ConflictInfo, error) {
	cfg, err := s.tunnelStore.Load(name)
	if err != nil {
		return nil, fmt.Errorf("loading tunnel %s: %w", name, err)
	}
	var allowedIPs []string
	for _, peer := range cfg.Peers {
		allowedIPs = append(allowedIPs, peer.AllowedIPs...)
	}
	// A tunnel can't conflict with itself — skip the interface it is
	// already running on, otherwise every one of its own CIDRs is
	// reported as overlapping (each contains itself) whenever the user
	// reconnects or the app restores the previous session.
	conflicts, err := diag.CheckConflicts(allowedIPs, cfg.Interface.Address, s.tunnelInterfaces(name)...)
	if err != nil {
		slog.Warn("conflict check failed", "tunnel", name, "error", err)
		// Non-fatal — don't block connect if the scan itself fails.
		return nil, nil
	}
	return conflicts, nil
}

// tunnelInterfaces returns the interface names the given tunnel is
// currently up on — usually zero (tunnel down) or one. The conflict
// scan skips them so a tunnel is never compared against its own
// routes, which would report each of its CIDRs as overlapping itself.
// A failed status query yields "no interfaces to skip", i.e. the
// previous behaviour; the scan itself is best-effort.
func (s *TunnelService) tunnelInterfaces(name string) []string {
	if name == "" {
		return nil
	}
	st, err := s.GetStatus()
	if err != nil || st == nil {
		return nil
	}
	all := make([]ConnectionStatus, 0, 1+len(st.Tunnels))
	all = append(all, *st)
	all = append(all, st.Tunnels...)
	var out []string
	for _, t := range all {
		if t.TunnelName == name && t.InterfaceName != "" {
			out = append(out, t.InterfaceName)
		}
	}
	return out
}

// Connect loads a tunnel config from local storage and asks the helper to
// bring it up. The helper re-validates server-side.
//
// Session lifecycle is owned entirely by ReconcileHistoryFromStatus —
// opening here would race the helper's StateConnecting status broadcast,
// which already includes the tunnel name in ActiveTunnels. The race
// produced spurious "Reconnected" rows on every user-initiated connect
// before this was unified.
func (s *TunnelService) Connect(name string) error {
	cfg, err := s.tunnelStore.Load(name)
	if err != nil {
		return fmt.Errorf("loading tunnel %s: %w", name, err)
	}

	// Inject the WireGuard-scripts opt-in from user settings. The helper
	// executes the config's Pre/PostUp/Down hooks only when this flag is
	// set — it stays OFF by default because scripts run as root/admin
	// (Settings shows a prominent warning when enabling).
	if st, err := s.settingsStore.Load(); err == nil && st != nil {
		cfg.EnableScripts = st.EnableWgScripts
		// Fast-fail AWG tunnels when the user has disabled AmneziaWG
		// support (Settings → Advanced). The helper re-checks this
		// authoritatively in doConnectHeld, so CLI and automation
		// connects are covered even if this gate is ever bypassed.
		if cfg.Protocol == domain.ProtocolAmneziaWG && !st.EnableAWG {
			return fmt.Errorf("%s — enable it in Settings → Advanced to connect this tunnel", domain.ErrAWGDisabled)
		}
	}

	// A manual connect latches the tunnel ON: the user has spoken, so the
	// automation engine must not silently tear it back down until they
	// disconnect it by hand once or the app restarts. This also clears any
	// manual-off latch (the two are mutually exclusive), so a manual
	// reconnect after a manual disconnect restores normal automation.
	// Done before the IPC so a failed attempt still leaves the latch set
	// consistently with the user's intent.
	s.setManualOn(name)

	slog.Info("tunnel: connect requested",
		"category", "tunnel",
		"tunnel", name,
		"protocol", cfg.Protocol,
		"scripts", cfg.EnableScripts,
		"endpoints", strings.Join(cfg.Endpoints(), ","))

	// Mark the RPC as in-flight so the health monitor doesn't falsely
	// detect helper death while the server is busy processing Connect
	// (which blocks the per-connection request loop, preventing pings).
	s.clients.MarkInflight()
	defer s.clients.UnmarkInflight()

	err = s.callLong(ipc.MethodConnect, ipc.ConnectRequest{
		Config: cfg,
	}, nil)
	if err != nil {
		// Warn, not Info: a failed connect is always worth noticing, and
		// the GUI surfaces its own toast — the log keeps the reason.
		slog.Warn("tunnel: connect failed",
			"category", "tunnel",
			"tunnel", name,
			"error", err)
		return err
	}
	slog.Info("tunnel: connected",
		"category", "tunnel",
		"tunnel", name,
		"protocol", cfg.Protocol)
	return nil
}

// Disconnect tears down whatever tunnel the helper currently has active.
// If the call fails with a "client closed" error (the health monitor may have
// swapped the client during a recovery), retry once with the fresh client.
//
// We mark lastKnownStats with reason "user" so the upcoming Reconcile (after
// the next status event with the tunnel gone) labels the closed session as
// a user disconnect. Existing fresh rx/tx counters from Reconcile's per-tick
// refresh are preserved — markUserDisconnect overwrites only the reason,
// not the bytes. This is what protects against rapid double-clicks: the
// second Disconnect's snapshot may be 0/0 (helper already tearing down)
// but the cache still holds the fresh values from the steady state.
func (s *TunnelService) Disconnect() error {
	name, rx, tx := s.snapshotActiveStats("")
	s.markUserDisconnect(name, rx, tx)
	slog.Info("tunnel: disconnect requested", "category", "tunnel", "tunnel", name)
	// Manual off wins over automation: latch every tunnel this tears down
	// so rules don't silently reconnect them until each is reconnected by
	// hand once or the app restarts.
	if name != "" {
		s.setManualOff(name)
	}

	s.clients.MarkInflight()
	defer s.clients.UnmarkInflight()
	err := s.callLong(ipc.MethodDisconnect, nil, nil)
	if err != nil && isClientClosed(err) {
		slog.Info("disconnect got client-closed, retrying with fresh client")
		err = s.callLong(ipc.MethodDisconnect, nil, nil)
	}
	if err != nil {
		// IPC failed — the "user" hint is suspect (helper might still
		// have the tunnel up). Clear the hint, KEEP the rx/tx so a
		// genuine helper-driven close still has accurate counters.
		s.clearUserDisconnect(name)
		slog.Warn("tunnel: disconnect failed", "category", "tunnel", "tunnel", name, "error", err)
		return err
	}
	slog.Info("tunnel: disconnected", "category", "tunnel", "tunnel", name)
	return nil
}

// DisconnectTunnel disconnects a specific tunnel by name. Mirrors
// Disconnect()'s "client closed" retry: a helper recovery during a
// per-tunnel disconnect (e.g. user clicks the tray's per-tunnel
// item right when the health monitor swaps clients) should be
// transparent, not surfaced as a confusing error.
func (s *TunnelService) DisconnectTunnel(name string) error {
	_, rx, tx := s.snapshotActiveStats(name)
	s.markUserDisconnect(name, rx, tx)
	slog.Info("tunnel: disconnect requested", "category", "tunnel", "tunnel", name)
	// Manual off wins over automation: latch the tunnel so its rules
	// don't silently reconnect it until the user reconnects it by hand
	// once or the app restarts.
	s.setManualOff(name)

	s.clients.MarkInflight()
	defer s.clients.UnmarkInflight()
	err := s.callLong(ipc.MethodDisconnect, ipc.DisconnectRequest{TunnelName: name}, nil)
	if err != nil && isClientClosed(err) {
		slog.Info("disconnect-tunnel got client-closed, retrying with fresh client",
			"tunnel", name)
		err = s.callLong(ipc.MethodDisconnect, ipc.DisconnectRequest{TunnelName: name}, nil)
	}
	if err != nil {
		s.clearUserDisconnect(name)
		slog.Warn("tunnel: disconnect failed", "category", "tunnel", "tunnel", name, "error", err)
		return err
	}
	slog.Info("tunnel: disconnected", "category", "tunnel", "tunnel", name)
	return nil
}

// markUserDisconnect sets the reason hint on a tunnel's lastKnownStats to
// "user", preserving any rx/tx the Reconcile-driven cache refresh has
// already recorded. snapRx/snapTx is a fallback for the cold case (no
// cache entry yet — first Disconnect before any status event was reconciled).
func (s *TunnelService) markUserDisconnect(name string, snapRx, snapTx int64) {
	if name == "" {
		return
	}
	if cached, ok := s.lastKnownStats.Load(name); ok {
		if st, ok := cached.(lastKnownTunnelStats); ok {
			s.lastKnownStats.Store(name, lastKnownTunnelStats{rx: st.rx, tx: st.tx, reason: "user"})
			return
		}
	}
	s.lastKnownStats.Store(name, lastKnownTunnelStats{rx: snapRx, tx: snapTx, reason: "user"})
}

// clearUserDisconnect removes the "user" reason hint after a failed
// Disconnect IPC, leaving rx/tx intact. Other reasons (or empty) are left
// alone — only the hint we set ourselves is cleared.
func (s *TunnelService) clearUserDisconnect(name string) {
	if name == "" {
		return
	}
	if cached, ok := s.lastKnownStats.Load(name); ok {
		if st, ok := cached.(lastKnownTunnelStats); ok && st.reason == "user" {
			s.lastKnownStats.Store(name, lastKnownTunnelStats{rx: st.rx, tx: st.tx, reason: ""})
		}
	}
}

// setManualOff records in settings that the user manually switched a
// tunnel off. The automation engine (helper) reads this list fresh on
// every evaluation, so once it's persisted no rule can reconnect the
// tunnel until the user reconnects it by hand (a manual Connect sets the
// mirror manual-on latch, which clears this one) or the app restarts
// (ClearAllManualOverrides).
func (s *TunnelService) setManualOff(name string) {
	if name == "" || s.settingsStore == nil {
		return
	}
	if err := s.settingsStore.Update(func(cfg *storage.Settings) error {
		cfg.SetManualOff(name)
		return nil
	}); err != nil {
		slog.Warn("failed to record manual-off latch", "tunnel", name, "error", err)
		return
	}
	// Logged because the latch is the #1 reason "my automation rule didn't
	// connect the tunnel": until it is cleared, every rule is suppressed.
	slog.Info("automation: manual-off latch set — rules suppressed until a manual reconnect",
		"category", "network",
		"tunnel", name)
}

// clearManualOff removes the manual-off latch for a tunnel: a manual
// reconnect releases it back to the automation rules.
func (s *TunnelService) clearManualOff(name string) {
	if name == "" || s.settingsStore == nil {
		return
	}
	if err := s.settingsStore.Update(func(cfg *storage.Settings) error {
		cfg.ClearManualOff(name)
		return nil
	}); err != nil {
		slog.Warn("failed to clear manual-off latch", "tunnel", name, "error", err)
		return
	}
	slog.Info("automation: manual-off latch cleared — rules active again",
		"category", "network",
		"tunnel", name)
}

// setManualOn records in settings that the user manually switched a tunnel
// on. It is the mirror of setManualOff: while the latch holds, the helper's
// automation engine refuses to auto-disconnect the tunnel no matter what the
// rules say — the manual on wins until the user disconnects it by hand once
// or the app restarts. Setting it also clears any manual-off latch (the two
// are mutually exclusive).
func (s *TunnelService) setManualOn(name string) {
	if name == "" || s.settingsStore == nil {
		return
	}
	if err := s.settingsStore.Update(func(cfg *storage.Settings) error {
		cfg.SetManualOn(name)
		return nil
	}); err != nil {
		slog.Warn("failed to record manual-on latch", "tunnel", name, "error", err)
		return
	}
	slog.Info("automation: manual-on latch set — auto-disconnect suppressed until a manual disconnect",
		"category", "network",
		"tunnel", name)
}

// ClearAllManualOverrides releases every manual latch (both off and on),
// restoring full automation. The GUI calls this on startup so a fresh app
// session resumes automatic rule enforcement ("manual override only lasts
// until the app is reopened").
func (s *TunnelService) ClearAllManualOverrides() {
	if s.settingsStore == nil {
		return
	}
	if err := s.settingsStore.Update(func(cfg *storage.Settings) error {
		cfg.ClearAllManualOverrides()
		return nil
	}); err != nil {
		slog.Warn("failed to clear manual latches", "error", err)
	}
}

// snapshotActiveStats returns (tunnelName, rx, tx) for the tunnel about to
// disconnect. Empty wantName picks the primary active tunnel. Returns zero
// values on any error — capturing stats is best-effort and never blocks
// disconnect.
func (s *TunnelService) snapshotActiveStats(wantName string) (string, int64, int64) {
	status, err := s.GetStatus()
	if err != nil || status == nil {
		return wantName, 0, 0
	}
	if wantName == "" {
		if status.TunnelName != "" {
			return status.TunnelName, status.RxBytes, status.TxBytes
		}
		if len(status.Tunnels) > 0 {
			t := status.Tunnels[0]
			return t.TunnelName, t.RxBytes, t.TxBytes
		}
		return "", 0, 0
	}
	if status.TunnelName == wantName {
		return wantName, status.RxBytes, status.TxBytes
	}
	for _, t := range status.Tunnels {
		if t.TunnelName == wantName {
			return wantName, t.RxBytes, t.TxBytes
		}
	}
	return wantName, 0, 0
}

// ReconcileHistoryFromStatus is the SINGLE source of truth for opening and
// closing history sessions. The event bridge calls it on every status event;
// it diffs the active set against activeSessions and:
//
//   - opens a session for any active tunnel not currently tracked (covers
//     both user-initiated Connect and helper-driven auto-reconnect / wifi
//     rules engine / sleep-wake)
//   - closes a session for any tracked tunnel that has disappeared (using
//     cached rx/tx + cached reason from lastKnownStats; falls back to
//     disappearReason — defaults to "reconnect")
//
// Stats cache is always refreshed for currently-active tunnels so disconnect
// closes have ≤ 1 status-event-tick stale counters. Reason hints set by
// user-initiated Disconnect / DisconnectTunnel are preserved across cache
// refreshes — only LoadAndDelete on close clears them.
//
// Fast-path: if the sorted active set — including each tunnel's handshake
// state — hasn't changed since the prior call (the steady-state case at 1 Hz),
// we skip the activeSessions Range and the open-session loop. The stats cache
// still gets updated so the eventual disappear-close uses fresh counters.
//
// Handshake gating: a session is opened only once the tunnel has actually
// completed a handshake. `ActiveTunnels` also reports StateConnecting, so a
// failed attempt (DNS lookup failure, adapter creation error, rejected
// handshake) used to produce a "connected then immediately disconnected"
// history row even though the tunnel never carried traffic. Those attempts
// are now held in pendingSessions and simply dropped when they disappear —
// or promoted to a real session the moment a handshake lands.
//
// handshakeMap may be nil (older/non-GUI callers, CLI). In that case the
// handshake state is unknown and we keep the previous behaviour of opening
// the session immediately, so history is never silently lost.
func (s *TunnelService) ReconcileHistoryFromStatus(activeNames []string, handshakeMap map[string]bool, rxByTunnel, txByTunnel map[string]int64, disappearReason string) {
	if s.historyStore == nil {
		return
	}
	if disappearReason == "" {
		disappearReason = "reconnect"
	}
	active := make(map[string]struct{}, len(activeNames))
	for _, n := range activeNames {
		if n != "" {
			active[n] = struct{}{}
		}
	}

	// Refresh cache for currently-active tunnels. Always done — even on the
	// fast path — so the next disappearance close sees fresh stats. Preserve
	// any reason hint already in the cache (set by Disconnect/DisconnectTunnel
	// before the IPC call).
	for name := range active {
		var rx, tx int64
		if rxByTunnel != nil {
			rx = rxByTunnel[name]
		}
		if txByTunnel != nil {
			tx = txByTunnel[name]
		}
		reason := ""
		if cached, ok := s.lastKnownStats.Load(name); ok {
			if st, ok := cached.(lastKnownTunnelStats); ok {
				reason = st.reason
			}
		}
		s.lastKnownStats.Store(name, lastKnownTunnelStats{rx: rx, tx: tx, reason: reason})
	}

	// Build a stable signature of the active set *and* its handshake state,
	// then compare to the prior one. The handshake component matters: a
	// tunnel that appears and then completes its handshake must change the
	// signature, otherwise the fast-path skip would swallow the promotion
	// from pending to a real session.
	sig := activeSetSignature(activeNames) + "|" + handshakeSignature(activeNames, handshakeMap)
	s.reconcileMu.Lock()
	unchanged := sig == s.lastReconcileSig
	s.lastReconcileSig = sig
	s.reconcileMu.Unlock()
	if unchanged {
		return
	}

	// Close sessions for tunnels that vanished.
	s.activeSessions.Range(func(k, v any) bool {
		name, _ := k.(string)
		id, _ := v.(string)
		if _, stillActive := active[name]; stillActive {
			return true
		}
		if id != "" {
			var rx, tx int64
			reason := disappearReason
			// Prefer cached last-seen counters and reason — the current event's
			// maps don't include this tunnel since it just disappeared, and a
			// pre-set reason from user Disconnect overrides the default.
			if cached, ok := s.lastKnownStats.LoadAndDelete(name); ok {
				if st, ok := cached.(lastKnownTunnelStats); ok {
					rx = st.rx
					tx = st.tx
					if st.reason != "" {
						reason = st.reason
					}
				}
			}
			s.historyStore.RecordDisconnect(id, rx, tx, reason)
		}
		s.activeSessions.Delete(k)
		return true
	})

	// Drop vanished attempts that never completed a handshake — they never
	// carried a single packet, so recording them would inflate the history
	// with phantom seconds-long sessions.
	s.pendingSessions.Range(func(k, v any) bool {
		name, _ := k.(string)
		if _, stillActive := active[name]; !stillActive {
			s.pendingSessions.Delete(k)
		}
		return true
	})

	for _, name := range activeNames {
		if name == "" {
			continue
		}
		if _, exists := s.activeSessions.Load(name); exists {
			continue
		}
		// Gate on handshake when we know it. Unknown (nil map) ⇒ open now.
		if handshakeMap != nil && !handshakeMap[name] {
			if _, pending := s.pendingSessions.Load(name); !pending {
				slog.Debug("history: connect attempt pending handshake — session not opened yet",
					"tunnel", name)
			}
			s.pendingSessions.Store(name, struct{}{})
			continue
		}
		id := s.historyStore.RecordConnect(name)
		s.activeSessions.Store(name, id)
		// Handshake landed: the attempt is no longer pending.
		s.pendingSessions.Delete(name)
	}
}

// handshakeSignature renders "name=0/1" pairs for the active set so the
// reconcile fast path also reacts to handshake transitions.
func handshakeSignature(activeNames []string, handshakeMap map[string]bool) string {
	if handshakeMap == nil {
		return "-"
	}
	var b strings.Builder
	for _, name := range activeNames {
		if name == "" {
			continue
		}
		b.WriteString(name)
		b.WriteByte('=')
		if handshakeMap[name] {
			b.WriteByte('1')
		} else {
			b.WriteByte('0')
		}
		b.WriteByte(';')
	}
	return b.String()
}

// activeSetSignature returns a stable string representation of activeNames
// suitable for equality comparison across status events. Sorted so the
// helper's order is irrelevant; null byte separator avoids collisions
// between names like ["a", "bc"] and ["ab", "c"].
func activeSetSignature(names []string) string {
	if len(names) == 0 {
		return ""
	}
	cp := make([]string, 0, len(names))
	for _, n := range names {
		if n != "" {
			cp = append(cp, n)
		}
	}
	sort.Strings(cp)
	return strings.Join(cp, "\x00")
}

// CloseHistorySessions closes any open history sessions with the given reason.
// Called from gui.Run during shutdown so the UI doesn't show phantom "Active"
// rows after a quit.
//
// Single GetStatus probe — the previous one-IPC-per-session pattern made
// shutdown latency scale linearly with active tunnel count. The status
// response carries every active tunnel's rx/tx, so one round-trip is enough.
// Falls back to the lastKnownStats cache (and ultimately zeros) when the
// helper is already unreachable. Skipped entirely when no sessions are open
// (the common quit-while-disconnected case) so shutdown stays snappy.
func (s *TunnelService) CloseHistorySessions(reason string) {
	if s.historyStore == nil {
		return
	}
	hasOpen := false
	s.activeSessions.Range(func(_, _ any) bool {
		hasOpen = true
		return false // first hit is enough
	})
	if !hasOpen {
		// Still call CloseOpenSessions: a previous crash could have left
		// open rows in the file that the GUI never tracked.
		s.historyStore.CloseOpenSessions(reason)
		return
	}
	rxByTunnel := make(map[string]int64)
	txByTunnel := make(map[string]int64)
	if status, err := s.GetStatus(); err == nil && status != nil {
		if status.TunnelName != "" {
			rxByTunnel[status.TunnelName] = status.RxBytes
			txByTunnel[status.TunnelName] = status.TxBytes
		}
		for _, ts := range status.Tunnels {
			rxByTunnel[ts.TunnelName] = ts.RxBytes
			txByTunnel[ts.TunnelName] = ts.TxBytes
		}
	}
	s.activeSessions.Range(func(k, v any) bool {
		name, _ := k.(string)
		id, _ := v.(string)
		if id != "" {
			rx, tx := rxByTunnel[name], txByTunnel[name]
			// Fall back to last-known cache when the helper status
			// didn't include this tunnel (e.g. helper already torn
			// down the interface but the GUI's session map is fresh).
			if rx == 0 && tx == 0 {
				if cached, ok := s.lastKnownStats.Load(name); ok {
					if st, ok := cached.(lastKnownTunnelStats); ok {
						rx, tx = st.rx, st.tx
					}
				}
			}
			s.historyStore.RecordDisconnect(id, rx, tx, reason)
		}
		s.activeSessions.Delete(k)
		s.lastKnownStats.Delete(k)
		return true
	})
	s.historyStore.CloseOpenSessions(reason)
}

// GetConnectionHistory returns recorded sessions newest-first. Always returns
// a non-nil slice so the frontend doesn't have to special-case "no history
// yet" vs. "load failed".
//
// Before returning, the store is pruned to the configured
// HistoryRetentionDays so the file can't grow without bound on machines
// that connect daily.
func (s *TunnelService) GetConnectionHistory() ([]storage.Session, error) {
	if s.historyStore == nil {
		return []storage.Session{}, nil
	}
	s.pruneHistoryLocked()
	out := s.historyStore.GetAll()
	if out == nil {
		out = []storage.Session{}
	}
	return out, nil
}

// pruneHistoryLocked trims history.json to the configured retention window.
func (s *TunnelService) pruneHistoryLocked() {
	if s.settingsStore == nil {
		return
	}
	cfg, err := s.settingsStore.Load()
	if err != nil || cfg == nil {
		return
	}
	days := cfg.HistoryRetentionDays
	if days <= 0 {
		days = storage.DefaultHistoryRetentionDays
	}
	if removed := s.historyStore.TrimByAge(time.Duration(days) * 24 * time.Hour); removed > 0 {
		slog.Info("history: pruned expired sessions",
			"removed", removed, "retention_days", days)
	}
}

// ClearConnectionHistory wipes the history file.
func (s *TunnelService) ClearConnectionHistory() error {
	if s.historyStore == nil {
		return nil
	}
	return s.historyStore.Clear()
}

// isClientClosed returns true for errors caused by the IPC client being closed
// mid-call (e.g., health monitor swapped clients during recovery). Uses
// errors.Is so wrapped errors still match — substring matching would have
// false positives on unrelated errors whose messages happen to contain
// "client closed".
func isClientClosed(err error) bool {
	return errors.Is(err, ipc.ErrClientClosed)
}

// GetStatus queries the helper for the current connection status. IPC errors
// are surfaced to the caller — the frontend needs to distinguish "helper says
// disconnected" from "helper unreachable".
func (s *TunnelService) GetStatus() (*ConnectionStatus, error) {
	var status ConnectionStatus
	if err := s.call(ipc.MethodStatus, nil, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// GetTunnelDetail returns the full WireGuardConfig for a tunnel. Used by the
// detail pane to show allowed IPs, DNS, public keys, etc.
func (s *TunnelService) GetTunnelDetail(name string) (*domain.WireGuardConfig, error) {
	return s.tunnelStore.Load(name)
}

// isActiveTunnel returns true if `name` is currently up. Uses the
// multi-tunnel ActiveTunnels list, NOT ActiveName, because the
// latter only returns the lexicographically-first connected tunnel
// — which made delete/rename/update incorrectly succeed against a
// non-primary connected tunnel and orphan the live interface.
func (s *TunnelService) isActiveTunnel(name string) (bool, error) {
	var resp ipc.ActiveTunnelsResponse
	if err := s.call(ipc.MethodActiveTunnels, nil, &resp); err != nil {
		return false, err
	}
	for _, n := range resp.Names {
		if n == name {
			return true, nil
		}
	}
	return false, nil
}

// DeleteTunnel removes a tunnel from local storage. Rejects deletion of the
// currently connected tunnel (would orphan the interface).
func (s *TunnelService) DeleteTunnel(name string) error {
	active, err := s.isActiveTunnel(name)
	if err != nil {
		return fmt.Errorf("cannot verify tunnel state (helper unreachable): %w", err)
	}
	if active {
		return fmt.Errorf("cannot delete connected tunnel %q — disconnect first", name)
	}
	if err := s.tunnelStore.Delete(name); err != nil {
		return err
	}
	slog.Info("tunnel: deleted", "category", "tunnel", "tunnel", name)
	// Drop the tunnel's automation rules so they don't linger and re-attach
	// to a future same-named tunnel (issue #12). Best-effort — the tunnel
	// is already gone.
	if err := s.settingsStore.Update(func(cfg *storage.Settings) error {
		cfg.DeleteTunnelRules(name)
		return nil
	}); err != nil {
		slog.Warn("deleted tunnel but could not remove its automation rules", "tunnel", name, "error", err)
	}
	return nil
}

// RenameTunnel changes a tunnel's name. Rejects rename of the connected
// tunnel since the interface name is derived from it.
//
// Routes through the helper's Tunnel.Rename so the active-tunnel check and
// the file rename both happen under the helper's connectMu — closing the
// race where a Connect arriving between the GUI's check and the rename
// could leave the new name in activeCfgs while the file path moved.
//
// Falls back to a direct local rename if the helper rejects the method
// (older helper that hasn't been upgraded yet).
func (s *TunnelService) RenameTunnel(oldName, newName string) error {
	if err := storage.ValidateTunnelName(newName); err != nil {
		return err
	}
	if oldName == newName {
		return nil
	}
	err := s.call(ipc.MethodRename, ipc.RenameRequest{OldName: oldName, NewName: newName}, nil)
	if err != nil {
		if !isMethodNotFound(err) {
			slog.Warn("tunnel: rename failed", "category", "tunnel", "from", oldName, "to", newName, "error", err)
			return err
		}
		active, activeErr := s.isActiveTunnel(oldName)
		if activeErr != nil {
			slog.Warn("tunnel: rename failed (helper unreachable)", "category", "tunnel", "from", oldName, "to", newName, "error", activeErr)
			return fmt.Errorf("cannot verify tunnel state (helper unreachable): %w", activeErr)
		}
		if active {
			slog.Warn("tunnel: rename rejected (tunnel connected)", "category", "tunnel", "from", oldName, "to", newName)
			return fmt.Errorf("cannot rename connected tunnel %q — disconnect first", oldName)
		}
		if err := s.tunnelStore.Rename(oldName, newName); err != nil {
			slog.Warn("tunnel: rename failed (storage)", "category", "tunnel", "from", oldName, "to", newName, "error", err)
			return err
		}
	}
	slog.Info("tunnel: renamed", "category", "tunnel", "from", oldName, "to", newName)
	// Carry the tunnel's automation rules over to the new name so they
	// aren't orphaned (issue #12). Best-effort — the rename succeeded.
	if err := s.settingsStore.Update(func(cfg *storage.Settings) error {
		cfg.RenameTunnelRules(oldName, newName)
		return nil
	}); err != nil {
		slog.Warn("renamed tunnel but could not move its automation rules",
			"from", oldName, "to", newName, "error", err)
	}
	return nil
}

// isMethodNotFound classifies an IPC error as "old helper doesn't know
// this method" so callers can fall back to a local-only path.
func isMethodNotFound(err error) bool {
	if err == nil {
		return false
	}
	var coded *ipc.Error
	if errors.As(err, &coded) {
		return coded.Code == ipc.ErrCodeMethodNotFound
	}
	return false
}

// TunnelExists reports whether a tunnel with the given name is stored.
func (s *TunnelService) TunnelExists(name string) bool {
	return s.tunnelStore.Exists(name)
}
