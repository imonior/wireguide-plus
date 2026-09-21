package helper

import (
	"log/slog"
	"net"
	"strings"
)

// checkAddressConflicts compares every stored tunnel's Address against the
// addresses currently held by all local interfaces, and logs a prominent
// warning when some OTHER adapter already owns a tunnel address. That
// situation means another WireGuard client (the official one, typically)
// is running the same tunnel: every connect attempt then fails at the
// assign-address step with netsh's "The object already exists.", and
// before the automation back-off existed, it did so every 30 seconds.
//
// The check is read-only, runs once per helper start in its own goroutine,
// and a store error simply means "cannot check" — startup never blocks on
// it and a missing store is not worth warning about.
func (h *Helper) checkAddressConflicts() {
	if h.userTunnelStore == nil {
		return
	}
	names, err := h.userTunnelStore.List()
	if err != nil {
		return
	}
	// Tunnels this helper currently has up legitimately own their address
	// on their own interface — they are the baseline, not a conflict.
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
		if active[name] {
			continue
		}
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
					slog.Warn(
						"tunnel address is already held by another adapter — connects will fail until the conflict is resolved",
						"category", "network",
						"tunnel", name,
						"address", ip.String(),
						"conflicting_adapter", ia.name,
						"note", "another WireGuard client appears to be running the same tunnel; stop it (e.g. Stop-Service 'WireGuardTunnel$"+ia.name+"') or exclude this tunnel from automation")
				}
			}
		}
	}
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
