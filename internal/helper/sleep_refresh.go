package helper

import (
	"log/slog"
	"time"
)

// sleepPreventionLoop reconciles the OS sleep override with connection state on
// a slow tick. It runs independently of any GUI subscription — the helper is a
// long-lived privileged daemon (launchd / systemd / RunAs) that outlives the
// GUI — so a background tunnel that survives a sleep/resume, or a connect that
// happens while the GUI is closed, still engages the override.
//
// refreshSystemSleepPrevention only calls the (idempotent) OS routine when the
// desired state actually changes, so the tick is cheap even at 1 Hz.
func (h *Helper) sleepPreventionLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-h.done:
			return
		case <-ticker.C:
			h.refreshSystemSleepPrevention()
		}
	}
}

// refreshSystemSleepPrevention engages or releases the OS sleep override based
// on the current DESIRED state: the user must have enabled it AND at least one
// tunnel must be actually connected (StateConnected). We deliberately do NOT
// keep the machine awake while the app is idle with no tunnel up — that would
// be a surprising "never sleep" for a laptop that merely has the app open.
//
// The requested flag (settings.prevent_system_sleep) is remembered even when no
// tunnel is up; the moment a tunnel connects the loop engages the override, and
// the moment the last tunnel disconnects it releases it.
func (h *Helper) refreshSystemSleepPrevention() {
	h.sleepMu.Lock()
	defer h.sleepMu.Unlock()
	connected := h.manager != nil && h.manager.IsConnected()
	desired := h.sleepRequested && connected
	if desired == h.sleepActive {
		return
	}
	if err := applySystemSleepPrevention(desired); err != nil {
		slog.Warn("apply system sleep prevention failed", "desired", desired, "error", err)
		return
	}
	h.sleepActive = desired
}
