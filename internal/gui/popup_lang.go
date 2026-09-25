package gui

// Popup language resolution, shared by the Windows GDI bubble (which draws
// translated text from Go) and unit-testable without a UI:
//
//   - Settings stores Language as "auto" | "en" | "zh" | "zh-TW" | "ja" |
//     "ko". "auto" means "follow the OS display language" — the same rule
//     the webview UI applies via navigator.language. The Go side must
//     resolve "auto" too, or the bubble defaults to English on a Chinese /
//     Japanese / Korean machine whose app UI is localised, and the two
//     surfaces disagree.
//   - The mapping mirrors the frontend detectLanguage(): zh splits between
//     Simplified and Traditional by the region sub-language.

// langFromUILanguage maps a Win32 UI LANGID to one of the app's language
// codes (en / zh / zh-TW / ja / ko). Anything unmapped falls back to "en",
// matching the translations bundle the frontend serves as the default.
func langFromUILanguage(langID uint16) string {
	switch langID & 0x3FF { // PRIMARYLANGID
	case 0x04: // Chinese
		switch langID >> 10 { // SUBLANGID
		case 0x01, 0x03, 0x05: // TW, HK, MO
			return "zh-TW"
		default: // CN, SG, anything else Chinese
			return "zh"
		}
	case 0x11: // Japanese
		return "ja"
	case 0x12: // Korean
		return "ko"
	}
	return "en"
}

// resolvePopupLang turns a Settings.Language value into a concrete
// language for the bubble; "auto" (and the empty legacy value) resolves
// through the OS UI language.
func resolvePopupLang(lang string, uiLangID uint16) string {
	switch lang {
	case "", "auto":
		return langFromUILanguage(uiLangID)
	}
	return lang
}
