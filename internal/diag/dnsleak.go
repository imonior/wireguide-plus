package diag

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/imonior/wireguide-plus/internal/sysexec"
	"github.com/imonior/wireguide-plus/internal/version"
)

// DNSLeakResult holds the DNS leak test results.
type DNSLeakResult struct {
	Leaked     bool        `json:"leaked"`
	DNSServers []DNSServer `json:"dns_servers"`
	TestDomain string      `json:"test_domain"`
	Error      string      `json:"error,omitempty"`
}

// DNSServer represents a detected DNS resolver.
type DNSServer struct {
	IP         string `json:"ip"`
	Hostname   string `json:"hostname"`
	IsVPN      bool   `json:"is_vpn"`     // true if this is the expected VPN DNS (tunnel config) or lives on a virtual tunnel interface
	IsLocal    bool   `json:"is_local"`   // true if this resolver is configured on a physical hardware interface (WLAN/Ethernet)
	SourceIface string `json:"source_iface,omitempty"` // name of the interface the resolver was found on (ipconfig /all style)
	Responds   bool   `json:"responds"`   // did the probe get a DNS reply (NXDOMAIN)?
	LatencyMs  int    `json:"latency_ms"` // probe round-trip; 0 if it timed out
	Status     string `json:"status"`     // "vpn" | "ok" | "leak" | "timeout"
	Encryption string `json:"encryption"` // "plain" | "dot" | "doh" | "plain+dot" | "plain+doh" | "none"
}

// systemResolver is a DNS server found in the host's own network
// configuration, tagged with whether it came from a physical (local)
// interface or a virtual tunnel (VPN) interface, plus the source name.
type systemResolver struct {
	IP      string
	IsLocal bool
	IsVPN   bool
	Iface   string
}

// probePlan is the deduplicated, ordered set of resolver IPs the leak test
// will probe, plus the per-IP flag maps derived from the system config.
type probePlan struct {
	targets   []string
	localSet  map[string]bool
	vpnSet    map[string]bool
	ifaceByIP map[string]string
}

// buildProbePlan merges the system-configured resolvers with the public
// cross-check list into the ordered probe set. The system block is ALWAYS
// first and ALWAYS included — no matter what the public list contains (nil,
// empty, or a fully custom non-empty set), the machine's own local/VPN DNS
// servers are always probed and listed at the top. Physical-interface
// (local) resolvers lead, then VPN resolvers, then the public list. A
// resolver present in both lists keeps only its system entry.
//
// A nil OR empty public list falls back to the built-in defaults: clearing
// the custom list restores the default cross-check set instead of disabling
// public probing, which is core to the leak test.
func buildProbePlan(systemDNS []systemResolver, publicList []string) probePlan {
	if len(publicList) == 0 {
		publicList = publicResolvers
	}
	plan := probePlan{
		targets:   make([]string, 0, len(systemDNS)+len(publicList)),
		localSet:  make(map[string]bool, len(systemDNS)),
		vpnSet:    make(map[string]bool, len(systemDNS)),
		ifaceByIP: make(map[string]string, len(systemDNS)),
	}
	seen := make(map[string]bool, len(systemDNS)+len(publicList))
	for _, r := range systemDNS {
		if !seen[r.IP] {
			seen[r.IP] = true
			plan.targets = append(plan.targets, r.IP)
		}
		if r.IsLocal {
			plan.localSet[r.IP] = true
		}
		if r.IsVPN {
			plan.vpnSet[r.IP] = true
		}
		if plan.ifaceByIP[r.IP] == "" && r.Iface != "" {
			plan.ifaceByIP[r.IP] = r.Iface
		}
	}
	for _, dns := range publicList {
		if !seen[dns] {
			seen[dns] = true
			plan.targets = append(plan.targets, dns)
		}
	}
	return plan
}

