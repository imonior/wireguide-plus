package helper

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/imonior/wireguide-plus/internal/ipc"
)

// Runtime enforcement of the per-tunnel "DNS resolve path" policy
// (principle 33).
//
// The switch itself is a plain per-tunnel boolean (TunnelMeta.DNSResolvePath)
// gated by a master switch in Settings. This file owns the runtime half that
// the pure analyzer in internal/policy cannot express, because it depends on
// WHO IS CONNECTED RIGHT NOW rather than on configuration:
//
//   - several tunnels may have the switch ON at the same time — that is a
//     legal configuration and saving only warns (principle 22);
//   - but at most one CONNECTED tunnel can carry the system's DNS, because
//     the enforcement is machine-wide (drop port 53/853 everywhere except
//     through the owning tunnel);
//   - so when tunnel B asks for the path while connected tunnel A already
//     owns it, B's connect is PARKED: the helper broadcasts the conflict and
//     waits for the user's decision (turn B's switch off and connect, or
//     stop connecting). Automation never parks — with nobody to answer it
//     would hang — it skips and notifies instead.
//
// When no tunnel owns the path, DNS behaves exactly as the OS configured it.

const (
	// dnsPathWaitTimeout is how long a parked manual connect waits for the
	// user. Long enough to read a dialog and act, short enough that a
	// headless `ctl connect` fails rather than hanging forever.
	dnsPathWaitTimeout = 120 * time.Second
	// dnsPathPollInterval is how often a parked connect re-checks whether
	// the owning tunnel has gone away. It makes "the user disconnected the
	// other tunnel instead" work without a dialog round-trip.
	dnsPathPollInterval = 500 * time.Millisecond
)

// dnsPathDecision is the user's answer to a parked connect.
type dnsPathDecision string

const (
	dnsPathProceed dnsPathDecision = "proceed"
	dnsPathAbort   dnsPathDecision = "abort"
)

// dnsPathMasterEnabled reports whether the master switch is on. The feature
// does not exist when it is off: no enforcement, no conflict, no UI.
func (h *Helper) dnsPathMasterEnabled() bool {
	settings, err := h.loadUserSettings()
	if err != nil || settings == nil {
		return false
	}
	return settings.DNSResolvePath
}

// tunnelClaimsDNSPath reports whether a tunnel wants to own the system's DNS
// resolve path — master switch on, the tunnel's own switch on, and the claim
// not waived for the current connect attempt.
func (h *Helper) tunnelClaimsDNSPath(name string) bool {
	if name == "" || h.userTunnelStore == nil || !h.dnsPathMasterEnabled() {
		return false
	}
	if h.dnsPathClaimWaived(name) {
		return false
	}
	meta, err := h.userTunnelStore.LoadMeta(name)
	if err != nil || meta == nil {
		return false
	}
	return meta.DNSResolvePath
}

// dnsPathBlockers lists the CONNECTED tunnels (other than exclude) that
// currently own the system's DNS resolve path. Sorted for stable logs and a
// stable dialog.
func (h *Helper) dnsPathBlockers(exclude string) []string {
	if h.userTunnelStore == nil || !h.dnsPathMasterEnabled() {
		return nil
	}
	h.mu.Lock()
	active := make([]string, 0, len(h.activeCfgs))
	for name := range h.activeCfgs {
		active = append(active, name)
	}
	h.mu.Unlock()
	sort.Strings(active)

	var out []string
	for _, name := range active {
		if name == exclude {
			continue
		}
		meta, err := h.userTunnelStore.LoadMeta(name)
		if err != nil || meta == nil || !meta.DNSResolvePath {
			continue
		}
		out = append(out, name)
	}
	return out
}

