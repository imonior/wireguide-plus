package app

import (
	"testing"

	"github.com/imonior/wireguide-plus/internal/ipc"
	"github.com/imonior/wireguide-plus/internal/storage"
)

// TestClearManualOverride_ReleasesBothLatches pins the UI's "restore
// automation" contract: whichever direction the user overrode automation
// (manual-off from a hand disconnect, manual-on from a hand connect), one
// call hands that tunnel back to its rules.
// TestClearManualOverride_ReleasesBothLatches 钉住界面「恢复自动规则」的契约：
// 不论用户当初往哪个方向接管（手动断开留下的 manual-off，还是手动连接留下的
// manual-on），一次调用即把该隧道交还给它的规则。
func TestClearManualOverride_ReleasesBothLatches(t *testing.T) {
	store := storage.NewSettingsStore(t.TempDir())
	svc := &TunnelService{settingsStore: store}
	if err := store.Update(func(cfg *storage.Settings) error {
		cfg.SetManualOff("wg-off")
		cfg.SetManualOn("wg-on")
		return nil
	}); err != nil {
		t.Fatalf("seed latches: %v", err)
	}
	for _, name := range []string{"wg-off", "wg-on"} {
		if err := svc.ClearManualOverride(name); err != nil {
			t.Fatalf("ClearManualOverride(%s): %v", name, err)
		}
	}
	got, err := svc.GetManualOverrides()
	if err != nil {
		t.Fatalf("GetManualOverrides: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("latches survived the restore: %+v", got)
	}
}

// TestClearManualOverride_RequestsReevaluation is the regression guard for
// the "I clicked restore and nothing happened" report: releasing the latch
// on disk is not enough. The helper's engine is event-driven (macOS has no
// 30s poll at all), so without an explicit re-evaluation request the rules
// that should reconnect the tunnel stayed suppressed until the next network
// event — which on a steady network never comes.
// TestClearManualOverride_RequestsReevaluation 是「点了恢复却没反应」的回归防线：
// 仅把闩锁从磁盘上释放并不够。helper 的引擎是事件驱动的（macOS 根本没有 30s
// 轮询），少了显式的重评请求，本该把隧道连回去的规则会一直处于抑制状态，直到
// 下一次网络事件——而网络不变时这一天可能永不到来。
func TestClearManualOverride_RequestsReevaluation(t *testing.T) {
	orig := automationEvalRequest
	defer func() { automationEvalRequest = orig }()
	asks := 0
	automationEvalRequest = func(s *TunnelService) error {
		asks++
		return nil
	}

	store := storage.NewSettingsStore(t.TempDir())
	// A holder with no live client stands in for "helper unreachable": the
	// request is best-effort and must still have been attempted.
	// 没有活跃 client 的 holder 代表「helper 连不上」：请求是尽力而为，但必须
	// 确实发起过。
	svc := &TunnelService{settingsStore: store, clients: ipc.NewClientHolder(nil)}
	if err := store.Update(func(cfg *storage.Settings) error {
		cfg.SetManualOff("wg-a")
		return nil
	}); err != nil {
		t.Fatalf("seed latch: %v", err)
	}
	if err := svc.ClearManualOverride("wg-a"); err != nil {
		t.Fatalf("ClearManualOverride: %v", err)
	}
	if asks != 1 {
		t.Fatalf("expected exactly one re-evaluation request, got %d", asks)
	}
}

// TestClearManualOverride_RejectsBadInput keeps the two guard clauses: an
// empty name and a missing settings store must both fail loudly instead of
// silently clearing nothing (or panicking on the best-effort eval ask).
// TestClearManualOverride_RejectsBadInput 守住两条守卫：空隧道名与缺失的设置存储
// 都必须明确报错，而不是静默什么都不清（或在尽力而为的重评请求上 panic）。
func TestClearManualOverride_RejectsBadInput(t *testing.T) {
	svc := &TunnelService{settingsStore: storage.NewSettingsStore(t.TempDir())}
	if err := svc.ClearManualOverride(""); err == nil {
		t.Fatal("expected an error for an empty tunnel name")
	}
	noStore := &TunnelService{}
	if err := noStore.ClearManualOverride("wg-a"); err == nil {
		t.Fatal("expected an error when the settings store is unavailable")
	}
}
