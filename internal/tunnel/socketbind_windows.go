//go:build windows
package tunnel

// Socket-level loop protection — IP_UNICAST_IF binding for the WireGuard
// UDP sockets, mirroring what the official wireguard-windows client does
// in tunnel/defaultroutemonitor.go (v0.1.1).
//
// Why this exists ALONGSIDE the WFP block + iphlpapi bypass routes the
// rest of this branch added:
//
//   - The /32 bypass + WFP BLOCK protect against the loop AT THE ROUTING
//     TABLE / FILTERING LAYER. Both depend on the kernel making a routing
//     decision and either (a) finding the /32 first or (b) hitting our
//     filter when it doesn't. Both have correctness invariants we have to
//     defend continuously (route ordering on install, filter ordering on
//     network state changes, etc.).
//
//   - IP_UNICAST_IF tells the WG UDP socket "regardless of the route
//     table, send through THIS specific interface index." It moves the
//     decision OUT of the route table entirely — the kernel skips its
//     routing lookup for traffic on the bound socket. The wintun adapter
//     can be the longest‑prefix match for the peer endpoint and our WG
//     send still goes out the physical NIC. That's the same trick the
//     official client uses as its PRIMARY anti‑loop measure.
//
// We keep all three defenses because they fail differently:
//
//   bind miss → routing decision picks wintun → /32 bypass catches it
//   /32 bypass missing or stale → WFP BLOCK at OUTBOUND_TRANSPORT drops it
//   WFP layer disabled by a third‑party security driver → watchdog trips
//
// Three layers in series. The official client gets away with one mostly
// because Donenfeld owns the BFE filter weight space; we're an uncertified
// app, so belt‑and‑suspenders is the right call.
//
// Change detection — NotifyRouteChange2 + NotifyIpInterfaceChange push
// notifications, ported from wireguard‑windows v0.1.1
// tunnel/defaultroutemonitor.go + tunnel/winipcfg/route_change_handler.go.
// Latency: ~150 ms from kernel route change to re‑pin, vs the ~5 s a
// polling design would deliver. Worth porting because the difference is
// the user‑visible "tunnel stuck for 5 s after Wi‑Fi → Ethernet handoff"
// gap.

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/imonior/wireguide-plus/internal/network"
	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

// debounce timing — ported verbatim from wireguard‑windows
// tunnel/defaultroutemonitor.go. 150 ms coalesces the typical burst of
// route/interface events Windows fires during a network handoff; the
// 2 s burst‑escape forces a re‑evaluation even if the events never
// stop arriving (e.g., a third‑party VPN spamming routes).
const (
	socketBindDebounce    = 150 * time.Millisecond
	socketBindBurstEscape = 2 * time.Second
	afInet                = 2
	afInet6               = 23

	// IfType from ipifcons.h
	ifTypePPP        = 23
	ifTypeL2TP       = 24   // ❌错误！L2TP标准IfType=24，你写成31
	ifTypeTunnel     = 31   // ❌错误！TUNNEL标准IfType=31，你写成131
	ifTypeSoftSwitch = 133
)

type candidateIf struct {
	ifIndex uint32
	metric  uint32
	virtual bool
	name    string
}

