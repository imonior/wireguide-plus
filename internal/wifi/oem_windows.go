//go:build windows

package wifi

import (
	"golang.org/x/sys/windows"
)

// decodeOEM converts a byte slice produced by a Windows console child
// (netsh, etc.) from the system OEM codepage to UTF-8. The codepage is
// CP_OEMCP (1): CP936/GBK on zh-CN systems, CP949 on ko-KR, CP932 on
// ja-JP, and UTF-8 when the "Beta: Use Unicode UTF-8" option is enabled.
// Without this, non-ASCII SSIDs printed by netsh surface in the GUI as
// U+FFFD replacement garbage. Returns the input unchanged on failure.
func decodeOEM(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	const cpOEMCP = 1
	wlen, err := windows.MultiByteToWideChar(cpOEMCP, 0, &b[0], int32(len(b)), nil, 0)
	if err != nil || wlen <= 0 {
		return string(b)
	}
	wbuf := make([]uint16, wlen)
	if _, err := windows.MultiByteToWideChar(cpOEMCP, 0, &b[0], int32(len(b)), &wbuf[0], wlen); err != nil {
		return string(b)
	}
	return windows.UTF16ToString(wbuf)
}
