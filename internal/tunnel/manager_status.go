package tunnel

import (
	"log/slog"
	"sort"
	"time"

	"github.com/imonior/wireguide-plus/internal/domain"
)

// Status returns the status of the first connected (or connecting) tunnel.
// For backward compatibility with single-tunnel callers. The returned
// ConnectionStatus includes ActiveTunnels listing all active tunnel names.
func (m *Manager) Status() *ConnectionStatus {
	m.mu.Lock()
	// Find the "primary" tunnel (first connected, or first connecting).
	var primary *tunnelEntry
	var primaryName string
	activeTunnels := m.activeTunnelNamesLocked()
	for _, name := range activeTunnels {
		e := m.tunnels[name]
		if e.state == domain.StateConnected {
			primary = e
			primaryName = name
			break
		}
	}
	if primary == nil {
		for _, name := range activeTunnels {
			e := m.tunnels[name]
			if e.state == domain.StateConnecting || e.state == stateDisconnecting {
				primary = e
				primaryName = name
				break
			}
		}
	}
	// Check for error state tunnels if nothing else found.
	if primary == nil {
		for name, e := range m.tunnels {
			if e.state == domain.StateError {
				primary = e
				primaryName = name
				break
			}
		}
	}

	if primary == nil {
		m.mu.Unlock()
		return &ConnectionStatus{
			State:         domain.StateDisconnected,
			ActiveTunnels: activeTunnels,
		}
	}

	state := primary.state
	engine := primary.engine
	connectedAt := primary.connectedAt
	cfgName := ""
	if primary.cfg != nil {
		cfgName = primary.cfg.Name
	}
	m.mu.Unlock()

	switch state {
	case domain.StateConnecting, stateDisconnecting:
		return &ConnectionStatus{
			State:         domain.StateConnecting,
			TunnelName:    cfgName,
			ActiveTunnels: activeTunnels,
		}
	case domain.StateDisconnected:
		return &ConnectionStatus{
			State:         domain.StateDisconnected,
			ActiveTunnels: activeTunnels,
		}
	case domain.StateError:
		return &ConnectionStatus{
			State:         domain.StateError,
			TunnelName:    cfgName,
			ActiveTunnels: activeTunnels,
		}
	}

	// StateConnected — talk to wgctrl without holding m.mu.
	if engine == nil {
		return &ConnectionStatus{
			State:         domain.StateDisconnected,
			ActiveTunnels: activeTunnels,
		}
	}
	status, err := getStatusForEngine(engine, cfgName, connectedAt)
	if err != nil {
		slog.Warn("failed to get status", "error", err)
		return &ConnectionStatus{
			State:         domain.StateError,
			TunnelName:    cfgName,
			ActiveTunnels: activeTunnels,
		}
	}
	// Reuse the honest-duration accumulator maintained by AllStatuses' 1Hz
	// loop so on-demand reads match the broadcasted uptime. The accumulator
	// only advances while the peer is answering, so a dead upstream reads as
	// paused rather than a growing number.
	m.mu.Lock()
	if e := m.tunnels[primaryName]; e != nil {
		status.Duration = domain.FormatDuration(e.healthyAccum)
		status.Paused = e.paused
	}
	m.mu.Unlock()
	status.ActiveTunnels = activeTunnels
	return status
}

// StatusFor returns the status of a specific tunnel by name.
func (m *Manager) StatusFor(name string) *ConnectionStatus {
	m.mu.Lock()
	entry, ok := m.tunnels[name]
	if !ok {
		m.mu.Unlock()
		return &ConnectionStatus{State: domain.StateDisconnected, TunnelName: name}
	}
	state := entry.state
	engine := entry.engine
	connectedAt := entry.connectedAt
	cfgName := ""
	if entry.cfg != nil {
		cfgName = entry.cfg.Name
	}
	m.mu.Unlock()

	switch state {
	case domain.StateConnecting, stateDisconnecting:
		return &ConnectionStatus{State: domain.StateConnecting, TunnelName: cfgName}
	case domain.StateDisconnected:
		return &ConnectionStatus{State: domain.StateDisconnected, TunnelName: cfgName}
	case domain.StateError:
		return &ConnectionStatus{State: domain.StateError, TunnelName: cfgName}
	}

	if engine == nil {
		return &ConnectionStatus{State: domain.StateDisconnected, TunnelName: cfgName}
	}
	status, err := getStatusForEngine(engine, cfgName, connectedAt)
	if err != nil {
		return &ConnectionStatus{State: domain.StateError, TunnelName: cfgName}
	}
	// Reuse the honest-duration accumulator (see Status for rationale).
	m.mu.Lock()
	if e := m.tunnels[name]; e != nil {
		status.Duration = domain.FormatDuration(e.healthyAccum)
		status.Paused = e.paused
	}
	m.mu.Unlock()
	return status
}