// findFallbackInterface scan all interfaces, pick usable underlay, select minimal metric.
// Priority: UP physical nic(with default route) min‑metric > UP virtual nic min‑metric > 0(blackhole).
// Skip loopback, skip tunnel self‑interface.
func findFallbackInterface(tunnelInterfaceName string, ipv6 bool) uint32 {
	ifaces, err := net.Interfaces()
	if err != nil {
		slog.Warn("findFallbackInterface failed to list interfaces", "error", err)
		return 0
	}
	var physicalCandidates []candidateIf
	var virtualCandidates []candidateIf
	var familyCode uint16
	if ipv6 {
		familyCode = afInet6
	} else {
		familyCode = afInet
	}
	for _, iface := range ifaces {
		// Must be UP, not loopback, not our wintun tunnel
		if (iface.Flags&net.FlagUp) == 0 ||
			(iface.Flags&net.FlagLoopback) != 0 ||
			iface.Name == tunnelInterfaceName {
			continue
		}


		addrs, errAddrs := iface.Addrs()
		if errAddrs != nil || len(addrs) == 0 {
			continue
		}
		// check interface has correct‑family non‑link‑local ip
		hasValidIP := false
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if ipnet.IP.IsLoopback() || ipnet.IP.IsLinkLocalUnicast() {
				continue
			}
			if (!ipv6 && ipnet.IP.To4() != nil) || (ipv6 && ipnet.IP.To4() == nil && ipnet.IP.To16() != nil) {
				hasValidIP = true
				break
			}
		}
		if !hasValidIP {
			continue
		}
		// 查询**该网卡自身**是否存在默认路由以及对应metric
		hasDefRoute, nicMetric := network.HasDefaultRouteOnInterface(familyCode, uint32(iface.Index))
		if !hasDefRoute {
			slog.Debug("findFallbackInterface: skip candidate, no default route on this interface",
				"name", iface.Name, "ifIndex", iface.Index, "family", familyName(ipv6))
			continue
		}
		isVirtual := strings.HasPrefix(iface.Name, "vEthernet (")
		entry := candidateIf{
			ifIndex: uint32(iface.Index),
			metric:  nicMetric,
			virtual: isVirtual,
			name:    iface.Name,
		}
		if isVirtual {
			virtualCandidates = append(virtualCandidates, entry)
		} else {
			physicalCandidates = append(physicalCandidates, entry)
		}
	}
	// pick minimal metric from physical candidates first
	best := func(list []candidateIf) *candidateIf {
		var sel *candidateIf
		for i := range list {
			item := list[i]
			if sel == nil || item.metric < sel.metric {
				sel = &item
			}
		}
		return sel
	}
	selected := best(physicalCandidates)
	if selected != nil {
		slog.Info("findFallbackInterface selected physical interface (min metric)",
			"family", familyName(ipv6),
			"ifIndex", selected.ifIndex,
			"name", selected.name,
			"metric", selected.metric)
		return selected.ifIndex
	}
	// fallback to virtual candidates
	selected = best(virtualCandidates)
	if selected != nil {
		slog.Warn("findFallbackInterface no physical interface, select best virtual interface",
			"family", familyName(ipv6),
			"ifIndex", selected.ifIndex,
			"name", selected.name,
			"metric", selected.metric)
		return selected.ifIndex
	}
	slog.Warn("findFallbackInterface no usable interface found", "family", familyName(ipv6))
	return 0
}

// findDefaultUnderlayByLUID 复刻wireguard‑windows mtumonitor findDefaultLUID
// 使用GetIPForwardTable2读取完整路由表，规避GetBestInterfaceEx的256(ERROR_NO_SUCH_DEVICE)时序bug
// 增强：过滤第三方VPN隧道类型接口；跳过自身wintun LUID；按route.Metric+interface.Metric选最优
// 失败返回ifIndex=0，上层调用方继续使用findFallbackInterface做兜底扫描
func findDefaultUnderlayByLUID(tunnelLUID winipcfg.LUID, ipv6 bool) (winipcfg.LUID, uint32, error) {
	var family winipcfg.AddressFamily
	if ipv6 {
		family = windows.AF_INET6
	} else {
		family = windows.AF_INET
	}

	routes, err := winipcfg.GetIPForwardTable2(family)
	if err != nil {
		return 0, 0, err
	}

	var lowestMetric uint32 = ^uint32(0)
	var bestLUID winipcfg.LUID
	var bestIfIndex uint32

	for _, row := range routes {
		// 只取默认路由 0.0.0.0/0 或 ::/0
		if row.DestinationPrefix.PrefixLength != 0 {
			continue
		}
		// 跳过本程序wintun隧道
		if row.InterfaceLUID == tunnelLUID {
			continue
		}

		ifRow, err := row.InterfaceLUID.Interface()
		if err != nil {
			continue
		}
		if ifRow.OperStatus != winipcfg.IfOperStatusUp {
			continue
		}


		// 过滤第三方VPN隧道类虚拟网卡，避免UDP socket绑定到其他VPN
		switch ifRow.Type {
		// IF_TYPE_PPP=23, IF_TYPE_L2TP=24, IF_TYPE_TUNNEL=31, IF_TYPE_SOFTWARE_LOOPBACK=24
		case winipcfg.IfType(ifTypePPP), winipcfg.IfType(ifTypeL2TP), winipcfg.IfType(ifTypeTunnel):
			slog.Info("skip third‑party vpn/tunnel interface", "ifType", ifRow.Type, "idx", ifRow.InterfaceIndex)
			continue
		}


		ifaceCfg, err := row.InterfaceLUID.IPInterface(family)
		if err != nil {
			continue
		}
		totalMetric := row.Metric + ifaceCfg.Metric
		if totalMetric < lowestMetric {
			lowestMetric = totalMetric
			bestLUID = row.InterfaceLUID
			bestIfIndex = ifRow.InterfaceIndex
		}
	}

	if bestLUID == 0 {
		return 0, 0, errors.New("no valid underlay default route from ip forward table")
	}
	return bestLUID, bestIfIndex, nil
}

