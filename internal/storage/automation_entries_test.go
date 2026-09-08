package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/imonior/wireguide-plus/internal/wifi"
)

// Automation policy state reaches config.json through three independent
// write paths, and they must all land the SAME normalized model:
//
//  1. config/migration — a settings file written by a pre-Default-State
//     build (rules still carrying a none_match "Otherwise" rule), loaded
//     + EnsureAutomation'd by the helper engine and persisted by the UI.
//  2. CLI — `wireguideplus ctl automation add/rm/default` mutators
//     (mirrors internal/cli/cli.go automationAdd / automationRm).
//  3. GUI — the Automation editor's SaveAutomationRules (mirrors
//     internal/app/settings_ops.go).
//
// Whatever the path, the engine (helper) and every other entry point
// re-read the file later, so divergence here would mean one entry point's
// writes silently change what the others execute. These tests pin the
// shared invariants: no none_match survives on disk, a rules-less tunnel
// may carry an explicit Default State (default-only policy), Normalize is
// idempotent across a reload, and equal logical policies from different
// entry points evaluate to equal desired states.
func TestAutomationEntryPointsConverge(t *testing.T) {
	officeRule := wifi.Rule{When: []wifi.Condition{{Type: wifi.CondSSID, SSID: "office"}}, Do: wifi.ActionConnect}

	// assertNormalized reloads the store from disk and checks every
	// invariant the three entry points must jointly uphold.
	assertNormalized := func(t *testing.T, dir, tunnel string, wantDef wifi.Action) *Settings {
		t.Helper()
		st, err := NewSettingsStore(dir).Load()
		if err != nil {
			t.Fatalf("reload: %v", err)
		}
		st.EnsureAutomation()
		if st.Automation == nil {
			t.Fatal("automation missing after reload")
		}
		for name, rules := range st.Automation.PerTunnel {
			for i, r := range rules {
				for _, c := range r.When {
					if c.Type == wifi.CondNoneMatch {
						t.Errorf("tunnel %q rule %d: none_match survived on disk (entry-point divergence)", name, i)
					}
				}
			}
		}
		rules := st.Automation.PerTunnel[tunnel]
		def, hasDef := st.Automation.Defaults[tunnel]
		if len(rules) == 0 {
			// Rules-less: either no policy at all, or a default-only
			// policy (an explicit default with an empty entry). Both are
			// valid shapes; what must NOT happen is the entry silently
			// vanishing while a default survives.
			if !hasDef {
				if _, present := st.Automation.PerTunnel[tunnel]; present {
					t.Errorf("tunnel %q has neither rules nor default but a PerTunnel entry — dead config", tunnel)
				}
			}
			if hasDef && def != wantDef {
				t.Errorf("tunnel %q default-only policy: got %q, want %q", tunnel, def, wantDef)
			}
			return st
		}
		if def != wifi.ActionConnect && def != wifi.ActionDisconnect {
			t.Errorf("tunnel %q has rules but default %q is invalid", tunnel, def)
		}
		if def != wantDef {
			t.Errorf("tunnel %q default: got %q, want %q", tunnel, def, wantDef)
		}
		return st
	}

	// --- Path 1: legacy config file with a none_match "Otherwise" rule ---
	dir := filepath.Join(t.TempDir(), "p1")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	legacy := `{
	  "automation": {
	    "per_tunnel_rules": {
	      "work": [
	        {"when": [{"type": "ssid", "ssid": "office"}], "do": "disconnect"},
	        {"when": [{"type": "none_match"}], "do": "connect"},
	        {"when": [{"type": "ssid", "ssid": "never-reachable"}], "do": "connect"}
	      ]
	    }
	  }
	}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	st1, err := NewSettingsStore(dir).Load()
	if err != nil {
		t.Fatal(err)
	}
	st1.EnsureAutomation()
	if err := NewSettingsStore(dir).Save(st1); err != nil {
		t.Fatal(err)
	}
	s1 := assertNormalized(t, dir, "work", wifi.ActionConnect)
	// The trailing rule after none_match was unreachable under the old
	// engine and must be gone (Normalize truncates at the none_match).
	if rules := s1.Automation.PerTunnel["work"]; len(rules) != 1 {
		t.Errorf("path 1: work rules after migration: got %d, want 1 (none_match and everything after removed)", len(rules))
	}
	// Idempotent on the persisted form: another engine load must not
	// change anything.
	if err := NewSettingsStore(dir).Update(func(st *Settings) error {
		st.EnsureAutomation()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	assertNormalized(t, dir, "work", wifi.ActionConnect)

	// --- Path 2: CLI `automation add` mutator (mirrors automationAdd) ---
	dir = filepath.Join(t.TempDir(), "p2")
	store2 := NewSettingsStore(dir)
	if err := store2.Update(func(st *Settings) error {
		st.EnsureAutomation()
		if st.Automation.PerTunnel == nil {
			st.Automation.PerTunnel = map[string][]wifi.Rule{}
		}
		st.Automation.PerTunnel["work"] = append(st.Automation.PerTunnel["work"], officeRule)
		if st.Automation.Defaults == nil {
			st.Automation.Defaults = map[string]wifi.Action{}
		}
		// First rule on a tunnel with no default yet: the CLI fills the
		// conservative disconnect default.
		if _, ok := st.Automation.Defaults["work"]; !ok {
			st.Automation.Defaults["work"] = wifi.ActionDisconnect
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	s2 := assertNormalized(t, dir, "work", wifi.ActionDisconnect)
	// Path 2 (CLI) stored the mirror-image policy — office → connect with
	// the conservative disconnect default — and must decide accordingly.
	if got := wifi.EvaluatePolicy(s2.Automation.PerTunnel["work"], s2.Automation.Defaults["work"], wifi.NetworkContext{SSID: "office"}); got != wifi.StateConnect {
		t.Errorf("CLI policy on office: got %v, want connect", got)
	}
	if got := wifi.EvaluatePolicy(s2.Automation.PerTunnel["work"], s2.Automation.Defaults["work"], wifi.NetworkContext{SSID: "home"}); got != wifi.StateDisconnect {
		t.Errorf("CLI policy on home: got %v, want disconnect (default)", got)
	}

	// CLI `automation rm` of the last rule keeps the tunnel's Default
	// State as a default-only policy (mirrors automationRm) — the tunnel
	// now always converges to its default.
	if err := store2.Update(func(st *Settings) error {
		st.EnsureAutomation()
		rules := st.Automation.PerTunnel["work"]
		st.Automation.PerTunnel["work"] = append(rules[:0:0], rules[1:]...)
		if len(st.Automation.PerTunnel["work"]) == 0 {
			if def := st.Automation.Defaults["work"]; def != wifi.ActionConnect && def != wifi.ActionDisconnect {
				delete(st.Automation.PerTunnel, "work")
				if st.Automation.Defaults != nil {
					delete(st.Automation.Defaults, "work")
				}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	s2 = assertNormalized(t, dir, "work", wifi.ActionDisconnect)
	// The default-only policy must actually decide: with no rules the
	// default applies on every network.
	if got := wifi.EvaluatePolicy(s2.Automation.PerTunnel["work"], s2.Automation.Defaults["work"], wifi.NetworkContext{SSID: "anywhere"}); got != wifi.StateDisconnect {
		t.Errorf("default-only CLI policy: got %v, want disconnect", got)
	}

	// --- Path 3: GUI SaveAutomationRules mutator (mirrors settings_ops) ---
	dir = filepath.Join(t.TempDir(), "p3")
	store3 := NewSettingsStore(dir)
	saveLikeGUI := func(rules []wifi.Rule, defaultState string) error {
		return store3.Update(func(st *Settings) error {
			st.EnsureAutomation()
			def := wifi.Action(defaultState)
			if def != wifi.ActionConnect && def != wifi.ActionDisconnect {
				def = wifi.ActionDisconnect // GUI normalizes an absent default
			}
			if len(rules) == 0 {
				// Zero rules + explicit default = default-only policy; the GUI
				// save path keeps the entry so the engine keeps driving it.
				if st.Automation.PerTunnel == nil {
					st.Automation.PerTunnel = map[string][]wifi.Rule{}
				}
				if st.Automation.Defaults == nil {
					st.Automation.Defaults = map[string]wifi.Action{}
				}
				st.Automation.PerTunnel["work"] = []wifi.Rule{}
				st.Automation.Defaults["work"] = def
				return nil
			}
			if st.Automation.PerTunnel == nil {
				st.Automation.PerTunnel = map[string][]wifi.Rule{}
			}
			if st.Automation.Defaults == nil {
				st.Automation.Defaults = map[string]wifi.Action{}
			}
			st.Automation.PerTunnel["work"] = rules
			st.Automation.Defaults["work"] = def
			return nil
		})
	}
	if err := saveLikeGUI([]wifi.Rule{officeRule}, "connect"); err != nil {
		t.Fatal(err)
	}
	s3 := assertNormalized(t, dir, "work", wifi.ActionConnect)
	// Saving an empty rule list with a default keeps a default-only
	// policy: the tunnel always converges to the default.
	if err := saveLikeGUI(nil, "connect"); err != nil {
		t.Fatal(err)
	}
	s3 = assertNormalized(t, dir, "work", wifi.ActionConnect)
	if got := wifi.EvaluatePolicy(s3.Automation.PerTunnel["work"], s3.Automation.Defaults["work"], wifi.NetworkContext{}); got != wifi.StateConnect {
		t.Errorf("default-only GUI policy: got %v, want connect", got)
	}

	// --- Cross-path: equal logical policies must evaluate equally ---
	// Save through the GUI path the policy the legacy config migrated to
	// (office → disconnect, no-match → connect) and compare decisions on
	// every probed context: the config, CLI and GUI paths must never
	// disagree about what one logical policy means.
	legacyRule := wifi.Rule{When: []wifi.Condition{{Type: wifi.CondSSID, SSID: "office"}}, Do: wifi.ActionDisconnect}
	if err := saveLikeGUI([]wifi.Rule{legacyRule}, "connect"); err != nil {
		t.Fatal(err)
	}
	s3 = assertNormalized(t, dir, "work", wifi.ActionConnect)
	ctxs := []wifi.NetworkContext{
		{SSID: "office"},
		{SSID: "home"},
		{},
	}
	for i, ctx := range ctxs {
		got := wifi.EvaluatePolicy(s3.Automation.PerTunnel["work"], s3.Automation.Defaults["work"], ctx)
		want := wifi.EvaluatePolicy(s1.Automation.PerTunnel["work"], s1.Automation.Defaults["work"], ctx)
		if got != want {
			t.Errorf("ctx %d: GUI-saved policy decided %v, migrated legacy policy decided %v", i, got, want)
		}
	}
	// The migrated semantics must survive: disconnect on the office
	// network (the explicit rule), connect elsewhere (the old none_match
	// "else", now carried by the Default State).
	if got := wifi.EvaluatePolicy(s1.Automation.PerTunnel["work"], s1.Automation.Defaults["work"], wifi.NetworkContext{SSID: "office"}); got != wifi.StateDisconnect {
		t.Errorf("migrated policy on office: got %v, want disconnect", got)
	}
	if got := wifi.EvaluatePolicy(s1.Automation.PerTunnel["work"], s1.Automation.Defaults["work"], wifi.NetworkContext{SSID: "home"}); got != wifi.StateConnect {
		t.Errorf("migrated policy on home: got %v, want connect (from the old none_match else)", got)
	}
}