// AllStatuses returns the status of every tunnel that has an entry.
func (m *Manager) AllStatuses() []*ConnectionStatus {
	m.mu.Lock()
	type snap struct {
		name        string
		state       domain.State
		engine      *Engine
		connectedAt time.Time
		cfgName     string
	}
	var snaps []snap
	for name, e := range m.tunnels {
		cfgName := ""
		if e.cfg != nil {
			cfgName = e.cfg.Name
		}
		snaps = append(snaps, snap{name, e.state, e.engine, e.connectedAt, cfgName})
	}
	m.mu.Unlock()

	var out []*ConnectionStatus
	for _, s := range snaps {
		switch s.state {
		case domain.StateConnecting, stateDisconnecting:
			out = append(out, &ConnectionStatus{State: domain.StateConnecting, TunnelName: s.cfgName})
		case domain.StateDisconnected:
			out = append(out, &ConnectionStatus{State: domain.StateDisconnected, TunnelName: s.cfgName})
		case domain.StateError:
			out = append(out, &ConnectionStatus{State: domain.StateError, TunnelName: s.cfgName})
		case domain.StateConnected:
			if s.engine == nil {
				out = append(out, &ConnectionStatus{State: domain.StateDisconnected, TunnelName: s.cfgName})
				continue
			}
			st, err := getStatusForEngine(s.engine, s.cfgName, s.connectedAt)
			if err != nil {
				out = append(out, &ConnectionStatus{State: domain.StateError, TunnelName: s.cfgName})
				break
			}
			// Advance the honest-duration accumulator (only counts time the
			// peer was actually answering) and stamp it onto the status, so a
			// dead upstream freezes the timer instead of inflating uptime.
			m.mu.Lock()
			if e := m.tunnels[s.name]; e != nil {
				m.advanceDuration(e, st, time.Now())
				st.Duration = domain.FormatDuration(e.healthyAccum)
				st.Paused = e.paused
			} else {
				st.Duration = domain.FormatDuration(time.Since(s.connectedAt))
			}
			m.mu.Unlock()
			out = append(out, st)
		}
	}
	return out
}

// advanceDuration updates e.healthyAccum to reflect only the wall-clock time
// this tunnel spent genuinely connected during the latest tick. Caller MUST
// hold m.mu. `st` is the status computed for this tick (carries
// HandshakeStale); `now` is the tick time.
//
// A WireGuard interface can stay UP while its peer is unreachable — the NIC
// went to sleep, the lid closed, the upstream died — in which case handshakes
// stop but StateConnected does not change. Counting that dead time as
// "connected" is exactly the bug where the UI shows "22h connected" for a link
// that dropped hours ago. So we only add elapsed time when the handshake is NOT
// stale; otherwise we freeze the counter and mark it paused.
//
// The logic is idempotent across call sites: it is driven from AllStatuses()
// (the 1Hz event loop) but also safe if Status()/StatusFor() or the reconnect
// monitor call it again, because elapsed is measured from lastTick — a second
// call in the same interval adds ≈0.
func (m *Manager) advanceDuration(e *tunnelEntry, st *domain.ConnectionStatus, now time.Time) {
	if e == nil || st == nil {
		return
	}
	if e.lastTick.IsZero() {
		// First accounting tick since (re)connect: anchor the baseline,
		// don't count the pre-existing gap.
		e.lastTick = now
		e.healthyAccum = 0
		e.paused = false
		return
	}
	elapsed := now.Sub(e.lastTick)
	e.lastTick = now
	if elapsed <= 0 {
		return
	}
	if !st.HandshakeStale {
		e.healthyAccum += elapsed
		e.paused = false
	} else {
		e.paused = true
	}
}

// IsConnected returns true if ANY tunnel is fully established.
func (m *Manager) IsConnected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.tunnels {
		if e.state == domain.StateConnected {
			return true
		}
	}
	return false
}

// IsTunnelConnected returns true if the named tunnel is fully established.
func (m *Manager) IsTunnelConnected(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.tunnels[name]
	return ok && e.state == domain.StateConnected
}

// ResolvedEndpointIPs returns the union of pre-resolved endpoint IP addresses
// from all active engines. Returns nil if no tunnel is connected.
func (m *Manager) ResolvedEndpointIPs() []string {
	m.mu.Lock()
	var engines []*Engine
	for _, e := range m.tunnels {
		if e.engine != nil {
			engines = append(engines, e.engine)
		}
	}
	m.mu.Unlock()

	if len(engines) == 0 {
		return nil
	}
	var all []string
	for _, eng := range engines {
		all = append(all, eng.ResolvedEndpointIPs()...)
	}
	return all
}

// ResolvedEndpoints returns the union of pre-resolved endpoint ip:port pairs
// from all active engines. Returns nil if no tunnel is connected.
func (m *Manager) ResolvedEndpoints() []string {
	m.mu.Lock()
	var engines []*Engine
	for _, e := range m.tunnels {
		if e.engine != nil {
			engines = append(engines, e.engine)
		}
	}
	m.mu.Unlock()

	if len(engines) == 0 {
		return nil
	}
	var all []string
	for _, eng := range engines {
		all = append(all, eng.ResolvedEndpoints()...)
	}
	return all
}

// ActiveTunnel returns the name of the first connected (or connecting)
// tunnel, or "" if none. Kept for backward compatibility — callers that
// only support a single tunnel can use this.
func (m *Manager) ActiveTunnel() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := m.activeTunnelNamesLocked()
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

// EstablishedTunnels returns the names of tunnels that have *completed*
// setup, i.e. reached StateConnected.
//
// Deliberately narrower than ActiveTunnels: that one also contains
// StateConnecting and stateDisconnecting, which describe a transition that
// may still fail. Anything that renders a "connected" badge, a green icon or
// a "Connected" tray bubble must use this list instead — otherwise a tunnel
// whose endpoint could not even be resolved (DNS not up yet at boot, a wrong
// hostname) shows as connected for the whole duration of its attempt and
// then silently vanishes, which reads as "connected, then dropped".
func (m *Manager) EstablishedTunnels() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var names []string
	for name, e := range m.tunnels {
		if e.state == domain.StateConnected {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// ActiveTunnels returns the names of all connected or connecting tunnels,
// sorted alphabetically for deterministic ordering.
func (m *Manager) ActiveTunnels() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeTunnelNamesLocked()
}

// activeTunnelNamesLocked returns sorted names of all active tunnels.
// Caller MUST hold m.mu.
func (m *Manager) activeTunnelNamesLocked() []string {
	var names []string
	for name, e := range m.tunnels {
		if e.state == domain.StateConnected || e.state == domain.StateConnecting || e.state == stateDisconnecting {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}
