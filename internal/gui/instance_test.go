package gui

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestInstanceLockMutualExclusion: a second exclusive acquire must fail
// while the first holder is alive, and must succeed once it releases —
// that release-on-death property is what keeps a crashed instance from
// locking the user out of the app.
func TestInstanceLockMutualExclusion(t *testing.T) {
	path := filepath.Join(t.TempDir(), instanceLockName)
	first, err := acquireInstanceLock(path)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if _, err := acquireInstanceLock(path); err == nil {
		t.Fatal("second acquire succeeded while the lock was held")
	}
	if err := first.Close(); err != nil {
		t.Fatalf("release: %v", err)
	}
	third, err := acquireInstanceLock(path)
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	third.Close()
}

// TestInstanceWakeRoundTrip covers both sides of the wake channel: the
// listener publishes its record into the lock file, the second-launcher
// path reads that record and lands a verified onInstanceWake callback —
// and a mismatched credential lands nowhere.
func TestInstanceWakeRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), instanceLockName)
	lock, err := acquireInstanceLock(path)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer lock.Close()
	if err := startWakeListener(lock, path); err != nil {
		t.Fatalf("startWakeListener: %v", err)
	}

	fired := make(chan struct{}, 4)
	orig := onInstanceWake
	onInstanceWake = func() { fired <- struct{}{} }
	defer func() { onInstanceWake = orig }()

	wakeExistingInstance(path)
	select {
	case <-fired:
	case <-time.After(3 * time.Second):
		t.Fatal("wake ping never reached the listener")
	}

	// A forged ping must be dropped: dial the published port with a bogus
	// record (the token is the credential; only a reader of the lock file
	// has it).
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read record: %v", err)
	}
	port := strings.Fields(strings.TrimSpace(string(data)))[1]
	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		t.Fatalf("dial wake port: %v", err)
	}
	conn.Write([]byte("SHOW 1 65535 deadbeef\n"))
	conn.Close()
	select {
	case <-fired:
		t.Fatal("wake fired on a bad record")
	case <-time.After(500 * time.Millisecond):
	}
}
