//go:build darwin || linux

package helper

import (
	"fmt"
	"os"
)

// prepareLogsDir creates the helper's daily-log directory (dir is inside the
// USER's home, e.g. ~/Library/Logs/wireguideplus) and hands ownership to the
// desktop user identified by uid. The helper runs as root, so a helper that
// spawns before the GUI (LaunchDaemon at boot, or a fresh install) would
// otherwise leave the directory root-owned at 0700 — every later GUI start
// then dies at "create dirs: ... exists but is not writable". Re-chowning an
// existing directory also self-heals machines already stuck in that state.
func prepareLogsDir(dir string, uid int) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	if uid < 0 {
		return nil
	}
	if err := os.Chown(dir, uid, -1); err != nil {
		return fmt.Errorf("chown %s to uid %d: %w", dir, uid, err)
	}
	return nil
}
