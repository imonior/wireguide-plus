package helper

import (
	"testing"
	"time"

	"github.com/imonior/wireguide-plus/internal/domain"
	"github.com/imonior/wireguide-plus/internal/storage"
)

// TestEffectiveCooldown: the cooldown is always floored at minEndpointCooldown
// (240s) so it never drops below the 180s dead-link action window.
// TestEffectiveCooldown：冷却恒不低于下限 240s，从而永远高于 180s 死链动作窗口。
func TestEffectiveCooldown(t *testing.T) {
	cases := []struct {
		in   int
		want time.Duration
	}{
		{0, 300 * time.Second},   // unset → Effective defaults to 300
		{100, 300 * time.Second}, // below the floor → Effective defaults to 300
		{240, 240 * time.Second}, // explicit floor is reachable
		{300, 300 * time.Second},
		{600, 600 * time.Second},
	}
	for _, c := range cases {
		s := &storage.Settings{EndpointCooldownSec: c.in}
		if got := effectiveCooldown(s); got != c.want {
			t.Fatalf("cooldown in=%d want %v got %v", c.in, c.want, got)
		}
	}
}

// TestConfirmImprovement_ZeroWindow: a zero sustained window commits
// immediately and leaves no pending entry behind.
// TestConfirmImprovement_ZeroWindow：零窗口立即提交且不留待定项。
func TestConfirmImprovement_ZeroWindow(t *testing.T) {
	delete(pendingImprovements.m, "t-zero")
	if !confirmImprovement("t-zero", "b:51820", 0) {
		t.Fatal("zero window should commit immediately")
	}
	if _, ok := pendingImprovements.m["t-zero"]; ok {
		t.Fatal("zero window should not leave a pending entry")
	}
}

// TestConfirmImprovement_FirstSightingWaits: the first time an improvement is
// proposed it must wait and record the candidate.
// TestConfirmImprovement_FirstSightingWaits：首次见到择优提案应等待并记录候选。
func TestConfirmImprovement_FirstSightingWaits(t *testing.T) {
	delete(pendingImprovements.m, "t-first")
	if confirmImprovement("t-first", "b:51820", 30) {
		t.Fatal("first sighting should wait")
	}
	prev, ok := pendingImprovements.m["t-first"]
	if !ok || prev.candidate != "b:51820" {
		t.Fatal("first sighting should record candidate")
	}
}

// TestConfirmImprovement_ConfirmedAcrossWindow: the same candidate proposed
// again after the sustained window commits and clears the entry.
// TestConfirmImprovement_ConfirmedAcrossWindow：同一候选在窗口后再被提案应提交并清理。
func TestConfirmImprovement_ConfirmedAcrossWindow(t *testing.T) {
	name := "t-window"
	delete(pendingImprovements.m, name)
	pendingImprovements.m[name] = pendingImprovement{
		candidate: "b:51820",
		at:        time.Now().Add(-35 * time.Second),
	}
	if !confirmImprovement(name, "b:51820", 30) {
		t.Fatal("same candidate after window should commit")
	}
	if _, ok := pendingImprovements.m[name]; ok {
		t.Fatal("committed entry should be cleared")
	}
}

// TestConfirmImprovement_DifferentCandidateResets: a different candidate even
// after the old one elapsed must wait, and replaces the pending entry.
// TestConfirmImprovement_DifferentCandidateResets：不同候选即便旧的已超时也要等，并替换待定项。
func TestConfirmImprovement_DifferentCandidateResets(t *testing.T) {
	name := "t-reset"
	delete(pendingImprovements.m, name)
	pendingImprovements.m[name] = pendingImprovement{
		candidate: "b:51820",
		at:        time.Now().Add(-35 * time.Second),
	}
	if confirmImprovement(name, "c:51820", 30) {
		t.Fatal("different candidate should wait even if old elapsed")
	}
	prev := pendingImprovements.m[name]
	if prev.candidate != "c:51820" || time.Since(prev.at) > time.Second {
		t.Fatal("different candidate should replace and reset timer")
	}
}

// TestSwitchTracker_Allow: the first switch is allowed, an immediate second one
// within the cooldown is blocked.
// TestSwitchTracker_Allow：首次切换放行，冷却期内紧接的第二次被拦。
func TestSwitchTracker_Allow(t *testing.T) {
	endpointSwitches.mu.Lock()
	delete(endpointSwitches.at, "t-track")
	endpointSwitches.mu.Unlock()
	if !endpointSwitches.allow("t-track", minEndpointCooldown) {
		t.Fatal("first switch should be allowed")
	}
	if endpointSwitches.allow("t-track", minEndpointCooldown) {
		t.Fatal("immediate second switch within cooldown should be blocked")
	}
}

// TestFirstPeerEndpoint: returns the first peer's endpoint; empty config
// yields "".
// TestFirstPeerEndpoint：返回第一个 peer 的 endpoint；空配置返回 ""。
func TestFirstPeerEndpoint(t *testing.T) {
	cfg := &domain.WireGuardConfig{}
	if got := firstPeerEndpoint(cfg); got != "" {
		t.Fatalf("empty config want empty, got %q", got)
	}
	cfg.Peers = []domain.PeerConfig{{Endpoint: "a:51820"}, {Endpoint: "b:51820"}}
	if got := firstPeerEndpoint(cfg); got != "a:51820" {
		t.Fatalf("want first peer endpoint a:51820, got %q", got)
	}
}
