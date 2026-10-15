package storage

import "testing"

// TestEndpointSwitchTiming pins the normalization truth table: disarmed is
// always "off" regardless of the stored mode, armed-but-empty falls back to
// the historical reconnect behaviour, and only the exact "dynamic" literal
// opts into proactive switching.
// TestEndpointSwitchTiming 钉住归一化真值表：未启用时无论存储什么模式都是 "off"，
// 已启用但模式为空回落到历史 reconnect 行为，只有精确的 "dynamic" 字面量才
// 选择主动切换。
func TestEndpointSwitchTiming(t *testing.T) {
	cases := []struct {
		name     string
		armed    bool
		mode     string
		expected string
	}{
		{"disarmed with empty mode", false, "", "off"},
		{"disarmed with dynamic mode", false, EndpointSwitchDynamic, "off"},
		{"armed with empty mode", true, "", EndpointSwitchReconnect},
		{"armed with reconnect mode", true, EndpointSwitchReconnect, EndpointSwitchReconnect},
		{"armed with dynamic mode", true, EndpointSwitchDynamic, EndpointSwitchDynamic},
		{"armed with unknown mode", true, "whenever", EndpointSwitchReconnect},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &TunnelMeta{EndpointAutoSwitch: tc.armed, EndpointSwitchMode: tc.mode}
			if got := m.EndpointSwitchTiming(); got != tc.expected {
				t.Fatalf("EndpointSwitchTiming() = %q, want %q", got, tc.expected)
			}
			if got := m.EndpointSwitchDynamic(); got != (tc.expected == EndpointSwitchDynamic) {
				t.Fatalf("EndpointSwitchDynamic() = %v, want %v", got, tc.expected == EndpointSwitchDynamic)
			}
		})
	}
}

// TestEndpointSwitchTimingNilReceiver guards the nil-meta path: every
// accessor must treat a missing sidecar as "off", never panic.
// TestEndpointSwitchTimingNilReceiver 防御 nil-meta 路径：所有访问器都必须把
// 缺失的 sidecar 当作 "off"，绝不 panic。
func TestEndpointSwitchTimingNilReceiver(t *testing.T) {
	var m *TunnelMeta
	if got := m.EndpointSwitchTiming(); got != "off" {
		t.Fatalf("nil meta timing = %q, want off", got)
	}
	if m.EndpointSwitchDynamic() {
		t.Fatal("nil meta must not be dynamic")
	}
}
