package storage

import (
	"os"
	"path/filepath"
	"runtime"
)

// writeBytesAtomic replaces a file's content with data in one step: write to
// a temp file in the destination directory, fsync it, then rename over the
// target. Readers see either the old bytes or the new ones, never a truncated
// middle, and a failed write leaves the previous file intact. The temp file is
// created with os.CreateTemp's 0600 and stays 0600, which is what every
// caller here wants (these files hold keys and settings).
func writeBytesAtomic(data []byte, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmpFile, err := os.CreateTemp(dir, ".wireguide-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := atomicRenameDurable(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}

// atomicRenameDurable renames src→dst and then fsyncs the containing
// directory so the rename's directory entry survives a power loss. The
// per-file fsync in the writers makes the CONTENT durable, but the
// rename itself isn't durable until the directory metadata is flushed —
// without this a save can report success and then vanish on reboot.
// Best-effort on the directory sync: not all filesystems/platforms
// support directory fsync (notably Windows, where opening a directory as
// a file fails), so a sync error there is not treated as a save failure.
func atomicRenameDurable(src, dst string) error {
	if err := os.Rename(src, dst); err != nil {
		return err
	}
	syncDir(filepath.Dir(dst))
	return nil
}

// syncDir best-effort fsyncs a directory. No-op on Windows (directory
// handles can't be opened as files there) and silently ignores errors on
// filesystems that don't support it.
func syncDir(dir string) {
	if runtime.GOOS == "windows" {
		return
	}
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}
