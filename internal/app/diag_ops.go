package app

import (
	"context"
	"net"
	"strings"
	"time"

	"github.com/imonior/wireguide-plus/internal/diag"
	"github.com/imonior/wireguide-plus/internal/storage"
)

// DNSLeakResult mirrors diag.DNSLeakResult for Wails JSON serialisation.
type DNSLeakResult struct {
	Leaked     bool        `json:"leaked"`
	DNSServers []DNSServer `json:"dns_servers"`
	TestDomain string      `json:"test_domain"`
	Error      string      `json:"error,omitempty"`
}

// DNSServer mirrors diag.DNSServer.
type DNSServer struct {
	IP          string `json:"ip"`
	Hostname    string `json:"hostname"`
	IsVPN       bool   `json:"is_vpn"`
	IsLocal     bool   `json:"is_local"`
	SourceIface string `json:"source_iface,omitempty"`
	Responds    bool   `json:"responds"`
	LatencyMs   int    `json:"latency_ms"`
	Status      string `json:"status"`
	Encryption  string `json:"encryption"`
}

// RouteEntry mirrors diag.RouteEntry for Wails JSON serialisation.
type RouteEntry struct {
	Destination string `json:"destination"`
	Gateway     string `json:"gateway"`
	Interface   string `json:"interface"`
	Flags       string `json:"flags"`
	IsVPN       bool   `json:"is_vpn,omitempty"`
}

// effectivePublicResolvers returns the public-resolver cross-check list the
// DNS leak test will probe. Precedence:
//
//	user's customized list (non-empty)  → verbatim
//	network-fetched list (non-empty)    → verbatim (live refresh)
//	otherwise                           → built-in defaults
//
// A nil/empty result is never passed to diag: public cross-check probing is
// core to the leak test and always remains enabled. The system (local/VPN)
// DNS block is completely independent of this list and is always probed and
// displayed first.
func (s *TunnelService) effectivePublicResolvers() []string {
	st, err := s.settingsStore.Load()
	if err == nil {
		if len(st.DNSTestPublicServers) > 0 {
			return append([]string(nil), st.DNSTestPublicServers...)
		}
		if len(st.DNSTestPublicFetched) > 0 {
			return append([]string(nil), st.DNSTestPublicFetched...)
		}
	}
	return diag.DefaultPublicResolvers()
}

// RunDNSLeakTest performs a DNS leak test using the currently active tunnel's
// DNS servers as the expected (VPN) resolvers. If no tunnel is connected, the
// expected set is empty — all detected resolvers will be flagged as leaks.
//
// The result always lists the system-configured (local/VPN) resolvers FIRST,
// in the same order `ipconfig /all` / `scutil --dns` report them, with the
// public cross-check resolvers after. Public cross-check probing is never
// disabled by the settings (an empty custom list just falls back to the
// built-in / network-fetched defaults).
func (s *TunnelService) RunDNSLeakTest() (*DNSLeakResult, error) {
	// Best-effort: find the active tunnel's DNS config to know which resolvers
	// are expected to be in use. Ignore IPC errors — an empty expected set is
	// still a valid (conservative) test.
	var expectedDNS []string
	if status, err := s.GetStatus(); err == nil && status != nil && status.TunnelName != "" {
		if cfg, err := s.tunnelStore.Load(status.TunnelName); err == nil && cfg != nil {
			expectedDNS = cfg.Interface.DNS
		}
	}

	publicDNS := s.effectivePublicResolvers()
	r := diag.RunDNSLeakTestWithPublic(expectedDNS, publicDNS)
	out := &DNSLeakResult{
		Leaked:     r.Leaked,
		TestDomain: r.TestDomain,
		Error:      r.Error,
	}
	for _, srv := range r.DNSServers {
		out.DNSServers = append(out.DNSServers, DNSServer{
			IP:          srv.IP,
			Hostname:    srv.Hostname,
			IsVPN:       srv.IsVPN,
			IsLocal:     srv.IsLocal,
			SourceIface: srv.SourceIface,
			Responds:    srv.Responds,
			LatencyMs:   srv.LatencyMs,
			Status:      srv.Status,
			Encryption:  srv.Encryption,
		})
	}
	return out, nil
}

// GetPublicDNSServers returns the public-resolver cross-check list the DNS
// leak test will probe — i.e. the same list effectivePublicResolvers
// computes. The UI seeds its editable list from this value; saving an edited
// (or empty) list back via SavePublicDNSServers persists it as the user's
// custom list.
func (s *TunnelService) GetPublicDNSServers() []string {
	return s.effectivePublicResolvers()
}

