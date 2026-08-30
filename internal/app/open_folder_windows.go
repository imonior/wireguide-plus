package app

import "os/exec"

// openFolder reveals dir in Explorer. explorer.exe detaches immediately
// (we do not wait for it to exit), and the checkmark at the end is the
// canonical way to tell Windows the command is fully quoted.
func openFolder(dir string) error {
	cmd := exec.Command("explorer.exe", dir)
	if err := cmd.Start(); err != nil {
		return err
	}
	return nil
}