// publicResolvers lists well-known public recursive resolvers that the test
// probes in addition to the system-configured DNS servers. They act as a
// cross-check: the UI shows them after the system resolvers with "OK" meaning
// "reachable but not part of the system configuration" — NOT a leak, because
// the probe deliberately sends them the query.
//
// This is the BUILT-IN DEFAULT list, used whenever the user has not supplied
// a customized list AND the network-fetched list is unavailable. Users can
// edit/delete entries and add their own via the DNS leak panel; the
// customized list is persisted in settings (dns_test_public_servers) and
// passed into RunDNSLeakTestWithPublic.
var publicResolvers = []string{
	// Google Public DNS
	"8.8.8.8", "8.8.4.4",
	// Cloudflare
	"1.1.1.1", "1.0.0.1",
	// OpenDNS (Cisco)
	"208.67.222.222", "208.67.220.220",
	// Quad9
	"9.9.9.9",
	// Alibaba Public DNS
	"223.5.5.5", "223.6.6.6",
	// Tencent DNSPod
	"119.29.29.29",
	// 114 DNS
	"114.114.114.114",
	// Baidu DNS
	"180.76.76.76",
	// AdGuard DNS
	"94.140.14.14", "94.140.15.15",
	// NextDNS
	"45.90.28.190", "45.90.30.190",
	// Comodo Secure DNS
	"8.26.56.26", "8.20.247.20",
	// IPv6 (Google, Cloudflare)
	"2001:4860:4860::8888", "2606:4700:4700::1111",
}

// publicDNSInfoURL is the remote source for live-refreshing the public
// resolver list. public-dns.info publishes a JSON array of the resolvers it
// monitors, each with reliability/error/DNSSEC metadata; we filter to the
// healthy, highly-reliable entries so the cross-check set stays current.
const publicDNSInfoURL = "https://public-dns.info/nameservers.json"

// publicDNSInfoEntry is one item of the public-dns.info nameservers.json feed.
type publicDNSInfoEntry struct {
	IP          string  `json:"ip"`
	Reliability float64 `json:"reliability"`
	Error       string  `json:"error"`
}

// publicFetchLimit caps how many resolvers a network refresh may return. The
// feed lists tens of thousands of resolvers; a cross-check probe against all
// of them would take minutes. Limiting to the most reliable keeps the test
// snappy while still refreshing the useful subset.
const publicFetchLimit = 30

// FetchPublicResolvers downloads the current public DNS resolver list from
// public-dns.info and returns the most-reliable healthy entries, capped at
// publicFetchLimit. An error is returned if the feed cannot be fetched or
// parsed; callers fall back to the built-in DefaultPublicResolvers list.
// The ctx bounds the whole operation (10s timeout in the UI).
func FetchPublicResolvers(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, publicDNSInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("public-dns.info: build request: %w", err)
	}
	req.Header.Set("User-Agent", "wireguide-plus/"+version.Version)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("public-dns.info: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("public-dns.info: HTTP %d", resp.StatusCode)
	}

	// The full feed is tens of thousands of entries (~several MB); cap the
	// read so a misbehaving endpoint cannot exhaust memory.
	dec := json.NewDecoder(io.LimitReader(resp.Body, 16<<20))
	var entries []publicDNSInfoEntry
	if err := dec.Decode(&entries); err != nil {
		return nil, fmt.Errorf("public-dns.info: parse JSON: %w", err)
	}

	type scored struct {
		ip          string
		reliability float64
	}
	var scoredList []scored
	seen := make(map[string]bool)
	for _, e := range entries {
		ip := strings.TrimSpace(e.IP)
		if ip == "" || e.Error != "" || seen[ip] {
			continue
		}
		if net.ParseIP(ip) == nil {
			continue
		}
		seen[ip] = true
		scoredList = append(scoredList, scored{ip: ip, reliability: e.Reliability})
	}
	sort.SliceStable(scoredList, func(i, j int) bool {
		return scoredList[i].reliability > scoredList[j].reliability
	})
	if len(scoredList) > publicFetchLimit {
		scoredList = scoredList[:publicFetchLimit]
	}
	out := make([]string, 0, len(scoredList))
	for _, s := range scoredList {
		out = append(out, s.ip)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("public-dns.info: no usable resolvers in feed")
	}
	return out, nil
}

// DefaultPublicResolvers returns a copy of the built-in public resolver list
// so callers (settings persistence, the UI) can seed a user-editable list
// without aliasing the global slice.
func DefaultPublicResolvers() []string {
	return append([]string(nil), publicResolvers...)
}

