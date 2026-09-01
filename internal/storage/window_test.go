package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowStateSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	store := NewWindowStateStore(dir)

	want := &WindowState{X: 180, Y: 96, Width: 1280, Height: 800}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got == nil {
		t.Fatal("Load returned nil state")
	}
	if *got != *want {
		t.Fatalf("round-trip mismatch: got %+v, want %+v", *got, *want)
	}

	// Saved file must be regular window.json in the config dir.
	if _, err := os.Stat(filepath.Join(dir, "window.json")); err != nil {
		t.Fatalf("window.json not created: %v", err)
	}
}

func TestWindowStateLoadMissing(t *testing.T) {
	store := NewWindowStateStore(t.TempDir())
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load on missing file should not error, got: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil state on first run, got %+v", got)
	}
}

func TestWindowStateSaveInvalid(t *testing.T) {
	dir := t.TempDir()
	store := NewWindowStateStore(dir)

	// A degenerate state must not be persisted (and must not error).
	if err := store.Save(nil); err != nil {
		t.Fatalf("Save(nil) errored: %v", err)
	}
	if err := store.Save(&WindowState{X: 1, Y: 1, Width: 0, Height: 0}); err != nil {
		t.Fatalf("Save(degenerate) errored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "window.json")); !os.IsNotExist(err) {
		t.Fatalf("window.json should not exist after degenerate saves, stat err: %v", err)
	}
}

func TestWindowStateLoadCorrupt(t *testing.T) {
	dir := t.TempDir()
	store := NewWindowStateStore(dir)
	if err := os.WriteFile(filepath.Join(dir, "window.json"), []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil {
		t.Fatal("expected error for corrupt window state file")
	}
}
