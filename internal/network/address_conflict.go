package network

import (
	"net"
)

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
