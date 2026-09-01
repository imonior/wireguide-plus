package storage

// Interactive cross-version data migration.
//
// Before the app was renamed to "wireguideplus" it stored its user data in a
// directory called "wireguide". Older builds silently copied that data over on
// first launch (migrateFromLegacyPaths in paths.go); that auto-migration was
// removed in favour of a user-visible prompt so the user can decide whether,
// what (logs are optional) and how (overwrite?) to migrate, and can compare the
// old and new folders side by side before committing.
//
// The GUI calls DetectLegacyData once at startup and shows a modal when it
// finds legacy data; the modal then calls MigrateLegacyData. CLI commands do
// NOT auto-migrate — they operate on the current paths, and the first GUI
// launch offers the migration.

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// LegacyDataKind classifies a migratable file so the UI can group and label
// items.
type LegacyDataKind string

const (
	LegacyConfigFile LegacyDataKind = "config" // config.json, history.json
	LegacyTunnelFile LegacyDataKind = "tunnel" // tunnels/*.conf
	LegacyLogFile    LegacyDataKind = "log"    // legacy logs dir files
)

// LegacyDataItem describes one file that exists in the legacy directory and
// can be copied to the current location.
type LegacyDataItem struct {
	Kind         LegacyDataKind `json:"kind"`
	Name         string         `json:"name"` // display name, e.g. "config.json", "tunnels/office.conf", "logs/wireguide.log"
	Size         int64          `json:"size"`
	SourcePath   string         `json:"source_path"`
	TargetPath   string         `json:"target_path"`
	TargetExists bool           `json:"target_exists"` // a file with the same name already exists at the target
}

// LegacyDataReport is the result of a legacy-data scan. Found is true when at
// least one migratable item exists. Migrated/Dismissed reflect the persisted
// migration state so the GUI can avoid nagging a user who already decided.
type LegacyDataReport struct {
	Found           bool             `json:"found"`
	LegacyConfigDir string           `json:"legacy_config_dir"`
	LegacyLogsDir   string           `json:"legacy_logs_dir"`
	Items           []LegacyDataItem `json:"items"`
	ConfigCount     int              `json:"config_count"`
	TunnelCount     int              `json:"tunnel_count"`
	LogCount        int              `json:"log_count"`
	ConflictCount   int              `json:"conflict_count"` // items whose target already exists
	Migrated        bool             `json:"migrated"`
	Dismissed       bool             `json:"dismissed"`
}

// MigrateOptions controls a migration run.
type MigrateOptions struct {
	// Overwrite replaces target files that already exist. Without it,
	// conflicting items are skipped (reported in MigrateResult.Skipped).
	Overwrite bool `json:"overwrite"`
	// IncludeLogs migrates the legacy logs directory. Off by default:
	// logs are recreatable and often useless after an upgrade.
	IncludeLogs bool `json:"include_logs"`
}

// MigrateResult reports per-file outcomes by display name.
type MigrateResult struct {
	Migrated []string `json:"migrated"`
	Skipped  []string `json:"skipped"`
	Failed   []string `json:"failed"`
}

// LegacyConfigDir returns the pre-rename config directory for this OS, or ""
// when it cannot be determined.
func LegacyConfigDir() string {
	return legacyConfigDir()
}

// LegacyLogsDir returns the pre-rename logs directory for this OS, or "" when
// it cannot be determined.
func LegacyLogsDir() string {
	return legacyLogsDir()
}

// legacyLogsDir mirrors legacyConfigDir for the logs location the pre-plus
// builds wrote to.
func legacyLogsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Logs", legacyAppName)
	case "linux":
		dataHome := os.Getenv("XDG_DATA_HOME")
		if dataHome == "" {
			dataHome = filepath.Join(home, ".local", "share")
		}
		return filepath.Join(dataHome, legacyAppName, "logs")
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, legacyAppName, "logs")
		}
		return filepath.Join(home, "AppData", "Roaming", legacyAppName, "logs")
	}
	return ""
}

