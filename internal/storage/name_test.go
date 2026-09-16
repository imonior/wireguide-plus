package storage

import (
	"strings"
	"testing"
)

func TestValidateTunnelName(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"simple", "home", false},
		{"with dash", "work-vpn", false},
		{"with underscore", "corp_vpn", false},
		{"mixed case and digits", "ProdRegion2", false},

		{"empty", "", true},
		{"space in middle", "my vpn", false},
		{"space in name with digits", "Work VPN 2", false},
		{"leading space", " vpn", true},
		{"trailing space", "vpn ", true},
		{"dot (would confuse extension)", "work.vpn", true},
		{"slash (path traversal)", "a/b", true},
		{"backslash", "a\\b", true},
		{"dot dot", "..", true},
		// Reserved device names — rejected on every platform, not just
		// Windows, so a synced config stays usable there.
		{"reserved CON", "CON", true},
		{"reserved con lowercase", "con", true},
		{"reserved NUL", "NUL", true},
		{"reserved COM1", "COM1", true},
		{"reserved CONIN$", "CONIN$", true},
		{"reserved prefix is fine", "CONSOLE", false},
		{"reserved-ish COM10 is fine", "COM10", false},
		// 100 valid characters — exercises the length limit, not the
		// character class (100 null bytes would trip the char check first).
		{"too long", strings.Repeat("a", 100), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateTunnelName(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateTunnelName(%q) err=%v, wantErr=%v", tc.input, err, tc.wantErr)
			}
		})
	}
}

// TestReservedDeviceNamesCoverage pins the reserved-name list itself.
// ValidateTunnelName can never reach the CONIN$/CONOUT$ entries in
// practice — the character check rejects '$' first — so without this a
// regression in those entries would go unnoticed.
func TestReservedDeviceNamesCoverage(t *testing.T) {
	want := []string{
		"CON", "PRN", "AUX", "NUL",
		"CONIN$", "CONOUT$",
		"COM1", "COM9", "LPT1", "LPT9",
	}
	for _, n := range want {
		if !reservedDeviceNames[n] {
			t.Errorf("reservedDeviceNames[%q] = false, want true", n)
		}
	}
}
