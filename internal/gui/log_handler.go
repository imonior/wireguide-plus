package gui

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/imonior/wireguide-plus/internal/ipc"
	"github.com/imonior/wireguide-plus/internal/logging"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// guiLogHandler chains to a stderr handler AND emits a Wails "log" event
// for every record. The frontend LogViewer subscribes to that event, so
// anything the GUI slogs (Wails bootstrap, helper lifecycle monitor,
// event bridge, etc) shows up in the viewer alongside the helper's
// forwarded records.
//
// Level is controlled by a shared slog.LevelVar. SetLogLevel writes to it
// and the change takes effect immediately for subsequent records. Info
// by default.
type guiLogHandler struct {
	levelVar *slog.LevelVar
	stderr   slog.Handler
	file     *logging.DailyHandler // may be nil until setGUILogFile runs
	app      *application.App // may be nil before Wails finishes bootstrap

	mu    sync.Mutex
	attrs []slog.Attr
	// pending buffers records emitted before bindAppToLogHandler so
	// the user-visible log viewer doesn't miss the entire bootstrap
	// (helper spawn, ensureHelper, version checks). Flushed on bind;
	// capped at pendingCap to avoid unbounded growth if Wails fails
	// to bootstrap entirely.
	pending []ipc.LogEntry
}

// pendingCap is the maximum number of pre-Wails-bind log entries we
// retain. Bootstrap on a slow disk + verbose DEBUG level can emit
// several hundred entries (helper spawn, version probe, plist drift
// check, IPC handshake, initial status fetch) — 200 was too tight and
// silently dropped the tail. 1000 covers worst-case verbose paths with
// minimal memory impact (each LogEntry ~150 bytes → ~150 KB ceiling).
const pendingCap = 1000

var guiLogLevel = new(slog.LevelVar)

// guiLogHandlerRef is shared so SetApp() can wire up the Wails app after
// bootstrap finishes. Until then, log records only hit stderr (and are
// lost to the viewer, which matches reality — the viewer isn't up yet).
var (
	guiLogRefMu sync.Mutex
	guiLogRef   *guiLogHandler
)

// installGUILogHandler builds the handler and installs it as slog default.
// Call before the first slog record you want captured.
func installGUILogHandler() {
	stderr := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level:     guiLogLevel,
		AddSource: true,
	})
	h := &guiLogHandler{
		levelVar: guiLogLevel,
		stderr:   stderr,
	}
	guiLogRefMu.Lock()
	guiLogRef = h
	guiLogRefMu.Unlock()
	slog.SetDefault(slog.New(h))
}

// bindAppToLogHandler tells the handler about the Wails app so subsequent
// records can be emitted as events. Called from gui.Run right after
// application.New. Flushes any records buffered before bind so the
// log viewer can display the bootstrap diagnostics.
func bindAppToLogHandler(app *application.App) {
	guiLogRefMu.Lock()
	defer guiLogRefMu.Unlock()
	if guiLogRef == nil {
		return
	}
	guiLogRef.mu.Lock()
	guiLogRef.app = app
	pending := guiLogRef.pending
	guiLogRef.pending = nil
	guiLogRef.mu.Unlock()
	for _, entry := range pending {
		app.Event.Emit("log", entry)
	}
}

// setGUILogLevel updates the threshold for both stderr and the Wails
// broadcast. Mirrors the helper-side SetLogLevel behaviour.
func setGUILogLevel(level string) {
	guiLogLevel.Set(parseGUILevel(level))
}

func parseGUILevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "all":
		// Log everything — lower than the lowest named level so no
		// record is ever dropped, including future trace-level entries.
		return slog.Level(-8)
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func (h *guiLogHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.levelVar.Level()
}

func (h *guiLogHandler) Handle(ctx context.Context, r slog.Record) error {
	_ = h.stderr.Handle(ctx, r)
	if h.file != nil {
		_ = h.file.Handle(ctx, r)
	}

	// The "category" attr is carried separately (entry.Category) and
	// deliberately omitted from the flattened text shown in the viewer.
	var b strings.Builder
	b.WriteString(r.Message)
	h.mu.Lock()
	for _, a := range h.attrs {
		if a.Key == "category" {
			continue
		}
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value.Any())
	}
	app := h.app
	attrs := h.attrs
	h.mu.Unlock()
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "category" {
			return true
		}
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value.Any())
		return true
	})
	if r.PC != 0 {
		fs := runtime.CallersFrames([]uintptr{r.PC})
		if frame, _ := fs.Next(); frame.File != "" {
			fmt.Fprintf(&b, " source=%s:%d", filepath.Base(frame.File), frame.Line)
		}
	}

	entry := ipc.LogEntry{
		Time:     r.Time.UTC().Format(time.RFC3339Nano),
		Level:    strings.ToLower(r.Level.String()),
		Source:   "gui",
		Category: logging.CategoryFromRecord(r, attrs),
		Message:  b.String(),
	}
	if app != nil {
		app.Event.Emit("log", entry)
	} else {
		// App not bound yet — buffer for replay so the bootstrap
		// records actually reach the log viewer.
		h.mu.Lock()
		if len(h.pending) < pendingCap {
			h.pending = append(h.pending, entry)
		}
		h.mu.Unlock()
	}
	return nil
}

func (h *guiLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h.mu.Lock()
	combined := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	combined = append(combined, h.attrs...)
	combined = append(combined, attrs...)
	app := h.app
	h.mu.Unlock()
	return &guiLogHandler{
		levelVar: h.levelVar,
		stderr:   h.stderr.WithAttrs(attrs),
		file:     h.file,
		app:      app,
		attrs:    combined,
	}
}

func (h *guiLogHandler) WithGroup(name string) slog.Handler {
	h.mu.Lock()
	app := h.app
	h.mu.Unlock()
	return &guiLogHandler{
		levelVar: h.levelVar,
		stderr:   h.stderr.WithGroup(name),
		file:     h.file,
		app:      app,
		attrs:    h.attrs,
	}
}

// setGUILogFile starts mirroring every GUI log record into a daily file
// wireguideplus-YYYY-MM-DD.log in logsDir. Called after storage.EnsureDirs
// so the directory is guaranteed to exist; records emitted before this
// point are only in stderr/pending and are not retroactively written.
func setGUILogFile(logsDir string) {
	path := filepath.Join(logsDir, "wireguideplus.log")
	fileHandler := logging.NewDailyHandler(logsDir, "wireguideplus", guiLogLevel)
	guiLogRefMu.Lock()
	if guiLogRef != nil {
		guiLogRef.mu.Lock()
		guiLogRef.file = fileHandler
		guiLogRef.mu.Unlock()
	}
	guiLogRefMu.Unlock()
	slog.Info("file log enabled (daily)", "path", path)
}
