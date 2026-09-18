package domain

import (
	"fmt"
	"time"
)

// State represents the tunnel connection state.
type State string

const (
	StateDisconnected State = "disconnected"
	StateConnecting   State = "connecting"
	StateConnected    State = "connected"
	StateError        State = "error"
)

// ConnectionStatus is the single source of truth for tunnel connection state
// across the whole application. It carries both wire-safe fields (strings,
// JSON-tagged) that are sent to the frontend, and internal fields (time.Time,
// `json:"-"`) that the reconnect monitor and other backend services use for
// duration math.
//
// Note that `LastHandshake` is the *formatted age string* (e.g. "5s", "2m 10s")
// that the frontend displays, while `LastHandshakeTime` is the absolute
// timestamp used internally. Wire callers see only the former.
type ConnectionStatus struct {
	State             State     `json:"state"`
	TunnelName        string    `json:"tunnel_name"`
	InterfaceName     string    `json:"interface_name,omitempty"`
	ConnectedAt       time.Time `json:"-"`
	Duration          string    `json:"duration,omitempty"`
	RxBytes           int64     `json:"rx_bytes"`
	TxBytes           int64     `json:"tx_bytes"`
	LastHandshakeTime time.Time `json:"-"`
	LastHandshake     string    `json:"last_handshake,omitempty"`
	// LastHandshakeUnix is LastHandshakeTime as Unix seconds — the one
	// handshake field that crosses the wire (LastHandshakeTime itself is
	// internal). The UI ages it against its own clock so the rendered age
	// keeps moving even when the status stream stalls, and so a tunnel whose
	// peer stopped answering stops claiming a fresh handshake. 0 means no
	// handshake has ever completed.
	LastHandshakeUnix int64 `json:"last_handshake_unix,omitempty"`
	// HandshakeStale is true when the tunnel has been up longer than the
	// stale threshold yet its peer has not completed a WireGuard handshake
	// within that window. A healthy tunnel re-handshakes on its keepalive
	// interval (often 25s); a tunnel whose upstream dropped — the local NIC
	// and WireGuard interface stay up, but the peer is unreachable — stops
	// handshaking and this flips true. The UI renders it as a degraded
	// "connected but no recent handshake" state instead of a falsely-healthy
	// one, so a dead WAN link no longer reads as a working tunnel.
	HandshakeStale bool `json:"handshake_stale,omitempty"`
	Endpoint          string    `json:"endpoint,omitempty"`
	// LatencyMs is the most recent measured round-trip time to the
	// endpoint in milliseconds. 0 means "no measurement yet" or "endpoint
	// unreachable" — the frontend treats both the same (renders "—").
	LatencyMs    float64 `json:"latency_ms,omitempty"`
	ErrorMessage string  `json:"error_message,omitempty"`

	// LatencyProbeTarget is the address that actually produced LatencyMs
	// (one of the configured candidates), so the UI can show what the
	// number refers to instead of an unattributed figure.
	LatencyProbeTarget string `json:"latency_probe_target,omitempty"`

	// LatencyProbeState reports the outcome of the last probe cycle:
	//   "ok"          — a candidate answered; LatencyMs is its RTT.
	//   "unreachable" — every candidate failed. The tunnel may still be up
	//                   (ICMP is commonly filtered), but from the user's
	//                   point of view nothing behind it answers.
	//   ""            — not measured yet.
	// Distinct from LatencyMs == 0, which is ambiguous between "no
	// measurement" and "nothing reachable".
	LatencyProbeState string `json:"latency_probe_state,omitempty"`

	// LatencyProbeResults carries the per-candidate breakdown behind
	// LatencyMs: every candidate is probed every cycle, so the UI can
	// render one row per target with its own health colour instead of a
	// single unattributed number. Empty until the first probe completes.
	LatencyProbeResults []ProbeResult `json:"latency_probe_results,omitempty"`

	// ActiveTunnels lists the names of all currently connected (or connecting)
	// tunnels. Populated by the multi-tunnel manager so the frontend can show
	// which tunnels are active.
	//
	// "Active" here means *transitioning or up*: it includes a tunnel that is
	// still connecting and may yet fail. Use EstablishedTunnels for anything
	// that must not lie to the user (green badge, tray icon, tray bubble).
	ActiveTunnels []string `json:"active_tunnels,omitempty"`

	// EstablishedTunnels lists the names of tunnels that have finished
	// setup and reached StateConnected — the subset of ActiveTunnels that
	// genuinely carries traffic. Rendered states (badge, icon, "Connected"
	// popup) key off this list, so a tunnel whose connect attempt fails is
	// never advertised as connected for the duration of its attempt.
	EstablishedTunnels []string `json:"established_tunnels,omitempty"`

	// Tunnels carries per-tunnel status for multi-tunnel setups. The frontend
	// uses this to show stats for the selected tunnel rather than the "primary".
	Tunnels []ConnectionStatus `json:"tunnels,omitempty"`
}

// ProbeResult is the outcome of pinging ONE candidate address.
//
// Every candidate is probed on every cycle — not "first one that answers
// wins" — because each row answers a different question: the public probes
// say whether the tunnel path works, the endpoint says whether the peer is
// reachable, and a user-pinned target says whether that specific host is up.
// Aggregating them into a single number hid exactly the information that
// makes the reading useful.
type ProbeResult struct {
	// Target is what was pinged, as displayed (may be a hostname).
	Target string `json:"target"`
	// ResolvedIP is Target resolved to an address, when it is not already
	// one. Empty for a literal IP or when resolution failed.
	ResolvedIP string `json:"resolved_ip,omitempty"`
	// Kind classifies the row so the UI can label it:
	//   "public"   — a built-in public probe (full tunnels only)
	//   "endpoint" — the peer endpoint
	//   "custom"   — the address the user typed
	Kind string `json:"kind,omitempty"`
	// Slot is the editor row this probe came from, or -1 for the endpoint
	// (which is not configurable). It lets the UI show a row's live resolved
	// address next to the box the user typed it in — the mapping cannot be
	// done by name, because two slots may hold the same hostname after an
	// edit and the endpoint appears as a target of its own.
	Slot int `json:"slot"`
	// Coverage reports whether the probed address actually travels through
	// this tunnel:
	//   "inside"  — inside AllowedIPs (or a full tunnel)
	//   "outside" — outside AllowedIPs on a split tunnel: the probe never
	//               enters the tunnel, so its RTT describes the plain
	//               internet path and the value is meaningless
	//   ""        — not judgeable (unresolvable, or the config is unreadable)
	// Re-evaluated every cycle on purpose: AllowedIPs can be edited and a
	// DDNS name can move to an address outside the tunnel *after* the target
	// was accepted, and a saved-then-invalid target is exactly the case the
	// user asked to be told about.
	Coverage string `json:"coverage,omitempty"`
	// Reachable reports whether the target answered ICMP.
	Reachable bool `json:"reachable"`
	// LatencyMs is the round-trip time; 0 when unreachable.
	LatencyMs float64 `json:"latency_ms"`
}

// FormatDuration renders a duration in a compact "1h 2m 3s" form used by the
// UI. Negative durations (possible if the system clock jumps backward relative
// to a stored timestamp) are clamped to "0s" rather than producing a
// confusing "-5s" in the UI.
func FormatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