// pinSocketToPhysical does one pass of (find best non‑tunnel default,
// call BindSocketToInterface). Used at connect time after monitor started,
// and as the implementation of every monitor re‑evaluation.
// Returns the (v4, v6) ifIndex pair actually bound (0 = blackhole/no
// underlay for that family).
func pinSocketToPhysical(bind conn.Bind, tunnelInterfaceName string, tunnelLUID winipcfg.LUID) (uint32, uint32) {
	binder, ok := bind.(conn.BindSocketToInterface)
	if !ok {
		return 0, 0
	}
	v4 := pinFamily(binder, tunnelInterfaceName, tunnelLUID, false)
	v6 := pinFamily(binder, tunnelInterfaceName, tunnelLUID, true)
	return v4, v6
}

// pinFamily resolves one address family's best non‑tunnel default route
// and binds the corresponding socket. Returns the bound ifIndex (0 if
// no usable underlay was found and blackhole was applied).
func pinFamily(binder conn.BindSocketToInterface, tunnelInterfaceName string, tunnelLUID winipcfg.LUID, ipv6 bool) uint32 {
	var ifIndex uint32
	_, ifIndex, err := findDefaultUnderlayByLUID(tunnelLUID, ipv6)
	if err != nil {
		slog.Warn("findDefaultUnderlayByLUID failed, falling back to interface scan",
			"family", familyName(ipv6), "error", err)
		ifIndex = findFallbackInterface(tunnelInterfaceName, ipv6)
	}

	// 如果拿到的默认路由网卡就是隧道自身，直接走fallback
	if ifIndex > 0 {
		iface, errIface := net.InterfaceByIndex(int(ifIndex))
		if errIface == nil && iface.Name == tunnelInterfaceName {
			slog.Warn("pinFamily: default‑route points to tunnel itself, trigger fallback scan",
				"family", familyName(ipv6), "tunnel_ifIndex", ifIndex)
			ifIndex = findFallbackInterface(tunnelInterfaceName, ipv6)
		}
	}

	// validate: route‑returned interface must be UP
	if ifIndex > 0 {
		iface, errIface := net.InterfaceByIndex(int(ifIndex))
		if errIface != nil || (iface.Flags&net.FlagUp) == 0 {
			slog.Warn("pinFamily: default‑route interface is down/invalid, trigger fallback scan",
				"family", familyName(ipv6), "invalid_ifIndex", ifIndex)
			ifIndex = findFallbackInterface(tunnelInterfaceName, ipv6)
		}
	}

	blackhole := ifIndex == 0
	var errBind error
	if ipv6 {
		errBind = binder.BindSocketToInterface6(ifIndex, blackhole)
	} else {
		errBind = binder.BindSocketToInterface4(ifIndex, blackhole)
	}
	if errBind != nil {
		slog.Warn("socket pin failed",
			"family", familyName(ipv6),
			"ifIndex", ifIndex,
			"blackhole", blackhole,
			"error", errBind)
		return 0
	}
	slog.Info("WG socket pinned to underlay",
		"family", familyName(ipv6),
		"ifIndex", ifIndex,
		"blackhole", blackhole,
		"tunnel_excluded", tunnelInterfaceName)
	return ifIndex
}

