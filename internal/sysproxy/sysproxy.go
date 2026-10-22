// Package sysproxy reports the proxy address the operating system is
// currently configured to use.
//
// Only the update checker and the installer download reach out to the
// network, so this is the one place that knows what "follow the system
// proxy" means. Each platform reports its proxy in a different shape
// (a plist-ish dictionary on macOS, GSettings keys on GNOME, one registry
// value on Windows), and all three are read by asking the OS every time
// rather than caching: a proxy is something the user changes by switching
// networks or toggling a proxy client, and a cached answer would quietly
// contradict the OS hours later.
//
// Package sysproxy 报告操作系统当前配置的代理地址。
//
// 只有更新检查与安装包下载会主动往外发请求，所以「跟随系统代理」这一件事的
// 知识集中在这里。三个平台各自的表达形状不同（macOS 是一份类 plist 字典、
// GNOME 是 GSettings 键、Windows 是一个注册表值），三者都是**每次现问系统**
// 而不是缓存：代理是用户切个网络、开关一次代理客户端就会变的东西，缓存下来
// 会在几小时后悄悄与系统设置相矛盾。
package sysproxy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// maxProxyLen caps a proxy address. A "scheme://host[:port]" never needs
// more than 512 characters, so a longer string is treated as a typo rather
// than probed: it becomes part of a dial address, and an absurd length is
// itself a signal that the value is not a proxy.
// maxProxyLen 限制代理地址长度。"scheme://host[:port]" 用不到 512 个字符，
// 更长的按写错处理而不去尝试：它会成为拨号地址的一部分，而离谱的长度本身就是
// 「这不是代理地址」的信号。
const maxProxyLen = 512

// proxySchemes are the URL schemes accepted as a proxy address — the ones
// Go's http.Transport can actually dial through. The whitelist also keeps
// "file://" and "data://" out: a proxy address must never be able to turn
// an outbound request into a local file read.
// proxySchemes 是可用作代理地址的 URL 方案，即 Go 的 http.Transport 真正能
// 拨出去的那几个。白名单同时把 "file://" 与 "data://" 挡在外面：一个代理地址
// 不该有本事把对外请求变成本地文件读取。
var proxySchemes = []string{"http", "https", "socks4", "socks4a", "socks5", "socks5h"}

// detectTimeout bounds one call to an external detection command. Reading
// the system proxy is never worth blocking the caller on.
// detectTimeout 限制单次外部探测命令的耗时。读系统代理不值得让调用方卡住。
const detectTimeout = 3 * time.Second

// Resolve returns the proxy address the OS is configured to use right now,
// in the form http.Transport expects (e.g. "http://127.0.0.1:7890"). It
// returns "" when the OS reports no proxy, when this platform cannot
// report one, or when the reported value is not a usable proxy address.
//
// Resolve 返回操作系统当前配置的代理地址，格式为 http.Transport 期望的形式
// （如 "http://127.0.0.1:7890"）。系统未配置代理、本平台无法读取、或读到的值
// 不是可用的代理地址时返回空串。
func Resolve() string {
	switch runtime.GOOS {
	case "darwin":
		// scutil is authoritative on macOS: the environment variables are
		// not set by the system proxy switch, so reading only those would
		// report "no proxy" on a machine that is in fact proxied.
		// scutil 在 macOS 上是权威来源：系统代理开关并不会设置环境变量，
		// 只环境变量会得出「没配代理」，而机器其实正走在代理上。
		return normalize(ParseScutil(runOutput("scutil", "--proxy")))
	case "linux":
		// Environment first (that is what curl and Go's own
		// ProxyFromEnvironment honour), then the GNOME desktop setting.
		// 先环境变量（curl 与 Go 自己的 ProxyFromEnvironment 认的就是它），
		// 再 GNOME 桌面设置。
		if p := normalize(envProxy()); p != "" {
			return p
		}
		return normalize(gnomeProxy())
	case "windows":
		return normalize(osProxy())
	}
	return ""
}

