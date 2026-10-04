//go:build darwin

package elevate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// testArgs returns a representative Args for plist generation.
func testArgs() Args {
	return Args{
		SocketPath: "/var/run/wireguideplus/wireguideplus.sock",
		SocketUID:  501,
		DataDir:    "/Library/Application Support/wireguideplus",
	}
}

// TestGeneratedPlistLints guards the XML comments embedded in the plist
// template. plutil is what installAndLoadDaemon runs before attempting the
// install, so a malformed template would surface as a failed admin-prompt
// install rather than a build error.
func TestGeneratedPlistLints(t *testing.T) {
	plist := generatePlistContent("/Library/PrivilegedHelperTools/com.wireguideplus.helper", testArgs())

	path := filepath.Join(t.TempDir(), "test.plist")
	if err := os.WriteFile(path, []byte(plist), 0644); err != nil {
		t.Fatalf("write plist: %v", err)
	}
	if out, err := exec.Command("plutil", "-lint", path).CombinedOutput(); err != nil {
		t.Fatalf("plutil -lint rejected the generated plist: %v\n%s", err, out)
	}
}

// TestPlistDoesNotRunAtLoad pins the helper's boot behaviour. RunAtLoad=false
// is the whole reason a closed WireGuide Plus leaves no root process behind: with
// it true, launchd starts the helper at every boot with no GUI, no window and
// no tray icon, and the helper's Wi-Fi automation rules could bring a tunnel
// up while the user believes the app is closed.
//
// The runtime half of the same rule lives in helper.Run, which arms the
// startup grace window unconditionally. Both must hold.
func TestPlistDoesNotRunAtLoad(t *testing.T) {
	plist := generatePlistContent("/Library/PrivilegedHelperTools/com.wireguideplus.helper", testArgs())

	path := filepath.Join(t.TempDir(), "test.plist")
	if err := os.WriteFile(path, []byte(plist), 0644); err != nil {
		t.Fatalf("write plist: %v", err)
	}

	// Read the key back through plutil rather than string-matching, so an
	// XML comment mentioning RunAtLoad can't make this pass spuriously.
	out, err := exec.Command("plutil", "-extract", "RunAtLoad", "raw", "-o", "-", path).CombinedOutput()
	if err != nil {
		t.Fatalf("plutil -extract RunAtLoad: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "false" {
		t.Errorf("RunAtLoad = %q, want \"false\" — the helper must not start at boot; "+
			"users who want WireGuide Plus from login enable auto_start, which installs the GUI LaunchAgent", got)
	}
}

// TestInstallScriptKickstarts pairs with RunAtLoad=false: `launchctl
// bootstrap` only registers the job, so without an explicit kickstart the
// helper never starts and installAndLoadDaemon's readiness poll times out
// with "daemon installed but socket not live after 6s".
func TestInstallScriptKickstarts(t *testing.T) {
	// Mirror the command construction in installAndLoadDaemon closely
	// enough to catch a bootstrap that lost its kickstart.
	src, err := os.ReadFile("spawn_darwin.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	s := string(src)
	if !strings.Contains(s, "launchctl bootstrap system %[") {
		t.Fatal("install script no longer bootstraps the daemon")
	}
	if !strings.Contains(s, "launchctl kickstart -k system/%[") {
		t.Error("install script bootstraps but never kickstarts; with RunAtLoad=false " +
			"the helper process would never start")
	}
}

// TestInstallScriptReplacesBinaryAtomically pins the fix for the
// crash/reinstall loop: the helper binary in /Library/PrivilegedHelperTools
// is the very file a running helper process is executing. `cp -f src dst`
// truncates and rewrites it in place, which corrupts the running process's
// text pages — the helper then dies with a fatal Go runtime error
// (`runtime.findfunc: index out of range`), launchd restarts it, the GUI
// sees a dead helper and reinstalls, and the machine loops with an admin
// prompt every few seconds.
//
// The invariant is: stage a fresh copy, then rename it into place. rename(2)
// is atomic, so a live helper keeps its old (now unlinked) inode intact and
// only fresh starts see the new binary.
func TestInstallScriptReplacesBinaryAtomically(t *testing.T) {
	src, err := os.ReadFile("spawn_darwin.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	s := string(src)

	// 1. The live binary is never written in place.
	if strings.Contains(s, "cp -f %[1]s %[2]s") {
		t.Error("install script copies straight onto the running helper binary; " +
			"that rewrites the text pages of a live process and crashes the helper")
	}

	// 2. It is staged to a temp path and renamed into place instead.
	if !strings.Contains(s, "cp -f %[1]s %[4]s") {
		t.Error("install script does not stage the new binary at a temp path")
	}
	if !strings.Contains(s, "mv -f %[4]s %[2]s") {
		t.Error("install script does not rename the staged binary into place; " +
			"a rename is what makes the replacement atomic")
	}

	// 3. The staging path must share a filesystem with the destination,
	//    otherwise rename(2) silently degrades to copy+truncate — exactly
	//    the in-place rewrite we're avoiding.
	if filepath.Dir(stagingBinary) != filepath.Dir(daemonBinary) {
		t.Errorf("staging binary %q and helper binary %q live on different directories; "+
			"rename(2) is only atomic within one filesystem",
			stagingBinary, daemonBinary)
	}
	if stagingBinary == daemonBinary {
		t.Error("staging binary is the helper binary itself; the replacement is not atomic")
	}

	// 4. The old daemon must be stopped BEFORE its binary is swapped, or
	//    the still-running helper is the one holding the old inode and the
	//    whole exercise is pointless.
	bootout := strings.Index(s, "launchctl bootout system/%[3]s")
	rename := strings.Index(s, "mv -f %[4]s %[2]s")
	if bootout < 0 || rename < 0 {
		t.Fatalf("cannot locate bootout (%d) or rename (%d) in the install script", bootout, rename)
	}
	if bootout > rename {
		t.Error("install script swaps the helper binary before booting out the old daemon")
	}
}

// TestCreatePlistTempIsPrivate pins the two properties the staged plist needs:
// an unpredictable name, so nothing can be pre-planted (including as a
// symlink) at the path we are about to write, and 0600, because the plist
// names the user's socket, data and log directories and only root needs to
// read it for the install copy.
func TestCreatePlistTempIsPrivate(t *testing.T) {
	const body = "<?xml version=\"1.0\"?>\n<plist><true/></plist>\n"

	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		path, err := createPlistTemp(body)
		if err != nil {
			t.Fatalf("createPlistTemp: %v", err)
		}
		t.Cleanup(func() { os.Remove(path) })

		if path == filepath.Join(os.TempDir(), daemonLabel+".plist") {
			t.Errorf("staged plist is at the predictable path %q", path)
		}
		if !strings.HasPrefix(filepath.Base(path), daemonLabel) {
			t.Errorf("staged plist %q is not namespaced to the daemon label", filepath.Base(path))
		}
		// plutil -lint keys off the extension, and the install copies this
		// file into /Library/LaunchDaemons under the .plist name.
		if filepath.Ext(path) != ".plist" {
			t.Errorf("staged plist %q lost its .plist extension", path)
		}

		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("staged plist mode = %O, want 0600", perm)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if string(data) != body {
			t.Errorf("staged content = %q, want %q", data, body)
		}
		seen[path] = true
	}
	if len(seen) != 2 {
		t.Error("two calls staged the plist at the same path")
	}
}

// TestInstallStagesPlistThroughCreatePlistTemp is the same rule at the call
// site: installAndLoadDaemon must not write a world-readable plist to a fixed
// path in the shared temp directory.
func TestInstallStagesPlistThroughCreatePlistTemp(t *testing.T) {
	src, err := os.ReadFile("spawn_darwin.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	s := string(src)

	if strings.Contains(s, "os.WriteFile(tmpPlist") {
		t.Error("install stage writes the plist with os.WriteFile; " +
			"the fixed path it targeted can be pre-created by another account")
	}
	if !strings.Contains(s, "createPlistTemp(plist)") {
		t.Error("install stage no longer routes the plist through createPlistTemp")
	}
	if !strings.Contains(s, "defer os.Remove(tmpPlist)") {
		t.Error("the staged plist is never removed; it holds the user's paths " +
			"for as long as the temp directory lives")
	}
}
