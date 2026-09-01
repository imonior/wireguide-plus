//go:build !windows

package tunnel

import "time"

// getStatusForEngine on Linux/Darwin keeps the wgctrl path so Linux can
// continue to use the kernel netlink interface (much faster than the
// userspace UAPI text protocol) and Darwin can keep its already-working
// /var/run/wireguard/<iface>.sock socket. Only Windows hits the named-
// pipe ACL problem that forces the in-process IpcGet path.
//
// AmneziaWG tunnels always take the in-process path: they have no
// external UAPI socket (we deliberately skip creating one — the `wg` CLI
// can't talk to an AWG device anyway), and wgctrl's netlink path only
// understands standard WireGuard devices.
func getStatusForEngine(engine *Engine, tunnelName string, connectedAt time.Time) (*ConnectionStatus, error) {
	if engine.IsAmneziaWG() {
		return GetStatusFromEngine(engine, tunnelName, connectedAt)
	}
	return GetStatus(engine.InterfaceName(), tunnelName, connectedAt)
}
