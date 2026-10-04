package app

import (
	"strings"
	"testing"

	"github.com/imonior/wireguide-plus/internal/storage"
	"github.com/imonior/wireguide-plus/internal/update"
	"github.com/imonior/wireguide-plus/internal/wifi"
)

// TestOpenURL_AllowedURL verifies that a valid GitHub URL passes validation.
// With a nil app field, the function reaches the "app not initialized" fallback
// after passing the URL allowlist check — which is the expected path.
func TestOpenURL_AllowedURL(t *testing.T) {
	svc := &TunnelService{} // nil app, nil stores — only URL validation runs
	err := svc.OpenURL("https://github.com/imonior/wireguide-plus")
	if err == nil {
		t.Fatal("expected error (app not initialized) but got nil")
	}
	if !strings.Contains(err.Error(), "app not initialized") {
		t.Errorf("expected 'app not initialized' error, got: %v", err)
	}
}

func TestOpenURL_EvilDomain(t *testing.T) {
	svc := &TunnelService{}
	err := svc.OpenURL("https://evil.com")
	if err == nil {
		t.Fatal("expected error for disallowed URL")
	}
	if !strings.Contains(err.Error(), "URL not allowed") {
		t.Errorf("expected 'URL not allowed' error, got: %v", err)
	}
}

func TestOpenURL_FileScheme(t *testing.T) {
	svc := &TunnelService{}
	err := svc.OpenURL("file:///etc/passwd")
	if err == nil {
		t.Fatal("expected error for file:// URL")
	}
	if !strings.Contains(err.Error(), "URL not allowed") {
		t.Errorf("expected 'URL not allowed' error, got: %v", err)
	}
}

func TestOpenURL_JavascriptScheme(t *testing.T) {
	svc := &TunnelService{}
	err := svc.OpenURL("javascript:alert(1)")
	if err == nil {
		t.Fatal("expected error for javascript: URL")
	}
	if !strings.Contains(err.Error(), "URL not allowed") {
		t.Errorf("expected 'URL not allowed' error, got: %v", err)
	}
}

// TestRunUpdate_NilInfo: RunUpdate must reject a nil/not-available
// UpdateInfo before dispatching to any platform-specific path — with a
// nil app and nil stores, the dispatch would otherwise panic.
func TestRunUpdate_NilInfo(t *testing.T) {
	svc := &TunnelService{}
	err := svc.RunUpdate(nil)
	if err == nil {
		t.Fatal("expected error for nil UpdateInfo")
	}
	if !strings.Contains(err.Error(), "no update available") {
		t.Errorf("expected 'no update available' error, got: %v", err)
	}
}

func TestRunUpdate_NotAvailable(t *testing.T) {
	svc := &TunnelService{}
	err := svc.RunUpdate(&update.UpdateInfo{Available: false, Version: "9.9.9"})
	if err == nil {
		t.Fatal("expected error for unavailable UpdateInfo")
	}
	if !strings.Contains(err.Error(), "no update available") {
		t.Errorf("expected 'no update available' error, got: %v", err)
	}
}

