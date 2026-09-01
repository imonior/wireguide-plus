//go:build darwin

package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// installDarwinBundle performs the macOS in-place update:
//
//  1. Extract the downloaded asset — .dmg (mounted read-only) or .zip
//     (ditto-extracted) — into a private temp location.
//  2. Verify the bundle's code signature. The release CI ad-hoc signs the
//     bundle on macOS runners; a missing/invalid signature is a tamper
//     signal and refuses to install.
//  3. Resolve the installed bundle's location (darwinInstallTarget).
//  4. Run the install script, elevated via osascript when the target
//     directory is not user-writable (the standard macOS password prompt —
//     no TTY needed, same rationale as pkexec on Linux).
//
// The script kills the running app, replaces the bundle, strips the
// quarantine xattr and relaunches. If the target is the running bundle —
// the common case — this process does not survive the script, exactly like
// the brew cask postflight path.
func installDarwinBundle(assetPath string) error {
	appPath, cleanup, err := extractMacApp(assetPath)
	if err != nil {
		return err
	}
	defer cleanup()

	// The bundle must keep the canonical name — the kill/relaunch and the
	// install-target resolution all rely on it.
	if filepath.Base(appPath) != "wireguideplus.app" {
		return fmt.Errorf("unexpected app bundle name %q (expected wireguideplus.app)", filepath.Base(appPath))
	}
	if err := verifyBundleSignature(appPath); err != nil {
		return err
	}

	target, err := darwinInstallTarget()
	if err != nil {
		return err
	}

	scriptPath, err := writeInstallScript(darwinInstallScript(appPath, target))
	if err != nil {
		return fmt.Errorf("write install script: %w", err)
	}
	defer os.Remove(scriptPath)

	if !dirWritable(filepath.Dir(target)) {
		// /Applications and other system locations need root. osascript's
		// "with administrator privileges" pops the standard GUI password
		// dialog and runs the script as root — same UX as the Windows UAC
		// prompt and the Linux polkit dialog.
		cmd := exec.Command("osascript", "-e",
			"do shell script "+applescriptQuote(scriptPath)+" with administrator privileges")
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("elevated install failed: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		return nil
	}

	out, err := exec.Command("/bin/sh", scriptPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("install failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// extractMacApp unpacks the downloaded .dmg or .zip and returns the path
// to the .app bundle inside, plus a cleanup func that unmounts/removes
// the temp area. Callers must run cleanup (it is idempotent).
func extractMacApp(assetPath string) (string, func(), error) {
	switch strings.ToLower(filepath.Ext(assetPath)) {
	case ".dmg":
		return extractDmg(assetPath)
	case ".zip":
		return extractZip(assetPath)
	default:
		return "", nil, fmt.Errorf("unsupported macOS update asset %q: expected .dmg or .zip", assetPath)
	}
}

// extractDmg mounts the .dmg read-only at a private mount point and
// returns the app bundle found at its root.
func extractDmg(dmgPath string) (string, func(), error) {
	mountPoint, err := os.MkdirTemp("", "wireguideplus-update-*")
	if err != nil {
		return "", nil, err
	}
	// -nobrowse keeps the volume out of Finder; -readonly matches the
	// release's UDZO dmg; -mountpoint pins the mount to our temp dir so we
	// never have to parse "Disk X had N volumes" output or guess a /Volumes
	// path that may be localized.
	cmd := exec.Command("hdiutil", "attach", dmgPath,
		"-nobrowse", "-readonly", "-mountpoint", mountPoint)
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(mountPoint)
		return "", nil, fmt.Errorf("mount dmg: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	cleanup := func() {
		_ = exec.Command("hdiutil", "detach", mountPoint, "-quiet").Run()
		_ = os.RemoveAll(mountPoint)
	}
	appPath := findAppBundle(mountPoint)
	if appPath == "" {
		cleanup()
		return "", nil, fmt.Errorf("no .app bundle found inside the dmg")
	}
	return appPath, cleanup, nil
}

// extractZip unzips the archive with ditto (which preserves macOS
// metadata, unlike plain unzip) into a private temp directory and returns
// the app bundle at its root.
func extractZip(zipPath string) (string, func(), error) {
	dest, err := os.MkdirTemp("", "wireguideplus-update-*")
	if err != nil {
		return "", nil, err
	}
	cmd := exec.Command("ditto", "-x", "-k", zipPath, dest)
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(dest)
		return "", nil, fmt.Errorf("extract zip: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	cleanup := func() { _ = os.RemoveAll(dest) }
	appPath := findAppBundle(dest)
	if appPath == "" {
		cleanup()
		return "", nil, fmt.Errorf("no .app bundle found inside the zip")
	}
	return appPath, cleanup, nil
}

// verifyBundleSignature checks that the extracted bundle is properly code
// signed. Ad-hoc signatures (release CI: codesign --force --deep --sign -)
// pass; a missing or invalid signature means the bundle was tampered with
// after the download hash was computed, and must not be installed.
func verifyBundleSignature(appPath string) error {
	out, err := exec.Command("codesign", "--verify", "--deep", "--strict", "--verbose=2", appPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("code signature verification failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// writeInstallScript materializes the install script to a temp file so it
// can be handed to osascript / `sh` without quoting the whole body through
// another layer of escaping.
func writeInstallScript(content string) (string, error) {
	f, err := os.CreateTemp("", "wireguideplus-update-install-*.sh")
	if err != nil {
		return "", err
	}
	name := f.Name()
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		os.Remove(name)
		return "", err
	}
	if err := f.Chmod(0o755); err != nil {
		f.Close()
		os.Remove(name)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	return name, nil
}

// applescriptQuote escapes s as a double-quoted AppleScript string (used
// inside the osascript -e argument). CreateTemp paths never contain quotes
// or backslashes, but being defensive here keeps the escalation call safe
// for any future caller.
func applescriptQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
