//go:build !darwin

package update

import "fmt"

// installDarwinBundle is unreachable on non-darwin platforms — Install
// only dispatches to it from the darwin branch — but must exist so the
// package compiles everywhere.
func installDarwinBundle(assetPath string) error {
	return fmt.Errorf("macOS in-place update is only available on darwin (got asset %q)", assetPath)
}
