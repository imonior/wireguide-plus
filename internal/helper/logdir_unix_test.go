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
