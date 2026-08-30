package tunnel

import (
	"context"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// scriptTimeout bounds each Pre/PostUp/Down command. WireGuard scripts
// routinely sleep or wait for a peer; 30 s is long enough for real use
// while still preventing a hung script from wedging connect/disconnect.
const scriptTimeout = 30 * time.Second

// RunScript executes a single Pre/PostUp/Down hook command inside the
// helper (therefore as root/admin). This is intentionally an opt-in
// feature (Settings → advanced → "WireGuard scripts"): the commands run
// with full system privileges and a malicious tunnel config could do
// anything, so the GUI shows a prominent warning before enabling.
//
// Execution mirrors wg-quick: `sh -c` on Unix, `cmd.exe /C` on Windows.
// Success/failure (with output) is logged under the tunnel category so
// the log tells the script story end to end.
func RunScript(hook, command string) {
	ctx, cancel := context.WithTimeout(context.Background(), scriptTimeout)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd.exe", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	out, err := cmd.CombinedOutput()

	// Trim and cap the echoed output so a chatty script can't flood the
	// log viewer with hundreds of lines.
	echo := strings.TrimSpace(string(out))
	if len(echo) > 1000 {
		echo = echo[:1000] + "…"
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			slog.Warn("wireguard script timed out",
				"category", "tunnel", "hook", hook, "timeout", scriptTimeout)
		} else {
			slog.Warn("wireguard script failed",
				"category", "tunnel", "hook", hook, "error", err)
		}
		if echo != "" {
			slog.Warn("wireguard script output", "category", "tunnel", "hook", hook, "output", echo)
		}
		return
	}
	if echo != "" {
		slog.Info("wireguard script output", "category", "tunnel", "hook", hook, "output", echo)
	} else {
		slog.Info("wireguard script completed", "category", "tunnel", "hook", hook)
	}
}
