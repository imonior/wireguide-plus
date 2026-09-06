//go:build darwin

package app

import (
	"context"
	"net"
	"os/exec"
	"strings"
	"time"
)

// darwinCmdTimeout bounds the networksetup / route probes so a wedged
// system daemon can never stall the GUI's binding dropdown.
const darwinCmdTimeout = 3 * time.Second

// runDarwin runs a helper with LC_ALL=C (stable parsing) and a hard timeout.
func runDarwin(path string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), darwinCmdTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, args...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// listPhysicalInterfaces enumerates interfaces for the egress dropdown on
// macOS.
//
// `networksetup -listallhardwareports` is the only userland source for the
// GENERIC port name ("Wi-Fi", "Ethernet", "USB 10/100/1000 LAN") — the same
// strings System Settings shows. Go's net.Interfaces() only yields the
// kernel device name (en0/en1), which is meaningless to most users, and the
// IOKit description is the vendor model (like the Realtek strings on
// Windows).
//
// networksetup needs no privileges and lives at /usr/sbin/networksetup on
// every supported macOS, so a failure is treated as "fall back to the raw
// device list" rather than a hard error.
func listPhysicalInterfaces() ([]PhysicalInterface, error) {
	def := darwinDefaultInterfaces()
	ports := darwinHardwarePorts()
	if len(ports) == 0 {
		return listViaNetInterfacesDarwin(def)
	}
	out := make([]PhysicalInterface, 0, len(ports))
	for _, p := range ports {
		if skipDarwinDevice(p.device) {
			continue
		}
		ifc, err := net.InterfaceByName(p.device)
		idx := -1
		isUp := false
		if err == nil {
			idx = ifc.Index
			isUp = ifc.Flags&net.FlagUp != 0
		}
		friendly := p.port
		if friendly == "" {
			friendly = p.device
		}
		out = append(out, PhysicalInterface{
			Index:      idx,
			Name:       p.device,
			Friendly:   friendly,
			Hardware:   p.device,
			IsPhysical: true,
			IsUp:       isUp,
			HasDefault: def[p.device],
		})
	}
	return out, nil
}

type hwPort struct {
	port   string // generic name ("Wi-Fi")
	device string // kernel device ("en0")
}

// darwinHardwarePorts parses `networksetup -listallhardwareports`. The output
// is a repeating pair of "Hardware Port: X" / "Device: Y" blocks separated by
// blank lines; a trailing "VLAN Configurations" section uses a different
// format and is naturally skipped because it has neither prefix.
func darwinHardwarePorts() []hwPort {
	out, err := runDarwin("/usr/sbin/networksetup", "-listallhardwareports")
	if err != nil {
		return nil
	}
	var ports []hwPort
	var cur hwPort
	flush := func() {
		if cur.device != "" {
			ports = append(ports, cur)
		}
		cur = hwPort{}
	}
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			flush()
			continue
		}
		if v, ok := strings.CutPrefix(line, "Hardware Port:"); ok {
			flush()
			cur.port = strings.TrimSpace(v)
			continue
		}
		if v, ok := strings.CutPrefix(line, "Device:"); ok {
			cur.device = strings.TrimSpace(v)
			continue
		}
	}
	flush()
	return ports
}

// skipDarwinDevice drops the virtual interfaces macOS always advertises as
// hardware ports but which can never carry WireGuard traffic.
func skipDarwinDevice(dev string) bool {
	switch {
	case dev == "":
		return true
	case strings.HasPrefix(dev, "awdl"),
		strings.HasPrefix(dev, "llw"),
		strings.HasPrefix(dev, "utun"),
		strings.HasPrefix(dev, "lo"),
		strings.HasPrefix(dev, "gif"),
		strings.HasPrefix(dev, "stf"),
		strings.HasPrefix(dev, "p2p"),
		strings.HasPrefix(dev, "ipsec"),
		strings.HasPrefix(dev, "anpi"):
		return true
	}
	return false
}

// darwinDefaultInterfaces returns the set of device names carrying the
// current v4 and/or v6 default route.
func darwinDefaultInterfaces() map[string]bool {
	set := make(map[string]bool)
	for _, fam := range []string{"-inet", "-inet6"} {
		out, err := exec.Command("/sbin/route", "-n", "get", "default", fam).Output()
		if err != nil {
			continue
		}
		for _, raw := range strings.Split(string(out), "\n") {
			line := strings.TrimSpace(raw)
			if v, ok := strings.CutPrefix(line, "interface:"); ok {
				set[strings.TrimSpace(v)] = true
			}
		}
	}
	return set
}

// listViaNetInterfacesDarwin is the networksetup-unavailable fallback: raw
// kernel device names, virtual interfaces filtered out.
func listViaNetInterfacesDarwin(def map[string]bool) ([]PhysicalInterface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	out := make([]PhysicalInterface, 0, len(ifaces))
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagLoopback != 0 || skipDarwinDevice(ifc.Name) {
			continue
		}
		out = append(out, PhysicalInterface{
			Index:      ifc.Index,
			Name:       ifc.Name,
			Friendly:   ifc.Name,
			Hardware:   ifc.Name,
			IsPhysical: true,
			IsUp:       ifc.Flags&net.FlagUp != 0,
			HasDefault: def[ifc.Name],
		})
	}
	return out, nil
}
