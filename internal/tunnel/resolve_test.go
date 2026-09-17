package tunnel

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

// The retry policy exists for exactly one scenario: a boot-time autostart
// whose first lookups land before the system resolver answers. These tests
// pin the policy, not the network.

func TestResolveWithRetry_SucceedsFirstTry(t *testing.T) {
	calls := 0
	lookup := func(context.Context, string) ([]string, error) {
		calls++
		return []string{"203.0.113.7"}, nil
	}

	ips, err := resolveWithRetry(context.Background(), "peer.example", lookup, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("want exactly 1 lookup, got %d", calls)
	}
	if len(ips) != 1 || ips[0] != "203.0.113.7" {
		t.Fatalf("unexpected ips: %v", ips)
	}
}

func TestResolveWithRetry_RecoversFromTransientFailures(t *testing.T) {
	transient := &net.DNSError{Err: "no such host", Name: "peer.example", IsNotFound: true}
	calls := 0
	lookup := func(context.Context, string) ([]string, error) {
		calls++
		if calls < 3 {
			// Boot: the resolver is not up yet.
			return nil, transient
		}
		return []string{"198.51.100.9"}, nil
	}

	ips, err := resolveWithRetry(context.Background(), "peer.example", lookup, time.Millisecond)
	if err != nil {
		t.Fatalf("want success after retries, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("want 3 attempts, got %d", calls)
	}
	if len(ips) != 1 || ips[0] != "198.51.100.9" {
		t.Fatalf("unexpected ips: %v", ips)
	}
}

// A configuration error must not be retried: waiting cannot fix a bad
// address, and the user is staring at a Connect button.
func TestResolveWithRetry_DoesNotRetryPermanentError(t *testing.T) {
	permanent := errors.New("malformed endpoint")
	calls := 0
	lookup := func(context.Context, string) ([]string, error) {
		calls++
		return nil, permanent
	}

	_, err := resolveWithRetry(context.Background(), "peer.example", lookup, time.Millisecond)
	if !errors.Is(err, permanent) {
		t.Fatalf("want the resolver error back, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("want 1 attempt for a permanent error, got %d", calls)
	}
}

func TestResolveWithRetry_GivesUpWhenBudgetExpires(t *testing.T) {
	transient := &net.DNSError{Err: "server misbehaving", Name: "peer.example", IsTemporary: true}
	calls := 0
	lookup := func(context.Context, string) ([]string, error) {
		calls++
		return nil, transient
	}

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	_, err := resolveWithRetry(ctx, "peer.example", lookup, 10*time.Millisecond)
	if err == nil {
		t.Fatal("want failure once the budget is spent")
	}
	// The last resolver error is returned, not a generic "timeout": the
	// caller's message has to name the real DNS failure.
	if !errors.Is(err, transient) {
		t.Fatalf("want the last resolver error, got %v", err)
	}
	if calls < 2 {
		t.Fatalf("want several attempts before giving up, got %d", calls)
	}
}

func TestIsTransientResolveError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"not found", &net.DNSError{Err: "no such host", IsNotFound: true}, true},
		{"temporary", &net.DNSError{Err: "server misbehaving", IsTemporary: true}, true},
		{"dns timeout", &net.DNSError{Err: "i/o timeout", IsTimeout: true}, true},
		{"context deadline", context.DeadlineExceeded, true},
		{"plain error", errors.New("boom"), false},
		{"net timeout", timeoutErr{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTransientResolveError(tc.err); got != tc.want {
				t.Fatalf("isTransientResolveError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// timeoutErr is a bare net.Error that timed out — the shape a stalled
// dial/POSIX resolver surfaces, with no *net.DNSError wrapper.
type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return false }
