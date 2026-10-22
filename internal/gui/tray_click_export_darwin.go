package gui

// This file only hosts the cgo //export bridge. Per cgo rules a file
// containing //export may only DECLARE things in its preamble, so the
// Objective-C implementation lives in tray_click_darwin.go.

/*
#include <stdint.h>
*/
import "C"

//export wgTrayDoubleClick
func wgTrayDoubleClick() {
	if cb := trayDoubleClickCB.Load(); cb != nil {
		(*cb)()
	}
}
