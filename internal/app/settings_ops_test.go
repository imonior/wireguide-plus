package app

import (
	"strings"
	"testing"

	"github.com/imonior/wireguide-plus/internal/update"
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