// normalize validates raw and returns it in canonical form, or "" when it
// is not a usable proxy address.
// normalize 校验 raw 并规范化后返回；不是可用的代理地址时返回空串。
func normalize(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" || len(s) > maxProxyLen {
		return ""
	}
	// Whitespace and control characters are rejected outright: the value
	// becomes part of a dial string, and a newline is the classic way to
	// smuggle a second field into one.
	// 空白与控制字符一律拒绝：这个值会成为拨号串的一部分，而换行是把第二个
	// 字段塞进一个字段的经典手法。
	if strings.ContainsAny(s, " \t\r\n\x00") {
		return ""
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return ""
		}
	}
	scheme, rest, ok := strings.Cut(s, "://")
	if !ok {
		return ""
	}
	scheme = strings.ToLower(scheme)
	allowed := false
	for _, p := range proxySchemes {
		if p == scheme {
			allowed = true
			break
		}
	}
	if !allowed {
		return ""
	}
	// A path segment has no meaning on a proxy address; accepting one would
	// only make the user believe it took effect.
	// 代理地址上的路径段没有任何语义，收下只会让人以为它起了作用。
	if rest == "" || strings.Contains(rest, "/") {
		return ""
	}
	// Strip any userinfo before checking the host so "http://@host" is not
	// mistaken for a present host.
	// 检查主机前先剥掉 userinfo，免得 "http://@host" 被当成主机存在。
	host := rest
	if i := strings.LastIndex(host, "@"); i >= 0 {
		host = host[i+1:]
	}
	if host == "" || strings.HasPrefix(host, ":") {
		return ""
	}
	return scheme + "://" + rest
}

// ParseScutil turns the output of `scutil --proxy` into a proxy address,
// or "" when no explicit proxy is configured.
//
// Preference is HTTPS proxy, then HTTP proxy, then SOCKS: every outbound
// request this app makes is https, so a configured HTTPS proxy is what the
// user means. The HTTPS/HTTP entries are HTTP proxies used with CONNECT and
// are therefore emitted with an "http://" scheme — writing "https://" there
// would make the dialer speak TLS to a CONNECT port. SOCKS becomes
// "socks5://", matching what macOS means by its own SOCKS setting.
//
// Only explicit configuration is visible here. A PAC file
// (ProxyAutoConfigEnable : 1) is deliberately NOT evaluated: running a
// JavaScript program to decide the proxy is not this function's job, so
// that case reports "no proxy".
//
// ParseScutil 把 `scutil --proxy` 的输出转成代理地址；没有显式配置时返回空串。
//
// 优先级为 HTTPS 代理 → HTTP 代理 → SOCKS：本应用发出的请求都是 https，系统里
// 配了 HTTPS 代理时按它走才是用户的意思。HTTPS/HTTP 两项本质是用 CONNECT 的
// HTTP 代理，因此产出 "http://" 方案——写成 "https://" 会让拨号端对 CONNECT
// 端口说 TLS。SOCKS 产出 "socks5://"，与 macOS 自己那份 SOCKS 设置语义一致。
//
// 这里只看得到显式配置。PAC 文件（ProxyAutoConfigEnable : 1）刻意不求值：
// 跑一段 JavaScript 来决定代理不该由这个函数做，因此那种情况报「没有代理」。
func ParseScutil(text string) string {
	values := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		// Shape is `  HTTPProxy : 127.0.0.1`; both sides may be padded.
		// 形状是 `  HTTPProxy : 127.0.0.1`；两侧都可能有空格。
		values[strings.TrimSpace(key)] = strings.TrimSpace(val)
	}
	// Entries in preference order: scheme, host key, port key, enable flag.
	// 条目按优先顺序：方案、主机键、端口键、启用标志。
	entries := []struct {
		scheme  string
		hostKey string
		portKey string
		flag    string
	}{
		{"http", "HTTPSProxy", "HTTPSPort", "HTTPSEnable"},
		{"http", "HTTPProxy", "HTTPPort", "HTTPEnable"},
		{"socks5", "SOCKSProxy", "SOCKSPort", "SOCKSEnable"},
	}
	for _, e := range entries {
		if values[e.flag] != "1" {
			continue
		}
		host := values[e.hostKey]
		if host == "" {
			continue
		}
		port, err := strconv.ParseUint(values[e.portKey], 10, 32)
		if err != nil || port == 0 {
			// "host:0" cannot be dialled; treat it as unconfigured and
			// look at the next entry rather than emitting a broken URL.
			// "host:0" 拨不出去，视为未配置并看下一个条目，而不是拼出一个坏 URL。
			continue
		}
		return fmt.Sprintf("%s://%s:%d", e.scheme, host, port)
	}
	return ""
}

