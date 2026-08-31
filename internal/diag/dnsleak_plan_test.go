package diag

import (
	"reflect"
	"testing"
)

func TestBuildProbePlanSystemDNSAlwaysFirst(t *testing.T) {
	system := []systemResolver{
		{IP: "192.168.1.1", IsLocal: true, Iface: "enp3s0"},
		{IP: "10.10.0.1", IsVPN: true, Iface: "wg0"},
	}

	cases := []struct {
		name     string
		public   []string
		wantIPs  []string
		wantLen  int
	}{
		{
			name:    "nil public list falls back to built-in defaults",
			public:  nil,
			wantLen: len(system) + len(publicResolvers),
		},
		{
			name:    "empty public list falls back to built-in defaults",
			public:  []string{},
			wantLen: len(system) + len(publicResolvers),
		},
		{
			name:    "custom non-empty list is appended after system DNS",
			public:  []string{"9.9.9.9", "149.112.112.112"},
			wantIPs: []string{"192.168.1.1", "10.10.0.1", "9.9.9.9", "149.112.112.112"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			plan := buildProbePlan(system, c.public)

			// System DNS must always be present and in front, regardless of
			// the public list — the user's core requirement.
			if len(plan.targets) < len(system) {
				t.Fatalf("probe targets %v dropped system resolvers %v", plan.targets, system)
			}
			for i, s := range system {
				if plan.targets[i] != s.IP {
					t.Errorf("targets[%d] = %q, want system resolver %q first; targets=%v", i, plan.targets[i], s.IP, plan.targets)
				}
			}

			if c.wantIPs != nil {
				if !reflect.DeepEqual(plan.targets, c.wantIPs) {
					t.Errorf("targets = %v, want %v", plan.targets, c.wantIPs)
				}
			} else if len(plan.targets) != c.wantLen {
				t.Errorf("len(targets) = %d, want %d (system %d + defaults %d)",
					len(plan.targets), c.wantLen, len(system), len(publicResolvers))
			}

			// Flag maps must still be populated from the system entries.
			if !plan.localSet["192.168.1.1"] {
				t.Error("localSet missing 192.168.1.1")
			}
			if !plan.vpnSet["10.10.0.1"] {
				t.Error("vpnSet missing 10.10.0.1")
			}
			if plan.ifaceByIP["192.168.1.1"] != "enp3s0" {
				t.Errorf("ifaceByIP[192.168.1.1] = %q, want enp3s0", plan.ifaceByIP["192.168.1.1"])
			}
		})
	}
}

func TestBuildProbePlanDedup(t *testing.T) {
	// A resolver configured on the system that also appears in the public
	// list must keep only its system entry (no duplicate probe).
	system := []systemResolver{
		{IP: "8.8.8.8", IsLocal: true, Iface: "eth0"},
	}
	plan := buildProbePlan(system, []string{"8.8.8.8", "1.1.1.1"})
	want := []string{"8.8.8.8", "1.1.1.1"}
	if !reflect.DeepEqual(plan.targets, want) {
		t.Errorf("targets = %v, want %v", plan.targets, want)
	}
}

func TestBuildProbePlanNoSystemDNS(t *testing.T) {
	// No system resolvers detected — the public list must still probe.
	plan := buildProbePlan(nil, []string{"9.9.9.9"})
	want := []string{"9.9.9.9"}
	if !reflect.DeepEqual(plan.targets, want) {
		t.Errorf("targets = %v, want %v", plan.targets, want)
	}
}
