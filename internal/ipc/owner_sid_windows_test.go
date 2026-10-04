//go:build windows

package ipc

import (
	"testing"

	"golang.org/x/sys/windows"
)

// testOwnerSID is the SID of the user running the test binary. The pipe has
// no ownerless mode any more — an empty SID fails the listener instead of
// falling back to a grant for every interactive user — so the tests hand over
// a real one exactly as the GUI does.
func testOwnerSID(t *testing.T) string {
	t.Helper()
	tok := windows.GetCurrentProcessToken()
	u, err := tok.GetTokenUser()
	if err != nil || u == nil || u.User.Sid == nil {
		t.Skipf("no process-token user SID to scope the test pipe with: %v", err)
	}
	return u.User.Sid.String()
}
