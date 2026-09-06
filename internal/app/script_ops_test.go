package app

import (
	"strings"
	"testing"
)

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