// DetectLegacyData scans the legacy "wireguide" directories for migratable
// data and reports it. It never copies anything — the UI decides after showing
// the report.
func DetectLegacyData(current *Paths) (*LegacyDataReport, error) {
	return detectLegacyData(current, legacyConfigDir(), legacyLogsDir())
}

// detectLegacyData is DetectLegacyData with the legacy locations passed in
// explicitly (the OS-specific resolvers are hard to override in tests).
func detectLegacyData(current *Paths, legacyConfig, legacyLogs string) (*LegacyDataReport, error) {
	report := &LegacyDataReport{
		LegacyConfigDir: legacyConfig,
		LegacyLogsDir:   legacyLogs,
	}

	// Configuration files live at the top level of the legacy config dir.
	if report.LegacyConfigDir != "" && pathExists(report.LegacyConfigDir) {
		for _, name := range []string{"config.json", "history.json"} {
			src := filepath.Join(report.LegacyConfigDir, name)
			info, err := os.Stat(src)
			if err != nil || info.IsDir() {
				continue
			}
			report.Items = append(report.Items, legacyItem(
				LegacyConfigFile, name, src, filepath.Join(current.ConfigDir, name), info.Size()))
			report.ConfigCount++
		}
		// Tunnel configs live in the legacy tunnels/ subdirectory.
		legacyTunnels := filepath.Join(report.LegacyConfigDir, "tunnels")
		if entries, err := os.ReadDir(legacyTunnels); err == nil {
			for _, e := range entries {
				if e.IsDir() || filepath.Ext(e.Name()) != ".conf" {
					continue
				}
				rel := filepath.Join("tunnels", e.Name())
				info, err := e.Info()
				if err != nil {
					continue
				}
				report.Items = append(report.Items, legacyItem(
					LegacyTunnelFile, rel, filepath.Join(legacyTunnels, e.Name()),
					filepath.Join(current.TunnelsDir, e.Name()), info.Size()))
				report.TunnelCount++
			}
		}
	}

	// Log files come from the legacy logs dir.
	if report.LegacyLogsDir != "" && pathExists(report.LegacyLogsDir) {
		if entries, err := os.ReadDir(report.LegacyLogsDir); err == nil {
			for _, e := range entries {
				if e.IsDir() || filepath.Ext(e.Name()) != ".log" {
					continue
				}
				info, err := e.Info()
				if err != nil {
					continue
				}
				rel := filepath.Join("logs", e.Name())
				report.Items = append(report.Items, legacyItem(
					LegacyLogFile, rel, filepath.Join(report.LegacyLogsDir, e.Name()),
					filepath.Join(current.LogsDir, e.Name()), info.Size()))
				report.LogCount++
			}
		}
	}

	report.Found = len(report.Items) > 0
	for _, item := range report.Items {
		if item.TargetExists {
			report.ConflictCount++
		}
	}

	state := readLegacyState(current)
	report.Migrated = state.MigratedAt != ""
	report.Dismissed = state.Dismissed
	return report, nil
}

// legacyItem builds a LegacyDataItem, computing TargetExists from the current
// filesystem so the UI can flag conflicts before the user decides.
func legacyItem(kind LegacyDataKind, name, src, dst string, size int64) LegacyDataItem {
	item := LegacyDataItem{
		Kind:       kind,
		Name:       name,
		Size:       size,
		SourcePath: src,
		TargetPath: dst,
	}
	if _, err := os.Stat(dst); err == nil {
		item.TargetExists = true
	}
	return item
}

