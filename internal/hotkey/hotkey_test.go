//go:build darwin || windows

package hotkey

import "testing"

func TestParseCombo(t *testing.T) {
	cases := []struct {
		in     string
		mods   int
		key    string
		hasErr bool
	}{
		{in: "Ctrl+Alt+Space", mods: ModCtrl | ModAlt, key: "Space"},
		{in: "Win+Shift+T", mods: ModWin | ModShift, key: "T"},
		{in: "Ctrl+KeyT", mods: ModCtrl, key: "T"},                   // Key-prefixed letter
		{in: "Alt+Digit1", mods: ModAlt, key: "1"},                   // Digit-prefixed number
		{in: "ctrl+alt+Space", mods: ModCtrl | ModAlt, key: "Space"}, // case-insensitive mods, canonical key code
		{in: "Cmd+K", mods: ModWin, key: "K"},                        // Cmd alias
		{in: "Control+Option+F12", mods: ModCtrl | ModAlt, key: "F12"},
		{in: "  Shift + Comma  ", mods: ModShift, key: "Comma"}, // surrounding whitespace
		// Error cases.
		{in: "", hasErr: true},
		{in: "   ", hasErr: true},
		{in: "Space", hasErr: true},       // no modifier
		{in: "Foo+Space", hasErr: true},   // unknown modifier
		{in: "Ctrl+", hasErr: true},       // empty key segment
		{in: "Ctrl++Space", hasErr: true}, // empty middle segment
		{in: "Ctrl+ ", hasErr: true},      // whitespace-only key
	}
	for _, c := range cases {
		got, err := ParseCombo(c.in)
		if c.hasErr {
			if err == nil {
				t.Errorf("ParseCombo(%q) expected error, got %+v", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseCombo(%q) unexpected error: %v", c.in, err)
			continue
		}
		if got.Modifiers != c.mods || got.Key != c.key {
			t.Errorf("ParseCombo(%q) = {mods:%d key:%q}, want {mods:%d key:%q}",
				c.in, got.Modifiers, got.Key, c.mods, c.key)
		}
	}
}

func TestNormalizeKeyToken(t *testing.T) {
	cases := map[string]string{
		"KeyT":    "T",
		"KeyA":    "A",
		"Digit1":  "1",
		"Digit0":  "0",
		"Space":   "Space",
		"F12":     "F12",
		"ArrowUp": "ArrowUp",
		"Comma":   "Comma",
		"Key":     "Key",   // not length 4 -> kept as-is
		"Digit":   "Digit", // not length 6 -> kept as-is
		"":        "",
		"  KeyZ ": "Z",
	}
	for in, want := range cases {
		if got := normalizeKeyToken(in); got != want {
			t.Errorf("normalizeKeyToken(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestManagerEmptyKeyNoRegister verifies that registering an empty-key combo is
// a no-op (clears any active hotkey) and never touches the platform backend,
// so it is safe to assert without a run loop / message pump.
// TestManagerEmptyKeyNoRegister 验证注册空按键组合键是空操作（清除当前热键），
// 且绝不触碰平台后端，因此在没有运行循环/消息泵时也能安全断言。
func TestManagerEmptyKeyNoRegister(t *testing.T) {
	m := NewManager(func() {})
	if err := m.Register(Combo{}); err != nil {
		t.Fatalf("Register(empty) unexpected error: %v", err)
	}
	if m.handle != 0 || m.stop != nil {
		t.Errorf("Register(empty) left stale state: handle=%d hasStop=%v", m.handle, m.stop != nil)
	}
}

// TestManagerClosedRejectsRegister verifies Close is terminal: a later Register
// returns an error instead of touching the platform backend.
// TestManagerClosedRejectsRegister 验证 Close 是终态：后续 Register 返回错误，
// 而不会去触碰平台后端。
func TestManagerClosedRejectsRegister(t *testing.T) {
	m := NewManager(func() {})
	if err := m.Close(); err != nil {
		t.Fatalf("Close() unexpected error: %v", err)
	}
	if err := m.Register(Combo{Modifiers: ModCtrl, Key: "Space"}); err == nil {
		t.Error("Register after Close expected error, got nil")
	}
}
