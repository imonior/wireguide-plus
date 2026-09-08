package diag

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/imonior/wireguide-plus/internal/sysexec"
)

// conflictCmdTimeout bounds the conflict-detection commands. These run on
// every Connect; a hung command must not block the entire Connect path.
const conflictCmdTimeout = 10 * time.Second

func runConflictCmd(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), conflictCmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	sysexec.Hide(cmd)
	return cmd.CombinedOutput()
}

func runConflictCmdLC(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), conflictCmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(cmd.Environ(), "LC_ALL=C", "LANG=C")
	sysexec.Hide(cmd)
	return cmd.CombinedOutput()
}

// ConflictInfo describes a routing conflict with an existing interface.
type ConflictInfo struct {
	InterfaceName  string   `json:"interface_name"`
	Owner          string   `json:"owner"`           // "WireGuide", "Tailscale", "WireGuard", "Unknown"
	OverlappingIPs []string `json:"overlapping_ips"` // CIDRs that overlap
}

// CheckConflicts scans existing interfaces for routing conflicts
// with the given AllowedIPs. When the same logical conflict appears
// for both IPv4 and IPv6 against the same other-tunnel (e.g.
// 0.0.0.0/0 vs ::/0 against Tailscale), the entries are merged so
// the user sees one warning per conflicting interface, not two.
//
// excludeIfaces names interfaces to skip, typically the interface the
// tunnel being checked is ALREADY running on. A tunnel can never
// conflict with itself: its own routes come from its own AllowedIPs, so
// scanning them reports every one of its CIDRs as "conflicting" (each
// contains itself) — pure noise that made reconnect, session restore
// and the pre-connect dialog look broken for a tunnel that was already
// up. Pass the interface names reported by the helper's status for this
// tunnel; an empty set keeps the previous behaviour.
func CheckConflicts(newAllowedIPs []string, excludeIfaces ...string) ([]ConflictInfo, error) {
	interfaces, err := scanWireGuardInterfaces(excludeIfaces...)
	if err != nil {
		return nil, err
	}

	merged := make(map[string]*ConflictInfo)
	var order []string
	for _, iface := range interfaces {
		overlaps := findOverlaps(newAllowedIPs, iface.Routes)
		if len(overlaps) == 0 {
			continue
		}
		key := iface.Name + "\x00" + iface.Owner
		if existing, ok := merged[key]; ok {
			existing.OverlappingIPs = appendUnique(existing.OverlappingIPs, overlaps)
			continue
		}
		ci := &ConflictInfo{
			InterfaceName:  iface.Name,
			Owner:          iface.Owner,
			OverlappingIPs: overlaps,
		}
		merged[key] = ci
		order = append(order, key)
	}
	conflicts := make([]ConflictInfo, 0, len(order))
	for _, key := range order {
		conflicts = append(conflicts, *merged[key])
	}
	return conflicts, nil
}

func appendUnique(dst, src []string) []string {
	seen := make(map[string]struct{}, len(dst))
	for _, s := range dst {
		seen[s] = struct{}{}
	}
	for _, s := range src {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		dst = append(dst, s)
	}
	return dst
}

// ExistingInterface represents a detected WireGuard-like interface.
type ExistingInterface struct {
	Name   string
	Owner  string   // Identified owner
	Routes []string // Known routes via this interface
}

func scanWireGuardInterfaces(excludeIfaces ...string) ([]ExistingInterface, error) {
	var result []ExistingInterface

	skip := make(map[string]bool, len(excludeIfaces))
	for _, n := range excludeIfaces {
		if n != "" {
			skip[n] = true
		}
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	// Resolve once per scan: which interfaces actually carry
	// Tailscale's IPs. Without this, identifyOwner labelled every
	// utun as "Tailscale" the moment tailscaled was running, which
	// hid the real owner of co-resident WireGuard tunnels.
	tsIfaces := tailscaleInterfaces()

	for _, iface := range ifaces {
		name := iface.Name
		// Only check utun (macOS), wg (Linux), or WireGuard-like interfaces
		if !isWireGuardLike(name) {
			continue
		}
		// The tunnel under test's own interface is never a conflict.
		if skip[name] {
			continue
		}

		owner := identifyOwner(name, tsIfaces)
		routes := getInterfaceRoutes(name)

		if len(routes) > 0 {
			result = append(result, ExistingInterface{
				Name:   name,
				Owner:  owner,
				Routes: routes,
			})
		}
	}

	return result, nil
}

// tailscaleInterfaces returns the set of local interfaces that carry
// at least one of Tailscale's CGNAT IPs. Empty when tailscale isn't
// installed or returns nothing — callers must treat absence as "not
// Tailscale" rather than "unknown".
//
// Why we need this: tailscaled runs once and owns one utun, but the
// system can have several utunN interfaces from other tools
// (WireGuide, raw wireguard-go, vpnd). Without per-interface
// disambiguation, the conflict warning UI confused users by claiming
// "Tailscale conflict" against unrelated tunnels.
func tailscaleInterfaces() map[string]bool {
	ips := tailscaleLocalIPs()
	if len(ips) == 0 {
		return nil
	}
	netIfs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	result := make(map[string]bool)
	for _, ifc := range netIfs {
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipStr := a.String()
			if i := strings.Index(ipStr, "/"); i >= 0 {
				ipStr = ipStr[:i]
			}
			for _, tsIP := range ips {
				if ipStr == tsIP {
					result[ifc.Name] = true
				}
			}
		}
	}
	return result
}

