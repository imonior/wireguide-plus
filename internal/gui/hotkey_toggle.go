package gui

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// toggleMainWindow shows, focuses, or hides the main window with the global
// hotkey. If the window is visible AND focused we hide it (go to tray);
// otherwise we restore + show + focus it (covers hidden, minimised, and
// visible-but-unfocused cases). Every window call runs on the UI thread via
// InvokeAsync because the hotkey callback may arrive on a non-UI thread (the
// Windows message-pump goroutine, or — on macOS — the Carbon run loop which is
// the same main thread and short-circuits InvokeAsync).
//
// toggleMainWindow 用全局热键显示、聚焦或隐藏主窗口。当窗口可见且获得焦点时
// 隐藏它（转入托盘）；否则还原 + 显示 + 聚焦（涵盖隐藏、最小化、可见但失焦三
// 种情况）。所有窗口调用都经 InvokeAsync 在 UI 线程执行，因为热键回调可能来自
// 非 UI 线程（Windows 消息泵协程；macOS 上则是同一主线程的 Carbon run loop，
// InvokeAsync 会短路）。
func toggleMainWindow(win *application.WebviewWindow) {
	application.InvokeAsync(func() {
		if win == nil {
			return
		}
		if win.IsVisible() && win.IsFocused() {
			win.Hide()
			hideDock() // no-op off macOS; hides the dock icon on macOS
		} else {
			if win.IsMinimised() {
				win.Restore()
			}
			win.Show()
			win.Focus()
		}
	})
}
