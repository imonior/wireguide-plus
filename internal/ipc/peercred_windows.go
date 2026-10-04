//go:build windows

package ipc

import (
	"errors"
	"fmt"
	"net"

	"golang.org/x/sys/windows"
)

// getPeerCredential is a no-op on Windows — peer identity is checked by
// SID in verifyPeer below, not by UID.
func getPeerCredential(conn net.Conn) (uid uint32, pid int32, err error) {
	return 0, 0, nil
}

// verifyPeer checks that the connecting process's token user matches the
// expected owner SID (issue #20). The pipe's SDDL is the first gate; this
// is the per-connection second gate, protecting against ACL regressions
// and any path that loosens the descriptor.
//
// expectedSID == "" is not a state a production helper can reach: Listen
// refuses to open the pipe without an owner SID, so a helper that is listening
// always knows whose SID to check. The branch survives for callers that build
// a Server without going through Listen (this package's tests), where the
// per-connection check has nothing to compare against and the pipe ACL is the
// only gate. A caller that asks for UID enforcement is refused rather than
// waved through, because Windows has no UID to enforce.
func verifyPeer(conn net.Conn, expectedUID int, expectedSID string) error {
	if expectedSID == "" {
		if expectedUID < 0 {
			return nil
		}
		return errors.New("verifyPeer: no owner SID configured; per-connection UID check not possible on Windows")
	}

	want, err := windows.StringToSid(expectedSID)
	if err != nil {
		return fmt.Errorf("verifyPeer: invalid expected SID %q: %w", expectedSID, err)
	}

	fdc, ok := conn.(interface{ Fd() uintptr })
	if !ok {
		return errors.New("verifyPeer: pipe connection does not expose a handle")
	}
	var pid uint32
	if err := windows.GetNamedPipeClientProcessId(windows.Handle(fdc.Fd()), &pid); err != nil {
		return fmt.Errorf("verifyPeer: GetNamedPipeClientProcessId: %w", err)
	}
	proc, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return fmt.Errorf("verifyPeer: OpenProcess(%d): %w", pid, err)
	}
	defer windows.CloseHandle(proc)
	var tok windows.Token
	if err := windows.OpenProcessToken(proc, windows.TOKEN_QUERY, &tok); err != nil {
		return fmt.Errorf("verifyPeer: OpenProcessToken(%d): %w", pid, err)
	}
	defer tok.Close()
	u, err := tok.GetTokenUser()
	if err != nil {
		return fmt.Errorf("verifyPeer: GetTokenUser(%d): %w", pid, err)
	}

	// The owner's own elevated processes keep the same user SID
	// (elevation changes group membership, not the user), so an admin
	// terminal run by the owner still passes. SYSTEM is allowed for
	// service-context tooling (e.g. the helper health-checking itself).
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err == nil && u.User.Sid.Equals(system) {
		return nil
	}
	if !u.User.Sid.Equals(want) {
		return fmt.Errorf("verifyPeer: pid %d user %s does not match pipe owner %s",
			pid, u.User.Sid.String(), expectedSID)
	}
	return nil
}
