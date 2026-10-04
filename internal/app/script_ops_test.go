package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imonior/wireguide-plus/internal/storage"
)

func boolPtr(v bool) *bool { return &v }

const sampleConf = `[Interface]
PrivateKey = AAAA
Address = 10.0.0.2/24
PostUp = echo existing

[Peer]
PublicKey = BBBB
AllowedIPs = 0.0.0.0/0
`

func TestGetHookFromText(t *testing.T) {
	if got := getHookFromText(sampleConf, "PostUp"); got != "echo existing" {
		t.Errorf("PostUp = %q, want %q", got, "echo existing")
	}
	if got := getHookFromText(sampleConf, "PreUp"); got != "" {
		t.Errorf("PreUp = %q, want empty", got)
	}
	// Hooks declared inside [Peer] must NOT be picked up.
	peerConf := "[Interface]\nPrivateKey = A\n\n[Peer]\nPublicKey = B\nPreUp = evil\n"
	if got := getHookFromText(peerConf, "PreUp"); got != "" {
		t.Errorf("PreUp from [Peer] leaked: %q", got)
	}
	// Multiple lines join for display.
	multi := "[Interface]\nPreUp = a\nPreUp = b\n"
	if got := getHookFromText(multi, "PreUp"); got != "a; b" {
		t.Errorf("PreUp = %q, want %q", got, "a; b")
	}
}

func TestSetHookInTextInsert(t *testing.T) {
	out := setHookInText(sampleConf, "PreUp", `bash "/x/run.sh"`)
	if !strings.Contains(out, `PreUp = bash "/x/run.sh"`) {
		t.Fatalf("inserted line missing:\n%s", out)
	}
	// Must land inside [Interface], before [Peer].
	iface := strings.Index(out, "[Interface]")
	peer := strings.Index(out, "[Peer]")
	line := strings.Index(out, "PreUp = ")
	if !(iface < line && line < peer) {
		t.Fatalf("line inserted outside [Interface]:\n%s", out)
	}
	// Existing PostUp line untouched.
	if !strings.Contains(out, "PostUp = echo existing") {
		t.Fatalf("PostUp clobbered:\n%s", out)
	}
}

func TestSetHookInTextReplaceAndClear(t *testing.T) {
	out := setHookInText(sampleConf, "PostUp", "echo new")
	if strings.Contains(out, "echo existing") {
		t.Fatalf("old PostUp not replaced:\n%s", out)
	}
	if strings.Count(out, "PostUp = ") != 1 {
		t.Fatalf("expected exactly one PostUp line:\n%s", out)
	}
	cleared := setHookInText(out, "PostUp", "")
	if strings.Contains(cleared, "PostUp") {
		t.Fatalf("clear left residue:\n%s", cleared)
	}
}

func TestSetHookInTextCaseInsensitiveMatch(t *testing.T) {
	conf := "[Interface]\npreup = old\n"
	out := setHookInText(conf, "PreUp", "new")
	if strings.Contains(out, "old") {
		t.Fatalf("lowercase preup not matched:\n%s", out)
	}
	if !strings.Contains(out, "PreUp = new") {
		t.Fatalf("new line missing:\n%s", out)
	}
}

func TestSetHookInTextCRLF(t *testing.T) {
	conf := strings.ReplaceAll(sampleConf, "\n", "\r\n")
	out := setHookInText(conf, "PreUp", "echo hi")
	if !strings.Contains(out, "PreUp = echo hi\r") {
		t.Fatalf("CRLF not preserved on inserted line:\n%q", out)
	}
	if !strings.Contains(out, "PrivateKey = AAAA\r\n") {
		t.Fatalf("existing CRLF lines damaged:\n%q", out)
	}
}

