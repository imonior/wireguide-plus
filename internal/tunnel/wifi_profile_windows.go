//go:build windows

package tunnel

import (
	"bytes"
	"os/exec"
	"strings"
)

// GetSavedWifiProfiles returns the SSIDs of the Wi-Fi profiles saved on
// this machine.
func GetSavedWifiProfiles() ([]string, error) {
	cmd := exec.Command("netsh", "wlan", "show", "profiles")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	var list []string
	lines := strings.Split(out.String(), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// netsh sample line: "All User Profile     : ASUS_QS_JYH"
		if _, after, ok := strings.Cut(line, ":"); ok {
			ssid := strings.TrimSpace(after)
			if ssid != "" {
				list = append(list, ssid)
			}
		}
	}
	// De-duplicate.
	m := make(map[string]bool)
	var res []string
	for _, s := range list {
		if !m[s] {
			m[s] = true
			res = append(res, s)
		}
	}
	return res, nil
}
