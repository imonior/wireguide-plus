package gui

import "time"

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

// String is the wire form each renderer localises into its own caption; it
// matches the overviewRow.State vocabulary exactly so both bubble modes can
// share one row renderer.
func (s popupState) String() string {
	switch s {
	case popupStateConnecting:
		return "connecting"
	case popupStateConnected:
		return "connected"
	}
	return "disconnected"
}

// popupPersistent is the duration that turns a status bubble into a
// standing notification: it stays up until the user acts (✕, Open Window,
// Disconnect, or a newer transition replacing it). Deliberate design, not
// an oversight loop — a disconnected-tunnel warning that auto-vanished after
// ten seconds was a message the user could miss entirely while working in
// another app (and on macOS, a non-activating bubble is precisely NOT
// allowed to interrupt, so it must also not evaporate). Only the
// informational launch overview keeps a timed life.
const popupPersistent = time.Duration(-1)

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

// transitionRows turns the tunnels named by one status transition (all in
// the same state) into the row form both bubble renderers draw: one line per
// tunnel, "state: name", in the state's colour. Rows beat a joined names
// line because a flapping tunnel must read as its own line, not scroll off
// the ellipsis of a comma-joined list.
func transitionRows(names []string, state popupState) []overviewRow {
	rows := make([]overviewRow, 0, len(names))
	s := state.String()
	for _, n := range names {
		rows = append(rows, overviewRow{Name: n, State: s})
	}
	return rows
}
