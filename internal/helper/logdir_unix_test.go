//go:build darwin || linux

package helper

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func statUID(t *testing.T, path string) int {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("unexpected stat type %T for %s", fi.Sys(), path)
	}
	return int(st.Uid)
}

func TestPrepareLogsDirCreatesOwnedDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	if err := prepareLogsDir(dir, os.Getuid()); err != nil {
		t.Fatalf("prepareLogsDir: %v", err)
	}
	if got := statUID(t, dir); got != os.Getuid() {
		t.Errorf("created dir uid = %d, want %d", got, os.Getuid())
	}
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o700 {
		t.Errorf("created dir perm = %o, want 700", perm)
	}
}

func TestPrepareLogsDirRepairsExistingDir(t *testing.T) {
	dir := t.TempDir()
	if err := prepareLogsDir(dir, os.Getuid()); err != nil {
		t.Fatalf("prepareLogsDir on existing dir: %v", err)
	}
	if got := statUID(t, dir); got != os.Getuid() {
		t.Errorf("existing dir uid = %d, want %d", got, os.Getuid())
	}
}

// TestPrepareLogsDirRepairsContents verifies that prepareLogsDir walks the
// directory and chowns the log files it finds back to the target uid, not just
// the top-level directory. DailyHandler writes flat <prefix>-YYYY-MM-DD.log
// files, so those are the entries that matter in practice; a nested directory
// is created too so the walk is proven to recurse rather than only handle the
// top level. This is what lets a running helper re-heal a log directory it
// populated with root-owned files between starts.
func TestPrepareLogsDirRepairsContents(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "helper-2026-10-03.log")
	if err := os.WriteFile(logFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "nested")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	nestedFile := filepath.Join(sub, "helper-2026-10-02.log")
	if err := os.WriteFile(nestedFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := prepareLogsDir(dir, os.Getuid()); err != nil {
		t.Fatalf("prepareLogsDir: %v", err)
	}
	if got := statUID(t, dir); got != os.Getuid() {
		t.Errorf("dir uid = %d, want %d", got, os.Getuid())
	}
	if got := statUID(t, logFile); got != os.Getuid() {
		t.Errorf("log file uid = %d, want %d", got, os.Getuid())
	}
	if got := statUID(t, sub); got != os.Getuid() {
		t.Errorf("nested dir uid = %d, want %d", got, os.Getuid())
	}
	if got := statUID(t, nestedFile); got != os.Getuid() {
		t.Errorf("nested log file uid = %d, want %d", got, os.Getuid())
	}
}