// awaitDNSResolvePathClear parks a MANUAL connect while another connected
// tunnel owns the DNS resolve path. It returns nil as soon as the path is
// free (the owner went down, or the user waived this tunnel's claim) and an
// error when the connect must not proceed.
//
// It must be called WITHOUT connectMu held: the whole point is that the user
// can still disconnect the owning tunnel (or connect something else) while
// the dialog is up.
func (h *Helper) awaitDNSResolvePathClear(name string) error {
	if !h.tunnelClaimsDNSPath(name) {
		return nil
	}
	blockers := h.dnsPathBlockers(name)
	if len(blockers) == 0 {
		return nil
	}
	// Nobody to answer a dialog (CLI / automation): refuse instead of
	// parking the caller for two minutes.
	if !h.server.HasControlConn() {
		return fmt.Errorf("DNS resolve path is already owned by %s — disconnect it or turn off this tunnel's DNS resolve path before connecting",
			strings.Join(blockers, ", "))
	}

	wait := h.beginDNSPathWait(name)
	defer h.endDNSPathWait(name)

	h.server.Broadcast(ipc.EventDNSPathConflict, ipc.DNSPathConflictPayload{
		Tunnel:   name,
		Blockers: blockers,
	})
	slog.Warn("connect parked: another tunnel owns the DNS resolve path",
		"category", "policy", "tunnel", name, "blockers", blockers)

	deadline := time.NewTimer(dnsPathWaitTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(dnsPathPollInterval)
	defer ticker.Stop()

	for {
		select {
		case decision := <-wait:
			if decision == dnsPathProceed {
				slog.Info("DNS resolve path conflict resolved, continuing connect",
					"category", "policy", "tunnel", name)
				return nil
			}
			return fmt.Errorf("connect cancelled: %s kept the DNS resolve path", strings.Join(blockers, ", "))
		case <-ticker.C:
			if len(h.dnsPathBlockers(name)) == 0 {
				slog.Info("DNS resolve path freed, continuing connect",
					"category", "policy", "tunnel", name)
				return nil
			}
		case <-deadline.C:
			return fmt.Errorf("connect cancelled: no answer about the DNS resolve path held by %s",
				strings.Join(blockers, ", "))
		}
	}
}

// beginDNSPathWait registers the parked connect's decision channel.
func (h *Helper) beginDNSPathWait(name string) chan dnsPathDecision {
	ch := make(chan dnsPathDecision, 1)
	h.dnsPathMu.Lock()
	if h.dnsPathWaits == nil {
		h.dnsPathWaits = make(map[string]chan dnsPathDecision)
	}
	h.dnsPathWaits[name] = ch
	h.dnsPathMu.Unlock()
	return ch
}

func (h *Helper) endDNSPathWait(name string) {
	h.dnsPathMu.Lock()
	delete(h.dnsPathWaits, name)
	h.dnsPathMu.Unlock()
}

// resolveDNSPathWait delivers a decision to a parked connect, if any.
func (h *Helper) resolveDNSPathWait(name string, decision dnsPathDecision) bool {
	h.dnsPathMu.Lock()
	ch, ok := h.dnsPathWaits[name]
	h.dnsPathMu.Unlock()
	if !ok {
		return false
	}
	select {
	case ch <- decision:
	default:
	}
	return true
}

// waiveDNSPathClaim suppresses a tunnel's claim for the current connect
// attempt. Used when the user answers "turn it off and connect": the GUI has
// already persisted the switch change, and this covers the case where the
// write has not reached the helper's view yet (or failed).
func (h *Helper) waiveDNSPathClaim(name string) {
	h.dnsPathMu.Lock()
	if h.dnsPathWaived == nil {
		h.dnsPathWaived = make(map[string]bool)
	}
	h.dnsPathWaived[name] = true
	h.dnsPathMu.Unlock()
}

func (h *Helper) dnsPathClaimWaived(name string) bool {
	h.dnsPathMu.Lock()
	defer h.dnsPathMu.Unlock()
	return h.dnsPathWaived[name]
}

// clearDNSPathWaiver forgets a waived claim. Called when a new connect starts
// (so the question is asked again) and when the tunnel disconnects.
func (h *Helper) clearDNSPathWaiver(name string) {
	h.dnsPathMu.Lock()
	delete(h.dnsPathWaived, name)
	h.dnsPathMu.Unlock()
}

// handleResolveDNSPathConflict is the GUI's answer to a parked connect.
func (h *Helper) handleResolveDNSPathConflict(params json.RawMessage) (interface{}, error) {
	var req ipc.DNSPathResolveRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	if req.Tunnel == "" {
		return nil, fmt.Errorf("tunnel is required")
	}
	switch req.Action {
	case ipc.DNSPathResolveDisable:
		h.waiveDNSPathClaim(req.Tunnel)
		if !h.resolveDNSPathWait(req.Tunnel, dnsPathProceed) {
			slog.Debug("DNS resolve path decision with no parked connect",
				"category", "policy", "tunnel", req.Tunnel)
		}
	case ipc.DNSPathResolveCancel:
		if !h.resolveDNSPathWait(req.Tunnel, dnsPathAbort) {
			slog.Debug("DNS resolve path cancel with no parked connect",
				"category", "policy", "tunnel", req.Tunnel)
		}
	default:
		return nil, fmt.Errorf("unknown action %q", req.Action)
	}
	return ipc.Empty{}, nil
}
