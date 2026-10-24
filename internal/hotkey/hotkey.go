// Package hotkey implements a cross-platform global hotkey that toggles the
// main application window (show / hide / focus). It supports macOS
// (RegisterEventHotKey via Carbon) and Windows (RegisterHotKey + a message
// pump); every other platform returns ErrUnsupported so the UI can disable
// the feature gracefully.
//
// The combo string is canonical and platform-agnostic: modifiers are
// Ctrl / Alt / Shift / Win (case-insensitive) joined with "+", and the final
// segment is a KeyboardEvent.code token captured from a DOM keydown
// (e.g. "Space", "KeyT", "F12", "Digit1", "ArrowUp", "Comma"). The frontend
// is responsible for capturing and displaying the combo; this package only
// parses and installs it.
//
// Package hotkey 实现跨平台全局热键，用于切换（显示/隐藏/聚焦）主窗口。
// 支持 macOS（通过 Carbon 的 RegisterEventHotKey）与 Windows（RegisterHotKey +
// 消息泵）；其余平台返回 ErrUnsupported，便于界面优雅地禁用该功能。
//
// 组合键字符串是规范且与平台无关的：修饰键为 Ctrl/Alt/Shift/Win（不区分大小写），
// 用 "+" 连接，末段是 DOM keydown 捕获到的 KeyboardEvent.code 令牌
// （如 "Space"、"KeyT"、"F12"、"Digit1"、"ArrowUp"、"Comma"）。前端负责捕获与
// 展示组合键，本包只解析并注册它。
package hotkey

import (
	"errors"
	"runtime"
	"strings"
	"sync"
)

// Modifier bits (platform-agnostic) for a parsed combo.
// Modifier 位（平台无关）对应解析后的组合键。
const (
	ModCtrl  = 1 << iota // Ctrl / Control
	ModAlt               // Alt / Option
	ModShift             // Shift
	ModWin               // Win / Cmd / Meta / Super
)

// ErrUnsupported is returned when global hotkeys are not available on the
// current platform (e.g. Linux without a supported backend, or a key the
// platform cannot map).
// ErrUnsupported 在平台不支持全局热键时返回（如缺少受支持后端的 Linux，或平台
// 无法映射的按键）。
var ErrUnsupported = errors.New("global hotkeys are not supported on this platform")

// Combo is a parsed hotkey combination. Key is the normalized token taken
// from a DOM KeyboardEvent.code (e.g. "Space", "KeyT", "F12", "Digit1",
// "ArrowUp", "Comma"). Modifiers is a bitmask of Mod*.
// Combo 是解析后的热键组合。Key 为取自 DOM KeyboardEvent.code 的规范化令牌；
// Modifiers 是 Mod* 的位掩码。
type Combo struct {
	Modifiers int
	Key       string
}

// ParseCombo parses a canonical combo string such as "Ctrl+Alt+Space" or
// "Win+Shift+T". The final segment is the key token; preceding segments are
// modifiers. Recognized modifiers (case-insensitive): Ctrl/Control, Alt/Option,
// Shift, Win/Cmd/Command/Meta/Super. It returns an error for an empty string,
// an unrecognized modifier, a missing key, or a combo with no modifier.
//
// ParseCombo 解析形如 "Ctrl+Alt+Space" 或 "Win+Shift+T" 的规范组合键字符串。
// 末段为按键令牌，其余为修饰键。可识别的修饰键（不区分大小写）：Ctrl/Control、
// Alt/Option、Shift、Win/Cmd/Command/Meta/Super。对空字符串、无法识别的修饰键、
// 缺少按键或不含修饰键的组合返回错误。
func ParseCombo(s string) (Combo, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Combo{}, errors.New("empty hotkey combo")
	}
	parts := strings.Split(s, "+")
	if len(parts) < 2 {
		return Combo{}, errors.New("hotkey combo must include at least one modifier and a key")
	}
	var c Combo
	for i, p := range parts {
		tok := strings.TrimSpace(strings.ToLower(p))
		if tok == "" {
			return Combo{}, errors.New("empty segment in hotkey combo")
		}
		if i == len(parts)-1 {
			key := normalizeKeyToken(parts[i])
			if key == "" {
				return Combo{}, errors.New("invalid hotkey key: " + parts[i])
			}
			c.Key = key
			continue
		}
		switch tok {
		case "ctrl", "control":
			c.Modifiers |= ModCtrl
		case "alt", "option":
			c.Modifiers |= ModAlt
		case "shift":
			c.Modifiers |= ModShift
		case "win", "cmd", "command", "meta", "super":
			c.Modifiers |= ModWin
		default:
			return Combo{}, errors.New("unrecognized modifier: " + parts[i])
		}
	}
	return c, nil
}

