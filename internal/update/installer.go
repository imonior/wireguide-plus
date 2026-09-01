package update

// Install / installDarwin / installLinux / installWindows are used by the
// native auto-update flow (RunUpdate in internal/app/settings_ops.go): the
// app downloads the release asset with DownloadUpdateProgress, then hands
// the verified file to Install, which dispatches to the platform installer.

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// InstallOptions controls how Install runs the platform installer.
type InstallOptions struct {
	// Silent installs without the interactive installer UI. Used by the
	// "auto silent update" setting (Settings → Updates); the default
	// (false) launches the regular installer the user already knows from a
	// manual install — on Windows the NSIS wizard, whose finish page offers
	// to launch the app. macOS in-place updates are inherently quiet, so
	// the flag mainly drives Windows.
	Silent bool
}

// Install runs the OS-specific installer for the downloaded update.
// The caller must pass the UpdateInfo whose HashVerified field was set by
// DownloadUpdate. Install refuses to proceed if the hash was not verified,
// and — in builds that require signed updates — if the Ed25519 signature
// was not verified. The latter re-check matters because Install execs the
// file: it must enforce the same policy as DownloadUpdate rather than
// trust that every (future) caller went through it.
func Install(filePath string, info *UpdateInfo, opts InstallOptions) error {
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
		return installLinux(filePath, opts.Silent)
	case "windows":
		return installWindows(filePath, opts.Silent)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func installDarwin(path string) error {
	// Non-brew macOS installs now update in place: the downloaded asset (a
	// .dmg or .zip containing wireguideplus.app) is mounted/extracted, its
	// code signature verified, and the running bundle replaced by an
	// (elevated, if needed) install script that kills, swaps, de-quarantines
	// and relaunches the app — the same "Update now" UX as Windows/Linux.
	// Implementation lives in installer_darwin.go.
	return installDarwinBundle(path)
}

func installLinux(path string, silent bool) error {
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
		if err := runPkexec("dpkg", "-i", persistentPath); err != nil {
			return err
		}
		// Package-manager updates replace the binary on disk but never
		// start the GUI, so launch the fresh version ourselves. silent has
		// no effect here: pkexec's polkit dialog is the interactive step,
		// identical for both modes.
		return relaunchLinuxApp()
	case ".rpm":
		if err := runPkexec("rpm", "-U", persistentPath); err != nil {
			return err
		}
		return relaunchLinuxApp()
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

// relaunchLinuxApp starts the freshly installed GUI after a deb/rpm update.
//
// The updater's own process is still running the OLD binary — Linux lets
// dpkg/rpm overwrite a live executable — so the app must be restarted to
// pick up the new version. We render a tiny shell script and run it
// detached: the script kills every running instance (this process
// included, exactly like the macOS install script and the Windows
// installer's taskkill), then starts the new binary from PATH. Running the
// launch through a separate shell keeps it alive even though the calling
// process dies in the pkill. The nfpm package ships the binary as
// /usr/local/bin/wireguideplus, which is on PATH for most setups.
func relaunchLinuxApp() error {
	// The nfpm package ships /usr/local/bin/wireguideplus; check that first
	// because a desktop session's PATH may not include /usr/local/bin.
	bin := "/usr/local/bin/wireguideplus"
	if _, err := os.Stat(bin); err != nil {
		if resolved, lookErr := exec.LookPath("wireguideplus"); lookErr == nil {
			bin = resolved
		} else {
			// Installed fine but the binary is nowhere findable (unusual).
			// Not a reason to fail the update — the user can launch it by hand.
			return nil
		}
	}
	script := "#!/bin/sh\n" +
		"pkill -x wireguideplus 2>/dev/null || true\n" +
		"/bin/sleep 1\n" +
		"nohup " + shellQuote(bin) + " >/dev/null 2>&1 &\n"
	f, err := os.CreateTemp("", "wireguideplus-relaunch-*.sh")
	if err != nil {
		return err
	}
	name := f.Name()
	if _, err := f.WriteString(script); err != nil {
		f.Close()
		_ = os.Remove(name)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	_ = os.Chmod(name, 0o755)
	cmd := exec.Command("/bin/sh", name)
	if err := cmd.Start(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return cmd.Process.Release()
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

// --- macOS in-place install helpers (platform-neutral) -----------------
//
// These functions are pure logic (no macOS-only syscalls), so they live in
// the shared file and are unit-testable on any platform. The OS commands
// that act on them (hdiutil / ditto / codesign / osascript) live in
// installer_darwin.go.

// darwinBundleDirFromExecutable returns the absolute path of the .app
// bundle the current executable is running from, or "" when the executable
// is not inside a *.app (a bare dev binary, a symlinked launcher).
//
// macOS updates replace the bundle "in place": wherever the user launched
// the app from — /Applications, ~/Applications, or a scratch folder — the
// new version lands in the same spot, so the user's dock icon / Finder
// placement survives the upgrade.
func darwinBundleDirFromExecutable() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	// .../Foo.app/Contents/MacOS/<binary>  →  three levels up is the bundle.
	appDir := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	if strings.HasSuffix(appDir, ".app") {
		return appDir
	}
	return ""
}

// darwinInstallTarget decides where the freshly downloaded bundle goes:
// the bundle the running executable lives in first (most accurate), else
// the first existing standard location (user-owned ~/Applications
// preferred over /Applications), else /Applications (the default for a
// fresh install — the installer elevates when needed).
func darwinInstallTarget() (string, error) {
	if dir := darwinBundleDirFromExecutable(); dir != "" {
		return dir, nil
	}
	home, _ := os.UserHomeDir()
	for _, base := range []string{
		filepath.Join(home, "Applications"),
		"/Applications",
	} {
		p := filepath.Join(base, "wireguideplus.app")
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p, nil
		}
	}
	return "/Applications/wireguideplus.app", nil
}

// dirWritable reports whether dir allows creating files as the current
// user, by attempting an actual create+remove. Go has no os.Access, and
// probing beats guessing from stat permission bits, which are unreliable
// for ACLs, read-only mounts and root ownership.
func dirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".wireguideplus-write-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	_ = os.Remove(name)
	return true
}

// findAppBundle returns the path of the first *.app directory inside dir,
// or "" if there is none. The release .dmg/.zip are created with the app
// at the archive root (release.yml: hdiutil -srcfolder wireguideplus.app /
// ditto --keepParent), so a top-level scan is sufficient.
func findAppBundle(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), ".app") {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

// shellQuote wraps s in single quotes so it is safe as one POSIX shell
// word. Embedded single quotes are closed and re-opened ('\'').
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// darwinInstallScript renders the POSIX shell script that installs newApp
// over targetApp. It is written to a temp file and executed either directly
// (user-writable target) or via osascript "with administrator privileges"
// (root-owned targets like /Applications).
//
// The script must be self-contained because it kills the running GUI
// process (killall wireguideplus) as part of the swap — every step that has
// to happen after the kill (copy, de-quarantine, relaunch) lives inside the
// script rather than back in the Go process.
func darwinInstallScript(newApp, targetApp string) string {
	q := shellQuote
	return strings.Join([]string{
		"#!/bin/sh",
		"set -e",
		// Stop the running app; a not-running / already-dead app is fine.
		"/usr/bin/killall wireguideplus 2>/dev/null || true",
		"/bin/sleep 1",
		// Remove the old bundle, then copy the new one with ditto (which
		// preserves metadata better than plain cp).
		"/bin/rm -rf " + q(targetApp),
		"/usr/bin/ditto " + q(newApp) + " " + q(targetApp),
		// The in-process download carries no quarantine xattr (it arrived
		// over HTTP, not a browser), but stripping it is idempotent and
		// matches the brew cask postflight — belt and braces against
		// Gatekeeper on the fresh copy.
		"/usr/bin/xattr -dr com.apple.quarantine " + q(targetApp) + " 2>/dev/null || true",
		// Relaunch as the logged-in GUI user, not as root: when the script
		// runs elevated (osascript), a bare `open` would start the app as
		// root. launchctl asuser hands it to the console user's session.
		"OPEN_USER=$(/usr/bin/stat -f '%Su' /dev/console 2>/dev/null || /usr/bin/whoami)",
		"if [ -n \"$OPEN_USER\" ] && [ \"$OPEN_USER\" != \"root\" ]; then",
		"  /bin/launchctl asuser \"$(/usr/bin/id -u \"$OPEN_USER\")\" /usr/bin/open " + q(targetApp),
		"else",
		"  /usr/bin/open " + q(targetApp),
		"fi",
		"",
	}, "\n")
}

