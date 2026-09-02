//go:build windows

package diag

import "github.com/imonior/wireguide-plus/internal/network"

// getRoutesWindows returns the IPv4 destinations routed through the
// interface with the given friendly name. It reuses the same iphlpapi
// GetIpForwardTable2 snapshot that GetRoutingTable consumes, so the
// match is consistent with the Diagnostics → Routes view and immune to
// the locale/console quirks that broke the old `route print` parser.
//
// Previously conflict detection on Windows always returned nil here,
// silently disabling routing-conflict warnings on the platform where
// full-tunnel (0.0.0.0/0) defaults make overlaps the most likely.
func getRoutesWindows(ifaceName string) []string {
	rows := network.EnumerateIPv4Routes()
	if len(rows) == 0 {
		return nil
	}
	var routes []string
	for _, r := range rows {
		if r.Interface != ifaceName {
			continue
		}
		routes = append(routes, r.Destination)
	}
	return routes
}
