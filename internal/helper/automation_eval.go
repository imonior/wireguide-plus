package helper

import (
	"log/slog"
)

// Automation evaluation coalescing.
//
// The engine used to call reevaluateAutomation directly from every
// trigger: SSID transitions, the route-change monitor, the 30s poll and
// the post-connect re-check. reevalMu made those calls safe, but not
// CHEAP: a single Wi-Fi join fires a burst of route-table events plus an
// SSID transition, and each burst element queued its own full evaluation
// (settings read + network-context probe + per-tunnel connect/disconnect)
// behind the mutex. The queued evaluations each re-sampled the network
// context at their turn, so every one but the last acted on a STALE
// context — wasted work at best, mid-flap connect/disconnect flapping at
// worst.
//
// The fix is discard-stale coalescing (deliberately NOT sleep debounce):
//
//   - evalRequests is a mailbox with exactly ONE slot. A trigger posts a
//     request non-blocking; if the slot is full (a request is pending or
//     an evaluation is running with one queued) the event is discarded as
//     stale — the pending/follow-up evaluation samples a FRESH
//     NetworkContext when it runs, so acting on the dropped event
//     separately could only duplicate work against the same or older
//     state. A burst of N events therefore collapses to at most one
//     queued request.
//   - automationEvalLoop pops a request and evaluates immediately (no
//     settle window — lower latency than debounce). If further events
//     arrived DURING the evaluation, one survives in the mailbox and the
//     loop re-evaluates right away with the post-change context: the
//     engine converges on the final network state without ever holding a
//     backlog.
//   - The one hazard this cannot cover is a context change that fires NO
//     event after a transient mid-flap read. The 30s poll (non-darwin)
//     and the next real event remain the safety net, as before.
//
// reevalMu is retained inside reevaluateAutomation as belt-and-braces
// serialisation; with a single consumer goroutine it no longer does the
// heavy lifting.

// evalReasonMailboxCap is the mailbox size. Exactly one slot: one request
// may wait while one evaluation runs; everything else in a burst is stale.
const evalReasonMailboxCap = 1

// requestAutomationEval coalesces an automation trigger into a single
// pending evaluation. Never blocks, so it is safe to call from the
// route-monitor callback, the Wi-Fi monitor goroutine, the poll ticker
// or the post-connect timer. reason is recorded for the log line of the
// evaluation that eventually runs; when the request is dropped the
// reason only survives in this log line.
// ensureEvalMailbox creates the one-slot eval mailbox exactly once. Both
// Run() and requestAutomationEval call it: the field must never be left
// nil, because on a nil channel a send inside a select always takes the
// default branch (request dropped, no error anywhere) and a receive
// blocks forever — the automation engine dies without a single log line.
func (h *Helper) ensureEvalMailbox() {
	h.evalMailboxOnce.Do(func() {
		h.evalRequests = make(chan struct{}, evalReasonMailboxCap)
	})
}

func (h *Helper) requestAutomationEval(reason string) {
	// Regression guard: never post into a nil mailbox.
	h.ensureEvalMailbox()

	// Capture the pending request's reason BEFORE overwriting it, so the
	// drop log can name both the discarded event and the one still queued.
	pending := h.latestEvalReason()
	h.evalReason.Store(reason)
	select {
	case h.evalRequests <- struct{}{}:
	default:
		slog.Debug("automation: coalesced duplicate eval request",
			"dropped_reason", reason,
			"pending_reason", pending)
	}
}

// latestEvalReason returns the reason of the most recent request, for
// log lines. Best-effort: with concurrent triggers the reported reason is
// whichever was stored last, which is the best available description of
// why the evaluation ran.
func (h *Helper) latestEvalReason() string {
	if v, ok := h.evalReason.Load().(string); ok {
		return v
	}
	return ""
}

// automationEvalLoop is the single consumer of the eval mailbox. All
// network-event triggers funnel through requestAutomationEval; this loop
// is the only caller of reevaluateAutomation.
func (h *Helper) automationEvalLoop() {
	coalesceEvalLoop(h.done, h.evalRequests, func() {
		h.reevaluateAutomation(h.latestEvalReason())
	})
}

// coalesceEvalLoop is the platform-free core of automationEvalLoop,
// factored out for testing: pop a request, run the evaluation, repeat —
// discarding everything that queued behind a pending request, and
// re-evaluating once more per request that arrived during an evaluation.
func coalesceEvalLoop(done <-chan struct{}, requests <-chan struct{}, eval func()) {
	for {
		select {
		case <-done:
			return
		case <-requests:
			// Re-check done BEFORE evaluating: shutdown may have begun
			// while this request was waiting, and cleanup() tears down
			// every tunnel — an evaluation racing it could reconnect
			// one. A request that outlived the helper is by definition
			// stale, so drop it.
			select {
			case <-done:
				return
			default:
			}
		}
		eval()
	}
}