// RunDNSLeakTest is a context-less convenience wrapper for callers that
// don't have one. Bounded by a hard 10-second cap so a hung resolver
// can't lock up the diagnostic panel.
func RunDNSLeakTest(expectedDNS []string) *DNSLeakResult {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return RunDNSLeakTestContext(ctx, expectedDNS)
}

// RunDNSLeakTestWithPublic is RunDNSLeakTest with a caller-supplied list of
// public resolvers to probe in addition to the system-configured DNS. The
// list REPLACES the built-in default (so a user's customized set in settings
// is honoured verbatim), but an empty list falls back to the built-in
// defaults — clearing the custom list restores the default cross-check set
// instead of disabling public probing entirely, which is the feature's
// intended behaviour.
func RunDNSLeakTestWithPublic(expectedDNS, publicDNS []string) *DNSLeakResult {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return RunDNSLeakTestContextWithPublic(ctx, expectedDNS, publicDNS)
}

// RunDNSLeakTestContext checks if DNS queries are going through the VPN.
// It resolves a random subdomain via each system-configured DNS server in
// PARALLEL (listed first) plus the built-in set of well-known public
// resolvers as a cross-check, and flags leaks only when a non-VPN *system*
// resolver answers — a reachable public resolver is "ok", not a leak,
// because the probe deliberately sent it the query. The whole test honours
// ctx — if the caller cancels (user closes the diagnostics panel) every
// in-flight resolver lookup aborts within its own per-request timeout slot.
//
// Parallel execution caps wall-clock at the slowest single resolver
// (typically <1s for working DNS, 3s for a dead one) instead of the sum
// (which on a machine with many resolver entries could exceed a minute).
func RunDNSLeakTestContext(ctx context.Context, expectedDNS []string) *DNSLeakResult {
	return RunDNSLeakTestContextWithPublic(ctx, expectedDNS, nil)
}

