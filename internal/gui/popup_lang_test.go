package gui

import "testing"

// LANGIDs the mapping must honour: ENGLISH=0x0409, the four Chinese
// Simplified variants, the Traditional-region ones, JA=0x0411, KO=0x0412.
func TestLangFromUILanguage(t *testing.T) {
	cases := []struct {
		langID uint16
		want   string
	}{
		{0x0409, "en"}, // English US
		{0x0404, "zh-TW"},
		{0x0C04, "zh-TW"}, // zh-HK
		{0x1404, "zh-TW"}, // zh-MO
		{0x0804, "zh"},    // zh-CN
		{0x1004, "zh"},    // zh-SG
		{0x0411, "ja"},
		{0x0412, "ko"},
		{0x0407, "en"}, // German → default en
		{0x0000, "en"},
	}
	for _, tc := range cases {
		if got := langFromUILanguage(tc.langID); got != tc.want {
			t.Errorf("langFromUILanguage(0x%04X) = %q, want %q", tc.langID, got, tc.want)
		}
	}
}

func TestResolvePopupLang(t *testing.T) {
	// auto / empty follow the OS UI language…
	if got := resolvePopupLang("auto", 0x0804); got != "zh" {
		t.Errorf("auto on zh-CN machine = %q, want zh", got)
	}
	if got := resolvePopupLang("", 0x0411); got != "ja" {
		t.Errorf("empty on ja machine = %q, want ja", got)
	}
	// …an explicit pick wins over the OS.
	if got := resolvePopupLang("ko", 0x0804); got != "ko" {
		t.Errorf("explicit ko = %q, want ko", got)
	}
	if got := resolvePopupLang("zh-TW", 0x0411); got != "zh-TW" {
		t.Errorf("explicit zh-TW = %q, want zh-TW", got)
	}
}
