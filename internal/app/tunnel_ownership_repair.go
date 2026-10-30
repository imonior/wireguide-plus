package app

import (
	"errors"
	"log/slog"
	"os"
	"sync/atomic"

	"github.com/imonior/wireguide-plus/internal/ipc"
)

// tunnelOwnershipRepairInFlight dedups concurrent repair attempts. Loading a
// tunnel that a root-owned file blocks fails on every list refresh, so an
// unguarded trigger would spam the helper with an RPC per refresh until the
// repair lands; the flag lets exactly one attempt run at a time, and a failed
// attempt (helper unavailable) is naturally retried by the next refresh.
//
// tunnelOwnershipRepairInFlight 对并发的修复请求去重。被 root 属主文件挡住的
// 隧道在每次列表刷新时都会加载失败，不去重的话每次刷新都会向 helper 发一个
// RPC 直到修复落地；该标志保证同一时间只有一个请求在跑，失败的请求（helper
// 不可用）会被下一次刷新自然重试。
var tunnelOwnershipRepairInFlight atomic.Bool

// isPermissionLoadError reports whether err (or anything it wraps) is a
// filesystem permission failure — the signature of a tunnel file left owned
// by root by the privileged helper.
//
// isPermissionLoadError 判断 err（或其包装链）是否为文件系统权限错误——这是
// 特权 helper 留下 root 属主隧道文件的典型特征。
func isPermissionLoadError(err error) bool {
	return errors.Is(err, os.ErrPermission)
}

// requestTunnelOwnershipRepair asks the privileged helper (running as root)
// to chown the user's tunnels directory back to the desktop user. Fire-and-
// forget: the current list keeps showing the load error, and the next refresh
// after the repair succeeds renders the tunnel normally again.
//
// requestTunnelOwnershipRepair 请求特权 helper（以 root 运行）把用户的隧道
// 目录 chown 回桌面用户。发后即忘：当前列表仍显示加载错误，修复成功后的
// 下一次刷新即可正常渲染该隧道。
func (s *TunnelService) requestTunnelOwnershipRepair() {
	if !tunnelOwnershipRepairInFlight.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer tunnelOwnershipRepairInFlight.Store(false)
		if err := s.call(ipc.MethodRepairTunnelOwnership, ipc.Empty{}, nil); err != nil {
			slog.Warn("tunnel ownership repair failed", "error", err)
			return
		}
		slog.Info("tunnel ownership repaired by helper")
	}()
}
