//go:build darwin

package helper

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"log/slog"
)

// caffeinateMu guards the long-lived caffeinate child process that keeps the
// machine awake while "keep running in background" is enabled.
var (
	caffeinateMu  sync.Mutex
	caffeinateCmd *exec.Cmd
)

// applySystemSleepPrevention mirrors the Windows behaviour on macOS: it keeps
// the system (and display) awake so the NIC never loses power and tunnels stay
// connected through screensaver / screen-off / suspend.
//
// macOS has no SetThreadExecutionState equivalent, but the bundled `caffeinate`
// CLI does exactly this. We spawn it as a child and tie its lifetime to ours
// via -w <pid>, so it exits automatically when the helper dies — no orphaned
// override. Disabling kills the child, releasing the override immediately.
func applySystemSleepPrevention(enabled bool) error {
	caffeinateMu.Lock()
	defer caffeinateMu.Unlock()

	if !enabled {
		if caffeinateCmd != nil && caffeinateCmd.Process != nil {
			_ = caffeinateCmd.Process.Kill()
		}
		caffeinateCmd = nil
		return nil
	}

	// Already running and still alive — idempotent across startup restore +
	// live toggles. Signal(0) is a no-op probe that FAILS if the process
	// already exited, so we drop the stale handle and respawn below rather
	// than trusting a dead child and silently leaving the machine unguarded.
	if caffeinateCmd != nil && caffeinateCmd.Process != nil {
		if err := caffeinateCmd.Process.Signal(syscall.Signal(0)); err == nil {
			return nil
		}
		caffeinateCmd = nil
	}

	// -i : prevent idle system sleep
	// -d : prevent display sleep (mirrors ES_DISPLAY_REQUIRED on Windows)
	// -w : exit when the given pid exits, so the override is never leaked
	cmd := exec.Command("caffeinate", "-i", "-d", "-w", fmt.Sprint(os.Getpid()))
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start caffeinate: %w", err)
	}
	caffeinateCmd = cmd
	go func() {
		_ = cmd.Wait()
		caffeinateMu.Lock()
		if caffeinateCmd == cmd {
			caffeinateCmd = nil
		}
		caffeinateMu.Unlock()
	}()
	slog.Info("system sleep prevention enabled (macOS caffeinate)")
	return nil
}
