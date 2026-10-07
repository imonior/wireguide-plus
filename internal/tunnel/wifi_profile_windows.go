//go:build windows

package tunnel

import (
	"bytes"
	"os/exec"
	"strings"
)

// GetSavedWifiProfiles returns the SSIDs of the Wi-Fi profiles saved on
// this machine.
//
// GetSavedWifiProfiles 获取本机已保存的 Wi-Fi 配置文件的 SSID 列表。
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
		// netsh 输出示例："所有用户配置文件     : ASUS_QS_JYH"
		if _, after, ok := strings.Cut(line, ":"); ok {
			ssid := strings.TrimSpace(after)
			if ssid != "" {
				list = append(list, ssid)
			}
		}
	}
	// De-duplicate. / 去重
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
