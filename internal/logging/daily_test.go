package logging

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCleanupOldLogs(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	mustWrite := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Same family: keep recent, delete old.
	mustWrite("wireguideplus-" + now.AddDate(0, 0, -3).Format("2006-01-02") + ".log")
	mustWrite("wireguideplus-" + now.AddDate(0, 0, -9).Format("2006-01-02") + ".log")
	mustWrite("wireguideplus-" + now.AddDate(0, 0, -30).Format("2006-01-02") + ".log")
	// Other family: must be untouched.
	mustWrite("helper-" + now.AddDate(0, 0, -30).Format("2006-01-02") + ".log")
	// Non-daily file: untouched.
	mustWrite("wireguideplus.log")
	// Wrong prefix: untouched.
	mustWrite("backup-" + now.AddDate(0, 0, -30).Format("2006-01-02") + ".log")

	removed, err := CleanupOldLogs(dir, "wireguideplus", 7)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 2 {
		t.Fatalf("removed = %d, want 2", removed)
	}
	for _, name := range []string{
		"wireguideplus-" + now.AddDate(0, 0, -3).Format("2006-01-02") + ".log",
		"helper-" + now.AddDate(0, 0, -30).Format("2006-01-02") + ".log",
		"wireguideplus.log",
		"backup-" + now.AddDate(0, 0, -30).Format("2006-01-02") + ".log",
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("file %s should still exist: %v", name, err)
		}
	}
}

func TestCleanupOldLogsDisabled(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wireguideplus-2000-01-01.log"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	n, err := CleanupOldLogs(dir, "wireguideplus", 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("retention 0 must disable cleanup, removed %d", n)
	}
}

func TestDailyHandlerRotatesAtMidnight(t *testing.T) {
	dir := t.TempDir()
	h := NewDailyHandler(dir, "helper", slog.LevelInfo)

	today := time.Now()
	yesterday := today.AddDate(0, 0, -1)

	if err := h.Handle(context.Background(), slog.NewRecord(today, slog.LevelInfo, "today", 0)); err != nil {
		t.Fatal(err)
	}
	if err := h.Handle(context.Background(), slog.NewRecord(yesterday, slog.LevelInfo, "yesterday", 0)); err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 daily files, got %d", len(entries))
	}
	for _, e := range entries {
		content, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case strings.HasSuffix(e.Name(), today.Format("2006-01-02")+".log"):
			if !strings.Contains(string(content), "today") {
				t.Errorf("today's file missing 'today' record")
			}
		case strings.HasSuffix(e.Name(), yesterday.Format("2006-01-02")+".log"):
			if !strings.Contains(string(content), "yesterday") {
				t.Errorf("yesterday's file missing 'yesterday' record")
			}
		default:
			t.Errorf("unexpected file %s", e.Name())
		}
	}
}

func TestCategoryFromRecord(t *testing.T) {
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "hello", 0)
	r.AddAttrs(slog.String("category", "update"))
	if got := CategoryFromRecord(r, nil); got != "update" {
		t.Fatalf("category = %q, want update", got)
	}

	// No category attr anywhere → "app".
	if got := CategoryFromRecord(slog.NewRecord(time.Now(), slog.LevelInfo, "hello", 0), nil); got != "app" {
		t.Fatalf("category = %q, want app", got)
	}

	// Attrs captured via WithAttrs (the handler-level attrs) are honored
	// when the record itself carries no category.
	attrs := []slog.Attr{slog.String("category", "settings")}
	if got := CategoryFromRecord(slog.NewRecord(time.Now(), slog.LevelInfo, "hello", 0), attrs); got != "settings" {
		t.Fatalf("category = %q, want settings", got)
	}
}
