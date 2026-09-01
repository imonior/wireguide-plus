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
//   - File, Edit, View and Window are dropped: the app has no
//     file/edit operations, and zoom, fullscreen and minimise are all
//     available in the app's own UI or on the window's title bar.
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
	// File/Edit/View/Window menus omitted: nothing useful left to offer —
	// see doc comment above.
	// Our Help menu: opens the GitHub project page in the system browser.
	help := menu.AddSubmenu("Help")
	help.Add("Learn More").OnClick(func(*application.Context) {
		if err := app.Browser.OpenURL(update.GitHubRepoURL); err != nil {
			slog.Warn("menu: opening GitHub project failed", "error", err)
		}
	})
	app.Menu.SetApplicationMenu(menu)
}
