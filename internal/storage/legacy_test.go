package storage

import (
	"os"
	"path/filepath"
	"testing"
)

// testMigrationPaths builds a current Paths tree plus a legacy config/logs
// tree in temp dirs, so detectLegacyData/migrateLegacyData can be exercised
// without touching the real user home.
func testMigrationPaths(t *testing.T) (current *Paths, legacyConfig, legacyLogs string) {
	t.Helper()
	root := t.TempDir()
	current = &Paths{
		ConfigDir:  filepath.Join(root, "current", "config"),
		TunnelsDir: filepath.Join(root, "current", "tunnels"),
		LogsDir:    filepath.Join(root, "current", "logs"),
		DataDir:    filepath.Join(root, "current", "data"),
	}
	legacyConfig = filepath.Join(root, "legacy", "config")
	legacyLogs = filepath.Join(root, "legacy", "logs")
	for _, d := range []string{current.ConfigDir, current.TunnelsDir, current.LogsDir, legacyConfig, filepath.Join(legacyConfig, "tunnels"), legacyLogs} {
		if err := os.MkdirAll(d, 0700); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	return current, legacyConfig, legacyLogs
}

func seedLegacyFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestDetectLegacyDataNothing(t *testing.T) {
	current, legacyConfig, legacyLogs := testMigrationPaths(t)
	// Empty legacy dirs → not found.
	report, err := detectLegacyData(current, legacyConfig, legacyLogs)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if report.Found {
		t.Fatal("expected Found=false for empty legacy dirs")
	}
	if len(report.Items) != 0 {
		t.Fatalf("expected no items, got %d", len(report.Items))
	}
}

func TestDetectLegacyDataFull(t *testing.T) {
	current, legacyConfig, legacyLogs := testMigrationPaths(t)
	seedLegacyFile(t, filepath.Join(legacyConfig, "config.json"), `{"language":"ko"}`)
	seedLegacyFile(t, filepath.Join(legacyConfig, "history.json"), `[]`)
	seedLegacyFile(t, filepath.Join(legacyConfig, "tunnels", "office.conf"), "[Interface]\n")
	seedLegacyFile(t, filepath.Join(legacyConfig, "tunnels", "home.conf"), "[Interface]\n")
	seedLegacyFile(t, filepath.Join(legacyLogs, "wireguide.log"), "log line\n")
	// A non-.conf file in tunnels and a non-.log file in logs must be ignored.
	seedLegacyFile(t, filepath.Join(legacyConfig, "tunnels", "notes.txt"), "not a tunnel")
	seedLegacyFile(t, filepath.Join(legacyLogs, "something.dat"), "binary")

	report, err := detectLegacyData(current, legacyConfig, legacyLogs)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if !report.Found {
		t.Fatal("expected Found=true")
	}
	if report.ConfigCount != 2 {
		t.Errorf("expected 2 config items, got %d", report.ConfigCount)
	}
	if report.TunnelCount != 2 {
		t.Errorf("expected 2 tunnel items, got %d", report.TunnelCount)
	}
	if report.LogCount != 1 {
		t.Errorf("expected 1 log item, got %d", report.LogCount)
	}
	if len(report.Items) != 5 {
		t.Fatalf("expected 5 items, got %d", len(report.Items))
	}
	if report.ConflictCount != 0 {
		t.Errorf("expected 0 conflicts, got %d", report.ConflictCount)
	}
	if report.Migrated || report.Dismissed {
		t.Error("expected no migration state on a fresh scan")
	}

	// Pre-existing current config.json must be flagged as a conflict.
	seedLegacyFile(t, filepath.Join(current.ConfigDir, "config.json"), `{"language":"en"}`)
	report, err = detectLegacyData(current, legacyConfig, legacyLogs)
	if err != nil {
		t.Fatalf("re-detect: %v", err)
	}
	if report.ConflictCount != 1 {
		t.Errorf("expected 1 conflict, got %d", report.ConflictCount)
	}
}

func TestMigrateLegacyDataBasic(t *testing.T) {
	current, legacyConfig, legacyLogs := testMigrationPaths(t)
	seedLegacyFile(t, filepath.Join(legacyConfig, "config.json"), `{"language":"ko"}`)
	seedLegacyFile(t, filepath.Join(legacyConfig, "tunnels", "office.conf"), "[Interface]\n")
	seedLegacyFile(t, filepath.Join(legacyLogs, "wireguide.log"), "log line\n")

	res, err := migrateLegacyData(current, legacyConfig, legacyLogs, MigrateOptions{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Default: logs excluded.
	if len(res.Migrated) != 2 {
		t.Fatalf("expected 2 migrated (config + tunnel), got %v", res.Migrated)
	}
	if len(res.Failed) != 0 || len(res.Skipped) != 0 {
		t.Fatalf("unexpected failures/skips: %v / %v", res.Failed, res.Skipped)
	}
	if _, err := os.Stat(filepath.Join(current.ConfigDir, "config.json")); err != nil {
		t.Errorf("expected config.json to be copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(current.TunnelsDir, "office.conf")); err != nil {
		t.Errorf("expected office.conf to be copied to tunnels dir: %v", err)
	}
	// Logs must NOT have been copied...
	if _, err := os.Stat(filepath.Join(current.LogsDir, "wireguide.log")); err == nil {
		t.Error("logs should not be migrated by default")
	}
	// ...and the un-migrated legacy log file/dir must survive.
	if _, err := os.Stat(filepath.Join(legacyLogs, "wireguide.log")); err != nil {
		t.Error("un-migrated legacy log file should not be removed")
	}
	// Migrated sources are removed and the emptied legacy config dir
	// (including its empty tunnels/ subdir) is dropped.
	for _, gone := range []string{
		filepath.Join(legacyConfig, "config.json"),
		filepath.Join(legacyConfig, "tunnels", "office.conf"),
		legacyConfig,
	} {
		if _, err := os.Stat(gone); err == nil {
			t.Errorf("expected %s to be removed after migration", gone)
		}
	}
	// Migration must be recorded.
	report, _ := detectLegacyData(current, legacyConfig, legacyLogs)
	if !report.Migrated {
		t.Error("expected migrated=true after a successful migration")
	}
}

func TestMigrateLegacyDataIncludeLogs(t *testing.T) {
	current, legacyConfig, legacyLogs := testMigrationPaths(t)
	seedLegacyFile(t, filepath.Join(legacyConfig, "config.json"), `{}`)
	seedLegacyFile(t, filepath.Join(legacyLogs, "wireguide.log"), "log line\n")

	res, err := migrateLegacyData(current, legacyConfig, legacyLogs, MigrateOptions{IncludeLogs: true})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(res.Migrated) != 2 {
		t.Fatalf("expected 2 migrated (config + log), got %v", res.Migrated)
	}
	if _, err := os.Stat(filepath.Join(current.LogsDir, "wireguide.log")); err != nil {
		t.Errorf("expected log to be copied: %v", err)
	}
	// With logs included, the legacy log file and its emptied folder go away.
	for _, gone := range []string{filepath.Join(legacyLogs, "wireguide.log"), legacyLogs} {
		if _, err := os.Stat(gone); err == nil {
			t.Errorf("expected %s to be removed after migration", gone)
		}
	}
	if _, err := os.Stat(legacyConfig); err == nil {
		t.Error("expected legacy config dir to be removed after migration")
	}
}

func TestMigrateLegacyDataConflicts(t *testing.T) {
	current, legacyConfig, legacyLogs := testMigrationPaths(t)
	seedLegacyFile(t, filepath.Join(legacyConfig, "config.json"), `{"language":"ko"}`)
	seedLegacyFile(t, filepath.Join(current.ConfigDir, "config.json"), `{"language":"en"}`)

	// Without overwrite the conflicting file must be skipped and the
	// migration NOT recorded (user still has pending data).
	res, err := migrateLegacyData(current, legacyConfig, legacyLogs, MigrateOptions{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(res.Migrated) != 0 || len(res.Skipped) != 1 {
		t.Fatalf("expected 1 skipped, got migrated=%v skipped=%v", res.Migrated, res.Skipped)
	}
	report, _ := detectLegacyData(current, legacyConfig, legacyLogs)
	if report.Migrated {
		t.Error("expected no migration record when everything was skipped")
	}
	// Current config must be untouched.
	data, _ := os.ReadFile(filepath.Join(current.ConfigDir, "config.json"))
	if string(data) != `{"language":"en"}` {
		t.Errorf("current config was clobbered: %s", data)
	}

	// With overwrite the legacy file replaces the current one and the
	// migration is recorded.
	res, err = migrateLegacyData(current, legacyConfig, legacyLogs, MigrateOptions{Overwrite: true})
	if err != nil {
		t.Fatalf("migrate overwrite: %v", err)
	}
	if len(res.Migrated) != 1 || len(res.Skipped) != 0 {
		t.Fatalf("expected 1 migrated with overwrite, got migrated=%v skipped=%v", res.Migrated, res.Skipped)
	}
	data, _ = os.ReadFile(filepath.Join(current.ConfigDir, "config.json"))
	if string(data) != `{"language":"ko"}` {
		t.Errorf("overwrite did not apply: %s", data)
	}
	report, _ = detectLegacyData(current, legacyConfig, legacyLogs)
	if !report.Migrated {
		t.Error("expected migrated=true after overwrite migration")
	}
	// Overwritten source is removed and the emptied legacy dir dropped.
	for _, gone := range []string{filepath.Join(legacyConfig, "config.json"), legacyConfig} {
		if _, err := os.Stat(gone); err == nil {
			t.Errorf("expected %s to be removed after overwrite migration", gone)
		}
	}
}

func TestMigrateLegacyDataNothingToMigrate(t *testing.T) {
	current, legacyConfig, legacyLogs := testMigrationPaths(t)
	res, err := migrateLegacyData(current, legacyConfig, legacyLogs, MigrateOptions{IncludeLogs: true})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(res.Migrated) != 0 || len(res.Failed) != 0 {
		t.Fatalf("expected empty result, got %+v", res)
	}
}

func TestLegacyStateDismissAndReset(t *testing.T) {
	current, legacyConfig, legacyLogs := testMigrationPaths(t)
	seedLegacyFile(t, filepath.Join(legacyConfig, "config.json"), `{}`)

	// Dismiss → report shows dismissed, prompt suppressed.
	if err := MarkLegacyDismissed(current); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	report, _ := detectLegacyData(current, legacyConfig, legacyLogs)
	if !report.Dismissed || report.Migrated {
		t.Fatalf("expected dismissed=true migrated=false, got %+v", report)
	}

	// Reset → prompt shows again.
	if err := ResetLegacyState(current); err != nil {
		t.Fatalf("reset: %v", err)
	}
	report, _ = detectLegacyData(current, legacyConfig, legacyLogs)
	if report.Dismissed || report.Migrated {
		t.Fatalf("expected clean state after reset, got %+v", report)
	}

	// Migrate → migrated=true, dismissed=false.
	if _, err := migrateLegacyData(current, legacyConfig, legacyLogs, MigrateOptions{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	report, _ = detectLegacyData(current, legacyConfig, legacyLogs)
	if !report.Migrated || report.Dismissed {
		t.Fatalf("expected migrated=true dismissed=false, got %+v", report)
	}
}
