//go:build !windows

package gui

import (
	"errors"
	"os"
	"syscall"
)

// acquireInstanceLock takes a non-blocking exclusive flock on the file.
// flock lives on the open-file description, so it survives even a same-
// process second attempt (unlike POSIX record locks) and is released by
// the kernel when the process dies — no stale-lock cleanup on crash.
func acquireInstanceLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// isInstanceLockConflict reports whether a failed acquire means "a twin
// instance holds the lock" rather than "the lock could not be taken".
// flock(LOCK_NB) reports EWOULDBLOCK for the former; everything else
// (EACCES on a root-owned lock file, a read-only config directory) is an
// environment failure that must not be mistaken for a running twin.
func isInstanceLockConflict(err error) bool {
	return errors.Is(err, syscall.EWOULDBLOCK)
}
