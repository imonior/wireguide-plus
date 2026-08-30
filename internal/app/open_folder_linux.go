package app

import "os/exec"

// openFolder reveals dir in the default file manager via xdg-open.
// Best-effort: if no file manager is installed this simply fails
// (recorded in the caller's error), which is acceptable on headless
// systems.
func openFolder(dir string) error {
	cmd := exec.Command("xdg-open", dir)
	if err := cmd.Start(); err != nil {
		return err
	}
	return nil
}
