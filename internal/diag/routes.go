package diag

import (
	"context"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/imonior/wireguide-plus/internal/sysexec"
)

// routeCmdTimeout bounds the route-table-listing commands. These are called
// from the diagnostics UI; a hung command would freeze the helper.
const routeCmdTimeout = 10 * time.Second

func runRouteCmd(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), routeCmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	sysexec.Hide(cmd)
	return cmd.CombinedOutput()
}

// RouteEntry represents a single routing table entry.
type RouteEntry struct {
	Destination string `json:"destination"`
	Gateway     string `json:"gateway"`
	Interface   string `json:"interface"`
	Flags       string `json:"flags"`
	// IsVPN is set by the app layer when this route's interface matches
	// a currently active tunnel interface. The diagnostics UI uses it to
	// distinguish "through the tunnel" routes from direct ones.
	IsVPN bool `json:"is_vpn,omitempty"`
}

// GetRoutingTable returns the current OS routing table.
func GetRoutingTable() ([]RouteEntry, error) {
	switch runtime.GOOS {
	case "darwin":
		return getRoutesDarwinFull()
	case "linux":
		return getRoutesLinuxFull()
	case "windows":
		return getRoutesWindowsFull()
	default:
		return nil, nil
	}
}

func getRoutesDarwinFull() ([]RouteEntry, error) {
	// Run both `inet` and `inet6` so IPv6 routes (Tailscale, full
	// IPv6 tunnels, ULA prefixes) show up in diagnostics. Without
	// `-f inet6` an IPv6-only tunnel was completely invisible.
	v4, err := runRouteCmd("netstat", "-rn", "-f", "inet")
	if err != nil {
		return nil, err
	}
	v6, err := runRouteCmd("netstat", "-rn", "-f", "inet6")
	if err != nil {
		// Non-fatal: IPv6 may be disabled on this system. Return
		// just the v4 routes rather than the whole call failing.
		return parseDarwinRouteOutput(string(v4)), nil
	}
	routes := parseDarwinRouteOutput(string(v4))
	routes = append(routes, parseDarwinRouteOutput(string(v6))...)
	return routes, nil
}

func parseDarwinRouteOutput(out string) []RouteEntry {
	var routes []RouteEntry
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		// Skip header / banner lines (`Internet:`, `Internet6:`, etc.)
		if fields[0] == "Destination" || fields[0] == "Routing" ||
			strings.HasPrefix(fields[0], "Internet") {
			continue
		}
		entry := RouteEntry{
			// macOS netstat elides trailing ".0" octets for network routes
			// (127.0.0.0/8 prints as "127", 169.254.0.0/16 as "169.254",
			// 192.168.1.0/24 as "192.168.1"); expand them back to canonical
			// dotted-quad + prefix so the diagnostics UI shows unambiguous
			// destinations instead of truncated-looking "127".
			Destination: expandDarwinNetAddr(fields[0]),
			Gateway:     fields[1],
		}
		if len(fields) > 2 {
			entry.Flags = fields[2]
		}
		if len(fields) > 3 {
			entry.Interface = fields[3]
		}
		routes = append(routes, entry)
	}
	return routes
}

// expandDarwinNetAddr normalizes macOS netstat's compressed IPv4 network
// notation back into canonical dotted-quad + prefix form. `netstat -rn
// -f inet` on macOS/FreeBSD prints network routes with the trailing zero
// octets elided:
//
//	127.0.0.0/8   → "127"
//	169.254.0.0/16 → "169.254"
//	192.168.1.0/24 → "192.168.1"
//
// Host addresses (127.0.0.1), IPv6 addresses, "default" and link-layer
// names (link#4) pass through untouched.
func expandDarwinNetAddr(s string) string {
	// IPv6 (contains ':') and anything with a netmask/prefix already
	// present are left alone.
	if strings.Contains(s, ":") || strings.Contains(s, "/") {
		return s
	}
	parts := strings.Split(s, ".")
	if len(parts) > 3 {
		// Full dotted-quad — already canonical.
		return s
	}
	// 1..3 dotted-decimal octets: verify every octet is pure digits so
	// "default", "link#4", "fe80" etc. never get mangled.
	octets := len(parts)
	for _, p := range parts {
		if p == "" {
			return s
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return s
			}
		}
	}
	for len(parts) < 4 {
		parts = append(parts, "0")
	}
	return strings.Join(parts, ".") + "/" + strconv.Itoa(octets*8)
}

func getRoutesLinuxFull() ([]RouteEntry, error) {
	out, err := runRouteCmd("ip", "route", "show")
	if err != nil {
		return nil, err
	}
	var routes []RouteEntry
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		entry := RouteEntry{}
		fields := strings.Fields(line)
		if len(fields) > 0 {
			entry.Destination = fields[0]
		}
		for i, f := range fields {
			if f == "via" && i+1 < len(fields) {
				entry.Gateway = fields[i+1]
			}
			if f == "dev" && i+1 < len(fields) {
				entry.Interface = fields[i+1]
			}
		}
		routes = append(routes, entry)
	}
	return routes, nil
}

// getRoutesWindowsFull is defined in routes_windows.go so the iphlpapi
// dependency stays platform-scoped. The previous implementation here
// parsed `route print -4` output and produced an empty list on at least
// one user's machine; the iphlpapi path uses the same kernel API that
// PowerShell's Get-NetRoute calls and is immune to console-process
// quirks and locale differences in the route.exe output.