// ParseGnomeEntry turns the two `gsettings get` outputs for one proxy entry
// into a host and a port. GSettings prints strings quoted ("'127.0.0.1'")
// and integers with a type prefix ("int32 7890"), and "unset" shows up as
// an empty quoted string plus port 0 — which is reported as not usable so
// the caller moves on to the next entry.
//
// ParseGnomeEntry 把一条代理配置对应的两个 `gsettings get` 输出转成主机与端口。
// GSettings 打印字符串时带引号（"'127.0.0.1'"）、整数带类型前缀（"int32 7890"），
// 而「未配置」表现为空引号串加端口 0 —— 这种情况报不可用，让调用方去看下一条。
func ParseGnomeEntry(hostText, portText string) (string, uint32, bool) {
	host := strings.Trim(strings.TrimSpace(hostText), "'\"")
	if host == "" {
		return "", 0, false
	}
	p := strings.TrimSpace(portText)
	p = strings.TrimPrefix(p, "int32")
	p = strings.TrimSpace(p)
	port, err := strconv.ParseUint(p, 10, 32)
	if err != nil || port == 0 {
		return "", 0, false
	}
	return host, uint32(port), true
}

// envProxy returns the proxy named by the conventional environment
// variables. Bare "host:port" values (no scheme) are read as http, which is
// what curl and most tooling assume.
// envProxy 返回环境变量里那个代理。裸的 "host:port"（无方案）按 http 读，
// 这也是 curl 与多数工具的默认假设。
func envProxy() string {
	for _, key := range []string{"https_proxy", "HTTPS_PROXY", "http_proxy", "HTTP_PROXY", "all_proxy", "ALL_PROXY"} {
		v := strings.TrimSpace(os.Getenv(key))
		if v == "" {
			continue
		}
		if !strings.Contains(v, "://") {
			v = "http://" + v
		}
		if n := normalize(v); n != "" {
			return n
		}
	}
	return ""
}

// gnomeProxy reads the GNOME desktop proxy setting. Only "manual" mode
// yields an address: "none" means no proxy and "auto" is a PAC file, which
// is not evaluated here (see ParseScutil).
// gnomeProxy 读取 GNOME 桌面的代理设置。只有 "manual" 模式会产出地址：
// "none" 表示没有代理，"auto" 是 PAC 文件，这里不求值（见 ParseScutil）。
func gnomeProxy() string {
	// gsettings prints enum values quoted ("'manual'"), so the quotes are
	// stripped before comparing.
	// gsettings 打印枚举值时带引号（"'manual'"），比较前先去掉引号。
	if mode := stripQuotes(gsettings("org.gnome.system.proxy", "mode")); mode != "manual" {
		return ""
	}
	if host, port, ok := gnomeEntry("org.gnome.system.proxy.http"); ok {
		return fmt.Sprintf("http://%s:%d", host, port)
	}
	if host, port, ok := gnomeEntry("org.gnome.system.proxy.socks"); ok {
		return fmt.Sprintf("socks5://%s:%d", host, port)
	}
	return ""
}

func gnomeEntry(schema string) (string, uint32, bool) {
	return ParseGnomeEntry(gsettings(schema, "host"), gsettings(schema, "port"))
}

// stripQuotes removes one layer of surrounding single or double quotes.
// stripQuotes 去掉外层的一对单引号或双引号。
func stripQuotes(s string) string {
	t := strings.TrimSpace(s)
	if len(t) >= 2 {
		if (t[0] == '\'' && t[len(t)-1] == '\'') || (t[0] == '"' && t[len(t)-1] == '"') {
			return t[1 : len(t)-1]
		}
	}
	return t
}

// gsettings runs one `gsettings get` and returns its trimmed output, or ""
// when the command is missing (not a GNOME desktop) or fails.
// gsettings 跑一次 `gsettings get` 并返回去掉首尾空白的输出；命令不存在
// （非 GNOME 桌面）或报错时返回空串。
func gsettings(schema, key string) string {
	out := runOutput("gsettings", "get", schema, key)
	return strings.TrimSpace(out)
}

// runOutput runs a detection command with a hard timeout and returns its
// stdout, or "" on any failure. Detection is best-effort: a missing or
// failing command simply means "this platform reports nothing".
// runOutput 带硬超时地跑一条探测命令并返回 stdout，任何失败都返回空串。
// 探测是尽力而为：命令不存在或报错就等于「这个平台报不出东西」。
func runOutput(name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), detectTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return ""
	}
	return string(out)
}
