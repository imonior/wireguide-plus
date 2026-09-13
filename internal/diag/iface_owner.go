package diag

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

// EnrichInterfaceOwners fills InterfaceDetail for anonymous virtual NICs
// (utun / tun / wg / tap ...) by detecting which application created them.
// It only writes when InterfaceDetail is currently empty, so name-based
// heuristics (classifyIface / ifaceDetail) and the app's own self-created
// tunnels keep priority. It is a no-op when detection is unavailable or
// yields nothing.
func EnrichInterfaceOwners(entries []RouteEntry) []RouteEntry {
	if len(entries) == 0 {
		return entries
	}
	return applyOwners(entries, detectInterfaceOwners())
}

// applyOwners fills InterfaceDetail for entries whose detail is empty using the
// supplied interface->owner map. It never overwrites an existing detail, so
// name heuristics and the app's own self-created tunnels keep priority. Pure
// mapping logic, isolated for testing.
func applyOwners(entries []RouteEntry, owners map[string]string) []RouteEntry {
	if len(owners) == 0 {
		return entries
	}
	for i := range entries {
		if entries[i].InterfaceDetail != "" {
			continue
		}
		if name, ok := owners[entries[i].Interface]; ok && name != "" {
			entries[i].InterfaceDetail = name
		}
	}
	return entries
}

// detectInterfaceOwners returns a map of interface name -> creating software
// name for virtual tunnel interfaces the OS does not label itself. Detection
// shells out to ordinary user-permission commands (no privileged helper) and
// is best-effort: any failure simply yields fewer (or no) entries.
func detectInterfaceOwners() map[string]string {
	switch runtime.GOOS {
	case "darwin":
		return detectOwnersDarwin()
	case "linux":
		return detectOwnersLinux()
	case "windows":
		return detectOwnersWindows()
	default:
		return nil
	}
}

// ---------------------------------------------------------------------------
// Shared: Clash-family TUN (FlClash / mihomo / sing-box / Karing / Clash)
// ---------------------------------------------------------------------------

// clashAPIPorts are the default external-controller ports the Clash family
// listens on. Querying each is cheap (one local HTTP GET) and we stop caring
// about non-responders.
var clashAPIPorts = []string{"9090", "9097", "7892", "11227"}

type clashConfigs struct {
	Tun struct {
		Enable bool   `json:"enable"`
		Device string `json:"device"`
		Mode   string `json:"mode"`
	} `json:"tun"`
	MixedPort int `json:"mixed-port"`
	SocksPort int `json:"socks-port"`
	Port      int `json:"port"`
}

// detectClashTunOwners probes the Clash-family external-controller API on the
// well-known ports and, for every instance with TUN enabled, maps its TUN
// device (the real utun/tun interface name) to the app that owns it. This is
// the single most reliable cross-platform attribution because the API returns
// the exact device name rather than forcing us to guess from port numbers.
func detectClashTunOwners() map[string]string {
	owners := make(map[string]string)
	procList := runningProcessList()
	listening := listeningProcessMap()
	client := &http.Client{Timeout: 1200 * time.Millisecond}
	for _, port := range clashAPIPorts {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
			"http://127.0.0.1:"+port+"/configs", nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		var cfg clashConfigs
		_ = json.NewDecoder(resp.Body).Decode(&cfg)
		resp.Body.Close()
		// No TUN (or not a Clash API at all) -> nothing to attribute.
		if cfg.Tun.Device == "" {
			continue
		}
		owners[cfg.Tun.Device] = determineClashAppName(procList, listening, port)
	}
	return owners
}

// determineClashAppName resolves which Clash-family binary owns an API port.
// It prefers the process actually listening on the API port, then falls back
// to scanning the running-process list for a known binary name.
func determineClashAppName(procList string, listening map[string]string, apiPort string) string {
	if listener, ok := listening[apiPort]; ok {
		switch {
		case listener == "FlClash" || listener == "FlClashCore":
			return "FlClash"
		case strings.Contains(strings.ToLower(listener), "karing"):
			return "Karing"
		case strings.Contains(strings.ToLower(listener), "sing"):
			return "sing-box"
		case strings.Contains(strings.ToLower(listener), "mihomo"):
			return "FlClash"
		}
	}
	if strings.Contains(procList, "FlClash") {
		return "FlClash"
	}
	if strings.Contains(procList, "Karing") || strings.Contains(procList, "karing") {
		return "Karing"
	}
	if strings.Contains(procList, "sing-box") {
		return "sing-box"
	}
	return "Clash"
}

