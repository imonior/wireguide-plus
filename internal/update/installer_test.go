package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellQuote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"simple", "'simple'"},
		{"/Applications/My App.app", "'/Applications/My App.app'"},
		{"it's", "'it'\\''s'"},
		{`a"b`, `'a"b'`},
		{"", "''"},
	}
	for _, c := range cases {
		if got := shellQuote(c.in); got != c.want {
			t.Errorf("shellQuote(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestFindAppBundle(t *testing.T) {
	dir := t.TempDir()
	appDir := filepath.Join(dir, "wireguideplus.app")
	if err := os.MkdirAll(filepath.Join(appDir, "Contents", "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := findAppBundle(dir); got != appDir {
		t.Errorf("findAppBundle = %q, want %q", got, appDir)
	}

	empty := filepath.Join(dir, "empty")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := findAppBundle(empty); got != "" {
		t.Errorf("findAppBundle(empty) = %q, want \"\"", got)
	}
}

func TestDirWritable(t *testing.T) {
	if !dirWritable(t.TempDir()) {
		t.Error("dirWritable on a fresh temp dir should be true")
	}
}

func TestDarwinInstallTarget(t *testing.T) {
	target, err := darwinInstallTarget()
	if err != nil {
		t.Fatalf("darwinInstallTarget: %v", err)
	}
	if !strings.HasSuffix(target, "wireguideplus.app") {
		t.Errorf("darwinInstallTarget() = %q, want path ending in wireguideplus.app", target)
	}
}

func TestDarwinBundleDirFromExecutable(t *testing.T) {
	// Must not panic and must return either "" (bare binary / non-macOS)
	// or a path ending in .app (a macOS bundle).
	dir := darwinBundleDirFromExecutable()
	if dir != "" && !strings.HasSuffix(dir, ".app") {
		t.Errorf("darwinBundleDirFromExecutable() = %q, want \"\" or a *.app path", dir)
	}
}

func TestDarwinInstallScript(t *testing.T) {
	newApp := "/tmp/new/wireguideplus.app"
	target := "/Applications/wireguideplus.app"
	script := darwinInstallScript(newApp, target)

	for _, want := range []string{
		"#!/bin/sh",
		"set -e",
		"killall wireguideplus",
		"/bin/rm -rf '/Applications/wireguideplus.app'",
		"/usr/bin/ditto '/tmp/new/wireguideplus.app' '/Applications/wireguideplus.app'",
		"com.apple.quarantine",
		"launchctl asuser",
		"/usr/bin/open '/Applications/wireguideplus.app'",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("install script missing %q\n---\n%s", want, script)
		}
	}

	// Paths with spaces must be fully single-quoted.
	spaced := darwinInstallScript("/tmp/new dir/wireguideplus.app", "/Applications/My Apps/wireguideplus.app")
	if !strings.Contains(spaced, "ditto '/tmp/new dir/wireguideplus.app' '/Applications/My Apps/wireguideplus.app'") {
		t.Errorf("spaced paths not quoted as single words\n---\n%s", spaced)
	}
}
