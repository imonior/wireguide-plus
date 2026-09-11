package app

import (
	"log/slog"

	"github.com/imonior/wireguide-plus/internal/policy"
)

// PolicyReport returns the conflicts involving one tunnel ("" = all
// tunnels) across the routing, DNS and traffic-protection domains.
//
// This is the window the frontend uses for BOTH halves of the manual
// Conflict Policy (principles 22-24):
//   - Save:            always allowed, warnings surfaced in the editor.
//   - Manual connect:  blocked by default when the report is blocking;
//     the user can override with "Connect Anyway".
//
// The call is a pure read — it never connects or disconnects anything and
// never rewrites configuration (principle 26/30).
func (s *TunnelService) PolicyReport(name string) (policy.Report, error) {
	report, err := s.analyzePolicy()
	if err != nil {
		return policy.Report{}, err
	}
	if name == "" {
		return report, nil
	}
	return report.ForTunnel(name), nil
}

// PolicyReportAll is the unfiltered report, used by the settings/tunnels
// overview to show every conflict at once.
func (s *TunnelService) PolicyReportAll() (policy.Report, error) {
	return s.analyzePolicy()
}

// analyzePolicy builds the tunnel snapshot and runs the analyzers.
func (s *TunnelService) analyzePolicy() (policy.Report, error) {
	names, err := s.tunnelStore.List()
	if err != nil {
		return policy.Report{}, err
	}
	// The master switch gates the whole feature: when it is off a tunnel's
	// stored DNSResolvePath flag is inert — not shown in the UI, not
	// enforced, and not reported as a conflict.
	master := false
	if st, err := s.settingsStore.Load(); err == nil {
		master = st.DNSResolvePath
	}
	views := policy.SnapshotFrom(names,
		s.tunnelStore.Load,
		func(n string) (policy.Meta, error) {
			m, err := s.tunnelStore.LoadMeta(n)
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
		s.isTunnelActive,
	)
	return policy.Analyze(views), nil
}

// isTunnelActive reports whether a tunnel is currently up. Best-effort: a
// failed status query degrades to "not active", which only affects how
// conflicts are ranked, not which conflicts exist.
func (s *TunnelService) isTunnelActive(name string) bool {
	st, err := s.GetStatus()
	if err != nil || st == nil {
		return false
	}
	for _, n := range st.ActiveTunnels {
		if n == name {
			return true
		}
	}
	if st.TunnelName == name && st.InterfaceName != "" {
		return true
	}
	for _, t := range st.Tunnels {
		if t.TunnelName == name && t.InterfaceName != "" {
			return true
		}
	}
	return false
}

// logPolicyWarnings records what the analyzers found after a config save.
// Saving is ALWAYS allowed (principle 22): a conflicting configuration is
// not an invalid one — the user may be mid-edit, or may intend to run the
// tunnels at different times. The analyzer informs; it never blocks a save
// and never rewrites AllowedIPs (principle 26).
func (s *TunnelService) logPolicyWarnings(name string) {
	report, err := s.PolicyReport(name)
	if err != nil || report.Empty() {
		return
	}
	for _, c := range report.Routes {
		slog.Info("policy: route conflict", "category", "policy",
			"tunnel", name, "kind", c.Kind, "severity", c.Severity, "detail", c.Message)
	}
	for _, c := range report.DNS {
		slog.Info("policy: dns conflict", "category", "policy",
			"tunnel", name, "kind", c.Kind, "severity", c.Severity, "detail", c.Message)
	}
	for _, c := range report.Protection {
		slog.Info("policy: protection conflict", "category", "policy",
			"tunnel", name, "kind", c.Kind, "severity", c.Severity, "detail", c.Message)
	}
	if report.Blocking() {
		slog.Warn("policy: saved configuration has a blocking conflict — connecting will require confirmation",
			"category", "policy", "tunnel", name)
	}
}
