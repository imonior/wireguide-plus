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