// runningProcessList returns the newline-separated list of running process
// comm names (best effort).
func runningProcessList() string {
	if runtime.GOOS == "windows" {
		out, err := runRouteCmd("tasklist", "/NH", "/FO", "CSV")
		if err != nil {
			return ""
		}
		var b strings.Builder
		for _, line := range strings.Split(string(out), "\n") {
			f := strings.Split(line, ",")
			if len(f) < 1 {
				continue
			}
			name := strings.Trim(f[0], `"`)
			name = strings.TrimSuffix(strings.TrimSpace(name), ".exe")
			b.WriteString(name)
			b.WriteByte('\n')
		}
		return b.String()
	}
	out, err := runRouteCmd("ps", "-axo", "comm=")
	if err != nil {
		return ""
	}
	return string(out)
}

// listeningProcessMap maps a local TCP LISTEN port to the owning process name.
func listeningProcessMap() map[string]string {
	m := make(map[string]string)
	if runtime.GOOS == "windows" {
		// No lsof on Windows; the API device from detectClashTunOwners is
		// enough to attribute, so we intentionally return empty here and let
		// determineClashAppName fall back to the process-list scan.
		return m
	}
	out, err := runRouteCmd("lsof", "-iTCP", "-sTCP:LISTEN", "-P", "-n")
	if err != nil {
		return m
	}
	re := regexp.MustCompile(`:(\d+)`)
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "LISTEN") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if m2 := re.FindStringSubmatch(line); m2 != nil {
			m[m2[1]] = fields[0]
		}
	}
	return m
}

// ---------------------------------------------------------------------------
// Shared: WireGuard run dir
// ---------------------------------------------------------------------------

// wireGuardInterfaceMap reads /var/run/wireguard/*.name (and *.sock) which
// wg-quick / WireGuard.app populate with the interface name, mapping each
// utun/tun device to its configuration name.
func wireGuardInterfaceMap() map[string]string {
	m := make(map[string]string)
	dir := "/var/run/wireguard/"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return m
	}
	var names, socks []string
	for _, e := range entries {
		n := e.Name()
		switch {
		case strings.HasSuffix(n, ".name"):
			names = append(names, strings.TrimSuffix(n, ".name"))
		case strings.HasSuffix(n, ".sock"):
			socks = append(socks, strings.TrimSuffix(n, ".sock"))
		}
	}
	for _, cfg := range names {
		data, err := os.ReadFile(dir + cfg + ".name")
		if err != nil {
			continue
		}
		if iface := strings.TrimSpace(string(data)); iface != "" {
			m[iface] = cfg
		}
	}
	if len(m) > 0 {
		return m
	}
	// Fallback: when *.name contents are unreadable but names line up 1:1
	// with the *.sock (interface) names, assume sorted correspondence.
	if len(names) > 0 && len(names) == len(socks) {
		sort.Strings(names)
		sort.Strings(socks)
		for i := range names {
			m[socks[i]] = names[i]
		}
	} else {
		for _, s := range socks {
			if _, ok := m[s]; !ok {
				m[s] = "WireGuard"
			}
		}
	}
	return m
}

// ---------------------------------------------------------------------------
// macOS
// ---------------------------------------------------------------------------

func detectOwnersDarwin() map[string]string {
	owners := make(map[string]string)
	// 1. Clash-family TUN devices (most reliable: exact device name).
	for dev, app := range detectClashTunOwners() {
		owners[dev] = app
	}
	// 2. WireGuard (wg-quick / WireGuard.app).
	for iface, name := range wireGuardInterfaceMap() {
		if _, ok := owners[iface]; !ok {
			owners[iface] = name
		}
	}
	// 3. System NEVPN services. Modern macOS does not expose the utun name
	//    in `scutil --nc status` output from user space, so this primarily
	//    surfaces the service name; when an interface line is present (older
	//    macOS) we attribute it directly.
	for iface, name := range systemVPNsDarwin() {
		if iface != "" {
			owners[iface] = name
		}
	}
	return owners
}

// systemVPNsDarwin parses `scutil --nc list` for the service name and status,
// then `scutil --nc status <name>` for the owning interface. Returns a map of
// interface -> display name (empty interface when the OS hides it).
func systemVPNsDarwin() map[string]string {
	out := make(map[string]string)
	res, err := runRouteCmd("/usr/sbin/scutil", "--nc", "list")
	if err != nil {
		return out
	}
	// Each service line: "* (Status) <UUID> VPN (type) "Name" [VPN:...]"
	re := regexp.MustCompile(`\((\w+)\)\s+\S+\s+VPN\s*\([^)]*\)\s+"([^"]+)"`)
	for _, line := range strings.Split(string(res), "\n") {
		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		status, svc := m[1], m[2]
		if status != "Connected" {
			continue
		}
		display := vpnServiceDisplayName(svc)
		iface := ""
		if st, err := runRouteCmd("/usr/sbin/scutil", "--nc", "status", svc); err == nil {
			for _, l := range strings.Split(string(st), "\n") {
				if v, ok := strings.CutPrefix(strings.TrimSpace(l), "interface:"); ok {
					iface = strings.Fields(v)[0]
					break
				}
				if v, ok := strings.CutPrefix(strings.TrimSpace(l), "InterfaceName:"); ok {
					iface = strings.Fields(v)[0]
					break
				}
			}
		}
		out[iface] = display
	}
	return out
}

