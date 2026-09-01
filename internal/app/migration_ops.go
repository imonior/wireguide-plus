package app

// Legacy data migration bindings. The frontend calls CheckLegacyData at
// startup; when it returns Found (and the user hasn't already migrated or
// dismissed), it shows the migration modal which then drives
// MigrateLegacyData / DismissLegacyMigration / OpenFolder.

import (
	"fmt"

	"github.com/imonior/wireguide-plus/internal/storage"
)

// CheckLegacyData scans for data left behind by pre-rename ("wireguide")
// installs and reports what could be migrated, including any name conflicts
// with the current install. Nothing is copied by this call.
func (s *TunnelService) CheckLegacyData() (*storage.LegacyDataReport, error) {
	paths, err := storage.GetPaths()
	if err != nil {
		return nil, fmt.Errorf("resolve app paths: %w", err)
	}
	report, err := storage.DetectLegacyData(paths)
	if err != nil {
		return nil, fmt.Errorf("detect legacy data: %w", err)
	}
	return report, nil
}

// MigrateLegacyData copies the legacy config/tunnels/logs into the current
// locations. Overwrite controls whether existing target files are replaced;
// IncludeLogs controls whether the legacy logs directory is migrated.
func (s *TunnelService) MigrateLegacyData(opts storage.MigrateOptions) (*storage.MigrateResult, error) {
	paths, err := storage.GetPaths()
	if err != nil {
		return nil, fmt.Errorf("resolve app paths: %w", err)
	}
	res, err := storage.MigrateLegacyData(paths, opts)
	if err != nil {
		return nil, fmt.Errorf("migrate legacy data: %w", err)
	}
	return res, nil
}

// DismissLegacyMigration records the user's "don't remind me again" choice so
// the startup prompt stays hidden. The migration entry in Settings can
// re-trigger the flow later.
func (s *TunnelService) DismissLegacyMigration() error {
	paths, err := storage.GetPaths()
	if err != nil {
		return fmt.Errorf("resolve app paths: %w", err)
	}
	if err := storage.MarkLegacyDismissed(paths); err != nil {
		return fmt.Errorf("dismiss legacy migration: %w", err)
	}
	return nil
}

// ResetLegacyMigration clears the persisted migration state so the startup
// prompt shows again. Used by the Settings migration entry.
func (s *TunnelService) ResetLegacyMigration() error {
	paths, err := storage.GetPaths()
	if err != nil {
		return fmt.Errorf("resolve app paths: %w", err)
	}
	if err := storage.ResetLegacyState(paths); err != nil {
		return fmt.Errorf("reset legacy migration state: %w", err)
	}
	return nil
}
