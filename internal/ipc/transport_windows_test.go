//go:build windows

package ipc

import (
	"strings"
	"testing"
)

// The tests in this package stand up their own pipe listener in-process.
// `go test` normally runs unelevated, so that pipe is owned by the user SID
// rather than SY/BA and Dial's verifyPipeOwner would reject every
// connection. Accept a self-owned pipe for the duration of the test binary
// so the RPC/framing tests exercise the real transport instead of being
// skipped -- there is no CI job running them elevated.
func init() { allowSelfOwnedPipes = true }

func TestBuildPipeSDDLScopesToOwnerSID(t *testing.T) {
	got, err := buildPipeSDDL("S-1-5-21-1-2-3-1001")
	if err != nil {
		t.Fatalf("valid SID rejected: %v", err)
	}
	if !strings.Contains(got, "(A;;GRGW;;;S-1-5-21-1-2-3-1001)") {
		t.Errorf("owner grant missing from descriptor: %s", got)
	}
	// The hole this replaces: IU is the Interactive Users well-known SID,
	// i.e. every logged-on account on the machine.
	if strings.Contains(got, ";;;IU") {
		t.Errorf("descriptor still grants Interactive Users: %s", got)
	}
}

// TestBuildPipeSDDLRefusesUnscoped: no owner SID, or one that isn't a SID,
// must be a startup failure. Falling back to the IU grant here is what made
// the helper controllable by other users look like a working app.
func TestBuildPipeSDDLRefusesUnscoped(t *testing.T) {
	for _, sid := range []string{
		"",
		"not-a-sid",
		"IU",
		"D:(A;;GRGW;;;IU)",
		"S-1-5-21-1-2-3-1001)(A;;GRGW;;;IU",
	} {
		sddl, err := buildPipeSDDL(sid)
		if err == nil {
			t.Errorf("empty/invalid SID %q accepted, descriptor %q", sid, sddl)
			continue
		}
		if sddl != "" {
			t.Errorf("SID %q rejected but still returned a descriptor: %q", sid, sddl)
		}
	}
}

// TestListenRefusesWithoutOwnerSID: the failure has to surface from the real
// entry point the helper calls, not just from the descriptor builder.
func TestListenRefusesWithoutOwnerSID(t *testing.T) {
	l, err := Listen(testSocketPath(t), -1, "")
	if err == nil {
		l.Close()
		t.Fatal("listener opened a pipe with no owner SID")
	}
	if !strings.Contains(err.Error(), "owner SID") {
		t.Errorf("unexpected error, want the owner-SID refusal: %v", err)
	}
}
