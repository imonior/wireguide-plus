//go:build linux

package app

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// listPhysicalInterfaces enumerates interfaces for the egress dropdown on
// Linux. An interface counts as a hardware NIC when it has a backing device
// (/sys/class/net/<name>/device → PCI/USB/SDIO); virtual kinds (lo, bridges,
// veth, TUN devices) are excluded from the IsPhysical set and skipped
// entirely for the dropdown, matching the user's mental model of "physical
// egress".
//
// Naming mirrors the other platforms: Friendly is the GENERIC kind
// ("Wi-Fi" / "Ethernet"), Hardware carries the kernel device name (and the
// driver when readable) so two identical adapters can be told apart.
func listPhysicalInterfaces() ([]PhysicalInterface, error) {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		// Fall back to Go's interface list if sysfs is unavailable.
		return listViaNetInterfaces()
	}
	out := make([]PhysicalInterface, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if name == "lo" {
			continue
		}
		base := filepath.Join("/sys/class/net", name)
		// Hardware NICs have a ../device symlink (PCI/USB/SDIO bus).
		if _, err := os.Stat(filepath.Join(base, "device")); err != nil {
			continue
		}
		operstate := ""
		if b, err := os.ReadFile(filepath.Join(base, "operstate")); err == nil {
			operstate = strings.TrimSpace(string(b))
		}
		ifc, err := net.InterfaceByName(name)
		isUp := err == nil && ifc.Flags&net.FlagUp != 0
		friendly, hardware := linuxIfaceNames(base, name)
		p := PhysicalInterface{
			Index:      -1,
			Name:       name,
			Friendly:   friendly,
			Hardware:   hardware,
			IsPhysical: true,
			IsUp:       isUp && operstate == "up",
		}
		if ifc != nil {
			p.Index = ifc.Index
		}
		if p.IsUp {
			p.HasDefault = linuxHasDefaultRoute(name)
		}
		out = append(out, p)
	}
	return out, nil
}

// linuxIfaceNames derives the generic kind and the hardware detail for one
// sysfs interface. Wireless devices expose a `wireless/` directory or a
// `phy80211` symlink; everything else with a backing device is Ethernet.
func linuxIfaceNames(base, name string) (string, string) {
	kind := "Ethernet"
	if _, err := os.Stat(filepath.Join(base, "wireless")); err == nil {
		kind = "Wi-Fi"
	} else if _, err := os.Stat(filepath.Join(base, "phy80211")); err == nil {
		kind = "Wi-Fi"
	} else if strings.HasPrefix(name, "wl") {
		kind = "Wi-Fi"
	} else if strings.HasPrefix(name, "wwan") || strings.HasPrefix(name, "ww") {
		kind = "Cellular"
	}
	hw := name
	if drv := linuxDriver(base); drv != "" {
		hw = name + " · " + drv
	}
	return kind, hw
}

// linuxDriver reads DRIVER= from the device's uevent. Best-effort: a
// missing/unreadable uevent just leaves the device name as the hardware hint.
func linuxDriver(base string) string {
	b, err := os.ReadFile(filepath.Join(base, "device", "uevent"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(line, "DRIVER="); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// listViaNetInterfaces is the sysfs-unavailable fallback.
func listViaNetInterfaces() ([]PhysicalInterface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	out := make([]PhysicalInterface, 0, len(ifaces))
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		out = append(out, PhysicalInterface{
			Index:      ifc.Index,
			Name:       ifc.Name,
			Friendly:   ifc.Name,
			Hardware:   ifc.Name,
			IsPhysical: true,
			IsUp:       ifc.Flags&net.FlagUp != 0,
		})
	}
	return out, nil
}

// linuxHasDefaultRoute greps `ip route show default dev <name>` for v4 and
// `ip -6 route show default dev <name>` for v6. Exec-free parsing of
// /proc is not reliable for multipath, so shell out to ip(8) — the helper
// already uses ip(8) for route management.
//
// The device name MUST be part of the query: without it `ip route show
// default` answers "does this box have a default route", which marked every
// up interface as carrying one.
func linuxHasDefaultRoute(name string) bool {
	if name == "" {
		return false
	}
	return hasDefaultViaIP("", name) || hasDefaultViaIP("-6", name)
}

func hasDefaultViaIP(proto, dev string) bool {
	args := []string{"route", "show", "default", "dev", dev}
	if proto != "" {
		args = append([]string{proto}, args...)
	}
	out, err := runIPOutput(args...)
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) != ""
}

// runIPOutput runs ip(8) with a bounded timeout and LC_ALL=C so the output
// is parseable regardless of the user's locale. The helper already shells
// out to ip(8) for route management, so this adds no new dependency.
func runIPOutput(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ip", args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
