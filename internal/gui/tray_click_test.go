package gui

import (
	"testing"
	"time"
)

// waitEvent reports which callback fired first (or nil for neither) within
// the given window.
func waitEvent(t *testing.T, open, show chan struct{}, within time.Duration) string {
	t.Helper()
	select {
	case <-open:
		return "open"
	case <-show:
		return "show"
	case <-time.After(within):
		return ""
	}
}

func TestTrayClickRouterSingleOpensMenu(t *testing.T) {
	open := make(chan struct{}, 1)
	show := make(chan struct{}, 1)
	r := newTrayClickRouter(
		func() { open <- struct{}{} },
		func() { show <- struct{}{} },
	)

	r.onClick()
	if got := waitEvent(t, open, show, 2*time.Second); got != "open" {
		t.Fatalf("single click: first event = %q, want open", got)
	}
	// Nothing else may follow.
	select {
	case <-open:
		t.Fatalf("single click: menu opened twice")
	case <-show:
		t.Fatalf("single click: window shown")
	case <-time.After(2 * trayClickWindow):
	}
}

func TestTrayClickRouterDoubleShowsWindow(t *testing.T) {
	open := make(chan struct{}, 1)
	show := make(chan struct{}, 1)
	r := newTrayClickRouter(
		func() { open <- struct{}{} },
		func() { show <- struct{}{} },
	)

	r.onClick()
	time.Sleep(trayClickWindow / 4) // well inside the double window
	r.onClick()
	if got := waitEvent(t, open, show, 2*time.Second); got != "show" {
		t.Fatalf("double click: first event = %q, want show", got)
	}
	// The pending menu open must have been cancelled, not delayed past
	// the second click.
	select {
	case <-open:
		t.Fatalf("double click: menu opened anyway")
	case <-time.After(2 * trayClickWindow):
	}
}

func TestTrayClickRouterSlowClicksAreTwoSingles(t *testing.T) {
	open := make(chan struct{}, 2)
	show := make(chan struct{}, 1)
	r := newTrayClickRouter(
		func() { open <- struct{}{} },
		func() { show <- struct{}{} },
	)

	r.onClick()
	if got := waitEvent(t, open, show, 2*time.Second); got != "open" {
		t.Fatalf("first slow click: got %q, want open", got)
	}
	r.onClick() // a full window later — a new single, not a double
	if got := waitEvent(t, open, show, 2*time.Second); got != "open" {
		t.Fatalf("second slow click: got %q, want open (no chaining into a double)", got)
	}
	select {
	case <-show:
		t.Fatalf("two slow clicks must never show the window")
	default:
	}
}