// MigrateLegacyData copies the legacy data into the current paths according to
// opts. It re-scans (rather than trusting a previously returned report) so the
// copy always reflects the on-disk reality at call time.
//
// Once a file has been successfully copied, its source is removed so the
// legacy directory is progressively emptied; the legacy config/logs folders
// themselves are dropped when they end up empty. Files that were skipped or
// failed to copy (or that this app doesn't migrate, e.g. a legacy
// update-state.json) keep the folder around untouched.
//
// On success (at least one file copied) it records the migration so the GUI
// stops prompting. A run where everything was skipped (conflicts + no
// overwrite) or failed does NOT record the migration — the user still has
// un-migrated data and should be reminded on the next launch.
func MigrateLegacyData(current *Paths, opts MigrateOptions) (*MigrateResult, error) {
	return migrateLegacyData(current, legacyConfigDir(), legacyLogsDir(), opts)
}

func migrateLegacyData(current *Paths, legacyConfig, legacyLogs string, opts MigrateOptions) (*MigrateResult, error) {
	report, err := detectLegacyData(current, legacyConfig, legacyLogs)
	if err != nil {
		return nil, err
	}
	if !report.Found {
		return &MigrateResult{}, nil
	}

	res := &MigrateResult{}
	for _, item := range report.Items {
		if item.Kind == LegacyLogFile && !opts.IncludeLogs {
			continue
		}
		if item.TargetExists && !opts.Overwrite {
			res.Skipped = append(res.Skipped, item.Name)
			continue
		}
		if err := copyFile(item.SourcePath, item.TargetPath); err != nil {
			slog.Warn("legacy migration failed", "item", item.Name, "error", err)
			res.Failed = append(res.Failed, item.Name)
			continue
		}
		res.Migrated = append(res.Migrated, item.Name)
		// The data is safe at the target now — drop the source so the
		// legacy directory empties out. A removal failure is non-fatal:
		// the copy succeeded, and the empty-dir cleanup below will simply
		// keep the folder.
		if err := os.Remove(item.SourcePath); err != nil {
			slog.Warn("legacy migration: source cleanup failed", "item", item.Name, "error", err)
		}
	}

	// Drop the legacy folders once they are empty. os.Remove only succeeds
	// on empty directories, so any non-migrated or unrelated file keeps the
	// folder intact.
	if legacyConfig != "" {
		_ = os.Remove(filepath.Join(legacyConfig, "tunnels"))
		_ = os.Remove(legacyConfig)
	}
	if opts.IncludeLogs && legacyLogs != "" {
		_ = os.Remove(legacyLogs)
	}

	if len(res.Migrated) > 0 {
		if err := MarkLegacyMigrated(current); err != nil {
			slog.Warn("legacy migration succeeded but marking failed", "error", err)
		}
	}
	return res, nil
}

// MarkLegacyMigrated records that the user has migrated their legacy data (or
// explicitly decided the outcome is final), suppressing the startup prompt.
func MarkLegacyMigrated(current *Paths) error {
	state := readLegacyState(current)
	state.MigratedAt = time.Now().Format(time.RFC3339)
	state.Dismissed = false
	return writeLegacyState(current, state)
}

// MarkLegacyDismissed records that the user chose "don't remind me again",
// suppressing the startup prompt without migrating anything.
func MarkLegacyDismissed(current *Paths) error {
	state := readLegacyState(current)
	state.Dismissed = true
	return writeLegacyState(current, state)
}

// ResetLegacyState clears the persisted migration state so the prompt shows
// again (used by the Settings migration entry to re-trigger the flow).
func ResetLegacyState(current *Paths) error {
	_ = os.Remove(legacyStatePath(current))
	return nil
}

type legacyMigrationState struct {
	MigratedAt string `json:"migrated_at,omitempty"`
	Dismissed  bool   `json:"dismissed"`
}

func legacyStatePath(current *Paths) string {
	return filepath.Join(current.ConfigDir, "legacy-migration.json")
}

func readLegacyState(current *Paths) legacyMigrationState {
	var state legacyMigrationState
	data, err := os.ReadFile(legacyStatePath(current))
	if err != nil {
		return state
	}
	_ = json.Unmarshal(data, &state)
	return state
}

func writeLegacyState(current *Paths, state legacyMigrationState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(legacyStatePath(current), data, 0600)
}
