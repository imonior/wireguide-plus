// Package logging provides daily-rotating slog file handlers and
// retention-based cleanup for WireGuide's log files.
//
// Design: one file per calendar day, named <prefix>-YYYY-MM-DD.log.
// A record written just before midnight and one just after land in
// different files, which makes "delete logs older than N days" trivial:
// retention is decided by the date embedded in the filename, not by
// fragile mtime heuristics.
package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// DefaultRetentionDays is the default number of daily log files to keep.
const DefaultRetentionDays = 7

// ValidCategories is the closed set of category values used across the
// codebase. The frontend LogViewer filters on this list; keep it in sync
// when adding a new category.
var ValidCategories = []string{
	"app",      // general / unclassified
	"update",   // update checks, proxy/mirror, downloads
	"settings", // settings saves and applied changes
	"tunnel",   // connect/disconnect, scripts, health
	"network",  // firewall, DNS protection, interfaces
	"system",   // lifecycle, crash recovery, spawn
}

// DailyHandler is a slog.Handler that appends each record to
// <dir>/<prefix>-YYYY-MM-DD.log, switching files at local midnight.
// Thread-safe: slog may call Handle from any goroutine.
type DailyHandler struct {
	mu     sync.Mutex
	dir    string
	prefix string
	level  slog.Leveler
	file   *os.File
	day    string // "YYYY-MM-DD" of the currently-open file
	attrs  []slog.Attr
}

// NewDailyHandler creates a handler writing to <dir>/<prefix>-<date>.log.
// The file is created lazily on the first Handle so an empty dir is fine.
func NewDailyHandler(dir, prefix string, level slog.Leveler) *DailyHandler {
	return &DailyHandler{
		dir:    dir,
		prefix: prefix,
		level:  level,
	}
}

func (h *DailyHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level.Level()
}

func (h *DailyHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.ensureOpen(r.Time); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString(r.Time.Format("2006-01-02 15:04:05.000"))
	fmt.Fprintf(&b, "\t%s", strings.ToUpper(r.Level.String()))
	fmt.Fprintf(&b, "\t%s", r.Message)
	for _, a := range h.attrs {
		if a.Key == "category" {
			continue // already rendered by CategoryFromRecord via caller? no — keep it simple: include attrs verbatim below
		}
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value.Any())
	}
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value.Any())
		return true
	})
	if r.PC != 0 {
		fs := runtime.CallersFrames([]uintptr{r.PC})
		if frame, _ := fs.Next(); frame.File != "" {
			fmt.Fprintf(&b, " source=%s:%d", filepath.Base(frame.File), frame.Line)
		}
	}
	b.WriteByte('\n')
	_, err := h.file.WriteString(b.String())
	return err
}

// ensureOpen opens (or switches to) the file for the record's local date.
// Callers must hold h.mu.
func (h *DailyHandler) ensureOpen(t time.Time) error {
	day := t.Format("2006-01-02")
	if h.file != nil && h.day == day {
		return nil
	}
	if h.file != nil {
		_ = h.file.Close()
		h.file = nil
	}
	if err := os.MkdirAll(h.dir, 0o700); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}
	path := filepath.Join(h.dir, fmt.Sprintf("%s-%s.log", h.prefix, day))
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open log file %s: %w", path, err)
	}
	h.file = f
	h.day = day
	return nil
}

// Close flushes and closes the current log file. Safe to call multiple
// times; nil after first close.
func (h *DailyHandler) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.file == nil {
		return nil
	}
	err := h.file.Close()
	h.file = nil
	h.day = ""
	return err
}

func (h *DailyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h.mu.Lock()
	combined := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	combined = append(combined, h.attrs...)
	combined = append(combined, attrs...)
	h.mu.Unlock()
	return &DailyHandler{
		dir:    h.dir,
		prefix: h.prefix,
		level:  h.level,
		attrs:  combined,
	}
}

func (h *DailyHandler) WithGroup(name string) slog.Handler {
	return &DailyHandler{
		dir:    h.dir,
		prefix: h.prefix,
		level:  h.level,
		attrs:  h.attrs,
	}
}

// CategoryFromRecord extracts the "category" attr from a record (and any
// WithAttrs-captured attrs). Returns "app" when absent — every LogEntry
// exposed to the frontend gets a classification so the LogViewer can
// filter consistently.
func CategoryFromRecord(r slog.Record, attrs []slog.Attr) string {
	category := ""
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "category" && a.Value.Kind() != slog.KindGroup {
			category = a.Value.String()
			return false
		}
		return true
	})
	if category != "" {
		return category
	}
	for _, a := range attrs {
		if a.Key == "category" && a.Value.Kind() != slog.KindGroup {
			return a.Value.String()
		}
	}
	return "app"
}

// dailyLogNameRe matches <prefix>-YYYY-MM-DD.log. Group 1 is the prefix,
// group 2 the date.
var dailyLogNameRe = regexp.MustCompile(`^(.+)-(\d{4}-\d{2}-\d{2})\.log$`)

// CleanupOldLogs deletes daily log files in dir whose embedded date is
// older than retentionDays. Only files whose prefix matches are touched,
// so unrelated files in the same directory are left alone. retentionDays
// <= 0 disables cleanup. Returns the number of files removed.
func CleanupOldLogs(dir, prefix string, retentionDays int) (int, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays).Format("2006-01-02")
	removed := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := dailyLogNameRe.FindStringSubmatch(e.Name())
		if len(m) != 3 || m[1] != prefix {
			continue
		}
		if m[2] < cutoff { // ISO dates compare lexicographically
			if err := os.Remove(filepath.Join(dir, e.Name())); err == nil {
				removed++
			}
		}
	}
	return removed, nil
}
