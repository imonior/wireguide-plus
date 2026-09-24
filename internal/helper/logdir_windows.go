//go:build windows

package helper

import "os"

// prepareLogsDir creates the helper's daily-log directory. No ownership
// handover exists here: the directory lives under %APPDATA%, whose ACL grants
// the user full control by inheritance, so a SYSTEM-created directory stays
// usable by the GUI.
func prepareLogsDir(dir string, uid int) error {
	return os.MkdirAll(dir, 0o700)
}
