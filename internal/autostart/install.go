package autostart

import (
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/imonior/wireguide-plus/internal/sysexec"
)

// InstallAutostart sets up OS-level autostart for the GUI app.
func InstallAutostart(appPath string) error {
	switch runtime.GOOS {
	case "darwin":
		return installMacAutostart(appPath)
	case "linux":
		return installLinuxAutostart(appPath)
	case "windows":
		return installWindowsAutostart(appPath)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// RemoveAutostart removes OS-level autostart.
func RemoveAutostart() error {
	switch runtime.GOOS {
	case "darwin":
		return removeMacAutostart()
	case "linux":
		return removeLinuxAutostart()
	case "windows":
		return removeWindowsAutostart()
	default:
		return nil
	}
}

// --- macOS: LaunchAgent ---

// currentHome resolves the home directory for the plist we are about to
// write. Unlike the GUI's own path resolution this always runs inside the
// app (an interactive session), so $HOME is normally present; the user-
// database fallback only matters if it ever isn't.
func currentHome() (string, error) {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home, nil
	}
	if u, err := user.Current(); err == nil && u.HomeDir != "" {
		return u.HomeDir, nil
	}
	return "", fmt.Errorf("cannot determine home directory ($HOME is not defined)")
}

// escapeXML returns s escaped for embedding in a plist <string>, refusing
// to continue if the escaper itself fails (see installMacAutostart).
func escapeXML(s string) (string, error) {
	var b strings.Builder
	if err := xml.EscapeText(&b, []byte(s)); err != nil {
		return "", err
	}
	return b.String(), nil
}

func installMacAutostart(appPath string) error {
	home, err := currentHome()
	if err != nil {
		return err
	}
	plistDir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(plistDir, 0755); err != nil {
		return fmt.Errorf("creating LaunchAgents dir: %w", err)
	}

	// XML-escape both values to prevent plist injection from special
	// characters. xml.EscapeText returning an error means the escaping
	// itself failed (extremely rare — only from the io.Writer surface)
	// and the buffer may contain partial unescaped bytes. We MUST refuse
	// to write the plist in that case, otherwise an attacker who controls
	// the path could inject `</string>...<key>...` and modify our plist.
	safeAppPath, err := escapeXML(appPath)
	if err != nil {
		return fmt.Errorf("xml-escape app path: %w", err)
	}
	safeHome, err := escapeXML(home)
	if err != nil {
		return fmt.Errorf("xml-escape home: %w", err)
	}

	// Notes on the keys below:
	//
	// EnvironmentVariables/HOME — launchd hands LaunchAgents a minimal
	// environment (PATH and little else): **$HOME is not set**. The GUI
	// resolves its config/log/tunnel directories from it, so without this
	// the app was launched at login and died immediately with
	// "paths: $HOME is not defined" — the "Launch at startup" switch
	// looked like it did nothing.
	//
	// LimitLoadToSessionType/ProcessType — confine the job to graphical
	// sessions and run it as an interactive process so the Wails/Cocoa
	// window server context is a normal one instead of a daemon spawn.
	//
	// RunAtLoad only: no KeepAlive. A restart loop would fight the user
	// when they quit the app on purpose.
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.wireguideplus.gui</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>EnvironmentVariables</key>
    <dict>
        <key>HOME</key>
        <string>%s</string>
    </dict>
    <key>LimitLoadToSessionType</key>
    <string>Aqua</string>
    <key>ProcessType</key>
    <string>Interactive</string>
</dict>
</plist>
`, safeAppPath, safeHome)

	return os.WriteFile(filepath.Join(plistDir, "com.wireguideplus.gui.plist"), []byte(plist), 0644)
}

func removeMacAutostart() error {
	home, err := currentHome()
	if err != nil {
		return err
	}
	// Unload first so launchd forgets the job, then remove the file.
	// Remove the current plist, plus the pre-plus "com.wireguide.gui.plist"
	// left behind by an older install, so upgrades don't orphan a launch item.
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	for _, name := range []string{"com.wireguideplus.gui.plist", "com.wireguide.gui.plist"} {
		label := strings.TrimSuffix(name, ".plist")
		// Non-fatal: the job is usually not loaded, and a missing
		// plist is exactly the end state we want anyway.
		_ = exec.Command("launchctl", "bootout", domain+"/"+label).Run()
		_ = os.Remove(filepath.Join(home, "Library", "LaunchAgents", name))
	}
	return nil
}

// --- Linux: XDG autostart ---

func installLinuxAutostart(appPath string) error {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot determine home directory: %w", err)
		}
		configHome = filepath.Join(home, ".config")
	}
	autostartDir := filepath.Join(configHome, "autostart")
	if err := os.MkdirAll(autostartDir, 0755); err != nil {
		return fmt.Errorf("creating autostart dir: %w", err)
	}

	// Quote the Exec path per Desktop Entry Spec to handle spaces/special chars.
	quotedPath := `"` + strings.ReplaceAll(appPath, `"`, `\"`) + `"`
	desktop := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=WireGuide Plus
Exec=%s
Icon=wireguideplus
Terminal=false
StartupNotify=false
X-GNOME-Autostart-enabled=true
`, quotedPath)

	return os.WriteFile(filepath.Join(autostartDir, "wireguideplus.desktop"), []byte(desktop), 0644)
}

func removeLinuxAutostart() error {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, _ := os.UserHomeDir()
		configHome = filepath.Join(home, ".config")
	}
	// Remove the current desktop file, plus the pre-plus "wireguide.desktop"
	// an older install may have left behind.
	for _, name := range []string{"wireguideplus.desktop", "wireguide.desktop"} {
		_ = os.Remove(filepath.Join(configHome, "autostart", name))
	}
	return nil
}

// --- Windows: Registry Run key ---

func installWindowsAutostart(appPath string) error {
	// M15: Wrap the path in quotes so spaces in the path are handled correctly
	// by the Windows shell when the registry value is used to launch the app.
	quotedPath := `"` + appPath + `"`
	cmd := exec.Command("reg", "add",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
		"/v", "WireGuidePlus", "/t", "REG_SZ", "/d", quotedPath, "/f")
	sysexec.Hide(cmd)
	return cmd.Run()
}

func removeWindowsAutostart() error {
	// Delete the current value name plus the pre-plus "WireGuide" value an
	// older install may have created, so upgrades don't leave a stale entry.
	for _, name := range []string{"WireGuidePlus", "WireGuide"} {
		cmd := exec.Command("reg", "delete",
			`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
			"/v", name, "/f")
		sysexec.Hide(cmd)
		out, err := cmd.CombinedOutput()
		if err != nil && !strings.Contains(string(out), "not found") {
			return err
		}
	}
	return nil
}