// RunDNSLeakTestContextWithPublic is RunDNSLeakTestContext with an explicit
// public resolver list. A nil OR empty list means the built-in defaults are
// used; a non-empty slice replaces them verbatim, which is how the
// user-editable settings list is honoured. Empty is deliberately treated as
// "restore defaults" (not "disable public probing") so that clearing the
// custom list in settings can never turn off the cross-check — the built-in
// public DNS list always remains the floor.
func RunDNSLeakTestContextWithPublic(ctx context.Context, expectedDNS, publicDNS []string) *DNSLeakResult {
	result := &DNSLeakResult{}

	// Generate a fresh random subdomain so the test query can't be
	// served from any resolver's cache. crypto/rand (16 bytes hex)
	// gives 128 bits of randomness — far beyond any feasible cache
	// pre-population. .invalid is reserved by RFC 6761 for "must
	// always return NXDOMAIN" so the test can't accidentally hit a
	// real domain or load a third-party authoritative server.
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		result.Error = "cannot generate random domain"
		return result
	}
	testDomain := "wireguide-" + hex.EncodeToString(nonce[:]) + ".invalid"
	result.TestDomain = testDomain

	// Check system resolver configuration
	// On macOS: scutil --dns, on Linux: /etc/resolv.conf, on Windows:
	// per-interface Get-DnsClientServerAddress tagged local/vpn.
	systemDNS := getSystemDNSServers()

	expectedSet := make(map[string]bool)
	for _, dns := range expectedDNS {
		expectedSet[dns] = true
	}

	// Sort system resolvers so physical (local) interfaces come first,
	// then virtual (VPN) interfaces — the UI shows local hardware DNS at
	// the top as the baseline, matching `ipconfig /all` adapter order.
	sort.SliceStable(systemDNS, func(i, j int) bool {
		li, lj := systemDNS[i].IsLocal, systemDNS[j].IsLocal
		if li != lj {
			return li
		}
		return false
	})

	// Probe set = system-configured resolvers first (the baseline the UI
	// shows on top), then public cross-check resolvers — either the built-in
	// defaults or the user's customized list from settings.
	// buildProbePlan handles the nil/empty → built-in defaults fallback.
	plan := buildProbePlan(systemDNS, publicDNS)
	probeTargets := plan.targets
	localSet := plan.localSet
	vpnSet := plan.vpnSet
	ifaceByIP := plan.ifaceByIP

	type probeResult struct {
		idx        int
		hostname   string
		responds   bool
		latency    time.Duration
		encryption string
	}

	// Pre-fill DNSServers so each slot has at least the IP and IsVPN
	// flag even if its probe never returns. The earlier version left
	// timed-out slots zero-valued (empty IP, empty hostname, IsVPN=false),
	// so the UI rendered placeholder "!" badges with no IP next to them
	// whenever any probe hit the outer ctx deadline. The probe loop now
	// just overlays Hostname/IsVPN-promotion on top of these defaults.
	result.DNSServers = make([]DNSServer, len(probeTargets))
	for i, dns := range probeTargets {
		result.DNSServers[i] = DNSServer{
			IP:          dns,
			IsVPN:       expectedSet[dns] || vpnSet[dns],
			IsLocal:     localSet[dns],
			SourceIface: ifaceByIP[dns],
		}
	}

	probes := make(chan probeResult, len(probeTargets))
	for i, dns := range probeTargets {
		go func(idx int, dnsIP string) {
			// Per-resolver budget: 3s. Caller's ctx still acts as the
			// global ceiling — whichever expires first wins.
			lookupCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()

			hn := ""
			if names, err := (&net.Resolver{}).LookupAddr(lookupCtx, dnsIP); err == nil && len(names) > 0 {
				hn = names[0]
			}
			// Time the probe so the UI can show per-resolver latency
			// (a slow-but-responding resolver is a useful signal: e.g.
			// a blocked/rate-limited DNS vs a healthy one).
			probeStart := time.Now()
			responds := testDNSServerCtx(lookupCtx, dnsIP, testDomain)
			latency := time.Since(probeStart)
			enc := probeEncryption(lookupCtx, dnsIP, responds)
			probes <- probeResult{idx: idx, hostname: hn, responds: responds, latency: latency, encryption: enc}
		}(i, dns)
	}

	leaked := false
	for range probeTargets {
		select {
		case <-ctx.Done():
			result.Error = "test cancelled or timed out"
			result.Leaked = leaked
			return result
		case p := <-probes:
			entry := &result.DNSServers[p.idx]
			entry.Hostname = p.hostname
			entry.Responds = p.responds
			entry.LatencyMs = int(p.latency.Milliseconds())
			entry.Encryption = p.encryption
			switch {
			case entry.IsVPN:
				entry.Status = "vpn"
			case p.responds && entry.IsLocal:
				// A resolver configured on a physical hardware interface (the
				// host's own WLAN/Ethernet DNS) that is not the VPN's answered
				// the probe — real DNS traffic can leave through it: a genuine
				// leak. Virtual/tunnel-interface DNS is VPN-side, not a leak.
				entry.Status = "leak"
				leaked = true
			case p.responds:
				// Public resolver reachable: informational (reachability, latency,
				// encryption), NOT a leak — the probe deliberately sent the query.
				entry.Status = "ok"
			default:
				entry.Status = "timeout"
			}
		}
	}

	// Every DNS server slot should now have a status; belt-and-braces for
	// any slot that somehow skipped a probe result.
	for i := range result.DNSServers {
		if result.DNSServers[i].Status == "" {
			if result.DNSServers[i].IsVPN {
				result.DNSServers[i].Status = "vpn"
			} else {
				result.DNSServers[i].Status = "timeout"
			}
		}
	}

	result.Leaked = leaked
	return result
}