func TestScriptInvocation(t *testing.T) {
	cases := map[string]string{
		`C:\s\a.ps1`: `powershell -NoProfile -ExecutionPolicy Bypass -File "C:\s\a.ps1"`,
		`C:\s\a.bat`: `"C:\s\a.bat"`,
		`C:\s\a.cmd`: `"C:\s\a.cmd"`,
		`/x/a.sh`:    `bash "/x/a.sh"`,
		`/x/a`:       `"/x/a"`,
	}
	for in, want := range cases {
		if got := scriptInvocation(in); got != want {
			t.Errorf("scriptInvocation(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveScriptRef(t *testing.T) {
	if p, ok := resolveScriptRef(`powershell -NoProfile -ExecutionPolicy Bypass -File "C:\s\a.ps1"`); !ok || p != `C:\s\a.ps1` {
		t.Errorf("ps1 ref: %q %v", p, ok)
	}
	if p, ok := resolveScriptRef(`bash "/x/a.sh"`); !ok || p != "/x/a.sh" {
		t.Errorf("sh ref: %q %v", p, ok)
	}
	if p, ok := resolveScriptRef(`"C:\s\a.bat"`); !ok || p != `C:\s\a.bat` {
		t.Errorf("bat ref: %q %v", p, ok)
	}
	// Inline commands must stay inline.
	for _, cmd := range []string{"iptables -A FORWARD -i %i", "", `echo "hello world"`} {
		if _, ok := resolveScriptRef(cmd); ok {
			t.Errorf("inline %q misdetected as file ref", cmd)
		}
	}
}

// roundTripConf is a valid WireGuard config (keys from the storage tests).
const roundTripConf = `[Interface]
PrivateKey = yAnz5TF+lXXJte14tji3zlMNq+hd2rYUIgJBgB3fBmk=
Address = 10.0.0.2/24

[Peer]
PublicKey = xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg=
AllowedIPs = 0.0.0.0/0
`

// useHome points every per-user config location at one scratch directory so
// storage.GetPaths() resolves the whole app data folder under the test's
// TempDir. Each platform branch reads a different variable, so all of them
// are set — the unused ones are simply ignored.
func useHome(t *testing.T, home string) {
	t.Helper()
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatalf("mkdir scratch home: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
}

func storeFor(t *testing.T) (*TunnelService, *storage.Paths) {
	t.Helper()
	paths, err := storage.GetPaths()
	if err != nil {
		t.Fatalf("resolve app paths: %v", err)
	}
	for _, dir := range []string{paths.TunnelsDir, paths.ConfigDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	return &TunnelService{
		tunnelStore:   storage.NewTunnelStore(paths.TunnelsDir),
		settingsStore: storage.NewSettingsStore(paths.ConfigDir),
	}, paths
}

func zipEntryNames(t *testing.T, path string) map[string]bool {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open export: %v", err)
	}
	defer r.Close()
	names := map[string]bool{}
	for _, f := range r.File {
		names[filepath.ToSlash(f.Name)] = true
	}
	return names
}

func metaOf(t *testing.T, svc *TunnelService, name string) storage.TunnelMeta {
	t.Helper()
	meta, err := svc.tunnelStore.LoadMeta(name)
	if err != nil {
		t.Fatalf("load meta of %s: %v", name, err)
	}
	return *meta
}

// TestSettingsZipRoundTripRestoresPolicySidecars covers the whole backup
// path across two scratch "machines": everything that lives outside the
// .conf has to survive export → import.
//
// Before this, the archive carried only tunnels/*.conf, scripts/* and
// config.json, so a restore brought back the connection and silently lost
// traffic protection, the DNS resolve path, the automation exemption, the
// monitored domains, the notes and the probe targets — the per-tunnel
// policy the sidecars hold.
func TestSettingsZipRoundTripRestoresPolicySidecars(t *testing.T) {
	scratch := t.TempDir()

	useHome(t, filepath.Join(scratch, "source"))
	src, srcPaths := storeFor(t)
	if _, err := src.ImportConfig("wg-home", roundTripConf); err != nil {
		t.Fatalf("seed source tunnel: %v", err)
	}
	want := storage.TunnelMeta{
		TrafficProtect:       true,
		DNSResolvePath:       true,
		AutomationDisabled:   true,
		UseAsDefaultDNS:      true,
		Notes:                "office uplink",
		Domains:              []string{"intranet.example.com"},
		LatencyProbeTargets:  []string{"", "", "1.2.3.4", ""},
		KeepConnectionOnIdle: boolPtr(false),
	}
	if err := src.tunnelStore.SaveMeta("wg-home", &want); err != nil {
		t.Fatalf("seed source meta: %v", err)
	}
	if err := src.settingsStore.Update(func(st *storage.Settings) error {
		st.Language = "ko"
		st.LogLevel = "debug"
		return nil
	}); err != nil {
		t.Fatalf("seed source settings: %v", err)
	}

	archive := filepath.Join(scratch, "backup.zip")
	if err := src.writeSettingsZip(archive); err != nil {
		t.Fatalf("writeSettingsZip: %v", err)
	}
	// The export half of the bug: a package that never contains the sidecar
	// can't restore it, however good the import pass is.
	entries := zipEntryNames(t, archive)
	for _, want := range []string{"tunnels/wg-home.conf", "tunnels/wg-home.meta.json", "config.json"} {
		if !entries[want] {
			t.Errorf("export is missing %s (got %v)", want, entries)
		}
	}

	r, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatalf("reopen export: %v", err)
	}
	defer r.Close()

	// A machine that has nothing yet.
	useHome(t, filepath.Join(scratch, "target"))
	dst, _ := storeFor(t)
	res, err := dst.importSettingsReader(&r.Reader)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if !res.SettingsApplied {
		t.Error("imported config.json was not applied")
	}
	gotSettings, err := dst.settingsStore.Load()
	if err != nil {
		t.Fatalf("load imported settings: %v", err)
	}
	if gotSettings.Language != "ko" || gotSettings.LogLevel != "debug" {
		t.Errorf("imported settings did not land: language=%q log_level=%q",
			gotSettings.Language, gotSettings.LogLevel)
	}
	got := metaOf(t, dst, "wg-home")
	if !got.TrafficProtect || !got.DNSResolvePath || !got.AutomationDisabled || !got.UseAsDefaultDNS {
		t.Errorf("policy flags lost: %+v", got)
	}
	if got.Notes != "office uplink" || strings.Join(got.Domains, ",") != "intranet.example.com" {
		t.Errorf("notes/domains lost: %+v", got)
	}
	if strings.Join(got.ProbeTargets(), ",") != ",,1.2.3.4," {
		t.Errorf("probe target lost: %v", got.ProbeTargets())
	}
	if got.KeepConnectionOnIdle == nil || *got.KeepConnectionOnIdle {
		t.Errorf("idle override lost: %v", got.KeepConnectionOnIdle)
	}

	// Importing into a machine that ALREADY has wg-home de-duplicates the
	// name to wg-home-1. The sidecar is keyed by the archive's base name, so
	// without the base→imported-name map it would be dropped exactly when it
	// matters most — merging a second machine's policies into one install.
	if _, err := dst.importSettingsReader(&r.Reader); err != nil {
		t.Fatalf("second import: %v", err)
	}
	dup := metaOf(t, dst, "wg-home-1")
	if !dup.TrafficProtect || !dup.DNSResolvePath || dup.Notes != "office uplink" {
		t.Errorf("policy lost when the tunnel was renamed on import: %+v", dup)
	}

	_ = srcPaths
}

// TestImportSettingsReaderRejectsUnparsableConfig: the archive's config.json
// used to be written with a bare os.WriteFile, so a truncated or hand-edited
// settings document landed on disk and the app quarantined it to
// .corrupt on the next start — the user lost every setting with no way back.
// The import now validates before replacing, and the replace itself goes
// through the settings store (file lock + atomic rename).
func TestImportSettingsReaderRejectsUnparsableConfig(t *testing.T) {
	scratch := t.TempDir()
	useHome(t, filepath.Join(scratch, "home"))
	svc, paths := storeFor(t)
	if err := svc.settingsStore.Update(func(st *storage.Settings) error {
		st.Language = "ja"
		return nil
	}); err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("tunnels/wg-a.conf")
	if err != nil {
		t.Fatalf("create conf entry: %v", err)
	}
	if _, err := w.Write([]byte(roundTripConf)); err != nil {
		t.Fatalf("write conf entry: %v", err)
	}
	w, err = zw.Create("config.json")
	if err != nil {
		t.Fatalf("create config entry: %v", err)
	}
	// Valid JSON, wrong shape: this is what a truncated or edited export
	// looks like to the parser.
	if _, err := w.Write([]byte(`{"language": [`)); err != nil {
		t.Fatalf("write config entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	r, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("build reader: %v", err)
	}

	res, err := svc.importSettingsReader(r)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.SettingsApplied {
		t.Error("bad config.json reported as applied")
	}
	after, err := svc.settingsStore.Load()
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if after.Language != "ja" {
		t.Errorf("existing settings damaged by a rejected import: language=%q", after.Language)
	}
	// The rejected write must not leave a temp file next to config.json.
	leftovers, err := filepath.Glob(filepath.Join(paths.ConfigDir, ".wireguide-*.tmp"))
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}
	if len(leftovers) != 0 {
		t.Errorf("temp files left behind: %v", leftovers)
	}
	if _, err := svc.tunnelStore.Load("wg-a"); err != nil {
		t.Errorf("a bad config.json must not stop the tunnels importing: %v", err)
	}
}

// TestImportSettingsReaderToleratesOrphanSidecar: packages exported before
// sidecars were part of them, and packages whose .conf failed to import,
// must not fail the restore over policy that has nowhere to go.
func TestImportSettingsReaderToleratesOrphanSidecar(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("tunnels/gone.meta.json")
	if err != nil {
		t.Fatalf("create entry: %v", err)
	}
	meta, err := json.Marshal(storage.TunnelMeta{TrafficProtect: true})
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}
	if _, err := w.Write(meta); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	w, err = zw.Create("tunnels/wg-b.conf")
	if err != nil {
		t.Fatalf("create conf entry: %v", err)
	}
	if _, err := w.Write([]byte(roundTripConf)); err != nil {
		t.Fatalf("write conf entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	r, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("build reader: %v", err)
	}

	useHome(t, t.TempDir())
	svc, _ := storeFor(t)
	res, err := svc.importSettingsReader(r)
	if err != nil {
		t.Fatalf("import with an orphan sidecar: %v", err)
	}
	if len(res.Tunnels) != 1 || res.Tunnels[0].Error != "" {
		t.Errorf("unexpected tunnel results: %+v", res.Tunnels)
	}
	if m := metaOf(t, svc, "wg-b"); m.TrafficProtect {
		t.Error("orphan sidecar written onto an unrelated tunnel")
	}
}
