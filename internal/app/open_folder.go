package app

import (
	"fmt"

	"github.com/imonior/wireguide-plus/internal/storage"
)

// FolderKind is a closed set of app-managed directories the frontend may
// ask to reveal in the platform file manager.
type FolderKind string

const (
	FolderConfig  FolderKind = "config"
	FolderTunnels FolderKind = "tunnels"
	FolderLogs    FolderKind = "logs"
)

// OpenFolder reveals one of the app-managed directories in the platform
// file manager. Only directories the app itself owns are accepted — a
// compromised frontend cannot point the opener at arbitrary paths.
//
// kind ∈ {"config", "tunnels", "logs"}.
func (s *TunnelService) OpenFolder(kind string) error {
	paths, err := storage.GetPaths()
	if err != nil {
		return fmt.Errorf("resolve app paths: %w", err)
	}
	var dir string
	switch FolderKind(kind) {
	case FolderConfig:
		dir = paths.ConfigDir
	case FolderTunnels:
		dir = paths.TunnelsDir
	case FolderLogs:
		dir = paths.LogsDir
	default:
		return fmt.Errorf("unknown folder kind: %s", kind)
	}
	if dir == "" {
		return fmt.Errorf("folder path not available for kind %q", kind)
	}
	if err := openFolder(dir); err != nil {
		return fmt.Errorf("open folder %s: %w", dir, err)
	}
	return nil
}
