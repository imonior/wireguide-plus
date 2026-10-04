//go:build windows

package ipc

import (
	"fmt"
	"net"
	"time"
	"unsafe"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

// Listen creates a named pipe listener. ownerUID is ignored on Windows;
// ownerSID (the spawning user's SID) scopes the pipe ACL to that user.
func Listen(addr string, ownerUID int, ownerSID string) (net.Listener, error) {
	_ = ownerUID
	sddl, err := buildPipeSDDL(ownerSID)
	if err != nil {
		return nil, err
	}
	config := &winio.PipeConfig{
		SecurityDescriptor: sddl,
		MessageMode:        false,
		InputBufferSize:    65536,
		OutputBufferSize:   65536,
	}

	l, err := winio.ListenPipe(addr, config)
	if err != nil {
		return nil, fmt.Errorf("listen pipe %s: %w", addr, err)
	}
	return l, nil
}

// buildPipeSDDL returns the pipe's security descriptor. SYSTEM and
// Administrators get full control (GA), and the read+write (GRGW) grant that
// lets the unprivileged GUI connect is scoped to the spawning user's SID by
// ownerGrant — the previous grant to Interactive Users (IU / S-1-5-4) let
// EVERY logged-on account on a multi-user machine drive a SYSTEM helper:
// disconnect tunnels, drop per-tunnel System DNS enforcement, forge SSIDs into the
// automation engine, force shutdown (issue #20).
//
// A missing or malformed SID is a hard failure. It used to fall back to that
// IU grant so a helper started without --owner-sid (an older GUI, or a manual
// start) still worked — which meant the one build that most needed scoping
// was the one that quietly reopened the hole, and the app would run fine
// while every other logged-in user kept control of it. Failing closed makes
// the un-scoped configuration visible as a startup error instead.
//
// GUI and helper are always the same binary (the helper is spawned from
// os.Executable()), so an older GUI that never passes --owner-sid cannot meet
// a newer helper in the field; only a hand-started `--helper` can, and that
// operator gets an actionable message. windows.StringToSid is the last word
// before the value is interpolated into the descriptor.
func buildPipeSDDL(ownerSID string) (string, error) {
	grant, err := ownerGrant(ownerSID)
	if err != nil {
		return "", err
	}
	if _, err := windows.StringToSid(ownerSID); err != nil {
		return "", fmt.Errorf("invalid owner SID %q for the helper pipe: %w", ownerSID, err)
	}
	return "D:(A;;GA;;;SY)(A;;GA;;;BA)" + grant, nil
}

// Dial connects to a named pipe and verifies the server is owned by a trusted
// principal (Local System or Built-in Administrators).
func Dial(addr string) (net.Conn, error) {
	timeout := 5 * time.Second
	conn, err := winio.DialPipe(addr, &timeout)
	if err != nil {
		return nil, err
	}

	// H15: Verify pipe ownership after connecting. Extract the underlying
	// file handle and check the owner SID.
	if err := verifyPipeOwner(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("pipe ownership verification failed: %w", err)
	}

	return conn, nil
}

// allowSelfOwnedPipes additionally accepts a pipe owned by the current
// user. It is set only by this package's tests, which create their own
// listener in-process: an unelevated test binary owns its objects as the
// user SID, so every dial would otherwise be rejected. Unexported and
// false in every non-test build, so production always requires SY/BA.
//
// An elevated process owns its objects as BA (measured), which is why the
// helper's real pipe passes the production check.
var allowSelfOwnedPipes bool

// ownedByCurrentUser reports whether sid is this process's user SID.
func ownedByCurrentUser(sid *windows.SID) bool {
	tok := windows.GetCurrentProcessToken()
	u, err := tok.GetTokenUser()
	if err != nil {
		return false
	}
	return windows.EqualSid(sid, u.User.Sid)
}

// verifyPipeOwner checks that the named pipe is owned by Local System (SY)
// or Built-in Administrators (BA). This prevents a malicious process from
// creating a pipe with the same name before the legitimate helper starts.
func verifyPipeOwner(conn net.Conn) error {
	// The go-winio pipe connection embeds a *os.File. We need the Windows
	// handle to call GetSecurityInfo. Use a type assertion to get the
	// syscall handle.
	type handleGetter interface {
		Fd() uintptr
	}
	hg, ok := conn.(handleGetter)
	if !ok {
		// Fail closed: if we can't get the handle, reject the connection.
		// This could happen if go-winio changes its internal types.
		return fmt.Errorf("cannot extract file handle for pipe ownership verification")
	}
	handle := windows.Handle(hg.Fd())

	// Get the owner SID of the pipe.
	sd, err := windows.GetSecurityInfo(handle,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("GetSecurityInfo: %w", err)
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return fmt.Errorf("getting owner SID: %w", err)
	}
	if owner == nil {
		return fmt.Errorf("pipe has no owner SID")
	}

	// Check against well-known SIDs: Local System and Built-in Administrators.
	localSystem, _ := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	builtinAdmins, _ := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)

	if (localSystem != nil && windows.EqualSid(owner, localSystem)) ||
		(builtinAdmins != nil && windows.EqualSid(owner, builtinAdmins)) {
		return nil
	}

	if allowSelfOwnedPipes && ownedByCurrentUser(owner) {
		return nil
	}

	return fmt.Errorf("pipe owned by unexpected SID: %s", sidToString(owner))
}

func sidToString(sid *windows.SID) string {
	if sid == nil {
		return "<nil>"
	}
	// Use ConvertSidToStringSid via unsafe pointer to get the string form.
	var strSid *uint16
	err := windows.ConvertSidToStringSid(sid, &strSid)
	if err != nil {
		return "<unknown>"
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(strSid)))
	return windows.UTF16PtrToString(strSid)
}
