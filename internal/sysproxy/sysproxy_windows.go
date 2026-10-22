//go:build windows

package sysproxy

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

// internetSettingsKey is where Windows keeps the proxy configuration that
// every WinINET/WinHTTP client — and therefore every browser and most
// update tooling — reads. It is the same value the Settings app edits.
//
// Two copies of that key exist: the per-user one under HKCU and the
// machine-wide one under HKLM. Which is in force depends on policy (see
// machineWideProxy), so both are consulted.
//
// internetSettingsKey 是 Windows 存放代理配置的位置，WinINET/WinHTTP 客户端
// （也就是所有浏览器与多数升级工具）都读这一份，也正是「设置」应用改的那一份。
//
// 这份键有两份副本：HKCU 下的每用户一份与 HKLM 下的机器级一份。哪份生效取决于
// 策略（见 machineWideProxy），所以两份都要看。
const internetSettingsKey = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

// policyKey is where Group Policy parks the proxy settings it enforces.
// ProxySettingsPerUser = 0 there means "one proxy for the whole machine":
// Windows then reads HKLM's Internet Settings and ignores HKCU entirely, so
// a per-user-only reader would report "no proxy" on a managed machine that
// is in fact proxied.
// policyKey 是组策略存放强制代理设置的位置。那里的 ProxySettingsPerUser = 0
// 表示「整机共用一个代理」：此时 Windows 读 HKLM 的 Internet Settings 并完全忽略
// HKCU，只看每用户那份的实现会在一台真实走代理的受管机器上报「没有代理」。
const policyKey = `Software\Policies\Microsoft\Windows\CurrentVersion\Internet Settings`

// osProxy returns the Windows system proxy, or "" when proxying is off or
// the value cannot be read.
//
// The registry is read directly instead of calling WinHTTP through cgo: it
// is a plain key read, needs no extra runtime dependency, and returns the
// same value the user sees in Settings → Network & Internet → Proxy.
//
// Two cases are deliberately NOT answered here:
//
//   - A PAC / auto-config script (AutoConfigURL, "Use setup script"): the
//     address is produced by running a JavaScript program per URL, and
//     evaluating that is not this package's job — the same rule macOS
//     follows (see ParseScutil). Such a machine reports "" and the user
//     should pin the proxy address manually.
//   - The bypass list (ProxyOverride): it is ignored because the only
//     outbound targets are GitHub hosts, which no sane bypass list covers.
//
// osProxy 返回 Windows 的系统代理；未启用或读不到时返回空串。
//
// 这里直接读注册表而不是经 cgo 调 WinHTTP：只是一次普通键值读取，不需要额外的
// 运行时依赖，返回的就是用户在「设置 → 网络和 Internet → 代理」里看到的那个值。
//
// 两种情况刻意不在这里作答：
//
//   - PAC / 自动配置脚本（AutoConfigURL、"使用设置脚本"）：地址由一段按 URL 执行的
//     JavaScript 算出，求值不是本包的职责——与 macOS 同一规则（见 ParseScutil）。
//     这类机器返回空串，用户应手动指定代理地址。
//   - 绕过列表（ProxyOverride）：忽略，因为唯一的对外目标都是 GitHub 主机，
//     任何正常的绕过列表都不会覆盖它们。
func osProxy() string {
	machine := readProxyStore(registry.LOCAL_MACHINE)
	if machineWideProxy() {
		return machine
	}
	if perUser := readProxyStore(registry.CURRENT_USER); perUser != "" {
		return perUser
	}
	// Fall back to the machine-wide store: a proxy configured for the whole
	// machine is still in force for this user even without the policy flag
	// (it is how a system-wide proxy set outside Group Policy shows up).
	// 回退到机器级存储：为整机配置的代理即使没有策略标志，对本用户依然生效
	// （组策略之外的整机代理就是这样出现的）。
	return machine
}

// machineWideProxy reports whether policy forces one machine-wide proxy.
// ProxySettingsPerUser is 0 in that case; any other value (or a missing
// policy key, the normal home-machine case) means the per-user setting wins.
// machineWideProxy 报告策略是否强制整机共用一个代理。此时 ProxySettingsPerUser
// 为 0；其它取值或策略键不存在（家用机器的常态）表示每用户设置优先。
func machineWideProxy() bool {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, policyKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	v, _, err := key.GetIntegerValue("ProxySettingsPerUser")
	if err != nil {
		return false
	}
	return v == 0
}

// readProxyStore reads ProxyEnable + ProxyServer from the Internet Settings
// key under root (HKCU = per-user, HKLM = machine-wide) and returns one proxy
// address, or "" when the store has proxying switched off or unreadable.
//
// ProxyEnable is 0 when the system proxy switch is off. A configured address
// with the switch off is not in effect, so it must not be used.
//
// readProxyStore 从 root（HKCU = 每用户，HKLM = 机器级）下的 Internet Settings
// 键读取 ProxyEnable 与 ProxyServer，返回一个代理地址；该存储关闭了代理或读不到时
// 返回空串。
//
// ProxyEnable 为 0 表示系统代理开关关闭。关着时配置的地址并未生效，不能用。
func readProxyStore(root registry.Key) string {
	key, err := registry.OpenKey(root, internetSettingsKey, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()

	enabled, _, err := key.GetIntegerValue("ProxyEnable")
	if err != nil || enabled == 0 {
		return ""
	}
	server, _, err := key.GetStringValue("ProxyServer")
	if err != nil {
		return ""
	}
	return windowsServer(strings.TrimSpace(server))
}
