//go:build windows

package tunnel

// TunnelLUID returns the wintun adapter's LUID as a uint64.
//
// Both protocol backends expose a *NativeTun with a LUID() uint64 method
// (wireguard-go and amneziawg-go return uint64), so a small interface
// assertion works for whichever backend is in use — no need to import a
// concrete tun package here.
//
// TunnelLUID 以 uint64 返回 wintun 网卡的 LUID。
// 两种协议后端的 *NativeTun 都带 LUID() uint64 方法
// （wireguard-go 与 amneziawg-go 均返回 uint64），
// 因此用一个小接口断言即可适配当前后端，无需在此引入具体的 tun 包。
func (e *Engine) TunnelLUID() uint64 {
	if nt, ok := e.tunDevice.(interface{ LUID() uint64 }); ok {
		return nt.LUID()
	}
	return 0
}
