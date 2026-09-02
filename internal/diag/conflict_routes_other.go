//go:build !windows

package diag

// getRoutesWindows exists so the platform switch in conflict.go resolves
// on every build target; non-Windows builds never call it.
func getRoutesWindows(string) []string { return nil }
