package app

import "os/exec"

// openFolder reveals dir in Finder via `open` (which detaches).
func openFolder(dir string) error {
	cmd := exec.Command("open", dir)
	if err := cmd.Start(); err != nil {
		return err
	}
	return nil
}
