// Command checkrelease verifies the hand-maintained release artifacts that
// bumpversion cannot derive from VERSION:
//
//  1. i18n parity — frontend/src/i18n/{en,zh,zh-TW,ja,ko}.json must expose the
//     exact same set of (flattened) keys, so a missing translation can never
//     hide behind the silent English fallback at runtime.
//  2. Changelog parity — CHANGELOG.md and its four translations must each
//     contain a `## [X.Y.Z]` section for the version being released.
//
// The tool is strictly read-only: the i18n JSON files are CRLF and
// byte-sensitive, so it never writes to them.
//
// Usage (run from the repository root):
//
//	go run ./tools/checkrelease          # check against the VERSION file
//	go run ./tools/checkrelease 1.2.3    # check against an explicit version
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var locales = []string{"en", "zh", "zh-TW", "ja", "ko"}

func main() {
	if len(os.Args) > 2 {
		fmt.Fprintln(os.Stderr, "usage: checkrelease [version]")
		os.Exit(2)
	}
	version := ""
	if len(os.Args) == 2 {
		version = os.Args[1]
	} else {
		raw, err := os.ReadFile("VERSION")
		if err != nil {
			fmt.Fprintln(os.Stderr, "checkrelease: cannot read VERSION:", err)
			os.Exit(1)
		}
		version = strings.TrimSpace(string(raw))
	}
	if version == "" {
		fmt.Fprintln(os.Stderr, "checkrelease: empty version")
		os.Exit(1)
	}

	failed := false
	fail := func(format string, args ...any) {
		failed = true
		fmt.Printf("FAIL  "+format+"\n", args...)
	}

	ok := true
	reference, err := flatKeys("frontend/src/i18n/en.json")
	if err != nil {
		fail("i18n en.json: %v", err)
		ok = false
	}
	if ok {
		fmt.Printf("i18n  en.json: %d keys\n", len(reference))
		for _, locale := range locales[1:] {
			path := "frontend/src/i18n/" + locale + ".json"
			keys, err := flatKeys(path)
			if err != nil {
				fail("i18n %s: %v", path, err)
				continue
			}
			missing, extra := diffKeys(reference, keys)
			if len(missing) > 0 || len(extra) > 0 {
				fail("i18n %s: %d missing, %d extra vs en", path, len(missing), len(extra))
				for _, k := range head(missing, 10) {
					fmt.Println("        -missing", k)
				}
				for _, k := range head(extra, 10) {
					fmt.Println("        +extra  ", k)
				}
				continue
			}
			fmt.Printf("i18n  %s: %d keys, in sync\n", path, len(keys))
		}
	}

	section := regexp.MustCompile(`(?m)^## \[` + regexp.QuoteMeta(version) + `\]`)
	for _, locale := range locales {
		path := "CHANGELOG.md"
		if locale != "en" {
			path = "CHANGELOG." + locale + ".md"
		}
		data, err := os.ReadFile(path)
		if err != nil {
			fail("changelog %s: %v", path, err)
			continue
		}
		if !section.Match(data) {
			fail("changelog %s: no `## [%s]` section", path, version)
			continue
		}
		fmt.Printf("changelog %s: has [%s]\n", path, version)
	}

	if failed {
		os.Exit(1)
	}
	fmt.Println("checkrelease: all consistency checks passed for", version)
}

// flatKeys loads a locale JSON file and returns every leaf key path
// ("popup.connected"), rejecting any value type the locales don't use so a
// structural surprise fails loudly instead of being skipped.
func flatKeys(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var root map[string]any
	dec := json.NewDecoder(bufio.NewReader(strings.NewReader(string(data))))
	if err := dec.Decode(&root); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	keys := map[string]bool{}
	var walk func(prefix string, m map[string]any)
	walk = func(prefix string, m map[string]any) {
		for k, v := range m {
			full := k
			if prefix != "" {
				full = prefix + "." + k
			}
			switch tv := v.(type) {
			case map[string]any:
				walk(full, tv)
			case string:
				keys[full] = true
			default:
				keys[full+"!<"+fmt.Sprintf("%T", v)+">"] = true
			}
		}
	}
	walk("", root)
	return keys, nil
}

func diffKeys(a, b map[string]bool) (missing, extra []string) {
	for k := range a {
		if !b[k] {
			missing = append(missing, k)
		}
	}
	for k := range b {
		if !a[k] {
			extra = append(extra, k)
		}
	}
	return missing, extra
}

func head(s []string, n int) []string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
