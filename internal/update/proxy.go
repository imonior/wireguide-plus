package update

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/imonior/wireguide-plus/internal/sysproxy"
)

// proxyConfig is the outbound-proxy setting applied to update-check HTTP
// clients.
type proxyConfig struct {
	mode string // "direct", "system", "mirror" or "manual"
	url  string // manual proxy URL or mirror prefix (see SetProxy)
}

var currentProxy atomic.Value // stores proxyConfig

// SetProxy updates the outbound proxy used by update-check HTTP clients.
//   - mode "direct": no proxy at all (explicit — even if the environment
//     defines HTTP_PROXY/HTTPS_PROXY).
//   - mode "system": ask the OS for its proxy on every request.
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
func proxyTransport() *http.Transport {
	if v := currentProxy.Load(); v != nil {
		if cfg, ok := v.(proxyConfig); ok {
			return PathTransport(cfg.mode, cfg.url)
		}
	}
	return PathTransport("direct", "")
}

// PathTransport returns a Transport that sends requests out through the
// given path. It is the single mapping from a stored (or not-yet-stored)
// path to "how this request leaves the machine", so an update check, a
// per-path speed test and the public-IP readout cannot disagree about what
// a path means — which is exactly the bug that appears when the probe uses
// one interpretation and the actual request another.
//
//   - "direct" and "mirror": no proxy. A mirror is a URL rewrite, not an
//     outbound route, so it still leaves the machine directly.
//   - "system": the OS proxy, resolved per request (see systemProxyURL).
//   - "manual": rawURL as an HTTP/SOCKS proxy. An invalid URL falls back to
//     no proxy so an update check never hard-fails on a half-typed address.
//
// PathTransport 返回一个让请求经给定「通路」外发的 Transport。它是从（已存或
// 未存的）通路到「这次请求怎么出去」的唯一映射，于是更新检查、逐通路测速与公网
// IP 读数不可能对同一条通路的含义产生分歧——而「探测用一种解释、真实请求用另一种」
// 正是最难查的那类 bug。
//
//   - "direct" 与 "mirror"：不用代理。镜像是 URL 改写而不是出口路由，机器依然是
//     直连出去的。
//   - "system"：每次请求现问系统的代理（见 systemProxyURL）。
//   - "manual"：用 rawURL 作为 HTTP/SOCKS 代理。地址非法时回退到不用代理，
//     好让更新检查不会因为一个打到一半的地址而彻底失败。
func PathTransport(mode, rawURL string) *http.Transport {
	tr := &http.Transport{}
	switch mode {
	case "system":
		tr.Proxy = systemProxyURL
	case "manual":
		if rawURL == "" {
			return tr
		}
		if u, err := url.Parse(rawURL); err == nil && ValidProxyURL(u) {
			tr.Proxy = http.ProxyURL(u)
		} else {
			slog.Warn("update: ignoring invalid manual proxy URL", "url", rawURL)
		}
	}
	return tr
}

// systemProxyURL resolves the OS proxy for one request. It is called per
// request rather than once at startup because the OS answer changes when the
// user switches networks or toggles a proxy client; returning nil means
// "no proxy", which is also the right answer when the OS reports none.
//
// systemProxyURL 为单次请求解析系统代理。每次请求都调用而不是启动时算一次，
// 因为用户切网络或开关代理客户端后系统的答案就变了；返回 nil 表示「不用代理」，
// 系统没有配置时这也正是正确答案。
func systemProxyURL(req *http.Request) (*url.URL, error) {
	raw := sysproxy.Resolve()
	if raw == "" {
		return nil, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		slog.Warn("update: ignoring unparsable system proxy", "url", raw)
		return nil, nil
	}
	return u, nil
}

// PathRequestURL returns the URL an update check hits when it goes out
// through the given path: the official API endpoint, or that endpoint
// rewritten through a mirror prefix. Only "mirror" changes the URL — a proxy
// changes how the request leaves, not where it goes.
//
// PathRequestURL 返回更新检查经给定通路发出时请求的 URL：官方 API 端点，或经
// 镜像前缀改写后的端点。只有 "mirror" 会改变 URL——代理改变的是请求怎么出去，
// 而不是去哪。
func PathRequestURL(mode, prefix string) string {
	if mode == "mirror" {
		return mirrorEndpoint(prefix)
	}
	return apiEndpoint
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
