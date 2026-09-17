//go:build !windows

package tunnel

import (
	"context"
	"log/slog"
	"net"
	"sync/atomic"
	"time"
)

// startSocketBindMonitor mirrors the Windows pinned-egress monitor for the
// platforms whose egress binding is route-level rather than socket-level:
//
//   - Linux pins the peer-endpoint routes via `dev <bindIface>` in the main
//     table (network.LinuxManager.addEndpointBindRoutes).
//   - macOS scopes them with `route add ... -ifscope <bindIface>`
//     (DarwinManager.addBypassForIP).
//
// On BOTH platforms a user-explicitly-pinned egress MUST NOT silently fail
// over to another NIC when the pinned one disappears. The kernel keeps the
// dead route (no auto-failover) and the connect path already fails closed
// when the pinned NIC is missing at connect time — but nothing told the GUI
// that the tunnel is now stuck. This monitor fills that gap so Linux/macOS
// behave identically to Windows: when the pinned interface is lost we
// surface EventEgressInterfaceLost and the tunnel STAYS pinned.
//
// Implementation note: we POLL the interface's administrative/link state
// every egressPollInterval rather than subscribing to kernel events. A
// push design (netlink on Linux, the existing rmMgr route monitor on macOS)
// would be lower-latency but is platform-specific and would duplicate the
// subscription machinery each OS already has for route changes. Polling is a
// single portable path with no extra syscalls or subprocesses and is plenty
// fast for a human-visible "NIC unplugged" event (the same ~1-2 s budget
// Windows' debounce delivers).
//
// Lifecycle: ctx is the per-tunnel socketBindCancel context created in
// Manager.Connect and cancelled on disconnect, so the goroutine drains
// cleanly with the tunnel.
func startSocketBindMonitor(ctx context.Context, _ any, _ string, _ uint64, pinnedIfName string, tunnelName string, onLost EgressLostHook) {
	if pinnedIfName == "" {
		// Auto-select (no pinned egress): nothing to watch.
		return
	}
	mon := &egressLostMonitor{
		ifName:     pinnedIfName,
		tunnelName: tunnelName,
		onLost:     onLost,
	}
	slog.Info("egress lost monitor started", "tunnel", tunnelName, "pinned_if", pinnedIfName)
	go mon.run(ctx)
}

// egressPollInterval is the cadence at which we re-check the pinned NIC.
// Matches the order of magnitude of Windows' socket-bind debounce so the
// user-visible "tunnel stuck after NIC drop" gap is comparable across OSes.
const egressPollInterval = 2 * time.Second

// egressLostMonitor watches one pinned physical interface and reports a
// single loss episode (latched) to the GUI until the NIC returns.
type egressLostMonitor struct {
	ifName     string
	tunnelName string
	onLost     EgressLostHook
	// lostReported latches the current loss episode so a flapping NIC does
	// not spam the GUI with repeated dialogs. Cleared when the NIC comes
	// back, re-arming future reports.
	lostReported atomic.Bool
	stopped      atomic.Bool
}

// run drives the poll loop until ctx is cancelled (tunnel disconnect).
func (m *egressLostMonitor) run(ctx context.Context) {
	// Report immediately if the NIC is already gone at connect time.
	m.check()
	ticker := time.NewTicker(egressPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			m.stopped.Store(true)
			return
		case <-ticker.C:
			m.check()
		}
	}
}

// check evaluates the pinned interface and, if it is missing or down,
// reports exactly one loss episode per outage. reason ∈ {"missing","down"}.
func (m *egressLostMonitor) check() {
	if m.stopped.Load() {
		return
	}
	ifc, err := net.InterfaceByName(m.ifName)
	up := err == nil && (ifc.Flags&net.FlagUp) != 0
	if up {
		// NIC is healthy again — clear the latch so a later loss reports.
		m.lostReported.Store(false)
		return
	}
	if m.onLost == nil {
		return
	}
	if m.lostReported.Swap(true) {
		return // already reported this outage
	}
	reason := "down"
	idx := 0
	if err != nil {
		reason = "missing"
	} else if ifc != nil {
		idx = ifc.Index
	}
	slog.Warn("pinned egress interface lost — notifying GUI",
		"tunnel", m.tunnelName, "pinned_if", m.ifName, "if_index", idx, "reason", reason)
	hook, tunnelName, ifName := m.onLost, m.tunnelName, m.ifName
	go hook(tunnelName, ifName, idx, reason)
}
