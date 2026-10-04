package gui

import (
	"log/slog"
	"sync/atomic"

	"github.com/imonior/wireguide-plus/internal/ipc"
)

// logsDirRepairPending records that the GUI's log directory was found owned by
// another user (normally the root helper) and still needs the privileged helper
// to chown it back. It stays set until a repair actually succeeds, so a helper
// that is unavailable at startup — the user cancelled the elevation prompt, or
// all three ensureHelper attempts failed — is retried the moment the health
// monitor later finds a live helper.
var logsDirRepairPending atomic.Bool

// markLogsDirNeedsRepair records that the log directory ownership must be
// repaired by the helper.
func markLogsDirNeedsRepair() {
	logsDirRepairPending.Store(true)
}

// logsDirRepairNeeded reports whether a pending repair is still outstanding.
func logsDirRepairNeeded() bool {
	return logsDirRepairPending.Load()
}

// repairLogsDirOwnership asks the privileged helper to chown the user-side log
// directory (and everything it wrote there) back to the desktop user, then
// re-points GUI file logging at the now-writable directory. Safe to call
// whenever a helper connection is available and the repair is still pending —
// it is a no-op otherwise, so the health monitor can call it on every tick
// without extra bookkeeping.
//
// A failure leaves the flag set so a later helper (or a later tick) retries.
func repairLogsDirOwnership(c *ipc.Client, logsDir string) {
	if c == nil || logsDir == "" || !logsDirRepairNeeded() {
		return
	}
	if err := c.Call(ipc.MethodRepairLogOwnership, ipc.Empty{}, nil); err != nil {
		slog.Warn("log directory ownership repair failed", "dir", logsDir, "error", err)
		return
	}
	logsDirRepairPending.Store(false)
	slog.Info("log directory ownership repaired by helper", "dir", logsDir)
	setGUILogFile(logsDir)
}
