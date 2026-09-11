package helper

import (
	"errors"
	"testing"
)

// stubFirewall is a FirewallManager whose DNS-protection half records calls;
// the kill-switch / endpoint-primitive half is unreachable from the code
// paths under test and stays panic-on-use so accidental use is loud.
type stubFirewall struct {
	disableDNSCalls int
	disableDNSErr   error
}

func (s *stubFirewall) EnableKillSwitch(string, []string, []string) error          { panic("not used in test") }
func (s *stubFirewall) AddKillSwitchTunnel(string, []string, []string) error       { panic("not used in test") }
func (s *stubFirewall) RemoveKillSwitchTunnel(string) error                        { panic("not used in test") }
func (s *stubFirewall) DisableKillSwitch() error                                   { panic("not used in test") }
func (s *stubFirewall) EnableEndpointProtection(string, []string) error            { panic("not used in test") }
func (s *stubFirewall) DisableEndpointProtection(string) error                     { panic("not used in test") }
func (s *stubFirewall) EnableDNSProtection(string, []string) error                 { panic("not used in test") }
func (s *stubFirewall) IsKillSwitchEnabled() bool                                  { return false }
func (s *stubFirewall) IsDNSProtectionEnabled() bool                               { return false }
func (s *stubFirewall) Cleanup() error                                             { panic("not used in test") }
func (s *stubFirewall) RecoverFromCrash() bool                                     { return false }
func (s *stubFirewall) DisableDNSProtection() error {
	s.disableDNSCalls++
	return s.disableDNSErr
}

// handleClearDNSPathEnforcement must tear down a live enforcement when an
// owner exists (master switch OFF means the feature stops existing — the
// rules a still-connected tunnel installed may not outlive the toggle).
func TestClearDNSPathEnforcementTearsDownOwner(t *testing.T) {
	fw := &stubFirewall{}
	h := &Helper{firewall: fw}
	h.dnsPathOwner = "alpha"

	if _, err := h.handleClearDNSPathEnforcement(nil); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if fw.disableDNSCalls != 1 {
		t.Errorf("DisableDNSProtection calls = %d, want 1", fw.disableDNSCalls)
	}
	if h.dnsPathOwner != "" {
		t.Errorf("dnsPathOwner = %q, want cleared", h.dnsPathOwner)
	}
}

// No owner (nothing enforcing) → strict no-op: DisableDNSProtection must not
// run, otherwise the master-off teardown could rip out rules another flow
// (suspend/resume around reconnect) still needs.
func TestClearDNSPathEnforcementNoOwnerIsNoop(t *testing.T) {
	fw := &stubFirewall{}
	h := &Helper{firewall: fw}

	if _, err := h.handleClearDNSPathEnforcement(nil); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if fw.disableDNSCalls != 0 {
		t.Errorf("DisableDNSProtection calls = %d, want 0", fw.disableDNSCalls)
	}
}

// A firewall error must not leave the owner latched: the GUI already
// persisted master=off, so a wedged ruleset would silently keep blocking
// port 53 forever if the owner were kept and reconnect re-enabled nothing.
func TestClearDNSPathEnforcementErrorStillClearsOwner(t *testing.T) {
	fw := &stubFirewall{disableDNSErr: errors.New("pfctl: bad state")}
	h := &Helper{firewall: fw}
	h.dnsPathOwner = "beta"

	if _, err := h.handleClearDNSPathEnforcement(nil); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if fw.disableDNSCalls != 1 {
		t.Errorf("DisableDNSProtection calls = %d, want 1", fw.disableDNSCalls)
	}
	if h.dnsPathOwner != "" {
		t.Errorf("dnsPathOwner = %q, want cleared despite firewall error", h.dnsPathOwner)
	}
}
