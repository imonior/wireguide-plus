// Command bumpversion keeps every build/package metadata file in sync with the
// single source of truth for the app version — the repository-root VERSION file.
//
// Go binaries don't need this tool: build/*/Taskfile.yml read VERSION at build
// time and inject it via -ldflags into internal/version.Version. But the
// packaging metadata below is static and consumed by third-party toolchains
// (wails3, goversioninfo, makensis, MSIX, nfpm, the .app bundle), so it must be
// rewritten on each version bump:
//
//   - build/config.yml                     info.version
//   - build/windows/info.json              fixed.file_version / info.ProductVersion
//   - build/windows/versioninfo.json       StringFileInfo 4-part versions only
//     (FixedFileInfo numbers stay 0 — see tools/genverinfo; the committed file
//     is the reference for versioninfo.json.tmpl and the build itself uses the
//     genverinfo-rendered versioninfo.gen.json)
//   - build/windows/wails.exe.manifest     assemblyIdentity version (4-part)
//   - build/windows/nsis/wails_tools.nsh   INFO_PRODUCTVERSION
//   - build/windows/msix/template.xml      Package Identity Version (4-part)
//   - build/windows/msix/app_manifest.xml  Identity Version (4-part)
//   - build/linux/nfpm/nfpm.yaml           version
//   - build/darwin/Info.plist / Info.dev.plist  CFBundleVersion / CFBundleShortVersionString
//
// Usage (run from the repository root):
//
//	go run ./tools/bumpversion          # sync from VERSION
//	go run ./tools/bumpversion 1.1.2    # write VERSION, then sync
//
// Stdlib-only so CI needs nothing beyond the Go toolchain, mirroring
// tools/genverinfo and tools/updatesign.
package main

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type target struct {
	path string
	re   *regexp.Regexp
	repl string
}

// newTarget builds a target whose pattern has exactly two capture groups
// surrounding the version digits, e.g. `(prefix )\d+\.\d+\.\d+( suffix)`.
func newTarget(path, pattern, newVer string) target {
	re := regexp.MustCompile(pattern)
	if strings.Count(pattern, "(") != 2 {
		panic("bumpversion: pattern for " + path + " must have exactly two capture groups")
	}
	return target{path: path, re: re, repl: "${1}" + newVer + "${2}"}
}

