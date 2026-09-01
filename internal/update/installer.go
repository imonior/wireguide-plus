package update

// Install / installLinux / installWindows are used by the native
// auto-update flow (RunUpdate in internal/app/settings_ops.go): the app
// downloads the release asset with DownloadUpdateProgress, then hands the
// verified file to Install, which dispatches to the platform installer.

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
	// Copy the downloaded asset to a persistent location first. The caller
	// removes the temp download as soon as Install returns, and AppImage
	// launches asynchronously — deleting the file in that window can break
	// the launch, exactly like the Windows installer race fixed in 1.3.7.
	// dpkg/rpm also read from this path (harmless, just consistent).
	persistentPath, err := stageLinuxInstaller(path)
	if err != nil {
		return fmt.Errorf("stage installer: %w", err)
	}

	// Match extensions case-insensitively and via the true extension, not a
	// slice of the last 4 bytes: .deb / .rpm / .AppImage all differ in length.
	switch strings.ToLower(filepath.Ext(persistentPath)) {
	case ".deb":
		return runPkexec("dpkg", "-i", persistentPath)
	case ".rpm":
		return runPkexec("rpm", "-U", persistentPath)
	case ".appimage":
		if err := exec.Command("chmod", "+x", persistentPath).Run(); err != nil {
			return fmt.Errorf("chmod +x: %w", err)
		}
		cmd := exec.Command(persistentPath)
		if err := cmd.Start(); err != nil {
			return err
		}
		// Release the process so it doesn't become a zombie when the parent exits.
		return cmd.Process.Release()
	default:
		// Unknown format — fail loudly so the caller falls back to the
		// release page instead of trying to execute a tar/zip as a program.
		return fmt.Errorf("unsupported update asset format %q: expected .deb, .rpm or .AppImage", filepath.Ext(path))
	}
}

// runPkexec runs a package-manager command through pkexec (a GUI polkit
// prompt — works without a TTY, unlike sudo). The combined output is kept so
// the error message says why elevation failed (e.g. no polkit agent running).
func runPkexec(prog string, args ...string) error {
	cmd := exec.Command("pkexec", append([]string{prog}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s via pkexec failed: %w (output: %s)",
			prog, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// stageLinuxInstaller copies the update asset to a persistent location
// ($XDG_DATA_HOME/wireguideplus/updates, falling back to ~/.local/share)
// so the caller can safely remove the temp download right after Install
// returns. Stale installers from previous attempts are removed first.
func stageLinuxInstaller(src string) (string, error) {
	dir := linuxUpdatesDir()
	if dir == "" {
		return "", fmt.Errorf("cannot resolve a persistent updates directory")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create updates dir: %w", err)
	}
	stale, _ := filepath.Glob(filepath.Join(dir, "wireguideplus-update-installer.*"))
	for _, f := range stale {
		_ = os.Remove(f)
	}
	ext := filepath.Ext(src)
	if ext == "" {
		ext = ".bin"
	}
	dest := filepath.Join(dir, "wireguideplus-update-installer"+ext)
	if err := copyFile(src, dest); err != nil {
		return "", fmt.Errorf("copy installer: %w", err)
	}
	return dest, nil
}

func linuxUpdatesDir() string {
	if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
		return filepath.Join(dataHome, "wireguideplus", "updates")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "wireguideplus", "updates")
}

// copyFile copies src to dst (mode 0644). Shared by the Windows and Linux
// staging paths.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

