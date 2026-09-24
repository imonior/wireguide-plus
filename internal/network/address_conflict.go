package network

import (
	"fmt"
	"net"
)

// AddressConflictError is returned by AssignAddress when the address could
// not be assigned because a DIFFERENT interface already holds it — the
// "two WireGuard clients running the same tunnel" situation. Callers (the
// automation engine) classify it with errors.As to pause auto-connect for
// the tunnel until the user decides; the message keeps naming the holding
// adapter so logs and dialogs stay actionable.
type AddressConflictError struct {
	Address string // the requested address, CIDR as configured
	Holder  string // the other interface that already owns it
	Kind    string // display noun: "adapter" on Windows, "interface" elsewhere
	Err     error  // the underlying command failure
}

func (e *AddressConflictError) Error() string {
	kind := e.Kind
	if kind == "" {
		kind = "interface"
	}
	return fmt.Sprintf("assigning address %s: %v (address is already in use by %s %q — another WireGuard client appears to be running the same tunnel)",
		e.Address, e.Err, kind, e.Holder)
}

func (e *AddressConflictError) Unwrap() error { return e.Err }

// addressOnInterface reports whether ip is already assigned to the named
// interface. Used to make AssignAddress idempotent: a connect attempt that
// crashed before rollback (or a half-failed netsh run) can leave the desired
// address in place, and re-assigning it must succeed rather than abort the
// whole connect.
func addressOnInterface(ifaceName string, ip net.IP) bool {
	if ifaceName == "" || ip == nil {
		return false
	}
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return false
	}
	return interfaceHasIP(*iface, ip)
}

// interfaceHoldingIP returns the name of the interface that currently owns
// ip, or "" when none does. Used to turn netsh's bare "The object already
// exists." into an actionable message naming the competing adapter — most
// commonly another WireGuard client running the same tunnel on the same
// Address.
func interfaceHoldingIP(ip net.IP) string {
	if ip == nil {
		return ""
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for i := range ifaces {
		if interfaceHasIP(ifaces[i], ip) {
			return ifaces[i].Name
		}
	}
	return ""
}

// interfaceHasIP reports whether the given interface has ip among its
// unicast addresses.
func interfaceHasIP(iface net.Interface, ip net.IP) bool {
	addrs, err := iface.Addrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && ipnet.IP.Equal(ip) {
			return true
		}
	}
	return false
}