func main() {
	newVer := ""
	for _, a := range os.Args[1:] {
		if a != "" {
			newVer = a
			break
		}
	}
	if newVer == "" {
		data, err := os.ReadFile("VERSION")
		if err != nil {
			fmt.Fprintln(os.Stderr, "bumpversion: no version argument and cannot read VERSION:", err)
			fmt.Fprintln(os.Stderr, "usage: go run ./tools/bumpversion [x.y.z]")
			os.Exit(1)
		}
		newVer = strings.TrimSpace(string(data))
	}
	if !validThreePart(newVer) {
		fmt.Fprintf(os.Stderr, "bumpversion: %q is not a valid x.y.z version\n", newVer)
		os.Exit(1)
	}
	fourPart := newVer + ".0"

	targets := []target{
		newTarget("build/config.yml", `(version: ")\d+\.\d+\.\d+(")`, newVer),
		newTarget("build/windows/info.json", `("file_version": ")\d+\.\d+\.\d+\.\d+(")`, fourPart),
		newTarget("build/windows/info.json", `("ProductVersion": ")\d+\.\d+\.\d+\.\d+(")`, fourPart),
		newTarget("build/windows/versioninfo.json", `("FileVersion": ")\d+\.\d+\.\d+\.\d+(")`, fourPart),
		newTarget("build/windows/versioninfo.json", `("ProductVersion": ")\d+\.\d+\.\d+\.\d+(")`, fourPart),
		newTarget("build/windows/wails.exe.manifest", `(<assemblyIdentity type="win32" name="com\.imonior\.wireguide-plus" version=")\d+\.\d+\.\d+\.\d+(")`, fourPart),
		newTarget("build/windows/nsis/wails_tools.nsh", `(!define INFO_PRODUCTVERSION ")\d+\.\d+\.\d+(")`, newVer),
		newTarget("build/windows/msix/template.xml", `(\n\s*Version=")\d+\.\d+\.\d+\.\d+(")`, fourPart),
		newTarget("build/windows/msix/app_manifest.xml", `(\n\s*Version=")\d+\.\d+\.\d+\.\d+(")`, fourPart),
		newTarget("build/linux/nfpm/nfpm.yaml", `(version: ")\d+\.\d+\.\d+(")`, newVer),
		// Match only the version keys — a bare `(<string>)\d+\.\d+\.\d+(</string>)`
		// pattern also rewrote LSMinimumSystemVersion (the minimum supported
		// macOS, 10.15.0) into the app version, corrupting the deployed .app.
		newTarget("build/darwin/Info.plist", `(<key>CFBundleVersion</key>\s+<string>)\d+\.\d+\.\d+(</string>)`, newVer),
		newTarget("build/darwin/Info.plist", `(<key>CFBundleShortVersionString</key>\s+<string>)\d+\.\d+\.\d+(</string>)`, newVer),
		newTarget("build/darwin/Info.dev.plist", `(<key>CFBundleVersion</key>\s+<string>)\d+\.\d+\.\d+(</string>)`, newVer),
		newTarget("build/darwin/Info.dev.plist", `(<key>CFBundleShortVersionString</key>\s+<string>)\d+\.\d+\.\d+(</string>)`, newVer),
	}

	// Note: versioninfo.json's FixedFileInfo version numbers are intentionally
	// left at 0 (see tools/genverinfo): goversioninfo v1.7 declares its struct
	// as Major/Minor/Patch/Build, so explicit numbers produce a corrupt binary
	// version. The 4-part StringFileInfo strings above are the single input;
	// goversioninfo derives FixedFileInfo from them.

	// Group targets by path so each file is read/written once.
	byPath := map[string][]target{}
	for _, t := range targets {
		byPath[t.path] = append(byPath[t.path], t)
	}

	// Pre-flight: every pattern must match its file, and this runs before a
	// single byte is written. The old behaviour printed "SKIP <file>" and
	// exited 0, so a bump could rewrite six of ten files and the release then
	// shipped artefacts that disagree about their version — which is what the
	// Release workflow's preflight job now has to catch after the fact. An
	// unmatched pattern is a bug in this tool or a restructured file, and
	// either way the tree must stay untouched until it is fixed.
	contents := make(map[string][]byte, len(byPath))
	var unmatched []string
	for path, tlist := range byPath {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bumpversion: read", path, ":", err)
			os.Exit(1)
		}
		contents[path] = data
		for _, t := range tlist {
			if !t.re.MatchString(string(data)) {
				unmatched = append(unmatched, fmt.Sprintf("%s  pattern %s", path, t.re))
			}
		}
	}
	if len(unmatched) > 0 {
		sort.Strings(unmatched)
		fmt.Fprintln(os.Stderr, "bumpversion: no version pattern matched — nothing was written:")
		for _, u := range unmatched {
			fmt.Fprintln(os.Stderr, "  ", u)
		}
		os.Exit(1)
	}

	changed := 0
	for path, tlist := range byPath {
		out := string(contents[path])
		contentChanged := false
		for _, t := range tlist {
			next := t.re.ReplaceAllString(out, t.repl)
			if next != out {
				contentChanged = true
			}
			out = next
		}
		if !contentChanged {
			fmt.Printf("OK    %s (already %s)\n", path, newVer)
			continue
		}
		if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "bumpversion: write", path, ":", err)
			os.Exit(1)
		}
		fmt.Printf("OK    %s -> %s\n", path, newVer)
		changed++
	}

	if err := os.WriteFile("VERSION", []byte(newVer), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "bumpversion: write VERSION:", err)
		os.Exit(1)
	}
	fmt.Printf("VERSION written: %s (%d files updated)\n", newVer, changed)
}

func validThreePart(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return false
		}
	}
	return true
}
