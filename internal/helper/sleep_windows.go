//go:build windows

package helper

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// SetThreadExecutionState flags (EXECUTION_STATE, from winbase.h). Defined
// locally because golang.org/x/sys does not export them in the version this
// module pins, and the values are stable Win32 constants.
const (
	esContinuous       = 0x80000000
	esSystemRequired   = 0x00000001
	esDisplayRequired  = 0x00000002
	esAwayModeRequired = 0x00000040
)

var (
	kernel32                    = windows.NewLazySystemDLL("kernel32.dll")
	procSetThreadExecutionState = kernel32.NewProc("SetThreadExecutionState")
)

// setThreadExecutionState calls the Win32 API. It returns the previous state
// flags, or 0 on failure (the documented "request not honoured" signal).
func setThreadExecutionState(flags uint32) (uint32, error) {
	r, _, e := procSetThreadExecutionState.Call(uintptr(flags))
	prev := uint32(r)
	if prev == 0 {
		if e != nil && e != windows.ERROR_SUCCESS {
			return 0, e
		}
		return 0, fmt.Errorf("SetThreadExecutionState(0x%x) returned 0 (request not honoured)", flags)
	}
	return prev, nil
}

// applySystemSleepPrevention tells Windows to keep the machine awake while
// WireGuide Plus runs in the background. The user's intent is
// "ignore screensaver / ignore screen-off / prohibit sleep", which maps to:
//
//	ES_CONTINUOUS        — make the override sticky until we clear it
//	ES_SYSTEM_REQUIRED   — block sleep / suspend (S1-S3)
//	ES_DISPLAY_REQUIRED  — keep the display on (ignore screen-off / screensaver)
//	ES_AWAYMODE_REQUIRED — allow away mode but never fully suspend
//
// Disabled resets to ES_CONTINUOUS alone, which clears every other flag and
// hands control back to the system power policy. The call is per-thread; the
// helper is a long-lived process, so the override lives for its whole lifetime
// (and is released automatically when the process exits).
func applySystemSleepPrevention(enabled bool) error {
	var flags uint32 = esContinuous
	if enabled {
		flags |= esSystemRequired | esDisplayRequired | esAwayModeRequired
	}
	if _, err := setThreadExecutionState(flags); err != nil {
		return fmt.Errorf("setThreadExecutionState(0x%x): %w", flags, err)
	}
	return nil
}
