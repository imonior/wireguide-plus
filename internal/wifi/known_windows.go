//go:build windows

package wifi

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/imonior/wireguide-plus/internal/sysexec"
)

// knownSSIDsWindows enumerates every Wi-Fi profile the machine has saved
// via `netsh wlan show profiles`. Best-effort: an empty slice is normal
// when the WLAN AutoConfig service is off or there is no wireless
// adapter. The console window is hidden so a GUI never flashes a black
// box while enumerating.
func knownSSIDsWindows() []string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "netsh", "wlan", "show", "profiles")
	sysexec.Hide(cmd)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil
	}

	seen := make(map[string]bool)
	list := make([]string, 0, 16)
	for _, line := range strings.Split(out.String(), "\n") {
		// Profile rows read "All User Profile     : MyWiFi" (English) or
		// "所有用户配置文件 : MyWiFi" (localized); netsh keeps the colon
		// between the label and the name in every language. Taking
		// everything after the FIRST colon naturally drops header rows
		// like "Interface WLAN on profiles:" (nothing after the colon)
		// and netsh's own error prose.
		_, after, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		ssid := strings.TrimSpace(after)
		if ssid == "" || seen[ssid] {
			continue
		}
		seen[ssid] = true
		list = append(list, ssid)
	}
	return list
}
