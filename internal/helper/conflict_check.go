package helper

import (
	"net"
	"strings"
)

// checkAddressConflicts compares every stored tunnel's Address against the
// addresses currently held by all local interfaces, and reports a prominent
// conflict when some OTHER adapter already owns a tunnel address. That
// situation means another WireGuard client (the official one, typically) is
// running the same tunnel: the two clients fight over the address, so our
// tunnel connects, gets its address stolen, drops, reconnects — the exact
// "TS453Dmini keeps connecting" loop. The check pauses that tunnel's
// automation (conflict_pause.go) and broadcasts EventAddressConflict so the
// GUI can keep an interactive dialog up until the user decides — the helper
// never forcibly disconnects or permanently opts the tunnel out on its own.
//
// The check itself is read-only, runs once per helper start in its own
// goroutine, and a store error simply means "cannot check" — startup never
// blocks on it and a missing store is not worth warning about. Conflicts
// that appear only at runtime are caught by the connect path, which
// classifies the assign-address failure and pauses through the same entry
// point.
func (h *Helper) checkAddressConflicts() {
	if h.userTunnelStore == nil {
		return
	}
	names, err := h.userTunnelStore.List()
	if err != nil {
		return
	}
	// Tunnels this helper currently has up legitimately own their address
	// on their own interface — they are the baseline, not a conflict. But a
	// tunnel we have up can STILL conflict: if another client also holds the
	// same address on a different adapter, both are "up" and fighting. So we
	// do NOT skip active tunnels here — the adapter-name exclusion below is
	// what separates "our own interface" from "someone else's".
	active := make(map[string]bool)
	for _, n := range h.manager.ActiveTunnels() {
		active[n] = true
	}
	// Snapshot each interface's addresses once; tunnels far outnumber
	// adapters and the enumeration is the expensive part.
	ifaces, err := net.Interfaces()
	if err != nil {
		return
	}
	type ifaceAddrs struct {
		name string
		ips  []net.IP
	}
	snapshot := make([]ifaceAddrs, 0, len(ifaces))
	for i := range ifaces {
		addrs, err := ifaces[i].Addrs()
		if err != nil {
			continue
		}
		ia := ifaceAddrs{name: ifaces[i].Name}
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok {
				ia.ips = append(ia.ips, ipnet.IP)
			}
		}
		snapshot = append(snapshot, ia)
	}

	for _, name := range names {
		cfg, err := h.userTunnelStore.Load(name)
		if err != nil || cfg == nil {
			continue
		}
		for _, addrCIDR := range cfg.Interface.Address {
			ip, _, err := net.ParseCIDR(addrCIDR)
			if err != nil || ip == nil {
				continue
			}
			for _, ia := range snapshot {
				if isOwnTunnelIface(ia.name) {
					continue
				}
				for _, held := range ia.ips {
					if !held.Equal(ip) {
						continue
					}
					state := "down"
					if active[name] {
						state = "active"
					}
					// One funnel: pauseForAddressConflict logs the detail,
					// pauses this tunnel's automation, and broadcasts the
					// dialog event (deduplicated by pause transition).
					h.pauseForAddressConflict(name, addrCIDR, ia.name, state)
				}
			}
		}
	}
}

// inferConflictingSoftware guesses which client owns an adapter that holds
// one of our tunnel addresses. Best-effort from the adapter name: the official
// Windows WireGuard service creates "WireGuardTunnel$<name>" adapters; the
// official macOS/Linux clients use "utunN". Anything else is reported as
// "unknown" rather than guessed — the dialog shows the raw adapter name either
// way so the user can identify it.
func inferConflictingSoftware(adapter string) string {
	if strings.HasPrefix(adapter, "WireGuardTunnel$") {
		return "WireGuard (official client)"
	}
	if strings.HasPrefix(adapter, "utun") {
		return "WireGuard (official client)"
	}
	return "unknown"
}

// isOwnTunnelIface reports whether an adapter name belongs to one of this
// app's own tunnel interfaces ("WireGuidePlus-<hash>" — the naming the
// engine uses on every platform it creates a named adapter). Their
// addresses are the normal result of an active tunnel, not conflicts.
// Interfaces with other names (a utun created by the official client, a
// physical LAN adapter) are all candidates for a genuine conflict.
func isOwnTunnelIface(name string) bool {
	return strings.HasPrefix(name, "WireGuidePlus-")
}
