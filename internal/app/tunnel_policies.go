package app

import (
	"slices"

	"github.com/imonior/wireguide-plus/internal/storage"
)

// TunnelPolicies is the wire form of a tunnel's WireGuide Plus private
// policies. Like the egress binding, all three live in the .meta.json
// sidecar and NEVER in the .conf (policy principle 1/2) so the config file
// stays a portable standard WireGuard/AWG document.
type TunnelPolicies struct {
	TrafficProtect  bool     `json:"traffic_protect"`
	Domains         []string `json:"domains"`
	UseAsDefaultDNS bool     `json:"use_as_default_dns"`
	DNSResolvePath  bool     `json:"dns_resolve_path"`
}

// GetTunnelPolicies returns the private policies of a tunnel.
func (s *TunnelService) GetTunnelPolicies(name string) (*TunnelPolicies, error) {
	meta, err := s.tunnelStore.LoadMeta(name)
	if err != nil {
		return nil, err
	}
	return &TunnelPolicies{
		TrafficProtect:  meta.TrafficProtect,
		Domains:         meta.Domains,
		UseAsDefaultDNS: meta.UseAsDefaultDNS,
		DNSResolvePath:  meta.DNSResolvePath,
	}, nil
}

// SetTunnelPolicies persists the private policies and re-runs the analyzers.
//
// Saving is always allowed (principle 22) — a conflicting policy is a legal
// configuration, not an invalid one. What the save does do is log what the
// analyzers found, so "why won't my tunnel auto-connect" is answerable from
// the log rather than guessed at. Nothing here rewrites AllowedIPs
// (principle 26).
func (s *TunnelService) SetTunnelPolicies(name string, policies TunnelPolicies) error {
	domains := make([]string, 0, len(policies.Domains))
	for _, d := range policies.Domains {
		d = storage.NormalizeDomainEntry(d)
		if d != "" && !slices.Contains(domains, d) {
			domains = append(domains, d)
		}
	}
	if err := s.tunnelStore.UpdateMeta(name, func(meta *storage.TunnelMeta) {
		meta.TrafficProtect = policies.TrafficProtect
		meta.Domains = domains
		meta.UseAsDefaultDNS = policies.UseAsDefaultDNS
		meta.DNSResolvePath = policies.DNSResolvePath
	}); err != nil {
		return err
	}
	s.logPolicyWarnings(name)
	return nil
}
