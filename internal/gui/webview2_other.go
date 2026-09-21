//go:build !windows

package gui

// ensureWebView2 is a no-op outside Windows: WebView2 is a Windows-only
// runtime and the macOS/Linux builds use their platform-native webviews.
func ensureWebView2() {}

// reportWebView2RunFailure is a no-op outside Windows — see the Windows
// implementation for why the post-launch fallback exists.
func reportWebView2RunFailure(err error) { _ = err }