// tailscaleLocalIPs invokes `tailscale ip` to enumerate the host's
// Tailscale IPs (typically one IPv4 and one IPv6). 1.5s cap so a
// hung tailscaled doesn't block CheckConflicts.
func tailscaleLocalIPs() []string {
	if _, err := exec.LookPath("tailscale"); err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tailscale", "ip").CombinedOutput()
	if err != nil {
		return nil
	}
	var ips []string
	for _, line := range strings.Split(string(out), "\n") {
		ip := strings.TrimSpace(line)
		if ip == "" {
			continue
		}
		if net.ParseIP(ip) != nil {
			ips = append(ips, ip)
		}
	}
	return ips
}

func isWireGuardLike(name string) bool {
	// Unix: utun (macOS / wireguard-go), wg/tun (Linux / wireguard-go,
	// and the `wg-quick` convention of "wg0", "tun0" etc.).
	if strings.HasPrefix(name, "utun") ||
		strings.HasPrefix(name, "wg") ||
		strings.HasPrefix(name, "tun") {
		return true
	}
	// Windows: wintun adapters are named after the owning app. This app
	// creates "WireGuidePlus-<hash>" (legacy "WireGuide-<hash>" from
	// before the rebrand), the official WireGuard client uses "WireGuard".
	// net.Interfaces() reports the adapter FriendlyName, so without these
	// prefixes scanWireGuardInterfaces skipped every tunnel on Windows
	// and routing-conflict warnings never fired there.
	if runtime.GOOS == "windows" &&
		(strings.HasPrefix(name, "WireGuidePlus-") ||
			strings.HasPrefix(name, "WireGuide-") ||
			strings.HasPrefix(name, "WireGuard")) {
		return true
	}
	return false
}

// identifyOwner determines who created this interface by checking UAPI sockets
// and the per-scan Tailscale interface map. tsIfaces maps interface names that
// actually carry a Tailscale IP — pass nil when tailscaled isn't running.
func identifyOwner(ifaceName string, tsIfaces map[string]bool) string {
	// Windows: there is no /var/run socket — wintun adapter names carry
	// the owner. Our adapters are "WireGuidePlus-<hash>" (legacy
	// "WireGuide-<hash>"), the official WireGuard client's are "WireGuard".
	if runtime.GOOS == "windows" {
		if strings.HasPrefix(ifaceName, "WireGuidePlus-") ||
			strings.HasPrefix(ifaceName, "WireGuide-") {
			return "WireGuide"
		}
		if strings.HasPrefix(ifaceName, "WireGuard") {
			return "WireGuard"
		}
	}

	// Check WireGuide socket — the current /var/run/wireguideplus/ path,
	// plus the pre-plus /var/run/wireguide/ path an older install's
	// running instance may still be serving.
	if socketExists("/var/run/wireguideplus/"+ifaceName+".sock") ||
		socketExists("/var/run/wireguide/"+ifaceName+".sock") {
		return "WireGuide"
	}

	// Check WireGuard socket
	if socketExists("/var/run/wireguard/" + ifaceName + ".sock") {
		return "WireGuard"
	}

	// Tailscale ownership requires tailscaled to be live AND this
	// specific interface to host one of its IPs. Without the per-
	// interface check, every utun got mis-labelled as Tailscale.
	if tsIfaces[ifaceName] {
		return "Tailscale"
	}

	if processOwnsInterface(ifaceName, "wireguard-go") {
		return "WireGuard"
	}

	return "Unknown"
}

func socketExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	// Only accept actual sockets — regular files with similar names do NOT
	// indicate a running peer. Previously we OR'd with IsRegular() which
	// produced false positives on stale leftover files.
	return info.Mode()&os.ModeSocket != 0
}

func processExists(name string) bool {
	switch runtime.GOOS {
	case "darwin", "linux":
		out, _ := runConflictCmd("pgrep", "-x", name)
		return len(strings.TrimSpace(string(out))) > 0
	case "windows":
		out, _ := runConflictCmd("tasklist", "/FI", "IMAGENAME eq "+name+".exe")
		return strings.Contains(string(out), name)
	}
	return false
}

func processOwnsInterface(ifaceName, processName string) bool {
	// NOTE: This is a simplification — it checks whether the process is running
	// at all, not whether it actually owns this specific interface. A more
	// accurate implementation would inspect /proc/<pid>/fd on Linux or use
	// lsof on macOS to correlate the TUN fd with the interface. Acceptable
	// for now because false positives only produce a warning, not a hard block.
	return processExists(processName)
}

