//go:build windows
package tunnel

import (
	"golang.zx2c4.com/wireguard/tun"
)

// TunnelLUID 返回wintun适配器LUID，返回uint64
func (e *Engine) TunnelLUID() uint64 {
	nt, ok := e.tunDevice.(*tun.NativeTun)
	if !ok {
		return 0
	}
	return uint64(nt.LUID())
}
