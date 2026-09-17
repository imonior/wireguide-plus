package tunnel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"
)

// Endpoint hostname resolution, with a bounded retry.
//
// The failure this exists for: at boot the GUI is launched by the OS login
// item, and the helper immediately after it, before the system resolver is
// usable — the default route may still be missing or the local DNS service
// may not have started. The first LookupHost then fails with a transient
// error ("no such host" / "server misbehaving" / connection refused) and
// Connect aborted outright. That is the "connected three times, all failed"
// pattern in boot logs: the automation engine retried, but each evaluation
// landed inside the same window and hit the same one-shot failure.
//
// A single lookup is not wrong for a steady-state reconnect — the resolver
// is up and the answer is authoritative. It is wrong for the first seconds
// after boot, when the SAME question gets a different answer a moment
// later. So: retry, but bounded, and only while the error looks transient.
const (
	// endpointResolveBudget bounds the WHOLE retry sequence, not one
	// attempt: Connect blocks the caller for at most this long before
	// reporting the same failure it always did.
	endpointResolveBudget = 15 * time.Second
	// endpointResolveAttemptTimeout is per attempt. A resolver that is up
	// answers in milliseconds; this is already generous, and it keeps one
	// hung attempt from eating the entire budget.
	endpointResolveAttemptTimeout = 5 * time.Second
	// endpointResolveBackoff is the pause before the second attempt; it
	// doubles each round (800ms, 1.6s, 3.2s …) so a resolver that comes up
	// late is picked up promptly without hammering it.
	endpointResolveBackoff = 800 * time.Millisecond
)

// resolveEndpointWithRetry resolves host, retrying transient resolver
// failures until ctx is done. The returned error is the last resolver
// error, so the caller's message still names the real cause.
func resolveEndpointWithRetry(ctx context.Context, host string) ([]string, error) {
	return resolveWithRetry(ctx, host, lookupHostOnce, endpointResolveBackoff)
}

// lookupFunc is the resolver used by resolveWithRetry. Injectable so the
// retry policy can be tested without real DNS, real delays or a network.
type lookupFunc func(context.Context, string) ([]string, error)

// resolveWithRetry is the policy behind resolveEndpointWithRetry, with the
// resolver and the first backoff lifted out as parameters.
func resolveWithRetry(ctx context.Context, host string, lookup lookupFunc, backoff time.Duration) ([]string, error) {
	var lastErr error
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, err
		}

		ips, err := lookup(ctx, host)
		if err == nil {
			if attempt > 1 {
				// Worth a line at Info: it is the difference between "the
				// tunnel took a while to come up" and "the tunnel failed",
				// which is exactly what a boot-time bug report needs.
				slog.Info("endpoint resolved after retry",
					"category", "network", "host", host,
					"attempt", attempt, "ips", ips)
			}
			return ips, nil
		}
		lastErr = err

		if !isTransientResolveError(err) {
			return nil, err
		}
		if attempt == 1 {
			slog.Info("endpoint resolve failed, retrying",
				"category", "network", "host", host, "error", err)
		} else {
			slog.Debug("endpoint resolve still failing",
				"category", "network", "host", host, "attempt", attempt, "error", err)
		}

		// Never sleep past the budget: a paused attempt that outlives ctx
		// would turn "give up after 15s" into "give up after 15s + backoff".
		select {
		case <-ctx.Done():
			slog.Warn("endpoint resolve gave up",
				"category", "network", "host", host,
				"attempts", attempt, "error", lastErr)
			return nil, lastErr
		case <-time.After(backoff):
		}
		if backoff < endpointResolveBudget/2 {
			backoff *= 2
		}
	}
}

// lookupHostOnce performs a single bounded resolution. Split out so the
// retry loop above stays readable, and so the "no addresses" case is
// normalised in one place (LookupHost can return (nil, nil) on some
// resolver edge cases).
func lookupHostOnce(ctx context.Context, host string) ([]string, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, endpointResolveAttemptTimeout)
	defer cancel()

	ips, err := net.DefaultResolver.LookupHost(attemptCtx, host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no addresses found for %q", host)
	}
	return ips, nil
}

// isTransientResolveError reports whether waiting could plausibly change
// the outcome.
//
// Everything the net package reports for a lookup surfaces as a
// *net.DNSError, so the question is which of those are worth another try.
// A timeout or a temporary failure is the boot case verbatim.
//
// The awkward one is IsNotFound: it is BOTH a genuinely nonexistent name
// and what the resolver synthesises when it cannot reach any server at
// boot ("no such host"), and the two are indistinguishable here. Treating
// it as transient costs a bounded wait on a typo; treating it as fatal
// costs a failed connect on every boot. The bounded wait is the cheaper
// mistake — and the budget caps it.
func isTransientResolveError(err error) bool {
	if err == nil {
		return false
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}
