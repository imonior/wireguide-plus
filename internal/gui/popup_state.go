package gui

// popupState is what the connection-status bubble is allowed to claim about
// the tunnels it lists. Three states rather than two: a tunnel that is still
// connecting has NOT connected yet, and announcing it as "connected" only to
// have it vanish when the attempt fails is the exact confusion this
// distinction exists to prevent (a boot-time DNS miss was the original case).
//
// Declared here, without a build tag, rather than next to the Windows drawing
// code: the tray manager computes the state on every platform and the
// non-Windows showStatusPopup stub takes it as a parameter. Keeping it in the
// //go:build windows file made macOS and Linux fail to compile.
type popupState int

const (
	popupStateDisconnected popupState = iota
	popupStateConnecting
	popupStateConnected
)

// overviewRow is one line of the launch status-overview bubble: a tunnel,
// its connection state ("connected" | "connecting" | "disconnected"), and
// whether automation may act on it (Auto = automation not disabled). The
// state travels as a string, like popupPayload.State, so each renderer
// localises the caption itself instead of receiving pre-translated text.
type overviewRow struct {
	Name  string `json:"name"`
	State string `json:"state"`
	Auto  bool   `json:"auto"`
}