func familyName(ipv6 bool) string {
	if ipv6 {
		return "v6"
	}
	return "v4"
}

// startSocketBindMonitor wires NotifyRouteChange2 + NotifyIpInterfaceChange
// kernel callbacks. Any route or interface‑parameter change pumps the
// debounce timer; the timer fires re‑evaluation in pinSocketToPhysical's
// idempotent path (which is a no‑op when the best underlay hasn't moved).
//
// Timing: register kernel callbacks FIRST, then perform initial pin, align official wireguard‑windows.
//
// Cleanup: ctx cancellation unregisters both callbacks and stops the
// debounce timer. The callbacks themselves are guarded by sync.WaitGroup
// so concurrent goroutines spawned by the kernel callback drain before
// the manager calls engine.Close.
func startSocketBindMonitor(ctx context.Context, bind conn.Bind, tunnelInterfaceName string, tunnelLUID uint64) {

	realTunnelLUID := winipcfg.LUID(tunnelLUID)

	// 在这里插入一行日志
	slog.Info("socket bind monitor started", "tunnelLUID", realTunnelLUID)

	if bind == nil {
		return
	}
	binder, ok := bind.(conn.BindSocketToInterface)
	if !ok {
		return
	}
	mon := &socketBindMonitor{
		binder:              binder,
		tunnelInterfaceName: tunnelInterfaceName,
		tunnelLUID:          realTunnelLUID,
	}
	mon.lastV4.Store(0)
	mon.lastV6.Store(0)
	// Burst‑debounce timer. Reset on every bump; fires reevaluate() once
	// the burst quiets down for socketBindDebounce. Initial reset to a
	// very long duration so it doesn't fire before the first bump.
	mon.burstTimer = time.AfterFunc(time.Hour*200, mon.reevaluate)
	mon.burstTimer.Stop()
	// Register kernel callbacks FIRST
	if err := mon.registerCallbacks(); err != nil {
		slog.Warn("WG socket bind monitor: callback registration failed; reverting to no monitor",
			"error", err)
		return
	}
	// After callback registered: do initial pin
	initialV4, initialV6 := pinSocketToPhysical(bind, tunnelInterfaceName, realTunnelLUID)
	mon.lastV4.Store(initialV4)
	mon.lastV6.Store(initialV6)
	// startup grace poll: short‑time fallback for wintun‑up kernel route table lag
	go func() {
		const gracePeriod = 1200 * time.Millisecond
		start := time.Now()
		for time.Since(start) < gracePeriod {
			if mon.stopped.Load() {
				return
			}
			v4, v6 := pinSocketToPhysical(bind, tunnelInterfaceName, realTunnelLUID)
			if v4 != 0 && v6 != 0 {
				slog.Info("startup grace poll: got valid underlay index", "v4", v4, "v6", v6)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		slog.Warn("startup grace poll timed‑out; waiting for kernel route events")
	}()
	// Tie lifecycle to ctx. When ctx is cancelled (DisconnectTunnel),
	// unregister and stop the timer.
	go func() {
		<-ctx.Done()
		mon.stop()
		slog.Info("WG socket bind monitor stopped", "tunnel", tunnelInterfaceName)
	}()
}

// socketBindMonitor holds the runtime state for one tunnel's bind monitor.
// Lifecycle: created in startSocketBindMonitor, torn down via stop()
// when ctx is cancelled.
type socketBindMonitor struct {
	binder              conn.BindSocketToInterface
	tunnelInterfaceName string
	tunnelLUID          winipcfg.LUID
	lastV4              atomic.Uint32
	lastV6              atomic.Uint32
	burstMu             sync.Mutex
	burstTimer          *time.Timer
	firstBurst          time.Time // first event of the current burst; zero between bursts
	cbMu                sync.Mutex
	routeHnd            windows.Handle
	ifaceHnd            windows.Handle
	pending             sync.WaitGroup // tracks in‑flight callback goroutines
	stopped             atomic.Bool
}

// bump is called from kernel notification callbacks. Resets the
// debounce timer; if the current burst has lasted > burstEscape,
// force‑fires reevaluate immediately so a continuously‑noisy network
// doesn't permanently postpone the re‑pin.
func (mon *socketBindMonitor) bump() {
	mon.burstMu.Lock()
	defer mon.burstMu.Unlock()
	mon.burstTimer.Reset(socketBindDebounce)
	if mon.firstBurst.IsZero() {
		mon.firstBurst = time.Now()
		return
	}
	if time.Since(mon.firstBurst) > socketBindBurstEscape {
		mon.firstBurst = time.Time{}
		mon.burstTimer.Stop()
		go mon.reevaluate()
	}
}

// reevaluate fires after the debounce timer has gone quiet, or
// (force‑path) from bump() when a burst has exceeded its escape budget.
func (mon *socketBindMonitor) reevaluate() {
	if mon.stopped.Load() {
		return
	}
	mon.burstMu.Lock()
	mon.firstBurst = time.Time{}
	mon.burstMu.Unlock()
	newV4 := evaluateOneFamily(mon.binder, mon.tunnelInterfaceName, mon.tunnelLUID, false, mon.lastV4.Load())
	mon.lastV4.Store(newV4)
	newV6 := evaluateOneFamily(mon.binder, mon.tunnelInterfaceName, mon.tunnelLUID, true, mon.lastV6.Load())
	mon.lastV6.Store(newV6)
}

// evaluateOneFamily resolves the best non‑tunnel for one address family and (re‑)binds.
func evaluateOneFamily(binder conn.BindSocketToInterface, tunnelInterfaceName string, tunnelLUID winipcfg.LUID, ipv6 bool, previous uint32) uint32 {
	var ifIndex uint32
	_, ifIndex, err := findDefaultUnderlayByLUID(tunnelLUID, ipv6)
	if err != nil {
		slog.Warn("evaluateOneFamily: findDefaultUnderlayByLUID failed, fallback to interface scan",
			"family", familyName(ipv6), "error", err)
		ifIndex = findFallbackInterface(tunnelInterfaceName, ipv6)
	}

	// 如果拿到的默认路由网卡就是隧道自身，直接走fallback
	if ifIndex > 0 {
		iface, errIface := net.InterfaceByIndex(int(ifIndex))
		if errIface == nil && iface.Name == tunnelInterfaceName {
			slog.Warn("evaluateOneFamily: default‑route points to tunnel itself, trigger fallback scan",
				"family", familyName(ipv6), "tunnel_ifIndex", ifIndex)
			ifIndex = findFallbackInterface(tunnelInterfaceName, ipv6)
		}
	}

	// validate route‑returned interface status on runtime re‑evaluation
	if ifIndex > 0 {
		iface, errIface := net.InterfaceByIndex(int(ifIndex))
		if errIface != nil || (iface.Flags&net.FlagUp) == 0 {
			slog.Warn("evaluateOneFamily: default‑route interface down/invalid, trigger fallback scan",
				"family", familyName(ipv6), "invalid_ifIndex", ifIndex)
			ifIndex = findFallbackInterface(tunnelInterfaceName, ipv6)
		}
	}

	if ifIndex == previous {
		return previous
	}

	blackhole := ifIndex == 0
	var errBind error
	if ipv6 {
		errBind = binder.BindSocketToInterface6(ifIndex, blackhole)
	} else {
		errBind = binder.BindSocketToInterface4(ifIndex, blackhole)
	}
	if errBind != nil {
		slog.Warn("socket re‑pin failed",
			"family", familyName(ipv6),
			"new_ifIndex", ifIndex,
			"previous_ifIndex", previous,
			"blackhole", blackhole,
			"error", errBind)
		return previous
	}
	slog.Info("WG socket re‑pinned (underlay changed)",
		"family", familyName(ipv6),
		"previous_ifIndex", previous,
		"new_ifIndex", ifIndex,
		"blackhole", blackhole)
	return ifIndex
}

// --- Kernel callback wiring ----------------------------------------
var (
	procNotifyRouteChange2      = modIphlpapiSocketbind.NewProc("NotifyRouteChange2")
	procNotifyIpInterfaceChange = modIphlpapiSocketbind.NewProc("NotifyIpInterfaceChange")
	procCancelMibChangeNotify2  = modIphlpapiSocketbind.NewProc("CancelMibChangeNotify2")
	modIphlpapiSocketbind       = windows.NewLazySystemDLL("iphlpapi.dll")
)

func (mon *socketBindMonitor) registerCallbacks() error {
	routeCB := windows.NewCallback(func(callerContext uintptr, row uintptr, notificationType uint32) uintptr {
		if mon.stopped.Load() {
			return 0
		}
		mon.pending.Add(1)
		go func() {
			defer mon.pending.Done()
			mon.bump()
		}()
		return 0
	})
	ifaceCB := windows.NewCallback(func(callerContext uintptr, row uintptr, notificationType uint32) uintptr {
		const mibParameterNotification uint32 = 0
		if notificationType != mibParameterNotification {
			return 0
		}
		if mon.stopped.Load() {
			return 0
		}
		mon.pending.Add(1)
		go func() {
			defer mon.pending.Done()
			mon.bump()
		}()
		return 0
	})
	mon.cbMu.Lock()
	defer mon.cbMu.Unlock()
	var rh windows.Handle
	if err := notifyRouteChange2(windows.AF_UNSPEC, routeCB, 0, false, &rh); err != nil {
		return err
	}
	mon.routeHnd = rh
	var ih windows.Handle
	if err := notifyIpInterfaceChange(windows.AF_UNSPEC, ifaceCB, 0, false, &ih); err != nil {
		_ = cancelMibChangeNotify2(mon.routeHnd)
		mon.routeHnd = 0
		return err
	}
	mon.ifaceHnd = ih
	return nil
}

func (mon *socketBindMonitor) stop() {
	if !mon.stopped.CompareAndSwap(false, true) {
		return
	}
	mon.cbMu.Lock()
	rh, ih := mon.routeHnd, mon.ifaceHnd
	mon.routeHnd, mon.ifaceHnd = 0, 0
	mon.cbMu.Unlock()
	if rh != 0 {
		_ = cancelMibChangeNotify2(rh)
	}
	if ih != 0 {
		_ = cancelMibChangeNotify2(ih)
	}
	mon.burstMu.Lock()
	if mon.burstTimer != nil {
		mon.burstTimer.Stop()
	}
	mon.burstMu.Unlock()
	mon.pending.Wait()
}

func notifyRouteChange2(family uint16, callback uintptr, callerContext uintptr, initialNotification bool, notificationHandle *windows.Handle) error {
	var initial uint32
	if initialNotification {
		initial = 1
	}
	r0, _, _ := syscall.SyscallN(procNotifyRouteChange2.Addr(),
		uintptr(family),
		callback,
		uintptr(callerContext),
		uintptr(initial),
		uintptr(unsafe.Pointer(notificationHandle)))
	if r0 != 0 {
		return errors.New("NotifyRouteChange2 failed: " + windows.Errno(r0).Error())
	}
	return nil
}

func notifyIpInterfaceChange(family uint16, callback uintptr, callerContext uintptr, initialNotification bool, notificationHandle *windows.Handle) error {
	var initial uint32
	if initialNotification {
		initial = 1
	}
	r0, _, _ := syscall.SyscallN(procNotifyIpInterfaceChange.Addr(),
		uintptr(family),
		callback,
		uintptr(callerContext),
		uintptr(initial),
		uintptr(unsafe.Pointer(notificationHandle)))
	if r0 != 0 {
		return errors.New("NotifyIpInterfaceChange failed: " + windows.Errno(r0).Error())
	}
	return nil
}

func cancelMibChangeNotify2(handle windows.Handle) error {
	r0, _, _ := syscall.SyscallN(procCancelMibChangeNotify2.Addr(), uintptr(handle))
	if r0 != 0 {
		return errors.New("CancelMibChangeNotify2 failed: " + windows.Errno(r0).Error())
	}
	return nil
}
