//go:build !windows

package update

import (
	"fmt"
	"runtime"
)

func installWindows(path string) error {
	return fmt.Errorf("windows installer not available on %s", runtime.GOOS)
}

func runInstallerElevated(path string, args ...string) error {
	return fmt.Errorf("runas elevation not available on %s", runtime.GOOS)
}
