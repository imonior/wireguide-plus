package helper

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"

	"github.com/imonior/wireguide-plus/internal/ipc"
)

// Address-conflict auto-connect pause.
//
// When the helper learns that a tunnel's Address is held by a DIFFERENT
// adapter — detected at startup or, more commonly, when a connect attempt
// dies at the assign-address step with a network.AddressConflictError — the
// tunnel's automation is PAUSED until the user decides what to do:
// "stop automation for this tunnel" or "keep trying". Retrying on a 5-minute
// backoff forever is not a decision, it is the loop the user sees as a fake
// connection storm, so the engine stops touching the tunnel and the GUI
// keeps an interactive dialog up instead.
//
// The set is runtime state, keyed by tunnel name and holding the payload so
// a GUI that (re)starts later can pull the still-unresolved conflicts and
// re-show the dialog. A helper restart re-derives the set from the startup
// check; nothing survives to be confused with a stale pause.

// pauseForAddressConflict marks the tunnel's automation as paused on an
// address conflict and broadcasts EventAddressConflict so the GUI raises its
// dialog. Broadcasting is deduplicated by the pause transition itself: a
// tunnel already awaiting a decision is not re-announced. Returns true when
// this call produced a new pause.
func (h *Helper) pauseForAddressConflict(tunnel, addressCIDR, holder, state string) bool {
	h.addrConflictMu.Lock()
	if h.conflictPaused == nil {
		h.conflictPaused = make(map[string]ipc.AddressConflictPayload)
	}
	if _, already := h.conflictPaused[tunnel]; already {
		h.addrConflictMu.Unlock()
		return false
	}
	payload := ipc.AddressConflictPayload{
		Tunnel:   tunnel,
		Address:  stripCIDR(addressCIDR),
		Adapter:  holder,
		Software: inferConflictingSoftware(holder),
		State:    state,
	}
	h.conflictPaused[tunnel] = payload
	h.addrConflictMu.Unlock()

	slog.Warn("tunnel address is held by another adapter — auto-connect paused until the user decides",
		"category", "network",
		"tunnel", tunnel,
		"address", payload.Address,
		"conflicting_adapter", holder,
		"conflicting_software", payload.Software,
		"state", state)

	// Stop any in-flight monitor retry for this tunnel so the pause holds:
	// without this the dead-connection monitor would keep re-driving the
	// attempt that just failed. (The monitor may not exist yet during the
	// startup check goroutine; nil-guard covers that window.)
	if h.monitor != nil {
		h.monitor.CancelRetryFor(tunnel)
	}
	if h.server != nil {
		h.server.Broadcast(ipc.EventAddressConflict, payload)
	}
	return true
}

// isAutoConnectPaused reports whether the tunnel awaits a user decision on
// an address conflict. Paused tunnels are left alone by the evaluation
// loop, the reconnect monitor, and every retry path.
func (h *Helper) isAutoConnectPaused(tunnel string) bool {
	h.addrConflictMu.Lock()
	defer h.addrConflictMu.Unlock()
	_, ok := h.conflictPaused[tunnel]
	return ok
}

// resumeAutoConnect lifts the pause (the dialog's "keep trying" answer, or
// a manual connect that actually got through). Returns true when a pause
// was cleared.
func (h *Helper) resumeAutoConnect(tunnel string) bool {
	h.addrConflictMu.Lock()
	wasPaused := false
	if h.conflictPaused != nil {
		_, wasPaused = h.conflictPaused[tunnel]
		delete(h.conflictPaused, tunnel)
	}
	h.addrConflictMu.Unlock()
	if wasPaused {
		slog.Info("auto-connect resumed after address conflict decision",
			"category", "network", "tunnel", tunnel)
		h.clearAutoConnectBackoff(tunnel)
	}
	return wasPaused
}

// pendingAddressConflicts snapshots the conflicts still awaiting a user
// decision — the pull counterpart of EventAddressConflict, so a GUI that
// starts after the broadcast doesn't miss an unresolved pause. Tunnels whose
// automation has since been switched off are excluded: the dialog's other
// button (or the automation switch) already answered the question, and the
// engine won't touch them anyway. Their pause entries drop out when the user
// re-enables automation (the app-side SetTunnelPolicies hook).
func (h *Helper) pendingAddressConflicts() []ipc.AddressConflictPayload {
	h.addrConflictMu.Lock()
	all := make([]ipc.AddressConflictPayload, 0, len(h.conflictPaused))
	for _, p := range h.conflictPaused {
		all = append(all, p)
	}
	h.addrConflictMu.Unlock()

	out := make([]ipc.AddressConflictPayload, 0, len(all))
	for _, p := range all {
		if h.automationDisabled(p.Tunnel) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// automationBlockedForReconnect is the single "the engine must leave this
// tunnel alone" question asked by the automation loop and fed to the
// reconnect monitor: either the durable sidecar exemption or a pending
// address-conflict pause.
func (h *Helper) automationBlockedForReconnect(tunnel string) bool {
	return h.automationDisabled(tunnel) || h.isAutoConnectPaused(tunnel)
}

// stripCIDR renders the bare address for the dialog, falling back to the
// raw string when it does not parse (never worse than the old text).
func stripCIDR(addr string) string {
	if ip, _, err := net.ParseCIDR(addr); err == nil {
		return ip.String()
	}
	return addr
}

// handleResumeAutoConnect is the conflict dialog's "keep trying" answer:
// lift the pause, clear any backoff, and post an evaluation so the retry
// happens now rather than on the next poll tick. If the other adapter still
// holds the address the attempt fails, re-pauses, and the dialog comes back
// — which is the "stay up until the user actually resolves it" semantics.
func (h *Helper) handleResumeAutoConnect(params json.RawMessage) (interface{}, error) {
	var req ipc.ResumeAutoConnectRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	if req.Tunnel == "" {
		return nil, fmt.Errorf("tunnel is required")
	}
	h.resumeAutoConnect(req.Tunnel)
	h.requestAutomationEval("conflict-resume")
	return ipc.Empty{}, nil
}

// handlePendingAddressConflicts serves the GUI's startup pull of conflicts
// whose tunnels are still paused awaiting a decision.
func (h *Helper) handlePendingAddressConflicts(_ json.RawMessage) (interface{}, error) {
	return ipc.AddressConflictsResponse{Conflicts: h.pendingAddressConflicts()}, nil
}
