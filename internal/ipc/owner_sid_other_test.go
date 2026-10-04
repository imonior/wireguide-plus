//go:build !windows

package ipc

import "testing"

// testOwnerSID is a no-op off Windows: the Unix transport takes no SID at
// all, since the socket's inode ownership and mode are the access gate.
func testOwnerSID(t *testing.T) string {
	t.Helper()
	return ""
}
