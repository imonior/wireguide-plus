package tunnel

import "log/slog"

// SetPinInterface flips the "bind tunnel traffic to a physical interface"
// master switch (Settings → pin_interface) and propagates it to every
// active tunnel's NetworkManager.
//
// The feature has two platform implementations, so an error here must mean
// "this platform cannot do it at all" — not "no active tunnel happens to
// implement the darwin-only -ifscope hook":
//
//   - macOS  : NetworkManager.SetPinInterface(bool) — -ifscope bypass routes.
//   - Windows: per-tunnel BindIfIndex (IP_UNICAST_IF socket pinning), read
//     from the tunnel's meta sidecar by the helper at connect time.
//   - Linux  : per-tunnel BindIfName (explicit `ip route ... dev <iface>`).
//
// The Windows/Linux binding is resolved when a tunnel CONNECTS, so toggling
// the switch while tunnels are up does not rebind them live; it only takes
// effect on the next connect (or reconnect). That is expected, not a
// failure — returning an error here made the GUI show "not supported on
// this platform" and roll the toggle back, which was simply wrong.
func (m *Manager) SetPinInterface(enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pinInterface = enabled

	var active, applied int
	for _, e := range m.tunnels {
		if e.netMgr == nil {
			continue
		}
		active++
		if dm, ok := e.netMgr.(interface{ SetPinInterface(bool) }); ok {
			dm.SetPinInterface(enabled)
			applied++
		}
	}
	// Informational only. On Windows/Linux the per-tunnel binding is
	// applied at connect time from the meta sidecar, so there is nothing
	// to push into a running NetworkManager — `applied == 0` is normal.
	if active > 0 && applied == 0 {
		slog.Info("pin-interface toggle: active tunnels keep their current egress binding until reconnect",
			"enabled", enabled, "active", active)
	}
	return nil
}
