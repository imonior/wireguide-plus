package app

import (
	"github.com/imonior/wireguide-plus/internal/storage"
)

// PhysicalInterface describes one network interface offered in the
// per-tunnel "physical egress" dropdown (Settings → interface binding).
//
// The naming is deliberately split: `Friendly` is the GENERIC name a human
// recognises ("以太网" / "WLAN" / "Wi-Fi" / "Ethernet"), while `Hardware`
// carries the vendor model string ("Realtek PCIe GbE Family Controller").
// Showing only the hardware name made every entry look alike and gave no
// clue which port was which.
type PhysicalInterface struct {
	Index      int    `json:"index"`       // OS interface index (ifIndex)
	Name       string `json:"name"`        // OS device name ("en0", "wlan0", "Ethernet")
	Friendly   string `json:"friendly"`    // generic name: "Wi-Fi", "以太网", "Ethernet"
	Hardware   string `json:"hardware"`    // vendor/model detail or device name
	IsPhysical bool   `json:"is_physical"` // hardware NIC (not tunnel/loopback/virtual)
	IsUp       bool   `json:"is_up"`
	HasDefault bool   `json:"has_default"` // carries a default route (v4 or v6)
}

// TunnelMetaBinding is the wire form of the per-tunnel egress binding.
type TunnelMetaBinding struct {
	BindIfIndex int    `json:"bind_if_index"`
	BindIfName  string `json:"bind_if_name"`
}

// ListPhysicalInterfaces enumerates candidate egress interfaces for the
// per-tunnel binding dropdown. Platform implementations live in
// interface_ops_windows.go / interface_ops_linux.go / interface_ops_other.go.
func (s *TunnelService) ListPhysicalInterfaces() ([]PhysicalInterface, error) {
	return listPhysicalInterfaces()
}

// GetTunnelBinding returns the saved physical-egress binding for a tunnel.
func (s *TunnelService) GetTunnelBinding(name string) (*TunnelMetaBinding, error) {
	meta, err := s.tunnelStore.LoadMeta(name)
	if err != nil {
		return nil, err
	}
	return &TunnelMetaBinding{
		BindIfIndex: meta.BindIfIndex,
		BindIfName:  meta.BindIfName,
	}, nil
}

// SetTunnelBinding persists (or clears, when ifIndex <= 0) the physical
// egress binding of a tunnel in its meta sidecar.
func (s *TunnelService) SetTunnelBinding(name string, ifIndex int, ifName string) error {
	return s.tunnelStore.UpdateMeta(name, func(meta *storage.TunnelMeta) {
		if ifIndex <= 0 {
			meta.BindIfIndex = 0
			meta.BindIfName = ""
			return
		}
		meta.BindIfIndex = ifIndex
		meta.BindIfName = ifName
	})
}
