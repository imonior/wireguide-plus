package helper

import (
	"fmt"
	"strings"

	"github.com/imonior/wireguide-plus/internal/domain"
	"github.com/imonior/wireguide-plus/internal/ipc"
	"github.com/imonior/wireguide-plus/internal/policy"
)

// policyBlockFor evaluates the policy layer for one tunnel and returns a
// non-nil payload when an AUTOMATIC connection must be refused.
//
// This is the automation half of the Conflict Policy (principle 25): block
// by default, then tell the user through a tray notification and the log.
// Manual connects do not come through here — the GUI asks the user with a
// "Connect Anyway" confirmation instead (principle 23/24).
//
// The call is a pure read of the tunnel store: it never connects, never
// rewrites AllowedIPs (principle 26) and never modifies system routes. A
// failure to build the snapshot fails OPEN — an unreadable config must not
// silently disable automation, and the conflict will still surface in the
// GUI's own connect path.
func (h *Helper) policyBlockFor(name string) *ipc.PolicyBlockedPayload {
	if h.userTunnelStore == nil || name == "" {
		return nil
	}
	names, err := h.userTunnelStore.List()
	if err != nil {
		return nil
	}
	// The master switch gates the feature: with it off the per-tunnel flag
	// is inert and must not block automation.
	master := false
	if settings, err := h.loadUserSettings(); err == nil && settings != nil {
		master = settings.DNSResolvePath
	}
	views := policy.SnapshotFrom(names,
		func(n string) (*domain.WireGuardConfig, error) { return h.userTunnelStore.Load(n) },
		func(n string) (policy.Meta, error) {
			m, err := h.userTunnelStore.LoadMeta(n)
			if err != nil || m == nil {
				return policy.Meta{}, err
			}
			return policy.Meta{
				TrafficProtect:  m.TrafficProtect,
				Domains:         m.Domains,
				UseAsDefaultDNS: m.UseAsDefaultDNS,
				DNSResolvePath:  m.DNSResolvePath && master,
			}, nil
		},
		func(n string) bool {
			h.connectMu.Lock()
			defer h.connectMu.Unlock()
			_, ok := h.activeCfgs[n]
			return ok
		},
	)
	report := policy.Analyze(views).ForTunnel(name)
	if !report.Blocking() {
		return nil
	}

	// Report the most specific reason available. Routing ties are the
	// classic case (identical prefixes); DNS and protection are separate
	// analyzer domains (principles 27/28) and get their own wording so the
	// notification tells the user what to actually fix.
	switch {
	case hasBlockingRoute(report):
		return &ipc.PolicyBlockedPayload{
			Tunnel:  name,
			Reason:  "route",
			Summary: describeRouteBlock(name, report),
		}
	case hasBlockingDNS(report):
		return &ipc.PolicyBlockedPayload{Tunnel: name, Reason: "dns", Summary: describeDNSBlock(name, report)}
	default:
		return &ipc.PolicyBlockedPayload{Tunnel: name, Reason: "protection", Summary: describeProtectionBlock(name, report)}
	}
}

func hasBlockingRoute(r policy.Report) bool {
	for _, c := range r.Routes {
		if c.Severity == policy.SeverityBlocking {
			return true
		}
	}
	return false
}

func hasBlockingDNS(r policy.Report) bool {
	for _, c := range r.DNS {
		if c.Severity == policy.SeverityBlocking {
			return true
		}
	}
	return false
}

func describeRouteBlock(name string, r policy.Report) string {
	var parts []string
	for _, c := range r.Routes {
		if c.Severity != policy.SeverityBlocking {
			continue
		}
		other := c.B
		if c.B == name {
			other = c.A
		}
		parts = append(parts, fmt.Sprintf("%s claims the same prefix as %s (%s)", name, other, c.APrefix))
	}
	return strings.Join(parts, "; ")
}

func describeDNSBlock(name string, r policy.Report) string {
	var parts []string
	for _, c := range r.DNS {
		if c.Severity != policy.SeverityBlocking {
			continue
		}
		parts = append(parts, c.Message)
	}
	return strings.Join(parts, "; ")
}

func describeProtectionBlock(name string, r policy.Report) string {
	var parts []string
	for _, c := range r.Protection {
		if c.Severity != policy.SeverityBlocking {
			continue
		}
		parts = append(parts, c.Message)
	}
	return strings.Join(parts, "; ")
}
