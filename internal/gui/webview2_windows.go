//go:build windows

package gui

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// webview2ClientGUID is the EdgeUpdate client GUID for the WebView2
// Evergreen runtime. Its "pv" (version) value is written to the registry
// when the runtime is installed — system-wide or per-user — which is the
// same probe NSIS ran before this app stopped bundling the runtime.
const webview2ClientGUID = "{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"

// Registry view flags. WOW64_32KEY redirects a path under Software\ to
// Software\WOW6432Node\, WOW64_64KEY forces the native 64-bit view. Neither
// is exported by golang.org/x/sys/windows/registry, so we spell them out.
const (
	wow64_32key = 0x0200
	wow64_64key = 0x0100
)

// webview2InstallDirs are the on-disk locations of the Evergreen runtime.
// Used as a belt-and-braces fallback for the (rare) case where the registry
// key is unreadable but the runtime is plainly installed.
var webview2InstallDirs = []string{
	`C:\Program Files (x86)\Microsoft\EdgeWebView\Application`,
	`C:\Program Files\Microsoft\EdgeWebView\Application`,
}

// webview2DownloadURL is Microsoft's canonical Evergreen bootstrapper
// download link (the same one Wails bundles as the webview2 setup exe).
const webview2DownloadURL = "https://go.microsoft.com/fwlink/p/?LinkId=2124703"

// ensureWebView2 aborts the launch when the WebView2 runtime is missing.
// Wails renders the whole UI inside a WebView2 webview, so without the
// runtime app.Run() fails before any window appears. Instead of letting it
// fail cryptically, show a native dialog with the download link and exit.
func ensureWebView2() {
	if webview2Installed() {
		return
	}
	if showWebView2MissingDialog() {
		// Open the Microsoft download page in the default browser. The
		// Wails app isn't up yet, so call ShellExecute directly.
		if url, err := windows.UTF16PtrFromString(webview2DownloadURL); err == nil {
			_ = windows.ShellExecute(0, nil, url, nil, nil, windows.SW_SHOWNORMAL)
		}
	}
	os.Exit(1)
}

// webview2Installed reports whether the WebView2 Evergreen runtime is
// present.
//
// We must probe BOTH the 32- and 64-bit registry views, not just the native
// one: the Evergreen *Runtime* installer is 32-bit, so it registers its
// EdgeUpdate client key under
//
//	HKLM\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{GUID}
//
// on 64-bit Windows. A 64-bit build reading the native view
// (HKLM\SOFTWARE\Microsoft\EdgeUpdate\Clients\{GUID}) therefore finds
// nothing and wrongly concludes the runtime is absent — the bug that made
// 2.3.0 pop a bogus "WebView2 not installed" download prompt on machines
// where it was installed all along. Per-user installs write to HKCU, so we
// check that hive too. As a final fallback we look for the runtime's
// on-disk install directory.
func webview2Installed() bool {
	const sub = `Software\Microsoft\EdgeUpdate\Clients\` + webview2ClientGUID
	probes := []struct {
		root   registry.Key
		access uint32
	}{
		{registry.LOCAL_MACHINE, registry.READ | wow64_32key},
		{registry.LOCAL_MACHINE, registry.READ | wow64_64key},
		{registry.CURRENT_USER, registry.READ | wow64_32key},
		{registry.CURRENT_USER, registry.READ | wow64_64key},
	}
	for _, p := range probes {
		key, err := registry.OpenKey(p.root, sub, p.access)
		if err != nil {
			continue
		}
		v, _, err := key.GetStringValue("pv")
		key.Close()
		if err == nil && v != "" {
			return true
		}
	}
	return webview2RuntimeOnDisk()
}

// webview2RuntimeOnDisk returns true when a versioned runtime folder
// containing msedgewebview2.exe exists under either install root.
func webview2RuntimeOnDisk() bool {
	for _, dir := range webview2InstallDirs {
		matches, _ := filepath.Glob(filepath.Join(dir, "*", "msedgewebview2.exe"))
		if len(matches) > 0 {
			return true
		}
	}
	return false
}

// showWebView2MissingDialog shows a native message box. It returns true if
// the user chose to open the download page (OK), false if they cancelled.
// The Wails app isn't running yet, so this must be a raw user32 call.
func showWebView2MissingDialog() bool {
	const idOK = 1 // IDOK — not exported by x/sys/windows
	const style = windows.MB_OKCANCEL | windows.MB_ICONINFORMATION |
		windows.MB_SETFOREGROUND | windows.MB_TOPMOST
	text, err := windows.UTF16PtrFromString(
		"WireGuide needs the Microsoft WebView2 Runtime to display its window.\n\n" +
			"It is not installed on this computer.\n\n" +
			"Download it from:\n" + webview2DownloadURL + "\n\n" +
			"Click OK to open the download page, then restart WireGuide.")
	if err != nil {
		return false
	}
	caption, err := windows.UTF16PtrFromString("WireGuide")
	if err != nil {
		return false
	}
	ret, _ := windows.MessageBox(0, text, caption, style)
	return ret == idOK
}

// looksLikeWebView2Failure reports whether err names WebView2 or the
// missing-runtime HRESULT. Kept deliberately narrow: matching every
// "file not found" would attribute unrelated start-up failures to WebView2
// and raise a bogus download prompt — the exact false positive this gate
// exists to avoid.
func looksLikeWebView2Failure(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{
		"webview2",
		"webview",
		"0x80070002", // HRESULT_FROM_WIN32(ERROR_FILE_NOT_FOUND)
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// reportWebView2RunFailure is the post-launch half of the WebView2 gate.
// ensureWebView2() pre-flights the runtime before app.Run(); this covers the
// inverse case — the probe found a runtime (or the registry was unreadable)
// yet app.Run() still failed to bring up a window. When the failure is
// plausibly WebView2's fault, show the same native download prompt the
// pre-flight uses.
//
// Strictly additive: it only runs after app.Run() has already returned an
// error, so an install that works can never be interrupted by it. Unlike
// ensureWebView2 it does not call os.Exit — Run()'s error return already
// takes the process down with a non-zero status.
func reportWebView2RunFailure(runErr error) {
	if runErr == nil || !looksLikeWebView2Failure(runErr) {
		return
	}
	slog.Warn("gui: app.Run failed with a WebView2-related error; showing the runtime download prompt",
		"category", "app", "error", runErr)
	if showWebView2MissingDialog() {
		if url, err := windows.UTF16PtrFromString(webview2DownloadURL); err == nil {
			_ = windows.ShellExecute(0, nil, url, nil, nil, windows.SW_SHOWNORMAL)
		}
	}
}
