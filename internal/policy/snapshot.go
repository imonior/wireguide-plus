package policy

import "github.com/imonior/wireguide-plus/internal/domain"

// SnapshotFrom builds the analyzer input from the caller's data sources.
//
// The analyzers never read files or talk to the helper themselves — that
// would make them depend on process state and break determinism. Callers
// (GUI and helper) supply closures instead, so the policy layer stays pure
// and trivially testable (principle 30).
//
// load returns a tunnel's config; meta returns its private policy flags. A
// failing tunnel is skipped rather than aborting the whole snapshot: a
// broken config must not stop the other tunnels from being analysed.
func SnapshotFrom(
	names []string,
	load func(string) (*domain.WireGuardConfig, error),
	meta func(string) (Meta, error),
	active func(string) bool,
) []TunnelView {
	out := make([]TunnelView, 0, len(names))
	for _, name := range names {
		cfg, err := load(name)
		if err != nil || cfg == nil {
			continue
		}
		view := TunnelView{
			Name:   name,
			Active: active != nil && active(name),
		}
		for _, peer := range cfg.Peers {
			view.AllowedIPs = append(view.AllowedIPs, peer.AllowedIPs...)
		}
		if meta != nil {
			if m, err := meta(name); err == nil {
				view.TrafficProtect = m.TrafficProtect
				view.Domains = m.Domains
				view.UseAsDefaultDNS = m.UseAsDefaultDNS
				view.DNSResolvePath = m.DNSResolvePath
			}
		}
		out = append(out, view)
	}
	return out
}

// Meta carries the WireGuide Plus private policies of one tunnel, as read
// from the .meta.json sidecar. Defined here (rather than importing storage)
// so the policy package has no dependency on persistence.
type Meta struct {
	TrafficProtect  bool
	Domains         []string
	UseAsDefaultDNS bool
	DNSResolvePath  bool
}
