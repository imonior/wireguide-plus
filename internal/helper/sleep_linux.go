//go:build linux

package helper

import (
	"errors"
	"fmt"
	"os"
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
// under an inhibitor lock; when the command ends the lock is released. Rather
// than holding `sleep infinity` open — whose grandchild survives a helper
// SIGKILL because Pdeathsig only reaps the direct child — we run a tiny
// self-watching shell that polls the helper's own PID: it exits the instant
// the helper disappears (SIGKILL, crash, OOM), releasing the inhibitor lock
// with it. Setpgid keeps the whole group (systemd-inhibit + the shell + its
// brief `sleep 5`) as one unit so a normal disable kills them all at once.
// Pdeathsig is belt-and-braces for the direct child. Hosts without
// systemd-inhibit (non-systemd init) degrade to a logged warning instead of
// failing the toggle.
func applySystemSleepPrevention(enabled bool) error {
	inhibitMu.Lock()
	defer inhibitMu.Unlock()

	if !enabled {
		if inhibitCmd != nil && inhibitCmd.Process != nil {
			pid := inhibitCmd.Process.Pid
			// Negative pid targets the whole process group (Setpgid made the
			// child the leader), so systemd-inhibit, the watching shell and
			// its `sleep 5` all die together. The direct Kill is a
			// best-effort fallback.
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
	//
	// systemd-inhibit holds an inhibitor lock while running its CHILD COMMAND;
	// when that child exits, the lock is released. The child command is a
	// shell that watches THIS helper's PID: it loops until the helper is
	// gone, then exits, which releases the inhibitor lock. This is what
	// guarantees no orphan when the helper is SIGKILLed — `sleep infinity`
	// could never do this because its grandchild outlived a SIGKILL of the
	// helper (Pdeathsig only reaps the direct child). The `sleep 5` inside is
	// short-lived and self-exits within 5s once the shell exits. (The macOS
	// `caffeinate -w` path uses the same "watch the PID, die with it" idea.)
	cmd := exec.Command("systemd-inhibit",
		"--what=idle:sleep",
		"--why=WireGuide Plus background keep-alive",
		"--mode=block",
		"sh", "-c",
		fmt.Sprintf("while kill -0 %d 2>/dev/null; do sleep 5; done", os.Getpid()))
	// Setpgid makes this child the leader of a fresh process group, and
	// Pdeathsig reaps it if the helper dies unexpectedly — so a normal
	// disable (kill -pid) takes the whole group down at once and the
	// inhibitor lock is never left dangling.
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
