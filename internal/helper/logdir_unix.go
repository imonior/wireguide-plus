//go:build darwin || linux

package helper

import (
	"fmt"
	"os"
	"path/filepath"
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
	if err := chownR(dir, uid); err != nil {
		return fmt.Errorf("chown %s to uid %d: %w", dir, uid, err)
	}
	return nil
}

// chownR walks dir and chowns every entry (and dir itself) to uid, leaving the
// group unchanged. The helper runs as root, so this repairs ownership of the
// whole log directory it populated — the top-level directory plus every flat
// <prefix>-YYYY-MM-DD.log file it created across days of operation, not just the
// top-level directory. It does not follow symlinks (filepath.WalkDir never
// descends into them) and stops at the first error rather than risk a partial
// repair masking a real problem.
func chownR(dir string, uid int) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e := os.Chown(path, uid, -1); e != nil {
			return fmt.Errorf("chown %s: %w", path, e)
		}
		return nil
	})
}
