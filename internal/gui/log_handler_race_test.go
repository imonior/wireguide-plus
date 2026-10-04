package gui

import (
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestSetGUILogFileConcurrentWithLogging covers a re-bind of GUI file logging
// while records are still flowing: setGUILogFile writes guiLogHandler.file
// under mu and closes the handler it replaced, so a reader on the record path
// must take the same lock or it can both race the swap and write into a closed
// DailyHandler. Any startup path that re-points file logging hits this — the
// health monitor's ticks are concurrent with everything that logs.
func TestSetGUILogFileConcurrentWithLogging(t *testing.T) {
	previous := slog.Default()
	installGUILogHandler()
	t.Cleanup(func() {
		slog.SetDefault(previous)
		guiLogRefMu.Lock()
		h := guiLogRef
		guiLogRef = nil
		guiLogRefMu.Unlock()
		if h != nil {
			h.mu.Lock()
			old := h.file
			h.file = nil
			h.mu.Unlock()
			if old != nil {
				old.Close()
			}
		}
	})

	logsDir := filepath.Join(t.TempDir(), "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatalf("create logs dir: %v", err)
	}

	var wg sync.WaitGroup
	for writer := 0; writer < 4; writer++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				slog.Info("race probe", "n", i)
			}
		}()
	}
	// Several re-binds: one swap can land between two records and still miss
	// the window, so the test hammers it the way a retrying monitor would.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			setGUILogFile(logsDir)
		}
	}()
	wg.Wait()
}
