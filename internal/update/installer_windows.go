//go:build windows

package update

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func installWindows(path string) error {
	// Copy the downloaded installer to a persistent location before launching
	// it. The caller removes the temp download as soon as Install returns, but
	// ShellExecute("runas") returns immediately after requesting elevation —
	// the UAC dialog may still be shown and the installer has not yet locked
	// the file. If the temp file is deleted in that window, the elevated
	// process fails to start.
	persistentPath, err := stageInstaller(path)
	if err != nil {
		return fmt.Errorf("stage installer: %w", err)
	}

	// MSI: silent install; msiexec itself needs admin rights. An MSI upgrade
	// keeps the original install location (keyed by ProductCode), so no
	// directory pinning is needed here.
	if len(path) > 4 && strings.EqualFold(path[len(path)-4:], ".msi") {
		return runInstallerElevated("msiexec", "/i", persistentPath, "/qn")
	}
	// NSIS .exe installer: request elevation (UAC) and run silently so the
	// user only sees the UAC prompt. A direct exec.Command from a
	// non-elevated process fails with ERROR_ELEVATION_REQUIRED because the
	// installer's manifest requests admin rights (it writes to Program
	// Files). /AUTOSTART makes the installer relaunch the app after the
	// swap — its silent mode skips the finish page whose "run now" checkbox
	// would otherwise be the only way to open the app (project.nsi starts
	// it via explorer.exe so it runs with the user's token).
	//
	// /D pins the install directory to the folder the currently running
	// (pre-update) binary lives in. project.nsi declares a fixed default
	// InstallDir and does NOT remember a custom location, so without /D a
	// silent upgrade of a user who picked a custom folder would install a
	// SECOND copy into Program Files instead of overwriting the original.
	// NSIS requires /D to be the LAST argument and its value must not be
	// quoted; paths with spaces are fine (NSIS takes the whole tail).
	args := []string{"/S", "/AUTOSTART"}
	if dir := currentInstallDir(); dir != "" {
		args = append(args, "/D="+dir)
	}
	return runInstallerElevated(persistentPath, args...)
}

// currentInstallDir returns the directory containing the currently running
// executable — the folder the existing installation lives in. It anchors the
// /D= flag so an in-app upgrade always overwrites in place. The executable
// path is normally the installed copy; in the unlikely case it is a portable
// copy launched from a scratch folder, /D points there and the upgrade
// mirrors where the app was launched from, which is still "not a second
// copy in Program Files".
func currentInstallDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(exe)
}

// stageInstaller copies the installer from its temp location to
// %LOCALAPPDATA%\wireguideplus\updates so it survives the caller's cleanup.
// It also removes stale installers from previous update attempts.
func stageInstaller(src string) (string, error) {
	dir := filepath.Join(os.Getenv("LOCALAPPDATA"), "wireguideplus", "updates")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create updates dir: %w", err)
	}

	stale, _ := filepath.Glob(filepath.Join(dir, "wireguideplus-update-installer.*"))
	for _, f := range stale {
		_ = os.Remove(f)
	}

	ext := filepath.Ext(src)
	if ext == "" {
		ext = ".exe"
	}
	dest := filepath.Join(dir, "wireguideplus-update-installer"+ext)
	if err := copyFile(src, dest); err != nil {
		return "", fmt.Errorf("copy installer: %w", err)
	}
	return dest, nil
}

// runInstallerElevated launches the installer via the Windows shell "runas"
// verb. This triggers the UAC elevation prompt when the parent process is not
// already elevated, and is a no-op prompt when it already is. The installer is
// detached so the parent can exit.
func runInstallerElevated(path string, args ...string) error {
	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return fmt.Errorf("encode verb: %w", err)
	}
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("encode path: %w", err)
	}

	var params *uint16
	if len(args) > 0 {
		p, err := windows.UTF16PtrFromString(strings.Join(args, " "))
		if err != nil {
			return fmt.Errorf("encode arguments: %w", err)
		}
		params = p
	}

	if err := windows.ShellExecute(0, verb, file, params, nil, windows.SW_SHOWNORMAL); err != nil {
		return fmt.Errorf("failed to start installer with administrator rights: %w", describeElevationError(err))
	}
	return nil
}

func describeElevationError(err error) error {
	var errno windows.Errno
	if !errors.As(err, &errno) {
		return err
	}
	switch errno {
	case windows.ERROR_CANCELLED:
		return errors.New("user cancelled the UAC prompt")
	case windows.ERROR_FILE_NOT_FOUND:
		return errors.New("installer file not found")
	case windows.ERROR_PATH_NOT_FOUND:
		return errors.New("installer directory not found")
	case windows.ERROR_ACCESS_DENIED:
		return errors.New("access denied while requesting elevation")
	}
	return err
}
