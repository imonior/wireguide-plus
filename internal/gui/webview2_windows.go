//go:build windows

package gui

import (
	"os"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// webview2ClientGUID is the EdgeUpdate client GUID for the WebView2
// Evergreen runtime. Its "pv" (version) value is written to the registry
// when the runtime is installed — system-wide or per-user — which is the
// same probe NSIS ran before this app stopped bundling the runtime.
const webview2ClientGUID = "{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"

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
// present, mirroring the registry probe Wails/NSIS use.
func webview2Installed() bool {
	const sub = `Software\Microsoft\EdgeUpdate\Clients\` + webview2ClientGUID
	for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		key, err := registry.OpenKey(root, sub, registry.READ)
		if err != nil {
			continue
		}
		v, _, err := key.GetStringValue("pv")
		key.Close()
		if err == nil && v != "" {
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