// probeEncryption fingerprints the resolver's transport layer so the UI can
// tell the user whether their DNS traffic is (likely) encrypted:
//
//	"plain"     — UDP 53 answered; traffic goes out in cleartext
//	"dot"       — TCP 853 spoke TLS (DoT) but plain 53 did not answer
//	"doh"       — TCP 443 is open (DoH candidate) but 853/plain did not work
//	"plain+dot" — both UDP 53 and TLS on 853 answered
//	"plain+doh" — UDP 53 answered and 443 is open (DoH candidate)
//	"none"      — nothing reachable
//
// The probes are best-effort with short per-port budgets so a firewall that
// silently drops TCP SYN does not stall the test: a "none"/"timeout" result
// simply means we could not establish a transport, not proof the port is
// closed. TCP-443-open is only a DoH *candidate* — any HTTPS service shares
// that port — so the UI words it as "possibly DoH".
func probeEncryption(ctx context.Context, server string, respondsUDP bool) string {
	host := server
	if h, _, err := net.SplitHostPort(server); err == nil {
		host = h
	}
	host = strings.TrimSpace(host)
	if host == "" || net.ParseIP(host) == nil {
		return "none"
	}

	type portResult struct {
		port string
		dot  bool
	}
	results := make(chan portResult, 2)

	// DoT: TCP 853 + TLS handshake. A ServerHello — even one that fails
	// certificate validation (we skip verification — we only care that a
	// TLS service actually answered on 853) — proves the port speaks TLS.
	results <- func() portResult {
		d := net.Dialer{Timeout: 1500 * time.Millisecond}
		conn, err := tls.DialWithDialer(&d, "tcp", net.JoinHostPort(host, "853"), &tls.Config{
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS12,
		})
		if err != nil {
			return portResult{port: "853"}
		}
		conn.Close()
		return portResult{port: "853", dot: true}
	}()

	// DoH: TCP 443 reachable.
	results <- func() portResult {
		d := net.Dialer{Timeout: 1500 * time.Millisecond}
		conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, "443"))
		if err != nil {
			return portResult{port: "443"}
		}
		conn.Close()
		return portResult{port: "443", dot: true}
	}()

	dot := false
	doh := false
	for range 2 {
		select {
		case r := <-results:
			if r.port == "853" {
				dot = r.dot
			} else if r.port == "443" {
				doh = r.dot
			}
		case <-ctx.Done():
			// ctx expired mid-probe; whatever we have is the best answer
		}
	}

	switch {
	case respondsUDP && dot:
		return "plain+dot"
	case respondsUDP && doh:
		return "plain+doh"
	case respondsUDP:
		return "plain"
	case dot:
		return "dot"
	case doh:
		return "doh"
	}
	return "none"
}

// testDNSServerCtx is testDNSServer with caller-supplied context. The
// existing testDNSServer wraps this for the (deprecated) context-less
// callers.
func testDNSServerCtx(ctx context.Context, server, domain string) bool {
	if _, _, err := net.SplitHostPort(server); err != nil {
		server = net.JoinHostPort(server, "53")
	}
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(_ context.Context, _, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 3 * time.Second}
			return d.DialContext(ctx, "udp", server)
		},
	}
	_, err := resolver.LookupHost(ctx, domain)
	if err != nil {
		if dnsErr, ok := err.(*net.DNSError); ok && dnsErr.IsNotFound {
			return true
		}
		return false
	}
	return true
}

func getSystemDNSServers() []systemResolver {
	servers, err := readSystemResolvers()
	if err != nil {
		return nil
	}
	return servers
}

// readSystemResolvers detects configured DNS servers using OS-specific methods.
func readSystemResolvers() ([]systemResolver, error) {
	switch runtime.GOOS {
	case "linux":
		return readLinuxResolvers()
	case "darwin":
		return readDarwinResolvers()
	case "windows":
		return readWindowsResolvers()
	default:
		return nil, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// readLinuxResolvers detects DNS servers per network interface. On systemd
// systems it prefers `resolvectl status` (per-link DNS, so wg0/tun0 resolvers
// get tagged vpn and eth0/wlan0 resolvers local); otherwise it falls back to
// /etc/resolv.conf, which has no interface attribution — those entries are
// treated as local (the host's own hardware DNS) unless they match the tunnel
// config later.
func readLinuxResolvers() ([]systemResolver, error) {
	if servers, err := readResolvectlResolvers(); err == nil && len(servers) > 0 {
		return servers, nil
	}

	f, err := os.Open("/etc/resolv.conf")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var servers []systemResolver
	seen := make(map[string]bool)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "nameserver" {
			ip := fields[1]
			if !seen[ip] {
				seen[ip] = true
				servers = append(servers, systemResolver{IP: ip, IsLocal: true, Iface: "resolv.conf"})
			}
		}
	}
	return servers, scanner.Err()
}

// readResolvectlResolvers enumerates DNS servers per network link. It tries
// `resolvectl status` first (block format, may wrap DNS lists across lines),
// then `resolvectl dns` (one line per link), tagging each link vpn or local
// from its interface name (wg0/tun0/utun0 → vpn, eth0/wlan0 → local).
func readResolvectlResolvers() ([]systemResolver, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "resolvectl", "status").Output()
	if err == nil {
		if servers := parseResolvectlStatus(string(out)); len(servers) > 0 {
			return servers, nil
		}
	}
	// Fallback: `resolvectl dns` — "Link 2 (enp3s0): 192.168.1.1 8.8.8.8"
	out, err = exec.CommandContext(ctx, "resolvectl", "dns").Output()
	if err != nil {
		return nil, fmt.Errorf("resolvectl status/dns: %w", err)
	}

	var servers []systemResolver
	seen := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Global") {
			continue
		}
		// "Link 2 (enp3s0): 1.1.1.1 8.8.8.8" or older "Link 2: 1.1.1.1"
		m := linkDNSEntryRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		iface, ips := m[1], m[2]
		for _, ip := range strings.Fields(ips) {
			if i := strings.IndexByte(ip, '%'); i >= 0 {
				ip = ip[:i]
			}
			if net.ParseIP(ip) == nil || seen[ip] {
				continue
			}
			seen[ip] = true
			vpn := ifaceNameIsVPN(iface)
			servers = append(servers, systemResolver{
				IP:      ip,
				IsLocal: !vpn,
				IsVPN:   vpn,
				Iface:   iface,
			})
		}
	}
	return servers, nil
}

