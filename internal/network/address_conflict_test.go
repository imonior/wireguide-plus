package network

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestAddressConflictError_AsAndMessage(t *testing.T) {
	inner := errors.New("exit status 1 (The object already exists.)")
	orig := &AddressConflictError{Address: "10.20.25.5/32", Holder: "TS453Dmini_hpg168.f3322.net", Kind: "adapter", Err: inner}

	// Callers classify through wrapping layers (TunnelError, engine
	// phase prefixes) — errors.As must find the conflict type.
	wrapped := fmt.Errorf("assigning address: %w", orig)
	var acf *AddressConflictError
	if !errors.As(wrapped, &acf) {
		t.Fatalf("errors.As did not find AddressConflictError through wrapping")
	}
	if acf.Address != "10.20.25.5/32" || acf.Holder != "TS453Dmini_hpg168.f3322.net" {
		t.Fatalf("payload fields lost: %+v", acf)
	}
	msg := acf.Error()
	for _, want := range []string{"10.20.25.5/32", `"TS453Dmini_hpg168.f3322.net"`, "adapter", "another WireGuard client"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q missing %q", msg, want)
		}
	}
	if !errors.Is(wrapped, inner) {
		t.Errorf("Unwrap chain broken: %v", wrapped)
	}
}

func TestAddressConflictError_KindDefaultsToInterface(t *testing.T) {
	e := &AddressConflictError{Address: "10.0.0.2/32", Holder: "utun3", Err: errors.New("busy")}
	if !strings.Contains(e.Error(), `interface "utun3"`) {
		t.Errorf("empty Kind should render as %q, got %q", "interface", e.Error())
	}
}
