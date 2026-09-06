//go:build windows

package app

import (
	"net"
	"strings"

	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

// IfType values from ipifcons.h that mark non-physical interfaces we never
// offer as egress candidates.
const (
	ifTypePPP         = 23
	ifTypeL2TP        = 24
	ifTypeTunnel      = 31
	ifTypePropVirtual = 53 // proprietary virtual (Hyper-V vEthernet etc.)
)

// listPhysicalInterfaces enumerates interfaces for the egress dropdown on
// Windows. Uses the IP helper API (same source as the socket-bind monitor)
// so the description matches what the user sees in ncpa.cpl.
func listPhysicalInterfaces() ([]PhysicalInterface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	out := make([]PhysicalInterface, 0, len(ifaces))
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		if strings.HasPrefix(ifc.Name, "vEthernet (") {
			continue
		}

		isUp := ifc.Flags&net.FlagUp != 0
		friendly := ifc.Name
		hardware := ""
		luid, luidErr := winipcfg.LUIDFromIndex(uint32(ifc.Index))
		if luidErr == nil {
			row, rowErr := luid.Interface()
			if rowErr == nil && row != nil {
				switch row.Type {
				case ifTypePPP, ifTypeL2TP, ifTypeTunnel, ifTypePropVirtual:
					continue
				}
				// Alias is the GENERIC name Windows shows in ncpa.cpl
				// ("以太网", "WLAN", "Ethernet 2"). Description is the
				// vendor model ("Realtek PCIe GbE Family Controller") —
				// useful as a secondary hint, useless as a primary label
				// because every Realtek NIC renders the same string.
				if a := row.Alias(); a != "" {
					friendly = a
				}
				if d := row.Description(); d != "" && d != friendly {
					hardware = d
				}
			}
		}
		if hardware == "" && friendly != ifc.Name {
			hardware = ifc.Name
		}

		hasDefault := false
		for _, family := range []winipcfg.AddressFamily{2, 23} { // AF_INET, AF_INET6
			if ok, _ := networkHasDefaultRoute(family, uint32(ifc.Index)); ok {
				hasDefault = true
				break
			}
		}

		out = append(out, PhysicalInterface{
			Index:      ifc.Index,
			Name:       ifc.Name,
			Friendly:   friendly,
			Hardware:   hardware,
			IsPhysical: true,
			IsUp:       isUp,
			HasDefault: hasDefault,
		})
	}
	return out, nil
}

// networkHasDefaultRoute reports whether the interface has a default route
// in the given family — the same routing-table scan the socket-bind monitor
// uses, via winipcfg.
func networkHasDefaultRoute(family winipcfg.AddressFamily, ifIndex uint32) (bool, error) {
	routes, err := winipcfg.GetIPForwardTable2(family)
	if err != nil {
		return false, err
	}
	for _, row := range routes {
		if row.DestinationPrefix.PrefixLength != 0 {
			continue
		}
		ifRow, err := row.InterfaceLUID.Interface()
		if err != nil {
			continue
		}
		if uint32(ifRow.InterfaceIndex) == ifIndex {
			return true, nil
		}
	}
	return false, nil
}