// parseResolvectlStatus parses `resolvectl status` output, which groups
// settings per link:
//
//	Link 2 (enp3s0)
//	      Current Scopes: DNS
//	             DNS Servers: 192.168.1.1
//	                          8.8.8.8
//
// DNS lists may wrap onto continuation lines (extra indentation); those are
// collected too.
func parseResolvectlStatus(out string) []systemResolver {
	var servers []systemResolver
	seen := make(map[string]bool)
	currentIface := ""
	inDNSSection := false
	for _, line := range strings.Split(out, "\n") {
		raw := line
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// A "Link N (ifname)" header switches the current interface and
		// ends any ongoing DNS list.
		if m := linkHeaderRe.FindStringSubmatch(line); m != nil {
			currentIface = m[1]
			inDNSSection = false
			continue
		}
		// "DNS Servers: 1.1.1.1 8.8.8.8" — capture values and enter
		// continuation mode in case the list wraps to the next line.
		if m := dnsServersRe.FindStringSubmatch(line); m != nil {
			collectResolvectlDNS(&servers, seen, currentIface, m[1])
			inDNSSection = true
			continue
		}
		// Continuation lines of a wrapped DNS list are bare indented IPs.
		if inDNSSection && strings.TrimSpace(line) != "" && strings.HasPrefix(raw, " ") {
			collectResolvectlDNS(&servers, seen, currentIface, line)
			continue
		}
		// Any other setting ends the DNS list.
		inDNSSection = false
	}
	return servers
}

func collectResolvectlDNS(servers *[]systemResolver, seen map[string]bool, iface, list string) {
	vpn := ifaceNameIsVPN(iface)
	for _, ip := range strings.Fields(list) {
		if i := strings.IndexByte(ip, '%'); i >= 0 {
			ip = ip[:i]
		}
		if net.ParseIP(ip) == nil || seen[ip] {
			continue
		}
		seen[ip] = true
		*servers = append(*servers, systemResolver{
			IP:      ip,
			IsLocal: !vpn,
			IsVPN:   vpn,
			Iface:   iface,
		})
	}
}

var (
	// linkHeaderRe matches "Link 4 (wg0)" in resolvectl status output.
	linkHeaderRe = regexp.MustCompile(`^Link\s+\d+\s+\(([^)]+)\)\s*:?`)
	// dnsServersRe matches "DNS Servers: 1.1.1.1 8.8.8.8".
	dnsServersRe = regexp.MustCompile(`^DNS Servers:\s+(.+)$`)
	// linkDNSEntryRe matches the one-line-per-link form "Link 2 (enp3s0): 1.1.1.1 8.8.8.8".
	linkDNSEntryRe = regexp.MustCompile(`^Link\s+\d+\s+\(([^)]+)\)\s*:\s*(.+)$`)
)

