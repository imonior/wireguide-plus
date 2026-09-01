//go:build windows
package tunnel

// TunnelLUID 返回wintun适配器LUID，返回uint64。
//
// Both protocol backends expose a *NativeTun with a LUID() uint64 method
// (wireguard-go and amneziawg-go return uint64), so a small interface
// assertion works for whichever backend is in use — no need to import a
// concrete tun package here.
func (e *Engine) TunnelLUID() uint64 {
	if nt, ok := e.tunDevice.(interface{ LUID() uint64 }); ok {
		return nt.LUID()
	}
	return 0
}
