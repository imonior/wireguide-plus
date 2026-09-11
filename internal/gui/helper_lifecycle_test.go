package gui

import (
	"sync"
	"testing"
	"time"
)

// TestShutdownGateTripIsIdempotent covers the gate's contract: every quit
// path (tray item, dock, ApplicationWillTerminate, `ctl stop`) funnels into
// doShutdown, which is itself Once-guarded — but the gate must tolerate a
// direct Trip too, because a second close(ch) would panic the app at quit.
func TestShutdownGateTripIsIdempotent(t *testing.T) {
	g := newShutdownGate()
	if g.Tripped() {
		t.Fatal("fresh gate reports tripped")
	}
	g.Trip()
	if !g.Tripped() {
		t.Fatal("Tripped() still false after Trip()")
	}
	select {
	case <-g.Signal():
	default:
		t.Fatal("Signal() channel was not closed by Trip()")
	}
	// Must not panic on repeat.
	g.Trip()
	g.Trip()
}

// TestRecoverHelperSkippedDuringShutdown is the regression test for the quit
// deadlock. doShutdown sends the helper a Shutdown RPC; the health monitor
// sees the socket die and must NOT conclude the helper crashed, because
// recovery ends in `osascript … with administrator privileges`, which is
// started un-cancellable and would park the goroutine forever — and the
// WaitGroup wait in Run() would then block app termination indefinitely.
//
// nil clients/bridge is deliberate: if the gate check ever moves below a
// dereference, this test panics instead of silently passing.
func TestRecoverHelperSkippedDuringShutdown(t *testing.T) {
	g := newShutdownGate()
	g.Trip()

	done := make(chan struct{})
	defer close(done)

	start := time.Now()
	if recoverHelper(nil, nil, t.TempDir(), done, g) {
		t.Fatal("recoverHelper attempted a recovery while shutting down")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("recoverHelper took %v while shutting down; it should return immediately", elapsed)
	}
}

// TestWaitForShutdownGivesUp pins the backstop: a goroutine parked in an
// un-cancellable child process never calls Done, and an unbounded WaitGroup
// wait would hang the app at quit. The wait must be bounded.
func TestWaitForShutdownGivesUp(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1) // never Done — simulates a goroutine stuck in osascript

	start := time.Now()
	waitForShutdown(&wg, 100*time.Millisecond)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("waitForShutdown blocked for %v, want it to give up after ~100ms", elapsed)
	}
	wg.Done() // leave the WaitGroup usable for the rest of the test binary
}

// TestWaitForShutdownReturnsWhenDone makes sure the bounded wait doesn't
// degrade the normal path into a fixed-latency sleep.
func TestWaitForShutdownReturnsWhenDone(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		time.Sleep(10 * time.Millisecond)
		wg.Done()
	}()

	start := time.Now()
	waitForShutdown(&wg, 5*time.Second)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("waitForShutdown returned after %v; should have returned as soon as the group drained (~10ms)", elapsed)
	}
}
