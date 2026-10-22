//go:build !windows

package sysproxy

// osProxy exists so the Windows branch of Resolve compiles on every
// platform. Only Windows keeps its proxy in a registry key; the other
// platforms are handled inside Resolve itself (macOS through scutil, Linux
// through the environment and GSettings).
//
// osProxy 的存在是为了让 Resolve 里的 Windows 分支在任何平台都能编译。只有
// Windows 把代理存在注册表键里；其余平台在 Resolve 内部处理（macOS 走 scutil、
// Linux 走环境变量与 GSettings）。
func osProxy() string { return "" }
