package update

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
)

// proxyConfig is the outbound-proxy setting applied to update-check HTTP
// clients.
type proxyConfig struct {
	mode string // "direct", "mirror" or "manual"
	url  string // manual proxy URL or mirror prefix (see SetProxy)
}

var currentProxy atomic.Value // stores proxyConfig

// SetProxy updates the outbound proxy used by update-check HTTP clients.
//   - mode "direct": no proxy at all (explicit — even if the environment
//     defines HTTP_PROXY/HTTPS_PROXY).
//   - mode "mirror": rawURL is a GitHub accelerator mirror prefix (e.g.
//     "https://ghfast.top"); the API endpoint is rewritten to
//     "<mirror>/<official endpoint>" and fetched directly.
//   - mode "manual": routes requests through rawURL as an HTTP/SOCKS proxy.
//
// Safe to call at any time; applies to newly created clients.
func SetProxy(mode, rawURL string) {
	currentProxy.Store(proxyConfig{mode: mode, url: rawURL})
}

// ValidProxyURL reports whether u can be used as a CONNECT proxy. Go's
// url.Parse accepts "https://" (empty host), which http.Transport then
// turns into CONNECT dials that fail with "tls: either ServerName or
// InsecureSkipVerify must be specified in the tls.Config". Guard against
// that here so a half-typed URL degrades to direct instead of failing
// every update check.
func ValidProxyURL(u *url.URL) bool {
	if u == nil || u.Hostname() == "" {
		return false
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
		return true
	}
	return false
}

// ValidMirrorPrefix reports whether prefix can be used as a GitHub
// accelerator mirror prefix (http/https with a non-empty host).
func ValidMirrorPrefix(prefix string) bool {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(prefix), "/"))
	if err != nil || u == nil || u.Hostname() == "" {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

// proxyTransport builds a Transport honoring the last SetProxy call.
// Manual mode parses the URL once per request setup; an invalid URL
// silently falls back to direct so update checks never hard-fail.
func proxyTransport() *http.Transport {
	tr := &http.Transport{}
	if v := currentProxy.Load(); v != nil {
		if cfg, ok := v.(proxyConfig); ok && cfg.mode == "manual" && cfg.url != "" {
			if u, err := url.Parse(cfg.url); err == nil && ValidProxyURL(u) {
				tr.Proxy = http.ProxyURL(u)
			} else {
				slog.Warn("update: ignoring invalid manual proxy URL", "url", cfg.url)
			}
		}
	}
	return tr
}

// resolvedEndpoint returns the GitHub API endpoint the updater should hit
// right now: the official endpoint, or the official endpoint rewritten
// through the configured mirror prefix when mode is "mirror".
func resolvedEndpoint() string {
	if v := currentProxy.Load(); v != nil {
		if cfg, ok := v.(proxyConfig); ok && cfg.mode == "mirror" && cfg.url != "" {
			return mirrorEndpoint(cfg.url)
		}
	}
	return apiEndpoint
}

// mirrorEndpoint rewrites the official API endpoint through the given
// accelerator prefix (e.g. "https://ghfast.top"). Callers use it both for
// the live checker (resolvedEndpoint) and for the user-facing connectivity
// test of an as-yet-unsaved mirror setting.
func mirrorEndpoint(prefix string) string {
	prefix = strings.TrimRight(strings.TrimSpace(prefix), "/")
	if prefix == "" || !ValidMirrorPrefix(prefix) {
		slog.Warn("update: ignoring invalid mirror prefix", "prefix", prefix)
		return apiEndpoint
	}
	return prefix + "/" + apiEndpoint
}
