package app

import (
	"github.com/imonior/wireguide-plus/internal/diag"
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
	IP         string `json:"ip"`
	Hostname   string `json:"hostname"`
	IsVPN      bool   `json:"is_vpn"`
	Responds   bool   `json:"responds"`
	LatencyMs  int    `json:"latency_ms"`
	Status     string `json:"status"`
	Encryption string `json:"encryption"`
}

// RouteEntry mirrors diag.RouteEntry for Wails JSON serialisation.
type RouteEntry struct {
	Destination string `json:"destination"`
	Gateway     string `json:"gateway"`
	Interface   string `json:"interface"`
	Flags       string `json:"flags"`
	IsVPN       bool   `json:"is_vpn,omitempty"`
}

// RunDNSLeakTest performs a DNS leak test using the currently active tunnel's
// DNS servers as the expected (VPN) resolvers. If no tunnel is connected, the
// expected set is empty — all detected resolvers will be flagged as leaks.
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

	r := diag.RunDNSLeakTest(expectedDNS)
	out := &DNSLeakResult{
		Leaked:     r.Leaked,
		TestDomain: r.TestDomain,
		Error:      r.Error,
	}
	for _, srv := range r.DNSServers {
		out.DNSServers = append(out.DNSServers, DNSServer{
			IP:         srv.IP,
			Hostname:   srv.Hostname,
			IsVPN:      srv.IsVPN,
			Responds:   srv.Responds,
			LatencyMs:  srv.LatencyMs,
			Status:     srv.Status,
			Encryption: srv.Encryption,
		})
	}
	return out, nil
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
