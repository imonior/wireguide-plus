//go:build windows

package diag

import "github.com/imonior/wireguide-plus/internal/network"

// getRoutesWindowsFull enumerates the IPv4 routing table via iphlpapi
// (GetIpForwardTable2), the same kernel API PowerShell's Get-NetRoute
// uses. We previously parsed `route print -4` output here but the GUI
// process (where this runs) silently produced an empty list on at
// least one user's machine, and the parser was also dependent on the
// console binary's locale-specific column layout. iphlpapi is
// locale-independent, allocates no console child (no conhost flash),
// and is the same code path the reconnect detector already trusts for
// default-route lookup.
func getRoutesWindowsFull() ([]RouteEntry, error) {
	rows := network.EnumerateIPv4Routes()
	// Go's net.Interface Name on Windows IS the adapter FriendlyName the
	// iphlpapi path reports, so the address map keys match directly.
	addrs := buildIfaceAddrs()
	out := make([]RouteEntry, 0, len(rows))
	for _, r := range rows {
		// The scheme renders the IPv4 default route as "default" rather
		// than the synthetic 0.0.0.0/0.
		dest := r.Destination
		if dest == "0.0.0.0/0" || dest == "::/0" {
			dest = "default"
		}
		// Keep only real unicast forwarding routes.
		if !shouldKeepRoute(dest, r.Interface, 4, addrs) {
			continue
		}
		kind, detail := classifyIface(r.Interface, "")
		out = append(out, RouteEntry{
			Destination:     dest,
			Gateway:         resolveOnLinkGateway(r.Gateway),
			Interface:       r.Interface,
			InterfaceType:   kind,
			InterfaceDetail: detail,
		})
	}
	return out, nil
}
