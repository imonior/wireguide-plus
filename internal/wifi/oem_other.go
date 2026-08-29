//go:build !windows

package wifi

// decodeOEM is a no-op passthrough on non-Windows platforms where console
// child output is already UTF-8.
func decodeOEM(b []byte) string {
	return string(b)
}