// windowsAdapterDNS is one row of the per-interface DNS query: the adapter's
// identity (so we can tell a physical hardware interface from a virtual
// tunnel) plus the DNS servers assigned to it.
type windowsAdapterDNS struct {
	Name    string `json:"name"`
	IfType  uint   `json:"ifType"`
	Desc    string `json:"desc"`
	DNSList string `json:"dns"`
}

// readWindowsResolvers enumerates DNS servers per network adapter and tags
// each with whether it belongs to a physical hardware interface (local) or a
// virtual tunnel interface (VPN). This mirrors `ipconfig /all`, which lists
// each adapter block ("无线局域网适配器 WLAN", "以太网适配器 Ethernet", ...)
// with its own "DNS 服务器" line — a flat Get-DnsClientServerAddress dump
// cannot tell a WLAN's DNS from a WireGuard tunnel's, which is what caused
// virtual-adapter resolvers to be mislabelled as system DNS.
func readWindowsResolvers() ([]systemResolver, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	script := `
# Force UTF-8 on stdout. Windows PowerShell 5.1 writes to a redirected
# stdout with the system ANSI code page (e.g. GBK on zh-CN) by default,
# so a Chinese interface name like "以太网" would arrive as GBK bytes and
# be mis-decoded as UTF-8 by Go, producing U+FFFD garbage in the source
# column. Setting the console encoding guarantees UTF-8 regardless of the
# process code page.
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$rows = @()
foreach ($a in (Get-NetAdapter -ErrorAction SilentlyContinue)) {
    if ($a.Status -eq 'Disabled') { continue }
    $dns = @(Get-DnsClientServerAddress -InterfaceIndex $a.ifIndex -AddressFamily IPv4,IPv6 -ErrorAction SilentlyContinue | ForEach-Object { $_.ServerAddresses })
    if ($dns.Count -gt 0) {
        $rows += [PSCustomObject]@{
            name   = $a.Name
            ifType = $a.ifType
            desc   = $a.InterfaceDescription
            dns    = ($dns -join ',')
        }
    }
}
$rows | ConvertTo-Json -Compress
`
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script)
	sysexec.Hide(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("per-interface DNS enumeration: %w", err)
	}

	var adapters []windowsAdapterDNS
	if err := json.Unmarshal(out, &adapters); err != nil {
		// PowerShell may emit an empty JSON when no adapter has DNS;
		// ConvertTo-Json on an empty $rows yields nothing (empty output).
		if len(strings.TrimSpace(string(out))) == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("parse per-interface DNS JSON: %w", err)
	}

	var servers []systemResolver
	seen := make(map[string]bool)
	for _, a := range adapters {
		physical := isPhysicalIface(a.IfType, a.Desc)
		for _, ip := range strings.Split(a.DNSList, ",") {
			ip = strings.TrimSpace(ip)
			// IPv6 link-local addresses come back with a scope id ("%12");
			// net.ParseIP rejects it, and the probe would fail anyway without
			// a zone. Strip it — the probe then treats such a resolver as a
			// timeout rather than a false leak.
			if i := strings.IndexByte(ip, '%'); i >= 0 {
				ip = ip[:i]
			}
			if net.ParseIP(ip) == nil || seen[ip] {
				continue
			}
			seen[ip] = true
			servers = append(servers, systemResolver{
				IP:      ip,
				IsLocal: physical,
				IsVPN:   !physical,
				Iface:   a.Name,
			})
		}
	}
	return servers, nil
}

// isPhysicalIface reports whether a Windows adapter is real hardware (WLAN,
// Ethernet) rather than a virtual tunnel. The interface description is
// checked FIRST because virtual adapters frequently masquerade as physical
// types: OpenVPN TAP emulates ethernet (ifType 6), Tailscale's interface
// shows "Tailscale Tunnel", and some VPN drivers report ifType 6/71 too.
// Only when the description gives no signal do we fall back to the IANA
// ifType MIB value: 6 = ethernetCsmacd, 71 = ieee80211, 72 = ieee80216WMAN,
// 117 = gigabitEthernet, 243 = ieee802154; tunnels (Wintun/WireGuard, TAP)
// report 131 (tunnel) or 65534 (tunnel, commonly used by VPN software).
func isPhysicalIface(ifType uint, desc string) bool {
	d := strings.ToLower(desc)
	for _, kw := range []string{
		"wintun", "wireguard", "openvpn", "tailscale", "nordlynx",
		"tap-windows", "tap adapter", "virtual", "tunnel", "vpn", "ppp",
	} {
		if strings.Contains(d, kw) {
			return false
		}
	}
	switch ifType {
	case 6, 71, 72, 117, 243, 69, 73, 75, 76, 77, 78, 79, 80:
		return true
	case 131, 65534, 53, 1:
		return false
	}
	// Unknown adapter types default to physical — the conservative choice
	// for leak detection (a real physical interface's DNS flagged as local).
	return true
}

