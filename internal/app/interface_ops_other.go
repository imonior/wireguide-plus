//go:build !windows && !linux && !darwin

package app

import "errors"

// listPhysicalInterfaces is unsupported on platforms with no egress-binding
// implementation. Windows (IP_UNICAST_IF socket pinning), Linux
// (`ip route ... dev <iface>`) and macOS (`route ... -ifscope <iface>`) all
// have real implementations in their per-platform files, so this stub only
// catches anything else.
func listPhysicalInterfaces() ([]PhysicalInterface, error) {
	return nil, errors.New("interface binding is not supported on this platform")
}