// SavePublicDNSServers persists the user's customized public-resolver list
// for the DNS leak test. An empty list clears the customization and restores
// the default cross-check set (network-fetched if available, else built-in) —
// it does NOT disable public probing, which is core to the feature. Entries
// are trimmed and deduplicated; invalid entries are skipped. Only entries
// that are valid IP addresses or resolvable hostnames are kept.
func (s *TunnelService) SavePublicDNSServers(list []string) error {
	clean := sanitizeDNSServerList(list)
	return s.settingsStore.Update(func(st *storage.Settings) error {
		st.DNSTestPublicServers = clean // empty ⇒ clear custom list ⇒ fall back to defaults
		return nil
	})
}

// ResetPublicDNSServers clears the user's customized public-resolver list so
// the DNS leak test falls back to the network-fetched list (if available)
// or the built-in defaults.
func (s *TunnelService) ResetPublicDNSServers() error {
	return s.settingsStore.Update(func(st *storage.Settings) error {
		st.DNSTestPublicServers = nil
		return nil
	})
}

// RefreshPublicDNSServers fetches the latest public DNS resolver list from
// the network (public-dns.info) and caches it in settings as the new default
// cross-check set. On success the fetched list becomes effective immediately
// unless the user has a custom list (which still takes precedence). The
// result reports the fetched list, the list now in effect, the fetch
// timestamp, and whether the fetch itself succeeded.
func (s *TunnelService) RefreshPublicDNSServers() (*PublicDNSRefresh, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	list, err := diag.FetchPublicResolvers(ctx)
	if err != nil {
		// Keep whatever was in effect before; report the error so the UI can
		// tell the user the refresh failed without losing the old list.
		return &PublicDNSRefresh{
			Fetched:   nil,
			Effective: s.effectivePublicResolvers(),
			FetchedAt: 0,
			Error:     err.Error(),
		}, nil
	}
	if err := s.settingsStore.Update(func(st *storage.Settings) error {
		st.DNSTestPublicFetched = append([]string(nil), list...)
		st.DNSTestPublicFetchedAt = time.Now().Unix()
		return nil
	}); err != nil {
		return nil, err
	}
	return &PublicDNSRefresh{
		Fetched:   append([]string(nil), list...),
		Effective: s.effectivePublicResolvers(),
		FetchedAt: time.Now().Unix(),
	}, nil
}

// PublicDNSRefresh is the result of RefreshPublicDNSServers: the list
// fetched from the live feed, the resolver list now in effect (custom list
// takes precedence over the fetched one), when the live feed was last
// fetched (0 = never / failed), and an optional fetch error string.
type PublicDNSRefresh struct {
	Fetched   []string `json:"fetched,omitempty"`
	Effective []string `json:"effective"`
	FetchedAt int64    `json:"fetched_at"`
	Error     string   `json:"error,omitempty"`
}

// sanitizeDNSServerList trims, deduplicates and keeps only valid DNS server
// addresses (IP or hostname). Invalid entries are skipped, never rejected
// wholesale — a single typo must not block saving the rest of the list.
func sanitizeDNSServerList(list []string) []string {
	seen := make(map[string]bool, len(list))
	out := make([]string, 0, len(list))
	for _, raw := range list {
		s := strings.TrimSpace(raw)
		if s == "" || seen[s] {
			continue
		}
		if !validDNSServerAddr(s) {
			continue // not a valid IP/hostname — drop it silently
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// validDNSServerAddr reports whether s is a bare IP address or a plausible
// hostname (labels of letters/digits/hyphens separated by dots). It must not
// contain a port, path or scheme — the leak test dials it as a resolver.
func validDNSServerAddr(s string) bool {
	if net.ParseIP(s) != nil {
		return true
	}
	if strings.ContainsAny(s, ":/@ ") || strings.HasPrefix(s, ".") || strings.HasSuffix(s, ".") {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		for _, r := range label {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
				continue
			}
			return false
		}
	}
	return true
}

// GetRoutingTable returns the current OS routing table, flagging every row
// whose interface matches an active tunnel so the UI can distinguish
// VPN traffic from direct traffic.
func (s *TunnelService) GetRoutingTable() ([]RouteEntry, error) {
	entries, err := diag.GetRoutingTable()
	if err != nil {
		return nil, err
	}
	// Collect the interface names of every currently active tunnel
	// (primary + multi-tunnel members).
	vpnIfaces := map[string]bool{}
	if st, err := s.GetStatus(); err == nil && st != nil {
		all := make([]ConnectionStatus, 0, 1+len(st.Tunnels))
		all = append(all, *st)
		all = append(all, st.Tunnels...)
		for _, t := range all {
			if t.InterfaceName != "" {
				vpnIfaces[t.InterfaceName] = true
			}
		}
	}
	out := make([]RouteEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, RouteEntry{
			Destination: e.Destination,
			Gateway:     e.Gateway,
			Interface:   e.Interface,
			Flags:       e.Flags,
			IsVPN:       vpnIfaces[e.Interface],
		})
	}
	return out, nil
}