// TestApplySettingsPayloadKeepsBackendOwnedFields pins the merge a
// whole-settings save does: the payload is a snapshot taken before somebody
// else wrote the backend-owned fields, and those writes have to survive the
// save. A plain whole-object write (what this replaced) reverted all five.
func TestApplySettingsPayloadKeepsBackendOwnedFields(t *testing.T) {
	store := storage.NewSettingsStore(t.TempDir())

	// What landed on disk after the frontend loaded its copy.
	if err := store.Update(func(st *storage.Settings) error {
		st.SetManualOff("wg-a")
		st.SetManualOn("wg-b")
		st.DNSTestPublicServers = []string{"1.1.1.1"}
		st.DNSTestPublicFetched = []string{"9.9.9.9"}
		st.DNSTestPublicFetchedAt = 1234
		st.LogLevel = "info"
		return nil
	}); err != nil {
		t.Fatalf("seed disk state: %v", err)
	}

	disk, err := store.Load()
	if err != nil {
		t.Fatalf("load seed: %v", err)
	}
	// The stale snapshot the save carries: the five fields as they were before
	// the writes above, plus the one edit the user really made.
	payload := *disk
	payload.ManualOffTunnels = nil
	payload.ManualOnTunnels = nil
	payload.DNSTestPublicServers = nil
	payload.DNSTestPublicFetched = nil
	payload.DNSTestPublicFetchedAt = 0
	payload.LogLevel = "debug"

	if err := store.Update(func(cur *storage.Settings) error {
		applySettingsPayload(cur, &payload)
		return nil
	}); err != nil {
		t.Fatalf("save payload: %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("load after save: %v", err)
	}
	if got.LogLevel != "debug" {
		t.Errorf("UI-owned field did not land: log_level=%q", got.LogLevel)
	}
	if strings.Join(got.ManualOffTunnels, ",") != "wg-a" {
		t.Errorf("manual-off latch rolled back: %v", got.ManualOffTunnels)
	}
	if strings.Join(got.ManualOnTunnels, ",") != "wg-b" {
		t.Errorf("manual-on latch rolled back: %v", got.ManualOnTunnels)
	}
	if strings.Join(got.DNSTestPublicServers, ",") != "1.1.1.1" {
		t.Errorf("custom resolvers rolled back: %v", got.DNSTestPublicServers)
	}
	if strings.Join(got.DNSTestPublicFetched, ",") != "9.9.9.9" {
		t.Errorf("fetched resolvers rolled back: %v", got.DNSTestPublicFetched)
	}
	if got.DNSTestPublicFetchedAt != 1234 {
		t.Errorf("fetch timestamp rolled back: %d", got.DNSTestPublicFetchedAt)
	}

	// Resurrect-on-clear. The user released every latch, so the disk list is
	// now empty while the payload still lists tunnels. The empty list must
	// win, otherwise wg-a is switched off again by the next automation pass
	// even though the user un-latched it by hand.
	if err := store.Update(func(st *storage.Settings) error {
		st.ClearAllManualOverrides()
		return nil
	}); err != nil {
		t.Fatalf("clear latches: %v", err)
	}
	stale := payload
	if err := store.Update(func(cur *storage.Settings) error {
		applySettingsPayload(cur, &stale)
		return nil
	}); err != nil {
		t.Fatalf("save payload after clear: %v", err)
	}
	got, err = store.Load()
	if err != nil {
		t.Fatalf("load after clear save: %v", err)
	}
	if len(got.ManualOffTunnels) != 0 || len(got.ManualOnTunnels) != 0 {
		t.Errorf("cleared latches resurrected: off=%v on=%v",
			got.ManualOffTunnels, got.ManualOnTunnels)
	}
}

// TestApplySettingsPayloadKeepsAutomationFromPayload asserts the deliberate
// other half of the merge: Automation is payload-owned, because a policy can
// arrive through a whole-settings save rather than SaveAutomationRules
// (SaveSettings keys an immediate re-evaluation off that field).
func TestApplySettingsPayloadKeepsAutomationFromPayload(t *testing.T) {
	store := storage.NewSettingsStore(t.TempDir())
	if err := store.Update(func(st *storage.Settings) error {
		st.Automation = &wifi.Automation{
			Defaults: map[string]wifi.Action{"wg-old": wifi.ActionDisconnect},
		}
		return nil
	}); err != nil {
		t.Fatalf("seed disk automation: %v", err)
	}

	payload := &storage.Settings{
		Automation: &wifi.Automation{
			Defaults: map[string]wifi.Action{"wg-new": wifi.ActionConnect},
		},
	}
	if err := store.Update(func(cur *storage.Settings) error {
		applySettingsPayload(cur, payload)
		return nil
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Automation == nil || got.Automation.Defaults["wg-new"] != wifi.ActionConnect {
		t.Errorf("payload automation was not persisted: %+v", got.Automation)
	}
	if _, still := got.Automation.Defaults["wg-old"]; still {
		t.Errorf("disk automation was kept instead of the payload's: %+v", got.Automation.Defaults)
	}
}
