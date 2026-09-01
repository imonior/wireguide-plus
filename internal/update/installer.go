package update

// CURRENTLY UNREFERENCED FROM PRODUCTION CODE — see the longer note on
// DownloadUpdate in checker.go. Kept (and fully tested) so that adding
// native Linux/Windows update flows later doesn't require re-implementing
// the install path from scratch.

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/imonior/wireguide-plus/internal/sysexec"
)

// Install runs the OS-specific installer for the downloaded update.
// The caller must pass the UpdateInfo whose HashVerified field was set by
// DownloadUpdate. Install refuses to proceed if the hash was not verified,
// and — in builds that require signed updates — if the Ed25519 signature
// was not verified. The latter re-check matters because Install execs the
// file: it must enforce the same policy as DownloadUpdate rather than
// trust that every (future) caller went through it.
func Install(filePath string, info *UpdateInfo) error {
	if info == nil || !info.HashVerified {
		return fmt.Errorf("refusing to install: checksum was not verified")
	}
	if requireSignedUpdates && !info.SignatureVerified {
		return fmt.Errorf("refusing to install: signature was not verified (this build requires signed updates)")
	}
	switch runtime.GOOS {
	case "darwin":
		return installDarwin(filePath)
	case "linux":
		return installLinux(filePath)
	case "windows":
		return installWindows(filePath)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func installDarwin(path string) error {
	// For non-brew installs, open the GitHub releases page in the browser
	// instead of trying to auto-replace the app bundle (which would need
	// sudo and has many failure modes). The user downloads and replaces
	// the app manually — same UX as most indie macOS apps.
	return exec.Command("open", "https://github.com/imonior/wireguide-plus/releases/latest").Run()
}

func installLinux(path string) error {
	// Try dpkg for .deb — use pkexec instead of sudo (works with GUI, no TTY needed)
	if len(path) > 4 && path[len(path)-4:] == ".deb" {
		return exec.Command("pkexec", "dpkg", "-i", path).Run()
	}
	// Try rpm for .rpm — use pkexec for the same reason
	if len(path) > 4 && path[len(path)-4:] == ".rpm" {
		return exec.Command("pkexec", "rpm", "-U", path).Run()
	}
	// AppImage — make executable and run
	if err := exec.Command("chmod", "+x", path).Run(); err != nil {
		return fmt.Errorf("chmod +x: %w", err)
	}
	cmd := exec.Command(path)
	if err := cmd.Start(); err != nil {
		return err
	}
	// Release the process so it doesn't become a zombie when the parent exits.
	return cmd.Process.Release()
}

func installWindows(path string) error {
	// MSI: silent install; msiexec itself needs admin rights.
	if len(path) > 4 && strings.EqualFold(path[len(path)-4:], ".msi") {
		return runInstallerElevated("msiexec", "/i", path, "/qn")
	}
	// NSIS .exe installer: request elevation and run silently so the user
	// only sees the UAC prompt. A direct exec.Command from a non-elevated
	// process fails with ERROR_ELEVATION_REQUIRED because the installer's
	// manifest requests admin rights (it writes to Program Files).
	return runInstallerElevated(path, "/S")
}

// runInstallerElevated launches the installer via PowerShell's
// Start-Process -Verb RunAs. This triggers the Windows UAC elevation prompt
// when the parent process is not already elevated, and is a no-op prompt
// when it already is. The installer is detached so the parent can exit.
func runInstallerElevated(path string, args ...string) error {
	// Pass the target path as $args[0] to avoid PowerShell quoting pitfalls
	// for paths that may contain single quotes.
	quoted := make([]string, 0, len(args))
	for _, a := range args {
		// Escape single quotes by doubling them; Windows paths cannot contain
		// double quotes, so single-quoting each argument is safe.
		quoted = append(quoted, "'"+strings.ReplaceAll(a, "'", "''")+"'")
	}
	argList := strings.Join(quoted, ",")
	script := fmt.Sprintf("Start-Process -Verb RunAs -FilePath $args[0] -ArgumentList %s", argList)
	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-Command", script, path)
	sysexec.Hide(cmd)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start installer with administrator rights: %w", err)
	}
	return nil
}