// getInterfaceRoutes returns routes via the given interface.
func getInterfaceRoutes(ifaceName string) []string {
	switch runtime.GOOS {
	case "darwin":
		return getRoutesDarwin(ifaceName)
	case "linux":
		return getRoutesLinux(ifaceName)
	case "windows":
		return getRoutesWindows(ifaceName)
	default:
		return nil
	}
}

func getRoutesDarwin(ifaceName string) []string {
	out, err := runConflictCmdLC("netstat", "-rn", "-f", "inet")
	if err != nil {
		return nil
	}

	// Parse header dynamically. netstat column order is stable in practice,
	// but hardcoding index 3 has broken in the past when flags shifted.
	destIdx, netifIdx := -1, -1
	var routes []string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if netifIdx < 0 {
			for i, f := range fields {
				switch f {
				case "Destination":
					destIdx = i
				case "Netif":
					netifIdx = i
				}
			}
			continue
		}
		if len(fields) <= netifIdx || destIdx < 0 {
			continue
		}
		if fields[netifIdx] != ifaceName {
			continue
		}
		route := fields[destIdx]
		if route == "default" {
			routes = append(routes, "0.0.0.0/0")
		} else {
			routes = append(routes, route)
		}
	}
	return routes
}

func getRoutesLinux(ifaceName string) []string {
	out, err := runConflictCmd("ip", "route", "show", "dev", ifaceName)
	if err != nil {
		return nil
	}
	var routes []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 0 {
			route := fields[0]
			if route == "default" {
				routes = append(routes, "0.0.0.0/0")
			} else {
				routes = append(routes, route)
			}
		}
	}
	return routes
}

// findOverlaps checks if any of newIPs overlap with existingIPs.
// To avoid producing hundreds of entries when newCIDR is a supernet
// (e.g. full-tunnel 0.0.0.0/0 matching every existing route), each
// newCIDR contributes at most one overlap entry.
//
// "Overlap" is judged by cidrOverlaps, which requires the two ranges to
// actually INTERSECT: sibling prefixes of the same size (10.30.30.0/24
// and 10.30.35.0/24) are disjoint and must never be reported, while a
// supernet/subnet pair (10.30.0.0/16 vs 10.30.30.0/24) is a genuine
// routing conflict.
func findOverlaps(newIPs, existingIPs []string) []string {
	var overlaps []string
	seen := map[string]bool{}
	for _, newCIDR := range newIPs {
		newNet, err := parseNet(newCIDR)
		if err != nil {
			continue
		}
		for _, existCIDR := range existingIPs {
			existNet, err := parseNet(existCIDR)
			if err != nil {
				continue
			}
			if cidrOverlaps(newNet, existNet) {
				if seen[newCIDR] {
					continue
				}
				overlaps = append(overlaps, fmt.Sprintf("%s <> %s", newCIDR, existCIDR))
				seen[newCIDR] = true
			}
		}
	}
	return overlaps
}

// parseNet turns a possibly-abbreviated route string into a masked
// *net.IPNet. Unparseable input yields an error and the caller skips
// that entry rather than guessing.
func parseNet(s string) (*net.IPNet, error) {
	_, n, err := net.ParseCIDR(normalizeCIDR(s))
	if err != nil {
		return nil, err
	}
	return n, nil
}

// cidrOverlaps reports whether two IP networks share at least one
// address. Two networks intersect exactly when one's network address
// falls inside the other, so this is a single containment test in each
// direction — but the address families are compared FIRST:
//
//   - IPv4 vs IPv6 can never overlap, and net.IPNet.Contains already
//     returns false across families, so this is belt-and-braces against
//     a future parser change that yields 4-in-6 addresses.
//   - Sibling prefixes (10.30.30.0/24, 10.30.35.0/24) contain each
//     other's network address in neither direction → disjoint → false.
//
// Host bits are already masked off by net.ParseCIDR, so a route read
// back as 10.30.35.7/24 is treated as 10.30.35.0/24 and cannot be
// mistaken for a wider range than it is.
func cidrOverlaps(a, b *net.IPNet) bool {
	if a == nil || b == nil {
		return false
	}
	if (a.IP.To4() != nil) != (b.IP.To4() != nil) {
		return false
	}
	return a.Contains(b.IP) || b.Contains(a.IP)
}

func normalizeCIDR(s string) string {
	// If it's just an IP without mask, add /32
	if !strings.Contains(s, "/") {
		if strings.Contains(s, ":") {
			return s + "/128"
		}
		return s + "/32"
	}
	// macOS netstat abbreviates CIDRs: "0/1" means "0.0.0.0/1",
	// "128.0/1" means "128.0.0.0/1", "10.99/16" means "10.99.0.0/16".
	// Expand to full dotted-quad so net.ParseCIDR succeeds.
	parts := strings.SplitN(s, "/", 2)
	ip := parts[0]
	mask := parts[1]
	if !strings.Contains(ip, ":") {
		octets := strings.Split(ip, ".")
		for len(octets) < 4 {
			octets = append(octets, "0")
		}
		ip = strings.Join(octets, ".")
	}
	return ip + "/" + mask
}
