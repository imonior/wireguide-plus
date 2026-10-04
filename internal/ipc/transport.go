package ipc

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
)

// ownerSIDPattern matches a numeric SID string (S-1-5-21-…-1001), the only
// shape that may be interpolated into the pipe's security descriptor. SDDL
// aliases such as IU (Interactive Users) or BA are rejected: the descriptor
// exists to name ONE user, and an alias names everybody.
var ownerSIDPattern = regexp.MustCompile(`^S-1-\d+(-\d+)+$`)

// broadPrincipals are the well-known SIDs whose numeric form the pattern above
// happily accepts — Everyone, Interactive Users, Authenticated Users, the
// built-in groups and the service accounts. None of them is ever a real user's
// token SID (that is S-1-5-21-… on every machine and domain account), and all
// of them stand for "more than one person", which is the issue #20 hole in a
// different spelling: the grant would reach other logged-on accounts instead
// of the GUI's own user.
var broadPrincipals = map[string]struct{}{
	"S-1-1-0":      {}, // Everyone
	"S-1-1-1":      {}, // Local
	"S-1-1-2":      {}, // Console Logon
	"S-1-5-4":      {}, // Interactive Users (IU)
	"S-1-5-6":      {}, // Network logon
	"S-1-5-7":      {}, // Authenticated Users
	"S-1-5-9":      {}, // Enterprise Domain Controllers
	"S-1-5-15":     {}, // External
	"S-1-5-18":     {}, // LOCAL SYSTEM
	"S-1-5-19":     {}, // LOCAL SERVICE
	"S-1-5-20":     {}, // NETWORK SERVICE
	"S-1-5-32-544": {}, // Built-in Administrators (BA)
	"S-1-5-32-545": {}, // Built-in Users
	"S-1-5-32-546": {}, // Built-in Guests
	"S-1-5-32-562": {}, // Remote Management Users
	"S-1-5-1000":   {}, // Other Organization
}

// ownerGrant returns the SDDL access block that lets the unprivileged GUI read
// and write the helper's pipe. It fails closed when there is no single owner
// to grant it to — the alternative was historically a grant to every
// logged-on account (issue #20).
//
// This lives in the portable file, not transport_windows.go, so the rule
// itself is testable on every platform's test runner; the Windows wire-up
// additionally parses the SID with windows.StringToSid before building the
// descriptor.
func ownerGrant(ownerSID string) (string, error) {
	if ownerSID == "" {
		return "", fmt.Errorf("no owner SID for the helper pipe: refusing to grant it to every interactive user")
	}
	if !ownerSIDPattern.MatchString(ownerSID) {
		return "", fmt.Errorf("invalid owner SID %q for the helper pipe", ownerSID)
	}
	// ownerSIDPattern is case-sensitive and requires the uppercase S-1-
	// prefix, so an SID that reached here is already in canonical form.
	if _, bad := broadPrincipals[ownerSID]; bad {
		return "", fmt.Errorf("owner SID %q is a shared principal, not a single user", ownerSID)
	}
	return "(A;;GRGW;;;" + ownerSID + ")", nil
}

// DefaultSocketPath returns the default socket/pipe address for this OS+user.
func DefaultSocketPath() string {
	switch runtime.GOOS {
	case "windows":
		// H14: Use a fixed well-known pipe name instead of deriving from the
		// USERNAME environment variable (which can be spoofed). Access control
		// is handled by the SDDL on the pipe itself, so the name does not
		// need to encode identity.
		return `\\.\pipe\wireguideplus`
	default:
		uid := os.Getuid()
		uidStr := strconv.Itoa(uid)

		// M18: Prefer $XDG_RUNTIME_DIR (typically /run/user/<uid>/) which is
		// a per-user tmpfs with restricted permissions.
		if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
			return filepath.Join(runtimeDir, "wireguideplus-"+uidStr+".sock")
		}

		if runtime.GOOS == "darwin" {
			// macOS: use /var/run/wireguideplus/ — the helper runs as root (via
			// LaunchDaemon or osascript) and creates this directory. The GUI
			// connects as an unprivileged user; the helper chowns the socket
			// so the GUI can read/write it. This path is stable across app
			// restarts and doesn't pollute the user's home directory.
			return "/var/run/wireguideplus/wireguideplus.sock"
		}

		// Linux fallback: create a private subdirectory under /tmp with mode 0700
		// so other users cannot place symlinks or interfere with the socket.
		dir := filepath.Join("/tmp", "wireguideplus-"+uidStr)
		if err := os.MkdirAll(dir, 0700); err != nil {
			slog.Error("failed to create IPC socket directory", "dir", dir, "error", err)
			return filepath.Join(dir, "wireguideplus.sock") // return best-effort path
		}
		// Ensure the directory has the correct permissions even if it already existed.
		if err := os.Chmod(dir, 0700); err != nil {
			slog.Warn("failed to set IPC socket directory permissions", "dir", dir, "error", err)
		}
		// Verify ownership to prevent an attacker from pre-creating the directory.
		// Returning a best-effort path on failure (instead of panicking)
		// keeps a hostile /tmp from killing the helper process at startup
		// — the subsequent listen will fail with a clear error which the
		// caller surfaces, rather than a stack trace. The Listen() path
		// also re-checks ownership before binding, so the socket can't
		// be hijacked even if this function returns a tainted path.
		if err := verifyDirOwnership(dir, uid); err != nil {
			slog.Error("IPC socket directory ownership check failed; subsequent Listen will fail with a clear error",
				"dir", dir, "error", err)
		}
		return filepath.Join(dir, "wireguideplus.sock")
	}
}