// normalizeKeyToken keeps the event.code token as-is for punctuation / named
// keys, and strips the "Key"/"Digit" prefix from letter/digit tokens so the
// platform maps (indexed by "A".."Z" and "0".."9") can resolve them. Returns
// "" for an empty token.
// normalizeKeyToken 对标点/具名按键保留 event.code 原令牌，并去掉字母/数字令牌
// 的 "Key"/"Digit" 前缀，以便按 "A".."Z"、"0".."9" 索引的平台映射表解析。
// 空令牌返回 ""。
func normalizeKeyToken(tok string) string {
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return ""
	}
	if strings.HasPrefix(tok, "Key") && len(tok) == 4 {
		return tok[3:]
	}
	if strings.HasPrefix(tok, "Digit") && len(tok) == 6 {
		return tok[5:]
	}
	return tok
}

// Supported reports whether global hotkeys can be registered on the current
// platform. It is compile-time deterministic: only macOS and Windows have a
// backend.
// Supported 报告当前平台是否支持注册全局热键。该结果在编译期即确定：仅 macOS
// 与 Windows 有后端实现。
func Supported() bool {
	return runtime.GOOS == "darwin" || runtime.GOOS == "windows"
}

// Manager registers and unregisters a single global hotkey and forwards
// presses to a callback. Register is safe to call repeatedly (it re-installs);
// passing a combo with an empty Key uninstalls the active hotkey. The callback
// runs on the platform's event-handling path, so callers that touch UI must
// marshal to the UI thread themselves.
//
// Manager 注册/注销单个全局热键，并把按键事件转发给回调。Register 可重复调用
// （会重新安装）；传入 Key 为空的组合键会注销当前热键。回调运行在平台的事件
// 处理路径上，因此触碰 UI 的调用方需自行编组到 UI 线程。
type Manager struct {
	mu      sync.Mutex
	combo   Combo
	handle  uintptr
	stop    func()
	onFired func()
	closed  bool
}

// NewManager creates a Manager with the given press callback.
// NewManager 以给定按键回调创建 Manager。
func NewManager(onFired func()) *Manager {
	return &Manager{onFired: onFired}
}

// Register installs (or re-installs) the hotkey described by combo. It returns
// ErrUnsupported on platforms without a backend. A combo with an empty Key is
// treated as "no hotkey" and simply clears any active registration.
//
// Register 安装（或重新安装）combo 描述的热键。无后端的平台返回 ErrUnsupported。
// Key 为空的组合键视为「无热键」，仅清除当前注册。
func (m *Manager) Register(combo Combo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("hotkey: manager is closed")
	}
	if m.stop != nil {
		m.stop()
		m.stop = nil
	}
	m.handle = 0
	m.combo = Combo{}
	if combo.Key == "" {
		return nil
	}
	h, stop, err := platformRegister(combo, m.onFired)
	if err != nil {
		return err
	}
	m.handle = h
	m.stop = stop
	m.combo = combo
	return nil
}

// Unregister removes the active hotkey (if any).
// Unregister 移除当前热键（若存在）。
func (m *Manager) Unregister() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stop != nil {
		m.stop()
		m.stop = nil
	}
	m.handle = 0
	m.combo = Combo{}
	return nil
}

// Close tears the manager down for good (used on app shutdown). After Close,
// Register returns an error.
// Close 彻底拆除 Manager（用于应用退出）。Close 之后 Register 将返回错误。
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	if m.stop != nil {
		m.stop()
		m.stop = nil
	}
	m.handle = 0
	m.combo = Combo{}
	return nil
}
