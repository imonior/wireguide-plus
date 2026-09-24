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
	// KeepConnectionOnIdle is the per-tunnel override of the global
	// "keep connection on idle" switch. nil in JSON means "inherit global"
	// (encoded as the field being absent); the GUI sends null for that
	// state and true/false for an explicit override.
	KeepConnectionOnIdle *bool `json:"keep_connection_on_idle,omitempty"`
	// AutomationDisabled exempts this tunnel from the automation engine:
	// its rules/Default State stay stored, but the helper never acts on
	// them. Persisted (unlike the manual-off latch, which is per-session),
	// so "leave this tunnel to me" survives an app restart.
	AutomationDisabled bool `json:"automation_disabled"`
}

// GetTunnelPolicies returns the private policies of a tunnel.
func (s *TunnelService) GetTunnelPolicies(name string) (*TunnelPolicies, error) {
	meta, err := s.tunnelStore.LoadMeta(name)
	if err != nil {
		return nil, err
	}
	return &TunnelPolicies{
		TrafficProtect:       meta.TrafficProtect,
		Domains:              meta.Domains,
		UseAsDefaultDNS:      meta.UseAsDefaultDNS,
		DNSResolvePath:       meta.DNSResolvePath,
		KeepConnectionOnIdle: meta.KeepConnectionOnIdle,
		AutomationDisabled:   meta.AutomationDisabled,
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
	prevMeta, prevErr := s.tunnelStore.LoadMeta(name)
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
		meta.KeepConnectionOnIdle = policies.KeepConnectionOnIdle
		meta.AutomationDisabled = policies.AutomationDisabled
	}); err != nil {
		return err
	}
	// Turning automation back ON is a user decision about the tunnel's
	// automation, so it also lifts any address-conflict pause held from
	// before — otherwise the exemption the dialog paused them on would
	// silently keep the engine away even after the user re-enabled it.
	// Best-effort: helper-not-running just means nothing to lift.
	if prevErr == nil && prevMeta != nil && prevMeta.AutomationDisabled && !policies.AutomationDisabled {
		_ = s.ResumeAutoConnect(name)
	}
	s.logPolicyWarnings(name)
	return nil
}