// vpnServiceDisplayName normalizes well-known macOS VPN service names.
func vpnServiceDisplayName(svc string) string {
	lower := strings.ToLower(svc)
	switch {
	case strings.Contains(lower, "hiddify"):
		return "Hiddify"
	case strings.Contains(lower, "proton"):
		return "ProtonVPN"
	case strings.Contains(lower, "tailscale"):
		return "Tailscale"
	case strings.Contains(lower, "nord"):
		return "NordVPN"
	case strings.Contains(lower, "expressvpn"):
		return "ExpressVPN"
	case strings.Contains(lower, "warp"), strings.Contains(lower, "cloudflare"):
		return "Cloudflare WARP"
	default:
		return svc
	}
}

// ---------------------------------------------------------------------------
// Linux
// ---------------------------------------------------------------------------

func detectOwnersLinux() map[string]string {
	owners := make(map[string]string)
	for dev, app := range detectClashTunOwners() {
		owners[dev] = app
	}
	for iface, name := range wireGuardInterfaceMap() {
		if _, ok := owners[iface]; !ok {
			owners[iface] = name
		}
	}
	// NetworkManager active VPN connections name the device directly.
	if res, err := runRouteCmd("nmcli", "-t", "-f", "NAME,DEVICE,TYPE", "connection", "show", "--active"); err == nil {
		for _, line := range strings.Split(string(res), "\n") {
			parts := strings.SplitN(line, ":", 3)
			if len(parts) < 3 {
				continue
			}
			name, dev, typ := parts[0], parts[1], parts[2]
			if strings.Contains(strings.ToLower(typ), "vpn") && dev != "" {
				if _, ok := owners[dev]; !ok {
					owners[dev] = name
				}
			}
		}
	}
	return owners
}

// ---------------------------------------------------------------------------
// Windows
// ---------------------------------------------------------------------------

func detectOwnersWindows() map[string]string {
	owners := make(map[string]string)
	for dev, app := range detectClashTunOwners() {
		owners[dev] = app
	}
	// Get-NetAdapter's Name IS the FriendlyName iphlpapi reports as the route
	// interface, so the keys line up with the routing table. We derive the
	// creator from the driver InterfaceDescription (e.g. "Wintun Userspace
	// Tunnel", "WireGuard Tunnel Adapter", "ZeroTier One Virtual Port").
	if res, err := runRouteCmd("powershell", "-NoProfile", "-Command",
		"Get-NetAdapter | Select-Object Name,InterfaceDescription | ConvertTo-Json"); err == nil {
		var adapters []struct {
			Name                  string `json:"Name"`
			InterfaceDescription  string `json:"InterfaceDescription"`
		}
		if json.Unmarshal(res, &adapters) == nil {
			for _, a := range adapters {
				if owner := vpnDriverOwner(a.InterfaceDescription); owner != "" {
					owners[a.Name] = owner
				}
			}
		}
	}
	// System RAS VPN connections (IKEv2/L2TP/PPTP/SSTP).
	if res, err := runRouteCmd("powershell", "-NoProfile", "-Command",
		"Get-VpnConnection | Select-Object Name,ConnectionStatus | ConvertTo-Json"); err == nil {
		var vpns []struct {
			Name              string `json:"Name"`
			ConnectionStatus  string `json:"ConnectionStatus"`
		}
		if json.Unmarshal(res, &vpns) == nil {
			for _, v := range vpns {
				if strings.EqualFold(v.ConnectionStatus, "Connected") {
					owners[v.Name] = v.Name
				}
			}
		}
	}
	return owners
}

// vpnDriverOwner maps a Windows network-adapter driver description to the
// creating software name.
func vpnDriverOwner(desc string) string {
	d := strings.ToLower(desc)
	switch {
	case strings.Contains(d, "wireguard"):
		return "WireGuard"
	case strings.Contains(d, "wintun"):
		return "Wintun"
	case strings.Contains(d, "tap-windows"):
		return "OpenVPN"
	case strings.Contains(d, "zerotier"):
		return "ZeroTier"
	case strings.Contains(d, "cloudflare warp"):
		return "Cloudflare WARP"
	case strings.Contains(d, "tailscale"):
		return "Tailscale"
	default:
		return ""
	}
}
