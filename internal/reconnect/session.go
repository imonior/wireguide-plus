package reconnect

// SessionDetector reports when the user session RESUMES from an idle state —
// the display turned back on, or the workstation was unlocked after a
// screen-saver / lock. The reconnect monitor uses this to self-heal a tunnel
// that died while the machine was idle (the NIC went to sleep, the upstream
// dropped) so the user doesn't come back to a silently-dead VPN.
//
// Display-OFF / lock events are intentionally NOT surfaced: there is nothing
// to "do" on idle — tearing the tunnel down on screen-saver would be exactly
// the wrong behaviour, and keeping the handshake flowing through a sleeping
// NIC is the job of the PersistentKeepalive injected unconditionally at
// connect time. When the session resumes, healIfNeeded repairs tunnels that
// died while idle (gated on the HealthCheck master switch).
type SessionDetector interface {
	Start()
	Stop()
	ResumeChan() <-chan struct{}
}
