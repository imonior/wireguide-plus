//go:build windows

package gui

import (
	"os"
	"syscall"
)

// acquireInstanceLock emulates flock with mandatory share modes: the file
// is opened for read+write while permitting other handles to open it for
// READING only. A second instance making the same read+write open fails
// with a sharing violation (= locked), yet wakeExistingInstance can still
// open the file read-only to pick up the wake record — something a plain
// share-none open could never allow. os.NewFile adopts the handle, so
// Close (or process exit) releases the lock.
func acquireInstanceLock(path string) (*os.File, error) {
	h, err := syscall.CreateFile(
		syscall.StringToUTF16Ptr(path),
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		syscall.FILE_SHARE_READ,
		nil,
		syscall.OPEN_ALWAYS,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, err
	}
	if h == syscall.InvalidHandle {
		// ERROR_SHARING_VIOLATION — Go's syscall exports the code under a
		// different spelling, so name it here rather than import it.
		return nil, syscall.Errno(32)
	}
	return os.NewFile(uintptr(h), path), nil
}
