package sysproxy

import "testing"

// The shape below is real `scutil --proxy` output from a machine running a
// local proxy client (one HTTP port and one SOCKS port). What is asserted
// is *which entry wins and which scheme it is emitted with*: a wrong scheme
// makes the dialer speak the wrong protocol to a healthy port, and the user
// only ever sees "the update check failed" with no reason.
// 下面是真实 `scutil --proxy` 输出的形状（本地代理客户端：一个 HTTP 端口、
// 一个 SOCKS 端口）。这里断言的是**取哪一条、以及用哪个方案产出**：方案拼错
// 会让拨号端对着一个健康的端口说错协议，而用户只看得到「更新检查失败」。
func TestParseScutilPrefersHTTPSThenHTTPThenSOCKS(t *testing.T) {
	both := `<dictionary> {
  ExcludeSimpleHostnames : 0
  HTTPEnable : 1
  HTTPPort : 7890
  HTTPProxy : 127.0.0.1
  HTTPSEnable : 1
  HTTPSPort : 7890
  HTTPSProxy : 127.0.0.1
  ProxyAutoConfigEnable : 0
  SOCKSEnable : 1
  SOCKSPort : 7891
  SOCKSProxy : 127.0.0.1
}`
	if got := ParseScutil(both); got != "http://127.0.0.1:7890" {
		t.Fatalf("HTTPS proxy must win: got %q", got)
	}

	// SOCKS only: the scheme has to be socks5, otherwise an HTTP dial is
	// made against a SOCKS port.
	// 只有 SOCKS：方案必须是 socks5，否则会拿 HTTP 语法去连 SOCKS 端口。
	socksOnly := `<dictionary> {
  HTTPEnable : 0
  HTTPPort : 7890
  HTTPProxy : 127.0.0.1
  SOCKSEnable : 1
  SOCKSPort : 7891
  SOCKSProxy : 127.0.0.1
}`
	if got := ParseScutil(socksOnly); got != "socks5://127.0.0.1:7891" {
		t.Fatalf("socks-only: got %q", got)
	}

	// A PAC file is not evaluated here, so it reports "no proxy" rather
	// than guessing an address.
	// PAC 文件在这里不求值，因此报「没有代理」而不是猜一个地址。
	pac := "<dictionary> {\n  ProxyAutoConfigEnable : 1\n  ProxyAutoConfigURLString : http://intranet/proxy.pac\n}"
	if got := ParseScutil(pac); got != "" {
		t.Fatalf("PAC must not be evaluated: got %q", got)
	}
	if got := ParseScutil("<dictionary> {\n}"); got != "" {
		t.Fatalf("empty dictionary: got %q", got)
	}

	// Enabled but with no port: "host:0" cannot be dialled, which is the
	// same as unconfigured.
	// 开了但没配端口："host:0" 拨不出去，等同未配置。
	broken := "<dictionary> {\n  HTTPEnable : 1\n  HTTPProxy : 127.0.0.1\n}"
	if got := ParseScutil(broken); got != "" {
		t.Fatalf("missing port must be ignored: got %q", got)
	}
}

// GSettings prints quoted strings and integer values with an "int32"
// prefix, and "unset" is an empty quoted string with port 0.
// GSettings 打印字符串带引号、整数带 "int32" 前缀，而「未配置」是空引号串
// 加端口 0。
func TestParseGnomeEntry(t *testing.T) {
	if host, port, ok := ParseGnomeEntry("'127.0.0.1'", "int32 7890"); !ok || host != "127.0.0.1" || port != 7890 {
		t.Fatalf("plain entry: got %q %d %v", host, port, ok)
	}
	if _, _, ok := ParseGnomeEntry("''", "int32 7890"); ok {
		t.Fatal("an empty host is not a usable entry")
	}
	if _, _, ok := ParseGnomeEntry("'proxy.internal'", "int32 0"); ok {
		t.Fatal("port 0 is not a usable entry")
	}
	if _, _, ok := ParseGnomeEntry("'127.0.0.1'", "not-a-number"); ok {
		t.Fatal("an unparsable port is not a usable entry")
	}
}

