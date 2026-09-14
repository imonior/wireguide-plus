package gui

import (
	"log/slog"
	"runtime"

	"github.com/imonior/wireguide-plus/internal/update"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// installCustomMenuBar replaces the macOS menu bar that Wails synthesizes.
//
// Wails' default menu bar (DefaultApplicationMenu) is App/File/Edit/View/
// Window/Help; its Help → "Learn More" calls SetURL("https://wails.io") —
// it navigates the WebView itself, so clicking it leaves the user stranded
// on the Wails website with no way back to the GUI.
//
// We rebuild the bar explicitly instead:
//   - App is mandatory on macOS (About/Hide/Quit) and kept as-is.
//   - File, View and Window are dropped: there are no file operations and
//     zoom/fullscreen/minimise live in the app UI or the window title bar.
//   - Edit is KEPT (see the AddRole call below) — the WebView's text
//     editing (paste/copy/cut in the config/field/script editors) depends
//     on it; without it those fields silently lose paste.
//   - Help opens the GitHub project page in the system default browser
//     (the WebView is never touched).
//
// Windows and Linux never show this menu bar (Windows only gets a window
// menu via window.SetMenu, which this app never calls; Linux has no global
// menu bar in Wails), so this is a no-op there. The GOOS check is explicit
// so the behavior stays obvious if a platform menu is ever enabled.
func installCustomMenuBar(app *application.App) {
	if runtime.GOOS != "darwin" {
		return
	}
	menu := application.NewMenu()
	menu.AddRole(application.AppMenu)
	// Edit is deliberately KEPT: on macOS the WebView routes text-editing
	// commands (Cut/Copy/Paste/Select All/Undo/Redo → Cmd+X/C/V/⌘Z) through
	// the Edit menu's responder chain. Dropping it silently breaks paste,
	// copy and cut in EVERY text field — the config/field/script editors
	// included — because no menu item validates/answers the paste: selector.
	menu.AddRole(application.EditMenu)
	// Help opens the GitHub project page in the system default browser
	// (the WebView is never touched).
	help := menu.AddSubmenu("Help")
	help.Add("Learn More").OnClick(func(*application.Context) {
		if err := app.Browser.OpenURL(update.GitHubRepoURL); err != nil {
			slog.Warn("menu: opening GitHub project failed", "error", err)
		}
	})
	app.Menu.SetApplicationMenu(menu)
}
