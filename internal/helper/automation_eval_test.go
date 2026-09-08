package helper

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/imonior/wireguide-plus/internal/wifi"
)

// TestCoalesceEvalLoop_BurstCollapses pins the discard-stale invariant:
// a burst of N requests collapses to at most one queued evaluation per
// "generation", and events arriving DURING an evaluation trigger exactly
// one follow-up evaluation (which samples the fresh network context) —
// never a per-event backlog.
func TestCoalesceEvalLoop_BurstCollapses(t *testing.T) {
	done := make(chan struct{})
	requests := make(chan struct{}, evalReasonMailboxCap)

	var evals int32
	entered := make(chan int32, 8) // signals each evaluation start
	gates := make(chan struct{}, 8)

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		coalesceEvalLoop(done, requests, func() {
			n := atomic.AddInt32(&evals, 1)
			entered <- n
			<-gates // block the evaluation until the test releases it
		})
	}()

	post := func(n int) {
		for i := 0; i < n; i++ {
			select {
			case requests <- struct{}{}:
			default: // mailbox full → stale event discarded, as designed
			}
		}
	}

	// Generation 1: one request starts the first evaluation; the 99
	// follow-up requests of the burst must collapse into (at most) one
	// queued request.
	post(100)
	if n := <-entered; n != 1 {
		t.Fatalf("first evaluation did not start, got #%d", n)
	}
	post(99) // all stale — mailbox already holds one
	gates <- struct{}{}

	// Generation 2: the one queued request from the burst triggers
	// exactly one follow-up evaluation. Events arriving during it are
	// again collapsed.
	if n := <-entered; n != 2 {
		t.Fatalf("follow-up evaluation did not start, got #%d", n)
	}
	post(99)
	gates <- struct{}{}

	// Generation 3: the request queued during evaluation 2 runs. Nothing
	// more was sent during it, so after this one the loop must go quiet.
	if n := <-entered; n != 3 {
		t.Fatalf("second follow-up did not start, got #%d", n)
	}
	gates <- struct{}{}

	// No backlog: without new requests there must be NO evaluation #4.
	select {
	case n := <-entered:
		t.Fatalf("backlog evaluation #%d ran — coalescing failed", n)
	case <-time.After(150 * time.Millisecond):
	}

	close(done)
	select {
	case <-loopDone:
	case <-time.After(time.Second):
		t.Fatal("loop did not exit after done was closed")
	}
	if got := atomic.LoadInt32(&evals); got != 3 {
		t.Fatalf("evaluations: got %d, want 3 (one per generation, rest discarded)", got)
	}
}

// TestCoalesceEvalLoop_StopsOnDone: a pending request must not keep the
// loop alive after shutdown — and, more importantly, must not fire an
// evaluation once cleanup() has begun tearing tunnels down.
func TestCoalesceEvalLoop_StopsOnDone(t *testing.T) {
	done := make(chan struct{})
	requests := make(chan struct{}, evalReasonMailboxCap)

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		coalesceEvalLoop(done, requests, func() {
			t.Error("evaluation ran after done was already closed")
		})
	}()

	close(done) // shutdown first — the pending request must lose the race
	requests <- struct{}{}

	select {
	case <-loopDone:
	case <-time.After(time.Second):
		t.Fatal("loop did not exit after done was closed")
	}
}

// TestRequestAutomationEval_DropsWhenFull: the non-blocking post never
// deadlocks when the mailbox is already full, and the recorded reason is
// always the most recent trigger's.
func TestRequestAutomationEval_DropsWhenFull(t *testing.T) {
	h := &Helper{
		done:         make(chan struct{}),
		evalRequests: make(chan struct{}, evalReasonMailboxCap),
	}

	h.requestAutomationEval("ssid-change")
	h.requestAutomationEval("network-change") // must not block; drops
	h.requestAutomationEval("poll")           // ditto

	if got := h.latestEvalReason(); got != "poll" {
		t.Errorf("latest reason: got %q, want %q", got, "poll")
	}
	// Exactly one slot is occupied.
	select {
	case <-h.evalRequests:
	default:
		t.Fatal("mailbox empty after requests — post failed")
	}
	select {
	case <-h.evalRequests:
		t.Fatal("mailbox held more than one request — coalescing broken")
	default:
	}
}

