package storage

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
)

const appName = "wireguideplus"

// canWriteDir tests whether the current process can create files in dir by
// writing and immediately removing a temp file. Used by EnsureDirs to
// distinguish "can't chmod but can still use" from "truly inaccessible".
func canWriteDir(dir string) bool {
	tmp := filepath.Join(dir, ".wireguide-write-test")
	f, err := os.Create(tmp)
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(tmp)
	return true
}

// Paths holds all OS-specific directory paths for the application.
type Paths struct {
	ConfigDir  string // App settings (config.json)
	TunnelsDir string // .conf files
	LogsDir    string // Log files
	DataDir    string // Daemon state / recovery journal (system-level)
}

// GetPaths returns OS-specific paths for the application.
func GetPaths() (*Paths, error) {
	var p Paths

	switch runtime.GOOS { //nolint:exhaustive
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		appSupport := filepath.Join(home, "Library", "Application Support", appName)
		p.ConfigDir = appSupport
		p.TunnelsDir = filepath.Join(appSupport, "tunnels")
		p.LogsDir = filepath.Join(home, "Library", "Logs", appName)
		p.DataDir = filepath.Join("/Library", "Application Support", appName)

	case "linux":
		configHome := os.Getenv("XDG_CONFIG_HOME")
		if configHome == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}
			configHome = filepath.Join(home, ".config")
		}
		p.ConfigDir = filepath.Join(configHome, appName)
		p.TunnelsDir = filepath.Join(configHome, appName, "tunnels")

		dataHome := os.Getenv("XDG_DATA_HOME")
		if dataHome == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}
			dataHome = filepath.Join(home, ".local", "share")
		}
		p.LogsDir = filepath.Join(dataHome, appName, "logs")
		p.DataDir = filepath.Join("/var", "lib", appName)

	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		p.ConfigDir = filepath.Join(appData, appName)
		p.TunnelsDir = filepath.Join(appData, appName, "tunnels")
		p.LogsDir = filepath.Join(appData, appName, "logs")

		programData := os.Getenv("PROGRAMDATA")
		if programData == "" {
			programData = `C:\ProgramData`
		}
		p.DataDir = filepath.Join(programData, appName)

	default:
		return nil, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	// NOTE: legacy "wireguide" data is NOT migrated here anymore. Pre-rename
	// builds auto-copied it when the new directory was empty; that silent
	// behaviour was removed in favour of an interactive prompt (see
	// legacy.go) so the user decides whether to migrate configs, tunnels and
	// logs, how to handle conflicts, and can compare the old/new folders
	// first. The GUI calls DetectLegacyData/MigrateLegacyData at startup.

	return &p, nil
}

// legacyAppName is the config-directory name used before the rename.
const legacyAppName = "wireguide"

// legacyConfigDir returns the pre-rename config directory for this OS.
func legacyConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", legacyAppName)
	case "linux":
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			return filepath.Join(xdg, legacyAppName)
		}
		return filepath.Join(home, ".config", legacyAppName)
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, legacyAppName)
		}
		return filepath.Join(home, "AppData", "Roaming", legacyAppName)
	}
	return ""
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// EnsureDirs creates all necessary directories if they don't exist.
// ConfigDir and TunnelsDir use 0700 to prevent other users from listing
// config filenames on multi-user systems. LogsDir and DataDir use 0700 as well.
//
// DataDir may require root permissions (e.g. /var/lib/wireguideplus on Linux,
// /Library/Application Support/wireguideplus on macOS). If creation fails due
// to insufficient privileges, the error is logged as a warning instead of
// failing the entire startup — the helper process will create it when running
// as root.
func (p *Paths) EnsureDirs() error {
	userDirs := []string{p.ConfigDir, p.TunnelsDir, p.LogsDir}
	for _, dir := range userDirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		// Enforce permissions even if the directory already existed with
		// wider permissions (e.g., 0755 from a previous version).
		if err := os.Chmod(dir, 0700); err != nil {
			// Chmod fails when the directory is owned by another user (e.g.
			// root created it during a previous helper spawn). As long as
			// we can actually write to it, proceed with a warning — crashing
			// the entire app over a permission tightening failure on a
			// directory we can still use is worse than running with 0755.
			if canWriteDir(dir) {
				slog.Warn("cannot tighten dir permissions (owned by another user)",
					"dir", dir, "error", err)
			} else {
				return fmt.Errorf("directory %s exists but is not writable: %w", dir, err)
			}
		}
	}
	// DataDir may require elevated privileges; warn instead of failing.
	if p.DataDir != "" {
		if err := os.MkdirAll(p.DataDir, 0700); err != nil {
			slog.Warn("cannot create DataDir (may need root)", "dir", p.DataDir, "error", err)
		} else if err := os.Chmod(p.DataDir, 0700); err != nil {
			slog.Warn("cannot set DataDir permissions", "dir", p.DataDir, "error", err)
		}
	}
	return nil
}
