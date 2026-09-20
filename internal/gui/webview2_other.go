//go:build !windows

package gui

// ensureWebView2 is a no-op outside Windows: WebView2 is a Windows-only
// runtime and the macOS/Linux builds use their platform-native webviews.
func ensureWebView2() {}
