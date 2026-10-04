// Command checkrelease verifies the hand-maintained release artifacts that
// bumpversion cannot derive from VERSION:
//
//  1. i18n parity — frontend/src/i18n/{en,zh,zh-TW,ja,ko}.json must expose the
//     exact same set of (flattened) keys, so a missing translation can never
//     hide behind the silent English fallback at runtime.
//  2. Changelog parity — CHANGELOG.md and its four translations must each
//     contain a `## [X.Y.Z]` section for the version being released.
//  3. Translation-key usage — every `$t('ns.key')` literal in frontend/src must
//     exist in en.json. translate() returns the key itself on a miss, so an
//     unregistered namespace renders as a bare "helper.disconnected" toast in
//     all five languages and parity check 1 cannot see it (the key is absent
//     from every catalogue, so the catalogues still agree).
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
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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

	if ok {
		checkKeysAgainstUsage(reference, fail)
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

// tLiteral matches $t('ns.key'), optionally followed by `+ something` — the
// concatenation form ($t('automation.day_' + d)) whose key cannot be resolved
// statically but whose prefix can.
var tLiteral = regexp.MustCompile(`\$t\(\s*'([A-Za-z0-9_.\-]+)'`)

// tTemplate matches $t(`ns.prefix_${kind}`), the one dynamic form whose key can
// still be checked statically: the literal segments around the interpolation
// constrain the key set.
var tTemplate = regexp.MustCompile("(?s)\\$t\\(\\s*`([^`]*)`")

// checkKeysAgainstUsage fails when a $t() call asks for a key en.json does not
// define. translate() returns the key itself on a miss, so the UI shows
// "helper.disconnected" and the parity check above stays green — the key is
// absent from all five catalogues, so they agree with each other.
func checkKeysAgainstUsage(reference map[string]bool, fail func(string, ...any)) {
	literals := map[string][]string{}
	prefixes := map[string][]string{}
	var dynamic []string
	seen := map[string]bool{}

	err := filepath.WalkDir("frontend/src", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		switch filepath.Ext(path) {
		case ".svelte", ".js":
		default:
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(data), "\n") {
			// Prose that quotes the call form is not a call site: the i18n
			// module's own doc comment reads `$t('some.key')`.
			trimmed := strings.TrimLeft(line, " \t")
			if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") ||
				strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "<!--") {
				continue
			}
			for _, m := range tLiteral.FindAllStringSubmatchIndex(line, -1) {
				key := line[m[2]:m[3]]
				seen[key] = true
				loc := fmt.Sprintf("%s:%d", path, i+1)
				if strings.HasPrefix(strings.TrimLeft(line[m[1]:], " \t"), "+") {
					// 'prefix_' + value: check the prefix instead of the
					// (unresolvable) whole key.
					prefixes[key] = append(prefixes[key], loc)
					continue
				}
				if !reference[key] {
					literals[key] = append(literals[key], loc)
				}
			}
			for _, m := range tTemplate.FindAllStringSubmatch(line, -1) {
				dynamic = append(dynamic, fmt.Sprintf("%s:%d $t(`%s`)", path, i+1, m[1]))
			}
		}
		return nil
	})
	if err != nil {
		fail("i18n usage: cannot scan frontend/src: %v", err)
		return
	}

	// A matching regex that matched nothing would make this check silently
	// agree with everything, so require evidence that it actually read calls.
	if len(seen) == 0 {
		fail("i18n usage: no $t('…') literal found in frontend/src — the extractor matched nothing")
		return
	}

	keys := make([]string, 0, len(literals))
	for k := range literals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		fail("i18n usage: %d $t() key(s) absent from en.json", len(keys))
		for _, k := range head(keys, 20) {
			fmt.Printf("        %s  used at %s\n", k, strings.Join(literals[k], ", "))
		}
	}

	// Prefix forms: at least one key must live under each prefix, otherwise
	// $t('automation.day_' + d) resolves to nothing for every d.
	pkeys := make([]string, 0, len(prefixes))
	for k := range prefixes {
		pkeys = append(pkeys, k)
	}
	sort.Strings(pkeys)
	for _, p := range pkeys {
		matched := 0
		for k := range reference {
			if strings.HasPrefix(k, p) {
				matched++
			}
		}
		if matched == 0 {
			fail("i18n usage: no en.json key starts with %q (used at %s)", p, strings.Join(prefixes[p], ", "))
			continue
		}
		fmt.Printf("i18n  usage: prefix %q resolves to %d key(s)\n", p, matched)
	}

	checked := 0
	for _, entry := range dynamic {
		open := strings.Index(entry, "`")
		close := strings.LastIndex(entry, "`")
		if open < 0 || close <= open {
			continue
		}
		re, err := templateKeyRegexp(entry[open+1 : close])
		if err != nil {
			fail("i18n usage: cannot read %s: %v", entry, err)
			continue
		}
		checked++
		matched := 0
		for k := range reference {
			if re.MatchString(k) {
				matched++
			}
		}
		if matched == 0 {
			fail("i18n usage: %s matches no key in en.json", entry)
		}
	}

	if len(keys) == 0 {
		fmt.Printf("i18n  usage: %d literal keys, %d prefix form(s) and %d template form(s) all present in en.json\n",
			len(seen), len(pkeys), checked)
	}
}

// templateKeyRegexp turns `tools.route_iface_type_${kind}` into an anchored
// pattern whose literal segments must all appear in order.
func templateKeyRegexp(tmpl string) (*regexp.Regexp, error) {
	parts := strings.Split(tmpl, "${")
	expr := make([]string, 0, len(parts)*2)
	expr = append(expr, "^", regexp.QuoteMeta(parts[0]))
	for _, p := range parts[1:] {
		idx := strings.Index(p, "}")
		if idx < 0 {
			return nil, fmt.Errorf("unterminated ${…} in %q", tmpl)
		}
		expr = append(expr, ".*", regexp.QuoteMeta(p[idx+1:]))
	}
	expr = append(expr, "$")
	re, err := regexp.Compile(strings.Join(expr, ""))
	if err != nil {
		return nil, fmt.Errorf("compile %q: %w", tmpl, err)
	}
	return re, nil
}
