package ipc

import (
	"net"
	"sync"
	"testing"
)

// TestShutdownConcurrentCallers covers the helper's real teardown paths: the
// grace timer in helper.Run, handleShutdown and handleRequestQuit each call
// Shutdown from their own goroutine, and none of them runs under a recover.
// A caller that closed the already-closed shutdown channel therefore panicked
// the process instead of logging — which is how a GUI quit racing the
// disconnect timer used to take the helper down with the tunnel still up.
//
// Each round needs a Server whose channel is still open. The pre-fix guard was
// "select on shutdownCh, take default, close it": once that channel is closed
// the receive succeeds, default is never selected, and no later call on that
// same server can double-close. Reusing one server across rounds therefore made
// every round after the first a no-op — the whole test rested on round 0 and
// caught the bug in roughly 2 of 30 runs.
func TestShutdownConcurrentCallers(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	// The race window is a few instructions wide, so a single round proves
	// little; rounds are the only way to cover it.
	const rounds = 400
	const callers = 64
	for round := 0; round < rounds; round++ {
		server := NewServer(listener)
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < callers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				server.Shutdown()
			}()
		}
		close(start)
		wg.Wait()
	}
}