// ifaceNameIsVPN reports whether a network interface name belongs to a
// virtual tunnel rather than physical hardware. Used on Linux (resolvectl
// reports "Link N (wg0)") and macOS (scutil reports "if_index : 4 (en0)").
// Matches by name prefix: wg*/tun*/tap*/utun*/ppp* are VPN-ish, en*/eth*/
// wlan*/wl* are hardware.
func ifaceNameIsVPN(name string) bool {
	n := strings.ToLower(name)
	switch {
	case strings.HasPrefix(n, "wg"), strings.HasPrefix(n, "tun"), strings.HasPrefix(n, "tap"),
		strings.HasPrefix(n, "utun"), strings.HasPrefix(n, "ppp"), strings.HasPrefix(n, "vpn"),
		strings.HasPrefix(n, "tailscale"), strings.HasPrefix(n, "zerotier"),
		strings.Contains(n, "openvpn"):
		return true
	}
	return false
}

// readDarwinResolvers uses `scutil --dns` to extract nameserver addresses.
// macOS reports per-scope resolver blocks; each block carries its own
// "if_index : 4 (en0)" line identifying the interface, so we can tag utun*
// resolvers as vpn and en*/awdl* as local — the same classification the
// Windows and Linux paths apply.
func readDarwinResolvers() ([]systemResolver, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "scutil", "--dns").Output()
	if err != nil {
		return nil, fmt.Errorf("scutil --dns: %w", err)
	}
	return parseDarwinScutil(string(out)), nil
}

// parseDarwinScutil parses `scutil --dns` output. macOS groups resolvers
// into blocks; each block lists its nameservers first, then an "if_index :
// 4 (en0)" line. The nameservers and the interface are collected per block
// and attributed together when the block ends (utun* → vpn, en*/awdl* →
// local). A block with no if_index (the primary resolver) is treated as
// local, since it mirrors the hardware interface's config.
func parseDarwinScutil(out string) []systemResolver {
	var servers []systemResolver
	seen := make(map[string]bool)
	// Pending nameservers and interface for the block currently being
	// parsed: macOS emits nameserver lines BEFORE the if_index line, so we
	// cannot tag them until the block's interface is known.
	var pending []string
	var blockIface string
	flush := func() {
		vpn := ifaceNameIsVPN(blockIface)
		for _, ip := range pending {
			if i := strings.IndexByte(ip, '%'); i >= 0 {
				ip = ip[:i]
			}
			if net.ParseIP(ip) == nil || seen[ip] {
				continue
			}
			seen[ip] = true
			servers = append(servers, systemResolver{
				IP:      ip,
				IsLocal: !vpn,
				IsVPN:   vpn,
				Iface:   blockIface,
			})
		}
		pending = nil
		blockIface = ""
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "resolver #") || strings.HasPrefix(line, "scoped resolver #"):
			// A new resolver block starts: flush whatever the previous
			// block accumulated and reset the interface attribution.
			flush()
		case strings.HasPrefix(line, "if_index :"):
			// Format: "if_index : 4 (en0)"
			if m := scutilIfaceRe.FindStringSubmatch(line); m != nil {
				blockIface = m[1]
			}
		case strings.HasPrefix(line, "nameserver["):
			// Format: "nameserver[0] : 8.8.8.8"
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				pending = append(pending, strings.TrimSpace(parts[1]))
			}
		}
	}
	flush() // trailing block after the last resolver header
	return servers
}

// scutilIfaceRe matches "if_index : 4 (en0)" and captures the interface name.
var scutilIfaceRe = regexp.MustCompile(`if_index\s*:\s*\d+\s+\(([^)]+)\)`)
