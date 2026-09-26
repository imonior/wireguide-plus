package gui

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// Single-instance gate. Two GUI processes sharing one helper are a recipe
// for mayhem: whichever one quits first sends its disconnect-on-quit and
// Shutdown RPCs, tearing down the other's tunnels and killing the helper it
// believes it owns. The survivor reads that as a helper crash, pops a fresh
// LaunchDaemon install (with an admin prompt on macOS), and the automation
// rules reconnect — so the user watches "disconnected" and "connected"
// bubbles fire back to back while the window shows the opposite state. One
// GUI per data directory, period.
//
// The gate is a lock file held exclusively for the process lifetime (see
// acquireInstanceLock) plus a 127.0.0.1 wake listener whose address lives
// in that file. A rejected second launch pings the running instance to
// surface its window before exiting, so a re-launch looks like a re-launch
// to the user rather than a silent no-op.

const instanceLockName = "gui-instance.lock"

// instanceLock keeps the acquired lock (and its OS handle/fd) alive for the
// whole process; releasing it would let a second instance in.
var instanceLock io.Closer

// onInstanceWake is what a wake request does. A variable so tests can
// observe the ping without a windowing system.
var onInstanceWake = showDock

// ensureSingleInstance returns true when this process may continue. A false
// return means another instance holds the lock — the running instance has
// been asked (best effort) to show its window, and the caller must exit.
func ensureSingleInstance(configDir string) bool {
	path := filepath.Join(configDir, instanceLockName)
	lock, err := acquireInstanceLock(path)
	if err != nil {
		slog.Info("instance: another GUI instance holds the lock", "path", path, "error", err)
		wakeExistingInstance(path)
		return false
	}
	instanceLock = lock
	if err := startWakeListener(lock, path); err != nil {
		// A missing wake channel degrades, not breaks: the lock still bars
		// the twin; a second launch just exits without the window pop.
		slog.Warn("instance: wake listener unavailable", "error", err)
	}
	return true
}

// startWakeListener binds a 127.0.0.1 socket, publishes
// "pid port token" into the (already-locked) file, and answers wake
// requests that echo that exact record back. The token is why: the file is
// user-readable, so any other local process could learn the port — the
// record doubles as the credential, and only a reader of the lock file can
// forge a valid ping. That keeps "pop the window" gated to same-user
// processes, which is exactly the set that can contain another instance.
func startWakeListener(lock io.WriteCloser, path string) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		ln.Close()
		return err
	}
	record := fmt.Sprintf("%d %d %s", os.Getpid(),
		ln.Addr().(*net.TCPAddr).Port, hex.EncodeToString(token))
	if _, err := lock.Write([]byte(record + "\n")); err != nil {
		ln.Close()
		return err
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go serveWake(c, record)
		}
	}()
	return nil
}

// serveWake answers exactly one wake attempt: the client must send
// "SHOW <record>" with the record it read from the lock file. Anything
// else is dropped without touching the window.
func serveWake(c net.Conn, record string) {
	defer c.Close()
	buf := make([]byte, 256)
	n, err := c.Read(buf)
	if err != nil {
		return
	}
	if strings.TrimSpace(string(buf[:n])) != "SHOW "+record {
		slog.Debug("instance: wake request rejected (bad record)")
		return
	}
	slog.Info("instance: relaunch while running — showing existing window")
	onInstanceWake()
}

// wakeExistingInstance is the second-launcher side: read the running
// instance's record from the lock file and ping its wake listener.
// Best-effort — a stale record (the holder died between our lock failure
// and this read, or the listener was never started) just logs.
func wakeExistingInstance(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		slog.Debug("instance: cannot read lock record", "error", err)
		return
	}
	record := strings.TrimSpace(string(data))
	fields := strings.Fields(record)
	if len(fields) != 3 {
		slog.Debug("instance: malformed lock record", "record", record)
		return
	}
	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", fields[1]))
	if err != nil {
		slog.Debug("instance: wake dial failed", "error", err)
		return
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("SHOW " + record + "\n")); err != nil {
		slog.Debug("instance: wake write failed", "error", err)
	}
}