// What is rejected is not "ugly" but "must never be dialled": the scheme
// whitelist keeps file:// and data:// out, and the whitespace check blocks
// turning one value into two fields.
// 被拒的不是「不好看」而是「绝不许拨出去」：方案白名单挡掉 file:// 与 data://，
// 空白检查挡掉把一个值变成两个字段的写法。
func TestNormalizeAcceptsOnlySchemePlusAuthority(t *testing.T) {
	ok := []struct{ in, want string }{
		{"http://127.0.0.1:7890", "http://127.0.0.1:7890"},
		{"  http://127.0.0.1:7890  ", "http://127.0.0.1:7890"},
		{"socks5://10.0.0.1:1080", "socks5://10.0.0.1:1080"},
		{"socks5h://10.0.0.1", "socks5h://10.0.0.1"},
		{"http://user:pw@10.0.0.1:8080", "http://user:pw@10.0.0.1:8080"},
		// The scheme is lower-cased so the shown value and the dialled
		// value are the same string.
		// 方案统一小写，好让显示的那一份与拨出去的那份是同一个字符串。
		{"HTTPS://proxy.internal:3128", "https://proxy.internal:3128"},
	}
	for _, c := range ok {
		if got := normalize(c.in); got != c.want {
			t.Fatalf("normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	bad := []string{
		"",
		"   ",
		"127.0.0.1:7890",
		"ftp://10.0.0.1:21",
		"file:///etc/passwd",
		"data://x",
		"http://",
		"http://127.0.0.1:7890/some/path",
		"http://127.0. 0.1:7890",
		"http://127.0.0.1:7890\n--proxy=http://evil",
		"http://\x01127.0.0.1:1",
		"http://@:1",
		"http://" + string(make([]byte, maxProxyLen)) + ":1",
		// socks4/socks4a are NOT usable proxy addresses for this app: Go's
		// http.Transport only dials http/https/socks5/socks5h and would
		// speak CONNECT to a SOCKS4 server. Reporting them keeps "follow
		// system" honest — a direct connection instead of a guaranteed
		// proxy-side failure.
		// socks4/socks4a 对本应用不是可用地址：Go 的 http.Transport 只拨
		// http/https/socks5/socks5h，否则会对 SOCKS4 服务器说 CONNECT。
		// 拒掉它们才能让「跟随系统」诚实——宁可直连，也不要注定失败的
		// 代理侧错误。
		"socks4://10.0.0.1:1080",
		"socks4a://10.0.0.1:1080",
	}
	for _, in := range bad {
		if got := normalize(in); got != "" {
			t.Fatalf("normalize(%q) = %q, want rejection", in, got)
		}
	}
}

// GSettings enum values come back quoted, so the quotes have to be stripped
// before the mode is compared — otherwise a desktop configured for a manual
// proxy reads as something else entirely.
// GSettings 的枚举值带引号，比较模式前必须去掉引号——否则一个确实配了手动代理的
// 桌面会被读成别的模式。
func TestStripQuotes(t *testing.T) {
	for in, want := range map[string]string{
		"'manual'": "manual",
		"\"auto\"": "auto",
		"none":     "none",
		"  'x'  ":  "x",
		"'":        "'",
	} {
		if got := stripQuotes(in); got != want {
			t.Fatalf("stripQuotes(%q) = %q, want %q", in, got, want)
		}
	}
}

// The Windows ProxyServer value comes in three shapes: a bare address, a
// bare address with its own scheme, and a per-protocol list. What is
// asserted is *which entry wins and which scheme it is emitted with*: a
// wrong scheme makes the dialer speak the wrong protocol to a healthy
// port, and the user only ever sees "the update check failed" with no
// reason.
// Windows 的 ProxyServer 值有三种形状：裸地址、自带方案的裸地址、分协议列表。
// 这里断言的是**取哪一条、以及用哪个方案产出**：方案拼错会让拨号端对着一个
// 健康的端口说错协议，而用户只看得到「更新检查失败」。
func TestWindowsServerParsesRegistryShapes(t *testing.T) {
	ok := []struct{ in, want string }{
		// Bare address: one proxy for every protocol; a scheme has to be
		// added because Windows stores it bare.
		// 裸地址：所有协议共用；Windows 存的是裸值，需要补上方案。
		{"127.0.0.1:7890", "http://127.0.0.1:7890"},
		{"proxy.corp:8080", "http://proxy.corp:8080"},
		// A bare value that already carries a scheme is kept as written.
		// 自带方案的裸值按原样保留。
		{"http://127.0.0.1:7890", "http://127.0.0.1:7890"},
		{"socks5://10.0.0.1:1080", "socks5://10.0.0.1:1080"},
		// Per-protocol list: https wins over http over socks, whatever
		// order the entries appear in.
		// 分协议列表：无论条目顺序如何，https 胜过 http 胜过 socks。
		{"http=127.0.0.1:7890;https=127.0.0.1:7890;socks=127.0.0.1:7891", "http://127.0.0.1:7890"},
		{"socks=127.0.0.1:7891;http=127.0.0.1:7890", "http://127.0.0.1:7890"},
		{"http=127.0.0.1:7890", "http://127.0.0.1:7890"},
		{"socks=127.0.0.1:7891", "socks5://127.0.0.1:7891"},
		// Empty entries are skipped rather than treated as configured.
		// 空条目被跳过，而不是当成已配置。
		{"https=;http=127.0.0.1:7890", "http://127.0.0.1:7890"},
		{"http=;socks=127.0.0.1:7891", "socks5://127.0.0.1:7891"},
		{"https=;http=;socks=", ""},
		// A per-protocol value carrying its own scheme is preserved
		// verbatim — the shape some tools and hand-edited registries write.
		// Prefixing another scheme would double it into an address
		// normalize rejects, silently reading as "no proxy".
		// 自带方案的分协议值原样保留——某些工具与手改的注册表正是这种形状。
		// 再拼一个方案会双重前缀成被 normalize 拒绝的地址，悄悄变成「没有代理」。
		{"https=https://proxy.corp:8080;http=127.0.0.1:7890", "https://proxy.corp:8080"},
		{"http=http://127.0.0.1:7890", "http://127.0.0.1:7890"},
		{"socks=socks5://127.0.0.1:7891", "socks5://127.0.0.1:7891"},
	}
	for _, c := range ok {
		if got := windowsServer(c.in); got != c.want {
			t.Errorf("windowsServer(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Every shape windowsServer can emit must still be a usable address after
// normalize — a shape that parses here but dies there is a silent "no
// proxy": the panel reports the system has no proxy configured and every
// update leaves the machine directly. This is the regression guard for the
// double-prefixed "http://https://…" bug.
// windowsServer 产出的每种形状都必须过得了 normalize——在这里解析成功却在
// 那里死掉的形状就是一次静默的「没有代理」：面板报告系统没配代理，全部更新
// 直连外出。这是 "http://https://…" 双重前缀 bug 的回归防线。
func TestWindowsServerOutputSurvivesNormalize(t *testing.T) {
	for _, in := range []string{
		"127.0.0.1:7890",
		"http=127.0.0.1:7890;https=127.0.0.1:7890;socks=127.0.0.1:7891",
		"https=https://proxy.corp:8080;http=127.0.0.1:7890",
		"http=http://127.0.0.1:7890",
		"socks=socks5://127.0.0.1:7891",
	} {
		if got := normalize(windowsServer(in)); got == "" {
			t.Errorf("normalize(windowsServer(%q)) = \"\", want a usable address", in)
		}
	}
}