// TestReconcileAction_Idempotent pins the reconcile invariants:
//
//  1. Applying a reconcile decision and re-evaluating against the SAME
//     context yields NO further action — the engine converges to a
//     fixpoint in one pass (no connect/disconnect oscillation).
//  2. The manual-off latch suppresses connect decisions unconditionally,
//     at every step, and stays stable.
func TestReconcileAction_Idempotent(t *testing.T) {
	office := wifi.Rule{When: []wifi.Condition{{Type: wifi.CondSSID, SSID: "office"}}, Do: wifi.ActionConnect}
	home := wifi.Rule{When: []wifi.Condition{{Type: wifi.CondSSID, SSID: "home"}}, Do: wifi.ActionDisconnect}
	anyWiFi := wifi.Rule{When: []wifi.Condition{{Type: wifi.CondWiFi}}, Do: wifi.ActionDisconnect}

	policies := []struct {
		name   string
		rules  []wifi.Rule
		def    wifi.Action
		ctx    wifi.NetworkContext
		active bool
	}{
		{"connect-rule-on-active-tunnel", []wifi.Rule{office}, wifi.ActionDisconnect, wifi.NetworkContext{SSID: "office"}, true},
		{"connect-rule-on-idle-tunnel", []wifi.Rule{office}, wifi.ActionDisconnect, wifi.NetworkContext{SSID: "office"}, false},
		{"default-disconnect-on-idle-tunnel", []wifi.Rule{office}, wifi.ActionDisconnect, wifi.NetworkContext{SSID: "elsewhere"}, false},
		{"default-connect-on-idle-tunnel", []wifi.Rule{office}, wifi.ActionConnect, wifi.NetworkContext{SSID: "elsewhere"}, false},
		{"disconnect-rule-on-active-tunnel", []wifi.Rule{home}, wifi.ActionConnect, wifi.NetworkContext{SSID: "home"}, true},
		{"unmanaged-no-rules", nil, wifi.ActionConnect, wifi.NetworkContext{SSID: "office"}, true},
		{"unmanaged-no-match", []wifi.Rule{home}, wifi.ActionDisconnect, wifi.NetworkContext{SSID: "elsewhere"}, true},
		{"wifi-rule-on-active-tunnel", []wifi.Rule{anyWiFi}, wifi.ActionConnect, wifi.NetworkContext{SSID: "cafe"}, true},
	}

	for _, tc := range policies {
		for _, manualOff := range []bool{false, true} {
			active := tc.active

			first := reconcileAction(wifi.EvaluatePolicy(tc.rules, tc.def, tc.ctx), active, manualOff)
			// Apply the decision exactly like the engine does.
			switch first {
			case "connect":
				active = true
			case "disconnect":
				active = false
			}

			// Invariant 1: one pass reaches the fixpoint — a second
			// evaluation against the SAME context must not change tunnel
			// state again (no connect/disconnect oscillation).
			// "skip-manual-off" is a deliberate no-op, so repeating it is
			// stable rather than a second action.
			second := reconcileAction(wifi.EvaluatePolicy(tc.rules, tc.def, tc.ctx), active, manualOff)
			if second == "connect" || second == "disconnect" {
				t.Errorf("%s (manualOff=%v): not idempotent — after %q another state change %q followed",
					tc.name, manualOff, first, second)
			}

			// Invariant 2: while the manual-off latch is set, the engine
			// never decides to connect, at any step.
			if manualOff && first == "connect" {
				t.Errorf("%s (manualOff=%v): connect decided despite manual-off latch", tc.name, manualOff)
			}
		}
	}
}

// TestReconcileAction_Outcomes spot-checks the mapping itself, including
// the cases the idempotency sweep cannot distinguish.
func TestReconcileAction_Outcomes(t *testing.T) {
	cases := []struct {
		state     wifi.DesiredState
		active    bool
		manualOff bool
		want      string
	}{
		{wifi.StateConnect, false, false, "connect"},
		{wifi.StateConnect, true, false, ""},                // already where it should be
		{wifi.StateConnect, false, true, "skip-manual-off"}, // latch wins
		{wifi.StateConnect, true, true, "skip-manual-off"},
		{wifi.StateDisconnect, true, false, "disconnect"},
		{wifi.StateDisconnect, false, false, ""},         // nothing to tear down
		{wifi.StateDisconnect, true, true, "disconnect"}, // latch never blocks a teardown
		{wifi.StateDisconnect, false, true, ""},
		{wifi.StateUnmanaged, true, false, ""}, // no policy → never touched
		{wifi.StateUnmanaged, false, false, ""},
	}
	for i, tc := range cases {
		if got := reconcileAction(tc.state, tc.active, tc.manualOff); got != tc.want {
			t.Errorf("case %d (state=%v active=%v manualOff=%v): got %q, want %q",
				i, tc.state, tc.active, tc.manualOff, got, tc.want)
		}
	}
}
