//go:build linux

package helper

import (
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"syscall"

	"log/slog"
)

// inhibitMu guards the long-lived systemd-inhibit child process that blocks
// suspend while "keep running in background" is enabled.
var (
	inhibitMu  sync.Mutex
	inhibitCmd *exec.Cmd
)

// applySystemSleepPrevention mirrors the Windows behaviour on Linux: it blocks
// the system from idling into suspend/hibernate so the NIC keeps power and
// tunnels survive screensaver / screen-off / suspend.
//
// The bundled `systemd-inhibit` (every systemd-logind desktop) runs a command
// under an inhibitor lock; when the command ends the lock is released. We hold
// it open with `sleep infinity`. Setpgid makes the child its own process-group
// leader so we can kill the WHOLE group — systemd-inhibit AND its `sleep`
// grandchild — on disable, instead of leaving the `sleep infinity` process
// orphaned and spinning forever. Pdeathsig guarantees the child is reaped if
// the helper crashes, so the override never leaks. Hosts without
// systemd-inhibit (non-systemd init) degrade to a logged warning instead of
// failing the toggle.
func applySystemSleepPrevention(enabled bool) error {
	inhibitMu.Lock()
	defer inhibitMu.Unlock()

	if !enabled {
		if inhibitCmd != nil && inhibitCmd.Process != nil {
			pid := inhibitCmd.Process.Pid
			// Negative pid targets the whole process group (Setpgid made the
			// child the leader), so the orphaned `sleep infinity` is killed
			// too. The direct Kill is a best-effort fallback.
			_ = syscall.Kill(-pid, syscall.SIGKILL)
			_ = inhibitCmd.Process.Kill()
		}
		inhibitCmd = nil
		return nil
	}

	// Already running and still alive — idempotent across startup restore +
	// live toggles. Signal(0) is a no-op probe that FAILS if the process
	// already exited, so we drop the stale handle and respawn below rather
	// than trusting a dead child and silently leaving the machine unguarded.
	if inhibitCmd != nil && inhibitCmd.Process != nil {
		if err := inhibitCmd.Process.Signal(syscall.Signal(0)); err == nil {
			return nil
		}
		inhibitCmd = nil
	}

	// --what=idle  : block auto idle-suspend
	// --what=sleep : block manual/auto suspend & hibernate
	// --mode=block : prevent, not just delay
	cmd := exec.Command("systemd-inhibit",
		"--what=idle:sleep",
		"--why=WireGuide Plus background keep-alive",
		"--mode=block",
		"sleep", "infinity")
	// Setpgid makes this child the leader of a fresh process group, and
	// Pdeathsig reaps it if the helper dies unexpectedly — so the inhibitor
	// lock is never left dangling.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGKILL,
	}
	if err := cmd.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			slog.Warn("system sleep prevention unavailable: systemd-inhibit not found (non-systemd host?)")
			return nil
		}
		return fmt.Errorf("start systemd-inhibit: %w", err)
	}
	inhibitCmd = cmd
	go func() {
		_ = cmd.Wait()
		inhibitMu.Lock()
		if inhibitCmd == cmd {
			inhibitCmd = nil
		}
		inhibitMu.Unlock()
	}()
	slog.Info("system sleep prevention enabled (Linux systemd-inhibit)")
	return nil
}
