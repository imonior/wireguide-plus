package storage

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// WindowState is the persisted geometry of the main GUI window. The GUI
// saves it whenever the window is closed-to-tray, minimised, or the app
// quits, and restores it on the next launch so the window reappears
// exactly where the user left it.
type WindowState struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// WindowStateStore persists WindowState to <configDir>/window.json.
//
// Unlike SettingsStore it needs no cross-process file lock: window
// geometry is GUI-only state — the `ctl` CLI never touches it, and a
// single GUI process is the only writer. Writes are still atomic
// (temp file + rename), so a crash mid-write can never corrupt the
// previously saved state.
type WindowStateStore struct {
	mu   sync.Mutex
	path string
}

// NewWindowStateStore creates a store for the given config directory.
func NewWindowStateStore(configDir string) *WindowStateStore {
	return &WindowStateStore{path: filepath.Join(configDir, "window.json")}
}

// Load reads the last saved window geometry. Returns (nil, nil) when no
// state has been saved yet (first run) — callers should fall back to
// their default window size. A corrupt file yields (nil, err); the
// caller can simply ignore it and use defaults.
func (s *WindowStateStore) Load() (*WindowState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var st WindowState
	if err := json.Unmarshal(data, &st); err != nil {
		slog.Warn("window state file is corrupt, ignoring", "path", s.path, "error", err)
		return nil, err
	}
	return &st, nil
}

// Save atomically persists the window geometry. Degenerate states (nil,
// non-positive size) are silently dropped so a bad read can never
// overwrite the last good geometry.
func (s *WindowStateStore) Save(st *WindowState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st == nil || st.Width <= 0 || st.Height <= 0 {
		return nil
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	tmpFile, err := os.CreateTemp(filepath.Dir(s.path), ".wireguide-window-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := atomicRenameDurable(tmpPath, s.path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}
